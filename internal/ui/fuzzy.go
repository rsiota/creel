package ui

import (
	"sort"
	"strings"
)

// fuzzyMatch performs case-insensitive subsequence matching.
// Returns matched rune indices (nil if no match) and a score (lower = better).
func fuzzyMatch(query, s string) ([]int, int) {
	if query == "" {
		return nil, 0
	}
	q := []rune(strings.ToLower(query))
	target := []rune(strings.ToLower(s))

	var indices []int
	qi := 0
	for si := 0; si < len(target) && qi < len(q); si++ {
		if target[si] == q[qi] {
			indices = append(indices, si)
			qi++
		}
	}
	if qi < len(q) {
		return nil, 0
	}

	// Score: lower = better. Penalize gaps, reward consecutive/boundary matches.
	score := len(target) // mild preference for shorter names
	for i := 1; i < len(indices); i++ {
		gap := indices[i] - indices[i-1] - 1
		score += gap * 3
		if indices[i] == indices[i-1]+1 {
			score -= 5
		}
	}
	for _, idx := range indices {
		if idx == 0 || target[idx-1] == '_' || target[idx-1] == '.' {
			score -= 2
		}
	}
	return indices, score
}

// fuzzyResult pairs an item with its fuzzy match metadata.
type fuzzyResult[T any] struct {
	Item     T
	Index    int // original position in the input slice
	MatchIdx []int
	Score    int
}

// fuzzyRank filters items whose extracted text fuzzy-matches the query and
// returns them ranked best-match-first. The query must be non-empty; callers
// handle the empty case themselves (typically returning all items in input
// order). Items that don't match are omitted. lessTie breaks score ties; pass
// nil to preserve input order on ties.
func fuzzyRank[T any](query string, items []T, textOf func(T) string, lessTie func(a, b fuzzyResult[T]) bool) []fuzzyResult[T] {
	var results []fuzzyResult[T]
	for i, it := range items {
		idx, score := fuzzyMatch(query, textOf(it))
		if idx == nil {
			continue
		}
		results = append(results, fuzzyResult[T]{Item: it, Index: i, MatchIdx: idx, Score: score})
	}
	sort.SliceStable(results, func(i, j int) bool {
		if results[i].Score != results[j].Score {
			return results[i].Score < results[j].Score
		}
		if lessTie != nil {
			return lessTie(results[i], results[j])
		}
		return false
	})
	return results
}
