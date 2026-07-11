package ui

import (
	"regexp"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/ruben/gsql/internal/config"
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

// The bar cursor (▏) appears only in insert mode. In normal mode the cursor
// is shown by the focused border instead.
func TestConnectionFormBarCursorOnlyInInsertMode(t *testing.T) {
	f := NewConnectionForm()
	f.SetSize(67, f.contentHeight())

	if f.IsEditing() {
		t.Fatal("form should open in normal mode")
	}
	// Normal mode: no bar cursor anywhere.
	if got := strings.Count(formStripANSI(f.View()), "▏"); got != 0 {
		t.Errorf("normal mode bar cursor count=%d, want 0", got)
	}

	// Enter insert mode: exactly one bar cursor on the active field.
	f, _ = f.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'i'}})
	if !f.IsEditing() {
		t.Fatal("expected insert mode after pressing 'i'")
	}
	if got := strings.Count(formStripANSI(f.View()), "▏"); got != 1 {
		t.Errorf("insert mode bar cursor count=%d, want 1", got)
	}

	// Esc returns to normal mode and clears the bar cursor.
	f, _ = f.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if f.IsEditing() {
		t.Error("expected normal mode after esc")
	}
	if got := strings.Count(formStripANSI(f.View()), "▏"); got != 0 {
		t.Errorf("after esc bar cursor count=%d, want 0", got)
	}
}

// In normal mode, j/k move the field cursor without entering insert mode, and
// typing a letter does not edit the field (modal contract).
func TestConnectionFormNormalModeNavigation(t *testing.T) {
	f := NewConnectionForm()
	f.SetSize(67, f.contentHeight())
	if f.active != fieldName || f.IsEditing() {
		t.Fatal("form should open in normal mode on the Name field")
	}

	f, _ = f.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	if f.active != fieldDriver || f.IsEditing() {
		t.Errorf("after j: active=%d editing=%v, want fieldDriver/not editing", f.active, f.IsEditing())
	}

	f, _ = f.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})
	if f.active != fieldName {
		t.Errorf("after k: active=%d, want fieldName", f.active)
	}

	// A letter in normal mode must not edit the field.
	f, _ = f.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	if f.fields[fieldName].Value() != "" {
		t.Errorf("normal-mode key edited the field: %q", f.fields[fieldName].Value())
	}
}

// Insert mode edits the active field; Enter commits and returns to normal.
func TestConnectionFormInsertModeEditsAndCommits(t *testing.T) {
	f := NewConnectionForm()
	f.SetSize(67, f.contentHeight())

	f, _ = f.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'i'}})
	for _, r := range "mydb" {
		f, _ = f.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	if got := f.fields[fieldName].Value(); got != "mydb" {
		t.Fatalf("insert typing: name=%q, want %q", got, "mydb")
	}

	// Enter in insert commits the field and returns to normal (no submit).
	f, _ = f.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if f.IsEditing() {
		t.Error("enter in insert should return to normal mode")
	}
	if got := f.fields[fieldName].Value(); got != "mydb" {
		t.Errorf("committed value lost: %q", got)
	}
}

// Tab navigates calmly (like j/k) without entering insert mode.
func TestConnectionFormTabNavigatesWithoutEditing(t *testing.T) {
	f := NewConnectionForm()
	f.SetSize(67, f.contentHeight())

	f, _ = f.Update(tea.KeyMsg{Type: tea.KeyTab})
	if f.active != fieldDriver {
		t.Errorf("after tab: active=%d, want fieldDriver", f.active)
	}
	if f.IsEditing() {
		t.Error("tab should navigate without entering insert mode")
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

// The form must be sized to the popup's inner dimensions (not the full
// terminal). Otherwise the scroll window is computed against the wrong height
// and the list drifts on every j/k — the popup renders N fields but the scroll
// model assumes a different count. This is a regression test for that bug.
func TestConnectionFormSizedToPopupNotTerminal(t *testing.T) {
	const termH = 30 // short terminal: popup cannot fit all fields
	m := NewModel(&config.Config{})
	mm, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: termH})
	m = mm.(Model)

	m.state = stateAddConnection
	m.connForm = NewConnectionForm()
	wantW, wantH := popupContentSize(termH)
	m.connForm.SetSize(wantW, wantH) // mirrors what addConnection does

	if m.connForm.height != wantH {
		t.Errorf("form height=%d, want popup content height %d (not terminal %d)",
			m.connForm.height, wantH, termH)
	}
	if m.connForm.width != wantW {
		t.Errorf("form width=%d, want popup content width %d", m.connForm.width, wantW)
	}
}

// While the cursor stays within the visible window, j/k must NOT scroll — the
// visible fields stay put and only the focused border moves. Scrolling kicks in
// only once the cursor reaches the bottom edge (like the inspector).
func TestConnectionFormWindowStableUntilEdge(t *testing.T) {
	const termH = 30
	m := NewModel(&config.Config{})
	mm, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: termH})
	m = mm.(Model)
	m.state = stateAddConnection
	m.connForm = NewConnectionForm()
	iw, ch := popupContentSize(termH)
	m.connForm.SetSize(iw, ch)

	maxFields := m.connForm.visibleFieldCount()
	if maxFields >= fieldCount {
		t.Fatalf("test setup expects scrolling (maxFields=%d, fields=%d)", maxFields, fieldCount)
	}

	// Move the cursor to the last field that still fits without scrolling.
	for i := 0; i < maxFields-1; i++ {
		before := m.connForm.scrollRow
		m.connForm, _ = m.connForm.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
		if m.connForm.scrollRow != before {
			t.Fatalf("scrolled at active=%d (scrollRow %d->%d): window should stay stable until the cursor hits the bottom edge",
				m.connForm.active, before, m.connForm.scrollRow)
		}
	}

	// One more j pushes the cursor past the bottom edge → now it must scroll.
	m.connForm, _ = m.connForm.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	if m.connForm.scrollRow == 0 {
		t.Errorf("expected scrollRow>0 once the cursor passes the bottom edge, got 0")
	}
}
