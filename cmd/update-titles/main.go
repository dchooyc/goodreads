// Command update-titles re-fetches every book in the output folder and
// refreshes its rating, ratings, reviews, cover, authors and genres. It works
// chunk by chunk with resume state, flushes progress continuously, and never
// lets a mismatched or broken page overwrite a stored book.
package main

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"goodreads/internal/crawl"

	"github.com/dchooyc/book"
)

func main() {
	outputDir := flag.String("output", "output", "output folder to update in place")
	statePath := flag.String("state", "update-state.json", "checkpoint state file (per chunk)")
	workers := flag.Int("workers", 50, "concurrent workers")
	delay := flag.Duration("delay", 100*time.Millisecond, "minimum delay between requests (~10 req/s default)")
	timeout := flag.Duration("timeout", 30*time.Second, "per-request timeout")
	maxRetries := flag.Int("max-retries", 3, "max retries per URL")
	limit := flag.Int("limit", 0, "stop after this many books (sample run; chunk is not marked done)")
	force := flag.Bool("force", false, "re-update chunks already completed in the state file")
	flushEvery := flag.Int("flush-every", 200, "rewrite the chunk file after this many refreshed books")
	resort := flag.Bool("resort", true, "after a full update, resort all books by ratings descending")
	userAgent := flag.String("user-agent", crawl.DefaultUserAgent, "identifying User-Agent header")
	flag.Parse()

	chunks, err := crawl.ListBookChunks(*outputDir)
	if err != nil {
		fail(err)
	}
	if len(chunks) == 0 {
		fail(fmt.Errorf("no chunk files in %s", *outputDir))
	}

	state, err := crawl.LoadState(*statePath)
	if err != nil {
		fail(err)
	}

	client := crawl.NewClient(*userAgent, *delay, *timeout, *maxRetries)

	stats := struct{ updated, warned, failed, processed int }{}
	blocked := false
	// A book filed under many genres appears in many chunks; the cache makes
	// sure each unique URL is fetched only once per run.
	cache := map[string]book.Book{}

	for _, chunkPath := range chunks {
		chunkKey, err := filepath.Rel(*outputDir, chunkPath)
		if err != nil {
			chunkKey = chunkPath
		}
		if !*force && state.IsDone(chunkKey) {
			continue
		}
		if blocked || (*limit > 0 && stats.processed >= *limit) {
			break
		}

		books, err := crawl.ReadBooksFile(chunkPath)
		if err != nil {
			fail(err)
		}

		todo := len(books.Books)
		if *limit > 0 && stats.processed+todo > *limit {
			todo = *limit - stats.processed
		}

		fmt.Printf("chunk %s: refreshing %d of %d books\n", chunkKey, todo, len(books.Books))
		completed := updateChunk(client, books, todo, *workers, *flushEvery, chunkPath, &stats, &blocked, cache)

		if completed && todo == len(books.Books) {
			if err := state.Update(chunkKey, crawl.StatusUpdated, nil, ""); err != nil {
				fail(err)
			}
		}
	}

	fmt.Printf("\nprocessed=%d updated=%d warnings=%d fetch_failed=%d\n",
		stats.processed, stats.updated, stats.warned, stats.failed)

	if blocked {
		fmt.Fprintln(os.Stderr, "update-titles: hard stop — repeated 403/429. Progress is saved; rerun to resume.")
		os.Exit(2)
	}

	if *resort && *limit == 0 {
		all, err := crawl.ReadBooksDir(*outputDir)
		if err != nil {
			fail(err)
		}
		sort.Slice(all.Books, func(i, j int) bool {
			if all.Books[i].Ratings != all.Books[j].Ratings {
				return all.Books[i].Ratings > all.Books[j].Ratings
			}
			return all.Books[i].ID < all.Books[j].ID
		})
		if err := crawl.WriteBooksDir(*outputDir, *all, crawl.DefaultChunkSize); err != nil {
			fail(err)
		}
		fmt.Println("resorted output by ratings descending")
	}
}

// updateChunk refreshes the first todo books of a chunk in place, flushing
// the chunk file periodically. Returns false when the run was hard-stopped.
func updateChunk(client *crawl.Client, books *book.Books, todo, workers, flushEvery int,
	chunkPath string, stats *struct{ updated, warned, failed, processed int }, blocked *bool,
	cache map[string]book.Book) bool {

	var mu sync.Mutex
	flush := func() {
		if err := crawl.WriteJSONFileAtomic(chunkPath, books); err != nil {
			fmt.Println("warning: chunk flush failed:", err)
		}
	}

	jobs := make(chan int)
	var wg sync.WaitGroup
	if workers < 1 {
		workers = 1
	}
	if workers > todo {
		workers = todo
	}

	sinceFlush := 0
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range jobs {
				mu.Lock()
				stop := *blocked
				mu.Unlock()
				if stop {
					continue
				}

				old := books.Books[i]

				mu.Lock()
				cached, hit := cache[old.URL]
				mu.Unlock()
				if hit {
					mu.Lock()
					books.Books[i] = cached
					mu.Unlock()
					continue
				}

				refreshed, warn := fetchAndRefresh(client, old, blocked, &mu)

				mu.Lock()
				books.Books[i] = refreshed
				if warn == "" && old.URL != "" {
					cache[old.URL] = refreshed
				}
				stats.processed++
				if warn == "" {
					stats.updated++
				} else if warn == "fetch failed" {
					stats.failed++
				} else {
					stats.warned++
					fmt.Printf("  %s: %s\n", old.URL, warn)
				}
				sinceFlush++
				if flushEvery > 0 && sinceFlush >= flushEvery {
					sinceFlush = 0
					flush()
				}
				mu.Unlock()
			}
		}()
	}

	for i := 0; i < todo; i++ {
		jobs <- i
	}
	close(jobs)
	wg.Wait()

	flush()
	return !*blocked
}

func fetchAndRefresh(client *crawl.Client, old book.Book, blocked *bool, mu *sync.Mutex) (book.Book, string) {
	body, _, err := client.Fetch(old.URL)
	if err != nil {
		if errors.Is(err, crawl.ErrBlocked) {
			mu.Lock()
			*blocked = true
			mu.Unlock()
		}
		return old, "fetch failed"
	}

	parsed, err := book.GetBook(bytes.NewReader(body))
	if err != nil {
		return old, "parse failed: " + err.Error()
	}
	return crawl.RefreshBook(old, parsed)
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "update-titles:", err)
	os.Exit(1)
}
