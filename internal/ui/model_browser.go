package ui

// ModelBrowser is the modal behind `m` on the assistant panel: it fetches the
// models the active provider exposes (GET <base_url>/models, the standard
// OpenAI-compatible listing) and lets the user pick one, persisting the choice
// to that provider's `model:` in the config. It has three display states —
// loading, error, and the model list — all rendered in the shared modal frame.
type ModelBrowser struct {
	models   []string
	cursor   int
	visible  bool
	loading  bool
	errMsg   string
	provider string // the provider this list belongs to
}

// NewModelBrowser returns an empty browser.
func NewModelBrowser() ModelBrowser { return ModelBrowser{} }

// Show opens the browser in the loading state for the given provider. The
// current model is shown as a placeholder until the live list arrives.
func (b *ModelBrowser) Show(provider, current string) {
	b.visible = true
	b.loading = true
	b.errMsg = ""
	b.provider = provider
	b.models = []string{current}
	b.cursor = 0
}

// SetModels populates the list (loading done), centring the cursor on the
// provider's current model when present.
func (b *ModelBrowser) SetModels(models []string, current string) {
	b.loading = false
	b.errMsg = ""
	b.models = models
	b.cursor = 0
	for i, m := range models {
		if m == current {
			b.cursor = i
			break
		}
	}
}

// SetError records a fetch failure to display inline (loading done).
func (b *ModelBrowser) SetError(msg string) {
	b.loading = false
	b.errMsg = msg
}

// Hide closes the browser without changing anything.
func (b *ModelBrowser) Hide() { b.visible = false }

// IsVisible reports whether the browser is shown.
func (b ModelBrowser) IsVisible() bool { return b.visible }

// Provider returns the provider name this browser is listing models for.
func (b ModelBrowser) Provider() string { return b.provider }

// Up moves the cursor up (wrapping); a no-op until the list is loaded.
func (b *ModelBrowser) Up() {
	if len(b.models) == 0 || b.loading {
		return
	}
	if b.cursor > 0 {
		b.cursor--
	} else {
		b.cursor = len(b.models) - 1
	}
}

// Down moves the cursor down (wrapping); a no-op until the list is loaded.
func (b *ModelBrowser) Down() {
	if len(b.models) == 0 || b.loading {
		return
	}
	b.cursor = (b.cursor + 1) % len(b.models)
}

// Selected returns the model id under the cursor, or "" if the list is empty,
// still loading, or showing an error (those states are not selectable).
func (b ModelBrowser) Selected() string {
	if len(b.models) == 0 || b.loading || b.errMsg != "" {
		return ""
	}
	return b.models[b.cursor]
}

// View renders the browser via the shared modal-list frame. The loading and
// error states render a single non-selectable row (cursor -1).
func (b ModelBrowser) View() string {
	switch {
	case b.loading:
		return renderModalList([]string{"Loading models…"}, -1, modalListWidth, 1)
	case b.errMsg != "":
		return renderModalList([]string{b.errMsg}, -1, modalListWidth, 1)
	default:
		return renderModalList(b.models, b.cursor, modalListWidth, 16)
	}
}
