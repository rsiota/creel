package ui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// aiModelOption is one entry in the model picker.
type aiModelOption struct {
	id    string
	label string // short description shown beside the id
}

// aiModelOptions is the curated set offered in the picker. These are the
// z.ai GLM coding models (the provider this app targets by default); for
// other OpenAI-compatible providers the ids may differ, but the picker still
// works as a quick switch and GSQL_AI_MODEL / the choice overrides the env
// default. glm-4.5-air is a reasoning model — selecting it is what makes the
// streamed "thinking" preview actually appear.
var aiModelOptions = []aiModelOption{
	{"glm-4.6", "fast · non-reasoning (default)"},
	{"glm-4.5-air", "reasoning · slower"},
	{"glm-4.5", "glm-4.5"},
	{"glm-5.1", "glm-5.1"},
	{"glm-5.2", "glm-5.2"},
}

// ModelPicker is a small modal single-select for the AI model. With only a
// handful of options it skips filtering/scroll: up/down moves the cursor,
// enter commits, esc cancels.
type ModelPicker struct {
	options []aiModelOption
	cursor  int
	visible bool
}

// NewModelPicker returns a picker over the curated model set.
func NewModelPicker() ModelPicker { return ModelPicker{options: aiModelOptions} }

// Show reveals the picker with the cursor on the current model (matched by id;
// unmatched falls through to the first option).
func (p *ModelPicker) Show(current string) {
	p.visible = true
	p.cursor = 0
	for i, o := range p.options {
		if o.id == current {
			p.cursor = i
			break
		}
	}
}

// Hide closes the picker without changing the selection.
func (p *ModelPicker) Hide() { p.visible = false }

// IsVisible reports whether the picker is shown.
func (p ModelPicker) IsVisible() bool { return p.visible }

// Up moves the cursor up (wrapping).
func (p *ModelPicker) Up() {
	if p.cursor > 0 {
		p.cursor--
	} else {
		p.cursor = len(p.options) - 1
	}
}

// Down moves the cursor down (wrapping).
func (p *ModelPicker) Down() {
	p.cursor = (p.cursor + 1) % len(p.options)
}

// Selected returns the model id under the cursor.
func (p ModelPicker) Selected() string {
	if len(p.options) == 0 {
		return ""
	}
	return p.options[p.cursor].id
}

// modelPickerWidth is the exterior cell width of the picker popup. It
// matches the small confirm pickers (truncate/drop table), so the model
// picker lines up with them.
const modelPickerWidth = 46

// View renders the picker as a bordered panel: just the model ids, with the
// cursor row highlighted like the ctrl+p command palette (full-width primary
// background, inverted text, "❯" chevron). No title/footer — the status bar
// surfaces the keybindings (j/k move · enter select · esc cancel).
func (p ModelPicker) View() string {
	// Text-area width = panel width minus the horizontal padding (Padding(0,1)).
	rowW := modelPickerWidth - 2
	var rows []string
	for i, o := range p.options {
		if i == p.cursor {
			// Pad to the full row width so the primary background fills the row
			// (a short id would otherwise leave a partial highlight).
			label := "❯ " + o.id + strings.Repeat(" ", rowW-2-runeLen(o.id))
			rows = append(rows, lipgloss.NewStyle().
				Background(colorPrimary).
				Foreground(colorBg).
				Render(label))
		} else {
			id := lipgloss.NewStyle().Foreground(colorFg).Render(o.id)
			rows = append(rows, "  "+id)
		}
	}
	body := strings.Join(rows, "\n")

	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(colorPrimary).
		Padding(0, 1).
		Width(modelPickerWidth).
		Render(body)
}
