package book

import (
	"fmt"
	"io"
	"strconv"
	"strings"

	"golang.org/x/net/html"
)

const (
	BookURLIndicator     = "/book/show/"
	BookIDIndicator      = "/work/quotes/"
	BookCoverIndicator   = "BookCover__image"
	BookAuthorsIndicator = "ContributorLinksList"
	BookGenresIndicator  = "/genres/"
	BookRatingIndicator  = "RatingStatistics__rating"
	BookStatsIndicator   = "RatingStatistics__meta"
)

type Book struct {
	Title    string
	ID       string
	CoverUrl string
	Authors  []string
	Genres   []string
	Rating   float64
	Ratings  int
	Reviews  int
}

func GetBookURLs(r io.Reader) ([]string, error) {
	bookURLs := []string{}
	doc, err := html.Parse(r)
	if err != nil {
		return nil, err
	}
	ExtractURLs(doc, &bookURLs)
	return bookURLs, nil
}

func ExtractURLs(n *html.Node, urls *[]string) {
	if n.Type == html.ElementNode && n.Data == "a" {
		for _, attr := range n.Attr {
			if attr.Key == "href" {
				url := attr.Val
				if strings.HasPrefix(url, BookURLIndicator) {
					*urls = append(*urls, url)
				}
				break
			}
		}
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		ExtractURLs(c, urls)
	}
}

func GetBook(r io.Reader) (*Book, error) {
	doc, err := html.Parse(r)
	if err != nil {
		return nil, err
	}
	book := CreateBook(doc)
	return book, nil
}

func CreateBook(n *html.Node) *Book {
	curBook := &Book{}
	ExtractBookInfo(n, curBook)
	return curBook
}

func ExtractBookInfo(n *html.Node, curBook *Book) {
	if n.Type == html.ElementNode && n.Data == "a" {
		ExtractID(n, curBook)
		ExtractGenres(n, curBook)
	}
	if n.Type == html.ElementNode && n.Data == "div" {
		ExtractCover(n, curBook)
		ExtractRating(n, curBook)
		ExtractStats(n, curBook)
		ExtractAuthors(n, curBook)
	}
	if n.Type == html.ElementNode && n.Data == "h1" {
		ExtractTitle(n, curBook)
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		ExtractBookInfo(c, curBook)
	}
}

func ExtractRating(n *html.Node, curBook *Book) {
	for _, attr := range n.Attr {
		if attr.Key == "class" && attr.Val == BookRatingIndicator {
			textNode := n.FirstChild
			if textNode != nil {
				val, err := strconv.ParseFloat(textNode.Data, 64)
				if err != nil {
					fmt.Println(err)
					break
				}
				curBook.Rating = val
			}
		}
	}
}

func ExtractStats(n *html.Node, curBook *Book) {
	correctClass, val := false, ""
	for _, attr := range n.Attr {
		if attr.Key == "class" && attr.Val == BookStatsIndicator {
			correctClass = true
		}
		if attr.Key == "aria-label" {
			val = attr.Val
		}
	}
	if correctClass {
		parts := strings.Split(val, " ")
		ratings := parts[0]
		reviews := parts[3]
		ratings = strings.Join(strings.Split(ratings, ","), "")
		reviews = strings.Join(strings.Split(reviews, ","), "")
		ratingsVal, err := strconv.Atoi(ratings)
		if err != nil {
			fmt.Println(err)
			return
		}
		curBook.Ratings = ratingsVal
		reviewsVal, err := strconv.Atoi(reviews)
		if err != nil {
			fmt.Println(err)
			return
		}
		curBook.Reviews = reviewsVal
	}
}

func ExtractGenres(n *html.Node, curBook *Book) {
	for _, attr := range n.Attr {
		if attr.Key == "href" {
			url := attr.Val
			if strings.Contains(url, BookGenresIndicator) {
				parts := strings.Split(url, "/")
				genre := parts[len(parts)-1]
				curBook.Genres = append(curBook.Genres, genre)
			}
			break
		}
	}
}

func ExtractAuthors(n *html.Node, curBook *Book) {
	for _, attr := range n.Attr {
		if attr.Key == "class" && attr.Val == BookAuthorsIndicator {
			authors := []string{}
			for c := n.FirstChild; c != nil; c = c.NextSibling {
				aNode := c.FirstChild
				if aNode == nil || aNode.Data != "a" {
					continue
				}
				spanNode := aNode.FirstChild
				if spanNode == nil || spanNode.Data != "span" {
					continue
				}
				name := spanNode.FirstChild
				if name.Type != html.TextNode {
					continue
				}
				authors = append(authors, name.Data)
			}
			if len(authors) != 0 {
				curBook.Authors = authors
			}
		}
	}
}

func ExtractCover(n *html.Node, curBook *Book) {
	for _, attr := range n.Attr {
		if attr.Key == "class" && attr.Val == BookCoverIndicator {
			targetDiv := n.FirstChild
			if targetDiv == nil {
				continue
			}
			imageNode := targetDiv.FirstChild
			if imageNode == nil || imageNode.Data != "img" {
				continue
			}
			correctClass, correctRole, imgSRC := false, false, ""
			for _, attr := range imageNode.Attr {
				if attr.Key == "class" && attr.Val == "ResponsiveImage" {
					correctClass = true
				}
				if attr.Key == "role" && attr.Val == "presentation" {
					correctRole = true
				}
				if attr.Key == "src" {
					imgSRC = attr.Val
				}
			}
			if correctClass && correctRole {
				curBook.CoverUrl = imgSRC
			}
		}
	}
}

func ExtractID(n *html.Node, curBook *Book) {
	for _, attr := range n.Attr {
		if attr.Key == "href" {
			url := attr.Val
			if strings.Contains(url, BookIDIndicator) {
				parts := strings.Split(url, "/")
				id := parts[len(parts)-1]
				curBook.ID = id
			}
			break
		}
	}
}

func ExtractTitle(n *html.Node, curBook *Book) {
	correctClass, correctData, title := false, false, ""
	for _, attr := range n.Attr {
		if attr.Key == "class" && attr.Val == "Text Text__title1" {
			correctClass = true
		}
		if attr.Key == "data-testid" && attr.Val == "bookTitle" {
			correctData = true
		}
		if attr.Key == "aria-label" {
			title = attr.Val
		}
	}
	if correctClass && correctData {
		curBook.Title = title
	}
}
