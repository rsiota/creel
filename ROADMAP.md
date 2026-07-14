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
plus a read-only **Structure** view (`d` in the sidebar) showing columns,
PK, FKs, indexes, triggers, and view definitions.

**Follow-ups still open:**
- Check constraints ✅ DONE (2026-07-14): new `CheckConstraints(table)` DB
  method + a **Checks** tab in the structure view (Columns · Indexes ·
  Foreign Keys · Checks · Triggers · Definition). PostgreSQL queries
  `pg_constraint` (`contype='c'`, unwrapped via `pg_get_constraintdef`);
  MySQL joins `information_schema.check_constraints` + `table_constraints`
  (8.0.16+); SQLite parses `CHECK (...)` groups out of the table DDL with a
  literal/comment/paren-aware scanner, associating column-level checks with
  their column. Loaded async with its own per-section error like the other
  metadata tabs.
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

### 6. More export formats ✅ DONE (2026-07-11)
The `g X` results-export path now offers a format picker (`format_picker.go`):
CSV, JSON (array), JSONL, Markdown, TSV, and SQL INSERT dump, all sharing one
`serializeFormat` renderer and filename/extension mapping. The legacy `x` keeps
CSV as the one-key default.
- Files: `internal/ui/export_import.go`, `internal/ui/format_picker.go`,
  `internal/ui/export_format_test.go`.

### 7. App-level settings in config ✅ DONE (2026-07-12)
Added a `Settings` block to the config with a custom YAML `Duration` type
(friendly `30s` / `2m` form) and an `Effective()` normalizer. Currently wired:
`page_size`, `query_timeout` (replacing the hardcoded `defaultPageSize`/`defaultQueryTimeout`),
and `default_driver` (seeds the add-connection form, validated with a sqlite
fallback). Zero values fall back to defaults, and a missing/zero settings block
is omitted on save so connection edits don't sprout a `settings:` block.

**Follow-ups still open:**
- `confirm_destructive` ✅ DONE (2026-07-14): a `Model.confirmDestructive()`
  helper (default true / safe; `false` skips) guards every destructive trigger
  site — drop table, drop database, truncate, delete rows, discard edits, drop
  column, clear history, clear bookmarks. When skipped, the action runs
  immediately via the same exec helpers the confirmation handlers use
  (`execTruncate`, `execDeleteRows`, `execDropDatabase`, `execSchemaDDL`, plus
  a new shared `execDropTable`). The confirmation dialogs themselves are
  unchanged.
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
  reverts. The theme's background is painted via a compositing pass
  (`background.go`) so light themes render readably; `transparent_background`
  in config opts out to keep terminal transparency. Regenerate the derived
  catalog with `go run ./cmd/genthemes`; see `THIRDPARTY.md` for attribution.
- `cursor_style`: needs results-cursor rendering work. Field reserved.
- Letting `query_timeout: 0` disable the deadline via config — currently
  `Effective` replaces 0 with the 30s default (the runner already supports 0 =
  no deadline; needs a sentinel to opt out through config).
- Files: `internal/config/settings.go`, `internal/config/config.go`,
  `internal/ui/app.go`, `internal/ui/connection_form.go`.

### 8. Configurable query timeout ✅ DONE
Superseded by #7's `settings.query_timeout` (wired into `ExecuteContext` in
2026-07-11 commit `e0a785e`). The only remaining bit is the `query_timeout: 0`
opt-out sentinel, tracked under #7 follow-ups. Kept for history; no standalone
work remains.

### 9. Session restore
Persist the active tab's editor content + table per connection so reopening a
connection restores where you left off. History store is a natural home.
- Files: `internal/history/`, `internal/ui/tabs.go`, `internal/ui/app.go`.

---

## 🟢 Polish / nice-to-haves

### 10. Vim `:` ex-command mode ✅ DONE (2026-07-14, v1)
A modal `:` command line (`internal/ui/excmd.go`) opens globally on `:` and is
routed ahead of all other workspace keys. `enter` parses and runs, `esc`
cancels, `↑`/`↓` recalls history. v1 commands: `:q`/`:q!`/`:w`/`:wq`/`:x`
(close tab, blocking on unsaved edits unless forced), `:sort <col>`
(`sortByColName`), `:goto <table>`/`:gt` (exact/substring match → `SELECT * FROM`),
`:help`/`:h`. A shell-like parser (`splitShellFields`) honors quotes/escapes and
a trailing `!`. Unknown input is `E492: not a command`; a bare identifier in the
results view falls back to a fuzzy column jump (`bestColumnMatch`), preserving
the legacy `:` behaviour — the standalone column-jump prompt (`columnJumping`)
was removed and its render/mouse/tab sites now key off `ex.IsVisible()`.
Feedback/errors surface via the transient status-bar message.
- Files: `internal/ui/excmd.go`, `internal/ui/app.go`, `internal/ui/tabs.go`,
  `internal/ui/mouse.go`, `internal/ui/registry.go`.
  Tests: `internal/ui/excmd_test.go` (parser, state, key handling, each command,
  fallback, unknown).

**Follow-ups still open (v2):** argument commands needing more wiring
(`:filter <expr>`, `:open`/`:save` — pairs with #14, `:theme <name>`,
`:set <opt>`), and live command completion/suggestions as you type.
- Originally: `:w` save edits, `:q` close tab, `:sort name`, `:goto users`,
  `:filter status=active`. The palette's replay mechanism (`keymsg.go`) could
  execute some.

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
The original top three are all shipped (#1, #4, #3). Next up, in suggested
order:
1. **`confirm_destructive`** (#7 follow-up) — field reserved, ~8 well-defined
   sites, safety win. _(done — 2026-07-14)_
2. **Check constraints** (#4 follow-up) — closes the visible gap in the
   structure view; PG/MySQL are clean, SQLite parses DDL.
   _(done — 2026-07-14)_
3. **`.sql` file integration** (#14) — reuses the statement splitter, high
   everyday value, mostly entry-point wiring.
4. **User-facing transactions** (#5) — `Begin()` exists; the work is UI +
   status indicator + read-only interplay.
5. **Session restore** (#9) — persistence delight feature.

Original historical order (all complete): keyring storage (#1),
indexes/triggers/views (#4), read-only mode (#3).
