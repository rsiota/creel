package ui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// exportFormat is a results-export file format. It is a UI-layer concept,
// distinct from db.Format (which drives whole-table SQL dumps): the results
// exporter serializes a result set — already-fetched rows — to a file.
type exportFormat string

const (
	fmtCSV      exportFormat = "csv"
	fmtJSON     exportFormat = "json"
	fmtJSONL    exportFormat = "jsonl"
	fmtMarkdown exportFormat = "md"
	fmtTSV      exportFormat = "tsv"
)

// exportFormats is the ordered list shown in the format picker.
var exportFormats = []exportFormat{fmtCSV, fmtJSON, fmtJSONL, fmtMarkdown, fmtTSV}

// exportFormatLabel is the human-readable name shown in the picker.
var exportFormatLabel = map[exportFormat]string{
	fmtCSV:      "CSV",
	fmtJSON:     "JSON",
	fmtJSONL:    "JSON Lines",
	fmtMarkdown: "Markdown",
	fmtTSV:      "TSV",
}

// exportFormatExt is the file extension used for each format.
var exportFormatExt = map[exportFormat]string{
	fmtCSV:      "csv",
	fmtJSON:     "json",
	fmtJSONL:    "jsonl",
	fmtMarkdown: "md",
	fmtTSV:      "tsv",
}

// parseExportFormat resolves a user-typed format name (case-insensitive) to an
// exportFormat. It accepts the canonical token ("csv"), the extension
// ("md"), and the human label ("markdown", "json lines"). Used by the
// ":export <fmt>" ex command — a non-interactive shortcut over the g X picker.
// Returns false when the name doesn't match any format.
func parseExportFormat(s string) (exportFormat, bool) {
	s = strings.ToLower(strings.TrimSpace(s))
	for _, f := range exportFormats {
		if s == string(f) || s == exportFormatExt[f] || s == strings.ToLower(exportFormatLabel[f]) {
			return f, true
		}
	}
	return "", false
}

// FormatPicker is a single-select overlay for choosing a results-export
// format (opened with g x in the results panel). It remembers the last-used
// format so the cursor starts there next time, keeping a repeat export a
// two-keystroke affair (g x, enter).
type FormatPicker struct {
	items    []exportFormat
	cursor   int
	visible  bool
	lastUsed exportFormat
}

// NewFormatPicker returns a format picker defaulting to CSV.
func NewFormatPicker() FormatPicker {
	return FormatPicker{items: exportFormats, lastUsed: fmtCSV}
}

// Show reveals the picker with the cursor on the last-used format.
func (p *FormatPicker) Show() {
	p.visible = true
	p.cursor = 0
	for i, f := range p.items {
		if f == p.lastUsed {
			p.cursor = i
			break
		}
	}
}

// Hide hides the picker without selecting.
func (p *FormatPicker) Hide() { p.visible = false }

// IsVisible reports whether the picker is shown.
func (p FormatPicker) IsVisible() bool { return p.visible }

// Selected returns the format under the cursor (falls back to the first).
func (p FormatPicker) Selected() exportFormat {
	if p.cursor < 0 || p.cursor >= len(p.items) {
		return p.items[0]
	}
	return p.items[p.cursor]
}

// Up moves the cursor up, clamped.
func (p *FormatPicker) Up() {
	if p.cursor > 0 {
		p.cursor--
	}
}

// Down moves the cursor down, clamped.
func (p *FormatPicker) Down() {
	if p.cursor < len(p.items)-1 {
		p.cursor++
	}
}

// Commit hides the picker and records the chosen format as last-used so the
// cursor starts there next time. Returns the chosen format.
func (p *FormatPicker) Commit() exportFormat {
	f := p.Selected()
	p.lastUsed = f
	p.visible = false
	return f
}

// View renders the picker as a centered bordered overlay panel.
func (p FormatPicker) View() string {
	if !p.visible {
		return ""
	}

	title := lipgloss.NewStyle().Foreground(colorPrimary).Bold(true).Render(" Export as")

	var rows []string
	for i, f := range p.items {
		label := exportFormatLabel[f]
		if i == p.cursor {
			marker := lipgloss.NewStyle().Foreground(colorPrimary).Render("›")
			name := lipgloss.NewStyle().Foreground(colorFg).Bold(true).Render(label)
			rows = append(rows, marker+" "+name)
		} else {
			marker := lipgloss.NewStyle().Foreground(colorMuted).Render(" ")
			name := lipgloss.NewStyle().Foreground(colorMuted).Render(label)
			rows = append(rows, marker+" "+name)
		}
	}

	content := lipgloss.JoinVertical(lipgloss.Left,
		title,
		"",
		strings.Join(rows, "\n"),
	)

	return lipgloss.NewStyle().
		Width(32).
		Border(panelBorder()).
		BorderForeground(colorPrimary).
		Padding(0, 1).
		Render(content)
}
