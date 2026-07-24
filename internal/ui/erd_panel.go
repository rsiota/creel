package ui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// ERDPanel displays a generated Mermaid erDiagram as a scrollable, read-only
// overlay — the static counterpart to the interactive `g r` relationship
// explorer. It mirrors ExplainPanel/LookupPanel's scroll machinery and adds a
// one-line footer hint (y copy · s save · esc close). The panel itself holds
// only rendered text lines; the copy/save side effects live in the app's key
// dispatch (they touch the clipboard / filesystem / status bar).
type ERDPanel struct {
	visible bool
	title   string
	lines   []string
	scroll  int
	cursor  int
	width   int
	height  int
}

func (e ERDPanel) IsVisible() bool { return e.visible }

// Show populates the panel with a title and the diagram lines, then makes it
// visible with the cursor at the top.
func (e *ERDPanel) Show(title string, lines []string) {
	e.visible = true
	e.title = title
	e.lines = lines
	e.cursor = 0
	e.scroll = 0
}

// Lines returns the diagram source (used by the app's copy/save handlers).
func (e ERDPanel) Lines() []string { return e.lines }

// Hide hides the panel.
func (e *ERDPanel) Hide() { e.visible = false }

// SetSize sets the outer dimensions of the panel (including border).
func (e *ERDPanel) SetSize(width, height int) {
	e.width = width
	e.height = height
}

func (e ERDPanel) contentHeight() int {
	h := e.height - borderOverhead
	if h < 1 {
		h = 1
	}
	return h
}

// Update handles scroll keys. Returns the updated panel.
func (e ERDPanel) Update(msg tea.KeyMsg) ERDPanel {
	// Reserve one line for the footer hint.
	n := len(e.lines)
	vh := e.contentHeight() - 1
	if vh < 1 {
		vh = 1
	}
	switch msg.String() {
	case "j", "down":
		if e.cursor < n-1 {
			e.cursor++
			e.adjustScroll(vh)
		}
	case "k", "up":
		if e.cursor > 0 {
			e.cursor--
			e.adjustScroll(vh)
		}
	case "g":
		e.cursor = 0
		e.scroll = 0
	case "G":
		e.cursor = n - 1
		e.adjustScroll(vh)
	case "ctrl+d":
		e.cursor += vh / 2
		if e.cursor >= n {
			e.cursor = n - 1
		}
		e.adjustScroll(vh)
	case "ctrl+u":
		e.cursor -= vh / 2
		if e.cursor < 0 {
			e.cursor = 0
		}
		e.adjustScroll(vh)
	}
	return e
}

func (e *ERDPanel) adjustScroll(vh int) {
	if e.cursor < e.scroll {
		e.scroll = e.cursor
	}
	if e.cursor >= e.scroll+vh {
		e.scroll = e.cursor - vh + 1
	}
}

// View renders the panel with a border, a titled header, the scroll window,
// and a footer hint line.
func (e ERDPanel) View() string {
	n := len(e.lines)
	vh := e.contentHeight() - 1 // -1 for the footer
	if vh < 1 {
		vh = 1
	}

	if e.scroll > n {
		e.scroll = 0
	}
	if e.scroll < 0 {
		e.scroll = 0
	}
	end := e.scroll + vh
	if end > n {
		end = n
	}

	var visible []string
	for i := e.scroll; i < end; i++ {
		visible = append(visible, e.lines[i])
	}
	for len(visible) < vh {
		visible = append(visible, "")
	}

	header := lipgloss.NewStyle().Foreground(colorPrimary).Bold(true).Render(e.title)
	body := lipgloss.JoinVertical(lipgloss.Left, visible...)
	footer := mutedStyle.Render("j/k scroll · y copy · s save · esc close")

	content := lipgloss.JoinVertical(lipgloss.Left, header, body, footer)

	return lipgloss.NewStyle().
		Width(e.width).
		Height(e.height).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(colorPrimary).
		Padding(0, 1).
		Render(content)
}

// joinERDLines is a small helper for the copy/save handlers.
func joinERDLines(lines []string) string { return strings.Join(lines, "\n") + "\n" }
