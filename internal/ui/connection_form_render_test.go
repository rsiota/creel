package ui

import (
	"regexp"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// formStripANSI removes ANSI escapes for structural assertions.
func formStripANSI(s string) string {
	return regexp.MustCompile("\x1b\\[[0-9;]*m").ReplaceAllString(s, "")
}

// countTopBorders counts field-box top borders ("┌"), one per rendered field.
func countTopBorders(s string) int {
	return strings.Count(s, "┌")
}

// On a tall enough area, every field renders (no scrolling).
func TestConnectionFormRendersAllFields(t *testing.T) {
	f := NewConnectionForm()
	f.SetSize(67, f.contentHeight()) // tall enough to fit everything

	if got := f.contentHeight(); got != fieldCount*linesPerField+1 {
		t.Fatalf("contentHeight=%d, want %d", got, fieldCount*linesPerField+1)
	}

	view := formStripANSI(f.View())
	if got := countTopBorders(view); got != fieldCount {
		t.Errorf("visible fields=%d, want %d (all fields should render)", got, fieldCount)
	}

	// Every label is present.
	for _, label := range formLabels {
		if !strings.Contains(view, label) {
			t.Errorf("label %q missing from form view", label)
		}
	}
}

// Optional fields show an "(optional)" marker; required fields do not.
func TestConnectionFormOptionalMarkers(t *testing.T) {
	f := NewConnectionForm()
	f.SetSize(67, f.contentHeight())

	view := formStripANSI(f.View())

	want := 0
	for _, opt := range formOptional {
		if opt {
			want++
		}
	}
	if got := strings.Count(view, "(optional)"); got != want {
		t.Errorf("(optional) count=%d, want %d", got, want)
	}
}

// The active field shows the bar cursor (▏); inactive fields do not.
func TestConnectionFormActiveFieldShowsBarCursor(t *testing.T) {
	f := NewConnectionForm()
	f.SetSize(67, f.contentHeight())

	view := formStripANSI(f.View())
	// Exactly one bar cursor for the single active field.
	if got := strings.Count(view, "▏"); got != 1 {
		t.Errorf("bar cursor count=%d, want 1 (only the active field)", got)
	}
}

// Tabbing past the bottom of a short, scrolling form scrolls the active field
// into view (scrollRow advances).
func TestConnectionFormTabScrollsActiveFieldIntoView(t *testing.T) {
	f := NewConnectionForm()
	// Small area: 12 fields-height -> maxFields = 3.
	f.SetSize(67, 13)
	if f.visibleFieldCount() != 3 {
		t.Fatalf("visibleFieldCount=%d, want 3", f.visibleFieldCount())
	}

	// Initially fields 0,1,2 are visible; active = 0.
	if f.scrollRow != 0 {
		t.Fatalf("initial scrollRow=%d, want 0", f.scrollRow)
	}

	// Tab three times -> active = fieldHost (3), which must scroll into view.
	for i := 0; i < 3; i++ {
		updated, _ := f.Update(tea.KeyMsg{Type: tea.KeyTab})
		f = updated
	}
	if f.active != fieldHost {
		t.Fatalf("active=%d, want fieldHost (%d)", f.active, fieldHost)
	}
	if f.scrollRow != 1 {
		t.Errorf("scrollRow=%d, want 1 after tabbing to a field below the fold", f.scrollRow)
	}

	// The active field's label must be visible in the rendered output.
	view := formStripANSI(f.View())
	if !strings.Contains(view, formLabels[fieldHost]) {
		t.Errorf("active field %q not visible after scrolling", formLabels[fieldHost])
	}
	if countTopBorders(view) != 3 {
		t.Errorf("visible fields=%d, want 3 (only the window should render)", countTopBorders(view))
	}
}

// Shift-Tab from the first field wraps to the last and scrolls it into view.
func TestConnectionFormShiftTabWrapsAndScrolls(t *testing.T) {
	f := NewConnectionForm()
	f.SetSize(67, 13) // maxFields = 3

	updated, _ := f.Update(tea.KeyMsg{Type: tea.KeyShiftTab})
	f = updated
	if f.active != fieldCount-1 {
		t.Fatalf("active=%d, want last field (%d)", f.active, fieldCount-1)
	}
	// Last field is beyond the first window, so scrollRow must advance.
	if f.scrollRow <= 0 {
		t.Errorf("scrollRow=%d, want >0 after wrapping to the last field", f.scrollRow)
	}
}
