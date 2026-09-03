package ui

import (
	"strings"

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
	// complete returns argument candidates for the token currently being
	// typed (partial may be ""); args holds the already-typed preceding
	// arguments. nil means no argument completion (verb-only). Used by the
	// ":" popup once the cursor moves past the verb.
	complete func(m *Model, args []string, partial string) []string
}

// exArgKind classifies a command's arguments, used by autocomplete hints and
// (later) by argument completion.
type exArgKind int

const (
	exArgNone     exArgKind = iota // no arguments
	exArgOptional                  // zero or one argument
	exArgRequired                  // exactly one argument required
	exArgTable                     // optional table name; defaults to current
	exArgText                      // free-form text: the whole rest of the line
)

// exCommands returns every ":" command, in display order. To add a command,
// append a spec here; runExCommand, autocomplete, and the help listing all
// pick it up automatically. The executors are thin wrappers over the ex*
// helper methods in excmd.go, so this table is pure dispatch — no logic lives
// here that isn't "which verb runs which method with which args".
func exCommands() []exCmdSpec {
	return []exCmdSpec{
		{
			verbs:    []string{"edit", "e"},
			desc:     "load a file into the editor",
			usage:    ":e <file>",
			argKind:  exArgRequired,
			complete: completePath,
			run: func(m *Model, args []string, _ bool) tea.Cmd {
				if len(args) == 0 {
					m.schemaMsg = ":e needs a file path"
					return nil
				}
				return m.exEditFile(args[0])
			},
		},
		{
			verbs:    []string{"write", "w"},
			desc:     "save edits, or write the editor buffer to a file",
			usage:    ":w [file]",
			argKind:  exArgOptional,
			complete: completePath,
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
			verbs:   []string{"qa"},
			desc:    "quit the app, closing all tabs",
			usage:   ":qa[!]",
			argKind: exArgNone,
			run:     func(m *Model, _ []string, force bool) tea.Cmd { return m.exQuitAll(force) },
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
				m.beginQuit()
				if save == nil {
					return tea.Quit
				}
				return tea.Sequence(save, tea.Quit)
			},
		},
		{
			verbs:   []string{"tabnew"},
			desc:    "open a new results tab",
			usage:   ":tabnew",
			argKind: exArgNone,
			run:     func(m *Model, _ []string, _ bool) tea.Cmd { return m.exTabNew() },
		},
		{
			verbs:   []string{"tabclose"},
			desc:    "close the active tab (not the last)",
			usage:   ":tabclose[!]",
			argKind: exArgNone,
			run:     func(m *Model, _ []string, force bool) tea.Cmd { return m.exTabClose(force) },
		},
		{
			verbs:   []string{"tabnext", "tabn"},
			desc:    "go to the next tab",
			usage:   ":tabnext",
			argKind: exArgNone,
			run:     func(m *Model, _ []string, _ bool) tea.Cmd { return m.exTabNext() },
		},
		{
			verbs:   []string{"tabprev", "tabp"},
			desc:    "go to the previous tab",
			usage:   ":tabprev",
			argKind: exArgNone,
			run:     func(m *Model, _ []string, _ bool) tea.Cmd { return m.exTabPrev() },
		},
		{
			verbs:   []string{"tabs"},
			desc:    "list open tabs",
			usage:   ":tabs",
			argKind: exArgNone,
			run:     func(m *Model, _ []string, _ bool) tea.Cmd { return m.exTabs() },
		},
		{
			verbs:   []string{"diff"},
			desc:    "diff result pages of two tabs (current pages, not schema)",
			usage:   ":diff [a] [b]",
			argKind: exArgOptional,
			run: func(m *Model, args []string, _ bool) tea.Cmd {
				return m.exDiff(args)
			},
		},
		{
			verbs:    []string{"sort"},
			desc:     "sort results by a column",
			usage:    ":sort <column>",
			argKind:  exArgRequired,
			complete: completeColumn,
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
			complete: completeTable,
		},
		{
			verbs:   []string{"export"},
			desc:    "export result rows to ~/Downloads (optional column list)",
			usage:   ":export <csv|json|jsonl|md|tsv> [col,...]",
			argKind: exArgOptional,
			run: func(m *Model, args []string, _ bool) tea.Cmd {
				return m.exExport(args)
			},
			complete: completeExport,
		},
		{
			verbs:   []string{"copy"},
			desc:    "copy the cell under the cursor to the clipboard",
			usage:   ":copy",
			argKind: exArgNone,
			run:     func(m *Model, _ []string, _ bool) tea.Cmd { return m.exCopy() },
		},
		{
			verbs:    []string{"setnull"},
			desc:     "stage SQL NULL on the cursor cell (or named column on the row)",
			usage:    ":setnull [column]",
			argKind:  exArgOptional,
			complete: completeColumn,
			run: func(m *Model, args []string, _ bool) tea.Cmd {
				return m.exSetNull(args)
			},
		},
		{
			verbs:   []string{"discard"},
			desc:    "discard staged cell edits",
			usage:   ":discard[!]",
			argKind: exArgNone,
			run:     func(m *Model, _ []string, force bool) tea.Cmd { return m.exDiscard(force) },
		},
		{
			verbs:   []string{"clone"},
			desc:    "duplicate the marked or cursor row",
			usage:   ":clone",
			argKind: exArgNone,
			run:     func(m *Model, _ []string, _ bool) tea.Cmd { return m.exClone() },
		},
		{
			verbs:   []string{"follow"},
			desc:    "follow the foreign key under the cursor",
			usage:   ":follow",
			argKind: exArgNone,
			run:     func(m *Model, _ []string, _ bool) tea.Cmd { return m.exFollow() },
		},
		{
			verbs:   []string{"back"},
			desc:    "return to the previous query",
			usage:   ":back",
			argKind: exArgNone,
			run:     func(m *Model, _ []string, _ bool) tea.Cmd { return m.exBack() },
		},
		{
			verbs:   []string{"keep"},
			desc:    "keep rows equal to the cursor cell",
			usage:   ":keep",
			argKind: exArgNone,
			run:     func(m *Model, _ []string, _ bool) tea.Cmd { return m.exKeep() },
		},
		{
			verbs:   []string{"hide"},
			desc:    "hide rows equal to the cursor cell",
			usage:   ":hide",
			argKind: exArgNone,
			run:     func(m *Model, _ []string, _ bool) tea.Cmd { return m.exHide() },
		},
		{
			verbs:   []string{"undo"},
			desc:    "remove the last filter",
			usage:   ":undo",
			argKind: exArgNone,
			run:     func(m *Model, _ []string, _ bool) tea.Cmd { return m.exUndo() },
		},
		{
			verbs:   []string{"unfilter"},
			desc:    "clear all filters",
			usage:   ":unfilter",
			argKind: exArgNone,
			run:     func(m *Model, _ []string, _ bool) tea.Cmd { return m.exUnfilter() },
		},
		{
			verbs:   []string{"copyinsert"},
			desc:    "copy result rows as INSERT to clipboard",
			usage:   ":copyinsert",
			argKind: exArgNone,
			run:     func(m *Model, _ []string, _ bool) tea.Cmd { return m.exCopyInsert() },
		},
		{
			verbs:   []string{"copyrow"},
			desc:    "copy marked/cursor row(s) to clipboard (tsv default)",
			usage:   ":copyrow [csv|json|jsonl|md|tsv]",
			argKind: exArgOptional,
			run: func(m *Model, args []string, _ bool) tea.Cmd {
				return m.exCopyRow(args)
			},
			complete: completeCopyRow,
		},
		{
			verbs:   []string{"regex"},
			desc:    "regex search the current page",
			usage:   ":regex <pattern>",
			argKind: exArgRequired,
			run:     func(m *Model, args []string, _ bool) tea.Cmd { return m.exRegex(args) },
		},
		{
			verbs:    []string{"hidecolumn"},
			desc:     "hide a column (cursor or named)",
			usage:    ":hidecolumn [col]",
			argKind:  exArgOptional,
			complete: completeColumn,
			run:      func(m *Model, args []string, _ bool) tea.Cmd { return m.exHideColumn(args) },
		},
		{
			verbs:   []string{"showcolumns"},
			desc:    "reveal all hidden columns",
			usage:   ":showcolumns",
			argKind: exArgNone,
			run:     func(m *Model, _ []string, _ bool) tea.Cmd { return m.exShowColumns() },
		},
		{
			verbs:    []string{"connect", "c"},
			desc:     "switch connection by name, or open the connection list",
			usage:    ":connect [name]",
			argKind:  exArgOptional,
			run:      func(m *Model, args []string, _ bool) tea.Cmd { return m.exConnect(args) },
			complete: completeConnection,
		},
		{
			verbs:   []string{"reconnect"},
			desc:    "rebuild the active connection without leaving the workspace",
			usage:   ":reconnect",
			argKind: exArgNone,
			run:     func(m *Model, _ []string, _ bool) tea.Cmd { return m.exReconnect() },
		},
		{
			verbs:   []string{"connections"},
			desc:    "open the connection list",
			usage:   ":connections",
			argKind: exArgNone,
			run:     func(m *Model, _ []string, _ bool) tea.Cmd { return m.exConnections() },
		},
		{
			verbs:   []string{"db", "use"},
			desc:    "list or switch databases",
			usage:   ":db [database]",
			argKind: exArgOptional,
			run:     func(m *Model, args []string, _ bool) tea.Cmd { return m.exDB(args) },
		},
		{
			verbs:   []string{"schema"},
			desc:    "list or switch schemas (Postgres; MySQL = :db)",
			usage:   ":schema [name]",
			argKind: exArgOptional,
			run:     func(m *Model, args []string, _ bool) tea.Cmd { return m.exSchema(args) },
		},
		{
			verbs:   []string{"backup", "mysqldump"},
			desc:    "backup the current MySQL database with mysqldump to ~/Downloads",
			usage:   ":backup",
			argKind: exArgNone,
			run:     func(m *Model, _ []string, _ bool) tea.Cmd { return m.exBackup() },
		},
		{
			verbs:    []string{"import"},
			desc:     "import a SQL dump file into the database",
			usage:    ":import <file>",
			argKind:  exArgRequired,
			complete: completePath,
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
			desc:    "inbound FKs; per-row counts when focused",
			usage:   ":refs [table]",
			argKind: exArgTable,
			run: func(m *Model, args []string, _ bool) tea.Cmd {
				arg := ""
				if len(args) > 0 {
					arg = args[0]
				}
				return m.exRefs(arg)
			},
			complete: completeTable,
		},
		{
			verbs:   []string{"explore", "explorer", "er"},
			desc:    "toggle the relationship explorer for the focused row (g r)",
			usage:   ":explore [panel]",
			argKind: exArgOptional,
			run:     func(m *Model, _ []string, _ bool) tea.Cmd { return m.openDockedExplorer() },
		},
		{
			verbs:   []string{"erd"},
			desc:    "static ERD (Mermaid erDiagram) of the schema or a table's neighbourhood",
			usage:   ":erd [table|save [file]]",
			argKind: exArgText,
			run:     func(m *Model, args []string, _ bool) tea.Cmd { return m.exERD(args) },
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
			complete: completeTable,
		},
		{
			verbs:    []string{"begin", "transaction"},
			desc:     "start a manual transaction (optional isolation)",
			usage:    ":begin [serializable|repeatable read|read committed|read uncommitted]",
			argKind:  exArgText,
			complete: completeIsolation,
			run:      func(m *Model, args []string, _ bool) tea.Cmd { return m.exBegin(args) },
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
			verbs:   []string{"run", "r"},
			desc:    "run the statement under the cursor",
			usage:   ":run",
			argKind: exArgNone,
			run:     func(m *Model, _ []string, _ bool) tea.Cmd { return m.exRun() },
		},
		{
			verbs:   []string{"explain", "plan"},
			desc:    "show the query plan for the editor's statement",
			usage:   ":explain",
			argKind: exArgNone,
			run:     func(m *Model, _ []string, _ bool) tea.Cmd { return m.explainQuery() },
		},
		{
			verbs:   []string{"new"},
			desc:    "clear the editor to an empty scratch buffer",
			usage:   ":new",
			argKind: exArgNone,
			run:     func(m *Model, _ []string, _ bool) tea.Cmd { return m.exNew() },
		},
		{
			verbs:   []string{"version"},
			desc:    "show the creel build version",
			usage:   ":version",
			argKind: exArgNone,
			run:     func(m *Model, _ []string, _ bool) tea.Cmd { return m.exVersion() },
		},
		{
			verbs:   []string{"recent"},
			desc:    "list or re-open recently touched tables",
			usage:   ":recent [n|name]",
			argKind: exArgOptional,
			run:     func(m *Model, args []string, _ bool) tea.Cmd { return m.exRecent(args) },
		},
		{
			verbs:   []string{"truncate"},
			desc:    "delete all rows from a table",
			usage:   ":truncate[!] [table]",
			argKind: exArgTable,
			run: func(m *Model, args []string, force bool) tea.Cmd {
				name := ""
				if len(args) > 0 {
					name = args[0]
				}
				return m.exTruncate(name, force)
			},
			complete: completeTable,
		},
		{
			verbs:   []string{"drop"},
			desc:    "drop a table",
			usage:   ":drop[!] [table]",
			argKind: exArgTable,
			run: func(m *Model, args []string, force bool) tea.Cmd {
				name := ""
				if len(args) > 0 {
					name = args[0]
				}
				return m.exDrop(name, force)
			},
			complete: completeTable,
		},
		{
			verbs:   []string{"rename"},
			desc:    "rename a table (form, or old→new)",
			usage:   ":rename [old] [new]",
			argKind: exArgOptional,
			run:     func(m *Model, args []string, _ bool) tea.Cmd { return m.exRename(args) },
		},
		{
			verbs:   []string{"create"},
			desc:    "open the create-table designer",
			usage:   ":create",
			argKind: exArgNone,
			run:     func(m *Model, _ []string, _ bool) tea.Cmd { return m.exCreate() },
		},
		{
			verbs:    []string{"addcolumn"},
			desc:     "add a column to a table (form, or direct)",
			usage:    ":addcolumn [table] <name> <type> [\u2026]",
			argKind:  exArgOptional,
			run:      func(m *Model, args []string, _ bool) tea.Cmd { return m.exAddColumn(args) },
			complete: completeTable,
		},
		{
			verbs:   []string{"createdb"},
			desc:    "create a database (MySQL/Postgres)",
			usage:   ":createdb <name>",
			argKind: exArgRequired,
			run: func(m *Model, args []string, _ bool) tea.Cmd {
				name := ""
				if len(args) > 0 {
					name = args[0]
				}
				return m.exCreateDatabase(name)
			},
		},
		{
			verbs:   []string{"dropdb"},
			desc:    "drop a database (defaults to current)",
			usage:   ":dropdb[!] [name]",
			argKind: exArgOptional,
			run: func(m *Model, args []string, force bool) tea.Cmd {
				name := ""
				if len(args) > 0 {
					name = args[0]
				}
				return m.exDropDatabase(name, force)
			},
		},
		{
			verbs:   []string{"refresh", "reload"},
			desc:    "refresh schema and re-run the last query",
			usage:   ":refresh",
			argKind: exArgNone,
			run:     func(m *Model, _ []string, _ bool) tea.Cmd { return m.refreshSchema() },
		},
		{
			verbs:   []string{"sidebar"},
			desc:    "toggle the table sidebar",
			usage:   ":sidebar",
			argKind: exArgNone,
			run:     func(m *Model, _ []string, _ bool) tea.Cmd { m.toggleSidebar(); return nil },
		},
		{
			verbs:   []string{"editor"},
			desc:    "toggle the query editor and tab bar",
			usage:   ":editor",
			argKind: exArgNone,
			run:     func(m *Model, _ []string, _ bool) tea.Cmd { m.toggleEditor(); return nil },
		},
		{
			verbs:   []string{"inspector"},
			desc:    "toggle the row inspector panel",
			usage:   ":inspector",
			argKind: exArgNone,
			run:     func(m *Model, _ []string, _ bool) tea.Cmd { m.toggleInspector(); return nil },
		},
		{
			verbs:   []string{"assistant"},
			desc:    "toggle the AI assistant panel",
			usage:   ":assistant",
			argKind: exArgNone,
			run:     func(m *Model, _ []string, _ bool) tea.Cmd { m.toggleAssistant(); return nil },
		},
		{
			verbs:   []string{"zen"},
			desc:    "toggle results-only layout (sidebar, editor, and side panels hidden)",
			usage:   ":zen [off]",
			argKind: exArgOptional,
			run:     func(m *Model, args []string, _ bool) tea.Cmd { return m.exZen(args) },
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
			verbs:    []string{"tail"},
			desc:     "stream the newest rows of a table on a timer",
			usage:    ":tail [table] [n]",
			argKind:  exArgTable,
			run:      func(m *Model, args []string, _ bool) tea.Cmd { return m.exTail(args) },
			complete: completeTable,
		},
		{
			verbs:    []string{"param", "params"},
			desc:     "set or list named query parameters (:name in SQL)",
			usage:    ":param[!] [name] [value…]",
			argKind:  exArgOptional,
			complete: completeParam,
			run:      func(m *Model, args []string, force bool) tea.Cmd { return m.exParam(args, force) },
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
			verbs:    []string{"set"},
			desc:     "view or change a config setting",
			usage:    ":set [option] [value]",
			argKind:  exArgOptional,
			complete: completeSet,
			run: func(m *Model, args []string, _ bool) tea.Cmd {
				return m.exSet(args)
			},
		},
		{
			verbs:    []string{"peek"},
			desc:     "one-glance summary of a table",
			usage:    ":peek [table]",
			argKind:  exArgTable,
			run:      func(m *Model, args []string, _ bool) tea.Cmd { return m.exPeek(args) },
			complete: completeTable,
		},
		{
			verbs:    []string{"filter"},
			desc:     "add a WHERE filter (col op value)",
			usage:    ":filter <col><op><value>|off",
			argKind:  exArgRequired,
			complete: completeColumn,
			run:      func(m *Model, args []string, _ bool) tea.Cmd { return m.exFilter(args) },
		},
		{
			verbs:    []string{"open", "o"},
			desc:     "load a file into the editor (alias of :e)",
			usage:    ":open <file>",
			argKind:  exArgRequired,
			complete: completePath,
			run: func(m *Model, args []string, _ bool) tea.Cmd {
				if len(args) == 0 {
					m.schemaMsg = ":open needs a file path"
					return nil
				}
				return m.exEditFile(args[0])
			},
		},
		{
			verbs:    []string{"save"},
			desc:     "write the editor buffer to a file (alias of :w)",
			usage:    ":save <file>",
			argKind:  exArgRequired,
			complete: completePath,
			run: func(m *Model, args []string, _ bool) tea.Cmd {
				if len(args) == 0 {
					m.schemaMsg = ":save needs a file path"
					return nil
				}
				return m.exWriteFile(args[0])
			},
		},
		{
			verbs:    []string{"saveblob"},
			desc:     "save the cursor cell's binary value to a file",
			usage:    ":saveblob <file>",
			argKind:  exArgRequired,
			complete: completePath,
			run: func(m *Model, args []string, _ bool) tea.Cmd {
				if len(args) == 0 {
					m.schemaMsg = ":saveblob needs a file path"
					return nil
				}
				return m.exSaveBlob(args[0])
			},
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
			verbs:   []string{"session"},
			desc:    "manage the saved workspace session",
			usage:   ":session [clear|save]",
			argKind: exArgOptional,
			run:     func(m *Model, args []string, _ bool) tea.Cmd { return m.exSession(args) },
		},
		{
			verbs:   []string{"describe", "desc", "d"},
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
			complete: completeTable,
		},
		{
			verbs:   []string{"columns"},
			desc:    "open the Columns structure tab",
			usage:   ":columns [table]",
			argKind: exArgTable,
			run: func(m *Model, args []string, _ bool) tea.Cmd {
				name := ""
				if len(args) > 0 {
					name = args[0]
				}
				return m.exOpenStructureTab(name, seTabColumns)
			},
			complete: completeTable,
		},
		{
			verbs:   []string{"indexes"},
			desc:    "open the Indexes structure tab",
			usage:   ":indexes [table]",
			argKind: exArgTable,
			run: func(m *Model, args []string, _ bool) tea.Cmd {
				name := ""
				if len(args) > 0 {
					name = args[0]
				}
				return m.exOpenStructureTab(name, seTabIndexes)
			},
			complete: completeTable,
		},
		{
			verbs:   []string{"constraints"},
			desc:    "open the Checks structure tab",
			usage:   ":constraints [table]",
			argKind: exArgTable,
			run: func(m *Model, args []string, _ bool) tea.Cmd {
				name := ""
				if len(args) > 0 {
					name = args[0]
				}
				return m.exOpenStructureTab(name, seTabChecks)
			},
			complete: completeTable,
		},
		{
			verbs:   []string{"fk"},
			desc:    "open the Foreign Keys structure tab",
			usage:   ":fk [table]",
			argKind: exArgTable,
			run: func(m *Model, args []string, _ bool) tea.Cmd {
				name := ""
				if len(args) > 0 {
					name = args[0]
				}
				return m.exOpenStructureTab(name, seTabFK)
			},
			complete: completeTable,
		},
		{
			verbs:   []string{"tables", "dt"},
			desc:    "list tables in the lookup overlay",
			usage:   ":tables",
			argKind: exArgNone,
			run:     func(m *Model, _ []string, _ bool) tea.Cmd { return m.exTables() },
		},
		{
			verbs:   []string{"sizes"},
			desc:    "list tables with row and on-disk size estimates",
			usage:   ":sizes",
			argKind: exArgNone,
			run:     func(m *Model, _ []string, _ bool) tea.Cmd { return m.exSizes() },
		},
		{
			verbs:   []string{"views", "dv"},
			desc:    "list views in the lookup overlay",
			usage:   ":views",
			argKind: exArgNone,
			run:     func(m *Model, _ []string, _ bool) tea.Cmd { return m.exViews() },
		},
		{
			verbs:   []string{"schemas"},
			desc:    "list schemas in the lookup overlay",
			usage:   ":schemas",
			argKind: exArgNone,
			run:     func(m *Model, _ []string, _ bool) tea.Cmd { return m.exSchemasList() },
		},
		{
			verbs:   []string{"search", "find"},
			desc:    "fuzzy-find tables, views, and columns by name",
			usage:   ":search <name>",
			argKind: exArgRequired,
			run: func(m *Model, args []string, _ bool) tea.Cmd {
				if len(args) == 0 {
					m.schemaMsg = ":search needs a name"
					return nil
				}
				return m.exSearch(args[0])
			},
		},
		{
			verbs:    []string{"stats"},
			desc:     "summary stats for a column (min/max/avg/…)",
			usage:    ":stats [column]",
			argKind:  exArgOptional,
			complete: completeColumn,
			run: func(m *Model, args []string, _ bool) tea.Cmd {
				arg := ""
				if len(args) > 0 {
					arg = args[0]
				}
				return m.exStats(arg)
			},
		},
		{
			verbs:    []string{"bar"},
			desc:     "bar chart from result columns; one column counts (bang = all rows)",
			usage:    ":bar[!] [label] [value] [sum|count|avg]",
			argKind:  exArgOptional,
			complete: completeBarColumns,
			run:      func(m *Model, args []string, force bool) tea.Cmd { return m.exBar(args, force) },
		},
		{
			verbs:    []string{"freq"},
			desc:     "frequency bar chart of a column (bang = all rows)",
			usage:    ":freq[!] [column]",
			argKind:  exArgOptional,
			complete: completeColumn,
			run:      func(m *Model, args []string, force bool) tea.Cmd { return m.exFreq(args, force) },
		},
		{
			verbs:    []string{"pie"},
			desc:     "pie chart of a column (same data as :freq; bang = all rows)",
			usage:    ":pie[!] [column]",
			argKind:  exArgOptional,
			complete: completeColumn,
			run:      func(m *Model, args []string, force bool) tea.Cmd { return m.exPie(args, force) },
		},
		{
			verbs:    []string{"line"},
			desc:     "line chart from two numeric or datetime columns (bang = all rows)",
			usage:    ":line[!] [x] [y]",
			argKind:  exArgOptional,
			complete: completeLineColumns,
			run:      func(m *Model, args []string, force bool) tea.Cmd { return m.exLine(args, force) },
		},
		{
			verbs:    []string{"scatter"},
			desc:     "scatter chart from two numeric or datetime columns (bang = all rows)",
			usage:    ":scatter[!] [x] [y]",
			argKind:  exArgOptional,
			complete: completeLineColumns,
			run:      func(m *Model, args []string, force bool) tea.Cmd { return m.exScatter(args, force) },
		},
		{
			verbs:    []string{"hist"},
			desc:     "histogram of a numeric column (bang = all rows)",
			usage:    ":hist[!] [column] [bins]",
			argKind:  exArgOptional,
			complete: completeHistColumns,
			run:      func(m *Model, args []string, force bool) tea.Cmd { return m.exHist(args, force) },
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
			complete: completeTable,
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
			complete: completeTable,
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
			complete: completeTheme,
		},
		{
			verbs:   []string{"icons"},
			desc:    "switch the tree expand/collapse glyph set",
			usage:   ":icons <unicode|nerdfont>",
			argKind: exArgRequired,
			run: func(m *Model, args []string, _ bool) tea.Cmd {
				if len(args) == 0 {
					m.schemaMsg = ":icons needs a name (unicode or nerdfont)"
					return nil
				}
				return m.exIcons(args[0])
			},
			complete: completeEnum("unicode", "nerdfont"),
		},
		{
			verbs:   []string{"ai"},
			desc:    "ask the AI to write SQL from a natural-language request",
			usage:   ":ai <request>",
			argKind: exArgText,
			run: func(m *Model, args []string, _ bool) tea.Cmd {
				return m.exAI(strings.Join(args, " "))
			},
		},
		{
			verbs: []string{"aifix", "fixsql"},
			desc:  "ask the AI to fix the last failed query",
			usage: ":aifix",
			run: func(m *Model, _ []string, _ bool) tea.Cmd {
				return m.exAIFix()
			},
		},
		{
			verbs:   []string{"aiexplain", "why"},
			desc:    "ask the AI to explain the query (attaches EXPLAIN plan)",
			usage:   ":aiexplain [focus]",
			argKind: exArgText,
			run: func(m *Model, args []string, _ bool) tea.Cmd {
				return m.exAIExplain(strings.Join(args, " "))
			},
		},
	}
}

// exLookup resolves a parsed verb (already lower-cased by parseExLine) to its
// command spec, or nil if no command matches. Linear scan: the registry is
// small and lookup happens once per executed command, so a map cache isn't
// worth the global state. The returned pointer is only valid for the duration
// of the call (it points into a freshly built local slice).
// --- argument completers --------------------------------------------------
//
// Each returns candidate strings for one command's first argument; ranking by
// the partial token and popup rendering happen in recomputeExCompletion. They
// return nil past the argument they own, so a popup simply hides there.

// completeTable offers the current connection's table/view names as the first
// argument (the table most ":" commands operate on). Past the first argument
// there is nothing useful to suggest (counts, names, types are free-form).
func completeTable(m *Model, args []string, _ string) []string {
	if len(args) > 0 {
		return nil
	}
	var names []string
	for _, it := range m.sidebarItems() {
		if !it.isColumn {
			names = append(names, it.text)
		}
	}
	return names
}

// completeColumn offers the current result set's column names as the first
// argument — the columns :sort/:hidecolumn/:stats/:filter/:freq operate on (each
// resolves the name case-insensitively against the results grid). It reads the
// results grid rather than the schema cache so it stays correct for a custom
// query whose columns differ from the focused table's schema. Past the first
// argument there is nothing useful to suggest; with no results there are none.
func completeColumn(m *Model, args []string, _ string) []string {
	if len(args) > 0 || m.results.NumCols() == 0 {
		return nil
	}
	names := make([]string, 0, m.results.NumCols())
	for i := 0; i < m.results.NumCols(); i++ {
		names = append(names, m.results.ColumnName(i))
	}
	return names
}

// completeBarColumns offers result columns for :bar's label and value args,
// then sum/count/avg for the optional aggregate. With no args yet, both
// columns and aggregates are offered so `:bar count` works on marked columns
// and `:bar <label>` (frequency) is completable.
func completeBarColumns(m *Model, args []string, _ string) []string {
	aggs := []string{"sum", "count", "avg"}
	if len(args) >= 3 {
		return nil
	}
	if len(args) == 2 {
		return aggs
	}
	if len(args) == 1 {
		if _, ok := parseBarAgg(args[0]); ok {
			return nil
		}
	}
	if m.results.NumCols() == 0 {
		if len(args) == 0 {
			return aggs
		}
		return nil
	}
	names := make([]string, 0, m.results.NumCols())
	for i := 0; i < m.results.NumCols(); i++ {
		names = append(names, m.results.ColumnName(i))
	}
	if len(args) == 0 {
		return append(names, aggs...)
	}
	return names
}

// completeLineColumns offers result columns for :line / :scatter x and y args.
func completeLineColumns(m *Model, args []string, _ string) []string {
	if len(args) >= 2 || m.results.NumCols() == 0 {
		return nil
	}
	names := make([]string, 0, m.results.NumCols())
	for i := 0; i < m.results.NumCols(); i++ {
		names = append(names, m.results.ColumnName(i))
	}
	return names
}

// completeHistColumns offers result columns for :hist's column arg.
func completeHistColumns(m *Model, args []string, _ string) []string {
	if len(args) >= 1 || m.results.NumCols() == 0 {
		return nil
	}
	names := make([]string, 0, m.results.NumCols())
	for i := 0; i < m.results.NumCols(); i++ {
		names = append(names, m.results.ColumnName(i))
	}
	return names
}

// completeConnection offers configured connection names as the sole argument.
func completeConnection(m *Model, args []string, _ string) []string {
	if len(args) > 0 || m.config == nil {
		return nil
	}
	names := make([]string, 0, len(m.config.Connections))
	for _, c := range m.config.Connections {
		names = append(names, c.Name)
	}
	return names
}

// completeTheme offers every available theme name as the sole argument.
func completeTheme(_ *Model, args []string, _ string) []string {
	if len(args) > 0 {
		return nil
	}
	return themeNames()
}

// completeIsolation offers isolation-level tokens for :begin. Hyphenated
// single tokens complete first; after "read" / "repeatable" the second word
// is offered so spaced forms work too.
func completeIsolation(_ *Model, args []string, partial string) []string {
	switch len(args) {
	case 0:
		return rankStrings(partial, []string{
			"serializable",
			"repeatable-read",
			"read-committed",
			"read-uncommitted",
			"rr", "rc", "s", "ru",
		})
	case 1:
		switch strings.ToLower(args[0]) {
		case "read":
			return rankStrings(partial, []string{"committed", "uncommitted"})
		case "repeatable":
			return rankStrings(partial, []string{"read"})
		}
	}
	return nil
}

// completeEnum builds a completer for a fixed set of literal values (e.g.
// ":export" formats, ":icons" glyph sets).
func completeEnum(options ...string) func(*Model, []string, string) []string {
	return func(_ *Model, args []string, _ string) []string {
		if len(args) > 0 {
			return nil
		}
		return options
	}
}

// completeExport completes :export arguments: the format for the first arg,
// then result-set column names for subsequent args (so `:export csv na<Tab>`
// completes to a matching column). Columns come from the live results grid.
func completeExport(m *Model, args []string, partial string) []string {
	if len(args) == 0 {
		return rankStrings(partial, []string{"csv", "json", "jsonl", "md", "tsv"})
	}
	cols := make([]string, m.results.NumCols())
	for i := 0; i < m.results.NumCols(); i++ {
		cols[i] = m.results.ColumnName(i)
	}
	return rankStrings(partial, cols)
}

// completeCopyRow completes :copyrow's single optional format argument. TSV
// is listed first since it's the default.
func completeCopyRow(_ *Model, args []string, partial string) []string {
	if len(args) == 0 {
		return rankStrings(partial, []string{"tsv", "csv", "md", "json", "jsonl"})
	}
	return nil
}

// completePath offers filesystem entries matching the typed path prefix as the
// first argument of the file commands (:e/:w/:import/:open/:save/:saveblob). It reuses
// the import prompt's path engine (completeFilePath) but returns full-path
// candidates (dir + name) rather than bare entry names, so the ":" popup's
// fuzzy ranker — which matches the typed prefix against each candidate —
// keeps them (the prefix is a subsequence of the full path, not of the bare
// name), and Tab fills the whole path. Past the first argument there is
// nothing useful to suggest; with no directory prefix typed there is nothing
// (start with ~/, ./, or /).
func completePath(_ *Model, args []string, partial string) []string {
	if len(args) > 0 {
		return nil
	}
	dir, _ := splitPathVal(partial)
	base := completeFilePath(partial)
	if len(base) == 0 {
		return nil
	}
	out := make([]string, len(base))
	for i, name := range base {
		out[i] = dir + name
	}
	return out
}

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
