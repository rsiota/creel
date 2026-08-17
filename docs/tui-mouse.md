# TUI mouse handling notes

Reference for mouse-event plumbing in `creel`. Written to stop the same
round-trips recurring — most of it was learned the hard way while building
the ERD drag (2026-07-27) and is the prerequisite for the **hover tooltips**
item on the `ROADMAP.md` ERD section.

Everything here is against `github.com/charmbracelet/bubbletea` **v1.3.10**
(see `go.mod`).

---

## TL;DR — the one gotcha that costs a round-trip

bubbletea reports **left-button drag motion as `Type == MouseLeft` +
`Action == MouseActionMotion`**, *not* as `Type == MouseMotion`.

`Type == MouseMotion` is only set for **button-less** motion (hover), which
creel does not currently receive (see [Program options](#program-options)
below — it needs `WithMouseAllMotion`, which we do not enable).

So: if you route a drag by switching on `msg.Type`, every motion event
re-enters your `case tea.MouseLeft:` press handler and the drag never
starts. Route drag motion on `msg.Action` **first**, before the `msg.Type`
switch. The ERD handler does exactly this:

```go
// internal/ui/mouse.go — handleERDMouse
if msg.Action == tea.MouseActionMotion {
    // …promote a pending press to a drag, then move the card…
    return m, nil
}
switch msg.Type {
case tea.MouseWheelUp:   // …
case tea.MouseRelease:   // …
case tea.MouseLeft:      // press / click logic
}
```

The test that pins this behaviour is `TestERDDragViaMouseEvents`
(`erd_test.go`), and its comment restates the gotcha verbatim.

---

## The two-field model

A `tea.MouseMsg` carries the same event two ways:

| Field | What it is | Status |
|-------|-----------|--------|
| `Type` (`MouseEventType`) | Backward-compat enum: `MouseLeft`, `MouseRight`, `MouseWheelUp`, `MouseRelease`, `MouseMotion`, … | **Deprecated** by upstream; still the convenient thing to switch on for simple clicks/wheels |
| `Action` (`MouseAction`) + `Button` (`MouseButton`) | The modern pair | Preferred for anything involving motion |

`Action` is one of `MouseActionPress`, `MouseActionRelease`,
`MouseActionMotion`. `Button` is one of `MouseButtonLeft/Middle/Right`,
`MouseButtonWheelUp/Down/Left/Right`, `MouseButtonBackward/Forward`, or
`MouseButtonNone`. (X11 button codes under the hood — see bubbletea's
`parseMouseButton`.)

The deprecated `Type` is *derived* from `Action`+`Button` in
`parseMouseButton` (`bubbletea/mouse.go`), and the derivation has one
sharp edge that produces the gotcha above:

```go
case m.Action == MouseActionMotion:
    m.Type = MouseMotion
    switch m.Button {
    case MouseButtonLeft:   m.Type = MouseLeft   // overrides MouseMotion!
    case MouseButtonMiddle: m.Type = MouseMiddle
    case MouseButtonRight:  m.Type = MouseRight
    …
    }
```

i.e. **motion keeps the button's `Type`**; only button-less motion (the
default arm, `Button == MouseButtonNone`) is left as `MouseMotion`.

Modifiers land on `msg.Alt`, `msg.Ctrl`, `msg.Shift` (set from the X10/SGR
bit-flags). creel uses `msg.Shift` to turn the vertical wheel sideways in
the ERD (`mouse.go`).

---

## Program options

Mouse reporting is opt-in at program start. creel uses **cell motion**:

```go
// internal/ui/statusbar.go:372
p := tea.NewProgram(m, tea.WithAltScreen(), tea.WithMouseCellMotion())
```

| Option | Delivers | Doesn't deliver |
|--------|----------|-----------------|
| `WithMouseCellMotion()` ← **creel** | clicks, releases, wheel, and **motion only while a button is held** (drag) | button-less motion (hover) |
| `WithMouseAllMotion()` | everything cell-motion delivers **plus button-less motion** (hover) | nothing mouse-related |
| neither | nothing (mouse off) | — |

Upstream notes worth remembering:

- The two options are **mutually exclusive** — `WithMouseAllMotion` clears
  `withMouseCellMotion` and vice versa (`options.go`). "Many modern
  terminals support [all motion], but not all. … Cell motion mode is better
  supported than all motion mode." So flipping to all-motion is a real
  compatibility trade, not a freebie.
- Both try SGR-1006 extended mode first and fall back to X10. SGR gives
  accurate release events and coordinates > 223; X10 fakes releases
  (`Button == None`, `Action == Release`) and caps coords.
- "Wheel buttons don't have release events" — wheel motion is reported as a
  press; you never see a wheel release. (`parseSGRMouseEvent`.)

### Implication for hover tooltips (ROADMAP ERD item)

To show a tooltip on hover over an ERD card/column you need
**button-less motion**, which means flipping the program to
`WithMouseAllMotion()`. That is the only way to make `Type == MouseMotion`
events arrive at all. The cost is the terminal-support variance above;
treat it as a "best effort, degrade gracefully to no-hover" feature, and
do not make hover the *only* way to see the info (a `?`/key path must
exist). See [Forward-looking: hover tooltips](#forward-looking-hover-tooltips).

---

## Routing patterns in use

### Pattern A — `Type` switch (simple clicks + wheel)

For panels that only care about left-clicks and the wheel, switch on
`msg.Type`. This is every handler except the ERD's:

- `handleConnectionsMouse`, `handleDatabasePickerMouse`,
  `handleWorkspaceMouse` (sidebar / tab bar / editor / results),
  `handleSchemaEditorMouse`, `handleTableDesignerMouse`,
  `handleHelpMouse`.

They `return m, nil` for any `Type` they don't recognise, so the
drag-motion quirk is harmless there (they'd just no-op a motion event —
and under cell-motion, panels without a press in flight never even receive
one).

### Pattern B — `Action`-first (drag)

Use when you need to distinguish press / motion / release for the same
button (i.e. drag). The ERD is the only consumer today:

1. `Action == MouseActionMotion` → if a press is pending, promote it to a
   drag and `dragMove`; otherwise ignore.
2. `switch msg.Type` for wheel and press/click logic.

`MouseRelease` (`Type` value) closes the loop: `dragCommit` if a drag is
active, else run the deferred click (see [Drag state machine](#the-presdragrelease-state-machine)).

---

## The press→drag→release state machine

The ERD needs a click *and* a drag off the same gesture. The resolution is
the classic "defer the click to release" pattern:

```
press (MouseLeft, Action=Press)  → dragBeginPress(card)        [dragPending = card]
motion (Action=Motion)           → dragPromote → dragMove      [dragCard = card]
release (MouseRelease)           → if dragCard: dragCommit      [click never fires]
                                    else:       runERDCardClick [single/double click]
```

State lives on `ERDPanel` as `dragPending` (a press that hasn't moved yet)
and `dragCard` (an active drag). Rules that fall out of this:

- **A press with no motion is still a click.** The click logic runs on
  *release* (`runERDCardClick`), never on press, so a drag never steals a
  click and vice-versa.
- **Drag cancels the click's side effects mid-gesture.** `dragPromote`
  clears the double-click window (`m.lastERDClickTime = time.Time{}`) so
  the next genuine click isn't seen as the second half of a double.
- **Esc cancels an in-flight drag** without committing.
- **Headers aren't draggable.** A press on a header (cued by `◎`) drills
  in immediately on *press* — there's no press→release dance, because the
  header click and a body drag share the same card.

Anything that adds drag elsewhere (e.g. moving a results column, panel
splitters) should reuse this shape rather than inventing a fourth mouse
protocol. The editor↔results, sidebar↔centre, and centre↔right-slot
splitters (`handleSplitDrag` / `handleSidebarDrag` / `handleRightSlotDrag`)
are the second consumers. The ERD mini-map is the exception that proves the
rule: click and drag are the *same* action there (pan), so it pans on press
and on every motion instead of using pending/promote — see
[ERD mini-map](#erd-mini-map).

---

## Double-click detection

Done by hand against a time window (`doubleClickInterval = 500ms`,
`mouse.go`), since bubbletea gives no native double-click. The pattern:

```go
if !m.lastXClickTime.IsZero() &&
    time.Since(m.lastXClickTime) <= doubleClickInterval &&
    m.lastXClickCell == cell {           // same target
    m.lastXClickTime = time.Time{}       // consume so a triple isn't a double+single
    // …fire double-click action…
}
m.lastXClickTime = time.Now()
m.lastXClickCell = cell
```

Three call sites mirror it: results grid cell, inspector field, ERD card.
**Always clear the timestamp when firing** (or when a drag promotes, or on
an empty-space press) — otherwise the next click inherits a stale window.

---

## Wheel semantics

The wheel arrives as `Type == MouseWheelUp/Down/Left/Right`. Under cell
motion, vertical wheel always works; **horizontal wheel
(`MouseWheelLeft/Right`) is terminal-dependent** — some terminals emit it
natively, others don't. So for sideways pan the ERD also honours
**Shift+vertical-wheel** as a portable fallback:

```go
case tea.MouseWheelUp:
    if msg.Shift { m.erdPanel = m.erdPanel.Wheel(0, -1) } // horizontal
    else         { m.erdPanel = m.erdPanel.Wheel(-1, 0) } // vertical
```

Wheel has no release event (see [Program options](#program-options)); don't
write a handler that waits for one.

---

## Coordinate translation

Mouse coordinates (`msg.X`, `msg.Y`) are **screen-relative, 0-based**.
Most panels sit at a non-origin offset (sidebar width 30, borders, tab
bar, inspector slot…), so handlers translate to a content-relative grid
before hit-testing. Two conventions:

- **Arithmetic offset** — used by the workspace/inspector/sidebar/results:
  e.g. `dataRow := msg.Y - headerY - 2`, `relX := msg.X - sidebarWidth - 1`.
- **Helper method** — the ERD's `contentToCanvas` /
  `contentToCanvasUnbounded`, which fold in scroll + centring offsets.

### The "size the panel before hit-testing" trap

`View()` is a value-receiver, so the `Model` that receives a mouse event
in `Update`/`handle…Mouse` is **not** the copy that just rendered — its
panel sizes/column widths can be stale. The schema editor and table
designer hit this; both re-`SetSize` at the top of their mouse handler
*before* translating coordinates:

```go
// internal/ui/mouse.go — handleSchemaEditorMouse / handleTableDesignerMouse
m.schemaEditor.SetSize(contentW, contentH)   // refresh colWidths/height
if x < 0 || x >= contentW || y < 0 || y >= contentH {
    return m, nil // outside the editor's content area
}
```

The ERD does the equivalent in `app.go` (`m.erdPanel.SetSize(...)`
immediately before `handleERDMouse`). **Any new mouse-enabled panel must
do the same**, or click mapping drifts after a resize.

---

## Hover tooltips (implemented 2026-07-30)

The ERD hover-tooltip item (ROADMAP ERD section) was the immediate consumer
of the notes above; it's now shipped. How each sketch step landed, and where
it deviated:

1. **Enable button-less motion.** ✅ Flipped `statusbar.go`'s `tea.NewProgram`
   to `WithMouseAllMotion()`. App-wide (not ERD-scoped) as warned, but safe:
   every other mouse handler routes on `msg.Type` with no `default` arm and
   early-returns on anything that isn't `MouseLeft`/a wheel, so the new
   `MouseMotion` events no-op everywhere except the ERD. **Caveat still open:**
   terminal support for all-motion varies; treat hover as best-effort. If a
   terminal floods/breaks on it, this is the line to revisit (a per-feature
   enable would need an upstream `EnableMouseAllMotion` cmd toggle).
2. **Throttle.** ✅ Done exactly as sketched — `handleERDMouseHover` only
   writes `hoverCard` when the card under the cursor changes, so an unchanged
   hover produces a byte-identical model and the diff renderer skips. No
   time-based debounce turned out to be needed; identity-based mutation is
   enough.
3. **Route motion before everything else.** ✅ The `Action`-first block in
   `handleERDMouse` now splits `MouseActionMotion` on `Type`: `MouseMotion`
   (button-less) → hover; `MouseLeft`/… (held) → drag. The test that pins this
   is `TestERDHoverViaMouseEvents` (hover counterpart of
   `TestERDDragViaMouseEvents`).
4. **Don't steal the press/click/release flow.** ✅ `hoverCard` is purely
   presentational — `View` reads it; nothing in the drag/click state machine
   touches it except to clear it.
5. **Clear path.** ✅ `hoverCard` is cleared on any viewport-changing input:
   `Update` (every key — focus move/fold/scroll/mermaid toggle), `Wheel`, and
   `dragPromote`. The rule is "hover yields to any interaction and is
   re-established on the next mouse motion," which avoids per-handler clearing
   sprawl. `Show`/`Hide` reset it too.

**What it shows — only non-redundant info.** The card already paints each
 column's marker/name/type, so the tooltip deliberately never repeats those;
 it surfaces what the card *doesn't*:
  - **Expanded card** → its FK references only (`◇ col → refTable.refColumn`),
    the one detail absent from the card (you'd otherwise trace the arrow).
    Suppressed entirely when the card has no FKs — there's nothing to add.
  - **Collapsed card** (`zc`) → a full column reveal (the fold hides them),
    with each FK column annotated with its target.
 FK targets come from `layout.arrows` (the drawn connections), so they match
 the diagram. Overlaid beside the card (right side preferred, flips left /
 clamps to stay on-screen) via the existing `placeOverlay` helper.

**Known limitation (data model):** the sketch mentioned "comments," but
`db.Column` carries only `Name` + `Type` — there is no comment/nullable/default
field anywhere in the schema extraction (`TableColumnInfo` has NotNull/Default
but the ERD cards only load `[]db.Column`). Showing those would need a
per-table async `TableColumnInfo` fetch; deferred — the type + PK/FK markers
are the available detail today.

---

## ERD mini-map (implemented 2026-08-17)

A bottom-right overlay when the graph is larger than the viewport. Cards
paint as `█` blocks; a box-drawing rectangle traces the current view.
Click or drag to pan.

**Why it doesn't use the press→promote→click machine.** That protocol exists
because a card press has two meanings (click = highlight, drag = move) and
the click must not fire until release proves there was no motion. On the
mini-map, click and drag are the *same* action — pan the viewport — so pan
happens on press and on every subsequent motion. `minimapDrag` is just a
"button is down on the map" flag, cleared on release. Hit-testing the overlay
*before* `contentToCanvas` is the other essential: without it, a click on
the map would land on whatever card sits underneath in canvas space.

Hover over the map clears `hoverCard` so a tooltip never paints for a card
the overlay is hiding. An in-flight *card* drag keeps going if the cursor
crosses the map (card drag is checked first on button-held motion).

Tests: `erd_minimap_test.go`.

---

## Terminal compatibility notes (known variance)

- **SGR vs X10.** SGR (1006) gives reliable releases + large coordinates;
  X10 synthesises releases and caps at row/col 223. bubbletea auto-negotiates;
  creel code never reads which mode is active, and shouldn't start.
- **Release events are unreliable on X10 / some terminals.** This is *why*
  the ERD click logic also runs from the press path as a fallback shape —
  don't make a mouse feature depend solely on receiving a clean release.
- **Horizontal wheel** (`MouseWheelLeft/Right`) is patchy; always provide
  a Shift+wheel fallback (see [Wheel semantics](#wheel-semantics)).
- **All-motion (hover)** is the least portable of the lot — see
  [Program options](#program-options).
- **macOS Terminal.app** historically lags iTerm2/WezTerm/Kitty/Alacritty
  on mouse extensions; if a bug report is "mouse does nothing", check the
  terminal before the code.

---

## Testing conventions

Construct `tea.MouseMsg` literals directly in tests; **set the `Action`
field when you mean a specific phase**, because the zero value
(`MouseActionPress`) is usually-but-not-always what you want:

```go
// press
tea.MouseMsg{Type: tea.MouseLeft, Action: tea.MouseActionPress, X: gx, Y: gy}
// motion (the gotcha — note Type is still MouseLeft)
tea.MouseMsg{Type: tea.MouseLeft, Action: tea.MouseActionMotion, X: mx, Y: my}
// release
tea.MouseMsg{Type: tea.MouseRelease, Action: tea.MouseActionRelease, X: mx, Y: my}
// wheel
tea.MouseMsg{Type: tea.MouseWheelDown, Shift: shift}
```

- For click-only flows (no drag), tests commonly omit `Action` and rely on
  the `Type` switch — fine, and matches how Pattern-A handlers reason.
- For drag flows, always send the full press→motion→release triple with
  explicit `Action`s; `TestERDDragViaMouseEvents` is the template and its
  comment is the canonical statement of the gotcha.
- Handlers are methods on the value-receiver `Model`, so tests chain
  `mm, _ := m.handle…Mouse(msg); m = mm.(Model)`. Remember the returned
  model is a *copy* — assert against the returned one, not the input.

---

## Checklist: adding a new mouse feature

- [ ] Does it need motion (drag or hover)? If yes, pick
      `WithMouseCellMotion` (drag only) vs `WithMouseAllMotion` (+hover),
      and accept the compatibility cost of the latter.
- [ ] Route on `msg.Action` *before* `msg.Type` if drag/hover is involved.
- [ ] If drag: reuse the press→promote→commit/click state machine; never
      fire the click on press.
- [ ] Translate `msg.X/Y` to content-relative coords; `SetSize` the panel
      first if its size is computed in `View()`.
- [ ] Don't gate behaviour on a wheel release (it never fires).
- [ ] Provide a Shift+wheel fallback for horizontal pan.
- [ ] Provide a keyboard equivalent (SSH/no-mouse contexts) — see the
      ROADMAP ERD "keyboard equivalent of drag" follow-up for the model.
- [ ] In tests, set `Action` explicitly on motion/release events.
