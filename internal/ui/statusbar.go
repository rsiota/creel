package ui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/ruben/gsql/internal/config"
	"github.com/ruben/gsql/internal/db"
)

// popupDim returns the fixed popup dimensions matching the connection form.
func popupDim() (w, h int) {
	return 71, 19
}

func (m Model) connectionInfo(name string) string {
	if m.connection == nil {
		return sbMuted.Render("not connected")
	}
	s := sbSuccess.Render("● " + name)
	if (m.connection.Config().Driver == db.DriverMySQL || m.connection.Config().Driver == db.DriverPostgres) && m.connection.Config().Database != "" {
		s += sbMuted.Render(" / ") + sbLabel.Render(m.connection.Config().Database)
	}
	return s
}

// currentTable returns the table the user is currently working with, if known.
// It prefers the editable results source, then the focused sidebar selection.
func (m Model) currentTable() string {
	if t := m.results.SourceTable(); t != "" {
		return t
	}
	if m.focus == FocusConnections && !m.sidebarFiltering {
		if item := m.currentSidebarItem(); item != nil && !item.isColumn {
			return item.text
		}
	}
	return ""
}

// flashSnapshot captures all transient status-bar fields as an opaque value
// so the Update wrapper can detect whether a message changed them.
func (m Model) flashSnapshot() [7]string {
	return [7]string{
		m.statsMsg, m.exportMsg, m.searchMsg,
		m.truncateMsg, m.deleteRowsMsg, m.schemaMsg,
		m.bookmarkMsg,
	}
}

// flashChanged reports whether any transient field differs from a prior
// snapshot taken by flashSnapshot.
func (m Model) flashChanged(prev [7]string) bool {
	return m.statsMsg != prev[0] ||
		m.exportMsg != prev[1] ||
		m.searchMsg != prev[2] ||
		m.truncateMsg != prev[3] ||
		m.deleteRowsMsg != prev[4] ||
		m.schemaMsg != prev[5] ||
		m.bookmarkMsg != prev[6]
}

// anyFlashActive reports whether any transient status-bar field is non-empty.
func (m Model) anyFlashActive() bool {
	return m.statsMsg != "" ||
		m.exportMsg != "" ||
		m.searchMsg != "" ||
		m.truncateMsg != "" ||
		m.deleteRowsMsg != "" ||
		m.schemaMsg != "" ||
		m.bookmarkMsg != ""
}

// clearFlash empties every transient status-bar field.
func (m *Model) clearFlash() {
	m.statsMsg = ""
	m.exportMsg = ""
	m.searchMsg = ""
	m.truncateMsg = ""
	m.deleteRowsMsg = ""
	m.schemaMsg = ""
	m.bookmarkMsg = ""
}

// statusMessage returns the most relevant transient message for the status bar
// (copy confirmation, save state, errors, pagination), or "" if none.
func (m Model) statusMessage() string {
	switch {
	case m.results.SaveError() != "":
		return sbError.Render(m.results.SaveError())
	case m.results.IsEditing():
		return sbMuted.Render("editing")
	case m.inspector.IsInserting():
		return sbMuted.Render("inserting")
	case m.results.IsCopied():
		return sbSuccess.Render("copied to clipboard")
	case m.results.IsSaved():
		return sbSuccess.Render("saved")
	case m.exportMsg != "":
		return sbSuccess.Render(m.exportMsg)
	case m.truncateMsg != "":
		if strings.HasPrefix(m.truncateMsg, "truncate failed:") {
			return sbError.Render(m.truncateMsg)
		}
		return sbSuccess.Render(m.truncateMsg)
	case m.deleteRowsMsg != "":
		if strings.HasPrefix(m.deleteRowsMsg, "delete failed:") {
			return sbError.Render(m.deleteRowsMsg)
		}
		return sbSuccess.Render(m.deleteRowsMsg)
	case m.schemaMsg != "":
		if strings.HasPrefix(m.schemaMsg, "schema change failed:") {
			return sbError.Render(m.schemaMsg)
		}
		return sbSuccess.Render(m.schemaMsg)
	case m.bookmarkMsg != "":
		return sbSuccess.Render(m.bookmarkMsg)
	case m.statsMsg != "":
		return sbPrimary.Render(m.statsMsg)
	case m.searchMsg != "":
		return sbPrimary.Render(m.searchMsg)
	case m.results.HasDirtyCells():
		return sbMuted.Render(fmt.Sprintf("%d unsaved", m.results.DirtyCellCount()))
	case m.totalRowsSet && m.pageMsg != "":
		return sbMuted.Render(m.pageMsg)
	case m.results.HasResult() && m.results.Message() != "":
		return sbSuccess.Render(m.results.Message())
	case m.pageMsg != "":
		return sbMuted.Render(m.pageMsg)
	}
	return ""
}

// statusBar renders the single-line status bar shown at the bottom of the
// workspace. It carries contextual info (connection, database, table, result
// dimensions, transient messages) plus a single "?" hint for the help overlay.
// All other keybindings live behind the "?" overlay.
func (m Model) statusBar(connName string) string {
	sep := sbMuted.Render(" / ")
	parts := []string{m.connectionInfo(connName)}

	if m.results.IsVisualMode() {
		parts = append(parts, sbAccent.Render(
			fmt.Sprintf("VISUAL %d", m.results.VisualRangeSize())))
	}

	if t := m.currentTable(); t != "" {
		parts = append(parts,
			sbLabel.Render(t))
	}

	// Show row count for the highlighted sidebar table (if available).
	if m.focus == FocusConnections {
		if item := m.currentSidebarItem(); item != nil && !item.isColumn {
			if count, ok := m.tableRowCounts[item.text]; ok && count >= 0 {
				parts = append(parts, sbMuted.Render(fmt.Sprintf("~%s rows", formatCount(int(count)))))
			}
		}
	}

	if n := m.results.MarkCount(); n > 0 {
		parts = append(parts, sbMark.Render(fmt.Sprintf("◆ %d", n)))
	}

	if n := m.results.HiddenCount(); n > 0 {
		parts = append(parts, sbAccent.Render(fmt.Sprintf("⊫ %d", n)))
	}

	if msg := m.statusMessage(); msg != "" {
		parts = append(parts, msg)
	}

	if len(m.filters) > 0 {
		short := make([]string, len(m.filters))
		for i, f := range m.filters {
			short[i] = compactFilter(f)
		}
		parts = append(parts, sbSuccess.Render(strings.Join(short, " ")))
	}

	parts = append(parts, sbLabel.Render("?")+sbMuted.Render(" help"))

	left := strings.Join(parts, sep)

	// Right-align context keybinding hints.
	// The caller prepends a single space, so effective width is m.width-1.
	hints := m.hintList()
	if len(hints) > 0 {
		flashActive := m.hintFlash != "" && time.Since(m.hintFlashAt) < hintFlashDuration
		keyStyle := sbLabel
		flashStyle := sbFg
		sepStyle := sbMuted
		var hintsStyled string
		for i, group := range hints {
			if i > 0 {
				hintsStyled += sepStyle.Render("/")
			}
			if group == "/" {
				if flashActive && group == m.hintFlash {
					hintsStyled += flashStyle.Render(group)
				} else {
					hintsStyled += keyStyle.Render(group)
				}
				continue
			}
			for ki, k := range strings.Split(group, "/") {
				if ki > 0 {
					hintsStyled += sepStyle.Render("/")
				}
				if flashActive && k == m.hintFlash {
					hintsStyled += flashStyle.Render(k)
				} else {
					hintsStyled += keyStyle.Render(k)
				}
			}
		}
		gapW := m.width - 1 - lipgloss.Width(left) - lipgloss.Width(hintsStyled)
		if gapW < 1 {
			gapW = 1
		}
		line := left + sbMuted.Render(strings.Repeat(" ", gapW)) + hintsStyled
		if lipgloss.Width(line) > m.width {
			line = lipgloss.NewStyle().MaxWidth(m.width).Background(colorStatusBarBg).Render(line)
		}
		return line
	}

	if lipgloss.Width(left) > m.width {
		left = lipgloss.NewStyle().MaxWidth(m.width).Background(colorStatusBarBg).Render(left)
	}
	return left
}

func (m Model) borderForFocus(f Focus) lipgloss.Color {
	if m.focus == f {
		return colorPrimary
	}
	return colorBorderUnfocused
}

func copyFlashTickCmd() tea.Cmd {
	return tea.Tick(time.Duration(copyFlashInterval)*time.Millisecond, func(time.Time) tea.Msg {
		return copyFlashTickMsg{}
	})
}

func copyFeedbackCmd() tea.Cmd {
	return tea.Batch(
		copyFlashTickCmd(),
		tea.Tick(time.Duration(copyMessageDuration)*time.Second, func(time.Time) tea.Msg {
			return copyCopiedClearMsg{}
		}),
	)
}

// truncateSidebarLine truncates a rendered (possibly ANSI-styled) string
// to fit within maxVisible visible characters, appending "…" if truncated.
func truncateSidebarLine(line string, maxVisible int) string {
	// Measure visible width by counting non-ANSI characters.
	visible := lipgloss.Width(line)
	if visible <= maxVisible {
		return line
	}

	// lipgloss doesn't have a truncate-with-ansi helper we can use directly,
	// so use its MaxWidth style which handles ANSI-aware truncation.
	return lipgloss.NewStyle().MaxWidth(maxVisible).Render(line)
}

// Run starts the application.
func Run(cfg *config.Config) error {
	m := NewModel(cfg)
	p := tea.NewProgram(m, tea.WithAltScreen(), tea.WithMouseCellMotion())
	if _, err := p.Run(); err != nil {
		return fmt.Errorf("running application: %w", err)
	}
	return nil
}

// historyDir returns the directory for storing query history files.
func historyDir() string {
	configDir := os.Getenv("XDG_CONFIG_HOME")
	if configDir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return filepath.Join(".config", "gsql")
		}
		configDir = filepath.Join(home, ".config")
	}
	return filepath.Join(configDir, "gsql")
}
