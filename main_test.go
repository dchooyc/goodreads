package main

import (
	"errors"
	"testing"

	"github.com/dchooyc/book"
)

func fakeGetBook(b *book.Book, err error) bookFetcher {
	return func(url string) (*book.Book, error) {
		if err != nil {
			return nil, err
		}
		copied := *b
		return &copied, nil
	}
}

func goodBook() *book.Book {
	return &book.Book{
		Title:   "Divergent",
		ID:      "13155899",
		Rating:  4.13,
		Ratings: 4299183,
	}
}

func TestDetailSuccessExpansionFailureStillSavesBook(t *testing.T) {
	getBook := fakeGetBook(goodBook(), nil)
	getBookURLs := func(id string) ([]string, error) {
		return nil, errors.New("similar fetch exploded")
	}

	urlToBook := make(map[string]*book.Book)
	queue := []string{"https://www.goodreads.com/book/show/13335037-divergent"}

	processQueue(false, 1, queue, urlToBook, getBook, getBookURLs)

	saved, ok := urlToBook["https://www.goodreads.com/book/show/13335037-divergent"]
	if !ok || saved == nil {
		t.Fatal("book must be saved even though similar-books expansion failed")
	}
	if saved.Title != "Divergent" {
		t.Fatalf("wrong book saved: %+v", saved)
	}
}

func TestDetailFailureDoesNotSaveBook(t *testing.T) {
	getBook := fakeGetBook(nil, errors.New("detail fetch exploded"))
	getBookURLs := func(id string) ([]string, error) { return nil, nil }

	urlToBook := make(map[string]*book.Book)
	queue := []string{"https://www.goodreads.com/book/show/13335037-divergent"}

	processQueue(false, 1, queue, urlToBook, getBook, getBookURLs)

	if len(urlToBook) != 0 {
		t.Fatalf("no book should be saved on detail failure, got %d", len(urlToBook))
	}
}

func TestProcessBookSeparatesOutcomes(t *testing.T) {
	getBook := fakeGetBook(goodBook(), nil)
	failExpansion := func(id string) ([]string, error) { return nil, errors.New("boom") }

	p := processBook(false, "https://example.com/book", getBook, failExpansion)
	if p.detailErr != nil {
		t.Fatalf("detail must succeed, got %v", p.detailErr)
	}
	if p.expansionErr == nil {
		t.Fatal("expansion error must be recorded")
	}
	if p.book == nil {
		t.Fatal("book must be returned despite expansion failure")
	}

	okExpansion := func(id string) ([]string, error) { return []string{"https://example.com/similar"}, nil }
	p = processBook(false, "https://example.com/book", getBook, okExpansion)
	if p.expansionErr != nil || len(p.similarBooks) != 1 {
		t.Fatalf("expansion should succeed: %+v", p)
	}
}

func TestExpansionSkippedOnLastDepth(t *testing.T) {
	getBook := fakeGetBook(goodBook(), nil)
	getBookURLs := func(id string) ([]string, error) {
		t.Fatal("expansion must not run at max depth")
		return nil, nil
	}

	p := processBook(true, "https://example.com/book", getBook, getBookURLs)
	if p.book == nil || p.detailErr != nil {
		t.Fatalf("book should be saved at last depth: %+v", p)
	}
}
