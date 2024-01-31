package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"sort"
	"strconv"
	"sync"

	"github.com/dchooyc/book"
)

const (
	goodreadsPrefix     = "https://www.goodreads.com"
	similarPath         = "/book/similar/"
	pragmaticProgrammer = goodreadsPrefix + "/book/show/4099.The_Pragmatic_Programmer"
	out                 = "output.json"
	in                  = "input.json"
)

type processedBook struct {
	book         *book.Book
	err          error
	similarBooks []string
}

func main() {
	root := flag.String("url", pragmaticProgrammer, "The url to begin crawling from")
	input := flag.String("input", in, "Input location")
	output := flag.String("output", out, "Output location")
	maxDepth := flag.Int("depth", 2, "The depth at which to stop crawling")
	numWorkers := flag.Int("workers", 20, "The number of workers to process books")
	flag.Parse()

	file, err := os.Create(*output)
	if err != nil {
		panic(err)
	}

	urlToBook, err := retrieveFile(*input)
	if err != nil {
		fmt.Printf("retrieve file failed: %s", *input)
		urlToBook = make(map[string]*book.Book)
	}

	queue := createQueue(urlToBook, *root)
	findBooks(queue, urlToBook, *maxDepth, *numWorkers)
	books := arrangeBooks(urlToBook)

	jsonData, err := json.Marshal(books)
	if err != nil {
		panic(err)
	}

	_, err = file.Write(jsonData)
	if err != nil {
		fmt.Println("writing to file: ", err)
	}
}

func createQueue(urlToBook map[string]*book.Book, root string) []string {
	bookIDs := make(chan string, len(urlToBook))
	urls := make(chan string, len(urlToBook))
	var wg sync.WaitGroup

	for i := 0; i < 20; i++ {
		go func(workerID int) {
			for bookID := range bookIDs {
				bookURLs, err := getBookURLs(bookID)
				if err != nil {
					formattedError := fmt.Errorf("error getting similar books %s: %w", bookID, err)
					fmt.Println(formattedError)
					continue
				}

				if bookURLs != nil {
					fmt.Printf("Worker %d: %s\n", workerID, bookID)
				}

				for _, bookURL := range bookURLs {
					urls <- bookURL
				}

				wg.Done()
			}
		}(i)
	}

	for _, b := range urlToBook {
		cur := *b
		wg.Add(1)
		bookIDs <- cur.ID
	}

	close(bookIDs)

	go func() {
		wg.Wait()
		close(urls)
	}()

	queue := []string{}

	for url := range urls {
		if _, ok := urlToBook[url]; !ok {
			queue = append(queue, url)
		}
	}

	if _, ok := urlToBook[root]; !ok {
		queue = append(queue, root)
	}

	return queue
}

func retrieveFile(target string) (map[string]*book.Book, error) {
	file, err := os.Open(target)
	if err != nil {
		return nil, fmt.Errorf("open file failed: %w", err)
	}
	defer file.Close()

	bytes, err := io.ReadAll(file)
	if err != nil {
		return nil, fmt.Errorf("read file failed: %w", err)
	}

	var books book.Books
	err = json.Unmarshal(bytes, &books)
	if err != nil {
		return nil, fmt.Errorf("unmarshal json failed: %w", err)

	}

	urlToBook := make(map[string]*book.Book)

	for _, b := range books.Books {
		cur := b
		urlToBook[cur.URL] = &cur
	}

	return urlToBook, nil
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

func findBooks(queue []string, urlToBook map[string]*book.Book, maxDepth, numWorkers int) {
	for i := 1; i <= maxDepth; i++ {
		fmt.Println("depth: " + strconv.Itoa(i))
		fmt.Println("books: " + strconv.Itoa(len(queue)))
		isLast := false

		if i == maxDepth {
			isLast = true
		}

		queue = processQueue(isLast, numWorkers, queue, urlToBook)
	}
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
	rating := curBook.Rating >= 4.0
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
