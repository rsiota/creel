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

### Step 3 — Action layer + palette-by-action
- `Action` type: ID, keybinding (display+tokens), optional ex verbs, executor.
- Palette calls executors by ID instead of replaying keys → chords become
  reachable. Key dispatch stays as-is initially; `:` and palette share executors.

### Step 4 — Semantic fixes
- `:q` = quit app (vim); `:bd`/`:tabclose` = close tab.
- Split the `:w` overload.
- Update tests + help.

### Step 5 — High-value aliases (now one table entry each)
`:describe`/`:desc`, `:explain`, `:stats`, `:connect`, `:theme`, `:refresh`,
`:readonly`, `:history`, `:bookmarks`, `:format`.

### Step 6 (optional) — `:map` / keymap config
Enabled once actions have stable IDs + executors.
