package ui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/rsiota/creel/internal/config"
	"github.com/rsiota/creel/internal/db"
	"github.com/rsiota/creel/internal/secrets"
)

func (m Model) updateConnections(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// Help overlay is modal: scroll/tab keys navigate it, any other key
	// (incl. esc/?) closes it.
	if m.help.IsVisible() {
		if m.help.HandleKey(msg) {
			return m, nil
		}
		m.help.Hide()
		return m, nil
	}

	// Connection-deletion confirmation is modal — intercept all keys while the
	// y/n prompt is up. y/enter runs the delete (and its keychain purge); n/esc
	// cancels, leaving the selection untouched.
	if m.deleteConnConfirm != "" {
		switch msg.String() {
		case "y", "Y", "enter":
			name := m.deleteConnConfirm
			m.deleteConnConfirm = ""
			return m.execDeleteConnection(name)
		case "n", "N", "esc", "ctrl+c":
			m.deleteConnConfirm = ""
			return m, nil
		}
		return m, nil
	}

	// Filter mode intercepts all keys.
	if m.connList.IsFiltering() {
		switch msg.String() {
		case "esc", "ctrl+c":
			m.connList.CancelFilter()
			return m, nil
		case "enter":
			// Connect using the current filtered selection, then commit.
			cmd := m.connectToDB()
			m.connList.CommitFilter()
			return m, cmd
		case "backspace":
			m.connList.FilterBackspace()
			return m, nil
		case "up", "k":
			m.connList.MoveCursor(-1)
			return m, nil
		case "down", "j":
			m.connList.MoveCursor(1)
			return m, nil
		}
		if ch, ok := keyFilterChar(msg); ok {
			m.connList.FilterAddChar(ch)
			return m, nil
		}
		return m, nil
	}

	switch msg.String() {
	case "enter":
		return m, m.connectToDB()
	case "[", "left", "h":
		if m.connList.hasGroups() {
			m.connList.MoveGroupTab(-1)
			return m, nil
		}
	case "]", "right", "l":
		if m.connList.hasGroups() {
			m.connList.MoveGroupTab(1)
			return m, nil
		}
	case "?":
		m.help.Show()
		return m, nil
	case "n":
		m.state = stateAddConnection
		m.connForm = NewConnectionForm()
		m.connForm.setDriverField(m.settings.DefaultDriver)
		iw, ch := popupContentSize(m.height)
		m.connForm.SetSize(iw, ch)
		cmd := m.connForm.Focus()
		return m, cmd
	case "e":
		return m.openEditForm()
	case "d":
		return m.deleteSelectedConnection()
	case "/", "i":
		m.connList.StartFilter()
		return m, nil
	case "esc", "q":
		m.beginQuit()
		return m, tea.Quit
	case "up", "k":
		m.connList.MoveCursor(-1)
		return m, nil
	case "down", "j":
		m.connList.MoveCursor(1)
		return m, nil
	case "G":
		m.connList.SetCursor(m.connList.lastConnRow())
		return m, nil
	case "g":
		m.connList.SetCursor(m.connList.firstConnRow())
		return m, nil
	}

	return m, nil
}

func (m Model) openEditForm() (tea.Model, tea.Cmd) {
	name := m.connList.SelectedName()
	if name == "" {
		return m, nil
	}
	existing := m.config.GetConnection(name)
	if existing == nil {
		return m, nil
	}
	m.state = stateAddConnection
	m.connForm = NewConnectionFormEdit(*existing)
	iw, ch := popupContentSize(m.height)
	m.connForm.SetSize(iw, ch)
	cmd := m.connForm.Focus()
	return m, cmd
}

func (m Model) deleteSelectedConnection() (tea.Model, tea.Cmd) {
	name := m.connList.SelectedName()
	if name == "" {
		return m, nil
	}
	if m.confirmDestructive() {
		m.deleteConnConfirm = name
		return m, nil
	}
	return m.execDeleteConnection(name)
}

// execDeleteConnection removes the named connection and its keychain secrets.
// Shared by the gated (y-confirmed) and ungated (confirm_destructive: false)
// paths so the confirmed action is identical either way.
func (m Model) execDeleteConnection(name string) (tea.Model, tea.Cmd) {
	if name == "" {
		return m, nil
	}
	// Best-effort purge of any keychain secrets for this connection. A missing
	// key (the connection never used the keychain) is not an error.
	_ = secrets.DeleteAll(name)
	if m.recentStore != nil {
		_ = m.recentStore.Remove(name)
	}
	m.config.RemoveConnection(name)
	if err := m.config.Save(); err != nil {
		m.connError = err.Error()
		return m, nil
	}
	m.connError = ""
	m.loadConnections()
	return m, nil
}

func (m Model) updateAddConnection(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// ctrl+t tests the connection from any mode (disabled while a test runs).
	if msg.String() == "ctrl+t" {
		if m.connForm.testing {
			return m, nil
		}
		return m, m.testConnection()
	}

	// Submit (enter) and cancel (esc) only fire from normal mode. In insert
	// mode they fall through to the form, which exits insert instead.
	if !m.connForm.editing {
		switch msg.String() {
		case "esc":
			m.state = stateConnections
			m.connError = ""
			return m, nil
		case "enter":
			connCfg, errMsg := m.connForm.EnterPressed()
			if errMsg != "" {
				m.connForm.SetError(errMsg)
				return m, nil
			}

			if m.connForm.mode == formModeEdit {
				m.config.RemoveConnection(m.connForm.editName)
			}

			// Migrate secret fields to the OS keychain when requested. Falls back
			// to plaintext (in the config file) if the keychain is unavailable.
			connCfg, secErr := storeConnSecrets(connCfg, m.connForm.secretsMode())
			m.connError = ""
			if secErr != nil {
				m.connError = secErr.Error()
			}

			m.config.AddConnection(connCfg)
			if err := m.config.Save(); err != nil {
				m.connForm.SetError(err.Error())
				return m, nil
			}

			m.state = stateConnections
			m.loadConnections()
			return m, nil
		case "p":
			// Paste a postgres:// / mysql:// / sqlite URI from the clipboard.
			if clip, ok := readOSClipboard(); ok {
				m.pasteConnURI(clip)
				return m, nil
			}
			if cmd := m.beginClipQuery(clipPasteURI, ""); cmd != nil {
				return m, cmd
			}
			return m, nil
		}
	}

	var cmd tea.Cmd
	m.connForm, cmd = m.connForm.Update(msg)
	return m, cmd
}

// storeConnSecrets migrates a connection's secret fields to the OS keychain
// when mode is "keychain", replacing them in the config with opaque references.
// It returns the (possibly modified) config and an error describing why the
// keychain could not be used; in that case the config is returned unchanged so
// the caller falls back to storing plaintext.
//
// The secret fields the form exposes (password, ssh_password, ssh_passphrase)
// are managed here. Each is migrated to the OS keychain when mode is
// "keychain", replacing the plaintext value in the config with an opaque
// reference. Empty values and existing references are left untouched.
func storeConnSecrets(cfg config.ConnectionConfig, mode string) (config.ConnectionConfig, error) {
	if mode != "keychain" {
		return cfg, nil
	}
	if !secrets.Available() {
		return cfg, fmt.Errorf("keychain unavailable on this system; secrets stored in config file")
	}
	type secretField struct {
		name string
		val  string
	}
	fields := []secretField{
		{secrets.FieldPassword, cfg.Password},
		{secrets.FieldSSHPassword, cfg.SSHPassword},
		{secrets.FieldSSHPassphrase, cfg.SSHPassphrase},
	}
	for _, fl := range fields {
		if fl.val == "" || secrets.IsReference(fl.val) {
			continue
		}
		ref, err := secrets.Store(cfg.Name, fl.name, fl.val)
		if err != nil {
			return cfg, fmt.Errorf("storing %s: %w", fl.name, err)
		}
		switch fl.name {
		case secrets.FieldPassword:
			cfg.Password = ref
		case secrets.FieldSSHPassword:
			cfg.SSHPassword = ref
		case secrets.FieldSSHPassphrase:
			cfg.SSHPassphrase = ref
		}
	}
	return cfg, nil
}

func (m Model) updateWorkspace(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// Ensure panels are correctly sized (handles state transitions where
	// WindowSizeMsg hasn't re-fired).
	m.layoutWorkspace()

	var cmd tea.Cmd

	// Clear transient status-bar messages on any key press.
	m.clearFlash()

	// Help overlay is modal: scroll/tab keys navigate it, any other key
	// (incl. esc/?) closes it.
	if m.help.IsVisible() {
		if m.help.HandleKey(msg) {
			return m, nil
		}
		m.help.Hide()
		return m, nil
	}
	// Drop-database typed confirmation — intercepts all keys when active.
	if m.dropDBConfirm != "" {
		switch msg.String() {
		case "enter":
			if m.dropDBInput == m.dropDBConfirm {
				dbName := m.dropDBConfirm
				m.dropDBConfirm = ""
				m.dropDBInput = ""
				return m, m.execDropDatabase(dbName)
			}
			return m, nil
		case "esc", "ctrl+c":
			m.dropDBConfirm = ""
			m.dropDBInput = ""
			return m, nil
		case "backspace":
			if len(m.dropDBInput) > 0 {
				m.dropDBInput = m.dropDBInput[:len(m.dropDBInput)-1]
			}
			return m, nil
		}
		if ch, ok := keyFilterChar(msg); ok {
			m.dropDBInput += ch
			return m, nil
		}
		return m, nil
	}

	// Create-database name input — intercepts all keys when active.
	if m.createDBActive {
		switch msg.String() {
		case "enter":
			name := strings.TrimSpace(m.createDBInput)
			if name == "" {
				return m, nil
			}
			m.createDBInput = ""
			m.createDBActive = false
			return m, m.execCreateDatabase(name)
		case "esc", "ctrl+c":
			m.createDBActive = false
			m.createDBInput = ""
			m.createDBErr = ""
			return m, nil
		case "backspace":
			if len(m.createDBInput) > 0 {
				m.createDBInput = m.createDBInput[:len(m.createDBInput)-1]
			}
			m.createDBErr = ""
			return m, nil
		}
		if ch, ok := keyFilterChar(msg); ok {
			m.createDBInput += ch
			m.createDBErr = ""
			return m, nil
		}
		return m, nil
	}

	// Database picker is modal — intercept all keys when visible.
	if m.dbPicker.IsVisible() {
		// Filter mode (default): typing filters, esc → normal mode.
		if m.dbPicker.Filtering() {
			switch msg.String() {
			case "esc", "ctrl+c":
				m.dbPicker.StopFiltering()
				return m, nil
			case "enter":
				name := m.dbPicker.SelectedDatabase()
				m.dbPicker.Hide()
				return m, m.selectDatabase(name)
			case "up", "k":
				m.dbPicker.CursorUp()
				return m, nil
			case "down", "j":
				m.dbPicker.CursorDown()
				return m, nil
			case "backspace":
				m.dbPicker.FilterBackspace()
				return m, nil
			}
			if ch, ok := keyFilterChar(msg); ok {
				m.dbPicker.FilterAddChar(ch)
				return m, nil
			}
			return m, nil
		}

		// Normal mode: single-letter commands.
		switch msg.String() {
		case "esc", "ctrl+c":
			if m.dbPicker.MustChoose() {
				m.rollbackTxn()
				m.connection.Close()
				m.connection = nil
				m.dbPicker.Hide()
				m.state = stateConnections
				m.focus = FocusConnections
				m.loadConnections()
			} else {
				m.dbPicker.Hide()
			}
			return m, nil
		case "enter":
			name := m.dbPicker.SelectedDatabase()
			m.dbPicker.Hide()
			return m, m.selectDatabase(name)
		case "/":
			m.dbPicker.StartFiltering()
			return m, nil
		case "j", "down":
			m.dbPicker.CursorDown()
			return m, nil
		case "k", "up":
			m.dbPicker.CursorUp()
			return m, nil
		case "N":
			m.createDBActive = true
			m.createDBInput = ""
			m.createDBErr = ""
			return m, nil
		case "D":
			name := m.dbPicker.SelectedDatabase()
			if name != "" {
				if m.confirmDestructive() {
					m.dropDBConfirm = name
					m.dropDBInput = ""
					return m, nil
				}
				return m, m.execDropDatabase(name)
			}
			return m, nil
		}
		return m, nil
	}

	// Filter picker is modal — intercept all keys when visible.
	if m.filterPicker.IsVisible() {
		// ctrl+a / ctrl+n work in any state (empty or filtered) and never
		// collide with search typing since they're KeyCtrl, not KeyRunes.
		switch msg.String() {
		case "ctrl+a":
			m.filterPicker.SelectAll()
			return m, nil
		case "ctrl+n":
			m.filterPicker.SelectNone()
			return m, nil
		}
		// Navigation is arrow-keys only so every letter (including j/k) can
		// be typed into the filter at any time.
		switch msg.String() {
		case "esc", "ctrl+c":
			m.filterPicker.Hide()
			return m, nil
		case "enter":
			return m, m.applyFilterPickerSelection()
		case " ":
			m.filterPicker.ToggleSelected()
			return m, nil
		case "up":
			m.filterPicker.CursorUp()
			return m, nil
		case "down":
			m.filterPicker.CursorDown()
			return m, nil
		case "backspace":
			m.filterPicker.FilterBackspace()
			return m, nil
		}
		if msg.Type == tea.KeyRunes {
			m.filterPicker.FilterAddChar(msg.String())
			return m, nil
		}
		return m, nil
	}

	// Column visibility picker is modal — intercept all keys when visible.
	if m.columnPicker.IsVisible() {
		// ctrl+a / ctrl+n work in any state (empty or filtered).
		switch msg.String() {
		case "ctrl+a":
			m.columnPicker.SelectAll()
			return m, nil
		case "ctrl+n":
			m.columnPicker.SelectNone()
			return m, nil
		}
		// Navigation is arrow-keys only so every letter (including j/k) can
		// be typed into the filter at any time.
		switch msg.String() {
		case "esc", "ctrl+c":
			m.columnPicker.Hide()
			return m, nil
		case "enter":
			return m, m.applyColumnVisibility()
		case " ":
			m.columnPicker.ToggleSelected()
			return m, nil
		case "up":
			m.columnPicker.CursorUp()
			return m, nil
		case "down":
			m.columnPicker.CursorDown()
			return m, nil
		case "backspace":
			m.columnPicker.FilterBackspace()
			return m, nil
		}
		if msg.Type == tea.KeyRunes {
			m.columnPicker.FilterAddChar(msg.String())
			return m, nil
		}
		return m, nil
	}

	// Import prompt is modal — intercept all keys.
	if m.importPrompt.IsVisible() {
		switch msg.String() {
		case "esc", "ctrl+c":
			m.importPrompt.Hide()
			return m, nil
		case "enter":
			// Prefer accepting the path-completion dropdown (mirrors Tab / ex-line
			// Enter) over submitting the import.
			if m.importPrompt.AcceptPathCompletion() {
				return m, nil
			}
			path, err := m.importPrompt.ExpandPath()
			if err != nil {
				m.exportMsg = fmt.Sprintf("import failed: %v", err)
				return m, nil
			}
			m.importPrompt.Hide()
			return m, m.execImportSQL(path)
		}
		var cmd tea.Cmd
		m.importPrompt, cmd = m.importPrompt.Update(msg)
		return m, cmd
	}

	// Export picker is modal — intercept all keys.
	if m.exportPicker.IsVisible() {
		switch msg.String() {
		case "esc", "ctrl+c":
			m.exportPicker.Hide()
			return m, nil
		case "enter":
			tables := m.exportPicker.SelectedTables()
			if len(tables) == 0 {
				return m, nil
			}
			m.exportPicker.Hide()
			return m, m.execExportDump(tables)
		case " ":
			m.exportPicker.ToggleSelected()
			return m, nil
		case "a":
			m.exportPicker.SelectAll()
			return m, nil
		case "n":
			m.exportPicker.SelectNone()
			return m, nil
		case "f":
			m.exportPicker.CycleFormat()
			return m, nil
		case "up", "k":
			m.exportPicker.CursorUp()
			return m, nil
		case "down", "j":
			m.exportPicker.CursorDown()
			return m, nil
		}
		return m, nil
	}

	// Backup picker (:backup) is modal — schema/data per table.
	if m.backupPicker.IsVisible() {
		switch msg.String() {
		case "esc", "ctrl+c":
			m.backupPicker.Hide()
			return m, nil
		case "enter":
			plan := m.backupPicker.DumpPlan()
			if !plan.HasContent() {
				return m, nil
			}
			bin := m.backupPicker.Bin()
			m.backupPicker.Hide()
			return m, m.execNativeBackup(bin, plan)
		case " ":
			m.backupPicker.ToggleInclude()
			return m, nil
		case "s":
			m.backupPicker.ToggleSchema()
			return m, nil
		case "d":
			m.backupPicker.ToggleData()
			return m, nil
		case "a":
			m.backupPicker.SelectAll()
			return m, nil
		case "n":
			m.backupPicker.SelectNone()
			return m, nil
		case "o":
			m.backupPicker.SchemaOnlyAll()
			return m, nil
		case "up", "k":
			m.backupPicker.CursorUp()
			return m, nil
		case "down", "j":
			m.backupPicker.CursorDown()
			return m, nil
		case "g":
			m.backupPicker.CursorTop()
			return m, nil
		case "G":
			m.backupPicker.CursorBottom()
			return m, nil
		}
		return m, nil
	}

	// Export dialog (g X) is modal — intercept all keys.
	if m.exportOverlay.IsVisible() {
		switch msg.String() {
		case "esc", "ctrl+c":
			m.exportOverlay.Hide()
			return m, nil
		case "enter":
			format, cols, scope := m.exportOverlay.Commit()
			return m, m.exportResults(format, cols, scope)
		case " ":
			m.exportOverlay.Activate()
			return m, nil
		case "a":
			m.exportOverlay.SelectAllCols()
			return m, nil
		case "n":
			m.exportOverlay.SelectNoneCols()
			return m, nil
		case "up", "k":
			m.exportOverlay.CursorUp()
			return m, nil
		case "down", "j":
			m.exportOverlay.CursorDown()
			return m, nil
		}
		return m, nil
	}

	// Theme picker (g c) is modal — intercept all keys. Moving the cursor
	// live-previews the theme (the picker applies the palette itself); enter
	// persists the choice to the config, esc reverts to the open-time theme.
	// Provider picker (M from the assistant panel) is modal — intercept all
	// keys. j/k or up/down moves the cursor, enter commits the choice (and
	// persists it as the config's active provider), esc cancels.
	// Provider-deletion confirmation is stacked over the picker — it must be
	// checked first so y/enter run the delete and n/esc cancel while the prompt
	// is up (the picker swallows keys below this).
	if m.deleteProviderConfirm != "" {
		switch msg.String() {
		case "y", "Y", "enter":
			name := m.deleteProviderConfirm
			m.deleteProviderConfirm = ""
			return m, m.deleteProvider(name)
		case "n", "N", "esc", "ctrl+c":
			m.deleteProviderConfirm = ""
			return m, nil
		}
		return m, nil
	}
	if m.providerPicker.IsVisible() {
		switch msg.String() {
		case "esc", "ctrl+c":
			m.providerPicker.Hide()
			return m, nil
		case "enter":
			name := m.providerPicker.Selected()
			m.providerPicker.Hide()
			if name == "" {
				return m, nil
			}
			m.aiProvider = name
			m.config.AI.Default = name
			_ = m.config.Save()
			if p, ok := m.activeProvider(); ok && p.Model != "" {
				m.aiMsg = "provider: " + name + " (" + p.Model + ")"
			} else {
				m.aiMsg = "provider: " + name
			}
			return m, nil
		case "up", "k":
			m.providerPicker.Up()
			return m, nil
		case "down", "j":
			m.providerPicker.Down()
			return m, nil
		case "n":
			// New provider: open the add/edit form over the workspace.
			return m, func() tea.Msg { return openProviderFormAddMsg{} }
		case "e":
			return m, func() tea.Msg { return openProviderFormEditMsg{name: m.providerPicker.Selected()} }
		case "d":
			return m, m.deleteSelectedProvider()
		}
		return m, nil // swallow other keys while open
	}

	// Model browser (m from the assistant panel) is modal — intercept all keys.
	// j/k or up/down moves the cursor, enter commits the chosen model to the
	// active provider's config (and persists it), esc cancels. While the list
	// is still loading, navigation and enter are no-ops (Selected returns "").
	if m.modelBrowser.IsVisible() {
		switch msg.String() {
		case "esc", "ctrl+c":
			m.modelBrowser.Hide()
			return m, nil
		case "enter":
			sel := m.modelBrowser.Selected()
			if sel == "" {
				return m, nil // still loading or errored
			}
			m.modelBrowser.Hide()
			// Persist the chosen model to the active provider's `model:`.
			if name := m.modelBrowser.Provider(); name != "" {
				for i := range m.config.AI.Providers {
					if m.config.AI.Providers[i].Name == name {
						m.config.AI.Providers[i].Model = sel
						break
					}
				}
				_ = m.config.Save()
			}
			m.aiMsg = "model: " + sel
			return m, nil
		case "up", "k":
			m.modelBrowser.Up()
			return m, nil
		case "down", "j":
			m.modelBrowser.Down()
			return m, nil
		}
		return m, nil // swallow other keys while open
	}

	// Provider form (n/e from the `M` picker) is modal — intercept all keys.
	// It shares the connection form's vim model: ctrl+t probes /models, enter
	// saves (persisting the key to the keychain when requested), esc returns to
	// the provider picker. Insert-mode keys (esc/enter to commit, then back to
	// normal) are handled inside the form.
	if m.providerForm.IsVisible() {
		// ctrl+t tests from any mode (disabled while a probe is in flight).
		if msg.String() == "ctrl+t" {
			if m.providerForm.testing {
				return m, nil
			}
			return m, m.testProvider()
		}
		if !m.providerForm.editing {
			switch msg.String() {
			case "esc":
				m.providerForm.Hide()
				m.providerPicker.Show(m.config.AI.Providers, m.effectiveProviderName())
				return m, nil
			case "enter":
				return m, m.saveProviderForm()
			}
		}
		var cmd tea.Cmd
		m.providerForm, cmd = m.providerForm.Update(msg)
		return m, cmd
	}

	// Theme picker (g c) is modal — intercept all keys. Arrow keys move the
	// cursor (live-previewing the theme); every other key filters the list by
	// display name. enter persists the choice to the config (a no-op if the
	// filter has no matches), esc reverts to the open-time theme.
	if m.themePicker.IsVisible() {
		switch msg.String() {
		case "esc", "ctrl+c":
			applyTheme(m.themePicker.AppliedAtOpen(), m.settings.ThemeOverrides)
			m.themePicker.Hide()
			return m, nil
		case "enter":
			name := m.themePicker.Commit()
			if name == "" {
				return m, nil // no match — keep the picker open
			}
			m.settings.Theme = name
			m.config.Settings.Theme = name
			_ = m.config.Save()
			return m, nil
		case "up":
			m.themePicker.Up()
			return m, nil
		case "down":
			m.themePicker.Down()
			return m, nil
		case "backspace":
			m.themePicker.FilterBackspace()
			return m, nil
		}
		if ch, ok := keyFilterChar(msg); ok {
			m.themePicker.FilterAddChar(ch)
			return m, nil
		}
		return m, nil
	}

	// Drop-table typed-name confirmation is modal — intercept all keys.
	if m.dropTableConfirm != "" {
		switch msg.String() {
		case "enter":
			if m.dropTableInput == m.dropTableConfirm {
				table := m.dropTableConfirm
				m.dropTableConfirm = ""
				m.dropTableInput = ""
				return m, m.execDropTable(table)
			}
			return m, nil
		case "esc", "ctrl+c":
			m.dropTableConfirm = ""
			m.dropTableInput = ""
			return m, nil
		case "backspace":
			if len(m.dropTableInput) > 0 {
				m.dropTableInput = m.dropTableInput[:len(m.dropTableInput)-1]
			}
			return m, nil
		}
		if ch, ok := keyFilterChar(msg); ok {
			m.dropTableInput += ch
			return m, nil
		}
		return m, nil
	}

	// Destructive schema DDL confirmation (drop column only).
	if m.schemaConfirmSQL != "" {
		switch msg.String() {
		case "y", "Y", "enter":
			table := m.schemaConfirmTable
			query := m.schemaConfirmSQL
			action := m.schemaConfirmAction
			return m, m.execSchemaDDL(table, query, action, "")
		case "n", "N", "esc", "ctrl+c":
			m.clearSchemaConfirm()
			return m, nil
		}
		return m, nil
	}

	// Add-column form is modal.
	if m.addColumnForm.IsVisible() {
		switch msg.String() {
		case "esc", "ctrl+c":
			m.addColumnForm.Hide()
			m.clearSchemaConfirm()
			return m, nil
		case "enter", "ctrl+s":
			sql, errMsg := m.addColumnForm.Submit()
			if errMsg != "" {
				m.addColumnForm.SetError(errMsg)
				return m, nil
			}
			table := m.addColumnForm.Table()
			m.addColumnForm.SetError("")
			return m, m.execSchemaDDL(table, sql, db.SchemaAddColumn, "")
		}
		var cmd tea.Cmd
		m.addColumnForm, cmd = m.addColumnForm.Update(msg)
		return m, cmd
	}

	// Table rename form is modal.
	if m.tableRenameForm.IsVisible() {
		switch msg.String() {
		case "esc", "ctrl+c":
			m.tableRenameForm.Hide()
			m.clearSchemaConfirm()
			return m, nil
		case "enter", "ctrl+s":
			sql, errMsg := m.tableRenameForm.Submit()
			if errMsg != "" {
				m.tableRenameForm.SetError(errMsg)
				return m, nil
			}
			oldTable := m.tableRenameForm.Table()
			newTable := m.tableRenameForm.NewName()
			m.tableRenameForm.SetError("")
			return m, m.execSchemaDDL(oldTable, sql, db.SchemaRenameTable, newTable)
		}
		var cmd tea.Cmd
		m.tableRenameForm, cmd = m.tableRenameForm.Update(msg)
		return m, cmd
	}

	// Table designer takes over the workspace.
	if m.tableDesigner.IsVisible() {
		switch msg.String() {
		case "esc", "ctrl+c":
			if m.tableDesigner.IsEditing() {
				m.tableDesigner, _ = m.tableDesigner.Update(msg)
				return m, nil
			}
			m.tableDesigner.Hide()
			m.clearSchemaConfirm()
			return m, nil
		case "enter", "ctrl+s":
			if m.tableDesigner.IsEditing() {
				// From edit mode, enter commits the cell; submit is
				// triggered only when not editing.
				break
			}
			sql, errMsg := m.tableDesigner.Submit()
			if errMsg != "" {
				m.tableDesigner.SetError(errMsg)
				return m, nil
			}
			table := m.tableDesigner.TableName()
			m.tableDesigner.SetError("")
			return m, m.execSchemaDDL(table, sql, db.SchemaCreateTable, "")
		}
		var cmd tea.Cmd
		m.tableDesigner, cmd = m.tableDesigner.Update(msg)
		return m, cmd
	}

	// Schema editor takes over the workspace.
	if m.schemaEditor.IsVisible() {
		onColumnsTab := m.schemaEditor.ActiveTab() == seTabColumns
		switch msg.String() {
		case "esc", "ctrl+c":
			if m.schemaEditor.IsEditing() {
				m.schemaEditor, _ = m.schemaEditor.Update(msg)
				return m, nil
			}
			m.schemaEditor.Hide()
			return m, nil
		case "enter":
			// Column editing only applies on the Columns tab; read-only tabs
			// (e.g. expand a trigger) handle enter inside Update.
			if onColumnsTab && !m.schemaEditor.IsReadOnly() {
				if m.schemaEditor.IsEditing() {
					m.schemaEditor, _ = m.schemaEditor.Update(msg)
					// For existing rows, fire per-cell DDL immediately on commit.
					// For new rows, the user fills in cells one by one and
					// presses enter again (not editing) to submit ADD COLUMN.
					if !m.schemaEditor.IsNewRow() {
						sql, action, errMsg := m.schemaEditor.PendingEditDDL()
						if errMsg != "" {
							m.schemaEditor.SetError(errMsg)
							return m, nil
						}
						if sql != "" {
							m.schemaEditor.SetError("")
							return m, m.execSchemaDDL(m.schemaEditor.Table(), sql, action, "")
						}
					}
					return m, nil
				}
				// Not editing: fire pending DDL (ADD COLUMN for new rows).
				sql, action, errMsg := m.schemaEditor.PendingEditDDL()
				if errMsg != "" {
					m.schemaEditor.SetError(errMsg)
					return m, nil
				}
				if sql == "" {
					return m, nil
				}
				m.schemaEditor.SetError("")
				return m, m.execSchemaDDL(m.schemaEditor.Table(), sql, action, "")
			}
		case "d":
			// dd to drop column — only existing rows go through the confirm
			// flow, and only on the Columns tab. New rows are removed locally
			// by the editor's Update.
			if onColumnsTab && !m.schemaEditor.IsReadOnly() && m.schemaEditor.pendingD && !m.schemaEditor.IsNewRow() {
				m.schemaEditor.pendingD = false
				return m, m.dropCurrentColumn()
			}
		}
		var cmd tea.Cmd
		m.schemaEditor, cmd = m.schemaEditor.Update(msg)
		return m, cmd
	}

	// Discard / truncate / delete-rows confirmation dialogs are modal — intercept all keys.
	if m.discardConfirm || m.deleteRowsConfirmTable != "" || m.clearHistoryConfirm || m.clearBookmarksConfirm {
		switch msg.String() {
		case "y", "Y", "enter":
			if m.discardConfirm {
				m.results.DiscardEdits()
				m.discardConfirm = false
				return m, nil
			}
			if m.deleteRowsConfirmTable != "" {
				table := m.deleteRowsConfirmTable
				query := m.deleteRowsConfirmQuery
				count := m.deleteRowsConfirmCount
				m.deleteRowsConfirmTable = ""
				m.deleteRowsConfirmQuery = ""
				m.deleteRowsConfirmCount = 0
				return m, m.execDeleteRows(table, query, count)
			}
			if m.clearHistoryConfirm {
				m.clearHistoryConfirm = false
				if m.connection != nil && m.historyStore != nil {
					m.historyStore.Clear(m.connection.Config().Name)
				}
				m.history.SetEntries(nil)
				m.history.StartFilter()
				return m, nil
			}
			if m.clearBookmarksConfirm {
				m.clearBookmarksConfirm = false
				if m.connection != nil && m.bookmarkStore != nil {
					m.bookmarkStore.Clear(m.connection.Config().Name)
				}
				m.bookmarks.SetEntries(nil)
				m.bookmarks.StartFilter()
				return m, nil
			}
		case "n", "N", "esc", "ctrl+c":
			m.discardConfirm = false
			m.deleteRowsConfirmTable = ""
			m.deleteRowsConfirmQuery = ""
			m.deleteRowsConfirmCount = 0
			m.clearHistoryConfirm = false
			m.clearBookmarksConfirm = false
			return m, nil
		}
		return m, nil
	}

	// Truncate confirmation is modal — uses enter/esc (not y/n).
	if m.truncateConfirm != "" {
		switch msg.String() {
		case "enter":
			table := m.truncateConfirm
			m.truncateConfirm = ""
			return m, m.execTruncate(table)
		case "esc", "ctrl+c":
			m.truncateConfirm = ""
			return m, nil
		}
		return m, nil
	}

	// Kill-session confirmation is modal — enter/esc.
	if m.killConfirm != "" {
		switch msg.String() {
		case "enter":
			pid := m.killConfirm
			m.killConfirm = ""
			return m, m.execKill(pid)
		case "esc", "ctrl+c":
			m.killConfirm = ""
			return m, nil
		}
		return m, nil
	}

	// EXPLAIN ANALYZE confirmation — enter runs the timed plan; esc cancels.
	if m.explainAnalyzeConfirm {
		switch msg.String() {
		case "enter", "y", "Y":
			m.explainAnalyzeConfirm = false
			return m, m.execExplainAnalyze()
		case "esc", "ctrl+c", "n", "N":
			m.explainAnalyzeConfirm = false
			return m, nil
		}
		return m, nil
	}

	// Cell-edit popup is modal — vim editing; ctrl+s stages; esc insert→normal→close.
	if m.cellEdit.IsVisible() {
		switch msg.String() {
		case "ctrl+s":
			if m.cellEdit.IsReadOnly() {
				return m, nil
			}
			val := m.cellEdit.Value()
			if compacted, ok := compactJSON(val); ok {
				val = compacted
			}
			orig := m.results.RawRowValue(m.cellEdit.Row(), m.cellEdit.Col())
			if orig == "NULL" {
				orig = ""
			}
			if origCompacted, ok := compactJSON(orig); ok {
				orig = origCompacted
			}
			if val != orig {
				m.results.SetDirtyCell(m.cellEdit.Row(), m.cellEdit.Col(), val)
			}
			m.cellEdit.Hide()
			return m, nil
		case "esc", "ctrl+c":
			if handled, close := m.cellEdit.ConsumeEsc(); handled {
				if close {
					m.cellEdit.Hide()
				}
				return m, nil
			}
		case "q":
			if !m.cellEdit.IsReadOnly() && m.cellEdit.VimMode() == VimNormal {
				m.cellEdit.Hide()
				return m, nil
			}
		}
		var cmd tea.Cmd
		m.cellEdit, cmd = m.cellEdit.Update(msg)
		return m, cmd
	}

	// Explain panel is modal — j/k scroll, esc/q close.
	if m.explainPanel.IsVisible() {
		switch msg.String() {
		case "esc", "q", "ctrl+c":
			m.explainPanel.Hide()
			return m, nil
		}
		m.explainPanel = m.explainPanel.Update(msg)
		return m, nil
	}

	// Diff panel is modal — j/k scroll, a toggles all/changes, esc/q close.
	if m.diffPanel.IsVisible() {
		switch msg.String() {
		case "esc", "q", "ctrl+c":
			m.diffPanel.Hide()
			return m, nil
		}
		m.diffPanel = m.diffPanel.Update(msg)
		return m, nil
	}

	// Chart panel replaces the results grid — j/k scroll, esc/q close.
	// Keep : / ? / ctrl+p available so :watch (and help/palette) work without
	// closing the chart first; once the ex line or palette is open they own keys.
	if m.chartPanel.IsVisible() {
		if m.ex.visible {
			return m.handleExKey(msg)
		}
		if m.palette.visible {
			return m.handlePaletteKey(msg)
		}
		switch msg.String() {
		case ":":
			m.ex.Open()
			m.layoutWorkspace()
			return m, nil
		case "?":
			m.help.Show()
			return m, nil
		case "ctrl+p":
			m.palette.Open(m.paletteJumpSrc())
			return m, nil
		case "esc", "q", "ctrl+c":
			m.chartPanel.Hide()
			return m, nil
		case "x":
			m.exportChart(chartExportTXT)
			return m, nil
		case "X":
			m.exportChart(chartExportSVG)
			return m, nil
		case "enter":
			return m, m.drillChartBar()
		}
		m.chartPanel = m.chartPanel.Update(msg)
		return m, nil
	}

	// Lookup panel is modal — j/k scroll, enter jumps / opens editor, y yanks, esc/q close.
	if m.lookupPanel.IsVisible() {
		switch msg.String() {
		case "esc", "q", "ctrl+c":
			m.lookupPanel.Hide()
			return m, nil
		case "y":
			if text := m.lookupPanel.SelectedCopyText(); text != "" {
				if err := writeClipboard(text); err != nil {
					m.schemaMsg = "clipboard: " + err.Error()
				} else {
					m.schemaMsg = "copied to clipboard"
				}
			}
			return m, nil
		case "enter":
			if jump := m.lookupPanel.SelectedJump(); jump != "" {
				m.lookupPanel.Hide()
				m.syncSidebarCursorToTable(jump)
				return m, m.openTable(jump)
			}
			if text := m.lookupPanel.SelectedEditText(); text != "" {
				m.lookupPanel.Hide()
				m.editor.SetValue(text)
				m.focus = FocusEditor
				m.applyFocus()
				m.schemaMsg = "loaded into editor"
				return m, m.editor.Focus()
			}
			return m, nil
		}
		m.lookupPanel = m.lookupPanel.Update(msg)
		return m, nil
	}

	// ERD panel is modal — j/k scroll, y copy, s save, esc/q close.
	if m.erdPanel.IsVisible() {
		// A mouse drag is modal: esc cancels it (restoring the card), and every
		// other key is swallowed so keyboard focus can't race the in-flight move.
		if m.erdPanel.dragCard != "" {
			if msg.String() == "esc" {
				m.erdPanel = m.erdPanel.dragCancel()
			}
			return m, nil
		}
		// While the panel's "/" jump bar is open it consumes all keys
		// (including esc/enter, which the app would otherwise grab).
		if !m.erdPanel.searching {
			switch msg.String() {
			case "esc":
				// Esc steps back: clear an active FK path before closing the panel.
				if m.erdPanel.pathFrom != "" || len(m.erdPanel.pathCards) > 0 {
					m.erdPanel.zPrefix = false
					m.erdPanel = m.erdPanel.clearPath()
					return m, nil
				}
				m.hideERD()
				return m, nil
			case "q", "ctrl+c":
				m.hideERD()
				return m, nil
			case "y", "Y":
				m.erdPanel.zPrefix = false // these app-level actions aren't fold
				if err := writeClipboard(joinERDLines(m.erdPanel.MermaidLines())); err != nil {
					m.schemaMsg = "clipboard: " + err.Error()
				} else {
					m.schemaMsg = "erd copied to clipboard"
				}
				return m, nil
			case "s":
				m.erdPanel.zPrefix = false // family second keys, so drop a pending `z`
				m.saveERDToFile("erd.mmd", m.erdPanel.MermaidLines())
				return m, nil
			case "enter":
				// Browse the focused card (SELECT *); neighbourhood drill is `f`.
				nm, cmd := m.erdEnter()
				return nm, cmd
			case "f":
				nm, cmd := m.erdDrillIn()
				return nm, cmd
			case "i", "I":
				m.erdPanel.zPrefix = false
				if len(m.erdPanel.pathCards) >= 2 {
					return m.erdInsertPathJoin()
				}
				m.schemaMsg = "trace an FK path first (p)"
				return m, nil
			}
		}
		m.erdPanel = m.erdPanel.Update(msg)
		m.snapshotERDPositions()
		return m, nil
	}

	// Command palette is modal — intercept all keys when visible.
	if m.palette.visible {
		return m.handlePaletteKey(msg)
	}

	// Ex command line (":") is modal — intercept all keys when visible.
	if m.ex.visible {
		return m.handleExKey(msg)
	}

	// Tab navigation keys work from any panel (except editor insert mode).
	if m.handleTabKey(msg) {
		return m, nil
	}

	// Global workspace keys
	switch msg.String() {
	case "?":
		m.help.Show()
		return m, nil
	case ":":
		// Ex command line — not while a text-input / editing mode is active.
		if m.results.IsEditing() || m.inspector.IsEditing() || m.inspector.IsInserting() || m.inspector.IsFiltering() ||
			m.sidebarFiltering || m.searching || m.backendSearching ||
			m.crossSearch.IsVisible() || m.history.IsVisible() || m.bookmarks.IsVisible() ||
			(m.focus == FocusEditor && m.editor.CapturingKeys()) {
			break
		}
		m.ex.Open()
		return m, nil
	case "ctrl+p":
		// Jump-anywhere palette — but not while the editor is in insert mode,
		// where ctrl+p navigates the completion popup.
		if m.focus == FocusEditor && m.editor.CapturingKeys() {
			break
		}
		m.palette.Open(m.paletteJumpSrc())
		return m, nil
	case "q":
		// Quit — but only when no text-input / editing context is active,
		// otherwise 'q' must remain available for typing.
		if m.results.IsEditing() ||
			m.inspector.IsEditing() || m.inspector.IsInserting() || m.inspector.IsFiltering() ||
			m.sidebarFiltering ||
			m.history.IsVisible() ||
			m.bookmarks.IsVisible() ||
			m.crossSearch.IsVisible() ||
			m.backendSearching ||
			m.focus == FocusAssistant ||
			(m.focus == FocusEditor && m.editor.CapturingKeys()) {
			break
		}
		m.beginQuit()
		return m, tea.Quit
	case "ctrl+y":
		m.toggleHistory()
		return m, nil
	case "ctrl+e":
		return m, m.executeQuery()
	case "\\":
		if m.focus == FocusEditor && m.editor.VimMode() == VimNormal && !m.editor.CompletionVisible() {
			return m, m.executeQuery()
		}
	case "ctrl+d":
		// Vim-style page navigation — only when not in the editor
		// (Ctrl+D/U scroll within the editor in vim normal mode).
		// Also block when editing a cell or have unsaved edits.
		if m.focus != FocusEditor {
			if m.results.IsEditing() || m.results.HasDirtyCells() || m.inspector.IsInserting() {
				return m, nil
			}
			return m, m.nextPage()
		}
	case "ctrl+u":
		if m.focus != FocusEditor {
			if m.results.IsEditing() || m.results.HasDirtyCells() || m.inspector.IsInserting() {
				return m, nil
			}
			return m, m.prevPage()
		}
	case "ctrl+r":
		// Refresh schema (tables + columns) and re-run the last query.
		cmd := m.refreshSchema()
		return m, cmd
	case "ctrl+w":
		m.editorMaximized = !m.editorMaximized
		m.layoutWorkspace()
		return m, nil
	case "ctrl+b":
		// Browse databases (MySQL only).
		if m.connection != nil && (m.connection.Config().Driver == db.DriverMySQL || m.connection.Config().Driver == db.DriverPostgres) {
			return m, m.openDatabasePicker(false)
		}
		return m, nil
	case "ctrl+g":
		m.toggleBookmarks()
		return m, nil
	case "B":
		// Bookmark the current editor query (shared with :bookmark). Don't
		// intercept it while typing in the editor's insert mode.
		if m.focus == FocusEditor && m.editor.CapturingKeys() {
			break
		}
		m.bookmarkCurrentQuery()
		return m, nil
	case "ctrl+h", "ctrl+j", "ctrl+k", "ctrl+l":
		// Directional panel navigation — not while editing or in insert mode.
		if m.results.IsEditing() || m.inspector.IsEditing() || m.inspector.IsInserting() {
			return m, nil
		}
		if m.focus == FocusEditor && m.editor.CapturingKeys() {
			break // let it fall through to the editor
		}
		m = m.moveFocus(msg.String())
		return m, nil
	case "alt+h", "alt+j", "alt+k", "alt+l",
		"alt+ctrl+h", "alt+ctrl+j", "alt+ctrl+k", "alt+ctrl+l":
		// Nudge the adjacent seam in that direction. alt+ctrl+… is the
		// Bubble Tea encoding of ctrl+alt+letter (useful when plain alt is
		// claimed by the window manager). Same guards as focus movement so
		// typing in insert mode is unaffected.
		if m.results.IsEditing() || m.inspector.IsEditing() || m.inspector.IsInserting() {
			return m, nil
		}
		if m.focus == FocusEditor && m.editor.CapturingKeys() {
			break
		}
		m = m.resizePane(msg.String())
		return m, nil
	case "alt+b":
		if m.state != stateWorkspace {
			return m, nil
		}
		if m.results.IsEditing() || m.inspector.IsEditing() || m.inspector.IsInserting() {
			return m, nil
		}
		if m.focus == FocusEditor && m.editor.CapturingKeys() {
			break
		}
		m.toggleSidebar()
		return m, nil
	case "alt+e":
		if m.state != stateWorkspace {
			return m, nil
		}
		if m.results.IsEditing() || m.inspector.IsEditing() || m.inspector.IsInserting() {
			return m, nil
		}
		if m.focus == FocusEditor && m.editor.CapturingKeys() {
			break
		}
		m.toggleEditor()
		return m, nil
	case "ctrl+o":
		m.toggleInspector()
		return m, nil
	case "ctrl+f":
		m.toggleAssistant()
		return m, nil
	case "tab":
		// Don't cycle focus while editing a cell or inspector field.
		if m.results.IsEditing() || m.inspector.IsEditing() || m.inspector.IsInserting() {
			return m, nil
		}
		// Tab accepts completion when popup is visible.
		if m.focus == FocusEditor && m.editor.CompletionVisible() {
			m.editor.AcceptCompletion()
			return m, nil
		}
		m = m.cycleFocus()
		return m, nil
	case "shift+tab":
		if m.results.IsEditing() || m.inspector.IsEditing() || m.inspector.IsInserting() {
			return m, nil
		}
		m = m.cycleFocusBack()
		return m, nil
	case "ctrl+t":
		return m, m.showConnectionList()
	case "esc":
		// Close cross-search panel if visible.
		if m.crossSearch.IsVisible() {
			m.crossSearch.Hide()
			return m, nil
		}
		// Close bookmarks panel if visible.
		if m.bookmarks.IsVisible() {
			m.bookmarks.Toggle()
			return m, nil
		}
		// Close history panel if visible.
		if m.history.IsVisible() {
			m.history.Toggle()
			return m, nil
		}
		// If in visual mode, let the focused panel handle esc.
		if m.results.IsVisualMode() {
			break
		}
		// If in search mode, let the focused panel handle esc.
		if m.searching {
			break
		}
		// If in backend search mode, let the focused panel handle esc.
		if m.backendSearching {
			break
		}
		// Clear committed search highlighting (vim :nohl).
		if m.lastSearch != "" {
			m.lastSearch = ""
			m.results.SetSearchMatcher(nil)
			m.searchMsg = ""
			return m, nil
		}
		// If actively editing a cell or inspector field, let the focused
		// panel handle esc (to cancel the edit) instead of swallowing it.
		if m.results.IsEditing() || m.inspector.IsEditing() {
			break
		}
		// Exit sidebar fuzzy filter mode.
		if m.focus == FocusConnections && m.sidebarFiltering {
			m.sidebarFiltering = false
			m.sidebarFilter = ""
			m.sidebarCursor = 0
			return m, nil
		}
		// Exit inspector filter mode.
		if m.focus == FocusInspector && m.inspector.IsFiltering() {
			m.inspector.CancelFilter()
			m.inspector.cursorField = 0
			return m, nil
		}
		// Cancel new-record mode.
		if m.inspector.IsInserting() && !m.inspector.IsEditing() {
			m.inspector.CancelInsert()
			return m, m.maybeRestoreExplorerAfterInsert()
		}
		// In insert mode, esc goes to the editor for vim mode switching.
		// In normal mode, esc is a no-op (or could blur the editor).
		if m.focus == FocusEditor && m.editor.CapturingKeys() {
			m.editor, cmd = m.editor.Update(msg)
			return m, cmd
		}
		// The assistant handles esc itself: leave compose (insert) mode, or
		// close the panel in browse mode. Break out of this global switch so
		// it reaches the focus routing (the panel's HandleKey).
		if m.focus == FocusAssistant {
			break
		}
		return m, nil
	}

	// History panel takes over navigation when visible
	if m.history.IsVisible() {
		switch msg.String() {
		case "esc":
			m.history.CancelFilter()
			m.history.Toggle()
			return m, nil
		case "ctrl+c":
			m.history.CancelFilter()
			m.history.Toggle()
			return m, nil
		case "enter":
			q := m.history.SelectedQuery()
			if q != "" {
				m.editor.SetValue(q)
				m.focus = FocusEditor
				m.applyFocus()
			}
			m.history.CancelFilter()
			m.history.Toggle()
			return m, m.editor.Focus()
		case "backspace":
			if len(m.history.filter) > 0 {
				m.history.filter = m.history.filter[:len(m.history.filter)-1]
				m.history.cursor = 0
				m.history.scrollRow = 0
			}
			return m, nil
		case "up", "k":
			m.history.CursorUp()
			return m, nil
		case "down", "j":
			m.history.CursorDown()
			return m, nil
		case "s":
			m.history.ToggleSort()
			return m, nil
		case "D":
			if m.confirmDestructive() {
				m.clearHistoryConfirm = true
				return m, nil
			}
			if m.connection != nil && m.historyStore != nil {
				m.historyStore.Clear(m.connection.Config().Name)
			}
			m.history.SetEntries(nil)
			m.history.StartFilter()
			return m, nil
		case "b":
			// Promote the selected history entry to bookmarks.
			q := m.history.SelectedQuery()
			if q != "" && m.connection != nil && m.bookmarkStore != nil {
				m.bookmarkStore.Add(m.connection.Config().Name, q)
			}
			m.history.CancelFilter()
			m.history.Toggle()
			return m, nil
		}
		// Printable characters extend the filter.
		if ch, ok := keyFilterChar(msg); ok {
			m.history.filter += ch
			m.history.cursor = 0
			m.history.scrollRow = 0
			return m, nil
		}
	}

	// Bookmarks panel takes over navigation when visible
	if m.bookmarks.IsVisible() {
		switch msg.String() {
		case "esc":
			m.bookmarks.CancelFilter()
			m.bookmarks.Toggle()
			return m, nil
		case "ctrl+c":
			m.bookmarks.CancelFilter()
			m.bookmarks.Toggle()
			return m, nil
		case "enter":
			q := m.bookmarks.SelectedQuery()
			if q != "" {
				m.editor.SetValue(q)
				m.focus = FocusEditor
				m.applyFocus()
			}
			m.bookmarks.CancelFilter()
			m.bookmarks.Toggle()
			return m, m.editor.Focus()
		case "backspace":
			if len(m.bookmarks.filter) > 0 {
				m.bookmarks.filter = m.bookmarks.filter[:len(m.bookmarks.filter)-1]
				m.bookmarks.cursor = 0
				m.bookmarks.scrollRow = 0
			}
			return m, nil
		case "up", "k":
			m.bookmarks.CursorUp()
			return m, nil
		case "down", "j":
			m.bookmarks.CursorDown()
			return m, nil
		case "d":
			// Delete the bookmark under the cursor.
			if m.connection != nil && m.bookmarkStore != nil && m.bookmarks.CursorIndex() >= 0 {
				idx := len(m.bookmarks.entries) - 1 - m.bookmarks.CursorIndex()
				m.bookmarkStore.RemoveAt(m.connection.Config().Name, idx)
				entries, _ := m.bookmarkStore.Get(m.connection.Config().Name)
				m.bookmarks.SetEntries(entries)
			}
			return m, nil
		case "D":
			if m.confirmDestructive() {
				m.clearBookmarksConfirm = true
				return m, nil
			}
			if m.connection != nil && m.bookmarkStore != nil {
				m.bookmarkStore.Clear(m.connection.Config().Name)
			}
			m.bookmarks.SetEntries(nil)
			m.bookmarks.StartFilter()
			return m, nil
		}
		// Printable characters extend the filter.
		if ch, ok := keyFilterChar(msg); ok {
			m.bookmarks.filter += ch
			m.bookmarks.cursor = 0
			m.bookmarks.scrollRow = 0
			return m, nil
		}
	}

	// Cross-search panel takes over navigation when visible
	if m.crossSearch.IsVisible() {
		switch msg.String() {
		case "esc", "ctrl+c":
			m.crossSearch.Hide()
			return m, nil
		case "enter":
			// Open the selected hit when the query matches the last search;
			// otherwise (edited query, or no hits yet) start/re-run search.
			if m.crossSearch.CanOpenResult() {
				if r := m.crossSearch.SelectedResult(); r != nil {
					m.crossSearch.Hide()
					m.syncSidebarCursorToTable(r.Table)
					m.editor.SetValue(fmt.Sprintf("SELECT * FROM %s WHERE %s = '%s';",
						r.Table, r.Column, strings.ReplaceAll(r.Value, "'", "''")))
					return m, m.executeQuery()
				}
				return m, nil
			}
			if m.crossSearch.Query() != "" && !m.crossSearch.searching {
				return m, m.startCrossSearch()
			}
			return m, nil
		case "backspace":
			m.crossSearch.Backspace()
			return m, nil
		case "up":
			m.crossSearch.CursorUp()
			return m, nil
		case "down":
			m.crossSearch.CursorDown()
			return m, nil
		case "k":
			if m.crossSearch.HasResults() {
				m.crossSearch.CursorUp()
				return m, nil
			}
		case "j":
			if m.crossSearch.HasResults() {
				m.crossSearch.CursorDown()
				return m, nil
			}
		case "g":
			if m.crossSearch.HasResults() {
				m.crossSearch.CursorTop()
				return m, nil
			}
		case "G":
			if m.crossSearch.HasResults() {
				m.crossSearch.CursorBottom()
				return m, nil
			}
		case "ctrl+n":
			if gen, ok := m.crossSearch.ContinueSearch(); ok {
				return m, m.runCrossSearchBatch(m.crossSearch.Query(), gen)
			}
			return m, nil
		}
		if ch, ok := keyFilterChar(msg); ok {
			m.crossSearch.AddQueryChar(ch)
			return m, nil
		}
	}

	return m.updateFocusedPanel(msg, cmd)
}
