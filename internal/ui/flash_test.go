package ui

import "testing"

func TestFlashSnapshotChangedActiveClear(t *testing.T) {
	var m Model

	// Empty model: nothing active.
	if m.anyFlashActive() {
		t.Error("expected no flash active on empty model")
	}

	prev := m.flashSnapshot()

	// Set one flash field.
	m.schemaMsg = "schema change failed: FK constraint"
	if !m.flashChanged(prev) {
		t.Error("expected flashChanged after setting schemaMsg")
	}
	if !m.anyFlashActive() {
		t.Error("expected anyFlashActive after setting schemaMsg")
	}

	// Clear should empty all six fields.
	m.clearFlash()
	if m.anyFlashActive() {
		t.Error("expected no flash active after clearFlash")
	}
	for i, v := range m.flashSnapshot() {
		if v != "" {
			t.Errorf("field %d not cleared: %q", i, v)
		}
	}
}

func TestFlashChangedDetectsAllFields(t *testing.T) {
	fields := []struct {
		setter func(m *Model)
	}{
		{func(m *Model) { m.statsMsg = "x" }},
		{func(m *Model) { m.exportMsg = "x" }},
		{func(m *Model) { m.searchMsg = "x" }},
		{func(m *Model) { m.truncateMsg = "x" }},
		{func(m *Model) { m.deleteRowsMsg = "x" }},
		{func(m *Model) { m.schemaMsg = "x" }},
	}
	for i, f := range fields {
		var m Model
		prev := m.flashSnapshot()
		f.setter(&m)
		if !m.flashChanged(prev) {
			t.Errorf("field %d: flashChanged not detected", i)
		}
	}
}
