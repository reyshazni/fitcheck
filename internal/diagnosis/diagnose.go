package diagnosis

import (
	"time"

	corev1 "k8s.io/api/core/v1"
)

type nodeCheckResults struct {
	fitting          int
	firstRejection   *Rejection
	startupRejection *Rejection
	hasYoungStartup  bool
}

// DiagnoseNodepool checks whether a pod can be scheduled on any node
// in the given nodepool. It returns Accepted if at least one node fits,
// Initializing if all blocking nodes have startup taints within startupTimeout,
// or Rejected if no node fits.
func DiagnoseNodepool(pod *corev1.Pod, np NodepoolInfo, startupTimeout time.Duration) NodepoolDiagnosis {
	result := NodepoolDiagnosis{
		NodepoolID:   np.ID,
		NodepoolName: np.Name,
		TotalNodes:   len(np.Nodes),
	}

	if len(np.Nodes) == 0 {
		result.Verdict = Rejected
		result.Rejection = &Rejection{Reason: "nodepool has no ready nodes"}

		return result
	}

	requests := aggregateRequests(pod)
	nodeResults := checkAllNodes(pod, np.Nodes, requests, startupTimeout)
	result.FittingNodes = nodeResults.fitting

	switch {
	case result.FittingNodes > 0:
		result.Verdict = Accepted
	case nodeResults.hasYoungStartup:
		result.Verdict = Initializing
		result.Rejection = nodeResults.startupRejection
	default:
		result.Verdict = Rejected
		result.Rejection = nodeResults.firstRejection
	}

	return result
}

func checkAllNodes(
	pod *corev1.Pod,
	nodes []NodeInfo,
	requests corev1.ResourceList,
	startupTimeout time.Duration,
) nodeCheckResults {
	var res nodeCheckResults

	for i := range nodes {
		rejection := checkNode(pod, nodes[i], requests)
		if rejection == nil {
			res.fitting++

			continue
		}

		if res.firstRejection == nil {
			res.firstRejection = rejection
		}

		if rejection.Category == CategoryStartupTaint && time.Since(nodes[i].CreationTimestamp) < startupTimeout {
			res.hasYoungStartup = true

			if res.startupRejection == nil {
				res.startupRejection = rejection
			}
		}
	}

	return res
}

func checkNode(pod *corev1.Pod, node NodeInfo, requests corev1.ResourceList) *Rejection {
	if r := CheckTaints(pod.Spec.Tolerations, node.Taints); r != nil {
		return r
	}

	if r := CheckNodeSelector(pod.Spec.NodeSelector, node.Labels); r != nil {
		return r
	}

	if pod.Spec.Affinity != nil {
		if r := CheckNodeAffinity(pod.Spec.Affinity.NodeAffinity, node.Labels); r != nil {
			return r
		}
	}

	if r := CheckResources(requests, node.Allocatable); r != nil {
		return r
	}

	return nil
}

func aggregateRequests(pod *corev1.Pod) corev1.ResourceList {
	total := corev1.ResourceList{}

	for i := range pod.Spec.Containers {
		addResources(total, pod.Spec.Containers[i].Resources.Requests)
	}

	for i := range pod.Spec.InitContainers {
		maxResources(total, pod.Spec.InitContainers[i].Resources.Requests)
	}

	return total
}

func addResources(total, add corev1.ResourceList) {
	for name, qty := range add {
		if curr, ok := total[name]; ok {
			curr.Add(qty)
			total[name] = curr
		} else {
			total[name] = qty.DeepCopy()
		}
	}
}

func maxResources(total, init corev1.ResourceList) {
	for name, qty := range init {
		if curr, ok := total[name]; ok {
			if qty.Cmp(curr) > 0 {
				total[name] = qty.DeepCopy()
			}
		} else {
			total[name] = qty.DeepCopy()
		}
	}
}
