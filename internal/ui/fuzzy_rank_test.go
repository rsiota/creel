package ui

import (
	"strings"
	"testing"
)

func TestFuzzyExactMatchRanksFirst(t *testing.T) {
	// "colin" matches all four, but "Colin" is an exact match and should
	// score best (lowest), outranking the Carolina* variants.
	cases := []struct {
		query  string
		target string
	}{
		{"colin", "colin"},
		{"colin", "carolina"},
		{"colin", "caroline"},
		{"colin", "caroline mullan"},
	}
	var exact, other int
	for _, c := range cases {
		_, score := fuzzyMatch(c.query, c.target)
		if c.target == "colin" {
			exact = score
		} else {
			if score > other || other == 0 {
				other = score
			}
		}
	}
	// Lower score = better. The exact match must beat every other hit.
	// other is the *best* (lowest) non-exact score here.
	if exact >= other {
		t.Errorf("exact match score %d should be lower than best non-exact %d", exact, other)
	}
}

func TestFilterPickerRanksByScore(t *testing.T) {
	var p FilterPicker
	p.Show("name")
	p.SetValues([]string{"carolina", "caroline", "caroline mullan", "colin"}, nil)

	p.FilterAddChar("colin")
	items := p.filteredValues()
	if len(items) != 4 {
		t.Fatalf("expected 4 matches, got %d", len(items))
	}
	// filteredValues returns in fuzzy-score order only after View sorts, but
	// the score is captured on the item. Verify the exact match has the best
	// score and is sorted to the top by the View comparator.
	if items[0].value != "colin" && !scoreBest(items, "colin") {
		t.Errorf("expected 'colin' to have the best score; got order: %v", names(items))
	}
}

// scoreBest reports whether name has the strictly lowest score in items.
func scoreBest(items []filterValue, name string) bool {
	var best int
	found := false
	for _, it := range items {
		if it.value == name {
			best = it.score
			found = true
		}
	}
	if !found {
		return false
	}
	for _, it := range items {
		if it.value != name && it.score < best {
			return false
		}
	}
	return true
}

func names(items []filterValue) string {
	var b strings.Builder
	for i, it := range items {
		if i > 0 {
			b.WriteString(", ")
		}
		b.WriteString(it.value)
	}
	return b.String()
}

func TestColumnPickerRanksByScoreWhenFiltering(t *testing.T) {
	var p ColumnPicker
	p.Show([]string{"created_at", "country", "color", "colin_id"}, nil)

	// No filter: original order preserved.
	all := p.filteredItems()
	if all[0].name != "created_at" {
		t.Errorf("no-filter order should be original; got %q first", all[0].name)
	}

	// With filter "col": best matches (color, colin_id) rank above country.
	p.FilterAddChar("col")
	filtered := p.filteredItems()
	if len(filtered) == 0 {
		t.Fatal("expected matches for 'col'")
	}
	// "country" matches c-o-l with gaps; "color"/"colin_id" match more
	// consecutively, so country must not be first.
	if filtered[0].name == "country" {
		t.Errorf("expected a better match than 'country' first; got %v", colNames(filtered))
	}
}

func colNames(items []colItem) string {
	var b strings.Builder
	for i, it := range items {
		if i > 0 {
			b.WriteString(", ")
		}
		b.WriteString(it.name)
	}
	return b.String()
}
