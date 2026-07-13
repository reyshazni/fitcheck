package diagnosis

import (
	"fmt"
	"strings"
)

type verdictGroup struct {
	label   string
	entries []string
}

const maxNoteBytes = 1000

func FormatSummary(diagnoses []NodepoolDiagnosis) (normal, warning string) {
	accepted := collectAccepted(diagnoses)
	rejected := collectByVerdict(diagnoses, Rejected, "rejected")
	noStock := collectByVerdict(diagnoses, NoStock, "no-stock")
	candidate := collectByVerdict(diagnoses, Candidate, "candidate")

	normal = formatGroup(accepted)
	if len(normal) > maxNoteBytes {
		normal = truncateResult(normal)
	}

	warning = joinWarningGroups(rejected, noStock, candidate)

	return normal, warning
}

func collectAccepted(diagnoses []NodepoolDiagnosis) verdictGroup {
	g := verdictGroup{label: "accepted"}

	for i := range diagnoses {
		if diagnoses[i].Verdict == Accepted {
			entry := fmt.Sprintf("%s(%d/%d)",
				diagnoses[i].NodepoolName,
				diagnoses[i].FittingNodes,
				diagnoses[i].TotalNodes,
			)
			g.entries = append(g.entries, entry)
		}
	}

	return g
}

func collectByVerdict(
	diagnoses []NodepoolDiagnosis, v Verdict, label string,
) verdictGroup {
	g := verdictGroup{label: label}

	for i := range diagnoses {
		if diagnoses[i].Verdict == v {
			entry := fmt.Sprintf("%s: %s",
				diagnoses[i].NodepoolName,
				diagnoses[i].Rejection.Reason,
			)
			g.entries = append(g.entries, entry)
		}
	}

	return g
}

func formatGroup(g verdictGroup) string {
	if len(g.entries) == 0 {
		return ""
	}

	return fmt.Sprintf("[%s] %s", g.label, strings.Join(g.entries, ", "))
}

func joinWarningGroups(groups ...verdictGroup) string {
	var lines []string

	for _, g := range groups {
		if line := formatGroup(g); line != "" {
			lines = append(lines, line)
		}
	}

	result := strings.Join(lines, "\n")

	if len(result) <= maxNoteBytes {
		return result
	}

	return truncateWarning(lines)
}

func truncateWarning(lines []string) string {
	for {
		total := totalLen(lines)
		if total <= maxNoteBytes {
			return strings.Join(lines, "\n")
		}

		longest, longestLen := 0, 0

		for i, line := range lines {
			if len(line) > longestLen {
				longest = i
				longestLen = len(line)
			}
		}

		shortened := truncateLine(lines[longest])
		if shortened == lines[longest] {
			result := strings.Join(lines, "\n")
			if len(result) > maxNoteBytes {
				return result[:maxNoteBytes]
			}

			return result
		}

		lines[longest] = shortened
	}
}

func truncateResult(line string) string {
	for len(line) > maxNoteBytes {
		shortened := truncateLine(line)
		if shortened == line {
			return line[:maxNoteBytes]
		}

		line = shortened
	}

	return line
}

func truncateLine(line string) string {
	prefixEnd := strings.Index(line, "] ")
	if prefixEnd < 0 {
		return line
	}

	prefix := line[:prefixEnd+2]
	body := line[prefixEnd+2:]

	// Extract existing "... +N more" suffix if present
	dropped := 0
	if idx := strings.LastIndex(body, ", ... +"); idx >= 0 {
		suffix := body[idx+len(", ... +"):]
		if n, err := fmt.Sscanf(suffix, "%d more", &dropped); n == 1 && err == nil {
			body = body[:idx]
		}
	}

	parts := strings.Split(body, ", ")
	if len(parts) <= 1 {
		return line
	}

	parts = parts[:len(parts)-1]
	dropped++

	return fmt.Sprintf("%s%s, ... +%d more",
		prefix, strings.Join(parts, ", "), dropped,
	)
}

func totalLen(lines []string) int {
	n := 0

	for i, line := range lines {
		n += len(line)
		if i > 0 {
			n++
		}
	}

	return n
}
