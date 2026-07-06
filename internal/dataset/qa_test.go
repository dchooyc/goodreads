package dataset

import (
	"strings"
	"testing"

	"github.com/dchooyc/book"

	"goodreads/internal/model"
	"goodreads/internal/store"
)

func TestQAFailsWhenP0MissingUndecided(t *testing.T) {
	oldBooks, _ := store.ReadBooksFile("testdata/old-output.json")
	newBooks, _ := store.ReadBooksFile("testdata/new-output-missing.json")

	report := RunQA(oldBooks.Books, newBooks.Books, nil)

	if report.Passed {
		t.Fatal("QA must fail: Divergent (P0) is missing with no recovery decision")
	}
	if report.Metrics.P0MissingCount != 1 || report.Metrics.P0MissingUndecided != 1 {
		t.Fatalf("P0 metrics wrong: %+v", report.Metrics)
	}
	if len(report.Metrics.Top100MissingTitles) == 0 {
		t.Error("top missing titles must be listed")
	}
}

func TestQAPassesWithRecoveryDecisions(t *testing.T) {
	oldBooks, _ := store.ReadBooksFile("testdata/old-output.json")
	newBooks, _ := store.ReadBooksFile("testdata/new-output-missing.json")

	var recovery model.RecoveryOutput
	if err := store.ReadJSONFile("testdata/recovered-output.json", &recovery); err != nil {
		t.Fatal(err)
	}

	// Evaluate the merged output, as the runbook does.
	merged, _ := MergeOutputs(newBooks.Books, recovery)
	report := RunQA(oldBooks.Books, merged.Books, &recovery)

	if !report.Passed {
		t.Fatalf("QA should pass once missing books are recovered/decided, failures: %v", report.Failures)
	}
	if report.Metrics.P0MissingCount != 0 {
		t.Fatalf("recovered P0 should no longer be missing: %+v", report.Metrics)
	}
}

func TestQAFailsOnCanonicalCountDrop(t *testing.T) {
	oldBooks, _ := store.ReadBooksFile("testdata/old-output.json")

	// New output lost most canonical IDs.
	report := RunQA(oldBooks.Books, oldBooks.Books[:1], nil)

	if !hasFailure(report, "canonical ID count dropped") {
		t.Fatalf("canonical count drop gate did not fire: %v", report.Failures)
	}
}

func TestQABlankIDRateGate(t *testing.T) {
	oldBooks, _ := store.ReadBooksFile("testdata/old-output.json")

	// Same books, but one gains a blank ID (25% blank rate vs 0% before).
	newBooks := make([]book.Book, len(oldBooks.Books))
	copy(newBooks, oldBooks.Books)
	newBooks[0].ID = ""

	report := RunQA(oldBooks.Books, newBooks, nil)

	if !hasFailure(report, "blank ID rate") {
		t.Fatalf("blank ID rate gate did not fire: %v", report.Failures)
	}
}

func hasFailure(r QAReport, substr string) bool {
	for _, f := range r.Failures {
		if strings.Contains(f, substr) {
			return true
		}
	}
	return false
}
