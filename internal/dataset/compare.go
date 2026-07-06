package dataset

import (
	"sort"
	"strings"

	"goodreads/internal/identity"
	"goodreads/internal/model"

	"github.com/dchooyc/book"
)

// PriorityFor buckets a missing work by how important it was in the previous
// snapshot.
func PriorityFor(ratings, reviews int) string {
	switch {
	case ratings >= 100000 || reviews >= 10000:
		return "P0"
	case ratings >= 25000 || reviews >= 2500:
		return "P1"
	case ratings >= 5000 || reviews >= 500:
		return "P2"
	default:
		return "P3"
	}
}

func isReviewURL(url string) bool {
	return strings.Contains(url, "/review/")
}

// betterCanonicalRow reports whether row a (at original index ai) should be
// preferred over row b (at index bi) as the canonical row for a work ID.
func betterCanonicalRow(a, b book.Book, ai, bi int) bool {
	if ra, rb := isReviewURL(a.URL), isReviewURL(b.URL); ra != rb {
		return !ra
	}
	if a.Ratings != b.Ratings {
		return a.Ratings > b.Ratings
	}
	if a.Reviews != b.Reviews {
		return a.Reviews > b.Reviews
	}
	if ca, cb := a.CoverUrl != "", b.CoverUrl != ""; ca != cb {
		return ca
	}
	if len(a.Authors) != len(b.Authors) {
		return len(a.Authors) > len(b.Authors)
	}
	if len(a.Title) != len(b.Title) {
		return len(a.Title) > len(b.Title)
	}
	return ai < bi
}

type canonicalEntry struct {
	book     book.Book
	index    int
	aliasURL map[string]bool
}

// CanonicalizeBooks groups raw rows by nonblank work ID and deterministically
// selects one canonical row per work. It also returns, per ID, the alias URLs
// of the non-selected rows.
func CanonicalizeBooks(rows []book.Book) (map[string]book.Book, map[string][]string) {
	entries := make(map[string]*canonicalEntry)

	for i, row := range rows {
		if row.ID == "" {
			continue
		}
		e, ok := entries[row.ID]
		if !ok {
			entries[row.ID] = &canonicalEntry{book: row, index: i, aliasURL: map[string]bool{}}
			continue
		}
		if betterCanonicalRow(row, e.book, i, e.index) {
			if e.book.URL != "" && e.book.URL != row.URL {
				e.aliasURL[e.book.URL] = true
			}
			e.book, e.index = row, i
		} else if row.URL != "" && row.URL != e.book.URL {
			e.aliasURL[row.URL] = true
		}
	}

	canonical := make(map[string]book.Book, len(entries))
	aliases := make(map[string][]string, len(entries))
	for id, e := range entries {
		canonical[id] = e.book
		urls := make([]string, 0, len(e.aliasURL))
		for u := range e.aliasURL {
			urls = append(urls, u)
		}
		sort.Strings(urls)
		aliases[id] = urls
	}
	return canonical, aliases
}

// CompareResult is everything produced by comparing old and new outputs.
type CompareResult struct {
	Targets           model.MissingTargetsFile
	BlankIDCandidates []model.BlankIDCandidate
}

// CompareOutputs canonicalizes both outputs, computes the missing set
// (old IDs absent from new), prioritizes it, and reports possible same-title
// or blank-ID rows in the new raw output. Output ordering is deterministic.
func CompareOutputs(oldRows, newRows []book.Book, oldFile, newFile, generatedAt string) CompareResult {
	oldCanonical, oldAliases := CanonicalizeBooks(oldRows)
	newCanonical, _ := CanonicalizeBooks(newRows)

	// Index the new raw output for same-title / blank-ID detection.
	newByURL := make(map[string]bool, len(newRows))
	newByTitle := make(map[string][]book.Book)
	for _, row := range newRows {
		if row.URL != "" {
			newByURL[row.URL] = true
		}
		if t := identity.NormalizeTitle(row.Title); t != "" {
			newByTitle[t] = append(newByTitle[t], row)
		}
	}

	missingIDs := make([]string, 0)
	for id := range oldCanonical {
		if _, ok := newCanonical[id]; !ok {
			missingIDs = append(missingIDs, id)
		}
	}
	sort.Slice(missingIDs, func(i, j int) bool {
		a, b := oldCanonical[missingIDs[i]], oldCanonical[missingIDs[j]]
		if a.Ratings != b.Ratings {
			return a.Ratings > b.Ratings
		}
		return missingIDs[i] < missingIDs[j]
	})

	sharedCount := 0
	for id := range oldCanonical {
		if _, ok := newCanonical[id]; ok {
			sharedCount++
		}
	}

	priorityCounts := map[string]int{}
	matchCounts := map[string]int{
		"same_url_in_new_raw":                               0,
		"same_normalized_title_in_new_raw":                  0,
		"same_normalized_title_and_first_author_in_new_raw": 0,
		"has_blank_id_possible_match":                       0,
		"has_different_id_possible_match":                   0,
		"no_new_raw_match":                                  0,
	}

	targets := make([]model.RecoveryTarget, 0, len(missingIDs))
	candidates := make([]model.BlankIDCandidate, 0)

	for _, id := range missingIDs {
		old := oldCanonical[id]
		priority := PriorityFor(old.Ratings, old.Reviews)
		priorityCounts[priority]++

		targets = append(targets, model.RecoveryTarget{
			Priority:                priority,
			ExpectedGoodreadsWorkID: id,
			Title:                   old.Title,
			Authors:                 old.Authors,
			PrimaryOldURL:           old.URL,
			OldRating:               old.Rating,
			OldRatings:              old.Ratings,
			OldReviews:              old.Reviews,
			OldCoverURL:             old.CoverUrl,
			OldGenres:               old.Genres,
			OldAliasURLs:            oldAliases[id],
			RecoveryStatus:          "pending",
		})

		// Diagnose what the new raw output knows about this work.
		matched := false
		if newByURL[old.URL] {
			matchCounts["same_url_in_new_raw"]++
			matched = true
		}

		titleRows := newByTitle[identity.NormalizeTitle(old.Title)]
		if len(titleRows) > 0 {
			matchCounts["same_normalized_title_in_new_raw"]++
			matched = true
		}

		for _, row := range titleRows {
			authorOK := identity.AuthorMatch(old.Authors, row.Authors)
			if authorOK {
				matchCounts["same_normalized_title_and_first_author_in_new_raw"]++
			}

			switch {
			case row.ID == "" && authorOK:
				matchCounts["has_blank_id_possible_match"]++
				candidates = append(candidates, model.BlankIDCandidate{
					Title:                         row.Title,
					Authors:                       row.Authors,
					URL:                           row.URL,
					Ratings:                       row.Ratings,
					PossibleExpectedWorkIDFromOld: id,
					MatchConfidence:               "medium",
					Reason:                        "same normalized title and first author as missing old work",
				})
			case row.ID == "":
				candidates = append(candidates, model.BlankIDCandidate{
					Title:                         row.Title,
					Authors:                       row.Authors,
					URL:                           row.URL,
					Ratings:                       row.Ratings,
					PossibleExpectedWorkIDFromOld: id,
					MatchConfidence:               "low",
					Reason:                        "same normalized title as missing old work but author differs",
				})
			case row.ID != id && authorOK:
				matchCounts["has_different_id_possible_match"]++
			}
			break // count each target once against its best title match set
		}

		if !matched {
			matchCounts["no_new_raw_match"]++
		}
	}

	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].Ratings != candidates[j].Ratings {
			return candidates[i].Ratings > candidates[j].Ratings
		}
		return candidates[i].PossibleExpectedWorkIDFromOld < candidates[j].PossibleExpectedWorkIDFromOld
	})

	summary := model.MissingSummary{
		GeneratedAt:                  generatedAt,
		OldFile:                      oldFile,
		NewFile:                      newFile,
		ComparisonMethod:             "Canonicalize by nonblank Goodreads work ID; sort missing targets by previous ratings count descending.",
		OldRawRows:                   len(oldRows),
		NewRawRows:                   len(newRows),
		OldCanonicalBooks:            len(oldCanonical),
		NewCanonicalBooks:            len(newCanonical),
		SharedCanonicalBooks:         sharedCount,
		MissingCanonicalBooksToCheck: len(targets),
		NewCanonicalBooksNotInOld:    len(newCanonical) - sharedCount,
		MissingPriorityCounts:        priorityCounts,
		PossibleNewRawMatchCounts:    matchCounts,
		ImportantNote: "These books are not necessarily gone from Goodreads. They were present in the previous canonical " +
			"output and absent by work ID from the new canonical output. Some appear in the new raw output with blank or " +
			"different IDs, so the crawler/parser/recovery pipeline must verify and recover them carefully.",
	}

	return CompareResult{
		Targets:           model.MissingTargetsFile{Summary: summary, CheckTargets: targets},
		BlankIDCandidates: candidates,
	}
}
