package main

import (
	"flag"
	"fmt"
	"goodreads/book"
	"net/http"
)

const (
	similarPrefix   = "https://www.goodreads.com/book/similar/"
	goodreadsPrefix = "https://www.goodreads.com"
)

func main() {
	rootUrl := flag.String("url", goodreadsPrefix, "The url to begin crawling from")
	// maxDepth := flag.Int("depth", 2, "The depth at which to stop crawling")
	// genre := flag.String("genre", "computer-science", "The genre of books to crawl for")
	flag.Parse()
	b := getBook(*rootUrl)
	if b != nil && len(b.ID) != 0 {
		simBooks := similarPrefix + b.ID
		simBooksURLs := getBookURLs(simBooks)
		fmt.Println(simBooksURLs)
		fmt.Println(*b)
	}
}

func getBookURLs(urlString string) []string {
	resp, err := http.Get(urlString)
	if err != nil {
		panic(err)
	}
	defer resp.Body.Close()
	urls, err := book.GetBookURLs(resp.Body)
	if err != nil {
		fmt.Println(err)
	}
	fullURLs := make(map[string]bool)
	for _, url := range urls {
		fullURLs[goodreadsPrefix+url] = true
	}
	res, i := make([]string, len(fullURLs)), 0
	for url, _ := range fullURLs {
		res[i] = url
		i++
	}
	return res
}

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
