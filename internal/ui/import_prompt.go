package ui

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

const maxPathCompletions = 8

// ImportPrompt is a modal text-input overlay for entering a SQL dump file path.
// It provides live filesystem autocompletion: as the user types a path, a
// dropdown lists matching files and directories in the implied directory.
type ImportPrompt struct {
	input   textinput.Model
	visible bool
	width   int
	height  int

	completions  []string
	compSelected int
	compVisible  bool
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
	p.completions = nil
	p.compVisible = false
}

// Hide closes the prompt.
func (p *ImportPrompt) Hide() {
	p.input.Blur()
	p.input.Reset()
	p.visible = false
	p.completions = nil
	p.compVisible = false
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
	return expandTilde(raw)
}

// expandTilde resolves a leading ~ (~ or ~/rest) to the user's home directory.
// Paths without a leading ~ are returned unchanged. The caller is expected to
// have run filepath.Clean first (ExpandPath does; the ex :e/:w handlers do).
func expandTilde(raw string) (string, error) {
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

// splitPathVal splits the input into the directory prefix (including the
// trailing /) and the partial entry name being typed after it.
func splitPathVal(val string) (dir, partial string) {
	idx := strings.LastIndex(val, "/")
	if idx == -1 {
		return "", val
	}
	return val[:idx+1], val[idx+1:]
}

// resolveDir expands ~ in a directory prefix so it can be passed to os.ReadDir.
func resolveDir(dir string) string {
	if dir == "~/" || dir == "~" {
		home, err := os.UserHomeDir()
		if err != nil {
			return dir
		}
		return home
	}
	if len(dir) > 2 && dir[:2] == "~/" {
		home, err := os.UserHomeDir()
		if err != nil {
			return dir
		}
		return filepath.Join(home, dir[2:])
	}
	return dir
}

// refreshCompletions reads the directory implied by the current input and
// populates the completion list with entries whose names start with the
// partial prefix.
func (p *ImportPrompt) refreshCompletions() {
	val := p.input.Value()
	dir, partial := splitPathVal(val)

	if dir == "" {
		p.completions = nil
		p.compVisible = false
		return
	}

	resolved := resolveDir(dir)
	entries, err := os.ReadDir(resolved)
	if err != nil {
		p.completions = nil
		p.compVisible = false
		return
	}

	var matches []string
	for _, e := range entries {
		name := e.Name()
		if !strings.HasPrefix(name, partial) {
			continue
		}
		if strings.HasPrefix(name, ".") && !strings.HasPrefix(partial, ".") {
			continue
		}
		if e.IsDir() {
			name += "/"
		}
		matches = append(matches, name)
	}

	sort.Strings(matches)
	p.completions = matches
	p.compSelected = 0
	p.compVisible = len(matches) > 0
}

// acceptCompletion replaces the partial portion of the input with the
// currently selected completion entry and refreshes completions for the
// new path (so drilling into directories feels natural).
func (p *ImportPrompt) acceptCompletion() {
	if p.compSelected >= len(p.completions) {
		return
	}
	entry := p.completions[p.compSelected]
	val := p.input.Value()
	dir, _ := splitPathVal(val)
	p.input.SetValue(dir + entry)
	p.input.CursorEnd()
	p.refreshCompletions()
}

func (p *ImportPrompt) moveCompletion(delta int) {
	n := len(p.completions)
	if n == 0 {
		return
	}
	p.compSelected = (p.compSelected + delta + n) % n
}

// Update handles keyboard input for the prompt.
func (p ImportPrompt) Update(msg tea.Msg) (ImportPrompt, tea.Cmd) {
	if !p.visible {
		return p, nil
	}

	if key, ok := msg.(tea.KeyMsg); ok {
		switch key.String() {
		case "tab":
			if p.compVisible && len(p.completions) > 0 {
				p.acceptCompletion()
				return p, nil
			}
		case "down":
			if p.compVisible {
				p.moveCompletion(1)
				return p, nil
			}
		case "up":
			if p.compVisible {
				p.moveCompletion(-1)
				return p, nil
			}
		}
	}

	var cmd tea.Cmd
	p.input, cmd = p.input.Update(msg)
	p.refreshCompletions()
	return p, cmd
}

// renderCompletionItems renders the inner content of the dropdown (without
// an outer border — the border is applied by CompletionView so it can be
// sized independently from the base panel).
func (p ImportPrompt) renderCompletionItems() string {
	start := 0
	if p.compSelected >= maxPathCompletions {
		start = p.compSelected - maxPathCompletions + 1
	}
	end := start + maxPathCompletions
	if end > len(p.completions) {
		end = len(p.completions)
	}

	var lines []string
	for i := start; i < end; i++ {
		name := p.completions[i]
		var style lipgloss.Style
		if i == p.compSelected {
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

// CompletionView returns a standalone bordered popup containing the
// filesystem completion dropdown, or "" if no completions are visible.
// It is rendered as a separate overlay so it does not resize the base panel.
func (p ImportPrompt) CompletionView() string {
	if !p.compVisible || len(p.completions) == 0 {
		return ""
	}
	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(colorBorder).
		Render(p.renderCompletionItems())
}

// View renders the prompt panel.
func (p ImportPrompt) View() string {
	if !p.visible {
		return ""
	}

	title := titleStyle.Render("Import SQL Dump")

	inputLine := lipgloss.NewStyle().Foreground(colorLabel).Render("File: ") +
		p.input.View()

	contentLines := []string{title, "", inputLine, "", mutedStyle.Render("Path to a .sql dump file (~ expanded)")}

	content := lipgloss.JoinVertical(lipgloss.Left, contentLines...)

	panel := lipgloss.NewStyle().
		Width(60).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(colorPrimary).
		Padding(1, 2).
		Render(content)

	return panel
}
