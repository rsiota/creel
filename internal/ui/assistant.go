package ui

import (
	"strings"

	"github.com/charmbracelet/bubbles/textarea"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// AssistantWidth is the column width reserved for the AI assistant panel
// (including borders) when it is visible. It shares the inspector's right-hand
// slot, so the two are mutually exclusive: opening one closes the other.
const AssistantWidth = 52

// assistantRole tags a transcript turn.
type assistantRole int

const (
	assistantUser assistantRole = iota
	assistantAI
	assistantErr
)

// assistantMessage is one entry in the chat transcript. For assistant turns,
// sql holds the extracted statement (what gets applied to the editor / sent
// back as context) while text holds a short, display-friendly summary.
type assistantMessage struct {
	role assistantRole
	text string // user question, or a one-line summary of the AI reply
	sql  string // extracted SQL (assistant turns only)
}

// Assistant is a right-hand chat panel, sized and placed like the inspector,
// that turns natural-language requests into SQL via the model configured for
// :ai. It keeps a multi-turn transcript (sent back as conversation context so
// follow-ups like "now filter to active users" work) and never auto-executes:
// generated SQL is applied to the query editor for the user to review and run.
type Assistant struct {
	width     int
	height    int
	visible   bool
	messages  []assistantMessage
	input     textarea.Model // multiline compose box (soft-wraps long prompts)
	composing bool           // true while the compose box accepts typing
	scrollRow int            // transcript scroll offset (lines from the top)
	pending   bool           // a model request for this panel is in flight
	dirty     bool           // transcript changed since last layout/view (forces scroll-to-bottom)
	spinner   int            // current spinner animation frame (set by the app each render)
}

// composeHeight is the number of lines reserved for the compose box at the
// bottom of the panel: up to 3 wrapped lines of input plus a separator.
const composeHeight = 4

// NewAssistant creates the panel with an initialized (but blurred) multiline
// compose box.
func NewAssistant() Assistant {
	ta := textarea.New()
	ta.Prompt = ""
	ta.ShowLineNumbers = false
	ta.CharLimit = 0
	ta.SetHeight(composeHeight - 1) // leave one line for the separator
	ta.Placeholder = ""
	return Assistant{input: ta}
}

// Toggle shows or hides the panel, entering compose mode when shown.
func (a *Assistant) Toggle() {
	a.visible = !a.visible
	a.composing = a.visible
	a.scrollRow = 0
	if a.visible {
		a.input.Focus()
	} else {
		a.input.Blur()
	}
}

// Show opens the panel in compose mode.
func (a *Assistant) Show() {
	a.visible = true
	a.composing = true
	a.input.Focus()
}

// Hide closes the panel.
func (a *Assistant) Hide() {
	a.visible = false
	a.composing = false
	a.input.Blur()
}

// IsVisible reports whether the panel is shown.
func (a Assistant) IsVisible() bool { return a.visible }

// IsComposing reports whether the compose box is capturing typing.
func (a Assistant) IsComposing() bool { return a.composing }

// StartCompose focuses the compose box.
func (a *Assistant) StartCompose() {
	a.composing = true
	a.input.Focus()
}

// CancelCompose leaves compose mode (keeps the panel open for browsing).
func (a *Assistant) CancelCompose() {
	a.composing = false
	a.input.Blur()
	a.input.SetValue("")
}

// SetSize sets the panel's content dimensions (excluding its own border).
func (a *Assistant) SetSize(width, height int) {
	a.width = width
	a.height = height
	// The textarea soft-wraps to this width; the separator sits in its own
	// line, so the input gets the full interior width.
	iw := width - 2
	if iw < 4 {
		iw = 4
	}
	a.input.SetWidth(iw)
	if a.dirty {
		a.scrollToBottom()
		a.dirty = false
	}
}

// InputValue returns the current compose-box text.
func (a Assistant) InputValue() string { return a.input.Value() }

// SetPending marks a request in flight (shows a "thinking…" line).
func (a *Assistant) SetPending(v bool) {
	a.pending = v
	a.dirty = true
}

// IsPending reports whether a request for this panel is in flight.
func (a Assistant) IsPending() bool { return a.pending }

// AppendUser records a user question and pins the transcript to the bottom.
func (a *Assistant) AppendUser(q string) {
	a.messages = append(a.messages, assistantMessage{role: assistantUser, text: q})
	a.dirty = true
}

// AppendAssistant records a completed assistant turn with its extracted SQL.
func (a *Assistant) AppendAssistant(summary, sql string) {
	a.messages = append(a.messages, assistantMessage{role: assistantAI, text: summary, sql: sql})
	a.dirty = true
}

// AppendError records a failed turn so the user sees what went wrong in place.
func (a *Assistant) AppendError(text string) {
	a.messages = append(a.messages, assistantMessage{role: assistantErr, text: text})
	a.dirty = true
}

// LatestSQL returns the most recent assistant turn's SQL, or "" if none.
func (a Assistant) LatestSQL() string {
	for i := len(a.messages) - 1; i >= 0; i-- {
		if a.messages[i].role == assistantAI && a.messages[i].sql != "" {
			return a.messages[i].sql
		}
	}
	return ""
}

// HasTurns reports whether the transcript has any content.
func (a Assistant) HasTurns() bool { return len(a.messages) > 0 }

// Clear empties the transcript.
func (a *Assistant) Clear() {
	a.messages = nil
	a.scrollRow = 0
	a.dirty = true
}

// ConversationMessages renders the transcript as provider messages for
// multi-turn context: user turns as "user", assistant turns (using their
// extracted SQL) as "assistant". Errors are skipped so a failed turn doesn't
// pollute the model's view of the conversation.
func (a Assistant) ConversationMessages() []aiMessageOut {
	out := make([]aiMessageOut, 0, len(a.messages))
	for _, m := range a.messages {
		switch m.role {
		case assistantUser:
			out = append(out, aiMessageOut{role: "user", content: m.text})
		case assistantAI:
			if m.sql != "" {
				out = append(out, aiMessageOut{role: "assistant", content: m.sql})
			}
		}
	}
	return out
}

// aiMessageOut is a local mirror of ai.Message to keep the panel decoupled
// from the ai package's wire types; the app layer converts when dispatching.
type aiMessageOut struct {
	role    string
	content string
}

// HandleKey routes a key to the panel when it has focus. It returns the
// updated panel and a command only when an action needs app-level work
// (submitting a question, applying SQL, closing); nil otherwise. Global
// focus-movement keys (ctrl+h/j/k/l) are handled by the app before this is
// called, so they keep working.
func (a *Assistant) HandleKey(msg tea.KeyMsg) (Assistant, tea.Cmd) {
	if !a.composing {
		return a.handleBrowseKey(msg)
	}
	return a.handleComposeKey(msg)
}

func (a *Assistant) handleComposeKey(msg tea.KeyMsg) (Assistant, tea.Cmd) {
	switch msg.String() {
	case "enter":
		q := strings.TrimSpace(a.input.Value())
		if q == "" {
			// Empty box: send the latest AI SQL to the editor (if any), so the
			// natural flow is "type question → enter (send) → enter (insert)".
			if a.LatestSQL() != "" {
				return *a, func() tea.Msg { return applyAssistantSQLMsg{} }
			}
			return *a, nil
		}
		if a.pending {
			return *a, nil
		}
		a.input.SetValue("")
		return *a, func() tea.Msg { return submitAssistantMsg{question: q} }
	case "esc":
		a.CancelCompose()
		return *a, nil
	case "ctrl+c":
		a.input.SetValue("")
		a.CancelCompose()
		return *a, nil
	}
	// Everything else (typing, cursor motion) goes to the input.
	var cmd tea.Cmd
	a.input, cmd = a.input.Update(msg)
	return *a, cmd
}

func (a *Assistant) handleBrowseKey(msg tea.KeyMsg) (Assistant, tea.Cmd) {
	switch msg.String() {
	case "i", "o", "a", "I":
		a.StartCompose()
		return *a, nil
	case "enter":
		return *a, func() tea.Msg { return applyAssistantSQLMsg{} }
	case "j", "down":
		a.scrollDown(1)
		return *a, nil
	case "k", "up":
		a.scrollUp(1)
		return *a, nil
	case "G":
		a.scrollToBottom()
		return *a, nil
	case "c":
		a.Clear()
		return *a, nil
	case "esc", "q":
		a.Hide()
		return *a, func() tea.Msg { return closeAssistantMsg{} }
	}
	return *a, nil
}

func (a *Assistant) scrollUp(lines int) {
	a.scrollRow -= lines
	if a.scrollRow < 0 {
		a.scrollRow = 0
	}
}

func (a *Assistant) scrollDown(lines int) {
	max := a.transcriptHeight() - a.viewportLines()
	if max < 0 {
		max = 0
	}
	a.scrollRow += lines
	if a.scrollRow > max {
		a.scrollRow = max
	}
}

func (a *Assistant) scrollToBottom() {
	max := a.transcriptHeight() - a.viewportLines()
	if max < 0 {
		max = 0
	}
	a.scrollRow = max
}

// viewportLines is the number of transcript lines visible at once.
func (a Assistant) viewportLines() int {
	// Interior content height (a.height already excludes the panel border)
	// minus the compose area. At least 1.
	v := a.height - 2 - composeHeight
	if v < 1 {
		v = 1
	}
	return v
}

// transcriptHeight is the total rendered line count of all turns.
func (a Assistant) transcriptHeight() int {
	return len(a.renderTranscriptLines())
}

// scrollUpToTop was reserved for a "g g" chord; left out for now.

// View renders the panel: the scrollable transcript, a pending indicator when
// a request is in flight, and the compose box at the bottom.
func (a Assistant) View() string {
	if !a.visible {
		return ""
	}
	contentW := a.width - 2 // interior width inside the border
	if contentW < 4 {
		contentW = 4
	}

	lines := a.renderTranscriptLines()

	viewport := a.viewportLines()
	// Clamp scroll to the valid range, then slice the visible window.
	max := len(lines) - viewport
	if max < 0 {
		max = 0
	}
	if a.scrollRow > max {
		a.scrollRow = max
	}
	if a.scrollRow < 0 {
		a.scrollRow = 0
	}
	start := a.scrollRow
	end := start + viewport
	if end > len(lines) {
		end = len(lines)
	}
	visible := lines[start:end]

	// Pad the transcript to fill the viewport so the compose box stays pinned
	// to the bottom regardless of how many turns there are.
	for len(visible) < viewport {
		visible = append(visible, "")
	}

	body := strings.Join(visible, "\n")

	// Compose area: a separator rule, then the multiline input (soft-wrapped)
	// or a browse-mode hint, padded to a fixed height so it stays pinned to
	// the bottom and long prompts wrap instead of scrolling the panel.
	sep := lipgloss.NewStyle().Foreground(colorBorder).Render(strings.Repeat("─", contentW))
	var inputArea string
	if a.composing {
		inputArea = a.input.View()
	} else {
		inputArea = mutedStyle.Render(" i ask · enter apply SQL · c clear · j/k scroll · esc close")
	}
	composeBox := sep + "\n" + padLines(inputArea, composeHeight-1)

	content := lipgloss.JoinVertical(lipgloss.Left,
		body,
		composeBox,
	)

	panel := lipgloss.NewStyle().
		Width(a.width - 2).
		Height(a.height - 2).
		Render(content)
	return panel
}

// padLines ensures s occupies exactly n lines, padding with blanks or
// truncating, so a stacked block has a stable height.
func padLines(s string, n int) string {
	lines := strings.Split(s, "\n")
	if len(lines) > n {
		lines = lines[:n]
	}
	for len(lines) < n {
		lines = append(lines, "")
	}
	return strings.Join(lines, "\n")
}

// renderTranscriptLines turns the message history into display lines, wrapping
// each to the interior width. Returns the FULL list (the view slices it).
func (a Assistant) renderTranscriptLines() []string {
	contentW := a.width - 2
	if contentW < 8 {
		contentW = 8
	}
	var lines []string
	for _, m := range a.messages {
		var label, body string
		switch m.role {
		case assistantUser:
			label = "YOU:"
			body = m.text
		case assistantAI:
			label = "AI:"
			if m.sql == "" {
				body = "(no SQL returned)"
			} else {
				body = m.sql
			}
		case assistantErr:
			label = "ERR:"
			body = m.text
		}
		roleColor := colorPrimary
		switch m.role {
		case assistantUser:
			roleColor = colorAccent
		case assistantErr:
			roleColor = colorError
		}
		marker := lipgloss.NewStyle().Foreground(roleColor).Bold(true).Render(label)
		indent := len([]rune(label)) + 1 // label + separating space
		wrapW := contentW - indent
		if wrapW < 4 {
			wrapW = 4
		}
		for i, wl := range wrapRunes(body, wrapW) {
			if i == 0 {
				lines = append(lines, marker+" "+wl)
			} else {
				lines = append(lines, strings.Repeat(" ", indent)+wl)
			}
		}
		lines = append(lines, "") // blank line between turns
	}
	if a.pending {
		frame := spinnerFrames[a.spinner%len(spinnerFrames)]
		spin := lipgloss.NewStyle().Foreground(colorPrimary).Bold(true).Render(frame)
		thinking := lipgloss.NewStyle().Foreground(colorAccent).Render("thinking…")
		lines = append(lines, "  "+spin+" "+thinking)
	}
	return lines
}

// Internal messages produced by the panel, handled by the app.
type submitAssistantMsg struct{ question string }
type applyAssistantSQLMsg struct{}
type closeAssistantMsg struct{}
