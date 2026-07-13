package diagnosis

import corev1 "k8s.io/api/core/v1"

// DiagnoseAll runs scheduling diagnosis for each nodepool and returns
// the results. One NodepoolDiagnosis is produced per nodepool.
func DiagnoseAll(pod *corev1.Pod, nodepools []NodepoolInfo) []NodepoolDiagnosis {
	results := make([]NodepoolDiagnosis, 0, len(nodepools))

	for _, np := range nodepools {
		results = append(results, DiagnoseNodepool(pod, np))
	}

	return results
}
