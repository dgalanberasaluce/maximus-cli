package tui

import (
	"fmt"
	"strings"

	"maximus-cli/internal/brew"
	"maximus-cli/internal/db"
	"maximus-cli/internal/home"

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
		content = m.viewport.View() + "\n\n" + helpStyle.Render("(press q or esc to go back)") + "\n"

	case stateBrewLogs:
		content = m.renderLogs()

	case stateBrewVersion:
		content = m.renderVersionTable()

	case stateUnstaged:
		content = m.renderUnstaged()

	case stateUpgradePkgs:
		content = m.renderUpgradePkgs()

	case stateBrewMenu:
		content = m.brewList.View()

	case stateHomeMenu:
		content = m.homeList.View()

	case stateHomeDotfiles:
		content = m.renderDotfileTable()

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
			label += warningStyle.Render("[" + m.logFilter + "]")
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
	sb.WriteString(helpStyle.Render("  "+strings.Repeat("─", 76)) + "\n")
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

	// Calculate viewport for rows.
	const chromeHeight = 12
	maxRows := m.height - chromeHeight
	if maxRows < 1 {
		maxRows = 1
	}

	start := 0
	if len(m.unstagedPackages) > maxRows {
		start = m.unstagedCursor - (maxRows / 2)
		if start < 0 {
			start = 0
		}
		if start+maxRows > len(m.unstagedPackages) {
			start = len(m.unstagedPackages) - maxRows
		}
	}
	end := start + maxRows
	if end > len(m.unstagedPackages) {
		end = len(m.unstagedPackages)
	}

	for i := start; i < end; i++ {
		p := m.unstagedPackages[i]
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

// renderUpgradePkgs renders the list of outdated packages with their current and new versions.
func (m Model) renderUpgradePkgs() string {
	var sb strings.Builder
	sb.WriteString("\n")
	sb.WriteString(headerStyle.Render("  Upgrade Package(s)"))
	sb.WriteString("\n\n")

	if len(m.upgradePkgs) == 0 {
		sb.WriteString("  ✓ Everything in your Brewfile is up to date!\n")
		sb.WriteString("\n")
		sb.WriteString(helpStyle.Render("  q / esc  back"))
		sb.WriteString("\n")
		return sb.String()
	}

	selectedCount := 0
	for _, sel := range m.upgradeSelected {
		if sel {
			selectedCount++
		}
	}

	sb.WriteString(warningStyle.Render(fmt.Sprintf(
		"  %d package(s) can be upgraded (%d selected):",
		len(m.upgradePkgs), selectedCount,
	)))
	sb.WriteString("\n\n")

	// Column header
	sb.WriteString(fmt.Sprintf("  %-4s  %-28s  %-16s  %s\n", "", "PACKAGE", "CURRENT", "AVAILABLE"))
	sb.WriteString(helpStyle.Render("  "+strings.Repeat("─", 68)) + "\n")

	// Calculate viewport for rows.
	const chromeHeight = 12
	maxRows := m.height - chromeHeight
	if maxRows < 1 {
		maxRows = 1
	}

	start := 0
	if len(m.upgradePkgs) > maxRows {
		start = m.upgradeCursor - (maxRows / 2)
		if start < 0 {
			start = 0
		}
		if start+maxRows > len(m.upgradePkgs) {
			start = len(m.upgradePkgs) - maxRows
		}
	}
	end := start + maxRows
	if end > len(m.upgradePkgs) {
		end = len(m.upgradePkgs)
	}

	for i := start; i < end; i++ {
		p := m.upgradePkgs[i]
		checked := "[ ]"
		if m.upgradeSelected[i] {
			checked = "[✓]"
		}
		row := fmt.Sprintf("  %-4s  %-28s  %-16s  %s", checked, truncate(p.Name, 28), truncate(p.CurrentVersion, 16), truncate(p.LatestVersion, 16))
		if i == m.upgradeCursor {
			row = lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.Color("212")).
				Render(row)
		}
		sb.WriteString(row + "\n")
	}

	sb.WriteString("\n")
	sb.WriteString(helpStyle.Render(
		"  ↑/↓ or j/k navigate · space toggle · a select/deselect all · enter upgrade selected · q back",
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
// filterable table with a scrollable viewport.
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
		// Calculate viewport.
		// We subtract lines used by headers and footers from total height.
		const chromeHeight = 12
		maxRows := m.height - chromeHeight
		if maxRows < 1 {
			maxRows = 1
		}

		start := 0
		if len(m.versionFiltered) > maxRows {
			start = m.versionCursor - (maxRows / 2)
			if start < 0 {
				start = 0
			}
			if start+maxRows > len(m.versionFiltered) {
				start = len(m.versionFiltered) - maxRows
			}
		}
		end := start + maxRows
		if end > len(m.versionFiltered) {
			end = len(m.versionFiltered)
		}

		sb.WriteString(versionTableRows(m.versionFiltered[start:end], m.versionSortField, m.versionSortAsc, m.versionCursor-start))
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
		"  ↑/↓ navigate · ←/→ sort col · / filter · r reset · s/o order · q back",
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

// renderDotfileTable renders the dotfiles and directories table with scrollable viewport,
// filtering and sorting.
func (m Model) renderDotfileTable() string {
	var sb strings.Builder

	// ── Header ───────────────────────────────────────────────────────────────
	sb.WriteString("\n")
	sb.WriteString(headerStyle.Render("  Home Dotfiles & Directories"))

	// Show the most recent scan timestamp if available.
	if len(m.dotfileItems) > 0 {
		latest := m.dotfileItems[0].ScannedAt
		for _, e := range m.dotfileItems[1:] {
			if e.ScannedAt.After(latest) {
				latest = e.ScannedAt
			}
		}
		sb.WriteString(helpStyle.Render("  (last updated: " + latest.Local().Format("2006-01-02 15:04:05") + ")"))
	}
	sb.WriteString("\n\n")

	// ── Prompts (highest priority down to lowest) ───────────────────────────
	if m.dotfileDeleteMode && m.dotfileCursor >= 0 && m.dotfileCursor < len(m.dotfileFiltered) {
		name := m.dotfileFiltered[m.dotfileCursor].Name
		sb.WriteString("  " + warningStyle.Render("CONFIRM DELETION of "+name+":") + "\n")
		sb.WriteString("  " + helpStyle.Render("To delete this file/directory, type its exact name and press enter:") + "\n")
		sb.WriteString("  " + m.dotfileDeleteInput.View() + "\n")
		sb.WriteString("  " + helpStyle.Render("enter to confirm · esc to cancel") + "\n\n")
	} else if m.dotfileToolEditMode && m.dotfileCursor >= 0 && m.dotfileCursor < len(m.dotfileFiltered) {
		name := m.dotfileFiltered[m.dotfileCursor].Name
		sb.WriteString("  " + warningStyle.Render("Edit Tool for "+name+":") + " " + m.dotfileToolInput.View() + "\n")
		sb.WriteString(helpStyle.Render("  enter to save · esc to cancel") + "\n\n")
	} else if m.dotfileInputMode {
		// ── Text filter input ────────────────────────────────────────────────
		sb.WriteString("  Filter: " + m.dotfileInput.View() + "\n")
		sb.WriteString(helpStyle.Render("  enter to apply · esc to cancel") + "\n\n")
	} else {
		// ── Static filter bar ────────────────────────────────────────────────
		// Text filter
		label := "  Filter: "
		if m.dotfileFilter != "" {
			label += warningStyle.Render("[" + m.dotfileFilter + "]")
		} else {
			label += helpStyle.Render("none")
		}

		// Type filter badge
		typeLabel := ""
		switch m.dotfileTypeFilter {
		case typeFilterFiles:
			typeLabel = warningStyle.Render("  [Type: Files]")
		case typeFilterDirs:
			typeLabel = warningStyle.Render("  [Type: Dirs]")
		default:
			typeLabel = helpStyle.Render("  [Type: All]")
		}

		// Active panel indicator
		panelLabel := helpStyle.Render("  [Active: Table Panel]")
		if m.dotfilePreviewFocused {
			panelLabel = warningStyle.Render("  [Active: Preview Panel]")
		}

		sb.WriteString(label + typeLabel + panelLabel + "\n\n")
	}

	// ── Table & Preview Side-by-Side ─────────────────────────────────────────
	var mainBody string
	if len(m.dotfileFiltered) == 0 {
		mainBody = "  No dotfiles found.\n"
	} else {
		// Calculate scrollable viewport height.
		const chromeHeight = 12
		maxRows := m.height - chromeHeight
		if maxRows < 1 {
			maxRows = 1
		}

		start := 0
		if len(m.dotfileFiltered) > maxRows {
			start = m.dotfileCursor - (maxRows / 2)
			if start < 0 {
				start = 0
			}
			if start+maxRows > len(m.dotfileFiltered) {
				start = len(m.dotfileFiltered) - maxRows
			}
		}
		end := start + maxRows
		if end > len(m.dotfileFiltered) {
			end = len(m.dotfileFiltered)
		}

		// 1. Render Left Column (Table)
		tableStr := dotfileTableRows(m.dotfileFiltered[start:end], m.dotfileSortField, m.dotfileSortAsc, m.dotfileCursor-start)

		// 2. Render Right Column (Preview Panel Viewport)
		const minTableWidth = 86
		previewWidth := m.width - minTableWidth - 4
		if previewWidth < 25 {
			previewWidth = 25
		}

		// Apply styling to preview panel
		borderCol := "240"
		if m.dotfilePreviewFocused {
			borderCol = "212" // active focus border highlight
		}
		previewStyle := lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color(borderCol)).
			Padding(0, 1).
			Width(previewWidth).
			Height(maxRows + 1)

		selectedItem := m.dotfileFiltered[m.dotfileCursor]
		previewTitle, _ := home.GetPreview(selectedItem.Name, selectedItem.IsDir)

		previewHeading := headerStyle.Render(previewTitle) + "\n"
		previewBody := previewHeading + m.previewViewport.View()

		previewPanel := previewStyle.Render(previewBody)

		// Combine horizontally!
		mainBody = lipgloss.JoinHorizontal(lipgloss.Top, tableStr, "  ", previewPanel)
	}
	sb.WriteString(mainBody)

	// ── Stats line ───────────────────────────────────────────────────────────
	sb.WriteString("\n")
	sb.WriteString(helpStyle.Render(fmt.Sprintf(
		"  Showing %d of %d items",
		len(m.dotfileFiltered), len(m.dotfileItems),
	)))
	sb.WriteString("\n")

	// ── Key hints ────────────────────────────────────────────────────────────
	if m.dotfilePreviewFocused {
		sb.WriteString(helpStyle.Render(
			"  tab table panel · ↑/↓ scroll preview · e edit file · q back",
		))
	} else {
		sb.WriteString(helpStyle.Render(
			"  tab preview panel · ↑/↓ navigate · ←/→ sort · / filter · t type · e edit tool · d delete · q back",
		))
	}
	sb.WriteString("\n")
	sb.WriteString(helpStyle.Render(
		"  * = manually edited tool description",
	))
	sb.WriteString("\n")

	return sb.String()
}

// WordWrap wraps the string s to the given width.
func WordWrap(s string, width int) string {
	if width <= 0 {
		return s
	}
	var sb strings.Builder
	lines := strings.Split(s, "\n")
	for i, line := range lines {
		if i > 0 {
			sb.WriteString("\n")
		}
		if strings.TrimSpace(line) == "" {
			continue
		}
		words := strings.Fields(line)
		currentLineLen := 0
		for j, word := range words {
			if currentLineLen+len(word)+1 > width {
				sb.WriteString("\n")
				sb.WriteString(word)
				currentLineLen = len(word)
			} else {
				if j > 0 && currentLineLen > 0 {
					sb.WriteString(" ")
					currentLineLen++
				}
				sb.WriteString(word)
				currentLineLen += len(word)
			}
		}
	}
	return sb.String()
}

// dotfileTableRows formats the filtered dotfiles as a fixed-width table string.
func dotfileTableRows(items []db.DotfileEntry, sortField dotfileSortField, sortAsc bool, cursor int) string {
	var sb strings.Builder

	// Column widths.
	const nameW, typeW, toolW, dateW = 30, 6, 18, 12

	// Column headers helper
	sortDFLabel := func(col, active dotfileSortField, asc bool, label string, width int) string {
		if col != active {
			return fmt.Sprintf("%-*s", width, label)
		}
		indicator := "↑"
		if !asc {
			indicator = "↓"
		}
		return fmt.Sprintf("%-*s", width, label+" "+indicator)
	}

	// Header line
	header := fmt.Sprintf("  %-*s  %-*s  %-*s  %-*s  %s",
		nameW, sortDFLabel(sortDFByName, sortField, sortAsc, "NAME", nameW),
		typeW, sortDFLabel(sortDFByType, sortField, sortAsc, "TYPE", typeW),
		toolW, sortDFLabel(sortDFByTool, sortField, sortAsc, "TOOL", toolW),
		dateW, sortDFLabel(sortDFByModified, sortField, sortAsc, "MODIFIED", dateW),
		sortDFLabel(sortDFByCreated, sortField, sortAsc, "CREATED", 12),
	)
	sb.WriteString(headerStyle.Render(header) + "\n")
	sb.WriteString(helpStyle.Render("  "+strings.Repeat("─", 84)) + "\n")

	highlighted := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("212"))

	for i, p := range items {
		tStr := "File"
		if p.IsDir {
			tStr = "Dir"
		}
		modDate := "—"
		if !p.ModifiedAt.IsZero() {
			modDate = p.ModifiedAt.Format("2006-01-02")
		}
		creDate := "—"
		if !p.CreatedAt.IsZero() {
			creDate = p.CreatedAt.Format("2006-01-02")
		}

		toolName := p.Tool
		if p.ToolManual {
			toolName = "* " + p.Tool
		}

		row := fmt.Sprintf("  %-*s  %-*s  %-*s  %-*s  %s",
			nameW, truncate(p.Name, nameW),
			typeW, tStr,
			toolW, truncate(toolName, toolW),
			dateW, modDate,
			creDate,
		)
		if i == cursor {
			row = highlighted.Render(row)
		}
		sb.WriteString(row + "\n")
	}
	return sb.String()
}
