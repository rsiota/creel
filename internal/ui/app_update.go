package ui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/rsiota/creel/internal/db"
)

func (m Model) update(msg tea.Msg) (tea.Model, tea.Cmd) {
	// Invalidate the view cache by default. A coalesced results wheel event is
	// the only view-neutral message; it re-enables the cache in its handler so
	// the wheel flood doesn't rebuild the screen thousands of times.
	m.viewCached = false
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m = m.updateLayout()
		if m.erdPanel.IsVisible() {
			m.erdPanel.SetSize(m.width, m.height-1)
		}
		return m, nil

	case osc52TimeoutMsg:
		return m.handleOSC52Timeout(msg)

	case osc52SinkDoneMsg:
		return m.handleOSC52SinkDone(msg)

	case tea.KeyMsg:
		// An OSC 52 reply is ordinary key events. Swallow them while a query
		// is in flight, and for a short window after a timeout so a late
		// reply is not typed into the focused panel. ctrl+c still quits.
		if m.clipKind != clipNone && (msg.String() == "ctrl+c" || msg.String() == "ctrl+q") {
			m.clipKind = clipNone
			m.clipCollect = osc52Collect{}
			m.clipFallback = ""
			m.clipSinking = false
			m.clipGen++
			m.clearReadingMsg()
		} else if m.clipSinking || m.clipKind != clipNone {
			var cmd tea.Cmd
			var consumed bool
			m, cmd, consumed = m.feedClipKey(msg)
			if consumed {
				return m, cmd
			}
		}

		// While a query is in flight, esc and ctrl+c cancel the query
		// instead of their normal behaviour. All other keys are swallowed
		// so the user can't trigger overlapping operations.
		// Backend search mode is exempt: it needs keystrokes to pass
		// through to updateWorkspace so the user can keep typing while
		// the previous search query is still running.
		if m.queryRunning && !m.backendSearching {
			if key.Matches(msg, key.NewBinding(key.WithKeys("esc", "ctrl+c"))) {
				m.queryCancelled = true
				if m.queryCancel != nil {
					m.queryCancel()
					m.queryCancel = nil
				}
				return m, nil
			}
			return m, nil
		}

		// While an AI request is in flight (:ai), esc and ctrl+c cancel it.
		// Other keys are swallowed, matching the query model, so a slow model
		// can't race a second request.
		if m.aiRunning {
			if key.Matches(msg, key.NewBinding(key.WithKeys("esc", "ctrl+c"))) {
				if m.aiCancel != nil {
					m.aiCancel()
					m.aiCancel = nil
				}
				m.aiRunning = false
				m.aiQuestion = ""
				m.aiMsg = "ai: cancelled"
				return m, nil
			}
			return m, nil
		}

		switch {
		case key.Matches(msg, key.NewBinding(key.WithKeys("ctrl+c"))):
			m.beginQuit()
			return m, tea.Quit
		case key.Matches(msg, key.NewBinding(key.WithKeys("ctrl+q"))):
			m.beginQuit()
			return m, tea.Quit
		}

		// Flash the matching hint group on the status bar (set before
		// dispatch so the value survives the value-receiver copy), and stage
		// the pressed key's description to show inline for a moment.
		if matched := matchHint(m.hintList(), msg.String()); matched != "" {
			m.hintFlash = matched
			m.hintFlashAt = time.Now()
			if d := m.hintDescription(matched); d != "" {
				m.hintDesc = d
				m.hintDescAt = time.Now()
			}
		} else {
			m.hintFlash = ""
			m.hintDesc = ""
		}

		if m.state == stateConnections {
			return m.updateConnections(msg)
		}
		if m.state == stateAddConnection {
			return m.updateAddConnection(msg)
		}
		nm, ncmd := m.updateWorkspace(msg)
		// Docked explorer is cursor-driven: if the results cursor landed on a
		// different row, re-root the tree. Cheap anchor check; reloads only on a
		// real change.
		if mm, ok := nm.(Model); ok {
			if rcmd := (&mm).maybeReloadDockedExplorer(); rcmd != nil {
				ncmd = tea.Batch(ncmd, rcmd)
			}
			return mm, ncmd
		}
		return nm, ncmd

	case tea.MouseMsg:
		// Help overlay is modal: the mouse wheel scrolls it; other mouse
		// events are ignored while it's open.
		if m.help.IsVisible() {
			return m.handleHelpMouse(msg)
		}
		// ERD panel is a full-screen overlay: it owns all mouse events while
		// visible (and stops them leaking through to the workspace behind it).
		if m.erdPanel.IsVisible() {
			// Size the persistent panel (View only sizes a discarded copy), so
			// the click hit-test reads the real viewport, not a zero-sized one.
			m.erdPanel.SetSize(m.width, m.height-1)
			return m.handleERDMouse(msg)
		}
		if m.state == stateConnections {
			return m.handleConnectionsMouse(msg)
		}
		if m.state == stateAddConnection {
			return m.handleConnectionFormMouse(msg)
		}
		if m.state == stateWorkspace {
			return m.handleWorkspaceMouse(msg)
		}
		return m, nil

	case paletteJumpMsg:
		return m, m.applyPaletteJump(msg)

	case queryExecutedMsg:
		m.queryRunning = false
		m.queryCancel = nil

		// Silently discard results from queries that were superseded (not
		// user-cancelled) — the newer query's result will replace them.
		if msg.cancelled && !m.queryCancelled {
			return m, nil
		}

		// Capture elapsed for :timing (after the superseded-query discard, so a
		// stale cancelled query doesn't overwrite the displayed duration).
		m.lastQueryElapsed = time.Since(m.queryStart)

		m.layoutWorkspace()

		// User-cancelled queries show a message but keep existing results.
		if m.queryCancelled {
			m.results.SetError("Query cancelled")
			m.queryCancelled = false
			return m, nil
		}

		// A query that exceeded the deadline gets a clear, distinct message
		// (the raw driver error is opaque). Existing results are kept.
		if msg.timedOut {
			m.results.SetError(fmt.Sprintf("Query timed out (limit %s) — press esc in-flight to cancel sooner", m.queryTimeout))
			return m, nil
		}

		// Record to history
		if m.connection != nil && m.historyStore != nil {
			m.historyStore.Record(m.connection.Config().Name, msg.query, m.lastQueryElapsed, msg.err == nil)
		}
		var cmd tea.Cmd
		if msg.err != nil {
			if ok, rcmd := m.maybeReconnectOnError(msg.err, true); ok {
				return m, rcmd
			}
			m.recordQueryFailure(msg.query, msg.err)
			errText := msg.err.Error()
			if msg.multiFail > 0 {
				errText = fmt.Sprintf("statement %d/%d: %s", msg.multiFail, msg.multiTotal, errText)
			}
			m.results.SetError(errText)
			if m.restoreCursor {
				m.restoreCursor = false
			}
			m.maybeJumpToQueryError(msg.err, msg.query, msg.execQuery)
		} else {
			m.clearQueryFailure()
			if msg.multiTotal > 0 {
				m.lastQuery = msg.query
				m.baseQuery = msg.query
				if msg.multiTotal > 1 {
					m.schemaMsg = fmt.Sprintf("ran %d statements", msg.multiRan)
				}
			}
			cols := make([]string, len(msg.result.Columns))
			for i, c := range msg.result.Columns {
				cols[i] = c.Name
			}
			display, renamed := disambiguateColumnNames(cols)
			m.colsDisambiguated = renamed
			if msg.setWrap {
				m.wrapSource = msg.wrapSource
			}

			// Check for "has next page" — we fetched pageSize+1 rows
			rows := msg.result.Rows
			blobs := msg.result.Blobs
			hasNext := false
			if len(rows) > msg.pageSize {
				hasNext = true
				rows = rows[:msg.pageSize]
				blobs = db.TrimBlobs(blobs, msg.pageSize)
			}

			m.results.SetResult(display, rows, msg.result.Message)
			m.results.SetBlobs(blobs)

			// Watch/tail: tint rows whose content wasn't on the previous page.
			if m.watchActive {
				if m.watchPrevRows != nil {
					m.results.SetWatchDelta(computeWatchDelta(m.watchPrevRows, rows))
				}
				m.watchPrevRows = cloneResultRows(rows)
			} else {
				m.watchPrevRows = nil
			}

			// Keep an open chart in sync with the new page (or re-fetch bang
			// charts). Manual queries that weren't charting still close it.
			redrawChart := m.chartPanel.IsVisible() && m.lastChartOK
			var chartCmd tea.Cmd
			if redrawChart {
				chartCmd = m.redrawLastChart(false)
			} else {
				m.chartPanel.Hide()
			}

			colTypes := make(map[string]string, len(msg.result.Columns))
			for i, c := range msg.result.Columns {
				colTypes[display[i]] = c.Type
			}
			m.results.SetColumnTypes(colTypes)
			m.page = msg.page

			// Build pagination status message
			m.pageMsg = m.buildPageMsg(msg.page, msg.pageSize, len(rows), hasNext)

			// Fire a background COUNT(*) on the first page of a table browse.
			if msg.page == 0 {
				cmd = m.fetchTotalRows()
			}
			if chartCmd != nil {
				cmd = tea.Batch(cmd, chartCmd)
			}

			// Enable inline editing and foreign-key navigation for simple table SELECTs.
			m.detectResultMetadata(msg.query)
			// Apply + grow remembered column widths for the backing table so
			// paging onto a short page (or re-querying) does not shrink columns.
			m.syncColWidthMemory()
			m.inspector.Reset()
			m.insertTarget = nil
			if m.restoreCursor {
				m.results.SetCursor(m.restoreCursorRow, m.restoreCursorCol)
				m.restoreCursor = false
			}
			m.syncInspectorFieldFromGrid()
			// If the relationship explorer is open, refresh it for the new
			// focused row. This covers drill-in (Enter), back, and any
			// manual query — the panel always reflects the current location.
			if m.explorer.IsVisible() {
				if cmd == nil {
					cmd = m.loadExplorer()
				} else {
					cmd = tea.Batch(cmd, m.loadExplorer())
				}
			}
		}
		return m, cmd

	case spinnerTickMsg:
		if !m.queryRunning && !m.aiRunning && !m.reconnecting {
			return m, nil
		}
		m.querySpinner = (m.querySpinner + 1) % len(spinnerFrames)
		return m, spinnerTick()

	case saveResultMsg:
		if msg.err != nil {
			if ok, rcmd := m.maybeReconnectOnError(msg.err, false); ok {
				return m, rcmd
			}
			m.results.SetSaveError(msg.err.Error())
		} else {
			m.results.ApplySavedEdits()
		}
		return m, nil

	case insertResultMsg:
		if msg.err != nil {
			if ok, rcmd := m.maybeReconnectOnError(msg.err, false); ok {
				return m, rcmd
			}
			m.results.SetSaveError(msg.err.Error())
		} else {
			m.inspector.CancelInsert()
			m.results.ConfirmSaved()
			m.insertTarget = nil
			if m.restoreExplorerAfterInsert {
				m.restoreExplorerPanel()
				m.explorer.markLoading()
				return m, m.loadExplorer()
			}
			return m, m.runPageQuery()
		}
		return m, nil

	case cloneResultMsg:
		if msg.err != nil {
			m.schemaMsg = fmt.Sprintf("clone failed: %v", msg.err)
		} else {
			m.schemaMsg = fmt.Sprintf("cloned %d row%s into %s", msg.count, plural(msg.count), msg.table)
			m.results.ClearMarks()
			return m, m.runPageQuery()
		}
		return m, nil

	case truncateResultMsg:
		if msg.err != nil {
			m.truncateMsg = fmt.Sprintf("truncate failed: %v", msg.err)
		} else {
			m.truncateMsg = fmt.Sprintf("truncated %s", msg.table)
			if m.resultsShowTable(msg.table) {
				if m.results.HasDirtyCells() {
					m.results.DiscardEdits()
				}
				return m, m.runPageQuery()
			}
		}
		return m, nil

	case deleteRowsResultMsg:
		if msg.err != nil {
			m.deleteRowsMsg = fmt.Sprintf("delete failed: %v", msg.err)
		} else {
			m.deleteRowsMsg = fmt.Sprintf("deleted %d row%s from %s", msg.count, pluralIf(msg.count != 1, "s"), msg.table)
			m.results.ClearMarks()
			if m.resultsShowTable(msg.table) {
				if m.results.HasDirtyCells() {
					m.results.DiscardEdits()
				}
				return m, m.runPageQuery()
			}
		}
		return m, nil

	case schemaResultMsg:
		if msg.err != nil {
			m.schemaMsg = fmt.Sprintf("schema change failed: %v", msg.err)
			m.clearSchemaConfirm()
			switch msg.action {
			case db.SchemaAddColumn:
				m.addColumnForm.SetError(msg.err.Error())
			case db.SchemaRenameTable:
				m.tableRenameForm.SetError(msg.err.Error())
			case db.SchemaCreateTable:
				m.tableDesigner.SetError(msg.err.Error())
			case db.SchemaRenameColumn, db.SchemaModifyType, db.SchemaModifyNullable, db.SchemaModifyDefault, db.SchemaDropColumn:
				m.schemaEditor.SetError(msg.err.Error())
			}
		} else {
			if msg.action == db.SchemaRenameTable {
				m.schemaMsg = fmt.Sprintf("renamed %s to %s", msg.table, msg.newTable)
				m.applyTableRename(msg.table, msg.newTable)
			} else if msg.action == db.SchemaCreateTable {
				m.schemaMsg = fmt.Sprintf("created table %s", msg.table)
				m.loadTables()
				m.syncSidebarCursorToTable(msg.table)
			} else if msg.action == db.SchemaDropTable {
				m.schemaMsg = fmt.Sprintf("dropped table %s", msg.table)
				delete(m.expanded, msg.table)
				delete(m.columnCache, msg.table)
				delete(m.pkCache, msg.table)
				delete(m.fkCache, msg.table)
				m.loadTables()
				// Clamp the sidebar cursor into the (now shorter) list.
				items := m.sidebarItems()
				if len(items) == 0 {
					m.sidebarCursor = 0
				} else if m.sidebarCursor >= len(items) {
					m.sidebarCursor = len(items) - 1
				}
				if m.schemaEditor.IsVisible() && m.schemaEditor.Table() == msg.table {
					m.schemaEditor.Hide()
				}
				// If the results panel was showing the dropped table, clear it so
				// the stale query isn't re-run (which would error).
				if m.resultsShowTable(msg.table) {
					m.results.Clear()
					m.lastQuery = ""
					m.baseQuery = ""
					m.clearAliasState()
					m.filters = nil
					m.editor.SetValue("")
				}
			} else {
				m.schemaMsg = schemaChangeMessage(msg.action, msg.table)
				m = m.refreshTableSchemaSync(msg.table)
				m.reloadSchemaPanel(msg.table)
			}
			m.clearSchemaConfirm()
			m.addColumnForm.Hide()
			m.tableRenameForm.Hide()
			m.tableDesigner.Hide()
			if msg.action == db.SchemaRenameTable && m.resultsShowTable(msg.newTable) {
				return m, tea.Batch(m.prefetchSchemas(), m.runPageQuery())
			}
			if m.resultsShowTable(msg.table) {
				return m, tea.Batch(m.prefetchSchemas(), m.runPageQuery())
			}
			return m, m.prefetchSchemas()
		}
		return m, nil

	case schemasLoadedMsg:
		m.columnCache = msg.schemas
		m.pkCache = msg.pks
		m.fkCache = msg.fks
		m.refreshCompletionCandidates()
		return m, nil

	case schemaTablesLoadedMsg:
		m.schemaTableCache = msg.cache
		m.refreshCompletionCandidates()
		return m, m.ensureSchemaCompletionFetch()

	case qualifiedTableSchemaMsg:
		if msg.err == nil && msg.key != "" {
			if m.columnCache == nil {
				m.columnCache = make(map[string][]db.Column)
			}
			m.columnCache[msg.key] = msg.cols
			m.refreshCompletionCandidates()
			if m.editor.CompletionVisible() {
				m.editor.StartCompletion() // refilter with new columns
			}
		}
		return m, nil

	case tableRowCountsMsg:
		m.tableRowCounts = msg.counts
		return m, nil

	case structureLoadedMsg:
		// Route read-only metadata into the schema editor's structure tabs.
		// Ignore stale results from a previous table.
		if m.schemaEditor.IsVisible() && m.schemaEditor.Table() == msg.table {
			m.schemaEditor.LoadStructure(msg.data)
		}
		return m, nil

	case connTestResultMsg:
		// Only relevant while the form is open; a save (enter) leaves the
		// form state, so a late result is dropped.
		if m.state != stateAddConnection {
			return m, nil
		}
		if msg.err != nil {
			m.connForm.SetTestResult("✗ "+db.FormatConnectError(msg.driver, msg.err), msg.err)
		} else {
			m.connForm.SetTestResult(fmt.Sprintf("✓ Connected (%s)", msg.driver), nil)
		}
		return m, nil

	case crossSearchStartMsg:
		// Begin searching from the first table.
		query := m.crossSearch.Query()
		gen := m.crossSearch.StartSearch(len(m.tables))
		return m, m.runCrossSearchBatch(query, gen)

	case crossSearchResultMsg:
		// Drop stale batches after Hide or a newer StartSearch.
		if msg.gen != m.crossSearch.Gen() || !m.crossSearch.IsSearching() {
			return m, nil
		}
		m.crossSearch.AddResults(msg.results, msg.tablesDone)
		m.crossSearch.AddBatchMeta(msg.skipped, false)
		m.crossSearch.applyScan(msg.nextTable, msg.tableOffset, msg.deferred)
		if !msg.done && len(m.crossSearch.results) < m.crossSearch.hitLimit {
			return m, m.runCrossSearchBatch(m.crossSearch.Query(), msg.gen)
		}
		// Page is full, or the schema has no further hits. more stays set
		// when the cursor can still yield rows so ctrl+n can continue.
		m.crossSearch.more = !msg.done
		m.crossSearch.capped = m.crossSearch.more
		m.crossSearch.FinishSearch()
		return m, nil

	case copyFlashTickMsg:
		if m.results.AdvanceCopyFlash() {
			return m, copyFlashTickCmd()
		}
		return m, nil

	case wheelTickMsg:
		// Apply the accumulated wheel delta in one scroll and release the
		// pending flag so the next wheel event can arm a fresh tick.
		m.wheelTickPending = false
		delta := m.wheelAccum
		m.wheelAccum = 0
		if delta != 0 {
			m.results.ScrollBy(delta)
		}
		return m, nil

	case copyCopiedClearMsg:
		m.results.ClearCopiedMessage()
		return m, nil

	case filterValuesMsg:
		if m.filterPicker.IsVisible() && m.filterPicker.Column() == msg.column {
			// Pre-select values that are already in an existing filter.
			preSelected := make(map[string]bool)
			if _, vals, found := findEqualityFilter(m.filters, msg.column); found {
				for _, v := range vals {
					preSelected[v] = true
				}
			}
			m.filterPicker.SetValues(msg.values, preSelected)
			pw, ph := popupDim()
			m.filterPicker.SetSize(pw, ph)
		}
		return m, nil

	case statsMsg:
		m.statsMsg = fmt.Sprintf("%s: %s", msg.column, msg.stats)
		return m, nil

	case chartReadyMsg:
		m.applyChartReady(msg)
		m.layoutWorkspace()
		return m, nil

	case countMsg:
		if msg.err == nil {
			m.totalRows = msg.total
			m.totalRowsSet = true
			// Rebuild pageMsg now that the total is known.
			rowCount := m.results.NumRows()
			hasNext := rowCount > m.pageSize
			if hasNext {
				rowCount = m.pageSize
			}
			m.pageMsg = m.buildPageMsg(m.page, m.pageSize, rowCount, hasNext)
		}
		return m, nil

	case exportDoneMsg:
		m.exportMsg = exportStatusMessage(msg.path, msg.count, msg.err)
		return m, nil

	case exportProgressMsg:
		if msg.err != nil {
			if msg.file != nil {
				msg.file.Close()
			}
			m.exportMsg = fmt.Sprintf("export failed: %v", msg.err)
			return m, nil
		}
		percent := (msg.index + 1) * 100 / msg.total
		m.exportMsg = fmt.Sprintf("Exporting %d/%d: %s (%d%%)", msg.index+1, msg.total, msg.name, percent)
		if msg.index+1 < msg.total {
			next := msg.index + 1
			return m, dumpTableCmd(msg.file, msg.bw, m.connection.DB(), m.connection.Config().Driver,
				msg.tables[next], next, msg.total, msg.tables, msg.path)
		}
		return m, dumpFooterCmd(msg.file, msg.bw, m.connection.Config().Driver, msg.total, msg.path)

	case exportDumpMsg:
		if msg.err != nil {
			m.exportMsg = fmt.Sprintf("export failed: %v", msg.err)
		} else {
			m.exportMsg = fmt.Sprintf("dumped %d table%s → %s", msg.tables, pluralIf(msg.tables != 1, "s"), msg.path)
		}
		return m, nil
	case backupProgressWrapper:
		m.exportMsg = backupProgressStatus(msg.msg.bytes, m.backupStarted)
		return m, waitForBackupProgress(msg.progress, msg.done)
	case backupPickerMsg:
		if msg.err != nil {
			m.schemaMsg = msg.err.Error()
			return m, nil
		}
		m.backupPicker.Show(msg.sizes, msg.bin)
		bw, bh := backupPickerDim(m.width, m.height)
		m.backupPicker.SetSize(bw, bh)
		return m, nil
	case backupDoneMsg:
		if msg.err != nil {
			m.exportMsg = fmt.Sprintf("backup failed: %v", msg.err)
		} else {
			size := db.FormatDumpSize(msg.bytes)
			m.exportMsg = fmt.Sprintf("backed up %s → %s", size, msg.path)
		}
		return m, nil
	case restoreProgressWrapper:
		m.exportMsg = restoreProgressStatus(msg.msg.bytes, m.restoreStarted)
		return m, waitForRestoreProgress(msg.progress, msg.done)
	case restoreDoneMsg:
		if msg.err != nil {
			hint := ""
			if !msg.continued {
				hint = " — :restore! to continue past SQL errors"
			}
			m.exportMsg = fmt.Sprintf("restore failed: %v%s", msg.err, hint)
			return m, nil
		}
		size := db.FormatDumpSize(msg.bytes)
		lines := db.StderrErrorLines(msg.clientStderr)
		if n := len(lines); n > 0 {
			m.exportMsg = fmt.Sprintf("restored %s ← %s (%d errors — review overlay)", size, msg.path, n)
			m.showRestoreErrorOverlay(msg.path, lines)
		} else {
			m.exportMsg = fmt.Sprintf("restored %s ← %s", size, msg.path)
		}
		m.loadTables()
		return m, nil

	case importProgressWrapper:
		percent := 0
		if msg.msg.total > 0 {
			percent = int(msg.msg.read * 100 / msg.msg.total)
		}
		m.exportMsg = fmt.Sprintf("Importing %s… %d%%", msg.msg.filename, percent)
		return m, waitForImportProgress(msg.progress, msg.done)

	case importDoneMsg:
		if msg.err != nil {
			m.exportMsg = fmt.Sprintf("import failed: %v", msg.err)
		} else {
			m.exportMsg = msg.result.Summary(msg.filename)
			m.loadTables()
			if len(msg.result.Errors) > 0 {
				m.showImportErrorOverlay(msg.filename, msg.result.Errors)
			}
		}
		return m, nil

	case flashTickMsg:
		// Only clear if no newer flash has arrived since this tick was armed.
		if msg.gen == m.flashGen {
			m.clearFlash()
		}
		return m, nil

	case watchTickMsg:
		return m.handleWatchTick(msg)

	case keepAliveTickMsg:
		return m.handleKeepAliveTick(msg)

	case keepAliveFailMsg:
		return m.handleKeepAliveFail(msg)

	case reconnectResultMsg:
		return m.handleReconnectResult(msg)

	case explainResultMsg:
		if msg.err != nil {
			if msg.forAI {
				m.aiMsg = fmt.Sprintf("EXPLAIN error: %v", msg.err)
			} else {
				m.statsMsg = fmt.Sprintf("EXPLAIN error: %v", msg.err)
			}
			return m, nil
		}
		driver := db.DriverSQLite
		if m.connection != nil {
			driver = m.connection.Config().Driver
		}
		planText := formatExplainPlan(msg.result, driver)
		if msg.query != "" && planText != "" {
			m.lastExplainSQL = msg.query
			m.lastExplainText = planText
		}
		if msg.forAI {
			return m, m.dispatchAIExplain(msg.query, planText, msg.focus)
		}
		m.explainPanel.Show(msg.result, driver)
		return m, nil
	case diagnoseResultMsg:
		if msg.err != nil {
			m.schemaMsg = fmt.Sprintf("diagnose failed: %v", msg.err)
			return m, nil
		}
		if msg.query != "" && msg.planText != "" {
			m.lastExplainSQL = msg.query
			m.lastExplainText = msg.planText
		}
		m.lookupPanel.Show(msg.title, msg.result, msg.jumps)
		return m, nil
	case lookupResultMsg:
		if msg.err != nil {
			m.schemaMsg = fmt.Sprintf("lookup failed: %v", msg.err)
			return m, nil
		}
		m.lookupPanel.Show(msg.title, msg.result, msg.jumps)
		return m, nil
	case killDoneMsg:
		if msg.err != nil {
			m.schemaMsg = fmt.Sprintf("kill %s failed: %v", msg.pid, msg.err)
		} else {
			m.schemaMsg = fmt.Sprintf("killed session %s", msg.pid)
		}
		return m, nil
	case explorerLoadedMsg:
		// Only apply if the panel is still open (the user may have closed it
		// while a load was in flight).
		if !m.explorer.IsVisible() {
			return m, nil
		}
		switch {
		case msg.err != nil:
			m.explorer.applyRootError(msg.depth, msg.err)
		case msg.root == nil:
			m.explorer.applyEmpty(msg.depth, msg.emptyMsg)
		default:
			m.explorer.applyRoot(msg.root, msg.depth)
		}
		// Record the row this tree is now rooted at, so the docked panel can tell
		// a real cursor move from a redundant reload.
		m.explorer.anchor = m.explorerAnchor()
		return m, nil
	case explorerChildrenMsg:
		if !m.explorer.IsVisible() || msg.parent == nil {
			return m, nil
		}
		switch {
		case msg.err != nil:
			m.explorer.applyChildrenError(msg.parent, msg.err)
		case msg.fold:
			m.explorer.applyFold(msg.parent)
		default:
			m.explorer.applyChildren(msg.parent, msg.children)
		}
		return m, nil
	case aiStreamChunkMsg:
		// A token batch from the streamed reply: grow the live preview in the
		// panel (content + any reasoning), then keep draining.
		m.assistant.AppendStreamDelta(msg.content, msg.reasoning)
		if m.aiStream != nil {
			return m, waitAIStream(m.aiStream)
		}
		return m, nil

	case aiResultMsg:
		// An AI request finished. Clear the in-flight state first so the
		// pending hint and esc-cancel gating are gone even if we error.
		m.aiRunning = false
		m.aiCancel = nil
		m.aiStream = nil
		q := m.aiQuestion
		m.aiQuestion = ""
		if msg.err != nil {
			switch {
			case msg.toPanel:
				m.assistant.SetPending(false)
				m.assistant.AppendError(errString(msg.err) + aiAuthHint(msg.err))
			default:
				m.aiMsg = fmt.Sprintf("ai failed: %v%s", msg.err, aiAuthHint(msg.err))
			}
			return m, nil
		}
		switch {
		case msg.toPanel:
			m.assistant.SetPending(false)
			if q == aiExplainQuestion {
				// Prose explanation — keep the full reply; don't offer Apply SQL.
				m.assistant.AppendAssistant(strings.TrimSpace(msg.reply), "")
			} else {
				m.assistant.AppendAssistant(summaryFor(msg), msg.sql)
			}
			return m, nil
		default:
			// :ai / :aifix — editor or AI scratch tab (ai_dry_run).
			kind := "ai"
			if q == aiFixQuestion {
				kind = "fix"
			} else if q != "" {
				kind = q
			}
			return m, m.deliverGeneratedSQL(msg.sql, kind)
		}

	case submitAssistantMsg:
		// The panel submitted a question. Record it in the transcript
		// immediately (so the user sees it), mark pending, and dispatch.
		if m.aiRunning {
			return m, nil // one request at a time
		}
		m.assistant.AppendUser(msg.question)
		m.assistant.SetPending(true)
		m.assistant.CancelCompose() // back to browse: watch the stream / apply SQL / ask a follow-up with `i`
		return m, m.sendAssistant(msg.question)

	case applyAssistantSQLMsg:
		// Apply the latest assistant SQL — editor review, or AI scratch when
		// ai_dry_run is on.
		sql := m.assistant.LatestSQL()
		if sql == "" {
			m.aiMsg = "no SQL to apply yet"
			return m, nil
		}
		return m, m.deliverGeneratedSQL(sql, "apply")

	case closeAssistantMsg:
		m.assistant.Hide()
		if m.focus == FocusAssistant {
			m.focus = FocusResults
			m.applyFocus()
		}
		m.layoutWorkspace()
		return m, nil

	case openProviderPickerMsg:
		// Always open the picker, even with no providers configured: that is
		// exactly the state where the user wants to add one (n), and bailing
		// out here would make the form unreachable. The empty picker renders a
		// "press n to add" placeholder.
		m.providerPicker.Show(m.config.AI.Providers, m.effectiveProviderName())
		return m, nil

	case openModelBrowserMsg:
		// `m` browses the models for the active provider. With no provider
		// configured (env-only mode) there is no /models endpoint to query.
		p, ok := m.activeProvider()
		if !ok {
			m.aiMsg = "configure an ai: provider in ~/.config/creel/config.yaml to browse models"
			return m, nil
		}
		m.modelBrowser.Show(p.Name, p.Model)
		return m, m.fetchModelsCmd()

	case fetchModelsMsg:
		// Populate the browser, or surface the fetch failure inline. The
		// browser is only open if `m` was just pressed (esc / a successful
		// pick closes it), so a stray msg with no visible browser is ignored.
		if !m.modelBrowser.IsVisible() {
			return m, nil
		}
		if msg.err != nil {
			m.modelBrowser.SetError(errString(msg.err))
			return m, nil
		}
		if p, ok := m.activeProvider(); ok {
			m.modelBrowser.SetModels(msg.models, p.Model)
		} else {
			m.modelBrowser.SetModels(msg.models, "")
		}
		if len(msg.models) == 0 {
			m.modelBrowser.SetError("provider returned no models")
		}
		return m, nil

	case openProviderFormAddMsg:
		// `n` from the `M` picker: open the provider form in add mode. The
		// picker is hidden (the form returns to it on esc/save).
		m.providerPicker.Hide()
		m.providerForm.Show()
		iw, _ := popupContentSize(m.height)
		m.providerForm.SetSize(iw)
		return m, nil

	case openProviderFormEditMsg:
		// `e` from the `M` picker: open the form pre-filled from the selected
		// provider. A missing name (empty picker) is a no-op.
		if msg.name == "" {
			return m, nil
		}
		p := m.config.GetAIProvider(msg.name)
		if p == nil {
			return m, nil
		}
		m.providerPicker.Hide()
		m.providerForm.ShowEdit(*p)
		iw, _ := popupContentSize(m.height)
		m.providerForm.SetSize(iw)
		return m, nil

	case providerTestResultMsg:
		// ctrl+t from the provider form: route the /models probe result back to
		// the form (field tinting + message). A stray msg with no visible form
		// is ignored.
		if !m.providerForm.IsVisible() {
			return m, nil
		}
		if msg.err != nil {
			m.providerForm.SetTestResult("✗ "+msg.err.Error()+aiAuthHint(msg.err), msg.err)
		} else {
			m.providerForm.SetTestResult("✓ reachable — key and endpoint valid", nil)
		}
		return m, nil

	case backendSearchTickMsg:
		// Only execute if the input still matches (user may have typed more).
		if m.backendSearching && msg.input == m.backendSearchInput {
			return m, m.runBackendSearch(msg.input)
		}
		return m, nil

	case dropDBResultMsg:
		if msg.err != nil {
			m.exportMsg = fmt.Sprintf("drop database failed: %v", msg.err)
			return m, nil
		}
		m.exportMsg = fmt.Sprintf("dropped database %s", msg.database)
		// If the dropped database was the current one, reconnect without a
		// default database so the picker forces a new selection.
		wasCurrent := m.connection != nil &&
			m.connection.Config().Database == msg.database
		if wasCurrent {
			if err := m.connection.UseDatabase(""); err != nil {
				m.connError = err.Error()
			}
		}
		return m, m.openDatabasePicker(wasCurrent)

	case createDBResultMsg:
		if msg.err != nil {
			m.exportMsg = fmt.Sprintf("create database failed: %v", msg.err)
			return m, nil
		}
		m.exportMsg = fmt.Sprintf("created database %s", msg.database)
		// Switch to the newly created database.
		m.dbPicker.Hide()
		return m, m.selectDatabase(msg.database)
	}

	if m.state == stateWorkspace {
		var cmd tea.Cmd
		switch m.focus {
		case FocusEditor:
			m.editor, cmd = m.editor.Update(msg)
			return m, tea.Batch(cmd, m.ensureSchemaCompletionFetch())
		case FocusResults:
			m.results, cmd = m.results.Update(msg)
		}
		return m, cmd
	}

	if m.state == stateAddConnection {
		var cmd tea.Cmd
		m.connForm, cmd = m.connForm.Update(msg)
		return m, cmd
	}

	return m, nil
}
