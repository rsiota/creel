package ui

import (
	"fmt"
	"log"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/ruben/gsql/internal/config"
	"github.com/ruben/gsql/internal/db"
)

// Focus represents which panel currently has keyboard focus.
type Focus int

const (
	FocusConnections Focus = iota
	FocusEditor
	FocusResults
)

// state represents the current screen the app is showing.
type state int

const (
	stateConnections state = iota
	stateWorkspace
	stateAddConnection
)

// executeResultMsg carries the result of an async query execution.
type executeResultMsg struct {
	result db.Result
	err    error
}

// queryExecutedMsg is sent when a query finishes executing.
type queryExecutedMsg struct {
	result db.Result
	err    error
}

// Model is the top-level application model for the Bubble Tea architecture.
type Model struct {
	state    state
	focus    Focus
	width    int
	height   int
	quitting bool

	connList    ConnectionList
	connForm    ConnectionForm
	editor      QueryEditor
	results     ResultsTable
	tableScroll int

	config     *config.Config
	connection *db.Connection
	connError  string
	tables     []string
}

// NewModel creates a new top-level application model.
func NewModel(cfg *config.Config) Model {
	m := Model{
		state:    stateConnections,
		focus:    FocusConnections,
		config:   cfg,
		editor:   NewQueryEditor(),
		results:  NewResultsTable(),
		connList: NewConnectionList(),
	}
	m.loadConnections()
	return m
}

func (m *Model) loadConnections() {
	var entries []ConnectionEntry
	for _, conn := range m.config.Connections {
		detail := conn.Database
		if conn.Driver == "mysql" {
			detail = fmt.Sprintf("%s@%s:%d/%s", conn.Username, conn.Host, conn.Port, conn.Database)
		}
		entries = append(entries, ConnectionEntry{
			Name:   conn.Name,
			Driver: conn.Driver,
			Detail: detail,
		})
	}
	m.connList.SetItems(entries)
}

// Init initializes the application.
func (m Model) Init() tea.Cmd {
	return nil
}

// connectToDB establishes a connection to the selected database.
func (m *Model) connectToDB() tea.Cmd {
	name := m.connList.SelectedName()
	driver := m.connList.SelectedDriver()
	connCfg := m.config.GetConnection(name)
	if connCfg == nil {
		m.connError = fmt.Sprintf("connection '%s' not found", name)
		return nil
	}

	dbCfg := db.ConnectionConfig{
		Name:     connCfg.Name,
		Driver:   db.Driver(driver),
		Database: connCfg.Database,
		Host:     connCfg.Host,
		Port:     connCfg.Port,
		Username: connCfg.Username,
		Password: connCfg.Password,
	}

	conn, err := db.New(dbCfg)
	if err != nil {
		m.connError = err.Error()
		return nil
	}

	if err := conn.Connect(); err != nil {
		m.connError = err.Error()
		return nil
	}

	m.connection = conn
	m.state = stateWorkspace
	m.focus = FocusEditor

	cmd := m.editor.Focus()
	m.loadTables()
	m.layoutWorkspace()

	return cmd
}

func (m *Model) loadTables() {
	if m.connection == nil {
		return
	}
	tables, err := m.connection.DB().Tables()
	if err != nil {
		m.connError = err.Error()
		return
	}
	m.tables = tables
}

// executeQuery runs the current query asynchronously.
func (m *Model) executeQuery() tea.Cmd {
	query := m.editor.FormatQuery()
	if query == "" {
		return nil
	}

	conn := m.connection
	return func() tea.Msg {
		result, err := conn.DB().Execute(query)
		return queryExecutedMsg{result: result, err: err}
	}
}

// Update handles all application messages.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m = m.updateLayout()
		return m, nil

	case tea.KeyMsg:
		switch {
		case key.Matches(msg, key.NewBinding(key.WithKeys("ctrl+c"))):
			m.quitting = true
			return m, tea.Quit
		case key.Matches(msg, key.NewBinding(key.WithKeys("ctrl+q"))):
			m.quitting = true
			return m, tea.Quit
		}

		if m.state == stateConnections {
			return m.updateConnections(msg)
		}
		if m.state == stateAddConnection {
			return m.updateAddConnection(msg)
		}
		return m.updateWorkspace(msg)

	case queryExecutedMsg:
		m.layoutWorkspace()
		if msg.err != nil {
			m.results.SetError(msg.err.Error())
		} else {
			cols := make([]string, len(msg.result.Columns))
			for i, c := range msg.result.Columns {
				cols[i] = c.Name
			}
			m.results.SetResult(cols, msg.result.Rows, msg.result.Message)
		}
		return m, nil
	}

	if m.state == stateWorkspace {
		var cmd tea.Cmd
		switch m.focus {
		case FocusEditor:
			m.editor, cmd = m.editor.Update(msg)
		case FocusResults:
			m.results, cmd = m.results.Update(msg)
		}
		return m, cmd
	}

	if m.state == stateAddConnection {
		var cmd tea.Cmd
		m.connForm, cmd = m.connForm.Update(msg)
		return m, cmd
	}

	return m, nil
}

func (m Model) updateConnections(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "enter":
		return m, m.connectToDB()
	case "n":
		m.state = stateAddConnection
		m.connForm = NewConnectionForm()
		cmd := m.connForm.Focus()
		return m, cmd
	case "e":
		return m.openEditForm()
	case "d":
		return m.deleteSelectedConnection()
	case "esc":
		if m.connList.list.FilterState() == list.Filtering {
			break
		}
		m.quitting = true
		return m, tea.Quit
	}

	var cmd tea.Cmd
	m.connList, cmd = m.connList.Update(msg)
	return m, cmd
}

// addDefaultSQLiteConnection creates a quick local SQLite connection for convenience.
func (m Model) addDefaultSQLiteConnection() (tea.Model, tea.Cmd) {
	// For now just demonstrate; full add-connection UI is a future slice.
	return m, nil
}

func (m Model) openEditForm() (tea.Model, tea.Cmd) {
	name := m.connList.SelectedName()
	if name == "" {
		return m, nil
	}
	existing := m.config.GetConnection(name)
	if existing == nil {
		return m, nil
	}
	m.state = stateAddConnection
	m.connForm = NewConnectionFormEdit(*existing)
	cmd := m.connForm.Focus()
	return m, cmd
}

func (m Model) deleteSelectedConnection() (tea.Model, tea.Cmd) {
	name := m.connList.SelectedName()
	if name == "" {
		return m, nil
	}
	m.config.RemoveConnection(name)
	if err := m.config.Save(); err != nil {
		m.connError = err.Error()
		return m, nil
	}
	m.connError = ""
	m.loadConnections()
	return m, nil
}

func (m Model) updateAddConnection(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.state = stateConnections
		m.connError = ""
		return m, nil
	case "enter":
		connCfg, errMsg := m.connForm.EnterPressed()
		if errMsg != "" {
			m.connForm.SetError(errMsg)
			return m, nil
		}

		if m.connForm.mode == formModeEdit {
			m.config.RemoveConnection(m.connForm.editing)
		}

		m.config.AddConnection(connCfg)
		if err := m.config.Save(); err != nil {
			m.connForm.SetError(err.Error())
			return m, nil
		}

		m.state = stateConnections
		m.connError = ""
		m.loadConnections()
		return m, nil
	}

	var cmd tea.Cmd
	m.connForm, cmd = m.connForm.Update(msg)
	return m, cmd
}

func (m Model) updateWorkspace(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// Ensure panels are correctly sized (handles state transitions where
	// WindowSizeMsg hasn't re-fired).
	m.layoutWorkspace()

	// Global workspace keys
	switch msg.String() {
	case "ctrl+enter", "ctrl+j", "f5":
		return m, m.executeQuery()
	case "ctrl+r":
		m.editor.Reset()
		return m, nil
	case "tab":
		m = m.cycleFocus()
		return m, nil
	case "shift+tab":
		m = m.cycleFocusBack()
		return m, nil
	case "ctrl+t":
		// Return to connection screen
		if m.connection != nil {
			m.connection.Close()
			m.connection = nil
		}
		m.state = stateConnections
		m.focus = FocusConnections
		m.results.Clear()
		m.loadConnections()
		return m, nil
	case "esc":
		return m.escapeWorkspace()
	}

	// Dispatch to focused panel
	var cmd tea.Cmd
	switch m.focus {
	case FocusEditor:
		// Handle ctrl+arrow for result scrolling while in editor
		switch msg.String() {
		case "ctrl+up":
			m.results.ScrollUp()
			return m, nil
		case "ctrl+down":
			m.results.ScrollDown()
			return m, nil
		case "ctrl+left":
			m.results.ScrollLeft()
			return m, nil
		case "ctrl+right":
			m.results.ScrollRight()
			return m, nil
		}
		m.editor, cmd = m.editor.Update(msg)
	case FocusResults:
		switch msg.String() {
		case "up", "k":
			m.results.ScrollUp()
			return m, nil
		case "down", "j":
			m.results.ScrollDown()
			return m, nil
		case "left", "h":
			m.results.ScrollLeft()
			return m, nil
		case "right", "l":
			m.results.ScrollRight()
			return m, nil
		}
		m.results, cmd = m.results.Update(msg)
	case FocusConnections:
		switch msg.String() {
		case "up", "k":
			m.scrollTables(-1)
			return m, nil
		case "down", "j":
			m.scrollTables(1)
			return m, nil
		case "enter":
			tableName := m.tables[m.tableScroll]
			m.editor.SetValue(fmt.Sprintf("SELECT * FROM %s LIMIT 100;", tableName))
			m.focus = FocusEditor
			m.applyFocus()
			return m, m.editor.Focus()
		}
		m.connList, cmd = m.connList.Update(msg)
	}
	return m, cmd
}

func (m Model) escapeWorkspace() (tea.Model, tea.Cmd) {
	if m.focus == FocusEditor {
		m.editor.Blur()
	}
	return m, nil
}

func (m Model) scrollTables(delta int) Model {
	total := len(m.tables)
	sidebarHeight := m.height - 1
	maxVisible := sidebarHeight - 4
	if maxVisible < 1 {
		maxVisible = 1
	}

	m.tableScroll += delta
	if m.tableScroll < 0 {
		m.tableScroll = 0
	}
	if m.tableScroll > total-1 {
		m.tableScroll = total - 1
	}
	if m.tableScroll > total-maxVisible && total > maxVisible {
		m.tableScroll = total - maxVisible
	}
	return m
}

func (m Model) cycleFocus() Model {
	m.focus++
	if m.focus > FocusResults {
		m.focus = FocusConnections
	}
	m.applyFocus()
	return m
}

func (m Model) cycleFocusBack() Model {
	m.focus--
	if m.focus < FocusConnections {
		m.focus = FocusResults
	}
	m.applyFocus()
	return m
}

func (m Model) applyFocus() Model {
	m.editor.Blur()
	switch m.focus {
	case FocusEditor:
		m.editor.Focus()
	}
	return m
}

func (m Model) updateLayout() Model {
	if m.width == 0 || m.height == 0 {
		return m
	}

	if m.state == stateConnections {
		m.connList.SetSize(m.width, m.height)
		return m
	}

	if m.state == stateAddConnection {
		m.connForm.SetSize(m.width, m.height)
		return m
	}

	m.layoutWorkspace()
	return m
}

// layoutWorkspace sizes the workspace panels. Uses pointer receiver so it
// works correctly when called from both value and pointer receiver methods.
func (m *Model) layoutWorkspace() {
	sidebarWidth := 30
	statusHeight := 1
	borderOverhead := 2
	editorHeight := 8

	resultsHeight := m.height - editorHeight - statusHeight - (borderOverhead * 2)
	if resultsHeight < 3 {
		resultsHeight = 3
	}

	sidebarHeight := m.height - statusHeight - borderOverhead
	if sidebarHeight < 3 {
		sidebarHeight = 3
	}

	editorContentHeight := editorHeight - borderOverhead
	if editorContentHeight < 1 {
		editorContentHeight = 1
	}

	m.connList.SetSize(sidebarWidth-borderOverhead, sidebarHeight)
	m.editor.SetSize(m.width-sidebarWidth-borderOverhead, editorContentHeight)
	m.results.SetSize(m.width-sidebarWidth-borderOverhead, resultsHeight)
}

// View renders the entire application.
func (m Model) View() string {
	if m.quitting {
		return ""
	}

	if m.width == 0 {
		return "Loading..."
	}

	if m.state == stateAddConnection {
		return m.viewAddConnection()
	}

	if m.state == stateConnections {
		return m.viewConnections()
	}

	return m.viewWorkspace()
}

func (m Model) viewConnections() string {
	if m.connError != "" {
		return errorStyle.Render(m.connError)
	}

	header := titleStyle.Render("gsql") + mutedStyle.Render("  — a fast SQL TUI")
	body := m.connList.View()
	footer := helpStyle.Render("enter: connect  n: new  e: edit  d: delete  esc: quit  /: filter")

	return lipgloss.JoinVertical(lipgloss.Left,
		appStyle.Render(header),
		appStyle.Render(body),
		footer,
	)
}

func (m Model) viewAddConnection() string {
	return appStyle.Render(
		lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(colorBorder).
			Padding(1, 2).
			Render(m.connForm.View()),
	)
}

func (m Model) viewWorkspace() string {
	sidebarWidth := 30
	statusHeight := 1
	borderOverhead := 2
	editorHeight := 8
	// Editor and results are adjacent; their touching borders create a
	// 2-line visual gap matching the 2-char horizontal gap.
	resultsHeight := m.height - editorHeight - statusHeight - (borderOverhead * 2)
	if resultsHeight < 3 {
		resultsHeight = 3
	}
	sidebarHeight := m.height - statusHeight - borderOverhead

	sidebarTitle := titleStyle.Render("Tables")

	maxVisible := sidebarHeight - 4
	if maxVisible < 1 {
		maxVisible = 1
	}

	totalTables := len(m.tables)
	start := m.tableScroll
	end := start + maxVisible
	if end > totalTables {
		end = totalTables
	}
	if start > totalTables-maxVisible && totalTables > maxVisible {
		start = totalTables - maxVisible
	}
	if start < 0 {
		start = 0
	}

	tableList := strings.Builder{}
	for i := start; i < end; i++ {
		cursor := "  "
		style := normalStyle
		if m.focus == FocusConnections && i == m.tableScroll {
			cursor = "→ "
			style = selectedStyle
		}
		tableList.WriteString(style.Render(cursor + m.tables[i]))
		tableList.WriteString("\n")
	}
	if totalTables == 0 {
		tableList.WriteString(mutedStyle.Render("  (no tables)"))
	}

	scrollInfo := ""
	if totalTables > maxVisible {
		scrollInfo = mutedStyle.Render(fmt.Sprintf(" %d-%d of %d", start+1, end, totalTables))
	}

	sidebar := lipgloss.NewStyle().
		Width(sidebarWidth - borderOverhead).
		Height(sidebarHeight).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(colorBorder).
		Render(
			lipgloss.JoinVertical(lipgloss.Left, sidebarTitle, tableList.String(), scrollInfo),
		)

	editorTitle := titleStyle.Render("Query")
	modeIndicator := mutedStyle.Render(fmt.Sprintf("[%s]", m.editor.VimModeStr()))
	editorContent := lipgloss.JoinVertical(lipgloss.Left,
		editorTitle+"  "+modeIndicator,
		m.editor.View(),
	)
	editorPanel := lipgloss.NewStyle().
		Width(m.width - sidebarWidth - borderOverhead).
		Height(editorHeight - borderOverhead).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(m.borderForFocus(FocusEditor)).
		Render(editorContent)

	resultsTitle := titleStyle.Render("Results")
	resultsContent := lipgloss.JoinVertical(lipgloss.Left,
		resultsTitle,
		m.results.View(),
	)
	resultsPanel := lipgloss.NewStyle().
		Width(m.width - sidebarWidth - borderOverhead).
		Height(resultsHeight).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(m.borderForFocus(FocusResults)).
		Render(resultsContent)

	connName := ""
	if m.connection != nil {
		connName = m.connection.Config().Name
	}

	statusBar := lipgloss.NewStyle().
		Width(m.width).
		Height(statusHeight).
		Foreground(colorMuted).
		Render(
			fmt.Sprintf(" %s  │  %s  │  %s  │  %s",
				m.connectionInfo(connName),
				m.focusInfo(),
				m.editor.HelpText(),
				mutedStyle.Render("ctrl+t: switch  ctrl+q: quit"),
			),
		)

	rightPanel := lipgloss.JoinVertical(lipgloss.Left,
		editorPanel,
		resultsPanel,
	)

	workspace := lipgloss.JoinHorizontal(lipgloss.Top, sidebar, rightPanel)

	return lipgloss.JoinVertical(lipgloss.Left, workspace, statusBar)
}

func (m Model) connectionInfo(name string) string {
	if m.connection == nil {
		return mutedStyle.Render("not connected")
	}
	return successStyle.Render("● " + name)
}

func (m Model) focusInfo() string {
	switch m.focus {
	case FocusConnections:
		return mutedStyle.Render("focus: tables")
	case FocusEditor:
		return mutedStyle.Render("focus: editor")
	case FocusResults:
		return mutedStyle.Render("focus: results")
	default:
		return ""
	}
}

func (m Model) borderForFocus(f Focus) lipgloss.Color {
	if m.focus == f {
		return colorPrimary
	}
	return colorBorder
}

// Run starts the application.
func Run(cfg *config.Config) {
	m := NewModel(cfg)
	p := tea.NewProgram(m, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		log.Fatalf("Error running application: %v", err)
	}
}
