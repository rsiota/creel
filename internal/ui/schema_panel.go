package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/ruben/gsql/internal/db"
)

// SchemaPanel is a modal overlay showing table column metadata and actions.
type SchemaPanel struct {
	table      string
	driver     db.Driver
	columns    []db.TableColumnInfo
	visible    bool
	cursor     int
	scrollRow  int
	filter     string
	filtering  bool
	showActions bool
	actionCursor int
	notice     string
	width      int
	height     int
	listRows   int // visible column rows (computed from terminal height)
}

// NewSchemaPanel creates a hidden schema panel.
func NewSchemaPanel() SchemaPanel {
	return SchemaPanel{}
}

// Show opens the panel for a table.
func (p *SchemaPanel) Show(table string, driver db.Driver, columns []db.TableColumnInfo) {
	p.table = table
	p.driver = driver
	p.columns = append([]db.TableColumnInfo(nil), columns...)
	p.visible = true
	p.cursor = 0
	p.scrollRow = 0
	p.filter = ""
	p.filtering = false
	p.showActions = false
	p.actionCursor = 0
	p.notice = ""
}

// Hide closes the panel.
func (p *SchemaPanel) Hide() {
	p.visible = false
	p.table = ""
	p.columns = nil
	p.filter = ""
	p.filtering = false
	p.showActions = false
	p.notice = ""
}

// IsVisible reports whether the panel is open.
func (p SchemaPanel) IsVisible() bool {
	return p.visible
}

// InActionsMode reports whether the column action menu is shown.
func (p SchemaPanel) InActionsMode() bool {
	return p.showActions
}

// IsFiltering reports whether the column filter input is active.
func (p SchemaPanel) IsFiltering() bool {
	return p.filtering
}

// SelectedColumn returns the currently highlighted column.
func (p SchemaPanel) SelectedColumn() (db.TableColumnInfo, bool) {
	return p.selectedColumn()
}

// ColumnNames returns all column names in table order.
func (p SchemaPanel) ColumnNames() []string {
	names := make([]string, len(p.columns))
	for i, c := range p.columns {
		names[i] = c.Name
	}
	return names
}

// Table returns the table shown in the panel.
func (p SchemaPanel) Table() string {
	return p.table
}

// SetColumns replaces column metadata (e.g. after DDL).
func (p *SchemaPanel) SetColumns(columns []db.TableColumnInfo) {
	p.columns = append([]db.TableColumnInfo(nil), columns...)
	if p.cursor >= len(p.filteredColumns()) {
		p.cursor = 0
	}
	p.scrollRow = 0
}

// SetSize stores terminal dimensions and derives how many column rows fit.
func (p *SchemaPanel) SetSize(width, terminalHeight int) {
	p.width = width
	p.height = terminalHeight
	p.listRows = schemaPanelListRows(terminalHeight)
}

// schemaPanelListRows returns how many column rows fit in the popup for a terminal height.
func schemaPanelListRows(terminalHeight int) int {
	// margin + border(2) + padding(2) + title + header + scroll hint + footer + help
	const chrome = 1 + 2 + 2 + 5
	rows := terminalHeight - chrome
	if rows < 8 {
		rows = 8
	}
	if rows > 28 {
		rows = 28
	}
	return rows
}

// SetNotice sets a transient message shown in the panel footer.
func (p *SchemaPanel) SetNotice(msg string) {
	p.notice = msg
}

func (p SchemaPanel) filteredColumns() []db.TableColumnInfo {
	if p.filter == "" {
		return p.columns
	}
	var out []db.TableColumnInfo
	q := strings.ToLower(p.filter)
	for _, c := range p.columns {
		if strings.Contains(strings.ToLower(c.Name), q) ||
			strings.Contains(strings.ToLower(c.Type), q) {
			out = append(out, c)
		}
	}
	return out
}

func (p *SchemaPanel) selectedColumn() (db.TableColumnInfo, bool) {
	cols := p.filteredColumns()
	if p.cursor < 0 || p.cursor >= len(cols) {
		return db.TableColumnInfo{}, false
	}
	return cols[p.cursor], true
}

// CursorUp moves the selection up.
func (p *SchemaPanel) CursorUp() {
	if p.showActions {
		if p.actionCursor > 0 {
			p.actionCursor--
		}
		return
	}
	if p.cursor > 0 {
		p.cursor--
		p.adjustScroll()
	}
}

// CursorDown moves the selection down.
func (p *SchemaPanel) CursorDown() {
	if p.showActions {
		actions := p.currentActions()
		if p.actionCursor < len(actions)-1 {
			p.actionCursor++
		}
		return
	}
	cols := p.filteredColumns()
	if p.cursor < len(cols)-1 {
		p.cursor++
		p.adjustScroll()
	}
}

func (p *SchemaPanel) adjustScroll() {
	maxVisible := p.listHeight()
	if maxVisible < 1 {
		maxVisible = 1
	}
	if p.cursor < p.scrollRow {
		p.scrollRow = p.cursor
	}
	if p.cursor >= p.scrollRow+maxVisible {
		p.scrollRow = p.cursor - maxVisible + 1
	}
}

func (p SchemaPanel) listHeight() int {
	if p.listRows > 0 {
		return p.listRows
	}
	return schemaPanelListRows(p.height)
}

// StartFilter enters column filter mode.
func (p *SchemaPanel) StartFilter() {
	p.filtering = true
	p.filter = ""
	p.cursor = 0
	p.scrollRow = 0
	p.showActions = false
}

// CancelFilter exits filter mode.
func (p *SchemaPanel) CancelFilter() {
	p.filtering = false
	p.filter = ""
}

// FilterAddChar appends to the filter query.
func (p *SchemaPanel) FilterAddChar(ch string) {
	p.filter += ch
	p.cursor = 0
	p.scrollRow = 0
}

// FilterBackspace removes the last filter character.
func (p *SchemaPanel) FilterBackspace() {
	if len(p.filter) > 0 {
		p.filter = p.filter[:len(p.filter)-1]
		p.cursor = 0
		p.scrollRow = 0
	}
}

// OpenActions opens the action menu for the selected column.
func (p *SchemaPanel) OpenActions() {
	if len(p.currentActions()) == 0 {
		p.notice = "No column actions available for this driver"
		return
	}
	p.showActions = true
	p.actionCursor = 0
	p.notice = ""
}

// CloseActions returns to the column list.
func (p *SchemaPanel) CloseActions() {
	p.showActions = false
	p.actionCursor = 0
}

func (p SchemaPanel) currentActions() []db.SchemaAction {
	return db.ColumnSchemaActions(p.driver)
}

// SelectedAction returns the highlighted column action, if any.
func (p SchemaPanel) SelectedAction() (db.SchemaAction, bool) {
	if !p.showActions {
		return "", false
	}
	actions := p.currentActions()
	if p.actionCursor < 0 || p.actionCursor >= len(actions) {
		return "", false
	}
	return actions[p.actionCursor], true
}

// View renders the schema panel overlay content.
func (p SchemaPanel) View() string {
	if !p.visible {
		return ""
	}
	if p.showActions {
		return p.viewActions()
	}
	return p.viewSchema()
}

func (p SchemaPanel) viewSchema() string {
	title := titleStyle.Render(fmt.Sprintf("Schema: %s", p.table))
	header := lipgloss.JoinVertical(lipgloss.Left,
		title,
		p.renderColumnHeader(),
	)
	body := p.renderColumnList()
	footer := p.renderSchemaFooter()
	help := mutedStyle.Render("esc close   / filter   a add column   enter column actions")
	return lipgloss.JoinVertical(lipgloss.Left, header, body, footer, help)
}

func (p SchemaPanel) viewActions() string {
	col, ok := p.selectedColumn()
	title := titleStyle.Render(fmt.Sprintf("Actions · %s", p.table))
	if ok {
		title = titleStyle.Render(fmt.Sprintf("Actions · %s.%s", p.table, col.Name))
	}

	var parts []string
	parts = append(parts, title)
	if ok {
		parts = append(parts, lipgloss.NewStyle().Foreground(colorLabel).Render(formatColumnDetail(col)))
	} else {
		parts = append(parts, mutedStyle.Render("(no column selected)"))
	}

	body := p.padBody(p.renderActionMenu(), p.listHeight()+1)
	parts = append(parts, body)

	if p.notice != "" {
		parts = append(parts, errorStyle.Render(p.notice))
	} else {
		parts = append(parts, mutedStyle.Render(" "))
	}

	parts = append(parts, mutedStyle.Render("enter select   esc back"))
	return lipgloss.JoinVertical(lipgloss.Left, parts...)
}

// ContentHeight returns the fixed inner height of the panel in lines.
func (p SchemaPanel) ContentHeight() int {
	// title + secondary line + list body + footer + key help
	return p.listHeight() + 5
}

func (p SchemaPanel) renderColumnHeader() string {
	return mutedStyle.Render(fmt.Sprintf("%-18s %-14s %-12s %s",
		"Column", "Type", "Flags", "Default"))
}

func (p SchemaPanel) renderColumnList() string {
	cols := p.filteredColumns()
	maxVisible := p.listHeight()

	var lines []string
	if len(cols) == 0 {
		lines = append(lines, mutedStyle.Render("  (no columns)"))
		for i := 1; i < maxVisible; i++ {
			lines = append(lines, "")
		}
	} else {
		for row := 0; row < maxVisible; row++ {
			idx := p.scrollRow + row
			if idx >= len(cols) {
				lines = append(lines, "")
				continue
			}
			lines = append(lines, p.renderColumnLine(cols[idx], idx == p.cursor))
		}
	}

	lines = append(lines, p.renderScrollHint(len(cols), maxVisible))
	return strings.Join(lines, "\n")
}

func (p SchemaPanel) renderColumnLine(col db.TableColumnInfo, selected bool) string {
	marker := " "
	if selected {
		marker = lipgloss.NewStyle().Foreground(colorAccent).Render("›")
	}
	name := truncateRunes(col.Name, 18)
	typ := truncateRunes(col.Type, 14)
	flags := truncateRunes(formatColumnFlags(col), 12)
	def := truncateRunes(formatDefaultDisplay(col), 16)
	line := fmt.Sprintf("%s %-18s %-14s %-12s %s", marker, name, typ, flags, def)
	if selected {
		line = lipgloss.NewStyle().Foreground(colorFg).Bold(true).Render(line)
	}
	return line
}

func (p SchemaPanel) renderScrollHint(total, maxVisible int) string {
	if total > maxVisible {
		end := p.scrollRow + maxVisible
		if end > total {
			end = total
		}
		return mutedStyle.Render(fmt.Sprintf("  %d columns (%d–%d)", total, p.scrollRow+1, end))
	}
	return ""
}

func (p SchemaPanel) renderActionMenu() string {
	if _, ok := p.selectedColumn(); !ok {
		return mutedStyle.Render("  (no column selected)")
	}

	var lines []string
	actions := p.currentActions()
	for i, action := range actions {
		marker := " "
		if i == p.actionCursor {
			marker = lipgloss.NewStyle().Foreground(colorAccent).Render("›")
		}
		label := db.SchemaActionLabel(action)
		line := fmt.Sprintf("%s %s", marker, label)
		if i == p.actionCursor {
			line = lipgloss.NewStyle().Foreground(colorFg).Render(line)
		}
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n")
}

func (p SchemaPanel) padBody(body string, height int) string {
	lines := strings.Split(body, "\n")
	for len(lines) < height {
		lines = append(lines, "")
	}
	if len(lines) > height {
		lines = lines[:height]
	}
	return strings.Join(lines, "\n")
}

func (p SchemaPanel) renderSchemaFooter() string {
	if p.notice != "" {
		return errorStyle.Render(p.notice)
	}
	col, ok := p.selectedColumn()
	if !ok {
		return mutedStyle.Render(" ")
	}
	return lipgloss.NewStyle().Foreground(colorLabel).Render(formatColumnDetail(col))
}

func formatColumnFlags(col db.TableColumnInfo) string {
	var flags []string
	if col.PrimaryKey {
		flags = append(flags, "PK")
	}
	if col.AutoIncrement {
		flags = append(flags, "AI")
	}
	if col.NotNull {
		flags = append(flags, "NOT NULL")
	} else {
		flags = append(flags, "NULL")
	}
	if len(flags) == 0 {
		return "—"
	}
	return strings.Join(flags, " ")
}

func formatDefaultDisplay(col db.TableColumnInfo) string {
	if !col.HasDefault {
		return "—"
	}
	if col.DefaultValue == "" {
		return "NULL"
	}
	return col.DefaultValue
}

func formatColumnDetail(col db.TableColumnInfo) string {
	parts := []string{col.Name, col.Type, formatColumnFlags(col)}
	if col.HasDefault {
		parts = append(parts, "default "+formatDefaultDisplay(col))
	}
	return strings.Join(parts, " · ")
}

func truncateRunes(s string, max int) string {
	runes := []rune(s)
	if len(runes) <= max {
		return s
	}
	if max <= 1 {
		return string(runes[:max])
	}
	return string(runes[:max-1]) + "…"
}
