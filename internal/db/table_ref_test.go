package db

import "testing"

func TestSplitTableRef(t *testing.T) {
	sch, tbl := SplitTableRef("analytics.events")
	if sch != "analytics" || tbl != "events" {
		t.Fatalf("got %q %q", sch, tbl)
	}
	sch, tbl = SplitTableRef("users")
	if sch != "" || tbl != "users" {
		t.Fatalf("bare: got %q %q", sch, tbl)
	}
	sch, tbl = SplitTableRef("")
	if sch != "" || tbl != "" {
		t.Fatalf("empty: got %q %q", sch, tbl)
	}
}

func TestQuoteTableRef(t *testing.T) {
	got := QuoteTableRef(DriverPostgres, "analytics", "events")
	if got != `"analytics"."events"` {
		t.Fatalf("got %q", got)
	}
	got = QuoteTableRef(DriverPostgres, "", "users")
	if got != `"users"` {
		t.Fatalf("bare: got %q", got)
	}
}
