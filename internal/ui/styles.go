package ui

import (
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// panelBorder is the single source of truth for every panel's border, so the
// corner style can be changed app-wide in one place. Square (NormalBorder)
// gives the terminal-native, data-tool feel and renders consistently across
// fonts and terminals; swap to lipgloss.RoundedBorder() to try the softer,
// rounded look.
func panelBorder() lipgloss.Border {
	return lipgloss.NormalBorder()
}

// colorPalette is the full set of colors a theme provides. Every package-level
// color var and derived style below is (re)built from the active palette via
// applyPalette, so switching themes is a single call that propagates to every
// renderer on the next View() pass — no per-component plumbing required.
type colorPalette struct {
	primary         lipgloss.Color
	accent          lipgloss.Color
	success         lipgloss.Color
	mark            lipgloss.Color
	search          lipgloss.Color
	searchMatch     lipgloss.Color
	visual          lipgloss.Color
	cursorRow       lipgloss.Color
	edit            lipgloss.Color
	warn            lipgloss.Color
	err             lipgloss.Color
	muted           lipgloss.Color
	label           lipgloss.Color
	border          lipgloss.Color
	borderUnfocused lipgloss.Color
	bg              lipgloss.Color
	stripe          lipgloss.Color
	fg              lipgloss.Color
	highlight       lipgloss.Color
	statusBarBg     lipgloss.Color
	// fk is optional: when empty, applyPalette derives a soft primary→bg tint
	// for FK result cells (headers stay the normal primary style).
	fk lipgloss.Color
}

// defaultPalette is the Tokyo Night–inspired palette shipped as the default
// theme. Additional named palettes (gruvbox, nord, …) will be added alongside
// it as part of the theming roadmap.
var defaultPalette = colorPalette{
	primary:         lipgloss.Color("#7aa2f7"),
	accent:          lipgloss.Color("#bb9af7"),
	success:         lipgloss.Color("#9ece6a"),
	mark:            lipgloss.Color("#73daca"),
	search:          lipgloss.Color("#4c4c6e"),
	searchMatch:     lipgloss.Color("#5e81ac"), // Nord nord10 — punchy mid-blue for non-current search matches
	visual:          lipgloss.Color("#283457"),
	cursorRow:       lipgloss.Color("#3B4252"), // Nord nord1 — ambient cursor-row tint
	edit:            lipgloss.Color("#ff9e64"),
	warn:            lipgloss.Color("#e0af68"),
	err:             lipgloss.Color("#f7768e"),
	muted:           lipgloss.Color("#565f89"),
	label:           lipgloss.Color("#8089ab"),
	border:          lipgloss.Color("#3b4261"),
	borderUnfocused: lipgloss.Color("#5e6686"), // midpoint between border and label
	bg:              lipgloss.Color("#1a1b26"),
	stripe:          lipgloss.Color("#323946"), // subtle zebra tint: ~35% between bg and cursorRow
	fg:              lipgloss.Color("#c0caf5"),
	highlight:       lipgloss.Color("#292e42"),
	statusBarBg:     lipgloss.Color("#343B49"), // midpoint between bg and cursorRow
}

// Color palette — these are the only color values referenced elsewhere in the
// package. They carry no initializers: applyPalette (called from init with
// defaultPalette, and again on theme switch) assigns them all.
var (
	colorPrimary         lipgloss.Color
	colorAccent          lipgloss.Color
	colorSuccess         lipgloss.Color
	colorMark            lipgloss.Color
	colorSearch          lipgloss.Color
	colorSearchMatch     lipgloss.Color
	// Wash behind visual-mode rows and marked columns. May be strengthened
	// from the theme's raw visual when that tint is too faint on light bgs.
	colorVisual          lipgloss.Color
	colorCursorRow       lipgloss.Color
	colorEdit            lipgloss.Color
	colorWarn            lipgloss.Color
	colorError           lipgloss.Color
	colorMuted           lipgloss.Color
	colorLabel           lipgloss.Color
	colorBorder          lipgloss.Color
	colorBorderUnfocused lipgloss.Color
	colorBg              lipgloss.Color
	colorStripe          lipgloss.Color
	colorFg              lipgloss.Color
	colorHighlight       lipgloss.Color
	colorStatusBarBg     lipgloss.Color
	colorFK              lipgloss.Color // foreign-key cell cue (headers stay primary)
	// Soft status-cell hues (semantic colors blended toward bg, like colorFK).
	colorStatusOK     lipgloss.Color
	colorStatusWarn   lipgloss.Color
	colorStatusInfo   lipgloss.Color
	colorStatusBad    lipgloss.Color
	colorStatusQuiet  lipgloss.Color
	// Boolean ●/○ glyphs: default fg softened toward bg (~70% strength).
	colorBool lipgloss.Color
	// Soft accent wash behind unsaved (dirty) result cells — mark-row strength
	// so edits read clearly on light themes, purple/accent hue so dirty stays
	// distinct from blue visual selection and teal space marks.
	colorDirty lipgloss.Color
	// Soft error→bg wash behind connection-form fields that failed ctrl+t
	// (fail-only chrome; OK fields stay neutral aside from the ✓ marker).
	colorTestFailWash lipgloss.Color
	// Soft mark→bg wash behind space-marked result rows so the selection reads
	// as a row tint (not just teal text / ◆), especially on near-white themes.
	colorMarkRow lipgloss.Color
	// ERD selection chrome: synthesized so vivid (selected card / hot arrows)
	// and dim (faded cards / idle arrows) keep a contrast gap on every theme.
	colorERDVivid lipgloss.Color
	colorERDDim   lipgloss.Color
	// Overlay backdrop: a touch more faded than ERD dim — content behind
	// popups need not be readable, only recognisable as “there”.
	colorOverlayDim lipgloss.Color
	// Soft secondary text for intentionally recessed content (AI reasoning /
	// "thinking…"). On dark themes muted already reads dim; on light themes
	// muted is floored to WCAG AA and looks nearly as strong as body text, so
	// applyPalette synthesizes a softer wash that matches the dark-theme feel.
	colorThinking lipgloss.Color
)

// sbStyles are status-bar-specific styles that carry the status bar background
// so that ANSI resets within multi-segment rendered strings don't lose the bg.
// Declared here, assigned by applyPalette.
var (
	sbMuted   lipgloss.Style
	sbSuccess lipgloss.Style
	sbError   lipgloss.Style
	sbPrimary lipgloss.Style
	sbLabel   lipgloss.Style
	sbAccent  lipgloss.Style
	sbMark    lipgloss.Style
	sbFg      lipgloss.Style
	sbHintFlash lipgloss.Style // pressed key: cell fg + bold
)

// Shared styles used across components. Declared here, assigned by applyPalette.
var (
	appStyle           lipgloss.Style
	titleStyle         lipgloss.Style
	selectedStyle      lipgloss.Style
	panelSelectedStyle lipgloss.Style
	normalStyle        lipgloss.Style
	mutedStyle         lipgloss.Style
	errorStyle         lipgloss.Style
	slowStyle          lipgloss.Style
	successStyle       lipgloss.Style
	borderStyle        lipgloss.Style
)

func init() {
	applyPalette(defaultPalette)
}

// applyPalette sets every package-level color var from p and rebuilds the
// derived styles that capture those colors at construction — the sb*, shared,
// and tab-bar styles declared in this file, plus the SQL-highlight styles in
// sql_highlight.go.
//
// Because Bubble Tea re-runs View() on every frame, calling applyPalette is
// all it takes to re-theme the live UI. Today only defaultPalette is applied
// (at init); a later step will call this from a theme picker for live preview.
func applyPalette(p colorPalette) {
	colorPrimary = p.primary
	colorAccent = p.accent
	colorSuccess = p.success
	colorMark = p.mark
	colorSearch = p.search
	colorSearchMatch = p.searchMatch
	colorCursorRow = p.cursorRow
	colorEdit = p.edit
	colorWarn = p.warn
	colorError = p.err
	colorMuted = p.muted
	colorLabel = p.label
	colorBorder = p.border
	colorBorderUnfocused = p.borderUnfocused
	colorBg = p.bg
	colorStripe = p.stripe
	colorFg = p.fg
	colorHighlight = p.highlight
	colorStatusBarBg = p.statusBarBg
	// FK cells only: primary blue darkened toward bg so it sits clearly
	// below ordinary fg text. Headers stay primary; PK columns stay
	// unstyled aside from the existing * marker.
	colorFK = p.fk
	if colorFK == "" {
		colorFK = mixColors(p.primary, p.bg, 0.30)
	}
	// Status enum cells use the same soft blend so they match FK weight.
	const statusBlend = 0.30
	colorStatusOK = mixColors(p.success, p.bg, statusBlend)
	colorStatusWarn = mixColors(p.warn, p.bg, statusBlend)
	colorStatusInfo = mixColors(p.accent, p.bg, statusBlend)
	colorStatusBad = mixColors(p.err, p.bg, statusBlend)
	colorStatusQuiet = mixColors(p.muted, p.bg, statusBlend)
	// Boolean glyphs: same hue as cell text, ~70% strength (30% toward bg).
	colorBool = mixColors(p.fg, p.bg, 0.30)
	colorDirty = deriveDirtyBg(p)
	colorTestFailWash = deriveTintWash(p.err, p)
	colorMarkRow = deriveMarkRowBg(p)
	// Visual / marked-column wash: keep theme visual when it already pops;
	// otherwise a primary→bg wash at mark-row strength (blue selection, not
	// teal marks — so V and space marks stay distinguishable on light themes).
	colorVisual = deriveVisualRowBg(p)

	// ERD selection: vivid for the selected card + arrows that touch it; dim
	// for everything else. Reusing muted/primary collapses on light themes
	// (similar luminance); derive a pair with an explicit contrast gap.
	colorERDVivid, colorERDDim = deriveERDColors(p)
	// Popup backdrop: same wash helper, softer than ERD dim (legibility not needed).
	colorOverlayDim = erdDimWash(p.bg, p.fg, overlayDimMinBgContrast)
	colorThinking = deriveThinkingColor(p)

	// Status-bar styles (carry the status-bar bg so ANSI resets within
	// multi-segment rendered strings don't lose it).
	sbMuted = lipgloss.NewStyle().Foreground(colorMuted).Background(colorStatusBarBg)
	sbSuccess = lipgloss.NewStyle().Foreground(colorSuccess).Background(colorStatusBarBg)
	sbError = lipgloss.NewStyle().Foreground(colorError).Background(colorStatusBarBg)
	sbPrimary = lipgloss.NewStyle().Foreground(colorPrimary).Background(colorStatusBarBg)
	sbLabel = lipgloss.NewStyle().Foreground(colorLabel).Background(colorStatusBarBg)
	sbAccent = lipgloss.NewStyle().Foreground(colorAccent).Background(colorStatusBarBg)
	sbMark = lipgloss.NewStyle().Foreground(colorMark).Background(colorStatusBarBg)
	sbFg = lipgloss.NewStyle().Foreground(colorFg).Background(colorStatusBarBg)
	// Idle hints use muted; flash jumps to cell fg + bold so the pressed key
	// matches result-cell text and stands out against the dimmer idle set.
	sbHintFlash = lipgloss.NewStyle().Foreground(colorFg).Background(colorStatusBarBg).Bold(true)

	// Shared styles.
	appStyle = lipgloss.NewStyle().Padding(0, 1)
	titleStyle = lipgloss.NewStyle().
		Foreground(colorPrimary).
		Bold(true).
		Padding(0, 1)
	selectedStyle = lipgloss.NewStyle().
		Foreground(colorBg).
		Background(colorPrimary).
		Padding(0, 1)
	panelSelectedStyle = lipgloss.NewStyle().
		Foreground(colorPrimary).
		Bold(true).
		Padding(0, 1)
	normalStyle = lipgloss.NewStyle().
		Foreground(colorFg).
		Padding(0, 1)
	mutedStyle = lipgloss.NewStyle().
		Foreground(colorMuted)
	// slowStyle highlights notably slow query durations inline. It mirrors
	// mutedStyle but in the error colour so a slow elapsed stands out in the
	// history list without the padding normalStyle/errorStyle add (which would
	// misalign columns).
	slowStyle = lipgloss.NewStyle().
		Foreground(colorError)
	errorStyle = lipgloss.NewStyle().
		Foreground(colorError).
		Padding(0, 1)
	successStyle = lipgloss.NewStyle().
		Foreground(colorSuccess).
		Padding(0, 1)
	borderStyle = lipgloss.NewStyle().
		BorderForeground(colorBorder)

	// Tab bar styles — fixed width so every tab occupies the same space.
	// Inactive tabs carry no background tint; their text uses the same grey
	// (colorLabel) as the database name on the status bar, shifting to white
	// (colorFg) when the tab bar is focused so the user can tell the panel is
	// active even when looking at an inactive tab.
	activeTabStyle = lipgloss.NewStyle().
		Foreground(colorBg).
		Background(colorPrimary).
		Padding(0, 1).
		Width(tabWidth).
		Align(lipgloss.Center)
	activeTabStyleFocused = lipgloss.NewStyle().
		Foreground(colorBg).
		Background(colorPrimary).
		Padding(0, 1).
		Width(tabWidth).
		Align(lipgloss.Center).
		Bold(true)
	inactiveTabStyle = lipgloss.NewStyle().
		Foreground(colorLabel).
		Padding(0, 1).
		Width(tabWidth).
		Align(lipgloss.Center)
	inactiveTabStyleFocused = lipgloss.NewStyle().
		Foreground(colorFg).
		Padding(0, 1).
		Width(tabWidth).
		Align(lipgloss.Center)

	// SQL highlight styles (declared in sql_highlight.go) also capture colors
	// at construction, so rebuild them from the now-updated palette.
	rebuildSQLHighlightStyles()
}

// mixColors linearly interpolates between a and b by t (0 = a, 1 = b).
// Non-hex inputs return a unchanged.
func mixColors(a, b lipgloss.Color, t float64) lipgloss.Color {
	ar, ag, ab, okA := parseHexRGB(string(a))
	br, bg, bb, okB := parseHexRGB(string(b))
	if !okA || !okB {
		return a
	}
	if t < 0 {
		t = 0
	} else if t > 1 {
		t = 1
	}
	lerp := func(x, y int) int {
		return int(math.Round(float64(x)*(1-t) + float64(y)*t))
	}
	return lipgloss.Color(fmt.Sprintf("#%02x%02x%02x", lerp(ar, br), lerp(ag, bg), lerp(ab, bb)))
}

func parseHexRGB(s string) (r, g, b int, ok bool) {
	s = strings.TrimPrefix(strings.TrimSpace(s), "#")
	if len(s) != 6 {
		return 0, 0, 0, false
	}
	n, err := strconv.ParseUint(s, 16, 32)
	if err != nil {
		return 0, 0, 0, false
	}
	return int(n>>16) & 0xff, int(n>>8) & 0xff, int(n) & 0xff, true
}

// relLuminance returns the WCAG relative luminance of a hex color like "#7aa2f7".
func relLuminance(hex string) float64 {
	r, g, b, ok := parseHexRGB(hex)
	if !ok {
		return 0
	}
	channel := func(c int) float64 {
		v := float64(c) / 255
		if v <= 0.03928 {
			return v / 12.92
		}
		return math.Pow((v+0.055)/1.055, 2.4)
	}
	return 0.2126*channel(r) + 0.7152*channel(g) + 0.0722*channel(b)
}

// contrastRatio returns the WCAG contrast ratio between two hex colors.
func contrastRatio(a, b string) float64 {
	la := relLuminance(a)
	lb := relLuminance(b)
	if la < lb {
		la, lb = lb, la
	}
	return (la + 0.05) / (lb + 0.05)
}

// erdVividDimMinGap is the minimum contrast between ERD vivid and dim chrome
// so selection still reads. Kept modest so dimmed cards can stay legible.
const erdVividDimMinGap = 1.5

// erdDimMinBgContrast is the target for dimmed card text against the theme bg.
// ~2.2 keeps faded cards lightly readable while clearly softer than selected
// chrome (3.0 felt too strong; ~2.5 was almost there).
const erdDimMinBgContrast = 2.2

// overlayDimMinBgContrast is the target for content behind long-lived popups.
// Softer than ERD dim (~2.2): the backdrop only needs to read as recessed, not
// stay readable. 1.7 is a mild step down without collapsing into the bg.
const overlayDimMinBgContrast = 1.7

// thinkingDimMinBgContrast is the target for AI reasoning / "thinking…" text
// on light themes. Softer than AA-strong muted (~6) so chain-of-thought stays
// recessed next to the answer, but strong enough to read comfortably (~2.5).
const thinkingDimMinBgContrast = 2.5

// markRowBgBlend is the preferred tint→bg wash for mark / visual / dirty
// backgrounds when both bg distinction and fg readability can be met.
const markRowBgBlend = 0.78

// markRowBgMinContrast floors selection/edit washes vs panel bg when possible
// (GitHub Light needs a clearer step than near-white tints alone provided).
const markRowBgMinContrast = 1.12

// deriveDirtyBg builds the soft accent wash behind unsaved result cells.
// Same strength search as mark/visual rows, but tinted from accent so dirty
// (purple) stays distinct from visual (blue) and marks (teal).
func deriveDirtyBg(p colorPalette) lipgloss.Color {
	return deriveTintWash(p.accent, p)
}

// deriveMarkRowBg builds the soft mark wash behind space-marked result rows.
// Prefer a clear tint vs the panel bg, but never so strong that body text
// drops below WCAG AA (some themes have a low-contrast mark hue).
func deriveMarkRowBg(p colorPalette) lipgloss.Color {
	return deriveTintWash(p.mark, p)
}

// deriveVisualRowBg picks the wash behind visual-mode rows and marked columns.
// Keep the theme's visual when it already separates from the panel bg; faint
// light-theme selectionBackgrounds (GitHub Light #f0f6fd on white) get a
// primary→bg wash at mark-row strength so V pops without sharing the teal
// mark hue — selection (blue) stays distinct from space marks (teal).
func deriveVisualRowBg(p colorPalette) lipgloss.Color {
	if contrastRatio(string(p.visual), string(p.fg)) >= 4.5 &&
		contrastRatio(string(p.visual), string(p.bg)) >= markRowBgMinContrast {
		return p.visual
	}
	return deriveTintWash(p.primary, p)
}

// deriveTintWash mixes tint toward bg, picking the blend with the strongest
// bg distinction that still keeps body text at WCAG AA.
func deriveTintWash(tint lipgloss.Color, p colorPalette) lipgloss.Color {
	best := mixColors(tint, p.bg, markRowBgBlend)
	bestVsBg := -1.0
	for _, t := range []float64{
		0.65, 0.70, 0.75, 0.78, 0.82, 0.85, 0.88, 0.90, 0.93, 0.95, 0.97, 0.98, 0.99,
	} {
		cand := mixColors(tint, p.bg, t)
		if contrastRatio(string(cand), string(p.fg)) < 4.5 {
			continue
		}
		vsBg := contrastRatio(string(cand), string(p.bg))
		if vsBg > bestVsBg {
			bestVsBg = vsBg
			best = cand
		}
	}
	if bestVsBg >= 0 {
		return best
	}
	// Rare themes where tint/fg can't coexist as a wash: fall back to an
	// existing soft panel tint that keeps AA.
	for _, cand := range []lipgloss.Color{p.cursorRow, p.highlight, p.stripe, p.bg} {
		if contrastRatio(string(cand), string(p.fg)) >= 4.5 {
			return cand
		}
	}
	return p.bg
}

// deriveThinkingColor picks the foreground for AI chain-of-thought and the
// pending "thinking…" label. Dark themes keep muted (already recessed); light
// themes get a soft bg→fg wash so reasoning doesn't compete with the answer.
func deriveThinkingColor(p colorPalette) lipgloss.Color {
	if relLuminance(string(p.bg)) > 0.4 {
		return erdDimWash(p.bg, p.fg, thinkingDimMinBgContrast)
	}
	return p.muted
}

// deriveERDColors builds the ERD selection pair from a palette. Dim is chosen
// for readability on bg first; vivid (from primary) is then strengthened toward
// fg if needed so the vivid/dim gap clears. That way faded cards stay legible
// and selection still pops — the reverse of fading dim into invisibility.
func deriveERDColors(p colorPalette) (vivid, dim lipgloss.Color) {
	if relLuminance(string(p.bg)) > 0.4 {
		dim = erdDimWash(p.bg, p.fg, erdDimMinBgContrast)
	} else if contrastRatio(string(p.muted), string(p.bg)) >= erdDimMinBgContrast*0.85 {
		dim = p.muted
	} else {
		dim = erdDimWash(p.bg, p.fg, erdDimMinBgContrast)
	}
	vivid = erdVividAgainst(p.primary, p.fg, p.bg, dim)
	if string(vivid) == string(dim) || contrastRatio(string(vivid), string(dim)) < 1.05 {
		// Pathological schemes where primary≈muted: fall back to accent, then fg.
		for _, cand := range []lipgloss.Color{p.accent, p.fg, mixColors(p.primary, p.fg, 0.5)} {
			if contrastRatio(string(cand), string(dim)) >= 1.05 {
				vivid = cand
				break
			}
		}
	}
	return vivid, dim
}

// erdDimWash mixes bg toward fg to land near targetContrast against bg.
// Range covers both readable ERD dim (~2.2) and softer overlay dim (~1.7).
func erdDimWash(bg, fg lipgloss.Color, targetContrast float64) lipgloss.Color {
	best := mixColors(bg, fg, 0.40)
	bestDiff := absFloat(contrastRatio(string(best), string(bg)) - targetContrast)
	for t := 0.18; t <= 0.65; t += 0.02 {
		cand := mixColors(bg, fg, t)
		diff := absFloat(contrastRatio(string(cand), string(bg)) - targetContrast)
		if diff < bestDiff {
			best, bestDiff = cand, diff
		}
	}
	return best
}

// erdVividAgainst starts from primary and mixes toward fg until it gaps from
// dim and reads on bg. Returns the best effort if both floors can't be met.
func erdVividAgainst(primary, fg, bg, dim lipgloss.Color) lipgloss.Color {
	best := primary
	bestScore := -1.0
	for t := 0.0; t <= 1.0; t += 0.1 {
		cand := primary
		if t > 0 {
			cand = mixColors(primary, fg, t)
		}
		gap := contrastRatio(string(cand), string(dim))
		vsBg := contrastRatio(string(cand), string(bg))
		score := gap + vsBg*0.25
		if gap >= erdVividDimMinGap {
			score += 50
		}
		if vsBg >= 3.0 {
			score += 20
		}
		if score > bestScore {
			best, bestScore = cand, score
		}
		if gap >= erdVividDimMinGap && vsBg >= 3.0 {
			return cand
		}
	}
	return best
}

func absFloat(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}

// Tab bar styles — declared here, assigned by applyPalette.
var (
	activeTabStyle          lipgloss.Style
	activeTabStyleFocused   lipgloss.Style
	inactiveTabStyle        lipgloss.Style
	inactiveTabStyleFocused lipgloss.Style
)

// renderConfirmDialog builds a centered y/n confirmation overlay.
func renderConfirmDialog(prompt string) string {
	return renderConfirmDialogFooter(prompt, true)
}

// renderConfirmDialogBare builds a centered confirmation overlay without the
// keybinding footer.
func renderConfirmDialogBare(prompt string) string {
	return renderConfirmDialogFooter(prompt, false)
}

func renderConfirmDialogFooter(prompt string, showFooter bool) string {
	// Fixed content width (dialog Width 46 minus border 2 and padding 3*2).
	const contentW = 40
	primary := lipgloss.NewStyle().Width(contentW).Align(lipgloss.Center).Foreground(colorPrimary)
	muted := lipgloss.NewStyle().Width(contentW).Align(lipgloss.Center).Foreground(colorMuted)
	parts := strings.SplitN(prompt, "\n", 2)
	promptLines := []string{primary.Render(parts[0])}
	if len(parts) > 1 {
		promptLines = append(promptLines, muted.Render(parts[1]))
	}
	lines := []string{lipgloss.JoinVertical(lipgloss.Center, promptLines...)}
	if showFooter {
		footer := lipgloss.NewStyle().Width(contentW).Align(lipgloss.Center).Render(
			lipgloss.NewStyle().Foreground(colorLabel).Render("y") + mutedStyle.Render(" confirm    ") +
				lipgloss.NewStyle().Foreground(colorLabel).Render("n") + mutedStyle.Render(" cancel"),
		)
		lines = append(lines, "", footer)
	}
	return lipgloss.NewStyle().
		Border(panelBorder()).
		BorderForeground(colorPrimary).
		Padding(1, 3).
		Width(46).
		Align(lipgloss.Center).
		Render(lipgloss.JoinVertical(lipgloss.Center, lines...))
}

// renderTypedConfirmDialog builds a destructive-action overlay that requires
// the user to type an exact value (e.g. a table name) before it will proceed.
// renderTypedConfirmDialog builds a confirmation overlay requiring the user to
// type an exact string. width/height are the desired TOTAL dimensions (including
// border and padding); pass 0 for height for auto-sizing. When width/height
// match a triggering modal (e.g. the database picker), the dialog replaces it
// at the same size instead of appearing as a smaller box on top.
func renderTypedConfirmDialog(prompt, hint, input string, width, height int) string {
	// Content width = total width - border(2) - horizontal padding(3*2=6)
	contentW := width - 2 - 6
	if contentW < 30 {
		contentW = 30
	}
	promptBlock := lipgloss.NewStyle().
		Width(contentW).Align(lipgloss.Center).Foreground(colorPrimary).
		Render(prompt)
	hintLine := lipgloss.NewStyle().
		Width(contentW).Align(lipgloss.Center).Foreground(colorMuted).
		Render("Type " + hint + " to confirm:")
	inputLine := lipgloss.NewStyle().
		Width(contentW).Align(lipgloss.Center).Foreground(colorPrimary).
		Render("> " + input + "_")
	content := lipgloss.JoinVertical(lipgloss.Center,
		promptBlock, "", hintLine, inputLine,
	)
	style := lipgloss.NewStyle().
		Border(panelBorder()).
		BorderForeground(colorPrimary).
		Padding(1, 3)
	if width > 0 {
		// Width includes padding but not border; subtract border to match total.
		style = style.Width(width - 2)
	}
	if height > 0 {
		// Height includes padding but not border; subtract border to match total.
		style = style.Height(height - 2)
	}
	return style.Render(content)
}

// renderInputDialogBare builds a compact name-entry overlay without a keybinding
// footer. It auto-sizes to fit the content. Used for small popups like the
// create-database dialog.
func renderInputDialogBare(prompt, input, errMsg string) string {
	promptBlock := lipgloss.NewStyle().Foreground(colorPrimary).Render(prompt)
	inputLine := lipgloss.NewStyle().Foreground(colorPrimary).Render("> " + input + "_")

	lines := []string{promptBlock, "", inputLine}

	if errMsg != "" {
		lines = append(lines, "",
			lipgloss.NewStyle().Foreground(colorError).Render(errMsg))
	}

	content := lipgloss.JoinVertical(lipgloss.Center, lines...)
	return lipgloss.NewStyle().
		Border(panelBorder()).
		BorderForeground(colorPrimary).
		Padding(1, 3).
		Render(content)
}

// renderInputDialog builds a simple name-entry overlay for operations like
// creating a database. It shows a prompt, an input field, an optional error
// line, and a footer with key hints. width/height are the desired TOTAL
// dimensions (including border and padding), matching popupDim when needed.
func renderInputDialog(prompt, input, errMsg string, width, height int) string {
	contentW := width - 2 - 6
	if contentW < 30 {
		contentW = 30
	}
	promptBlock := lipgloss.NewStyle().
		Width(contentW).Align(lipgloss.Center).Foreground(colorPrimary).
		Render(prompt)
	inputLine := lipgloss.NewStyle().
		Width(contentW).Align(lipgloss.Center).Foreground(colorPrimary).
		Render("> " + input + "_")

	lines := []string{promptBlock, "", inputLine}

	if errMsg != "" {
		lines = append(lines, "",
			lipgloss.NewStyle().
				Width(contentW).Align(lipgloss.Center).Foreground(colorError).
				Render(errMsg))
	}

	lines = append(lines, "",
		lipgloss.NewStyle().Width(contentW).Align(lipgloss.Center).Render(
			lipgloss.NewStyle().Foreground(colorLabel).Render("enter")+mutedStyle.Render(" confirm    ")+
				lipgloss.NewStyle().Foreground(colorLabel).Render("esc")+mutedStyle.Render(" cancel"),
		))

	content := lipgloss.JoinVertical(lipgloss.Center, lines...)
	style := lipgloss.NewStyle().
		Border(panelBorder()).
		BorderForeground(colorPrimary).
		Padding(1, 3)
	if width > 0 {
		style = style.Width(width - 2)
	}
	if height > 0 {
		style = style.Height(height - 2)
	}
	return style.Render(content)
}

// renderTypedConfirmDialogBare builds a compact typed-confirmation overlay
// without a keybinding footer. It auto-sizes to fit the content, matching
// the style of the drop-table popup.
func renderTypedConfirmDialogBare(prompt, hint, input string) string {
	promptLines := strings.SplitN(prompt, "\n", 2)
	styledLines := []string{lipgloss.NewStyle().Foreground(colorPrimary).Render(promptLines[0])}
	if len(promptLines) > 1 {
		styledLines = append(styledLines, mutedStyle.Render(promptLines[1]))
	}
	hintLine := mutedStyle.Render("Type " + hint + " to confirm:")
	inputLine := lipgloss.NewStyle().Foreground(colorPrimary).Render("> " + input + "_")
	content := lipgloss.JoinVertical(lipgloss.Center,
		lipgloss.JoinVertical(lipgloss.Center, styledLines...),
		"",
		hintLine,
		inputLine,
	)
	return lipgloss.NewStyle().
		Border(panelBorder()).
		BorderForeground(colorPrimary).
		Padding(1, 3).
		Render(content)
}

// renderSQLConfirmDialog builds a confirmation overlay that includes SQL preview.
func renderSQLConfirmDialog(prompt, sql string) string {
	sqlStyled := lipgloss.NewStyle().Foreground(colorLabel).Render(sql)
	return lipgloss.NewStyle().
		Border(panelBorder()).
		BorderForeground(colorPrimary).
		Padding(1, 3).
		Width(64).
		Render(
			lipgloss.JoinVertical(lipgloss.Left,
				lipgloss.NewStyle().Foreground(colorPrimary).Render(prompt),
				"",
				sqlStyled,
				"",
				lipgloss.NewStyle().Foreground(colorLabel).Render("y")+mutedStyle.Render(" run    ")+
					lipgloss.NewStyle().Foreground(colorLabel).Render("n")+mutedStyle.Render(" back"),
			),
		)
}
