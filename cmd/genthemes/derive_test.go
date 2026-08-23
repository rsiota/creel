package main

import "testing"

func TestEnsureContrastKeepsGoodColor(t *testing.T) {
	bg := parseHex("#ffffff")
	fg := parseHex("#1f2328")
	ok := parseHex("#57606a") // GitHub Light brightBlack — already AA
	got := ensureContrast(ok, bg, fg, 4.5)
	if got != ok {
		t.Fatalf("ensureContrast changed a good color: got %s want %s", got.hex(), ok.hex())
	}
}

func TestEnsureContrastLiftsLightMuted(t *testing.T) {
	bg := parseHex("#ffffff")
	fg := parseHex("#1f2328")
	// Typical light-scheme brightBlack: fine as a border, illegible as text.
	bad := parseHex("#a1a6c5") // TokyoNight Day–like
	got := ensureContrast(bad, bg, fg, 4.5)
	if contrast(got, bg) < 4.5 {
		t.Fatalf("ensureContrast(%s) = %s contrast %.2f < 4.5", bad.hex(), got.hex(), contrast(got, bg))
	}
	if got == bad {
		t.Fatalf("ensureContrast left the illegible color unchanged")
	}
}

func TestEnsureBgContrastFixesInvertedSelection(t *testing.T) {
	bg := parseHex("#ffffff")
	fg := parseHex("#1f2328")
	primary := parseHex("#0969da")
	// GitHub Light selectionBackground == foreground → invisible marked columns.
	bad := parseHex("#1f2328")
	got := ensureBgContrast(bad, fg, bg, primary, 4.5)
	if contrast(got, fg) < 4.5 {
		t.Fatalf("ensureBgContrast(%s) = %s contrast vs fg %.2f < 4.5",
			bad.hex(), got.hex(), contrast(got, fg))
	}
	if got == bad {
		t.Fatal("ensureBgContrast left the inverted selection unchanged")
	}
}

func TestDeriveGitHubLightVisualReadable(t *testing.T) {
	s := scheme{
		Name:                "GitHub Light Default",
		Background:          "#ffffff",
		Foreground:          "#1f2328",
		SelectionBackground: "#1f2328",
		Black:               "#24292f",
		BrightBlack:         "#57606a",
		Red:                 "#cf222e",
		Green:               "#116329",
		Yellow:              "#4d2d00",
		Blue:                "#0969da",
		Purple:              "#8250df",
		Cyan:                "#1b7c83",
		White:               "#6e7781",
	}
	p := derive(s)
	if contrast(parseHex(p.visual), parseHex(p.fg)) < 4.5 {
		t.Errorf("visual %s vs fg %s contrast %.2f < 4.5",
			p.visual, p.fg, contrast(parseHex(p.visual), parseHex(p.fg)))
	}
	if p.visual == "#1f2328" {
		t.Errorf("visual stayed as inverted selectionBackground %s", p.visual)
	}
}
