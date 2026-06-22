package ui

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// ImportPrompt is a modal text-input overlay for entering a SQL dump file path.
type ImportPrompt struct {
	input   textinput.Model
	visible bool
	width   int
	height  int
}

// NewImportPrompt returns a hidden ImportPrompt.
func NewImportPrompt() ImportPrompt {
	ti := textinput.New()
	ti.Prompt = ""
	ti.Placeholder = "~/Downloads/backup.sql"
	return ImportPrompt{input: ti}
}

// Show opens the prompt, pre-filled with the given default directory.
func (p *ImportPrompt) Show(defaultDir string) {
	p.input.Reset()
	if defaultDir != "" {
		p.input.SetValue(filepath.Join(defaultDir, ""))
	}
	p.input.Focus()
	p.visible = true
}

// Hide closes the prompt.
func (p *ImportPrompt) Hide() {
	p.input.Blur()
	p.input.Reset()
	p.visible = false
}

// IsVisible reports whether the prompt is shown.
func (p ImportPrompt) IsVisible() bool { return p.visible }

// Value returns the current input text.
func (p ImportPrompt) Value() string { return p.input.Value() }

// SetSize sets the rendering dimensions for the prompt panel.
func (p *ImportPrompt) SetSize(width, height int) {
	p.width = width
	p.height = height
}

// ExpandPath resolves ~ to the user's home directory.
func (p ImportPrompt) ExpandPath() (string, error) {
	raw := filepath.Clean(p.input.Value())
	if raw == "" {
		return "", fmt.Errorf("no file path entered")
	}
	if raw == "~" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		return home, nil
	}
	if len(raw) > 2 && raw[:2] == "~/" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		return filepath.Join(home, raw[2:]), nil
	}
	return raw, nil
}

// Update handles keyboard input for the prompt.
func (p ImportPrompt) Update(msg tea.Msg) (ImportPrompt, tea.Cmd) {
	if !p.visible {
		return p, nil
	}
	var cmd tea.Cmd
	p.input, cmd = p.input.Update(msg)
	return p, cmd
}

// View renders the prompt panel.
func (p ImportPrompt) View() string {
	if !p.visible {
		return ""
	}

	title := titleStyle.Render("Import SQL Dump")

	inputLine := lipgloss.NewStyle().Foreground(colorLabel).Render("File: ") +
		p.input.View()

	content := lipgloss.JoinVertical(lipgloss.Left,
		title,
		"",
		inputLine,
		"",
		mutedStyle.Render("Path to a .sql dump file (~ expanded)"),
	)

	panel := lipgloss.NewStyle().
		Width(60).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(colorPrimary).
		Padding(1, 2).
		Render(content)

	return panel
}
