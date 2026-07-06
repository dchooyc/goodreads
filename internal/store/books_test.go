package store

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/dchooyc/book"
)

func genreBooks() book.Books {
	return book.Books{Books: []book.Book{
		{ID: "1", Title: "Epic", Genres: []string{"fantasy", "fiction"}, Ratings: 900},
		{ID: "2", Title: "Saga", Genres: []string{"fantasy"}, Ratings: 800},
		{ID: "3", Title: "Rare", Genres: []string{"obscure-genre"}, Ratings: 10},
		{ID: "4", Title: "Naked", Genres: nil, Ratings: 5},
	}}
}

func TestWriteBooksDirGroupsByGenre(t *testing.T) {
	dir := t.TempDir()
	if err := WriteBooksDir(dir, genreBooks(), 10); err != nil {
		t.Fatal(err)
	}

	for _, want := range []string{
		"e-f/fantasy/books-00001.json",
		"e-f/fiction/books-00001.json",
		"o-r/obscure-genre/books-00001.json",
		"t-z/uncategorized/books-00001.json",
		GenresManifest,
	} {
		if _, err := os.Stat(filepath.Join(dir, want)); err != nil {
			t.Errorf("missing %s: %v", want, err)
		}
	}

	fantasy, err := ReadBooksFile(filepath.Join(dir, "e-f/fantasy/books-00001.json"))
	if err != nil {
		t.Fatal(err)
	}
	if len(fantasy.Books) != 2 || fantasy.Books[0].ID != "1" {
		t.Fatalf("fantasy should hold books 1,2 sorted by ratings: %+v", fantasy.Books)
	}
}

func TestSmallGenresAreNeverDropped(t *testing.T) {
	dir := t.TempDir()
	if err := WriteBooksDir(dir, genreBooks(), 10); err != nil {
		t.Fatal(err)
	}

	var manifest GenresFile
	if err := ReadJSONFile(filepath.Join(dir, GenresManifest), &manifest); err != nil {
		t.Fatal(err)
	}
	if manifest.TotalBooks != 4 {
		t.Fatalf("total books wrong: %d", manifest.TotalBooks)
	}

	counts := map[string]int{}
	for _, g := range manifest.Genres {
		counts[g.Name] = g.Books
	}
	if counts["obscure-genre"] != 1 {
		t.Fatalf("single-book genre must survive: %v", counts)
	}
	if counts["uncategorized"] != 1 || counts["fantasy"] != 2 || counts["fiction"] != 1 {
		t.Fatalf("genre counts wrong: %v", counts)
	}
}

func TestReadBooksDirDeduplicates(t *testing.T) {
	dir := t.TempDir()
	if err := WriteBooksDir(dir, genreBooks(), 10); err != nil {
		t.Fatal(err)
	}

	out, err := ReadBooksDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Books) != 4 {
		t.Fatalf("book 1 appears in two genres but the dataset has 4 unique books, got %d", len(out.Books))
	}
	if out.Books[0].ID != "1" {
		t.Fatalf("dataset should be sorted by ratings desc: %+v", out.Books[0])
	}
}

func TestWriteBooksDirCleansLegacyAndStale(t *testing.T) {
	dir := t.TempDir()

	// Legacy flat layout plus a genre that will disappear.
	if err := WriteJSONFileAtomic(filepath.Join(dir, "books-00001.json"), genreBooks()); err != nil {
		t.Fatal(err)
	}
	if err := WriteBooksDir(dir, genreBooks(), 10); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "books-00001.json")); !os.IsNotExist(err) {
		t.Error("legacy flat chunk should be removed")
	}

	// Shrink: only book 2 (fantasy) remains -> other genre dirs must go.
	one := book.Books{Books: genreBooks().Books[1:2]}
	if err := WriteBooksDir(dir, one, 10); err != nil {
		t.Fatal(err)
	}
	for _, gone := range []string{"e-f/fiction", "o-r/obscure-genre", "t-z/uncategorized"} {
		if _, err := os.Stat(filepath.Join(dir, gone)); !os.IsNotExist(err) {
			t.Errorf("stale genre dir %s should be removed", gone)
		}
	}

	out, _ := ReadBooksDir(dir)
	if len(out.Books) != 1 || out.Books[0].ID != "2" {
		t.Fatalf("shrunk dataset wrong: %+v", out.Books)
	}
}

func TestChunkingWithinGenre(t *testing.T) {
	dir := t.TempDir()
	books := book.Books{}
	for i := 0; i < 25; i++ {
		books.Books = append(books.Books, book.Book{
			ID: fmt.Sprintf("id-%03d", i), Genres: []string{"fantasy"}, Ratings: 100 - i,
		})
	}
	if err := WriteBooksDir(dir, books, 10); err != nil {
		t.Fatal(err)
	}

	chunks, _ := ListBookChunks(dir)
	if len(chunks) != 3 {
		t.Fatalf("want 3 chunks (10+10+5), got %d: %v", len(chunks), chunks)
	}
	out, _ := ReadBooksDir(dir)
	if len(out.Books) != 25 {
		t.Fatalf("want 25 books, got %d", len(out.Books))
	}
}

func TestReadBooksHandlesFileAndDir(t *testing.T) {
	dir := t.TempDir()
	in := genreBooks()

	file := filepath.Join(dir, "single.json")
	if err := WriteBooks(file, in, 0); err != nil {
		t.Fatal(err)
	}
	fromFile, err := ReadBooks(file)
	if err != nil || len(fromFile.Books) != 4 {
		t.Fatalf("file read failed: %v", err)
	}

	folder := filepath.Join(dir, "output")
	if err := WriteBooks(folder, in, 2); err != nil {
		t.Fatal(err)
	}
	fromDir, err := ReadBooks(folder)
	if err != nil || len(fromDir.Books) != 4 {
		t.Fatalf("dir read failed: %v", err)
	}
}

func TestSanitizeGenre(t *testing.T) {
	cases := []struct{ in, want string }{
		{"fantasy", "fantasy"},
		{"Young Adult", "young-adult"},
		{"sci%20fi", ""},
		{"  -weird-  ", "weird"},
		{"40k", "40k"},
	}
	for _, c := range cases {
		if got := sanitizeGenre(c.in); got != c.want {
			t.Errorf("sanitizeGenre(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestWriteBooksDirEmpty(t *testing.T) {
	dir := t.TempDir()
	if err := WriteBooksDir(dir, book.Books{}, 10); err != nil {
		t.Fatal(err)
	}
	out, err := ReadBooksDir(dir)
	if err != nil || len(out.Books) != 0 {
		t.Fatalf("empty dataset round trip failed: %v", err)
	}
}

func TestBucketFor(t *testing.T) {
	cases := []struct{ genre, want string }{
		{"40k", "0-9"},
		{"biography", "a-b"},
		{"fantasy", "e-f"},
		{"kids", "i-l"},
		{"science-fiction", "s"},
		{"uncategorized", "t-z"},
		{"zombies", "t-z"},
	}
	for _, c := range cases {
		if got := bucketFor(c.genre); got != c.want {
			t.Errorf("bucketFor(%q) = %q, want %q", c.genre, got, c.want)
		}
	}
}

func TestManifestIncludesPaths(t *testing.T) {
	dir := t.TempDir()
	if err := WriteBooksDir(dir, genreBooks(), 10); err != nil {
		t.Fatal(err)
	}
	var manifest GenresFile
	if err := ReadJSONFile(filepath.Join(dir, GenresManifest), &manifest); err != nil {
		t.Fatal(err)
	}
	for _, g := range manifest.Genres {
		if g.Name == "fantasy" && g.Path != "e-f/fantasy" {
			t.Errorf("fantasy path = %q, want e-f/fantasy", g.Path)
		}
	}
}

func TestWriteBooksDirMigratesLegacyGenreLayout(t *testing.T) {
	dir := t.TempDir()
	// Old unbucketed layout: genre folder at the top level.
	if err := os.MkdirAll(filepath.Join(dir, "fantasy"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := WriteJSONFileAtomic(filepath.Join(dir, "fantasy", "books-00001.json"), genreBooks()); err != nil {
		t.Fatal(err)
	}

	loaded, err := ReadBooksDir(dir)
	if err != nil || len(loaded.Books) != 4 {
		t.Fatalf("legacy genre layout should load: %v", err)
	}

	if err := WriteBooksDir(dir, *loaded, 10); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "fantasy")); !os.IsNotExist(err) {
		t.Error("legacy genre dir should be cleaned up")
	}
	if _, err := os.Stat(filepath.Join(dir, "e-f", "fantasy", "books-00001.json")); err != nil {
		t.Errorf("bucketed layout missing: %v", err)
	}
}
