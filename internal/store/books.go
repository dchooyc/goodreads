package store

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/dchooyc/book"
)

// The output folder holds the dataset as fixed-size chunk files:
//
//	output/
//	  books-00001.json   // ~2000 books each (~1 MB), sorted by ratings desc
//	  books-00002.json
//	  ...
//
// Reads dedupe by work ID. Legacy genre-grouped layouts (genre folders,
// optionally under alphabetical buckets) still load, and are cleaned up by
// the next write.

// DefaultChunkSize is how many books go into one chunk file (~1 MB), small
// enough that GitHub never complains.
const DefaultChunkSize = 2000

func chunkName(i int) string { return fmt.Sprintf("books-%05d.json", i+1) }

func dedupeBooks(books []book.Book) []book.Book {
	seen := map[string]bool{}
	out := make([]book.Book, 0, len(books))
	for _, b := range books {
		key := b.ID
		if key == "" {
			key = "url:" + b.URL
		}
		if key == "url:" || !seen[key] {
			seen[key] = true
			out = append(out, b)
		}
	}
	return out
}

// ListBookChunks returns every chunk file of an output folder in order —
// top-level chunks first, then any legacy genre-layout chunks.
func ListBookChunks(dir string) ([]string, error) {
	chunks := []string{}
	for _, pattern := range []string{
		filepath.Join(dir, "books-*.json"),           // current flat layout
		filepath.Join(dir, "*", "books-*.json"),      // legacy genre dirs
		filepath.Join(dir, "*", "*", "books-*.json"), // legacy bucket/genre dirs
	} {
		found, err := filepath.Glob(pattern)
		if err != nil {
			return nil, err
		}
		sort.Strings(found)
		chunks = append(chunks, found...)
	}
	return chunks, nil
}

// WriteBooksDir writes books into dir as fixed-size chunk files. Input
// duplicates are removed by work ID and books are sorted by ratings
// descending. Chunks are written atomically; stale chunks, legacy genre
// folders and a leftover genres.json are removed afterwards.
func WriteBooksDir(dir string, books book.Books, chunkSize int) error {
	if chunkSize <= 0 {
		chunkSize = DefaultChunkSize
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("mkdir %s: %w", dir, err)
	}

	rows := dedupeBooks(books.Books)
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].Ratings != rows[j].Ratings {
			return rows[i].Ratings > rows[j].Ratings
		}
		return rows[i].ID < rows[j].ID
	})

	n := 0
	for start := 0; start < len(rows); start += chunkSize {
		end := start + chunkSize
		if end > len(rows) {
			end = len(rows)
		}
		if err := WriteJSONFileAtomic(filepath.Join(dir, chunkName(n)), book.Books{Books: rows[start:end]}); err != nil {
			return err
		}
		n++
	}
	if len(rows) == 0 {
		if err := WriteJSONFileAtomic(filepath.Join(dir, chunkName(0)), book.Books{Books: []book.Book{}}); err != nil {
			return err
		}
		n = 1
	}

	if err := removeStaleChunks(dir, n); err != nil {
		return err
	}
	if err := removeLegacyGenreDirs(dir); err != nil {
		return err
	}
	if err := os.Remove(filepath.Join(dir, "genres.json")); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func removeStaleChunks(dir string, valid int) error {
	chunks, err := filepath.Glob(filepath.Join(dir, "books-*.json"))
	if err != nil {
		return err
	}
	keep := map[string]bool{}
	for i := 0; i < valid; i++ {
		keep[chunkName(i)] = true
	}
	for _, path := range chunks {
		if !keep[filepath.Base(path)] {
			if err := os.Remove(path); err != nil {
				return fmt.Errorf("remove stale chunk %s: %w", path, err)
			}
		}
	}
	return nil
}

// removeLegacyGenreDirs deletes genre-layout folders (one or two levels of
// nesting) left over from the genre-grouped era. Only folders whose contents
// are chunk files or further genre folders are removed.
func removeLegacyGenreDirs(dir string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		sub := filepath.Join(dir, e.Name())
		nested, err := filepath.Glob(filepath.Join(sub, "*", "books-*.json"))
		if err != nil {
			return err
		}
		direct, err := filepath.Glob(filepath.Join(sub, "books-*.json"))
		if err != nil {
			return err
		}
		if len(nested) == 0 && len(direct) == 0 {
			continue // unrelated folder; leave it alone
		}
		for _, c := range append(nested, direct...) {
			if err := os.Remove(c); err != nil {
				return err
			}
		}
		if len(nested) > 0 {
			subdirs, err := os.ReadDir(sub)
			if err != nil {
				return err
			}
			for _, s := range subdirs {
				if s.IsDir() {
					os.Remove(filepath.Join(sub, s.Name())) // only removes empty dirs
				}
			}
		}
		os.Remove(sub) // only removes the folder once empty
	}
	return nil
}

// ReadBooksDir loads an output folder and returns the deduplicated dataset,
// sorted by ratings descending. Legacy genre-grouped layouts are supported.
func ReadBooksDir(dir string) (*book.Books, error) {
	chunks, err := ListBookChunks(dir)
	if err != nil {
		return nil, err
	}
	if len(chunks) == 0 {
		return nil, fmt.Errorf("no books-*.json chunks in %s", dir)
	}

	all := []book.Book{}
	for _, path := range chunks {
		part, err := ReadBooksFile(path)
		if err != nil {
			return nil, err
		}
		all = append(all, part.Books...)
	}

	unique := dedupeBooks(all)
	sort.Slice(unique, func(i, j int) bool {
		if unique[i].Ratings != unique[j].Ratings {
			return unique[i].Ratings > unique[j].Ratings
		}
		return unique[i].ID < unique[j].ID
	})
	return &book.Books{Books: unique}, nil
}

// ReadBooks loads books from either a single JSON file or an output folder,
// so every command accepts both layouts.
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
