# Command registry: unifying `:` commands, keybindings, and the palette

## Original problem (resolved)

When this doc started, keybindings already had `registry()` feeding `?` and
`Ctrl+P`, but ex commands were a bare `switch` with no autocomplete or listing,
and the palette could not replay chords (`g d`, `dd`, …). Steps 1–5 below
closed that gap (ex registry + completion, sequence replay, `:q` semantics,
high-value aliases). What remains optional is a unified Action ID layer for
`:map` / config — see Step 6.

## Goal

One **Action layer** that keybindings, `:` commands, the palette, help, and
(later) `:map` all funnel through. Add commands in one place; everything else
picks them up. Shared helpers + `exCommands()` already deliver most of this;
stable Action IDs are still optional.

## Steps (each ships independently)

### Step 1 — Ex command registry (refactor, no behavior change) ✅ DONE
- Add `exCmdSpec` + `exCommands()` + `exLookup()` in `excmd_registry.go`.
- Slim `runExCommand` to: look up the verb, call its executor, else fall back
  to column-jump / `E492` (unchanged).
- Add `TestExRegistryConsistency` + `TestExLookupResolvesKnownVerbs`.
- All existing `excmd_test.go` tests keep passing unchanged.

### Step 2 — Make `:` discoverable ✅ DONE
- `:` autocomplete: prefix-filter `exCommands()` by the verb being typed;
  Tab completes to the top match. Popup (rounded border, bold top row — same
  conventions as the editor/palette popups) sits above the prompt; typing `:`
  alone lists every command.
- Folded a two-column **Commands** block into the `?` help sheet (verb usage +
  desc), driven from `exCommands()`. `:help`/`?` now document commands too.
- Did **not** add a `:commands` verb — it would be redundant with `:help` now
  that help lists commands, which is exactly the noise the design discussion
  warned against. (If you'd prefer a commands-only panel later, it's a small
  addition.)
- Tests: `excmd_completion_test.go`, `help_commands_test.go`.
- `:` autocomplete: filter `exCommands()` by verb prefix; complete on Tab.
- `:commands` listing; fold Ex verbs into the `?` help sheet (verb, usage, desc).
- Drift test: every spec appears in the help output.

### Step 3 — Palette reaches chords (sequence replay) ✅ DONE
The palette now replays chord/double-press bindings through the *existing*
dispatch via `tea.Sequence` (no parallel executors, no behaviour divergence):
- `palette.go`: `paletteItem.token` → `replay []string`; `Binding.replayTokens()`
  resolves an explicit sequence from `chordReplays`, else a single token, else nil.
- `keymsg.go`: `replayKeySequence` builds a `tea.Sequence` of synthesised keys so
  the stateful pending-G/pending-D flag set by key 1 is consumed by key 2.
- `chordReplays` (palette.go) lists the unambiguous single-action chords now
  reachable from Ctrl+P: `g d/b/r/R/f/s/e/E/H//X`, `g c`, `g x`, `g t`/`g T`,
  `g g`, `dd`, `y y`, `y r`, `==`. Former alternative-action lines were split
  into one-action entries (`g t` / `g T`, `g g` / `G`, `ctrl+e` / `\`) so each
  is palette-reachable. Navigation clusters (`j/k`, `ctrl+h/j/k/l`, …) stay
  combined and non-executable from the palette.
- Confirming a panel-scoped row focuses that panel before replaying:
  Results / Tabs / Theme Picker → results (so g-chords get a pending-G flag),
  Sidebar (Tables) → table list, Global `\` → editor (`\` only runs from vim
  normal).
- Tests: `TestPaletteChordsExecutableViaSequence`,
  `TestPaletteSplitDualActionRowsExecutable`,
  `TestPaletteDualActionRowsFocusPanel`, `TestReplayKeySequence`,
  `TestChordReplaysAreRealBindings` (drift guard).

The full `Action` type (ID + keybinding + ex verbs + executor, merging the
keybinding and ex registries) was DEFERRED — sequence-replay already delivers
the reachability goal without a big refactor or behaviour-divergence risk.

### Step 4 — Semantic fixes ✅ DONE
- `:q`/`:q!` now closes the active tab, and quits the app when it is the last
  tab — vim-true (`:q` on the final window exits), resolving the `:q`/`q`/`g x`
  clash WITHOUT adding a `:bd`/`:tabclose` command. `:wq`/`:x` save then close
  the tab, or save-then-quit if last (sequenced via `tea.Sequence` so the write
  completes before exit).
- `:w` left as-is: the `:w` (save edits) / `:w <file>` (write buffer) overload
  mirrors vim's `:w`/`:w file` and is documented in help; splitting it would
  add commands without clear benefit. The `ctrl+s` overlap is intended (two
  interfaces, one action).
- Tests: `:q` last-tab now quits (was refused); added `:x` last-tab-quit.

### Step 5 — High-value aliases ✅ DONE
Added as single registry entries (autocomplete + help pick them up for free):
`:explain`, `:refresh`/`:reload`, `:reconnect`, `:history`, `:bookmarks`/`:bm`,
`:describe`/`:desc [table]`, `:stats [column]`, `:bar[!] [label] [value] [sum|count|avg]`, `:freq[!] [column]`, `:line[!] [x] [y]`, `:scatter[!] [x] [y]`, `:hist[!] [column] [bins]`, `:format`, `:theme <name>`, `:color [slot] [hex|default]`, `:colors`.

Each shares the SAME implementation as its keybinding, not a duplicate:
- `:refresh`/`:reload`, `:history`, `:bookmarks` call freshly-extracted
  `refreshSchema` / `toggleHistory` / `toggleBookmarks`, now also used by
  ctrl+r / ctrl+y / ctrl+g.
- `:explain`→`explainQuery`, `:stats`→`fetchColumnStats`, `:bar`→`exBar`, `:freq`→`exFreq`, `:line`→`exLine`, `:scatter`→`exScatter`, `:hist`→`exHist`,
  `:describe`→`openSchemaPanel`, `:format`→`formatSQL` (the editor `==` path).

`:connect <name>` shipped with Tier 5 Wave B. `:readonly` stays a
connection/CLI flag at engine level (not a runtime toggle). Tests:
`excmd_aliases_test.go`; known-verb set extended.

### Step 6 (optional) — `:map` / keymap config
Enabled once actions have stable IDs + executors.

### Catalog expansion (ongoing)
With the registry in place, the `:` line is the cheap, self-documenting home
for new commands. Tiers 1–3, Tier 4 DBA (`:who` / `:locks` / `:kill` /
`:diagnose`, …), and Tier 5 Waves A–E are complete — see `ROADMAP.md`
History #15. High-level key mirrors (`:refresh`, `:explain`, `:history`,
`:bookmarks`, `:import`, `:bookmark`) share helpers with their bindings —
that pattern is the **preferred** way to grow the set.

**Principle (2026-07-17):** overlap with shortcuts is intentional for
discoverability. Add a `:` verb for high-level actions even when a key
exists (`:run` ↔ `ctrl+e`); skip pure UI chrome (`:cursor-down`). Always
extract/share one helper. Living conventions: `ROADMAP.md` **Design
guidance**.

Live product backlog: `ROADMAP.md` **Open work**. Optional next *in this
doc*: Step 6 (`:map`) if Action IDs land.
