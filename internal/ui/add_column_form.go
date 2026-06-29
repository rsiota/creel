package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/ruben/gsql/internal/db"
)

const (
	acFieldName = iota
	acFieldType
	acFieldNullable
	acFieldDefault
	acFieldCount
)

// AddColumnForm is a modal form for adding a column to a table.
type AddColumnForm struct {
	table    string
	driver   db.Driver
	existing []string
	fields   []textinput.Model
	active   int
	visible  bool
	errMsg   string
	width    int
	height   int
}

// NewAddColumnForm creates a hidden add-column form.
func NewAddColumnForm() AddColumnForm {
	return AddColumnForm{}
}

// Show opens the form for the given table.
func (f *AddColumnForm) Show(table string, driver db.Driver, existing []string) {
	f.table = table
	f.driver = driver
	f.existing = append([]string(nil), existing...)
	f.visible = true
	f.errMsg = ""
	f.active = acFieldName
	f.fields = newAddColumnFields(driver)
}

// Hide closes the form.
func (f *AddColumnForm) Hide() {
	f.visible = false
	f.table = ""
	f.existing = nil
	f.fields = nil
	f.errMsg = ""
	f.active = 0
}

// IsVisible reports whether the form is open.
func (f AddColumnForm) IsVisible() bool {
	return f.visible
}

// Table returns the table being altered.
func (f AddColumnForm) Table() string {
	return f.table
}

func newAddColumnFields(driver db.Driver) []textinput.Model {
	typeHint := "TEXT, INTEGER, REAL, BLOB"
	if driver == db.DriverMySQL {
		typeHint = "VARCHAR(255), INT, TEXT, DATETIME"
	}

	fields := make([]textinput.Model, acFieldCount)
	fields[acFieldName] = newAddColumnInput("column name", "")
	fields[acFieldType] = newAddColumnInput(typeHint, "")
	fields[acFieldNullable] = newAddColumnInput("nullable (yes/no)", "yes")
	fields[acFieldDefault] = newAddColumnInput("optional: NULL, 0, 'text'", "")
	fields[acFieldName].Focus()
	return fields
}

func newAddColumnInput(placeholder, def string) textinput.Model {
	ti := textinput.New()
	ti.Placeholder = placeholder
	ti.PlaceholderStyle = lipgloss.NewStyle().Foreground(colorMuted)
	ti.Prompt = ""
	if def != "" {
		ti.SetValue(def)
	}
	return ti
}

// Update handles keyboard input for the form.
func (f AddColumnForm) Update(msg tea.Msg) (AddColumnForm, tea.Cmd) {
	if !f.visible {
		return f, nil
	}
	var cmd tea.Cmd
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "tab":
			f.fields[f.active].Blur()
			f.active++
			if f.active >= acFieldCount {
				f.active = acFieldName
			}
			cmd = f.fields[f.active].Focus()
			return f, cmd
		case "shift+tab":
			f.fields[f.active].Blur()
			f.active--
			if f.active < 0 {
				f.active = acFieldCount - 1
			}
			cmd = f.fields[f.active].Focus()
			return f, cmd
		}
	}
	f.fields[f.active], cmd = f.fields[f.active].Update(msg)
	return f, cmd
}

// Submit validates the form and returns generated SQL, or an error message.
func (f AddColumnForm) Submit() (string, string) {
	col, errMsg := f.columnDef()
	if errMsg != "" {
		return "", errMsg
	}
	sql, err := db.BuildAddColumnSQL(f.driver, f.table, col, f.existing)
	if err != nil {
		return "", err.Error()
	}
	return sql, ""
}

func (f AddColumnForm) columnDef() (db.ColumnDef, string) {
	name := strings.TrimSpace(f.fields[acFieldName].Value())
	colType := strings.TrimSpace(f.fields[acFieldType].Value())
	nullable, errMsg := parseNullable(f.fields[acFieldNullable].Value())
	if errMsg != "" {
		return db.ColumnDef{}, errMsg
	}
	defaultVal := strings.TrimSpace(f.fields[acFieldDefault].Value())
	col := db.ColumnDef{
		Name:    name,
		Type:    colType,
		NotNull: !nullable,
	}
	if defaultVal != "" {
		col.HasDefault = true
		col.Default = defaultVal
	}
	if err := db.ValidateAddColumn(col, f.existing); err != nil {
		return db.ColumnDef{}, err.Error()
	}
	return col, ""
}

func parseNullable(raw string) (bool, string) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "", "yes", "y", "true":
		return true, ""
	case "no", "n", "false":
		return false, ""
	default:
		return false, fmt.Sprintf("nullable must be yes or no, got %q", raw)
	}
}

// SetError sets a validation or execution error on the form.
func (f *AddColumnForm) SetError(msg string) {
	f.errMsg = msg
}

// SetMaxWidth adjusts input widths for the popup panel.
func (f *AddColumnForm) SetMaxWidth(width int) {
	f.width = width
	for i := range f.fields {
		f.fields[i].Width = width - 22
		if f.fields[i].Width < 20 {
			f.fields[i].Width = 20
		}
	}
}

// Focus focuses the first field.
func (f *AddColumnForm) Focus() tea.Cmd {
	if len(f.fields) == 0 {
		return nil
	}
	return f.fields[acFieldName].Focus()
}

// View renders the add-column form content.
func (f AddColumnForm) View() string {
	if !f.visible {
		return ""
	}

	var lines []string

	labels := []string{"Name", "Type", "Nullable", "Default"}
	for i, label := range labels {
		labelStyled := lipgloss.NewStyle().
			Foreground(colorPrimary).
			Bold(true).
			Width(10).
			Render(label)
		inputRendered := f.fields[i].View()
		if i == f.active {
			inputRendered = lipgloss.NewStyle().Foreground(colorFg).Render(inputRendered)
		}
		lines = append(lines, fmt.Sprintf("%s %s", labelStyled, inputRendered))
	}

	if f.errMsg != "" {
		lines = append(lines, errorStyle.Render(f.errMsg))
	}

	return lipgloss.JoinVertical(lipgloss.Left, lines...)
}
