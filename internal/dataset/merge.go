package dataset

import (
	"sort"

	"goodreads/internal/identity"
	"goodreads/internal/model"

	"github.com/dchooyc/book"
)

// MergeReport is the sidecar audit trail for a merge. Provenance lives here
// so the public books schema stays unchanged.
type MergeReport struct {
	Summary                  MergeSummary        `json:"summary"`
	KeptFromPreviousSnapshot []string            `json:"kept_from_previous_snapshot"`
	ReplacedByRecovered      []string            `json:"replaced_by_recovered"`
	RecoveredNewWorks        []string            `json:"recovered_new_works"`
	Aliases                  map[string][]string `json:"aliases"`
	BlankIDQuarantined       []book.Book         `json:"blank_id_quarantined"`
}

// MergeSummary aggregates merge counts.
type MergeSummary struct {
	NewRawRows             int `json:"new_raw_rows"`
	NewCanonicalBooks      int `json:"new_canonical_books"`
	RecoveredBooks         int `json:"recovered_books"`
	FinalBooks             int `json:"final_books"`
	ReplacedByRecovered    int `json:"replaced_by_recovered"`
	AddedByRecovery        int `json:"added_by_recovery"`
	KeptFromPrevious       int `json:"kept_from_previous_snapshot"`
	BlankIDRowsInNew       int `json:"blank_id_rows_in_new"`
	BlankIDRowsQuarantined int `json:"blank_id_rows_quarantined"`
	DuplicateRowsCollapsed int `json:"duplicate_rows_collapsed"`
}

// recoveredRowWins decides between the new-output row and a recovered row
// for the same work ID: highest ratings, then reviews, then nonblank cover,
// then author/title quality. The recovered row is the later fetch, so it
// wins ties.
func recoveredRowWins(newRow, recovered book.Book) bool {
	if newRow.Ratings != recovered.Ratings {
		return recovered.Ratings > newRow.Ratings
	}
	if newRow.Reviews != recovered.Reviews {
		return recovered.Reviews > newRow.Reviews
	}
	if (newRow.CoverUrl != "") != (recovered.CoverUrl != "") {
		return recovered.CoverUrl != ""
	}
	if len(newRow.Authors) != len(recovered.Authors) {
		return len(recovered.Authors) > len(newRow.Authors)
	}
	if len(newRow.Title) != len(recovered.Title) {
		return len(recovered.Title) > len(newRow.Title)
	}
	return true // latest source wins when otherwise equal
}

// MergeOutputs starts from the new canonical output, layers in recovered
// books by work ID, and quarantines blank-ID rows that match recovered
// works. The merged output never contains duplicate or blank work IDs.
func MergeOutputs(newRows []book.Book, recovery model.RecoveryOutput) (book.Books, MergeReport) {
	newCanonical, aliases := CanonicalizeBooks(newRows)

	keptIDs := map[string]bool{}
	for _, r := range recovery.TargetResults {
		if r.Status == model.StatusKeptFromPrevious {
			keptIDs[r.ExpectedGoodreadsWorkID] = true
		}
	}

	report := MergeReport{
		KeptFromPreviousSnapshot: []string{},
		ReplacedByRecovered:      []string{},
		RecoveredNewWorks:        []string{},
		Aliases:                  aliases,
		BlankIDQuarantined:       []book.Book{},
	}

	merged := make(map[string]book.Book, len(newCanonical))
	for id, row := range newCanonical {
		merged[id] = row
	}

	recoveredTitles := map[string]bool{}
	for _, rec := range recovery.Books {
		if rec.ID == "" {
			continue // never admit blank IDs into the canonical set
		}
		recoveredTitles[identity.NormalizeTitle(rec.Title)] = true

		existing, ok := merged[rec.ID]
		if !ok {
			merged[rec.ID] = rec
			if keptIDs[rec.ID] {
				report.KeptFromPreviousSnapshot = append(report.KeptFromPreviousSnapshot, rec.ID)
			} else {
				report.RecoveredNewWorks = append(report.RecoveredNewWorks, rec.ID)
			}
			continue
		}

		if recoveredRowWins(existing, rec) {
			merged[rec.ID] = rec
			report.ReplacedByRecovered = append(report.ReplacedByRecovered, rec.ID)
		}
	}

	// Quarantine blank-ID rows from the new raw output that correspond to
	// recovered works, so they are visible instead of silently dropped.
	blankRows := 0
	for _, row := range newRows {
		if row.ID != "" {
			continue
		}
		blankRows++
		if recoveredTitles[identity.NormalizeTitle(row.Title)] {
			report.BlankIDQuarantined = append(report.BlankIDQuarantined, row)
		}
	}

	sort.Strings(report.KeptFromPreviousSnapshot)
	sort.Strings(report.ReplacedByRecovered)
	sort.Strings(report.RecoveredNewWorks)

	final := make([]book.Book, 0, len(merged))
	for _, row := range merged {
		final = append(final, row)
	}
	sort.Slice(final, func(i, j int) bool {
		if final[i].Ratings != final[j].Ratings {
			return final[i].Ratings > final[j].Ratings
		}
		return final[i].ID < final[j].ID
	})

	report.Summary = MergeSummary{
		NewRawRows:             len(newRows),
		NewCanonicalBooks:      len(newCanonical),
		RecoveredBooks:         len(recovery.Books),
		FinalBooks:             len(final),
		ReplacedByRecovered:    len(report.ReplacedByRecovered),
		AddedByRecovery:        len(report.RecoveredNewWorks) + len(report.KeptFromPreviousSnapshot),
		KeptFromPrevious:       len(report.KeptFromPreviousSnapshot),
		BlankIDRowsInNew:       blankRows,
		BlankIDRowsQuarantined: len(report.BlankIDQuarantined),
		DuplicateRowsCollapsed: len(newRows) - blankRows - len(newCanonical),
	}

	return book.Books{Books: final}, report
}
