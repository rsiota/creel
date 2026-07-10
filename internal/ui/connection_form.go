package ui

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/ruben/gsql/internal/config"
	"github.com/ruben/gsql/internal/secrets"
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
	fieldSSHHost
	fieldSSHPort
	fieldSSHUser
	fieldSSHKeyPath
	fieldSSHPassword
	fieldSecrets  // keychain vs plaintext storage for secret fields
	fieldReadOnly // yes/no toggle
	fieldCount
)

// formLabelWidth is the rendered width of the label column.
// formLabelOverhead is label width + 1 space separator.
const (
	formLabelWidth   = 12
	formLabelOverhead = formLabelWidth + 1
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
	f.fields[fieldPass].SetValue(resolveSecretOrKeep(cfg.Password))
	f.fields[fieldSSHHost].SetValue(cfg.SSHHost)
	if cfg.SSHPort > 0 {
		f.fields[fieldSSHPort].SetValue(strconv.Itoa(cfg.SSHPort))
	}
	f.fields[fieldSSHUser].SetValue(cfg.SSHUser)
	f.fields[fieldSSHKeyPath].SetValue(cfg.SSHKeyPath)
	f.fields[fieldSSHPassword].SetValue(resolveSecretOrKeep(cfg.SSHPassword))

	// Resolve any keychain references into plaintext so the masked fields show
	// the real value (same UX as a plaintext config). If a reference cannot be
	// resolved (e.g. keychain locked), leave the reference in place so a save
	// without changes preserves it as-is.
	f.fields[fieldSecrets].SetValue(secretsModeFromConfig(cfg))
	f.fields[fieldReadOnly].SetValue(boolField(cfg.ReadOnly))
	f.setDriverField(cfg.Driver)
	return f
}

// secretsModeFromConfig infers the secret-storage preference from an existing
// config: "keychain" if any secret field is a reference, otherwise "plain" so
// re-saving does not silently migrate a plaintext config.
func secretsModeFromConfig(cfg config.ConnectionConfig) string {
	if secrets.IsReference(cfg.Password) ||
		secrets.IsReference(cfg.SSHPassword) ||
		secrets.IsReference(cfg.SSHPassphrase) {
		return "keychain"
	}
	return "plain"
}

// resolveSecretOrKeep returns the plaintext value for a possibly-referenced
// secret. If resolution fails (e.g. the keychain is unavailable or locked) the
// original reference is returned unchanged so it is not silently lost on save.
func resolveSecretOrKeep(v string) string {
	resolved, err := secrets.Resolve(v)
	if err != nil {
		return v
	}
	return resolved
}

func newForm(mode formMode, name string) ConnectionForm {
	fields := make([]textinput.Model, fieldCount)

	fields[fieldName] = newTextInput("Connection name", "my-db", false)
	fields[fieldDriver] = newTextInput("Driver (sqlite/mysql/postgres)", "sqlite", false)
	fields[fieldDatabase] = newTextInput("Database (sqlite path; optional for mysql/pg)", "/path/to/db.sqlite", false)
	fields[fieldHost] = newTextInput("Host (mysql/postgres only)", "localhost", false)
	fields[fieldPort] = newTextInput("Port (mysql default 3306, postgres default 5432)", "3306", false)
	fields[fieldUser] = newTextInput("Username (mysql/postgres only)", "root", false)
	fields[fieldPass] = newTextInput("Password (mysql/postgres only)", "", true)
	fields[fieldSSHHost] = newTextInput("SSH host (optional)", "", false)
	fields[fieldSSHPort] = newTextInput("SSH port (default 22)", "22", false)
	fields[fieldSSHUser] = newTextInput("SSH user", "", false)
	fields[fieldSSHKeyPath] = newTextInput("SSH key path (~/.ssh/id_rsa)", "", false)
	fields[fieldSSHPassword] = newTextInput("SSH password (optional)", "", true)
	fields[fieldSecrets] = newTextInput("Secret storage (keychain/plain)", "", false)
	fields[fieldReadOnly] = newTextInput("Read-only (yes/no)", "", false)

	fields[fieldName].SetValue(name)
	fields[fieldSecrets].SetValue("keychain")
	fields[fieldReadOnly].SetValue("no")
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
	ti.PlaceholderStyle = lipgloss.NewStyle().Foreground(colorMuted)
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

	labels := []string{
		"Name", "Driver", "Database", "Host", "Port", "Username", "Password",
		"SSH Host", "SSH Port", "SSH User", "SSH Key", "SSH Pass", "Secrets",
		"Read-only",
	}

	for i, label := range labels {
		labelStyled := lipgloss.NewStyle().
			Foreground(colorPrimary).
			Bold(true).
			Width(formLabelWidth).
			Render(label)

		var inputRendered string
		if i == f.active {
			inputRendered = renderEditInput(f.fields[i], f.fields[i].Width, colorFg)
		} else {
			inputRendered = f.fields[i].View()
		}

		line := fmt.Sprintf("%s %s", labelStyled, inputRendered)
		b = append(b, line)
	}

	if f.errMsg != "" {
		b = append(b, errorStyle.Render(f.errMsg))
	}

	b = append(b, "")

	return lipgloss.JoinVertical(lipgloss.Left, b...)
}

// SetSize sets the dimensions of the form.
func (f *ConnectionForm) SetSize(width, height int) {
	f.width = width
	f.height = height
	for i := range f.fields {
		f.fields[i].Width = width - formLabelOverhead
	}
}

// SetMaxWidth adjusts the text input widths so the form renders at the given
// content width (excluding border/padding).
func (f *ConnectionForm) SetMaxWidth(width int) {
	f.width = width
	for i := range f.fields {
		f.fields[i].Width = width - formLabelOverhead
	}
}

// Focus first field.
func (f *ConnectionForm) Focus() tea.Cmd {
	return f.fields[fieldName].Focus()
}

// EnterPressed is called when enter is pressed; it validates and returns the
// connection config, or an error message.
func (f *ConnectionForm) EnterPressed() (config.ConnectionConfig, string) {
	name := strings.TrimSpace(f.fields[fieldName].Value())
	if name == "" {
		return config.ConnectionConfig{}, "name is required"
	}

	driver := strings.TrimSpace(f.fields[fieldDriver].Value())
	if driver != "sqlite" && driver != "mysql" && driver != "postgres" {
		return config.ConnectionConfig{}, "driver must be 'sqlite', 'mysql', or 'postgres'"
	}

	database := f.fields[fieldDatabase].Value()
	if database == "" && driver != "mysql" && driver != "postgres" {
		return config.ConnectionConfig{}, "database is required"
	}

	defaultPort := 3306
	if driver == "postgres" {
		defaultPort = 5432
	}
	port := defaultPort
	if portStr := f.fields[fieldPort].Value(); portStr != "" {
		var err error
		port, err = strconv.Atoi(portStr)
		if err != nil {
			return config.ConnectionConfig{}, "port must be a number"
		}
	}

	sshPort := 22
	if sshPortStr := f.fields[fieldSSHPort].Value(); sshPortStr != "" {
		var err error
		sshPort, err = strconv.Atoi(sshPortStr)
		if err != nil {
			return config.ConnectionConfig{}, "SSH port must be a number"
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
		ReadOnly: parseBoolField(f.fields[fieldReadOnly].Value()),

		SSHHost:     f.fields[fieldSSHHost].Value(),
		SSHPort:     sshPort,
		SSHUser:     f.fields[fieldSSHUser].Value(),
		SSHKeyPath:  f.fields[fieldSSHKeyPath].Value(),
		SSHPassword: f.fields[fieldSSHPassword].Value(),
	}, ""
}

// ClearError resets the error message.
func (f *ConnectionForm) ClearError() {
	f.errMsg = ""
}

// secretsMode returns the normalized secret-storage preference from the form:
// "keychain" (the default when blank) or "plain". Any value not starting with
// "key" is treated as plaintext so typos never silently enable the keychain.
func (f ConnectionForm) secretsMode() string {
	v := strings.ToLower(strings.TrimSpace(f.fields[fieldSecrets].Value()))
	if v == "" || strings.HasPrefix(v, "key") {
		return "keychain"
	}
	return "plain"
}

// SetError sets an error message.
func (f *ConnectionForm) SetError(msg string) {
	f.errMsg = msg
}

// boolField renders a bool as the form's toggle vocabulary ("yes"/"no").
func boolField(b bool) string {
	if b {
		return "yes"
	}
	return "no"
}

// parseBoolField interprets the read-only toggle. "yes", "true", "y", "1",
// and "ro" enable it; anything else is off.
func parseBoolField(v string) bool {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "yes", "true", "y", "1", "ro", "readonly", "read-only":
		return true
	}
	return false
}
