package ui

import (
	"regexp"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/rsiota/creel/internal/config"
)

// formStripANSI removes ANSI escapes for structural assertions.
func formStripANSI(s string) string {
	return regexp.MustCompile("\x1b\\[[0-9;]*m").ReplaceAllString(s, "")
}

// countTopBorders counts field-box top borders ("┌"), one per rendered field.
func countTopBorders(s string) int {
	return strings.Count(s, "┌")
}

// formKey builds a single-rune key message for compact test input.
func formKey(r rune) tea.KeyMsg {
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}}
}

// The visible field set depends on the page and driver. Network drivers use a
// consistent 6 / 6 / 6 layout across Connection · SSH · Options:
//
//	Connection · sqlite           → 3  (Name, Driver, Database)
//	Connection · mysql/pg         → 6  (Name, Driver, Host, User, Pass, Database)
//	Options    · sqlite           → 3  (Secrets, Read-only, Group)
//	SSH        · mysql/pg         → 6  (Host, User, Pass, Key, Port, Passphrase)
//	Options    · mysql/pg         → 6  (Port, Socket, SSL, Secrets, Read-only, Group)
//
// An SSH tunnel is used when SSH Host is non-empty (no separate toggle).
func TestConnectionFormConditionalFields(t *testing.T) {
	f := NewConnectionForm()
	f.SetSize(67, f.contentHeight()) // tall enough to fit every visible field

	// --- Connection · sqlite ---
	view := formStripANSI(f.View())
	if got := countTopBorders(view); got != 3 {
		t.Errorf("sqlite connection: visible fields=%d, want 3\n%s", got, view)
	}
	tabs := f.availablePages()
	if len(tabs) != 2 || tabs[0] != formPageConnection || tabs[1] != formPageOptions {
		t.Errorf("sqlite tabs=%v, want Connection·Options", tabs)
	}
	for _, l := range []string{"Name", "Driver", "Database"} {
		if !strings.Contains(view, l) {
			t.Errorf("sqlite connection: %q should be visible", l)
		}
	}
	for _, l := range []string{"Host", "Port", "Username", "Password", "SSL", "Socket", "SSH Host", "Secrets", "Read-only", "Group"} {
		if strings.Contains(view, l) {
			t.Errorf("sqlite connection: %q should be hidden", l)
		}
	}

	// --- Options · sqlite ---
	f.setPage(formPageOptions)
	f.SetSize(67, f.contentHeight())
	view = formStripANSI(f.View())
	if got := countTopBorders(view); got != 3 {
		t.Errorf("sqlite options: visible fields=%d, want 3\n%s", got, view)
	}
	for _, l := range []string{"Secrets", "Read-only", "Group"} {
		if !strings.Contains(view, l) {
			t.Errorf("sqlite options: %q should be visible", l)
		}
	}
	if strings.Contains(view, "SSH Host") || strings.Contains(view, "Socket") || strings.Contains(view, "Port") {
		t.Error("sqlite options should not show network/SSH fields")
	}

	// --- Connection · mysql ---
	f.setPage(formPageConnection)
	f.fields[fieldDriver].SetValue("mysql")
	f.SetSize(67, f.contentHeight())
	view = formStripANSI(f.View())
	if got := countTopBorders(view); got != 6 {
		t.Errorf("mysql connection: visible fields=%d, want 6\n%s", got, view)
	}
	tabs = f.availablePages()
	if len(tabs) != 3 {
		t.Errorf("mysql tabs=%v, want Connection·SSH·Options", tabs)
	}
	wantConn := []string{"Name", "Driver", "Host", "Username", "Password", "Database"}
	for _, l := range wantConn {
		if !strings.Contains(view, l) {
			t.Errorf("mysql connection: %q should be visible", l)
		}
	}
	for i, fi := range f.connectionFields() {
		if formLabels[fi] != wantConn[i] {
			t.Errorf("connectionFields[%d]=%q, want %q", i, formLabels[fi], wantConn[i])
		}
	}
	for _, l := range []string{"Port", "SSL", "Socket", "SSH Host", "Secrets"} {
		if strings.Contains(view, l) {
			t.Errorf("mysql connection: %q should not be on Connection", l)
		}
	}

	// --- SSH · mysql ---
	f.setPage(formPageSSH)
	f.SetSize(67, f.contentHeight())
	view = formStripANSI(f.View())
	if got := countTopBorders(view); got != 6 {
		t.Errorf("mysql ssh: visible fields=%d, want 6\n%s", got, view)
	}
	wantSSH := []string{"SSH Host", "SSH User", "SSH Pass", "SSH Key", "SSH Port", "SSH Passphrase"}
	for _, l := range wantSSH {
		if !strings.Contains(view, l) {
			t.Errorf("mysql ssh: %q should be visible", l)
		}
	}
	for i, fi := range f.sshFields() {
		if formLabels[fi] != wantSSH[i] {
			t.Errorf("sshFields[%d]=%q, want %q", i, formLabels[fi], wantSSH[i])
		}
	}
	for _, l := range []string{"SSL", "Secrets", "Username", "SSH Tunnel"} {
		if strings.Contains(view, l) {
			t.Errorf("mysql ssh: %q should be hidden", l)
		}
	}

	// --- Options · mysql ---
	f.setPage(formPageOptions)
	f.SetSize(67, f.contentHeight())
	view = formStripANSI(f.View())
	if got := countTopBorders(view); got != 6 {
		t.Errorf("mysql options: visible fields=%d, want 6\n%s", got, view)
	}
	wantOpt := []string{"Port", "Socket", "SSL", "Secrets", "Read-only", "Group"}
	for _, l := range wantOpt {
		if !strings.Contains(view, l) {
			t.Errorf("mysql options: %q should be visible", l)
		}
	}
	for i, fi := range f.optionsFields() {
		if formLabels[fi] != wantOpt[i] {
			t.Errorf("optionsFields[%d]=%q, want %q", i, formLabels[fi], wantOpt[i])
		}
	}
	if strings.Contains(view, "SSH Host") || strings.Contains(view, "SSH Passphrase") {
		t.Error("mysql options should not show SSH host/passphrase")
	}
}

func TestConnectionFormPageSwapKeys(t *testing.T) {
	f := NewConnectionForm()
	f.fields[fieldDriver].SetValue("mysql")
	f.SetSize(67, 40)

	f, _ = f.Update(formKey(']'))
	if f.page != formPageSSH {
		t.Fatalf("] should open SSH, page=%d", f.page)
	}
	if !strings.Contains(formStripANSI(f.View()), "SSH Host") {
		t.Error("SSH page should show SSH Host")
	}

	f, _ = f.Update(formKey(']'))
	if f.page != formPageOptions {
		t.Fatalf("second ] should open Options, page=%d", f.page)
	}
	if !strings.Contains(formStripANSI(f.View()), "Secrets") {
		t.Error("Options page should show Secrets")
	}

	f, _ = f.Update(formKey(']'))
	if f.page != formPageConnection {
		t.Fatalf("third ] should wrap to Connection, page=%d", f.page)
	}

	f, _ = f.Update(formKey('['))
	if f.page != formPageOptions {
		t.Fatalf("[ should wrap to Options, page=%d", f.page)
	}
}

func TestConnectionFormSqliteHidesSSHTab(t *testing.T) {
	f := NewConnectionForm()
	f.SetSize(67, 40)
	f, _ = f.Update(formKey(']'))
	if f.page != formPageOptions {
		t.Fatalf("sqlite ] should skip SSH and open Options, page=%d", f.page)
	}
	f.setPage(formPageSSH) // unavailable
	if f.page != formPageOptions {
		t.Errorf("setPage(SSH) on sqlite should be no-op, page=%d", f.page)
	}
}

func TestConnectionFormValidationJumpsToPage(t *testing.T) {
	f := NewConnectionForm()
	f.fields[fieldDriver].SetValue("mysql")
	f.setPage(formPageConnection)
	f.fields[fieldPort].SetValue("nope")
	f.fields[fieldName].SetValue("x")

	_, errMsg := f.EnterPressed()
	if errMsg == "" {
		t.Fatal("expected port validation error")
	}
	if f.page != formPageOptions {
		t.Errorf("page=%d, want Options after port error", f.page)
	}
	if f.activeField() != fieldPort {
		t.Errorf("activeField=%d, want Port", f.activeField())
	}
}

// No field carries an "(optional)" marker anymore (it was pure noise).
func TestConnectionFormNoOptionalMarkers(t *testing.T) {
	for _, driver := range []string{"sqlite", "mysql", "postgres"} {
		f := NewConnectionForm()
		f.fields[fieldDriver].SetValue(driver)
		f.fields[fieldSSHHost].SetValue("bastion")
		f.SetSize(67, f.contentHeight())
		if got := strings.Count(formStripANSI(f.View()), "(optional)"); got != 0 {
			t.Errorf("%s: found %d '(optional)' markers, want 0", driver, got)
		}
	}
}

// Choice fields render as a cycling selector "< value >".
func TestConnectionFormSelectorRendering(t *testing.T) {
	f := NewConnectionForm()
	f.SetSize(67, f.contentHeight())
	view := formStripANSI(f.View())
	if !strings.Contains(view, "< sqlite >") {
		t.Errorf("driver selector not rendered: missing '< sqlite >'\n%s", view)
	}
}

// The insert-mode cursor is an underline overlay on the edited field — never
// an inserted "▏" glyph — so it shows only while editing and leaves no
// artifact when returning to normal mode.
func TestConnectionFormUnderlineCursorOnlyInInsertMode(t *testing.T) {
	f := NewConnectionForm()
	f.SetSize(67, f.contentHeight())

	if f.IsEditing() {
		t.Fatal("form should open in normal mode")
	}
	// Normal mode: no bar glyph, and the active field has no underline cursor.
	if got := strings.Count(formStripANSI(f.View()), "▏"); got != 0 {
		t.Errorf("normal mode bar glyph count=%d, want 0", got)
	}
	if hasUnderline(f.fieldValueContent(f.activeField(), 40)) {
		t.Error("normal mode field shows an underline cursor")
	}

	// Enter insert mode on the active field: underline cursor appears; no glyph.
	f, _ = f.Update(formKey('i'))
	if !f.IsEditing() {
		t.Fatal("expected insert mode after pressing 'i'")
	}
	if got := strings.Count(formStripANSI(f.View()), "▏"); got != 0 {
		t.Errorf("insert mode bar glyph count=%d, want 0", got)
	}
	if !hasUnderline(f.fieldValueContent(f.activeField(), 40)) {
		t.Error("insert mode field missing underline cursor")
	}

	// Esc returns to normal mode and clears the underline cursor.
	f, _ = f.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if f.IsEditing() {
		t.Error("expected normal mode after esc")
	}
	if hasUnderline(f.fieldValueContent(f.activeField(), 40)) {
		t.Error("field still shows underline cursor after esc")
	}
}

// In normal mode, j/k move the field cursor without entering insert mode, and
// typing a letter does not edit the field (modal contract).
func TestConnectionFormNormalModeNavigation(t *testing.T) {
	f := NewConnectionForm()
	f.SetSize(67, f.contentHeight())
	if f.active != 0 || f.activeField() != fieldName || f.IsEditing() {
		t.Fatal("form should open in normal mode on the Name field")
	}

	f, _ = f.Update(formKey('j'))
	if f.activeField() != fieldDriver || f.IsEditing() {
		t.Errorf("after j: activeField=%d editing=%v, want Driver/not editing", f.activeField(), f.IsEditing())
	}

	f, _ = f.Update(formKey('k'))
	if f.activeField() != fieldName {
		t.Errorf("after k: activeField=%d, want Name", f.activeField())
	}

	// A letter in normal mode must not edit the field.
	f, _ = f.Update(formKey('x'))
	if f.fields[fieldName].Value() != "" {
		t.Errorf("normal-mode key edited the field: %q", f.fields[fieldName].Value())
	}
}

// Insert mode edits the active free-text field; Enter commits and returns to normal.
func TestConnectionFormInsertModeEditsAndCommits(t *testing.T) {
	f := NewConnectionForm()
	f.SetSize(67, f.contentHeight())

	f, _ = f.Update(formKey('i'))
	for _, r := range "mydb" {
		f, _ = f.Update(formKey(r))
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
	if f.activeField() != fieldDriver {
		t.Errorf("after tab: activeField=%d, want Driver", f.activeField())
	}
	if f.IsEditing() {
		t.Error("tab should navigate without entering insert mode")
	}
}

// Tabbing past the bottom of a short, scrolling form scrolls the active field
// into view (scrollRow advances).
func TestConnectionFormTabScrollsActiveFieldIntoView(t *testing.T) {
	f := NewConnectionForm()
	f.fields[fieldDriver].SetValue("mysql") // 6 connection fields
	// Small area: effectiveHeight=13 -> maxFields = (13-tab-msg)/4 = 2.
	f.SetSize(67, 13)
	if f.visibleFieldCount() != 2 {
		t.Fatalf("visibleFieldCount=%d, want 2", f.visibleFieldCount())
	}
	if f.scrollRow != 0 {
		t.Fatalf("initial scrollRow=%d, want 0", f.scrollRow)
	}

	// Tab twice -> cursor at position 2, below the 2-field window -> scroll.
	for i := 0; i < 2; i++ {
		f, _ = f.Update(tea.KeyMsg{Type: tea.KeyTab})
	}
	if f.active != 2 {
		t.Fatalf("active=%d, want 2", f.active)
	}
	if f.scrollRow != 1 {
		t.Errorf("scrollRow=%d, want 1 after tabbing below the fold", f.scrollRow)
	}

	view := formStripANSI(f.View())
	if !strings.Contains(view, formLabels[f.activeField()]) {
		t.Errorf("active field %q not visible after scrolling", formLabels[f.activeField()])
	}
	if countTopBorders(view) != 2 {
		t.Errorf("visible fields=%d, want 2 (only the window should render)", countTopBorders(view))
	}
}

// Shift-Tab from the first field wraps to the last visible field and scrolls it in.
func TestConnectionFormShiftTabWrapsAndScrolls(t *testing.T) {
	f := NewConnectionForm()
	f.fields[fieldDriver].SetValue("mysql") // 6 connection fields
	f.SetSize(67, 13)                       // maxFields = 2

	f, _ = f.Update(tea.KeyMsg{Type: tea.KeyShiftTab})
	last := len(f.visibleFields()) - 1
	if f.active != last {
		t.Fatalf("active=%d, want last visible position %d", f.active, last)
	}
	if f.scrollRow <= 0 {
		t.Errorf("scrollRow=%d, want >0 after wrapping to the last field", f.scrollRow)
	}
}

// h/l cycle the Driver selector and the visible field set follows instantly.
func TestConnectionFormDriverSelectorCycling(t *testing.T) {
	f := NewConnectionForm()
	f.SetSize(67, fieldCount*linesPerField+1) // cap that always fits

	// Move to the Driver field.
	f, _ = f.Update(formKey('j'))
	if f.activeField() != fieldDriver {
		t.Fatalf("activeField=%d, want Driver", f.activeField())
	}

	// l: sqlite -> mysql (network connection fields appear).
	f, _ = f.Update(formKey('l'))
	if f.driver() != "mysql" {
		t.Errorf("after l: driver=%q, want mysql", f.driver())
	}
	if got := len(f.visibleFields()); got != 6 {
		t.Errorf("mysql connection visible fields=%d, want 6", got)
	}

	// l: mysql -> postgres.
	f, _ = f.Update(formKey('l'))
	if f.driver() != "postgres" {
		t.Errorf("after l: driver=%q, want postgres", f.driver())
	}

	// l: postgres -> sqlite (wraps; network fields disappear).
	f, _ = f.Update(formKey('l'))
	if f.driver() != "sqlite" {
		t.Errorf("after l: driver=%q, want sqlite (wrap)", f.driver())
	}
	if got := len(f.visibleFields()); got != 3 {
		t.Errorf("sqlite connection visible fields=%d, want 3", got)
	}

	// h: sqlite -> postgres (backward).
	f, _ = f.Update(formKey('h'))
	if f.driver() != "postgres" {
		t.Errorf("after h: driver=%q, want postgres", f.driver())
	}

	// On a free-text field, h/l must do nothing (selectors only).
	f2 := NewConnectionForm()
	f2.SetSize(67, f2.contentHeight())
	// Cursor is on Name (free text); l should not change anything.
	f2, _ = f2.Update(formKey('l'))
	if f2.fields[fieldName].Value() != "" {
		t.Errorf("l on a free-text field should be a no-op, got name=%q", f2.fields[fieldName].Value())
	}
}

// An SSH tunnel is active when SSH Host is set; clearing it turns the tunnel off.
func TestConnectionFormSSHEnabledFromHost(t *testing.T) {
	f := NewConnectionForm()
	f.fields[fieldDriver].SetValue("mysql")
	f.setPage(formPageSSH)
	f.SetSize(67, 40)

	if f.sshEnabled() {
		t.Fatal("empty SSH Host should mean no tunnel")
	}
	f.fields[fieldSSHHost].SetValue("bastion")
	if !f.sshEnabled() {
		t.Fatal("non-empty SSH Host should enable the tunnel")
	}
	f.fields[fieldSSHHost].SetValue("  ")
	if f.sshEnabled() {
		t.Fatal("whitespace-only SSH Host should mean no tunnel")
	}
}

// The form must be sized to the popup's inner dimensions (not the full
// terminal). Otherwise the scroll window is computed against the wrong height
// and the list drifts on every j/k. This is a regression test for that bug.
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
	m.connForm.fields[fieldDriver].SetValue("mysql")
	m.connForm.fields[fieldSSHHost].SetValue("bastion")
	m.connForm.setPage(formPageSSH) // 5 fields
	iw, _ := popupContentSize(termH)
	m.connForm.SetSize(iw, 13) // short viewport so the list must scroll

	visN := len(m.connForm.visibleFields())
	maxFields := m.connForm.visibleFieldCount()
	if maxFields >= visN {
		t.Fatalf("test setup expects scrolling (maxFields=%d, visible=%d)", maxFields, visN)
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
