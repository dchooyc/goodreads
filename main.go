package main

import (
	"bytes"
	"flag"
	"fmt"
	"sort"
	"strconv"
	"sync"
	"time"

	"goodreads/internal/fetch"
	"goodreads/internal/store"

	"github.com/dchooyc/book"
)

const (
	goodreadsPrefix     = "https://www.goodreads.com"
	similarPath         = "/book/similar/"
	pragmaticProgrammer = goodreadsPrefix + "/book/show/4099.The_Pragmatic_Programmer"
	out                 = "output"
	in                  = "input.json"
)

// processedBook separates the detail-page outcome from the similar-books
// expansion outcome. A successful detail fetch must be saved even when the
// expansion fails; only detailErr prevents saving the book.
type processedBook struct {
	book         *book.Book
	detailErr    error
	expansionErr error
	similarBooks []string
	attemptedURL string
}

type bookFetcher func(url string) (*book.Book, error)
type similarFetcher func(id string) ([]string, error)

func main() {
	root := flag.String("url", pragmaticProgrammer, "The url to begin crawling from")
	input := flag.String("input", in, "Input location (file or output folder)")
	output := flag.String("output", out, "Output location (folder, or a .json file)")
	maxDepth := flag.Int("depth", 2, "The depth at which to stop crawling")
	numWorkers := flag.Int("workers", 2, "The number of workers to process books")
	delay := flag.Duration("delay", 2*time.Second, "Minimum delay between requests")
	timeout := flag.Duration("timeout", 30*time.Second, "Per-request timeout")
	maxRetries := flag.Int("max-retries", 3, "Max retries per URL")
	flushEvery := flag.Int("flush-every", 100, "checkpoint the output after this many new books (0 = only at the end)")
	userAgent := flag.String("user-agent", fetch.DefaultUserAgent, "User-Agent header")
	flag.Parse()

	retrieved, err := retrieveFile(*input)
	if err != nil {
		fmt.Printf("retrieve file failed: %s\n", *input)
	}

	client := fetch.NewClient(*userAgent, *delay, *timeout, *maxRetries)
	getBook := makeGetBook(client)
	getBookURLs := makeGetBookURLs(client)

	urlToBook := make(map[string]*book.Book)

	// flush checkpoints everything collected so far; a crash or kill loses at
	// most the books gathered since the previous flush.
	flush := func() {
		if err := store.WriteBooks(*output, arrangeBooks(urlToBook), store.DefaultChunkSize); err != nil {
			fmt.Println("checkpoint write failed:", err)
		}
	}

	queue := createQueue(retrieved, *root)
	findBooks(queue, urlToBook, *maxDepth, *numWorkers, getBook, getBookURLs, *flushEvery, flush)
	flush()
	fmt.Printf("wrote %d books to %s\n", len(arrangeBooks(urlToBook).Books), *output)
}

func createQueue(retrieved *book.Books, root string) []string {
	if retrieved == nil {
		return []string{root}
	}

	queue, seen := []string{}, make(map[string]bool)

	for i := 0; i < len(retrieved.Books); i++ {
		book := retrieved.Books[i]
		url := book.URL
		queue = append(queue, url)
		seen[url] = true
	}

	if !seen[root] {
		queue = append(queue, root)
	}

	return queue
}

func retrieveFile(target string) (*book.Books, error) {
	return store.ReadBooks(target)
}

func arrangeBooks(urlToBook map[string]*book.Book) book.Books {
	arranged := []book.Book{}

	for _, curBook := range urlToBook {
		if curBook != nil && meetsCriteria(curBook) {
			arranged = append(arranged, *curBook)
		}
	}

	sort.Slice(arranged, func(i, j int) bool {
		return arranged[i].Ratings > arranged[j].Ratings
	})

	return book.Books{Books: arranged}
}

func findBooks(queue []string, urlToBook map[string]*book.Book, maxDepth, numWorkers int, getBook bookFetcher, getBookURLs similarFetcher, flushEvery int, flush func()) {
	for i := 1; i <= maxDepth; i++ {
		fmt.Println("depth: " + strconv.Itoa(i))
		fmt.Println("books: " + strconv.Itoa(len(queue)))
		isLast := false

		if i == maxDepth {
			isLast = true
		}

		queue = processQueue(isLast, numWorkers, queue, urlToBook, getBook, getBookURLs, flushEvery, flush)
		if flush != nil {
			flush()
		}
	}
}

func processQueue(isLast bool, numWorkers int, queue []string, urlToBook map[string]*book.Book, getBook bookFetcher, getBookURLs similarFetcher, flushEvery int, flush func()) []string {
	urls := make(chan string, len(queue))
	processedBooks := make(chan *processedBook, len(queue))
	var wg sync.WaitGroup

	createWorkers(min(len(queue), numWorkers), isLast, urls, processedBooks, &wg, getBook, getBookURLs)

	for _, url := range queue {
		wg.Add(1)
		urls <- url
	}

	close(urls)

	go func() {
		wg.Wait()
		close(processedBooks)
	}()

	collect := make(map[string]bool)
	saved := 0

	for pBook := range processedBooks {
		if pBook.detailErr != nil {
			fmt.Println("detail failed:", pBook.detailErr)
			continue
		}

		// Expansion failure is reported but must not discard the book.
		if pBook.expansionErr != nil {
			fmt.Println("expansion failed (book kept):", pBook.expansionErr)
		}

		if pBook.book != nil {
			urlToBook[pBook.book.URL] = pBook.book
			saved++
			if flush != nil && flushEvery > 0 && saved%flushEvery == 0 {
				flush()
			}
		}

		for _, bookURL := range pBook.similarBooks {
			collect[bookURL] = true
		}
	}

	next := []string{}

	for url := range collect {
		if _, ok := urlToBook[url]; !ok {
			next = append(next, url)
		}
	}

	return next
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func createWorkers(numWorkers int, isLast bool, urls <-chan string, processedBooks chan<- *processedBook, wg *sync.WaitGroup, getBook bookFetcher, getBookURLs similarFetcher) {
	for w := 0; w < numWorkers; w++ {
		go worker(w, isLast, urls, processedBooks, wg, getBook, getBookURLs)
	}
}

func worker(workerID int, isLast bool, urls <-chan string, processedBooks chan<- *processedBook, wg *sync.WaitGroup, getBook bookFetcher, getBookURLs similarFetcher) {
	for url := range urls {
		pBook := processBook(isLast, url, getBook, getBookURLs)
		if pBook.book != nil {
			fmt.Printf("Worker %d: %s\n", workerID, pBook.book.Title)
		}
		processedBooks <- pBook
		wg.Done()
	}
}

func processBook(isLast bool, url string, getBook bookFetcher, getBookURLs similarFetcher) *processedBook {
	res := &processedBook{attemptedURL: url}

	curBook, err := getBook(url)
	if err != nil {
		res.detailErr = fmt.Errorf("error getting %s: %w", url, err)
		return res
	}

	curBook.URL = url
	res.book = curBook
	id := curBook.ID

	if id != "" && !isLast && meetsCriteria(curBook) {
		bookURLs, err := getBookURLs(id)
		if err != nil {
			res.expansionErr = fmt.Errorf("error getting similar books %s: %w", id, err)
			return res
		}

		res.similarBooks = bookURLs
	}

	return res
}

func meetsCriteria(curBook *book.Book) bool {
	english := isEnglish(curBook.Title)
	ratings := curBook.Ratings >= 500
	rating := curBook.Rating >= 4.0
	return english && ratings && rating
}

func isEnglish(text string) bool {
	for _, char := range text {
		if char > 255 {
			return false
		}
	}
	return true
}

func makeGetBookURLs(client fetch.Fetcher) similarFetcher {
	return func(id string) ([]string, error) {
		path := goodreadsPrefix + similarPath + id

		body, _, err := client.Fetch(path)
		if err != nil {
			return nil, fmt.Errorf("fetch failed: %w", err)
		}

		urls, err := book.GetBookURLs(bytes.NewReader(body))
		if err != nil {
			return nil, fmt.Errorf("get book urls failed: %w", err)
		}

		fullURLs := make(map[string]bool)

		for _, url := range urls {
			fullURLs[goodreadsPrefix+url] = true
		}

		bookURLs, i := make([]string, len(fullURLs)), 0

		for url := range fullURLs {
			bookURLs[i] = url
			i++
		}

		return bookURLs, nil
	}
}

func makeGetBook(client fetch.Fetcher) bookFetcher {
	return func(urlString string) (*book.Book, error) {
		body, _, err := client.Fetch(urlString)
		if err != nil {
			return nil, fmt.Errorf("fetch failed: %w", err)
		}

		curBook, err := book.GetBook(bytes.NewReader(body))
		if err != nil {
			return nil, fmt.Errorf("get book details failed: %w", err)
		}

		return curBook, nil
	}
}
