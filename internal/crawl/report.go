package crawl

import (
	"sort"
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
