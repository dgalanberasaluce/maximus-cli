package tui

import (
	"fmt"
	"strings"

	"maximus-cli/internal/db"

	"github.com/charmbracelet/lipgloss"
)

// View renders the current model state to a string.
func (m Model) View() string {
	switch m.state {
	case stateLoading:
		return "\n\n   " + m.spinner.View() + " " + m.loadingText + "\n\n"

	case stateResult:
		wrapWidth := m.width - 4
		if wrapWidth < 40 {
			wrapWidth = 80
		}
		formatted := lipgloss.NewStyle().Width(wrapWidth).Render(m.result)
		formatted = strings.ReplaceAll(formatted, "Error:", "\n⚠  Error:\n")
		return "\n" + formatted + "\n\n" + helpStyle.Render("(press q or esc to go back)") + "\n"

	case stateBrewLogs:
		return m.renderLogs()

	case stateBrewMenu:
		return m.brewList.View()

	default: // stateMainMenu
		return m.mainList.View()
	}
}

// renderLogs renders the paginated upgrade log table.
func (m Model) renderLogs() string {
	var sb strings.Builder

	// ── Header ──────────────────────────────────────────────────────────────
	sb.WriteString("\n")
	sb.WriteString(headerStyle.Render("  Upgrade Logs"))
	sb.WriteString("\n\n")

	// ── Filter bar ──────────────────────────────────────────────────────────
	if m.logInputMode {
		sb.WriteString("  Filter: " + m.logInput.View() + "\n")
		sb.WriteString(helpStyle.Render("  enter to apply · esc to cancel") + "\n\n")
	} else {
		label := "  Filter: "
		if m.logFilter != "" {
			label += warningStyle.Render("["+m.logFilter+"]")
		} else {
			label += helpStyle.Render("none")
		}
		sb.WriteString(label + "\n\n")
	}

	// ── Table ────────────────────────────────────────────────────────────────
	if len(m.logEntries) == 0 {
		sb.WriteString("  No upgrade logs found.\n")
	} else {
		sb.WriteString(renderLogTable(m.logEntries))
	}

	// ── Pagination info ──────────────────────────────────────────────────────
	sb.WriteString("\n")
	if m.logTotal > 0 {
		start := m.logPage*logPageSize + 1
		end := m.logPage*logPageSize + len(m.logEntries)
		maxPage := (m.logTotal - 1) / logPageSize
		sb.WriteString(helpStyle.Render(fmt.Sprintf(
			"  Showing %d–%d of %d  (page %d/%d)",
			start, end, m.logTotal, m.logPage+1, maxPage+1,
		)))
		sb.WriteString("\n")
	}

	// ── Key hints ────────────────────────────────────────────────────────────
	sb.WriteString(helpStyle.Render("  / filter · r reset · n/p next/prev page · q back"))
	sb.WriteString("\n")

	return sb.String()
}

// renderLogTable formats a slice of UpgradeLog entries as a fixed-width table.
func renderLogTable(entries []db.UpgradeLog) string {
	var sb strings.Builder
	header := fmt.Sprintf("  %-28s  %-16s  %-16s  %s",
		"PACKAGE", "FROM", "TO", "DATE")
	sb.WriteString(headerStyle.Render(header) + "\n")
	sb.WriteString(helpStyle.Render("  " + strings.Repeat("─", 76)) + "\n")
	for _, e := range entries {
		date := e.UpgradedAt.Format("2006-01-02 15:04")
		sb.WriteString(fmt.Sprintf("  %-28s  %-16s  %-16s  %s\n",
			truncate(e.PackageName, 28),
			truncate(e.OldVersion, 16),
			truncate(e.NewVersion, 16),
			date,
		))
	}
	return sb.String()
}

// truncate clips s to max length, appending "…" if needed.
func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max-1] + "…"
}
