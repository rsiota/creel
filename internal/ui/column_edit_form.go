package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/ruben/gsql/internal/db"
)

// ColumnEditForm edits an existing column via schema-panel actions.
type ColumnEditForm struct {
	action   db.SchemaAction
	table    string
	driver   db.Driver
	column   db.TableColumnInfo
	existing []string
	fields   []textinput.Model
	labels   []string
	active   int
	visible  bool
	errMsg   string
	width    int
	height   int
}

// NewColumnEditForm creates a hidden column edit form.
func NewColumnEditForm() ColumnEditForm {
	return ColumnEditForm{}
}

// Show opens the form for a column action.
func (f *ColumnEditForm) Show(action db.SchemaAction, table string, driver db.Driver, column db.TableColumnInfo, existing []string) {
	f.action = action
	f.table = table
	f.driver = driver
	f.column = column
	f.existing = append([]string(nil), existing...)
	f.visible = true
	f.errMsg = ""
	f.active = 0
	f.fields, f.labels = newColumnEditFields(action, column)
}

// Hide closes the form.
func (f *ColumnEditForm) Hide() {
	f.visible = false
	f.fields = nil
	f.labels = nil
	f.errMsg = ""
	f.active = 0
}

// IsVisible reports whether the form is open.
func (f ColumnEditForm) IsVisible() bool {
	return f.visible
}

// Action returns the schema action being edited.
func (f ColumnEditForm) Action() db.SchemaAction {
	return f.action
}

// Table returns the table being altered.
func (f ColumnEditForm) Table() string {
	return f.table
}

func newColumnEditFields(action db.SchemaAction, column db.TableColumnInfo) ([]textinput.Model, []string) {
	switch action {
	case db.SchemaRenameColumn:
		ti := newAddColumnInput("new column name", column.Name)
		return []textinput.Model{ti}, []string{"New name"}
	case db.SchemaModifyType:
		ti := newAddColumnInput("new type", column.Type)
		return []textinput.Model{ti}, []string{"Type"}
	case db.SchemaModifyNullable:
		val := "yes"
		if !column.NotNull {
			val = "yes"
		} else {
			val = "no"
		}
		ti := newAddColumnInput("nullable (yes/no)", val)
		return []textinput.Model{ti}, []string{"Nullable"}
	case db.SchemaModifyDefault:
		def := ""
		if column.HasDefault {
			def = column.DefaultValue
		}
		ti := newAddColumnInput("default (empty removes)", def)
		return []textinput.Model{ti}, []string{"Default"}
	default:
		return nil, nil
	}
}

// Update handles keyboard input for the form.
func (f ColumnEditForm) Update(msg tea.Msg) (ColumnEditForm, tea.Cmd) {
	if !f.visible || len(f.fields) == 0 {
		return f, nil
	}
	var cmd tea.Cmd
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "tab":
			f.fields[f.active].Blur()
			f.active++
			if f.active >= len(f.fields) {
				f.active = 0
			}
			cmd = f.fields[f.active].Focus()
			return f, cmd
		case "shift+tab":
			f.fields[f.active].Blur()
			f.active--
			if f.active < 0 {
				f.active = len(f.fields) - 1
			}
			cmd = f.fields[f.active].Focus()
			return f, cmd
		}
	}
	f.fields[f.active], cmd = f.fields[f.active].Update(msg)
	return f, cmd
}

// Submit validates the form and returns generated SQL, or an error message.
func (f ColumnEditForm) Submit() (string, string) {
	switch f.action {
	case db.SchemaRenameColumn:
		newName := strings.TrimSpace(f.fields[0].Value())
		sql, err := db.BuildRenameColumnSQL(f.driver, f.table, f.column.Name, newName, f.existing)
		if err != nil {
			return "", err.Error()
		}
		return sql, ""
	case db.SchemaModifyType:
		col := db.ColumnDefFromInfo(f.column)
		col.Type = strings.TrimSpace(f.fields[0].Value())
		sql, err := db.BuildModifyColumnSQL(f.driver, f.table, col)
		if err != nil {
			return "", err.Error()
		}
		return sql, ""
	case db.SchemaModifyNullable:
		nullable, errMsg := parseNullable(f.fields[0].Value())
		if errMsg != "" {
			return "", errMsg
		}
		col := db.ColumnDefFromInfo(f.column)
		col.NotNull = !nullable
		sql, err := db.BuildModifyColumnSQL(f.driver, f.table, col)
		if err != nil {
			return "", err.Error()
		}
		return sql, ""
	case db.SchemaModifyDefault:
		col := db.ColumnDefFromInfo(f.column)
		defaultVal := strings.TrimSpace(f.fields[0].Value())
		if defaultVal == "" {
			col.HasDefault = false
			col.Default = ""
		} else {
			col.HasDefault = true
			col.Default = defaultVal
		}
		sql, err := db.BuildModifyColumnSQL(f.driver, f.table, col)
		if err != nil {
			return "", err.Error()
		}
		return sql, ""
	default:
		return "", fmt.Sprintf("unsupported action %q", f.action)
	}
}

// SetError sets a validation or execution error on the form.
func (f *ColumnEditForm) SetError(msg string) {
	f.errMsg = msg
}

// SetMaxWidth adjusts input widths for the popup panel.
func (f *ColumnEditForm) SetMaxWidth(width int) {
	f.width = width
	for i := range f.fields {
		f.fields[i].Width = width - 22
		if f.fields[i].Width < 20 {
			f.fields[i].Width = 20
		}
	}
}

// Focus focuses the first field.
func (f *ColumnEditForm) Focus() tea.Cmd {
	if len(f.fields) == 0 {
		return nil
	}
	return f.fields[0].Focus()
}

// View renders the column edit form content.
func (f ColumnEditForm) View() string {
	if !f.visible {
		return ""
	}

	title := fmt.Sprintf("%s: %s.%s", db.SchemaActionLabel(f.action), f.table, f.column.Name)
	var lines []string
	lines = append(lines, titleStyle.Render(title))

	for i, label := range f.labels {
		marker := " "
		if i == f.active {
			marker = mutedStyle.Render("→")
		}
		labelStyled := lipgloss.NewStyle().
			Foreground(colorPrimary).
			Bold(true).
			Width(10).
			Render(label)
		inputRendered := f.fields[i].View()
		if i == f.active {
			inputRendered = lipgloss.NewStyle().Foreground(colorFg).Render(inputRendered)
		}
		lines = append(lines, fmt.Sprintf("%s %s %s", marker, labelStyled, inputRendered))
	}

	if f.errMsg != "" {
		lines = append(lines, errorStyle.Render(f.errMsg))
	}

	lines = append(lines, "")
	lines = append(lines, mutedStyle.Render("enter submit   esc cancel   tab next field"))
	return lipgloss.JoinVertical(lipgloss.Left, lines...)
}

// GuardColumnAction returns an error if a column action should not run.
func GuardColumnAction(action db.SchemaAction, col db.TableColumnInfo) string {
	switch action {
	case db.SchemaDropColumn:
		if err := db.ValidateDropColumn(col); err != nil {
			return err.Error()
		}
	case db.SchemaRenameColumn:
		if col.PrimaryKey && col.AutoIncrement {
			return fmt.Sprintf("renaming auto-increment column %q is not supported", col.Name)
		}
	case db.SchemaModifyType, db.SchemaModifyNullable, db.SchemaModifyDefault:
		if col.AutoIncrement {
			return fmt.Sprintf("cannot modify auto-increment column %q", col.Name)
		}
	}
	return ""
}
