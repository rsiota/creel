package ui

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/rsiota/creel/internal/config"
)

func TestResolveNameInList(t *testing.T) {
	names := []string{"prod", "staging", "analytics"}
	cases := []struct {
		q, want string
	}{
		{"prod", "prod"},
		{"PROD", "prod"},
		{"stag", "staging"},
		{"nope", ""},
		{"", ""},
	}
	for _, c := range cases {
		if got := resolveNameInList(c.q, names); got != c.want {
			t.Errorf("resolveNameInList(%q) = %q, want %q", c.q, got, c.want)
		}
	}
}

func TestExConnectNoConnections(t *testing.T) {
	m := &Model{config: &config.Config{}, editor: NewQueryEditor(), results: NewResultsTable()}
	m.runExCommand("connect prod")
	if !strings.Contains(m.schemaMsg, "no connections") {
		t.Errorf(":connect with empty config -> %q", m.schemaMsg)
	}
}

func TestExConnectUnknown(t *testing.T) {
	m := &Model{
		config:  &config.Config{Connections: []config.ConnectionConfig{{Name: "prod", Driver: "sqlite", Database: ":memory:"}}},
		editor:  NewQueryEditor(),
		results: NewResultsTable(),
	}
	m.runExCommand("connect nope")
	if !strings.Contains(m.schemaMsg, "no such connection") {
		t.Errorf(":connect unknown -> %q", m.schemaMsg)
	}
}

func TestExConnectByNameSQLite(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")
	// Touch an empty file — SQLite creates on connect.
	m := &Model{
		config: &config.Config{Connections: []config.ConnectionConfig{
			{Name: "local", Driver: "sqlite", Database: dbPath},
		}},
		editor:      NewQueryEditor(),
		results:     NewResultsTable(),
		resultsTabs: []*ResultsTab{NewResultsTab(0, "New Query")},
		tabBar:      NewTabBar(),
		width:       120,
		height:      40,
	}
	cmd := m.runExCommand("c local")
	if m.connection == nil {
		t.Fatalf(":c local failed: %q", m.schemaMsg)
	}
	if !strings.Contains(m.schemaMsg, "connected: local") {
		t.Errorf("success msg -> %q", m.schemaMsg)
	}
	if m.state != stateWorkspace {
		t.Errorf("state = %v, want workspace", m.state)
	}
	// cmd may be a batch (focus + prefetch); just ensure no panic.
	_ = cmd
	m.connection.Close()
}

func TestExConnectBareOpensList(t *testing.T) {
	m := &Model{
		config: &config.Config{Connections: []config.ConnectionConfig{
			{Name: "local", Driver: "sqlite", Database: ":memory:"},
		}},
		editor:      NewQueryEditor(),
		results:     NewResultsTable(),
		resultsTabs: []*ResultsTab{NewResultsTab(0, "New Query")},
		tabBar:      NewTabBar(),
		state:       stateWorkspace,
		width:       120,
		height:      40,
	}
	m.runExCommand("connect")
	if m.state != stateConnections {
		t.Errorf("bare :connect -> state %v, want connections", m.state)
	}
}

func TestExConnections(t *testing.T) {
	m := &Model{
		config:      &config.Config{},
		editor:      NewQueryEditor(),
		results:     NewResultsTable(),
		resultsTabs: []*ResultsTab{NewResultsTab(0, "New Query")},
		tabBar:      NewTabBar(),
		state:       stateWorkspace,
		width:       120,
		height:      40,
	}
	m.runExCommand("connections")
	if m.state != stateConnections {
		t.Errorf(":connections -> state %v, want connections", m.state)
	}
}

func TestExDBNotConnected(t *testing.T) {
	m := &Model{}
	m.runExCommand("db")
	if !strings.Contains(m.schemaMsg, "not connected") {
		t.Errorf(":db -> %q", m.schemaMsg)
	}
}

func TestExDBSQLite(t *testing.T) {
	conn := newSQLiteTestConn(t)
	defer conn.Close()
	m := &Model{connection: conn}
	m.runExCommand("use other")
	if !strings.Contains(m.schemaMsg, "not supported") {
		t.Errorf(":use on sqlite -> %q", m.schemaMsg)
	}
}

func TestExSchemaSQLite(t *testing.T) {
	conn := newSQLiteTestConn(t)
	defer conn.Close()
	m := &Model{connection: conn}
	m.runExCommand("schema public")
	if !strings.Contains(m.schemaMsg, "not supported") {
		t.Errorf(":schema on sqlite -> %q", m.schemaMsg)
	}
}

func TestExSchemaNotConnected(t *testing.T) {
	m := &Model{}
	m.runExCommand("schema")
	if !strings.Contains(m.schemaMsg, "not connected") {
		t.Errorf(":schema -> %q", m.schemaMsg)
	}
}
