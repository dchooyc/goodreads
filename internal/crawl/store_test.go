package crawl

import (
	"fmt"
	"path/filepath"
	"testing"

	"github.com/dchooyc/book"
)

func sampleBooks(n int) book.Books {
	books := make([]book.Book, n)
	for i := range books {
		books[i] = book.Book{ID: fmt.Sprintf("id-%04d", i), Title: fmt.Sprintf("Book %d", i), Ratings: n - i}
	}
	return book.Books{Books: books}
}

func TestBooksDirRoundTrip(t *testing.T) {
	dir := t.TempDir()
	in := sampleBooks(25)

	if err := WriteBooksDir(dir, in, 10); err != nil {
		t.Fatal(err)
	}
	chunks, _ := ListBookChunks(dir)
	if len(chunks) != 3 {
		t.Fatalf("want 3 chunks (10+10+5), got %d: %v", len(chunks), chunks)
	}

	out, err := ReadBooksDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Books) != 25 {
		t.Fatalf("want 25 books, got %d", len(out.Books))
	}
	for i, b := range out.Books {
		if b.ID != in.Books[i].ID {
			t.Fatalf("order not preserved at %d: %s != %s", i, b.ID, in.Books[i].ID)
		}
	}
}

func TestWriteBooksDirRemovesStaleChunks(t *testing.T) {
	dir := t.TempDir()

	if err := WriteBooksDir(dir, sampleBooks(50), 10); err != nil { // 5 chunks
		t.Fatal(err)
	}
	if err := WriteBooksDir(dir, sampleBooks(15), 10); err != nil { // 2 chunks
		t.Fatal(err)
	}

	chunks, _ := ListBookChunks(dir)
	if len(chunks) != 2 {
		t.Fatalf("stale chunks not removed, got %d: %v", len(chunks), chunks)
	}
	out, _ := ReadBooksDir(dir)
	if len(out.Books) != 15 {
		t.Fatalf("want 15 books after shrink, got %d", len(out.Books))
	}
}

func TestReadBooksHandlesFileAndDir(t *testing.T) {
	dir := t.TempDir()
	in := sampleBooks(5)

	file := filepath.Join(dir, "single.json")
	if err := WriteBooks(file, in, 0); err != nil {
		t.Fatal(err)
	}
	fromFile, err := ReadBooks(file)
	if err != nil || len(fromFile.Books) != 5 {
		t.Fatalf("file read failed: %v, %d", err, len(fromFile.Books))
	}

	folder := filepath.Join(dir, "output")
	if err := WriteBooks(folder, in, 2); err != nil {
		t.Fatal(err)
	}
	fromDir, err := ReadBooks(folder)
	if err != nil || len(fromDir.Books) != 5 {
		t.Fatalf("dir read failed: %v, %d", err, len(fromDir.Books))
	}
}

func TestWriteBooksDirEmpty(t *testing.T) {
	dir := t.TempDir()
	if err := WriteBooksDir(dir, book.Books{}, 10); err != nil {
		t.Fatal(err)
	}
	out, err := ReadBooksDir(dir)
	if err != nil || len(out.Books) != 0 {
		t.Fatalf("empty dataset round trip failed: %v, %+v", err, out)
	}
}
