package crawl

import (
	"strings"
	"unicode"

	"github.com/dchooyc/book"
)

// Confidence describes how sure we are that a parsed page is the same work
// as a recovery target.
type Confidence string

const (
	ConfidenceHigh         Confidence = "high"
	ConfidenceMedium       Confidence = "medium"
	ConfidenceManualReview Confidence = "needs_manual_review"
	ConfidenceRejected     Confidence = "rejected"
)

// IdentityResult is the outcome of validating a parsed book against a target.
type IdentityResult struct {
	Confidence       Confidence `json:"confidence"`
	ResolvedID       string     `json:"resolved_id"`
	FilledExpectedID bool       `json:"filled_expected_id"`
	Reason           string     `json:"reason"`
	Warnings         []string   `json:"warnings,omitempty"`
}

var diacriticFolds = map[rune]string{
	'á': "a", 'à': "a", 'â': "a", 'ä': "a", 'ã': "a", 'å': "a", 'ā': "a", 'ă': "a", 'ą': "a",
	'æ': "ae", 'ç': "c", 'ć': "c", 'č': "c", 'đ': "d", 'ď': "d",
	'é': "e", 'è': "e", 'ê': "e", 'ë': "e", 'ē': "e", 'ė': "e", 'ę': "e", 'ě': "e",
	'í': "i", 'ì': "i", 'î': "i", 'ï': "i", 'ī': "i", 'į': "i",
	'ł': "l", 'ñ': "n", 'ń': "n", 'ň': "n",
	'ó': "o", 'ò': "o", 'ô': "o", 'ö': "o", 'õ': "o", 'ø': "o", 'ō': "o", 'œ': "oe",
	'ř': "r", 'ś': "s", 'š': "s", 'ş': "s", 'ß': "ss",
	'ť': "t", 'ú': "u", 'ù': "u", 'û': "u", 'ü': "u", 'ū': "u", 'ů': "u",
	'ý': "y", 'ÿ': "y", 'ź': "z", 'ż': "z", 'ž': "z",
}

func normalize(s string) string {
	s = strings.ToLower(s)
	var b strings.Builder
	b.Grow(len(s))

	for _, r := range s {
		if fold, ok := diacriticFolds[r]; ok {
			b.WriteString(fold)
			continue
		}
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
			continue
		}
		b.WriteRune(' ')
	}

	return strings.Join(strings.Fields(b.String()), " ")
}

// NormalizeTitle lowercases, folds common diacritics, strips punctuation and
// collapses repeated whitespace.
func NormalizeTitle(s string) string { return normalize(s) }

// NormalizeAuthor applies the same normalization as NormalizeTitle.
func NormalizeAuthor(s string) string { return normalize(s) }

// TitleSimilarity returns a similarity ratio in [0, 1] between two titles
// based on Levenshtein distance over normalized forms.
func TitleSimilarity(a, b string) float64 {
	na, nb := NormalizeTitle(a), NormalizeTitle(b)
	if na == nb {
		return 1
	}
	if na == "" || nb == "" {
		return 0
	}

	dist := levenshtein([]rune(na), []rune(nb))
	longest := max(len([]rune(na)), len([]rune(nb)))
	return 1 - float64(dist)/float64(longest)
}

func levenshtein(a, b []rune) int {
	prev := make([]int, len(b)+1)
	cur := make([]int, len(b)+1)

	for j := 0; j <= len(b); j++ {
		prev[j] = j
	}

	for i := 1; i <= len(a); i++ {
		cur[0] = i
		for j := 1; j <= len(b); j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			cur[j] = min3(prev[j]+1, cur[j-1]+1, prev[j-1]+cost)
		}
		prev, cur = cur, prev
	}

	return prev[len(b)]
}

func min3(a, b, c int) int {
	if b < a {
		a = b
	}
	if c < a {
		a = c
	}
	return a
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// AuthorMatch reports whether the first old author matches any new author,
// or the first new author matches any old author, after normalization.
func AuthorMatch(oldAuthors, newAuthors []string) bool {
	if len(oldAuthors) == 0 || len(newAuthors) == 0 {
		return false
	}

	oldSet := make(map[string]bool, len(oldAuthors))
	for _, a := range oldAuthors {
		if n := NormalizeAuthor(a); n != "" {
			oldSet[n] = true
		}
	}

	for _, a := range newAuthors {
		if oldSet[NormalizeAuthor(a)] {
			return true
		}
	}

	return false
}

// ValidateRecoveredBook decides whether a parsed page confidently matches a
// recovery target. It never silently replaces the expected work ID with a
// different parsed ID.
func ValidateRecoveredBook(target RecoveryTarget, parsed book.Book) IdentityResult {
	titleExact := NormalizeTitle(target.Title) == NormalizeTitle(parsed.Title)
	titleSim := TitleSimilarity(target.Title, parsed.Title)
	authorOK := AuthorMatch(target.Authors, parsed.Authors)

	switch {
	case parsed.ID != "" && parsed.ID == target.ExpectedGoodreadsWorkID:
		return IdentityResult{
			Confidence: ConfidenceHigh,
			ResolvedID: parsed.ID,
			Reason:     "parsed work ID equals expected work ID",
		}

	case parsed.ID == "" && titleExact && authorOK:
		return IdentityResult{
			Confidence:       ConfidenceMedium,
			ResolvedID:       target.ExpectedGoodreadsWorkID,
			FilledExpectedID: true,
			Reason:           "blank parsed ID but exact normalized title and matching author",
			Warnings: []string{
				"parsed page had blank work ID; filled expected ID " + target.ExpectedGoodreadsWorkID + " from previous snapshot",
			},
		}

	case parsed.ID != "" && parsed.ID != target.ExpectedGoodreadsWorkID && titleExact && authorOK:
		return IdentityResult{
			Confidence: ConfidenceManualReview,
			ResolvedID: "",
			Reason: "parsed work ID " + parsed.ID + " differs from expected " +
				target.ExpectedGoodreadsWorkID + " despite matching title and author; possible edition replacement",
		}

	case titleSim >= 0.85 && authorOK:
		return IdentityResult{
			Confidence: ConfidenceManualReview,
			ResolvedID: "",
			Reason:     "title only approximately matches; author matches",
		}

	default:
		return IdentityResult{
			Confidence: ConfidenceRejected,
			ResolvedID: "",
			Reason:     "title and author do not match the expected work",
		}
	}
}
