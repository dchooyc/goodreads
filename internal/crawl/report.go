package crawl

import (
	"fmt"
	"sort"
	"strings"
)

// BuildRecoveryOutput merges target results (new results override earlier
// ones for the same work ID, so resumed runs stay consistent) and computes
// the summary. Books are sorted by ratings descending.
func BuildRecoveryOutput(targetsRead int, previous []TargetResult, current []TargetResult) RecoveryOutput {
	byID := make(map[string]TargetResult)
	order := []string{}
	for _, r := range append(append([]TargetResult{}, previous...), current...) {
		if _, seen := byID[r.ExpectedGoodreadsWorkID]; !seen {
			order = append(order, r.ExpectedGoodreadsWorkID)
		}
		byID[r.ExpectedGoodreadsWorkID] = r
	}

	out := RecoveryOutput{
		Summary:       RecoverySummary{TargetsRead: targetsRead},
		Books:         nil,
		TargetResults: make([]TargetResult, 0, len(order)),
	}

	for _, id := range order {
		r := byID[id]
		out.TargetResults = append(out.TargetResults, r)

		switch r.Status {
		case StatusRecovered:
			out.Summary.Recovered++
		case StatusKeptFromPrevious:
			out.Summary.KeptFromPreviousSnapshot++
		case StatusManualReview:
			out.Summary.NeedsManualReview++
		case StatusHTTPFailed, StatusParseFailed, StatusBlocked:
			out.Summary.Failed++
		}

		if r.FinalBook != nil && (r.Status == StatusRecovered || r.Status == StatusKeptFromPrevious) {
			out.Books = append(out.Books, *r.FinalBook)
		}
	}

	out.Summary.StillMissing = out.Summary.TargetsRead - out.Summary.Recovered - out.Summary.KeptFromPreviousSnapshot

	sort.Slice(out.Books, func(i, j int) bool {
		if out.Books[i].Ratings != out.Books[j].Ratings {
			return out.Books[i].Ratings > out.Books[j].Ratings
		}
		return out.Books[i].ID < out.Books[j].ID
	})

	return out
}

// RenderRecoveryMarkdown produces recovery-report.md.
func RenderRecoveryMarkdown(out RecoveryOutput) string {
	var b strings.Builder
	s := out.Summary

	b.WriteString("# Recovery Report\n\n")
	b.WriteString("## Summary\n\n")
	fmt.Fprintf(&b, "| Metric | Count |\n|---|---|\n")
	fmt.Fprintf(&b, "| Targets read | %d |\n", s.TargetsRead)
	fmt.Fprintf(&b, "| Recovered | %d |\n", s.Recovered)
	fmt.Fprintf(&b, "| Kept from previous snapshot | %d |\n", s.KeptFromPreviousSnapshot)
	fmt.Fprintf(&b, "| Needs manual review | %d |\n", s.NeedsManualReview)
	fmt.Fprintf(&b, "| Failed | %d |\n", s.Failed)
	fmt.Fprintf(&b, "| Still missing | %d |\n\n", s.StillMissing)

	byStatus := make(map[string][]TargetResult)
	for _, r := range out.TargetResults {
		byStatus[r.Status] = append(byStatus[r.Status], r)
	}

	statusOrder := []string{
		StatusManualReview, StatusBlocked, StatusHTTPFailed, StatusParseFailed,
		StatusBelowThreshold, StatusKeptFromPrevious, StatusRecovered,
	}

	for _, status := range statusOrder {
		results := byStatus[status]
		if len(results) == 0 {
			continue
		}
		fmt.Fprintf(&b, "## %s (%d)\n\n", status, len(results))
		for _, r := range results {
			fmt.Fprintf(&b, "- **%s** (`%s`, %s)", r.Title, r.ExpectedGoodreadsWorkID, r.Priority)
			if len(r.Warnings) > 0 {
				fmt.Fprintf(&b, " — %s", r.Warnings[len(r.Warnings)-1])
			} else if len(r.Errors) > 0 {
				fmt.Fprintf(&b, " — %s", r.Errors[len(r.Errors)-1])
			}
			b.WriteString("\n")
		}
		b.WriteString("\n")
	}

	return b.String()
}
