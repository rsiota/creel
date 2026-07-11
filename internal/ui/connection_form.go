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

// FormField indices. The order is fixed; which of these are actually shown is
// decided dynamically by visibleFields() based on the selected driver and
// whether the SSH tunnel toggle is on.
const (
	fieldName = iota
	fieldDriver
	fieldDatabase
	fieldHost
	fieldPort
	fieldUser
	fieldPass
	fieldSSHTunnel // no/yes toggle: reveals the SSH fields below when "yes"
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
	"SSH Tunnel", "SSH Host", "SSH Port", "SSH User", "SSH Key", "SSH Pass",
	"Secrets", "Read-only",
}

// formChoices lists the selectable values for "choice" fields, rendered as
// cycling selectors (e.g. < sqlite >) and changed with h/l in normal mode.
// Fields absent from the map are free-text and edited with e/i/a.
var formChoices = map[int][]string{
	fieldDriver:   {"sqlite", "mysql", "postgres"},
	fieldSSHTunnel: {"no", "yes"},
	fieldSecrets:  {"keychain", "plain"},
	fieldReadOnly: {"no", "yes"},
}

// isChoiceField reports whether fi is a cycling-selector field.
func isChoiceField(fi int) bool {
	_, ok := formChoices[fi]
	return ok
}

// formMode determines whether we are adding or editing.
type formMode int

const (
	formModeAdd formMode = iota
	formModeEdit
)

// ConnectionForm is the add/edit connection form. It shows only the fields
// relevant to the selected driver (and SSH toggle), so a sqlite connection
// presents a short list while a tunneled mysql connection shows everything.
type ConnectionForm struct {
	fields    []textinput.Model
	active    int    // cursor position within the visible field list
	mode      formMode
	errMsg    string
	width     int
	height    int    // available content height (the cap); actual = effectiveHeight()
	scrollRow int    // first visible position in the field list when scrolling
	editing   bool   // insert mode: typing into the active (free-text) field
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

	// Reveal the SSH group if the saved connection used a tunnel.
	f.fields[fieldSSHTunnel].SetValue(boolField(cfg.SSHHost != ""))

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
	fields[fieldDriver] = newTextInput("sqlite / mysql / postgres", "sqlite", false)
	fields[fieldDatabase] = newTextInput("Database (sqlite path; optional for mysql/pg)", "/path/to/db.sqlite", false)
	fields[fieldHost] = newTextInput("Host", "localhost", false)
	fields[fieldPort] = newTextInput("Port (mysql default 3306, postgres default 5432)", "3306", false)
	fields[fieldUser] = newTextInput("Username", "root", false)
	fields[fieldPass] = newTextInput("Password", "", true)
	fields[fieldSSHTunnel] = newTextInput("no / yes", "", false)
	fields[fieldSSHHost] = newTextInput("SSH host", "", false)
	fields[fieldSSHPort] = newTextInput("SSH port (default 22)", "22", false)
	fields[fieldSSHUser] = newTextInput("SSH user", "", false)
	fields[fieldSSHKeyPath] = newTextInput("SSH key path (~/.ssh/id_rsa)", "", false)
	fields[fieldSSHPassword] = newTextInput("SSH password", "", true)
	fields[fieldSecrets] = newTextInput("keychain / plain", "", false)
	fields[fieldReadOnly] = newTextInput("no / yes", "", false)

	fields[fieldName].SetValue(name)
	fields[fieldDriver].SetValue("sqlite")
	fields[fieldSSHTunnel].SetValue("no")
	fields[fieldSecrets].SetValue("keychain")
	fields[fieldReadOnly].SetValue("no")

	return ConnectionForm{
		fields: fields,
		active: 0,
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

// --- dynamic field list -----------------------------------------------------

// driver returns the currently selected driver.
func (f ConnectionForm) driver() string {
	return strings.TrimSpace(f.fields[fieldDriver].Value())
}

// isNetworkDriver reports whether d uses Host/Port/User/Password (mysql/pg).
func isNetworkDriver(d string) bool {
	return d == "mysql" || d == "postgres"
}

// sshEnabled reports whether the SSH tunnel toggle is on.
func (f ConnectionForm) sshEnabled() bool {
	return parseBoolField(f.fields[fieldSSHTunnel].Value())
}

// visibleFields returns the field indices relevant to the current driver and
// SSH-toggle state, in display order. This is the dynamic field list the form
// renders and navigates, mirroring the inspector's fieldList(). SQLite shows a
// short list (no host/port/user/pass, no SSH); network drivers add those, and
// enabling the SSH toggle reveals the SSH sub-fields.
func (f ConnectionForm) visibleFields() []int {
	out := []int{fieldName, fieldDriver, fieldDatabase}
	if isNetworkDriver(f.driver()) {
		out = append(out, fieldHost, fieldPort, fieldUser, fieldPass, fieldSSHTunnel)
		if f.sshEnabled() {
			out = append(out, fieldSSHHost, fieldSSHPort, fieldSSHUser, fieldSSHKeyPath, fieldSSHPassword)
		}
	}
	out = append(out, fieldSecrets, fieldReadOnly)
	return out
}

// activeField returns the field index at the current cursor position.
func (f ConnectionForm) activeField() int {
	v := f.visibleFields()
	if len(v) == 0 {
		return fieldName
	}
	if f.active < 0 || f.active >= len(v) {
		return v[0]
	}
	return v[f.active]
}

// --- update -----------------------------------------------------------------

// Update handles messages for the form using a vim-like modal model that
// mirrors the inspector. In normal mode j/k (and Tab/arrows) move the field
// cursor without editing; h/l cycle the value of a selector field (Driver,
// SSH Tunnel, Secrets, Read-only); e/i/a enter insert mode on free-text fields.
// In insert mode, keys edit the field; Esc/Enter return to normal mode.
func (f ConnectionForm) Update(msg tea.Msg) (ConnectionForm, tea.Cmd) {
	kmsg, isKey := msg.(tea.KeyMsg)
	if !isKey {
		// Non-key messages (e.g. WindowSizeMsg) only matter in insert mode,
		// where they are forwarded to the active textinput.
		if f.editing {
			var cmd tea.Cmd
			fi := f.activeField()
			f.fields[fi], cmd = f.fields[fi].Update(msg)
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
			f.fields[f.activeField()].Blur()
			return f, nil
		}
		f.clearTransient()
		var cmd tea.Cmd
		fi := f.activeField()
		f.fields[fi], cmd = f.fields[fi].Update(msg)
		return f, cmd
	}

	// Normal mode: navigate the field cursor, cycle selectors, or enter insert.
	fi := f.activeField()
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
		f.active = 0
		f.ensureFieldVisible()
	case "G":
		f.active = len(f.visibleFields()) - 1
		f.ensureFieldVisible()
	case "l", "right":
		if isChoiceField(fi) {
			f.cycleChoice(fi, 1)
		}
	case "h", "left":
		if isChoiceField(fi) {
			f.cycleChoice(fi, -1)
		}
	case "e", "i", "a":
		// Free-text fields enter insert mode; choice fields are changed with h/l.
		if !isChoiceField(fi) {
			f.editing = true
			f.clearTransient()
			cmd := f.fields[fi].Focus()
			return f, cmd
		}
	}
	return f, nil
}

// moveActive advances the cursor by delta within the visible field list (wrapping).
func (f *ConnectionForm) moveActive(delta int) {
	n := len(f.visibleFields())
	if n == 0 {
		return
	}
	f.active += delta
	if f.active >= n {
		f.active = 0
	}
	if f.active < 0 {
		f.active = n - 1
	}
	f.ensureFieldVisible()
}

// cycleChoice advances a selector field by dir (wrapping) and re-clamps the
// cursor, since changing the driver or SSH toggle grows or shrinks the visible
// field list.
func (f *ConnectionForm) cycleChoice(fi, dir int) {
	choices := formChoices[fi]
	cur := f.fields[fi].Value()
	idx := 0
	for i, c := range choices {
		if c == cur {
			idx = i
			break
		}
	}
	idx = (idx + dir + len(choices)) % len(choices)
	f.fields[fi].SetValue(choices[idx])
	f.clearTransient()
	n := len(f.visibleFields())
	if f.active >= n {
		f.active = n - 1
	}
	if f.active < 0 {
		f.active = 0
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

// ActiveIsChoice reports whether the active field is a cycling selector, used
// for context-sensitive keybinding hints (h/l vs e).
func (f ConnectionForm) ActiveIsChoice() bool {
	return isChoiceField(f.activeField())
}

// --- view -------------------------------------------------------------------

// View renders the form as a scrollable list of bordered fields, matching
// the inspector's field-box look (shared via renderFieldBox). One line is
// reserved at the bottom for the validation/test-connection message.
func (f ConnectionForm) View() string {
	contentW := f.width
	valueWidth := contentW - 4
	if valueWidth < 1 {
		valueWidth = 1
	}

	vis := f.visibleFields()
	n := len(vis)

	fieldsHeight := f.effectiveHeight() - 1 // reserve 1 line for the message
	if fieldsHeight < linesPerField {
		fieldsHeight = linesPerField
	}
	maxFields := fieldsHeight / linesPerField

	start := f.scrollRow
	if start > n-maxFields && n > maxFields {
		start = n - maxFields
	}
	if start < 0 {
		start = 0
	}
	end := start + maxFields
	if end > n {
		end = n
	}

	labelSty := lipgloss.NewStyle().Foreground(colorLabel)

	var rendered strings.Builder
	for p := start; p < end; p++ {
		fi := vis[p]
		labelStr := labelSty.Render(formLabels[fi])
		rendered.WriteString(renderFieldBox(labelStr, "", f.fieldValueContent(fi, valueWidth), contentW, p == f.active))
		rendered.WriteString("\n")
	}

	fieldsBlock := lipgloss.NewStyle().
		Height(fieldsHeight).
		Render(strings.TrimRight(rendered.String(), "\n"))

	return fieldsBlock + "\n" + f.messageLine()
}

// fieldValueContent returns the already-styled, valueWidth-wide interior for
// field fi's value box. Choice fields render as a selector (< value >); the
// active free-text field in insert mode renders a bar cursor; other free-text
// fields show their value, a masked value for password fields, or the muted
// placeholder when empty.
func (f ConnectionForm) fieldValueContent(fi, valueWidth int) string {
	if isChoiceField(fi) {
		return renderSelector(f.fields[fi].Value(), valueWidth)
	}
	if fi == f.activeField() && f.editing {
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

// renderSelector renders a cycling-selector value as "< value >", padded to
// valueWidth so the value box stays aligned. Chevrons are muted, the value is
// foreground-coloured.
func renderSelector(value string, width int) string {
	chevSty := lipgloss.NewStyle().Foreground(colorMuted)
	valSty := lipgloss.NewStyle().Foreground(colorFg)
	pad := width - (runeLen(value) + 4) // "<" sp value sp ">"
	if pad < 0 {
		pad = 0
	}
	return chevSty.Render("<") + " " + valSty.Render(value) + " " + chevSty.Render(">") +
		strings.Repeat(" ", pad)
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

// --- sizing -----------------------------------------------------------------

// contentHeight returns the total content height needed to show every currently
// visible field plus the message line (before capping to the viewport).
func (f ConnectionForm) contentHeight() int {
	return len(f.visibleFields())*linesPerField + 1
}

// effectiveHeight is the actual content height used for rendering: the needed
// height, capped to the available height (set via SetSize). Because this
// derives from the current visible field list, the popup grows and shrinks as
// the driver or SSH toggle changes without needing another SetSize call.
func (f ConnectionForm) effectiveHeight() int {
	h := f.contentHeight()
	if h > f.height {
		h = f.height
	}
	if h < linesPerField+1 {
		h = linesPerField + 1
	}
	return h
}

// visibleFieldCount returns how many complete fields fit in the fields area.
func (f ConnectionForm) visibleFieldCount() int {
	fieldsHeight := f.effectiveHeight() - 1
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

// popupContentSize computes the inner content dimensions (width, height) the
// add-connection form popup should be sized to, given the terminal height. The
// popup has a fixed width (from popupDim); the returned height is the maximum
// available content height (the cap) — the form itself shrinks below it to fit
// the current field count (see effectiveHeight). This is the single source of
// truth used by layout (which sizes the real model) and rendering, so the scroll
// math and what is drawn always agree.
func popupContentSize(termHeight int) (width, height int) {
	popupW, _ := popupDim()
	const (
		borderOverhead = 2 // rounded border on each side
		padding        = 2 // padding(0,1) = 1 left + 1 right
	)
	innerW := popupW - borderOverhead - padding
	maxContentH := termHeight - 1 - 4 // status bar + top/bottom margin
	if maxContentH < 12 {
		maxContentH = 12
	}
	return innerW, maxContentH
}

// SetSize sets the content dimensions of the form: width is the usable content
// width (inside border and padding), height is the maximum available content
// height (the form shrinks below it to fit the visible fields, and scrolls if
// even that is not enough).
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

// Focus is a no-op: the form opens in normal mode (field cursor on Name, not
// editing). Insert mode is entered explicitly with e/i/a, which focuses the
// underlying textinput. The form renders its own bar cursor, so the textinput's
// blink command is not needed.
func (f *ConnectionForm) Focus() tea.Cmd {
	return nil
}

// --- submit -----------------------------------------------------------------

// EnterPressed validates the visible, relevant fields and returns the resulting
// connection config (or an error message). Fields that are not shown for the
// current driver/SSH state are left empty, so a sqlite connection never carries
// stale host or SSH values.
func (f *ConnectionForm) EnterPressed() (config.ConnectionConfig, string) {
	name := strings.TrimSpace(f.fields[fieldName].Value())
	if name == "" {
		return config.ConnectionConfig{}, "name is required"
	}

	driver := f.driver()
	if driver != "sqlite" && driver != "mysql" && driver != "postgres" {
		return config.ConnectionConfig{}, "driver must be 'sqlite', 'mysql', or 'postgres'"
	}

	database := f.fields[fieldDatabase].Value()
	if database == "" && driver == "sqlite" {
		return config.ConnectionConfig{}, "database path is required"
	}

	cfg := config.ConnectionConfig{
		Name:     name,
		Driver:   driver,
		Database: database,
		ReadOnly: parseBoolField(f.fields[fieldReadOnly].Value()),
	}

	if isNetworkDriver(driver) {
		cfg.Host = f.fields[fieldHost].Value()

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
		cfg.Port = port

		cfg.Username = f.fields[fieldUser].Value()
		cfg.Password = f.fields[fieldPass].Value()

		if f.sshEnabled() {
			cfg.SSHHost = f.fields[fieldSSHHost].Value()
			sshPort := 22
			if sshPortStr := f.fields[fieldSSHPort].Value(); sshPortStr != "" {
				var err error
				sshPort, err = strconv.Atoi(sshPortStr)
				if err != nil {
					return config.ConnectionConfig{}, "SSH port must be a number"
				}
			}
			cfg.SSHPort = sshPort
			cfg.SSHUser = f.fields[fieldSSHUser].Value()
			cfg.SSHKeyPath = f.fields[fieldSSHKeyPath].Value()
			cfg.SSHPassword = f.fields[fieldSSHPassword].Value()
		}
	}

	return cfg, ""
}

// --- accessors used by the app ---------------------------------------------

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

// parseBoolField interprets the read-only/ssh toggle. "yes", "true", "y", "1",
// and "ro" enable it; anything else is off.
func parseBoolField(v string) bool {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "yes", "true", "y", "1", "ro", "readonly", "read-only":
		return true
	}
	return false
}
