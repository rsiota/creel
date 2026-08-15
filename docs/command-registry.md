# Command registry: unifying `:` commands, keybindings, and the palette

## Problem

Today there are two parallel, un-unified surfaces:

- **Keybindings** have `registry()` (`internal/ui/registry.go`) as a single
  source of truth. Both the `?` help overlay (`help.go`) and the `Ctrl+P`
  palette (`palette.go`) consume it. The palette "replays" a binding by
  synthesizing its key (`keymsg.go`).
- **Ex commands** (`:`) are a bare `switch verb` inside `runExCommand`
  (`excmd.go`). There is **no registry, no autocomplete, and no listing**.
  `:help` just opens the keybinding sheet, which doesn't enumerate the verbs.

So the most discoverable surface (`:`) is the least self-documenting one. The
palette is also hobbled: `execToken()` returns `""` for any multi-token
binding, so chord actions (`g d`, `g e`, `g s`, `g X`, `dd`, …) can't be
reached from `Ctrl+P` at all.

There is also some tangled duplication to resolve before adding more:
- `:q` closes a *tab*, but `q`/`ctrl+q` quits the *app* and `g x` also closes a
  tab — three "quit-ish" things, and `:q` breaks vim expectations.
- `:w` is overloaded (save edits with no arg; write buffer to file with arg)
  and overlaps `ctrl+s`.

## Goal

One **Action layer** that keybindings, `:` commands, the palette, help, and
(later) `:map` all funnel through. Add commands in one place; everything else
picks them up.

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
  reachable from Ctrl+P: `g d/b/f/s/e/H//X`, `g c`, `g x`, `dd`, `y y`, `==`.
  Alternative-action lines (`g t / g T`, `g g / G`, `ctrl+e / \`) stay
  non-executable until split into one-action entries.
- Tests: `TestPaletteChordsExecutableViaSequence`, `TestReplayKeySequence`,
  `TestChordReplaysAreRealBindings` (drift guard).

The full `Action` type (ID + keybinding + ex verbs + executor, merging the
keybinding and ex registries) was DEFERRED — sequence-replay already delivers
the reachability goal without a big refactor or behaviour-divergence risk.
Known limitation (pre-existing, shared with single-key palette actions): a
results-context chord only fires when results is focused, since replay drives
the panel-specific dispatch.

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
`:explain`, `:refresh`/`:reload`, `:history`, `:bookmarks`/`:bm`,
`:describe`/`:desc [table]`, `:stats [column]`, `:bar[!] [label] [value] [sum|count|avg]`, `:freq[!] [column]`, `:line[!] [x] [y]`, `:hist[!] [column] [bins]`, `:format`, `:theme <name>`.

Each shares the SAME implementation as its keybinding, not a duplicate:
- `:refresh`/`:reload`, `:history`, `:bookmarks` call freshly-extracted
  `refreshSchema` / `toggleHistory` / `toggleBookmarks`, now also used by
  ctrl+r / ctrl+y / ctrl+g.
- `:explain`→`explainQuery`, `:stats`→`fetchColumnStats`, `:bar`→`exBar`, `:freq`→`exFreq`, `:line`→`exLine`, `:hist`→`exHist`,
  `:describe`→`openSchemaPanel`, `:format`→`formatSQL` (the editor `==` path).

Deferred from this step (now scheduled in `ROADMAP.md` #15 Tier 5):
`:connect <name>` (Wave B — async + keyring), and `:readonly` (connection/CLI
flag at engine level, not a runtime toggle). Tests: `excmd_aliases_test.go`;
known-verb set extended.

### Step 6 (optional) — `:map` / keymap config
Enabled once actions have stable IDs + executors.

### Catalog expansion (ongoing)
With the registry in place, the `:` line is the cheap, self-documenting home
for new commands. Tiers 1–3 from `ROADMAP.md` #15 are complete (parameterized
file/txn/export verbs, monitoring `:watch`/`:tail`/`:refs`/`:uses`, and small
wins like `:count`/`:peek`/`:rerun`/`:limit`). High-level key mirrors already
in the catalog (`:refresh`, `:explain`, `:history`, `:bookmarks`, `:import`,
`:bookmark`) share helpers with their bindings — that pattern is now the
**preferred** way to grow the set, not something to avoid.

**Principle (2026-07-17):** overlap with shortcuts is intentional for
discoverability. Add a `:` verb for high-level actions even when a key
exists (`:run` ↔ `ctrl+e`); skip pure UI chrome (`:cursor-down`). Always
extract/share one helper. Full priority list: **Tier 5** in `ROADMAP.md` #15
(Wave A mirrors → Wave B connect/nav → Wave C schema → Wave D QoL).

Next implementation target: session restore (#9) or Tier 4 DBA verbs if
requested. Optional: `:createdb`/`:dropdb`. Tier 5 Waves A–E ✅.
