package ui

import (
	"github.com/rsiota/creel/internal/config"
)

// ProviderPicker is the modal behind `M` on the assistant panel: it lists the
// AI providers configured in the `ai:` block of the config and switches the
// active one (which carries its own API key, base URL, and default model).
// With no providers configured it is never shown — the panel falls back to the
// CREEL_AI_* environment variables instead.
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

// View renders the picker via the shared modal-list frame: provider names with
// the cursor row highlighted like the ctrl+p command palette. An empty list
// (no providers configured yet) renders a single placeholder row pointing at
// `n`, so the add-provider form is always reachable from `M`.
func (p ProviderPicker) View() string {
	if len(p.providers) == 0 {
		return renderModalList([]string{"No providers — press n to add one"}, -1, modalListWidth, 1)
	}
	names := make([]string, len(p.providers))
	for i, prov := range p.providers {
		names[i] = prov.Name
	}
	return renderModalList(names, p.cursor, modalListWidth, len(names))
}
