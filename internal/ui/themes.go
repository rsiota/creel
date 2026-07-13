package ui

import (
	"sort"

	"github.com/charmbracelet/lipgloss"
)

// defaultThemeName is the theme used when settings.Theme is empty or
// unrecognized. It matches the key for defaultPalette in the themes map.
const defaultThemeName = "tokyo-night"

// gruvboxPalette is a dark Gruvbox¹ mapping onto the app's semantic color
// slots. Accent hues follow Gruvbox's bright variants; backgrounds use bg0/bg1
// for the base, cursor row, and status bar.
//
// ¹ https://github.com/morhetz/gruvbox
var gruvboxPalette = colorPalette{
	primary:         lipgloss.Color("#83a598"), // blue
	accent:          lipgloss.Color("#d3869b"), // purple
	success:         lipgloss.Color("#b8bb26"), // green
	mark:            lipgloss.Color("#8ec07c"), // aqua
	search:          lipgloss.Color("#504945"), // bg2 — search-highlight bg
	visual:          lipgloss.Color("#504945"), // bg2 — visual-selection bg
	cursorRow:       lipgloss.Color("#3c3836"), // bg1
	edit:            lipgloss.Color("#fe8019"), // orange
	warn:            lipgloss.Color("#fabd2f"), // yellow
	err:             lipgloss.Color("#fb4934"), // red
	muted:           lipgloss.Color("#928374"), // neutral gray
	label:           lipgloss.Color("#a89984"), // fg4 — secondary text
	border:          lipgloss.Color("#504945"), // bg2
	borderUnfocused: lipgloss.Color("#7c7164"), // midpoint of border and label
	bg:              lipgloss.Color("#282828"), // bg0
	stripe:          lipgloss.Color("#32302f"), // midpoint of bg and cursorRow
	fg:              lipgloss.Color("#ebdbb2"), // light1
	highlight:       lipgloss.Color("#3c3836"), // bg1
	statusBarBg:     lipgloss.Color("#3c3836"), // bg1
}

// nordPalette is a Nord¹ mapping onto the app's semantic color slots. Nord's
// cool blues and frosted greens map onto primary/mark; the polar-night range
// (nord0–nord3) supplies backgrounds and borders.
//
// ¹ https://www.nordtheme.com
var nordPalette = colorPalette{
	primary:         lipgloss.Color("#81a1c1"), // nord9 — blue
	accent:          lipgloss.Color("#b48ead"), // nord15 — purple
	success:         lipgloss.Color("#a3be8c"), // nord14 — green
	mark:            lipgloss.Color("#88c0d0"), // nord8 — cyan
	search:          lipgloss.Color("#434c5e"), // nord2 — search-highlight bg
	visual:          lipgloss.Color("#434c5e"), // nord2 — visual-selection bg
	cursorRow:       lipgloss.Color("#3b4252"), // nord1
	edit:            lipgloss.Color("#d08770"), // nord12 — orange
	warn:            lipgloss.Color("#ebcb8b"), // nord13 — yellow
	err:             lipgloss.Color("#bf616a"), // nord11 — red
	muted:           lipgloss.Color("#616e88"), // dimmed snowstorm
	label:           lipgloss.Color("#8f99ac"), // midpoint of nord3 and nord4
	border:          lipgloss.Color("#4c566a"), // nord3
	borderUnfocused: lipgloss.Color("#6d777b"), // midpoint of border and label
	bg:              lipgloss.Color("#2e3440"), // nord0
	stripe:          lipgloss.Color("#343b49"), // midpoint of bg and cursorRow
	fg:              lipgloss.Color("#d8dee9"), // nord4
	highlight:       lipgloss.Color("#3b4252"), // nord1
	statusBarBg:     lipgloss.Color("#3b4252"), // nord1
}

// catppuccinPalette is a Catppuccin Mocha¹ mapping onto the app's semantic
// color slots. Mocha's pastel accents map onto primary/accent/success/mark;
// the base/mantle/surface range supplies backgrounds, borders, and the status
// bar.
//
// ¹ https://catppuccin.com
var catppuccinPalette = colorPalette{
	primary:         lipgloss.Color("#89b4fa"), // blue
	accent:          lipgloss.Color("#cba6f7"), // mauve
	success:         lipgloss.Color("#a6e3a1"), // green
	mark:            lipgloss.Color("#94e2d5"), // teal
	search:          lipgloss.Color("#45475a"), // surface1 — search-highlight bg
	visual:          lipgloss.Color("#45475a"), // surface1 — visual-selection bg
	cursorRow:       lipgloss.Color("#313244"), // surface0
	edit:            lipgloss.Color("#fab387"), // peach
	warn:            lipgloss.Color("#f9e2af"), // yellow
	err:             lipgloss.Color("#f38ba8"), // red
	muted:           lipgloss.Color("#6c7086"), // overlay0
	label:           lipgloss.Color("#9399b2"), // overlay2
	border:          lipgloss.Color("#313244"), // surface0
	borderUnfocused: lipgloss.Color("#62667b"), // midpoint of border and label
	bg:              lipgloss.Color("#1e1e2e"), // base
	stripe:          lipgloss.Color("#282839"), // midpoint of bg and cursorRow
	fg:              lipgloss.Color("#cdd6f4"), // text
	highlight:       lipgloss.Color("#313244"), // surface0
	statusBarBg:     lipgloss.Color("#181825"), // mantle
}

// lightPalette is a light theme (GitHub-Light-inspired) for bright
// environments. Backgrounds are white/near-white; text and accents use dark,
// sufficiently-contrasting hues so highlighted cells, the status bar, and
// selection backgrounds stay readable.
var lightPalette = colorPalette{
	primary:         lipgloss.Color("#0969da"), // blue
	accent:          lipgloss.Color("#8250df"), // purple
	success:         lipgloss.Color("#1a7f37"), // green
	mark:            lipgloss.Color("#0a7b83"), // teal
	search:          lipgloss.Color("#fff8c5"), // light yellow — search-highlight bg
	visual:          lipgloss.Color("#ddf4ff"), // light blue — visual-selection bg
	cursorRow:       lipgloss.Color("#eaeef2"), // light grey tint
	edit:            lipgloss.Color("#bc4c00"), // dark orange
	warn:            lipgloss.Color("#9a6700"), // dark amber
	err:             lipgloss.Color("#cf222e"), // red
	muted:           lipgloss.Color("#6e7781"), // grey
	label:           lipgloss.Color("#57606a"), // dark grey — secondary text
	border:          lipgloss.Color("#d0d7de"), // GitHub border grey
	borderUnfocused: lipgloss.Color("#e1e4e8"), // faded border for unfocused panels
	bg:              lipgloss.Color("#ffffff"), // white
	stripe:          lipgloss.Color("#f5f7f9"), // midpoint of bg and cursorRow
	fg:              lipgloss.Color("#1f2328"), // dark text
	highlight:       lipgloss.Color("#eaeef2"), // light grey
	statusBarBg:     lipgloss.Color("#eaeef2"), // light grey status bar
}

// curatedThemes are the hand-tuned palettes. They take precedence over any
// auto-derived entry (generatedThemes) with the same normalized name, so the
// shipped defaults always stay under our control.
var curatedThemes = map[string]colorPalette{
	"tokyo-night": defaultPalette,
	"gruvbox":     gruvboxPalette,
	"nord":        nordPalette,
	"catppuccin":  catppuccinPalette,
	"light":       lightPalette,
}

// curatedThemeNames is the display order for the curated themes (default
// first); auto-derived themes follow in sorted order in the picker.
var curatedThemeNames = []string{defaultThemeName, "gruvbox", "nord", "catppuccin", "light"}

// themes is the full registry: curated plus auto-derived (see
// themes_generated.go, produced by cmd/genthemes from iTerm2-Color-Schemes).
// Built in init so generatedThemes is available before any caller reads it.
var themes map[string]colorPalette

func init() {
	themes = make(map[string]colorPalette, len(curatedThemes)+len(generatedThemes))
	for k, v := range curatedThemes {
		themes[k] = v
	}
	for k, v := range generatedThemes {
		// Curated wins on collision: a generated entry with the same
		// normalized name is dropped so the hand-tuned palette is used.
		if _, ok := themes[k]; !ok {
			themes[k] = v
		}
	}
}

// themeNames returns the available theme names: curated themes first (default
// first, in curatedThemeNames order), then the auto-derived catalog in sorted
// order. Used by the theme picker and for validation.
func themeNames() []string {
	names := append([]string{}, curatedThemeNames...)
	gen := make([]string, 0, len(generatedThemes))
	for k := range generatedThemes {
		if _, ok := curatedThemes[k]; !ok {
			gen = append(gen, k)
		}
	}
	sort.Strings(gen)
	return append(names, gen...)
}

// paletteForTheme returns the palette for name, falling back to defaultPalette
// when name is empty or not recognized. The fallback is silent by design: an
// unknown theme in the config should never block startup.
func paletteForTheme(name string) colorPalette {
	if p, ok := themes[name]; ok {
		return p
	}
	return defaultPalette
}
