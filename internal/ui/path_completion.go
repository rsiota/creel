package ui

import (
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/lipgloss"
)

const maxPathCompletions = 8

// pathCompletion holds filesystem path autocomplete state (shared by the
// import prompt and connection-form path fields).
type pathCompletion struct {
	completions  []string
	compSelected int
	compVisible  bool
}

func (pc *pathCompletion) refresh(input string) {
	pc.completions = completeFilePath(input)
	pc.compSelected = 0
	pc.compVisible = len(pc.completions) > 0
}

func (pc *pathCompletion) clear() {
	pc.completions = nil
	pc.compSelected = 0
	pc.compVisible = false
}

// hasChoices reports whether the dropdown has a selectable entry.
func (pc pathCompletion) hasChoices() bool {
	return pc.compVisible && len(pc.completions) > 0
}

func (pc *pathCompletion) accept(input *textinput.Model) {
	if pc.compSelected >= len(pc.completions) {
		return
	}
	entry := pc.completions[pc.compSelected]
	val := input.Value()
	dir, _ := splitPathVal(val)
	input.SetValue(dir + entry)
	input.CursorEnd()
	pc.refresh(input.Value())
}

func (pc *pathCompletion) move(delta int) {
	n := len(pc.completions)
	if n == 0 {
		return
	}
	pc.compSelected = (pc.compSelected + delta + n) % n
}

func (pc pathCompletion) View() string {
	if !pc.compVisible || len(pc.completions) == 0 {
		return ""
	}
	return lipgloss.NewStyle().
		Border(panelBorder()).
		BorderForeground(colorBorder).
		Render(renderPathCompletionItems(pc.completions, pc.compSelected))
}

func renderPathCompletionItems(completions []string, selected int) string {
	start := 0
	if selected >= maxPathCompletions {
		start = selected - maxPathCompletions + 1
	}
	end := start + maxPathCompletions
	if end > len(completions) {
		end = len(completions)
	}

	var lines []string
	for i := start; i < end; i++ {
		name := completions[i]
		var style lipgloss.Style
		if i == selected {
			style = lipgloss.NewStyle().
				Bold(true).
				Background(colorHighlight).
				Foreground(colorFg).
				Padding(0, 1)
		} else {
			style = lipgloss.NewStyle().
				Foreground(colorFg).
				Padding(0, 1)
		}
		lines = append(lines, style.Render(name))
	}
	return strings.Join(lines, "\n")
}
