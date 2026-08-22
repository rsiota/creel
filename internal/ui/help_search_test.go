package ui

import (
	"regexp"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

func keyMsg(s string) tea.KeyMsg {
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
}

// searchText concatenates segment plain text so matching sees the real layout.
func TestHelpRowSearchText(t *testing.T) {
	row := helpRow{
		{text: "  ", style: lipgloss.NewStyle()},
		{text: "x", style: lipgloss.NewStyle().Foreground(colorLabel)},
		{text: "    ", style: lipgloss.NewStyle()},
		{text: "export to CSV", style: lipgloss.NewStyle().Foreground(colorFg)},
	}
	if got := row.searchText(); got != "  x    export to CSV" {
		t.Errorf("searchText=%q", got)
	}
}

// "/" opens typing mode and is consumed (the overlay stays open).
func TestHelpSearchOpensTyping(t *testing.T) {
	h := NewHelpPanel()
	h.Show()
	h.SetSize(120, 40)

	if !h.HandleKey(keyMsg("/")) {
		t.Fatal("/ should be consumed")
	}
	if !h.Typing() {
		t.Fatal("expected typing mode after /")
	}
}

// Typing a query compiles a regex and finds matches on the Keys page.
func TestHelpSearchTypingFindsMatches(t *testing.T) {
	h := NewHelpPanel()
	h.Show()
	h.SetSize(120, 40)

	h.HandleKey(keyMsg("/"))
	for _, r := range "export" {
		h.HandleKey(keyMsg(string(r)))
	}
	if h.query != "export" {
		t.Fatalf("query=%q want export", h.query)
	}
	if h.matchRe == nil {
		t.Fatal("matchRe should be compiled for a non-empty query")
	}
	if ms := h.matches(); len(ms) < 2 {
		t.Fatalf("expected >=2 'export' matches on Keys page, got %d", len(ms))
	}
}

// n advances the current match, scrolling it into view; N reverses; both wrap.
func TestHelpSearchAdvanceAndWrap(t *testing.T) {
	h := NewHelpPanel()
	h.Show()
	h.SetSize(120, 40)
	h.query = "export"
	h.rebuildMatchRe()
	ms := h.matches()
	if len(ms) < 2 {
		t.Fatalf("need >=2 matches, got %d", len(ms))
	}

	h.scrollToCurrentMatch() // matchIdx 0
	first := h.currentMatchLine()
	if first != ms[0] {
		t.Fatalf("first match line=%d want %d", first, ms[0])
	}

	// n×len wraps a full cycle back to the first match.
	for i := 0; i < len(ms); i++ {
		h.HandleKey(keyMsg("n"))
	}
	if got := h.currentMatchLine(); got != first {
		t.Errorf("after wrapping n×%d, match line=%d want %d", len(ms), got, first)
	}

	// One n moves to the second match; N reverses it.
	h.HandleKey(keyMsg("n"))
	second := h.currentMatchLine()
	if second != ms[1] {
		t.Errorf("after one n, match line=%d want %d", second, ms[1])
	}
	h.HandleKey(keyMsg("N"))
	if got := h.currentMatchLine(); got != first {
		t.Errorf("after N, match line=%d want %d (back to first)", got, first)
	}

	// N from the first wraps around to the last match.
	h.HandleKey(keyMsg("N"))
	if got := h.currentMatchLine(); got != ms[len(ms)-1] {
		t.Errorf("N wrap, match line=%d want %d (last)", got, ms[len(ms)-1])
	}
}

// n/N with no active search are no-ops but still consumed (keep help open).
func TestHelpSearchNoOpWithoutQuery(t *testing.T) {
	h := NewHelpPanel()
	h.Show()
	h.SetSize(120, 40)
	if !h.HandleKey(keyMsg("n")) {
		t.Error("n with no query should be consumed")
	}
	if !h.HandleKey(keyMsg("N")) {
		t.Error("N with no query should be consumed")
	}
}

// esc while typing clears the search and keeps the overlay open; a second esc
// (not typing) closes it.
func TestHelpSearchEscClearsThenCloses(t *testing.T) {
	h := NewHelpPanel()
	h.Show()
	h.SetSize(120, 40)
	h.HandleKey(keyMsg("/"))
	for _, r := range "export" {
		h.HandleKey(keyMsg(string(r)))
	}
	if !h.HandleKey(tea.KeyMsg{Type: tea.KeyEsc}) {
		t.Fatal("esc while typing should be consumed")
	}
	if h.Typing() || h.query != "" || h.matchRe != nil {
		t.Errorf("esc did not clear search: typing=%v query=%q", h.Typing(), h.query)
	}
	if h.HandleKey(tea.KeyMsg{Type: tea.KeyEsc}) {
		t.Error("esc while not typing should not be consumed (caller closes)")
	}
}

// After committing a search with enter (typing off, query still active), the
// first esc deactivates the search but keeps the panel open; only a second esc
// (no active search) closes it.
func TestHelpSearchCommittedEscClearsThenCloses(t *testing.T) {
	h := NewHelpPanel()
	h.Show()
	h.SetSize(120, 40)
	h.HandleKey(keyMsg("/"))
	for _, r := range "export" {
		h.HandleKey(keyMsg(string(r)))
	}
	if !h.HandleKey(keyMsg("enter")) {
		t.Fatal("enter should be consumed")
	}
	if h.Typing() || h.matchRe == nil {
		t.Fatal("enter should leave a committed (non-typing) search active")
	}

	// First esc: committed search active → clear it, keep the panel open.
	if !h.HandleKey(tea.KeyMsg{Type: tea.KeyEsc}) {
		t.Error("esc with an active search should be consumed (panel stays open)")
	}
	if h.matchRe != nil || h.query != "" {
		t.Errorf("esc did not clear committed search: query=%q", h.query)
	}

	// Second esc: no active search → not consumed (caller closes the panel).
	if h.HandleKey(tea.KeyMsg{Type: tea.KeyEsc}) {
		t.Error("esc with no search should not be consumed (caller closes)")
	}
}

// enter leaves typing mode but keeps the query active so n/N keep working.
func TestHelpSearchEnterKeepsQuery(t *testing.T) {
	h := NewHelpPanel()
	h.Show()
	h.SetSize(120, 40)
	h.HandleKey(keyMsg("/"))
	for _, r := range "tab" {
		h.HandleKey(keyMsg(string(r)))
	}
	if !h.HandleKey(keyMsg("enter")) {
		t.Fatal("enter should be consumed")
	}
	if h.Typing() {
		t.Error("enter should exit typing mode")
	}
	if h.query != "tab" {
		t.Errorf("query should persist after enter, got %q", h.query)
	}
	// n still advances after enter (when there's >1 match).
	if len(h.matches()) > 1 {
		before := h.currentMatchLine()
		h.HandleKey(keyMsg("n"))
		if h.currentMatchLine() == before {
			t.Error("n should advance after enter")
		}
	}
}

// Search is scoped to the current page: :goto usages appear on Commands only.
func TestHelpSearchCurrentPageOnly(t *testing.T) {
	h := NewHelpPanel()
	h.Show()
	h.SetSize(140, 50)

	h.page = helpPageKeys
	h.query = ":goto"
	h.rebuildMatchRe()
	if n := len(h.matches()); n != 0 {
		t.Errorf(":goto should not match the Keys page, got %d", n)
	}

	h.page = helpPageCommands
	if n := len(h.matches()); n == 0 {
		t.Error(":goto should match the Commands page")
	}
}

// Highlighting preserves the underlying text, and backgrounds only the
// matched substring — never the rest of the line (no full-line bar).
func TestHelpRenderHighlightPreservesText(t *testing.T) {
	row := helpRow{{text: "export to CSV", style: lipgloss.NewStyle().Foreground(colorFg)}}
	re := regexp.MustCompile("(?i)export")

	if got := stripAnsi(renderHelpRow(row, re, false)); !strings.Contains(got, "export to CSV") {
		t.Errorf("highlight lost text: %q", got)
	}
	// The current match does NOT paint the rest of the line: visible width is
	// just the text's (13), not padded out to fill the row.
	cur := renderHelpRow(row, re, true)
	if w := lipgloss.Width(cur); w != 13 {
		t.Errorf("current-match line width=%d, want 13 (no full-line bar)", w)
	}
	if got := stripAnsi(cur); !strings.Contains(got, "export to CSV") {
		t.Errorf("highlight lost text on current match: %q", got)
	}
}

// The status line reports a match count while searching, and reverts to the
// scroll readout (no "match n/N") without a search.
func TestHelpStatusLineMatchCount(t *testing.T) {
	readout := regexp.MustCompile(`match \d+/\d+`)
	h := NewHelpPanel()
	h.Show()
	h.SetSize(120, 40)

	h.query = "export"
	h.rebuildMatchRe()
	if !readout.MatchString(stripAnsi(h.View())) {
		t.Error("status line should show a 'match n/N' readout while searching")
	}

	h.clearSearch()
	if readout.MatchString(stripAnsi(h.View())) {
		t.Error("status line should not show a match readout without a search")
	}
}

// A typed query with no matches reports "no matches" in the status line.
func TestHelpStatusLineNoMatches(t *testing.T) {
	h := NewHelpPanel()
	h.Show()
	h.SetSize(120, 40)
	h.query = "zzqqxxnope"
	h.rebuildMatchRe()
	if !strings.Contains(stripAnsi(h.View()), "no matches") {
		t.Error("status line should report 'no matches'")
	}
}

// Searching scrolls the first match into the viewport (its line is visible).
// Going through the typing flow exercises the real afterQueryChange path.
func TestHelpSearchScrollsMatchIntoView(t *testing.T) {
	h := NewHelpPanel()
	h.Show()
	h.SetSize(120, 40)

	h.HandleKey(keyMsg("/"))
	for _, r := range "export" {
		h.HandleKey(keyMsg(string(r)))
	}
	ms := h.matches()
	if len(ms) == 0 {
		t.Fatal("expected matches")
	}
	target := ms[0]
	vp := h.scrollPage()
	off := h.curOff()
	if target < off || target >= off+vp {
		t.Errorf("first match line %d not in viewport [%d,%d)", target, off, off+vp)
	}
}
