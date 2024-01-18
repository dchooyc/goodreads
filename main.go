package main

import (
	"flag"
	"fmt"
	"goodreads/book"
	"net/http"
)

// https://www.goodreads.com/book/similar/372997-de-vita-caesarum

func main() {
	rootUrl := flag.String("url", "https://goodreads.com", "The url to begin crawling from")
	// maxDepth := flag.Int("depth", 2, "The depth at which to stop crawling")
	// genre := flag.String("genre", "computer-science", "The genre of books to crawl for")
	flag.Parse()
	book := getBook(*rootUrl)
	fmt.Println(*book)
}

func getBook(urlString string) *book.Book {
	resp, err := http.Get(urlString)
	if err != nil {
		panic(err)
	}
	defer resp.Body.Close()
	book, err := book.Parse(resp.Body)
	if err != nil {
		fmt.Println(err)
	}
	return book
}
