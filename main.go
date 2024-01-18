package main

import (
	"flag"
	"fmt"
	"goodreads/book"
	"net/http"
)

func main() {
	rootUrl := flag.String("url", "https://goodreads.com", "The url to begin crawling from")
	// maxDepth := flag.Int("depth", 2, "The depth at which to stop crawling")
	// genre := flag.String("genre", "computer-science", "The genre of books to crawl for")
	flag.Parse()
	resp, err := http.Get(*rootUrl)
	if err != nil {
		panic(err)
	}
	defer resp.Body.Close()
	// body, err := io.ReadAll(resp.Body)
	// fmt.Println(string(body))
	books, err := book.Parse(resp.Body)
	if err != nil {
		fmt.Println(err)
	}
	for _, book := range books {
		fmt.Println(*book)
	}
}
