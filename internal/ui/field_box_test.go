package ui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
)

func TestRenderFieldBoxFitsContentWidth(t *testing.T) {
	contentW := minRightSlotWidth - 2 // narrow inspector interior
	label := lipgloss.NewStyle().Bold(true).Render("* very_long_column_name ●")
	marker := lipgloss.NewStyle().Foreground(colorMuted).Render("varchar")
	val := truncateCell(strings.Repeat("x", 80), contentW-4)
	box := renderFieldBox(label, marker, lipgloss.NewStyle().Render(val), contentW, fieldBoxBorder(true))
	if got := lipgloss.Width(box); got != contentW {
		t.Fatalf("box width=%d, want %d", got, contentW)
	}
	for i, ln := range strings.Split(box, "\n") {
		if w := lipgloss.Width(ln); w > contentW {
			t.Errorf("line %d width=%d > contentW=%d", i, w, contentW)
		}
	}
}

func TestWorkspaceFitsAtMinRightSlotWithLongColumns(t *testing.T) {
	m := newGeomModel(120, 40)
	m.inspector.Toggle()
	m.rightSlotSplitW = minRightSlotWidth
	m.results.SetResult(
		[]string{"very_long_column_name", "another_quite_long_column"},
		[][]string{{"1", "2"}},
		"1 row",
	)
	m.layoutWorkspace()
	view := m.viewWorkspace()
	if got := lipgloss.Width(view); got != m.width {
		t.Errorf("view width=%d, want %d", got, m.width)
	}
	if got := lipgloss.Height(view); got != m.height {
		t.Errorf("view height=%d, want %d", got, m.height)
	}
}
