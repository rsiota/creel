package ui

import (
	"reflect"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/rsiota/creel/internal/config"
)

func TestPaletteOpenClose(t *testing.T) {
	var p palette
	if p.IsVisible() {
		t.Fatal("palette should start hidden")
	}
	p.Open(paletteJumpSrc{})
	if !p.IsVisible() {
		t.Fatal("palette should be visible after Open")
	}
	if p.input != "" {
		t.Fatal("input should be empty on open")
	}
	if len(p.items) == 0 {
		t.Fatal("palette should have items built from registry")
	}
	if len(p.filtered) != len(p.items) {
		t.Fatalf("filtered should equal items on open (%d vs %d)", len(p.filtered), len(p.items))
	}
	p.Hide()
	if p.IsVisible() {
		t.Fatal("palette should be hidden after Hide")
	}
}

func TestPaletteFuzzyFilter(t *testing.T) {
	var p palette
	p.Open(paletteJumpSrc{})

	// Type "export" — should match export-related bindings.
	simulateTyping(&p, "export")
	if len(p.filtered) == 0 {
		t.Fatal("expected matches for 'export'")
	}
	found := false
	for _, it := range p.filtered {
		if strings.Contains(strings.ToLower(it.desc), "export") {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("no filtered item mentions 'export'")
	}

	// Nonsense filter should produce no matches.
	p.input = ""
	p.refilter()
	simulateTyping(&p, "zzzzz")
	if len(p.filtered) != 0 {
		t.Fatalf("expected no matches for nonsense, got %d", len(p.filtered))
	}
}

func TestPaletteNavigation(t *testing.T) {
	var p palette
	p.Open(paletteJumpSrc{})
	initial := p.cursor

	p = updatePalette(t, p, tea.KeyMsg{Type: tea.KeyDown})
	if p.cursor != initial+1 {
		t.Fatalf("cursor should be %d after down, got %d", initial+1, p.cursor)
	}

	p = updatePalette(t, p, tea.KeyMsg{Type: tea.KeyUp})
	if p.cursor != initial {
		t.Fatalf("cursor should be %d after up, got %d", initial, p.cursor)
	}

	// Wrap from top to bottom.
	p.cursor = 0
	p = updatePalette(t, p, tea.KeyMsg{Type: tea.KeyUp})
	if p.cursor != len(p.filtered)-1 {
		t.Fatalf("cursor should wrap to %d, got %d", len(p.filtered)-1, p.cursor)
	}
}

func TestPaletteEscape(t *testing.T) {
	var p palette
	p.Open(paletteJumpSrc{})
	simulateTyping(&p, "hello")
	p, cmd := p.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if p.IsVisible() {
		t.Fatal("palette should be hidden after esc")
	}
	if cmd != nil {
		t.Fatal("cmd should be nil on esc")
	}
}

func TestPaletteEnterExecutableSingleKey(t *testing.T) {
	var p palette
	p.Open(paletteJumpSrc{})

	// Find a single-token binding (e.g. "ctrl+r" — refresh).
	target := -1
	for i, it := range p.items {
		if len(it.replay) == 1 && it.replay[0] == "ctrl+r" {
			target = i
			break
		}
	}
	if target == -1 {
		t.Fatal("could not find ctrl+r binding in palette items")
	}

	// Navigate to it.
	p.cursor = target
	seq := p.selectedReplay()
	if len(seq) != 1 || seq[0] != "ctrl+r" {
		t.Fatalf("expected replay [ctrl+r], got %v", seq)
	}

	p, cmd := p.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if p.IsVisible() {
		t.Fatal("palette should be hidden after enter")
	}
	if cmd == nil {
		t.Fatal("cmd should be non-nil to replay the key")
	}
}

func TestPaletteEnterNonExecutable(t *testing.T) {
	var p palette
	p.Open(paletteJumpSrc{})

	// Find a multi-token "alternative" binding (not executable): e.g. "g t / g T"
	// or "ctrl+e / \".
	target := -1
	for i, it := range p.items {
		if len(it.replay) == 0 {
			target = i
			break
		}
	}
	if target == -1 {
		t.Fatal("could not find a non-executable binding")
	}

	p.cursor = target
	p, cmd := p.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if p.IsVisible() {
		t.Fatal("palette should be hidden after enter")
	}
	if cmd != nil {
		t.Fatal("cmd should be nil for non-executable binding")
	}
}

func TestPaletteView(t *testing.T) {
	var p palette
	p.Open(paletteJumpSrc{})
	out := p.View(71, 19)
	if out == "" {
		t.Fatal("palette view should not be empty when visible")
	}
	if !strings.Contains(out, "❯") {
		t.Error("palette view should contain prompt")
	}

	p.Hide()
	if p.View(71, 19) != "" {
		t.Fatal("palette view should be empty when hidden")
	}
}

// Descriptions must align in one display column even when a key display has
// multi-byte glyphs ("↑/↓"). Padding used to be len()-based (bytes), so arrow
// rows drifted left of ASCII rows.
func TestPaletteDescriptionAlignmentWithArrowKeys(t *testing.T) {
	var p palette
	p.Open(paletteJumpSrc{})
	// "move" matches several bindings spanning ASCII ("j/k", "h/j/k/l") and
	// arrow ("↑/↓", "j/k, ↑/↓") keys. "remove row" also fuzzy-matches, so we
	// key off the fixed 2-space gap that precedes each description ("  move")
	// to catch only descs that START with move.
	p.input = "move"
	p.refilter()

	const gap = "  " // fixed gap between the key field and the description
	lines := strings.Split(stripAnsi(p.View(120, 40)), "\n")
	var cols []int
	for _, ln := range lines {
		if strings.Contains(ln, "❯") {
			continue // skip the prompt line (its input echoes "move")
		}
		idx := strings.Index(ln, gap+"move")
		if idx < 0 {
			continue
		}
		// Display column where the description starts (the gap is constant).
		cols = append(cols, runeLen(ln[:idx+len(gap)]))
	}
	if len(cols) < 2 {
		t.Fatalf("expected >=2 aligned rows to compare, got %d", len(cols))
	}
	for _, c := range cols[1:] {
		if c != cols[0] {
			t.Errorf("palette descriptions misaligned: columns=%v", cols)
			break
		}
	}
}

// Chords and double-presses are reachable from the palette by replaying their
// key sequence through the normal (stateful) dispatch — the pending-G /
// pending-D flag set by the first key is consumed by the second. This is the
// behaviour change Step 3 introduces: these used to be non-executable.
func TestPaletteChordsExecutableViaSequence(t *testing.T) {
	var p palette
	p.Open(paletteJumpSrc{})
	want := map[string][]string{
		"g d": {"g", "d"},
		"g e": {"g", "e"},
		"g s": {"g", "s"},
		"g X": {"g", "X"},
		"g c": {"g", "c"},
		"dd":  {"d", "d"},
		"y y": {"y", "y"},
		"==":  {"=", "="},
	}
	seen := map[string]bool{}
	for _, it := range p.items {
		if exp, ok := want[it.display]; ok {
			seen[it.display] = true
			if !reflect.DeepEqual(it.replay, exp) {
				t.Errorf("binding %q replay = %v, want %v", it.display, it.replay, exp)
			}
		}
	}
	for k := range want {
		if !seen[k] {
			t.Errorf("expected a palette item for %q", k)
		}
	}
}

func TestReplayKeySequence(t *testing.T) {
	if replayKeySequence(nil) != nil {
		t.Error("empty sequence should yield nil cmd")
	}
	if replayKeySequence([]string{"g", "d"}) == nil {
		t.Error("sequence [g d] should yield a non-nil cmd")
	}
	// An unsynthesizable token aborts the whole sequence (nothing replayed).
	if replayKeySequence([]string{"g", ""}) != nil {
		t.Error("sequence with a bad token should yield nil")
	}
}

// Every chordReplays entry must correspond to a real binding Display, so
// renaming a binding can't silently strand a chord replay.
func TestChordReplaysAreRealBindings(t *testing.T) {
	displays := map[string]bool{}
	for _, sec := range registry() {
		for _, b := range sec.Items {
			displays[b.Display] = true
		}
	}
	for k := range chordReplays {
		if !displays[k] {
			t.Errorf("chordReplays key %q is not a binding Display", k)
		}
	}
}

func TestSynthesizeKeyMsg(t *testing.T) {
	tests := []struct {
		token string
		want  string
	}{
		{"j", "j"},
		{"X", "X"},
		{"?", "?"},
		{"enter", "enter"},
		{"esc", "esc"},
		{"tab", "tab"},
		{"ctrl+r", "ctrl+r"},
		{"ctrl+e", "ctrl+e"},
		{"ctrl+c", "ctrl+c"},
		{"/", "/"},
		{"\\", "\\"},
		{"!", "!"},
		{"*", "*"},
	}
	for _, tt := range tests {
		kmsg, ok := synthesizeKeyMsg(tt.token)
		if !ok {
			t.Errorf("synthesizeKeyMsg(%q) returned ok=false", tt.token)
			continue
		}
		if got := kmsg.String(); got != tt.want {
			t.Errorf("synthesizeKeyMsg(%q).String() = %q, want %q", tt.token, got, tt.want)
		}
	}

	// Empty token should fail.
	if _, ok := synthesizeKeyMsg(""); ok {
		t.Error(`synthesizeKeyMsg("") should return ok=false`)
	}
}

// simulateTyping feeds printable characters into the palette via Update.
func simulateTyping(p *palette, s string) {
	for _, ch := range s {
		next, _ := p.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{ch}})
		*p = next
	}
}

// updatePalette is a test helper that asserts Update succeeds and returns
// the updated palette.
func updatePalette(t *testing.T, p palette, msg tea.KeyMsg) palette {
	t.Helper()
	next, _ := p.Update(msg)
	return next
}

func TestPaletteJumpAnywhereItems(t *testing.T) {
	var p palette
	p.Open(paletteJumpSrc{
		Tables:    []string{"users", "orders"},
		Bookmarks: []string{"SELECT * FROM users;"},
	})

	counts := map[string]int{}
	for _, it := range p.items {
		counts[it.section]++
	}
	if counts["Tables"] != 2 {
		t.Errorf("Tables = %d, want 2", counts["Tables"])
	}
	if counts["History"] != 0 {
		t.Errorf("History = %d, want 0 (history stays on Ctrl+Y)", counts["History"])
	}
	if counts["Bookmarks"] != 1 {
		t.Errorf("Bookmarks = %d, want 1", counts["Bookmarks"])
	}
	if counts["Themes"] < 1 {
		t.Error("expected at least one theme item")
	}

	simulateTyping(&p, "users")
	foundTable := false
	for _, it := range p.filtered {
		if it.jump == paletteJumpTable && it.payload == "users" {
			foundTable = true
			break
		}
	}
	if !foundTable {
		t.Fatal("fuzzy 'users' should match the users table jump")
	}
}

func TestPaletteEnterJumpEmitsMsg(t *testing.T) {
	var p palette
	p.Open(paletteJumpSrc{Tables: []string{"orders"}})

	target := -1
	for i, it := range p.items {
		if it.jump == paletteJumpTable && it.payload == "orders" {
			target = i
			break
		}
	}
	if target < 0 {
		t.Fatal("orders table item missing")
	}
	p.cursor = target
	p, cmd := p.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if p.IsVisible() {
		t.Fatal("palette should hide after enter")
	}
	if cmd == nil {
		t.Fatal("expected a jump cmd")
	}
	msg := cmd()
	jump, ok := msg.(paletteJumpMsg)
	if !ok {
		t.Fatalf("got %T, want paletteJumpMsg", msg)
	}
	if jump.kind != paletteJumpTable || jump.payload != "orders" {
		t.Errorf("jump = %+v, want table/orders", jump)
	}
}

func TestPaletteEnterThemeJump(t *testing.T) {
	var p palette
	p.Open(paletteJumpSrc{})
	simulateTyping(&p, "gruvbox")
	found := false
	for i, it := range p.filtered {
		if it.jump == paletteJumpTheme && it.payload == "gruvbox" {
			p.cursor = i
			found = true
			break
		}
	}
	if !found {
		t.Fatal("expected gruvbox theme in filtered list")
	}
	_, cmd := p.Update(tea.KeyMsg{Type: tea.KeyEnter})
	msg := cmd()
	jump, ok := msg.(paletteJumpMsg)
	if !ok || jump.kind != paletteJumpTheme || jump.payload != "gruvbox" {
		t.Fatalf("got %#v, want theme/gruvbox", msg)
	}
}

func TestFlattenPaletteQuery(t *testing.T) {
	got := flattenPaletteQuery("SELECT  *\nFROM   users")
	if got != "SELECT * FROM users" {
		t.Errorf("flatten = %q", got)
	}
	long := strings.Repeat("a", maxPaletteQueryLen+10)
	if got := flattenPaletteQuery(long); runeLen(got) > maxPaletteQueryLen {
		t.Errorf("truncated length %d > %d", runeLen(got), maxPaletteQueryLen)
	}
}

func TestFitPaletteRowTruncatesLongDesc(t *testing.T) {
	long := "This Is An Extremely Long Theme Display Name That Would Wrap"
	desc, sec := fitPaletteRow(long, "Themes", 6, 40)
	row := "❯ " + "theme " + "  " + desc
	if sec != "" {
		row += "  " + sec
	}
	if w := runeLen(row); w > 40 {
		t.Errorf("fitted row width %d > 40: %q", w, row)
	}
	if !strings.Contains(desc, "…") {
		t.Errorf("expected ellipsis truncation, desc=%q", desc)
	}
}

func TestPaletteViewNoWrapOnLongTheme(t *testing.T) {
	var p palette
	p.Open(paletteJumpSrc{})
	// Force a long visible desc onto the first page.
	p.items = append([]paletteItem{{
		display: "theme",
		desc:    strings.Repeat("LongThemeName", 8),
		section: "Themes",
		jump:    paletteJumpTheme,
		payload: "long-theme",
	}}, p.items...)
	p.filtered = p.items
	p.cursor = 0

	const width, height = 71, 19
	out := p.View(width, height)
	inner := width - 4
	for i, line := range strings.Split(stripAnsi(out), "\n") {
		if line == "" {
			continue
		}
		if w := runeLen(line); w > inner+2 { // allow border noise from panel chrome in strip?
			// stripAnsi removes styles but panel border lines are full width-2
			if strings.HasPrefix(line, "│") || strings.HasPrefix(line, "┌") || strings.HasPrefix(line, "└") {
				continue
			}
			if w > inner {
				t.Errorf("line %d width %d > inner %d: %q", i, w, inner, line)
			}
		}
	}
}

func TestApplyPaletteJump(t *testing.T) {
	m := NewModel(&config.Config{})
	m.state = stateWorkspace
	m.tables = []string{"users"}
	m.editor.SetValue("old")
	// Don't persist theme changes from this unit test.
	m.config = nil

	m.applyPaletteJump(paletteJumpMsg{kind: paletteJumpBookmark, payload: "SELECT 42;"})
	if m.editor.Value() != "SELECT 42;" {
		t.Errorf("editor = %q, want SELECT 42;", m.editor.Value())
	}
	if m.focus != FocusEditor {
		t.Errorf("focus = %v, want editor", m.focus)
	}

	defer applyPalette(defaultPalette)
	m.applyPaletteJump(paletteJumpMsg{kind: paletteJumpTheme, payload: "nord"})
	if m.settings.Theme != "nord" {
		t.Errorf("theme = %q, want nord", m.settings.Theme)
	}
	if colorPrimary != nordPalette.primary {
		t.Error("nord palette was not applied")
	}
}
