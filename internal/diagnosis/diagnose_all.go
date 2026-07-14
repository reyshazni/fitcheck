package diagnosis

import (
	"time"

	corev1 "k8s.io/api/core/v1"
)

// DiagnoseAll runs scheduling diagnosis for each nodepool and returns
// the results. One NodepoolDiagnosis is produced per nodepool.
// TODO(task8): accept startupTimeout parameter instead of zero value.
func DiagnoseAll(pod *corev1.Pod, nodepools []NodepoolInfo) []NodepoolDiagnosis {
	results := make([]NodepoolDiagnosis, 0, len(nodepools))

	for _, np := range nodepools {
		results = append(results, DiagnoseNodepool(pod, np, 0*time.Second))
	}

	return results
}
