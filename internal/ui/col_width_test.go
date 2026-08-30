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

	// Cap at maxManualCellWidth (manual resize ceiling).
	r.ApplyRememberedWidths(map[string]int{"name": maxManualCellWidth + 50})
	if r.ColWidth(1) != maxManualCellWidth {
		t.Errorf("name width = %d, want capped %d", r.ColWidth(1), maxManualCellWidth)
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
	m.colWidthOverride = map[string]map[string]int{"t": {"c": 8}}
	m.saveSession()
	m.runExCommand("session clear")
	if m.colWidthMem != nil {
		t.Errorf("colWidthMem should be nil after clear, got %v", m.colWidthMem)
	}
	if m.colWidthOverride != nil {
		t.Errorf("colWidthOverride should be nil after clear, got %v", m.colWidthOverride)
	}
}

func TestResizeColumn(t *testing.T) {
	r := NewResultsTable()
	r.SetResult(
		[]string{"id", "email"},
		[][]string{{"1", "a@b.c"}},
		"",
	)
	r.SetCursor(0, 1)
	start := r.ColWidth(1)
	w, ok := r.ResizeColumn(colResizeStep)
	if !ok || w != start+colResizeStep {
		t.Fatalf("widen: ok=%v w=%d, want %d", ok, w, start+colResizeStep)
	}
	w, ok = r.ResizeColumn(-colResizeStep * 100)
	if !ok || w != minColWidth {
		t.Fatalf("shrink floor: ok=%v w=%d, want %d", ok, w, minColWidth)
	}
	// Grow past auto-fit cap.
	for r.ColWidth(1) < maxCellWidth+10 {
		if _, ok := r.ResizeColumn(colResizeStep); !ok {
			break
		}
	}
	if r.ColWidth(1) <= maxCellWidth {
		t.Fatalf("manual widen should exceed auto-fit cap %d, got %d", maxCellWidth, r.ColWidth(1))
	}
	// Cap at maxManualCellWidth.
	for {
		if _, ok := r.ResizeColumn(colResizeStep); !ok {
			break
		}
	}
	if r.ColWidth(1) != maxManualCellWidth {
		t.Fatalf("manual cap = %d, want %d", r.ColWidth(1), maxManualCellWidth)
	}
}

func TestResetColumnWidth(t *testing.T) {
	r := NewResultsTable()
	r.SetResult(
		[]string{"id", "email"},
		[][]string{{"1", "a@b.c"}},
		"",
	)
	r.SetCursor(0, 1)
	auto := r.ColWidth(1)
	if _, ok := r.ResizeColumn(colResizeStep * 10); !ok {
		t.Fatal("expected widen")
	}
	if r.ColWidth(1) <= auto {
		t.Fatalf("expected wider than auto %d, got %d", auto, r.ColWidth(1))
	}
	w, ok := r.ResetColumnWidth()
	if !ok || w != auto {
		t.Fatalf("reset: ok=%v w=%d, want %d", ok, w, auto)
	}
	if _, ok := r.ResetColumnWidth(); ok {
		t.Error("second reset should report unchanged")
	}
}

func TestResetColumnWidthClearsOverrideAndFloor(t *testing.T) {
	m := NewModel(&config.Config{})
	m.tables = []string{"users"}
	m.results.SetResult(
		[]string{"id", "email"},
		[][]string{{"1", "alice@example.com"}},
		"",
	)
	m.results.SetEditable("users", []string{"id"})
	m.results.SetCursor(0, 1)
	m.syncColWidthMemory()
	auto := m.results.ColWidth(1)

	m.resizeResultsColumn(colResizeStep * 8)
	wide := m.results.ColWidth(1)
	if wide <= auto {
		t.Fatalf("expected widen above auto %d, got %d", auto, wide)
	}
	if m.colOverridesFor("users")["email"] != wide {
		t.Fatalf("override missing after widen")
	}

	m.resetResultsColumnWidth()
	if m.results.ColWidth(1) != auto {
		t.Errorf("width after reset = %d, want auto %d", m.results.ColWidth(1), auto)
	}
	if m.colOverridesFor("users") != nil && m.colOverridesFor("users")["email"] != 0 {
		t.Errorf("override should be cleared, got %v", m.colOverridesFor("users"))
	}
	if got := m.colWidthsFor("users")["email"]; got != auto {
		t.Errorf("floor after reset = %d, want %d", got, auto)
	}

	// Sync must not re-inflate from a stale floor.
	m.syncColWidthMemory()
	if m.results.ColWidth(1) != auto {
		t.Errorf("after sync width = %d, want auto %d", m.results.ColWidth(1), auto)
	}
}

func TestResetColumnWidthKeybinding(t *testing.T) {
	m := newResultsWorkspaceModel()
	m.results.SetCursor(0, 1) // name
	auto := m.results.ColWidth(1)
	m.resizeResultsColumn(colResizeStep * 6)
	if m.results.ColWidth(1) <= auto {
		t.Fatal("expected widen before reset key")
	}
	m = press(m, keyRunes('='))
	if m.results.ColWidth(1) != auto {
		t.Errorf("after = width = %d, want %d", m.results.ColWidth(1), auto)
	}
}

func TestManualWidthOverrideSticksAcrossSync(t *testing.T) {
	m := NewModel(&config.Config{})
	m.tables = []string{"users"}
	m.results.SetResult(
		[]string{"id", "email"},
		[][]string{{"1", "alice@example.com"}},
		"",
	)
	m.results.SetEditable("users", []string{"id"})
	m.results.SetCursor(0, 1)
	m.syncColWidthMemory()
	auto := m.results.ColWidth(1)

	// Shrink below auto-fit and persist as override.
	for m.results.ColWidth(1) > minColWidth+2 {
		m.resizeResultsColumn(-colResizeStep)
	}
	want := m.results.ColWidth(1)
	if want >= auto {
		t.Fatalf("expected shrink below auto %d, got %d", auto, want)
	}
	if got := m.colOverridesFor("users")["email"]; got != want {
		t.Fatalf("override = %d, want %d", got, want)
	}

	// Re-query with longer content: override must win over auto-fit / floor.
	m.results.SetResult(
		[]string{"id", "email"},
		[][]string{{"2", "verylongemailaddress@example.com"}},
		"",
	)
	m.results.SetEditable("users", []string{"id"})
	m.syncColWidthMemory()
	if m.results.ColWidth(1) != want {
		t.Errorf("after sync width = %d, want override %d", m.results.ColWidth(1), want)
	}
}

func TestManualWidthSessionRoundTrip(t *testing.T) {
	dir := t.TempDir()
	store := session.NewStore(dir)
	conn := newNamedSQLiteConn(t, "ovrdb")

	m := NewModel(&config.Config{})
	m.sessionStore = store
	m.connection = conn
	m.tables = []string{"users"}
	m.results.SetResult(
		[]string{"id", "name"},
		[][]string{{"1", "alexandria"}},
		"",
	)
	m.results.SetEditable("users", []string{"id"})
	m.results.SetCursor(0, 1)
	m.syncColWidthMemory()
	m.resizeResultsColumn(colResizeStep * 5)
	want := m.results.ColWidth(1)
	m.saveSession()

	m2 := NewModel(&config.Config{})
	m2.sessionStore = store
	m2.connection = conn
	m2.tables = []string{"users"}
	m2.restoreSession()
	if got := m2.colOverridesFor("users")["name"]; got != want {
		t.Errorf("restored override = %d, want %d", got, want)
	}
}
