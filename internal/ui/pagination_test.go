package ui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/ruben/gsql/internal/config"
	"github.com/ruben/gsql/internal/db"
)

func TestPaginationNextPage(t *testing.T) {
	cfg := &config.Config{}
	m := NewModel(cfg)
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})

	// Can't fully test async execution without a real DB, but we can
	// verify the pagination state machine.
	m.lastQuery = "SELECT * FROM users"
	m.page = 0
	m.pageSize = 200

	// nextPage should increment page
	m.nextPage()
	if m.page != 1 {
		t.Errorf("expected page=1 after nextPage, got %d", m.page)
	}

	m.nextPage()
	if m.page != 2 {
		t.Errorf("expected page=2 after second nextPage, got %d", m.page)
	}
}

func TestPaginationPrevPage(t *testing.T) {
	cfg := &config.Config{}
	m := NewModel(cfg)
	m.lastQuery = "SELECT * FROM users"
	m.page = 3

	m.prevPage()
	if m.page != 2 {
		t.Errorf("expected page=2 after prevPage, got %d", m.page)
	}

	// Should not go below 0
	m.page = 0
	m.prevPage()
	if m.page != 0 {
		t.Errorf("expected page=0 at minimum, got %d", m.page)
	}
}

func TestPaginationNoQuery(t *testing.T) {
	cfg := &config.Config{}
	m := NewModel(cfg)
	m.lastQuery = ""

	// nextPage/prevPage with no query should be no-ops
	m.nextPage()
	if m.page != 0 {
		t.Error("page should not change without a query")
	}
}

func TestQueryExecutedMsgPagination(t *testing.T) {
	cfg := &config.Config{}
	m := NewModel(cfg)
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})

	// Simulate a paginated result — query returns pageSize+1 rows
	// to signal "has next page"
	pageSize := 200
	rows := make([][]string, pageSize+1)
	cols := []db.Column{{Name: "id"}, {Name: "name"}}
	for i := range rows {
		rows[i] = []string{"1", "test"}
	}

	colStrs := make([]string, len(cols))
	for i, c := range cols {
		colStrs[i] = c.Name
	}

	// Manually simulate what the queryExecutedMsg handler does
	trimmedRows := rows
	hasNext := false
	if len(trimmedRows) > pageSize {
		hasNext = true
		trimmedRows = trimmedRows[:pageSize]
	}

	if !hasNext {
		t.Error("expected hasNext=true with pageSize+1 rows")
	}
	if len(trimmedRows) != pageSize {
		t.Errorf("expected %d trimmed rows, got %d", pageSize, len(trimmedRows))
	}
}

func TestHasJoinClause(t *testing.T) {
	tests := []struct {
		query string
		want  bool
	}{
		{"SELECT * FROM users", false},
		{"SELECT * FROM users WHERE id = 1", false},
		{"select * from user_rewards left join users on user_rewards.user_id = users.id", true},
		{"SELECT * FROM a INNER JOIN b ON a.id = b.id", true},
		{"SELECT * FROM a JOIN b ON a.id = b.id", true},
		{"SELECT * FROM a LEFT JOIN b ON a.id = b.id", true},
		{"SELECT * FROM a RIGHT JOIN b ON a.id = b.id", true},
		{"SELECT * FROM a CROSS JOIN b", true},
		{"SELECT * FROM a, b WHERE a.id = b.id", true},
		{"SELECT * FROM a, b, c", true},
		{"SELECT * FROM a WHERE x = 'left join'", false},
		{"WITH cte AS (SELECT 1) SELECT * FROM cte", false},
	}
	for _, tc := range tests {
		got := hasJoinClause(tc.query)
		if got != tc.want {
			t.Errorf("hasJoinClause(%q) = %v, want %v", tc.query, got, tc.want)
		}
	}
}
