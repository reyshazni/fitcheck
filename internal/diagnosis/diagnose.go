package diagnosis

import corev1 "k8s.io/api/core/v1"

// DiagnoseNodepool checks whether a pod can be scheduled on any node
// in the given nodepool. It returns Accepted if at least one node fits,
// or Rejected if no node fits.
func DiagnoseNodepool(pod *corev1.Pod, np NodepoolInfo) NodepoolDiagnosis {
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
	var firstRejection *Rejection

	for i := range np.Nodes {
		rejection := checkNode(pod, np.Nodes[i], requests)
		if rejection == nil {
			result.FittingNodes++
		} else if firstRejection == nil {
			firstRejection = rejection
		}
	}

	if result.FittingNodes > 0 {
		result.Verdict = Accepted
	} else {
		result.Verdict = Rejected
		result.Rejection = firstRejection
	}

	return result
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
