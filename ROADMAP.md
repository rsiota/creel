# creel — Roadmap

Sourced from a code review on 2026-07-10. Nearly all of it has shipped; this
file is now split into **Open work** (the live items, kept short so it stays a
planning doc) and **History** (completed items, condensed to what + files +
tests). Full design rationale for any completed item lives in its commit and
the code comments it references.

Priorities here are suggestions; items are independent unless noted. Each item
lists the files most likely to change.

---

## Open work

### ERD (`g R` / `:erd`) — drag + fit-to-screen shipped; interactivity remains

The static ERD, its interactive tier (focus/highlight/path/search), free-form
mouse drag (2026-07-27), fit-to-screen `zz` (2026-07-27), and card
collapse/expand `zc`/`zo`/`za` (2026-07-29) are all done. What remains:

- **Hover tooltips** ✅ DONE (2026-07-30) — hovering an ERD card overlays a
  tooltip showing ONLY info the card itself doesn't paint, so it never reads
  as redundant: an **expanded** card lists its FK references
  (`◇ col → refTable.refColumn`) — the one detail you'd otherwise trace an
  arrow for — suppressed entirely when the card has no FKs; a **collapsed**
  card (`zc`) reveals its hidden columns, each FK annotated with its target.
  FK targets come from `layout.arrows` so they match the diagram. Required
  flipping the program to `WithMouseAllMotion` (button-less motion, app-wide —
  safe because every other mouse handler no-ops on `MouseMotion`); hover is
  throttled by card identity, routed on `Type` within the `MouseActionMotion`
  block, and cleared on any key/wheel/drag. See `docs/tui-mouse.md` (Hover
  tooltips section). Note: `db.Column` carries only name + type, so
  nullability/default/comments aren't shown — that needs a per-table
  `TableColumnInfo` fetch and is deferred.
- **Mini-map** ✅ DONE (2026-08-17) — a tiny overview with a viewport rectangle
  for very large schemas. Auto-shown in the bottom-right when the diagram is
  larger than the viewport (hidden in Mermaid, or when the terminal is too
  small to host it). Cards paint as filled blocks (focused = accent, selected
  / path = primary); a box traces the current view. Click or drag the map to
  pan — click and drag are the same action here, so pan happens on press and
  on every motion (no pending/promote step; see `docs/tui-mouse.md`). Files:
  `erd_minimap.go`, `erd_panel.go`, `mouse.go`. Tests: `erd_minimap_test.go`.
- **Drag deferred follow-ups:** persisted positions ✅ DONE (2026-08-17) —
  per-connection session snapshot, scoped by whole-schema (`*`) vs
  neighbourhood (focused table) so a drag in one view cannot land on the
  other; snap-to-grid on drop (free cell today); Level C bend-optimal
  routing (A* on the cell grid; the current Level B router always produces
  a visible, correct arrow but isn't bend-optimal in pathological layouts).
  Keyboard nudge (`H`/`J`/`K`/`L`) shipped 2026-08-17.

### `:` argument completion — wave 2 (#10 v2)
Wave 1 shipped (2026-07-28): each `exCmdSpec` gained an optional `complete`
closure, and `Model.recomputeExCompletion` drives the popup past the verb.
`completeTable` (15 table commands: `:goto`/`:describe`/`:columns`/`:indexes`/…),
`completeConnection`, `completeTheme`, and `completeEnum` (`:export`, `:icons`)
are wired; Tab completes the top match into the last token. Wave 2 column
completion shipped (2026-07-29): `completeColumn` (`:sort`/`:hidecolumn`/
`:stats`/`:filter`) reads the results grid so it stays correct for custom
queries; `completePath` (`:e`/`:w`/`:import`/`:open`/`:save`) reuses the
import prompt's filesystem engine, returning full-path candidates so the
fuzzy ranker keeps them and Tab fills the whole path. Files:
`excmd_registry.go`, `excmd.go`, `import_prompt.go`. Tests:
`excmd_completion_test.go`. Remaining:
- **Fuzzy (vs strict prefix) verb matching** — bundled freebie deferred to avoid
  changing the "g→goto only" behaviour the verb tests assert.

`up`/`down` popup selection shipped (2026-07-30): when the popup is visible,
  up/down move its selection (wrapping, mirroring the command palette) and Tab
  completes the highlighted row; the popup window scrolls once the cursor
  reaches its edge. A `recalling` flag keeps a history walk going even when a
  recalled value would itself show a popup, and typing clears it — so
  `:`+`up` still replays the last command, vim-style, and `up`/`up`/`down`
  walks history uninterrupted. Files: `excmd.go`. Tests:
  `excmd_completion_test.go`.

### `cursor_style` setting (#7 follow-up)
Reserved field in `Settings`; needs results-cursor rendering work. Parked
until someone wants configurable cursor shapes.

### `:set <opt>` (#15)
Deferred — the runtime toggles are already covered by `:timing`/`:limit`/
`:theme`; `:set` would mainly add `confirm_destructive`/`timeout` mirrors.

### Tier 4 — DBA / niche (#15)
`:who`, `:locks`, `:kill <pid>` — session/lock inspection. Driver-specific;
`:kill` is dangerous (`confirm_destructive`). **Demand-gated:** build only if
users ask; don't speculatively.

### Tech debt / docs
- **Unify ERD routing.** Two routing systems now coexist: the legacy three-mode
  router (side elbow / over-top lane) for the initial ranked layout, and the
  dynamic polyline router (`routeArrow`/`rerouteArrows`) for post-drag. Not
  urgent (the legacy path is proven and untouched), but consolidate onto the
  dynamic router next time the router is touched, so there's one path.
- **TUI mouse notes doc.** ✅ DONE (2026-07-30) — `docs/tui-mouse.md`
  captures the bubbletea mouse model end to end: the `Type` (deprecated) vs
  `Action`+`Button` two-field model; the drag-motion gotcha
  (`Type=MouseLeft`+`Action=MouseActionMotion`, **not** `Type=MouseMotion` —
  that's button-less hover needing `WithMouseAllMotion`); `WithMouseCellMotion`
  vs `WithMouseAllMotion` + terminal variance; the Action-first routing pattern,
  the press→drag→release state machine, double-click detection, wheel/Shift
  semantics, the size-the-panel-before-hit-testing trap, and a hover-tooltips
  implementation sketch for the next ERD item. See it before touching mouse code.

### Data-fidelity & UX gaps — 2026-08-04 review

Findings from a focused review of the core "browse and move data" loop. Four
siblings (NULL vs empty-string distinction, copy-row-to-clipboard, multi-line
cell viewer, CLI output format + connection reuse) shipped in this pass — see
the 2026-08-04 History entries; later follow-ups:

- **Binary / BLOB rendering** ✅ DONE (2026-08-12) — `<BLOB …>` placeholder,
  `Result.Blobs`, `:saveblob`, hex literals in dump/INSERT/clone. Demo:
  `demo/blob-demo.sql`. Also: TUI honors `-database` / `-c` (opens workspace
  instead of the connection picker).
- **Column-width memory** ✅ DONE (2026-08-12) — per-(connection,database,table)
  widths in session JSON; max-merge so short pages don't shrink columns.
  Cleared by `:session clear`.
- **Transaction isolation level** ✅ DONE — `:begin [serializable|repeatable
  read|read committed|read uncommitted]` (also `s`/`rr`/`rc`/`ru` and
  hyphenated forms). Status bar shows `TXN S` / `TXN RR` / …. Files:
  `isolation.go`, `db.go`, `excmd.go`, `excmd_registry.go`, `statusbar.go`.

### Graph UX — browse/edit via the FK graph (in progress)

Goal: the relationship explorer + ERD are how you *move through and edit*
relational data, not just inspect schema. Pitch: “browse like a folder tree of
related rows — edit without writing the JOIN.”

**Shipped — first slice:**
- Unified back from explorer: `u` / `backspace` / `g b` (same `queryStack`)
- **Insert related** — `A` on an **inbound** edge prefills the child FK and
  opens inspector insert against the child table **without navigating the
  grid** (explorer yields the right slot, then restores after save or cancel)
- **Open in a new tab** — `t` runs the node's drill query in a new results tab;
  `Enter` still re-roots the current tab
- Inbound edges with count `0` stay visible so the first related row is
  insertable
- Empty-state status lines truncated so a long `emptyMsg` cannot wrap and
  inflate the explorer past its slot (was clipping the three panels' top
  borders)

Files: `rel_explorer.go`, `editing.go`, `app.go`, `schema_ops.go`,
`inspector.go`, `excmd.go` (`loadRowEdges`), `registry.go`,
`graph_nav_test.go`.

**Next slices (suggested order):**
1. **Insert related without navigating away** — shipped: insert into the child
   table while the explorer root and grid stay on the parent.
2. **ERD as launcher** — shipped (2026-08-16): Enter / double-click a card
   → `SELECT *` and close the overlay; `f` (and the ◎ header click) keep
   neighbourhood drill. **Generate JOIN from shortest path** — shipped: trace
   with `p`, then `i` drops JOIN SQL into the editor.
3. **Cross-highlight** — shipped (2026-08-16): grid FK cell ↔ explorer
   outbound edge; opening the ERD traces the FK path from the current table to
   the referenced one. ERD is a full-screen overlay, so grid+ERD cannot share
   a frame — highlight is applied at open.

**Explicitly defer:** editing *through* ERD arrows as a general UPDATE engine;
force-directed layouts; omniscient DB “git blame.”

### Product review — 2026-08-16

Suggestions from a pass over the shipped surface: polish the daily-driver
loop, then deepen the graph+charts identity rather than growing another REPL.
The three **Now** items are the first slice; everything else stays demand-gated
or sequenced behind them.

**Now (shipping)** — all shipped; see History / later sections.
- **TLS / SSL + unix sockets** — shipped.
- **Editor undo + `/` + visual line** — shipped.
- **ERD as launcher + cross-highlight** — shipped.

**Next (connection / editing polish)** — all shipped.
- **Reconnect / keep-alive**, **transaction isolation**, **jump to syntax
  error**, **`y r` / `:copyrow` keybinding**.

**Graph / charts**
- **ERD mini-map**, **persist ERD drag positions**, **keyboard nudge**,
  **`:pie`**, **JOIN from ERD path** — shipped.
- **ERD tooltip nullability/defaults** — needs `TableColumnInfo` on hover
  (deferred from the 2026-07-30 tooltip work).
- **Export a chart** as a Unicode snapshot or SVG.
- **`:watch` + chart** — redraw the last chart on refresh; highlight changed
  rows on `:watch` / `:tail`.

**AI**
- “Explain this query” / “why is this slow” with the last `EXPLAIN` attached.
- **“Fix this error”** — shipped (`:aifix` / `:fixsql`).
- Restrict schema context to the focused table + FK neighbourhood — shipped.
- Optional: run generated SQL in a scratch tab (read-only) and iterate on the
  error. Keep `ctrl+e` as the default — do not auto-run DDL.

**Demand-gated / skip**
- `:who` / `:locks` / `:kill` — already demand-gated.
- Macros, `:shell`, a second favorites system — already rejected.
- More themes (~570 is enough).
- SQL Server / Turso just to match sqlit’s comparison table.

**Docs / consistency**
- Fuzzy verb matching for `:` (`:g<tab>` → `goto`) — last wave-2 leftover.
- `cursor_style` is reserved but unused; ship block/underline or drop the
  field.

### Product review — 2026-08-20

Prioritized next slices after another pass: strengthen “open → browse →
analyze → reuse” without diluting the vim + graph identity. Ship one slice
at a time so each can be checked before the next starts.

**Now (shipping) — slice 1** ✅ DONE (2026-08-20)
- **Recent connections + first-run demo** — persist an MRU of connection names
  (`internal/recent`, `~/.config/creel/recent.json`); reopen the picker with
  the last-used connection selected and a muted `recent` badge; when the list
  is empty, offer a selectable **Try the demo database** row (`demo.ResolvePath`
  — cwd `demo/creel-demo.db` if present, else materialize the embedded schema
  under the config dir). Files: `demo/embed.go`, `internal/recent/`,
  `connection_list.go`, `connection_ops.go`, `app.go`. Tests:
  `recent_test.go`, `embed_test.go`, `recent_demo_test.go`,
  `connection_list_test.go`.

**Next (one at a time)**
1. **Query parameters** — `:param start 2026-01-01` then
   `WHERE created_at > :start` (bookmarks become reusable workflows).
2. **`:watch` + chart** — redraw the last chart on refresh; highlight deltas
   on `:watch` / `:tail`.
3. **CLI stdin + non-zero exit** — `cat q.sql | creel -c prod -e -`; fail the
   process on query error.

**Then (daily-driver / identity)**
- Color PK / FK columns in the grid (the `→` follow cue already exists).
- Jump-anywhere palette — broaden Ctrl+P to tables, bookmarks, history, themes.
- Chart export (Unicode snapshot or SVG).
- AI “explain this query / why slow” with the last `EXPLAIN` attached.
- Optional AI scratch-tab dry-run (still no auto-run DDL).

**Fits the product (later)**
- DuckDB as a fourth driver (charts + CSV/Parquet; static-binary friendly).
- JSON/JSONB foldable tree in the inspector (`E` already pretty-prints).
- Result-set diff of two tabs (not schema-diff — that stays rejected).
- ERD tooltip nullability/defaults (`TableColumnInfo`).
- Unify the two ERD routing systems onto the dynamic polyline router.
- Discoverability content (short asciinema of `g r` → insert-related → ERD
  path → `i` JOIN) and broader packaging (scoop / nix / AUR).

**Still skip**
- Macros, `:shell`, second favorites, more themes, SQL Server/Turso for the
  comparison table, editing *through* ERD arrows as a general UPDATE engine.

---

## Design guidance (living)

Principles for growing the `:` command set, distilled from a command survey
(2026-07-17) and refined toward Helix-style discoverability. Still governs new
commands.

**Guiding principle:** Shortcuts are for muscle memory; `:` commands are for
discoverability, arguments, and (later) scripting. Overlap is *intentional*
for high-level noun/verb actions — `:run` ↔ `ctrl+e`, `:refresh` ↔ `ctrl+r` —
as long as both call the **same helper**. The three surfaces (keys, palette,
`:`) stay usefully distinct in *UX* but funnel through one implementation.

Add an ex command when at least one is true:
1. It takes an argument (`:goto users`, `:limit 100`, `:theme dark`)
2. It is stateful / mode-like (`:watch`, `:begin`, `:timing`)
3. It is a high-level verb users will search for (`:run`, `:describe`, `:connect`)
4. Default-object form helps (`:peek`, `:refs`, `:indexes` with no args)

Skip commands that are only interface chrome (no sensible noun/verb):
`:cursor-down`, `:focus-results`, `:scroll-page`, `:toggle-bottom-pane`.

**Architecture habit:** extract a helper first, then wire keybinding + `:` (+
palette). Never copy a key handler body into an ex executor. A full unified
`Action` registry (IDs for `:map` / config) remains optional — shared helpers +
`exCommands()` are enough until then. See `docs/command-registry.md`.

**Conventions:**
- **Default to the current object.** No-argument commands operate on the
  focused table (`currentTable()`): `:count`, `:sample`, `:refs`, `:peek`,
  `:indexes`. Nearly free; biggest lever for feeling native.
- **Explicit verbs over cryptic prefixes.** `:goto` is canonical; `:gt` is a
  tolerated alias. Do not grow a `:gv`/`:gf`/`:gp` family. New verbs are spelled
  out; short aliases land only after the feature ships.
- **Stateful commands own a status-bar indicator** (`TXN ●`, `WATCH 5s`, …).
- **Help stays scannable.** Prefer category grouping in `?` / `:help`.

**Explicitly rejected (wrong layer / scope creep):**
- UI chrome (`:cursor-*`, `:focus-*`, `:scroll-*`, `:toggle-*-pane`).
- Shell escape (`:shell` / `:cd` / `:ls`).
- Pager/tee (`:tee` / `:pager` — the TUI *is* the pager).
- Macros (`:record` / `:play` — revisit only with a real Action ID layer).
- Editor substitute (`:replace` — use vim `:s` in the editor).
- Schema-diff product (`:diff <a> <b>`).
- Favorites as a second bookmarks system (`:favorite`).

---

## History (completed)

### 🔴 High-value gaps

1. **Secret backend for passwords** (2026-07-10) — `internal/secrets/` + the
   connection form/save/connect/delete flows; `go-keyring`. Form exposes all
   secret fields incl. `ssh_passphrase` (editable since 2026-07-24); all three
   migrate to the keychain together when Secrets is `keychain`.
2. **"Test Connection" in the form** (2026-07-10) — `ctrl+t` opens the DB in a
   background goroutine without saving, surfacing the real error. Shared
   `connConfigToDB`. Files: `connection_form.go`, `connection_ops.go`, `app.go`,
   `hints.go`.
3. **Read-only / safe mode** (2026-07-10) — per-connection `readonly:` flag +
   global `--readonly`; shared `isWriteQuery`/`rejectWriteIfReadOnly` guard in
   `db.go`; engine opened read-only where supported; `READ-ONLY` badge.
4. **Indexes, triggers, views, constraints** (2026-07-10) — across all three
   drivers + read-only **Structure** view (`d`). Check constraints (2026-07-14,
   `CheckConstraints` + Checks tab). `view`/`table` sidebar badge (2026-07-24).
   SQLite partial-index predicate (2026-07-28) — `PRAGMA index_list` only
   reports a 0/1 flag, so the WHERE is parsed from `sqlite_master.sql`
   (`indexPartialPredicate` + `splitIndexWhere`, mirroring `ViewDefinition`);
   `Index.Partial` was already rendered by the Structure tab and PostgreSQL.

### 🟡 Medium-value improvements

5. **User-facing transactions** (2026-07-15) — `:begin`/`:commit`/`:rollback`
   + `TXN ●` indicator; editor statements route through the held tx. The
   v1/v2 fork collapsed (`Tx.Execute` was cheap). Cell edits/DDL blocked during
   tx; auto-rollback at lifecycle boundaries. Files: `db.go`, `app.go`,
   `excmd.go`, `editing.go`, `schema_ops.go`, `connection_ops.go`, `query.go`,
   `statusbar.go`. Tests: `transaction_test.go`.
6. **More export formats** (2026-07-11) — `g X` picker: CSV, JSON, JSONL,
   Markdown, TSV, SQL INSERT dump, sharing `serializeFormat`. Legacy `x` keeps
   CSV. Files: `export_import.go`, `format_picker.go`, `export_format_test.go`.
   Reworked (2026-07-29): `g X` is now an **export dialog** (format · columns ·
   scope) replacing the format-only picker. Column selection projects the
   export; **scope** adds *Whole table* (`SELECT cols FROM t`, no LIMIT — fixes
   the page-size cap), *Marked rows*, and *Current page*. `x` stays an instant
   current-page CSV; `:export <fmt> [cols...]` is the non-interactive path.
   Files: `export_overlay.go`, `export_format.go`, `export_import.go`,
   `excmd.go`, `app.go`. Tests: `export_overlay` (+`export_format_test.go`,
   `export_results_test.go`).
7. **App-level settings** (2026-07-12) — `Settings` block + YAML `Duration`.
   Wired: `page_size`, `query_timeout`, `default_driver`. Follow-ups done:
   `confirm_destructive` (2026-07-14), `theme` (2026-07-13, 5 hand-tuned +
   ~565 auto-derived via `cmd/genthemes`, live-preview picker), `query_timeout
   off` sentinel (2026-07-24). Files: `config/settings.go`, `config/config.go`,
   `app.go`, `connection_form.go`, `styles.go`, `themes.go`, `theme_picker.go`.
8. **Configurable query timeout** — superseded by #7.
9. **Session restore** (2026-07-23) — new `internal/session` package persists
   `State{Tabs, Active}` as JSON keyed by (connection, database); restores the
   editor buffer + last query (not results) on reconnect. `creel -f` takes
   precedence; all quit paths funnel through `beginQuit()`. `:session` manages
   it. Files: `internal/session/`, `session.go`, `connection_ops.go`, `app.go`,
   `excmd.go`, `excmd_registry.go`. Tests: `session_test.go`, `session_test.go`.

### 🟢 Polish

10. **Vim `:` ex-command mode** (2026-07-14) — modal `:` line (`excmd.go`),
    shell-like parser, `E492` for unknown, fallback column jump. v2 completion
    is open (above).
11. **ERD / relationship view:**
    - Interactive relationship explorer `g r` / `:explore` (2026-07-20) —
      expand-in-place FK tree (`rel_explorer.go`).
    - Static ERD `g R` / `:erd` (2026-07-24) — graphical cards-and-arrows +
      Mermaid, on a rune canvas. Files: `erd_graph.go`, `erd.go`,
      `erd_panel.go`, `app.go`, `excmd_registry.go`, `registry.go`.
    - Interactivity tier (2026-07-26) — keyboard focus (j/k/h/l spatial
      nearest-neighbour), Space highlight, Enter drill-in, `/` jump, `p`
      shortest-FK-path (BFS), two-pass dim/vivid render, mouse click/
      double-click/wheel.
    - Free-form mouse drag + Level B dynamic routing (2026-07-27) — click-and-
      drag a card; arrows re-route live around obstacles via a side-channel
      search with an over/under-lane fallback (`routeArrow`/`routeSide`/
      `routeLane`, `lanePacker`, `rerouteArrows`). Initial layout's three-mode
      routing left untouched. Files: `erd_graph.go`, `erd_panel.go`, `mouse.go`,
      `app.go`. Tests: `erd_test.go`.
    - Fit-to-screen `z` (2026-07-27) — scrolls the cards' bounding box to the
      viewport centre (`fitToScreen`).
    - Collapse/expand cards `zc`/`zo`/`za` + `zM`/`zR` (2026-07-29) — `z` is now a
      vim-style prefix: `zz` fits (was bare `z`), `zc`/`zo`/`za` collapse/
      expand/toggle the focused card (a sticky in-place shrink so fold composes
      with free-form drag), and `zM`/`zR` collapse/expand every card at once by
      re-running the ranked layout (`relayout`) — the columns contract to
      reclaim the space the folded bodies freed, arrows re-route, and the
      viewport reframes. `colRowY` returns the header centre for a folded card
      so the router copes with a header-only endpoint; `relayout` reconstructs
      the rank order + top-margin from the laid-out cards so a bulk fold is a
      clean re-organize (drag positions snap back to rank columns). Files:
      `erd_graph.go`, `erd_panel.go`, `app.go`, `registry.go`, `hints.go`.
      Tests: `erd_test.go`.
12. **Connection groups / folders** (2026-07-12) — optional `group` field;
    collapsible folder headers (▾/▸), flat when none grouped. Files:
    `config/config.go`, `connection_list.go`, `connection_form.go`, `app.go`,
    `mouse.go`. Tests: `connection_groups_test.go`.
13. **Per-query timing history** (2026-07-27) — query duration is persisted in
    each history `Entry` (`Elapsed time.Duration`, JSON-round-tripped) and shown
    per row in the history panel, coloured red (new padding-less `slowStyle`)
    when ≥ 1s. Press `s` to toggle between most-recent-first and slowest-first
    (stable sort, zero-elapsed legacy entries sink); the displayed rank stays
    the `:rerun` index either way. `history.Record` now takes the elapsed
    duration; `FormatElapsed` formats it. Files: `internal/history/history.go`,
    `history_panel.go`, `styles.go`, `registry.go`, `app.go`. Tests:
    `history_test.go`, `history_panel_test.go`.
14. **`.sql` file integration** (2026-07-17) — `:e`/`:edit`/`:w`/`:write` +
    `creel -f` startup flag; shared `expandTilde`/`loadStartupFile`. Files:
    `excmd.go`, `import_prompt.go`, `cmd/creel/main.go`, `statusbar.go`. Tests:
    `file_io_test.go`.
15. **`:` command-set** (2026-07-17) — Tiers 1–3 complete; Tier 5 Waves A–E
    done (`:run`, `:qa`, `:tab*`, `:copy`, `:connect`/`:c`, `:connections`,
    `:db`/`:use`, `:schema`, `:indexes`/`:columns`/`:constraints`/`:fk`,
    `:tables`/`:dt`/`:views`/`:dv`/`:schemas`, `:search`/`:find`, `:new`,
    `:version`, `:plan`, `:recent`, `:truncate`/`:drop`/`:rename`/`:create`,
    plus results verbs `:follow`/`:back`/`:keep`/`:hide`/`:undo`/`:unfilter`/
    `:hidecolumn`/`:showcolumns`/`:copyinsert`/`:regex`, and DDL
    `:createdb`/`:dropdb`/`:addcolumn`/`:discard`/`:clone`). All funnel through
    shared helpers (the architecture habit above). Tier 4 DBA remains open.

### 2026-08-04 — Data-fidelity review

16. **NULL vs empty-string distinction** (2026-08-04) — the in-memory cell
    model reserves the `"NULL"` sentinel for SQL NULL and scans genuine empty
    strings as `""`, but `sqlEscape` (the INSERT-literal renderer used by
    `:copyinsert` / `Y`) coerced `""` → NULL, silently corrupting real empty
    strings on copy. Fixed: `sqlEscape` now emits `''` for empty strings and
    `NULL` only for the sentinel; the results grid dims the NULL sentinel
    (`colorMuted`) so it reads as a marker, not data (matching the inspector).
    The parameterized inline-edit path was already correct (`"NULL"` → `nil`,
    `""` → empty string). Files: `results_table.go`. Tests: `null_copy_test.go`.
17. **Copy row(s) to clipboard** (2026-08-04) — `:copyrow [fmt]` copies the
    marked rows (or the cursor row when none are marked) to the clipboard as
    TSV (default) or `csv`/`md`/`json`/`jsonl`, reusing `serializeFormat`.
    Fills the gap between `:copy` (one cell) and `:copyinsert` (rows as SQL) —
    the common "paste this row into Sheets/Slack" case. Keybinding: `y r`
    (same helper). Files: `results_table.go`, `editing.go`, `excmd.go`,
    `excmd_registry.go`, `app.go`. Tests: `null_copy_test.go`,
    `excmd_results_test.go`, `copyrow_keybind_test.go`.
18. **Multi-line cell viewer** (2026-08-05) — the cell-expand popup (`E`,
    `CellEditPopup`) already covered long-truncated values and pretty-printed
    JSON, but it (a) was gated on editability so it no-op'd on read-only mode /
    custom queries / PK-less views, and (b) read the sanitized `rows` copy, so
    multi-line TEXT still rendered flat. Both fixed. `openCellEditPopup` now
    opens the popup **view-only** (`CellEditPopup` gained a `readOnly` mode:
    static render, no cursor, j/k/pgup/G scroll, ctrl+s is a no-op) whenever
    the results can't be written back, so `E` is always a usable peek — most
    useful in read-only mode, whose whole point is safe viewing. And
    `ResultsTable` now retains the un-sanitized `rawRows` (new `RawRowValue`),
    which seeds the popup (and the ctrl+s "did it change?" compare) so real
    newlines/tabs survive into the viewer. The grid display is untouched (it
    still reads the flattened copy for its single-line layout). Files:
    `cell_edit_popup.go`, `results_table.go`, `schema_ops.go`, `app.go`,
    `registry.go`. Tests: `cell_edit_popup_test.go`, `cell_edit_test.go`.
19. **CLI output format + connection reuse** (2026-08-06) — the headless path
    (`cmd/creel/main.go`) gained a `-format csv|json|jsonl|md|tsv` flag
    (default `tsv`; was hard-coded loose TSV) and `-c <name>` to target a saved
    connection, resolving its password / SSH passphrase from the keychain the
    same way the connection form does on connect — so connections saved in the
    TUI (incl. SSH-tunneled) work headlessly. `-c` **composes** with the flat
    `-driver`/`-database`/… flags: any explicitly-set one (detected via
    `flag.Visit`) overrides the matching field of the saved connection, so e.g.
    `-c localhost -database x` fills a different database instead of discarding
    the flag; `-c` also combines with `-readonly`. Output goes
    to stdout (pipes cleanly); the row-count summary stays on stderr. To reuse
    the existing (unexported) serializer + connection resolver without
    rippling through ~70 UI call sites, two thin exports were added on `ui`:
    `Serialize(format, cols, rows)` (wraps `serializeFormat`) and
    `ResolveConnection(cfg, name, forceReadOnly)` (wraps `connConfigToDB` +
    `resolveConnSecrets`). Files: `cmd/creel/main.go`, `internal/ui/cli.go`.
    Tests: `internal/ui/cli_test.go`, `cmd/creel/main_test.go`. Docs:
    `docs/cli.md`.
