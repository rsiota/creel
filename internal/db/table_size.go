package db

// TableSize holds row and on-disk size estimates for one base table.
type TableSize struct {
	Name       string
	Rows       int64
	RowsApprox bool  // true when Rows is a catalog estimate, not COUNT(*)
	DiskBytes  int64 // on-disk bytes; -1 when unavailable
}

// FormatTableDiskSize renders on-disk bytes for :sizes; negative means unknown.
func FormatTableDiskSize(n int64) string {
	if n < 0 {
		return "—"
	}
	return FormatByteSize(int(n))
}
