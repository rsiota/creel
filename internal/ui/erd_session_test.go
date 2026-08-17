package ui

import (
	"testing"

	"github.com/rsiota/creel/internal/config"
	"github.com/rsiota/creel/internal/db"
	"github.com/rsiota/creel/internal/session"
)

func TestSnapshotAndApplyERDPositions(t *testing.T) {
	tables, schemas, pks, fks := erdFixture()
	layout := computeERDLayout(tables, schemas, pks, fks)
	if layout == nil {
		t.Fatal("nil layout")
	}
	users := cardByName(layout.cards, "users")
	origX, origY := users.x, users.y
	users.x += 15
	users.y += 7

	saved := snapshotERDLayout(layout)
	if saved["users"] != (session.ERDPos{X: origX + 15, Y: origY + 7}) {
		t.Fatalf("snapshot users = %+v", saved["users"])
	}

	// A freshly ranked layout should pick the saved origin back up.
	fresh := computeERDLayout(tables, schemas, pks, fks)
	applyERDPositions(fresh, saved)
	got := cardByName(fresh.cards, "users")
	if got.x != origX+15 || got.y != origY+7 {
		t.Errorf("applied users at (%d,%d), want (%d,%d)", got.x, got.y, origX+15, origY+7)
	}
	if len(fresh.arrows) > 0 && fresh.arrows[0].pts == nil {
		t.Error("apply should re-route arrows after a move")
	}
}

func TestApplyERDPositionsCaseInsensitive(t *testing.T) {
	tables, schemas, pks, fks := erdFixture()
	layout := computeERDLayout(tables, schemas, pks, fks)
	users := cardByName(layout.cards, "users")
	wantX := users.x + 4
	applyERDPositions(layout, map[string]session.ERDPos{
		"USERS": {X: wantX, Y: users.y},
	})
	if users.x != wantX {
		t.Errorf("users.x = %d, want %d (case-insensitive key)", users.x, wantX)
	}
}

func TestOpenERDRestoresDraggedPositions(t *testing.T) {
	tables, schemas, pks, fks := erdFixture()
	m := Model{
		connection:  &db.Connection{},
		tables:      tables,
		columnCache: schemas,
		pkCache:     pks,
		fkCache:     fks,
		width:       80,
		height:      24,
		editor:      NewQueryEditor(),
	}
	m.openERD("")
	users := m.erdPanel.cardNamed("users")
	if users == nil {
		t.Fatal("missing users card")
	}
	wantX, wantY := users.x+12, users.y+6
	users.x, users.y = wantX, wantY
	m.hideERD()

	m.openERD("")
	got := m.erdPanel.cardNamed("users")
	if got == nil {
		t.Fatal("users missing after reopen")
	}
	if got.x != wantX || got.y != wantY {
		t.Errorf("restored users at (%d,%d), want (%d,%d)", got.x, got.y, wantX, wantY)
	}
}

func TestERDPositionsDoNotLeakAcrossScopes(t *testing.T) {
	tables, schemas, pks, fks := erdFixture()
	m := Model{
		connection:  &db.Connection{},
		tables:      tables,
		columnCache: schemas,
		pkCache:     pks,
		fkCache:     fks,
		width:       80,
		height:      24,
		editor:      NewQueryEditor(),
	}
	m.openERD("")
	users := m.erdPanel.cardNamed("users")
	dragged := users.x + 50
	users.x = dragged
	m.hideERD()

	// Neighbourhood layout uses a different scope — whole-schema coords must
	// not be applied (ranks differ).
	m.openERD("users")
	freshTargets := erdFocusSet("users", tables, fks)
	fresh := computeERDLayout(freshTargets, schemas, pks, fks)
	got := m.erdPanel.cardNamed("users")
	want := cardByName(fresh.cards, "users")
	if got.x == dragged && dragged != want.x {
		t.Fatalf("neighbourhood inherited whole-schema x=%d", got.x)
	}
	if got.x != want.x || got.y != want.y {
		t.Errorf("neighbourhood users at (%d,%d), want ranked (%d,%d)", got.x, got.y, want.x, want.y)
	}

	// A neighbourhood drag must not clobber the whole-schema memory.
	got.x += 9
	m.hideERD()
	m.openERD("")
	whole := m.erdPanel.cardNamed("users")
	if whole.x != dragged {
		t.Errorf("whole-schema users at %d, want dragged %d", whole.x, dragged)
	}
}

func TestERDPosMemorySessionRoundTrip(t *testing.T) {
	dir := t.TempDir()
	store := session.NewStore(dir)

	m := NewModel(&config.Config{})
	m.sessionStore = store
	conn := newNamedSQLiteConn(t, "erddb")
	m.connection = conn
	m.erdPosMem = map[string]map[string]session.ERDPos{
		"*": {"users": {X: 11, Y: 3}},
	}
	m.saveSession()

	m2 := NewModel(&config.Config{})
	m2.sessionStore = store
	m2.connection = conn
	m2.restoreSession()
	if got := m2.erdPosMem["*"]["users"]; got != (session.ERDPos{X: 11, Y: 3}) {
		t.Errorf("restored %+v", got)
	}

	// Positions load even when tabs are blank (HasContent false).
	_ = store.Save(conn.Config().Name, conn.Config().Database, session.State{
		ERDPositions: map[string]map[string]session.ERDPos{
			"*": {"orders": {X: 4, Y: 5}},
		},
	})
	m3 := NewModel(&config.Config{})
	m3.sessionStore = store
	m3.connection = conn
	m3.restoreSession()
	if got := m3.erdPosMem["*"]["orders"]; got != (session.ERDPos{X: 4, Y: 5}) {
		t.Errorf("positions-only restore = %+v", got)
	}
	if len(m3.resultsTabs) != 1 || m3.resultsTabs[0].EditorQuery != "" {
		t.Errorf("blank-tab session should keep default tab, got %+v", m3.resultsTabs)
	}
}

func TestRenameERDPosTable(t *testing.T) {
	m := NewModel(&config.Config{})
	m.erdPosMem = map[string]map[string]session.ERDPos{
		"*":     {"users": {X: 1, Y: 2}, "orders": {X: 3, Y: 4}},
		"users": {"users": {X: 8, Y: 9}, "orders": {X: 6, Y: 7}},
	}
	m.renameERDPosTable("users", "accounts")
	if _, ok := m.erdPosMem["users"]; ok {
		t.Error("old neighbourhood scope should be gone")
	}
	if _, ok := m.erdPosMem["*"]["users"]; ok {
		t.Error("old card key should be gone from whole-schema scope")
	}
	if got := m.erdPosMem["*"]["accounts"]; got != (session.ERDPos{X: 1, Y: 2}) {
		t.Errorf("whole-schema accounts = %+v", got)
	}
	if got := m.erdPosMem["accounts"]["accounts"]; got != (session.ERDPos{X: 8, Y: 9}) {
		t.Errorf("neighbourhood accounts = %+v", got)
	}
	if got := m.erdPosMem["accounts"]["orders"]; got != (session.ERDPos{X: 6, Y: 7}) {
		t.Errorf("neighbourhood orders = %+v", got)
	}
}

func TestSessionClearDropsERDPosMem(t *testing.T) {
	dir := t.TempDir()
	m := NewModel(&config.Config{})
	m.sessionStore = session.NewStore(dir)
	m.connection = newNamedSQLiteConn(t, "erdclr")
	m.erdPosMem = map[string]map[string]session.ERDPos{"*": {"t": {X: 1, Y: 1}}}
	m.saveSession()
	m.runExCommand("session clear")
	if m.erdPosMem != nil {
		t.Errorf("erdPosMem should be nil after clear, got %v", m.erdPosMem)
	}
}
