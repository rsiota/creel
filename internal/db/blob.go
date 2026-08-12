package db

import (
	"encoding/hex"
	"fmt"
	"strings"
)

// IsBinaryType reports whether a database column type name should be scanned
// as []byte rather than text. Matches SQLite BLOB, Postgres BYTEA, and the
// MySQL BLOB/BINARY family (including sized forms like VARBINARY(16)).
//
// ScanType alone is not reliable: several drivers report []uint8 for TEXT
// and VARCHAR too, so we key off DatabaseTypeName only.
func IsBinaryType(typeName string) bool {
	u := strings.ToUpper(strings.TrimSpace(typeName))
	switch u {
	case "BLOB", "TINYBLOB", "MEDIUMBLOB", "LONGBLOB",
		"BINARY", "VARBINARY", "BYTEA", "IMAGE":
		return true
	}
	if strings.HasPrefix(u, "BINARY(") || strings.HasPrefix(u, "VARBINARY(") {
		return true
	}
	return false
}

// BlobPlaceholder is the display sentinel written into Result.Rows for a
// binary cell. Size is humanized (e.g. "<BLOB 1.2KB>").
func BlobPlaceholder(n int) string {
	return "<BLOB " + FormatByteSize(n) + ">"
}

// IsBlobPlaceholder reports whether s looks like a BlobPlaceholder sentinel.
// Prefer Result.Blobs / ResultsTable.IsBlobCell as the source of truth when
// available; this is a display-side fallback (styling, export).
func IsBlobPlaceholder(s string) bool {
	return strings.HasPrefix(s, "<BLOB ") && strings.HasSuffix(s, ">")
}

// FormatByteSize renders a byte count for blob placeholders: "512B",
// "1.2KB", "3.0MB".
func FormatByteSize(n int) string {
	if n < 1024 {
		return fmt.Sprintf("%dB", n)
	}
	if n < 1024*1024 {
		return fmt.Sprintf("%.1fKB", float64(n)/1024)
	}
	return fmt.Sprintf("%.1fMB", float64(n)/(1024*1024))
}

// BlobSQLLiteral renders raw bytes as a SQL binary literal. BYTEA columns
// use Postgres hex-escape ('\x…'); everything else uses the X'…' form
// accepted by SQLite and MySQL.
func BlobSQLLiteral(data []byte, typ string) string {
	h := hex.EncodeToString(data)
	if strings.Contains(strings.ToUpper(typ), "BYTEA") {
		return `'\x` + h + `'`
	}
	return "X'" + h + "'"
}

// TrimBlobs returns a copy of blobs containing only entries with Row <
// maxRows. Used when a paged query fetches pageSize+1 rows and the UI
// discards the overflow row before display.
func TrimBlobs(blobs map[BlobKey][]byte, maxRows int) map[BlobKey][]byte {
	if len(blobs) == 0 {
		return nil
	}
	out := make(map[BlobKey][]byte, len(blobs))
	for k, v := range blobs {
		if k.Row < maxRows {
			out[k] = v
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}
