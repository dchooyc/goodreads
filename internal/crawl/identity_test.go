package crawl

import (
	"testing"

	"github.com/dchooyc/book"
)

func TestNormalizeTitle(t *testing.T) {
	cases := []struct{ in, want string }{
		{"Divergent", "divergent"},
		{"  The  Hobbit, or There and Back Again!  ", "the hobbit or there and back again"},
		{"Café: A Story", "cafe a story"},
		{"L'Étranger", "l etranger"},
	}
	for _, c := range cases {
		if got := NormalizeTitle(c.in); got != c.want {
			t.Errorf("NormalizeTitle(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestTitleSimilarity(t *testing.T) {
	if got := TitleSimilarity("Divergent", "Divergent!"); got != 1 {
		t.Errorf("punctuation-only difference should be 1, got %f", got)
	}
	if got := TitleSimilarity("Divergent", "Insurgent"); got > 0.8 {
		t.Errorf("different titles should score low, got %f", got)
	}
	if got := TitleSimilarity("", "Divergent"); got != 0 {
		t.Errorf("empty title should score 0, got %f", got)
	}
}

func TestAuthorMatch(t *testing.T) {
	if !AuthorMatch([]string{"Veronica Roth"}, []string{"veronica roth"}) {
		t.Error("case-insensitive author should match")
	}
	if !AuthorMatch([]string{"Gabriel García Márquez"}, []string{"Gabriel Garcia Marquez"}) {
		t.Error("diacritic-folded author should match")
	}
	if AuthorMatch([]string{"Veronica Roth"}, []string{"Suzanne Collins"}) {
		t.Error("different authors should not match")
	}
	if AuthorMatch(nil, []string{"Veronica Roth"}) {
		t.Error("empty old authors should not match")
	}
}

func target() RecoveryTarget {
	return RecoveryTarget{
		ExpectedGoodreadsWorkID: "13155899",
		Title:                   "Divergent",
		Authors:                 []string{"Veronica Roth"},
	}
}

func TestValidateExactIDMatch(t *testing.T) {
	res := ValidateRecoveredBook(target(), book.Book{ID: "13155899", Title: "Divergent", Authors: []string{"Veronica Roth"}})
	if res.Confidence != ConfidenceHigh {
		t.Fatalf("want high confidence, got %s (%s)", res.Confidence, res.Reason)
	}
	if res.ResolvedID != "13155899" {
		t.Fatalf("want resolved ID 13155899, got %q", res.ResolvedID)
	}
}

func TestValidatePunctuationTitleMatch(t *testing.T) {
	res := ValidateRecoveredBook(target(), book.Book{ID: "13155899", Title: "Divergent!", Authors: []string{"Veronica Roth"}})
	if res.Confidence != ConfidenceHigh {
		t.Fatalf("ID match should win regardless of title punctuation, got %s", res.Confidence)
	}
}

func TestValidateBlankParsedID(t *testing.T) {
	res := ValidateRecoveredBook(target(), book.Book{ID: "", Title: "Divergent", Authors: []string{"Veronica Roth"}})
	if res.Confidence != ConfidenceMedium {
		t.Fatalf("want medium confidence, got %s (%s)", res.Confidence, res.Reason)
	}
	if !res.FilledExpectedID || res.ResolvedID != "13155899" {
		t.Fatalf("expected ID should be filled from snapshot, got %+v", res)
	}
	if len(res.Warnings) == 0 {
		t.Fatal("blank-ID fill must carry a warning")
	}
}

func TestValidateDifferentParsedID(t *testing.T) {
	res := ValidateRecoveredBook(target(), book.Book{ID: "99999", Title: "Divergent", Authors: []string{"Veronica Roth"}})
	if res.Confidence != ConfidenceManualReview {
		t.Fatalf("different ID with matching title/author must be manual review, got %s", res.Confidence)
	}
	if res.ResolvedID != "" {
		t.Fatalf("must not resolve to a different ID, got %q", res.ResolvedID)
	}
}

func TestValidateDifferentAuthor(t *testing.T) {
	res := ValidateRecoveredBook(target(), book.Book{ID: "", Title: "Some Other Book Entirely", Authors: []string{"Somebody Else"}})
	if res.Confidence != ConfidenceRejected {
		t.Fatalf("different title and author must be rejected, got %s", res.Confidence)
	}
}
