package ui

import (
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/rsiota/creel/internal/config"
)

// paletteForTheme resolves known theme names to their palettes.
func TestPaletteForTheme(t *testing.T) {
	for _, name := range themeNames() {
		if got := paletteForTheme(name); got != themes[name] {
			t.Errorf("paletteForTheme(%q) returned a different palette than themes[%q]", name, name)
		}
	}
}

// paletteForTheme falls back to the default for empty or unknown names rather
// than panicking — an invalid theme in config must never block startup.
func TestPaletteForThemeFallsBackToDefault(t *testing.T) {
	for _, name := range []string{"", "unknown", "Tokyo-Night", "solarized"} {
		if got := paletteForTheme(name); got != defaultPalette {
			t.Errorf("paletteForTheme(%q) = %+v, want defaultPalette", name, got)
		}
	}
}

// themeNames lists the default first so the picker can land on it initially.
func TestThemeNamesDefaultFirst(t *testing.T) {
	names := themeNames()
	if len(names) == 0 {
		t.Fatal("themeNames() is empty")
	}
	if names[0] != defaultThemeName {
		t.Errorf("themeNames()[0] = %q, want %q", names[0], defaultThemeName)
	}
	// Every name resolves.
	for _, n := range names {
		if _, ok := themes[n]; !ok {
			t.Errorf("themeNames() lists %q which is not in themes map", n)
		}
	}
}

// NewModel applies the configured theme to the package-level color vars before
// the first render. Restores the default afterward so other tests are unaffected.
func TestNewModelAppliesTheme(t *testing.T) {
	defer applyPalette(defaultPalette)

	cfg := &config.Config{Settings: config.Settings{Theme: "gruvbox"}}
	NewModel(cfg)

	if colorPrimary != gruvboxPalette.primary {
		t.Errorf("colorPrimary = %s, want gruvbox %s", colorPrimary, gruvboxPalette.primary)
	}
	if colorBg != gruvboxPalette.bg {
		t.Errorf("colorBg = %s, want gruvbox %s", colorBg, gruvboxPalette.bg)
	}
	if colorFg == defaultPalette.fg {
		t.Errorf("colorFg still default after applying gruvbox")
	}
}

// NewModel with no theme setting leaves the default palette in place.
func TestNewModelAppliesDefaultThemeWhenUnset(t *testing.T) {
	defer applyPalette(defaultPalette)

	NewModel(&config.Config{})

	if colorPrimary != defaultPalette.primary {
		t.Errorf("colorPrimary = %s, want default %s", colorPrimary, defaultPalette.primary)
	}
}

// NewModel with an unknown theme falls back to the default rather than leaving
// the palette in a half-applied state.
func TestNewModelFallsBackForUnknownTheme(t *testing.T) {
	defer applyPalette(defaultPalette)

	cfg := &config.Config{Settings: config.Settings{Theme: "nope"}}
	NewModel(cfg)

	if colorPrimary != defaultPalette.primary {
		t.Errorf("colorPrimary = %s, want default %s after unknown theme", colorPrimary, defaultPalette.primary)
	}
}

// Every shipped palette defines all semantic slots as non-empty colors, so a
// theme can never silently render a component with a missing color.
func TestPalettesHaveNoEmptyColors(t *testing.T) {
	for name, p := range themes {
		for field, c := range map[string]lipgloss.Color{
			"primary": p.primary, "accent": p.accent, "success": p.success,
			"mark": p.mark, "search": p.search, "visual": p.visual,
			"cursorRow": p.cursorRow, "edit": p.edit, "warn": p.warn,
			"err": p.err, "muted": p.muted, "label": p.label,
			"border": p.border, "borderUnfocused": p.borderUnfocused,
			"bg": p.bg, "stripe": p.stripe, "fg": p.fg,
			"highlight": p.highlight, "statusBarBg": p.statusBarBg,
		} {
			if c == "" {
				t.Errorf("theme %q has empty color for %s", name, field)
			}
		}
	}
}

// themeNames exposes every theme in the registry (set comparison, not order).
func TestThemeNamesCoversRegistry(t *testing.T) {
	names := themeNames()
	seen := map[string]bool{}
	for _, n := range names {
		if seen[n] {
			t.Errorf("themeNames() has duplicate %q", n)
		}
		seen[n] = true
	}
	for n := range themes {
		if !seen[n] {
			t.Errorf("themeNames() missing %q (in themes map but not listed)", n)
		}
	}
	if len(names) != len(themes) {
		t.Errorf("themeNames() has %d names, themes map has %d", len(names), len(themes))
	}
	// Curated themes come first, in their defined order (default first); the
	// auto-derived catalog follows in sorted order.
	for i, n := range curatedThemeNames {
		if i >= len(names) || names[i] != n {
			t.Errorf("themeNames()[%d] = %q, want curated %q", i, names[i], n)
		}
	}
}

// Every theme's foreground/background contrast meets WCAG AA (>= 4.5) so text
// stays readable across all shipped palettes — especially the light theme,
// where it's easy to pick a fg that's too dim against a white background.
func TestThemeForegroundBackgroundContrast(t *testing.T) {
	for name, p := range themes {
		ratio := contrastRatio(string(p.fg), string(p.bg))
		if ratio < 4.5 {
			t.Errorf("theme %q: fg/bg contrast %.2f < 4.5 (fg=%s bg=%s)",
				name, ratio, p.fg, p.bg)
		}
	}
}

// muted and label are secondary text colours (connection details, ERD column
// types, dimmed cards, form/field labels). On light backgrounds, a brightBlack-
// derived muted is often itself a light grey and collapses to illegible
// contrast; those themes must clear WCAG AA. Dark themes keep softer curated
// greys (readable in practice, below AA) so this check is light-bg only.
func TestThemeMutedLabelBackgroundContrast(t *testing.T) {
	for name, p := range themes {
		if relLuminance(string(p.bg)) <= 0.4 {
			continue
		}
		for slot, c := range map[string]lipgloss.Color{"muted": p.muted, "label": p.label} {
			ratio := contrastRatio(string(c), string(p.bg))
			if ratio < 4.5 {
				t.Errorf("theme %q: %s/bg contrast %.2f < 4.5 (%s=%s bg=%s)",
					name, slot, ratio, slot, c, p.bg)
			}
		}
	}
}

// visual is the highlight BACKGROUND behind fg text (marked columns, visual
// row selection). Terminal selectionBackground is often inverted on light
// schemes; every theme must keep visual/fg above WCAG AA.
func TestThemeVisualForegroundContrast(t *testing.T) {
	for name, p := range themes {
		ratio := contrastRatio(string(p.visual), string(p.fg))
		if ratio < 4.5 {
			t.Errorf("theme %q: visual/fg contrast %.2f < 4.5 (visual=%s fg=%s)",
				name, ratio, p.visual, p.fg)
		}
	}
}

// ERD vivid/dim prioritize readable faded cards. Soft floors: dim should not
// collapse into the background, and vivid should still separate from dim on
// typical themes (exact dual floors are not always possible on every scheme).
func TestThemeERDVividDimGap(t *testing.T) {
	defer applyPalette(defaultPalette)
	for name, p := range themes {
		vivid, dim := deriveERDColors(p)
		vsBg := contrastRatio(string(dim), string(p.bg))
		if vsBg < 2.0 {
			t.Errorf("theme %q: ERD dim too faint vs bg (%.2f; dim=%s bg=%s)",
				name, vsBg, dim, p.bg)
		}
		gap := contrastRatio(string(vivid), string(dim))
		if gap < 1.05 {
			t.Errorf("theme %q: ERD vivid/dim collapsed (gap %.2f; vivid=%s dim=%s)",
				name, gap, vivid, dim)
		}
	}
}

// GitHub Light is the regression case: muted≈primary hid selection when reused
// for dim/vivid; the ultra-faint wash made dimmed cards unreadable. Expect a
// readable dim and a clear vivid/dim gap.
func TestDeriveERDColorsGitHubLight(t *testing.T) {
	p := themes["git-hub-light-default"]
	vivid, dim := deriveERDColors(p)
	if vsBg := contrastRatio(string(dim), string(p.bg)); vsBg < 2.0 {
		t.Fatalf("git-hub-light-default dim/bg %.2f < 2.0 (dim=%s) — faded cards unreadable",
			vsBg, dim)
	}
	if gap := contrastRatio(string(vivid), string(dim)); gap < erdVividDimMinGap {
		t.Fatalf("git-hub-light-default vivid/dim gap %.2f < %.1f (vivid=%s dim=%s)",
			gap, erdVividDimMinGap, vivid, dim)
	}
}
