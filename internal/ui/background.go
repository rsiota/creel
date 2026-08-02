package ui

import (
	"regexp"
	"strconv"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// paintBackground fills every cell in view that has no explicit background
// colour with bg. Cells whose styles set their own background (the status
// bar, selection highlights, table stripes, marked rows, …) keep that
// background untouched.
//
// This is how creel paints the app background with the active theme's bg
// colour — including during live theme preview — without threading a
// background through every renderer. The pass runs after the whole view
// (workspace + overlays) is assembled, so popups like the theme picker also
// pick up the bg automatically.
//
// It works by walking the rendered string and tracking the active background
// state through the SGR (color) escape sequences lipgloss emits. Whenever the
// background is "default" (never set, or cleared by a reset) at a printable
// cell, the theme bg is emitted first. Explicit backgrounds override it until
// the next reset returns to default.
func paintBackground(view string, bg lipgloss.Color) string {
	seq := ansiBgSeq(bg)
	if seq == "" {
		return view
	}

	var out strings.Builder
	out.Grow(len(view) + 64)

	// explicit: an explicit (non-default) background is active (don't paint).
	// injected: the theme-bg sequence is currently in effect in the output.
	explicit, injected := false, false

	pos := 0
	for {
		match := sgrRE.FindStringSubmatchIndex(view[pos:])
		if match == nil {
			// No more SGR sequences: copy the trailing literal run, painting
			// default-bg cells as we go.
			writeLiteral(&out, view[pos:], seq, &explicit, &injected)
			break
		}
		start, end := match[0], match[1]
		// Literal run before this SGR sequence.
		if start > 0 {
			writeLiteral(&out, view[pos:pos+start], seq, &explicit, &injected)
		}
		// Emit the SGR sequence verbatim, then fold its parameters into state.
		out.WriteString(view[pos+start : pos+end])
		applySGR(view[pos+match[2]:pos+match[3]], &explicit, &injected)
		pos += end
	}
	return out.String()
}

// writeLiteral emits a run of text containing no SGR sequences, injecting the
// theme bg before the first printable cell whenever the background is default.
// Printable here means rune >= 0x20, so spaces (padding) are painted while
// control characters (newline, ESC, …) are passed through without triggering
// an inject.
func writeLiteral(out *strings.Builder, s, seq string, explicit, injected *bool) {
	for _, r := range s {
		if r >= 0x20 && !*explicit && !*injected {
			out.WriteString(seq)
			*injected = true
		}
		out.WriteRune(r)
	}
}

// applySGR folds an SGR parameter list (the capture group, e.g. "0;38;2;1;2;3")
// into the background-tracking state. Parameters are applied in order so that
// a reset followed by a new background (e.g. "0;48;2;0;0;0") is resolved
// correctly. Colour-spec sub-parameters after 38/48 (5;N or 2;r;g;b) are
// consumed so a zero component can't be mistaken for a reset.
func applySGR(paramsStr string, explicit, injected *bool) {
	if paramsStr == "" {
		// CSI m with no parameters is equivalent to a full reset.
		*explicit = false
		*injected = false
		return
	}
	raw := strings.Split(paramsStr, ";")
	params := make([]int, len(raw))
	for i, p := range raw {
		v, err := strconv.Atoi(p)
		if err != nil {
			return // malformed; leave state untouched
		}
		params[i] = v
	}
	for i := 0; i < len(params); i++ {
		p := params[i]
		switch {
		case p == 0, p == 49: // full reset, or default background
			*explicit = false
			*injected = false
		case p == 48: // set background (48;5;N or 48;2;r;g;b)
			*explicit = true
			*injected = false
			i += consumeColorSpec(params[i+1:])
		case p == 38: // set foreground (38;5;N or 38;2;r;g;b) — no bg effect
			i += consumeColorSpec(params[i+1:])
		case (p >= 40 && p <= 47) || (p >= 100 && p <= 107): // basic/bright bg
			*explicit = true
			*injected = false
		}
	}
}

// consumeColorSpec returns how many extra parameters a 38/48 colour spec
// occupies beyond the mode byte: 1 for "5;N" (256-colour), 3 for "2;r;g;b"
// (truecolour). Unknown modes consume just the mode byte.
func consumeColorSpec(rest []int) int {
	if len(rest) == 0 {
		return 0
	}
	switch rest[0] {
	case 5:
		return 2 // mode + index
	case 2:
		return 4 // mode + r + g + b
	default:
		return 1 // just the mode byte
	}
}

// sgrRE matches an SGR sequence (CSI … m), capturing the parameter list.
var sgrRE = regexp.MustCompile(`\x1b\[([0-9;]*)m`)

// ansiBgSeq returns the SGR "set background" sequence for a lipgloss colour,
// using lipgloss's own colour-profile conversion so the emitted sequence
// matches whatever the terminal supports (truecolour / 256 / 16-colour). A
// colour lipgloss can't resolve yields "" so the caller skips painting.
func ansiBgSeq(c lipgloss.Color) string {
	if string(c) == "" {
		return ""
	}
	// Render a sentinel rune with this background; lipgloss emits the
	// profile-appropriate set-background sequence before it. '|' never appears
	// inside an SGR sequence, so the prefix up to it is exactly the bg code.
	styled := lipgloss.NewStyle().Background(c).Render("|")
	idx := strings.IndexByte(styled, '|')
	if idx <= 0 {
		return ""
	}
	return styled[:idx]
}

// paintBg applies the theme background to a view unless the user has opted into
// a transparent background via config.
func (m Model) paintBg(view string) string {
	if m.settings.TransparentBackground {
		return view
	}
	return paintBackground(view, colorBg)
}
