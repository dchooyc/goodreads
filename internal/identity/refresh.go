package identity

import (
	"fmt"

	"github.com/dchooyc/book"
)

// RefreshBook merges a freshly parsed page into the stored book row. The
// stored identity is protected: a page whose work ID differs from the stored
// nonblank ID never overwrites the row, and a parse that lost fields never
// blanks them. Returns the refreshed row and a warning ("" when clean).
func RefreshBook(old book.Book, parsed *book.Book) (book.Book, string) {
	if parsed == nil {
		return old, "no parsed page; kept stored row"
	}
	if parsed.ID != "" && old.ID != "" && parsed.ID != old.ID {
		return old, fmt.Sprintf("parsed work ID %s differs from stored %s; kept stored row", parsed.ID, old.ID)
	}

	updated := old
	if parsed.Title != "" {
		updated.Title = parsed.Title
	}
	if parsed.CoverUrl != "" {
		updated.CoverUrl = parsed.CoverUrl
	}
	if len(parsed.Authors) > 0 {
		updated.Authors = parsed.Authors
	}
	if len(parsed.Genres) > 0 {
		updated.Genres = parsed.Genres
	}
	// Stats are refreshed together, and only when the parse actually found
	// them — a page that parsed with zero ratings is a parser miss, not a
	// book that lost every rating.
	if parsed.Ratings > 0 {
		updated.Rating = parsed.Rating
		updated.Ratings = parsed.Ratings
		updated.Reviews = parsed.Reviews
	} else if old.Ratings > 0 {
		return updated, "parsed page had no ratings; kept stored stats"
	}

	return updated, ""
}
