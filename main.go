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
)

const (
	goodreadsPrefix     = "https://www.goodreads.com"
	similarPath         = "/book/similar/"
	pragmaticProgrammer = goodreadsPrefix + "/book/show/4099.The_Pragmatic_Programmer"
	output              = "output.json"
)

func main() {
	rootUrl := flag.String("url", pragmaticProgrammer, "The url to begin crawling from")
	target := flag.String("target", output, "Output location")
	maxDepth := flag.Int("depth", 2, "The depth at which to stop crawling")
	flag.Parse()

	file, err := os.Create(*target)
	if err != nil {
		panic(err)
	}

	books := getBooks(*rootUrl, *maxDepth)

	jsonData, err := json.Marshal(books)
	if err != nil {
		panic(err)
	}

	_, err = file.Write(jsonData)
	if err != nil {
		fmt.Println("writing to file: ", err)
	}
}

func getBooks(urlStr string, maxDepth int) book.Books {
	urlToBook := make(map[string]*book.Book)
	queue, next := []string{urlStr}, []string{}

	for i := 1; i <= maxDepth; i++ {
		fmt.Println("depth: " + strconv.Itoa(i))
		fmt.Println("books: " + strconv.Itoa(len(queue)))
		isLast := false

		if i == maxDepth {
			isLast = true
		}

		processQueue(isLast, &queue, &next, urlToBook)
	}

	return arrangeBooks(urlToBook)
}

func arrangeBooks(urlToBook map[string]*book.Book) book.Books {
	arranged, i := make([]book.Book, len(urlToBook)), 0

	for _, curBook := range urlToBook {
		arranged[i] = *curBook
		i++
	}

	sort.Slice(arranged, func(i, j int) bool {
		return arranged[i].Ratings > arranged[j].Ratings
	})

	return book.Books{Books: arranged}
}

func processQueue(isLast bool, queue, next *[]string, urlToBook map[string]*book.Book) {
	for _, url := range *queue {
		if _, ok := urlToBook[url]; !ok {
			curBook := getBook(url)
			// error handling here
			if curBook == nil {
				continue
			}

			curBook.URL = url
			fmt.Println(curBook.Title)

			id := curBook.ID

			if id != "" && !isLast {
				getSimBooks(id, next, urlToBook)
			}

			urlToBook[url] = curBook
		}
	}

	*queue, *next = *next, []string{}
}

func getSimBooks(id string, next *[]string, urlToBook map[string]*book.Book) {
	path := goodreadsPrefix + similarPath + id
	simBooks, err := getBookURLs(path)
	if err != nil {
		fmt.Println("get similar books urls failed: ", err)
		return
	}

	for _, url := range simBooks {
		if _, ok := urlToBook[url]; !ok {
			*next = append(*next, url)
		}
	}
}

func getBookURLs(urlString string) ([]string, error) {
	resp, err := http.Get(urlString)
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

// to do: error handling here
func getBook(urlString string) *book.Book {
	resp, err := http.Get(urlString)
	if err != nil {
		panic(err)
	}
	defer resp.Body.Close()
	b, err := book.GetBook(resp.Body)
	if err != nil {
		fmt.Println(err)
	}
	return b
}
