package metrics

import (
	"context"
	"encoding/json"
	"log/slog"

	"github.com/prometheus/client_golang/prometheus"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/reyshazni/fitcheck/internal/diagnosis"
)

// PendingPodCollector implements prometheus.Collector. It computes
// fitcheck_pending_pod_count gauge values at scrape time by listing
// Pending pods and parsing their diagnosis annotations.
type PendingPodCollector struct {
	reader client.Reader
	desc   *prometheus.Desc
}

type aggregationKey struct {
	Namespace string
	OwnerKind string
	OwnerName string
	Verdict   string
	Category  string
}

type podSummary struct {
	Verdict  string
	Category string
}

const (
	metricHelp    = "Number of pending pods grouped by owner and scheduling verdict"
	pendingMetric = "fitcheck_pending_pod_count"

	verdictAccepted     = "accepted"
	verdictCandidate    = "candidate"
	verdictInitializing = "initializing"
	verdictNoStock      = "no-stock"
	verdictRejected     = "rejected"

	labelCategory  = "category"
	labelNamespace = "namespace"
	labelOwnerKind = "owner_kind"
	labelOwnerName = "owner_name"
	labelVerdict   = "verdict"
)

// NewPendingPodCollector creates a collector that reads pod diagnosis
// annotations via the given client.Reader.
func NewPendingPodCollector(reader client.Reader) *PendingPodCollector {
	return &PendingPodCollector{
		reader: reader,
		desc: prometheus.NewDesc(
			pendingMetric,
			metricHelp,
			[]string{labelNamespace, labelOwnerKind, labelOwnerName, labelVerdict, labelCategory},
			nil,
		),
	}
}

// Describe sends the metric descriptor to the channel.
func (c *PendingPodCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- c.desc
}

// Collect computes metric values at scrape time.
func (c *PendingPodCollector) Collect(ch chan<- prometheus.Metric) {
	ctx := context.Background()

	var podList corev1.PodList
	if err := c.reader.List(ctx, &podList); err != nil {
		slog.Warn("failed to list pods for metrics", "error", err)

		return
	}

	counts := c.aggregatePods(ctx, podList.Items)

	for key, count := range counts {
		ch <- prometheus.MustNewConstMetric(
			c.desc,
			prometheus.GaugeValue,
			float64(count),
			key.Namespace, key.OwnerKind, key.OwnerName,
			key.Verdict, key.Category,
		)
	}
}

func (c *PendingPodCollector) aggregatePods(
	ctx context.Context,
	pods []corev1.Pod,
) map[aggregationKey]int {
	counts := make(map[aggregationKey]int)
	cache := make(map[types.UID]ownerInfo)

	for i := range pods {
		if pods[i].Status.Phase != corev1.PodPending {
			continue
		}

		raw, ok := pods[i].Annotations[diagnosis.AnnotationKey]
		if !ok {
			continue
		}

		var report diagnosis.DiagnosisReport
		if err := json.Unmarshal([]byte(raw), &report); err != nil {
			slog.Warn("failed to parse diagnosis annotation", "pod", pods[i].Name, "error", err)

			continue
		}

		summary := bestVerdict(report.Nodepools)
		owner := resolveOwner(ctx, c.reader, &pods[i], cache)

		key := aggregationKey{
			Namespace: pods[i].Namespace,
			OwnerKind: owner.Kind,
			OwnerName: owner.Name,
			Verdict:   summary.Verdict,
			Category:  summary.Category,
		}

		counts[key]++
	}

	return counts
}

// verdictPriority returns a score for verdict ranking.
// Higher is better: accepted > candidate > initializing > no-stock > rejected.
func verdictPriority(verdict string) int {
	switch verdict {
	case verdictAccepted:
		return 5
	case verdictCandidate:
		return 4
	case verdictInitializing:
		return 3
	case verdictNoStock:
		return 2
	case verdictRejected:
		return 1
	default:
		return 0
	}
}

func bestVerdict(nodepools []diagnosis.NodepoolResult) podSummary {
	if len(nodepools) == 0 {
		return podSummary{Verdict: verdictRejected}
	}

	best := nodepools[0]

	for _, np := range nodepools[1:] {
		if verdictPriority(np.Verdict) > verdictPriority(best.Verdict) {
			best = np
		}
	}

	return podSummary{Verdict: best.Verdict, Category: best.Category}
}
