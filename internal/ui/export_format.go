package ui

import "strings"

// exportFormat is a results-export file format. It is a UI-layer concept,
// distinct from db.Format (which drives whole-table SQL dumps): the results
// exporter serializes a result set — already-fetched or re-queried rows — to a
// file.
type exportFormat string

const (
	fmtCSV      exportFormat = "csv"
	fmtJSON     exportFormat = "json"
	fmtJSONL    exportFormat = "jsonl"
	fmtMarkdown exportFormat = "md"
	fmtTSV      exportFormat = "tsv"
)

// exportFormats is the ordered list shown in the export dialog.
var exportFormats = []exportFormat{fmtCSV, fmtJSON, fmtJSONL, fmtMarkdown, fmtTSV}

// exportFormatLabel is the human-readable name shown in the dialog.
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
// exportFormat. It accepts the canonical token ("csv"), the extension ("md"),
// and the human label ("markdown", "json lines"). Used by the ":export <fmt>"
// ex command — a non-interactive shortcut over the g X dialog. Returns false
// when the name doesn't match any format.
func parseExportFormat(s string) (exportFormat, bool) {
	s = strings.ToLower(strings.TrimSpace(s))
	for _, f := range exportFormats {
		if s == string(f) || s == exportFormatExt[f] || s == strings.ToLower(exportFormatLabel[f]) {
			return f, true
		}
	}
	return "", false
}
