package ui

import (
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/ruben/gsql/internal/config"
	"github.com/ruben/gsql/internal/secrets"
)

// AI provider form. This is the add/edit form opened from the `M` provider
// picker (n new / e edit): it mirrors the connection form's vim-modal look
// (j/k move · h/l cycle a selector · e/i/a edit free text) but is a flat,
// four-field list, because a provider has only one secret (the API key) and
// none of the connection form's driver/SSH conditional machinery.
//
// The API key gets the same keychain treatment as a connection password: the
// "Secrets" field (keychain/plain) decides whether it is stored in the OS
// keychain as a "secret://" reference or left as plaintext in the config.
// ctrl+t probes the provider's /models endpoint to validate the key and base
// URL together — the documented real-world pain is a key that is valid but
// pointed at the wrong endpoint (e.g. a z.ai coding-plan key hitting the
// generic /api/paas/v4 path), and a live probe surfaces that immediately
// instead of letting it surface as a confusing "unauthorized" on first use.

// pfField indices. The list is flat (always rendered in this order), unlike
// the connection form's driver-conditional visibleFields().
const (
	pfName = iota
	pfKey
	pfBaseURL
	pfSecrets
	pfCount
)

// pfLabels is the display label for each field, parallel to the indices above.
var pfLabels = [...]string{"Name", "API Key", "Base URL", "Secrets"}

// pfChoices lists the cycling-selector values, rendered as < value > and
// changed with h/l. The only selector is Secrets (keychain/plain); the rest
// are free-text, edited with e/i/a.
var pfChoices = map[int][]string{
	pfSecrets: {"keychain", "plain"},
}

// isPFChoiceField reports whether fi is a cycling-selector field.
func isPFChoiceField(fi int) bool {
	_, ok := pfChoices[fi]
	return ok
}

// ProviderForm is the add/edit form for an AI provider. Reuses formMode
// (formModeAdd/formModeEdit) and the shared renderFieldBox / renderSelector
// helpers so it renders identically to the connection form.
type ProviderForm struct {
	fields   []textinput.Model
	active   int
	mode     formMode
	visible  bool
	editing  bool
	editName string // name being edited (edit mode); "" in add mode
	errMsg   string
	width    int

	// Test feedback. testing is true while a /models probe is in flight;
	// testMsg/testOK hold the result once it lands. testStates attributes the
	// outcome to the Key / Base URL fields so they tint green/red like the
	// connection form's per-field test colours.
	testing    bool
	testMsg    string
	testOK     bool
	testStates map[int]testState
}

// NewProviderForm returns a fresh add-mode form (hidden). The provider list is
// populated at Show time.
func NewProviderForm() ProviderForm {
	f := ProviderForm{fields: make([]textinput.Model, pfCount), mode: formModeAdd}
	f.fields[pfName] = newTextInput("e.g. openai", "", false)
	f.fields[pfKey] = newTextInput("sk-…", "", true)
	f.fields[pfBaseURL] = newTextInput("blank = https://api.openai.com/v1", "", false)
	f.fields[pfSecrets] = newTextInput("keychain / plain", "", false)
	f.fields[pfSecrets].SetValue("keychain")
	return f
}

// Show opens the form in add mode (fields cleared, Secrets defaulting to
// keychain).
func (f *ProviderForm) Show() {
	f.visible = true
	f.mode = formModeAdd
	f.editName = ""
	f.editing = false
	f.active = 0
	f.errMsg = ""
	f.clearTest()
	for i := range f.fields {
		f.fields[i].SetValue("")
	}
	f.fields[pfSecrets].SetValue("keychain")
}

// ShowEdit opens the form pre-filled from an existing provider. The API key is
// resolved to plaintext (when it is a keychain ref) so the masked field shows
// the real value — the same UX as editing a connection password. A ref that
// cannot be resolved is kept verbatim so saving without changes preserves it.
func (f *ProviderForm) ShowEdit(p config.AIProvider) {
	f.visible = true
	f.mode = formModeEdit
	f.editName = p.Name
	f.editing = false
	f.active = 0
	f.errMsg = ""
	f.clearTest()
	f.fields[pfName].SetValue(p.Name)
	f.fields[pfKey].SetValue(resolveSecretOrKeep(p.APIKey))
	f.fields[pfBaseURL].SetValue(p.BaseURL)
	f.fields[pfSecrets].SetValue(pfSecretsModeFromConfig(p.APIKey))
}

// Hide closes the form without saving.
func (f *ProviderForm) Hide() { f.visible = false }

// IsVisible reports whether the form is shown.
func (f ProviderForm) IsVisible() bool { return f.visible }

// IsEditing reports whether the form is in insert mode (typing into a field).
func (f ProviderForm) IsEditing() bool { return f.editing }

// ActiveIsChoice reports whether the active field is a cycling selector, used
// for context-sensitive keybinding hints (h/l vs e).
func (f ProviderForm) ActiveIsChoice() bool {
	return isPFChoiceField(f.activeField())
}

// activeField returns the field index at the current cursor (trivial here: the
// list is flat, so the cursor index is the field index).
func (f ProviderForm) activeField() int {
	if f.active < 0 || f.active >= pfCount {
		return pfName
	}
	return f.active
}

// pfSecretsModeFromConfig infers the secret-storage preference from a stored
// API key: "keychain" if it is a reference, otherwise "plain" so re-saving
// does not silently migrate a plaintext config.
func pfSecretsModeFromConfig(apiKey string) string {
	if secrets.IsReference(apiKey) {
		return "keychain"
	}
	return "plain"
}

// --- update -----------------------------------------------------------------

// Update mirrors the connection form's modal model. Normal mode: j/k (and
// Tab/arrows) move the cursor; h/l cycle the Secrets selector; e/i/a enter
// insert mode on free-text fields. Insert mode: keys edit the field, Esc/Enter
// commit and return to normal mode.
func (f *ProviderForm) Update(msg tea.Msg) (ProviderForm, tea.Cmd) {
	kmsg, isKey := msg.(tea.KeyMsg)
	if !isKey {
		// Non-key messages only matter in insert mode, where they go to the
		// active textinput.
		if f.editing {
			var cmd tea.Cmd
			fi := f.activeField()
			f.fields[fi], cmd = f.fields[fi].Update(msg)
			return *f, cmd
		}
		return *f, nil
	}
	key := kmsg.String()

	if f.editing {
		if key == "esc" || key == "enter" {
			f.editing = false
			f.fields[f.activeField()].Blur()
			return *f, nil
		}
		f.clearTransient()
		var cmd tea.Cmd
		fi := f.activeField()
		f.fields[fi], cmd = f.fields[fi].Update(msg)
		return *f, cmd
	}

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
	case "G":
		f.active = pfCount - 1
	case "l", "right":
		if isPFChoiceField(fi) {
			f.cycleChoice(fi, 1)
		}
	case "h", "left":
		if isPFChoiceField(fi) {
			f.cycleChoice(fi, -1)
		}
	case "e", "i", "a":
		if !isPFChoiceField(fi) {
			f.editing = true
			f.clearTransient()
			cmd := f.fields[fi].Focus()
			return *f, cmd
		}
	}
	return *f, nil
}

// moveActive advances the cursor by delta (wrapping).
func (f *ProviderForm) moveActive(delta int) {
	f.active += delta
	if f.active >= pfCount {
		f.active = 0
	}
	if f.active < 0 {
		f.active = pfCount - 1
	}
}

// cycleChoice advances a selector field by dir (wrapping).
func (f *ProviderForm) cycleChoice(fi, dir int) {
	choices := pfChoices[fi]
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
}

// clearTransient wipes stale validation/test messages so the user is not
// misled by an outdated result once they start editing again.
func (f *ProviderForm) clearTransient() {
	f.errMsg = ""
	f.clearTest()
}

// clearTest resets the test-connection feedback.
func (f *ProviderForm) clearTest() {
	f.testing = false
	f.testMsg = ""
	f.testOK = false
	f.testStates = nil
}

// --- view -------------------------------------------------------------------

// View renders the form as a list of bordered fields (shared renderFieldBox),
// reserving one line at the bottom for the validation/test message.
func (f ProviderForm) View() string {
	contentW := f.width
	valueWidth := contentW - 4
	if valueWidth < 1 {
		valueWidth = 1
	}

	labelSty := lipgloss.NewStyle().Foreground(colorLabel)

	var rendered strings.Builder
	for fi := 0; fi < pfCount; fi++ {
		st := f.statusOf(fi)
		labelStr := labelSty.Render(pfLabels[fi])
		marker := fieldTestMarker(st)
		rendered.WriteString(renderFieldBox(labelStr, marker, f.fieldValueContent(fi, valueWidth), contentW, formFieldBorder(fi == f.activeField(), st)))
		rendered.WriteString("\n")
	}
	return strings.TrimRight(rendered.String(), "\n") + "\n" + f.messageLine()
}

// fieldValueContent returns the styled, valueWidth-wide interior for a field's
// value box. The Secrets selector renders as < value >; the active free-text
// field in insert mode renders a bar cursor; other free-text fields show their
// value, a masked value for the API key, or the muted placeholder when empty.
func (f ProviderForm) fieldValueContent(fi, valueWidth int) string {
	if isPFChoiceField(fi) {
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

// messageLine returns the validation/test status line (blank when idle).
func (f ProviderForm) messageLine() string {
	switch {
	case f.errMsg != "":
		return errorStyle.Render(f.errMsg)
	case f.testing:
		return mutedStyle.Render("Probing /models…")
	case f.testMsg != "":
		if f.testOK {
			return successStyle.Render(f.testMsg)
		}
		return errorStyle.Render(f.testMsg)
	}
	return ""
}

// --- sizing -----------------------------------------------------------------

// SetSize sets the form's content width (the usable width inside border and
// padding) and propagates the value-box width to each textinput.
func (f *ProviderForm) SetSize(width int) {
	f.width = width
	valueWidth := width - 4
	if valueWidth < 1 {
		valueWidth = 1
	}
	for i := range f.fields {
		f.fields[i].Width = valueWidth - 1
	}
}

// effectiveHeight is the rendered content height: four fields (each
// linesPerField lines) plus the message line. Used by the overlay placement so
// the popup is sized to its content.
func (f ProviderForm) effectiveHeight() int {
	return pfCount*linesPerField + 1
}

// Focus is a no-op: the form opens in normal mode (cursor on Name, not
// editing). Insert mode is entered explicitly with e/i/a.
func (f *ProviderForm) Focus() tea.Cmd { return nil }

// --- submit / accessors -----------------------------------------------------

// EnterPressed validates the fields and returns the resulting provider (or an
// error message). The API key is returned as-is (plaintext as typed, or a ref
// kept from edit mode); migration to the keychain happens in the app's save
// handler (storeProviderSecret), mirroring the connection form.
func (f *ProviderForm) EnterPressed() (config.AIProvider, string) {
	name := strings.TrimSpace(f.fields[pfName].Value())
	if name == "" {
		return config.AIProvider{}, "name is required"
	}
	key := f.fields[pfKey].Value()
	if strings.TrimSpace(key) == "" {
		return config.AIProvider{}, "api key is required"
	}
	return config.AIProvider{
		Name:    name,
		APIKey:  key,
		BaseURL: strings.TrimSpace(f.fields[pfBaseURL].Value()),
	}, ""
}

// secretsMode returns the normalized secret-storage preference: "keychain"
// (the default when blank) or "plain". Anything not starting with "key" is
// treated as plaintext so typos never silently enable the keychain.
func (f ProviderForm) secretsMode() string {
	v := strings.ToLower(strings.TrimSpace(f.fields[pfSecrets].Value()))
	if v == "" || strings.HasPrefix(v, "key") {
		return "keychain"
	}
	return "plain"
}

// SetError sets a validation error message.
func (f *ProviderForm) SetError(msg string) { f.errMsg = msg }

// SetTesting marks the form as running a /models probe.
func (f *ProviderForm) SetTesting(b bool) { f.testing = b }

// SetTestResult records the outcome of a probe, clearing any stale validation
// error. A nil err is success; a non-nil err is attributed (see
// classifyTestError) so the Key / Base URL fields tint accordingly.
func (f *ProviderForm) SetTestResult(msg string, err error) {
	f.testing = false
	f.testMsg = msg
	f.testOK = err == nil
	f.errMsg = ""
	f.testStates = f.classifyTestError(err)
}

// statusOf returns the recorded test outcome for a field (testNeutral if no
// test has run, or the field was not attributed).
func (f ProviderForm) statusOf(fi int) testState {
	if f.testStates == nil {
		return testNeutral
	}
	return f.testStates[fi] // absent → testNeutral (zero value)
}

// classifyTestError attributes a probe failure to the field most likely at
// fault, so the form can tint it. An auth-style error (401/403/unauthorized/
// token) points at the API key; a reachability error (dial/host/refused/
// timeout/tls/404) points at the base URL. Success tints both green. Anything
// else is left neutral (the message line still reports it).
func (f ProviderForm) classifyTestError(err error) map[int]testState {
	out := map[int]testState{}
	if err == nil {
		out[pfKey] = testOK
		out[pfBaseURL] = testOK
		return out
	}
	s := strings.ToLower(err.Error())
	switch {
	case strings.Contains(s, "401"), strings.Contains(s, "403"),
		strings.Contains(s, "unauthorized"), strings.Contains(s, "token expired"),
		strings.Contains(s, "invalid api key"), strings.Contains(s, "incorrect api key"),
		strings.Contains(s, "permission"):
		out[pfKey] = testFail
	case strings.Contains(s, "dial"), strings.Contains(s, "no such host"),
		strings.Contains(s, "connection refused"), strings.Contains(s, "timeout"),
		strings.Contains(s, "deadline"), strings.Contains(s, "tls"),
		strings.Contains(s, "dns"), strings.Contains(s, "404"),
		strings.Contains(s, "not found"), strings.Contains(s, "x509"):
		out[pfBaseURL] = testFail
	}
	return out
}
