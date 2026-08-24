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

type ImportPrompt struct {
	input   textinput.Model
	visible bool
	width   int
	height  int

	pathComp pathCompletion
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
	p.pathComp.clear()
}

// Hide closes the prompt.
func (p *ImportPrompt) Hide() {
	p.input.Blur()
	p.input.Reset()
	p.visible = false
	p.pathComp.clear()
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

// completeFilePath returns the directory entries matching a path prefix — the
// shared engine behind both the import prompt's live completion and the ":"
// file-argument completers (:e/:w/:import/:open/:save). It splits the input
// at the last "/", lists that directory (expanding ~), and returns the
// matching entry basenames — appending "/" to directories, sorting, and
// omitting hidden entries unless the partial itself starts with ".". Returns
// nil when the input has no directory prefix or the directory can't be read.
func completeFilePath(input string) []string {
	dir, partial := splitPathVal(input)
	if dir == "" {
		return nil
	}
	resolved := resolveDir(dir)
	entries, err := os.ReadDir(resolved)
	if err != nil {
		return nil
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
	return matches
}

// refreshCompletions reads the directory implied by the current input and
// populates the completion list with entries whose names start with the
// partial prefix.
func (p *ImportPrompt) refreshCompletions() {
	p.pathComp.refresh(p.input.Value())
}

func (p ImportPrompt) Update(msg tea.Msg) (ImportPrompt, tea.Cmd) {
	if !p.visible {
		return p, nil
	}

	if key, ok := msg.(tea.KeyMsg); ok {
		switch key.String() {
		case "tab":
			if p.pathComp.compVisible && len(p.pathComp.completions) > 0 {
				p.pathComp.accept(&p.input)
				return p, nil
			}
		case "down":
			if p.pathComp.compVisible {
				p.pathComp.move(1)
				return p, nil
			}
		case "up":
			if p.pathComp.compVisible {
				p.pathComp.move(-1)
				return p, nil
			}
		}
	}

	var cmd tea.Cmd
	p.input, cmd = p.input.Update(msg)
	p.refreshCompletions()
	return p, cmd
}

// CompletionView returns a standalone bordered popup containing the
// filesystem completion dropdown, or "" if no completions are visible.
func (p ImportPrompt) CompletionView() string {
	return p.pathComp.View()
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
		Border(panelBorder()).
		BorderForeground(colorPrimary).
		Padding(1, 2).
		Render(content)

	return panel
}
