// Command compare-missing compares the previous crawler output with a new
// one, canonicalized by Goodreads work ID, and writes a prioritized list of
// missing works to double-check plus a blank-ID candidate report.
package main

import (
	"flag"
	"fmt"
	"os"
	"time"

	"goodreads/internal/crawl"
)

func main() {
	oldPath := flag.String("old", "output.json", "previous crawler output")
	newPath := flag.String("new", "output(1).json", "new crawler output")
	outPath := flag.String("out", "topreads-missing-books-to-double-check.json", "target list output")
	blankOut := flag.String("blank-out", "blank_id_candidates.json", "blank-ID candidate report output")
	generatedAt := flag.String("generated-at", time.Now().UTC().Format("2006-01-02"), "generated_at stamp (fix for reproducible output)")
	flag.Parse()

	oldBooks, err := crawl.ReadBooksFile(*oldPath)
	if err != nil {
		fail(err)
	}
	newBooks, err := crawl.ReadBooksFile(*newPath)
	if err != nil {
		fail(err)
	}

	res := crawl.CompareOutputs(oldBooks.Books, newBooks.Books, *oldPath, *newPath, *generatedAt)

	if err := crawl.WriteJSONFileAtomic(*outPath, res.Targets); err != nil {
		fail(err)
	}
	if err := crawl.WriteJSONFileAtomic(*blankOut, res.BlankIDCandidates); err != nil {
		fail(err)
	}

	s := res.Targets.Summary
	fmt.Printf("old raw rows:        %d\n", s.OldRawRows)
	fmt.Printf("new raw rows:        %d\n", s.NewRawRows)
	fmt.Printf("old canonical:       %d\n", s.OldCanonicalBooks)
	fmt.Printf("new canonical:       %d\n", s.NewCanonicalBooks)
	fmt.Printf("shared canonical:    %d\n", s.SharedCanonicalBooks)
	fmt.Printf("missing to check:    %d %v\n", s.MissingCanonicalBooksToCheck, s.MissingPriorityCounts)
	fmt.Printf("blank-ID candidates: %d\n", len(res.BlankIDCandidates))
	fmt.Printf("wrote %s and %s\n", *outPath, *blankOut)
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "compare-missing:", err)
	os.Exit(1)
}
