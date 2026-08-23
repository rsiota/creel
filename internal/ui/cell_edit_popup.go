package ui

import (
	"strings"

	"github.com/charmbracelet/bubbles/textarea"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// CellEditPopup is a modal multiline editor for cell values whose content is
// too large to edit comfortably inline (truncated with an ellipsis in the grid
// or inspector). It opens a centered textarea large enough to visualize and
// edit the value. On commit the value is staged into the same dirtyCells
// pipeline used by the inline editor (no immediate DB flush).
type CellEditPopup struct {
	ta       textarea.Model
	visible  bool
	readOnly bool   // view-only: render statically, never stage edits
	scrollY  int    // top line offset when readOnly
	row      int    // results row index
	col      int    // results column index
	colName  string // column header, shown in the popup title line
	width    int    // content width (excludes border + padding)
	height   int    // content height in lines
	jsonMode bool   // when true, render highlighted pretty-printed JSON
}

// NewCellEditPopup creates a hidden cell-edit popup.
func NewCellEditPopup() CellEditPopup {
	return CellEditPopup{}
}

// Show opens the popup seeded with the given value. JSON objects and arrays
// are automatically pretty-printed and syntax-highlighted for readability.
// When readOnly is true the popup is a view-only peek: the value renders
// statically (no cursor, no edits staged) — used for results that can't be
// written back (read-only mode, custom queries, views without a primary key).
func (p *CellEditPopup) Show(val string, row, col int, colName string, readOnly bool) {
	ta := textarea.New()
	ta.Prompt = ""
	ta.ShowLineNumbers = false
	ta.CharLimit = 0 // no limit

	p.jsonMode = false
	if pretty, ok := formatJSON(val); ok {
		val = pretty
		p.jsonMode = true
	}

	ta.SetValue(val)
	// CursorLine must carry an explicit fg: bubbles paints the cursor line
	// with CursorLine alone (not Text), and an empty style leaves runes at
	// the terminal default FG — illegible once paintBg lays the theme bg
	// under a light palette on a dark terminal (or vice versa).
	editText := lipgloss.NewStyle().Foreground(colorFg)
	ta.FocusedStyle.CursorLine = editText
	ta.FocusedStyle.Text = editText
	ta.FocusedStyle.Placeholder = lipgloss.NewStyle().Foreground(colorMuted)
	ta.BlurredStyle = ta.FocusedStyle
	p.ta = ta
	p.visible = true
	p.readOnly = readOnly
	p.scrollY = 0
	p.row = row
	p.col = col
	p.colName = colName
}

// Hide closes the popup.
func (p *CellEditPopup) Hide() {
	p.visible = false
	p.ta = textarea.Model{}
	p.colName = ""
	p.jsonMode = false
	p.readOnly = false
	p.scrollY = 0
}

// IsVisible reports whether the popup is open.
func (p CellEditPopup) IsVisible() bool {
	return p.visible
}

// IsReadOnly reports whether the popup is open in view-only mode (no edits
// staged).
func (p CellEditPopup) IsReadOnly() bool {
	return p.readOnly
}

// Value returns the current editor contents.
func (p CellEditPopup) Value() string {
	return p.ta.Value()
}

// Row returns the results row the popup was opened on.
func (p CellEditPopup) Row() int {
	return p.row
}

// Col returns the results column the popup was opened on.
func (p CellEditPopup) Col() int {
	return p.col
}

// SetMaxSize clamps the popup's textarea dimensions to fit the available
// terminal area. contentW/contentH are the maximum usable cell counts for the
// textarea body; the bordered frame adds a fixed overhead.
func (p *CellEditPopup) SetMaxSize(contentW, contentH int) {
	// Reserve 4 cols for the "│ " + " │" padding around each textarea line.
	textW := contentW - 4
	if textW < 30 {
		textW = 30
	}
	p.width = textW
	if contentH < 3 {
		contentH = 3
	}
	p.height = contentH
	p.ta.SetWidth(textW)
	p.ta.SetHeight(contentH)
}

// Focus focuses the textarea. It is a no-op in read-only mode, where the
// value is rendered statically with no cursor.
func (p *CellEditPopup) Focus() tea.Cmd {
	if !p.visible || p.readOnly {
		return nil
	}
	return p.ta.Focus()
}

// Update forwards messages to the textarea, or — in read-only mode — handles
// just the scroll keys so the value can be paged through without being edited.
func (p CellEditPopup) Update(msg tea.Msg) (CellEditPopup, tea.Cmd) {
	if !p.visible {
		return p, nil
	}
	if p.readOnly {
		p.scrollReadOnly(msg)
		return p, nil
	}
	var cmd tea.Cmd
	p.ta, cmd = p.ta.Update(msg)
	return p, cmd
}

// scrollReadOnly adjusts the read-only viewport offset for navigation keys so
// long values can be paged through. Mouse and unrecognized keys are ignored.
func (p *CellEditPopup) scrollReadOnly(msg tea.Msg) {
	k, ok := msg.(tea.KeyMsg)
	if !ok {
		return
	}
	maxScroll := len(strings.Split(p.ta.Value(), "\n")) - p.height
	if maxScroll < 0 {
		maxScroll = 0
	}
	switch k.String() {
	case "j", "down":
		if p.scrollY < maxScroll {
			p.scrollY++
		}
	case "k", "up":
		if p.scrollY > 0 {
			p.scrollY--
		}
	case "pgdown", "ctrl+d":
		p.scrollY += p.height
		if p.scrollY > maxScroll {
			p.scrollY = maxScroll
		}
	case "pgup", "ctrl+u":
		p.scrollY -= p.height
		if p.scrollY < 0 {
			p.scrollY = 0
		}
	case "g", "home":
		p.scrollY = 0
	case "G", "end":
		p.scrollY = maxScroll
	}
}

// View renders the popup content (label + bordered textarea) without the outer
// rounded border, which is applied by the caller. The label and value border
// mirror the inspector's styling.
func (p CellEditPopup) View() string {
	label := lipgloss.NewStyle().Foreground(colorLabel).Render(p.colName)
	if p.readOnly {
		label += " " + lipgloss.NewStyle().Foreground(colorMuted).Render("(read-only)")
	}

	borderW := p.width + 2 // +2 for the inner " " padding on each side
	bs := lipgloss.NewStyle().Foreground(colorBorder)
	top := bs.Render("┌" + strings.Repeat("─", borderW) + "┐")
	bottom := bs.Render("└" + strings.Repeat("─", borderW) + "┘")

	var lines []string
	lines = append(lines, " "+label, top)

	if p.readOnly {
		lines = append(lines, p.readOnlyLines(bs)...)
		lines = append(lines, bottom)
		return lipgloss.JoinVertical(lipgloss.Left, lines...)
	}

	if p.jsonMode {
		cursorLine := p.ta.Line()
		cursorCol := p.ta.LineInfo().CharOffset
		cursorStyle := lipgloss.NewStyle().Foreground(colorFg).Underline(true)
		for idx, line := range strings.Split(p.ta.Value(), "\n") {
			raw := truncateCell(line, p.width)
			var content string
			if idx == cursorLine {
				runes := []rune(raw)
				if cursorCol < len(runes) {
					content = highlightJSON(string(runes[:cursorCol])) +
						cursorStyle.Render(string(runes[cursorCol])) +
						highlightJSON(string(runes[cursorCol+1:]))
				} else {
					content = highlightJSON(raw) + cursorStyle.Render(" ")
				}
			} else {
				content = highlightJSON(raw)
			}
			lines = append(lines, bs.Render("│ ")+content+bs.Render(" │"))
		}
	} else {
		// The bubbles textarea cursor hardcodes a reverse-video block —
		// cursor.Model.View() appends .Reverse(true), with no public override.
		// Every other style on this textarea is free of reverse, so the only
		// "\x1b[7m" in ta.View() is the cursor; swap it for the insert-mode
		// underline so the popup matches the rest of the app.
		under := sgrPrefix(lipgloss.NewStyle().Foreground(colorFg).Underline(true))
		view := strings.ReplaceAll(p.ta.View(), "\x1b[7m", under)
		for _, line := range strings.Split(view, "\n") {
			lines = append(lines, bs.Render("│ ")+line+bs.Render(" │"))
		}
	}

	lines = append(lines, bottom)
	return lipgloss.JoinVertical(lipgloss.Left, lines...)
}

// readOnlyLines renders the visible window of the value as static,
// non-editable lines (no cursor), padded to a fixed height so the box doesn't
// resize while scrolling.
func (p CellEditPopup) readOnlyLines(bs lipgloss.Style) []string {
	all := strings.Split(p.ta.Value(), "\n")
	start := p.scrollY
	if start < 0 {
		start = 0
	}
	if start > len(all) {
		start = len(all)
	}
	end := start + p.height
	if end > len(all) {
		end = len(all)
	}
	blank := bs.Render("│ ") + strings.Repeat(" ", p.width) + bs.Render(" │")
	var out []string
	for _, line := range all[start:end] {
		raw := truncateCell(line, p.width)
		var content string
		if p.jsonMode {
			content = highlightJSON(raw)
		} else {
			content = lipgloss.NewStyle().Foreground(colorFg).Render(raw)
		}
		out = append(out, bs.Render("│ ")+content+bs.Render(" │"))
	}
	for len(out) < p.height {
		out = append(out, blank)
	}
	return out
}

// sgrPrefix returns the leading SGR escape sequence sty applies, with no
// trailing reset or content. It splices a style's attributes onto a marker
// emitted elsewhere (e.g. replacing the textarea cursor's "\x1b[7m" reverse
// marker with an underline prefix), staying correct across color profiles.
func sgrPrefix(sty lipgloss.Style) string {
	r := sty.Render("\x00")
	i := strings.IndexByte(r, 0)
	if i < 0 {
		return ""
	}
	return r[:i]
}
