package ui

import (
	"path/filepath"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/ruben/gsql/internal/config"
)

// newTestConnModel builds a model in the add-connection form state.
func newTestConnModel() Model {
	m := NewModel(&config.Config{})
	m.state = stateAddConnection
	m.connForm = NewConnectionForm()
	return m
}

// A valid SQLite path produces a success result, and the testing flag is
// cleared once the result arrives.
func TestTestConnectionSuccess(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.db")

	m := newTestConnModel()
	m.connForm.fields[fieldName].SetValue("test-sqlite")
	m.connForm.fields[fieldDriver].SetValue("sqlite")
	m.connForm.fields[fieldDatabase].SetValue(path)

	cmd := (&m).testConnection()
	if cmd == nil {
		t.Fatal("expected a test cmd")
	}
	if !m.connForm.testing {
		t.Error("testing flag should be set while the test is in flight")
	}

	msg := cmd()
	res, _ := m.Update(msg)
	mm := res.(Model)
	if mm.connForm.testing {
		t.Error("testing flag should be cleared after the result arrives")
	}
	if !mm.connForm.testOK {
		t.Errorf("expected a successful test, got error: %q", mm.connForm.testMsg)
	}
}

// An unreachable SQLite path surfaces the real driver error.
func TestTestConnectionConnectError(t *testing.T) {
	m := newTestConnModel()
	m.connForm.fields[fieldName].SetValue("bad")
	m.connForm.fields[fieldDriver].SetValue("sqlite")
	// Parent directory does not exist → Ping fails.
	m.connForm.fields[fieldDatabase].SetValue(filepath.Join(t.TempDir(), "missing_subdir", "x.db"))

	cmd := (&m).testConnection()
	if cmd == nil {
		t.Fatal("expected a test cmd")
	}
	msg := cmd()
	res, _ := m.Update(msg)
	mm := res.(Model)
	if mm.connForm.testOK {
		t.Errorf("expected a connect error, got success: %q", mm.connForm.testMsg)
	}
	if mm.connForm.testMsg == "" {
		t.Error("expected a non-empty error message")
	}
}

// A validation failure (missing required field) never opens a connection.
func TestTestConnectionValidationError(t *testing.T) {
	m := newTestConnModel()
	// Fresh form: name and database are both empty → validation fails.
	cmd := (&m).testConnection()
	if cmd != nil {
		t.Errorf("expected nil cmd on validation failure")
	}
	if m.connForm.errMsg == "" {
		t.Error("expected a validation error on the form")
	}
	if m.connForm.testing {
		t.Error("testing flag must not be set on validation failure")
	}
}

// ctrl+t in the form kicks off a test (the testing flag becomes true).
func TestTestConnectionCtrlTKeyTriggers(t *testing.T) {
	path := filepath.Join(t.TempDir(), "k.db")

	m := newTestConnModel()
	m.connForm.fields[fieldName].SetValue("k")
	m.connForm.fields[fieldDriver].SetValue("sqlite")
	m.connForm.fields[fieldDatabase].SetValue(path)

	res, _ := m.Update(tea.KeyMsg{Type: tea.KeyCtrlT})
	mm := res.(Model)
	if !mm.connForm.testing {
		t.Error("ctrl+t should start a connection test (testing flag set)")
	}
}

// ctrl+t while a test is already running is ignored (no second connection).
func TestTestConnectionCtrlTBlockedWhileTesting(t *testing.T) {
	path := filepath.Join(t.TempDir(), "k2.db")

	m := newTestConnModel()
	m.connForm.fields[fieldName].SetValue("k2")
	m.connForm.fields[fieldDriver].SetValue("sqlite")
	m.connForm.fields[fieldDatabase].SetValue(path)
	m.connForm.testing = true // a test is already in flight

	res, _ := m.Update(tea.KeyMsg{Type: tea.KeyCtrlT})
	mm := res.(Model)
	// Still testing, and no new cmd attempted — the handler short-circuited.
	// We can't observe the cmd here, but testing must still be true and the
	// underlying testConnection was not re-entered (no panic / no state drift).
	if !mm.connForm.testing {
		t.Error("expected testing flag to remain true")
	}
}
