package controller

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/client-go/tools/events"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/reyshazni/fitcheck/internal/autoscaler"
	"github.com/reyshazni/fitcheck/internal/diagnosis"
	"github.com/reyshazni/fitcheck/internal/nodepool"
	"github.com/reyshazni/fitcheck/internal/provider"
)

// PodReconciler watches Pending pods and emits per-nodepool diagnostic events.
type PodReconciler struct {
	Client          client.Client
	Recorder        events.EventRecorder
	Provider        provider.Provider
	RecheckInterval time.Duration
	InitialDelay    time.Duration
	StatusReader    autoscaler.StatusReader
	StartupTimeout  time.Duration
}

// Reconcile handles a single pod reconciliation cycle.
func (r *PodReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	var pod corev1.Pod
	if err := r.Client.Get(ctx, req.NamespacedName, &pod); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}

		return ctrl.Result{}, fmt.Errorf("getting pod: %w", err)
	}

	if pod.Status.Phase != corev1.PodPending {
		r.removeAnnotation(ctx, &pod)
		return ctrl.Result{}, nil
	}

	if remaining := r.remainingDelay(&pod); remaining > 0 {
		return ctrl.Result{RequeueAfter: remaining}, nil
	}

	diagnoses, err := r.diagnose(ctx, &pod)
	if err != nil {
		return ctrl.Result{}, err
	}

	oldSummary := extractOldSummary(&pod)
	r.writeAnnotation(ctx, &pod, diagnoses)
	r.emitEvents(&pod, diagnoses, oldSummary)

	return ctrl.Result{RequeueAfter: r.RecheckInterval}, nil
}

func (r *PodReconciler) remainingDelay(pod *corev1.Pod) time.Duration {
	elapsed := time.Since(pod.CreationTimestamp.Time)
	if elapsed < r.InitialDelay {
		return r.InitialDelay - elapsed
	}

	return 0
}

func (r *PodReconciler) diagnose(ctx context.Context, pod *corev1.Pod) ([]diagnosis.NodepoolDiagnosis, error) {
	collector := nodepool.Collector{}

	pools, err := collector.Collect(ctx, r.Client, r.Provider.NodepoolLabelKey(), r.Provider.NameLabelKey())
	if err != nil {
		return nil, fmt.Errorf("collecting nodepools: %w", err)
	}

	diagnoses := diagnosis.DiagnoseAll(pod, pools, r.StartupTimeout)

	if r.StatusReader != nil {
		diagnoses, err = r.upgradeVerdicts(ctx, pod, pools, diagnoses)
		if err != nil {
			slog.Warn("failed to read autoscaler status", "error", err)
		}
	}

	return diagnoses, nil
}

func (r *PodReconciler) upgradeVerdicts(
	ctx context.Context,
	pod *corev1.Pod,
	pools []diagnosis.NodepoolInfo,
	diagnoses []diagnosis.NodepoolDiagnosis,
) ([]diagnosis.NodepoolDiagnosis, error) {
	ids := make([]string, 0, len(pools))
	for _, p := range pools {
		ids = append(ids, p.ID)
	}

	statuses, err := r.StatusReader.ReadStatus(ctx, r.Client, pod, ids)
	if err != nil {
		return diagnoses, fmt.Errorf("reading autoscaler status: %w", err)
	}

	for i := range diagnoses {
		applyAutoscalerStatus(&diagnoses[i], statuses)
	}

	return diagnoses, nil
}

func applyAutoscalerStatus(d *diagnosis.NodepoolDiagnosis, statuses map[string]autoscaler.AutoscalerStatus) {
	if d.Verdict != diagnosis.Rejected {
		return
	}

	s, ok := statuses[d.NodepoolID]
	if !ok {
		return
	}

	if s.InventoryUnhealthy {
		d.Verdict = diagnosis.NoStock
	} else if s.ScaleUpTriggered {
		d.Verdict = diagnosis.Candidate
	}
}

func (r *PodReconciler) writeAnnotation(
	ctx context.Context,
	pod *corev1.Pod,
	diagnoses []diagnosis.NodepoolDiagnosis,
) {
	report := diagnosis.BuildReport(diagnoses)

	data, err := diagnosis.MarshalReport(report)
	if err != nil {
		slog.Warn("failed to marshal diagnosis report", "error", err, "pod", pod.Name)
		return
	}

	patch := client.MergeFrom(pod.DeepCopy())

	if pod.Annotations == nil {
		pod.Annotations = make(map[string]string)
	}

	pod.Annotations[diagnosis.AnnotationKey] = data

	if err := r.Client.Patch(ctx, pod, patch); err != nil {
		slog.Warn("failed to write diagnosis annotation", "error", err, "pod", pod.Name)
	}
}

func (r *PodReconciler) removeAnnotation(ctx context.Context, pod *corev1.Pod) {
	if _, ok := pod.Annotations[diagnosis.AnnotationKey]; !ok {
		return
	}

	patch := client.MergeFrom(pod.DeepCopy())
	delete(pod.Annotations, diagnosis.AnnotationKey)

	if err := r.Client.Patch(ctx, pod, patch); err != nil {
		slog.Warn("failed to remove diagnosis annotation", "error", err, "pod", pod.Name)
	}
}

func extractOldSummary(pod *corev1.Pod) string {
	raw, ok := pod.Annotations[diagnosis.AnnotationKey]
	if !ok {
		return ""
	}

	var report diagnosis.DiagnosisReport
	if err := json.Unmarshal([]byte(raw), &report); err != nil {
		return ""
	}

	return report.Summary
}

func (r *PodReconciler) emitEvents(pod *corev1.Pod, diagnoses []diagnosis.NodepoolDiagnosis, oldSummary string) {
	summary := diagnosis.FormatEventSummary(diagnoses)
	if summary == "" || summary == oldSummary {
		return
	}

	eventType := corev1.EventTypeWarning

	for _, d := range diagnoses {
		if d.Verdict == diagnosis.Accepted || d.Verdict == diagnosis.Initializing || d.Verdict == diagnosis.Candidate {
			eventType = corev1.EventTypeNormal

			break
		}
	}

	r.Recorder.Eventf(pod, nil, eventType, "FitcheckDiagnosis", "diagnose", summary)
}
