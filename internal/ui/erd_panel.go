package ui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// ERDPanel displays a generated entity-relationship diagram as a scrollable,
// read-only overlay — the static counterpart to the interactive `g r`
// relationship explorer. It has two views:
//
//   - graph (default): bordered table cards laid out in dependency-ranked
//     columns with box-drawing arrows from each FK to the PK it references.
//   - mermaid: the same schema as a Mermaid erDiagram source (renders inline
//     in GitHub/GitLab markdown) — toggle with `m`.
//
// `y`/`s` always copy/save the Mermaid source (the exportable text format);
// the graph is for looking. j/k scroll, h/l pan horizontally (graph view),
// g/G/ctrl+d/ctrl+u page, esc/q close.
type ERDPanel struct {
	visible bool
	title   string
	graph   *gcanvas // graphical view; nil if there were no tables
	mermaid []string // Mermaid erDiagram source lines
	merm    bool     // show Mermaid source instead of the graph

	scrollY int // top visible row
	scrollX int // left visible column (graph view only)
	cursor  int // for relative paging
	width   int
	height  int
}

func (e ERDPanel) IsVisible() bool { return e.visible }

// Show populates the panel with both representations and shows the graph view.
func (e *ERDPanel) Show(title string, graph *gcanvas, mermaid []string) {
	e.visible = true
	e.title = title
	e.graph = graph
	e.mermaid = mermaid
	e.merm = false
	e.scrollY = 0
	e.scrollX = 0
	e.cursor = 0
}

// MermaidLines returns the Mermaid source (used by the app's copy/save handlers
// and `:erd save`, since Mermaid is the exportable format).
func (e ERDPanel) MermaidLines() []string { return e.mermaid }

// Hide hides the panel.
func (e *ERDPanel) Hide() { e.visible = false }

// SetSize sets the outer dimensions of the panel (including border).
func (e *ERDPanel) SetSize(width, height int) {
	e.width = width
	e.height = height
}

// The panel is frameless and fills the whole workspace, so the content area
// is the full size (no border/padding overhead).
func (e ERDPanel) contentHeight() int {
	if e.height < 1 {
		return 1
	}
	return e.height
}

func (e ERDPanel) contentWidth() int {
	if e.width < 1 {
		return 1
	}
	return e.width
}

// lineCount is the number of scrollable rows in the active view.
func (e ERDPanel) lineCount() int {
	if e.merm {
		return len(e.mermaid)
	}
	if e.graph != nil {
		return e.graph.h
	}
	return 1
}

// Update handles scroll/pan/toggle keys, returning the updated panel.
func (e ERDPanel) Update(msg tea.KeyMsg) ERDPanel {
	n := e.lineCount()
	vh := e.contentHeight()
	if vh < 1 {
		vh = 1
	}
	switch msg.String() {
	case "j", "down":
		if e.cursor < n-1 {
			e.cursor++
		}
	case "k", "up":
		if e.cursor > 0 {
			e.cursor--
		}
	case "g":
		e.cursor = 0
	case "G":
		e.cursor = n - 1
	case "ctrl+d":
		e.cursor += vh / 2
		if e.cursor >= n {
			e.cursor = n - 1
		}
	case "ctrl+u":
		e.cursor -= vh / 2
		if e.cursor < 0 {
			e.cursor = 0
		}
	case "h", "left":
		if !e.merm && e.graph != nil {
			e.scrollX -= e.contentWidth() / 2
			if e.scrollX < 0 {
				e.scrollX = 0
			}
		}
	case "l", "right":
		if !e.merm && e.graph != nil {
			max := e.graph.w - e.contentWidth()
			if max < 0 {
				max = 0
			}
			e.scrollX += e.contentWidth() / 2
			if e.scrollX > max {
				e.scrollX = max
			}
		}
	case "m":
		e.merm = !e.merm
		e.scrollX = 0
		e.cursor = 0
		e.scrollY = 0
	}
	e.adjustScroll(vh)
	return e
}

func (e *ERDPanel) adjustScroll(vh int) {
	if e.cursor < e.scrollY {
		e.scrollY = e.cursor
	}
	if e.cursor >= e.scrollY+vh {
		e.scrollY = e.cursor - vh + 1
	}
	if e.scrollY < 0 {
		e.scrollY = 0
	}
}

// View renders the active view's scroll window edge to edge — no border,
// title, or footer, so the diagram fills the whole workspace (the status line
// remains visible below it). Key hints live in the status bar / help overlay.
// The body is padded to the full content width/height so the overlay fully
// covers the workspace behind it.
func (e ERDPanel) View() string {
	cw := e.contentWidth()
	ch := e.contentHeight()
	n := e.lineCount()
	if ch < 1 {
		ch = 1
	}
	if e.scrollY > n {
		e.scrollY = 0
	}
	end := e.scrollY + ch
	if end > n {
		end = n
	}

	var body string
	if e.merm {
		var visible []string
		for i := e.scrollY; i < end; i++ {
			visible = append(visible, e.mermaid[i])
		}
		for len(visible) < ch {
			visible = append(visible, "")
		}
		body = lipgloss.JoinVertical(lipgloss.Left, visible...)
	} else if e.graph != nil {
		body = e.graph.Window(e.scrollX, cw, e.scrollY, ch)
		rows := strings.Count(body, "\n") + 1
		for rows < ch {
			body += "\n"
			rows++
		}
	} else {
		body = mutedStyle.Render("(no tables)")
	}
	return lipgloss.NewStyle().Width(cw).Height(ch).Render(body)
}

// joinERDLines joins diagram source lines for copy/save.
func joinERDLines(lines []string) string { return strings.Join(lines, "\n") + "\n" }
