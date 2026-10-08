package ui

import (
	"encoding/base64"
	"errors"
	"flag"
	"os"
	"strings"
	"time"

	"github.com/atotto/clipboard"
	"github.com/aymanbagabas/go-osc52/v2"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/mattn/go-isatty"
)

const (
	osc52QueryTimeout = 1500 * time.Millisecond
	osc52SinkWindow   = 2 * time.Second
	osc52MaxPayload   = 2 << 20
	osc52ReadingMsg   = "reading clipboard…"
)

// clipKind is why an OSC 52 paste query is in flight.
type clipKind int

const (
	clipNone clipKind = iota
	clipPasteCell
	clipPasteURI
	clipFillMarked
	clipFillVisual
)

// osc52TimeoutMsg fires when the terminal did not answer an OSC 52 query.
type osc52TimeoutMsg struct{ gen uint64 }

// osc52SinkDoneMsg ends the window that swallows a late OSC 52 reply.
type osc52SinkDoneMsg struct{ gen uint64 }

const (
	osc52WaitIntro = iota
	osc52Collecting
	osc52WaitST
)

// osc52Collect reassembles an OSC 52 reply from the key events Bubble Tea
// emits for it. The reply arrives as ESC ] 52 ; Pc ; <base64> BEL (or ST),
// which the input parser turns into alt+], rune chunks, then ctrl+g or alt+\.
type osc52Collect struct {
	phase int
	buf   string
}

// feed consumes one key. done reports a finished reply; ok is false when the
// payload is not a valid OSC 52 selection. A query echo (payload "?") resets
// the collector and does not finish.
func (c osc52Collect) feed(k tea.KeyMsg) (osc52Collect, bool, string, bool) {
	switch c.phase {
	case osc52WaitIntro:
		if k.Alt && k.Type == tea.KeyRunes && string(k.Runes) == "]" {
			c.phase = osc52Collecting
		}
		return c, false, "", false
	case osc52WaitST:
		if isOSC52ST(k) {
			return c.finish()
		}
		if k.Type == tea.KeyRunes && !k.Alt {
			c.phase = osc52Collecting
			return c.appendRunes(k.Runes)
		}
		return c, false, "", false
	default:
		if k.Type == tea.KeyCtrlG || isOSC52ST(k) {
			return c.finish()
		}
		if k.Type == tea.KeyEscape {
			c.phase = osc52WaitST
			return c, false, "", false
		}
		if isOSC52Ignored(k) {
			return c, false, "", false
		}
		if k.Type == tea.KeyRunes && !k.Alt {
			return c.appendRunes(k.Runes)
		}
		return c, false, "", false
	}
}

func isOSC52ST(k tea.KeyMsg) bool {
	return k.Alt && k.Type == tea.KeyRunes && string(k.Runes) == `\`
}

func isOSC52Ignored(k tea.KeyMsg) bool {
	switch k.Type {
	case tea.KeyEnter, tea.KeyCtrlJ, tea.KeySpace:
		return true
	default:
		return false
	}
}

func (c osc52Collect) appendRunes(r []rune) (osc52Collect, bool, string, bool) {
	if len(c.buf)+len(r) > osc52MaxPayload {
		return osc52Collect{}, true, "", false
	}
	c.buf += string(r)
	return c, false, "", false
}

func (c osc52Collect) finish() (osc52Collect, bool, string, bool) {
	if isOSC52QueryEcho(c.buf) {
		return osc52Collect{}, false, "", false
	}
	text, ok := decodeOSC52Payload(c.buf)
	return osc52Collect{}, true, text, ok
}

func isOSC52QueryEcho(payload string) bool {
	return payload == "52;c;?" || payload == "52;p;?" || payload == "52;?"
}

// decodeOSC52Payload parses the bytes after OSC introducer, without the
// terminator: "52;c;<base64>". An empty selection is ok with an empty string.
func decodeOSC52Payload(s string) (string, bool) {
	if !strings.HasPrefix(s, "52;") {
		return "", false
	}
	rest := s[len("52;"):]
	i := strings.IndexByte(rest, ';')
	if i < 0 {
		return "", false
	}
	b64 := rest[i+1:]
	if b64 == "" {
		return "", true
	}
	raw, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		raw, err = base64.RawStdEncoding.DecodeString(strings.TrimRight(b64, "="))
		if err != nil {
			return "", false
		}
	}
	return string(raw), true
}

func osc52CopySequence(text string) string {
	return osc52.New(text).Mode(osc52Mode()).String()
}

func osc52QuerySequence() string {
	return osc52.Query().Mode(osc52Mode()).String()
}

func osc52Mode() osc52.Mode {
	// GNU screen drops OSC 52 unless it is wrapped in DCS. tmux is left
	// unwrapped: with set-clipboard on (the usual setting) tmux forwards the
	// sequence itself. The DCS passthrough form needs allow-passthrough on,
	// which is off by default.
	if os.Getenv("TMUX") == "" && os.Getenv("STY") != "" {
		return osc52.ScreenMode
	}
	return osc52.DefaultMode
}

// osc52IOAllowed reports whether this process should talk to the terminal
// clipboard. Tests opt out so a `go test` in a real terminal does not
// overwrite the developer's clipboard.
func osc52IOAllowed() bool {
	if flag.Lookup("test.v") != nil {
		return false
	}
	fd := os.Stderr.Fd()
	return isatty.IsTerminal(fd) || isatty.IsCygwinTerminal(fd)
}

func emitOSC52(seq string) error {
	if seq == "" || !osc52IOAllowed() {
		return errors.New("terminal clipboard unavailable")
	}
	_, err := os.Stderr.WriteString(seq)
	return err
}

// writeClipboard copies text to the OS clipboard. When that fails (no
// pasteboard over SSH), it emits OSC 52 so the local terminal can take it.
func writeClipboard(text string) error {
	osErr := clipboard.WriteAll(text)
	if osErr == nil {
		return nil
	}
	if err := emitOSC52(osc52CopySequence(text)); err == nil {
		return nil
	}
	return osErr
}

// readOSClipboard returns the OS clipboard. ok is false when no pasteboard
// is available; an empty string with ok true is a real empty clipboard.
func readOSClipboard() (string, bool) {
	s, err := clipboard.ReadAll()
	if err != nil {
		return "", false
	}
	return s, true
}

// beginClipQuery asks the terminal for its clipboard. A nil command means the
// caller should use its non-OSC fallback (yank, or the cell under the cursor).
func (m *Model) beginClipQuery(kind clipKind, fallback string) tea.Cmd {
	if m.osc52PasteDead || !osc52IOAllowed() {
		return nil
	}
	if err := emitOSC52(osc52QuerySequence()); err != nil {
		return nil
	}
	m.clipKind = kind
	m.clipSinking = false
	m.clipGen++
	gen := m.clipGen
	m.clipFallback = fallback
	m.clipCollect = osc52Collect{}
	m.schemaMsg = osc52ReadingMsg
	return tea.Tick(osc52QueryTimeout, func(time.Time) tea.Msg {
		return osc52TimeoutMsg{gen: gen}
	})
}

func (m *Model) clearReadingMsg() {
	if m.schemaMsg == osc52ReadingMsg {
		m.schemaMsg = ""
	}
}

func (m Model) handleOSC52Timeout(msg osc52TimeoutMsg) (Model, tea.Cmd) {
	if msg.gen != m.clipGen || m.clipKind == clipNone {
		return m, nil
	}
	kind, fallback := m.clipKind, m.clipFallback
	sawIntro := m.clipCollect.phase != osc52WaitIntro
	m.clipKind = clipNone
	m.clipCollect = osc52Collect{}
	m.clipFallback = ""
	if !sawIntro {
		m.osc52PasteDead = true
	}
	m.clipSinking = true
	m.clipGen++
	sinkGen := m.clipGen
	m.clearReadingMsg()
	m, cmd := m.finishClip(kind, fallback, "", false)
	sink := tea.Tick(osc52SinkWindow, func(time.Time) tea.Msg {
		return osc52SinkDoneMsg{gen: sinkGen}
	})
	if cmd != nil {
		return m, tea.Batch(cmd, sink)
	}
	return m, sink
}

func (m Model) handleOSC52SinkDone(msg osc52SinkDoneMsg) (Model, tea.Cmd) {
	if msg.gen != m.clipGen || m.clipKind != clipNone {
		return m, nil
	}
	m.clipSinking = false
	m.clipCollect = osc52Collect{}
	return m, nil
}

// feedClipKey consumes keys that belong to an in-flight OSC 52 reply, or to
// the short sink that drops a reply arriving after the timeout. consumed is
// false when the key should be handled normally.
func (m Model) feedClipKey(msg tea.KeyMsg) (Model, tea.Cmd, bool) {
	if m.clipSinking && m.clipKind == clipNone {
		if m.clipCollect.phase == osc52WaitIntro && !isOSC52Intro(msg) {
			m.clipSinking = false
			m.clipCollect = osc52Collect{}
			return m, nil, false
		}
		next, done, _, _ := m.clipCollect.feed(msg)
		m.clipCollect = next
		if done {
			m.clipSinking = false
			m.clipCollect = osc52Collect{}
			// A reply showed up after the timeout, so the terminal does
			// speak OSC 52 — allow the next paste to ask again.
			m.osc52PasteDead = false
		}
		return m, nil, true
	}
	if m.clipKind == clipNone {
		return m, nil, false
	}
	next, done, text, ok := m.clipCollect.feed(msg)
	m.clipCollect = next
	if !done {
		return m, nil, true
	}
	kind, fallback := m.clipKind, m.clipFallback
	m.clipKind = clipNone
	m.clipCollect = osc52Collect{}
	m.clipFallback = ""
	m.clipSinking = false
	m.clipGen++
	m.clearReadingMsg()
	m, cmd := m.finishClip(kind, fallback, text, ok)
	return m, cmd, true
}

func isOSC52Intro(k tea.KeyMsg) bool {
	return k.Alt && k.Type == tea.KeyRunes && string(k.Runes) == "]"
}

func (m Model) finishClip(kind clipKind, fallback, text string, ok bool) (Model, tea.Cmd) {
	if !ok {
		text = ""
	}
	switch kind {
	case clipPasteCell:
		if text == "" {
			text = m.yank
		}
		return m, m.pasteIntoCursorCell(text)
	case clipPasteURI:
		if text != "" {
			m.pasteConnURI(text)
		}
		return m, nil
	case clipFillMarked:
		if text == "" {
			text = fallback
		}
		m.applyMarkedFill(text)
		return m, nil
	case clipFillVisual:
		if text == "" {
			text = fallback
		}
		m.applyVisualFill(text)
		return m, nil
	default:
		return m, nil
	}
}
