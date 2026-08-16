package ui

import (
	"strings"

	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/lipgloss"
	"github.com/rivo/uniseg"
)

type editorDisplayLine struct {
	segment    string
	cursorLine bool
	cursorCol  int
	hasCursor  bool
	visual     bool
}

func (e *QueryEditor) syncViewOffset() {
	cur := e.cursorDisplayLine()
	min := e.viewYOffset
	max := min + e.height - 1
	if e.height <= 0 {
		return
	}
	if cur < min {
		e.viewYOffset = cur
	}
	if cur > max {
		e.viewYOffset = cur - e.height + 1
	}
	if e.viewYOffset < 0 {
		e.viewYOffset = 0
	}
}

func (e QueryEditor) cursorDisplayLine() int {
	line := 0
	lines := strings.Split(e.textarea.Value(), "\n")
	cursorRow := e.textarea.Line()
	info := e.textarea.LineInfo()
	width := e.textarea.Width()

	for i := 0; i < cursorRow && i < len(lines); i++ {
		line += len(softWrapRunes([]rune(lines[i]), width))
	}
	line += info.RowOffset
	return line
}

func (e QueryEditor) editorStyle() *textarea.Style {
	if e.textarea.Focused() {
		return &e.textarea.FocusedStyle
	}
	return &e.textarea.BlurredStyle
}

func inheritStyle(base, s lipgloss.Style) lipgloss.Style {
	return s.Inherit(base).Inline(true)
}

func (e QueryEditor) highlightedView() string {
	ta := e.textarea
	style := e.editorStyle()

	if ta.Value() == "" && ta.Line() == 0 {
		return e.highlightedPlaceholderView(style)
	}

	displayLines := e.buildDisplayLines()
	bufHeight := e.height
	if e.searching && bufHeight > 1 {
		bufHeight--
	}

	var s strings.Builder
	for i := 0; i < bufHeight; i++ {
		displayIdx := e.viewYOffset + i
		prompt := inheritStyle(style.Base, style.Prompt).Render(ta.Prompt)

		if displayIdx < len(displayLines) {
			dl := displayLines[displayIdx]
			lineStyle := inheritStyle(style.Base, style.Text)
			if dl.cursorLine {
				lineStyle = inheritStyle(style.Base, style.CursorLine)
			}

			s.WriteString(lineStyle.Render(prompt))

			plain := dl.segment
			plainWidth := uniseg.StringWidth(plain)
			visualStyle := lipgloss.NewStyle().Background(colorPrimary).Foreground(colorBg)

			if dl.hasCursor && e.Focused() {
				segRunes := []rune(plain)
				absCol := dl.cursorCol
				if absCol > len(segRunes) {
					absCol = len(segRunes)
				}

				left := highlightSubstring(plain, 0, absCol)
				if dl.visual {
					left = visualStyle.Render(string(segRunes[:absCol]))
				}
				s.WriteString(left)

				cursorChar := " "
				if absCol < len(segRunes) {
					cursorChar = string(segRunes[absCol])
				}

				cursorStyle := lipgloss.NewStyle()
				if e.vimMode == VimInsert {
					cursorStyle = cursorStyle.Underline(true).Foreground(colorFg)
				} else {
					cursorStyle = cursorStyle.Reverse(true)
				}
				s.WriteString(cursorStyle.Render(cursorChar))

				right := highlightSubstring(plain, absCol+1, len(segRunes))
				if dl.visual && absCol+1 < len(segRunes) {
					right = visualStyle.Render(string(segRunes[absCol+1:]))
				}
				s.WriteString(right)
			} else if dl.visual {
				s.WriteString(visualStyle.Render(plain))
			} else {
				s.WriteString(highlightSegment(plain))
			}

			padding := ta.Width() - plainWidth
			if padding > 0 {
				s.WriteString(lineStyle.Render(strings.Repeat(" ", padding)))
			}
		} else {
			s.WriteString(prompt)
			eob := inheritStyle(style.Base, style.EndOfBuffer).Render(string(ta.EndOfBufferCharacter))
			rightGap := strings.Repeat(" ", max(0, ta.Width()-lipgloss.Width(eob)))
			s.WriteString(inheritStyle(style.Base, style.EndOfBuffer).Render(eob + rightGap))
		}
		s.WriteRune('\n')
	}

	if e.searching {
		s.WriteString(e.searchPrompt(ta.Width() + 1))
	}

	return style.Base.Render(strings.TrimSuffix(s.String(), "\n"))
}

func (e QueryEditor) highlightedPlaceholderView(style *textarea.Style) string {
	var s strings.Builder

	for i := 0; i < e.height; i++ {
		lineStyle := inheritStyle(style.Base, style.CursorLine)

		prompt := inheritStyle(style.Base, style.Prompt).Render(e.textarea.Prompt)
		s.WriteString(lineStyle.Render(prompt))

		if i == 0 && e.Focused() {
			cursorStyle := lipgloss.NewStyle()
			if e.vimMode == VimInsert {
				cursorStyle = cursorStyle.Underline(true).Foreground(colorFg)
			} else {
				cursorStyle = cursorStyle.Reverse(true)
			}
			s.WriteString(cursorStyle.Render(" "))
			s.WriteString(lineStyle.Render(strings.Repeat(" ", e.textarea.Width()-1)))
		} else {
			s.WriteString(lineStyle.Render(strings.Repeat(" ", e.textarea.Width())))
		}
		s.WriteRune('\n')
	}

	return style.Base.Render(strings.TrimSuffix(s.String(), "\n"))
}

func (e QueryEditor) buildDisplayLines() []editorDisplayLine {
	ta := e.textarea
	lines := strings.Split(ta.Value(), "\n")
	if len(lines) == 0 {
		lines = []string{""}
	}

	cursorRow := ta.Line()
	info := ta.LineInfo()
	width := ta.Width()
	visLo, visHi := e.visualRange()

	var out []editorDisplayLine
	for lineIdx, line := range lines {
		wrapped := softWrapRunes([]rune(line), width)

		for wrapIdx, segment := range wrapped {
			dl := editorDisplayLine{
				segment:    string(segment),
				cursorLine: lineIdx == cursorRow,
				visual:     e.visualLine && lineIdx >= visLo && lineIdx <= visHi,
			}

			if lineIdx == cursorRow && info.RowOffset == wrapIdx {
				dl.hasCursor = true
				dl.cursorCol = info.ColumnOffset
			}

			out = append(out, dl)
		}
	}
	return out
}
