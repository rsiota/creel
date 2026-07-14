package ui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/ruben/gsql/internal/db"
)

func eqSlice(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// --- parser ---

func TestParseExLine(t *testing.T) {
	cases := []struct {
		in    string
		verb  string
		args  []string
		force bool
	}{
		{"q", "q", nil, false},
		{"q!", "q", nil, true},
		{"Q!", "q", nil, true}, // verb is case-insensitive
		{"sort name", "sort", []string{"name"}, false},
		{"goto users", "goto", []string{"users"}, false},
		{"gt orders", "gt", []string{"orders"}, false},
		{`filter "a b"`, "filter", []string{"a b"}, false},
		{"wq", "wq", nil, false},
		{"", "", nil, false},
	}
	for _, c := range cases {
		verb, args, force := parseExLine(c.in)
		if verb != c.verb || force != c.force || !eqSlice(args, c.args) {
			t.Errorf("parseExLine(%q) = verb=%q args=%v force=%v; want verb=%q args=%v force=%v",
				c.in, verb, args, force, c.verb, c.args, c.force)
		}
	}
}

func TestSplitShellFields(t *testing.T) {
	cases := []struct {
		in   string
		want []string
	}{
		{"", nil},
		{"a", []string{"a"}},
		{"a b c", []string{"a", "b", "c"}},
		{`"a b" c`, []string{"a b", "c"}},
		{`'a b' c`, []string{"a b", "c"}},
		{`a\ b c`, []string{"a b", "c"}}, // backslash escapes the space
		{"  trim  ", []string{"trim"}},
	}
	for _, c := range cases {
		if got := splitShellFields(c.in); !eqSlice(got, c.want) {
			t.Errorf("splitShellFields(%q) = %v; want %v", c.in, got, c.want)
		}
	}
}

// --- exCmd state ---

func TestExCmdOpenHideViewRecall(t *testing.T) {
	var ex exCmd
	if ex.IsVisible() {
		t.Fatal("ex line should start hidden")
	}
	ex.Open()
	if !ex.IsVisible() || ex.input != "" {
		t.Fatal("Open should show the line with empty input")
	}

	ex.input = "q"
	if v := ex.View(); !strings.Contains(v, ":q") {
		t.Errorf("View should render ':q', got %q", v)
	}
	if v := ex.View(); !strings.Contains(v, "\x1b[4m") && !strings.Contains(v, ";4m") {
		t.Errorf("View should render an underline cursor, got %q", v)
	}

	// History recall: most-recent-last, ↑ older, ↓ newer, past-end = fresh.
	ex.hist = []string{"sort name", "q"}
	ex.histIdx = len(ex.hist)
	ex.recall(-1)
	if ex.input != "q" {
		t.Errorf("recall ↑ = %q want q", ex.input)
	}
	ex.recall(-1)
	if ex.input != "sort name" {
		t.Errorf("recall ↑ = %q want 'sort name'", ex.input)
	}
	ex.recall(1)
	if ex.input != "q" {
		t.Errorf("recall ↓ = %q want q", ex.input)
	}
	ex.recall(1)
	if ex.input != "" {
		t.Errorf("recall past end = %q want empty", ex.input)
	}
	// Clamps at the oldest entry.
	ex.recall(-1)
	ex.recall(-1)
	ex.recall(-1)
	if ex.input != "sort name" {
		t.Errorf("recall clamped = %q want 'sort name'", ex.input)
	}

	ex.Hide()
	if ex.IsVisible() {
		t.Error("Hide should hide the line")
	}
	if ex.View() != "" {
		t.Errorf("hidden View should be empty, got %q", ex.View())
	}
}

// --- handleExKey: typing, submit, cancel ---

func TestHandleExKeyTypingAndSubmit(t *testing.T) {
	m := &Model{results: NewResultsTable(), focus: FocusResults}
	m.ex.Open()

	for _, r := range "help" {
		m.handleExKey(runeKey(r))
	}
	if m.ex.input != "help" {
		t.Fatalf("after typing 'help', input=%q", m.ex.input)
	}
	// Backspace edits.
	m.handleExKey(tea.KeyMsg{Type: tea.KeyBackspace})
	if m.ex.input != "hel" {
		t.Errorf("backspace -> %q want hel", m.ex.input)
	}
	// Re-type, then submit with enter → runs the command and closes.
	for _, r := range "p" {
		m.handleExKey(runeKey(r))
	}
	m.handleExKey(tea.KeyMsg{Type: tea.KeyEnter})
	if m.ex.IsVisible() {
		t.Error("enter should close the ex line")
	}
	if !m.help.IsVisible() {
		t.Error(":help should have opened the help overlay")
	}
	// Submitted input is recorded to history.
	if len(m.ex.hist) != 1 || m.ex.hist[0] != "help" {
		t.Errorf("history = %v, want [help]", m.ex.hist)
	}
}

func TestHandleExKeyEscCancels(t *testing.T) {
	m := &Model{focus: FocusResults}
	m.ex.Open()
	m.ex.input = "sort name"
	m.handleExKey(tea.KeyMsg{Type: tea.KeyEsc})
	if m.ex.IsVisible() {
		t.Error("esc should close the ex line")
	}
	if m.sortCol != "" {
		t.Error("esc should not run the command")
	}
}

// Regression: Update (value receiver) asserts the returned model back to a
// VALUE Model via model.(Model). handleExKey is a *Model method, so it must
// return *m (the dereferenced value), not m (the pointer) — otherwise the
// assertion panics with "tea.Model is *ui.Model, not ui.Model" the moment the
// ex line handles any key. The other handleExKey/sub-handler tests above call
// handleExKey directly and so bypass the assertion; this one pins the contract.
func TestHandleExKeyReturnsValueModel(t *testing.T) {
	cases := []tea.KeyMsg{
		runeKey('q'),
		{Type: tea.KeyEsc},
		{Type: tea.KeyEnter},
		{Type: tea.KeyUp},
		{Type: tea.KeyDown},
		{Type: tea.KeyBackspace},
	}
	for _, k := range cases {
		m := &Model{results: NewResultsTable(), focus: FocusResults}
		m.ex.Open()
		mm, _ := m.handleExKey(k)
		if _, ok := mm.(Model); !ok {
			t.Errorf("key=%v: handleExKey returned %T, want value Model", k, mm)
		}
	}
}

func TestHandleExKeyHistoryRecall(t *testing.T) {
	m := &Model{focus: FocusResults}
	m.ex.hist = []string{"q", "sort name"}
	m.ex.Open()
	m.handleExKey(tea.KeyMsg{Type: tea.KeyUp})
	if m.ex.input != "sort name" {
		t.Errorf("↑ recall = %q want 'sort name'", m.ex.input)
	}
	m.handleExKey(tea.KeyMsg{Type: tea.KeyUp})
	if m.ex.input != "q" {
		t.Errorf("↑ recall = %q want q", m.ex.input)
	}
	m.handleExKey(tea.KeyMsg{Type: tea.KeyDown})
	if m.ex.input != "sort name" {
		t.Errorf("↓ recall = %q want 'sort name'", m.ex.input)
	}
}

// Regression: the space bar arrives as tea.KeySpace (NOT KeyRunes), so the
// insertion guard must accept it — otherwise every multi-word command loses
// its separator (":gt users" -> "gtusers" -> E492).
func TestHandleExKeyInsertsSpace(t *testing.T) {
	m := &Model{results: NewResultsTable(), focus: FocusResults}
	m.ex.Open()
	keys := []tea.KeyMsg{
		runeKey('g'), runeKey('t'),
		{Type: tea.KeySpace},
		runeKey('u'), runeKey('s'), runeKey('e'), runeKey('r'), runeKey('s'),
	}
	for _, k := range keys {
		m.handleExKey(k)
	}
	if m.ex.input != "gt users" {
		t.Fatalf("input=%q, want `gt users` (space must be inserted)", m.ex.input)
	}
}

// --- commands ---

func TestExHelp(t *testing.T) {
	m := &Model{}
	m.runExCommand("help")
	if !m.help.IsVisible() {
		t.Error(":help did not open the help overlay")
	}
}

func TestExSort(t *testing.T) {
	m := &Model{
		baseQuery:  "SELECT * FROM users",
		connection: &db.Connection{},
		results:    NewResultsTable(),
		editor:     NewQueryEditor(),
	}
	m.results.SetResult([]string{"id", "name"}, [][]string{{"1", "alice"}}, "")
	m.runExCommand("sort name")
	if m.sortCol != "name" || m.sortDir != "ASC" {
		t.Errorf(":sort name -> sortCol=%q sortDir=%q, want name/ASC", m.sortCol, m.sortDir)
	}
}

func TestExSortMissingArg(t *testing.T) {
	m := &Model{}
	m.runExCommand("sort")
	if !strings.Contains(m.schemaMsg, "column name") {
		t.Errorf(":sort with no arg -> %q", m.schemaMsg)
	}
}

func TestExWrite(t *testing.T) {
	t.Run("no changes", func(t *testing.T) {
		m := &Model{results: NewResultsTable()}
		cmd := m.runExCommand("w")
		if cmd != nil {
			t.Errorf(":w with no dirty cells should return nil, got %v", cmd)
		}
		if !strings.Contains(m.schemaMsg, "no changes") {
			t.Errorf(":w no-dirty -> %q", m.schemaMsg)
		}
	})
	t.Run("dirty saves", func(t *testing.T) {
		m := &Model{connection: &db.Connection{}, results: NewResultsTable()}
		m.results.SetResult([]string{"id", "name"}, [][]string{{"1", "alice"}}, "")
		m.results.SetDirtyCell(0, 1, "bob")
		if cmd := m.runExCommand("w"); cmd == nil {
			t.Error(":w with dirty cells should return a save command")
		}
	})
}

func TestExQuitGuards(t *testing.T) {
	t.Run("dirty blocks", func(t *testing.T) {
		m := &Model{results: NewResultsTable(), resultsTabs: []*ResultsTab{{ID: 1}, {ID: 2}}}
		m.results.SetResult([]string{"id"}, [][]string{{"1"}}, "")
		m.results.SetDirtyCell(0, 0, "x")
		m.runExCommand("q")
		if !strings.Contains(m.schemaMsg, "unsaved") {
			t.Errorf("dirty :q -> %q, want unsaved-changes warning", m.schemaMsg)
		}
		if len(m.resultsTabs) != 2 {
			t.Errorf("dirty :q should not close a tab, got %d tabs", len(m.resultsTabs))
		}
	})
	t.Run("force overrides dirty", func(t *testing.T) {
		m := &Model{
			results:     NewResultsTable(),
			editor:      NewQueryEditor(),
			resultsTabs: []*ResultsTab{{ID: 1}, {ID: 2}},
			activeTabID: 1,
			width:       120,
			height:      40,
		}
		m.results.SetResult([]string{"id"}, [][]string{{"1"}}, "")
		m.results.SetDirtyCell(0, 0, "x")
		m.runExCommand("q!")
		if len(m.resultsTabs) != 1 {
			t.Errorf(":q! should close the tab, got %d tabs", len(m.resultsTabs))
		}
	})
	t.Run("last tab refused", func(t *testing.T) {
		m := &Model{resultsTabs: []*ResultsTab{{ID: 1}}}
		m.runExCommand("q")
		if !strings.Contains(m.schemaMsg, "last tab") {
			t.Errorf(":q last tab -> %q", m.schemaMsg)
		}
	})
}

func TestExGoto(t *testing.T) {
	m := &Model{tables: []string{"users", "orders"}, editor: NewQueryEditor()}

	m.runExCommand("goto users")
	if m.sidebarCursor != 0 || !strings.Contains(m.editor.Value(), "SELECT * FROM users") {
		t.Errorf(":goto users -> cursor=%d editor=%q", m.sidebarCursor, m.editor.Value())
	}
	// Substring fallback.
	m.runExCommand("goto ord")
	if m.sidebarCursor != 1 || !strings.Contains(m.editor.Value(), "SELECT * FROM orders") {
		t.Errorf(":goto ord -> cursor=%d editor=%q", m.sidebarCursor, m.editor.Value())
	}
	// Unknown table.
	m.runExCommand("goto nope")
	if !strings.Contains(m.schemaMsg, "no such table") {
		t.Errorf(":goto nope -> %q", m.schemaMsg)
	}
}

// --- fallback column jump + unknown ---

func TestExFallbackColumnJump(t *testing.T) {
	m := &Model{results: NewResultsTable(), focus: FocusResults}
	m.results.SetResult([]string{"id", "name", "email"}, [][]string{{"1", "a", "b"}}, "")
	m.results.SetCursor(0, 0)

	m.runExCommand("email") // bare identifier, not a command → column jump
	if m.results.CursorCol() != 2 {
		t.Errorf("fallback :email -> cursor col %d, want 2", m.results.CursorCol())
	}
}

func TestExUnknownCommand(t *testing.T) {
	m := &Model{results: NewResultsTable(), focus: FocusResults}
	m.results.SetResult([]string{"id"}, [][]string{{"1"}}, "")
	m.runExCommand("frobnicate")
	if !strings.Contains(m.schemaMsg, "E492") || !strings.Contains(m.schemaMsg, "frobnicate") {
		t.Errorf("unknown command -> %q, want E492 + the input", m.schemaMsg)
	}
}

// Fallback only applies in the results view.
func TestExUnknownNotInResults(t *testing.T) {
	m := &Model{results: NewResultsTable(), focus: FocusEditor}
	m.results.SetResult([]string{"id"}, [][]string{{"1"}}, "")
	m.runExCommand("id")
	if !strings.Contains(m.schemaMsg, "E492") {
		t.Errorf("bare identifier outside results should be E492, got %q", m.schemaMsg)
	}
}
