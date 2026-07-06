package model

import (
	"github.com/dchooyc/book"
)

// MissingTargetsFile is the schema of topreads-missing-books-to-double-check.json.
type MissingTargetsFile struct {
	Summary      MissingSummary   `json:"summary"`
	CheckTargets []RecoveryTarget `json:"check_targets"`
}

// MissingSummary carries comparison metadata. Fields not listed here are
// preserved on read via the Extra map when re-marshalling is not needed.
type MissingSummary struct {
	GeneratedAt                  string         `json:"generated_at,omitempty"`
	OldFile                      string         `json:"old_file,omitempty"`
	NewFile                      string         `json:"new_file,omitempty"`
	ComparisonMethod             string         `json:"comparison_method,omitempty"`
	OldRawRows                   int            `json:"old_raw_rows"`
	NewRawRows                   int            `json:"new_raw_rows"`
	OldCanonicalBooks            int            `json:"old_canonical_books"`
	NewCanonicalBooks            int            `json:"new_canonical_books"`
	SharedCanonicalBooks         int            `json:"shared_canonical_books"`
	MissingCanonicalBooksToCheck int            `json:"missing_canonical_books_to_double_check"`
	NewCanonicalBooksNotInOld    int            `json:"new_canonical_books_not_in_old"`
	MissingPriorityCounts        map[string]int `json:"missing_priority_counts,omitempty"`
	PossibleNewRawMatchCounts    map[string]int `json:"possible_new_raw_match_counts,omitempty"`
	ImportantNote                string         `json:"important_note,omitempty"`
}

// RecoveryTarget is one previously known work that is absent from the new
// canonical output.
type RecoveryTarget struct {
	Priority                string   `json:"priority"`
	ExpectedGoodreadsWorkID string   `json:"expected_goodreads_work_id"`
	Title                   string   `json:"title"`
	Authors                 []string `json:"authors"`
	PrimaryOldURL           string   `json:"primary_old_url"`
	OldRating               float64  `json:"old_rating"`
	OldRatings              int      `json:"old_ratings"`
	OldReviews              int      `json:"old_reviews"`
	OldCoverURL             string   `json:"old_cover_url"`
	OldGenres               []string `json:"old_genres"`
	OldAliasURLs            []string `json:"old_alias_urls"`
	RecoveryStatus          string   `json:"recovery_status"`
}

// OldSnapshotBook reconstructs the previous snapshot row for a target.
func (t RecoveryTarget) OldSnapshotBook() book.Book {
	return book.Book{
		Title:    t.Title,
		URL:      t.PrimaryOldURL,
		ID:       t.ExpectedGoodreadsWorkID,
		CoverUrl: t.OldCoverURL,
		Authors:  t.Authors,
		Genres:   t.OldGenres,
		Rating:   t.OldRating,
		Ratings:  t.OldRatings,
		Reviews:  t.OldReviews,
	}
}

// Target result statuses.
const (
	StatusRecovered        = "recovered"
	StatusKeptFromPrevious = "kept_from_previous_snapshot"
	StatusBelowThreshold   = "below_threshold"
	StatusParseFailed      = "parse_failed"
	StatusHTTPFailed       = "http_failed"
	StatusBlocked          = "blocked"
	StatusManualReview     = "needs_manual_review"
	StatusUpdated          = "updated"
)

// Selected sources.
const (
	SourceFetchedCurrentPage = "fetched_current_page"
	SourcePreviousSnapshot   = "previous_snapshot_fallback"
	SourceSkipped            = "skipped"
)

// TargetResult is the audit record for one recovery target.
type TargetResult struct {
	ExpectedGoodreadsWorkID string     `json:"expected_goodreads_work_id"`
	Title                   string     `json:"title"`
	Authors                 []string   `json:"authors"`
	Priority                string     `json:"priority"`
	AttemptedURLs           []string   `json:"attempted_urls"`
	Status                  string     `json:"status"`
	SelectedSource          string     `json:"selected_source"`
	FinalBook               *book.Book `json:"final_book,omitempty"`
	Errors                  []string   `json:"errors"`
	Warnings                []string   `json:"warnings"`
	StartedAt               string     `json:"started_at"`
	FinishedAt              string     `json:"finished_at"`
}

// RecoverySummary aggregates a recovery run.
type RecoverySummary struct {
	TargetsRead              int `json:"targets_read"`
	Recovered                int `json:"recovered"`
	KeptFromPreviousSnapshot int `json:"kept_from_previous_snapshot"`
	StillMissing             int `json:"still_missing"`
	NeedsManualReview        int `json:"needs_manual_review"`
	Failed                   int `json:"failed"`
}

// RecoveryOutput is the schema of recovered-missing-books.json.
type RecoveryOutput struct {
	Summary       RecoverySummary `json:"summary"`
	Books         []book.Book     `json:"books"`
	TargetResults []TargetResult  `json:"target_results"`
}

// BlankIDCandidate reports a new-output row with useful data but a blank ID
// that plausibly corresponds to a missing old work.
type BlankIDCandidate struct {
	Title                         string   `json:"title"`
	Authors                       []string `json:"authors"`
	URL                           string   `json:"url"`
	Ratings                       int      `json:"ratings"`
	PossibleExpectedWorkIDFromOld string   `json:"possible_expected_work_id_from_old_snapshot"`
	MatchConfidence               string   `json:"match_confidence"`
	Reason                        string   `json:"reason"`
}
