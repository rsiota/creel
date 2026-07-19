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

// View renders the picker as a bordered panel: just the model ids under a
// selection marker (no per-model description, which cluttered the list).
func (p ModelPicker) View() string {
	var rows []string
	for i, o := range p.options {
		marker := " "
		markerStyle := lipgloss.NewStyle().Foreground(colorBorder)
		if i == p.cursor {
			marker = "▸"
			markerStyle = lipgloss.NewStyle().Foreground(colorAccent).Bold(true)
		}
		row := markerStyle.Render(marker) + " " +
			lipgloss.NewStyle().Foreground(colorFg).Render(o.id)
		rows = append(rows, row)
	}
	body := strings.Join(rows, "\n")

	title := lipgloss.NewStyle().Foreground(colorPrimary).Bold(true).Render("Model")

	// No in-popup footer: the status bar surfaces the picker's keybindings
	// (j/k move · enter select · esc cancel) like every other panel.
	content := lipgloss.JoinVertical(lipgloss.Left, title, body)
	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(colorPrimary).
		Padding(0, 1).
		Render(content)
}
