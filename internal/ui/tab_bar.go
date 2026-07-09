package ui

import (
	"fmt"

	"github.com/charmbracelet/lipgloss"
)

// tabWidth is the fixed rendered width of every tab (content + padding).
const tabWidth = 7

// TabBar displays and manages tab navigation.
type TabBar struct {
	tabs     []*ResultsTab
	activeID int
	width    int
	height   int
	focused  bool
}

// NewTabBar creates a new tab bar component.
func NewTabBar() TabBar {
	return TabBar{
		height: 1, // tab text only (rendered inside editor panel)
	}
}

// SetTabs updates the tab list and active tab.
func (t *TabBar) SetTabs(tabs []*ResultsTab, activeID int) {
	t.tabs = tabs
	t.activeID = activeID
}

// SetSize sets the tab bar dimensions.
func (t *TabBar) SetSize(width, height int) {
	t.width = width
	t.height = height
}

// Focus marks the tab bar as focused.
func (t *TabBar) Focus() {
	t.focused = true
}

// Blur removes focus from the tab bar.
func (t *TabBar) Blur() {
	t.focused = false
}

// IsFocused returns whether the tab bar has focus.
func (t TabBar) IsFocused() bool {
	return t.focused
}

// TabIndex returns the index of the active tab, or -1 if not found.
func (t TabBar) TabIndex() int {
	for i, tab := range t.tabs {
		if tab.ID == t.activeID {
			return i
		}
	}
	return -1
}

// NextTab moves to the next tab (cyclic).
func (t *TabBar) NextTab() int {
	if len(t.tabs) == 0 {
		return -1
	}
	idx := t.TabIndex()
	if idx < 0 {
		idx = 0
	} else {
		idx = (idx + 1) % len(t.tabs)
	}
	return t.tabs[idx].ID
}

// PrevTab moves to the previous tab (cyclic).
func (t *TabBar) PrevTab() int {
	if len(t.tabs) == 0 {
		return -1
	}
	idx := t.TabIndex()
	if idx <= 0 {
		idx = len(t.tabs) - 1
	} else {
		idx = idx - 1
	}
	return t.tabs[idx].ID
}

// GotoTab jumps to tab by absolute index (1-based).
func (t *TabBar) GotoTab(n int) int {
	if n < 1 || n > len(t.tabs) {
		return -1
	}
	return t.tabs[n-1].ID
}

// TabCount returns the number of tabs.
func (t TabBar) TabCount() int {
	return len(t.tabs)
}

// ActiveTabID returns the active tab ID.
func (t TabBar) ActiveTabID() int {
	return t.activeID
}

// ClickTab returns the tab ID at the given relative X coordinate within the
// tab bar, or -1 if the '+' button was clicked, or -2 if no tab was hit.
func (t TabBar) ClickAt(relX int) int {
	if len(t.tabs) == 0 || relX < 0 {
		return -2
	}
	idx := relX / tabWidth
	if idx < len(t.tabs) {
		return t.tabs[idx].ID
	}
	if idx == len(t.tabs) {
		return -1 // '+' button
	}
	return -2
}

// View renders the tab bar as a single line of tab labels.
func (t TabBar) View() string {
	if len(t.tabs) == 0 {
		return ""
	}

	var tabs []string
	for i, tab := range t.tabs {
		isActive := tab.ID == t.activeID

		// Tab label: 1-based index (matches `g 1`–`g 9` keybindings), with a
		// dirty indicator when the editor content differs from the last query.
		label := fmt.Sprintf("%d", i+1)
		if tab.EditorQuery != "" && tab.EditorQuery != tab.LastQuery {
			label += " ●"
		}

		var style lipgloss.Style
		if isActive {
			if t.focused {
				style = activeTabStyleFocused
			} else {
				style = activeTabStyle
			}
		} else {
			if t.focused {
				style = inactiveTabStyleFocused
			} else {
				style = inactiveTabStyle
			}
		}

		tabs = append(tabs, style.Render(label))
	}

	// '+' button to create a new tab.
	plusStyle := inactiveTabStyle
	if t.focused {
		plusStyle = inactiveTabStyleFocused
	}
	tabs = append(tabs, plusStyle.Render("+"))

	tabLine := lipgloss.JoinHorizontal(lipgloss.Top, tabs...)

	return tabLine
}