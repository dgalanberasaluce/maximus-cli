package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// View renders the current model state to a string.
func (m Model) View() string {
	switch m.state {
	case stateLoading:
		return "\n\n   " + m.spinner.View() + " " + m.loadingText + "\n\n"

	case stateResult:
		// Word-wrap the result to fit the terminal, with a small margin.
		wrapWidth := m.width - 4
		if wrapWidth < 40 {
			wrapWidth = 80 // fallback before first WindowSizeMsg
		}
		formatted := lipgloss.NewStyle().Width(wrapWidth).Render(m.result)
		// Ensure each "Error:" prefix stands out on its own line.
		formatted = strings.ReplaceAll(formatted, "Error:", "\n⚠  Error:\n")
		return "\n" + formatted + "\n\n" + helpStyle.Render("(press q or esc to go back)") + "\n"

	case stateBrewMenu:
		return m.brewList.View()

	default: // stateMainMenu
		return m.mainList.View()
	}
}
