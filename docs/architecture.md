# Architecture

How creel is put together. For build/test instructions see
[CONTRIBUTING](../CONTRIBUTING.md); for the quick orientation see the README's
[Architecture](../README.md#architecture) section. This page goes deeper: the
subsystem map and the design decisions behind them.

## Tech stack

- **Go 1.26+**
- **TUI**: [Bubble Tea](https://github.com/charmbracelet/bubbletea) (Elm-style architecture) + [Lipgloss](https://github.com/charmbracelet/lipgloss) (styling) + [Bubbles](https://github.com/charmbracelet/bubbles) (components)
- **SQLite**: `modernc.org/sqlite` — pure Go, no CGO
- **MySQL**: `github.com/go-sql-driver/mysql`
- **PostgreSQL**: `github.com/jackc/pgx/v5` (via `pgx/v5/stdlib` for `database/sql` compatibility)
- **Config**: `gopkg.in/yaml.v3`, stored at `~/.config/creel/config.yaml`
- **Secrets**: `github.com/zalando/go-keyring` (macOS Keychain / Windows Credential Manager / Linux Secret Service); the YAML holds `secret://<conn>/<field>` refs resolved at connect time, with plaintext fallback when no keychain is available

## Top-level layout

```
cmd/creel/        entry point (TUI + CLI modes; `-e -` / `-cli` read SQL from stdin)
cmd/genthemes/    codegen for the bundled theme palettes
internal/db/      database abstraction + drivers
internal/config/  YAML config load/save
internal/secrets/ OS keychain secret store
internal/history/ per-connection query history (JSON)
internal/bookmarks/ saved queries
internal/session/ per-connection workspace state (open tabs, restored on reconnect)
internal/recent/  connection-picker MRU list
internal/ai/      OpenAI-compatible client for the assistant
internal/version/ version reporting (ldflags-injected)
internal/ui/      all Bubble Tea components
```

## internal/db — database layer

The `DB` interface (`db.go`) is the contract every driver implements; a
`Connection` wrapper sits above it. Adding a database is self-contained —
implement the interface in a new file.

- `db.go` — `DB` interface, `Connection` wrapper, the shared read-only write guard (`rejectWriteIfReadOnly` / `isWriteQuery`)
- `sqlite.go` / `mysql.go` / `postgres.go` — driver implementations
- `schema.go` — DDL builders + validation (add column, create/rename table)
- `statements.go` — top-level statement splitter (powers "run statement under cursor")
- `dump.go` / `import.go` — pure-Go export and streaming SQL import (MySQL dumps
  use backslash string escapes, backticks, and `#` comments)
- `ssh_tunnel.go` — SSH tunnel for remote MySQL and PostgreSQL

The interface also exposes catalog metadata used by the structure panel:
`Indexes`, `Triggers`, `ViewDefinition`, `CheckConstraints`, `TableDefinition`
— each implemented per driver against `sqlite_master` / `information_schema` /
`pg_catalog` (MySQL `SHOW CREATE TABLE`). SQL dumps prefer `TableDefinition`
so indexes, named FKs, and table options survive a round-trip.

## Supporting packages

- `internal/config` — YAML load/save
- `internal/secrets` — OS keychain store: `Store` writes a value and returns a `secret://` ref, `Resolve` turns a ref (or plaintext) back into the value
- `internal/history` — per-connection query history (JSON, searchable)
- `internal/bookmarks` — persisted per-connection saved-query store
- `internal/session` — per-connection workspace snapshot (open tabs + editor buffers), restored on reconnect
- `internal/ai` — OpenAI-compatible HTTP client for the assistant
- `internal/version` — version string, injected via ldflags at release

## internal/ui — Bubble Tea components

The UI is one Elm-style state machine in `app.go`. Components are grouped by
subsystem:

**Core**
- `app.go` — top-level `Model` (the state machine: `stateConnections` → `stateWorkspace`, focus cycling)
- `layout.go` — workspace layout + panel sizing
- `statusbar.go` — bottom status bar (conn/db/table, counts, hints, messages)
- `hints.go` — context-sensitive keybinding hint line
- `styles.go` — shared color palette + lipgloss styles
- `tabs.go` / `tab_bar.go` — per-tab state (`ResultsTab`) and the tab bar
- `background.go` — async background-work plumbing

**Connections**
- `connection_list.go`, `connection_form.go`, `connection_form_status.go`, `connection_ops.go`
- `database_picker.go` — browse/switch/create/drop databases (MySQL)

**Query editor**
- `query_editor.go` — SQL editor with vim mode
- `query.go`, `editor_render.go` (highlighted, soft-wrapped viewport), `editor_wrap.go`
- `sql_highlight.go` — SQL tokenizer + syntax coloring
- `sql_formatter.go` — SQL pretty-printer (`==`)
- `completion.go` — fuzzy autocomplete popup (`ctrl+n`)

**Results grid**
- `results_table.go` — custom results renderer
- `filtering.go` — filter/sort stack, column visibility, backend LIKE search
- `filter_picker.go` — DISTINCT-value multi-select filter (`g f`)
- `stats.go` — column statistics + async total row count
- `chart_panel.go` / `chart_line.go` / `chart_hist.go` / `chart_query.go` — bar, line, scatter, and histogram charts in the results slot (`:bar` / `:line` / `:scatter` / `:hist` / `:freq` / column marks; bang re-queries the full result)
- `inspector.go` — record inspector (right-side form view)
- `cell_edit_popup.go`, `json_format.go` — cell editor + JSON pretty-print/highlight

**Row & schema editing**
- `editing.go` — edit staging, FK nav, table-name rewrite
- `insert.go`, `delete_rows.go`, `truncate.go` — INSERT/DELETE/TRUNCATE builders
- `fk_query.go` — foreign-key follow query builder (`g d` / `g b`)
- `schema_editor.go` — tabbed structure view (`d`): Columns grid + Indexes/FKs/Checks/Triggers/Definition
- `schema_ops.go` — async DDL execution + post-edit syncing
- `schema_guard.go` — pre-validates column drop/rename/modify (PK/auto-inc guards)
- `grid_table.go` — shared box-grid renderer for the read-only structure tabs
- `table_designer.go` — full-screen new-table grid editor (`N`)
- `add_column_form.go`, `table_rename_form.go`, `sidebar_ops.go`

**Relationships & ERD**
- `rel_explorer.go` — relationship explorer (`g r`): a row's inbound/outbound FK graph
- `erd.go` / `erd_graph.go` / `erd_panel.go` — static ERD (`g R`): layout, dependency ranking, Mermaid export

**Search & lookup**
- `cross_search_panel.go` / `cross_search_ops.go` — cross-table search (`S`)
- `lookup_panel.go`

**History, bookmarks, explain**
- `history_panel.go`, `bookmark_panel.go`, `explain_panel.go`

**Export / import**
- `export_import.go`, `export_picker.go`, `export_overlay.go`, `export_format.go`, `import_prompt.go`

**AI assistant**
- `assistant.go` — the assistant panel
- `ai.go`, `ai_provider_form.go`, `provider_picker.go`, `model_browser.go` — provider/model configuration

**Command system** (the unified action layer)
- `registry.go` — single source of truth for keybindings (`Binding` / `Section` types)
- `help.go` — help overlay (`?`), renders from the registry
- `palette.go` — fuzzy jump-anywhere palette (`Ctrl+P`: bindings, tables, bookmarks, themes)
- `keymsg.go` — maps dispatch token strings → `tea.KeyMsg` for replay
- `excmd.go` / `excmd_registry.go` — `:` Ex command line

**Themes & icons**
- `themes.go` / `themes_generated.go` (codegen from iTerm2-Color-Schemes) / `theme_picker.go`
- `icons.go` — unicode/nerdfont glyph sets

**Session & misc**
- `session.go` — workspace snapshot save/restore
- `mouse.go` — mouse routing (clicks, scroll, click-outside-to-dismiss)
- `fuzzy.go` — `fuzzyMatch` scorer + `fuzzyRank[T]` filter/sort helper used by every fuzzy list
- `column_picker.go`, `modal_list.go`, `field_box.go`

## Key design decisions

- **Driver interface** — `internal/db/db.go` defines the `DB` interface; every driver implements it, so adding a database is self-contained. It includes `Begin(level IsolationLevel) (Tx, error)` for transactional batch writes (inline edits & row clones are atomic) and for manual `:begin [isolation]` transactions.
- **Elm-style state machine** — the UI is an immutable state machine in `app.go` (`stateConnections` → `stateWorkspace`), with `Focus` cycling between panels. Model methods use value receivers (the model is immutable; updates return a new copy).
- **Pure-Go SQLite** — `modernc.org/sqlite`, no CGO, which simplifies cross-compilation. Please don't introduce a CGO dependency.
- **Keybinding registry** — `registry.go` is the single source of truth for both the `?` help overlay and the `Ctrl+P` palette. Each `Binding` carries a `Display` string, dispatch `Tokens`, and a `Desc`; `help.go` only renders it. The `TestKeybindingsMatchDispatch` test parses dispatch (`case` literals + `key.WithKeys` args) via `go/parser` and asserts every documented token is actually implemented, preventing help/dispatch drift.
- **Command palette replay** — `palette.go` fuzzy-searches the registry plus
  jump targets (tables, bookmarks, themes). Bindings replay via synthetic
  `tea.KeyMsg` (`keymsg.go`); jump rows emit `paletteJumpMsg` and share
  helpers with `:goto` / bookmark enter / `:theme`. History stays on its own
  panel (`Ctrl+Y`). Multi-action bundles and double-press chords that lack a
  single replay sequence stay discoverable but not auto-executable.
- **Read-only mode (defense in depth)** — a per-connection `readonly: true` flag or global `--readonly`. Writes are blocked twice: (1) a shared guard in `db.go` classifies the statement and rejects INSERT/UPDATE/DELETE/DDL in every driver's `Exec`; `Begin`/`Session` (imports) are refused outright. (2) the engine itself is opened read-only where supported — SQLite `PRAGMA query_only = ON`, Postgres `default_transaction_read_only=on` startup param; MySQL relies on the guard.
- **Secret storage** — `internal/secrets` wraps `go-keyring`. `Store` returns a `secret://<conn>/<field>` ref stored in config; `Resolve` turns a ref (or plaintext) back into the value at connect time. Plaintext fallback when no keychain is available; backward compatible (plaintext values pass through unchanged).
- **Fuzzy ranking** — `fuzzyRank[T]` in `fuzzy.go` is the generic filter+sort used by every fuzzy list (sidebar, history, bookmarks, palette, pickers, completion). Callers handle the empty-query case; the helper requires a non-empty query.
- **Result tabs** — `ResultsTab` (`tabs.go`) holds per-tab state (results, pagination, filters, sort, query stack, editor content). `m.results` is always the active tab's `ResultsTable` (value copy); `saveTabState`/`restoreTabState` copy between Model and tab on switch. `SetResult` creates fresh maps so value copies don't share mutable state.

## Conventions

- All colors/styles centralized in `internal/ui/styles.go` — use `lipgloss.Color()`, not raw strings.
- Database scans use `sql.NullString` for safe nullable handling.
- Commits follow [Conventional Commits](https://www.conventionalcommits.org); UI changes commonly scope as `(ui)`.
