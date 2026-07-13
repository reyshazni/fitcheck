package diagnosis_test

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"

	"github.com/reyshazni/fitcheck/internal/diagnosis"
)

func TestCheckResources_Fits(t *testing.T) {
	requests := corev1.ResourceList{
		corev1.ResourceCPU:    resource.MustParse("2"),
		corev1.ResourceMemory: resource.MustParse("4Gi"),
	}
	allocatable := corev1.ResourceList{
		corev1.ResourceCPU:    resource.MustParse("4"),
		corev1.ResourceMemory: resource.MustParse("8Gi"),
	}

	got := diagnosis.CheckResources(requests, allocatable)
	if got != nil {
		t.Errorf("CheckResources() = %v, want nil", got)
	}
}

func TestCheckResources_InsufficientCPU(t *testing.T) {
	requests := corev1.ResourceList{
		corev1.ResourceCPU: resource.MustParse("8"),
	}
	allocatable := corev1.ResourceList{
		corev1.ResourceCPU: resource.MustParse("4"),
	}

	got := diagnosis.CheckResources(requests, allocatable)
	if got == nil {
		t.Fatal("CheckResources() = nil, want Rejection")
	}

	if got.Category != diagnosis.CategoryResources {
		t.Errorf("Category = %d, want %d", got.Category, diagnosis.CategoryResources)
	}
}

func TestCheckResources_InsufficientMemory(t *testing.T) {
	requests := corev1.ResourceList{
		corev1.ResourceMemory: resource.MustParse("16Gi"),
	}
	allocatable := corev1.ResourceList{
		corev1.ResourceMemory: resource.MustParse("8Gi"),
	}

	got := diagnosis.CheckResources(requests, allocatable)
	if got == nil {
		t.Fatal("CheckResources() = nil, want Rejection")
	}

	if got.Category != diagnosis.CategoryResources {
		t.Errorf("Category = %d, want %d", got.Category, diagnosis.CategoryResources)
	}
}

func TestCheckResources_GPURequestedNotAvailable(t *testing.T) {
	gpuResource := corev1.ResourceName("nvidia.com/gpu")
	requests := corev1.ResourceList{
		gpuResource: resource.MustParse("1"),
	}
	allocatable := corev1.ResourceList{
		corev1.ResourceCPU:    resource.MustParse("4"),
		corev1.ResourceMemory: resource.MustParse("8Gi"),
	}

	got := diagnosis.CheckResources(requests, allocatable)
	if got == nil {
		t.Fatal("CheckResources() = nil, want Rejection")
	}

	if got.Category != diagnosis.CategoryResources {
		t.Errorf("Category = %d, want %d", got.Category, diagnosis.CategoryResources)
	}
}

func TestCheckResources_GPURequestedAndAvailable(t *testing.T) {
	gpuResource := corev1.ResourceName("nvidia.com/gpu")
	requests := corev1.ResourceList{
		gpuResource: resource.MustParse("1"),
	}
	allocatable := corev1.ResourceList{
		gpuResource: resource.MustParse("2"),
	}

	got := diagnosis.CheckResources(requests, allocatable)
	if got != nil {
		t.Errorf("CheckResources() = %v, want nil", got)
	}
}

func TestCheckResources_EmptyRequests(t *testing.T) {
	got := diagnosis.CheckResources(nil, corev1.ResourceList{
		corev1.ResourceCPU: resource.MustParse("4"),
	})
	if got != nil {
		t.Errorf("CheckResources() = %v, want nil", got)
	}
}
