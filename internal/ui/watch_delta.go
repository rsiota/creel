package ui

import "strings"

// rowFingerprint is a stable content key for one result row (used to detect
// new/changed rows across :watch / :tail refreshes).
func rowFingerprint(row []string) string {
	return strings.Join(row, "\x00")
}

// computeWatchDelta returns indexes in next whose content was not present in
// prev. Used to tint new/changed rows after a background refresh. An empty
// prev means "first snapshot" — no highlights (avoids flashing the whole grid
// when a watch starts).
func computeWatchDelta(prev, next [][]string) map[int]bool {
	if len(prev) == 0 || len(next) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(prev))
	for _, row := range prev {
		seen[rowFingerprint(row)] = struct{}{}
	}
	out := make(map[int]bool)
	for i, row := range next {
		if _, ok := seen[rowFingerprint(row)]; !ok {
			out[i] = true
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// cloneResultRows deep-copies a result page for the next watch comparison.
func cloneResultRows(rows [][]string) [][]string {
	if rows == nil {
		return nil
	}
	out := make([][]string, len(rows))
	for i, row := range rows {
		cp := make([]string, len(row))
		copy(cp, row)
		out[i] = cp
	}
	return out
}
