package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/ruben/creel/internal/db"
)

// TableRenameForm is a modal form for renaming a table from the sidebar.
type TableRenameForm struct {
	table    string
	driver   db.Driver
	existing []string
	field    textinput.Model
	visible  bool
	errMsg   string
	width    int
}

// NewTableRenameForm creates a hidden table rename form.
func NewTableRenameForm() TableRenameForm {
	return TableRenameForm{}
}

// Show opens the form for the given table.
func (f *TableRenameForm) Show(table string, driver db.Driver, existing []string) {
	f.table = table
	f.driver = driver
	f.existing = append([]string(nil), existing...)
	f.visible = true
	f.errMsg = ""
	f.field = newAddColumnInput("new table name", table)
	f.field.Focus()
}

// Hide closes the form.
func (f *TableRenameForm) Hide() {
	f.visible = false
	f.table = ""
	f.existing = nil
	f.field = textinput.Model{}
	f.errMsg = ""
}

// IsVisible reports whether the form is open.
func (f TableRenameForm) IsVisible() bool {
	return f.visible
}

// Table returns the table being renamed.
func (f TableRenameForm) Table() string {
	return f.table
}

// NewName returns the trimmed new table name from the input field.
func (f TableRenameForm) NewName() string {
	return strings.TrimSpace(f.field.Value())
}

// Update handles keyboard input for the form.
func (f TableRenameForm) Update(msg tea.Msg) (TableRenameForm, tea.Cmd) {
	if !f.visible {
		return f, nil
	}
	var cmd tea.Cmd
	f.field, cmd = f.field.Update(msg)
	return f, cmd
}

// Submit validates the form and returns generated SQL, or an error message.
func (f TableRenameForm) Submit() (string, string) {
	sql, err := db.BuildRenameTableSQL(f.driver, f.table, f.NewName(), f.existing)
	if err != nil {
		return "", err.Error()
	}
	return sql, ""
}

// SetError sets a validation or execution error on the form.
func (f *TableRenameForm) SetError(msg string) {
	f.errMsg = msg
}

// SetMaxWidth adjusts input width for the popup panel.
func (f *TableRenameForm) SetMaxWidth(width int) {
	f.width = width
	f.field.Width = width - 22
	if f.field.Width < 20 {
		f.field.Width = 20
	}
}

// Focus focuses the name field.
func (f *TableRenameForm) Focus() tea.Cmd {
	return f.field.Focus()
}

// View renders the table rename form content.
func (f TableRenameForm) View() string {
	if !f.visible {
		return ""
	}

	var lines []string
	labelStyled := lipgloss.NewStyle().
		Foreground(colorPrimary).
		Bold(true).
		Width(10).
		Render("New name")
	inputRendered := renderEditInput(f.field, f.field.Width, colorFg)
	lines = append(lines, fmt.Sprintf("%s %s", labelStyled, inputRendered))

	if f.errMsg != "" {
		lines = append(lines, errorStyle.Render(f.errMsg))
	}

	return lipgloss.JoinVertical(lipgloss.Left, lines...)
}
