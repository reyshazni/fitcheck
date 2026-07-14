package diagnosis

import (
	"fmt"
	"sort"
	"strings"
)

func FormatEventSummary(diagnoses []NodepoolDiagnosis) string {
	if len(diagnoses) == 0 {
		return ""
	}

	accepted := countAccepted(diagnoses)
	total := len(diagnoses)

	parts := []string{fmt.Sprintf("%d/%d nodepools fit", accepted, total)}

	if rejectedPart := formatRejectedBreakdown(diagnoses); rejectedPart != "" {
		parts = append(parts, rejectedPart)
	}

	if noStockCount := countByVerdict(diagnoses, NoStock); noStockCount > 0 {
		parts = append(parts, fmt.Sprintf("no-stock: %d", noStockCount))
	}

	if candidateCount := countByVerdict(diagnoses, Candidate); candidateCount > 0 {
		parts = append(parts, fmt.Sprintf("candidate: %d", candidateCount))
	}

	if initCount := countByVerdict(diagnoses, Initializing); initCount > 0 {
		parts = append(parts, fmt.Sprintf("initializing: %d", initCount))
	}

	return strings.Join(parts, " | ")
}

func countAccepted(diagnoses []NodepoolDiagnosis) int {
	n := 0

	for i := range diagnoses {
		if diagnoses[i].Verdict == Accepted {
			n++
		}
	}

	return n
}

func countByVerdict(diagnoses []NodepoolDiagnosis, v Verdict) int {
	n := 0

	for i := range diagnoses {
		if diagnoses[i].Verdict == v {
			n++
		}
	}

	return n
}

func formatRejectedBreakdown(diagnoses []NodepoolDiagnosis) string {
	counts := make(map[RejectionCategory]int)

	for i := range diagnoses {
		if diagnoses[i].Verdict == Rejected && diagnoses[i].Rejection != nil {
			counts[diagnoses[i].Rejection.Category]++
		}
	}

	if len(counts) == 0 {
		return ""
	}

	categories := make([]RejectionCategory, 0, len(counts))
	for c := range counts {
		categories = append(categories, c)
	}

	sort.Slice(categories, func(i, j int) bool {
		return int(categories[i]) < int(categories[j])
	})

	parts := make([]string, 0, len(categories))
	for _, c := range categories {
		parts = append(parts, fmt.Sprintf("%d %s", counts[c], categoryToString(c)))
	}

	return "rejected: " + strings.Join(parts, ", ")
}

func categoryToString(c RejectionCategory) string {
	switch c {
	case CategoryTaint:
		return "taint"
	case CategoryNodeSelector:
		return "selector"
	case CategoryAffinity:
		return "affinity"
	case CategoryResources:
		return "resource"
	case CategoryStartupTaint:
		return verdictInitializing
	default:
		return ""
	}
}
