package ui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/rsiota/creel/internal/config"
)

// themeBg returns the set-background sequence the pass emits for a hex colour,
// computed the same way paintBackground does (via lipgloss), so assertions are
// independent of the active colour profile.
func themeBg(t *testing.T, hex string) string {
	t.Helper()
	seq := ansiBgSeq(lipgloss.Color(hex))
	if seq == "" {
		t.Skipf("colour profile renders no background sequence for %s", hex)
	}
	return seq
}

func TestAnsiBgSeqLightTheme(t *testing.T) {
	seq := ansiBgSeq(lipgloss.Color("#ffffff"))
	if seq != "\x1b[48;2;255;255;255m" {
		t.Fatalf("#ffffff seq=%q want truecolor white", seq)
	}
	seq = ansiBgSeq(lipgloss.Color("#1a1b26"))
	if seq != "\x1b[48;2;26;27;38m" {
		t.Fatalf("#1a1b26 seq=%q want truecolor", seq)
	}
}

func TestPaintBackgroundReinjectsAfterNewline(t *testing.T) {
	bg := themeBg(t, "#ffffff")
	got := paintBackground("a\nb", "#ffffff")
	want := bg + "a\n" + bg + "b"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestPaintBackgroundFillsPlainCells(t *testing.T) {
	bg := themeBg(t, "#1a1b26")
	got := paintBackground("hello", "#1a1b26")
	want := bg + "hello"
	if got != want {
		t.Errorf("plain text not filled:\n got %q\nwant %q", got, want)
	}
}

func TestPaintBackgroundNoOpForNonHex(t *testing.T) {
	// A colour lipgloss can't resolve is a safe no-op (view unchanged).
	in := "hello"
	if got := paintBackground(in, lipgloss.Color("")); got != in {
		t.Errorf("empty colour should be a no-op, got %q", got)
	}
}

func TestPaintBackgroundPreservesExplicitBg(t *testing.T) {
	bg := themeBg(t, "#1a1b26")
	// A cell with its own explicit background keeps it; only the default-bg
	// cells around it get the theme bg.
	explicit := lipgloss.NewStyle().Background(lipgloss.Color("#ff0000")).Render("X")
	got := paintBackground("a"+explicit+"b", "#1a1b26")

	// 'a' (default bg) -> theme bg; "X" -> keeps its explicit bg; after the
	// style's trailing reset, 'b' (default bg) -> theme bg again.
	if !strings.HasPrefix(got, bg+"a") {
		t.Errorf("leading default cell not painted: %q", got)
	}
	if !strings.Contains(got, explicit) {
		t.Errorf("explicit bg not preserved: %q", got)
	}
	if !strings.HasSuffix(got, bg+"b") {
		t.Errorf("cell after reset not re-painted: %q", got)
	}
}

func TestPaintBackgroundTruecolorZeroNotMistakenForReset(t *testing.T) {
	// A pure-black background (#000000 -> "48;2;0;0;0") must not be treated
	// as a reset: its cell should keep the explicit (black) bg, not be
	// re-painted with the theme bg.
	bg := themeBg(t, "#1a1b26")
	black := "\x1b[48;2;0;0;0mX\x1b[0m"
	got := paintBackground(black, "#1a1b26")
	if strings.Contains(got, bg+"X") {
		t.Errorf("zero-valued truecolor bg was overridden by theme bg: %q", got)
	}
	if !strings.Contains(got, "\x1b[48;2;0;0;0mX") {
		t.Errorf("explicit black bg not preserved: %q", got)
	}
}

func TestPaintBackgroundResetReturnsToThemeBg(t *testing.T) {
	// A reset (0) clears an explicit bg; subsequent default cells get theme bg.
	bg := themeBg(t, "#1a1b26")
	colored := "\x1b[48;2;0;128;0mX\x1b[0m"
	got := paintBackground(colored+" rest", "#1a1b26")
	if !strings.Contains(got, "\x1b[0m"+bg+" rest") {
		t.Errorf("default bg not restored after reset: %q", got)
	}
}

func TestPaintBackgroundBasicBgCodeRecognized(t *testing.T) {
	// A 16-colour background code (40-47 / 100-107) must count as explicit so
	// it isn't overwritten by the theme bg. lipgloss emits these under a
	// downgraded colour profile.
	bg := themeBg(t, "#1a1b26")
	brightRed := "\x1b[101mX\x1b[0m"
	got := paintBackground(brightRed, "#1a1b26")
	if strings.Contains(got, bg+"X") {
		t.Errorf("16-colour bg was overridden by theme bg: %q", got)
	}
	if !strings.Contains(got, brightRed) {
		t.Errorf("16-colour bg not preserved: %q", got)
	}
}

func TestPaintBackgroundSpacesPadded(t *testing.T) {
	// Padding spaces (>= 0x20) receive the theme bg; the inject is emitted
	// once and stays active across them.
	bg := themeBg(t, "#1a1b26")
	got := paintBackground("a b", "#1a1b26")
	if strings.Count(got, bg) != 1 {
		t.Errorf("expected a single theme-bg inject, got %q", got)
	}
	if !strings.HasSuffix(got, "a b") {
		t.Errorf("content altered: %q", got)
	}
}

func TestPadViewHeightFillsWithSpaces(t *testing.T) {
	applyPalette(themes["git-hub-light-default"])
	got := padViewHeight("line", 5, 3)
	lines := strings.Split(got, "\n")
	if len(lines) != 3 {
		t.Fatalf("got %d lines, want 3", len(lines))
	}
	if lines[0] != "line" {
		t.Errorf("line 0 = %q, want line", lines[0])
	}
	if strings.TrimSpace(ansi.Strip(lines[1])) != "" || strings.TrimSpace(ansi.Strip(lines[2])) != "" {
		t.Errorf("padding lines should be blank spaces with bg: %q, %q", lines[1], lines[2])
	}
}

func TestPaintBackgroundFillsSpaceCanvas(t *testing.T) {
	bg := themeBg(t, "#ffffff")
	canvas := lipgloss.Place(8, 2, lipgloss.Left, lipgloss.Top, "",
		lipgloss.WithWhitespaceChars(" "))
	got := paintBackground(canvas, "#ffffff")
	if !strings.Contains(got, bg) {
		t.Fatalf("space canvas not painted with theme bg")
	}
}

func TestPaintBackgroundSkipsBareNewlineCanvas(t *testing.T) {
	bg := themeBg(t, "#ffffff")
	canvas := "\n\n"
	got := paintBackground(canvas, "#ffffff")
	if strings.Contains(got, bg) {
		t.Fatalf("bare newline canvas should stay unpainted, got %q", got)
	}
}

func TestPaintBgRespectsTransparentSetting(t *testing.T) {
	defer applyPalette(defaultPalette)
	applyPalette(defaultPalette)

	// With TransparentBackground set, paintBg is a no-op.
	m := Model{settings: config.Settings{TransparentBackground: true}}
	in := "hello"
	if got := m.paintBg(in); got != in {
		t.Errorf("transparent background should be a no-op, got %q", got)
	}
	// With it off, the view is painted.
	m2 := Model{settings: config.Settings{TransparentBackground: false}}
	if got := m2.paintBg(in); !strings.HasPrefix(got, "\x1b[") {
		t.Errorf("non-transparent background should paint, got %q", got)
	}
}
