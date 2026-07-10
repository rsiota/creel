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

### 2. "Test Connection" action in the form
`connection_form.go` validates fields but never opens the DB before saving. Add
a `T` (test) action that calls `db.New(cfg).Connect()` and reports the real
error (auth, host unreachable, bad DB name).
- Files: `internal/ui/connection_form.go`.

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

### 7. App-level settings in config
`Config` only has `Connections`. Move hardcoded values into a `Settings` block:
```yaml
settings:
  page_size: 500
  theme: tokyo-night      # tokyo-night | gruvbox | catppuccin | nord | light
  confirm_destructive: true
  default_driver: sqlite
  query_timeout: 30s
  cursor_style: block
```
- Files: `internal/config/config.go`, `internal/ui/app.go` (`defaultPageSize`),
  `internal/ui/styles.go`.

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

### 12. Connection groups / folders
Collapsible groups ("Work", "Personal", "Prod") in the connection list for users
with many connections.
- Files: `internal/ui/connection_list.go`, `internal/config/config.go`.

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
