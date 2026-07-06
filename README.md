# Goodreads Crawler

Crawls Goodreads book pages breadth-first from a root book via each book's
similar-books page. The dataset lives in the `output/` folder as small chunk
files (~2000 books, ~1 MB each) so no single file ever upsets GitHub.

## Dataset layout

```
output/
  books-00001.json   # {"books": [...]} — 2000 books, sorted by ratings desc
  books-00002.json
  ...
```

Every command accepts either a chunked folder or a single `.json` file for
its inputs; writers write a folder unless the path ends in `.json`.

## Crawler

    go run . -url <goodreads_book_url> -depth <depth> -workers <workers> -output output

Politeness flags: `-delay` (default 2s), `-timeout` (30s), `-max-retries`
(3), `-user-agent`. Progress is checkpointed: the crawler atomically rewrites
the output folder every `-flush-every` saved books (default 100) and after
each depth, so an interrupted crawl keeps everything collected so far. Point
`-input output` at an existing folder to seed the queue from it.

All HTTP goes through one shared polite client: per-request timeout,
identifying User-Agent, minimum delay with jitter, linear retry backoff
(10s/20s/30s), `Retry-After` support, and a hard stop after 3 consecutive
403/429 responses. A book whose detail page fetch succeeds is saved even if
its similar-books expansion fails.

## Updating all titles

    go run ./cmd/update-titles                 # full refresh of output/
    go run ./cmd/update-titles -limit 20       # sample run
    make update-titles

Re-fetches every book and refreshes rating/ratings/reviews/cover/authors/
genres in place, chunk by chunk. Defaults: 50 workers, 100ms delay
(~10 req/s — reasonably fast without being abusive), flush every 200 books,
per-chunk resume state in `update-state.json`. Interrupt any time; rerunning
skips completed chunks (`-force` redoes them). A page whose work ID differs
from the stored one, or that parses without ratings, never overwrites the
stored row. After a full run the folder is resorted by ratings descending
(`-resort=false` to skip). Full refresh of ~163k books takes ~4.5–5 h.

## Recovery pipeline (backfilling dropped books)

Verifies and backfills works present in a previous output but absent (by
Goodreads work ID) from a new one.

```bash
# 1. Compare old and new outputs -> prioritized target list (P0..P3)
go run ./cmd/compare-missing -old <old> -new <new> -out topreads-missing-books-to-double-check.json

# 2. Recover, most important first (resumable; state in recovery-state.json)
go run ./cmd/recover-missing -priority P0 -limit 100
go run ./cmd/recover-missing -priority all

# 3. Merge recovered books into the output folder
go run ./cmd/merge-outputs -new output -recovered recovered-missing-books.json -out output

# 4. QA gates (nonzero exit on regression)
go run ./cmd/crawl-qa -old <previous-accepted> -new output -recovered recovered-missing-books.json
```

Identity rules: a matching work ID is high confidence; a blank parsed ID with
matching title+author fills the expected ID (with a warning); a *different*
work ID is never silently accepted (`needs_manual_review`).
`-keep-previous-on-fail` keeps the previous snapshot row when a page cannot
be fetched. Recovery is checkpointed after every target and resumes where it
stopped; on repeated 403/429 it exits with code 2 — wait, then rerun.

QA gates: missing P0 works without a recovery decision, blank-ID rate rising
more than 0.25 pp, canonical count dropping more than 1%, or more than 10 of
the previous top-1000 disappearing.

## Utilities

    go run ./cmd/split-output -in big.json -out output   # split a monolithic file into chunks

## Tests

    go test ./...

## Installing Go

macOS: `brew install go` · Ubuntu: `sudo apt install golang-go` ·
Arch: `sudo pacman -S go` · Windows: install from the official Go website.
