package diagnosis

import (
	"strconv"

	corev1 "k8s.io/api/core/v1"
)

// CheckNodeAffinity checks whether node labels satisfy the pod's required
// node affinity. Only RequiredDuringSchedulingIgnoredDuringExecution is
// evaluated. Returns nil if affinity is nil or satisfied.
func CheckNodeAffinity(affinity *corev1.NodeAffinity, nodeLabels map[string]string) *Rejection {
	if affinity == nil {
		return nil
	}

	required := affinity.RequiredDuringSchedulingIgnoredDuringExecution
	if required == nil {
		return nil
	}

	for _, term := range required.NodeSelectorTerms {
		if matchesTerm(term, nodeLabels) {
			return nil
		}
	}

	return &Rejection{
		Category: CategoryAffinity,
		Reason:   "node affinity not satisfied",
	}
}

func matchesTerm(term corev1.NodeSelectorTerm, labels map[string]string) bool {
	for _, expr := range term.MatchExpressions {
		if !matchesExpression(expr, labels) {
			return false
		}
	}

	return true
}

func matchesExpression(expr corev1.NodeSelectorRequirement, labels map[string]string) bool {
	val, exists := labels[expr.Key]

	switch expr.Operator {
	case corev1.NodeSelectorOpIn:
		return exists && containsString(expr.Values, val)
	case corev1.NodeSelectorOpNotIn:
		return !exists || !containsString(expr.Values, val)
	case corev1.NodeSelectorOpExists:
		return exists
	case corev1.NodeSelectorOpDoesNotExist:
		return !exists
	case corev1.NodeSelectorOpGt:
		return exists && compareNumeric(val, expr.Values[0]) > 0
	case corev1.NodeSelectorOpLt:
		return exists && compareNumeric(val, expr.Values[0]) < 0
	default:
		return false
	}
}

func containsString(slice []string, s string) bool {
	for _, v := range slice {
		if v == s {
			return true
		}
	}

	return false
}

func compareNumeric(a, b string) int {
	ai, errA := strconv.ParseInt(a, 10, 64)
	bi, errB := strconv.ParseInt(b, 10, 64)

	if errA != nil || errB != nil {
		return 0
	}

	switch {
	case ai > bi:
		return 1
	case ai < bi:
		return -1
	default:
		return 0
	}
}
