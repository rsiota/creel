package ui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// CellEditPopup is a modal multiline editor for cell values whose content is
// too large to edit comfortably inline (truncated with an ellipsis in the grid
// or inspector). It opens a centered textarea large enough to visualize and
// edit the value. On commit the value is staged into the same dirtyCells
// pipeline used by the inline editor (no immediate DB flush).
type CellEditPopup struct {
	buf       VimBuffer
	visible   bool
	readOnly  bool
	scrollY   int
	row       int
	col       int
	colName   string
	width     int
	height    int
	jsonMode  bool
}

// NewCellEditPopup creates a hidden cell-edit popup.
func NewCellEditPopup() CellEditPopup {
	return CellEditPopup{
		buf: NewVimBuffer(VimBufferConfig{
			InitialMode:       VimInsert,
			EnableVisualLine:  false,
			EnableFormatEqual: false,
		}),
	}
}

// Show opens the popup seeded with the given value. JSON objects and arrays
// are automatically pretty-printed and syntax-highlighted for readability.
// When readOnly is true the popup is a view-only peek: the value renders
// statically (no cursor, no edits staged) — used for results that can't be
// written back (read-only mode, custom queries, views without a primary key).
func (p *CellEditPopup) Show(val string, row, col int, colName string, readOnly bool) {
	p.jsonMode = false
	if pretty, ok := formatJSON(val); ok {
		val = pretty
		p.jsonMode = true
	}

	p.buf = NewVimBuffer(VimBufferConfig{
		InitialMode:       VimInsert,
		EnableVisualLine:  false,
		EnableFormatEqual: false,
	})
	p.buf.SetCharLimit(0)
	p.buf.setValueRaw(val)
	if !readOnly {
		p.buf.beginInsert()
	}
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
	p.buf = VimBuffer{}
	p.colName = ""
	p.jsonMode = false
	p.readOnly = false
	p.scrollY = 0
}

func (p CellEditPopup) IsVisible() bool  { return p.visible }
func (p CellEditPopup) IsReadOnly() bool { return p.readOnly }
func (p CellEditPopup) Value() string    { return p.buf.Value() }
func (p CellEditPopup) Row() int         { return p.row }
func (p CellEditPopup) Col() int         { return p.col }
func (p CellEditPopup) VimMode() VimMode { return p.buf.Mode() }
func (p CellEditPopup) VimModeStr() string {
	if p.readOnly {
		return "VIEW"
	}
	return p.buf.ModeStr()
}

// ConsumeEsc handles esc in the cell popup. Read-only closes immediately;
// editable insert → normal; editable normal → close.
func (p *CellEditPopup) ConsumeEsc() (handled bool, shouldClose bool) {
	if !p.visible {
		return false, false
	}
	if p.readOnly {
		return true, true
	}
	return p.buf.ConsumeEsc(true)
}

func (p *CellEditPopup) SetMaxSize(contentW, contentH int) {
	textW := contentW - 4
	if textW < 30 {
		textW = 30
	}
	p.width = textW
	if contentH < 3 {
		contentH = 3
	}
	p.height = contentH
	p.buf.SetWidth(textW)
	p.buf.SetHeight(contentH)
}

func (p *CellEditPopup) Focus() tea.Cmd {
	if !p.visible || p.readOnly {
		return nil
	}
	return p.buf.Focus()
}

func (p CellEditPopup) Update(msg tea.Msg) (CellEditPopup, tea.Cmd) {
	if !p.visible {
		return p, nil
	}
	if p.readOnly {
		p.scrollReadOnly(msg)
		return p, nil
	}
	var cmd tea.Cmd
	p.buf, cmd = p.buf.Update(msg)
	return p, cmd
}

func (p *CellEditPopup) scrollReadOnly(msg tea.Msg) {
	k, ok := msg.(tea.KeyMsg)
	if !ok {
		return
	}
	maxScroll := len(strings.Split(p.buf.Value(), "\n")) - p.height
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

func (p CellEditPopup) View() string {
	label := lipgloss.NewStyle().Foreground(colorLabel).Render(p.colName)
	if p.readOnly {
		label += " " + lipgloss.NewStyle().Foreground(colorMuted).Render("(read-only)")
	} else if mode := p.VimModeStr(); mode != "" {
		label += " " + lipgloss.NewStyle().Foreground(colorMuted).Render("("+strings.ToLower(mode)+")")
	}

	borderW := p.width + 2
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
		lines = append(lines, p.jsonEditLines(bs)...)
	} else {
		lines = append(lines, p.plainEditLines(bs)...)
	}

	lines = append(lines, bottom)
	return lipgloss.JoinVertical(lipgloss.Left, lines...)
}

func (p CellEditPopup) plainEditLines(bs lipgloss.Style) []string {
	view := p.buf.View()
	if p.buf.Mode() == VimInsert {
		under := sgrPrefix(lipgloss.NewStyle().Foreground(colorFg).Underline(true))
		view = strings.ReplaceAll(view, "\x1b[7m", under)
	}
	var out []string
	for _, line := range strings.Split(view, "\n") {
		out = append(out, bs.Render("│ ")+line+bs.Render(" │"))
	}
	return out
}

func (p CellEditPopup) jsonEditLines(bs lipgloss.Style) []string {
	cursorLine := p.buf.Line()
	cursorCol := p.buf.LineInfo().CharOffset
	cursorStyle := lipgloss.NewStyle().Foreground(colorFg)
	if p.buf.Mode() == VimInsert {
		cursorStyle = cursorStyle.Underline(true)
	} else {
		cursorStyle = cursorStyle.Reverse(true)
	}
	var out []string
	for idx, line := range strings.Split(p.buf.Value(), "\n") {
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
		out = append(out, bs.Render("│ ")+content+bs.Render(" │"))
	}
	return out
}

func (p CellEditPopup) readOnlyLines(bs lipgloss.Style) []string {
	all := strings.Split(p.buf.Value(), "\n")
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
// trailing reset or content.
func sgrPrefix(sty lipgloss.Style) string {
	r := sty.Render("\x00")
	i := strings.IndexByte(r, 0)
	if i < 0 {
		return ""
	}
	return r[:i]
}
