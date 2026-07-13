package diagnosis

import (
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

// checkedResources lists the resource types to evaluate for scheduling fit.
var checkedResources = []corev1.ResourceName{
	corev1.ResourceCPU,
	corev1.ResourceMemory,
	corev1.ResourceName("nvidia.com/gpu"),
}

// CheckResources checks whether a node has enough allocatable resources
// for the pod's requests. Returns nil if all resources fit.
func CheckResources(requests, allocatable corev1.ResourceList) *Rejection {
	for _, name := range checkedResources {
		req, ok := requests[name]
		if !ok || req.IsZero() {
			continue
		}

		alloc, ok := allocatable[name]
		if !ok {
			alloc = resource.Quantity{}
		}

		if req.Cmp(alloc) > 0 {
			return &Rejection{
				Category: CategoryResources,
				Reason: fmt.Sprintf("%s requested %s, allocatable %s",
					name, req.String(), alloc.String()),
			}
		}
	}

	return nil
}
