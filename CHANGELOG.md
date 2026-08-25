# Changelog

All notable changes to creel are documented here.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

GitHub Release notes are copied from this file at release time; the GoReleaser
auto-changelog is kept only as a fallback (it excludes `docs:`/`ci:`/`test:`
commits, so it can come up empty).

## [Unreleased]

### Added
- `inspector_open` setting / `:set inspector_open on|off` — open the row
  inspector by default when entering a workspace (also opens/closes immediately
  from `:set`).

### Fixed
- Transparent background: reset default-bg cells when transparency is on so
  stale theme colour from a prior opaque frame does not linger (e.g. sidebar
  borders on light themes after `:set transparent_background on`).

## [0.3.1] - 2026-08-25

Patch release: Postgres connect fix, connection-form polish, and a few light-theme
/ UI fixes that landed after 0.3.0.

### Added
- Connection form: filesystem path completion and mouse field selection.

### Fixed
- Postgres: stop sending libpq `keepalives=*` as startup params (pgx was
  forwarding them and the server rejected connect with
  `unrecognized configuration parameter "keepalives"`). TCP keepalives stay
  enabled on the dialer. Fixes [#1](https://github.com/rsiota/creel/issues/1).
- Expand `~` in SSH private key paths.
- Light-theme popups: eliminate dark stray lines when
  `transparent_background` is on; respect that setting on the connection picker.
- Status-bar hint flash: dim idle keys and flash the pressed key as bold cell
  fg so active bindings stay readable on light themes.
- Connection form: debounce mouse-wheel to one field per notch.
- Close the inspector when starting a cell edit from the results grid (`e` /
  `i` / `E`).

## [0.3.0] - 2026-08-23

More chart types, AI explain/fix, a jump-anywhere palette, ERD polish, named
query params, and light-theme contrast fixes across the TUI.

### Added
- CLI: `-e -` (or `-cli` without `-e`) reads the query from stdin; failures
  exit status `1` (documented in `docs/cli.md`).
- `:watch` / `:tail` redraw an open chart on each refresh (bang charts
  re-fetch); new or changed result rows are tinted between ticks.
- Query parameters: `:param name value` then `:name` in SQL; `:param` lists,
  `:param!` / `:param! name` clears. Status bar shows `PARAM n`. Skips string
  literals, comments, and Postgres `::` casts.
- Connection picker MRU: remembers recently opened connections, selects the
  last-used entry on reopen, and shows a muted `recent` badge.
- Empty connection list offers **Try the demo database** (Enter) — opens
  `./demo/creel-demo.db` when present, otherwise materializes the embedded
  sample schema under the config dir.
- Editor SQL completion is contextual: after `FROM`/`JOIN` it offers tables, after `WHERE`/`ON`/`SET` only columns of those tables (and `alias.`).
- TLS (`sslmode`) and unix sockets on MySQL/Postgres connections (form, YAML, `-sslmode`/`-socket`). Empty `sslmode` means `prefer`.
- Query editor: `u`/`U` undo/redo, `/` buffer search with `n`/`N`, `V` visual-line yank/delete.
- ERD Enter / double-click browses the focused table (`SELECT *`); `f` keeps neighbourhood drill. Opening the ERD traces the FK path from the grid cursor; the explorer highlights the matching edge.
- Histogram charts (`:hist`) of a numeric column, with optional bin count.
- Frequency bar charts (`:freq`, or `:bar` with one column) of distinct values.
- Pie charts (`:pie`) of a column — same counts as `:freq`, Braille pie + legend.
- Scatter charts (`:scatter`) of two numeric or datetime columns, without connecting the points.
- `:line` / `:scatter` plot datetime x (ISO-8601, SQL timestamps, date-only) as Unix seconds, keeping the original text as the axis label.
- `Enter` on a bar/hist/freq chart keeps rows for that bar and restores the grid.
- Relationship explorer: `t` opens a node in a new tab; insert-related (`A`) inserts into the child table without leaving the parent grid, then restores the explorer after save or cancel.
- Auto-reconnect for MySQL/Postgres: keep-alive Ping, SSH tunnel keepalive, and in-place rebuild with a status-bar “reconnecting…” (`:reconnect`). Workspace tabs/editor stay put.
- Transaction isolation on `:begin` (`serializable`, `repeatable read`, `read committed`, `read uncommitted`, plus short forms). Status bar shows `TXN S` / `TXN RR` / ….
- Jump the editor cursor to MySQL/Postgres/SQLite syntax-error positions (line / `near "…"`, including pagination-wrapped SELECTs).
- Results chord `y r` copies marked/cursor rows as TSV (same as `:copyrow`); `Y` remains INSERT, `y y` remains cell copy.
- `:aifix` (alias `:fixsql`) asks the configured AI to rewrite the last failed query; the candidate lands in the editor for review (never auto-run).
- ERD `H`/`J`/`K`/`L` nudges the focused card (same as a mouse drag) so the diagram is usable over SSH / without a mouse.
- ERD card positions (mouse drag and `H`/`J`/`K`/`L`) persist in the per-connection session snapshot, scoped by whole-schema vs neighbourhood layout.
- ERD mini-map in the bottom-right when the diagram is larger than the viewport; click or drag it to pan.
- ERD `i` inserts JOIN SQL for a traced FK path (`p`) into the editor.
- AI schema context is the focused table plus its FK neighbours (and any tables named in the question or failed SQL), instead of the first 100 tables.
- Results grid: soft FK cell tint (primary blue darkened toward the theme
  background); PK columns unchanged aside from the existing `*` marker;
  headers stay primary. Cursor / mark / search / dirty styles still win.
- Results grid: status/state columns (`status`, `*_status`, …) get a soft
  colored `● value` tint (theme success/warn/accent/error blended toward
  the background, same weight as FK cells). Display only; copy and edit stay
  raw.
- Results grid: datetime columns show a compact absolute form
  (`2026-08-21 14:32`, dropping seconds/fraction/zone); date-only and
  time-only stay `2006-01-02` / `15:04`. Detected by type or `*_at` /
  similar names. Display only.
- Jump-anywhere palette (`Ctrl+P`): fuzzy-search tables, bookmarks, and
  themes in addition to keybindings. Enter opens a table (`SELECT *`), loads
  a bookmark into the editor, or applies a theme. Query history stays on
  `Ctrl+Y`.
- `:aiexplain` (alias `:why`): ask the AI to explain the statement under the
  cursor (or the last explained SQL), attaching the EXPLAIN / EXPLAIN QUERY
  PLAN output. Streams the prose reply into the assistant panel — never
  auto-runs. Optional focus text, e.g. `:aiexplain why is the join slow`.
- Keyboard pane resize: `alt+h/j/k/l` (or `ctrl+alt+…`) nudges the adjacent
  seam in that direction (same seams as mouse drag).
- `:set` / `:set option=value` changes runtime config settings from the TUI
  (with completion); values persist to the config file.

### Fixed
- ERD mini-map stayed visible for expanded (tall) diagrams instead of hiding when aspect-fitting made the overlay narrower than a minimum, and kept enough width that rank columns aren't cropped off the sides.
- SQL export (`X`) emits native CREATE TABLE DDL (MySQL `SHOW CREATE TABLE`, SQLite `sqlite_master`) so indexes, named foreign keys, ON DELETE/UPDATE, CHECK constraints, and ENGINE/CHARSET survive a dump round-trip. Unsigned integer values are no longer quoted as strings. MySQL string literals backslash-escape `\`, quotes, and control characters (mysqldump / Sequel Ace style) so PHP namespaces like `App\Models\User` round-trip.
- SQL import (`I`) honours MySQL backslash escapes (`\'`), backtick identifiers, and `#` comments, so Sequel Ace / mysqldump files no longer swallow `CREATE TABLE` statements after a quoted apostrophe. Failed statements are named in the status bar.
- Light-theme contrast: connection-picker labels, ERD column names, marked
  columns, `:pie` outlines, ERD selection vivid/dim, popup backdrops, cell-edit
  and SQL-editor cursor lines, and filter-picker rows stay readable under
  `paintBg` (notably GitHub Light Default).
- Help overlay stays open during fast mouse-wheel scrolls.
- Results grid stays aligned after multiline cell saves; filter/search prompts
  accept space; connect failures show friendly errors.

## [0.2.0] - 2026-08-14

Charts, a richer FK explorer, mouse-resizeable panels, and CLI output — plus a quicker path from the sidebar into results.

### Added
- Bar charts from marked result columns (`M` + `:bar`), with sum/count/avg grouping and a cursor.
- Line charts (`:line`) of two numeric columns, drawn as a continuous Braille series with a dim crosshair.
- Browse and insert related rows from the FK explorer.
- Remembered results-column widths per table.
- Mouse-resize for the sidebar↔centre, editor↔results, and centre↔right seams.
- `-format` query output and `-c` connection reuse with flag overrides.
- Read-only cell viewer (`E`) on non-editable results.
- `:copyrow` to copy the current result row.
- Safer BLOB rendering; `-database` is honored in the TUI.

### Changed
- Sidebar `l` jumps to the results grid; `tab` / `shift+tab` skip the tab bar.
- Relationship explorer no longer shows a breadcrumb path.

### Fixed
- Wheel events are coalesced so momentum scroll no longer runs away.
- Empty strings are no longer coerced to NULL when copying cells.

## [0.1.1] - 2026-08-03

A follow-up to the initial release: release tooling, install options, and docs polish.

### Added
- `-version` flag to print build version information.
- Homebrew install (`brew install rsiota/creel/creel`) and `go install` quick-start.
- Asciinema demo in the README.
- Issue templates (bug report, feature request) and a security policy ([SECURITY.md](SECURITY.md)).

### Changed
- Releases are now built and published with GoReleaser — cross-platform binaries (Linux/macOS/Windows on amd64/arm64) with checksums.

## [0.1.0] - 2026-08-02

First public release. creel succeeds `gsql` (the project was renamed) and migrates `~/.config/gsql/` to `~/.config/creel/` automatically on first launch.

### Added
- **Three databases, one interface** — SQLite, MySQL, and PostgreSQL, with SSH tunneling for remote MySQL.
- **Vim-mode editor** — normal/insert modes, motions (`h/j/k/l`, `w/b`), operators (`dd`, `dw`, `x`, `D`), yank/paste, and SQL autocompletion.
- **Static ERD** (`g R`) — graphical entity-relationship diagram with box-drawing FK arrows; fit-to-screen, collapse/expand, hover tooltips, and Mermaid export.
- **Relationship explorer** (`g r`) — browse a row's inbound/outbound foreign keys as an expandable object graph.
- **Results grid** — sort, filter, regex search, hide/show columns, follow foreign keys, mark and bulk-delete rows.
- **Inline editing** — edit cells, insert/clone rows, paste from clipboard.
- **Schema editing** — add columns, rename/create/drop/truncate tables, and a grid-based table designer.
- **Import / export** — streaming SQL dump importer and a pure-Go `mysqldump`-compatible exporter.
- **Secret storage** — passwords and SSH keys live in the OS keychain, never in plaintext config.
- **AI assistant** — natural-language → SQL via any OpenAI-compatible endpoint (key stored in the keychain).
- **Read-only mode** for safely pointing at production.
- **Session restore**, per-connection query history & bookmarks, EXPLAIN plans, and ~570 themes.

[Unreleased]: https://github.com/rsiota/creel/compare/v0.3.1...HEAD
[0.3.1]: https://github.com/rsiota/creel/compare/v0.3.0...v0.3.1
[0.3.0]: https://github.com/rsiota/creel/compare/v0.2.0...v0.3.0
[0.2.0]: https://github.com/rsiota/creel/compare/v0.1.1...v0.2.0
[0.1.1]: https://github.com/rsiota/creel/compare/v0.1.0...v0.1.1
[0.1.0]: https://github.com/rsiota/creel/releases/tag/v0.1.0
