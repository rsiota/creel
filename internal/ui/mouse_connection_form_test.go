package ui

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/rsiota/creel/internal/config"
)

func newConnFormMouseModel(t *testing.T) Model {
	t.Helper()
	cfg := &config.Config{}
	m := NewModel(cfg)
	mm, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = mm.(Model)
	m.state = stateAddConnection
	m.connForm = NewConnectionForm()
	innerW, capH := popupContentSize(m.height)
	m.connForm.SetSize(innerW, capH)
	return m
}

// connFormFieldScreenY returns the absolute screen Y of a field's label line.
func connFormFieldScreenY(t *testing.T, m Model, label string) int {
	t.Helper()
	view := ansiStrip.ReplaceAllString(m.viewAddConnection(), "")
	for i, line := range strings.Split(view, "\n") {
		if strings.Contains(line, label) {
			return i
		}
	}
	t.Fatalf("connection form field %q not found", label)
	return -1
}

func TestConnFormClickFocusesField(t *testing.T) {
	m := newConnFormMouseModel(t)
	x := m.width / 2
	y := connFormFieldScreenY(t, m, "Database")

	out, _ := m.handleConnectionFormMouse(tea.MouseMsg{Type: tea.MouseLeft, X: x, Y: y})
	m = out.(Model)
	if m.connForm.active != 2 {
		t.Errorf("active=%d, want 2 (database field for sqlite)", m.connForm.active)
	}
	if m.connForm.editing {
		t.Error("single click should not enter edit mode")
	}
}

func TestConnFormDoubleClickEntersEditMode(t *testing.T) {
	m := newConnFormMouseModel(t)
	x := m.width / 2
	y := connFormFieldScreenY(t, m, "Database")

	out, _ := m.handleConnectionFormMouse(tea.MouseMsg{Type: tea.MouseLeft, X: x, Y: y})
	m = out.(Model)
	if m.connForm.editing {
		t.Fatal("single click should not enter edit mode")
	}

	out, _ = m.handleConnectionFormMouse(tea.MouseMsg{Type: tea.MouseLeft, X: x, Y: y})
	m = out.(Model)
	if !m.connForm.editing {
		t.Error("double click should enter field edit mode")
	}
}

func TestConnFormDoubleClickDifferentFieldsDoesNotEdit(t *testing.T) {
	m := newConnFormMouseModel(t)
	x := m.width / 2
	yName := connFormFieldScreenY(t, m, "Name")
	yDB := connFormFieldScreenY(t, m, "Database")

	out, _ := m.handleConnectionFormMouse(tea.MouseMsg{Type: tea.MouseLeft, X: x, Y: yName})
	m = out.(Model)
	out, _ = m.handleConnectionFormMouse(tea.MouseMsg{Type: tea.MouseLeft, X: x, Y: yDB})
	m = out.(Model)
	if m.connForm.editing {
		t.Error("clicking two different fields should not enter edit mode")
	}
}

func TestConnFormDoubleClickChoiceFieldDoesNotEdit(t *testing.T) {
	m := newConnFormMouseModel(t)
	x := m.width / 2
	y := connFormFieldScreenY(t, m, "Driver")

	out, _ := m.handleConnectionFormMouse(tea.MouseMsg{Type: tea.MouseLeft, X: x, Y: y})
	m = out.(Model)
	out, _ = m.handleConnectionFormMouse(tea.MouseMsg{Type: tea.MouseLeft, X: x, Y: y})
	m = out.(Model)
	if m.connForm.editing {
		t.Error("double click on choice field should not enter edit mode")
	}
}

func TestConnFormClickExitsEditOnDifferentField(t *testing.T) {
	m := newConnFormMouseModel(t)
	x := m.width / 2
	yName := connFormFieldScreenY(t, m, "Name")
	yDB := connFormFieldScreenY(t, m, "Database")

	out, _ := m.handleConnectionFormMouse(tea.MouseMsg{Type: tea.MouseLeft, X: x, Y: yDB})
	m = out.(Model)
	out, _ = m.handleConnectionFormMouse(tea.MouseMsg{Type: tea.MouseLeft, X: x, Y: yDB})
	m = out.(Model)
	if !m.connForm.editing {
		t.Fatal("expected edit mode after double click")
	}

	out, _ = m.handleConnectionFormMouse(tea.MouseMsg{Type: tea.MouseLeft, X: x, Y: yName})
	m = out.(Model)
	if m.connForm.editing {
		t.Error("clicking another field should exit edit mode")
	}
	if m.connForm.active != 0 {
		t.Errorf("active=%d, want 0 (name field)", m.connForm.active)
	}
}

func TestConnFormWheelDebouncesNotch(t *testing.T) {
	m := newConnFormMouseModel(t)
	x := m.width / 2
	y := connFormFieldScreenY(t, m, "Name")
	if m.connForm.active != 0 {
		t.Fatalf("start active=%d, want 0", m.connForm.active)
	}

	// Three rapid wheel-downs (typical macOS notch) should advance one field.
	for i := 0; i < 3; i++ {
		out, _ := m.handleConnectionFormMouse(tea.MouseMsg{Type: tea.MouseWheelDown, X: x, Y: y})
		m = out.(Model)
	}
	if m.connForm.active != 1 {
		t.Errorf("after 3-event notch: active=%d, want 1", m.connForm.active)
	}

	// After the debounce window, another notch may advance again.
	m.lastConnFormWheelTime = time.Now().Add(-connFormWheelInterval - time.Millisecond)
	out, _ := m.handleConnectionFormMouse(tea.MouseMsg{Type: tea.MouseWheelDown, X: x, Y: y})
	m = out.(Model)
	if m.connForm.active != 2 {
		t.Errorf("after debounce window: active=%d, want 2", m.connForm.active)
	}
}

