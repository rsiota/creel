package ui

// Binding describes a single keybinding shown in the help overlay.
//
// Display is the human-readable key string the user sees. Tokens are the
// actual tea.KeyMsg.String() values that the key dispatch handles; the
// drift-detection test asserts every Token appears as a case literal (or a
// key.WithKeys argument) in the dispatch. Storing the two together keeps a
// binding's documentation and its implementation in one place, so they
// cannot silently drift apart.
type Binding struct {
	Display string   // e.g. "g g / G", "ctrl+h/j/k/l"
	Tokens  []string // dispatch tokens, e.g. ["g", "G"]
	Desc    string   // short human description
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
				{"ctrl+e / \\", []string{"ctrl+e", "\\"}, "run statement under cursor"},
				{"ctrl+r", []string{"ctrl+r"}, "clear editor"},
				{"ctrl+t", []string{"ctrl+t"}, "switch connection"},
				{"ctrl+b", []string{"ctrl+b"}, "browse databases (MySQL)"},
				{"ctrl+y", []string{"ctrl+y"}, "query history"},
				{"ctrl+g", []string{"ctrl+g"}, "bookmarks"},
				{"B", []string{"B"}, "bookmark current query"},
				{"ctrl+o", []string{"ctrl+o"}, "toggle inspector"},
				{"ctrl+h/j/k/l", []string{"ctrl+h", "ctrl+j", "ctrl+k", "ctrl+l"}, "move focus"},
				{"tab / shift+tab", []string{"tab", "shift+tab"}, "cycle focus"},
				{"ctrl+d / ctrl+u", []string{"ctrl+d", "ctrl+u"}, "next / prev page"},
				{"ctrl+p", []string{"ctrl+p"}, "command palette"},
				{"?", []string{"?"}, "toggle this help"},
				{"q / ctrl+q / ctrl+c", []string{"q", "ctrl+q", "ctrl+c"}, "quit (not while editing)"},
			},
		},
		{
			Title:  "Sidebar (Tables)",
			Source: "app.go",
			Items: []Binding{
				{"j/k, ↑/↓", []string{"j", "k", "up", "down"}, "move"},
				{"g g / G", []string{"g", "G"}, "top / bottom"},
				{"space", []string{" "}, "expand columns"},
				{"enter / s", []string{"enter", "s"}, "select * from table"},
				{"d", []string{"d"}, "edit schema (grid)"},
				{"a", []string{"a"}, "add column"},
				{"r", []string{"r"}, "rename table"},
				{"T", []string{"T"}, "truncate table"},
				{"D", []string{"D"}, "drop table"},
				{"N", []string{"N"}, "new table (grid editor)"},
				{"X", []string{"X"}, "export database"},
				{"I", []string{"I"}, "import SQL dump"},
				{"/", []string{"/"}, "filter tables"},
			},
		},
		{
			Title:  "Schema Editor",
			Source: "schema_editor.go app.go",
			Items: []Binding{
				{"h/j/k/l", []string{"h", "j", "k", "l"}, "move cell"},
				{"e / i", []string{"e", "i"}, "edit cell"},
				{"o", []string{"o"}, "add column"},
				{"enter", []string{"enter"}, "apply change"},
				{"dd", []string{"d"}, "drop column"},
				{"esc", []string{"esc"}, "done"},
			},
		},
		{
			Title:  "Export Picker",
			Source: "app.go",
			Items: []Binding{
				{"j/k", []string{"j", "k"}, "move"},
				{"space", []string{" "}, "toggle table"},
				{"a", []string{"a"}, "select all"},
				{"n", []string{"n"}, "select none"},
				{"enter", []string{"enter"}, "export"},
				{"esc", []string{"esc"}, "cancel"},
			},
		},
		{
			Title:  "History Panel",
			Source: "app.go",
			Items: []Binding{
				{"j/k", []string{"j", "k"}, "move"},
				{"enter", []string{"enter"}, "load query into editor"},
				{"b", []string{"b"}, "bookmark selected query"},
				{"D", []string{"D"}, "clear history"},
				{"esc", []string{"esc"}, "close"},
			},
		},
		{
			Title:  "Bookmarks Panel",
			Source: "app.go",
			Items: []Binding{
				{"j/k", []string{"j", "k"}, "move"},
				{"enter", []string{"enter"}, "load query into editor"},
				{"d", []string{"d"}, "delete bookmark"},
				{"D", []string{"D"}, "clear bookmarks"},
				{"esc", []string{"esc"}, "close"},
			},
		},
		{
			Title:  "Table Designer",
			Source: "table_designer.go app.go",
			Items: []Binding{
				{"h/j/k/l", []string{"h", "j", "k", "l"}, "move cell"},
				{"e / i", []string{"e", "i"}, "edit cell"},
				{"o / O", []string{"o", "O"}, "add row below / above"},
				{"dd", []string{"d"}, "remove row"},
				{"enter", []string{"enter"}, "create table"},
				{"esc", []string{"esc"}, "cancel"},
			},
		},
		{
			Title:  "Editor (Vim)",
			Source: "query_editor.go app.go",
			Items: []Binding{
				{"i/a/o/A/O", []string{"i", "a", "o", "A", "O"}, "insert mode"},
				{"esc", []string{"esc"}, "normal mode"},
				{"h/j/k/l, w/b", []string{"h", "j", "k", "l", "w", "b"}, "move"},
				{"x / dd / dw / D", []string{"x", "d", "w", "D"}, "delete"},
				{"y / p", []string{"y", "p"}, "yank / paste"},
				{"ctrl+n", []string{"ctrl+n"}, "autocomplete"},
			},
		},
		{
			Title:  "Results",
			Source: "app.go",
			Items: []Binding{
				{"h/j/k/l", []string{"h", "j", "k", "l"}, "move cursor"},
				{"g g / G", []string{"g", "G"}, "top / bottom"},
				{"y y", []string{"y"}, "copy cell"},
				{"g d", []string{"g", "d"}, "follow foreign key"},
				{"g b", []string{"g", "b"}, "go back"},
				{"/", []string{"/"}, "filter column values"},
				{"*", []string{"*"}, "keep rows equal to cursor cell"},
				{"!", []string{"!"}, "hide rows equal to cursor cell"},
				{"space", []string{" "}, "toggle row mark"},
				{"F", []string{"F"}, "filter by marked rows"},
				{"C", []string{"C"}, "clear marks"},
				{"dd", []string{"d"}, "delete marked or cursor row"},
				{"V", []string{"V"}, "visual mode (select range)"},
				{"u", []string{"u"}, "undo last filter"},
				{"c", []string{"c"}, "clear filters"},
				{"o", []string{"o"}, "sort column"},
				{"g s", []string{"g", "s"}, "column stats"},
				{":", []string{":"}, "jump to column"},
				{"H", []string{"H"}, "hide column"},
				{"g H", []string{"g", "H"}, "show all columns"},
				{"v", []string{"v"}, "column visibility"},
				{"g /", []string{"g", "/"}, "regex search"},
				{"n / N", []string{"n", "N"}, "next / prev match"},
				{"x", []string{"x"}, "export to CSV"},
				{"e", []string{"e"}, "edit cell"},
				{"ctrl+s", []string{"ctrl+s"}, "save edits"},
				{"A", []string{"A"}, "insert new row"},
				{"D", []string{"D"}, "discard edits"},
			},
		},
		{
			Title:  "Inspector",
			Source: "app.go",
			Items: []Binding{
				{"j/k", []string{"j", "k"}, "move field"},
				{"/", []string{"/"}, "filter fields"},
				{"e", []string{"e"}, "edit field"},
				{"ctrl+s", []string{"ctrl+s"}, "save"},
				{"ctrl+o", []string{"ctrl+o"}, "close"},
			},
		},
	}
}
