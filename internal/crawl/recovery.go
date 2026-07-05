package crawl

import (
	"errors"
	"fmt"
	"time"

	"github.com/dchooyc/book"
)

// RecoveryConfig wires the recovery runner. Fetcher, ParseBook, MeetsCriteria
// and Now are injectable for tests.
type RecoveryConfig struct {
	Fetcher            Fetcher
	State              *State
	ParseBook          func(body []byte) (*book.Book, error)
	MeetsCriteria      func(b *book.Book) bool
	KeepPreviousOnFail bool
	Force              bool
	Now                func() time.Time
	Logf               func(format string, args ...interface{})
	// OnResult is called after every target so callers can checkpoint the
	// output file incrementally.
	OnResult func(TargetResult)
}

// Runner executes the targeted recovery algorithm, one target at a time.
// Requests are already globally rate-limited by the polite client, so the
// runner is deliberately sequential: gentle, auditable, resumable.
type Runner struct {
	cfg RecoveryConfig
}

// NewRunner validates the config and fills defaults.
func NewRunner(cfg RecoveryConfig) (*Runner, error) {
	if cfg.Fetcher == nil || cfg.State == nil || cfg.ParseBook == nil {
		return nil, errors.New("recovery: Fetcher, State and ParseBook are required")
	}
	if cfg.MeetsCriteria == nil {
		cfg.MeetsCriteria = func(b *book.Book) bool { return true }
	}
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	if cfg.Logf == nil {
		cfg.Logf = func(format string, args ...interface{}) { fmt.Printf(format+"\n", args...) }
	}
	return &Runner{cfg: cfg}, nil
}

// Run processes targets in order. Targets already completed in the state file
// are skipped unless Force is set. Returns the results of this run (skipped
// targets are not re-emitted). A hard block (ErrBlocked) stops the run and is
// returned after in-flight bookkeeping completes.
func (r *Runner) Run(targets []RecoveryTarget) ([]TargetResult, error) {
	results := make([]TargetResult, 0, len(targets))

	for i, target := range targets {
		id := target.ExpectedGoodreadsWorkID
		if !r.cfg.Force && r.cfg.State.IsDone(id) {
			ts, _ := r.cfg.State.Get(id)
			r.cfg.Logf("[%d/%d] skip %s (%s): already %s", i+1, len(targets), id, target.Title, ts.Status)
			continue
		}

		r.cfg.Logf("[%d/%d] %s %s (%s)", i+1, len(targets), target.Priority, id, target.Title)
		result := r.processTarget(target)
		results = append(results, result)

		lastErr := ""
		if len(result.Errors) > 0 {
			lastErr = result.Errors[len(result.Errors)-1]
		}
		if err := r.cfg.State.Update(id, result.Status, result.AttemptedURLs, lastErr); err != nil {
			return results, fmt.Errorf("checkpoint after %s: %w", id, err)
		}
		if r.cfg.OnResult != nil {
			r.cfg.OnResult(result)
		}

		if result.Status == StatusBlocked {
			return results, ErrBlocked
		}
	}

	return results, nil
}

func (r *Runner) processTarget(target RecoveryTarget) TargetResult {
	result := TargetResult{
		ExpectedGoodreadsWorkID: target.ExpectedGoodreadsWorkID,
		Title:                   target.Title,
		Authors:                 target.Authors,
		Priority:                target.Priority,
		AttemptedURLs:           []string{},
		Errors:                  []string{},
		Warnings:                []string{},
		StartedAt:               r.cfg.Now().UTC().Format(time.RFC3339),
	}
	urls := make([]string, 0, 1+len(target.OldAliasURLs))
	if target.PrimaryOldURL != "" {
		urls = append(urls, target.PrimaryOldURL)
	}
	urls = append(urls, target.OldAliasURLs...)

	var manualReviewReason string
	fetchedAnything := false

	for _, url := range urls {
		result.AttemptedURLs = append(result.AttemptedURLs, url)

		body, status, err := r.cfg.Fetcher.Fetch(url)
		if err != nil {
			if errors.Is(err, ErrBlocked) {
				result.Errors = append(result.Errors, err.Error())
				return r.finish(result, StatusBlocked, SourceSkipped, nil)
			}
			result.Errors = append(result.Errors, fmt.Sprintf("fetch %s (status %d): %v", url, status, err))
			continue
		}

		fetchedAnything = true

		parsed, err := r.cfg.ParseBook(body)
		if err != nil || parsed == nil {
			result.Errors = append(result.Errors, fmt.Sprintf("parse %s: %v", url, err))
			continue
		}
		parsed.URL = url

		identity := ValidateRecoveredBook(target, *parsed)
		result.Warnings = append(result.Warnings, identity.Warnings...)

		switch identity.Confidence {
		case ConfidenceHigh, ConfidenceMedium:
			parsed.ID = identity.ResolvedID
			if !r.cfg.MeetsCriteria(parsed) {
				result.Warnings = append(result.Warnings,
					fmt.Sprintf("recovered page for %s no longer meets criteria (rating %.2f, ratings %d)",
						url, parsed.Rating, parsed.Ratings))
				return r.finish(result, StatusBelowThreshold, SourceSkipped, nil)
			}
			return r.finish(result, StatusRecovered, SourceFetchedCurrentPage, parsed)

		case ConfidenceManualReview:
			manualReviewReason = identity.Reason
			result.Warnings = append(result.Warnings, fmt.Sprintf("%s: %s", url, identity.Reason))

		default: // rejected — maybe an alias URL matches instead
			result.Warnings = append(result.Warnings, fmt.Sprintf("%s: %s", url, identity.Reason))
		}
	}

	if manualReviewReason != "" {
		return r.finish(result, StatusManualReview, SourceSkipped, nil)
	}

	if fetchedAnything {
		// Pages fetched but none parsed/validated into the expected work.
		if len(result.Errors) > 0 {
			return r.keepOrFail(result, target, StatusParseFailed)
		}
		return r.finish(result, StatusManualReview, SourceSkipped, nil)
	}

	return r.keepOrFail(result, target, StatusHTTPFailed)
}

// keepOrFail applies the -keep-previous-on-fail policy when the current page
// could not be used.
func (r *Runner) keepOrFail(result TargetResult, target RecoveryTarget, failStatus string) TargetResult {
	if r.cfg.KeepPreviousOnFail {
		old := target.OldSnapshotBook()
		result.Warnings = append(result.Warnings,
			"page fetch/parse failed; kept previous snapshot row so the dataset does not lose the book")
		return r.finish(result, StatusKeptFromPrevious, SourcePreviousSnapshot, &old)
	}
	return r.finish(result, failStatus, SourceSkipped, nil)
}

func (r *Runner) finish(result TargetResult, status, source string, final *book.Book) TargetResult {
	result.Status = status
	result.SelectedSource = source
	result.FinalBook = final
	result.FinishedAt = r.cfg.Now().UTC().Format(time.RFC3339)
	return result
}

// FilterTargets selects targets by priority ("all" keeps everything) and
// applies an optional limit (0 = no limit).
func FilterTargets(targets []RecoveryTarget, priority string, limit int) []RecoveryTarget {
	filtered := make([]RecoveryTarget, 0, len(targets))
	for _, t := range targets {
		if priority != "" && priority != "all" && t.Priority != priority {
			continue
		}
		filtered = append(filtered, t)
	}
	if limit > 0 && len(filtered) > limit {
		filtered = filtered[:limit]
	}
	return filtered
}
