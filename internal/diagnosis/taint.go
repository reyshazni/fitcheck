package diagnosis

import (
	"fmt"
	"strings"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/klog/v2"
)

var startupTaintKeys = map[string]bool{
	"node.kubernetes.io/not-ready":           true,
	"node.kubernetes.io/unreachable":         true,
	"node.kubernetes.io/network-unavailable": true,
}

// IsStartupTaint returns true if the taint key is a Kubernetes node
// lifecycle taint applied during node initialization.
func IsStartupTaint(key string) bool {
	return startupTaintKeys[key]
}

// CheckTaints checks whether the given tolerations can tolerate all
// blocking taints (NoSchedule and NoExecute). Returns nil if all taints
// are tolerated. Permanent untolerated taints take priority over startup
// taints; if only startup taints are untolerated, returns CategoryStartupTaint.
func CheckTaints(tolerations []corev1.Toleration, taints []corev1.Taint) *Rejection {
	var startupRejection *Rejection

	for i := range taints {
		if taints[i].Effect == corev1.TaintEffectPreferNoSchedule {
			continue
		}

		if isTaintTolerated(taints[i], tolerations) {
			continue
		}

		if IsStartupTaint(taints[i].Key) {
			if startupRejection == nil {
				startupRejection = &Rejection{
					Category: CategoryStartupTaint,
					Reason:   formatStartupTaintReason(taints[i]),
				}
			}

			continue
		}

		return &Rejection{
			Category: CategoryTaint,
			Reason:   formatTaintReason(taints[i]),
		}
	}

	return startupRejection
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

func formatStartupTaintReason(taint corev1.Taint) string {
	suffix := taint.Key
	if idx := strings.LastIndex(taint.Key, "/"); idx >= 0 {
		suffix = taint.Key[idx+1:]
	}

	return fmt.Sprintf("node initializing (%s), may resolve on its own", suffix)
}
