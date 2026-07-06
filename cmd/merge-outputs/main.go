// Command merge-outputs merges the new crawler output with recovered books
// into output.merged.json, with provenance in merge-report.json. It never
// overwrites its inputs.
package main

import (
	"flag"
	"fmt"
	"os"

	"goodreads/internal/crawl"
)

func main() {
	oldPath := flag.String("old", "", "previous crawler output (optional, stats only)")
	newPath := flag.String("new", "output.json", "new crawler output")
	recoveredPath := flag.String("recovered", "recovered-missing-books.json", "recovery result file")
	outPath := flag.String("out", "output.merged.json", "merged output file")
	flag.Parse()

	newBooks, err := crawl.ReadBooksFile(*newPath)
	if err != nil {
		fail(err)
	}

	var recovered crawl.RecoveryOutput
	if err := crawl.ReadJSONFile(*recoveredPath, &recovered); err != nil {
		fail(err)
	}

	merged, report := crawl.MergeOutputs(newBooks.Books, recovered)

	if *oldPath != "" {
		if oldBooks, err := crawl.ReadBooksFile(*oldPath); err == nil {
			oldCanonical, _ := crawl.CanonicalizeBooks(oldBooks.Books)
			fmt.Printf("old canonical books: %d\n", len(oldCanonical))
		} else {
			fmt.Println("warning: could not read old output:", err)
		}
	}

	if err := crawl.WriteJSONFileAtomic(*outPath, merged); err != nil {
		fail(err)
	}

	s := report.Summary
	fmt.Printf("new canonical:        %d\n", s.NewCanonicalBooks)
	fmt.Printf("recovered books:      %d\n", s.RecoveredBooks)
	fmt.Printf("added by recovery:    %d (kept from previous: %d)\n", s.AddedByRecovery, s.KeptFromPrevious)
	fmt.Printf("replaced by recovery: %d\n", s.ReplacedByRecovered)
	fmt.Printf("blank-ID quarantined: %d of %d\n", s.BlankIDRowsQuarantined, s.BlankIDRowsInNew)
	fmt.Printf("final books:          %d\n", s.FinalBooks)
	fmt.Printf("wrote %s\n", *outPath)
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "merge-outputs:", err)
	os.Exit(1)
}
