package ui

import (
	"strconv"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/rsiota/creel/internal/config"
	"github.com/rsiota/creel/internal/secrets"
)

// FormField indices. The order is fixed; which of these are actually shown is
// decided dynamically by visibleFields() based on the selected driver and
// page (Connection / SSH / Options).
const (
	fieldName = iota
	fieldDriver
	fieldDatabase
	fieldHost
	fieldPort
	fieldUser
	fieldPass
	fieldSSLMode // disable / prefer / require / verify-full
	fieldSocket  // unix socket path; TCP host is ignored at connect if set
	fieldSSHHost
	fieldSSHPort
	fieldSSHUser
	fieldSSHKeyPath
	fieldSSHPassword
	fieldSSHPassphrase // passphrase for the SSH private key (resolved at connect)
	fieldSecrets       // keychain vs plaintext storage for secret fields
	fieldReadOnly      // yes/no toggle
	fieldGroup         // optional folder label for grouping in the connection list
	fieldCount
)

// formLabels is the display label for each field, parallel to the field
// indices above. It is shared by rendering and any label lookups.
var formLabels = [...]string{
	"Name", "Driver", "Database", "Host", "Port", "Username", "Password",
	"SSL", "Socket",
	"SSH Host", "SSH Port", "SSH User", "SSH Key", "SSH Pass", "SSH Passphrase",
	"Secrets", "Read-only", "Group",
}

// formChoices lists the selectable values for "choice" fields, rendered as
// cycling selectors (e.g. < sqlite >) and changed with h/l in normal mode.
// Fields absent from the map are free-text and edited with e/i/a.
var formChoices = map[int][]string{
	fieldDriver:   {"sqlite", "mysql", "postgres"},
	fieldSSLMode:  {"disable", "prefer", "require", "verify-full"},
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

// formPage is which short page of the connection form is shown. Pages swap
// (they never stack): Connection for everyday fields, SSH for tunneling, and
// Options for rarely touched settings.
type formPage int

const (
	formPageConnection formPage = iota
	formPageSSH
	formPageOptions
)

// formTabBarLines is the height reserved at the top of the form for the
// Connection / SSH / Options tab strip (one content row).
const formTabBarLines = 1

// ConnectionForm is the add/edit connection form. Fields are split across
// swapped pages so the everyday path stays short; SSH is a first-class page
// for network drivers, and Options holds the rest.
type ConnectionForm struct {
	fields    []textinput.Model
	active    int // cursor position within the visible field list
	page      formPage
	mode      formMode
	errMsg    string
	width     int
	height    int    // available content height (the cap); actual = effectiveHeight()
	scrollRow int    // first visible position in the field list when scrolling
	editing   bool   // insert mode: typing into the active (free-text) field
	editName  string // name of connection being edited (for edit mode)

	// Test-connection feedback. testing is true while a background test is in
	// flight; testMsg/testOK hold the result once it completes. testStates maps
	// each visible field to its outcome (ok/fail) so the field box can be
	// tinted green/red like TablePlus; nil/untested entries stay neutral.
	testing    bool
	testMsg    string
	testOK     bool
	testStates map[int]testState

	// Live filesystem completion for sqlite database and SSH key path fields.
	pathComp pathCompletion
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
	ssl := cfg.SSLMode
	if ssl == "" {
		ssl = "prefer"
	}
	f.fields[fieldSSLMode].SetValue(ssl)
	f.fields[fieldSocket].SetValue(cfg.Socket)
	f.fields[fieldSSHHost].SetValue(cfg.SSHHost)
	if cfg.SSHPort > 0 {
		f.fields[fieldSSHPort].SetValue(strconv.Itoa(cfg.SSHPort))
	}
	f.fields[fieldSSHUser].SetValue(cfg.SSHUser)
	f.fields[fieldSSHKeyPath].SetValue(cfg.SSHKeyPath)
	f.fields[fieldSSHPassword].SetValue(resolveSecretOrKeep(cfg.SSHPassword))
	f.fields[fieldSSHPassphrase].SetValue(resolveSecretOrKeep(cfg.SSHPassphrase))

	// Resolve any keychain references into plaintext so the masked fields show
	// the real value (same UX as a plaintext config). If a reference cannot be
	// resolved (e.g. keychain locked), leave the reference in place so a save
	// without changes preserves it as-is.
	f.fields[fieldSecrets].SetValue(secretsModeFromConfig(cfg))
	f.fields[fieldReadOnly].SetValue(boolField(cfg.ReadOnly))
	f.fields[fieldGroup].SetValue(cfg.Group)
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
	fields[fieldSSLMode] = newTextInput("disable / prefer / require / verify-full", "prefer", false)
	fields[fieldSocket] = newTextInput("Unix socket path", "/tmp/mysql.sock", false)
	fields[fieldSSHHost] = newTextInput("SSH host", "", false)
	fields[fieldSSHPort] = newTextInput("SSH port (default 22)", "22", false)
	fields[fieldSSHUser] = newTextInput("SSH user", "", false)
	fields[fieldSSHKeyPath] = newTextInput("SSH key path (~/.ssh/id_rsa)", "", false)
	fields[fieldSSHPassword] = newTextInput("SSH password", "", true)
	fields[fieldSSHPassphrase] = newTextInput("SSH key passphrase", "", true)
	fields[fieldSecrets] = newTextInput("keychain / plain", "", false)
	fields[fieldReadOnly] = newTextInput("no / yes", "", false)
	fields[fieldGroup] = newTextInput("Folder name (optional, e.g. Work)", "", false)

	fields[fieldName].SetValue(name)
	fields[fieldDriver].SetValue("sqlite")
	fields[fieldSSLMode].SetValue("prefer")
	fields[fieldSecrets].SetValue("keychain")
	fields[fieldReadOnly].SetValue("no")
	fields[fieldGroup].SetValue("")

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
	f.fields[fieldDriver].SetValue(sanitizeDriver(driver))
	f.clampPage()
}

// sanitizeDriver normalizes a default-driver value to one of the supported
// drivers, falling back to sqlite for anything unrecognized (including the
// empty string) so a bad setting never leaves the form in an invalid state.
func sanitizeDriver(driver string) string {
	switch strings.ToLower(strings.TrimSpace(driver)) {
	case "mysql", "postgres", "sqlite":
		return strings.ToLower(strings.TrimSpace(driver))
	}
	return "sqlite"
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

// sshEnabled reports whether an SSH tunnel is configured. Filling SSH Host on
// the SSH tab is the signal — there is no separate on/off toggle.
func (f ConnectionForm) sshEnabled() bool {
	return strings.TrimSpace(f.fields[fieldSSHHost].Value()) != ""
}

// connectionFields returns the everyday fields shown on the Connection page.
// Network drivers omit Port (it lives on Options with Socket/SSL); defaults
// still apply on save when Port is blank.
func (f ConnectionForm) connectionFields() []int {
	out := []int{fieldName, fieldDriver}
	if isNetworkDriver(f.driver()) {
		out = append(out, fieldHost, fieldUser, fieldPass, fieldDatabase)
	} else {
		out = append(out, fieldDatabase)
	}
	return out
}

// sshFields returns the SSH page fields (network drivers only), in daily-use
// order. Leave SSH Host blank to connect without a tunnel.
func (f ConnectionForm) sshFields() []int {
	if !isNetworkDriver(f.driver()) {
		return nil
	}
	return []int{fieldSSHHost, fieldSSHUser, fieldSSHPassword, fieldSSHKeyPath, fieldSSHPort, fieldSSHPassphrase}
}

// optionsFields returns less-common settings on the Options page. For network
// drivers this is Port/Socket/SSL plus Secrets/Read-only/Group (six fields,
// matching Connection and SSH).
func (f ConnectionForm) optionsFields() []int {
	var out []int
	if isNetworkDriver(f.driver()) {
		out = append(out, fieldPort, fieldSocket, fieldSSLMode)
	}
	out = append(out, fieldSecrets, fieldReadOnly, fieldGroup)
	return out
}

// relevantFields returns every field that participates in the current
// driver/SSH config across all pages. Used for save/test attribution so a
// ctrl+t result is not limited to whichever page happens to be open.
func (f ConnectionForm) relevantFields() []int {
	out := f.connectionFields()
	out = append(out, f.sshFields()...)
	return append(out, f.optionsFields()...)
}

// availablePages returns the tab strip for the current driver. SQLite hides
// SSH (no tunnel), so the strip is Connection · Options.
func (f ConnectionForm) availablePages() []formPage {
	if isNetworkDriver(f.driver()) {
		return []formPage{formPageConnection, formPageSSH, formPageOptions}
	}
	return []formPage{formPageConnection, formPageOptions}
}

// visibleFields returns the field indices on the current page, in display
// order. This is what the form renders and navigates.
func (f ConnectionForm) visibleFields() []int {
	switch f.page {
	case formPageSSH:
		return f.sshFields()
	case formPageOptions:
		return f.optionsFields()
	default:
		return f.connectionFields()
	}
}

// pageForField returns which form page hosts fi for the current driver.
// Unknown fields fall back to Connection.
func (f ConnectionForm) pageForField(fi int) formPage {
	for _, id := range f.sshFields() {
		if id == fi {
			return formPageSSH
		}
	}
	for _, id := range f.optionsFields() {
		if id == fi {
			return formPageOptions
		}
	}
	return formPageConnection
}

// pageAvailable reports whether p is in the current tab strip.
func (f ConnectionForm) pageAvailable(p formPage) bool {
	for _, id := range f.availablePages() {
		if id == p {
			return true
		}
	}
	return false
}

// clampPage moves off a page that is no longer available (e.g. SSH after
// switching the driver to sqlite).
func (f *ConnectionForm) clampPage() {
	if f.pageAvailable(f.page) {
		return
	}
	f.page = formPageConnection
	f.active = 0
	f.scrollRow = 0
}

// setPage switches to page p, leaving insert mode and resetting the field
// cursor to the top of that page. No-op when p is unavailable for the driver.
func (f *ConnectionForm) setPage(p formPage) {
	if !f.pageAvailable(p) || p == f.page {
		return
	}
	if f.editing {
		f.fields[f.activeField()].Blur()
		f.editing = false
		f.pathComp.clear()
	}
	f.page = p
	f.active = 0
	f.scrollRow = 0
	f.ensureFieldVisible()
}

// movePage steps left/right through availablePages (wrapping).
func (f *ConnectionForm) movePage(delta int) {
	pages := f.availablePages()
	if len(pages) == 0 {
		return
	}
	idx := 0
	for i, p := range pages {
		if p == f.page {
			idx = i
			break
		}
	}
	idx = (idx + delta + len(pages)) % len(pages)
	f.setPage(pages[idx])
}

// focusField switches to the page that hosts fi (if needed) and moves the
// cursor onto that field. Used when validation fails so the error is on-screen.
func (f *ConnectionForm) focusField(fi int) {
	f.setPage(f.pageForField(fi))
	vis := f.visibleFields()
	for i, id := range vis {
		if id == fi {
			f.active = i
			f.ensureFieldVisible()
			return
		}
	}
	f.active = 0
	f.ensureFieldVisible()
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

// isPathField reports whether fi accepts filesystem path autocompletion.
func (f ConnectionForm) isPathField(fi int) bool {
	switch fi {
	case fieldSSHKeyPath:
		return true
	case fieldDatabase:
		return f.driver() == "sqlite"
	}
	return false
}

// ActiveIsPathField reports whether the focused field is a path field with
// live filesystem completion (sqlite database file or SSH private key).
func (f ConnectionForm) ActiveIsPathField() bool {
	return f.isPathField(f.activeField())
}

// CompletionView returns the path-completion dropdown while editing a path
// field, or "" otherwise.
func (f ConnectionForm) CompletionView() string {
	if !f.editing || !f.isPathField(f.activeField()) {
		return ""
	}
	return f.pathComp.View()
}

// ClickField moves the field cursor to the field at the given content-relative
// Y coordinate (0 = first line below the form panel's top border) and returns
// the field index constant for that field. Returns -1 when the click does not
// land on a field (e.g. on the tab bar, message line, or empty padding).
func (f *ConnectionForm) ClickField(contentY int) int {
	vis := f.visibleFields()
	n := len(vis)
	if n == 0 || contentY < formTabBarLines {
		return -1
	}
	contentY -= formTabBarLines

	fieldsHeight := f.effectiveHeight() - formTabBarLines - 1
	if fieldsHeight < linesPerField {
		fieldsHeight = linesPerField
	}
	if contentY >= fieldsHeight {
		return -1
	}

	maxFields := fieldsHeight / linesPerField
	start := f.scrollRow
	if start > n-maxFields && n > maxFields {
		start = n - maxFields
	}
	if start < 0 {
		start = 0
	}

	relField := contentY / linesPerField
	fieldIdx := start + relField
	if fieldIdx < 0 || fieldIdx >= n {
		return -1
	}

	if f.editing && fieldIdx != f.active {
		f.fields[f.activeField()].Blur()
		f.editing = false
		f.pathComp.clear()
	}
	f.active = fieldIdx
	f.ensureFieldVisible()
	return vis[fieldIdx]
}

// ClickPageTab switches to the Connection/SSH/Options tab under content-
// relative coordinates (contentX, contentY). Returns true when a tab was hit.
func (f *ConnectionForm) ClickPageTab(contentX, contentY int) bool {
	if contentY < 0 || contentY >= formTabBarLines {
		return false
	}
	p := f.pageTabAt(contentX)
	if p < 0 {
		return false
	}
	f.setPage(formPage(p))
	return true
}

// StartFieldEdit enters insert mode on the currently focused free-text field,
// mirroring the e/i/a key binding. Choice fields are changed with h/l instead.
func (f *ConnectionForm) StartFieldEdit() tea.Cmd {
	fi := f.activeField()
	if isChoiceField(fi) {
		return nil
	}
	f.editing = true
	f.clearTransient()
	if f.isPathField(fi) {
		f.pathComp.refresh(f.fields[fi].Value())
	} else {
		f.pathComp.clear()
	}
	return f.fields[fi].Focus()
}

// completionLineOffset returns the row (within the form body, below the top
// border) where the completion popup should anchor, or -1 when hidden.
func (f ConnectionForm) completionLineOffset() int {
	if f.CompletionView() == "" {
		return -1
	}
	vis := f.visibleFields()
	n := len(vis)
	fieldsHeight := f.effectiveHeight() - formTabBarLines - 1
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
	rel := f.active - start
	if rel < 0 || rel >= maxFields {
		return -1
	}
	// Value row is the third line of the field box; popup sits on the next row.
	return formTabBarLines + rel*linesPerField + 3
}

// --- update -----------------------------------------------------------------

// Update handles messages for the form using a vim-like modal model that
// mirrors the inspector. In normal mode j/k (and Tab/arrows) move the field
// cursor without editing; h/l cycle the value of a selector field (Driver,
// Secrets, Read-only); e/i/a enter insert mode on free-text fields.
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
		fi := f.activeField()
		if key == "esc" {
			f.editing = false
			f.pathComp.clear()
			f.fields[fi].Blur()
			return f, nil
		}
		if f.isPathField(fi) {
			switch key {
			case "tab", "enter":
				// Enter mirrors Tab when the dropdown is open (same polish as
				// the ex-line completion). With no choices, Enter still leaves
				// insert mode below.
				if f.pathComp.hasChoices() {
					f.pathComp.accept(&f.fields[fi])
					return f, nil
				}
			case "down":
				if f.pathComp.compVisible {
					f.pathComp.move(1)
					return f, nil
				}
			case "up":
				if f.pathComp.compVisible {
					f.pathComp.move(-1)
					return f, nil
				}
			}
		}
		if key == "enter" {
			f.editing = false
			f.pathComp.clear()
			f.fields[fi].Blur()
			return f, nil
		}
		f.clearTransient()
		var cmd tea.Cmd
		f.fields[fi], cmd = f.fields[fi].Update(msg)
		if f.isPathField(fi) {
			f.pathComp.refresh(f.fields[fi].Value())
		}
		return f, cmd
	}

	// Normal mode: navigate the field cursor, cycle selectors, or enter insert.
	fi := f.activeField()
	switch key {
	case "[":
		f.movePage(-1)
	case "]":
		f.movePage(1)
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
			if f.isPathField(fi) {
				f.pathComp.refresh(f.fields[fi].Value())
			} else {
				f.pathComp.clear()
			}
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
// cursor, since changing the driver grows or shrinks the available pages and
// visible field list.
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
	if fi == fieldDriver {
		f.clampPage()
	}
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
// not misled by an outdated result once they start editing again. Field test
// colours are cleared too — once a value changes, the old verdict is stale.
func (f *ConnectionForm) clearTransient() {
	f.errMsg = ""
	f.testMsg = ""
	f.testOK = false
	f.testStates = nil
}

// IsEditing reports whether the form is in insert mode (typing into a field).
func (f ConnectionForm) IsEditing() bool { return f.editing }

// ActiveIsChoice reports whether the active field is a cycling selector, used
// for context-sensitive keybinding hints (h/l vs e).
func (f ConnectionForm) ActiveIsChoice() bool {
	return isChoiceField(f.activeField())
}

// --- view -------------------------------------------------------------------

func formPageLabel(p formPage) string {
	switch p {
	case formPageSSH:
		return "SSH"
	case formPageOptions:
		return "Options"
	default:
		return "Connection"
	}
}

// renderPageTabBar renders the available page tabs for the current driver,
// right-aligned in the form content width.
func (f ConnectionForm) renderPageTabBar() string {
	var parts []string
	for _, p := range f.availablePages() {
		l := formPageLabel(p)
		var s string
		if p == f.page {
			s = lipgloss.NewStyle().
				Bold(true).
				Background(colorPrimary).
				Foreground(colorBg).
				Render(" " + l + " ")
		} else {
			s = lipgloss.NewStyle().Foreground(colorMuted).Render(" " + l + " ")
		}
		parts = append(parts, s)
	}
	tabs := strings.Join(parts, " ")
	w := f.width
	if w < 1 {
		return tabs
	}
	return lipgloss.NewStyle().Width(w).Align(lipgloss.Right).Render(tabs)
}

// pageTabWidth is the plain-text width of the tab strip (labels + padding +
// separators), matching renderPageTabBar layout for mouse hit-testing.
func (f ConnectionForm) pageTabWidth() int {
	pages := f.availablePages()
	if len(pages) == 0 {
		return 0
	}
	total := 0
	for i, p := range pages {
		total += 1 + len(formPageLabel(p)) + 1 // " label "
		if i < len(pages)-1 {
			total++ // separator space
		}
	}
	return total
}

// pageTabAt returns the formPage at content-relative column x, or -1.
// Tabs are right-aligned; layout matches renderPageTabBar.
func (f ConnectionForm) pageTabAt(x int) int {
	pages := f.availablePages()
	total := f.pageTabWidth()
	start := f.width - total
	if start < 0 {
		start = 0
	}
	cur := start
	for _, p := range pages {
		w := 1 + len(formPageLabel(p)) + 1
		if x >= cur && x < cur+w {
			return int(p)
		}
		cur += w + 1
	}
	return -1
}

// View renders the form as a scrollable list of bordered fields, matching
// the inspector's field-box look (shared via renderFieldBox). One line is
// reserved at the top for page tabs and one at the bottom for the
// validation/test-connection message.
func (f ConnectionForm) View() string {
	contentW := f.width
	valueWidth := contentW - 4
	if valueWidth < 1 {
		valueWidth = 1
	}

	vis := f.visibleFields()
	n := len(vis)

	fieldsHeight := f.effectiveHeight() - formTabBarLines - 1 // tabs + message
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
		st := f.statusOf(fi)
		labelStr := labelSty.Render(formLabels[fi])
		marker := fieldTestMarker(st)
		rendered.WriteString(renderFieldBox(labelStr, marker, f.fieldValueContent(fi, valueWidth), contentW, formFieldBorder(p == f.active, st)))
		rendered.WriteString("\n")
	}

	fieldsBlock := lipgloss.NewStyle().
		Height(fieldsHeight).
		Render(strings.TrimRight(rendered.String(), "\n"))

	return f.renderPageTabBar() + "\n" + fieldsBlock + "\n" + f.messageLine()
}

// fieldValueContent returns the already-styled, valueWidth-wide interior for
// field fi's value box. Choice fields render as a selector (< value >); the
// active free-text field in insert mode renders a bar cursor; other free-text
// fields show their value, a masked value for password fields, or the muted
// placeholder when empty. Failed ctrl+t fields get a soft error wash.
func (f ConnectionForm) fieldValueContent(fi, valueWidth int) string {
	var content string
	switch {
	case isChoiceField(fi):
		content = renderSelector(f.fields[fi].Value(), valueWidth)
	case fi == f.activeField() && f.editing:
		content = renderEditInput(f.fields[fi], valueWidth, colorFg)
	default:
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
		content = sty.Render(truncateCell(displayVal, valueWidth))
	}
	if f.statusOf(fi) == testFail {
		return applyFieldFailWash(content, valueWidth)
	}
	return content
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
// visible field plus the tab bar and message line (before capping to the viewport).
func (f ConnectionForm) contentHeight() int {
	return formTabBarLines + len(f.visibleFields())*linesPerField + 1
}

// effectiveHeight is the rendered content height. It uses the shell height from
// SetSize (shared with the connection list and database picker) so the popup
// footprint stays stable across drivers and pages; fields scroll inside when
// they do not all fit.
func (f ConnectionForm) effectiveHeight() int {
	h := f.height
	if h <= 0 {
		h = f.contentHeight()
	}
	minH := formTabBarLines + linesPerField + 1
	if h < minH {
		h = minH
	}
	return h
}

// visibleFieldCount returns how many complete fields fit in the fields area.
func (f ConnectionForm) visibleFieldCount() int {
	fieldsHeight := f.effectiveHeight() - formTabBarLines - 1
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

// SetSize sets the content dimensions of the form: width is the usable content
// width (inside border and padding), height is the shared popup shell height
// (see popupContentSize). Fields scroll when they do not all fit.
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
// stale host or SSH values. On validation failure the cursor jumps to the
// offending field (switching page if needed).
func (f *ConnectionForm) EnterPressed() (config.ConnectionConfig, string) {
	name := strings.TrimSpace(f.fields[fieldName].Value())
	if name == "" {
		f.focusField(fieldName)
		return config.ConnectionConfig{}, "name is required"
	}

	driver := f.driver()
	if driver != "sqlite" && driver != "mysql" && driver != "postgres" {
		f.focusField(fieldDriver)
		return config.ConnectionConfig{}, "driver must be 'sqlite', 'mysql', or 'postgres'"
	}

	database := f.fields[fieldDatabase].Value()
	if database == "" && driver == "sqlite" {
		f.focusField(fieldDatabase)
		return config.ConnectionConfig{}, "database path is required"
	}

	cfg := config.ConnectionConfig{
		Name:     name,
		Driver:   driver,
		Database: database,
		ReadOnly: parseBoolField(f.fields[fieldReadOnly].Value()),
		Group:    strings.TrimSpace(f.fields[fieldGroup].Value()),
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
				f.focusField(fieldPort)
				return config.ConnectionConfig{}, "port must be a number"
			}
		}
		cfg.Port = port

		cfg.Username = f.fields[fieldUser].Value()
		cfg.Password = f.fields[fieldPass].Value()
		cfg.SSLMode = strings.TrimSpace(f.fields[fieldSSLMode].Value())
		if cfg.SSLMode == "" {
			cfg.SSLMode = "prefer"
		}
		cfg.Socket = strings.TrimSpace(f.fields[fieldSocket].Value())

		if f.sshEnabled() {
			cfg.SSHHost = f.fields[fieldSSHHost].Value()
			sshPort := 22
			if sshPortStr := f.fields[fieldSSHPort].Value(); sshPortStr != "" {
				var err error
				sshPort, err = strconv.Atoi(sshPortStr)
				if err != nil {
					f.focusField(fieldSSHPort)
					return config.ConnectionConfig{}, "SSH port must be a number"
				}
			}
			cfg.SSHPort = sshPort
			cfg.SSHUser = f.fields[fieldSSHUser].Value()
			cfg.SSHKeyPath = f.fields[fieldSSHKeyPath].Value()
			cfg.SSHPassword = f.fields[fieldSSHPassword].Value()
			cfg.SSHPassphrase = f.fields[fieldSSHPassphrase].Value()
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
// validation error. A nil err is a success; a non-nil err is a failure whose
// fields are attributed (see classifyTestError) so the form can tint them.
func (f *ConnectionForm) SetTestResult(msg string, err error) {
	f.testing = false
	f.testMsg = msg
	f.testOK = err == nil
	f.errMsg = ""
	f.testStates = f.classifyTestError(err)
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
