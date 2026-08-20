package ui

import (
	"context"
	"errors"
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/rsiota/creel/internal/db"
)

// chartAllMaxRows caps a :bar! / :line! / :hist! / :freq! / :pie! / :scatter! re-query so a huge result
// cannot blow memory. The status bar notes when the chart truncated.
const chartAllMaxRows = 50_000

// chartReadyMsg delivers a finished chart, either built from the current page
// or from a full-query re-run (async :bar! / :line! / :hist!).
type chartReadyMsg struct {
	kind      chartKind
	title     string
	bars      []chartBar
	points    []chartPoint
	skipped   int
	agg       barAgg
	truncated bool
	filterCol string
	err       string
	spec      chartSpec // remembered for redraw on :watch / refresh
	all       bool      // true when built from a bang (full-query) fetch
	quiet     bool      // true = don't steal focus (background redraw)
}

type chartSpec struct {
	kind     chartKind
	agg      barAgg
	colNames []string // 1 for hist, 2 for bar/line
	bins     int      // hist only; 0 = auto
	title    string
	emptyErr string
}

func (m *Model) applyChartReady(msg chartReadyMsg) {
	m.applyChartReadyOpt(msg, !msg.quiet)
}

// applyChartReadyOpt applies a finished chart. When focusResults is false
// (watch/refresh redraw) the current focus is left alone.
func (m *Model) applyChartReadyOpt(msg chartReadyMsg, focusResults bool) {
	if msg.err != "" {
		m.schemaMsg = msg.err
		return
	}
	switch msg.kind {
	case chartKindLine:
		m.chartPanel.ShowLine(msg.title, msg.points, msg.skipped)
	case chartKindScatter:
		m.chartPanel.ShowScatter(msg.title, msg.points, msg.skipped)
	case chartKindHist:
		m.chartPanel.ShowHist(msg.title, msg.bars, msg.skipped)
	case chartKindPie:
		m.chartPanel.ShowPie(msg.title, msg.bars, msg.skipped)
	default:
		m.chartPanel.ShowBar(msg.title, msg.bars, msg.skipped, msg.agg)
	}
	m.chartPanel.filterCol = msg.filterCol
	m.lastChartSpec = msg.spec
	m.lastChartAll = msg.all || msg.truncated
	m.lastChartOK = msg.spec.title != "" || len(msg.spec.colNames) > 0
	if focusResults {
		m.focus = FocusResults
	}
	if msg.truncated {
		m.schemaMsg = fmt.Sprintf("charted first %s rows", formatCount(chartAllMaxRows))
	} else {
		// Clear "charting all rows…" after a bang fetch.
		m.schemaMsg = ""
	}
}

func (m *Model) runChart(spec chartSpec, all bool) tea.Cmd {
	if m.results.NumRows() == 0 {
		m.schemaMsg = "no results to chart"
		return nil
	}
	if !all {
		msg := buildChartReady(m.results, spec, false)
		msg.spec = spec
		msg.all = false
		m.applyChartReady(msg)
		return nil
	}
	if m.connection == nil {
		m.schemaMsg = "not connected"
		return nil
	}
	query := strings.TrimRight(strings.TrimSpace(m.lastQuery), ";")
	if query == "" {
		m.schemaMsg = "no query to re-run — :bar! / :line! / :hist! / :freq! / :pie! / :scatter! charts the last SELECT"
		return nil
	}
	expanded, err := m.expandQueryParams(query)
	if err != nil {
		m.schemaMsg = err.Error()
		return nil
	}
	query = expanded
	execQuery := query
	if isSelectQuery(query) && !hasJoinClause(query) {
		execQuery = fmt.Sprintf("SELECT * FROM (%s) AS _creel_chart LIMIT %d", query, chartAllMaxRows+1)
	}
	conn := m.connection
	tx := m.tx
	ctx, cancel := m.queryContext()
	displaySpec := spec
	if !strings.Contains(displaySpec.title, " · all") {
		displaySpec.title += " · all"
	}
	m.schemaMsg = "charting all rows…"
	return func() tea.Msg {
		defer cancel()
		var (
			result db.Result
			err    error
		)
		if tx != nil {
			result, err = tx.ExecuteContext(ctx, execQuery)
		} else {
			result, err = conn.DB().ExecuteContext(ctx, execQuery)
		}
		if err != nil {
			if errors.Is(err, context.Canceled) {
				return chartReadyMsg{err: "chart cancelled"}
			}
			if errors.Is(err, context.DeadlineExceeded) {
				return chartReadyMsg{err: "chart timed out"}
			}
			return chartReadyMsg{err: err.Error()}
		}
		truncated := false
		rows := result.Rows
		if len(rows) > chartAllMaxRows {
			rows = rows[:chartAllMaxRows]
			truncated = true
		}
		cols := make([]string, len(result.Columns))
		for i, c := range result.Columns {
			cols[i] = c.Name
		}
		tbl := NewResultsTable()
		tbl.SetResult(cols, rows, "")
		msg := buildChartReady(tbl, displaySpec, truncated)
		msg.spec = spec // keep base title (no · all) for later redraw
		msg.all = true
		return msg
	}
}

// redrawLastChart rebuilds the remembered chart after a results refresh.
// Page charts use the new grid; bang charts re-fetch. focusResults is false
// for background :watch ticks so the editor keeps focus.
func (m *Model) redrawLastChart(focusResults bool) tea.Cmd {
	if !m.lastChartOK {
		return nil
	}
	if m.lastChartAll {
		return m.runChartQuiet(m.lastChartSpec, true, !focusResults)
	}
	msg := buildChartReady(m.results, m.lastChartSpec, false)
	msg.spec = m.lastChartSpec
	msg.all = false
	msg.quiet = !focusResults
	m.applyChartReadyOpt(msg, focusResults)
	return nil
}

// runChartQuiet is runChart with an optional quiet flag (no focus steal).
func (m *Model) runChartQuiet(spec chartSpec, all, quiet bool) tea.Cmd {
	cmd := m.runChart(spec, all)
	if !quiet || cmd == nil {
		return cmd
	}
	// Wrap the async bang path so the resulting chartReadyMsg is quiet.
	return func() tea.Msg {
		msg := cmd()
		if c, ok := msg.(chartReadyMsg); ok {
			c.quiet = true
			return c
		}
		return msg
	}
}

func buildChartReady(r ResultsTable, spec chartSpec, truncated bool) chartReadyMsg {
	idxs := make([]int, len(spec.colNames))
	for i, name := range spec.colNames {
		idxs[i] = indexOfColumn(r, name)
		if idxs[i] < 0 {
			return chartReadyMsg{err: fmt.Sprintf("no such column: %s", name)}
		}
	}
	title := spec.title
	if truncated && !strings.Contains(title, " · all") {
		title += " · all"
	}
	msg := chartReadyMsg{kind: spec.kind, title: title, agg: spec.agg, truncated: truncated}
	if spec.kind != chartKindLine && spec.kind != chartKindScatter && len(spec.colNames) > 0 {
		msg.filterCol = spec.colNames[0]
	}
	switch spec.kind {
	case chartKindLine, chartKindScatter:
		pts, skipped := buildLineSeries(r, idxs[0], idxs[1])
		if len(pts) == 0 {
			msg.err = spec.emptyErr
			return msg
		}
		msg.points = pts
		msg.skipped = skipped
	case chartKindHist:
		bars, skipped := buildHistSeries(r, idxs[0], spec.bins)
		if len(bars) == 0 {
			msg.err = spec.emptyErr
			return msg
		}
		msg.bars = bars
		msg.skipped = skipped
	case chartKindPie:
		bars, skipped := buildBarSeries(r, idxs[0], idxs[1], barAggCount)
		if len(bars) == 0 {
			msg.err = spec.emptyErr
			return msg
		}
		msg.bars = bars
		msg.skipped = skipped
	default:
		bars, skipped := buildBarSeries(r, idxs[0], idxs[1], spec.agg)
		if len(bars) == 0 {
			msg.err = spec.emptyErr
			return msg
		}
		msg.bars = bars
		msg.skipped = skipped
	}
	return msg
}

func indexOfColumn(r ResultsTable, name string) int {
	for i := 0; i < r.NumCols(); i++ {
		if strings.EqualFold(r.ColumnName(i), name) {
			return i
		}
	}
	return -1
}

func (m *Model) resultColumnIndex(name string) int {
	return indexOfColumn(m.results, name)
}
