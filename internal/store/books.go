package store

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/dchooyc/book"
)

// The output folder groups books by genre, with genres bucketed into
// alphabetical ranges so the folder stays browsable. Every genre a book
// lists gets a copy, no genre is dropped for being small, and no genre is
// capped:
//
//	output/
//	  genres.json                   // manifest: every genre, count and path
//	  a-b/biography/books-00001.json
//	  e-f/fantasy/books-00001.json  // chunked, sorted by ratings descending
//	  e-f/fantasy/books-00002.json
//	  t-z/uncategorized/...         // books that list no genres
//
// Reads dedupe by work ID, so the logical dataset is unchanged by the
// duplication. Legacy layouts (genre dirs or books-*.json at the top level)
// still load, and are cleaned up by the next write.

// DefaultChunkSize is how many books go into one chunk file (~1 MB), small
// enough that GitHub never complains.
const DefaultChunkSize = 2000

// GenresManifest is the manifest filename inside an output folder.
const GenresManifest = "genres.json"

// UncategorizedGenre is where books without genres are filed.
const UncategorizedGenre = "uncategorized"

// genreBuckets are the alphabetical ranges genre folders are grouped into,
// sized so each bucket holds a comparable number of genres.
var genreBuckets = []string{"0-9", "a-b", "c-d", "e-f", "g-h", "i-l", "m-n", "o-r", "s", "t-z"}

// bucketFor returns the alphabetical bucket a genre folder lives in.
func bucketFor(genre string) string {
	if genre == "" {
		return genreBuckets[len(genreBuckets)-1]
	}
	c := genre[0]
	if c >= '0' && c <= '9' {
		return "0-9"
	}
	for _, b := range genreBuckets[1:] {
		lo, hi := b[0], b[len(b)-1]
		if c >= lo && c <= hi {
			return b
		}
	}
	return genreBuckets[len(genreBuckets)-1]
}

// GenreCount is one entry of genres.json.
type GenreCount struct {
	Name  string `json:"name"`
	Books int    `json:"books"`
	Path  string `json:"path"`
}

// GenresFile is the schema of genres.json.
type GenresFile struct {
	TotalBooks int          `json:"total_books"`
	Genres     []GenreCount `json:"genres"`
}

func chunkName(i int) string { return fmt.Sprintf("books-%05d.json", i+1) }

func sanitizeGenre(g string) string {
	if strings.Contains(g, "%") { // URL-encoded junk from the crawl
		return ""
	}
	var b strings.Builder
	for _, r := range strings.ToLower(strings.TrimSpace(g)) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '-', r == '_':
			b.WriteRune(r)
		case r == ' ':
			b.WriteRune('-')
		}
	}
	return strings.Trim(b.String(), "-")
}

// bookGenres returns the sanitized, deduplicated genres a book is filed
// under; books with no usable genre go to UncategorizedGenre.
func bookGenres(b book.Book) []string {
	seen := map[string]bool{}
	genres := []string{}
	for _, g := range b.Genres {
		s := sanitizeGenre(g)
		if s != "" && !seen[s] {
			seen[s] = true
			genres = append(genres, s)
		}
	}
	if len(genres) == 0 {
		return []string{UncategorizedGenre}
	}
	return genres
}

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
// bucketed genre folders first, then legacy genre folders, then legacy
// top-level chunks.
func ListBookChunks(dir string) ([]string, error) {
	chunks := []string{}
	for _, pattern := range []string{
		filepath.Join(dir, "*", "*", "books-*.json"), // bucket/genre
		filepath.Join(dir, "*", "books-*.json"),      // legacy genre
		filepath.Join(dir, "books-*.json"),           // legacy flat
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

// WriteBooksDir writes books into dir grouped by genre. Input duplicates are
// removed by work ID first; each unique book is then filed under every genre
// it lists. Chunks are written atomically, stale chunks/genres and legacy
// flat chunks are removed, and genres.json is refreshed.
func WriteBooksDir(dir string, books book.Books, chunkSize int) error {
	if chunkSize <= 0 {
		chunkSize = DefaultChunkSize
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("mkdir %s: %w", dir, err)
	}

	unique := dedupeBooks(books.Books)

	byGenre := map[string][]book.Book{}
	for _, b := range unique {
		for _, g := range bookGenres(b) {
			byGenre[g] = append(byGenre[g], b)
		}
	}

	genreNames := make([]string, 0, len(byGenre))
	for g := range byGenre {
		genreNames = append(genreNames, g)
	}
	sort.Strings(genreNames)

	manifest := GenresFile{TotalBooks: len(unique), Genres: []GenreCount{}}
	for _, g := range genreNames {
		rows := byGenre[g]
		sort.Slice(rows, func(i, j int) bool {
			if rows[i].Ratings != rows[j].Ratings {
				return rows[i].Ratings > rows[j].Ratings
			}
			return rows[i].ID < rows[j].ID
		})

		genreDir := filepath.Join(dir, bucketFor(g), g)
		if err := os.MkdirAll(genreDir, 0o755); err != nil {
			return fmt.Errorf("mkdir %s: %w", genreDir, err)
		}

		n := 0
		for start := 0; start < len(rows); start += chunkSize {
			end := start + chunkSize
			if end > len(rows) {
				end = len(rows)
			}
			if err := WriteJSONFileAtomic(filepath.Join(genreDir, chunkName(n)), book.Books{Books: rows[start:end]}); err != nil {
				return err
			}
			n++
		}
		if err := removeStaleChunks(genreDir, n); err != nil {
			return err
		}

		manifest.Genres = append(manifest.Genres, GenreCount{
			Name:  g,
			Books: len(rows),
			Path:  filepath.ToSlash(filepath.Join(bucketFor(g), g)),
		})
	}

	if err := removeStaleGenres(dir, byGenre); err != nil {
		return err
	}
	if err := removeStaleChunks(dir, 0); err != nil { // legacy flat chunks
		return err
	}

	return WriteJSONFileAtomic(filepath.Join(dir, GenresManifest), manifest)
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

func isBucketName(name string) bool {
	for _, b := range genreBuckets {
		if name == b {
			return true
		}
	}
	return false
}

// removeGenreDir deletes a genre folder's chunks and, if then empty, the
// folder itself. Folders holding unrelated files are left in place.
func removeGenreDir(genreDir string) error {
	chunks, err := filepath.Glob(filepath.Join(genreDir, "books-*.json"))
	if err != nil {
		return err
	}
	if len(chunks) == 0 {
		return nil // not a genre folder; leave it alone
	}
	for _, c := range chunks {
		if err := os.Remove(c); err != nil {
			return err
		}
	}
	if err := os.Remove(genreDir); err != nil && !errors.Is(err, fs.ErrExist) {
		return nil // directory had unrelated files; keep it
	}
	return nil
}

func removeStaleGenres(dir string, byGenre map[string][]book.Book) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}

		if !isBucketName(e.Name()) {
			// Legacy unbucketed genre folder from an earlier layout.
			if err := removeGenreDir(filepath.Join(dir, e.Name())); err != nil {
				return err
			}
			continue
		}

		bucketDir := filepath.Join(dir, e.Name())
		genreDirs, err := os.ReadDir(bucketDir)
		if err != nil {
			return err
		}
		for _, g := range genreDirs {
			if !g.IsDir() {
				continue
			}
			rows, ok := byGenre[g.Name()]
			if ok && len(rows) > 0 && bucketFor(g.Name()) == e.Name() {
				continue
			}
			if err := removeGenreDir(filepath.Join(bucketDir, g.Name())); err != nil {
				return err
			}
		}
		os.Remove(bucketDir) // removes the bucket only when empty
	}
	return nil
}

// ReadBooksDir loads an output folder and returns the deduplicated dataset,
// sorted by ratings descending. Both genre-grouped and legacy flat layouts
// are supported.
func ReadBooksDir(dir string) (*book.Books, error) {
	chunks, err := ListBookChunks(dir)
	if err != nil {
		return nil, err
	}
	if len(chunks) == 0 {
		if _, err := os.Stat(filepath.Join(dir, GenresManifest)); err == nil {
			return &book.Books{Books: []book.Book{}}, nil
		}
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
// to a genre-grouped folder.
func WriteBooks(path string, books book.Books, chunkSize int) error {
	if strings.HasSuffix(path, ".json") {
		return WriteJSONFileAtomic(path, books)
	}
	return WriteBooksDir(path, books, chunkSize)
}
