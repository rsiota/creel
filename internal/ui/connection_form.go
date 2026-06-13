package ui

import (
	"fmt"
	"strconv"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/ruben/gsql/internal/config"
)

// FormField indices.
const (
	fieldName = iota
	fieldDriver
	fieldDatabase
	fieldHost
	fieldPort
	fieldUser
	fieldPass
	fieldCount
)

// formMode determines whether we are adding or editing.
type formMode int

const (
	formModeAdd formMode = iota
	formModeEdit
)

// ConnectionForm is the add/edit connection form.
type ConnectionForm struct {
	fields  []textinput.Model
	active  int
	mode    formMode
	errMsg  string
	width   int
	height  int
	editing string // name of connection being edited (for edit mode)
}

// NewConnectionForm creates a new form for adding connections.
func NewConnectionForm() ConnectionForm {
	return newForm(formModeAdd, "")
}

// NewConnectionFormEdit creates a form pre-filled from an existing connection.
func NewConnectionFormEdit(cfg config.ConnectionConfig) ConnectionForm {
	f := newForm(formModeEdit, cfg.Name)
	f.editing = cfg.Name
	f.fields[fieldDatabase].SetValue(cfg.Database)
	f.fields[fieldHost].SetValue(cfg.Host)
	if cfg.Port > 0 {
		f.fields[fieldPort].SetValue(strconv.Itoa(cfg.Port))
	}
	f.fields[fieldUser].SetValue(cfg.Username)
	f.fields[fieldPass].SetValue(cfg.Password)
	f.setDriverField(cfg.Driver)
	return f
}

func newForm(mode formMode, name string) ConnectionForm {
	fields := make([]textinput.Model, fieldCount)

	fields[fieldName] = newTextInput("Connection name", "my-db", false)
	fields[fieldDriver] = newTextInput("Driver (sqlite/mysql)", "sqlite", false)
	fields[fieldDatabase] = newTextInput("Database (path for sqlite, name for mysql)", "/path/to/db.sqlite", false)
	fields[fieldHost] = newTextInput("Host (mysql only)", "localhost", false)
	fields[fieldPort] = newTextInput("Port (mysql only, default 3306)", "3306", false)
	fields[fieldUser] = newTextInput("Username (mysql only)", "root", false)
	fields[fieldPass] = newTextInput("Password (mysql only)", "", true)

	fields[fieldName].SetValue(name)
	fields[fieldName].Focus()

	return ConnectionForm{
		fields: fields,
		active: fieldName,
		mode:   mode,
	}
}

func newTextInput(placeholder, def string, masked bool) textinput.Model {
	ti := textinput.New()
	ti.Placeholder = placeholder
	ti.Prompt = ""
	if masked {
		ti.EchoMode = textinput.EchoPassword
	}
	return ti
}

func (f *ConnectionForm) setDriverField(driver string) {
	f.fields[fieldDriver].SetValue(driver)
}

// Update handles messages for the form.
func (f ConnectionForm) Update(msg tea.Msg) (ConnectionForm, tea.Cmd) {
	var cmd tea.Cmd
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "tab":
			f.fields[f.active].Blur()
			f.active++
			if f.active >= fieldCount {
				f.active = fieldName
			}
			cmd = f.fields[f.active].Focus()
			return f, cmd
		case "shift+tab":
			f.fields[f.active].Blur()
			f.active--
			if f.active < 0 {
				f.active = fieldCount - 1
			}
			cmd = f.fields[f.active].Focus()
			return f, cmd
		}
	}

	f.fields[f.active], cmd = f.fields[f.active].Update(msg)
	return f, cmd
}

// View renders the form.
func (f ConnectionForm) View() string {
	var b []string

	title := "Add New Connection"
	if f.mode == formModeEdit {
		title = "Edit Connection: " + f.editing
	}
	b = append(b, titleStyle.Render(title))

	labels := []string{
		"Name", "Driver", "Database", "Host", "Port", "Username", "Password",
	}

	for i, label := range labels {
		labelStyled := lipgloss.NewStyle().
			Foreground(colorPrimary).
			Bold(true).
			Width(12).
			Render(label)

		marker := " "
		if i == f.active {
			marker = mutedStyle.Render("→")
		}

		inputRendered := f.fields[i].View()
		if i == f.active {
			inputRendered = lipgloss.NewStyle().
				Foreground(colorFg).
				Render(inputRendered)
		}

		line := fmt.Sprintf("%s %s %s", marker, labelStyled, inputRendered)
		b = append(b, line)
	}

	if f.errMsg != "" {
		b = append(b, errorStyle.Render(f.errMsg))
	}

	b = append(b, "")
	b = append(b, helpStyle.Render("tab: next field  enter: save  esc: cancel"))

	return lipgloss.JoinVertical(lipgloss.Left, b...)
}

// SetSize sets the dimensions of the form.
func (f *ConnectionForm) SetSize(width, height int) {
	f.width = width
	f.height = height
	for i := range f.fields {
		f.fields[i].Width = width - 30
	}
}

// Focus first field.
func (f *ConnectionForm) Focus() tea.Cmd {
	return f.fields[fieldName].Focus()
}

// EnterPressed is called when enter is pressed; it validates and returns the
// connection config, or an error message.
func (f *ConnectionForm) EnterPressed() (config.ConnectionConfig, string) {
	name := f.fields[fieldName].Value()
	if name == "" {
		return config.ConnectionConfig{}, "name is required"
	}

	driver := f.fields[fieldDriver].Value()
	if driver != "sqlite" && driver != "mysql" {
		return config.ConnectionConfig{}, "driver must be 'sqlite' or 'mysql'"
	}

	database := f.fields[fieldDatabase].Value()
	if database == "" {
		return config.ConnectionConfig{}, "database is required"
	}

	port := 3306
	if portStr := f.fields[fieldPort].Value(); portStr != "" {
		var err error
		port, err = strconv.Atoi(portStr)
		if err != nil {
			return config.ConnectionConfig{}, "port must be a number"
		}
	}

	return config.ConnectionConfig{
		Name:     name,
		Driver:   driver,
		Database: database,
		Host:     f.fields[fieldHost].Value(),
		Port:     port,
		Username: f.fields[fieldUser].Value(),
		Password: f.fields[fieldPass].Value(),
	}, ""
}

// ClearError resets the error message.
func (f *ConnectionForm) ClearError() {
	f.errMsg = ""
}

// SetError sets an error message.
func (f *ConnectionForm) SetError(msg string) {
	f.errMsg = msg
}
