package ui

// Binding describes a single keybinding shown in the help overlay.
//
// Display is the human-readable key string the user sees. Tokens are the
// actual tea.KeyMsg.String() values that the key dispatch handles; the
// drift-detection test asserts every Token appears as a case literal (or a
// key.WithKeys argument) in the dispatch. Storing the two together keeps a
// binding's documentation and its implementation in one place, so they
// cannot silently drift apart.
//
// Hint is a compact, key-only string shown in the status-bar hint line for
// the current panel or modal overlay. When empty the binding is not shown in
// the hint line (it is still documented in the full help overlay).
type Binding struct {
	Display string   // e.g. "g g / G", "ctrl+h/j/k/l"
	Tokens  []string // dispatch tokens, e.g. ["g", "G"]
	Desc    string   // short human description
	Hint    string   // compact key-only string for the status-bar hint line; empty = not hint-worthy
}

// Section groups related bindings under a heading.
//
// Source is a space-separated list of source-file basenames (within this
// package) whose switch statements implement the section's keybindings. The
// drift test asserts every documented Token appears in at least one of those
// files.
type Section struct {
	Title  string
	Source string
	Items  []Binding
}

// registry is the single source of truth for the keybinding help overlay.
//
// To add or change a keybinding: update its Display (what the user sees) and
// Tokens (what the dispatch handles) here, then run the drift test to confirm
// the dispatch matches.
func registry() []Section {
	return []Section{
		{
			Title:  "Global",
			Source: "app.go",
			Items: []Binding{
				{"ctrl+e / \\", []string{"ctrl+e", "\\"}, "run statement under cursor", ""},
				{"ctrl+r", []string{"ctrl+r"}, "clear editor", ""},
			{"ctrl+w", []string{"ctrl+w"}, "maximize / restore editor", "ctrl+w"},
				{"ctrl+t", []string{"ctrl+t"}, "switch connection", ""},
				{"ctrl+b", []string{"ctrl+b"}, "browse databases (MySQL)", ""},
				{"ctrl+y", []string{"ctrl+y"}, "query history", ""},
				{"ctrl+g", []string{"ctrl+g"}, "bookmarks", ""},
				{"B", []string{"B"}, "bookmark current query", ""},
				{"ctrl+o", []string{"ctrl+o"}, "toggle inspector", ""},
				{"ctrl+h/j/k/l", []string{"ctrl+h", "ctrl+j", "ctrl+k", "ctrl+l"}, "move focus", ""},
				{"tab / shift+tab", []string{"tab", "shift+tab"}, "cycle focus", ""},
				{"ctrl+d / ctrl+u", []string{"ctrl+d", "ctrl+u"}, "next / prev page", ""},
				{"ctrl+p", []string{"ctrl+p"}, "command palette", ""},
				{"?", []string{"?"}, "toggle this help", ""},
				{"q / ctrl+q / ctrl+c", []string{"q", "ctrl+q", "ctrl+c"}, "quit (not while editing)", ""},
			},
		},
		{
			Title:  "Sidebar (Tables)",
			Source: "app.go",
			Items: []Binding{
				{"j/k, ↑/↓", []string{"j", "k", "up", "down"}, "move", "j/k"},
				{"g g / G", []string{"g", "G"}, "top / bottom", ""},
				{"space", []string{" "}, "expand columns", "space"},
				{"enter / s", []string{"enter", "s"}, "select * from table", "enter"},
				{"d", []string{"d"}, "edit schema (grid)", "d"},
				{"a", []string{"a"}, "add column", "a"},
				{"r", []string{"r"}, "rename table", "r"},
				{"T", []string{"T"}, "truncate table", "T"},
				{"D", []string{"D"}, "drop table", "D"},
				{"N", []string{"N"}, "new table (grid editor)", "N"},
				{"X", []string{"X"}, "export database", "X"},
				{"I", []string{"I"}, "import SQL dump", ""},
				{"/", []string{"/"}, "filter tables", "/"},
			},
		},
		{
			Title:  "Schema Editor",
			Source: "schema_editor.go app.go",
			Items: []Binding{
				{"h/j/k/l", []string{"h", "j", "k", "l"}, "move cell", ""},
				{"e / i", []string{"e", "i"}, "edit cell", ""},
				{"o", []string{"o"}, "add column", ""},
				{"enter", []string{"enter"}, "apply change", ""},
				{"dd", []string{"d"}, "drop column", ""},
				{"esc", []string{"esc"}, "done", ""},
			},
		},
		{
			Title:  "Export Picker",
			Source: "app.go",
			Items: []Binding{
				{"j/k", []string{"j", "k"}, "move", "j/k"},
				{"space", []string{" "}, "toggle table", "space"},
				{"a", []string{"a"}, "select all", "a"},
				{"n", []string{"n"}, "select none", "n"},
				{"enter", []string{"enter"}, "export", "enter"},
				{"esc", []string{"esc"}, "cancel", "esc"},
			},
		},
		{
			Title:  "History Panel",
			Source: "app.go",
			Items: []Binding{
				{"j/k", []string{"j", "k"}, "move", "j/k"},
				{"enter", []string{"enter"}, "load query into editor", "enter"},
				{"b", []string{"b"}, "bookmark selected query", ""},
				{"D", []string{"D"}, "clear history", ""},
				{"esc", []string{"esc"}, "close", "esc"},
			},
		},
		{
			Title:  "Bookmarks Panel",
			Source: "app.go",
			Items: []Binding{
				{"j/k", []string{"j", "k"}, "move", "j/k"},
				{"enter", []string{"enter"}, "load query into editor", "enter"},
				{"d", []string{"d"}, "delete bookmark", ""},
				{"D", []string{"D"}, "clear bookmarks", ""},
				{"esc", []string{"esc"}, "close", "esc"},
			},
		},
		{
			Title:  "Table Designer",
			Source: "table_designer.go app.go",
			Items: []Binding{
				{"h/j/k/l", []string{"h", "j", "k", "l"}, "move cell", ""},
				{"e / i", []string{"e", "i"}, "edit cell", ""},
				{"o / O", []string{"o", "O"}, "add row below / above", ""},
				{"dd", []string{"d"}, "remove row", ""},
				{"enter", []string{"enter"}, "create table", ""},
				{"esc", []string{"esc"}, "cancel", ""},
			},
		},
		{
			Title:  "Editor (Vim)",
			Source: "query_editor.go app.go",
			Items: []Binding{
				{"i/a/o/A/O", []string{"i", "a", "o", "A", "O"}, "insert mode", "i/a/o"},
				{"esc", []string{"esc"}, "normal mode", "esc"},
				{"h/j/k/l, w/b", []string{"h", "j", "k", "l", "w", "b"}, "move", "h/j/k/l"},
				{"x / dd / dw / D", []string{"x", "d", "w", "D"}, "delete", "x/dd"},
				{"y / p", []string{"y", "p"}, "yank / paste", "y/p"},
				{"ctrl+n", []string{"ctrl+n"}, "autocomplete", "ctrl+n"},
			},
		},
		{
			Title:  "Results",
			Source: "app.go",
			Items: []Binding{
				{"h/j/k/l", []string{"h", "j", "k", "l"}, "move cursor", "h/j/k/l"},
				{"0 / $", []string{"0", "$"}, "first / last column", "0/$"},
				{"g g / G", []string{"g", "G"}, "top / bottom", "G"},
				{"y y", []string{"y"}, "copy cell", "y"},
				{"g d", []string{"g", "d"}, "follow foreign key", ""},
				{"g b", []string{"g", "b"}, "go back", ""},
				{"/", []string{"/"}, "search all columns", "/"},
				{"g f", []string{"g", "f"}, "filter column values", ""},
				{"*", []string{"*"}, "keep rows equal to cursor cell", ""},
				{"!", []string{"!"}, "hide rows equal to cursor cell", ""},
				{"space", []string{" "}, "toggle row mark", "space"},
				{"F", []string{"F"}, "filter by marked rows", ""},
				{"C", []string{"C"}, "clear marks", ""},
				{"dd", []string{"d"}, "delete marked or cursor row", "dd"},
				{"V", []string{"V"}, "visual mode (select range)", "V"},
				{"u", []string{"u"}, "undo last filter", "u"},
				{"c", []string{"c"}, "clear filters", "c"},
				{"o", []string{"o"}, "sort column", "o"},
				{"g s", []string{"g", "s"}, "column stats", ""},
				{":", []string{":"}, "jump to column", ":"},
				{"H", []string{"H"}, "hide column", "H"},
				{"g H", []string{"g", "H"}, "show all columns", ""},
				{"v", []string{"v"}, "column visibility", "v"},
				{"g /", []string{"g", "/"}, "regex search", "g/"},
				{"n / N", []string{"n", "N"}, "next / prev match", "n"},
				{"x", []string{"x"}, "export to CSV", "x"},
				{"e", []string{"e"}, "edit cell", "e"},
				{"E", []string{"E"}, "expand cell (large values)", "E"},
				{"ctrl+s", []string{"ctrl+s"}, "save edits", "ctrl+s"},
				{"A", []string{"A"}, "insert new row", "A"},
				{"D", []string{"D"}, "discard edits", ""},
			},
		},
		{
			Title:  "Inspector",
			Source: "app.go",
			Items: []Binding{
				{"j/k", []string{"j", "k"}, "move field", "j/k"},
				{"/", []string{"/"}, "filter fields", "/"},
				{"e", []string{"e"}, "edit field", "e"},
				{"E", []string{"E"}, "expand field (large values)", "E"},
				{"ctrl+s", []string{"ctrl+s"}, "save", "ctrl+s"},
				{"ctrl+o", []string{"ctrl+o"}, "close", ""},
			},
		},
		{
			Title:  "Cell Editor",
			Source: "app.go",
			Items: []Binding{
				{"ctrl+s", []string{"ctrl+s"}, "stage edit & close", "ctrl+s"},
				{"esc", []string{"esc"}, "cancel", "esc"},
			},
		},
		{
			Title:  "Import Prompt",
			Source: "import_prompt.go app.go",
			Items: []Binding{
				{"tab", []string{"tab"}, "complete path", ""},
				{"↑/↓", []string{"up", "down"}, "navigate completions", ""},
				{"enter", []string{"enter"}, "import file", ""},
				{"esc", []string{"esc"}, "cancel", ""},
			},
		},
	}
}
