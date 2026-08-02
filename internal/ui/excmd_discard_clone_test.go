package ui

import (
	"strings"
	"testing"

	"github.com/rsiota/creel/internal/config"
	"github.com/rsiota/creel/internal/db"
)

func TestExDiscard(t *testing.T) {
	t.Run("nothing to discard", func(t *testing.T) {
		m := &Model{results: NewResultsTable()}
		m.results.SetEditable("users", []string{"id"}) // editable, but no dirty cells
		m.runExCommand("discard")
		if !strings.Contains(m.schemaMsg, "no changes to discard") {
			t.Errorf(":discard -> %q", m.schemaMsg)
		}
	})
	t.Run("not editable", func(t *testing.T) {
		m := &Model{results: NewResultsTable()}
		m.runExCommand("discard")
		if !strings.Contains(m.schemaMsg, "not editable") {
			t.Errorf(":discard not editable -> %q", m.schemaMsg)
		}
	})
	t.Run("stages confirm", func(t *testing.T) {
		yes := true
		m := &Model{results: NewResultsTable(), settings: config.Settings{ConfirmDestructive: &yes}}
		m.results.SetEditable("users", []string{"id"})
		m.results.SetDirtyCell(0, 0, "edited")
		cmd := m.runExCommand("discard")
		if cmd != nil {
			t.Fatal("expected nil cmd while confirm is staged")
		}
		if !m.discardConfirm {
			t.Error("expected discardConfirm to be staged")
		}
		if !m.results.HasDirtyCells() {
			t.Error("dirty cells should still be present while confirm is staged")
		}
	})
	t.Run("force discards immediately", func(t *testing.T) {
		yes := true
		m := &Model{results: NewResultsTable(), settings: config.Settings{ConfirmDestructive: &yes}}
		m.results.SetEditable("users", []string{"id"})
		m.results.SetDirtyCell(0, 0, "edited")
		m.runExCommand("discard!")
		if m.discardConfirm {
			t.Error("force should not stage a confirmation")
		}
		if m.results.HasDirtyCells() {
			t.Error("force should discard the dirty cells")
		}
	})
	t.Run("no confirm setting discards immediately", func(t *testing.T) {
		no := false
		m := &Model{results: NewResultsTable(), settings: config.Settings{ConfirmDestructive: &no}}
		m.results.SetEditable("users", []string{"id"})
		m.results.SetDirtyCell(0, 0, "edited")
		m.runExCommand("discard")
		if m.discardConfirm {
			t.Error("should not stage confirm when confirm_destructive is off")
		}
		if m.results.HasDirtyCells() {
			t.Error("should discard the dirty cells")
		}
	})
}

func TestExClone(t *testing.T) {
	t.Run("not connected", func(t *testing.T) {
		m := &Model{results: NewResultsTable()}
		m.runExCommand("clone")
		if !strings.Contains(m.schemaMsg, "not connected") {
			t.Errorf(":clone -> %q", m.schemaMsg)
		}
	})
	t.Run("nothing editable", func(t *testing.T) {
		conn := newSQLiteTestConn(t)
		defer conn.Close()
		m := &Model{connection: conn, results: NewResultsTable()}
		m.runExCommand("clone")
		if !strings.Contains(m.schemaMsg, "no editable rows") {
			t.Errorf(":clone not editable -> %q", m.schemaMsg)
		}
	})
	t.Run("editable but no rows", func(t *testing.T) {
		conn := newSQLiteTestConn(t)
		defer conn.Close()
		m := &Model{connection: conn, results: NewResultsTable()}
		m.results.SetEditable("users", []string{"id"}) // editable, zero rows
		m.runExCommand("clone")
		if !strings.Contains(m.schemaMsg, "no editable rows") {
			t.Errorf(":clone no rows -> %q", m.schemaMsg)
		}
	})
	t.Run("delegates to cloneRows", func(t *testing.T) {
		// Set up an editable result set with a row + table columns so
		// CloneRowsData yields a cloneable row. Don't call cmd(): that runs the
		// INSERT batch; the point here is that :clone reaches cloneRows.
		conn := newSQLiteTestConn(t)
		defer conn.Close()
		m := &Model{connection: conn, results: NewResultsTable()}
		m.results.SetEditable("users", []string{"id"})
		m.results.SetTableColumns([]db.TableColumnInfo{
			{Name: "id", Type: "INTEGER"},
			{Name: "name", Type: "TEXT"},
		})
		m.results.SetResult([]string{"id", "name"}, [][]string{{"1", "alice"}}, "")
		m.results.SetCursor(0, 0)
		cmd := m.runExCommand("clone")
		if cmd == nil {
			t.Fatalf(":clone -> %q", m.schemaMsg)
		}
	})
}
