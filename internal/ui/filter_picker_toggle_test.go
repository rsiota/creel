package ui

import "testing"

// TestFilterPickerToggleSelectsHighlightedValue guards against the stale-sort
// bug where the cursor's displayed position (sorted by score) disagreed with
// the index used by ToggleSelected (unsorted). After typing "colin", "colin"
// must be both the top item AND the value that gets selected on space.
func TestFilterPickerToggleSelectsHighlightedValue(t *testing.T) {
	var p FilterPicker
	p.Show("name")
	p.SetValues([]string{"carolina", "caroline", "caroline mullan", "colin"}, nil)

	// Filter to "colin" — four matches, but "colin" ranks first by score.
	p.FilterAddChar("colin")

	items := p.filteredValues()
	if items[0].value != "colin" {
		t.Fatalf("expected 'colin' first after filter, got %q", items[0].value)
	}

	// Cursor sits at 0 (reset on FilterAddChar). Toggling must select the
	// value under the cursor — 'colin' — not whatever was at index 0 in the
	// original (unsorted) order ('carolina').
	p.ToggleSelected()

	selected := p.SelectedValues()
	if len(selected) != 1 {
		t.Fatalf("expected 1 selected value, got %v", selected)
	}
	if selected[0] != "colin" {
		t.Errorf("expected 'colin' selected, got %q", selected[0])
	}
}

// TestFilterPickerCursorDownThenToggleSelectsSecondItem ensures the cursor
// index stays consistent between navigation and toggle after sorting.
func TestFilterPickerCursorDownThenToggleSelectsSecondItem(t *testing.T) {
	var p FilterPicker
	p.Show("name")
	p.SetValues([]string{"carolina", "caroline", "colin"}, nil)

	p.FilterAddChar("colin")
	items := p.filteredValues()
	second := items[1].value

	p.CursorDown() // cursor -> 1
	p.ToggleSelected()

	selected := p.SelectedValues()
	if len(selected) != 1 || selected[0] != second {
		t.Errorf("expected %q selected, got %v", second, selected)
	}
}
