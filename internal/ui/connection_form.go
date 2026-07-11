package ui

import (
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

// formLabels is the display label for each field, parallel to the field
// indices above. It is shared by rendering and any label lookups.
var formLabels = [...]string{
	"Name", "Driver", "Database", "Host", "Port", "Username", "Password",
	"SSH Host", "SSH Port", "SSH User", "SSH Key", "SSH Pass", "Secrets",
	"Read-only",
}

// formOptional marks fields that are not strictly required, shown with an
// "(optional)" marker in the field's right-aligned slot (mirroring the
// inspector's column-type marker). Required fields show no marker.
var formOptional = [...]bool{
	false, false, false, // name, driver, database
	true, true, true, true, // host, port, username, password
	true, true, true, true, true, // ssh host/port/user/key/pass
	true, true, // secrets (defaults to keychain), read-only (defaults to no)
}

// formMode determines whether we are adding or editing.
type formMode int

const (
	formModeAdd formMode = iota
	formModeEdit
)

// ConnectionForm is the add/edit connection form.
type ConnectionForm struct {
	fields    []textinput.Model
	active    int
	mode      formMode
	errMsg    string
	width     int
	height    int
	scrollRow int    // first visible field index when the form scrolls
	editing   bool   // insert mode: typing into the active field
	editName  string // name of connection being edited (for edit mode)

	// Test-connection feedback. testing is true while a background test is in
	// flight; testMsg/testOK hold the result once it completes.
	testing bool
	testMsg string
	testOK  bool
}

// NewConnectionForm creates a new form for adding connections.
func NewConnectionForm() ConnectionForm {
	return newForm(formModeAdd, "")
}

// NewConnectionFormEdit creates a form pre-filled from an existing connection.
func NewConnectionFormEdit(cfg config.ConnectionConfig) ConnectionForm {
	f := newForm(formModeEdit, cfg.Name)
	f.editName = cfg.Name
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
	fields[fieldSSHHost] = newTextInput("SSH host", "", false)
	fields[fieldSSHPort] = newTextInput("SSH port (default 22)", "22", false)
	fields[fieldSSHUser] = newTextInput("SSH user", "", false)
	fields[fieldSSHKeyPath] = newTextInput("SSH key path (~/.ssh/id_rsa)", "", false)
	fields[fieldSSHPassword] = newTextInput("SSH password", "", true)
	fields[fieldSecrets] = newTextInput("Secret storage (keychain/plain)", "", false)
	fields[fieldReadOnly] = newTextInput("Read-only (yes/no)", "", false)

	fields[fieldName].SetValue(name)
	fields[fieldSecrets].SetValue("keychain")
	fields[fieldReadOnly].SetValue("no")

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

// Update handles messages for the form using a vim-like modal model that
// mirrors the inspector: in normal mode j/k (and Tab/arrows) move the field
// cursor without editing, and e/i/a enter insert mode on the current field.
// In insert mode, keys edit the field; Esc/Enter return to normal mode.
func (f ConnectionForm) Update(msg tea.Msg) (ConnectionForm, tea.Cmd) {
	kmsg, isKey := msg.(tea.KeyMsg)
	if !isKey {
		// Non-key messages (e.g. WindowSizeMsg) only matter in insert mode,
		// where they are forwarded to the active textinput.
		if f.editing {
			var cmd tea.Cmd
			f.fields[f.active], cmd = f.fields[f.active].Update(msg)
			return f, cmd
		}
		return f, nil
	}
	key := kmsg.String()

	if f.editing {
		// Insert mode: Esc/Enter commit the field and return to normal mode;
		// any other key edits the active field.
		if key == "esc" || key == "enter" {
			f.editing = false
			f.fields[f.active].Blur()
			return f, nil
		}
		f.clearTransient()
		var cmd tea.Cmd
		f.fields[f.active], cmd = f.fields[f.active].Update(msg)
		return f, cmd
	}

	// Normal mode: navigate the field cursor or enter insert mode.
	switch key {
	case "j", "down":
		f.moveActive(1)
	case "k", "up":
		f.moveActive(-1)
	case "tab":
		f.moveActive(1)
	case "shift+tab":
		f.moveActive(-1)
	case "g":
		f.active = fieldName
		f.ensureFieldVisible()
	case "G":
		f.active = fieldCount - 1
		f.ensureFieldVisible()
	case "e", "i", "a":
		f.editing = true
		f.clearTransient()
		cmd := f.fields[f.active].Focus()
		return f, cmd
	}
	return f, nil
}

// moveActive advances the field cursor by delta (wrapping) and keeps it visible.
func (f *ConnectionForm) moveActive(delta int) {
	f.active += delta
	if f.active >= fieldCount {
		f.active = fieldName
	}
	if f.active < 0 {
		f.active = fieldCount - 1
	}
	f.ensureFieldVisible()
}

// clearTransient wipes stale validation/test-connection messages so the user is
// not misled by an outdated result once they start editing again.
func (f *ConnectionForm) clearTransient() {
	f.errMsg = ""
	f.testMsg = ""
	f.testOK = false
}

// IsEditing reports whether the form is in insert mode (typing into a field).
func (f ConnectionForm) IsEditing() bool { return f.editing }

// View renders the form as a scrollable list of bordered fields, matching
// the inspector's field-box look (shared via renderFieldBox). One line is
// reserved at the bottom for the validation/test-connection message.
func (f ConnectionForm) View() string {
	contentW := f.width
	valueWidth := contentW - 4
	if valueWidth < 1 {
		valueWidth = 1
	}

	fieldsHeight := f.height - 1 // reserve 1 line for the message
	if fieldsHeight < linesPerField {
		fieldsHeight = linesPerField
	}
	maxFields := fieldsHeight / linesPerField

	start := f.scrollRow
	if start > fieldCount-maxFields && fieldCount > maxFields {
		start = fieldCount - maxFields
	}
	if start < 0 {
		start = 0
	}
	end := start + maxFields
	if end > fieldCount {
		end = fieldCount
	}

	labelSty := lipgloss.NewStyle().Foreground(colorLabel)
	markerSty := lipgloss.NewStyle().Foreground(colorMuted)

	var rendered strings.Builder
	for fi := start; fi < end; fi++ {
		labelStr := labelSty.Render(formLabels[fi])
		markerStr := ""
		if formOptional[fi] {
			markerStr = markerSty.Render("(optional)")
		}
		rendered.WriteString(renderFieldBox(labelStr, markerStr, f.fieldValueContent(fi, valueWidth), contentW, fi == f.active))
		rendered.WriteString("\n")
	}

	fieldsBlock := lipgloss.NewStyle().
		Height(fieldsHeight).
		Render(strings.TrimRight(rendered.String(), "\n"))

	return fieldsBlock + "\n" + f.messageLine()
}

// fieldValueContent returns the already-styled, valueWidth-wide interior for
// field fi's value box. The active field renders a bar cursor; inactive fields
// show their value, a masked value for password fields, or the muted
// placeholder when empty.
func (f ConnectionForm) fieldValueContent(fi, valueWidth int) string {
	if fi == f.active && f.editing {
		return renderEditInput(f.fields[fi], valueWidth, colorFg)
	}
	ti := f.fields[fi]
	val := ti.Value()
	var displayVal string
	sty := lipgloss.NewStyle().Foreground(colorFg)
	switch {
	case ti.EchoMode == textinput.EchoPassword && val != "":
		displayVal = strings.Repeat("*", runeLen(val))
	case val == "":
		displayVal = ti.Placeholder
		sty = lipgloss.NewStyle().Foreground(colorMuted)
	default:
		displayVal = val
	}
	return sty.Render(truncateCell(displayVal, valueWidth))
}

// messageLine returns the validation/test-connection status line (blank when
// there is nothing to report).
func (f ConnectionForm) messageLine() string {
	switch {
	case f.errMsg != "":
		return errorStyle.Render(f.errMsg)
	case f.testing:
		return mutedStyle.Render("Testing connection…")
	case f.testMsg != "":
		if f.testOK {
			return successStyle.Render(f.testMsg)
		}
		return errorStyle.Render(f.testMsg)
	}
	return ""
}

// visibleFieldCount returns how many complete fields fit in the fields area.
func (f ConnectionForm) visibleFieldCount() int {
	fieldsHeight := f.height - 1
	if fieldsHeight < linesPerField {
		fieldsHeight = linesPerField
	}
	return fieldsHeight / linesPerField
}

// ensureFieldVisible adjusts scrollRow so the active field stays in view.
func (f *ConnectionForm) ensureFieldVisible() {
	max := f.visibleFieldCount()
	if f.active < f.scrollRow {
		f.scrollRow = f.active
	}
	if f.active >= f.scrollRow+max {
		f.scrollRow = f.active - max + 1
	}
	if f.scrollRow < 0 {
		f.scrollRow = 0
	}
}

// contentHeight returns the total content height needed to show every field
// plus the message line, so the popup can size itself (before capping).
func (f ConnectionForm) contentHeight() int {
	return fieldCount*linesPerField + 1
}

// SetSize sets the content dimensions of the form: width is the usable content
// width (inside border and padding), height is the available content height
// (the form scrolls if it cannot fit all fields).
func (f *ConnectionForm) SetSize(width, height int) {
	f.width = width
	f.height = height
	valueWidth := width - 4
	if valueWidth < 1 {
		valueWidth = 1
	}
	for i := range f.fields {
		f.fields[i].Width = valueWidth - 1
	}
}

// SetMaxWidth adjusts the form's content width (height unchanged). Kept for
// callers that only resize horizontally.
func (f *ConnectionForm) SetMaxWidth(width int) {
	f.width = width
	valueWidth := width - 4
	if valueWidth < 1 {
		valueWidth = 1
	}
	for i := range f.fields {
		f.fields[i].Width = valueWidth - 1
	}
}

// Focus first field.
// Focus is a no-op: the form opens in normal mode (field cursor on Name,
// not editing). Insert mode is entered explicitly with e/i/a, which focuses
// the underlying textinput. The form renders its own bar cursor, so the
// textinput's blink command is not needed.
func (f *ConnectionForm) Focus() tea.Cmd {
	return nil
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

// SetTesting marks the form as running a background connection test.
func (f *ConnectionForm) SetTesting(b bool) {
	f.testing = b
}

// SetTestResult records the outcome of a connection test, clearing any stale
// validation error. A non-empty message with ok=true is shown as success;
// otherwise it is shown as an error.
func (f *ConnectionForm) SetTestResult(msg string, ok bool) {
	f.testing = false
	f.testMsg = msg
	f.testOK = ok
	f.errMsg = ""
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
