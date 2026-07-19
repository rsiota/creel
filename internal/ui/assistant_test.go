package ui

import (
	"strconv"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/ruben/gsql/internal/config"
)

func TestAssistantLatestSQL(t *testing.T) {
	a := NewAssistant()
	if got := a.LatestSQL(); got != "" {
		t.Errorf("empty transcript: got %q, want empty", got)
	}
	a.AppendUser("10 recent users")
	a.AppendAssistant("SELECT * FROM users ORDER BY id DESC LIMIT 10", "SELECT * FROM users ORDER BY id DESC LIMIT 10")
	a.AppendError("boom")
	if got := a.LatestSQL(); got != "SELECT * FROM users ORDER BY id DESC LIMIT 10" {
		t.Errorf("got %q", got)
	}
	// A later AI turn with no SQL must not shadow an earlier real one... it
	// should return the *latest* AI turn that has SQL. Here the later turn is
	// an error, so the earlier SQL still wins.
}

func TestAssistantLatestSQLPrefersMostRecent(t *testing.T) {
	a := NewAssistant()
	a.AppendAssistant("", "SELECT 1;")
	a.AppendAssistant("", "SELECT 2;")
	if got := a.LatestSQL(); got != "SELECT 2;" {
		t.Errorf("expected most recent SQL, got %q", got)
	}
}

func TestAssistantConversationMessages(t *testing.T) {
	a := NewAssistant()
	a.AppendUser("list users")
	a.AppendAssistant("SELECT * FROM users", "SELECT * FROM users")
	a.AppendError("ignored failure") // errors must NOT pollute context
	a.AppendUser("now only active ones")
	msgs := a.ConversationMessages()
	if len(msgs) != 3 {
		t.Fatalf("expected 3 context turns (user, ai, user), got %d", len(msgs))
	}
	if msgs[0].role != "user" || msgs[0].content != "list users" {
		t.Errorf("msgs[0] = %+v", msgs[0])
	}
	if msgs[1].role != "assistant" || msgs[1].content != "SELECT * FROM users" {
		t.Errorf("msgs[1] = %+v", msgs[1])
	}
	if msgs[2].role != "user" || msgs[2].content != "now only active ones" {
		t.Errorf("msgs[2] = %+v", msgs[2])
	}
}

func TestAssistantConversationMessagesSkipsEmptySQL(t *testing.T) {
	a := NewAssistant()
	a.AppendAssistant("(no SQL returned)", "") // empty sql dropped from context
	a.AppendUser("again")
	msgs := a.ConversationMessages()
	if len(msgs) != 1 {
		t.Fatalf("expected only the user turn, got %d: %+v", len(msgs), msgs)
	}
}

func TestAssistantClear(t *testing.T) {
	a := NewAssistant()
	a.AppendUser("q")
	a.AppendAssistant("SELECT 1", "SELECT 1")
	a.Clear()
	if a.HasTurns() {
		t.Errorf("transcript should be empty after Clear")
	}
}

func TestAssistantHandleKeySubmit(t *testing.T) {
	a := NewAssistant()
	a.Show()
	a.StartCompose()
	a.input.SetValue("10 recent users")
	_, cmd := a.HandleKey(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("enter should return a command")
	}
	msg := cmd()
	if sm, ok := msg.(submitAssistantMsg); !ok || sm.question != "10 recent users" {
		t.Errorf("expected submitAssistantMsg{10 recent users}, got %#v", msg)
	}
	// The compose box is cleared on submit.
	if a.input.Value() != "" {
		t.Errorf("input should be cleared after submit, got %q", a.input.Value())
	}
}

func TestAssistantHandleKeySubmitIgnoresEmpty(t *testing.T) {
	a := NewAssistant()
	a.Show()
	a.StartCompose()
	a.input.SetValue("   ")
	_, cmd := a.HandleKey(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd != nil {
		t.Errorf("empty question should not submit, got %v", cmd())
	}
}

func TestAssistantHandleKeySubmitBlocksWhilePending(t *testing.T) {
	a := NewAssistant()
	a.Show()
	a.StartCompose()
	a.SetPending(true)
	a.input.SetValue("hello")
	_, cmd := a.HandleKey(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd != nil {
		t.Errorf("should not submit while a request is pending")
	}
}

func TestAssistantBrowseApplySQL(t *testing.T) {
	a := NewAssistant()
	a.Show()
	a.CancelCompose() // enter browse mode
	a.AppendAssistant("", "SELECT 1;")
	_, cmd := a.HandleKey(tea.KeyMsg{Type: tea.KeyEnter})
	msg := cmd()
	if _, ok := msg.(applyAssistantSQLMsg); !ok {
		t.Errorf("enter in browse should emit applyAssistantSQLMsg, got %#v", msg)
	}
}

func TestAssistantBrowseCloseEsc(t *testing.T) {
	a := NewAssistant()
	a.Show()
	a.CancelCompose()
	_, cmd := a.HandleKey(tea.KeyMsg{Type: tea.KeyEsc})
	if a.IsVisible() {
		t.Errorf("esc in browse should hide the panel")
	}
	msg := cmd()
	if _, ok := msg.(closeAssistantMsg); !ok {
		t.Errorf("expected closeAssistantMsg, got %#v", msg)
	}
}

func TestAssistantViewRendersTranscript(t *testing.T) {
	a := NewAssistant()
	a.SetSize(50, 20)
	a.Show()
	a.AppendUser("list users")
	a.AppendAssistant("", "SELECT * FROM users")
	out := a.View()
	plain := stripANSI(out)
	for _, want := range []string{"list users", "SELECT * FROM users"} {
		if !strings.Contains(plain, want) {
			t.Errorf("View missing %q", want)
		}
	}
}

// TestAssistantLongPromptWraps checks that a long compose prompt stays inside
// the panel — it soft-wraps to multiple lines rather than overflowing the box
// (the old single-line textinput glitched and shifted left on each keystroke).
func TestAssistantLongPromptWraps(t *testing.T) {
	const w = 50
	a := NewAssistant()
	a.SetSize(w, 24)
	a.Show()
	a.input.SetValue("give me the ten most recent users that signed up since last week with a verified email address")

	out := a.View()
	contentW := w // panel interior width (border added by viewWorkspace)
	for i, line := range strings.Split(stripANSI(out), "\n") {
		if rw := runeWidth(line); rw > contentW {
			t.Errorf("line %d width %d exceeds interior width %d: %q", i, rw, contentW, line)
		}
	}
}

// runeWidth is a rough visible-width measure for the wrap test.
func runeWidth(s string) int { return len([]rune(s)) }

// TestAssistantHintsOnStatusBar locks in that the AI panel surfaces its
// keybindings on the status bar (via hintList) rather than an in-panel footer:
//
//   - browse mode lists the full set (compose, model, scroll, clear, close),
//   - compose (insert) mode collapses to the send/leave keys,
//   - the model picker overlay lists its own move/select/cancel keys.
func TestAssistantHintsOnStatusBar(t *testing.T) {
	m := Model{assistant: NewAssistant()}
	m.state = stateWorkspace
	m.assistant.Show()
	m.focus = FocusAssistant

	browse := strings.Join(m.hintList(), " ")
	for _, want := range []string{"i/a/o", "M", "esc"} {
		if !strings.Contains(browse, want) {
			t.Errorf("browse hints missing %q: got %v", want, m.hintList())
		}
	}

	m.assistant.StartCompose()
	compose := m.hintList()
	if len(compose) != 2 || compose[0] != "enter" || compose[1] != "esc" {
		t.Errorf("compose hints = %v, want [enter esc]", compose)
	}

	m.assistant.CancelCompose()
	m.providerPicker = NewProviderPicker()
	m.providerPicker.Show([]config.AIProvider{{Name: "zai", APIKey: "k", Model: "glm-4.6"}}, "")
	picker := strings.Join(m.hintList(), " ")
	for _, want := range []string{"j/k", "enter", "esc"} {
		if !strings.Contains(picker, want) {
			t.Errorf("provider picker hints missing %q: got %v", want, m.hintList())
		}
	}
}

// TestModelBrowserStates exercises the three display states (loading, list,
// error) and the cursor behaviour of the model browser modal.
func TestModelBrowserStates(t *testing.T) {
	var b ModelBrowser
	b.Show("groq", "old-model")
	if !b.IsVisible() || !b.loading {
		t.Fatal("Show should open in the loading state")
	}
	if b.Provider() != "groq" {
		t.Errorf("Provider = %q, want groq", b.Provider())
	}
	// While loading, navigation and selection are no-ops.
	b.Down()
	if b.Selected() != "" {
		t.Errorf("Selected during loading = %q, want empty", b.Selected())
	}

	// Populating centres the cursor on the current model.
	b.SetModels([]string{"a", "new-model", "old-model"}, "old-model")
	if b.loading {
		t.Error("still loading after SetModels")
	}
	if got := b.Selected(); got != "old-model" {
		t.Errorf("cursor = %q, want old-model", got)
	}
	b.Up()
	if got := b.Selected(); got != "new-model" {
		t.Errorf("after Up = %q, want new-model", got)
	}
	b.Up()
	if got := b.Selected(); got != "a" {
		t.Errorf("after Up = %q, want a", got)
	}
	b.Down()
	if got := b.Selected(); got != "new-model" {
		t.Errorf("after Down = %q, want new-model", got)
	}

	// Error state renders the message inline.
	b.SetError("provider returned 401")
	if !strings.Contains(stripANSI(b.View()), "provider returned 401") {
		t.Errorf("error view missing message: %q", stripANSI(b.View()))
	}

	b.Hide()
	if b.IsVisible() {
		t.Error("Hide should close the browser")
	}
}

// TestProviderPickerStyling verifies the provider picker has no in-popup
// footer, uses the primary ("blue") border, matches the width of the small
// confirm pickers (truncate/drop table), and uses the palette-style "❯"
// selection marker with a full-width primary background.
func TestProviderPickerStyling(t *testing.T) {
	provs := []config.AIProvider{
		{Name: "zai", APIKey: "k", Model: "glm-4.6"},
		{Name: "openai", APIKey: "k", Model: "gpt-4o-mini"},
	}
	p := NewProviderPicker()
	p.Show(provs, "zai")
	view := p.View()
	plain := stripANSI(view)

	// No in-popup keybinding footer.
	if strings.Contains(plain, "enter select") || strings.Contains(plain, "esc cancel") {
		t.Errorf("provider picker shows an in-popup footer: %q", plain)
	}
	// Provider names render.
	for _, prov := range provs {
		if !strings.Contains(plain, prov.Name) {
			t.Errorf("provider %q not rendered: %q", prov.Name, plain)
		}
	}
	// Palette-style chevron marker.
	if !strings.Contains(plain, "❯") {
		t.Errorf("provider picker missing the ❯ selected marker (got %q)", plain)
	}
	// Primary border styling present.
	if !strings.Contains(view, "\x1b[") {
		t.Error("provider picker view has no ANSI styling (border color not applied)")
	}
	// Selected row carries a full-width primary background.
	foundHighlight := false
	for _, line := range strings.Split(view, "\n") {
		if !strings.Contains(stripANSI(line), "❯") {
			continue
		}
		foundHighlight = true
		if !hasBackgroundSGR(line) {
			t.Error("selected provider row has no background highlight (expected primary bg)")
		}
	}
	if !foundHighlight {
		t.Error("no highlighted (❯) row found in the picker")
	}
	// Exterior width matches the small confirm pickers (truncate/drop table).
	if got, want := lipgloss.Width(view), lipgloss.Width(renderConfirmDialog("Truncate table users?")); got != want {
		t.Errorf("provider picker width = %d, want %d (confirm dialog)", got, want)
	}
}

// hasBackgroundSGR reports whether s contains an ANSI SGR sequence that sets
// a background colour (basic 40-47, bright 100-107, or 256/true-colour 48;…).
// Used to assert a row carries a fill background regardless of palette.
func hasBackgroundSGR(s string) bool {
	for _, m := range ansiRe.FindAllString(s, -1) {
		body := strings.TrimSuffix(strings.TrimPrefix(m, "\x1b["), "m")
		for _, field := range strings.Split(body, ";") {
			if field == "" {
				continue
			}
			if field == "48" || (len(field) == 2 && field[0] == '4' && field >= "40" && field <= "47") ||
				(len(field) == 3 && strings.HasPrefix(field, "10") && field >= "100" && field <= "107") {
				return true
			}
		}
	}
	return false
}

// TestAssistantStreamReasoning verifies reasoning (chain-of-thought) renders
// dimmed above the SQL preview while streaming, and is cleared on finalize.
func TestAssistantStreamReasoning(t *testing.T) {
	a := NewAssistant()
	a.SetSize(60, 24)
	a.Show()
	a.SetPending(true)
	a.AppendStreamDelta("", "I need the top 10 users by signup")
	a.AppendStreamDelta("SELECT 1", "")

	plain := stripANSI(a.View())
	reason := "I need the top 10 users by signup"
	if !strings.Contains(plain, reason) {
		t.Errorf("reasoning not shown in view")
	}
	// Reasoning must appear above the SQL preview.
	if ri, si := strings.Index(plain, reason), strings.Index(plain, "SELECT"); ri < 0 || si < 0 || ri > si {
		t.Errorf("reasoning should precede the SQL (reason@%d, sql@%d)", ri, si)
	}

	a.AppendAssistant("", "SELECT 1")
	if a.streamReason != "" {
		t.Errorf("streamReason not cleared after finalize = %q", a.streamReason)
	}
	if a.streamText != "" {
		t.Errorf("streamText not cleared after finalize = %q", a.streamText)
	}
}

// TestAssistantStreamPreview verifies the streamed reply renders live (as a
// highlighted SQL preview with a trailing spinner) and is replaced by the
// committed turn on finalize.
func TestAssistantStreamPreview(t *testing.T) {
	a := NewAssistant()
	a.SetSize(60, 20)
	a.Show()
	a.SetPending(true)
	a.AppendStreamDelta("SELECT 1", "")

	view := a.View()
	// The partial SQL is highlighted like a finished AI turn.
	if want := sqlKeywordStyle.Render("SELECT"); !strings.Contains(view, want) {
		t.Errorf("stream preview missing highlighted SELECT: %q", view)
	}
	// A trailing animated spinner marks it as still in progress.
	if want := spinnerFrames[a.spinner%len(spinnerFrames)]; !strings.Contains(stripANSI(view), want) {
		t.Errorf("stream preview missing trailing spinner %q", want)
	}

	// Finalize: preview is dropped for the committed turn; no live tail left.
	a.AppendAssistant("", "SELECT 1")
	if a.streamText != "" {
		t.Errorf("streamText not cleared after finalize = %q", a.streamText)
	}
}

// TestAssistantHighlightsSQL verifies AI responses are syntax-highlighted
// like the query editor (keywords, numbers, …), not rendered as plain text.
func TestAssistantHighlightsSQL(t *testing.T) {
	a := NewAssistant()
	a.SetSize(60, 20)
	a.Show()
	a.AppendUser("list users")
	a.AppendAssistant("", "SELECT 42 FROM users")

	view := a.View()
	// The number and keyword tokens must carry the shared SQL highlight
	// styles; plain text would have no such wrapping.
	if want := sqlNumberStyle.Render("42"); !strings.Contains(view, want) {
		t.Errorf("highlighted number %q not in view", want)
	}
	if want := sqlKeywordStyle.Render("SELECT"); !strings.Contains(view, want) {
		t.Errorf("highlighted keyword %q not in view", want)
	}
}

// TestAssistantPanelHeightStable verifies the panel never grows or shrinks as
// the transcript fills — the rendered height stays at the interior height so
// the layout (and the panel's own border) can't shift when a turn is added.
func TestAssistantPanelHeightStable(t *testing.T) {
	const w, h = 52, 30
	a := NewAssistant()
	a.SetSize(w, h)
	a.Show()
	a.StartCompose()
	a.AppendUser("q1")
	a.AppendAssistant("", "SELECT 1")
	want := h // panel renders at the full content height (border added outside)
	if got := lipgloss.Height(a.View()); got != want {
		t.Errorf("panel height after 1 turn = %d, want %d", got, want)
	}
	// Flood the transcript and confirm the height stays put.
	for i := 0; i < 50; i++ {
		a.AppendUser("question " + strconv.Itoa(i))
		a.AppendAssistant("", "SELECT * FROM t WHERE id = "+strconv.Itoa(i))
	}
	if got := lipgloss.Height(a.View()); got != want {
		t.Errorf("panel height after many turns = %d, want %d", got, want)
	}
}
