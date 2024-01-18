package main

import (
	"flag"
	"fmt"
	"goodreads/book"
	"net/http"
	"strconv"
)

const (
	similarPrefix   = "https://www.goodreads.com/book/similar/"
	goodreadsPrefix = "https://www.goodreads.com"
)

func main() {
	rootUrl := flag.String("url", "https://www.goodreads.com", "The url to begin crawling from")
	maxDepth := flag.Int("depth", 2, "The depth at which to stop crawling")
	flag.Parse()
	books := bfs(*rootUrl, *maxDepth)
	for _, b := range books {
		fmt.Println(*b)
	}
}

func bfs(urlStr string, maxDepth int) []*book.Book {
	urlToBook := make(map[string]*book.Book)
	q := []string{urlStr}

	for i := 0; i < maxDepth; i++ {
		q2 := []string{}
		for _, url := range q {
			if _, ok := urlToBook[url]; !ok {
				b := getBook(url)
				if b != nil && len(b.ID) != 0 && i < maxDepth-1 {
					simBooks := similarPrefix + b.ID
					simBooksURLs := getBookURLs(simBooks)
					q2 = append(q2, simBooksURLs...)
				}
				urlToBook[url] = b
			}
		}
		fmt.Println("depth: " + strconv.Itoa(i))
		fmt.Println("books: " + strconv.Itoa(len(q)))
		q = q2
	}
	res, i := make([]*book.Book, len(urlToBook)), 0
	for _, b := range urlToBook {
		res[i] = b
		i++
	}
	return res
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
