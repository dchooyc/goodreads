// Command split-output converts a single large books JSON file into a folder
// of small chunk files (or re-chunks an existing folder), so GitHub never
// sees one oversized file.
package main

import (
	"flag"
	"fmt"
	"os"

	"goodreads/internal/store"
)

func main() {
	inPath := flag.String("in", "output.json", "books JSON file or folder to read")
	outDir := flag.String("out", "output", "output folder for chunk files")
	chunkSize := flag.Int("chunk", store.DefaultChunkSize, "books per chunk file")
	flag.Parse()

	books, err := store.ReadBooks(*inPath)
	if err != nil {
		fail(err)
	}

	if err := store.WriteBooksDir(*outDir, *books, *chunkSize); err != nil {
		fail(err)
	}

	chunks, err := store.ListBookChunks(*outDir)
	if err != nil {
		fail(err)
	}
	fmt.Printf("wrote %d books into %d chunks under %s/\n", len(books.Books), len(chunks), *outDir)
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "split-output:", err)
	os.Exit(1)
}
