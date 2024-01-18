package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"goodreads/book"
	"net/http"
	"os"
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

	file, err := os.Create("output.json")
	if err != nil {
		panic(err)
	}

	books := bfs(*rootUrl, *maxDepth)

	jsonData, err := json.Marshal(books)
	if err != nil {
		panic(err)
	}

	_, err = file.Write(jsonData)
	if err != nil {
		fmt.Println("writing to file: ", err)
	}
}

func bfs(urlStr string, maxDepth int) book.Books {
	urlToBook := make(map[string]*book.Book)
	q := []string{urlStr}

	for i := 0; i < maxDepth; i++ {
		fmt.Println("depth: " + strconv.Itoa(i))
		fmt.Println("books: " + strconv.Itoa(len(q)))
		q2 := []string{}
		for _, url := range q {
			if _, ok := urlToBook[url]; !ok {
				b := getBook(url)
				fmt.Println(b.Title)
				if b != nil && len(b.ID) != 0 && i < maxDepth-1 {
					simBooks := similarPrefix + b.ID
					simBooksURLs := getBookURLs(simBooks)
					for _, sbu := range simBooksURLs {
						if _, ok := urlToBook[sbu]; !ok {
							q2 = append(q2, sbu)
						}
					}
				}
				urlToBook[url] = b
			}
		}
		q = q2
	}
	res, i := make([]book.Book, len(urlToBook)), 0
	for _, b := range urlToBook {
		res[i] = *b
		i++
	}
	return book.Books{Books: res}
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
