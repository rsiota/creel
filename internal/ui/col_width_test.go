package ui

import (
	"testing"

	"github.com/rsiota/creel/internal/config"
	"github.com/rsiota/creel/internal/session"
)

func TestApplyRememberedWidths(t *testing.T) {
	r := ResultsTable{}
	r.SetResult(
		[]string{"id", "name", "bio"},
		[][]string{{"1", "a", "hi"}},
		"",
	)
	// Auto-fit is small for these values.
	shortName := r.ColWidth(1)
	if shortName <= 0 {
		t.Fatalf("expected positive auto width, got %d", shortName)
	}

	r.ApplyRememberedWidths(map[string]int{
		"name": 30,
		"bio":  10, // smaller than or equal to auto — should not shrink
		"nope": 20, // unknown column — ignored
	})
	if r.ColWidth(1) != 30 {
		t.Errorf("name width = %d, want 30", r.ColWidth(1))
	}
	if r.ColWidth(2) < shortName {
		t.Errorf("bio should not shrink below auto-fit")
	}

	// Cap at maxCellWidth.
	r.ApplyRememberedWidths(map[string]int{"name": maxCellWidth + 50})
	if r.ColWidth(1) != maxCellWidth {
		t.Errorf("name width = %d, want capped %d", r.ColWidth(1), maxCellWidth)
	}
}

func TestSyncColWidthMemoryGrowsAndSticks(t *testing.T) {
	m := NewModel(&config.Config{})
	m.tables = []string{"users"}

	// Wide page seeds memory.
	m.results.SetResult(
		[]string{"id", "email"},
		[][]string{{"1", "alice@example.com"}},
		"",
	)
	m.results.SetEditable("users", []string{"id"})
	m.syncColWidthMemory()
	wide := m.results.ColWidth(1)
	if wide < len("alice@example.com") {
		t.Fatalf("wide width = %d, want >= email len", wide)
	}
	if got := m.colWidthsFor("users")["email"]; got != wide {
		t.Errorf("memory email = %d, want %d", got, wide)
	}

	// Short page: auto-fit would shrink, memory should hold the wide width.
	m.results.SetResult(
		[]string{"id", "email"},
		[][]string{{"2", "a@b.c"}},
		"",
	)
	m.results.SetEditable("users", []string{"id"})
	autoShort := m.results.ColWidth(1)
	if autoShort >= wide {
		t.Fatalf("expected short auto-fit < wide; short=%d wide=%d", autoShort, wide)
	}
	m.syncColWidthMemory()
	if m.results.ColWidth(1) != wide {
		t.Errorf("after sync width = %d, want remembered %d", m.results.ColWidth(1), wide)
	}
}

func TestColWidthMemorySessionRoundTrip(t *testing.T) {
	dir := t.TempDir()
	store := session.NewStore(dir)

	m := NewModel(&config.Config{})
	m.sessionStore = store
	m.tables = []string{"users"}
	// Fake an open connection key via a real sqlite file so sessionKey works.
	conn := newNamedSQLiteConn(t, "memdb")
	m.connection = conn

	m.results.SetResult(
		[]string{"id", "name"},
		[][]string{{"1", "alexandria"}},
		"",
	)
	m.results.SetEditable("users", []string{"id"})
	m.syncColWidthMemory()
	want := m.colWidthsFor("users")["name"]
	if want == 0 {
		t.Fatal("expected remembered name width")
	}

	m.saveSession()

	m2 := NewModel(&config.Config{})
	m2.sessionStore = store
	m2.connection = conn
	m2.tables = []string{"users"}
	m2.restoreSession()
	if got := m2.colWidthsFor("users")["name"]; got != want {
		t.Errorf("restored name width = %d, want %d", got, want)
	}

	// Widths load even when tabs are blank (HasContent false).
	_ = store.Save(conn.Config().Name, conn.Config().Database, session.State{
		ColWidths: map[string]map[string]int{"users": {"name": 33}},
	})
	m3 := NewModel(&config.Config{})
	m3.sessionStore = store
	m3.connection = conn
	m3.restoreSession()
	if got := m3.colWidthsFor("users")["name"]; got != 33 {
		t.Errorf("widths-only restore = %d, want 33", got)
	}
	// Default tab left in place.
	if len(m3.resultsTabs) != 1 || m3.resultsTabs[0].EditorQuery != "" {
		t.Errorf("blank-tab session should keep default tab, got %+v", m3.resultsTabs)
	}
}

func TestRenameColWidthTable(t *testing.T) {
	m := NewModel(&config.Config{})
	m.colWidthMem = map[string]map[string]int{
		"users": {"email": 24},
	}
	m.renameColWidthTable("users", "accounts")
	if m.colWidthsFor("users") != nil {
		t.Error("old table key should be gone")
	}
	if got := m.colWidthsFor("accounts")["email"]; got != 24 {
		t.Errorf("renamed width = %d, want 24", got)
	}
}

func TestSessionClearDropsColWidthMem(t *testing.T) {
	dir := t.TempDir()
	m := NewModel(&config.Config{})
	m.sessionStore = session.NewStore(dir)
	m.connection = newNamedSQLiteConn(t, "clr")
	m.colWidthMem = map[string]map[string]int{"t": {"c": 10}}
	m.saveSession()
	m.runExCommand("session clear")
	if m.colWidthMem != nil {
		t.Errorf("colWidthMem should be nil after clear, got %v", m.colWidthMem)
	}
}
