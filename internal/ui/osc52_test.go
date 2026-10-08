package ui

import (
	"encoding/base64"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestOSC52CopySequence(t *testing.T) {
	t.Setenv("TMUX", "")
	t.Setenv("STY", "")
	t.Setenv("TERM", "xterm-256color")

	got := osc52CopySequence("hi")
	want := "\x1b]52;c;aGk=\x07"
	if got != want {
		t.Fatalf("copy sequence = %q, want %q", got, want)
	}

	ask := osc52QuerySequence()
	if ask != "\x1b]52;c;?\x07" {
		t.Fatalf("query sequence = %q", ask)
	}
}

func TestOSC52CopySequenceTmuxUnwrapped(t *testing.T) {
	t.Setenv("TMUX", "/tmp/tmux-1000/default,1,0")
	t.Setenv("STY", "")
	t.Setenv("TERM", "screen")

	got := osc52CopySequence("hi")
	if got != "\x1b]52;c;aGk=\x07" {
		t.Fatalf("tmux should forward a raw OSC 52 sequence, got %q", got)
	}
}

func TestOSC52CopySequenceScreen(t *testing.T) {
	t.Setenv("TMUX", "")
	t.Setenv("STY", "12345.pts-0.host")
	t.Setenv("TERM", "xterm-256color")

	got := osc52CopySequence("hi")
	if !strings.HasPrefix(got, "\x1bP") || !strings.Contains(got, "52;c;aGk=") {
		t.Fatalf("screen sequence = %q", got)
	}
}

func TestDecodeOSC52Payload(t *testing.T) {
	b64 := base64.StdEncoding.EncodeToString([]byte("hello"))
	got, ok := decodeOSC52Payload("52;c;" + b64)
	if !ok || got != "hello" {
		t.Fatalf("decode = %q ok=%v", got, ok)
	}
	got, ok = decodeOSC52Payload("52;c;")
	if !ok || got != "" {
		t.Fatalf("empty selection = %q ok=%v", got, ok)
	}
	if _, ok = decodeOSC52Payload("52;c;!!!"); ok {
		t.Fatal("invalid base64 should fail")
	}
	if _, ok = decodeOSC52Payload("not-osc"); ok {
		t.Fatal("non-OSC payload should fail")
	}
}

func TestOSC52CollectRoundTrip(t *testing.T) {
	b64 := base64.StdEncoding.EncodeToString([]byte("hello"))
	c := osc52Collect{}
	var done bool
	var text string
	var ok bool

	c, done, _, _ = c.feed(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{']'}, Alt: true})
	if done {
		t.Fatal("intro should not finish the reply")
	}
	// Split the payload the way a 256-byte read would.
	mid := len(b64) / 2
	c, done, _, _ = c.feed(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("52;c;" + b64[:mid])})
	if done {
		t.Fatal("partial payload should not finish")
	}
	c, done, _, _ = c.feed(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(b64[mid:])})
	if done {
		t.Fatal("payload should wait for the terminator")
	}
	_, done, text, ok = c.feed(tea.KeyMsg{Type: tea.KeyCtrlG})
	if !done || !ok || text != "hello" {
		t.Fatalf("done=%v ok=%v text=%q", done, ok, text)
	}
}

func TestOSC52CollectSTTerminator(t *testing.T) {
	b64 := base64.StdEncoding.EncodeToString([]byte("x"))
	c := osc52Collect{}
	c, _, _, _ = c.feed(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{']'}, Alt: true})
	c, _, _, _ = c.feed(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("52;c;" + b64)})
	_, done, text, ok := c.feed(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'\\'}, Alt: true})
	if !done || !ok || text != "x" {
		t.Fatalf("ST terminator: done=%v ok=%v text=%q", done, ok, text)
	}
}

func TestOSC52CollectIgnoresQueryEcho(t *testing.T) {
	c := osc52Collect{}
	c, _, _, _ = c.feed(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{']'}, Alt: true})
	c, _, _, _ = c.feed(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("52;c;?")})
	c, done, _, _ := c.feed(tea.KeyMsg{Type: tea.KeyCtrlG})
	if done || c.phase != osc52WaitIntro || c.buf != "" {
		t.Fatalf("query echo should reset, done=%v phase=%d buf=%q", done, c.phase, c.buf)
	}
}

func TestOSC52SinkLetsTypingThrough(t *testing.T) {
	m := newResultsWorkspaceModel()
	m.clipSinking = true
	m.clipGen = 1
	row := m.results.CursorRow()

	updated, _ := m.Update(keyRunes('j'))
	m = updated.(Model)
	if m.clipSinking {
		t.Fatal("a normal key should leave the late-reply sink")
	}
	if m.results.CursorRow() == row {
		t.Fatal("j should move the cursor after the sink stands down")
	}
}

func TestOSC52TimeoutUsesYank(t *testing.T) {
	m := newResultsWorkspaceModel()
	m = press(m, keyRunes('l')) // name column
	m.yank = "from-yank"
	m.clipKind = clipPasteCell
	m.clipGen = 3
	m.schemaMsg = osc52ReadingMsg

	updated, _ := m.Update(osc52TimeoutMsg{gen: 3})
	m = updated.(Model)
	if m.clipKind != clipNone {
		t.Fatalf("query still active: %d", m.clipKind)
	}
	if !m.osc52PasteDead {
		t.Fatal("timeout with no reply should stop further queries")
	}
	if !m.clipSinking {
		t.Fatal("timeout should arm the late-reply sink")
	}
	if got := m.results.RowValue(0, 1); got != "from-yank" {
		t.Fatalf("cell = %q, want from-yank", got)
	}
}
