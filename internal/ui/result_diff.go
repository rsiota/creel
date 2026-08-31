package ui

import (
	"fmt"
	"strings"
)

// diffKind classifies one aligned row pair in a result-set diff.
type diffKind int

const (
	diffSame    diffKind = iota // identical in both tabs
	diffAdded                   // present only in the right (B) tab
	diffRemoved                 // present only in the left (A) tab
	diffChanged                 // same key/index, different cell values
)

// diffSnapshot is a tab's loaded result page, ready for comparison.
type diffSnapshot struct {
	title       string
	cols        []string
	rows        [][]string // display values (includes dirty edits)
	pkCols      []string
	sourceTable string
}

func (s diffSnapshot) hasRows() bool {
	return len(s.cols) > 0 && len(s.rows) > 0
}

// snapshotFromTab captures the current page of a results tab for diffing.
func snapshotFromTab(tab *ResultsTab) diffSnapshot {
	if tab == nil {
		return diffSnapshot{}
	}
	r := tab.Results
	cols := r.ColumnNames()
	rows := make([][]string, r.NumRows())
	for i := 0; i < r.NumRows(); i++ {
		row := make([]string, len(cols))
		for j := range cols {
			row[j] = r.RowValue(i, j)
		}
		rows[i] = row
	}
	title := tab.Title
	if title == "" {
		title = "untitled"
	}
	pk := append([]string(nil), r.PKColumns()...)
	return diffSnapshot{
		title:       title,
		cols:        cols,
		rows:        rows,
		pkCols:      pk,
		sourceTable: r.SourceTable(),
	}
}

// diffEntry is one row in the computed diff (aligned to the union column set).
type diffEntry struct {
	Kind        diffKind
	LeftRow     int  // index in left snapshot, or -1
	RightRow    int  // index in right snapshot, or -1
	LeftCells   []string
	RightCells  []string
	ChangedCols []bool // per union column; only meaningful for diffChanged
}

// resultDiff is the full comparison of two result pages.
type resultDiff struct {
	LeftTitle  string
	RightTitle string
	Cols       []string
	Mode       string // "pk" or "row"
	Entries    []diffEntry
	Added      int
	Removed    int
	Changed    int
	Same       int
}

func (d resultDiff) summary() string {
	return fmt.Sprintf("+%d −%d ~%d =%d  (%s)", d.Added, d.Removed, d.Changed, d.Same, d.Mode)
}

// computeResultDiff compares two snapshots. Uses primary-key matching when both
// tabs share the same source table and PK columns; otherwise matches by row index.
func computeResultDiff(a, b diffSnapshot) resultDiff {
	cols, aIdx, bIdx := alignDiffColumns(a.cols, b.cols)
	out := resultDiff{
		LeftTitle:  a.title,
		RightTitle: b.title,
		Cols:       cols,
	}

	leftAligned := projectDiffRows(a.rows, aIdx, len(cols))
	rightAligned := projectDiffRows(b.rows, bIdx, len(cols))

	if canPKMatch(a, b, cols) {
		out.Mode = "pk"
		out.Entries = diffByPK(a, b, leftAligned, rightAligned, cols)
	} else {
		out.Mode = "row"
		out.Entries = diffByIndex(leftAligned, rightAligned)
	}

	for i := range out.Entries {
		switch out.Entries[i].Kind {
		case diffAdded:
			out.Added++
		case diffRemoved:
			out.Removed++
		case diffChanged:
			out.Changed++
		case diffSame:
			out.Same++
		}
	}
	return out
}

func alignDiffColumns(a, b []string) (union []string, aIdx, bIdx []int) {
	lowerA := make(map[string]int, len(a))
	for i, c := range a {
		lowerA[strings.ToLower(c)] = i
	}
	lowerB := make(map[string]int, len(b))
	for i, c := range b {
		lowerB[strings.ToLower(c)] = i
	}

	seen := make(map[string]bool)
	for _, c := range a {
		k := strings.ToLower(c)
		if seen[k] {
			continue
		}
		seen[k] = true
		ai := lowerA[k]
		bi, ok := lowerB[k]
		if !ok {
			bi = -1
		}
		union = append(union, a[ai]) // prefer left spelling
		aIdx = append(aIdx, ai)
		bIdx = append(bIdx, bi)
	}
	for _, c := range b {
		k := strings.ToLower(c)
		if seen[k] {
			continue
		}
		seen[k] = true
		union = append(union, c)
		aIdx = append(aIdx, -1)
		bIdx = append(bIdx, lowerB[k])
	}
	return union, aIdx, bIdx
}

func projectDiffRows(rows [][]string, idx []int, nCols int) [][]string {
	out := make([][]string, len(rows))
	for i, row := range rows {
		cells := make([]string, nCols)
		for j, src := range idx {
			if src >= 0 && src < len(row) {
				cells[j] = row[src]
			} else {
				cells[j] = "—"
			}
		}
		out[i] = cells
	}
	return out
}

func canPKMatch(a, b diffSnapshot, unionCols []string) bool {
	if a.sourceTable == "" || !strings.EqualFold(a.sourceTable, b.sourceTable) {
		return false
	}
	if len(a.pkCols) == 0 || len(a.pkCols) != len(b.pkCols) {
		return false
	}
	for i := range a.pkCols {
		if !strings.EqualFold(a.pkCols[i], b.pkCols[i]) {
			return false
		}
	}
	colSet := make(map[string]bool, len(unionCols))
	for _, c := range unionCols {
		colSet[strings.ToLower(c)] = true
	}
	for _, pk := range a.pkCols {
		if !colSet[strings.ToLower(pk)] {
			return false
		}
	}
	return true
}

func pkKeyFromAligned(row []string, cols, pkCols []string) string {
	lower := make(map[string]int, len(cols))
	for i, c := range cols {
		lower[strings.ToLower(c)] = i
	}
	parts := make([]string, len(pkCols))
	for i, pk := range pkCols {
		if idx, ok := lower[strings.ToLower(pk)]; ok && idx < len(row) {
			parts[i] = row[idx]
		}
	}
	return pkKey(parts)
}

func diffByPK(a, b diffSnapshot, left, right [][]string, cols []string) []diffEntry {
	rightByKey := make(map[string]int, len(right))
	for i, row := range right {
		rightByKey[pkKeyFromAligned(row, cols, a.pkCols)] = i
	}
	usedRight := make(map[int]bool, len(right))
	var entries []diffEntry
	for i, lrow := range left {
		key := pkKeyFromAligned(lrow, cols, a.pkCols)
		ri, ok := rightByKey[key]
		if !ok {
			entries = append(entries, diffEntry{
				Kind:       diffRemoved,
				LeftRow:    i,
				RightRow:   -1,
				LeftCells:  lrow,
				RightCells: emptyCells(len(cols)),
			})
			continue
		}
		usedRight[ri] = true
		rrow := right[ri]
		changed, mask := cellChanges(lrow, rrow)
		kind := diffSame
		if changed {
			kind = diffChanged
		}
		entries = append(entries, diffEntry{
			Kind:        kind,
			LeftRow:     i,
			RightRow:    ri,
			LeftCells:   lrow,
			RightCells:  rrow,
			ChangedCols: mask,
		})
	}
	for i, rrow := range right {
		if usedRight[i] {
			continue
		}
		entries = append(entries, diffEntry{
			Kind:       diffAdded,
			LeftRow:    -1,
			RightRow:   i,
			LeftCells:  emptyCells(len(cols)),
			RightCells: rrow,
		})
	}
	return entries
}

func diffByIndex(left, right [][]string) []diffEntry {
	n := len(left)
	if len(right) > n {
		n = len(right)
	}
	entries := make([]diffEntry, 0, n)
	for i := 0; i < n; i++ {
		switch {
		case i >= len(left):
			entries = append(entries, diffEntry{
				Kind:       diffAdded,
				LeftRow:    -1,
				RightRow:   i,
				LeftCells:  emptyCells(len(right[i])),
				RightCells: right[i],
			})
		case i >= len(right):
			entries = append(entries, diffEntry{
				Kind:       diffRemoved,
				LeftRow:    i,
				RightRow:   -1,
				LeftCells:  left[i],
				RightCells: emptyCells(len(left[i])),
			})
		default:
			changed, mask := cellChanges(left[i], right[i])
			kind := diffSame
			if changed {
				kind = diffChanged
			}
			entries = append(entries, diffEntry{
				Kind:        kind,
				LeftRow:     i,
				RightRow:    i,
				LeftCells:   left[i],
				RightCells:  right[i],
				ChangedCols: mask,
			})
		}
	}
	return entries
}

func cellChanges(a, b []string) (changed bool, mask []bool) {
	n := len(a)
	if len(b) > n {
		n = len(b)
	}
	mask = make([]bool, n)
	for i := 0; i < n; i++ {
		av, bv := "—", "—"
		if i < len(a) {
			av = a[i]
		}
		if i < len(b) {
			bv = b[i]
		}
		if av != bv {
			mask[i] = true
			changed = true
		}
	}
	return changed, mask
}

func emptyCells(n int) []string {
	out := make([]string, n)
	for i := range out {
		out[i] = "—"
	}
	return out
}
