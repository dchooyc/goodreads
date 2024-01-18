package book

import (
	"io"
	"strings"

	"golang.org/x/net/html"
)

const (
	BookIDIndicator      = "/work/quotes/"
	BookCoverIndicator   = "BookCover__image"
	BookAuthorsIndicator = "ContributorLinksList"
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

// to only run parse on a book url
func Parse(r io.Reader) ([]*Book, error) {
	doc, err := html.Parse(r)
	if err != nil {
		return nil, err
	}
	return FindBooks(doc)
}

func FindBooks(n *html.Node) ([]*Book, error) {
	// fmt.Printf("%#v\n", n)
	curBook := &Book{}
	ExtractBookInfo(n, curBook)
	return []*Book{curBook}, nil
}

func ExtractBookInfo(n *html.Node, curBook *Book) {
	if n.Type == html.ElementNode && n.Data == "a" {
		ExtractID(n, curBook)
	}
	if n.Type == html.ElementNode && n.Data == "div" {
		ExtractCover(n, curBook)
		ExtractAuthors(n, curBook)
	}
	if n.Type == html.ElementNode && n.Data == "h1" {
		ExtractTitle(n, curBook)
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		ExtractBookInfo(c, curBook)
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
