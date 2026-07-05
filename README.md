# Goodreads Crawler

Crawls Goodreads book pages breadth-first from a root book via each book's
similar-books page, and writes `output.json` (books sorted by ratings
descending). Includes a targeted recovery pipeline for backfilling books that
an earlier crawl knew about but a newer crawl dropped.

## Crawler

    go run . -url <goodreads_book_url> -depth <depth> -workers <workers> -output <json_file>

Extra politeness flags: `-delay` (min delay between requests, default 2s),
`-timeout` (default 30s), `-max-retries` (default 3), `-user-agent`.

All HTTP goes through a shared polite client: per-request timeout,
identifying User-Agent, minimum delay with jitter, bounded retries with
exponential backoff, `Retry-After` support, and a hard stop after 3
consecutive 403/429 responses. A book whose detail page fetch succeeds is
saved even if its similar-books expansion fails.

To build a binary: `go build` then `./goodreads <flags>`.

## Recovery runbook (missing books backfill)

The recovery pipeline verifies and backfills works that were present in a
previous canonical output but absent (by Goodreads work ID) from a new one.

**Note for the current recovery effort:** the repo's `output.json` is the NEW
crawl (157,636 raw rows). The old snapshot data lives inside
`topreads-missing-books-to-double-check.json` (14,247 targets), which was
already generated. Start at step 2.

```bash
# 1. Compare old and new outputs (only if you have both files)
make compare-missing OLD=output.json NEW=output\(1\).json
# writes topreads-missing-books-to-double-check.json + blank_id_candidates.json

# 2. Recover the highest-impact missing books first, small batch to validate
make recover-missing PRIORITY=P0 LIMIT=100
cat recovery-report.md        # inspect before continuing

# 3. Continue recovery by priority (each tier only after the previous looks clean)
make recover-missing PRIORITY=P0
make recover-missing PRIORITY=P1
make recover-missing PRIORITY=P2
make recover-missing PRIORITY=P3

# 4. Merge new output + recovered books
make merge-outputs NEW=output.json RECOVERED=recovered-missing-books.json
# writes output.merged.json + merge-report.json

# 5. Run QA gates against the previous accepted output
make crawl-qa OLD=output.json
# writes crawl-quality-report.{json,md}; exits nonzero on regression
```

Or use the wrapper script: `./scripts/recover-missing.sh P0 100`.

### Politeness settings and expected runtime

Recovery runs sequentially with a 3s minimum delay (plus jitter), 30s
timeout, and 3 retries with 10s→5m exponential backoff. Rough runtimes at
~3.5s/request (more if alias URLs get attempted):

| Tier | Targets | Approx. time |
|---|---|---|
| P0 | 129 | ~10 min |
| P1 | 467 | ~30 min |
| P2 | 1,990 | ~2.5 h |
| P3 | 11,661 | ~12 h |

### Resume behavior

`recovery-state.json` is checkpointed atomically after **every** target.
Interrupt the run at any time; rerunning the same command skips targets that
already reached a terminal status (`recovered`, `kept_from_previous_snapshot`,
`below_threshold`, `needs_manual_review`, `parse_failed`). Transient failures
(`http_failed`, `blocked`) are retried on the next run. Use `-force` to
re-process everything.

### Output files

| File | Contents |
|---|---|
| `topreads-missing-books-to-double-check.json` | prioritized recovery targets (P0–P3) with old snapshot data |
| `blank_id_candidates.json` | new-output rows with blank IDs matching missing old works |
| `recovered-missing-books.json` | recovered books + full per-target audit results |
| `recovery-report.md` | human-readable recovery summary grouped by status |
| `recovery-state.json` | per-target checkpoint state (resume file) |
| `output.merged.json` | new output + recovered books, no duplicate/blank IDs |
| `merge-report.json` | merge provenance: kept/replaced/added IDs, aliases, quarantined blank-ID rows |
| `crawl-quality-report.{json,md}` | QA metrics and gate results |

### Identity rules

- Parsed work ID equals expected ID → recovered (high confidence).
- Parsed ID blank but normalized title + author match → recovered with the
  expected ID filled from the previous snapshot, plus a warning.
- Parsed ID *different* but title/author match → `needs_manual_review`; a
  different work ID is never silently accepted.
- With `-keep-previous-on-fail` (the Makefile default), unfetchable pages
  keep the previous snapshot row so the dataset does not lose the book, and
  the fallback is recorded in the reports.

### If Goodreads returns 403/429

The client honours `Retry-After`, backs off exponentially, and hard-stops the
whole run after 3 consecutive 403/429 responses (exit code 2). Everything is
checkpointed, so wait — an hour is a good default — then rerun the same
command; it resumes where it stopped. Do not increase workers or shrink the
delay to push through blocks.

### QA gates (nonzero exit)

- Any P0 work missing without an explicit recovery decision.
- Blank-ID rate more than 0.25 percentage points above the previous output.
- Canonical ID count dropping more than 1% vs the previous accepted output.
- More than 10 of the previous top-1,000 books disappearing.

Keep the old output file until `output.merged.json` passes QA.

## Tests

    go test ./...

## Installing Go

macOS: `brew install go` · Ubuntu: `sudo apt install golang-go` ·
Arch: `sudo pacman -S go` · Windows: install from the official Go website.
