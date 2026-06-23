package ui

import (
	"testing"

	"github.com/charmbracelet/lipgloss"
)

func TestRenderTypedConfirmDialogDimensions(t *testing.T) {
	prompt := "Drop database mydb?\nThis permanently deletes every table."
	dialog := renderTypedConfirmDialog(prompt, "mydb", "", 71, 19)
	w := lipgloss.Width(dialog)
	h := lipgloss.Height(dialog)
	if w != 71 {
		t.Errorf("width = %d, want 71", w)
	}
	if h != 19 {
		t.Errorf("height = %d, want 19", h)
	}
}
