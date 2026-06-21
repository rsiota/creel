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

// TestFilterPickerSelectedBubbleToTopWithoutFilter guards the post-toggle UX:
// once a value is selected (and the filter is cleared by ToggleSelected), the
// selected item must appear at position 0 of filteredValues() so the user gets
// immediate visual feedback instead of having to scroll a long list to confirm.
func TestFilterPickerSelectedBubbleToTopWithoutFilter(t *testing.T) {
	var p FilterPicker
	p.Show("name")
	p.SetValues([]string{"alice", "bob", "carol", "dave"}, nil)

	// Walk down to "carol" (index 2 in canonical order) and toggle it.
	p.CursorDown()
	p.CursorDown()
	p.ToggleSelected()

	// ToggleSelected clears the filter, so without the sort fix the picker
	// would fall back to the original DB order and "carol" would sit at
	// index 2 — off-screen on a long list. It must now be at index 0.
	items := p.filteredValues()
	if len(items) == 0 {
		t.Fatal("expected filtered items, got none")
	}
	if items[0].value != "carol" {
		t.Errorf("expected 'carol' bubbled to index 0, got %q", items[0].value)
	}
	if !items[0].selected {
		t.Errorf("expected 'carol' to be selected")
	}

	// Cursor must also land on index 0 so the user sees their just-toggled
	// item under the cursor.
	if p.cursor != 0 {
		t.Errorf("expected cursor reset to 0, got %d", p.cursor)
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
