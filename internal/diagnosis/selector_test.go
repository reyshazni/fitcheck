package diagnosis_test

import (
	"testing"

	"github.com/reyshazni/fitcheck/internal/diagnosis"
)

func TestCheckNodeSelector_AllMatch(t *testing.T) {
	selector := map[string]string{testValueGPU: testValueTrue, testLabelZone: testZoneEast1a}
	labels := map[string]string{testValueGPU: testValueTrue, testLabelZone: testZoneEast1a, "os": "linux"}

	got := diagnosis.CheckNodeSelector(selector, labels)
	if got != nil {
		t.Errorf("CheckNodeSelector() = %v, want nil", got)
	}
}

func TestCheckNodeSelector_MissingLabel(t *testing.T) {
	selector := map[string]string{testValueGPU: testValueTrue}
	labels := map[string]string{testLabelZone: testZoneEast1a}

	got := diagnosis.CheckNodeSelector(selector, labels)
	if got == nil {
		t.Fatal("CheckNodeSelector() = nil, want Rejection")
	}

	if got.Category != diagnosis.CategoryNodeSelector {
		t.Errorf("Category = %d, want %d", got.Category, diagnosis.CategoryNodeSelector)
	}
}

func TestCheckNodeSelector_WrongValue(t *testing.T) {
	selector := map[string]string{testValueGPU: testValueTrue}
	labels := map[string]string{testValueGPU: "false"}

	got := diagnosis.CheckNodeSelector(selector, labels)
	if got == nil {
		t.Fatal("CheckNodeSelector() = nil, want Rejection")
	}

	if got.Category != diagnosis.CategoryNodeSelector {
		t.Errorf("Category = %d, want %d", got.Category, diagnosis.CategoryNodeSelector)
	}
}

func TestCheckNodeSelector_Empty(t *testing.T) {
	got := diagnosis.CheckNodeSelector(nil, map[string]string{testValueGPU: testValueTrue})
	if got != nil {
		t.Errorf("CheckNodeSelector() = %v, want nil", got)
	}
}

func TestCheckNodeSelector_MultipleOneFails(t *testing.T) {
	selector := map[string]string{testValueGPU: testValueTrue, testLabelZone: "us-west-2"}
	labels := map[string]string{testValueGPU: testValueTrue, testLabelZone: "us-east-1"}

	got := diagnosis.CheckNodeSelector(selector, labels)
	if got == nil {
		t.Fatal("CheckNodeSelector() = nil, want Rejection")
	}

	if got.Category != diagnosis.CategoryNodeSelector {
		t.Errorf("Category = %d, want %d", got.Category, diagnosis.CategoryNodeSelector)
	}
}
