package ui

import (
	"math"
	"strconv"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/ruben/gsql/internal/config"
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

// relLuminance returns the WCAG relative luminance of a hex color like "#7aa2f7".
func relLuminance(hex string) float64 {
	hex = strings.TrimPrefix(hex, "#")
	r, _ := strconv.ParseUint(hex[0:2], 16, 8)
	g, _ := strconv.ParseUint(hex[2:4], 16, 8)
	b, _ := strconv.ParseUint(hex[4:6], 16, 8)
	channel := func(c uint64) float64 {
		v := float64(c) / 255
		if v <= 0.03928 {
			return v / 12.92
		}
		return math.Pow((v+0.055)/1.055, 2.4)
	}
	return 0.2126*channel(r) + 0.7152*channel(g) + 0.0722*channel(b)
}

// contrastRatio returns the WCAG contrast ratio between two hex colors.
func contrastRatio(a, b string) float64 {
	la := relLuminance(a)
	lb := relLuminance(b)
	if la < lb {
		la, lb = lb, la
	}
	return (la + 0.05) / (lb + 0.05)
}
