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
// fitcheck_pending_pods gauge values at scrape time by listing Pending
// pods and parsing their diagnosis annotations.
type PendingPodCollector struct {
	reader client.Reader
	desc   *prometheus.Desc
}

type aggregationKey struct {
	Namespace string
	OwnerKind string
	OwnerName string
	Nodepool  string
	Verdict   string
	Category  string
}

const (
	metricHelp    = "Number of pending pods grouped by owner, nodepool, and scheduling verdict"
	pendingMetric = "fitcheck_pending_pods"

	labelNamespace = "namespace"
	labelOwnerKind = "owner_kind"
	labelOwnerName = "owner_name"
	labelNodepool  = "nodepool"
	labelVerdict   = "verdict"
	labelCategory  = "category"
)

// NewPendingPodCollector creates a collector that reads pod diagnosis
// annotations via the given client.Reader.
func NewPendingPodCollector(reader client.Reader) *PendingPodCollector {
	return &PendingPodCollector{
		reader: reader,
		desc: prometheus.NewDesc(
			pendingMetric,
			metricHelp,
			[]string{labelNamespace, labelOwnerKind, labelOwnerName, labelNodepool, labelVerdict, labelCategory},
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
			key.Nodepool, key.Verdict, key.Category,
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

		owner := resolveOwner(ctx, c.reader, &pods[i], cache)
		addReportCounts(counts, pods[i].Namespace, owner, report)
	}

	return counts
}

func addReportCounts(
	counts map[aggregationKey]int,
	namespace string,
	owner ownerInfo,
	report diagnosis.DiagnosisReport,
) {
	for _, np := range report.Nodepools {
		key := aggregationKey{
			Namespace: namespace,
			OwnerKind: owner.Kind,
			OwnerName: owner.Name,
			Nodepool:  np.Name,
			Verdict:   np.Verdict,
			Category:  np.Category,
		}

		counts[key]++
	}
}
