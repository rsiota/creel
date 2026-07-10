package db

import (
	"testing"
)

func TestSQLiteIndexes(t *testing.T) {
	s := setupSQLiteTestDB(t)
	// Add a secondary index and a unique index to `users`.
	if _, err := s.Exec(`CREATE INDEX idx_users_email ON users(email)`); err != nil {
		t.Fatalf("create index: %v", err)
	}
	if _, err := s.Exec(`CREATE UNIQUE INDEX idx_users_name ON users(name)`); err != nil {
		t.Fatalf("create unique index: %v", err)
	}

	idxs, err := s.Indexes("users")
	if err != nil {
		t.Fatalf("Indexes: %v", err)
	}

	want := map[string]struct {
		unique  bool
		columns []string
	}{
		"idx_users_email": {unique: false, columns: []string{"email"}},
		"idx_users_name":  {unique: true, columns: []string{"name"}},
	}
	got := map[string]Index{}
	for _, ix := range idxs {
		got[ix.Name] = ix
	}
	for name, w := range want {
		ix, ok := got[name]
		if !ok {
			t.Errorf("missing index %q; got %v", name, idxs)
			continue
		}
		if ix.Unique != w.unique {
			t.Errorf("index %q unique = %v, want %v", name, ix.Unique, w.unique)
		}
		if !equalStrings(ix.Columns, w.columns) {
			t.Errorf("index %q columns = %v, want %v", name, ix.Columns, w.columns)
		}
	}

	// The primary-key auto-index must not appear as a secondary index.
	for _, ix := range idxs {
		if ix.Name == "sqlite_autoindex_users_1" {
			t.Errorf("primary-key auto-index leaked into Indexes: %q", ix.Name)
		}
	}
}

func TestSQLiteIndexesNone(t *testing.T) {
	s := setupSQLiteTestDB(t)
	idxs, err := s.Indexes("orders")
	if err != nil {
		t.Fatalf("Indexes: %v", err)
	}
	if len(idxs) != 0 {
		t.Errorf("expected no indexes, got %v", idxs)
	}
}

func TestSQLiteTriggers(t *testing.T) {
	s := setupSQLiteTestDB(t)
	if _, err := s.Exec(`CREATE TRIGGER trg_users_audit AFTER UPDATE ON users
		BEGIN INSERT INTO orders(id) VALUES (NEW.id); END`); err != nil {
		t.Fatalf("create trigger: %v", err)
	}
	triggers, err := s.Triggers("users")
	if err != nil {
		t.Fatalf("Triggers: %v", err)
	}
	if len(triggers) != 1 {
		t.Fatalf("expected 1 trigger, got %d", len(triggers))
	}
	tr := triggers[0]
	if tr.Name != "trg_users_audit" {
		t.Errorf("name = %q", tr.Name)
	}
	if tr.Timing != "AFTER" {
		t.Errorf("timing = %q, want AFTER", tr.Timing)
	}
	if tr.Event != "UPDATE" {
		t.Errorf("event = %q, want UPDATE", tr.Event)
	}
	if tr.Statement == "" {
		t.Error("statement body is empty")
	}
}

func TestSQLiteViewDefinition(t *testing.T) {
	s := setupSQLiteTestDB(t)
	if _, err := s.Exec(`CREATE VIEW active_users AS SELECT id, name FROM users WHERE email IS NOT NULL`); err != nil {
		t.Fatalf("create view: %v", err)
	}
	def, err := s.ViewDefinition("active_users")
	if err != nil {
		t.Fatalf("ViewDefinition: %v", err)
	}
	if def == "" {
		t.Error("expected a non-empty view definition")
	}

	// A table is not a view.
	def, err = s.ViewDefinition("users")
	if err != nil {
		t.Fatalf("ViewDefinition on table: %v", err)
	}
	if def != "" {
		t.Errorf("expected empty definition for a table, got %q", def)
	}
}

func TestParseSQLiteTriggerTiming(t *testing.T) {
	cases := []struct {
		sql        string
		wantTiming string
		wantEvent  string
	}{
		{"CREATE TRIGGER t AFTER INSERT ON x BEGIN ... END", "AFTER", "INSERT"},
		{"CREATE TRIGGER t BEFORE UPDATE ON x BEGIN ... END", "BEFORE", "UPDATE"},
		{"CREATE TRIGGER \"weird name\" INSTEAD OF DELETE ON x BEGIN ... END", "INSTEAD OF", "DELETE"},
		{"CREATE TRIGGER t after insert on x", "AFTER", "INSERT"}, // case-insensitive
		{"garbage", "", ""},
	}
	for _, c := range cases {
		timing, event := parseSQLiteTriggerTiming(c.sql)
		if timing != c.wantTiming || event != c.wantEvent {
			t.Errorf("parse(%q) = (%q,%q), want (%q,%q)", c.sql, timing, event, c.wantTiming, c.wantEvent)
		}
	}
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
