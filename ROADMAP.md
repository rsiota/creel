# gsql — Roadmap & Improvement Ideas

Sourced from a code review on 2026-07-10. Priorities are suggestions; items are
independent unless noted. Each item lists the files most likely to change.

---

## 🔴 High-value gaps (clear missing features)

### 1. Secret backend for passwords ✅ DONE (2026-07-10)
Implemented in `internal/secrets/` + wired through the connection form, save,
connect, and delete flows. See README "Configuration" and the secrets package
doc for details. Keyring library: `github.com/zalando/go-keyring`. Remaining
follow-up: the connection form does not yet expose `ssh_passphrase` for editing
(it is preserved across edits and resolved at connect).

### 2. "Test Connection" action in the form ✅ DONE (2026-07-10)
`ctrl+t` in the add/edit connection form validates the fields and opens the
database in a background goroutine (without saving) to surface the real error
— auth failure, unreachable host, bad database name. A `connTestResultMsg`
reports `✓ Connected (<driver>)` on success or the driver error on failure,
shown inline in the form. The form screen gained a status bar so the
`enter`/`ctrl+t`/`esc` hints are visible. The `connectToDB` config mapping
was extracted into a shared `connConfigToDB` helper used by both connect and
test.
- Files: `internal/ui/connection_form.go`, `internal/ui/connection_ops.go`,
  `internal/ui/app.go`, `internal/ui/hints.go`.

### 3. Read-only / safe mode ✅ DONE (2026-07-10)
Implemented as defense in depth: a `readonly: true` per-connection config flag
(plus a global `--readonly` CLI flag) merges into the driver config at connect.
A shared `isWriteQuery`/`rejectWriteIfReadOnly` guard in `internal/db/db.go`
rejects writes in every driver's `Exec`/`ExecuteContext` with `ErrReadOnly`, and
`Begin`/`Session` are refused outright (no transactional edits or imports). The
engine is also opened read-only where supported — SQLite `PRAGMA query_only`,
Postgres `default_transaction_read_only=on`; MySQL relies on the guard (no
reliable pool-wide option). A `READ-ONLY` badge shows in the status bar, and the
connection form gained a `Read-only (yes/no)` toggle.

### 4. Indexes, triggers, views, and constraints ✅ DONE (2026-07-10)
Implemented `Indexes`, `Triggers`, `ViewDefinition` across sqlite/mysql/postgres
plus a read-only **Structure** overlay (`i` in the sidebar) showing columns,
PK, FKs, indexes, triggers, and view definitions. Check constraints are the
deferred fast-follow.

**Follow-ups still open:**
- Check constraints (PG `pg_constraint`; MySQL `information_schema.check_constraints`;
  SQLite parses them from the table DDL — fiddly).
- Surface the partial-index predicate on SQLite (PRAGMA only gives a 0/1 flag;
  would need parsing `sqlite_master.sql`).
- A small `view`/`table` type badge in the sidebar list (currently only shown
  inside the structure view's header).

---

## 🟡 Medium-value improvements

### 5. User-facing transactions
`Begin() (Tx, error)` exists and is used internally (`editing.go`). Expose a
transaction mode: begin → stage writes → commit/rollback, with a status-bar
indicator like `TXN ●`.
- Files: `internal/ui/app.go`, `internal/ui/editing.go`,
  `internal/ui/statusbar.go`, `internal/ui/registry.go`.

### 6. More export formats
`export_import.go` only writes CSV (plus `Y` → INSERT statements). Add a format
picker for JSON (array/lines), Markdown, TSV, and SQL INSERT dump. Reuse the
`export_picker.go` pattern.
- Files: `internal/ui/export_import.go`, `internal/ui/export_picker.go`.

### 7. App-level settings in config ✅ DONE (2026-07-12)
Added a `Settings` block to the config with a custom YAML `Duration` type
(friendly `30s` / `2m` form) and an `Effective()` normalizer. Currently wired:
`page_size`, `query_timeout` (replacing the hardcoded `defaultPageSize`/`defaultQueryTimeout`),
and `default_driver` (seeds the add-connection form, validated with a sqlite
fallback). Zero values fall back to defaults, and a missing/zero settings block
is omitted on save so connection edits don't sprout a `settings:` block.

**Follow-ups still open:**
- `confirm_destructive` (gate the ~8 destructive confirmation sites — drop
  table/db, truncate, clear history/bookmarks, discard edits, schema DDL).
  Safety-critical; the field is reserved on `Settings` but not yet applied.
- `theme` ✅ DONE (2026-07-13): a `colorPalette` type + `applyPalette` in
  `styles.go` rebuilds every package-level color and derived style var from a
  palette, so a single call re-themes the whole UI on the next `View()` pass.
  Five hand-tuned themes ship (`tokyo-night`, `gruvbox`, `nord`, `catppuccin`,
  `light`) in `internal/ui/themes.go`, plus ~565 auto-derived from
  iTerm2-Color-Schemes via `cmd/genthemes` (derives the 19 semantic slots
  from each scheme's 16 ANSI colors; skips any failing WCAG-AA fg/bg
  contrast). The picker shows capitalized display names and supports
  type-to-filter (fuzzy) + scrolling.
  `settings.theme` applies one at startup via `NewModel`, and `g c` opens a
  live-preview theme picker (`theme_picker.go`) — `↑`/`↓` (or typing to
  filter) applies the palette immediately, `enter` persists to config, `esc`
  reverts. Regenerate the derived catalog with `go run ./cmd/genthemes`;
  see `THIRDPARTY.md` for attribution.
- `cursor_style`: needs results-cursor rendering work. Field reserved.
- Letting `query_timeout: 0` disable the deadline via config — currently
  `Effective` replaces 0 with the 30s default (the runner already supports 0 =
  no deadline; needs a sentinel to opt out through config).
- Files: `internal/config/settings.go`, `internal/config/config.go`,
  `internal/ui/app.go`, `internal/ui/connection_form.go`.

### 8. Configurable query timeout
Only the SSH tunnel has a timeout (`ssh_tunnel.go`). A runaway `SELECT` hangs
the TUI until `esc`. Wire a `settings.query_timeout` into `ExecuteContext`.
- Files: `internal/db/db.go`, `sqlite.go`, `mysql.go`, `postgres.go`.

### 9. Session restore
Persist the active tab's editor content + table per connection so reopening a
connection restores where you left off. History store is a natural home.
- Files: `internal/history/`, `internal/ui/tabs.go`, `internal/ui/app.go`.

---

## 🟢 Polish / nice-to-haves

### 10. Vim `:` ex-command mode
Power-user delight, fits the vim ethos. `:w` save edits, `:q` close tab,
`:sort name`, `:goto users`, `:filter status=active`. The palette's replay
mechanism (`keymsg.go`) could execute some.
- Files: `internal/ui/app.go`, new `internal/ui/excmd.go`, `internal/ui/registry.go`.

### 11. ERD / relationship view
FK data already exists (`ForeignKeys`). A `g R` overlay rendering
boxes + arrows (Lipgloss) or exporting Mermaid would be distinctive.
- Files: new `internal/ui/erd.go`, `internal/ui/app.go`.

### 12. Connection groups / folders ✅ DONE (2026-07-12)
Connections carry an optional `group` field; the connection list renders
collapsible folder headers (▾/▸) when any connection is grouped, and stays flat
(byte-for-byte the old layout) when none are — so existing configs are
unaffected. Ungrouped connections lead under an "Ungrouped" header, then named
groups alphabetically; within a group, config order is preserved.

Navigation is unified over a single row sequence (headers + connection boxes):
- `space` (or `tab`) folds/unfolds the group under the cursor; `enter` toggles
  on a header and connects on a connection.
- `g`/`G` skip headers and land on the first/last connection.
- Filtering flattens to ranked matches (no headers); committing a filter keeps
  the cursor on the selected connection in the restored grouped layout.
- Mouse clicks fold a header / select+connect a connection.

Scroll is line-based (headers are 1 line, boxes are `linesPerField`), snapped to
row boundaries. Grouped connection boxes are indented under their header (with
a right-aligned count) so the parent-child relationship reads visually; flat
mode is unchanged. The popup height is based on the fully-expanded layout, so
it stays constant while filtering or folding. The add/edit form gained a
`Group` field (editing preserves the group). Collapse state survives connection
reloads.
- Files: `internal/config/config.go`, `internal/ui/connection_list.go`,
  `internal/ui/connection_form.go`, `internal/ui/app.go`, `internal/ui/mouse.go`.
  Tests: `internal/ui/connection_groups_test.go` (13).

### 13. Per-query timing history
`Elapsed` is captured per query. Persist it in history to surface slowest
queries or a sparkline.
- Files: `internal/history/`, `internal/ui/history_panel.go`.

### 14. `.sql` file integration
`gsql -f query.sql` and `:open file.sql` to load a script. Natural extension of
the statement splitter (`statements.go`).
- Files: `cmd/gsql/main.go`, `internal/ui/app.go`, `internal/db/statements.go`.

---

## Suggested starting order
1. **Keyring password storage** (#1) — self-contained, biggest real-world blocker.
2. **Indexes/triggers/views metadata** (#4) — most visible feature gap.
3. **Read-only mode** (#3) — small change, big safety win for prod.
