package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"goodreads/book"
	"net/http"
	"os"
	"sort"
	"strconv"
	"sync"
)

const (
	goodreadsPrefix     = "https://www.goodreads.com"
	similarPath         = "/book/similar/"
	pragmaticProgrammer = goodreadsPrefix + "/book/show/4099.The_Pragmatic_Programmer"
	output              = "output.json"
)

type processedBook struct {
	book         *book.Book
	err          error
	similarBooks []string
}

func main() {
	rootUrl := flag.String("url", pragmaticProgrammer, "The url to begin crawling from")
	target := flag.String("target", output, "Output location")
	maxDepth := flag.Int("depth", 2, "The depth at which to stop crawling")
	numWorkers := flag.Int("workers", 20, "The number of workers to process books")
	flag.Parse()

	file, err := os.Create(*target)
	if err != nil {
		panic(err)
	}

	books := arrangeBooks(findBooks(*rootUrl, *maxDepth, *numWorkers))

	jsonData, err := json.Marshal(books)
	if err != nil {
		panic(err)
	}

	_, err = file.Write(jsonData)
	if err != nil {
		fmt.Println("writing to file: ", err)
	}
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

func findBooks(urlStr string, maxDepth, numWorkers int) map[string]*book.Book {
	urlToBook := make(map[string]*book.Book)
	queue := []string{urlStr}

	for i := 1; i <= maxDepth; i++ {
		fmt.Println("depth: " + strconv.Itoa(i))
		fmt.Println("books: " + strconv.Itoa(len(queue)))
		isLast := false

		if i == maxDepth {
			isLast = true
		}

		queue = processQueue(isLast, numWorkers, queue, urlToBook)
	}

	return urlToBook
}

func processQueue(isLast bool, numWorkers int, queue []string, urlToBook map[string]*book.Book) []string {
	urls := make(chan string, len(queue))
	processedBooks := make(chan *processedBook, len(queue))
	var wg sync.WaitGroup

	createWorkers(min(len(queue), numWorkers), isLast, urls, processedBooks, &wg)

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

	for pBook := range processedBooks {
		if pBook.err != nil {
			fmt.Println(pBook.err)
			continue
		}

		urlToBook[pBook.book.URL] = pBook.book

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

func createWorkers(numWorkers int, isLast bool, urls <-chan string, processedBooks chan<- *processedBook, wg *sync.WaitGroup) {
	for w := 0; w < numWorkers; w++ {
		go worker(w, isLast, urls, processedBooks, wg)
	}
}

// give worker more details
func worker(workerID int, isLast bool, urls <-chan string, processedBooks chan<- *processedBook, wg *sync.WaitGroup) {
	for url := range urls {
		pBook := processBook(isLast, url)
		// do proper logging here
		// add depth and count
		if pBook.book != nil {
			fmt.Printf("Worker %d: %s\n", workerID, pBook.book.Title)
		}
		processedBooks <- pBook
		wg.Done()
	}
}

func processBook(isLast bool, url string) *processedBook {
	res := &processedBook{}

	curBook, err := getBook(url)
	if err != nil {
		res.err = fmt.Errorf("error getting %s: %w", url, err)
		return res
	}

	curBook.URL = url
	res.book = curBook
	id := curBook.ID

	if id != "" && !isLast && meetsCriteria(curBook) {
		bookURLs, err := getBookURLs(id)
		if err != nil {
			res.err = fmt.Errorf("error getting similar books %s: %w", id, err)
			return res
		}

		res.similarBooks = bookURLs
	}

	return res
}

func meetsCriteria(curBook *book.Book) bool {
	english := isEnglish(curBook.Title)
	ratings := curBook.Ratings >= 500
	rating := curBook.Rating >= 3.5
	return english && ratings && rating
}

func isEnglish(text string) bool {
	for _, char := range text {
		if char > 127 {
			return false
		}
	}
	return true
}

func getBookURLs(id string) ([]string, error) {
	path := goodreadsPrefix + similarPath + id
	resp, err := http.Get(path)
	if err != nil {
		return nil, fmt.Errorf("http get failed: %w", err)
	}

	defer resp.Body.Close()

	urls, err := book.GetBookURLs(resp.Body)
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

func getBook(urlString string) (*book.Book, error) {
	resp, err := http.Get(urlString)
	if err != nil {
		return nil, fmt.Errorf("http get failed: %w", err)
	}

	defer resp.Body.Close()

	curBook, err := book.GetBook(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("get book details failed: %w", err)
	}

	return curBook, nil
}
