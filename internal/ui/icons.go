package ui

import "strings"

// iconSet is the active pair of expand/collapse glyphs used by every tree-like
// renderer (sidebar schema tree, connection groups, relationship explorer).
// It is a package-level var set by applyIcons — the same "set once, every
// renderer reads on the next View()" model as applyPalette — so a config
// change to the `icons` setting needs no per-component plumbing.
//
// Default is portable Unicode triangles (▾/▸), which render acceptably across
// ordinary terminal fonts. The "nerdfont" set uses Nerd Font's angle glyphs
// (U+F107 / U+F105): open, rotationally-symmetric chevrons that look like the
// treemacs expand/collapse indicators, but only when the terminal runs a Nerd
// Font. Both glyphs are cell-width 1 (verified via uniseg.StringWidth), so the
// existing alignment math (truncateCell/padRight) is unaffected.
type iconSet struct {
	collapsed string // right-pointing, expandable
	expanded  string // down-pointing, open
}

const iconSetNerdFont = "nerdfont"

var icons = iconSet{collapsed: "▸", expanded: "▾"}

// applyIcons sets the active glyph pair from the `icons` config value. Empty
// or unknown names fall back to the portable Unicode triangles.
func applyIcons(name string) {
	if name == iconSetNerdFont {
		icons = iconSet{collapsed: "\uf105", expanded: "\uf107"}
		return
	}
	icons = iconSet{collapsed: "▸", expanded: "▾"}
}

// resolveIconSet maps a user-facing icon-set name (as typed in :icons or the
// config `icons:` value) to the canonical string stored in Settings.Icons,
// and reports whether the name is recognized. Accepted: "unicode" or
// "default" (→ "", the portable triangles, which also omits the field from
// YAML so a reset round-trips cleanly) and "nerdfont". The match is
// case-insensitive and whitespace-trimmed; empty input resolves to default.
func resolveIconSet(name string) (string, bool) {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "", "unicode", "default":
		return "", true
	case iconSetNerdFont:
		return iconSetNerdFont, true
	}
	return "", false
}
