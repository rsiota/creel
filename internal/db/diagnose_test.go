package db

import (
	"strings"
	"testing"
)

func TestDiagnosePostgresSeqScan(t *testing.T) {
	plan := Result{
		Columns: []Column{{Name: "QUERY PLAN"}},
		Rows: [][]string{
			{"Seq Scan on orders  (cost=0.00..1550.00 rows=100000 width=32)"},
			{"  Filter: (user_id = 1)"},
			{"Index Scan using idx_payments_id on payments  (cost=0.42..8.44 rows=1 width=16)"},
		},
	}
	idxs := IndexLookup(func(table string) ([]Index, error) {
		if table == "orders" {
			return nil, nil
		}
		return []Index{{Name: "idx_payments_id", Columns: []string{"id"}}}, nil
	})
	got := DiagnoseExplain(DriverPostgres, plan, idxs)
	if len(got) < 1 || got[0].Issue != "Sequential scan" || got[0].Table != "orders" {
		t.Fatalf("got %+v", got)
	}
	if !strings.Contains(got[0].Hint, "user_id") && !strings.Contains(got[0].Hint, "no secondary") {
		t.Fatalf("hint = %q", got[0].Hint)
	}
}

func TestDiagnoseMySQLFullScan(t *testing.T) {
	plan := Result{
		Columns: []Column{
			{Name: "id"}, {Name: "select_type"}, {Name: "table"}, {Name: "type"},
			{Name: "possible_keys"}, {Name: "key"}, {Name: "rows"}, {Name: "Extra"},
		},
		Rows: [][]string{
			{"1", "SIMPLE", "orders", "ALL", "NULL", "NULL", "500000", "Using where; Using filesort"},
		},
	}
	got := DiagnoseExplain(DriverMySQL, plan, func(string) ([]Index, error) {
		return []Index{{Name: "idx_status", Columns: []string{"status"}}}, nil
	})
	var sawScan, sawSort bool
	for _, f := range got {
		if f.Issue == "Full table scan" && f.Table == "orders" {
			sawScan = true
		}
		if f.Issue == "Using filesort" {
			sawSort = true
		}
	}
	if !sawScan || !sawSort {
		t.Fatalf("got %+v", got)
	}
}

func TestDiagnoseSQLiteScan(t *testing.T) {
	plan := Result{
		Columns: []Column{{Name: "id"}, {Name: "parent"}, {Name: "notused"}, {Name: "detail"}},
		Rows: [][]string{
			{"2", "0", "0", "SCAN TABLE users"},
			{"3", "0", "0", "SEARCH TABLE orders USING INDEX idx_orders_user (user_id=?)"},
		},
	}
	got := DiagnoseExplain(DriverSQLite, plan, nil)
	if len(got) != 1 || got[0].Table != "users" || got[0].Issue != "Table scan" {
		t.Fatalf("got %+v", got)
	}
}

func TestDiagnoseCleanPlan(t *testing.T) {
	plan := Result{
		Columns: []Column{{Name: "QUERY PLAN"}},
		Rows: [][]string{
			{"Index Scan using idx_orders_user on orders  (cost=0.42..8.44 rows=1 width=32)"},
			{"  Index Cond: (user_id = 1)"},
		},
	}
	got := DiagnoseExplain(DriverPostgres, plan, nil)
	if len(got) != 1 || got[0].Severity != "info" {
		t.Fatalf("got %+v", got)
	}
}
