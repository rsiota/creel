package ui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/rsiota/creel/internal/config"
	"github.com/rsiota/creel/internal/db"
)

// popupDim returns the fixed popup dimensions matching the connection form.
func popupDim() (w, h int) {
	return 71, 19
}

func (m Model) connectionInfo(name string) string {
	ro := ""
	if m.isReadOnly() {
		ro = " " + sbAccent.Render("READ-ONLY")
	}
	if m.connection == nil {
		return sbMuted.Render("not connected") + ro
	}
	s := sbSuccess.Render("● "+name) + ro
	if (m.connection.Config().Driver == db.DriverMySQL || m.connection.Config().Driver == db.DriverPostgres) && m.connection.Config().Database != "" {
		s += sbMuted.Render(" / ") + sbLabel.Render(m.connection.Config().Database)
	}
	return s
}

// txnStatusLabel is the status-bar token for an open manual transaction.
func txnStatusLabel(level db.IsolationLevel) string {
	if short := level.Short(); short != "" {
		return "TXN " + short
	}
	return "TXN ●"
}

// isReadOnly reports whether the active connection (or the global --readonly
// flag) disables writes, for status-bar indication.
func (m Model) isReadOnly() bool {
	return m.forceReadOnly || (m.connection != nil && m.connection.Config().ReadOnly)
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
func (m Model) flashSnapshot() [8]string {
	return [8]string{
		m.statsMsg, m.exportMsg, m.searchMsg,
		m.truncateMsg, m.deleteRowsMsg, m.schemaMsg,
		m.bookmarkMsg, m.aiMsg,
	}
}

// flashChanged reports whether any transient field differs from a prior
// snapshot taken by flashSnapshot.
func (m Model) flashChanged(prev [8]string) bool {
	return m.statsMsg != prev[0] ||
		m.exportMsg != prev[1] ||
		m.searchMsg != prev[2] ||
		m.truncateMsg != prev[3] ||
		m.deleteRowsMsg != prev[4] ||
		m.schemaMsg != prev[5] ||
		m.bookmarkMsg != prev[6] ||
		m.aiMsg != prev[7]
}

// anyFlashActive reports whether any transient status-bar field is non-empty.
func (m Model) anyFlashActive() bool {
	return m.statsMsg != "" ||
		m.exportMsg != "" ||
		m.searchMsg != "" ||
		m.truncateMsg != "" ||
		m.deleteRowsMsg != "" ||
		m.schemaMsg != "" ||
		m.bookmarkMsg != "" ||
		m.aiMsg != ""
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
	m.aiMsg = ""
}

// statusMessage returns the most relevant transient message for the status bar
// (copy confirmation, save state, errors, pagination), or "" if none.
func (m Model) statusMessage() string {
	// The AI pending hint takes priority — it is the most actionable status
	// while a model request is in flight. The spinner + live elapsed timer
	// make it clear the request is in progress (not frozen) and that esc
	// cancels; reasoning models can take 10-20s.
	if m.aiRunning {
		frame := spinnerFrames[m.querySpinner%len(spinnerFrames)]
		elapsed := time.Since(m.aiStart).Round(time.Second)
		return sbPrimary.Render(frame) +
			sbMuted.Render(fmt.Sprintf(" asking model… %s (esc to cancel)", elapsed))
	}
	switch {
	case m.reconnecting:
		frame := spinnerFrames[m.querySpinner%len(spinnerFrames)]
		return sbPrimary.Render(frame) + sbMuted.Render(" reconnecting…")
	case m.state == stateConnections && m.connError != "":
		return sbError.Render(m.connError)
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
	case m.chartPanel.IsVisible() && m.chartPanel.title != "":
		return sbMuted.Render(m.chartPanel.title)
	case m.bookmarkMsg != "":
		return sbSuccess.Render(m.bookmarkMsg)
	case m.statsMsg != "":
		return sbPrimary.Render(m.statsMsg)
	case m.searchMsg != "":
		return sbPrimary.Render(m.searchMsg)
	case m.aiMsg != "":
		if strings.HasPrefix(m.aiMsg, "ai failed:") {
			return sbError.Render(m.aiMsg)
		}
		return sbSuccess.Render(m.aiMsg)
	case m.results.HasDirtyCells():
		return sbMuted.Render(fmt.Sprintf("%d unsaved", m.results.DirtyCellCount()))
	case m.totalRowsSet && m.pageMsg != "":
		return sbMuted.Render(m.pageMsg)
	case m.results.HasResult() && m.results.NumCols() > 0 && m.results.Message() != "":
		// Only surface success/status messages (e.g. "42 rows") here. Query
		// errors live in the results panel via SetError — echoing them on the
		// status bar duplicates a long MySQL/Postgres message and wraps the
		// one-line bar, shoving the workspace up a row.
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

	if m.focus == FocusEditor && !m.cellEdit.IsVisible() {
		if mode := m.editor.VimModeStr(); mode != "" {
			parts = append(parts, sbAccent.Render(mode))
		}
	}

	if m.tx != nil {
		parts = append(parts, sbAccent.Render(txnStatusLabel(m.txIsolation)))
	}

	if label := m.paramStatusLabel(); label != "" {
		parts = append(parts, sbAccent.Render(label))
	}

	if m.watchActive {
		label := "WATCH"
		if m.watchMode == "tail" {
			label = "TAIL"
		}
		parts = append(parts, sbAccent.Render(
			fmt.Sprintf("%s %s", label, humanDuration(m.watchInterval))))
	}

	if m.showTiming && m.lastQueryElapsed > 0 {
		parts = append(parts, sbAccent.Render(
			fmt.Sprintf("%.3fs", m.lastQueryElapsed.Seconds())))
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

	if cols := m.results.MarkedColumns(); len(cols) > 0 {
		names := make([]string, len(cols))
		for i, c := range cols {
			names[i] = m.results.ColumnName(c)
		}
		parts = append(parts, sbMark.Render("▣ "+strings.Join(names, " › ")))
	}

	if n := m.results.HiddenCount(); n > 0 {
		parts = append(parts, sbAccent.Render(fmt.Sprintf("⊫ %d", n)))
	}

	if msg := m.statusMessage(); msg != "" {
		parts = append(parts, msg)
	}

	if m.state == stateConnections && !m.connList.HasSavedConnections() {
		parts = append(parts, sbMuted.Render("ctrl+p jump · ? help · : commands"))
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
		keyStyle := sbMuted
		flashStyle := sbHintFlash
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
		// When a key was just pressed, show its registry description briefly,
		// tucked just left of the hints (so it reads next to the keys). It is
		// truncated to whatever room remains after the left block + hints,
		// keeping at least a 1-col gap and a 2-col separator before the hints.
		// Expiry is render-time (like the key flash), so no timer is needed.
		var descStyled string
		if m.hintDesc != "" && time.Since(m.hintDescAt) < hintDescDuration {
			avail := m.width - 1 - lipgloss.Width(left) - lipgloss.Width(hintsStyled) - 2 - 1
			if avail >= 3 {
				descStyled = sbFg.Render(truncateRunes(m.hintDesc, avail)) + sbMuted.Render("  ")
			}
		}
		gapW := m.width - 1 - lipgloss.Width(left) - lipgloss.Width(descStyled) - lipgloss.Width(hintsStyled)
		if gapW < 1 {
			gapW = 1
		}
		line := left + sbMuted.Render(strings.Repeat(" ", gapW)) + descStyled + hintsStyled
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

// commandLine renders the bottom command/message line: the active ":" ex
// prompt, the "/" in-table search, or the backend-search prompt. It occupies a
// single full-width row directly below the status bar and returns "" when no
// prompt is active, so it adds no height at rest. This mirrors vim/helix,
// where the command line lives at the very bottom of the screen, rather than
// being wedged at the top of the results panel.
func (m Model) commandLine() string {
	var content string
	switch {
	case m.ex.visible:
		content = m.ex.View()
	case m.searching:
		content = lipgloss.NewStyle().Foreground(colorPrimary).Render("/"+m.searchQuery) +
			lipgloss.NewStyle().Foreground(colorAccent).Underline(true).Render(" ")
	case m.backendSearching:
		content = renderPalettePrompt(m.backendSearchInput, true)
	default:
		return ""
	}
	// No explicit background: paintBg fills the row with the theme bg (and
	// stays transparent under transparent_background), matching the workspace
	// panels rather than the status bar's distinct bg.
	return lipgloss.NewStyle().Width(m.width).Height(1).Render(" " + content)
}

func (m Model) borderForFocus(f Focus) lipgloss.Color {
	if m.focus == f || (f == FocusEditor && m.focus == FocusTabBar) {
		return colorPrimary
	}
	return colorBorderUnfocused
}

func copyFlashTickCmd() tea.Cmd {
	return tea.Tick(time.Duration(copyFlashInterval)*time.Millisecond, func(time.Time) tea.Msg {
		return copyFlashTickMsg{}
	})
}

// wheelTickInterval caps how often a coalesced wheel scroll is applied (and
// thus how often the results grid re-renders during a momentum-scroll flood).
// ~16ms ≈ one frame: fast enough to feel responsive, slow enough that the
// renderer never falls behind the event rate.
const wheelTickInterval = 16 * time.Millisecond

// scheduleWheelTick arms the timer that flushes the accumulated wheel delta.
// At most one is in flight at a time (guarded by Model.wheelTickPending).
func scheduleWheelTick() tea.Cmd {
	return tea.Tick(wheelTickInterval, func(time.Time) tea.Msg {
		return wheelTickMsg{}
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

// Run starts the interactive TUI. If startupFile is non-empty it is loaded
// into the editor before the program starts (the `creel -f` flag); a read
// failure fails fast with a wrapped error before any UI is shown.
//
// If startupConn is non-nil, creel opens that connection immediately and
// enters the workspace (the `creel -database …` / `creel -c …` path) instead
// of showing the connection picker. A connect failure fails fast the same way.
func Run(cfg *config.Config, forceReadOnly bool, startupFile string, startupConn *db.ConnectionConfig) error {
	m := NewModel(cfg)
	m.forceReadOnly = forceReadOnly
	if startupFile != "" {
		expanded, err := m.loadStartupFile(startupFile)
		if err != nil {
			return fmt.Errorf("loading %s: %w", startupFile, err)
		}
		m.schemaMsg = fmt.Sprintf("loaded %s — press ctrl+e to run", expanded)
	}
	if startupConn != nil {
		cmd := m.connectWithConfig(*startupConn)
		if m.connError != "" {
			return fmt.Errorf("connecting to %s: %s", startupConn.Database, m.connError)
		}
		m.startupCmd = cmd
	}
	// WithMouseAllMotion (not just cell motion) so the ERD can show hover
	// tooltips: button-less motion events are what surface a card's columns
	// without a click. Cell motion alone only reports motion while a button is
	// held (drag). See docs/tui-mouse.md for the trade-offs.
	p := tea.NewProgram(m, tea.WithAltScreen(), tea.WithMouseAllMotion())
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
			return filepath.Join(".config", "creel")
		}
		configDir = filepath.Join(home, ".config")
	}
	return filepath.Join(configDir, "creel")
}
