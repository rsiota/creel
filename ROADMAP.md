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

### 5. User-facing transactions ✅ DONE (2026-07-15, v1)
`Begin() (Tx, error)` was already used internally (`editing.go`) for atomic
batched saves. v1 exposes a manual transaction via the ex line: `:begin` /
`:commit` / `:rollback`, with a `TXN ●` status-bar indicator.

The planned v1/v2 fork (write-only Tx vs. read-in-tx) collapsed: extending
`Tx` with `Execute`/`ExecuteContext` was cheap — `executeRows` only needed
`QueryContext`, which `*sql.DB` and `*sql.Tx` already share, so a single
interface change let a transaction return result sets. v1 therefore routes
**editor-run statements** through the held tx: SELECTs run on the tx see its own
uncommitted writes, so you can stage writes and verify them before committing.

**Behaviour:**
- `:begin` is refused while read-only, while a query is in flight, or when a
  tx is already open; `:commit`/`:rollback` are refused with no tx or while a
  query runs. begin/commit/rollback run synchronously (rare, transfer no row
  data) and the `queryRunning` guard keeps them from racing in-flight query
  goroutines.
- Cell edits, inserts, deletes, and DDL are **blocked** for the duration
  (`txnBlocksWrite`): they use their own autocommit path and would commit
  outside the tx (and on MySQL/PG, DDL would implicitly commit the whole tx).
  Staged local edits aren't lost — they persist until saved after the tx ends.
- The tx is rolled back automatically at every connection lifecycle boundary
  (switch, disconnect, database change) via `rollbackTxn`.
- Files: `internal/db/db.go` (Tx.Execute, queryRunner), `internal/ui/app.go`,
  `internal/ui/excmd.go`, `internal/ui/editing.go`, `internal/ui/schema_ops.go`,
  `internal/ui/connection_ops.go`, `internal/ui/query.go`,
  `internal/ui/statusbar.go`.
  Tests: `internal/ui/transaction_test.go` (guards + live SQLite round-trip).

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

### 9. Session restore ✅ DONE (2026-07-23)
Persist the active tab's editor content + table per connection so reopening a
connection restores where you left off. History store is a natural home.
- Files: `internal/history/`, `internal/ui/tabs.go`, `internal/ui/app.go`.

Implemented as a new `internal/session` package (a sibling of `history`, since
it tracks UI workspace state rather than a query log) that persists a
`State{Tabs, Active}` as JSON under `<configDir>/sessions/`. State is keyed by
**(connection, database)** — `db.Connection.Config().Database` always reflects
the active database, so MySQL/Postgres get per-database sessions and SQLite
gets per-file behaviour for free.

**Behaviour:**
- Each tab persists its title, editor buffer (`EditorQuery`), and last executed
  statement (`LastQuery`). Result rows, cursors, and filters are intentionally
  **not** persisted — the buffer is restored verbatim and the user re-runs it
  with `ctrl+e`, mirroring the `gsql -f` startup flag. This avoids stale data
  and side-effecting writes on reconnect.
- `saveSession` is called at every teardown boundary (`showConnectionList`, the
  `connectByName` swap, `selectDatabase`) and on **quit** via a shared
  `beginQuit()` helper that every quit path (`ctrl+c`/`ctrl+q`/`q`/`:q`/`:qa`/
  `:wq`/`esc`) now funnels through, so the workspace is captured exactly once
  before exit.
- `restoreSession` runs after `loadTables` on connect / database switch. With
  no session (or a blank one) the default single "New Query" tab is untouched.
- A `gsql -f` startup file takes precedence: `loadStartupFile` arms a one-shot
  `startupFileLoaded` flag that suppresses the **first** connect's restore, so
  an explicitly-loaded buffer is never clobbered by a returning session;
  later connects restore normally.
- Files: `internal/session/` (store), `internal/ui/session.go` (save/restore/
  beginQuit), `internal/ui/connection_ops.go`, `internal/ui/app.go`,
  `internal/ui/excmd.go`, `internal/ui/excmd_registry.go`.
  Tests: `internal/session/session_test.go`, `internal/ui/session_test.go`.

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

**Follow-ups still open (v2):** live command completion/suggestions as you
  type. The argument commands are now wired: `:filter <col><op><value>`
  (2026-07-17; structured form reusing the m.filters infra — value type-quoted
  via formatFilterValue, `~` = LIKE substring, `off`/`clear` and bare-list), and
  `:open`/`:save` as non-vim aliases of `:e`/`:w`. `:theme <name>` already
  shipped (v1). `:set <opt>` is deferred (the runtime toggles are covered by
  :timing/:limit/:theme; :set would mainly add confirm_destructive/timeout).
- Originally: `:w` save edits, `:q` close tab, `:sort name`, `:goto users`,
  `:filter status=active`. The palette's replay mechanism (`keymsg.go`) could
  execute some.

### 11. ERD / relationship view
FK data already exists (`ForeignKeys`). A `g R` overlay rendering
boxes + arrows (Lipgloss) or exporting Mermaid would be distinctive.
- Files: new `internal/ui/erd.go`, `internal/ui/app.go`.

**Related, ✅ DONE (2026-07-20): interactive relationship explorer (`g r` /
`:explore`) — expand-in-place tree.** Rather than a static diagram, this ships
the navigation half of the idea as a modal overlay
(`internal/ui/rel_explorer.go`) that turns the focused results row into a
browsable folder: the row is the root; its inbound + outbound FK edges are the
first level (each with a **live count**, fanned out concurrently); expand an
edge (`→`) to load the related rows inline, expand a child row to load *its*
edges, and so on (depth-capped at `maxExplorerDepth`, child loads capped at
`explorerChildLimit` with a "+N more" escape hatch). The indentation is the
breadcrumb — you never lose context, which was the flaw in the earlier
drill-in/back model. `Enter` opens a node in the grid (a specific row via its
PK, or an edge's full set) and re-roots the tree there via `queryStack`.

It reuses existing machinery: `:refs`' count helper (`countRelated`), the
`queryStack` back-navigation that `g d`/`g b` already use, and the overlay /
modal-key-dispatch pattern from the lookup & explain panels. Both directions
are covered — outbound and inbound edges render uniformly as `table (count)`
(the gap `:refs` only half-filled), and only edges with a positive count are
shown so a row surfaces just the relationships it actually participates in.
Lazy loads (`loadExplorerChildren`) issue one query per expand; row nodes
derive PK-based labels and drill queries via `PrimaryKeys`.

**Still open (the static diagram):** the `g R` boxes-and-arrows / Mermaid
export ERD remains the visual complement to this navigator.

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

### 14. `.sql` file integration — ex commands ✅ DONE (2026-07-17)
The buffer-level commands and the `-f` startup flag both ship.

**Done:** `:e <file>` / `:edit <file>` loads a file into the editor (vim's
`:edit`, replacing the current buffer — run it from the editor as usual,
statements split at run time), and `:w <file>` / `:write <file>` writes the
editor buffer to disk. The argument presence disambiguates `:w`: no argument
still saves staged cell edits (the legacy meaning); with an argument it writes
the buffer. `~` is expanded (shared `expandTilde`, also used by the import
prompt); relative paths resolve against the working directory. `:w <file>`
overwrites — pass a versioned name to keep the old one.
- Files: `internal/ui/excmd.go`, `internal/ui/import_prompt.go`
  (`expandTilde` extracted from `ImportPrompt.ExpandPath`).
  Tests: `internal/ui/file_io_test.go`.

**Also done (2026-07-17):** `gsql -f query.sql` loads a script into the editor
at startup — the non-interactive counterpart of `:e`. It reuses the same
`expandTilde` + `os.ReadFile` path (factored as `loadStartupFile`), resolves
relative paths against the working directory, and fails fast (stderr + exit 1)
on a missing/unreadable file. The script is loaded, not executed — the user
reviews it in the editor and runs it with `ctrl+e`.
- Files: `cmd/gsql/main.go` (`-f` flag, passed through `ui.Run`),
  `internal/ui/statusbar.go` (`Run` loads the file before starting the program),
  `internal/ui/excmd.go` (`loadStartupFile`).
  Tests: `internal/ui/file_io_test.go` (`TestLoadStartupFile`).

### 15. `:` command-set roadmap
A prioritized plan for growing the ex command line, distilled from a command
survey and refined toward Helix-style discoverability (2026-07-17).

**Guiding principle (revised):** Shortcuts are for muscle memory; `:` commands
are for discoverability, arguments, and (later) scripting. Overlap is
*intentional* for high-level noun/verb actions — `:run` ↔ `ctrl+e`, `:refresh`
↔ `ctrl+r` — as long as both call the **same helper**. Do **not** think of
that as duplication of code paths; think of it as two interfaces to one
action. The three surfaces (keys, palette, `:`) stay usefully distinct in
*UX*, but should funnel through one implementation.

Add an ex command when at least one is true:
1. It takes an argument (`:goto users`, `:limit 100`, `:theme dark`)
2. It is stateful / mode-like (`:watch`, `:begin`, `:timing`)
3. It is a high-level verb users will search for (`:run`, `:describe`, `:connect`)
4. Default-object form helps (`:peek`, `:refs`, `:indexes` with no args)

Skip commands that are only interface chrome (no sensible noun/verb):
`:cursor-down`, `:focus-results`, `:scroll-page`, `:toggle-bottom-pane`.

**Architecture habit:** extract a helper first, then wire keybinding + `:`
(+ palette). Never copy a key handler body into an ex executor. A full
unified `Action` registry (IDs for `:map` / config) remains optional — shared
helpers + `exCommands()` are enough until then. See `docs/command-registry.md`.

**Conventions:**
- **Default to the current object.** Commands with no argument operate on the
  focused table (`currentTable()`): `:count`, `:sample`, `:refs`, `:peek`,
  `:indexes`. Nearly free to implement and the biggest lever for feeling native.
- **Explicit verbs over cryptic prefixes.** `:goto` is canonical; `:gt` is a
  tolerated alias. Do **not** grow a `:gv`/`:gf`/`:gp` family — one `:goto`
  covers sidebar objects. New verbs are spelled out; short aliases land only
  after the feature ships (canonical + one short alias is the sweet spot).
- **Stateful commands own a status-bar indicator** (`TXN ●`, `WATCH 5s`, …).
- **Help stays scannable.** Prefer category grouping in `?` / `:help` over
  refusing useful mirrors. Avoid three long names for the same action.

**Tier 1 — parameterized / stateful, high value:** ✅ complete
- `:begin` / `:commit` / `:rollback` (+ `TXN` indicator) — see #5.
- `:w file.sql` / `:e file.sql` — see #14; `gsql -f` startup flag.
- `:import <file>` — shares `execImportSQL` with the `I` key.
- `:export <fmt>` — non-interactive shortcut over the `g X` picker.

**Tier 2 — monitoring & graph:** ✅ complete
- `:watch [n]`, `:tail [table]`, `:refs [table]`, `:uses [table]`.

**Tier 3 — small wins:** ✅ complete
- `:count` / `:sample`/`:head` / `:peek` / `:bookmark` / `:rerun` /
  `:timing` / `:limit` — plus earlier aliases (`:explain`, `:refresh`,
  `:history`, `:bookmarks`, `:describe`/`:desc`, `:stats`, `:format`, `:theme`,
  `:filter`, `:open`/`:save`).

**Tier 4 — DBA / niche (opt-in, audience-driven):**
- `:who`, `:locks`, `:kill <pid>` — session / lock inspection; driver-specific;
  `:kill` is dangerous (confirm_destructive).

**Tier 5 — next commands (discoverability + remaining verbs):**

Ship in roughly this order. Each item notes the shared helper / key to reuse.

*Wave A — cheap high-level mirrors (extract-or-reuse helpers):* ✅ done (2026-07-17)
1. **`:run` / `:r`** ✅ — shares `executeQuery` with `ctrl+e` / `\`.
2. **`:d`** ✅ — short alias of `:describe`/`:desc`.
3. **`:qa` / `:qa!`** ✅ — quit all tabs; dirty check spans inactive tabs via
   `saveTabState`.
4. **`:tabnew`**, **`:tabclose[!]`**, **`:tabnext`/`:tabn`**, **`:tabprev`/`:tabp`**,
   **`:tabs`** ✅ — share `addTab` / `closeTab` / `TabBar.NextTab`/`PrevTab` with
   `t` / `g x` / `g t` / `g T`. `:tabclose` refuses the last tab (use `:q`).
5. **`:copy`** ✅ — shares `copyCursorCell` with the `yy` chord.

*Wave B — connections & navigation (parameterized):* ✅ done (2026-07-17)
6. **`:connect [name]` / `:c`** ✅ — bare opens the connection list; with a name
   resolves (EqualFold → substring) and connects via `connectByName` (new
   connection opens before the old one closes). Shares teardown/reset with
   `ctrl+t` via `showConnectionList` / `resetWorkspaceForNewConnection`.
7. **`:connections`** ✅ — mirror of `ctrl+t` (`showConnectionList`).
8. **`:db` / `:use [database]`** ✅ — bare opens the database picker (`ctrl+b`);
   with a name switches via `selectDatabase`. SQLite gets an explicit message.
9. **`:schema [name]`** ✅ — MySQL delegates to `:db`; Postgres lists schemas or
   switches `search_path` pool-safely (`Schemas` / `UseSchema` + reconnect);
   SQLite unsupported.

*Wave C — schema exploration (object-centric):* ✅ done (2026-07-17)
10. **`:indexes [table]`**, **`:columns [table]`**, **`:constraints [table]`**,
    **`:fk [table]`** ✅ — open the Structure panel on the matching tab via
    `exOpenStructureTab` / `SetActiveTab` (shares `openSchemaPanel` with `d` /
    `:describe`). Defaults to the current table.
11. **`:tables`/`:dt`**, **`:views`/`:dv`**, **`:schemas`** ✅ — list in the
    lookup overlay. `:tables` excludes views (new `DB.Views()`). `:schemas`
    (plural) is the list verb; `:schema` (singular) remains the switch/status
    verb from Wave B.
12. **`:search`/`:find <name>`** ✅ — fuzzy-find tables/views/columns over
    `m.tables` + `columnCache` (not cross-search cell values, not results `g /`).

*Wave D — quality-of-life:* ✅ done (2026-07-17)
13. **`:new`** ✅ — clears the editor to an empty scratch buffer (does not
    open a tab; use `:tabnew` for that).
14. **`:version`** ✅ — status-bar build label via `internal/version` /
    `debug.ReadBuildInfo`.
15. **`:plan`** ✅ — alias of `:explain` (same structure panel).
16. **`:recent [n|name]`** ✅ — in-memory MRU filled by `openTable` (shared by
    `:goto`, sidebar enter, mouse). Bare lists in the lookup overlay; a rank or
    name re-opens. Cleared on connection switch. Session persistence deferred
    to #9.

*Wave E — DDL (discoverability mirrors of sidebar actions):* ✅ done (2026-07-17)
17. **`:truncate[!] [table]`** ✅ — shares `execTruncate` with sidebar `T`;
    stages the enter/esc confirm unless `!` or `confirm_destructive: false`.
18. **`:drop[!] [table]`** ✅ — shares `execDropTable` with sidebar `D`; typed
    name confirm unless forced.
19. **`:rename [old] [new]`** ✅ — zero/one arg opens the rename form (sidebar
    `r`); two args renames via `BuildRenameTableSQL` + `execSchemaDDL`.
20. **`:create`** ✅ — opens the table designer (sidebar `N` /
    `openCreateTableForm`).

**Explicitly rejected (wrong layer / scope creep):**
- UI chrome: `:cursor-*`, `:focus-*`, `:scroll-*`, `:select-next-row`,
  `:toggle-*-pane` (unless the app becomes tmux-like).
- Shell escape: `:shell` / `:cd` / `:ls`.
- Pager/tee: `:tee` / `:pager` / `:nopager` (the TUI *is* the pager).
- Macros: `:record` / `:play` (revisit only with a real Action ID layer).
- Editor substitute: `:replace` (use vim `:s` in the editor).
- Persistence product: `:session save/load` (see #9 — not “just a command”).
- Schema-diff product: `:diff <a> <b>`.
- Favorites as a second bookmarks system: `:favorite`.

**psql aliases (`:dt` / `:dv` / `:df`, …):** `:dt` / `:dv` ship with Wave C as
aliases of `:tables` / `:views`. Remaining (`:df`, …) wait on underlying
features.

---

## Suggested starting order
The original top three are all shipped (#1, #4, #3), and the two polish
follow-ups are done (#7 `confirm_destructive`, #4 check constraints). Tiers
1–3 of the `:` command set (#15) are complete. Tier 5 Waves A–E are done.
Session restore (#9) shipped (2026-07-23). Next up:
1. **Tier 4** — opt-in DBA (`:who`/`:locks`/`:kill`) if users ask.
2. Optional DDL follow-ups: `:createdb` / `:dropdb` (db-picker `N`/`D`) ✅
   done (2026-07-17). `:addcolumn` (sidebar `a`) ✅ done (2026-07-17) — fills the
   column-DDL gap; 0–1 args open the form, `<table> <name> <type> [nullable]
   [default]` runs ALTER TABLE ADD COLUMN directly via the shared
   execSchemaDDL path. `:discard[!]` (results `D`) and `:clone` (results `P`)
   ✅ done (2026-07-17) — the discard confirm gating was extracted into
   discardResultsEdits, now shared by the key and the `:` verb (force skips
   the staged confirm); `:clone` delegates to cloneRows with feedback for the
   no-op cases the key swallows.

   Tier 3 results verbs ✅ done (2026-07-17): `:follow`/`:back` (g d / g b FK
   nav), `:keep`/`:hide` (* / ! filter by cell), `:undo`/`:unfilter` (u / c
   filter stack), `:hidecolumn [col]`/`:showcolumns` (H / g H),
   `:copyinsert` (Y — copyRowsAsInsert extracted), `:regex <pattern>` (g/ —
   applySearch extracted, patterns may contain spaces). All delegate to the
   shared helpers; `:copyinsert` and `:regex` required extracting the inlined
   key logic (copyRowsAsInsert, applySearch) so the key and `:` stay in sync.

Original historical order (all complete): keyring storage (#1),
indexes/triggers/views (#4), read-only mode (#3).
Shipped earlier on this track: transactions (#5), export (#6 / `:export`),
file integration (#14), monitoring (`:watch`/`:tail`/`:refs`/`:uses`),
Wave A–D ex-command expansion, Wave E DDL (`:truncate`/`:drop`/`:rename`/`:create`).
