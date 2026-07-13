package diagnosis

import (
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/klog/v2"
)

// CheckTaints checks whether the given tolerations can tolerate all
// blocking taints (NoSchedule and NoExecute). Returns nil if all taints
// are tolerated, or a Rejection for the first untolerated taint.
func CheckTaints(tolerations []corev1.Toleration, taints []corev1.Taint) *Rejection {
	for i := range taints {
		if taints[i].Effect == corev1.TaintEffectPreferNoSchedule {
			continue
		}

		if !isTaintTolerated(taints[i], tolerations) {
			return &Rejection{
				Category: CategoryTaint,
				Reason:   formatTaintReason(taints[i]),
			}
		}
	}

	return nil
}

func isTaintTolerated(taint corev1.Taint, tolerations []corev1.Toleration) bool {
	logger := klog.Background()

	for i := range tolerations {
		if tolerations[i].ToleratesTaint(logger, &taint, false) {
			return true
		}
	}

	return false
}

func formatTaintReason(taint corev1.Taint) string {
	if taint.Value == "" {
		return fmt.Sprintf("taint %s:%s not tolerated", taint.Key, taint.Effect)
	}

	return fmt.Sprintf("taint %s=%s:%s not tolerated", taint.Key, taint.Value, taint.Effect)
}
