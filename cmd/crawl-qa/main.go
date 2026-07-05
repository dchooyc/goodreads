// Command crawl-qa gates a crawl or recovery run: it compares a candidate
// output against the previous accepted output and exits nonzero when quality
// regressed (missing P0 works without a decision, blank-ID rate spike,
// canonical count drop, top-1000 disappearance).
package main

import (
	"flag"
	"fmt"
	"os"

	"goodreads/internal/crawl"
)

func main() {
	oldPath := flag.String("old", "output.json", "previous accepted output")
	newPath := flag.String("new", "output.merged.json", "candidate output to accept")
	recoveredPath := flag.String("recovered", "", "optional recovery result file (counts as recovery decisions)")
	jsonOut := flag.String("out-json", "crawl-quality-report.json", "JSON report path")
	mdOut := flag.String("out-md", "crawl-quality-report.md", "Markdown report path")
	flag.Parse()

	oldBooks, err := crawl.ReadBooksFile(*oldPath)
	if err != nil {
		fail(err)
	}
	newBooks, err := crawl.ReadBooksFile(*newPath)
	if err != nil {
		fail(err)
	}

	var recovery *crawl.RecoveryOutput
	if *recoveredPath != "" {
		var r crawl.RecoveryOutput
		if err := crawl.ReadJSONFile(*recoveredPath, &r); err != nil {
			fail(err)
		}
		recovery = &r
	}

	report := crawl.RunQA(oldBooks.Books, newBooks.Books, recovery)

	if err := crawl.WriteJSONFileAtomic(*jsonOut, report); err != nil {
		fail(err)
	}
	if err := os.WriteFile(*mdOut, []byte(crawl.RenderQAMarkdown(report)), 0o644); err != nil {
		fail(err)
	}

	m := report.Metrics
	fmt.Printf("raw rows: %d, canonical: %d, blank-ID rate: %.2f%%\n", m.RawRows, m.CanonicalNonblankIDs, m.BlankIDRate*100)
	fmt.Printf("old IDs missing from new: %d (P0: %d, undecided P0: %d)\n", m.OldIDsMissingFromNew, m.P0MissingCount, m.P0MissingUndecided)
	fmt.Printf("wrote %s and %s\n", *jsonOut, *mdOut)

	if !report.Passed {
		fmt.Fprintln(os.Stderr, "crawl-qa: FAILED")
		for _, f := range report.Failures {
			fmt.Fprintln(os.Stderr, " -", f)
		}
		os.Exit(1)
	}
	fmt.Println("crawl-qa: PASSED")
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "crawl-qa:", err)
	os.Exit(1)
}
