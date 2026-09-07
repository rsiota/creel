package ui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// themeOverrideSlots lists the semantic palette keys users can override via
// settings.theme_overrides / :color. Derived washes (dirty, FK tint, …) are
// rebuilt by applyPalette from these slots — they are not overrideable.
var themeOverrideSlots = []string{
	"primary",
	"accent",
	"success",
	"mark",
	"search",
	"search_match",
	"visual",
	"cursor_row",
	"edit",
	"warn",
	"err",
	"muted",
	"label",
	"border",
	"border_unfocused",
	"bg",
	"stripe",
	"fg",
	"highlight",
	"status_bar_bg",
	"fk",
}

// normalizeThemeSlot maps user-facing names onto the canonical override key.
func normalizeThemeSlot(name string) string {
	key := strings.ToLower(strings.TrimSpace(name))
	key = strings.ReplaceAll(key, "-", "_")
	switch key {
	case "error":
		return "err"
	case "foreground":
		return "fg"
	case "background":
		return "bg"
	case "statusbar", "status_bar", "statusbar_bg":
		return "status_bar_bg"
	case "cursor", "cursorrow":
		return "cursor_row"
	case "searchmatch":
		return "search_match"
	case "borderunfocused", "border_unfocus":
		return "border_unfocused"
	default:
		return key
	}
}

// isThemeOverrideSlot reports whether name is a known overridable slot.
func isThemeOverrideSlot(name string) bool {
	key := normalizeThemeSlot(name)
	for _, s := range themeOverrideSlots {
		if s == key {
			return true
		}
	}
	return false
}

// parseHexColor accepts #RGB, #RRGGBB, or RRGGBB (case-insensitive). A bare
// 3-character token is rejected so words like "bad"/"ace" are not colours.
func parseHexColor(raw string) (lipgloss.Color, bool) {
	s := strings.TrimSpace(raw)
	hasHash := strings.HasPrefix(s, "#")
	s = strings.TrimPrefix(s, "#")
	if len(s) == 3 && !hasHash {
		return "", false
	}
	if len(s) != 3 && len(s) != 6 {
		return "", false
	}
	for _, r := range s {
		isHex := (r >= '0' && r <= '9') ||
			(r >= 'a' && r <= 'f') ||
			(r >= 'A' && r <= 'F')
		if !isHex {
			return "", false
		}
	}
	if len(s) == 3 {
		s = string([]byte{s[0], s[0], s[1], s[1], s[2], s[2]})
	}
	return lipgloss.Color("#" + strings.ToLower(s)), true
}

// withThemeOverrides returns a copy of p with recognized overrides applied.
// Unknown slots and invalid colours are skipped (so a typo in config cannot
// block theme load).
func withThemeOverrides(p colorPalette, overrides map[string]string) colorPalette {
	if len(overrides) == 0 {
		return p
	}
	out := p
	for k, v := range overrides {
		slot := normalizeThemeSlot(k)
		c, ok := parseHexColor(v)
		if !ok || !isThemeOverrideSlot(slot) {
			continue
		}
		setPaletteSlot(&out, slot, c)
	}
	return out
}

func setPaletteSlot(p *colorPalette, slot string, c lipgloss.Color) {
	switch slot {
	case "primary":
		p.primary = c
	case "accent":
		p.accent = c
	case "success":
		p.success = c
	case "mark":
		p.mark = c
	case "search":
		p.search = c
	case "search_match":
		p.searchMatch = c
	case "visual":
		p.visual = c
	case "cursor_row":
		p.cursorRow = c
	case "edit":
		p.edit = c
	case "warn":
		p.warn = c
	case "err":
		p.err = c
	case "muted":
		p.muted = c
	case "label":
		p.label = c
	case "border":
		p.border = c
	case "border_unfocused":
		p.borderUnfocused = c
	case "bg":
		p.bg = c
	case "stripe":
		p.stripe = c
	case "fg":
		p.fg = c
	case "highlight":
		p.highlight = c
	case "status_bar_bg":
		p.statusBarBg = c
	case "fk":
		p.fk = c
	}
}

func paletteSlot(p colorPalette, slot string) (lipgloss.Color, bool) {
	switch normalizeThemeSlot(slot) {
	case "primary":
		return p.primary, true
	case "accent":
		return p.accent, true
	case "success":
		return p.success, true
	case "mark":
		return p.mark, true
	case "search":
		return p.search, true
	case "search_match":
		return p.searchMatch, true
	case "visual":
		return p.visual, true
	case "cursor_row":
		return p.cursorRow, true
	case "edit":
		return p.edit, true
	case "warn":
		return p.warn, true
	case "err":
		return p.err, true
	case "muted":
		return p.muted, true
	case "label":
		return p.label, true
	case "border":
		return p.border, true
	case "border_unfocused":
		return p.borderUnfocused, true
	case "bg":
		return p.bg, true
	case "stripe":
		return p.stripe, true
	case "fg":
		return p.fg, true
	case "highlight":
		return p.highlight, true
	case "status_bar_bg":
		return p.statusBarBg, true
	case "fk":
		return p.fk, true
	default:
		return "", false
	}
}

// applyTheme applies name's palette with optional overrides and rebuilds
// package-level styles. Empty / unknown names fall back like paletteForTheme.
func applyTheme(name string, overrides map[string]string) {
	applyPalette(withThemeOverrides(paletteForTheme(name), overrides))
}

// applyActiveTheme reapplies the model's configured theme plus overrides.
func (m *Model) applyActiveTheme() {
	applyTheme(m.settings.Theme, m.settings.ThemeOverrides)
}

// formatThemeOverrides summarises active overrides for status / :color.
func formatThemeOverrides(overrides map[string]string) string {
	if len(overrides) == 0 {
		return "no theme overrides"
	}
	seen := map[string]bool{}
	parts := make([]string, 0, len(overrides))
	for _, slot := range themeOverrideSlots {
		if v, ok := themeOverrideValue(overrides, slot); ok {
			parts = append(parts, slot+"="+v)
			seen[slot] = true
		}
	}
	for k, v := range overrides {
		slot := normalizeThemeSlot(k)
		if seen[slot] {
			continue
		}
		parts = append(parts, slot+"="+v)
	}
	return strings.Join(parts, "  ")
}

// exColor views or changes a semantic theme colour override
// (:color [slot] [hex|default]).
func (m *Model) exColor(args []string) tea.Cmd {
	switch len(args) {
	case 0:
		m.schemaMsg = formatThemeOverrides(m.settings.ThemeOverrides)
		return nil
	case 1:
		slot := normalizeThemeSlot(args[0])
		if !isThemeOverrideSlot(slot) {
			m.schemaMsg = fmt.Sprintf("unknown colour slot: %s (try :color)", args[0])
			return nil
		}
		if v, ok := themeOverrideValue(m.settings.ThemeOverrides, slot); ok {
			m.schemaMsg = fmt.Sprintf("%s=%s (override)", slot, v)
			return nil
		}
		base := paletteForTheme(m.settings.Theme)
		if c, ok := paletteSlot(base, slot); ok && c != "" {
			m.schemaMsg = fmt.Sprintf("%s=%s (theme)", slot, string(c))
			return nil
		}
		m.schemaMsg = fmt.Sprintf("%s=(derived)", slot)
		return nil
	default:
		slot := normalizeThemeSlot(args[0])
		if !isThemeOverrideSlot(slot) {
			m.schemaMsg = fmt.Sprintf("unknown colour slot: %s", args[0])
			return nil
		}
		val := strings.TrimSpace(args[1])
		switch strings.ToLower(val) {
		case "default", "clear", "off", "none", "reset":
			m.clearThemeOverride(slot)
			m.applyActiveTheme()
			m.schemaMsg = slot + "=default"
			return nil
		}
		c, ok := parseHexColor(val)
		if !ok {
			m.schemaMsg = ":color needs a hex colour (#rgb / #rrggbb) or default"
			return nil
		}
		m.setThemeOverride(slot, string(c))
		m.applyActiveTheme()
		m.schemaMsg = slot + "=" + string(c)
		return nil
	}
}

func themeOverrideValue(overrides map[string]string, slot string) (string, bool) {
	if overrides == nil {
		return "", false
	}
	if v, ok := overrides[slot]; ok {
		return v, true
	}
	for k, v := range overrides {
		if normalizeThemeSlot(k) == slot {
			return v, true
		}
	}
	return "", false
}

func (m *Model) setThemeOverride(slot, hex string) {
	if m.settings.ThemeOverrides == nil {
		m.settings.ThemeOverrides = map[string]string{}
	}
	// Drop any alias keys for the same slot, then write canonical.
	for k := range m.settings.ThemeOverrides {
		if normalizeThemeSlot(k) == slot {
			delete(m.settings.ThemeOverrides, k)
		}
	}
	m.settings.ThemeOverrides[slot] = hex
	m.saveThemeOverrides()
}

func (m *Model) clearThemeOverride(slot string) {
	if m.settings.ThemeOverrides == nil {
		return
	}
	for k := range m.settings.ThemeOverrides {
		if normalizeThemeSlot(k) == slot {
			delete(m.settings.ThemeOverrides, k)
		}
	}
	if len(m.settings.ThemeOverrides) == 0 {
		m.settings.ThemeOverrides = nil
	}
	m.saveThemeOverrides()
}

func (m *Model) saveThemeOverrides() {
	if m.config == nil {
		return
	}
	m.config.Settings.ThemeOverrides = m.settings.ThemeOverrides
	_ = m.config.Save()
}

// completeColor offers slot names (arg 0) or default/hex hints (arg 1).
func completeColor(_ *Model, args []string, partial string) []string {
	switch len(args) {
	case 0:
		return themeOverrideSlots
	case 1:
		return []string{"default", "#ffffff", "#000000"}
	default:
		return nil
	}
}
