// Command recover-missing re-fetches previously known works that vanished
// from the new crawler output. It is targeted, gentle, resumable and
// auditable: one shared polite HTTP client, checkpoint after every target,
// and a full per-target result trail.
package main

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"os"
	"strings"
	"time"

	"goodreads/internal/crawl"

	"github.com/dchooyc/book"
)

func main() {
	targetsPath := flag.String("targets", "topreads-missing-books-to-double-check.json", "missing-target input file")
	outPath := flag.String("out", "recovered-missing-books.json", "recovery result output file")
	reportPath := flag.String("report", "recovery-report.md", "markdown report output file")
	statePath := flag.String("state", "recovery-state.json", "checkpoint state file")
	priority := flag.String("priority", "all", "priority filter: all, P0, P1, P2, P3")
	limit := flag.Int("limit", 0, "optional dry batch size (0 = no limit)")
	workers := flag.Int("workers", 1, "workers (requests are globally rate-limited; kept for compatibility)")
	delay := flag.Duration("delay", 3*time.Second, "minimum delay between requests")
	timeout := flag.Duration("timeout", 30*time.Second, "per-request timeout")
	maxRetries := flag.Int("max-retries", 3, "max retries per URL")
	force := flag.Bool("force", false, "re-process targets already completed in the state file")
	keepPrevious := flag.Bool("keep-previous-on-fail", false, "fall back to the previous snapshot row when the page cannot be fetched")
	userAgent := flag.String("user-agent", crawl.DefaultUserAgent, "identifying User-Agent header")
	flag.Parse()

	if *workers > 1 {
		fmt.Println("note: requests are globally rate-limited by the polite client; running sequentially")
	}

	var targetsFile crawl.MissingTargetsFile
	if err := crawl.ReadJSONFile(*targetsPath, &targetsFile); err != nil {
		fail(err)
	}

	selected := crawl.FilterTargets(targetsFile.CheckTargets, *priority, *limit)
	fmt.Printf("targets: %d total, %d selected (priority=%s limit=%d)\n",
		len(targetsFile.CheckTargets), len(selected), *priority, *limit)

	state, err := crawl.LoadState(*statePath)
	if err != nil {
		fail(err)
	}

	// Keep results from earlier interrupted/resumed runs so the output file
	// stays complete.
	var previousResults []crawl.TargetResult
	var existing crawl.RecoveryOutput
	if err := crawl.ReadJSONFile(*outPath, &existing); err == nil {
		previousResults = existing.TargetResults
	} else if !errors.Is(err, fs.ErrNotExist) {
		fmt.Println("warning: could not read existing output, starting fresh:", err)
	}

	client := crawl.NewClient(*userAgent, *delay, *timeout, *maxRetries)

	currentResults := []crawl.TargetResult{}
	writeOutputs := func() crawl.RecoveryOutput {
		out := crawl.BuildRecoveryOutput(len(targetsFile.CheckTargets), previousResults, currentResults)
		if err := crawl.WriteJSONFileAtomic(*outPath, out); err != nil {
			fmt.Println("warning: writing output failed:", err)
		}
		if err := os.WriteFile(*reportPath, []byte(crawl.RenderRecoveryMarkdown(out)), 0o644); err != nil {
			fmt.Println("warning: writing report failed:", err)
		}
		return out
	}

	runner, err := crawl.NewRunner(crawl.RecoveryConfig{
		Fetcher:            client,
		State:              state,
		ParseBook:          parseBook,
		MeetsCriteria:      meetsCriteria,
		KeepPreviousOnFail: *keepPrevious,
		Force:              *force,
		OnResult: func(r crawl.TargetResult) {
			currentResults = append(currentResults, r)
			writeOutputs()
		},
	})
	if err != nil {
		fail(err)
	}

	_, runErr := runner.Run(selected)
	out := writeOutputs()

	fmt.Printf("\nsummary: recovered=%d kept=%d manual_review=%d failed=%d still_missing=%d\n",
		out.Summary.Recovered, out.Summary.KeptFromPreviousSnapshot,
		out.Summary.NeedsManualReview, out.Summary.Failed, out.Summary.StillMissing)
	fmt.Printf("wrote %s, %s, state in %s\n", *outPath, *reportPath, *statePath)

	if runErr != nil {
		if errors.Is(runErr, crawl.ErrBlocked) {
			fmt.Fprintln(os.Stderr, "recover-missing: hard stop — repeated 403/429 from server. Wait before resuming; the run is checkpointed.")
			os.Exit(2)
		}
		fail(runErr)
	}
}

func parseBook(body []byte) (*book.Book, error) {
	return book.GetBook(bytes.NewReader(body))
}

func meetsCriteria(b *book.Book) bool {
	return isEnglish(b.Title) && b.Ratings >= 500 && b.Rating >= 4.0
}

func isEnglish(text string) bool {
	for _, char := range text {
		if char > 255 {
			return false
		}
	}
	return len(strings.TrimSpace(text)) > 0
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "recover-missing:", err)
	os.Exit(1)
}
