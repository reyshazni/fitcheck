package diagnosis

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

type DiagnosisReport struct {
	Timestamp string           `json:"timestamp"`
	Summary   string           `json:"summary"`
	Nodepools []NodepoolResult `json:"nodepools"`
}

type NodepoolResult struct {
	Name     string `json:"name"`
	Verdict  string `json:"verdict"`
	Fitting  int    `json:"fitting,omitempty"`
	Total    int    `json:"total,omitempty"`
	Reason   string `json:"reason,omitempty"`
	Category string `json:"category,omitempty"`
}

const AnnotationKey = "fitcheck.io/diagnosis"

func BuildReport(diagnoses []NodepoolDiagnosis) DiagnosisReport {
	if len(diagnoses) == 0 {
		return DiagnosisReport{}
	}

	report := DiagnosisReport{
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		Nodepools: make([]NodepoolResult, len(diagnoses)),
	}

	for i, d := range diagnoses {
		report.Nodepools[i] = buildNodepoolResult(d)
	}

	report.Summary = FormatEventSummary(diagnoses)

	return report
}

func buildNodepoolResult(d NodepoolDiagnosis) NodepoolResult {
	r := NodepoolResult{
		Name:    d.NodepoolName,
		Verdict: verdictToString(d.Verdict),
	}

	if d.Verdict == Accepted {
		r.Fitting = d.FittingNodes
		r.Total = d.TotalNodes
	} else if d.Rejection != nil {
		r.Reason = d.Rejection.Reason
		r.Category = categoryToString(d.Rejection.Category)
	}

	return r
}

func MarshalReport(report DiagnosisReport) (string, error) {
	data, err := json.Marshal(report)
	if err != nil {
		return "", fmt.Errorf("marshaling diagnosis report: %w", err)
	}

	return string(data), nil
}

func verdictToString(v Verdict) string {
	switch v {
	case Accepted:
		return "accepted"
	case Rejected:
		return "rejected"
	case NoStock:
		return "no-stock"
	case Candidate:
		return "candidate"
	default:
		return strings.ToLower(string(v))
	}
}
