package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/rsiota/creel/internal/db"
)

// backupItem is one table row in the :backup picker.
type backupItem struct {
	name       string
	rows       int64
	rowsApprox bool
	diskBytes  int64
	schema     bool
	data       bool
}

// BackupPicker is a size-aware multi-select overlay for native :backup.
// Each table has independent schema and data checkboxes.
type BackupPicker struct {
	items     []backupItem
	cursor    int
	scrollRow int
	visible   bool
	width     int
	height    int
	bin       string // local dump binary path; empty when remote dump is enough
}

// NewBackupPicker returns a zero-value BackupPicker.
func NewBackupPicker() BackupPicker {
	return BackupPicker{}
}

// Show populates the picker from table sizes (largest-first), marks schema+data
// for every table, and places the cursor on the first (largest) row so the
// viewport starts at the top. Call SetSize after Show (from Update/layout, not
// View) so j/k scrolling uses the real viewport height — View is a value
// receiver and cannot persist size.
func (p *BackupPicker) Show(sizes []db.TableSize, bin string) {
	sorted := append([]db.TableSize(nil), sizes...)
	sortTableSizes(sorted)
	p.items = make([]backupItem, len(sorted))
	for i, ts := range sorted {
		p.items[i] = backupItem{
			name:       ts.Name,
			rows:       ts.Rows,
			rowsApprox: ts.RowsApprox,
			diskBytes:  ts.DiskBytes,
			schema:     true,
			data:       true,
		}
	}
	p.cursor = 0
	p.scrollRow = 0
	p.bin = bin
	p.visible = true
}

// backupPickerDim returns outer panel width/height for the :backup overlay.
func backupPickerDim(termW, termH int) (w, h int) {
	w = 78
	if termW > 0 && w > termW-4 {
		w = termW - 4
	}
	if w < 48 {
		w = 48
	}
	_, h = popupDim()
	if h < 16 {
		h = 16
	}
	if termH > 0 && h > termH-2 {
		h = termH - 2
	}
	return w, h
}

// exportOverlayDim returns outer dimensions for the g X export dialog.
func exportOverlayDim(termW, termH int) (w, h int) {
	w = 72
	if termW > 0 && w > termW-4 {
		w = termW - 4
	}
	h = termH - 2
	if h > 26 {
		h = 26
	}
	if h < 10 {
		h = 10
	}
	return w, h
}

// Hide clears state and hides the picker.
func (p *BackupPicker) Hide() {
	p.items = nil
	p.cursor = 0
	p.scrollRow = 0
	p.bin = ""
	p.visible = false
}

// IsVisible reports whether the picker is shown.
func (p BackupPicker) IsVisible() bool { return p.visible }

// Bin returns the resolved local dump binary (may be empty for remote dumps).
func (p BackupPicker) Bin() string { return p.bin }

// SetSize sets the rendering dimensions for the picker panel and re-clamps
// scroll so the cursor stays in the viewport after a real size is applied
// (Show may run before layout has set height).
func (p *BackupPicker) SetSize(width, height int) {
	p.width = width
	p.height = height
	p.adjustScroll()
}

// CursorUp moves the cursor up by one.
func (p *BackupPicker) CursorUp() {
	if p.cursor > 0 {
		p.cursor--
	}
	p.adjustScroll()
}

// CursorDown moves the cursor down by one.
func (p *BackupPicker) CursorDown() {
	if p.cursor < len(p.items)-1 {
		p.cursor++
	}
	p.adjustScroll()
}

// CursorTop moves to the first row (g), matching :sizes.
func (p *BackupPicker) CursorTop() {
	p.cursor = 0
	p.scrollRow = 0
}

// CursorBottom moves to the last row (G), matching :sizes.
func (p *BackupPicker) CursorBottom() {
	if len(p.items) == 0 {
		p.cursor = 0
		p.scrollRow = 0
		return
	}
	p.cursor = len(p.items) - 1
	p.adjustScroll()
}

func (p *BackupPicker) adjustScroll() {
	maxVisible := p.maxVisible()
	if p.cursor < p.scrollRow {
		p.scrollRow = p.cursor
	}
	if p.cursor >= p.scrollRow+maxVisible {
		p.scrollRow = p.cursor - maxVisible + 1
	}
}

func (p BackupPicker) maxVisible() int {
	// Content = height - 2 border - 1 header - 1 spacer - 1 footer.
	mv := p.height - 5
	if mv < 1 {
		mv = 1
	}
	return mv
}

// ToggleInclude flips both schema and data for the cursor row: if either is
// on, both turn off (omit); otherwise both turn on.
func (p *BackupPicker) ToggleInclude() {
	if p.cursor < 0 || p.cursor >= len(p.items) {
		return
	}
	item := &p.items[p.cursor]
	if item.schema || item.data {
		item.schema = false
		item.data = false
	} else {
		item.schema = true
		item.data = true
	}
}

// ToggleSchema flips the schema checkbox on the cursor row.
func (p *BackupPicker) ToggleSchema() {
	if p.cursor >= 0 && p.cursor < len(p.items) {
		p.items[p.cursor].schema = !p.items[p.cursor].schema
	}
}

// ToggleData flips the data checkbox on the cursor row.
func (p *BackupPicker) ToggleData() {
	if p.cursor >= 0 && p.cursor < len(p.items) {
		p.items[p.cursor].data = !p.items[p.cursor].data
	}
}

// SelectAll marks schema+data on every table.
func (p *BackupPicker) SelectAll() {
	for i := range p.items {
		p.items[i].schema = true
		p.items[i].data = true
	}
}

// SelectNone clears schema and data on every table.
func (p *BackupPicker) SelectNone() {
	for i := range p.items {
		p.items[i].schema = false
		p.items[i].data = false
	}
}

// SchemaOnlyAll keeps schema for every table and clears data.
func (p *BackupPicker) SchemaOnlyAll() {
	for i := range p.items {
		p.items[i].schema = true
		p.items[i].data = false
	}
}

// SchemaCount returns how many tables include schema.
func (p BackupPicker) SchemaCount() int {
	n := 0
	for _, item := range p.items {
		if item.schema {
			n++
		}
	}
	return n
}

// DataCount returns how many tables include data.
func (p BackupPicker) DataCount() int {
	n := 0
	for _, item := range p.items {
		if item.data {
			n++
		}
	}
	return n
}

// DumpPlan builds the native dump plan from the current selection.
// When every listed table has schema+data (or the list is empty), returns a
// full-database plan so views and other non-table objects are included.
func (p BackupPicker) DumpPlan() db.DumpPlan {
	if len(p.items) == 0 {
		return db.DumpPlan{} // full database dump
	}
	allFull := true
	specs := make([]db.DumpTableSpec, 0, len(p.items))
	for _, item := range p.items {
		if !item.schema || !item.data {
			allFull = false
		}
		if item.schema || item.data {
			specs = append(specs, db.DumpTableSpec{
				Name:   item.name,
				Schema: item.schema,
				Data:   item.data,
			})
		} else {
			allFull = false
		}
	}
	if allFull && len(specs) == len(p.items) {
		return db.DumpPlan{} // full database dump
	}
	return db.DumpPlan{Tables: specs}
}

// View renders the picker panel.
func (p BackupPicker) View() string {
	if !p.visible {
		return ""
	}

	innerW := p.width - 4 // border + padding
	if innerW < 40 {
		innerW = 40
	}

	maxVisible := p.maxVisible()
	end := p.scrollRow + maxVisible
	if end > len(p.items) {
		end = len(p.items)
	}

	header := backupPickerHeader(innerW)
	var rows []string
	for i := p.scrollRow; i < end; i++ {
		rows = append(rows, p.renderRow(i, innerW))
	}
	if len(p.items) == 0 {
		rows = append(rows, mutedStyle.Render("  no tables"))
	}
	for len(rows) < maxVisible {
		rows = append(rows, "")
	}

	listStyled := lipgloss.NewStyle().
		Height(maxVisible).
		Render(strings.Join(rows, "\n"))

	footer := mutedStyle.Render(fmt.Sprintf(
		"  Backup | %d schema · %d data | %d tables",
		p.SchemaCount(), p.DataCount(), len(p.items),
	))

	content := lipgloss.JoinVertical(lipgloss.Left,
		header,
		listStyled,
		"",
		footer,
	)

	panel := lipgloss.NewStyle().
		Width(p.width-2).
		Height(p.height-2).
		Border(panelBorder()).
		BorderForeground(colorPrimary).
		Padding(0, 1).
		Render(content)

	return panel
}

func backupPickerHeader(width int) string {
	nameW, rowsW, sizeW, schW, datW := backupColWidths(width)
	line := fmt.Sprintf("  %-*s %*s %*s %-*s %-*s",
		nameW, "Table",
		rowsW, "Rows",
		sizeW, "Size",
		schW, "Sch",
		datW, "Dat",
	)
	if len(line) > width {
		line = line[:width]
	}
	// Match :sizes column header: primary + bold.
	return lipgloss.NewStyle().Foreground(colorPrimary).Bold(true).Render(line)
}

func (p BackupPicker) renderRow(i, width int) string {
	item := p.items[i]
	nameW, rowsW, sizeW, schW, datW := backupColWidths(width)

	name := item.name
	if lipgloss.Width(name) > nameW {
		name = truncateWidth(name, nameW)
	}
	rows := formatTableSizeRows(db.TableSize{Rows: item.rows, RowsApprox: item.rowsApprox})
	size := db.FormatTableDiskSize(item.diskBytes)
	sch := backupCheckMark(item.schema)
	dat := backupCheckMark(item.data)

	gutter := " "
	if i == p.cursor {
		gutter = "❯"
	}
	line := fmt.Sprintf("%s %-*s %*s %*s %-*s %-*s",
		gutter,
		nameW, name,
		rowsW, rows,
		sizeW, size,
		schW, sch,
		datW, dat,
	)
	line = padBackupRow(ansi.Strip(line), width)

	// Match :sizes / palette: primary background, contrasting fg.
	if i == p.cursor {
		return lipgloss.NewStyle().
			Background(colorPrimary).
			Foreground(colorBg).
			Render(line)
	}
	return lipgloss.NewStyle().Foreground(colorFg).Render(line)
}

func padBackupRow(s string, width int) string {
	w := lipgloss.Width(s)
	if w < width {
		return s + strings.Repeat(" ", width-w)
	}
	if w > width {
		return truncateWidth(s, width)
	}
	return s
}

func backupCheckMark(on bool) string {
	if on {
		return "●"
	}
	return "·"
}

func backupColWidths(width int) (nameW, rowsW, sizeW, schW, datW int) {
	// gutter(2) + spaces between cols(4) + fixed cols
	rowsW, sizeW, schW, datW = 10, 8, 3, 3
	fixed := 2 + 4 + rowsW + sizeW + schW + datW
	nameW = width - fixed
	if nameW < 8 {
		nameW = 8
	}
	return
}

func truncateWidth(s string, max int) string {
	if max <= 0 {
		return ""
	}
	return ansi.Truncate(s, max, "…")
}
