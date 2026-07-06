package dataset

import (
	"fmt"
	"sort"
	"strings"

	"goodreads/internal/identity"
	"goodreads/internal/model"

	"github.com/dchooyc/book"
)

// QAMetrics are the crawl quality measurements for one candidate output
// compared against the previous accepted output.
type QAMetrics struct {
	RawRows               int            `json:"raw_rows"`
	CanonicalNonblankIDs  int            `json:"canonical_nonblank_ids"`
	BlankIDRows           int            `json:"blank_id_rows"`
	BlankTitleRows        int            `json:"blank_title_rows"`
	NullGenreRows         int            `json:"null_genre_rows"`
	DuplicateExtraRows    int            `json:"duplicate_extra_rows"`
	BlankIDRate           float64        `json:"parser_blank_id_rate"`
	PreviousRawRows       int            `json:"previous_raw_rows"`
	PreviousCanonicalIDs  int            `json:"previous_canonical_ids"`
	PreviousBlankIDRate   float64        `json:"previous_blank_id_rate"`
	OldIDsMissingFromNew  int            `json:"old_ids_missing_from_new"`
	P0MissingCount        int            `json:"p0_missing_count"`
	P1MissingCount        int            `json:"p1_missing_count"`
	P0MissingUndecided    int            `json:"p0_missing_without_recovery_decision"`
	Top1000Disappeared    int            `json:"previous_top_1000_disappeared"`
	SameTitleBlankIDRows  int            `json:"same_title_blank_id_rows"`
	Top100MissingTitles   []string       `json:"top_100_missing_titles_by_ratings"`
	FetchFailuresByStatus map[string]int `json:"fetch_failures_by_status,omitempty"`
}

// QAReport is written as crawl-quality-report.json.
type QAReport struct {
	Metrics  QAMetrics `json:"metrics"`
	Failures []string  `json:"failures"`
	Passed   bool      `json:"passed"`
}

// decisionStatuses are recovery outcomes that count as an explicit decision
// for a missing work.
var decisionStatuses = map[string]bool{
	model.StatusRecovered:        true,
	model.StatusKeptFromPrevious: true,
	model.StatusBelowThreshold:   true,
	model.StatusManualReview:     true,
}

// RunQA computes crawl quality metrics for newRows (the output being
// accepted) against oldRows (the previous accepted output), and applies the
// hard gates. recovery may be nil when no recovery run happened.
func RunQA(oldRows, newRows []book.Book, recovery *model.RecoveryOutput) QAReport {
	oldCanonical, _ := CanonicalizeBooks(oldRows)
	newCanonical, _ := CanonicalizeBooks(newRows)

	m := QAMetrics{
		RawRows:              len(newRows),
		CanonicalNonblankIDs: len(newCanonical),
		PreviousRawRows:      len(oldRows),
		PreviousCanonicalIDs: len(oldCanonical),
	}

	newTitles := make(map[string]bool)
	for _, row := range newRows {
		if row.ID == "" {
			m.BlankIDRows++
		}
		if strings.TrimSpace(row.Title) == "" {
			m.BlankTitleRows++
		}
		if row.Genres == nil {
			m.NullGenreRows++
		}
		if t := identity.NormalizeTitle(row.Title); t != "" {
			if row.ID == "" && newTitles[t] {
				m.SameTitleBlankIDRows++
			}
			newTitles[t] = true
		}
	}
	m.DuplicateExtraRows = len(newRows) - m.BlankIDRows - len(newCanonical)

	if len(newRows) > 0 {
		m.BlankIDRate = float64(m.BlankIDRows) / float64(len(newRows))
	}
	oldBlankRows := 0
	for _, row := range oldRows {
		if row.ID == "" {
			oldBlankRows++
		}
	}
	if len(oldRows) > 0 {
		m.PreviousBlankIDRate = float64(oldBlankRows) / float64(len(oldRows))
	}

	// Recovery decisions by work ID.
	decided := map[string]bool{}
	if recovery != nil {
		for _, r := range recovery.TargetResults {
			if decisionStatuses[r.Status] {
				decided[r.ExpectedGoodreadsWorkID] = true
			}
		}
		m.FetchFailuresByStatus = map[string]int{}
		for _, r := range recovery.TargetResults {
			if r.Status == model.StatusHTTPFailed || r.Status == model.StatusBlocked {
				m.FetchFailuresByStatus[r.Status]++
			}
		}
	}

	// Missing analysis.
	type missingWork struct {
		id      string
		title   string
		ratings int
	}
	var missing []missingWork
	for id, old := range oldCanonical {
		if _, ok := newCanonical[id]; ok {
			continue
		}
		missing = append(missing, missingWork{id: id, title: old.Title, ratings: old.Ratings})
		switch PriorityFor(old.Ratings, old.Reviews) {
		case "P0":
			m.P0MissingCount++
			if !decided[id] {
				m.P0MissingUndecided++
			}
		case "P1":
			m.P1MissingCount++
		}
	}
	m.OldIDsMissingFromNew = len(missing)

	sort.Slice(missing, func(i, j int) bool {
		if missing[i].ratings != missing[j].ratings {
			return missing[i].ratings > missing[j].ratings
		}
		return missing[i].id < missing[j].id
	})
	for i := 0; i < len(missing) && i < 100; i++ {
		m.Top100MissingTitles = append(m.Top100MissingTitles,
			fmt.Sprintf("%s (%s, %d ratings)", missing[i].title, missing[i].id, missing[i].ratings))
	}

	// Previous top-1000 disappearance.
	oldByRatings := make([]book.Book, 0, len(oldCanonical))
	for _, b := range oldCanonical {
		oldByRatings = append(oldByRatings, b)
	}
	sort.Slice(oldByRatings, func(i, j int) bool {
		if oldByRatings[i].Ratings != oldByRatings[j].Ratings {
			return oldByRatings[i].Ratings > oldByRatings[j].Ratings
		}
		return oldByRatings[i].ID < oldByRatings[j].ID
	})
	top := oldByRatings
	if len(top) > 1000 {
		top = top[:1000]
	}
	for _, b := range top {
		if _, ok := newCanonical[b.ID]; !ok {
			m.Top1000Disappeared++
		}
	}

	// Gates.
	var failures []string
	if m.P0MissingUndecided > 0 {
		failures = append(failures, fmt.Sprintf(
			"%d P0 works are missing without a recovery decision", m.P0MissingUndecided))
	}
	if len(oldRows) > 0 && m.BlankIDRate > m.PreviousBlankIDRate+0.0025 {
		failures = append(failures, fmt.Sprintf(
			"blank ID rate %.2f%% exceeds previous %.2f%% by more than 0.25 percentage points",
			m.BlankIDRate*100, m.PreviousBlankIDRate*100))
	}
	if len(oldCanonical) > 0 && float64(m.CanonicalNonblankIDs) < float64(m.PreviousCanonicalIDs)*0.99 {
		failures = append(failures, fmt.Sprintf(
			"canonical ID count dropped more than 1%%: %d -> %d",
			m.PreviousCanonicalIDs, m.CanonicalNonblankIDs))
	}
	if m.Top1000Disappeared > 10 {
		failures = append(failures, fmt.Sprintf(
			"%d of the previous top 1000 books disappeared (limit 10)", m.Top1000Disappeared))
	}

	return QAReport{Metrics: m, Failures: failures, Passed: len(failures) == 0}
}
