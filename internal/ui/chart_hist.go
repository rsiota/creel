package ui

import (
	"math"
)

// buildHistSeries bins one numeric column into equal-width bars. NULL /
// non-numeric cells are skipped; negatives are kept. bins<=0 picks a Sturges
// count clamped to 8–20 so the existing bar panel can show every bin.
func buildHistSeries(r ResultsTable, col, bins int) (bars []chartBar, skipped int) {
	n := r.NumRows()
	vals := make([]float64, 0, n)
	for i := 0; i < n; i++ {
		v, ok := parseFloat(r.RowValue(i, col))
		if !ok || math.IsNaN(v) || math.IsInf(v, 0) {
			skipped++
			continue
		}
		vals = append(vals, v)
	}
	if len(vals) == 0 {
		return nil, skipped
	}
	min, max := vals[0], vals[0]
	for _, v := range vals[1:] {
		if v < min {
			min = v
		}
		if v > max {
			max = v
		}
	}
	if bins < 1 {
		bins = defaultHistBins(len(vals))
	}
	if bins > 100 {
		bins = 100
	}
	if min == max {
		return []chartBar{{label: formatChartValue(min), value: float64(len(vals)), n: len(vals)}}, skipped
	}
	width := (max - min) / float64(bins)
	counts := make([]int, bins)
	for _, v := range vals {
		i := int(math.Floor((v - min) / width))
		if i >= bins {
			i = bins - 1
		}
		if i < 0 {
			i = 0
		}
		counts[i]++
	}
	bars = make([]chartBar, bins)
	for i, c := range counts {
		lo := min + float64(i)*width
		hi := lo + width
		bars[i] = chartBar{
			label: formatChartValue(lo) + "–" + formatChartValue(hi),
			value: float64(c),
			n:     c,
		}
	}
	return bars, skipped
}

func defaultHistBins(n int) int {
	if n <= 1 {
		return 1
	}
	bins := int(math.Ceil(math.Log2(float64(n)) + 1))
	if bins < 8 {
		return 8
	}
	if bins > 20 {
		return 20
	}
	return bins
}
