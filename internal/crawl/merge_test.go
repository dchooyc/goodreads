package crawl

import (
	"testing"

	"github.com/dchooyc/book"
)

func loadMergeFixtures(t *testing.T) ([]book.Book, RecoveryOutput) {
	t.Helper()
	newBooks, err := ReadBooksFile("testdata/new-output-blank-id.json")
	if err != nil {
		t.Fatal(err)
	}
	var recovered RecoveryOutput
	if err := ReadJSONFile("testdata/recovered-output.json", &recovered); err != nil {
		t.Fatal(err)
	}
	return newBooks.Books, recovered
}

func TestMergeAddsRecoveredBooks(t *testing.T) {
	newRows, recovered := loadMergeFixtures(t)

	merged, report := MergeOutputs(newRows, recovered)

	ids := map[string]int{}
	for _, b := range merged.Books {
		if b.ID == "" {
			t.Errorf("merged output must not contain blank IDs: %+v", b)
		}
		ids[b.ID]++
	}
	for id, n := range ids {
		if n > 1 {
			t.Errorf("duplicate ID %s in merged output", id)
		}
	}

	if _, ok := ids["13155899"]; !ok {
		t.Error("recovered Divergent must be in merged output")
	}
	if _, ok := ids["111222"]; !ok {
		t.Error("kept-from-previous book must be in merged output")
	}
	if _, ok := ids["2792775"]; !ok {
		t.Error("existing new-output book must be preserved")
	}

	if len(report.KeptFromPreviousSnapshot) != 1 || report.KeptFromPreviousSnapshot[0] != "111222" {
		t.Errorf("kept provenance wrong: %v", report.KeptFromPreviousSnapshot)
	}
	if report.Summary.BlankIDRowsInNew != 1 || report.Summary.BlankIDRowsQuarantined != 1 {
		t.Errorf("blank-ID Divergent row must be quarantined: %+v", report.Summary)
	}
}

func TestMergeSortedByRatingsDesc(t *testing.T) {
	newRows, recovered := loadMergeFixtures(t)
	merged, _ := MergeOutputs(newRows, recovered)

	for i := 1; i < len(merged.Books); i++ {
		if merged.Books[i-1].Ratings < merged.Books[i].Ratings {
			t.Fatalf("merged output not sorted by ratings desc at %d", i)
		}
	}
}

func TestMergeChoosesBestCanonicalRow(t *testing.T) {
	newRows := []book.Book{
		{ID: "1", Title: "Book", URL: "https://example.com/new", Ratings: 100, Reviews: 10, CoverUrl: "x"},
	}
	recovered := RecoveryOutput{
		Books: []book.Book{
			{ID: "1", Title: "Book", URL: "https://example.com/recovered", Ratings: 200, Reviews: 20, CoverUrl: "y"},
		},
	}

	merged, report := MergeOutputs(newRows, recovered)
	if len(merged.Books) != 1 {
		t.Fatalf("want 1 book, got %d", len(merged.Books))
	}
	if merged.Books[0].URL != "https://example.com/recovered" {
		t.Errorf("higher-ratings recovered row should win, got %s", merged.Books[0].URL)
	}
	if len(report.ReplacedByRecovered) != 1 {
		t.Errorf("replacement must be reported: %v", report.ReplacedByRecovered)
	}

	// Reverse: the new row is better and must be kept.
	newRows[0].Ratings = 300
	merged, report = MergeOutputs(newRows, recovered)
	if merged.Books[0].URL != "https://example.com/new" {
		t.Errorf("higher-ratings new row should win, got %s", merged.Books[0].URL)
	}
	if len(report.ReplacedByRecovered) != 0 {
		t.Errorf("no replacement should be reported: %v", report.ReplacedByRecovered)
	}
}

func TestMergeNeverAdmitsBlankRecoveredIDs(t *testing.T) {
	merged, _ := MergeOutputs(nil, RecoveryOutput{Books: []book.Book{{ID: "", Title: "Ghost"}}})
	if len(merged.Books) != 0 {
		t.Fatalf("blank-ID recovered book must not enter merged output: %+v", merged.Books)
	}
}
