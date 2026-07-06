package crawl

import (
	"testing"

	"github.com/dchooyc/book"
)

func storedBook() book.Book {
	return book.Book{
		ID: "111", Title: "Old Title", URL: "https://example.com/b",
		CoverUrl: "old.jpg", Authors: []string{"A"}, Genres: []string{"g"},
		Rating: 4.1, Ratings: 1000, Reviews: 50,
	}
}

func TestRefreshBookUpdatesStats(t *testing.T) {
	updated, warn := RefreshBook(storedBook(), &book.Book{
		ID: "111", Title: "New Title", CoverUrl: "new.jpg",
		Authors: []string{"A", "B"}, Genres: []string{"g", "h"},
		Rating: 4.3, Ratings: 2000, Reviews: 90,
	})
	if warn != "" {
		t.Fatalf("unexpected warning: %s", warn)
	}
	if updated.Ratings != 2000 || updated.Rating != 4.3 || updated.Reviews != 90 {
		t.Fatalf("stats not refreshed: %+v", updated)
	}
	if updated.Title != "New Title" || updated.CoverUrl != "new.jpg" || len(updated.Authors) != 2 {
		t.Fatalf("fields not refreshed: %+v", updated)
	}
	if updated.ID != "111" || updated.URL != "https://example.com/b" {
		t.Fatalf("identity fields must be stable: %+v", updated)
	}
}

func TestRefreshBookRejectsDifferentID(t *testing.T) {
	updated, warn := RefreshBook(storedBook(), &book.Book{ID: "999", Title: "New", Ratings: 5000})
	if warn == "" {
		t.Fatal("different work ID must warn")
	}
	if updated.Ratings != 1000 || updated.Title != "Old Title" {
		t.Fatalf("different work ID must not overwrite the stored row: %+v", updated)
	}
}

func TestRefreshBookKeepsStatsOnParserMiss(t *testing.T) {
	updated, warn := RefreshBook(storedBook(), &book.Book{ID: "111", Title: "New Title", Ratings: 0})
	if warn == "" {
		t.Fatal("zero-rating parse should warn")
	}
	if updated.Ratings != 1000 || updated.Rating != 4.1 {
		t.Fatalf("parser miss must not blank stats: %+v", updated)
	}
	if updated.Title != "New Title" {
		t.Fatalf("nonblank parsed fields should still refresh: %+v", updated)
	}
}

func TestRefreshBookKeepsBlankParsedFields(t *testing.T) {
	updated, _ := RefreshBook(storedBook(), &book.Book{ID: "", Title: "", Ratings: 1500, Rating: 4.0})
	if updated.ID != "111" || updated.Title != "Old Title" || updated.CoverUrl != "old.jpg" {
		t.Fatalf("blank parsed fields must not blank stored ones: %+v", updated)
	}
	if updated.Ratings != 1500 {
		t.Fatalf("stats should refresh: %+v", updated)
	}
}
