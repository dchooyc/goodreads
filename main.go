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
	similarPrefix   = "https://www.goodreads.com/book/similar/"
	goodreadsPrefix = "https://www.goodreads.com"
)

func main() {
	rootUrl := flag.String("url", goodreadsPrefix+"/book/show/4099.The_Pragmatic_Programmer", "The url to begin crawling from")
	genre := flag.String("genre", "computer-science", "A genre to target")
	maxDepth := flag.Int("depth", 2, "The depth at which to stop crawling")
	flag.Parse()

	file, err := os.Create("output.json")
	if err != nil {
		panic(err)
	}

	books := bfs(*rootUrl, *genre, *maxDepth)

	jsonData, err := json.Marshal(books)
	if err != nil {
		panic(err)
	}

	_, err = file.Write(jsonData)
	if err != nil {
		fmt.Println("writing to file: ", err)
	}
}

func bfs(urlStr, genre string, maxDepth int) book.Books {
	urlToBook := make(map[string]*book.Book)
	q := []string{urlStr}

	for i := 0; i < maxDepth; i++ {
		fmt.Println("depth: " + strconv.Itoa(i))
		fmt.Println("books: " + strconv.Itoa(len(q)))
		q2 := []string{}
		for _, url := range q {
			if _, ok := urlToBook[url]; !ok {
				b := getBook(url)
				b.URL = url
				fmt.Println(b.Title)
				if b != nil && len(b.ID) != 0 && i < maxDepth-1 && contains(b.Genres, genre) {
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
	res := []book.Book{}
	for _, b := range urlToBook {
		if contains(b.Genres, genre) {
			res = append(res, *b)
		}
	}
	sort.Slice(res, func(i, j int) bool {
		return res[i].Ratings > res[j].Ratings
	})
	return book.Books{Books: res}
}

func contains(arr []string, target string) bool {
	for _, val := range arr {
		if val == target {
			return true
		}
	}
	return false
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
	for url := range fullURLs {
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
