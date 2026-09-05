package ui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/rsiota/creel/internal/config"
)

// TestHelpGettingStartedTab pins the default help page to a short onboarding guide.
func TestHelpGettingStartedTab(t *testing.T) {
	h := NewHelpPanel()
	h.Show()
	h.SetSize(120, 40)
	out := stripAnsi(h.View())
	for _, want := range []string{"Getting started", "Try the demo database", "ctrl+p", "ctrl+enter"} {
		if !strings.Contains(out, want) {
			t.Errorf("Start tab missing %q", want)
		}
	}
	if h.page != helpPageStart {
		t.Errorf("Show() page = %d, want helpPageStart", h.page)
	}
}

// TestHelpListsExCommands pins that the Commands tab of the "?" overlay folds
// the ":" command registry into its output (title + usages), so the help sheet
// is the single place to discover both keybindings and commands.
func TestHelpListsExCommands(t *testing.T) {
	h := NewHelpPanel()
	h.Show()
	h.page = helpPageCommands // switch to the Commands tab
	h.SetSize(140, 50)
	out := stripAnsi(h.View())
	if !strings.Contains(out, "Commands") {
		t.Error("help overlay should include a Commands section")
	}
	// View only shows the scrolled viewport; assert against the full command
	// row set so mid/late registry entries (e.g. :param) are covered too.
	var all strings.Builder
	for _, row := range renderCommandsRows(140) {
		for _, seg := range row {
			all.WriteString(seg.text)
		}
		all.WriteByte('\n')
	}
	body := all.String()
	for _, want := range []string{":goto <table>", ":export", ":backup", ":restore", ":locks", ":refs", ":begin", ":param[!] [name] [value…]"} {
		if !strings.Contains(body, want) {
			t.Errorf("help Commands rows should list %q", want)
		}
	}
	// Sanity: the visible page still renders at least one known early command.
	if !strings.Contains(out, ":goto <table>") {
		t.Error("help viewport should show early commands like :goto")
	}
}

// TestHelpTabSwitchAndScroll exercises the overlay's navigation: tab switches
// pages (the Keys page shows the "Keybindings" header; the Commands page shows
// usages like ":goto <table>" that the Keys page never has), and scrolling the
// Keys page moves the viewport.
func TestHelpTabSwitchAndScroll(t *testing.T) {
	h := NewHelpPanel()
	h.Show()
	h.SetSize(120, 40)

	start := stripAnsi(h.View())
	if !strings.Contains(start, "Getting started") {
		t.Error("Start page should show the Getting started header")
	}
	if strings.Contains(start, "Keybindings") {
		t.Error("Keybindings header should not appear on the Start page")
	}

	if !h.HandleKey(tea.KeyMsg{Type: tea.KeyTab}) {
		t.Fatal("tab should be consumed to switch help pages")
	}
	keys := stripAnsi(h.View())
	if !strings.Contains(keys, "Keybindings") {
		t.Error("Keys page should show the Keybindings header")
	}
	if strings.Contains(keys, ":goto <table>") {
		t.Error("command usages should not appear on the Keys page")
	}

	if !h.HandleKey(tea.KeyMsg{Type: tea.KeyTab}) {
		t.Fatal("tab should be consumed to switch help pages")
	}
	if !strings.Contains(stripAnsi(h.View()), ":goto <table>") {
		t.Error("Commands page should list command usages")
	}

	h.page = helpPageKeys
	h.keysOff = 0
	if !strings.Contains(stripAnsi(h.View()), "Global") {
		t.Error("Global should be visible at the top of the Keys page")
	}
	for i := 0; i < 3; i++ {
		h.HandleKey(tea.KeyMsg{Type: tea.KeyPgDown})
	}
	if strings.Contains(stripAnsi(h.View()), "Global") {
		t.Error("Global should have scrolled off the top after paging down")
	}
}

// TestHelpMouseScroll confirms the mouse-wheel scroll path (ScrollBy) moves the
// viewport in both directions and is clamped at the ends.
func TestHelpMouseScroll(t *testing.T) {
	h := NewHelpPanel()
	h.Show()
	h.SetSize(120, 40)
	h.page = helpPageKeys
	h.keysOff = 0
	if !strings.Contains(stripAnsi(h.View()), "Global") {
		t.Fatal("Global should be visible at the top initially")
	}
	h.ScrollBy(1000) // scroll far down
	if strings.Contains(stripAnsi(h.View()), "Global") {
		t.Error("Global should scroll off after scrolling down")
	}
	h.ScrollBy(-1000) // back to the top
	if !strings.Contains(stripAnsi(h.View()), "Global") {
		t.Error("Global should reappear after scrolling back to the top")
	}
}

// TestHelpCloseKeysDismisses confirms that close keys (esc/?/q) are NOT consumed
// by HandleKey (the caller hides the overlay), while nav keys are consumed.
func TestHelpCloseKeysDismisses(t *testing.T) {
	h := NewHelpPanel()
	h.Show()
	for _, k := range []string{"esc", "?", "q"} {
		km := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(k)}
		if k == "esc" {
			km = tea.KeyMsg{Type: tea.KeyEsc}
		}
		if h.HandleKey(km) {
			t.Errorf("close key %q should not be consumed (caller hides)", k)
		}
	}
	// An unmapped key is consumed so the overlay stays open (stray keys from
	// rapid mouse-wheel overscroll must not dismiss help).
	if !h.HandleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("x")}) {
		t.Error("unmapped key should be consumed (keep help open)")
	}
	// Nav keys are consumed.
	if !h.HandleKey(tea.KeyMsg{Type: tea.KeyTab}) {
		t.Error("tab should be consumed")
	}
}

// TestHelpConstantHeightAcrossTabs guards against the panel resizing when the
// page changes (which made the top border jump up/down on tab switch). Both
// pages must render the same number of lines.
func TestHelpConstantHeightAcrossTabs(t *testing.T) {
	h := NewHelpPanel()
	h.Show()
	h.SetSize(120, 40)
	heights := make([]int, helpPageCount)
	for p := 0; p < helpPageCount; p++ {
		h.page = p
		heights[p] = lipgloss.Height(h.View())
	}
	for i := 1; i < helpPageCount; i++ {
		if heights[i] != heights[0] {
			t.Errorf("help panel height differs across tabs: page0=%d page%d=%d (border would jump)",
				heights[0], i, heights[i])
		}
	}
}

// TestHelpHintsWhenVisible confirms the status bar surfaces help-specific
// keybindings while the overlay is open (instead of the workspace hints).
func TestHelpHintsWhenVisible(t *testing.T) {
	m := Model{help: NewHelpPanel()}
	m.help.Show()
	hints := m.hintList()
	joined := strings.Join(hints, " ")
	if !strings.Contains(joined, "tab") || !strings.Contains(joined, "?") {
		t.Errorf("help-visible hints should mention tab and ?: got %v", hints)
	}
}

// TestHelpTabClickSwitchesPage covers the mouse-click tab hit boxes: a click
// on each tab switches to it, and clicks off the tab row (or off any tab) do
// nothing.
func TestHelpTabClickSwitchesPage(t *testing.T) {
	m := Model{help: NewHelpPanel()}
	m.help.Show()
	m.width = 120
	pl := helpPanelLeft(m.width)

	for _, want := range []int{helpPageStart, helpPageKeys, helpPageCommands} {
		// Find an x coordinate that lands on tab `want`.
		x := -1
		for xx := pl; xx < pl+30; xx++ {
			if helpTabAt(pl, xx) == want {
				x = xx
				break
			}
		}
		if x < 0 {
			t.Fatalf("no click x found for tab %d", want)
		}
		mm, _ := m.handleHelpMouse(tea.MouseMsg{Type: tea.MouseLeft, X: x, Y: helpTabRow})
		if got := mm.(Model).help.page; got != want {
			t.Errorf("click on tab %d (x=%d) set page=%d", want, x, got)
		}
	}

	// A click off the tab row leaves the page unchanged.
	m.help.page = helpPageKeys
	mm, _ := m.handleHelpMouse(tea.MouseMsg{Type: tea.MouseLeft, X: 5, Y: 0})
	if mm.(Model).help.page != helpPageKeys {
		t.Error("click off the tab row should not switch pages")
	}
	// A click on the tab row but past the tabs also does nothing.
	mm, _ = m.handleHelpMouse(tea.MouseMsg{Type: tea.MouseLeft, X: 200, Y: helpTabRow})
	if mm.(Model).help.page != helpPageKeys {
		t.Error("click past the tabs should not switch pages")
	}
}

// TestHelpScrollAfterG is a regression guard. G used to set the offset to a
// huge sentinel (1<<30) that was only clamped at render time, so any
// up-scroll / j / k afterward computed (sentinel ± 1) and was re-clamped to
// the same bottom line — scrolling looked frozen until the offset climbed
// back into range. The offset is now clamped on write, so G lands on the real
// max and scrolling resumes immediately (and the reverse — scrolling to the
// bottom, then up — works too).
func TestHelpScrollAfterG(t *testing.T) {
	h := NewHelpPanel()
	h.Show()
	h.SetSize(120, 40)
	h.page = helpPageKeys
	h.keysOff = 0

	maxOff := h.maxOff()
	if maxOff <= 0 {
		t.Fatalf("Keys page should scroll at 120x40 (maxOff=%d)", maxOff)
	}
	key := func(s string) tea.KeyMsg { return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)} }

	// G lands exactly on the bottom — not a sentinel past the end.
	h.HandleKey(key("G"))
	if off := h.curOff(); off != maxOff {
		t.Errorf("after G: offset=%d, want maxOff=%d", off, maxOff)
	}
	// Up-scroll immediately after G must move (the bug: it was a no-op).
	h.HandleKey(key("k"))
	if off := h.curOff(); off >= maxOff {
		t.Errorf("after G then k: offset=%d, want < maxOff=%d (up-scroll frozen after G)", off, maxOff)
	}

	// The reverse: scroll to the bottom with PgDn, then up must still move.
	h.HandleKey(key("G")) // reset to bottom
	for i := 0; i < 20; i++ {
		h.HandleKey(tea.KeyMsg{Type: tea.KeyPgDown}) // hammered past the end
	}
	if off := h.curOff(); off != maxOff {
		t.Errorf("after scrolling past bottom: offset=%d, want clamped to maxOff=%d", off, maxOff)
	}
	h.HandleKey(key("k"))
	if off := h.curOff(); off >= maxOff {
		t.Errorf("after bottom then k: offset=%d, want < maxOff=%d (up-scroll frozen at bottom)", off, maxOff)
	}
}

// On a page short enough to fit without scrolling, G is a no-op at offset 0.
func TestHelpGOnPageThatFits(t *testing.T) {
	h := NewHelpPanel()
	h.Show()
	h.SetSize(120, 40)
	h.page = helpPageCommands // the short page; fits at this size
	if h.maxOff() != 0 {
		t.Skipf("Commands page scrolls at this size (maxOff=%d)", h.maxOff())
	}
	h.HandleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("G")})
	if off := h.curOff(); off != 0 {
		t.Errorf("G on a page that fits: offset=%d, want 0", off)
	}
}

// TestHelpScrollClampMatchesRender is an integration guard against the real
// bug behind "G then j/k freezes": the help overlay's size must be set on the
// PERSISTED model, not just on the throwaway copy a value-receiver view method
// renders. updateLayout sizes the panel on resize; if it didn't, help's stored
// width/height stayed 0, so maxOff() (used by the scroll handlers during
// Update) clamped to a different offset than View. That let j climb past the
// real bottom — the view froze, and k had to "burn" all the way back before
// anything moved (and G looked frozen for the same reason).
func TestHelpScrollClampMatchesRender(t *testing.T) {
	m := NewModel(&config.Config{})
	mm, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = mm.(Model)

	key := func(s string) tea.Msg { return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)} }
	step := func(msg tea.Msg) {
		mm, _ = m.Update(msg)
		m = mm.(Model)
		_ = m.View() // full render, like the real loop
	}

	step(key("?")) // open help
	if !m.help.IsVisible() {
		t.Fatal("help not visible")
	}
	m.help.page = helpPageKeys
	m.help.keysOff = 0
	// The persisted panel must know its size, so Update's maxOff matches View's.
	if m.help.height == 0 {
		t.Error("help panel height not persisted after resize (SetSize only hit a view copy)")
	}

	// Hold j well past the bottom, then a single k must move the viewport.
	for i := 0; i < 300; i++ {
		step(key("j"))
	}
	viewAtBottom := stripAnsi(m.View())
	step(key("k"))
	viewAfterK := stripAnsi(m.View())
	if viewAtBottom == viewAfterK {
		t.Error("view did not change after holding j past the bottom then pressing k once (scroll burn-through)")
	}
}

// TestHelpViewFillsScreen verifies the overlay renders edge-to-edge at the full
// terminal size (minus the status bar): width == h.width, height == h.height.
// Guards the lipgloss Width/Height-vs-border accounting (border is drawn
// outside the declared size, so the style is sized term-2 in each axis).
func TestHelpViewFillsScreen(t *testing.T) {
	h := NewHelpPanel()
	h.Show()
	h.SetSize(80, 23) // 23 == terminal height - 1 (status bar)
	out := h.View()
	if w := lipgloss.Width(out); w != 80 {
		t.Errorf("overlay width = %d, want 80", w)
	}
	if hh := lipgloss.Height(out); hh != 23 {
		t.Errorf("overlay height = %d, want 23", hh)
	}
}

// TestHelpStaysOpenOnWheelAndStrayKeys is a regression for help vanishing when
// flinging the mouse wheel to the top/bottom: wheel events must scroll (not
// dismiss), and any stray KeyMsg injected by overscroll must not close help.
func TestHelpStaysOpenOnWheelAndStrayKeys(t *testing.T) {
	m := NewModel(&config.Config{})
	mm, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = mm.(Model)

	mm, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("?")})
	m = mm.(Model)
	if !m.help.IsVisible() {
		t.Fatal("help not visible after ?")
	}

	// Fling toward the bottom, then the top.
	for i := 0; i < 80; i++ {
		mm, _ = m.Update(tea.MouseMsg{
			Type:   tea.MouseWheelDown,
			Button: tea.MouseButtonWheelDown,
			Action: tea.MouseActionPress,
			X:      40, Y: 20,
		})
		m = mm.(Model)
	}
	if !m.help.IsVisible() {
		t.Fatal("help closed while scrolling down")
	}
	for i := 0; i < 80; i++ {
		mm, _ = m.Update(tea.MouseMsg{
			Type:   tea.MouseWheelUp,
			Button: tea.MouseButtonWheelUp,
			Action: tea.MouseActionPress,
			X:      40, Y: 20,
		})
		m = mm.(Model)
	}
	if !m.help.IsVisible() {
		t.Fatal("help closed while scrolling up")
	}

	// Stray key that used to mean "any key dismisses".
	mm, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("x")})
	m = mm.(Model)
	if !m.help.IsVisible() {
		t.Fatal("help closed on unmapped key (overscroll noise)")
	}

	// Explicit close still works.
	mm, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = mm.(Model)
	if m.help.IsVisible() {
		t.Fatal("esc should still close help")
	}
}
