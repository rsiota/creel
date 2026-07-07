# gsql — Project Memory

## Overview
A fast, memory-efficient SQL TUI inspired by [sqlit](https://github.com/Maxteabag/sqlit) (which is Python/Textual). Written in Go for speed. Currently supports **SQLite**, **MySQL**, and **PostgreSQL**.

## Tech Stack
- **Language**: Go 1.26+
- **TUI**: [Bubble Tea](https://github.com/charmbracelet/bubbletea) (Elm-style architecture) + [Lipgloss](https://github.com/charmbracelet/lipgloss) (styling) + [Bubbles](https://github.com/charmbracelet/bubbles) (components)
- **SQLite**: `modernc.org/sqlite` (pure Go, no CGO required)
- **MySQL**: `github.com/go-sql-driver/mysql`
- **PostgreSQL**: `github.com/jackc/pgx/v5` (via `pgx/v5/stdlib` for `database/sql` compatibility)
- **Config**: YAML (`gopkg.in/yaml.v3`), stored at `~/.config/gsql/config.yaml`

## Build & Run Commands
- **Build**: `go build -o gsql ./cmd/gsql/`
- **Build all packages**: `go build ./...`
- **Run TUI**: `./gsql`
- **CLI mode**: `./gsql -e "SELECT * FROM users" -database /tmp/test.db`
- **Vet**: `go vet ./...`
- **Tidy deps**: `go mod tidy`

## Architecture
```
cmd/gsql/main.go          — Entry point (TUI + CLI modes)
internal/db/              — Database abstraction layer
  db.go                   — DB interface, Connection wrapper
  sqlite.go               — SQLite implementation
  mysql.go                — MySQL implementation
  postgres.go              — PostgreSQL implementation (pgx/v5/stdlib)
  ssh_tunnel.go           — SSH tunnel (golang.org/x/crypto/ssh)
internal/config/          — Config loading/saving (YAML)
internal/history/         — Query history (per-connection JSON, searchable)
internal/ui/              — All Bubble Tea UI components
  app.go                  — Top-level Model (state machine)
  layout.go               — Workspace layout + panel sizing
  statusbar.go            — Bottom status bar (conn/db/table, counts, hints, messages)
  styles.go               — Shared color palette + lipgloss styles
  hints.go                — Context-sensitive keybinding hint line for the status bar
  connection_list.go      — Connection selection screen
  connection_form.go      — Add/edit connection form
  connection_ops.go       — Connection switch / open / clone actions
  database_picker.go      — Database switcher overlay (create/drop/browse, MySQL)
  query_editor.go         — SQL editor with vim mode (bubbles/textarea)
  editor_render.go        — Highlighted, soft-wrapped editor viewport rendering
  editor_wrap.go          — Word-wrap engine mirroring bubbles/textarea for cursor alignment
  sql_highlight.go        — SQL tokenizer + syntax coloring (keywords/strings/numbers/comments)
  sql_formatter.go        — SQL pretty-printer (keyword caps + clause line breaks, `==`)
  completion.go           — Fuzzy autocomplete popup (keywords/tables/columns, `ctrl+n`)
  results_table.go        — Query results table (custom renderer)
  stats.go                — Column stats (count/distinct/min/max/sum/avg) + async total row count
  filtering.go            — Filter/sort stack, column visibility, backend LIKE search
  filter_picker.go        — DISTINCT-value multi-select filter overlay (`g f`)
  inspector.go            — Record inspector (right-side vertical form editor)
  cell_edit_popup.go      — Modal multiline cell editor (JSON pretty-print + highlight)
  editing.go              — Edit staging, insert/clone/delete/inline-edit, FK nav, table-name rewrite
  insert.go               — Parameterized INSERT builder (`A`)
  delete_rows.go          — DELETE builder for marked/cursor rows (`dd`)
  truncate.go             — TRUNCATE / DELETE-FROM builder (`T`)
  fk_query.go             — Foreign-key follow query builder (`g d` / `g b`)
  cross_search_panel.go   — Cross-table search overlay (every table/column, `S`)
  cross_search_ops.go     — Async batched cross-table search execution
  schema_editor.go        — Inline column grid editor (rename/type/null/default, `d`)
  schema_ops.go           — Async DDL execution + post-edit state syncing
  schema_guard.go         — Pre-validates column drop/rename/modify (PK/auto-inc guards)
  table_designer.go       — Full-screen new-table grid editor (`N`)
  add_column_form.go      — Add-column modal form (`a`)
  table_rename_form.go    — Rename-table modal form (`r`)
  help.go                 — Help overlay panel (toggled with `?`); renders from `registry()`
  registry.go             — Single source of truth for keybinding help (Binding/Section types)
  palette.go              — Fuzzy command palette overlay (Ctrl+P); searches registry, replays keys
  keymsg.go               — synthesizeKeyMsg: maps dispatch token strings → tea.KeyMsg for replay
  fuzzy.go                — fuzzyMatch scorer + generic fuzzyRank[T] filter/sort helper
  history_panel.go        — Query history overlay panel
  bookmark_panel.go       — Saved-query overlay panel (per-connection JSON)
  explain_panel.go        — EXPLAIN query plan overlay (driver-aware: tree/table/text)
  export_import.go        — Export/import entry points + CSV export (`x`)
  export_picker.go        — Table-picker overlay for DB export (`X`)
  import_prompt.go        — File-path prompt overlay for SQL import (`I`)
  column_picker.go        — Column visibility overlay (`v`)
  mouse.go                — Mouse routing (clicks, scroll, click-outside-to-dismiss)
  json_format.go          — JSON pretty-print/compact/highlight for cell values
  *_test.go               — Tests (forms, editing, filtering, completion, schema, etc.)
internal/bookmarks/       — Persisted per-connection saved-query store (JSON)
internal/db/statements.go — Top-level statement splitter (run statement under cursor)
internal/db/schema.go     — DDL builders + validation (add column, create/rename table)
```

## Key Design Decisions
- **DB interface** in `internal/db/db.go` — all drivers implement this, making it trivial to add Postgres etc. later. Includes `Begin() (Tx, error)` for transactional batch writes (inline edits & row clones are atomic).
- **Elm-style state machine** in app.go: `stateConnections` → `stateWorkspace`, with `Focus` cycling between panels
- **Pure Go SQLite** (`modernc.org/sqlite`) — no CGO, simpler cross-compilation
- **lipgloss v1.1.0** — colors must use `lipgloss.Color()` not raw strings
- **Bubbles v1.0.0** — list delegate `Render` signature is `Render(w io.Writer, m Model, index int, item Item)`
- **Keybinding registry** (`internal/ui/registry.go`) is the single source of truth for the help overlay AND the command palette: each `Binding` carries a `Display` string + `Tokens` (the dispatch tokens) + `Desc`. `help.go` only renders it. The `TestKeybindingsMatchDispatch` test parses the dispatch (`case` literals + `key.WithKeys` args) via `go/parser` and asserts every documented token is implemented, preventing help/dispatch drift.
- **Command palette** (`internal/ui/palette.go`, Ctrl+P) fuzzy-searches the registry and replays single-key bindings via synthetic `tea.KeyMsg` (see `keymsg.go`), avoiding action closures or a dispatch refactor. Multi-action bundles and double-press chords are discoverable but not auto-executable.
- **Fuzzy ranking helper** (`internal/ui/fuzzy.go`) — `fuzzyRank[T]` is the generic filter+sort used by all fuzzy lists (sidebar, history, bookmarks, palette, pickers, completion). `fuzzyMatch` (the core subsequence scorer) also lives here. Callers handle the empty-query case themselves; the helper requires a non-empty query.

## Vertical Slices Progress
- [x] Slice 1: Project scaffold + DB abstraction + CLI mode
- [x] Slice 1b: TUI skeleton (connection list, editor, results, workspace layout)
- [x] Slice 2: Connection manager (add/edit/delete connections from TUI, saved to config)
- [x] Slice 3: Vim mode editing (normal/insert modes, h/j/k/l, i/a/o/A/O, dd/dw/x/D, y/p)
- [x] Slice 4: Query history (per-connection, persisted, searchable, overlay panel)
- [x] Slice 5: Full table browser (expand/collapse columns, schema view, quick actions)
- [x] Slice 6: Row pagination for large result sets (LIMIT/OFFSET wrapping, 200/page)
- [x] Fuzzy table search in sidebar (press `/`, type to filter, `enter` to select, `esc` to cancel)
- [x] SSH tunnel support (MySQL via bastion host; key-based auth with passphrase + password auth)
- [x] Record inspector (Ctrl+O toggles right-side panel; vertical form editor that tracks results cursor)
- [x] Help overlay (`?` toggles a full keybindings popup; status bar shows only context info — connection, table, dimensions, messages)
- [x] Column hide/visibility (`H` hides cursor column, `g H` shows all, `v` opens a column-visibility overlay; hidden cols are display-only and survive same-table re-queries)
- [x] Drop table (`D` in sidebar with typed-name confirmation)
- [x] DB export (pure-Go SQL dumper with table picker overlay `X`; mysqldump-compatible header/footer)
- [x] DB import (`I` in sidebar; streaming SQL parser in `internal/db/import.go`, collects failures without stopping)
- [x] PostgreSQL support (pgx/v5; `g e` EXPLAIN query plan view with driver-aware rendering)
- [x] Statement splitting — run only the statement under the cursor (`ctrl+e` / `\`); top-level semicolon splitter ignores quotes/comments
- [x] SQL syntax highlighting + formatter — tokenizer colors keywords/strings/numbers/comments; `==` pretty-prints with keyword caps + clause line breaks
- [x] SQL autocompletion — fuzzy popup of keywords/tables/columns while typing (`ctrl+n`)
- [x] Inline cell editing — `e` edits a cell, `E` opens a modal multiline editor (auto pretty-prints + highlights JSON), `ctrl+s` stages, `D` discards; edits batched in `dirtyCells`
- [x] Insert / clone / delete rows — `A` inserts a new row (parameterized, validates NOT NULL), `P` clones marked/cursor row, `dd` deletes marked or cursor row (composite-PK aware)
- [x] Truncate table — `T` builds `TRUNCATE TABLE` (MySQL) / `DELETE FROM` (SQLite)
- [x] Row marking + visual mode — `space` toggles a mark, `V` selects a range, `F` filters to marked rows, `C` clears marks
- [x] Row filtering — `g f` opens a DISTINCT-value multi-select picker; `*` keeps / `!` hides rows matching the cursor cell; `u` undoes last filter, `c` clears all; `/` searches all columns, `g /` regex search, `n`/`N` next/prev match
- [x] Foreign-key navigation — `g d` follows a FK to its referenced row, `g b` returns
- [x] Cross-table search — `S` async-searches a term across every table/column (batched, capped at 200 hits)
- [x] Schema editor — `d` opens an inline grid editor for column rename/type/null/default; each change commits its own DDL; `dd` drops a column (guarded against PK/auto-increment)
- [x] Table designer — `N` full-screen grid editor to define and create a new table
- [x] Add column / rename table — `a` modal add-column form, `r` rename-table form
- [x] Column statistics — `g s` computes count/distinct/min/max/sum/avg for the cursor column (server-side for `SELECT *`, client-side fallback)
- [x] Column sort + jump — `o` cycles sort on a column, `:` jumps to a column by name
- [x] Database picker — `ctrl+b` browse/switch/create/drop databases (MySQL) with fuzzy filter
- [x] Bookmarks — `ctrl+g` opens a per-connection saved-query panel, `B` bookmarks the current query; load/delete/clear inside the panel
- [x] Copy / export from results — `yy` copies a cell, `x` exports the result set to CSV, `Y` copies rows as INSERT statements
- [x] Mouse support — click tables/headers/cells, scroll lists and results, click-outside to dismiss overlays
- [x] Context-sensitive status-bar hints — compact keybinding hint line adapts to the active panel/modal

## Config Format (~/.config/gsql/config.yaml)
```yaml
connections:
  - name: local-dev
    driver: sqlite
    database: /path/to/db.sqlite
  - name: staging
    driver: mysql
    database: myapp
    host: 10.0.0.5
    port: 3306
    username: admin
    password: secret
```

## Conventions
- All colors/styles centralized in `internal/ui/styles.go`
- Tokyo Night color palette
- Value receivers in Bubble Tea Model methods (per Elm architecture — model is immutable)
- Database queries use `sql.NullString` for safe scanning
