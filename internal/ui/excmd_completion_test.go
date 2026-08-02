package ui

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/ruben/creel/internal/config"
)

// TestExCompletionShowsAllOnOpen: typing ":" alone lists every command — the
// core discoverability win. Each command contributes exactly one row even if
// several of its aliases would match the empty prefix.
func TestExCompletionShowsAllOnOpen(t *testing.T) {
	var ex exCmd
	ex.Open()
	if len(ex.comp) != len(exCommands()) {
		t.Errorf("after Open, comp has %d items, want all %d commands",
			len(ex.comp), len(exCommands()))
	}
}

func TestExCompletionFiltersByPrefix(t *testing.T) {
	var ex exCmd

	// "g" matches only goto (verbs goto/gt) → one row, still shown.
	ex.input = "g"
	ex.recomputeCompletion()
	if len(ex.comp) != 1 || ex.comp[0].verb != "goto" {
		t.Errorf("prefix %q -> comp=%+v, want [goto]", "g", ex.comp)
	}

	// Fully typing the canonical verb hides the popup.
	ex.input = "goto"
	ex.recomputeCompletion()
	if len(ex.comp) != 0 {
		t.Errorf("exact %q should hide popup, got %+v", "goto", ex.comp)
	}

	// Moving past the verb (a space) hides it too.
	ex.input = "goto "
	ex.recomputeCompletion()
	if len(ex.comp) != 0 {
		t.Errorf("past the verb should hide popup, got %+v", ex.comp)
	}

	// An unknown prefix yields no matches (and no popup).
	ex.input = "zzz"
	ex.recomputeCompletion()
	if len(ex.comp) != 0 {
		t.Errorf("unknown prefix should yield no matches, got %+v", ex.comp)
	}
}

// TestExCompletionTabCompletes: Tab fills the verb from the top match and then
// hides the popup (the verb is now exact).
func TestExCompletionTabCompletes(t *testing.T) {
	m := &Model{}
	m.ex.Open()
	for _, r := range "g" {
		m.handleExKey(runeKey(r))
	}
	if m.ex.input != "g" {
		t.Fatalf("after typing 'g', input=%q", m.ex.input)
	}
	m.handleExKey(tea.KeyMsg{Type: tea.KeyTab})
	if m.ex.input != "goto" {
		t.Errorf("Tab -> input=%q, want goto", m.ex.input)
	}
	if len(m.ex.comp) != 0 {
		t.Errorf("Tab should hide popup after exact match, got %+v", m.ex.comp)
	}
}

func TestExCompletionViewEmpty(t *testing.T) {
	var ex exCmd
	if ex.completionView(120) != "" {
		t.Error("completionView should be empty when not visible")
	}
	ex.visible = true
	ex.comp = nil
	if ex.completionView(120) != "" {
		t.Error("completionView should be empty with no candidates")
	}
}

// TestExCompletionViewRendersUsage: the popup contains the canonical command
// name (the blue command column) so the Tab target is readable. The full
// invocation form (with arguments) lives in :help, not the popup.
func TestExCompletionViewRendersUsage(t *testing.T) {
	ex := exCmd{visible: true}
	ex.input = "g"
	ex.recomputeCompletion()
	out := ex.completionView(120)
	if !strings.Contains(out, ":goto") {
		t.Errorf("completionView missing command name; got %q", out)
	}
	if strings.Contains(out, "<table>") {
		t.Errorf("completionView should not show arg syntax; got %q", out)
	}
}

// TestExCompletionFixedWidth verifies the verb-completion popup renders at a
// constant width regardless of which commands match: every row (including the
// highlight bar) is the same width, so the box doesn't jitter as you type.
func TestExCompletionFixedWidth(t *testing.T) {
	var ex exCmd
	ex.Open() // ":" alone -> every command is a candidate
	out := ex.completionView(120)
	if out == "" {
		t.Fatal("expected a completion popup for \":\"")
	}
	// Strip the border to measure content rows: drop the first/last line
	// (top/bottom border) and the side border chars from each remaining line.
	lines := strings.Split(out, "\n")
	if len(lines) < 3 {
		t.Fatalf("popup too short: %d lines", len(lines))
	}
	body := lines[1 : len(lines)-1]
	wantW := lipgloss.Width(body[0])
	for i, l := range body {
		if w := lipgloss.Width(l); w != wantW {
			t.Errorf("row %d width = %d, want fixed %d", i, w, wantW)
		}
	}

	// Narrow the filter to a few short commands and re-render: the width must
	// not shrink to fit them (the usage column is pinned to the global max).
	var ex2 exCmd
	ex2.Open()
	ex2.input = "co"
	ex2.recomputeCompletion()
	out2 := ex2.completionView(120)
	if out2 == "" {
		t.Fatal("expected a completion popup for \"co\"")
	}
	lines2 := strings.Split(out2, "\n")
	body2 := lines2[1 : len(lines2)-1]
	if w := lipgloss.Width(body2[0]); w != wantW {
		t.Errorf("narrowed popup width = %d, want fixed %d", w, wantW)
	}
}

// --- argument completion ---------------------------------------------------

// sameStrings reports whether two slices hold the same strings in order.
func sameStrings(a, b []string) bool {
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

// sameStringsSet compares two slices ignoring order (for completion candidates
// whose ranked order isn't stable across rankers/machines).
func sameStringsSet(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	saw := make(map[string]int, len(a))
	for _, s := range a {
		saw[s]++
	}
	for _, s := range b {
		saw[s]--
		if saw[s] < 0 {
			return false
		}
	}
	return true
}

// exCandidates extracts the candidate strings from a comp slice.
func exCandidates(items []exCompItem) []string {
	out := make([]string, len(items))
	for i, c := range items {
		out[i] = c.candidate
	}
	return out
}

// TestExArgCompletionTables: past the verb, table-name commands offer the
// connection's tables. Empty partial -> alphabetical.
func TestExArgCompletionTables(t *testing.T) {
	m := &Model{tables: []string{"users", "orders", "events"}}
	m.ex.input = "goto "
	m.recomputeExCompletion()
	if !m.ex.argMode {
		t.Fatal("expected arg mode after the verb")
	}
	got := exCandidates(m.ex.comp)
	want := []string{"events", "orders", "users"}
	if !sameStrings(got, want) {
		t.Errorf("goto candidates = %v, want %v", got, want)
	}
}

// TestExArgCompletionTablesWithPartial: a partial token fuzzy-filters.
func TestExArgCompletionTablesWithPartial(t *testing.T) {
	m := &Model{tables: []string{"users", "orders", "events"}}
	m.ex.input = "goto us"
	m.recomputeExCompletion()
	got := exCandidates(m.ex.comp)
	// "us" fuzzy-matches only "users".
	if !sameStrings(got, []string{"users"}) {
		t.Errorf("partial %q candidates = %v, want [users]", "us", got)
	}
}

// TestExArgCompletionHiddenPastFirstArg: a table command offers nothing once
// the table argument is already supplied (the rest is free-form).
func TestExArgCompletionHiddenPastFirstArg(t *testing.T) {
	m := &Model{tables: []string{"users", "orders"}}
	m.ex.input = "goto users "
	m.recomputeExCompletion()
	if len(m.ex.comp) != 0 {
		t.Errorf("expected no candidates past the table arg, got %+v", m.ex.comp)
	}
	if m.ex.completionView(120) != "" {
		t.Error("expected empty popup past the table arg")
	}
}

// TestExArgCompletionConnection: :connect offers configured connection names.
func TestExArgCompletionConnection(t *testing.T) {
	m := &Model{config: &config.Config{Connections: []config.ConnectionConfig{
		{Name: "prod"}, {Name: "dev"},
	}}}
	m.ex.input = "connect "
	m.recomputeExCompletion()
	got := exCandidates(m.ex.comp)
	want := []string{"dev", "prod"} // empty partial -> alphabetical
	if !sameStrings(got, want) {
		t.Errorf("connect candidates = %v, want %v", got, want)
	}
}

// TestExArgCompletionTheme: :theme offers every theme name.
func TestExArgCompletionTheme(t *testing.T) {
	m := &Model{}
	m.ex.input = "theme "
	m.recomputeExCompletion()
	if !m.ex.argMode {
		t.Fatal("expected arg mode")
	}
	want := rankStrings("", themeNames())
	got := exCandidates(m.ex.comp)
	if !sameStrings(got, want) {
		t.Errorf("theme candidates mismatch: got %d, want %d", len(got), len(want))
	}
}

// TestExArgCompletionExport: :export offers the results-export format set
// for the first argument. ("sql" is intentionally absent — it is the
// whole-database dump format driven by the sidebar X picker, not a results
// export, and parseExportFormat would reject it.)
func TestExArgCompletionExport(t *testing.T) {
	m := &Model{}
	m.ex.input = "export "
	m.recomputeExCompletion()
	got := exCandidates(m.ex.comp)
	want := []string{"csv", "json", "jsonl", "md", "tsv"}
	if !sameStrings(got, want) {
		t.Errorf("export candidates = %v, want %v", got, want)
	}
}

// TestExArgCompletionIcons: a partial ranks a boundary match first.
func TestExArgCompletionIcons(t *testing.T) {
	m := &Model{}
	m.ex.input = "icons n"
	m.recomputeExCompletion()
	got := exCandidates(m.ex.comp)
	// "n" matches both, but "nerdfont" (leading n) outranks "unicode".
	if len(got) == 0 || got[0] != "nerdfont" {
		t.Errorf("icons %q top candidate = %v, want nerdfont first", "n", got)
	}
}

// TestExArgCompletionTab: Tab replaces the partial token with the top match.
func TestExArgCompletionTab(t *testing.T) {
	m := &Model{tables: []string{"users", "orders"}}
	m.ex.Open()
	m.ex.input = "goto us"
	m.recomputeExCompletion()
	if len(m.ex.comp) == 0 || m.ex.comp[0].candidate != "users" {
		t.Fatalf("before Tab, comp = %+v, want [users]", m.ex.comp)
	}
	m.handleExKey(tea.KeyMsg{Type: tea.KeyTab})
	if m.ex.input != "goto users" {
		t.Errorf("after Tab, input = %q, want %q", m.ex.input, "goto users")
	}
}

// TestExArgCompletionVerbModeWhenNoSpace: before any space it stays verb mode.
func TestExArgCompletionVerbModeWhenNoSpace(t *testing.T) {
	m := &Model{tables: []string{"users"}}
	m.ex.input = "go"
	m.recomputeExCompletion()
	if m.ex.argMode {
		t.Error("expected verb mode (argMode=false) before any space")
	}
	if len(m.ex.comp) != 1 || m.ex.comp[0].verb != "goto" {
		t.Errorf("expected verb [goto], got %+v", m.ex.comp)
	}
}

// TestSplitArgsPartial covers the arg/partial split, including the trailing-
// space case (partial empty, args include the finished token).
func TestSplitArgsPartial(t *testing.T) {
	cases := []struct {
		rest        string
		wantArgs    []string
		wantPartial string
	}{
		{"", nil, ""},
		{"users", nil, "users"},
		{"users ", []string{"users"}, ""},
		{"users em", []string{"users"}, "em"},
		{"users em ", []string{"users", "em"}, ""},
	}
	for _, c := range cases {
		args, partial := splitArgsPartial(c.rest)
		if !sameStrings(args, c.wantArgs) || partial != c.wantPartial {
			t.Errorf("splitArgsPartial(%q) = (%v, %q), want (%v, %q)",
				c.rest, args, partial, c.wantArgs, c.wantPartial)
		}
	}
}

// TestApplyArgCompletion covers Tab's token replacement.
func TestApplyArgCompletion(t *testing.T) {
	cases := []struct {
		input, candidate, want string
	}{
		{"goto us", "users", "goto users"},
		{"goto users ", "50", "goto users 50"},
		{"goto", "users", "users"},
	}
	for _, c := range cases {
		if got := applyArgCompletion(c.input, c.candidate); got != c.want {
			t.Errorf("applyArgCompletion(%q, %q) = %q, want %q",
				c.input, c.candidate, got, c.want)
		}
	}
}

// TestExArgCompletionColumns: past the verb, column-name commands (:sort,
// :hidecolumn, :stats, :filter) offer the result set's columns. Empty partial
// -> alphabetical. The source is the results grid, not the schema cache, so a
// custom query's columns are offered too.
func TestExArgCompletionColumns(t *testing.T) {
	m := &Model{}
	m.results.SetResult([]string{"id", "name", "email"}, nil, "")
	m.ex.input = "sort "
	m.recomputeExCompletion()
	if !m.ex.argMode {
		t.Fatal("expected arg mode after the verb")
	}
	got := exCandidates(m.ex.comp)
	want := []string{"email", "id", "name"} // empty partial -> alphabetical
	if !sameStrings(got, want) {
		t.Errorf("sort candidates = %v, want %v", got, want)
	}
}

// TestExArgCompletionColumnsPartial: a partial token fuzzy-filters columns.
func TestExArgCompletionColumnsPartial(t *testing.T) {
	m := &Model{}
	m.results.SetResult([]string{"id", "name", "email", "user_id"}, nil, "")
	m.ex.input = "sort em"
	m.recomputeExCompletion()
	got := exCandidates(m.ex.comp)
	if !sameStrings(got, []string{"email"}) {
		t.Errorf("partial %q candidates = %v, want [email]", "em", got)
	}
}

// TestExArgCompletionColumnsHiddenPastFirstArg: a column command offers nothing
// once the column argument is already supplied.
func TestExArgCompletionColumnsHiddenPastFirstArg(t *testing.T) {
	m := &Model{}
	m.results.SetResult([]string{"id", "name"}, nil, "")
	m.ex.input = "sort id "
	m.recomputeExCompletion()
	if len(m.ex.comp) != 0 {
		t.Errorf("expected no candidates past the column arg, got %+v", m.ex.comp)
	}
	if m.ex.completionView(120) != "" {
		t.Error("expected empty popup past the column arg")
	}
}

// TestExArgCompletionColumnsNoResults: with no result set there is nothing to
// offer (and the commands would refuse anyway).
func TestExArgCompletionColumnsNoResults(t *testing.T) {
	m := &Model{}
	m.ex.input = "sort "
	m.recomputeExCompletion()
	if len(m.ex.comp) != 0 {
		t.Errorf("expected no candidates with no results, got %+v", m.ex.comp)
	}
}

// TestExArgCompletionColumnsAllVerbs: each of the column-name commands is wired
// to completeColumn (so a typo in the registry can't silently drop one).
func TestExArgCompletionColumnsAllVerbs(t *testing.T) {
	for _, verb := range []string{"sort", "hidecolumn", "stats", "filter"} {
		spec := exLookup(verb)
		if spec == nil {
			t.Errorf("missing spec for :%s", verb)
			continue
		}
		if spec.complete == nil {
			t.Errorf(":%s has no completer, want completeColumn", verb)
			continue
		}
		m := &Model{}
		m.results.SetResult([]string{"id", "name"}, nil, "")
		m.ex.input = verb + " "
		m.recomputeExCompletion()
		got := exCandidates(m.ex.comp)
		if !sameStrings(got, []string{"id", "name"}) {
			t.Errorf(":%s candidates = %v, want [id name]", verb, got)
		}
	}
}

// TestExArgCompletionColumnTab: Tab replaces the partial column token with the
// top match.
func TestExArgCompletionColumnTab(t *testing.T) {
	m := &Model{}
	m.results.SetResult([]string{"id", "name", "email"}, nil, "")
	m.ex.Open()
	m.ex.input = "sort em"
	m.recomputeExCompletion()
	if len(m.ex.comp) == 0 || m.ex.comp[0].candidate != "email" {
		t.Fatalf("before Tab, comp = %+v, want [email]", m.ex.comp)
	}
	m.handleExKey(tea.KeyMsg{Type: tea.KeyTab})
	if m.ex.input != "sort email" {
		t.Errorf("after Tab, input = %q, want %q", m.ex.input, "sort email")
	}
}

// --- file-path completion --------------------------------------------------

// setupPathFixture makes a temp directory with a known set of entries for
// path-completion tests, returning its path (no trailing slash):
//
//	alpha.sql  beta.txt  sub/  .secret
func setupPathFixture(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	must := func(err error) {
		if err != nil {
			t.Fatal(err)
		}
	}
	must(os.WriteFile(filepath.Join(dir, "alpha.sql"), nil, 0o644))
	must(os.WriteFile(filepath.Join(dir, "beta.txt"), nil, 0o644))
	must(os.Mkdir(filepath.Join(dir, "sub"), 0o755))
	must(os.WriteFile(filepath.Join(dir, ".secret"), nil, 0o644))
	return dir
}

// TestCompleteFilePath covers the shared engine directly: prefix filtering,
// directories get a trailing "/", hidden entries are omitted unless the partial
// itself starts with ".", an unmatched prefix yields nothing, and a value with
// no directory prefix yields nothing.
func TestCompleteFilePath(t *testing.T) {
	dir := setupPathFixture(t)

	// Whole dir (trailing slash): non-hidden entries, dirs suffixed, sorted.
	got := completeFilePath(dir + "/")
	want := []string{"alpha.sql", "beta.txt", "sub/"}
	if !sameStrings(got, want) {
		t.Errorf("completeFilePath(%q) = %v, want %v", dir+"/", got, want)
	}

	// Partial filename filters.
	if got := completeFilePath(dir + "/a"); !sameStrings(got, []string{"alpha.sql"}) {
		t.Errorf("partial 'a' = %v, want [alpha.sql]", got)
	}
	if got := completeFilePath(dir + "/su"); !sameStrings(got, []string{"sub/"}) {
		t.Errorf("partial 'su' = %v, want [sub/]", got)
	}

	// Hidden entries appear only when the partial starts with ".".
	if got := completeFilePath(dir + "/."); !sameStrings(got, []string{".secret"}) {
		t.Errorf("partial '.' = %v, want [.secret]", got)
	}

	// Unmatched prefix → nothing.
	if got := completeFilePath(dir + "/z"); got != nil {
		t.Errorf("partial 'z' = %v, want nil", got)
	}

	// No directory prefix → nothing (callers start with ~/, ./, or /).
	if got := completeFilePath("foo"); got != nil {
		t.Errorf("no-dir-prefix = %v, want nil", got)
	}
}

// TestExArgCompletionPaths: :e offers full-path candidates from the filesystem
// (dir + name), so the popup's fuzzy ranker keeps them and Tab fills the path.
func TestExArgCompletionPaths(t *testing.T) {
	dir := setupPathFixture(t)
	m := &Model{}
	m.ex.input = "e " + dir + "/"
	m.recomputeExCompletion()
	if !m.ex.argMode {
		t.Fatal("expected arg mode after the verb")
	}
	got := exCandidates(m.ex.comp)
	sort.Strings(got)
	want := []string{dir + "/alpha.sql", dir + "/beta.txt", dir + "/sub/"}
	sort.Strings(want)
	if !sameStrings(got, want) {
		t.Errorf(":e candidates = %v, want %v", got, want)
	}
}

// TestExArgCompletionPathsPartial: a partial filename filters to one match.
func TestExArgCompletionPathsPartial(t *testing.T) {
	dir := setupPathFixture(t)
	m := &Model{}
	m.ex.input = "w " + dir + "/be"
	m.recomputeExCompletion()
	got := exCandidates(m.ex.comp)
	if !sameStrings(got, []string{dir + "/beta.txt"}) {
		t.Errorf(":w partial 'be' = %v, want [%q]", got, dir+"/beta.txt")
	}
}

// TestExArgCompletionPathTab: Tab replaces the partial path token with the top
// match (a full path).
func TestExArgCompletionPathTab(t *testing.T) {
	dir := setupPathFixture(t)
	m := &Model{}
	m.ex.Open()
	m.ex.input = "e " + dir + "/be"
	m.recomputeExCompletion()
	m.handleExKey(tea.KeyMsg{Type: tea.KeyTab})
	if want := "e " + dir + "/beta.txt"; m.ex.input != want {
		t.Errorf("after Tab, input = %q, want %q", m.ex.input, want)
	}
}

// TestExArgCompletionPathsAllVerbs: each file command (and its alias) is wired
// to completePath, so a registry typo can't silently drop one.
func TestExArgCompletionPathsAllVerbs(t *testing.T) {
	for _, verb := range []string{"edit", "write", "import", "open", "save"} {
		spec := exLookup(verb)
		if spec == nil {
			t.Errorf("missing spec for :%s", verb)
			continue
		}
		if spec.complete == nil {
			t.Errorf(":%s has no completer, want completePath", verb)
		}
	}
	dir := setupPathFixture(t)
	for _, verb := range []string{"e", "w", "import", "open", "save"} {
		m := &Model{}
		m.ex.input = verb + " " + dir + "/su"
		m.recomputeExCompletion()
		got := exCandidates(m.ex.comp)
		if !sameStrings(got, []string{dir + "/sub/"}) {
			t.Errorf(":%s partial 'su' = %v, want [%q]", verb, got, dir+"/sub/")
		}
	}
}

// TestExArgCompletionPathNoDirPrefix: without a directory prefix there is
// nothing to offer (the engine has no directory to list).
func TestExArgCompletionPathNoDirPrefix(t *testing.T) {
	for _, in := range []string{"e ", "e foo"} {
		m := &Model{}
		m.ex.input = in
		m.recomputeExCompletion()
		if len(m.ex.comp) != 0 {
			t.Errorf("%q: expected no candidates, got %v", in, exCandidates(m.ex.comp))
		}
	}
}

// TestExArgCompletionPathWidthAdaptive: the argument popup sizes to its content
// (capped by terminal width), not a fixed 16 cells — so a long file path shows
// in full when the terminal is wide and is cropped only when it is genuinely
// too narrow to hold it.
func TestExArgCompletionPathWidthAdaptive(t *testing.T) {
	dir := t.TempDir()
	longName := strings.Repeat("x", 40) + ".sql" // 44 runes, well past the old 16-cell cap
	if err := os.WriteFile(filepath.Join(dir, longName), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	full := dir + "/" + longName
	fullLen := runeLen(full)

	m := &Model{}
	m.ex.Open()
	m.ex.input = "e " + dir + "/"
	m.recomputeExCompletion()

	// Wide terminal (a little more than the path needs): the full path fits
	// uncropped — which the old fixed 16-cell cap would never have allowed.
	if out := m.ex.completionView(fullLen + 10); !strings.Contains(out, full) {
		t.Errorf("wide terminal cropped the path; popup=%q", out)
	}

	// Narrow terminal: the path is cropped (no longer contains the full name).
	if out := m.ex.completionView(25); strings.Contains(out, full) {
		t.Errorf("narrow terminal should crop the path; popup=%q", out)
	}
}

// --- popup selection (up/down) ---------------------------------------------

// TestExCompletionPopupUpDownSelects: when the popup is visible, up/down move
// the selection (wrapping, like the command palette) and Tab completes the
// highlighted row rather than always the top match.
func TestExCompletionPopupUpDownSelects(t *testing.T) {
	dir := setupPathFixture(t) // candidates: alpha.sql, beta.txt, sub/
	alpha := dir + "/alpha.sql"
	beta := dir + "/beta.txt"
	sub := dir + "/sub/"

	m := &Model{}
	m.ex.Open()
	m.ex.input = "e " + dir + "/"
	m.recomputeExCompletion()
	if len(m.ex.comp) != 3 {
		t.Fatalf("expected 3 candidates, got %d: %v", len(m.ex.comp), exCandidates(m.ex.comp))
	}
	// The partial (dir+"/") is non-empty, so rankStrings fuzzy-ranks the rows;
	// snapshot that order rather than assuming alphabetical.
	cands := exCandidates(m.ex.comp)
	if !sameStringsSet(cands, []string{alpha, beta, sub}) {
		t.Fatalf("candidate set = %v, want {%s,%s,%s}", cands, alpha, beta, sub)
	}
	c1 := m.ex.comp[1].candidate // row down lands on
	c2 := m.ex.comp[2].candidate // row up-from-top wraps to
	if m.ex.selIdx != 0 {
		t.Fatalf("initial selIdx = %d, want 0", m.ex.selIdx)
	}

	// down → index 1; Tab completes it.
	m.handleExKey(tea.KeyMsg{Type: tea.KeyDown})
	if m.ex.selIdx != 1 {
		t.Errorf("after down, selIdx = %d, want 1", m.ex.selIdx)
	}
	m.handleExKey(tea.KeyMsg{Type: tea.KeyTab})
	if want := "e " + c1; m.ex.input != want {
		t.Errorf("after down+tab, input = %q, want %q", m.ex.input, want)
	}

	// up from the top wraps to the last row; Tab completes it.
	m.ex.input = "e " + dir + "/"
	m.recomputeExCompletion()
	m.handleExKey(tea.KeyMsg{Type: tea.KeyUp}) // wrap 0 → 2
	if m.ex.selIdx != 2 {
		t.Errorf("after up from 0, selIdx = %d, want 2 (wrap)", m.ex.selIdx)
	}
	m.handleExKey(tea.KeyMsg{Type: tea.KeyTab})
	if want := "e " + c2; m.ex.input != want {
		t.Errorf("after up+tab, input = %q, want %q", m.ex.input, want)
	}
}

// TestExCompletionUpDownRecallsHistoryWhenPopupHidden: with no popup visible,
// up/down recall command history (their pre-popup behaviour).
func TestExCompletionUpDownRecallsHistoryWhenPopupHidden(t *testing.T) {
	m := &Model{}
	m.ex.hist = []string{"select 1", "select 2"}
	m.ex.Open()
	m.ex.input = "select *" // no ":select" command → empty popup
	m.recomputeExCompletion()
	if len(m.ex.comp) != 0 {
		t.Fatalf("expected empty popup, got %v", exCandidates(m.ex.comp))
	}

	// up recalls the most-recent entry (histIdx starts at len → newest).
	m.handleExKey(tea.KeyMsg{Type: tea.KeyUp})
	if m.ex.input != "select 2" {
		t.Errorf("after up, input = %q, want %q", m.ex.input, "select 2")
	}
	// down steps back toward fresh input.
	m.handleExKey(tea.KeyMsg{Type: tea.KeyDown})
	if m.ex.input != "" {
		t.Errorf("after down, input = %q, want empty", m.ex.input)
	}
}

// TestExCompletionRecallPersistsAcrossPopup: once up/down starts walking
// history it keeps walking even when a recalled value would itself show a
// popup (the "recalling" flag); typing clears the flag so up/down returns to
// popup navigation. This preserves the vim-style ":<up><up><down>" history walk.
func TestExCompletionRecallPersistsAcrossPopup(t *testing.T) {
	dir := setupPathFixture(t)
	recent := "e " + dir + "/alpha.sql" // recalled value that has its own popup
	older := "e " + dir + "/"

	m := &Model{}
	m.ex.hist = []string{older, recent}
	m.ex.Open() // input empty → first up recalls rather than navigating

	// up → most recent (which itself would show a path popup).
	m.handleExKey(tea.KeyMsg{Type: tea.KeyUp})
	if m.ex.input != recent {
		t.Fatalf("first up: input = %q, want %q", m.ex.input, recent)
	}
	if len(m.ex.comp) == 0 {
		t.Fatal("recalled value should show a popup, but comp is empty")
	}
	// up again → keeps walking history despite the popup (recalling flag).
	m.handleExKey(tea.KeyMsg{Type: tea.KeyUp})
	if m.ex.input != older {
		t.Errorf("second up: input = %q, want %q (walk should continue)", m.ex.input, older)
	}
	// down → back toward the most recent.
	m.handleExKey(tea.KeyMsg{Type: tea.KeyDown})
	if m.ex.input != recent {
		t.Errorf("down: input = %q, want %q", m.ex.input, recent)
	}
}

// TestExCompletionPopupSelResetsOnType: moving the selection is forgotten once
// the list changes (typing recomputes and resets selIdx to the top match).
func TestExCompletionPopupSelResetsOnType(t *testing.T) {
	dir := setupPathFixture(t)
	m := &Model{}
	m.ex.Open()
	m.ex.input = "e " + dir + "/"
	m.recomputeExCompletion()
	m.handleExKey(tea.KeyMsg{Type: tea.KeyDown}) // selIdx → 1
	if m.ex.selIdx != 1 {
		t.Fatalf("selIdx = %d, want 1", m.ex.selIdx)
	}
	m.handleExKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}})
	if m.ex.selIdx != 0 {
		t.Errorf("after typing, selIdx = %d, want 0 (reset)", m.ex.selIdx)
	}
}

// TestExPopupWindow covers the sliding window: it keeps selIdx visible without
// running past the slice ends, and shows the whole list when it fits.
func TestExPopupWindow(t *testing.T) {
	const maxRows = 9
	comp := make([]exCompItem, 12) // content irrelevant; only length matters

	items, sel := exPopupWindow(comp, 0, maxRows)
	if len(items) != maxRows || sel != 0 {
		t.Errorf("selIdx=0: len=%d sel=%d, want %d/0", len(items), sel, maxRows)
	}
	items, sel = exPopupWindow(comp, 9, maxRows)
	if len(items) != maxRows || sel != maxRows-1 {
		t.Errorf("selIdx=9: len=%d sel=%d, want %d/%d", len(items), sel, maxRows, maxRows-1)
	}
	items, sel = exPopupWindow(comp, 11, maxRows)
	if len(items) != maxRows || sel != maxRows-1 {
		t.Errorf("selIdx=11: len=%d sel=%d, want %d/%d", len(items), sel, maxRows, maxRows-1)
	}
	items, sel = exPopupWindow(comp, -1, maxRows) // negative clamps to 0
	if sel != 0 {
		t.Errorf("selIdx=-1: sel=%d, want 0", sel)
	}
	short := make([]exCompItem, 3)
	items, sel = exPopupWindow(short, 2, maxRows)
	if len(items) != 3 || sel != 2 {
		t.Errorf("short list: len=%d sel=%d, want 3/2", len(items), sel)
	}
	items, sel = exPopupWindow(nil, 0, maxRows)
	if items != nil || sel != 0 {
		t.Errorf("empty: items=%v sel=%d, want nil/0", items, sel)
	}
}
