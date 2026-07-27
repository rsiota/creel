package ui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

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
	colorVisual = p.visual
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
	primary := lipgloss.NewStyle().Foreground(colorPrimary)
	parts := strings.SplitN(prompt, "\n", 2)
	promptLines := []string{primary.Render(parts[0])}
	if len(parts) > 1 {
		promptLines = append(promptLines, mutedStyle.Render(parts[1]))
	}
	lines := []string{lipgloss.JoinVertical(lipgloss.Center, promptLines...)}
	if showFooter {
		lines = append(lines, "",
			lipgloss.NewStyle().Foreground(colorLabel).Render("y")+mutedStyle.Render(" confirm    ")+
				lipgloss.NewStyle().Foreground(colorLabel).Render("n")+mutedStyle.Render(" cancel"))
	}
	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
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
		Border(lipgloss.RoundedBorder()).
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
		Border(lipgloss.RoundedBorder()).
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
		Border(lipgloss.RoundedBorder()).
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
		Border(lipgloss.RoundedBorder()).
		BorderForeground(colorPrimary).
		Padding(1, 3).
		Render(content)
}

// renderSQLConfirmDialog builds a confirmation overlay that includes SQL preview.
func renderSQLConfirmDialog(prompt, sql string) string {
	sqlStyled := lipgloss.NewStyle().Foreground(colorLabel).Render(sql)
	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
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
