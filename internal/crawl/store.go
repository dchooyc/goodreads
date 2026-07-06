package crawl

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/dchooyc/book"
)

// DefaultChunkSize is how many books go into one chunk file. ~2000 books is
// roughly 1 MB per file, small enough that GitHub never complains.
const DefaultChunkSize = 2000

func chunkName(i int) string { return fmt.Sprintf("books-%05d.json", i+1) }

// ListBookChunks returns the chunk files of an output folder in order.
func ListBookChunks(dir string) ([]string, error) {
	entries, err := filepath.Glob(filepath.Join(dir, "books-*.json"))
	if err != nil {
		return nil, err
	}
	sort.Strings(entries)
	return entries, nil
}

// WriteBooksDir writes books into dir as fixed-size chunk files. Each chunk
// is written atomically, and stale chunks from a previously larger dataset
// are removed afterwards.
func WriteBooksDir(dir string, books book.Books, chunkSize int) error {
	if chunkSize <= 0 {
		chunkSize = DefaultChunkSize
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("mkdir %s: %w", dir, err)
	}

	n := 0
	for start := 0; start < len(books.Books); start += chunkSize {
		end := start + chunkSize
		if end > len(books.Books) {
			end = len(books.Books)
		}
		chunk := book.Books{Books: books.Books[start:end]}
		if err := WriteJSONFileAtomic(filepath.Join(dir, chunkName(n)), chunk); err != nil {
			return err
		}
		n++
	}
	if len(books.Books) == 0 {
		if err := WriteJSONFileAtomic(filepath.Join(dir, chunkName(0)), book.Books{Books: []book.Book{}}); err != nil {
			return err
		}
		n = 1
	}

	existing, err := ListBookChunks(dir)
	if err != nil {
		return err
	}
	valid := make(map[string]bool, n)
	for i := 0; i < n; i++ {
		valid[chunkName(i)] = true
	}
	for _, path := range existing {
		if !valid[filepath.Base(path)] {
			if err := os.Remove(path); err != nil {
				return fmt.Errorf("remove stale chunk %s: %w", path, err)
			}
		}
	}
	return nil
}

// ReadBooksDir loads every chunk of an output folder, in order.
func ReadBooksDir(dir string) (*book.Books, error) {
	chunks, err := ListBookChunks(dir)
	if err != nil {
		return nil, err
	}
	if len(chunks) == 0 {
		return nil, fmt.Errorf("no books-*.json chunks in %s", dir)
	}

	all := &book.Books{}
	for _, path := range chunks {
		part, err := ReadBooksFile(path)
		if err != nil {
			return nil, err
		}
		all.Books = append(all.Books, part.Books...)
	}
	return all, nil
}

// ReadBooks loads books from either a single JSON file or a chunked output
// folder, so every command accepts both layouts.
func ReadBooks(path string) (*book.Books, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("stat %s: %w", path, err)
	}
	if info.IsDir() {
		return ReadBooksDir(path)
	}
	return ReadBooksFile(path)
}

// WriteBooks writes to a single file when the path ends in .json, otherwise
// to a chunked folder.
func WriteBooks(path string, books book.Books, chunkSize int) error {
	if strings.HasSuffix(path, ".json") {
		return WriteJSONFileAtomic(path, books)
	}
	return WriteBooksDir(path, books, chunkSize)
}
