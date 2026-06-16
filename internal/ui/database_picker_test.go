package ui

import "testing"

func TestDatabasePickerShowAndSelect(t *testing.T) {
	var p DatabasePicker
	p.Show([]string{"myapp", "testdb", "analytics"}, true)

	if !p.IsVisible() {
		t.Error("expected picker to be visible")
	}
	if !p.MustChoose() {
		t.Error("expected mustChoose to be true")
	}

	// First item should be "analytics" (sorted alphabetically).
	if got := p.SelectedDatabase(); got != "analytics" {
		t.Errorf("expected 'analytics', got '%s'", got)
	}

	p.CursorDown()
	if got := p.SelectedDatabase(); got != "myapp" {
		t.Errorf("expected 'myapp', got '%s'", got)
	}

	p.CursorDown()
	if got := p.SelectedDatabase(); got != "testdb" {
		t.Errorf("expected 'testdb', got '%s'", got)
	}

	// Can't go past the last item.
	p.CursorDown()
	if got := p.SelectedDatabase(); got != "testdb" {
		t.Errorf("expected 'testdb' at bottom, got '%s'", got)
	}

	p.Hide()
	if p.IsVisible() {
		t.Error("expected picker to be hidden")
	}
}

func TestDatabasePickerFilter(t *testing.T) {
	var p DatabasePicker
	p.Show([]string{"myapp", "testdb", "analytics", "metrics"}, false)

	p.FilterAddChar("app")
	if got := p.SelectedDatabase(); got != "myapp" {
		t.Errorf("expected 'myapp', got '%s'", got)
	}

	// Multiple backspaces clear the filter and reset cursor.
	p.FilterBackspace()
	p.FilterBackspace()
	p.FilterBackspace()
	if p.filter != "" {
		t.Errorf("expected empty filter, got '%s'", p.filter)
	}
}

func TestDatabasePickerNoMatches(t *testing.T) {
	var p DatabasePicker
	p.Show([]string{"myapp", "testdb"}, true)

	p.FilterAddChar("zzz")
	if got := p.SelectedDatabase(); got != "" {
		t.Errorf("expected empty selection for no matches, got '%s'", got)
	}
}
