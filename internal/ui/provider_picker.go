package ui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/ruben/gsql/internal/config"
)

// ProviderPicker is the modal behind `M` on the assistant panel: it lists the
// AI providers configured in the `ai:` block of the config and switches the
// active one (which carries its own API key, base URL, and default model).
// With no providers configured it is never shown — the panel falls back to the
// GSQL_AI_* environment variables instead.
type ProviderPicker struct {
	providers []config.AIProvider
	cursor    int
	visible   bool
}

// NewProviderPicker returns an empty picker; the provider list is supplied at
// Show time so it always reflects the current config.
func NewProviderPicker() ProviderPicker { return ProviderPicker{} }

// Show reveals the picker over the given providers, with the cursor on the
// active provider (matched by name; unmatched falls through to the first).
func (p *ProviderPicker) Show(providers []config.AIProvider, current string) {
	p.visible = true
	p.providers = providers
	p.cursor = 0
	for i, prov := range p.providers {
		if prov.Name == current {
			p.cursor = i
			break
		}
	}
}

// Hide closes the picker without changing the selection.
func (p *ProviderPicker) Hide() { p.visible = false }

// IsVisible reports whether the picker is shown.
func (p ProviderPicker) IsVisible() bool { return p.visible }

// Up moves the cursor up (wrapping).
func (p *ProviderPicker) Up() {
	if p.cursor > 0 {
		p.cursor--
	} else {
		p.cursor = len(p.providers) - 1
	}
}

// Down moves the cursor down (wrapping).
func (p *ProviderPicker) Down() {
	p.cursor = (p.cursor + 1) % len(p.providers)
}

// Selected returns the provider name under the cursor, or "" if the list is
// empty.
func (p ProviderPicker) Selected() string {
	if len(p.providers) == 0 {
		return ""
	}
	return p.providers[p.cursor].Name
}

// providerPickerWidth is the exterior cell width of the picker popup. It
// matches the small confirm pickers (truncate/drop table), so the provider
// picker lines up with them.
const providerPickerWidth = 46

// View renders the picker as a bordered panel: provider names, with the cursor
// row highlighted like the ctrl+p command palette (full-width primary
// background, inverted text, "❯" chevron). No title/footer — the status bar
// surfaces the keybindings (j/k move · enter select · esc cancel).
func (p ProviderPicker) View() string {
	// Text-area width = panel width minus the horizontal padding (Padding(0,1)).
	rowW := providerPickerWidth - 2
	var rows []string
	for i, prov := range p.providers {
		if i == p.cursor {
			// Pad to the full row width so the primary background fills the row
			// (a short name would otherwise leave a partial highlight).
			label := "❯ " + prov.Name + strings.Repeat(" ", rowW-2-runeLen(prov.Name))
			rows = append(rows, lipgloss.NewStyle().
				Background(colorPrimary).
				Foreground(colorBg).
				Render(label))
		} else {
			name := lipgloss.NewStyle().Foreground(colorFg).Render(prov.Name)
			rows = append(rows, "  "+name)
		}
	}
	body := strings.Join(rows, "\n")

	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(colorPrimary).
		Padding(0, 1).
		Width(providerPickerWidth).
		Render(body)
}
