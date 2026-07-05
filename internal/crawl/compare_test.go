package crawl

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestPriorityFor(t *testing.T) {
	cases := []struct {
		ratings, reviews int
		want             string
	}{
		{100000, 0, "P0"},
		{0, 10000, "P0"},
		{25000, 0, "P1"},
		{0, 2500, "P1"},
		{5000, 0, "P2"},
		{0, 500, "P2"},
		{4999, 499, "P3"},
	}
	for _, c := range cases {
		if got := PriorityFor(c.ratings, c.reviews); got != c.want {
			t.Errorf("PriorityFor(%d, %d) = %s, want %s", c.ratings, c.reviews, got, c.want)
		}
	}
}

func TestCompareOutputsFindsMissing(t *testing.T) {
	oldBooks, err := ReadBooksFile("testdata/old-output.json")
	if err != nil {
		t.Fatal(err)
	}
	newBooks, err := ReadBooksFile("testdata/new-output-missing.json")
	if err != nil {
		t.Fatal(err)
	}

	res := CompareOutputs(oldBooks.Books, newBooks.Books, "old.json", "new.json", "2026-07-06")

	var missingIDs []string
	for _, target := range res.Targets.CheckTargets {
		missingIDs = append(missingIDs, target.ExpectedGoodreadsWorkID)
	}
	// Divergent (4.3M ratings) sorts before Quiet Little Book (1200 ratings).
	want := []string{"13155899", "111222"}
	if !reflect.DeepEqual(missingIDs, want) {
		t.Fatalf("missing IDs = %v, want %v", missingIDs, want)
	}

	divergent := res.Targets.CheckTargets[0]
	if divergent.Priority != "P0" {
		t.Errorf("Divergent should be P0, got %s", divergent.Priority)
	}
	if divergent.PrimaryOldURL != "https://www.goodreads.com/book/show/13335037-divergent" {
		t.Errorf("canonical row should prefer higher-ratings row with cover, got %s", divergent.PrimaryOldURL)
	}
	if len(divergent.OldAliasURLs) != 1 || divergent.OldAliasURLs[0] != "https://www.goodreads.com/book/show/13335037-divergent-alias" {
		t.Errorf("alias URLs not captured: %v", divergent.OldAliasURLs)
	}

	if res.Targets.Summary.MissingPriorityCounts["P0"] != 1 || res.Targets.Summary.MissingPriorityCounts["P3"] != 1 {
		t.Errorf("priority counts wrong: %v", res.Targets.Summary.MissingPriorityCounts)
	}
	if res.Targets.Summary.SharedCanonicalBooks != 1 {
		t.Errorf("shared count wrong: %d", res.Targets.Summary.SharedCanonicalBooks)
	}
	if res.Targets.Summary.NewCanonicalBooksNotInOld != 1 {
		t.Errorf("new-not-in-old count wrong: %d", res.Targets.Summary.NewCanonicalBooksNotInOld)
	}
}

func TestCompareOutputsDeterministic(t *testing.T) {
	oldBooks, _ := ReadBooksFile("testdata/old-output.json")
	newBooks, _ := ReadBooksFile("testdata/new-output-missing.json")

	a := CompareOutputs(oldBooks.Books, newBooks.Books, "old.json", "new.json", "2026-07-06")
	b := CompareOutputs(oldBooks.Books, newBooks.Books, "old.json", "new.json", "2026-07-06")

	ja, _ := json.Marshal(a)
	jb, _ := json.Marshal(b)
	if string(ja) != string(jb) {
		t.Fatal("comparison output is not deterministic across runs")
	}
}

func TestCompareOutputsReportsBlankIDMatches(t *testing.T) {
	oldBooks, _ := ReadBooksFile("testdata/old-output.json")
	newBooks, _ := ReadBooksFile("testdata/new-output-blank-id.json")

	res := CompareOutputs(oldBooks.Books, newBooks.Books, "old.json", "new.json", "2026-07-06")

	if len(res.BlankIDCandidates) == 0 {
		t.Fatal("blank-ID same-title matches must be reported, not silently ignored")
	}
	c := res.BlankIDCandidates[0]
	if c.PossibleExpectedWorkIDFromOld != "13155899" {
		t.Errorf("candidate should point at the missing old work ID, got %q", c.PossibleExpectedWorkIDFromOld)
	}
	if c.MatchConfidence != "medium" {
		t.Errorf("title+author match should be medium confidence, got %q", c.MatchConfidence)
	}
	if res.Targets.Summary.PossibleNewRawMatchCounts["has_blank_id_possible_match"] != 1 {
		t.Errorf("blank-ID match count wrong: %v", res.Targets.Summary.PossibleNewRawMatchCounts)
	}
}
