package ui

import "github.com/charmbracelet/lipgloss"

// Color palette - Tokyo Night inspired.
var (
	colorPrimary   = lipgloss.Color("#7aa2f7")
	colorAccent    = lipgloss.Color("#bb9af7")
	colorSuccess   = lipgloss.Color("#9ece6a")
	colorMark      = lipgloss.Color("#73daca")
	colorSearch    = lipgloss.Color("#4c4c6e")
	colorVisual    = lipgloss.Color("#283457")
	colorEdit      = lipgloss.Color("#ff9e64")
	colorError     = lipgloss.Color("#f7768e")
	colorMuted     = lipgloss.Color("#565f89")
	colorLabel     = lipgloss.Color("#8089ab")
	colorBorder    = lipgloss.Color("#3b4261")
	colorBorderUnfocused = lipgloss.Color("#5e6686") // midpoint between colorBorder and colorLabel
	colorBg        = lipgloss.Color("#1a1b26")
	colorFg        = lipgloss.Color("#c0caf5")
	colorHighlight = lipgloss.Color("#292e42")
)

// Shared styles used across components.
var (
	appStyle = lipgloss.NewStyle().
			Padding(0, 1)

	titleStyle = lipgloss.NewStyle().
			Foreground(colorPrimary).
			Bold(true).
			Padding(0, 1)

	selectedStyle = lipgloss.NewStyle().
			Foreground(colorBg).
			Background(colorPrimary).
			Padding(0, 1)

	normalStyle = lipgloss.NewStyle().
			Foreground(colorFg).
			Padding(0, 1)

	mutedStyle = lipgloss.NewStyle().
			Foreground(colorMuted)

	errorStyle = lipgloss.NewStyle().
			Foreground(colorError).
			Padding(0, 1)

	successStyle = lipgloss.NewStyle().
			Foreground(colorSuccess).
			Padding(0, 1)

	borderStyle = lipgloss.NewStyle().
			BorderForeground(colorBorder)
)

// renderConfirmDialog builds a centered y/n confirmation overlay.
func renderConfirmDialog(prompt string) string {
	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(colorPrimary).
		Padding(1, 3).
		Width(46).
		Align(lipgloss.Center).
		Render(
			lipgloss.JoinVertical(lipgloss.Center,
				lipgloss.NewStyle().Foreground(colorPrimary).Render(prompt),
				"",
				lipgloss.NewStyle().Foreground(colorLabel).Render("y")+mutedStyle.Render(" confirm    ")+
					lipgloss.NewStyle().Foreground(colorLabel).Render("n")+mutedStyle.Render(" cancel"),
			),
		)
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
	footer := lipgloss.NewStyle().Width(contentW).Align(lipgloss.Center).Render(
		lipgloss.NewStyle().Foreground(colorLabel).Render("enter") + mutedStyle.Render(" confirm    ") +
			lipgloss.NewStyle().Foreground(colorLabel).Render("esc") + mutedStyle.Render(" cancel"),
	)
	content := lipgloss.JoinVertical(lipgloss.Center,
		promptBlock, "", hintLine, inputLine, "", footer,
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
			lipgloss.NewStyle().Foreground(colorLabel).Render("enter") + mutedStyle.Render(" confirm    ") +
				lipgloss.NewStyle().Foreground(colorLabel).Render("esc") + mutedStyle.Render(" cancel"),
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
