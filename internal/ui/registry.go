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
				{"ctrl+r", []string{"ctrl+r"}, "refresh schema & re-run query", ""},
				{"esc / ctrl+c", []string{"esc", "ctrl+c"}, "cancel running query", ""},
				{"ctrl+w", []string{"ctrl+w"}, "maximize / restore editor", ""},
				{"ctrl+t", []string{"ctrl+t"}, "switch connection", ""},
				{"ctrl+b", []string{"ctrl+b"}, "browse databases (MySQL)", ""},
				{"ctrl+y", []string{"ctrl+y"}, "query history", ""},
				{"ctrl+g", []string{"ctrl+g"}, "bookmarks", ""},
				{"B", []string{"B"}, "bookmark current query", ""},
				{"ctrl+o", []string{"ctrl+o"}, "toggle inspector", ""},
				{"ctrl+f", []string{"ctrl+f"}, "toggle AI assistant", ""},
				{"ctrl+h/j/k/l", []string{"ctrl+h", "ctrl+j", "ctrl+k", "ctrl+l"}, "move focus", ""},
				{"tab / shift+tab", []string{"tab", "shift+tab"}, "cycle focus (skips tab bar)", ""},
				{"ctrl+d / ctrl+u", []string{"ctrl+d", "ctrl+u"}, "next / prev page", ""},
				{"ctrl+p", []string{"ctrl+p"}, "command palette", ""},
				{":", []string{":"}, "ex command line", ":"},
				{"?", []string{"?"}, "toggle this help", ""},
				{"q / ctrl+q / ctrl+c", []string{"q", "ctrl+q", "ctrl+c"}, "quit (not while editing)", ""},
			},
		},
		{
			Title:  "Tabs",
			Source: "app.go",
			Items: []Binding{
				{"g t / g T", []string{"g", "t", "T"}, "next / previous tab", ""},
				{"t", []string{"t"}, "new tab", ""},
				{"g x", []string{"g", "x"}, "close tab", ""},
				{"g 1-9", []string{"g", "1", "2", "3", "4", "5", "6", "7", "8", "9"}, "go to tab 1-9", ""},
			},
		},
		{
			Title:  "Connections",
			Source: "app.go",
			Items: []Binding{
				{"j/k, ↑/↓", []string{"j", "k", "up", "down"}, "move", "j/k"},
				{"enter", []string{"enter"}, "connect", "enter"},
				{"n", []string{"n"}, "new connection", "n"},
				{"e", []string{"e"}, "edit connection", "e"},
				{"d", []string{"d"}, "delete connection", "d"},
				{"/", []string{"/"}, "filter connections", "/"},
				{"esc / q", []string{"esc", "q"}, "quit", "esc"},
			},
		},
		{
			Title:  "Tab Bar",
			Source: "app.go",
			Items: []Binding{
				{"h/l, ←/→", []string{"h", "l", "left", "right"}, "switch tab", "h/l"},
				{"t", []string{"t"}, "new tab", "t"},
				{"enter / j", []string{"enter", "j", "down"}, "focus editor", "enter"},
			},
		},
		{
			Title:  "Sidebar (Tables)",
			Source: "app.go",
			Items: []Binding{
				{"j/k, ↑/↓", []string{"j", "k", "up", "down"}, "move", "j/k"},
				{"l", []string{"l", "right"}, "focus results", "l"},
				{"g g / G", []string{"g", "G"}, "top / bottom", ""},
				{"space", []string{" "}, "expand columns", "space"},
				{"enter / s", []string{"enter", "s"}, "select * from table", "enter"},
				{"d", []string{"d"}, "structure (columns/indexes/triggers)", "d"},
				{"a", []string{"a"}, "add column", "a"},
				{"r", []string{"r"}, "rename table", "r"},
				{"T", []string{"T"}, "truncate table", "T"},
				{"D", []string{"D"}, "drop table", "D"},
				{"N", []string{"N"}, "new table (grid editor)", "N"},
				{"X", []string{"X"}, "export database", "X"},
				{"I", []string{"I"}, "import SQL dump", ""},
				{"S", []string{"S"}, "cross-table search", "S"},
				{"/", []string{"/"}, "filter tables", "/"},
			},
		},
		{
			Title:  "ERD",
			Source: "app.go erd_panel.go",
			Items: []Binding{
				{"esc / q / ctrl+c", []string{"esc", "q", "ctrl+c"}, "close ERD panel", "esc"},
				{"j/k/h/l", []string{"j", "k", "h", "l", "up", "down", "left", "right"}, "move focus between cards / scroll source", "j/k/h/l"},
				{"space", []string{" "}, "highlight focused card's relations", "space"},
				{"enter", []string{"enter"}, "browse the focused table (SELECT *)", "enter"},
				{"f", []string{"f"}, "re-focus on the focused card's neighbourhood", "f"},
				{"zz", []string{"z"}, "fit all cards to the viewport", "zz"},
				{"zc / zo / za", []string{"c", "o", "a"}, "collapse / expand / toggle the focused card", "zc/zo/za"},
				{"zM / zR", []string{"M", "R"}, "collapse / expand all cards (contract layout + re-route arrows)", "zM/zR"},
				{"/", []string{"/"}, "jump to a table by name (tab cycles matches)", "/"},
				{"p", []string{"p"}, "trace the FK path between two tables", "p"},
				{"m", []string{"m"}, "toggle Mermaid erDiagram source", "m"},
				{"g / G", []string{"g", "G"}, "top / bottom of diagram", "g/G"},
				{"ctrl+d / ctrl+u", []string{"ctrl+d", "ctrl+u"}, "page down / up", "ctrl+d/ctrl+u"},
				{"y", []string{"y", "Y"}, "copy Mermaid source to clipboard", "y"},
				{"s", []string{"s"}, "save Mermaid source to file", "s"},
			},
		},
		{
			Title:  "Relationship Explorer",
			Source: "app.go rel_explorer.go",
			Items: []Binding{
				{"j/k, ↑/↓", []string{"j", "k", "up", "down"}, "move", "j/k"},
				{"g / G", []string{"g", "G"}, "top / bottom", "g/G"},
				{"ctrl+d / ctrl+u", []string{"ctrl+d", "ctrl+u"}, "page down / up", "ctrl+d/ctrl+u"},
				{"h/l, ←/→", []string{"h", "l", "left", "right"}, "collapse / expand", "h/l"},
				{"enter", []string{"enter"}, "re-root grid on node", "enter"},
				{"t", []string{"t"}, "open node in a new tab", "t"},
				{"A", []string{"A"}, "insert related row (stays on parent)", "A"},
				{"u / g b", []string{"u", "g", "b", "backspace"}, "go back", "u"},
				{"r", []string{"r"}, "retarget / refresh", "r"},
				{"esc / q", []string{"esc", "q"}, "close", "esc"},
			},
		},
		{
			Title:  "Schema Editor",
			Source: "schema_editor.go app.go",
			Items: []Binding{
				{"h/j/k/l", []string{"h", "j", "k", "l"}, "move cell", "h/j/k/l"},
				{"e / i", []string{"e", "i"}, "edit cell", "e"},
				{"o", []string{"o"}, "add column", "o"},
				{"enter", []string{"enter"}, "apply change", "enter"},
				{"dd", []string{"d"}, "drop column", "dd"},
				{"H/L", []string{"H", "L"}, "switch tab", "H/L"},
				{"esc", []string{"esc"}, "done", "esc"},
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
			Title:  "Export Dialog",
			Source: "app.go export_overlay.go",
			Items: []Binding{
				{"j/k, ↑/↓", []string{"j", "k", "up", "down"}, "move (format / columns / scope)", "j/k"},
				{"space", []string{" "}, "select format / toggle column / select scope", "space"},
				{"a", []string{"a"}, "all columns", "a"},
				{"n", []string{"n"}, "no columns (keep one)", "n"},
				{"enter", []string{"enter"}, "export", "enter"},
				{"esc", []string{"esc"}, "cancel", "esc"},
			},
		},
		{
			Title:  "Theme Picker",
			Source: "app.go",
			Items: []Binding{
				{"g c", []string{"g", "c"}, "open theme picker (live preview)", ""},
				{"↑/↓", []string{"up", "down"}, "move; type to filter", "↑/↓"},
				{"enter", []string{"enter"}, "apply & save", "enter"},
				{"esc", []string{"esc"}, "revert", "esc"},
			},
		},
		{
			Title:  "Database Picker",
			Source: "app.go",
			Items: []Binding{
				{"j/k, ↑/↓", []string{"j", "k", "up", "down"}, "move", "j/k"},
				{"enter", []string{"enter"}, "select database", "enter"},
				{"N", []string{"N"}, "create database", "N"},
				{"D", []string{"D"}, "drop database", "D"},
				{"/", []string{"/"}, "filter databases", "/"},
				{"esc", []string{"esc"}, "cancel", "esc"},
			},
		},
		{
			Title:  "History Panel",
			Source: "app.go",
			Items: []Binding{
				{"j/k", []string{"j", "k"}, "move", "j/k"},
				{"s", []string{"s"}, "toggle sort (recent/slowest)", "s"},
				{"enter", []string{"enter"}, "load query into editor", "enter"},
				{"b", []string{"b"}, "bookmark selected query", "b"},
				{"D", []string{"D"}, "clear history", "D"},
				{"esc", []string{"esc"}, "close", "esc"},
			},
		},
		{
			Title:  "Bookmarks Panel",
			Source: "app.go",
			Items: []Binding{
				{"j/k", []string{"j", "k"}, "move", "j/k"},
				{"enter", []string{"enter"}, "load query into editor", "enter"},
				{"d", []string{"d"}, "delete bookmark", "d"},
				{"D", []string{"D"}, "clear bookmarks", "D"},
				{"esc", []string{"esc"}, "close", "esc"},
			},
		},
		{
			Title:  "Table Designer",
			Source: "table_designer.go app.go",
			Items: []Binding{
				{"h/j/k/l", []string{"h", "j", "k", "l"}, "move cell", "h/j/k/l"},
				{"e / i", []string{"e", "i"}, "edit cell", "e"},
				{"o / O", []string{"o", "O"}, "add row below / above", "o/O"},
				{"dd", []string{"d"}, "remove row", "dd"},
				{"enter", []string{"enter"}, "create table", "enter"},
				{"esc", []string{"esc"}, "cancel", "esc"},
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
				{"u / U", []string{"u", "U"}, "undo / redo", "u"},
				{"/", []string{"/"}, "search in buffer", "/"},
				{"n / N", []string{"n", "N"}, "next / prev search match", "n"},
				{"V", []string{"V"}, "visual line (yank/delete)", "V"},
				{"ctrl+n", []string{"ctrl+n"}, "autocomplete", "ctrl+n"},
				{"↑ / ↓", []string{"up", "down"}, "query history (normal mode)", ""},
				{"==", []string{"="}, "format SQL", ""},
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
				{"p", []string{"p"}, "paste clipboard to cell", "p"},
				{"g d", []string{"g", "d"}, "follow foreign key", ""},
				{"g b", []string{"g", "b"}, "go back", ""},
				{"g r", []string{"g", "r"}, "relationship explorer (panel)", ""},
				{"g R", []string{"g", "R"}, "static ERD (table cards + arrows)", ""},
				{"/", []string{"/"}, "search all columns", "/"},
				{"g f", []string{"g", "f"}, "filter column values", ""},
				{"*", []string{"*"}, "keep rows equal to cursor cell", ""},
				{"!", []string{"!"}, "hide rows equal to cursor cell", ""},
				{"space", []string{" "}, "toggle row mark", "space"},
				{"M", []string{"M"}, "toggle column mark (for :bar / :line / :scatter / :hist / :freq)", "M"},
				{"F", []string{"F"}, "filter by marked rows", ""},
				{"C", []string{"C"}, "clear marks", ""},
				{"dd", []string{"d"}, "delete marked or cursor row", "dd"},
				{"V", []string{"V"}, "visual mode (select range)", "V"},
				{"u", []string{"u"}, "undo last filter", "u"},
				{"c", []string{"c"}, "clear filters", "c"},
				{"o", []string{"o"}, "sort column", "o"},
				{"g s", []string{"g", "s"}, "column stats", ""},
				{"g e", []string{"g", "e"}, "explain query plan", ""},
				{"H", []string{"H"}, "hide column", "H"},
				{"g H", []string{"g", "H"}, "show all columns", ""},
				{"v", []string{"v"}, "column visibility", "v"},
				{"g /", []string{"g", "/"}, "regex search", "g/"},
				{"n / N", []string{"n", "N"}, "next / prev match", "n"},
				{"x", []string{"x"}, "export current page to CSV", "x"},
				{"g X", []string{"g", "X"}, "export as… (format, columns, scope)", ""},
				{"Y", []string{"Y"}, "copy rows as INSERT", "Y"},
				{"P", []string{"P"}, "clone marked/cursor row", "P"},
				{"e", []string{"e"}, "edit cell", "e"},
				{"E", []string{"E"}, "expand/view cell (multi-line)", "E"},
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
				{"E", []string{"E"}, "expand/view field (multi-line)", "E"},
				{"ctrl+s", []string{"ctrl+s"}, "save", "ctrl+s"},
				{"ctrl+o", []string{"ctrl+o"}, "close", ""},
			},
		},
		{
			Title:  "Help",
			Source: "help.go",
			Items: []Binding{
				{"tab", []string{"tab"}, "switch page", "tab"},
				{"j/k", []string{"j", "k", "up", "down"}, "scroll", "j/k"},
				{"/", []string{"/"}, "search", "/"},
				{"n / N", []string{"n", "N"}, "next / prev match", "n/N"},
				{"g / G", []string{"g", "G"}, "top / bottom", "g/G"},
				{"?", []string{"?"}, "close", "?"},
			},
		},
		{
			Title:  "Cross-Table Search",
			Source: "app.go",
			Items: []Binding{
				{"↑/↓", []string{"up", "down"}, "move result", "↑/↓"},
				{"enter", []string{"enter"}, "search / open result", "enter"},
				{"esc", []string{"esc", "ctrl+c"}, "close", "esc"},
			},
		},
		{
			Title:  "Lookup Panel",
			Source: "lookup_panel.go app.go",
			Items: []Binding{
				{"j/k", []string{"j", "k", "up", "down"}, "scroll", "j/k"},
				{"g / G", []string{"g", "G"}, "top / bottom", "g/G"},
				{"ctrl+d / ctrl+u", []string{"ctrl+d", "ctrl+u"}, "page down / up", "ctrl+d/ctrl+u"},
				{"esc", []string{"esc"}, "close", "esc"},
			},
		},
		{
			Title:  "Explain Panel",
			Source: "explain_panel.go app.go",
			Items: []Binding{
				{"j/k", []string{"j", "k", "up", "down"}, "scroll", "j/k"},
				{"g / G", []string{"g", "G"}, "top / bottom", "g/G"},
				{"ctrl+d / ctrl+u", []string{"ctrl+d", "ctrl+u"}, "page down / up", "ctrl+d/ctrl+u"},
				{"esc", []string{"esc"}, "close", "esc"},
			},
		},
		{
			Title:  "Chart Panel",
			Source: "chart_panel.go app.go",
			Items: []Binding{
				{"j/k/h/l", []string{"j", "k", "h", "l", "up", "down", "left", "right"}, "move", "j/k"},
				{"g / G", []string{"g", "G"}, "top / bottom", "g/G"},
				{"ctrl+d / ctrl+u", []string{"ctrl+d", "ctrl+u"}, "page down / up", "ctrl+d/ctrl+u"},
				{"o", []string{"o"}, "unfold / fold (other)", "o"},
				{"enter", []string{"enter"}, "keep rows for this bar", "enter"},
				{"esc", []string{"esc", "q"}, "close", "esc"},
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
				{"tab", []string{"tab"}, "complete path", "tab"},
				{"↑/↓", []string{"up", "down"}, "navigate completions", "↑/↓"},
				{"enter", []string{"enter"}, "import file", "enter"},
				{"esc", []string{"esc"}, "cancel", "esc"},
			},
		},
		{
			Title:  "Column Picker",
			Source: "column_picker.go app.go",
			Items: []Binding{
				{"↑/↓", []string{"up", "down"}, "move", "↑/↓"},
				{"space", []string{" "}, "toggle column", "space"},
				{"ctrl+a", []string{"ctrl+a"}, "show all", "ctrl+a"},
				{"ctrl+n", []string{"ctrl+n"}, "hide all", "ctrl+n"},
				{"enter", []string{"enter"}, "apply", "enter"},
				{"esc", []string{"esc"}, "cancel", "esc"},
			},
		},
		{
			Title:  "Filter Picker",
			Source: "filter_picker.go app.go",
			Items: []Binding{
				{"↑/↓", []string{"up", "down"}, "move", "↑/↓"},
				{"space", []string{" "}, "toggle value", "space"},
				{"ctrl+a", []string{"ctrl+a"}, "select all", "ctrl+a"},
				{"ctrl+n", []string{"ctrl+n"}, "select none", "ctrl+n"},
				{"enter", []string{"enter"}, "apply filter", "enter"},
				{"esc", []string{"esc"}, "cancel", "esc"},
			},
		},
		{
			Title:  "Add Column / Rename Table",
			Source: "add_column_form.go table_rename_form.go app.go",
			Items: []Binding{
				{"tab", []string{"tab"}, "next field", "tab"},
				{"enter", []string{"enter"}, "confirm", "enter"},
				{"esc", []string{"esc"}, "cancel", "esc"},
			},
		},
		{
			Title:  "Assistant",
			Source: "assistant.go app.go",
			Items: []Binding{
				{"i / a / o", []string{"i", "a", "o"}, "compose a question (insert mode)", "i/a/o"},
				{"enter", []string{"enter"}, "send / apply latest SQL to editor", "enter"},
				{"M", []string{"M"}, "switch provider", "M"},
				{"m", []string{"m"}, "browse models for active provider", "m"},
				{"j/k", []string{"j", "k", "up", "down"}, "scroll transcript", "j/k"},
				{"G", []string{"G"}, "bottom", "G"},
				{"c", []string{"c"}, "clear transcript", "c"},
				{"esc / q", []string{"esc", "q"}, "leave compose / close panel", "esc"},
			},
		},
		{
			Title:  "AI Provider",
			Source: "app.go",
			Items: []Binding{
				{"j/k, \u2191/\u2193", []string{"j", "k", "up", "down"}, "move", "j/k"},
				{"enter", []string{"enter"}, "select provider", "enter"},
				{"n", []string{"n"}, "new provider", "n"},
				{"e", []string{"e"}, "edit provider", "e"},
				{"d", []string{"d"}, "delete provider", "d"},
				{"esc", []string{"esc"}, "cancel", "esc"},
			},
		},
		{
			Title:  "Model Browser",
			Source: "app.go",
			Items: []Binding{
				{"j/k", []string{"j", "k", "up", "down"}, "move", "j/k"},
				{"enter", []string{"enter"}, "select model", "enter"},
				{"esc", []string{"esc"}, "cancel", "esc"},
			},
		},
	}
}
