package tui

import (
	"fmt"

	"maximus-cli/internal/brew"
	"maximus-cli/internal/db"

	tea "github.com/charmbracelet/bubbletea"
)

// Update processes incoming messages and updates model state.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	var cmds []tea.Cmd

	switch msg := msg.(type) {

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.mainList.SetSize(msg.Width, msg.Height)
		m.brewList.SetSize(msg.Width, msg.Height)

	case errMsg:
		m.result = fmt.Sprintf("Error:\n%v", error(msg))
		m.state = stateResult
		return m, nil

	case resultMsg:
		m.result = msg.content
		m.state = stateResult
		return m, nil

	case unstagedMsg:
		m.unstagedPackages = msg.packages
		m.unstagedCursor = 0
		// Pre-select all packages.
		sel := make(map[int]bool, len(msg.packages))
		for i := range msg.packages {
			sel[i] = true
		}
		m.unstagedSelected = sel
		m.state = stateUnstaged
		return m, nil

	case logsMsg:
		m.logEntries = msg.entries
		m.logTotal = msg.total
		m.state = stateBrewLogs
		return m, nil

	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c":
			return m, tea.Quit
		}

		// --- Log view input mode: route keys to the text input ---
		if m.state == stateBrewLogs && m.logInputMode {
			switch msg.String() {
			case "enter":
				// Apply filter and fetch page 0.
				m.logFilter = m.logInput.Value()
				m.logPage = 0
				m.logInputMode = false
				return m, m.fetchLogs()
			case "esc":
				m.logInputMode = false
				return m, nil
			default:
				m.logInput, cmd = m.logInput.Update(msg)
				return m, cmd
			}
		}

		// --- Unstaged packages screen ---
		if m.state == stateUnstaged {
			n := len(m.unstagedPackages)
			switch msg.String() {
			case "up", "k":
				if m.unstagedCursor > 0 {
					m.unstagedCursor--
				}
			case "down", "j":
				if m.unstagedCursor < n-1 {
					m.unstagedCursor++
				}
			case " ":
				// Toggle the currently highlighted package.
				if m.unstagedSelected == nil {
					m.unstagedSelected = make(map[int]bool)
				}
				m.unstagedSelected[m.unstagedCursor] = !m.unstagedSelected[m.unstagedCursor]
			case "a":
				// Toggle all: if all are selected, deselect all; otherwise select all.
				allSelected := len(m.unstagedSelected) == n && n > 0
				sel := make(map[int]bool, n)
				if !allSelected {
					for i := range n {
						sel[i] = true
					}
				}
				m.unstagedSelected = sel
			case "enter":
				// Add only the selected packages.
				var chosen []brew.UnstagedPackage
				for i, p := range m.unstagedPackages {
					if m.unstagedSelected[i] {
						chosen = append(chosen, p)
					}
				}
				if len(chosen) == 0 {
					m.state = stateBrewMenu
					return m, nil
				}
				brewinfile := m.brewfile
				database := m.database
				m.state = stateLoading
				m.loadingText = fmt.Sprintf("Adding %d package(s) to Brewfile...", len(chosen))
				return m, tea.Batch(m.spinner.Tick, unstagedAddAllCmd(chosen, brewinfile, database))
			case "esc", "q":
				m.state = stateBrewMenu
				return m, nil
			}
			return m, nil
		}

		// --- Log view navigation (input mode off) ---
		if m.state == stateBrewLogs {
			switch msg.String() {
			case "esc", "q":
				// Back to brew menu.
				m.state = stateBrewMenu
				return m, nil
			case "/":
				// Enter filter input mode.
				m.logInput.SetValue(m.logFilter)
				m.logInput.Focus()
				m.logInputMode = true
				return m, nil
			case "r":
				// Clear filter and reload.
				m.logFilter = ""
				m.logInput.SetValue("")
				m.logPage = 0
				return m, m.fetchLogs()
			case "n", "right":
				// Next page (if there are more entries).
				maxPage := (m.logTotal - 1) / logPageSize
				if m.logPage < maxPage {
					m.logPage++
					return m, m.fetchLogs()
				}
			case "p", "left":
				// Previous page.
				if m.logPage > 0 {
					m.logPage--
					return m, m.fetchLogs()
				}
			}
			return m, nil
		}

		// --- Global back / quit ---
		switch msg.String() {
		case "esc", "q":
			switch m.state {
			case stateResult:
				m.result = ""
				m.state = stateBrewMenu
				return m, nil
			case stateBrewMenu:
				m.state = stateMainMenu
				return m, nil
			}
		case "enter":
			switch m.state {
			case stateMainMenu:
				if i, ok := m.mainList.SelectedItem().(menuItem); ok {
					if i.title == "Brew" {
						m.brewList.SetSize(m.width, m.height)
						m.state = stateBrewMenu
						return m, nil
					}
				}
			case stateBrewMenu:
				if i, ok := m.brewList.SelectedItem().(menuItem); ok {
					return m.dispatchBrewCmd(i.title)
				}
			}
		}
	}

	// Delegate navigation updates to the active list/component.
	switch m.state {
	case stateMainMenu:
		m.mainList, cmd = m.mainList.Update(msg)
		cmds = append(cmds, cmd)
	case stateBrewMenu:
		m.brewList, cmd = m.brewList.Update(msg)
		cmds = append(cmds, cmd)
	case stateLoading:
		m.spinner, cmd = m.spinner.Update(msg)
		cmds = append(cmds, cmd)
	}

	return m, tea.Batch(cmds...)
}

// fetchLogs returns a Cmd that queries the DB for the current log page.
func (m Model) fetchLogs() tea.Cmd {
	database := m.database
	filter := m.logFilter
	page := m.logPage
	return func() tea.Msg {
		total, err := database.CountUpgradeLogs(filter)
		if err != nil {
			return errMsg(err)
		}
		entries, err := database.GetUpgradeLogs(filter, logPageSize, page*logPageSize)
		if err != nil {
			return errMsg(err)
		}
		return logsMsg{entries: entries, total: total}
	}
}

// dispatchBrewCmd maps a selected brew menu item to the appropriate background command.
func (m Model) dispatchBrewCmd(title string) (tea.Model, tea.Cmd) {
	brewfile := m.brewfile
	database := m.database

	switch title {
	case "Logs":
		// Load the first page immediately (no loading spinner needed).
		m.logPage = 0
		m.logFilter = ""
		m.logInput.SetValue("")
		m.logInputMode = false
		m.state = stateLoading
		m.loadingText = "Loading upgrade logs..."
		return m, tea.Batch(m.spinner.Tick, m.fetchLogs())

	case "Update":
		m.state = stateLoading
		m.loadingText = "Running brew update..."
		bgCmd := func() tea.Msg {
			out, err := brew.Update()
			if err != nil {
				return errMsg(err)
			}
			return resultMsg{content: out}
		}
		return m, tea.Batch(m.spinner.Tick, bgCmd)

	case "Upgrade All":
		m.state = stateLoading
		m.loadingText = "Running brew bundle install..."
		bgCmd := func() tea.Msg {
			// Capture what's about to be upgraded for logging.
			diffs, _ := brew.SmartDiff(brewfile)

			out, err := brew.Upgrade(brewfile)
			if err != nil {
				return errMsg(err)
			}
			// Log each upgraded package to the DB.
			for _, d := range diffs {
				_ = database.LogUpgrade(d.Name, d.CurrentVersion, d.LatestVersion)
			}
			return resultMsg{content: out}
		}
		return m, tea.Batch(m.spinner.Tick, bgCmd)

	case "Cleanup":
		m.state = stateLoading
		m.loadingText = "Running brew cleanup + autoremove..."
		bgCmd := func() tea.Msg {
			out, err := brew.Cleanup()
			if err != nil {
				return errMsg(err)
			}
			return resultMsg{content: out}
		}
		return m, tea.Batch(m.spinner.Tick, bgCmd)

	case "Diff":
		m.state = stateLoading
		m.loadingText = "Computing Smart Diff..."
		bgCmd := func() tea.Msg {
			results, err := brew.SmartDiff(brewfile)
			if err != nil {
				return errMsg(err)
			}
			return resultMsg{content: brew.FormatDiffResults(results)}
		}
		return m, tea.Batch(m.spinner.Tick, bgCmd)

	case "Unstaged":
		m.state = stateLoading
		m.loadingText = "Checking unstaged packages..."
		bgCmd := func() tea.Msg {
			pkgs, err := brew.Unstaged(brewfile)
			if err != nil {
				return errMsg(err)
			}
			return unstagedMsg{packages: pkgs}
		}
		return m, tea.Batch(m.spinner.Tick, bgCmd)

	case "Remove":
		m.state = stateLoading
		m.loadingText = "Running brew bundle cleanup --force..."
		bgCmd := func() tea.Msg {
			out, err := brew.Remove(brewfile)
			if err != nil {
				return errMsg(err)
			}
			return resultMsg{content: out}
		}
		return m, tea.Batch(m.spinner.Tick, bgCmd)

	case "Cheatsheet":
		m.state = stateLoading
		m.loadingText = "Loading cheatsheet..."
		bgCmd := func() tea.Msg {
			out, err := brew.Cheatsheet()
			if err != nil {
				return errMsg(err)
			}
			return resultMsg{content: out}
		}
		return m, tea.Batch(m.spinner.Tick, bgCmd)

	default:
		m.result = fmt.Sprintf("Unknown command: %q", title)
		m.state = stateResult
		return m, nil
	}
}

// logPageFromDB is a helper type so db is available without import cycle.
type logPageFromDB = db.UpgradeLog

// unstagedMsg carries the list of unstaged packages back to the model.
type unstagedMsg struct {
	packages []brew.UnstagedPackage
}

// unstagedAddAllCmd adds all packages to the Brewfile and logs them to DB.
func unstagedAddAllCmd(pkgs []brew.UnstagedPackage, brewfilePath string, database *db.DB) tea.Cmd {
	return func() tea.Msg {
		if err := brew.AddPackagesToBrewfile(brewfilePath, pkgs); err != nil {
			return errMsg(err)
		}
		for _, p := range pkgs {
			// Retrieve the installed version; if unavailable, use "(unknown)".
			version := installedVersion(p.Name)
			_ = database.LogAddition(p.Name, p.Kind, version)
		}
		return resultMsg{content: fmt.Sprintf("✓ Added %d package(s) to Brewfile and logged to database.", len(pkgs))}
	}
}

// installedVersion returns the installed version of a formula via brew info.
// Returns "(unknown)" on any error.
func installedVersion(name string) string {
	out, err := brew.InfoVersion(name)
	if err != nil || out == "" {
		return "(unknown)"
	}
	return out
}
