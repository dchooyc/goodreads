package recovery

import (
	"errors"
	"fmt"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"goodreads/internal/fetch"
	"goodreads/internal/model"
	"goodreads/internal/store"

	"github.com/dchooyc/book"
)

// fakeFetcher maps URL -> canned response.
type fakeFetcher struct {
	mu        sync.Mutex
	responses map[string]fakeResponse
	calls     []string
}

type fakeResponse struct {
	body   string
	status int
	err    error
}

func (f *fakeFetcher) Fetch(url string) ([]byte, int, error) {
	f.mu.Lock()
	f.calls = append(f.calls, url)
	f.mu.Unlock()
	r, ok := f.responses[url]
	if !ok {
		return nil, 0, fmt.Errorf("fetch %s: connection refused", url)
	}
	if r.err != nil {
		return nil, r.status, r.err
	}
	return []byte(r.body), r.status, nil
}

// fakeParser maps body content -> parsed book.
func fakeParser(books map[string]book.Book) func([]byte) (*book.Book, error) {
	return func(body []byte) (*book.Book, error) {
		b, ok := books[string(body)]
		if !ok {
			return nil, errors.New("parse failed: no book data")
		}
		copied := b
		return &copied, nil
	}
}

func testRunner(t *testing.T, fetcher fetch.Fetcher, parser func([]byte) (*book.Book, error), keep bool) (*Runner, *store.State) {
	t.Helper()
	state, err := store.LoadState(filepath.Join(t.TempDir(), "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	runner, err := NewRunner(RecoveryConfig{
		Fetcher:            fetcher,
		State:              state,
		ParseBook:          parser,
		MeetsCriteria:      func(b *book.Book) bool { return b.Ratings >= 500 && b.Rating >= 4.0 },
		KeepPreviousOnFail: keep,
		Now:                func() time.Time { return time.Date(2026, 7, 6, 0, 0, 0, 0, time.UTC) },
		Logf:               t.Logf,
	})
	if err != nil {
		t.Fatal(err)
	}
	return runner, state
}

func divergentTarget() model.RecoveryTarget {
	return model.RecoveryTarget{
		Priority:                "P0",
		ExpectedGoodreadsWorkID: "13155899",
		Title:                   "Divergent",
		Authors:                 []string{"Veronica Roth"},
		PrimaryOldURL:           "https://example.com/divergent",
		OldAliasURLs:            []string{"https://example.com/divergent-alias"},
		OldRating:               4.13,
		OldRatings:              4299183,
		OldReviews:              126073,
	}
}

func TestRecoverExactMatch(t *testing.T) {
	fetcher := &fakeFetcher{responses: map[string]fakeResponse{
		"https://example.com/divergent": {body: "divergent-page", status: 200},
	}}
	parser := fakeParser(map[string]book.Book{
		"divergent-page": {ID: "13155899", Title: "Divergent", Authors: []string{"Veronica Roth"}, Rating: 4.13, Ratings: 4400000},
	})

	runner, state := testRunner(t, fetcher, parser, false)
	results, err := runner.Run([]model.RecoveryTarget{divergentTarget()})
	if err != nil {
		t.Fatal(err)
	}

	if len(results) != 1 || results[0].Status != model.StatusRecovered {
		t.Fatalf("want recovered, got %+v", results)
	}
	if results[0].SelectedSource != model.SourceFetchedCurrentPage {
		t.Errorf("wrong source: %s", results[0].SelectedSource)
	}
	if results[0].FinalBook == nil || results[0].FinalBook.URL != "https://example.com/divergent" {
		t.Errorf("final book missing or wrong URL: %+v", results[0].FinalBook)
	}
	if !state.IsDone("13155899") {
		t.Error("state must record completion")
	}
}

func TestRecoverBlankIDFillsExpected(t *testing.T) {
	fetcher := &fakeFetcher{responses: map[string]fakeResponse{
		"https://example.com/divergent": {body: "divergent-page", status: 200},
	}}
	parser := fakeParser(map[string]book.Book{
		"divergent-page": {ID: "", Title: "Divergent", Authors: []string{"Veronica Roth"}, Rating: 4.13, Ratings: 4400000},
	})

	runner, _ := testRunner(t, fetcher, parser, false)
	results, _ := runner.Run([]model.RecoveryTarget{divergentTarget()})

	r := results[0]
	if r.Status != model.StatusRecovered {
		t.Fatalf("want recovered, got %s (%v)", r.Status, r.Errors)
	}
	if r.FinalBook.ID != "13155899" {
		t.Errorf("expected ID must be filled from snapshot, got %q", r.FinalBook.ID)
	}
	if len(r.Warnings) == 0 {
		t.Error("blank-ID fill must be reported as a warning")
	}
}

func TestRecoverDifferentIDNeedsManualReview(t *testing.T) {
	fetcher := &fakeFetcher{responses: map[string]fakeResponse{
		"https://example.com/divergent":       {body: "divergent-page", status: 200},
		"https://example.com/divergent-alias": {body: "divergent-page", status: 200},
	}}
	parser := fakeParser(map[string]book.Book{
		"divergent-page": {ID: "99999", Title: "Divergent", Authors: []string{"Veronica Roth"}, Rating: 4.13, Ratings: 4400000},
	})

	runner, _ := testRunner(t, fetcher, parser, false)
	results, _ := runner.Run([]model.RecoveryTarget{divergentTarget()})

	r := results[0]
	if r.Status != model.StatusManualReview {
		t.Fatalf("different ID must need manual review, got %s", r.Status)
	}
	if r.FinalBook != nil {
		t.Error("manual review must not silently accept a different work ID")
	}
}

func TestRecoverBelowThreshold(t *testing.T) {
	fetcher := &fakeFetcher{responses: map[string]fakeResponse{
		"https://example.com/divergent": {body: "divergent-page", status: 200},
	}}
	parser := fakeParser(map[string]book.Book{
		"divergent-page": {ID: "13155899", Title: "Divergent", Authors: []string{"Veronica Roth"}, Rating: 3.2, Ratings: 300},
	})

	runner, _ := testRunner(t, fetcher, parser, false)
	results, _ := runner.Run([]model.RecoveryTarget{divergentTarget()})

	if results[0].Status != model.StatusBelowThreshold {
		t.Fatalf("want below_threshold, got %s", results[0].Status)
	}
}

func TestRecoverKeepPreviousOnFail(t *testing.T) {
	fetcher := &fakeFetcher{responses: map[string]fakeResponse{}} // every fetch errors

	runner, _ := testRunner(t, fetcher, fakeParser(nil), true)
	results, _ := runner.Run([]model.RecoveryTarget{divergentTarget()})

	r := results[0]
	if r.Status != model.StatusKeptFromPrevious {
		t.Fatalf("want kept_from_previous_snapshot, got %s (%v)", r.Status, r.Errors)
	}
	if r.SelectedSource != model.SourcePreviousSnapshot {
		t.Errorf("wrong source: %s", r.SelectedSource)
	}
	if r.FinalBook == nil || r.FinalBook.ID != "13155899" || r.FinalBook.Ratings != 4299183 {
		t.Errorf("old snapshot row not preserved: %+v", r.FinalBook)
	}
	if len(r.AttemptedURLs) != 2 {
		t.Errorf("both primary and alias URLs should be attempted, got %v", r.AttemptedURLs)
	}
}

func TestRecoverHTTPFailedWithoutKeep(t *testing.T) {
	fetcher := &fakeFetcher{responses: map[string]fakeResponse{}}

	runner, _ := testRunner(t, fetcher, fakeParser(nil), false)
	results, _ := runner.Run([]model.RecoveryTarget{divergentTarget()})

	if results[0].Status != model.StatusHTTPFailed {
		t.Fatalf("want http_failed, got %s", results[0].Status)
	}
	if results[0].FinalBook != nil {
		t.Error("no book should be emitted on http failure without keep-previous")
	}
}

func TestRecoverResumeSkipsCompleted(t *testing.T) {
	fetcher := &fakeFetcher{responses: map[string]fakeResponse{
		"https://example.com/divergent": {body: "divergent-page", status: 200},
	}}
	parser := fakeParser(map[string]book.Book{
		"divergent-page": {ID: "13155899", Title: "Divergent", Authors: []string{"Veronica Roth"}, Rating: 4.13, Ratings: 4400000},
	})

	runner, state := testRunner(t, fetcher, parser, false)
	if _, err := runner.Run([]model.RecoveryTarget{divergentTarget()}); err != nil {
		t.Fatal(err)
	}
	firstCalls := len(fetcher.calls)

	// Second run resumes from the same state: nothing should be fetched.
	runner2, err := NewRunner(RecoveryConfig{
		Fetcher: fetcher, State: state, ParseBook: parser, Logf: t.Logf,
	})
	if err != nil {
		t.Fatal(err)
	}
	results, err := runner2.Run([]model.RecoveryTarget{divergentTarget()})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 0 {
		t.Fatalf("completed target must be skipped on resume, got %+v", results)
	}
	if len(fetcher.calls) != firstCalls {
		t.Fatalf("resume must not refetch completed targets: %v", fetcher.calls)
	}
}

func TestRecoverBlockedStopsRun(t *testing.T) {
	fetcher := &fakeFetcher{responses: map[string]fakeResponse{
		"https://example.com/divergent": {status: 403, err: fetch.ErrBlocked},
	}}

	second := divergentTarget()
	second.ExpectedGoodreadsWorkID = "222"
	second.PrimaryOldURL = "https://example.com/other"

	runner, _ := testRunner(t, fetcher, fakeParser(nil), false)
	results, err := runner.Run([]model.RecoveryTarget{divergentTarget(), second})

	if !errors.Is(err, fetch.ErrBlocked) {
		t.Fatalf("run must surface fetch.ErrBlocked, got %v", err)
	}
	if len(results) != 1 || results[0].Status != model.StatusBlocked {
		t.Fatalf("first target should be marked blocked and run stopped, got %+v", results)
	}
}

func TestRecoverConcurrentWorkers(t *testing.T) {
	const n = 40
	targets := make([]model.RecoveryTarget, 0, n)
	responses := map[string]fakeResponse{}
	books := map[string]book.Book{}
	for i := 0; i < n; i++ {
		id := fmt.Sprintf("id-%d", i)
		url := fmt.Sprintf("https://example.com/book-%d", i)
		body := fmt.Sprintf("page-%d", i)
		targets = append(targets, model.RecoveryTarget{
			Priority:                "P0",
			ExpectedGoodreadsWorkID: id,
			Title:                   fmt.Sprintf("Book %d", i),
			Authors:                 []string{"Author"},
			PrimaryOldURL:           url,
		})
		responses[url] = fakeResponse{body: body, status: 200}
		books[body] = book.Book{ID: id, Title: fmt.Sprintf("Book %d", i), Authors: []string{"Author"}, Rating: 4.5, Ratings: 1000}
	}

	state, err := store.LoadState(filepath.Join(t.TempDir(), "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	var onResultCalls int
	runner, err := NewRunner(RecoveryConfig{
		Fetcher:   &fakeFetcher{responses: responses},
		State:     state,
		ParseBook: fakeParser(books),
		Workers:   8,
		Logf:      func(string, ...interface{}) {},
		OnResult:  func(model.TargetResult) { onResultCalls++ }, // serialized by the runner
	})
	if err != nil {
		t.Fatal(err)
	}

	results, err := runner.Run(targets)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != n || onResultCalls != n {
		t.Fatalf("want %d results and callbacks, got %d and %d", n, len(results), onResultCalls)
	}
	for i := 0; i < n; i++ {
		if !state.IsDone(fmt.Sprintf("id-%d", i)) {
			t.Fatalf("target id-%d not checkpointed", i)
		}
	}
}

func TestFilterTargets(t *testing.T) {
	targets := []model.RecoveryTarget{
		{Priority: "P0", ExpectedGoodreadsWorkID: "1"},
		{Priority: "P1", ExpectedGoodreadsWorkID: "2"},
		{Priority: "P0", ExpectedGoodreadsWorkID: "3"},
	}

	p0 := FilterTargets(targets, "P0", 0)
	if len(p0) != 2 {
		t.Fatalf("want 2 P0 targets, got %d", len(p0))
	}
	limited := FilterTargets(targets, "all", 1)
	if len(limited) != 1 || limited[0].ExpectedGoodreadsWorkID != "1" {
		t.Fatalf("limit not applied: %+v", limited)
	}
}

func TestBuildRecoveryOutput(t *testing.T) {
	prev := []model.TargetResult{
		{ExpectedGoodreadsWorkID: "1", Status: model.StatusHTTPFailed},
		{ExpectedGoodreadsWorkID: "2", Status: model.StatusRecovered, FinalBook: &book.Book{ID: "2", Ratings: 100}},
	}
	cur := []model.TargetResult{
		{ExpectedGoodreadsWorkID: "1", Status: model.StatusRecovered, FinalBook: &book.Book{ID: "1", Ratings: 900}},
		{ExpectedGoodreadsWorkID: "3", Status: model.StatusManualReview},
	}

	out := BuildRecoveryOutput(10, prev, cur)

	if out.Summary.Recovered != 2 || out.Summary.NeedsManualReview != 1 || out.Summary.Failed != 0 {
		t.Fatalf("summary wrong: %+v", out.Summary)
	}
	if out.Summary.StillMissing != 8 {
		t.Fatalf("still missing wrong: %d", out.Summary.StillMissing)
	}
	if len(out.Books) != 2 || out.Books[0].ID != "1" {
		t.Fatalf("books should be sorted by ratings desc: %+v", out.Books)
	}
	if len(out.TargetResults) != 3 {
		t.Fatalf("results should be deduped by work ID: %d", len(out.TargetResults))
	}
}
