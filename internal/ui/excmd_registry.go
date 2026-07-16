package ui

import (
	tea "github.com/charmbracelet/bubbletea"
)

// exCmdSpec describes one ":" command: the verb(s) that invoke it, a short
// description and usage string for help/autocomplete, an argument kind for
// validation/hints, and the executor. The table returned by exCommands is the
// single source of truth for the ":" command line; runExCommand resolves the
// parsed verb here and calls the matching executor.
//
// verbs[0] is the canonical name (used in help/autocomplete); the rest are
// aliases. Verb lookup is case-insensitive — parseExLine lower-cases the verb.
type exCmdSpec struct {
	verbs   []string // canonical first, then aliases (e.g. ["write", "w"])
	desc    string   // short human description
	usage   string   // usage shown in help/autocomplete, e.g. ":w [file]"
	argKind exArgKind
	run     func(m *Model, args []string, force bool) tea.Cmd
}

// exArgKind classifies a command's arguments, used by autocomplete hints and
// (later) by argument completion.
type exArgKind int

const (
	exArgNone     exArgKind = iota // no arguments
	exArgOptional                  // zero or one argument
	exArgRequired                  // exactly one argument required
	exArgTable                     // optional table name; defaults to current
)

// exCommands returns every ":" command, in display order. To add a command,
// append a spec here; runExCommand, autocomplete, and the help listing all
// pick it up automatically. The executors are thin wrappers over the ex*
// helper methods in excmd.go, so this table is pure dispatch — no logic lives
// here that isn't "which verb runs which method with which args".
func exCommands() []exCmdSpec {
	return []exCmdSpec{
		{
			verbs:   []string{"edit", "e"},
			desc:    "load a file into the editor",
			usage:   ":e <file>",
			argKind: exArgRequired,
			run: func(m *Model, args []string, _ bool) tea.Cmd {
				if len(args) == 0 {
					m.schemaMsg = ":e needs a file path"
					return nil
				}
				return m.exEditFile(args[0])
			},
		},
		{
			verbs:   []string{"write", "w"},
			desc:    "save edits, or write the editor buffer to a file",
			usage:   ":w [file]",
			argKind: exArgOptional,
			run: func(m *Model, args []string, force bool) tea.Cmd {
				// With a file argument, write the editor buffer to disk
				// (vim :w file); without one, :w saves staged cell edits
				// (the legacy meaning).
				if len(args) > 0 {
					return m.exWriteFile(args[0])
				}
				return m.exWrite(force)
			},
		},
		{
			verbs:   []string{"quit", "q"},
			desc:    "close the active tab (quit if last)",
			usage:   ":q[!]",
			argKind: exArgNone,
			run:     func(m *Model, _ []string, force bool) tea.Cmd { return m.exQuit(force) },
		},
		{
			verbs:   []string{"wq", "x"},
			desc:    "save edits and close the tab (quit if last)",
			usage:   ":wq",
			argKind: exArgNone,
			run: func(m *Model, _ []string, _ bool) tea.Cmd {
				save := m.saveEdits() // no-op when there are no dirty cells
				if len(m.resultsTabs) > 1 {
					m.closeTab(m.activeTabID)
					return save
				}
				// Last tab: save (if any) then quit, sequenced so the write
				// completes before the program exits.
				m.quitting = true
				if save == nil {
					return tea.Quit
				}
				return tea.Sequence(save, tea.Quit)
			},
		},
		{
			verbs:   []string{"sort"},
			desc:    "sort results by a column",
			usage:   ":sort <column>",
			argKind: exArgRequired,
			run: func(m *Model, args []string, _ bool) tea.Cmd {
				if len(args) == 0 {
					m.schemaMsg = ":sort needs a column name"
					return nil
				}
				return m.sortByColName(args[0])
			},
		},
		{
			verbs:   []string{"goto", "gt"},
			desc:    "open a table by name (SELECT *)",
			usage:   ":goto <table>",
			argKind: exArgRequired,
			run: func(m *Model, args []string, _ bool) tea.Cmd {
				if len(args) == 0 {
					m.schemaMsg = ":goto needs a table name"
					return nil
				}
				return m.exGoto(args[0])
			},
		},
		{
			verbs:   []string{"export"},
			desc:    "export the current result set to ~/Downloads",
			usage:   ":export <csv|json|jsonl|md|tsv>",
			argKind: exArgOptional,
			run: func(m *Model, args []string, _ bool) tea.Cmd {
				arg := ""
				if len(args) > 0 {
					arg = args[0]
				}
				return m.exExport(arg)
			},
		},
		{
			verbs:   []string{"import"},
			desc:    "import a SQL dump file into the database",
			usage:   ":import <file>",
			argKind: exArgRequired,
			run: func(m *Model, args []string, _ bool) tea.Cmd {
				if len(args) == 0 {
					m.schemaMsg = ":import needs a file path"
					return nil
				}
				return m.exImport(args[0])
			},
		},
		{
			verbs:   []string{"refs", "references"},
			desc:    "foreign keys referencing a table",
			usage:   ":refs [table]",
			argKind: exArgTable,
			run: func(m *Model, args []string, _ bool) tea.Cmd {
				arg := ""
				if len(args) > 0 {
					arg = args[0]
				}
				return m.exRefs(arg)
			},
		},
		{
			verbs:   []string{"uses"},
			desc:    "objects referencing a table in their definition",
			usage:   ":uses [table]",
			argKind: exArgTable,
			run: func(m *Model, args []string, _ bool) tea.Cmd {
				arg := ""
				if len(args) > 0 {
					arg = args[0]
				}
				return m.exUses(arg)
			},
		},
		{
			verbs:   []string{"begin", "transaction"},
			desc:    "start a manual transaction",
			usage:   ":begin",
			argKind: exArgNone,
			run:     func(m *Model, _ []string, _ bool) tea.Cmd { return m.exBegin() },
		},
		{
			verbs:   []string{"commit"},
			desc:    "commit the active transaction",
			usage:   ":commit",
			argKind: exArgNone,
			run:     func(m *Model, _ []string, _ bool) tea.Cmd { return m.exCommit() },
		},
		{
			verbs:   []string{"rollback"},
			desc:    "roll back the active transaction",
			usage:   ":rollback",
			argKind: exArgNone,
			run:     func(m *Model, _ []string, _ bool) tea.Cmd { return m.exRollback() },
		},
		{
			verbs:   []string{"help", "h"},
			desc:    "open the help overlay",
			usage:   ":help",
			argKind: exArgNone,
			run:     func(m *Model, _ []string, _ bool) tea.Cmd { m.help.Show(); return nil },
		},
		{
			verbs:   []string{"explain"},
			desc:    "show the query plan for the editor's statement",
			usage:   ":explain",
			argKind: exArgNone,
			run:     func(m *Model, _ []string, _ bool) tea.Cmd { return m.explainQuery() },
		},
		{
			verbs:   []string{"refresh", "reload"},
			desc:    "refresh schema and re-run the last query",
			usage:   ":refresh",
			argKind: exArgNone,
			run:     func(m *Model, _ []string, _ bool) tea.Cmd { return m.refreshSchema() },
		},
		{
			verbs:   []string{"history"},
			desc:    "toggle the query history panel",
			usage:   ":history",
			argKind: exArgNone,
			run:     func(m *Model, _ []string, _ bool) tea.Cmd { m.toggleHistory(); return nil },
		},
		{
			verbs:   []string{"rerun"},
			desc:    "re-run a query from history by rank (1 = most recent)",
			usage:   ":rerun <n>",
			argKind: exArgRequired,
			run: func(m *Model, args []string, _ bool) tea.Cmd {
				if len(args) == 0 {
					m.schemaMsg = ":rerun needs a number (1 = most recent)"
					return nil
				}
				return m.exRerun(args[0])
			},
		},
		{
			verbs:   []string{"watch"},
			desc:    "re-run the last query on a timer",
			usage:   ":watch [n|off]",
			argKind: exArgOptional,
			run: func(m *Model, args []string, _ bool) tea.Cmd {
				arg := ""
				if len(args) > 0 {
					arg = args[0]
				}
				return m.exWatch(arg)
			},
		},
		{
			verbs:   []string{"tail"},
			desc:    "stream the newest rows of a table on a timer",
			usage:   ":tail [table] [n]",
			argKind: exArgTable,
			run:     func(m *Model, args []string, _ bool) tea.Cmd { return m.exTail(args) },
		},
		{
			verbs:   []string{"limit"},
			desc:    "set the results page size",
			usage:   ":limit <n>|off",
			argKind: exArgOptional,
			run: func(m *Model, args []string, _ bool) tea.Cmd {
				arg := ""
				if len(args) > 0 {
					arg = args[0]
				}
				return m.exLimit(arg)
			},
		},
		{
			verbs:   []string{"timing"},
			desc:    "toggle showing query elapsed time",
			usage:   ":timing [on|off]",
			argKind: exArgOptional,
			run: func(m *Model, args []string, _ bool) tea.Cmd {
				arg := ""
				if len(args) > 0 {
					arg = args[0]
				}
				return m.exTiming(arg)
			},
		},
		{
			verbs:   []string{"peek"},
			desc:    "one-glance summary of a table",
			usage:   ":peek [table]",
			argKind: exArgTable,
			run:     func(m *Model, args []string, _ bool) tea.Cmd { return m.exPeek(args) },
		},
		{
			verbs:   []string{"bookmark"},
			desc:    "bookmark the editor's current query",
			usage:   ":bookmark",
			argKind: exArgNone,
			run:     func(m *Model, _ []string, _ bool) tea.Cmd { m.bookmarkCurrentQuery(); return nil },
		},
		{
			verbs:   []string{"bookmarks", "bm"},
			desc:    "toggle the bookmarks panel",
			usage:   ":bookmarks",
			argKind: exArgNone,
			run:     func(m *Model, _ []string, _ bool) tea.Cmd { m.toggleBookmarks(); return nil },
		},
		{
			verbs:   []string{"describe", "desc"},
			desc:    "open the structure view for a table",
			usage:   ":describe [table]",
			argKind: exArgTable,
			run: func(m *Model, args []string, _ bool) tea.Cmd {
				name := ""
				if len(args) > 0 {
					name = args[0]
				}
				return m.exDescribe(name)
			},
		},
		{
			verbs:   []string{"stats"},
			desc:    "summary stats for a column (min/max/avg/…)",
			usage:   ":stats [column]",
			argKind: exArgOptional,
			run: func(m *Model, args []string, _ bool) tea.Cmd {
				arg := ""
				if len(args) > 0 {
					arg = args[0]
				}
				return m.exStats(arg)
			},
		},
		{
			verbs:   []string{"count"},
			desc:    "row count for a table (SELECT count(*))",
			usage:   ":count [table]",
			argKind: exArgTable,
			run: func(m *Model, args []string, _ bool) tea.Cmd {
				name := ""
				if len(args) > 0 {
					name = args[0]
				}
				return m.exCount(name)
			},
		},
		{
			verbs:   []string{"sample", "head"},
			desc:    "peek at the first rows of a table",
			usage:   ":sample [table]",
			argKind: exArgTable,
			run: func(m *Model, args []string, _ bool) tea.Cmd {
				name := ""
				if len(args) > 0 {
					name = args[0]
				}
				return m.exSample(name)
			},
		},
		{
			verbs:   []string{"format"},
			desc:    "format the SQL in the editor",
			usage:   ":format",
			argKind: exArgNone,
			run: func(m *Model, _ []string, _ bool) tea.Cmd {
				m.editor.SetValue(formatSQL(m.editor.Value()))
				return nil
			},
		},
		{
			verbs:   []string{"theme"},
			desc:    "switch to a named theme",
			usage:   ":theme <name>",
			argKind: exArgRequired,
			run: func(m *Model, args []string, _ bool) tea.Cmd {
				if len(args) == 0 {
					m.schemaMsg = ":theme needs a name (try :theme dark)"
					return nil
				}
				return m.exTheme(args[0])
			},
		},
	}
}

// exLookup resolves a parsed verb (already lower-cased by parseExLine) to its
// command spec, or nil if no command matches. Linear scan: the registry is
// small and lookup happens once per executed command, so a map cache isn't
// worth the global state. The returned pointer is only valid for the duration
// of the call (it points into a freshly built local slice).
func exLookup(verb string) *exCmdSpec {
	specs := exCommands()
	for i := range specs {
		for _, v := range specs[i].verbs {
			if v == verb {
				return &specs[i]
			}
		}
	}
	return nil
}
