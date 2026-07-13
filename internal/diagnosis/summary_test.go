package diagnosis_test

import (
	"testing"

	"github.com/reyshazni/fitcheck/internal/diagnosis"
)

const (
	summaryPoolAlpha   = "alpha"
	summaryPoolBravo   = "bravo"
	summaryPoolCharlie = "charlie"
	summaryPoolDelta   = "delta"
	summaryPoolEcho    = "echo"
	summaryPoolFoxtrot = "foxtrot"
	summaryPoolGolf    = "golf"
)

func TestFormatSummary_AcceptedOnly(t *testing.T) {
	diagnoses := []diagnosis.NodepoolDiagnosis{
		{
			NodepoolName: summaryPoolAlpha,
			Verdict:      diagnosis.Accepted,
			FittingNodes: 2,
			TotalNodes:   3,
		},
		{
			NodepoolName: summaryPoolBravo,
			Verdict:      diagnosis.Accepted,
			FittingNodes: 1,
			TotalNodes:   1,
		},
		{
			NodepoolName: summaryPoolCharlie,
			Verdict:      diagnosis.Accepted,
			FittingNodes: 5,
			TotalNodes:   5,
		},
	}

	normal, warning := diagnosis.FormatSummary(diagnoses)

	wantNormal := "[accepted] alpha(2/3), bravo(1/1), charlie(5/5)"
	if normal != wantNormal {
		t.Errorf("normal = %q, want %q", normal, wantNormal)
	}

	if warning != "" {
		t.Errorf("warning = %q, want empty", warning)
	}
}
