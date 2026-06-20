package ui

import "github.com/charmbracelet/lipgloss"

// Color palette - Tokyo Night inspired.
var (
	colorPrimary   = lipgloss.Color("#7aa2f7")
	colorAccent    = lipgloss.Color("#bb9af7")
	colorSuccess   = lipgloss.Color("#9ece6a")
	colorMark      = lipgloss.Color("#73daca")
	colorSearch    = lipgloss.Color("#4c4c6e")
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
