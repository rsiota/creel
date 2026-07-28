# gsql — Roadmap

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
mouse drag (2026-07-27), and fit-to-screen `z` (2026-07-27) are all done. What
remains:

- **Collapse/expand cards** (`zc`/`zo` vim-style) — fold a card to header-only
  to declutter dense schemas; pairs well with drag. **Caveat:** bare `z` is now
  fit-to-screen, so collapse needs a non-conflicting chord (or `z` becomes a
  prefix with `zz`/`zc`/`zo`). Decide the key scheme before implementing.
  Medium effort; the router must handle arrow endpoints on a header-only card
  (`colRowY` currently assumes column rows exist).
- **Hover tooltips** (mouse-motion) — full column type/comments on hover.
  Medium; needs `WithMouseAllMotion` (button-less motion) and throttling. See
  the TUI-mouse-notes tech-debt item — it hits the same event-mapping area.
- **Mini-map** — a tiny overview with a viewport rectangle for very large
  schemas. Medium-high effort.
- **Drag deferred follow-ups:** persisted positions across sessions (the
  in-memory MVP is intentional — wait for the ask); snap-to-grid on drop (free
  cell today); Level C bend-optimal routing (A* on the cell grid; the current
  Level B router always produces a visible, correct arrow but isn't
  bend-optimal in pathological layouts); a keyboard equivalent of drag for
  no-mouse/SSH contexts.

### `:` argument completion — wave 2 (#10 v2)
Wave 1 shipped (2026-07-28): each `exCmdSpec` gained an optional `complete`
closure, and `Model.recomputeExCompletion` drives the popup past the verb.
`completeTable` (15 table commands: `:goto`/`:describe`/`:columns`/`:indexes`/…),
`completeConnection`, `completeTheme`, and `completeEnum` (`:export`, `:icons`)
are wired; Tab completes the top match into the last token. Files:
`excmd_registry.go`, `excmd.go`. Tests: `excmd_completion_test.go`. Remaining:
- **Column names** (`:sort`, `:hidecolumn`, `:stats`, `:filter`) — needs the
  focused table's columns; a touch more plumbing.
- **File paths** (`:e`, `:w`, `:import`, `:open`, `:save`) — reuse the engine in
  `import_prompt.go` rather than reimplement.
- **`up`/`down` popup selection** — currently only Tab→top match; up/down still
  recall history, so this needs a popup-visible guard.
- **Fuzzy (vs strict prefix) verb matching** — bundled freebie deferred to avoid
  changing the "g→goto only" behaviour the verb tests assert.

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
- **TUI mouse notes doc.** bubbletea reports left-button drag motion as
  `Type=MouseLeft` + `Action=MouseActionMotion` (not `Type=MouseMotion` — that
  is button-less hover needing `WithMouseAllMotion`). This cost a round-trip
  during drag development; document it (and terminal mouse-support variance)
  before hover-tooltips re-enter the same area.

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
7. **App-level settings** (2026-07-12) — `Settings` block + YAML `Duration`.
   Wired: `page_size`, `query_timeout`, `default_driver`. Follow-ups done:
   `confirm_destructive` (2026-07-14), `theme` (2026-07-13, 5 hand-tuned +
   ~565 auto-derived via `cmd/genthemes`, live-preview picker), `query_timeout
   off` sentinel (2026-07-24). Files: `config/settings.go`, `config/config.go`,
   `app.go`, `connection_form.go`, `styles.go`, `themes.go`, `theme_picker.go`.
8. **Configurable query timeout** — superseded by #7.
9. **Session restore** (2026-07-23) — new `internal/session` package persists
   `State{Tabs, Active}` as JSON keyed by (connection, database); restores the
   editor buffer + last query (not results) on reconnect. `gsql -f` takes
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
    `gsql -f` startup flag; shared `expandTilde`/`loadStartupFile`. Files:
    `excmd.go`, `import_prompt.go`, `cmd/gsql/main.go`, `statusbar.go`. Tests:
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
