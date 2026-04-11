package tui

import (
	"fmt"
	"strings"

	"maximus-cli/internal/brew"
	"maximus-cli/internal/db"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// View renders the current model state as a tea.View.
func (m Model) View() tea.View {
	var content string
	switch m.state {
	case stateLoading:
		content = "\n\n   " + m.spinner.View() + " " + m.loadingText + "\n\n"

	case stateResult:
		wrapWidth := m.width - 4
		if wrapWidth < 40 {
			wrapWidth = 80
		}
		formatted := lipgloss.NewStyle().Width(wrapWidth).Render(m.result)
		formatted = strings.ReplaceAll(formatted, "Error:", "\n⚠  Error:\n")
		content = "\n" + formatted + "\n\n" + helpStyle.Render("(press q or esc to go back)") + "\n"

	case stateBrewLogs:
		content = m.renderLogs()

	case stateBrewVersion:
		content = m.renderVersionTable()

	case stateUnstaged:
		content = m.renderUnstaged()

	case stateBrewMenu:
		content = m.brewList.View()

	default: // stateMainMenu
		content = m.mainList.View()
	}

	v := tea.NewView(content)
	v.AltScreen = true
	return v
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

// renderUnstaged renders the list of packages installed but absent from the
// Brewfile, with per-package checkbox selection.
func (m Model) renderUnstaged() string {
	var sb strings.Builder
	sb.WriteString("\n")
	sb.WriteString(headerStyle.Render("  Unstaged Packages"))
	sb.WriteString("\n\n")

	if len(m.unstagedPackages) == 0 {
		sb.WriteString("  ✓ No packages found outside your Brewfile.\n")
		sb.WriteString("\n")
		sb.WriteString(helpStyle.Render("  q / esc  back"))
		sb.WriteString("\n")
		return sb.String()
	}

	selectedCount := len(m.unstagedSelected)
	sb.WriteString(warningStyle.Render(fmt.Sprintf(
		"  %d package(s) installed but not in Brewfile (%d selected):",
		len(m.unstagedPackages), selectedCount,
	)))
	sb.WriteString("\n\n")

	// Column header
	sb.WriteString(fmt.Sprintf("  %-4s  %-10s  %s\n", "", "TYPE", "PACKAGE"))
	sb.WriteString(helpStyle.Render("  "+strings.Repeat("─", 50)) + "\n")

	for i, p := range m.unstagedPackages {
		checked := "[ ]"
		if m.unstagedSelected[i] {
			checked = "[✓]"
		}
		row := fmt.Sprintf("  %-4s  %-10s  %s", checked, "["+p.Kind+"]", p.Name)
		if i == m.unstagedCursor {
			row = lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.Color("212")).
				Render(row)
		}
		sb.WriteString(row + "\n")
	}

	sb.WriteString("\n")
	sb.WriteString(helpStyle.Render(
		"  ↑/↓ or j/k navigate · space toggle · a select/deselect all · enter add selected · q back",
	))
	sb.WriteString("\n")
	return sb.String()
}

// truncate clips s to max length, appending "…" if needed.
func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max-1] + "…"
}

// renderVersionTable renders the installed Brewfile packages as a sortable,
// filterable table.
func (m Model) renderVersionTable() string {
	var sb strings.Builder

	// ── Header ──────────────────────────────────────────────────────────────
	sb.WriteString("\n")
	sb.WriteString(headerStyle.Render("  Installed Versions"))
	sb.WriteString("\n\n")

	// ── Filter bar ──────────────────────────────────────────────────────────
	if m.versionInputMode {
		sb.WriteString("  Filter: " + m.versionInput.View() + "\n")
		sb.WriteString(helpStyle.Render("  enter to apply · esc to cancel") + "\n\n")
	} else {
		label := "  Filter: "
		if m.versionFilter != "" {
			label += warningStyle.Render("[" + m.versionFilter + "]")
		} else {
			label += helpStyle.Render("none")
		}
		sb.WriteString(label + "\n\n")
	}

	if len(m.versionFiltered) == 0 {
		sb.WriteString("  No packages found.\n")
	} else {
		sb.WriteString(versionTableRows(m.versionFiltered, m.versionSortField, m.versionSortAsc, m.versionCursor))
	}

	// ── Stats line ──────────────────────────────────────────────────────────
	sb.WriteString("\n")
	sb.WriteString(helpStyle.Render(fmt.Sprintf(
		"  Showing %d of %d packages",
		len(m.versionFiltered), len(m.versionItems),
	)))
	sb.WriteString("\n")

	// ── Key hints ───────────────────────────────────────────────────────────
	sb.WriteString(helpStyle.Render(
		"  ↑/↓ navigate · / filter · r reset · s sort col · o order · q back",
	))
	sb.WriteString("\n")

	return sb.String()
}

// sortLabel returns the column label with a sort indicator when active.
func sortLabel(col, active versionSortField, asc bool, label string, width int) string {
	if col != active {
		return fmt.Sprintf("%-*s", width, label)
	}
	indicator := "↑"
	if !asc {
		indicator = "↓"
	}
	return fmt.Sprintf("%-*s", width, label+" "+indicator)
}

// versionTableRows formats the filtered rows as a fixed-width table string.
func versionTableRows(items []brew.PackageVersion, sortField versionSortField, sortAsc bool, cursor int) string {
	var sb strings.Builder

	// Column widths.
	const nameW, kindW, verW, dateW = 28, 6, 18, 12

	// Header
	header := fmt.Sprintf("  %-*s  %-*s  %-*s  %-*s  %s",
		nameW, sortLabel(sortByName, sortField, sortAsc, "NAME", nameW),
		kindW, sortLabel(sortByKind, sortField, sortAsc, "TYPE", kindW),
		verW, "VERSION",
		dateW, sortLabel(sortByMetaDate, sortField, sortAsc, "PKG DATE", dateW),
		sortLabel(sortByInstallDate, sortField, sortAsc, "INSTALL DATE", 12),
	)
	sb.WriteString(headerStyle.Render(header) + "\n")
	sb.WriteString(helpStyle.Render("  "+strings.Repeat("─", 90)) + "\n")

	highlighted := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("212"))

	for i, p := range items {
		metaDate := "—"
		if !p.MetadataDate.IsZero() {
			metaDate = p.MetadataDate.Format("2006-01-02")
		}
		installDate := "—"
		if !p.InstallDate.IsZero() {
			installDate = p.InstallDate.Format("2006-01-02")
		}
		row := fmt.Sprintf("  %-*s  %-*s  %-*s  %-*s  %s",
			nameW, truncate(p.Name, nameW),
			kindW, truncate(p.Kind, kindW),
			verW, truncate(p.Version, verW),
			dateW, metaDate,
			installDate,
		)
		if i == cursor {
			row = highlighted.Render(row)
		}
		sb.WriteString(row + "\n")
	}
	return sb.String()
}
