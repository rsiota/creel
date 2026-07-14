package ui

import (
	"time"
)

// ResultsTab represents a single result tab with its query, results, and UI state.
type ResultsTab struct {
	ID        int
	Title     string
	CreatedAt time.Time

	// The results table component (holds rows, cursor, scroll, marks, edits, etc.)
	Results ResultsTable

	// Per-tab query/pagination state (synced to/from Model on tab switch).
	Page         int
	PageSize     int
	LastQuery    string
	BaseQuery    string
	PageMsg      string
	TotalRows    int
	TotalRowsSet bool
	Filters      []string
	SortCol      string
	SortDir      string
	QueryStack   []queryStackEntry
	LastSearch   string
	StatsMsg     string
	EditorQuery  string
}

// NewResultsTab creates a new tab with minimal state.
func NewResultsTab(id int, title string) *ResultsTab {
	return &ResultsTab{
		ID:        id,
		Title:     title,
		CreatedAt: time.Now(),
		Results:   NewResultsTable(),
		PageSize:  defaultPageSize,
	}
}

// SetQuery updates the tab's query and marks it as executed.
func (t *ResultsTab) SetQuery(query string) {
	t.LastQuery = query
}

// GetDisplayTitle returns the title with a dirty indicator if the editor
// content differs from the last executed query.
func (t *ResultsTab) GetDisplayTitle() string {
	if t.EditorQuery != "" && t.EditorQuery != t.LastQuery {
		return t.Title + " ●"
	}
	return t.Title
}

// saveTabState copies per-tab state from the Model into the active tab.
func (m *Model) saveTabState() {
	tab := m.activeTab()
	if tab == nil {
		return
	}
	tab.Results = m.results
	tab.Page = m.page
	tab.PageSize = m.pageSize
	tab.LastQuery = m.lastQuery
	tab.BaseQuery = m.baseQuery
	tab.PageMsg = m.pageMsg
	tab.TotalRows = m.totalRows
	tab.TotalRowsSet = m.totalRowsSet
	tab.Filters = m.filters
	tab.SortCol = m.sortCol
	tab.SortDir = m.sortDir
	tab.QueryStack = m.queryStack
	tab.LastSearch = m.lastSearch
	tab.StatsMsg = m.statsMsg
	tab.EditorQuery = m.editor.Value()
}

// restoreTabState copies per-tab state from the active tab into the Model.
func (m *Model) restoreTabState() {
	tab := m.activeTab()
	if tab == nil {
		return
	}
	m.results = tab.Results
	m.page = tab.Page
	m.pageSize = tab.PageSize
	m.lastQuery = tab.LastQuery
	m.baseQuery = tab.BaseQuery
	m.pageMsg = tab.PageMsg
	m.totalRows = tab.TotalRows
	m.totalRowsSet = tab.TotalRowsSet
	m.filters = tab.Filters
	m.sortCol = tab.SortCol
	m.sortDir = tab.SortDir
	m.queryStack = tab.QueryStack
	m.lastSearch = tab.LastSearch
	m.statsMsg = tab.StatsMsg
	if tab.EditorQuery != "" {
		m.editor.SetValue(tab.EditorQuery)
	}
}

// cancelTransientModes exits active input modes that should not persist
// across tab switches (search, ex command line, backend search).
func (m *Model) cancelTransientModes() {
	m.searching = false
	m.searchQuery = ""
	m.ex.Hide()
	m.backendSearching = false
	m.backendSearchInput = ""
	if m.backendSearchTimer != nil {
		m.backendSearchTimer.Stop()
		m.backendSearchTimer = nil
	}
}
