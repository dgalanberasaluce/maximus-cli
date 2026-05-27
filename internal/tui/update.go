package tui

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"maximus-cli/internal/apps"
	"maximus-cli/internal/brew"
	"maximus-cli/internal/db"
	"maximus-cli/internal/home"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
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
		m.homeList.SetSize(msg.Width, msg.Height)
		m.brewList.SetSize(msg.Width, msg.Height)
		m.appsList.SetSize(msg.Width, msg.Height)
		m.vscodeList.SetSize(msg.Width, msg.Height)
		m.viewport.SetWidth(msg.Width)
		m.viewport.SetHeight(msg.Height - 6)
		m = updatePreview(m)
		m = updateServicesViewports(m)
		m = updateVSCodeProfilePreview(m)
		m = updateVSCodeHistoryPreview(m)
		m = updateVSCodeDepsPreview(m)

	case errMsg:
		m.result = fmt.Sprintf("Error:\n%v", error(msg))
		m.state = stateResult
		m.viewport.SetContent(m.formatResult(m.result))
		m.viewport.GotoTop()
		return m, nil

	case resultMsg:
		m.result = msg.content
		m.state = stateResult
		m.viewport.SetContent(m.formatResult(msg.content))
		m.viewport.GotoTop()
		return m, nil

	case editorFinishedMsg:
		// Editor finished. Return to the list and refresh.
		m = applyDotfileFilter(m)
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

	case upgradePkgsMsg:
		m.upgradePkgs = msg.packages
		m.upgradeCursor = 0
		// Pre-select all packages.
		sel := make(map[int]bool, len(msg.packages))
		for i := range msg.packages {
			sel[i] = true
		}
		m.upgradeSelected = sel
		m.state = stateUpgradePkgs
		return m, nil

	case logsMsg:
		m.logEntries = msg.entries
		m.logTotal = msg.total
		m.state = stateBrewLogs
		return m, nil

	case versionMsg:
		m.versionItems = msg.packages
		m.versionCursor = 0
		m.versionFilter = ""
		m.versionInput.SetValue("")
		m.versionInputMode = false
		m = applyVersionFilter(m)
		m.state = stateBrewVersion
		return m, nil

	case dotfileMsg:
		m.dotfileItems = msg.entries
		m.dotfileCursor = 0
		m.dotfileFilter = ""
		m.dotfileInput.SetValue("")
		m.dotfileInputMode = false
		m = applyDotfileFilter(m)
		m = updatePreview(m)
		m.state = stateHomeDotfiles
		return m, nil

	case servicesMsg:
		m.servicesItems = msg.services
		m.servicesCursor = 0
		m.servicesFocusPanel = 0
		m.servicesConfirmMode = false
		m.servicesLogSize = ""
		m.state = stateBrewServices
		m = updateServicesViewports(m)
		var detailCmd tea.Cmd
		m, detailCmd = updateSelectedServiceDetails(m)
		return m, detailCmd

	case servicesLogsMsg:
		m.servicesLogsContent = msg.content
		m.servicesLogsStreaming = msg.streaming
		if m.servicesLogsFilterMode && m.servicesLogsFilter != "" {
			m.servicesLogsVP.SetContent(applyLogsFilter(msg.content, m.servicesLogsFilter))
		} else {
			m.servicesLogsVP.SetContent(msg.content)
		}
		if msg.scrollBottom {
			m.servicesLogsVP.GotoBottom()
		}
		return m, nil

	case servicesLogTickMsg:
		// New line appended during streaming
		if m.servicesLogsStreaming {
			if m.servicesLogsContent != "" {
				m.servicesLogsContent += "\n" + msg.line
			} else {
				m.servicesLogsContent = msg.line
			}
			display := m.servicesLogsContent
			if m.servicesLogsFilter != "" {
				display = applyLogsFilter(display, m.servicesLogsFilter)
			}
			m.servicesLogsVP.SetContent(display)
			m.servicesLogsVP.GotoBottom()
			return m, waitForLogLine(msg.lines)
		}
		return m, nil

	case servicesLogStreamDoneMsg:
		m.servicesLogsStreaming = false
		m.servicesStreamProc = nil
		return m, nil

	case servicesLogSizeMsg:
		m.servicesLogSize = msg.human
		// Refresh info panel with the new size
		var detailCmd2 tea.Cmd
		m, detailCmd2 = updateSelectedServiceDetails(m)
		return m, detailCmd2

	case servicesActionDoneMsg:
		// Action complete — refresh the services list
		m.servicesConfirmMode = false
		m.servicesConfirmAction = ""
		m.state = stateLoading
		m.loadingText = "Refreshing services..."
		return m, tea.Batch(m.spinner.Tick, fetchServicesCmd())

	case vscodeSummaryMsg:
		m.vscodeSummary = msg.summary
		m.vscodeLastRefreshAt = msg.lastRefresh
		m.state = stateVSCodeInfo
		return m, nil

	case vscodeProfilesMsg:
		m.vscodeProfiles = msg.profiles
		m.vscodeLastRefreshAt = msg.lastRefresh
		m.vscodeProfileCursor = 0
		m.vscodeProfileFocusPanel = false
		m.state = stateVSCodeProfiles
		m = updateVSCodeProfilePreview(m)
		return m, nil

	case vscodeProfilesReloadMsg:
		m.vscodeProfiles = msg.profiles
		m.vscodeLastRefreshAt = msg.lastRefresh
		if msg.prevSelectedLocID != "" {
			found := false
			for idx, p := range m.vscodeProfiles {
				if p.LocationID == msg.prevSelectedLocID {
					m.vscodeProfileCursor = idx
					found = true
					break
				}
			}
			if !found {
				if m.vscodeProfileCursor >= len(m.vscodeProfiles) {
					m.vscodeProfileCursor = len(m.vscodeProfiles) - 1
				}
				if m.vscodeProfileCursor < 0 {
					m.vscodeProfileCursor = 0
				}
			}
		} else {
			if m.vscodeProfileCursor >= len(m.vscodeProfiles) {
				m.vscodeProfileCursor = len(m.vscodeProfiles) - 1
			}
			if m.vscodeProfileCursor < 0 {
				m.vscodeProfileCursor = 0
			}
		}
		m = updateVSCodeProfilePreview(m)
		return m, nil

	case vscodeHistoryMsg:
		m.vscodeRefreshHistory = msg.logs
		m.vscodeHistoryCursor = 0
		m.vscodeHistoryExpanded = false
		m.state = stateVSCodeHistory
		m = updateVSCodeHistoryPreview(m)
		return m, nil

	case vscodeDepsMsg:
		m.vscodeDeps = msg.deps
		m.vscodeLastRefreshAt = msg.lastRefresh
		m.vscodeDepsCursor = 0
		m.vscodeDepsFocusPanel = false
		m.vscodeDepsInputMode = false
		m.vscodeDepsShowLong = false
		m.vscodeDepsFiltered = m.vscodeDeps
		m.state = stateVSCodeDeps
		m = updateVSCodeDepsPreview(m)
		return m, nil

	case vscodeRefreshDoneMsg:
		// Format a beautiful diff summary
		var sb strings.Builder
		sb.WriteString("\n  ")
		sb.WriteString(headerStyle.Render("Resumen de Refresco de VSCode"))
		sb.WriteString("\n\n")

		if !msg.diff.HasAnyChange {
			sb.WriteString("  ✓ Todo está al día. Sin diferencias respecto al refresco anterior.\n")
		} else {
			sb.WriteString("  " + warningStyle.Render("Cambios detectados desde el último refresco:"))
			sb.WriteString("\n\n")

			if msg.diff.VersionChanged {
				sb.WriteString(fmt.Sprintf("  - Versión: %s → %s\n", msg.diff.OldVersion, msg.diff.NewVersion))
			} else {
				sb.WriteString(fmt.Sprintf("  - Versión: %s (Sin cambios)\n", msg.diff.NewVersion))
			}

			if len(msg.diff.PathsAdded) > 0 {
				sb.WriteString(fmt.Sprintf("  - Paths añadidos: %s\n", strings.Join(msg.diff.PathsAdded, ", ")))
			}
			if len(msg.diff.PathsRemoved) > 0 {
				sb.WriteString(fmt.Sprintf("  - Paths eliminados: %s\n", strings.Join(msg.diff.PathsRemoved, ", ")))
			}
			if len(msg.diff.ProfilesAdded) > 0 {
				sb.WriteString(fmt.Sprintf("  - Perfiles añadidos: %s\n", strings.Join(msg.diff.ProfilesAdded, ", ")))
			}
			if len(msg.diff.ProfilesRemoved) > 0 {
				sb.WriteString(fmt.Sprintf("  - Perfiles eliminados: %s\n", strings.Join(msg.diff.ProfilesRemoved, ", ")))
			}
			if len(msg.diff.ExtChanges) > 0 {
				sb.WriteString("  - Extensiones:\n")
				for _, line := range msg.diff.ExtChanges {
					sb.WriteString(fmt.Sprintf("    * %s\n", line))
				}
			}
		}

		sb.WriteString("\n")
		sb.WriteString("  ✓ Refresco completado. Presiona q para regresar.")
		sb.WriteString("\n")

		m.result = sb.String()
		m.state = stateResult
		m.viewport.SetContent(m.formatResult(sb.String()))
		m.viewport.GotoTop()
		return m, nil

	case tea.KeyPressMsg:
		switch msg.String() {
		case "ctrl+c":
			return m, tea.Quit
		}


		// --- Services panel logic ---
		if m.state == stateBrewServices {
			n := len(m.servicesItems)
			var cmd tea.Cmd

			// ── Priority 1: Confirmation popup ──────────────────────────────────
			if m.servicesConfirmMode {
				switch msg.String() {
				case "enter", "y", "Y":
					// Execute the confirmed action
					action := m.servicesConfirmAction
					m.servicesConfirmMode = false
					m.servicesConfirmAction = ""
					if n > 0 {
						s := m.servicesItems[m.servicesCursor]
						// Kill stream if running
						if m.servicesStreamProc != nil {
							_ = m.servicesStreamProc.Kill()
							m.servicesStreamProc = nil
							m.servicesLogsStreaming = false
						}
						m.state = stateLoading
						m.loadingText = fmt.Sprintf("Running brew services %s %s...", action, s.Name)
						return m, tea.Batch(m.spinner.Tick, runServiceActionCmd(s.Name, action))
					}
				case "n", "N", "esc":
					m.servicesConfirmMode = false
					m.servicesConfirmAction = ""
				}
				return m, nil
			}

			// ── Priority 2: Log filter text input ────────────────────────────────
			if m.servicesLogsFilterMode {
				switch msg.String() {
				case "enter":
					m.servicesLogsFilter = m.servicesLogsInput.Value()
					m.servicesLogsFilterMode = false
					display := applyLogsFilter(m.servicesLogsContent, m.servicesLogsFilter)
					m.servicesLogsVP.SetContent(display)
					m.servicesLogsVP.GotoTop()
					return m, nil
				case "esc":
					m.servicesLogsFilter = ""
					m.servicesLogsInput.SetValue("")
					m.servicesLogsFilterMode = false
					m.servicesLogsVP.SetContent(m.servicesLogsContent)
					return m, nil
				default:
					m.servicesLogsInput, cmd = m.servicesLogsInput.Update(msg)
					return m, cmd
				}
			}

			// ── Priority 3: Panel-specific navigation ───────────────────────────
			if m.servicesFocusPanel == 1 {
				// Info panel focused
				switch msg.String() {
				case "tab":
					m.servicesFocusPanel = (m.servicesFocusPanel + 1) % 3
					return m, nil
				case "esc", "q":
					m.servicesFocusPanel = 0
					return m, nil
				default:
					m.servicesInfoVP, cmd = m.servicesInfoVP.Update(msg)
					return m, cmd
				}
			} else if m.servicesFocusPanel == 2 {
				// Logs panel focused — extra shortcuts
				switch msg.String() {
				case "tab":
					m.servicesFocusPanel = (m.servicesFocusPanel + 1) % 3
					return m, nil
				case "esc":
					m.servicesFocusPanel = 0
					return m, nil
				case "q":
					// Stop stream first if running, then go back to list focus
					if m.servicesLogsStreaming && m.servicesStreamProc != nil {
						_ = m.servicesStreamProc.Kill()
						m.servicesStreamProc = nil
						m.servicesLogsStreaming = false
					}
					m.servicesFocusPanel = 0
					return m, nil
				case "g":
					m.servicesLogsVP.GotoTop()
					return m, nil
				case "G":
					m.servicesLogsVP.GotoBottom()
					return m, nil
				case "p":
					// Toggle streaming pause
					if m.servicesLogsStreaming && m.servicesStreamProc != nil {
						_ = m.servicesStreamProc.Kill()
						m.servicesStreamProc = nil
						m.servicesLogsStreaming = false
					} else if n > 0 {
						// Restart stream
						s := m.servicesItems[m.servicesCursor]
						m.servicesLogsContent = ""
						m.servicesLogsVP.SetContent("Connecting to log stream...")
						var startCmd tea.Cmd
						m, startCmd = startLogStreamCmd(m, s)
						return m, startCmd
					}
					return m, nil
				case "/":
					m.servicesLogsFilterMode = true
					m.servicesLogsInput.Focus()
					return m, nil
				default:
					m.servicesLogsVP, cmd = m.servicesLogsVP.Update(msg)
					return m, cmd
				}
			}

			// ── Main list has focus ──────────────────────────────────────────────
			switch msg.String() {
			case "esc", "q":
				// Kill stream before leaving
				if m.servicesStreamProc != nil {
					_ = m.servicesStreamProc.Kill()
					m.servicesStreamProc = nil
					m.servicesLogsStreaming = false
				}
				m.state = stateBrewMenu
				return m, nil
			case "tab":
				m.servicesFocusPanel = (m.servicesFocusPanel + 1) % 3
				return m, nil
			case "up", "k":
				if m.servicesCursor > 0 {
					// Stop current stream before switching service
					if m.servicesStreamProc != nil {
						_ = m.servicesStreamProc.Kill()
						m.servicesStreamProc = nil
						m.servicesLogsStreaming = false
					}
					m.servicesCursor--
					m.servicesLogSize = ""
					return updateSelectedServiceDetails(m)
				}
			case "down", "j":
				if m.servicesCursor < n-1 {
					// Stop current stream before switching service
					if m.servicesStreamProc != nil {
						_ = m.servicesStreamProc.Kill()
						m.servicesStreamProc = nil
						m.servicesLogsStreaming = false
					}
					m.servicesCursor++
					m.servicesLogSize = ""
					return updateSelectedServiceDetails(m)
				}
			case "R":
				if m.servicesStreamProc != nil {
					_ = m.servicesStreamProc.Kill()
					m.servicesStreamProc = nil
					m.servicesLogsStreaming = false
				}
				m.state = stateLoading
				m.loadingText = "Refreshing services..."
				return m, tea.Batch(m.spinner.Tick, fetchServicesCmd())
			case "s":
				if n > 0 {
					m.servicesConfirmMode = true
					m.servicesConfirmAction = "start"
				}
			case "x":
				if n > 0 {
					m.servicesConfirmMode = true
					m.servicesConfirmAction = "stop"
				}
			case "r":
				if n > 0 {
					m.servicesConfirmMode = true
					m.servicesConfirmAction = "restart"
				}
			case "K":
				if n > 0 {
					m.servicesConfirmMode = true
					m.servicesConfirmAction = "kill"
				}
			case "Z":
				if n > 0 {
					s := m.servicesItems[m.servicesCursor]
					return m, calcLogSizeCmd(s)
				}
			}
			return m, nil
		}

		// --- VSCode Menu keys ---
		if m.state == stateVSCodeMenu {
			switch msg.String() {
			case "esc", "q":
				m.state = stateAppsMenu
				return m, nil
			case "enter":
				if i, ok := m.vscodeList.SelectedItem().(menuItem); ok {
					return m.dispatchVSCodeCmd(i.title)
				}
			}
			m.vscodeList, cmd = m.vscodeList.Update(msg)
			return m, cmd
		}

		// --- VSCode Summary keys ---
		if m.state == stateVSCodeInfo {
			switch msg.String() {
			case "esc", "q":
				m.state = stateVSCodeMenu
				return m, nil
			case "r":
				newM, refreshCmd := m.dispatchVSCodeCmd("Refresh")
				if typedM, ok := newM.(Model); ok {
					typedM.returnState = stateVSCodeInfo
					return typedM, refreshCmd
				}
				return newM, refreshCmd
			}
			return m, nil
		}

		// --- VSCode Profiles keys ---
		if m.state == stateVSCodeProfiles {
			n := len(m.vscodeProfiles)
			var cmd tea.Cmd
			switch msg.String() {
			case "esc", "q":
				if m.vscodeProfileFocusPanel {
					m.vscodeProfileFocusPanel = false
					return m, nil
				}
				m.state = stateVSCodeMenu
				return m, nil
			case "tab":
				m.vscodeProfileFocusPanel = !m.vscodeProfileFocusPanel
				return m, nil
			case "up", "k":
				if m.vscodeProfileFocusPanel {
					m.vscodeProfileVP, cmd = m.vscodeProfileVP.Update(msg)
					return m, cmd
				} else {
					if m.vscodeProfileCursor > 0 {
						m.vscodeProfileCursor--
						m = updateVSCodeProfilePreview(m)
					}
				}
			case "down", "j":
				if m.vscodeProfileFocusPanel {
					m.vscodeProfileVP, cmd = m.vscodeProfileVP.Update(msg)
					return m, cmd
				} else {
					if m.vscodeProfileCursor < n-1 {
						m.vscodeProfileCursor++
						m = updateVSCodeProfilePreview(m)
					}
				}
			case "a":
				m.vscodeShowArchived = !m.vscodeShowArchived
				database := m.database
				showArchived := m.vscodeShowArchived
				var selectedLocID string
				if m.vscodeProfileCursor >= 0 && m.vscodeProfileCursor < len(m.vscodeProfiles) {
					selectedLocID = m.vscodeProfiles[m.vscodeProfileCursor].LocationID
				}
				bgCmd := func() tea.Msg {
					profs, err := apps.LoadVSCodeProfiles(database, showArchived)
					if err != nil {
						return errMsg(err)
					}
					ts, _ := database.GetLastVSCodeRefreshAt()
					return vscodeProfilesReloadMsg{profiles: profs, lastRefresh: ts, prevSelectedLocID: selectedLocID}
				}
				return m, bgCmd
			default:
				if m.vscodeProfileFocusPanel {
					m.vscodeProfileVP, cmd = m.vscodeProfileVP.Update(msg)
					return m, cmd
				}
			}
			return m, nil
		}

		// --- VSCode History keys ---
		if m.state == stateVSCodeHistory {
			n := len(m.vscodeRefreshHistory)
			var cmd tea.Cmd
			switch msg.String() {
			case "esc", "q":
				if m.vscodeHistoryExpanded {
					m.vscodeHistoryExpanded = false
					return m, nil
				}
				m.state = stateVSCodeMenu
				return m, nil
			case "space":
				if n > 0 {
					m.vscodeHistoryExpanded = !m.vscodeHistoryExpanded
				}
				return m, nil
			case "up", "k":
				if m.vscodeHistoryExpanded {
					m.vscodeHistoryDetailVP, cmd = m.vscodeHistoryDetailVP.Update(msg)
					return m, cmd
				} else {
					if m.vscodeHistoryCursor > 0 {
						m.vscodeHistoryCursor--
						m = updateVSCodeHistoryPreview(m)
					}
				}
			case "down", "j":
				if m.vscodeHistoryExpanded {
					m.vscodeHistoryDetailVP, cmd = m.vscodeHistoryDetailVP.Update(msg)
					return m, cmd
				} else {
					if m.vscodeHistoryCursor < n-1 {
						m.vscodeHistoryCursor++
						m = updateVSCodeHistoryPreview(m)
					}
				}
			default:
				if m.vscodeHistoryExpanded {
					m.vscodeHistoryDetailVP, cmd = m.vscodeHistoryDetailVP.Update(msg)
					return m, cmd
				}
			}
			return m, nil
		}

		// --- VSCode Dependencies keys ---
		if m.state == stateVSCodeDeps {
			n := len(m.vscodeDepsFiltered)
			var cmd tea.Cmd
			if m.vscodeDepsInputMode {
				switch msg.String() {
				case "enter", "esc":
					m.vscodeDepsInputMode = false
					m.vscodeDepsInput.Blur()
					return m, nil
				default:
					m.vscodeDepsInput, cmd = m.vscodeDepsInput.Update(msg)
					m.vscodeDepsFiltered = nil
					filter := strings.ToLower(m.vscodeDepsInput.Value())
					for _, d := range m.vscodeDeps {
						if strings.Contains(strings.ToLower(d.ID), filter) {
							m.vscodeDepsFiltered = append(m.vscodeDepsFiltered, d)
						}
					}
					if m.vscodeDepsCursor >= len(m.vscodeDepsFiltered) {
						m.vscodeDepsCursor = len(m.vscodeDepsFiltered) - 1
					}
					if m.vscodeDepsCursor < 0 {
						m.vscodeDepsCursor = 0
					}
					m = updateVSCodeDepsPreview(m)
					return m, cmd
				}
			}

			switch msg.String() {
			case "esc", "q":
				if m.vscodeDepsFocusPanel {
					m.vscodeDepsFocusPanel = false
					return m, nil
				}
				m.state = stateVSCodeMenu
				return m, nil
			case "/":
				if !m.vscodeDepsFocusPanel {
					m.vscodeDepsInputMode = true
					m.vscodeDepsInput.Focus()
					m.vscodeDepsInput.SetValue("")
					return m, nil
				}
			case "v":
				if !m.vscodeDepsFocusPanel {
					m.vscodeDepsShowLong = !m.vscodeDepsShowLong
					m = updateVSCodeDepsPreview(m)
					return m, nil
				}
			case "tab":
				m.vscodeDepsFocusPanel = !m.vscodeDepsFocusPanel
				return m, nil
			case "up", "k":
				if m.vscodeDepsFocusPanel {
					m.vscodeDepsVP, cmd = m.vscodeDepsVP.Update(msg)
					return m, cmd
				} else {
					if m.vscodeDepsCursor > 0 {
						m.vscodeDepsCursor--
						m = updateVSCodeDepsPreview(m)
					}
				}
			case "down", "j":
				if m.vscodeDepsFocusPanel {
					m.vscodeDepsVP, cmd = m.vscodeDepsVP.Update(msg)
					return m, cmd
				} else {
					if m.vscodeDepsCursor < n-1 {
						m.vscodeDepsCursor++
						m = updateVSCodeDepsPreview(m)
					}
				}
			default:
				if m.vscodeDepsFocusPanel {
					m.vscodeDepsVP, cmd = m.vscodeDepsVP.Update(msg)
					return m, cmd
				}
			}
			return m, nil
		}

		// --- Dotfiles table logic ---
		if m.state == stateHomeDotfiles {
			// Priority 1: delete confirmation input mode
			if m.dotfileDeleteMode {
				switch msg.String() {
				case "enter":
					if m.dotfileCursor >= 0 && m.dotfileCursor < len(m.dotfileFiltered) {
						selected := m.dotfileFiltered[m.dotfileCursor]
						if m.dotfileDeleteInput.Value() == selected.Name {
							// Perform filesystem deletion
							homeDir, err := os.UserHomeDir()
							if err == nil {
								fullPath := filepath.Join(homeDir, selected.Name)
								_ = os.RemoveAll(fullPath)
							}
							// Delete from DB
							_ = m.database.DeleteDotfile(selected.Name)

							// Remove from in-memory list
							for idx, item := range m.dotfileItems {
								if item.Name == selected.Name {
									m.dotfileItems = append(m.dotfileItems[:idx], m.dotfileItems[idx+1:]...)
									break
								}
							}

							m.dotfileDeleteMode = false
							m.dotfileCursor = 0
							m = applyDotfileFilter(m)
							m = updatePreview(m)
							return m, nil
						}
					}
				case "esc":
					m.dotfileDeleteMode = false
					return m, nil
				}
				m.dotfileDeleteInput, cmd = m.dotfileDeleteInput.Update(msg)
				return m, cmd
			}

			// Priority 2: tool-edit input mode
			if m.dotfileToolEditMode {
				switch msg.String() {
				case "enter":
					newTool := m.dotfileToolInput.Value()
					// Find the selected item in the full list by cursor position
					if m.dotfileCursor >= 0 && m.dotfileCursor < len(m.dotfileFiltered) {
						name := m.dotfileFiltered[m.dotfileCursor].Name
						_ = m.database.UpdateDotfileTool(name, newTool)
						// Update in-memory copies so the UI reflects the change instantly.
						for i, e := range m.dotfileItems {
							if e.Name == name {
								m.dotfileItems[i].Tool = newTool
								m.dotfileItems[i].ToolManual = true
								break
							}
						}
					}
					m.dotfileToolEditMode = false
					m = applyDotfileFilter(m)
					m = updatePreview(m)
					return m, nil
				case "esc":
					m.dotfileToolEditMode = false
					return m, nil
				}
				m.dotfileToolInput, cmd = m.dotfileToolInput.Update(msg)
				return m, cmd
			}

			// Priority 3: text filter input mode
			if m.dotfileInputMode {
				switch msg.String() {
				case "enter":
					m.dotfileFilter = m.dotfileInput.Value()
					m.dotfileInputMode = false
					m.dotfileCursor = 0
					m = applyDotfileFilter(m)
					m = updatePreview(m)
					return m, nil
				case "esc":
					m.dotfileInputMode = false
					return m, nil
				}
				m.dotfileInput, cmd = m.dotfileInput.Update(msg)
				return m, cmd
			}

			// Priority 4: navigation mode
			// Toggle focus with Tab
			if msg.String() == "tab" {
				m.dotfilePreviewFocused = !m.dotfilePreviewFocused
				return m, nil
			}

			// When preview panel is focused:
			if m.dotfilePreviewFocused {
				switch msg.String() {
				case "e":
					// Open fullPath file in EDITOR
					if m.dotfileCursor >= 0 && m.dotfileCursor < len(m.dotfileFiltered) {
						selected := m.dotfileFiltered[m.dotfileCursor]
						if !selected.IsDir {
							homeDir, err := os.UserHomeDir()
							if err == nil {
								fullPath := filepath.Join(homeDir, selected.Name)
								editor := os.Getenv("EDITOR")
								if editor == "" {
									editor = "vim"
								}
								cmd := exec.Command(editor, fullPath)
								return m, tea.ExecProcess(cmd, func(err error) tea.Msg {
									return editorFinishedMsg{err}
								})
							}
						}
					}
					return m, nil
				case "esc", "q":
					m.dotfilePreviewFocused = false
					return m, nil
				default:
					m.previewViewport, cmd = m.previewViewport.Update(msg)
					return m, cmd
				}
			}

			// When main table has focus:
			n := len(m.dotfileFiltered)
			switch msg.String() {
			case "up", "k":
				if m.dotfileCursor > 0 {
					m.dotfileCursor--
					m = updatePreview(m)
				}
			case "down", "j":
				if m.dotfileCursor < n-1 {
					m.dotfileCursor++
					m = updatePreview(m)
				}
			case "left", "h":
				m.dotfileSortField = (m.dotfileSortField + 4) % 5
				m.dotfileCursor = 0
				m = applyDotfileFilter(m)
				m = updatePreview(m)
			case "right", "l":
				m.dotfileSortField = (m.dotfileSortField + 1) % 5
				m.dotfileCursor = 0
				m = applyDotfileFilter(m)
				m = updatePreview(m)
			case "/":
				m.dotfileInput.SetValue(m.dotfileFilter)
				m.dotfileInput.Focus()
				m.dotfileInputMode = true
			case "t":
				// Cycle: All -> Files -> Dirs -> All
				m.dotfileTypeFilter = (m.dotfileTypeFilter + 1) % 3
				m.dotfileCursor = 0
				m = applyDotfileFilter(m)
				m = updatePreview(m)
			case "e":
				// Enter tool-edit mode for the highlighted row.
				if m.dotfileCursor >= 0 && m.dotfileCursor < len(m.dotfileFiltered) {
					current := m.dotfileFiltered[m.dotfileCursor].Tool
					m.dotfileToolInput.SetValue(current)
					m.dotfileToolInput.Focus()
					m.dotfileToolEditMode = true
				}
			case "p":
				// Open selected file with `less`. No-op for directories.
				if m.dotfileCursor >= 0 && m.dotfileCursor < len(m.dotfileFiltered) {
					selected := m.dotfileFiltered[m.dotfileCursor]
					if !selected.IsDir {
						homeDir, err := os.UserHomeDir()
						if err == nil {
							fullPath := filepath.Join(homeDir, selected.Name)
							lessCmd := exec.Command("less", fullPath)
							return m, tea.ExecProcess(lessCmd, func(err error) tea.Msg {
								return editorFinishedMsg{err}
							})
						}
					}
				}
			case "d":
				// Enter deletion confirmation mode for the highlighted row.
				if m.dotfileCursor >= 0 && m.dotfileCursor < len(m.dotfileFiltered) {
					m.dotfileDeleteInput.SetValue("")
					m.dotfileDeleteInput.Focus()
					m.dotfileDeleteMode = true
				}
			case "r":
				m.dotfileFilter = ""
				m.dotfileInput.SetValue("")
				m.dotfileTypeFilter = typeFilterAll
				m.dotfileCursor = 0
				m.dotfileSortField = sortDFByName
				m.dotfileSortAsc = true
				m = applyDotfileFilter(m)
				m = updatePreview(m)
			case "s", "o":
				m.dotfileSortAsc = !m.dotfileSortAsc
				m.dotfileCursor = 0
				m = applyDotfileFilter(m)
				m = updatePreview(m)
			case "esc", "q":
				m.state = stateHomeMenu
			}
			return m, nil
		}

		// --- Version table logic ---
		if m.state == stateBrewVersion {
			if m.versionInputMode {
				switch msg.String() {
				case "enter":
					m.versionFilter = m.versionInput.Value()
					m.versionInputMode = false
					m.versionCursor = 0
					m = applyVersionFilter(m)
					return m, nil
				case "esc":
					m.versionInputMode = false
					return m, nil
				}
				// Pass other keys to the textinput component.
				m.versionInput, cmd = m.versionInput.Update(msg)
				return m, cmd
			}

			// Navigation mode (not input mode)
			n := len(m.versionFiltered)
			switch msg.String() {
			case "up", "k":
				if m.versionCursor > 0 {
					m.versionCursor--
				}
			case "down", "j":
				if m.versionCursor < n-1 {
					m.versionCursor++
				}
			case "left", "h":
				// Move sort column left.
				m.versionSortField = (m.versionSortField + 3) % 4
				m.versionCursor = 0
				m = applyVersionFilter(m)
			case "right", "l":
				// Move sort column right.
				m.versionSortField = (m.versionSortField + 1) % 4
				m.versionCursor = 0
				m = applyVersionFilter(m)
			case "/":
				m.versionInput.SetValue(m.versionFilter)
				m.versionInput.Focus()
				m.versionInputMode = true
			case "r":
				m.versionFilter = ""
				m.versionInput.SetValue("")
				m.versionCursor = 0
				m.versionSortField = sortByName
				m.versionSortAsc = true
				m = applyVersionFilter(m)
			case "s", "o":
				// Toggle sort order.
				m.versionSortAsc = !m.versionSortAsc
				m.versionCursor = 0
				m = applyVersionFilter(m)
			case "esc", "q":
				m.state = stateBrewMenu
			}
			return m, nil
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
			case "space":
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

		// --- Upgrade packages screen ---
		if m.state == stateUpgradePkgs {
			n := len(m.upgradePkgs)
			switch msg.String() {
			case "up", "k":
				if m.upgradeCursor > 0 {
					m.upgradeCursor--
				}
			case "down", "j":
				if m.upgradeCursor < n-1 {
					m.upgradeCursor++
				}
			case "space":
				if m.upgradeSelected == nil {
					m.upgradeSelected = make(map[int]bool)
				}
				m.upgradeSelected[m.upgradeCursor] = !m.upgradeSelected[m.upgradeCursor]
			case "a":
				allSelected := len(m.upgradeSelected) == n && n > 0
				sel := make(map[int]bool, n)
				if !allSelected {
					for i := range n {
						sel[i] = true
					}
				}
				m.upgradeSelected = sel
			case "enter":
				var chosen []brew.DiffResult
				for i, p := range m.upgradePkgs {
					if m.upgradeSelected[i] {
						chosen = append(chosen, p)
					}
				}
				if len(chosen) == 0 {
					m.state = stateBrewMenu
					return m, nil
				}
				database := m.database
				m.state = stateLoading
				m.loadingText = fmt.Sprintf("Upgrading %d package(s)...", len(chosen))
				return m, tea.Batch(m.spinner.Tick, upgradePkgsCmd(chosen, database))
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
				m.state = m.returnState
				return m, nil
			case stateBrewMenu, stateHomeMenu, stateAppsMenu:
				m.state = stateMainMenu
				return m, nil
			case stateHomeDotfiles:
				m.state = stateHomeMenu
				return m, nil
			case stateVSCodeInfo, stateVSCodeMenu, stateVSCodeProfiles, stateVSCodeHistory:
				m.state = stateVSCodeMenu
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
					} else if i.title == "Home" {
						m.homeList.SetSize(m.width, m.height)
						m.state = stateHomeMenu
						return m, nil
					} else if i.title == "Applications" {
						m.appsList.SetSize(m.width, m.height)
						m.state = stateAppsMenu
						return m, nil
					}
				}
			case stateBrewMenu:
				if i, ok := m.brewList.SelectedItem().(menuItem); ok {
					return m.dispatchBrewCmd(i.title)
				}
			case stateHomeMenu:
				if i, ok := m.homeList.SelectedItem().(menuItem); ok {
					return m.dispatchHomeCmd(i.title)
				}
			case stateAppsMenu:
				if i, ok := m.appsList.SelectedItem().(menuItem); ok {
					return m.dispatchAppsCmd(i.title)
				}
			case stateVSCodeMenu:
				if i, ok := m.vscodeList.SelectedItem().(menuItem); ok {
					return m.dispatchVSCodeCmd(i.title)
				}
			}
		}
	}

	// Delegate navigation updates to the active list/component.
	switch m.state {
	case stateMainMenu:
		m.mainList, cmd = m.mainList.Update(msg)
		cmds = append(cmds, cmd)
	case stateHomeMenu:
		m.homeList, cmd = m.homeList.Update(msg)
		cmds = append(cmds, cmd)
	case stateBrewMenu:
		m.brewList, cmd = m.brewList.Update(msg)
		cmds = append(cmds, cmd)
	case stateAppsMenu:
		m.appsList, cmd = m.appsList.Update(msg)
		cmds = append(cmds, cmd)
	case stateVSCodeMenu:
		m.vscodeList, cmd = m.vscodeList.Update(msg)
		cmds = append(cmds, cmd)
	case stateVSCodeProfiles:
		m.vscodeProfileVP, cmd = m.vscodeProfileVP.Update(msg)
		cmds = append(cmds, cmd)
	case stateLoading:
		m.spinner, cmd = m.spinner.Update(msg)
		cmds = append(cmds, cmd)
	case stateResult:
		m.viewport, cmd = m.viewport.Update(msg)
		cmds = append(cmds, cmd)
	case stateBrewVersion:
		if m.versionInputMode {
			m.versionInput, cmd = m.versionInput.Update(msg)
			cmds = append(cmds, cmd)
		}
	case stateHomeDotfiles:
		if m.dotfileToolEditMode {
			m.dotfileToolInput, cmd = m.dotfileToolInput.Update(msg)
			cmds = append(cmds, cmd)
		} else if m.dotfileInputMode {
			m.dotfileInput, cmd = m.dotfileInput.Update(msg)
			cmds = append(cmds, cmd)
		}
	case stateBrewServices:
		if m.servicesFocusPanel == 1 {
			m.servicesInfoVP, cmd = m.servicesInfoVP.Update(msg)
			cmds = append(cmds, cmd)
		} else if m.servicesFocusPanel == 2 {
			m.servicesLogsVP, cmd = m.servicesLogsVP.Update(msg)
			cmds = append(cmds, cmd)
		}
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
	m.returnState = stateBrewMenu

	switch title {
	case "Services":
		m.state = stateLoading
		m.loadingText = "Listing services..."
		m.servicesCursor = 0
		m.servicesFocusPanel = 0
		return m, tea.Batch(m.spinner.Tick, fetchServicesCmd())

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

	case "Upgrade Package(s)":
		m.state = stateLoading
		m.loadingText = "Checking outdated packages..."
		bgCmd := func() tea.Msg {
			results, err := brew.SmartDiff(brewfile)
			if err != nil {
				return errMsg(err)
			}
			if len(results) == 0 {
				return resultMsg{content: "✓ Everything in your Brewfile is up to date!"}
			}
			return upgradePkgsMsg{packages: results}
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

	case "Version":
		m.state = stateLoading
		m.loadingText = "Loading installed versions..."
		brewfile := m.brewfile
		database := m.database
		bgCmd := func() tea.Msg {
			pkgs, err := brew.ListVersions(brewfile, database)
			if err != nil {
				return errMsg(err)
			}
			return versionMsg{packages: pkgs}
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

// versionMsg carries the fetched package list back to the model.
type versionMsg struct {
	packages []brew.PackageVersion
}

// unstagedMsg carries the list of unstaged packages back to the model.
type unstagedMsg struct {
	packages []brew.UnstagedPackage
}

// upgradePkgsMsg carries the list of outdated packages back to the model.
type upgradePkgsMsg struct {
	packages []brew.DiffResult
}

// upgradePkgsCmd executes brew upgrade on the selected packages and logs it in the database.
func upgradePkgsCmd(pkgs []brew.DiffResult, database *db.DB) tea.Cmd {
	return func() tea.Msg {
		names := make([]string, len(pkgs))
		for i, p := range pkgs {
			names[i] = p.Name
		}
		out, err := brew.UpgradePackages(names)
		if err != nil {
			return errMsg(err)
		}
		for _, p := range pkgs {
			_ = database.LogUpgrade(p.Name, p.CurrentVersion, p.LatestVersion)
		}
		return resultMsg{content: out}
	}
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

// applyVersionFilter applies the current filter and sort to m.versionItems,
// storing the result in m.versionFiltered.
func applyVersionFilter(m Model) Model {
	filter := strings.ToLower(m.versionFilter)
	var filtered []brew.PackageVersion
	for _, p := range m.versionItems {
		if filter == "" || strings.Contains(strings.ToLower(p.Name), filter) ||
			strings.Contains(strings.ToLower(p.Kind), filter) {
			filtered = append(filtered, p)
		}
	}

	// Sort.
	sort.SliceStable(filtered, func(i, j int) bool {
		var less bool
		switch m.versionSortField {
		case sortByKind:
			less = filtered[i].Kind < filtered[j].Kind
		case sortByMetaDate:
			less = filtered[i].MetadataDate.Before(filtered[j].MetadataDate)
		case sortByInstallDate:
			less = filtered[i].InstallDate.Before(filtered[j].InstallDate)
		default: // sortByName
			less = filtered[i].Name < filtered[j].Name
		}
		if m.versionSortAsc {
			return less
		}
		return !less
	})

	m.versionFiltered = filtered
	return m
}

// formatResult prepares a string for display in the results viewport.
func (m Model) formatResult(content string) string {
	wrapWidth := m.width - 4
	if wrapWidth < 40 {
		wrapWidth = 80
	}
	formatted := lipgloss.NewStyle().Width(wrapWidth).Render(content)
	formatted = strings.ReplaceAll(formatted, "Error:", "\n⚠  Error:\n")
	return "\n" + formatted
}

// dotfileMsg carries the list of dotfiles back to the model.
type dotfileMsg struct {
	entries []db.DotfileEntry
}

// dispatchHomeCmd maps a selected home menu item to the appropriate background command.
func (m Model) dispatchHomeCmd(title string) (tea.Model, tea.Cmd) {
	database := m.database
	m.returnState = stateHomeMenu

	switch title {
	case "Explain":
		m.state = stateLoading
		m.loadingText = "Loading dotfiles information..."
		bgCmd := func() tea.Msg {
			// Check if database already has entries
			count, err := database.CountDotfiles("")
			if err != nil {
				return errMsg(err)
			}
			var entries []db.DotfileEntry
			if count == 0 {
				// Auto-scan if empty
				scanRes, err := home.ScanDotfiles(database)
				if err != nil {
					return errMsg(err)
				}
				entries = scanRes.Entries
			} else {
				// Fetch from DB
				entries, err = database.GetDotfiles("", "name", true, -1, 0)
			}
			if err != nil {
				return errMsg(err)
			}
			return dotfileMsg{entries: entries}
		}
		return m, tea.Batch(m.spinner.Tick, bgCmd)

	case "Reload":
		m.state = stateLoading
		m.loadingText = "Scanning $HOME and refreshing database..."
		bgCmd := func() tea.Msg {
			scanRes, err := home.ScanDotfiles(database)
			if err != nil {
				return errMsg(err)
			}

			// Format detailed summary of changes!
			var sb strings.Builder
			sb.WriteString("\n  ")
			sb.WriteString(headerStyle.Render("Home Database Reload Summary"))
			sb.WriteString("\n\n")

			if !scanRes.LastScan.IsZero() {
				sb.WriteString(fmt.Sprintf("  Last scan was at: %s\n\n", scanRes.LastScan.Local().Format("2006-01-02 15:04:05")))
			} else {
				sb.WriteString("  Last scan was at: Never (first reload)\n\n")
			}

			sb.WriteString(fmt.Sprintf("  - Added files/dirs:   %d\n", len(scanRes.Added)))
			for _, name := range scanRes.Added {
				sb.WriteString(fmt.Sprintf("    + %s\n", name))
			}

			sb.WriteString(fmt.Sprintf("  - Updated files/dirs: %d\n", len(scanRes.Updated)))
			for _, name := range scanRes.Updated {
				sb.WriteString(fmt.Sprintf("    ~ %s\n", name))
			}

			sb.WriteString(fmt.Sprintf("  - Deleted files/dirs: %d\n", len(scanRes.Deleted)))
			for _, name := range scanRes.Deleted {
				sb.WriteString(fmt.Sprintf("    - %s\n", name))
			}

			sb.WriteString("\n")
			sb.WriteString("  " + warningStyle.Render("✓ Reload completed, press q to go back."))
			sb.WriteString("\n")

			return resultMsg{content: sb.String()}
		}
		return m, tea.Batch(m.spinner.Tick, bgCmd)

	default:
		m.result = fmt.Sprintf("Unknown command: %q", title)
		m.state = stateResult
		return m, nil
	}
}

// applyDotfileFilter applies the current filter and sort to m.dotfileItems,
// storing the result in m.dotfileFiltered.
func applyDotfileFilter(m Model) Model {
	textFilter := strings.ToLower(m.dotfileFilter)
	var filtered []db.DotfileEntry
	for _, p := range m.dotfileItems {
		// Apply type filter first.
		switch m.dotfileTypeFilter {
		case typeFilterFiles:
			if p.IsDir {
				continue
			}
		case typeFilterDirs:
			if !p.IsDir {
				continue
			}
		}
		// Apply text filter (name or tool).
		if textFilter != "" &&
			!strings.Contains(strings.ToLower(p.Name), textFilter) &&
			!strings.Contains(strings.ToLower(p.Tool), textFilter) {
			continue
		}
		filtered = append(filtered, p)
	}

	// Sort.
	sort.SliceStable(filtered, func(i, j int) bool {
		var less bool
		switch m.dotfileSortField {
		case sortDFByType:
			// Dirs first when ascending.
			less = filtered[i].IsDir && !filtered[j].IsDir
		case sortDFByTool:
			less = filtered[i].Tool < filtered[j].Tool
		case sortDFByModified:
			less = filtered[i].ModifiedAt.Before(filtered[j].ModifiedAt)
		case sortDFByCreated:
			less = filtered[i].CreatedAt.Before(filtered[j].CreatedAt)
		default: // sortDFByName
			less = filtered[i].Name < filtered[j].Name
		}
		if m.dotfileSortAsc {
			return less
		}
		return !less
	})

	m.dotfileFiltered = filtered
	return m
}

type editorFinishedMsg struct {
	err error
}

func updatePreview(m Model) Model {
	// Calculate maxRows exactly like in view.go
	const chromeHeight = 12
	maxRows := m.height - chromeHeight
	if maxRows < 1 {
		maxRows = 1
	}

	const minTableWidth = 86
	previewWidth := m.width - minTableWidth - 4
	if previewWidth < 25 {
		previewWidth = 25
	}

	m.previewViewport.SetWidth(previewWidth - 2)
	m.previewViewport.SetHeight(maxRows - 2) // space for the header title

	if len(m.dotfileFiltered) == 0 || m.dotfileCursor < 0 || m.dotfileCursor >= len(m.dotfileFiltered) {
		m.previewViewport.SetContent("")
		return m
	}
	selected := m.dotfileFiltered[m.dotfileCursor]
	_, previewText := home.GetPreview(selected.Name, selected.IsDir)

	// wrap text to fit previewWidth minus padding/borders
	wrapped := WordWrap(previewText, previewWidth-4)
	m.previewViewport.SetContent(wrapped)
	m.previewViewport.GotoTop()
	return m
}

// ─── Services message types ────────────────────────────────────────────────

type servicesMsg struct {
	services []brew.Service
}

type servicesLogsMsg struct {
	content      string
	streaming    bool
	scrollBottom bool
}

type servicesLogTickMsg struct {
	line  string
	lines chan string // channel to continue reading from
}

type servicesLogStreamDoneMsg struct{}

type servicesActionDoneMsg struct {
	output string
}

type servicesLogSizeMsg struct {
	size  int64
	human string
}

// ─── Services commands ──────────────────────────────────────────────────────

func fetchServicesCmd() tea.Cmd {
	return func() tea.Msg {
		pkgs, err := brew.ListServices()
		if err != nil {
			return errMsg(err)
		}
		return servicesMsg{services: pkgs}
	}
}

func runServiceActionCmd(name, action string) tea.Cmd {
	return func() tea.Msg {
		out, err := brew.ServiceAction(name, action)
		if err != nil {
			return errMsg(err)
		}
		return servicesActionDoneMsg{output: out}
	}
}

func calcLogSizeCmd(s brew.Service) tea.Cmd {
	return func() tea.Msg {
		size, human := brew.ServiceLogSize(s)
		return servicesLogSizeMsg{size: size, human: human}
	}
}

// startLogStreamCmd launches a tail -f process on the service log and returns
// the model with servicesStreamProc set, plus the first Cmd to begin reading.
func startLogStreamCmd(m Model, s brew.Service) (Model, tea.Cmd) {
	logFile := brew.ServiceLogFile(s)
	if logFile == "" {
		m.servicesLogsVP.SetContent("No log file found for streaming.")
		return m, nil
	}

	cmd := exec.Command("tail", "-n", "50", "-f", logFile)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		m.servicesLogsVP.SetContent(fmt.Sprintf("Error opening log pipe: %v", err))
		return m, nil
	}
	if err := cmd.Start(); err != nil {
		m.servicesLogsVP.SetContent(fmt.Sprintf("Error starting tail: %v", err))
		return m, nil
	}

	m.servicesStreamProc = cmd.Process
	m.servicesLogsStreaming = true
	m.servicesLogsContent = ""

	lines := make(chan string, 256)

	// Goroutine: read lines from tail and push them into the channel
	go func() {
		defer close(lines)
		buf := make([]byte, 4096)
		var partial string
		for {
			n, err := stdout.Read(buf)
			if n > 0 {
				chunk := partial + string(buf[:n])
				ls := strings.Split(chunk, "\n")
				partial = ls[len(ls)-1]
				for _, l := range ls[:len(ls)-1] {
					lines <- l
				}
			}
			if err != nil {
				if partial != "" {
					lines <- partial
				}
				return
			}
		}
	}()

	// Initial tick to start reading
	tickcmd := waitForLogLine(lines)
	return m, tickcmd
}

// waitForLogLine blocks until a new line arrives on the channel and returns a
// servicesLogTickMsg so the Bubble Tea runtime wakes up the model.
func waitForLogLine(lines chan string) tea.Cmd {
	return func() tea.Msg {
		line, ok := <-lines
		if !ok {
			return servicesLogStreamDoneMsg{}
		}
		return servicesLogTickMsg{line: line, lines: lines}
	}
}

// applyLogsFilter filters multi-line log content keeping only matching lines.
func applyLogsFilter(content, filter string) string {
	if filter == "" {
		return content
	}
	fl := strings.ToLower(filter)
	var out []string
	for _, line := range strings.Split(content, "\n") {
		if strings.Contains(strings.ToLower(line), fl) {
			out = append(out, line)
		}
	}
	if len(out) == 0 {
		return fmt.Sprintf("(no lines match filter %q)", filter)
	}
	return strings.Join(out, "\n")
}

// ─── Viewport sizing ────────────────────────────────────────────────────────

func updateServicesViewports(m Model) Model {
	// Total height for content
	contentH := m.height - 5
	if contentH < 10 {
		contentH = 10
	}

	// Left column width
	leftColW := 22
	rightColW := m.width - leftColW - 4
	if rightColW < 20 {
		rightColW = 20
	}

	infoH := (contentH * 60) / 100
	logsH := contentH - infoH - 2
	if infoH < 3 {
		infoH = 3
	}
	if logsH < 3 {
		logsH = 3
	}

	m.servicesInfoVP.SetWidth(rightColW - 2)
	m.servicesInfoVP.SetHeight(infoH - 2)

	m.servicesLogsVP.SetWidth(rightColW - 2)
	m.servicesLogsVP.SetHeight(logsH - 2)

	return m
}

// ─── VSCode helpers (from main) ──────────────────────────────────────────────

// vscodeInfoMsg carries the gathered VSCode summary back to the model.
type vscodeInfoMsg struct {
	summary apps.VSCodeSummary
}

// dispatchAppsCmd handles menu selection in the Aplicaciones submenu.
func (m Model) dispatchAppsCmd(title string) (tea.Model, tea.Cmd) {
	m.returnState = stateAppsMenu

	switch title {
	case "VSCode":
		m.state = stateVSCodeMenu
		m.vscodeList.SetSize(m.width, m.height)
		return m, nil

	default:
		m.result = fmt.Sprintf("Unknown application command: %q", title)
		m.state = stateResult
		return m, nil
	}
}

// Msg structures for VSCode
type vscodeSummaryMsg struct {
	summary     apps.VSCodeSummary
	lastRefresh time.Time
}

type vscodeProfilesMsg struct {
	profiles    []apps.VSCodeProfile
	lastRefresh time.Time
}

type vscodeProfilesReloadMsg struct {
	profiles          []apps.VSCodeProfile
	lastRefresh       time.Time
	prevSelectedLocID string
}

type vscodeHistoryMsg struct {
	logs []db.VSCodeRefreshLogRow
}

type vscodeRefreshDoneMsg struct {
	diff apps.VSCodeDiff
}

type vscodeDepsMsg struct {
	deps        []apps.VSCodeExtAgg
	lastRefresh time.Time
}

// dispatchVSCodeCmd handles choices inside the VSCode submenu
func (m Model) dispatchVSCodeCmd(title string) (tea.Model, tea.Cmd) {
	database := m.database
	m.returnState = stateVSCodeMenu

	switch title {
	case "Summary":
		m.state = stateLoading
		m.loadingText = "Loading VSCode summary..."
		bgCmd := func() tea.Msg {
			ts, _ := database.GetLastVSCodeRefreshAt()
			summary, err := apps.LoadVSCodeSummary(database)
			if err != nil {
				// Auto-refresh if empty
				_, err = apps.ScanVSCode(database)
				if err != nil {
					return errMsg(err)
				}
				summary, err = apps.LoadVSCodeSummary(database)
				if err != nil {
					return errMsg(err)
				}
				ts, _ = database.GetLastVSCodeRefreshAt()
			}
			return vscodeSummaryMsg{summary: summary, lastRefresh: ts}
		}
		return m, tea.Batch(m.spinner.Tick, bgCmd)

	case "Profiles":
		m.state = stateLoading
		m.loadingText = "Loading VSCode profiles..."
		bgCmd := func() tea.Msg {
			ts, _ := database.GetLastVSCodeRefreshAt()
			profs, err := apps.LoadVSCodeProfiles(database, m.vscodeShowArchived)
			if err != nil || len(profs) == 0 {
				// Auto-refresh if empty
				_, err = apps.ScanVSCode(database)
				if err != nil {
					return errMsg(err)
				}
				profs, err = apps.LoadVSCodeProfiles(database, m.vscodeShowArchived)
				if err != nil {
					return errMsg(err)
				}
				ts, _ = database.GetLastVSCodeRefreshAt()
			}
			return vscodeProfilesMsg{profiles: profs, lastRefresh: ts}
		}
		return m, tea.Batch(m.spinner.Tick, bgCmd)

	case "Dependencies":
		m.state = stateLoading
		m.loadingText = "Loading VSCode dependencies..."
		bgCmd := func() tea.Msg {
			deps, err := apps.LoadVSCodeDependencies(database)
			if err != nil {
				return errMsg(err)
			}
			ts, _ := database.GetLastVSCodeRefreshAt()
			return vscodeDepsMsg{deps: deps, lastRefresh: ts}
		}
		return m, tea.Batch(m.spinner.Tick, bgCmd)

	case "History":
		m.state = stateLoading
		m.loadingText = "Loading VSCode refresh history..."
		bgCmd := func() tea.Msg {
			logs, err := database.GetVSCodeRefreshLogs(true, 100, 0)
			if err != nil {
				return errMsg(err)
			}
			return vscodeHistoryMsg{logs: logs}
		}
		return m, tea.Batch(m.spinner.Tick, bgCmd)

	case "Refresh":
		m.state = stateLoading
		m.loadingText = "Se están refrescando los datos..."
		bgCmd := func() tea.Msg {
			diff, err := apps.ScanVSCode(database)
			if err != nil {
				return errMsg(err)
			}
			return vscodeRefreshDoneMsg{diff: diff}
		}
		return m, tea.Batch(m.spinner.Tick, bgCmd)

	default:
		m.result = fmt.Sprintf("Unknown VSCode command: %q", title)
		m.state = stateResult
		return m, nil
	}
}

func updateVSCodeProfilePreview(m Model) Model {
	if len(m.vscodeProfiles) == 0 || m.vscodeProfileCursor < 0 || m.vscodeProfileCursor >= len(m.vscodeProfiles) {
		m.vscodeProfileVP.SetContent("")
		return m
	}

	p := m.vscodeProfiles[m.vscodeProfileCursor]
	var sb strings.Builder

	sb.WriteString(lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("39")).Render("PERFIL: " + p.Name))
	sb.WriteString("\n\n")

	sb.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("240")).Render("Ruta de instalación:"))
	sb.WriteString("\n" + p.ProfilePath + "\n\n")

	sb.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("240")).Render("Última modificación:"))
	if !p.DirMtime.IsZero() {
		sb.WriteString("\n" + p.DirMtime.Local().Format("2006-01-02 15:04:05") + "\n\n")
	} else {
		sb.WriteString("\nNo disponible (no existe en disco)\n\n")
	}

	sb.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("240")).Render("Proyectos asociados:"))
	if len(p.Projects) == 0 {
		sb.WriteString("\nNinguno\n\n")
	} else {
		sb.WriteString("\n")
		for _, proj := range p.Projects {
			if proj.ExistsOnDisk {
				sb.WriteString("  ✓ " + proj.Path + "\n")
			} else {
				sb.WriteString("  " + lipgloss.NewStyle().Foreground(lipgloss.Color("208")).Render("[Arch] ") + proj.Path + "\n")
			}
		}
		sb.WriteString("\n")
	}

	sb.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("240")).Render("Dependencias (Extensiones):"))
	if len(p.Extensions) == 0 {
		sb.WriteString("\nNinguna\n")
	} else {
		sb.WriteString("\n")
		for _, ext := range p.Extensions {
			sb.WriteString(fmt.Sprintf("  • %s (%s)\n", ext.ID, ext.Version))
		}
	}

	const minTableWidth = 35
	previewWidth := m.width - minTableWidth - 4
	if previewWidth < 25 {
		previewWidth = 25
	}
	m.vscodeProfileVP.SetWidth(previewWidth - 2)
	m.vscodeProfileVP.SetHeight(m.height - 12)

	wrapped := WordWrap(sb.String(), previewWidth-4)
	m.vscodeProfileVP.SetContent(wrapped)
	m.vscodeProfileVP.GotoTop()

	return m
}

// ─── Info panel builder ─────────────────────────────────────────────────────

func updateSelectedServiceDetails(m Model) (Model, tea.Cmd) {
	if len(m.servicesItems) == 0 {
		m.servicesInfoVP.SetContent("No services available.")
		m.servicesLogsVP.SetContent("")
		return m, nil
	}

	s := m.servicesItems[m.servicesCursor]

	// Refresh log paths cache
	m.servicesLogPaths = brew.ServiceLogPaths(s)

	// Build info pane
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Name:        %s\n", s.Name))

	statusSymbol := "●"
	var statusColor string
	if s.Status == "started" {
		statusColor = "42" // Green
	} else if s.Status == "stopped" {
		statusColor = "196" // Red
	} else {
		statusColor = "243" // Gray
	}
	styledStatus := lipgloss.NewStyle().Foreground(lipgloss.Color(statusColor)).Bold(true).Render(fmt.Sprintf("%s %s", statusSymbol, s.Status))
	sb.WriteString(fmt.Sprintf("Status:      %s\n", styledStatus))

	if s.User != "" {
		sb.WriteString(fmt.Sprintf("User:        %s\n", s.User))
	}
	if s.Version != "" {
		sb.WriteString(fmt.Sprintf("Version:     %s\n", s.Version))
	}
	if s.Path != "" {
		sb.WriteString(fmt.Sprintf("Path:        %s\n", s.Path))
	}
	if s.File != "" {
		sb.WriteString(fmt.Sprintf("Plist File:  %s\n", s.File))
	}
	if s.ExitCode != 0 {
		sb.WriteString(fmt.Sprintf("Exit Code:   %d\n", s.ExitCode))
	}
	if s.Desc != "" {
		sb.WriteString(fmt.Sprintf("\nDesc:        %s\n", s.Desc))
	}
	if s.Homepage != "" {
		sb.WriteString(fmt.Sprintf("Homepage:    %s\n", s.Homepage))
	}

	// Log file paths
	if len(m.servicesLogPaths) > 0 {
		sb.WriteString("\nLog Files:\n")
		for _, p := range m.servicesLogPaths {
			sb.WriteString(fmt.Sprintf("  %s\n", p))
		}
	} else {
		sb.WriteString("\nLog Files:   (none found)\n")
	}

	// Log size (if computed)
	if m.servicesLogSize != "" {
		sb.WriteString(fmt.Sprintf("Log Size:    %s\n", m.servicesLogSize))
	} else {
		sb.WriteString("Log Size:    [press Z to calculate]\n")
	}

	sb.WriteString("\nActions:     [s]tart  [x]stop  [r]estart  [K]ill\n")

	wrappedInfo := WordWrap(sb.String(), m.servicesInfoVP.Width()-2)
	m.servicesInfoVP.SetContent(wrappedInfo)
	m.servicesInfoVP.GotoTop()

	// Load initial logs (static snapshot) and start streaming
	if m.servicesStreamProc != nil {
		_ = m.servicesStreamProc.Kill()
		m.servicesStreamProc = nil
		m.servicesLogsStreaming = false
	}
	m.servicesLogsContent = ""
	m.servicesLogsVP.SetContent("Starting log stream...")

	var streamCmd tea.Cmd
	m, streamCmd = startLogStreamCmd(m, s)
	return m, streamCmd
}

func updateVSCodeHistoryPreview(m Model) Model {
	if len(m.vscodeRefreshHistory) == 0 || m.vscodeHistoryCursor < 0 || m.vscodeHistoryCursor >= len(m.vscodeRefreshHistory) {
		m.vscodeHistoryDetailVP.SetContent("")
		return m
	}

	entry := m.vscodeRefreshHistory[m.vscodeHistoryCursor]
	var sb strings.Builder

	date := entry.RefreshedAt.Local().Format("2006-01-02 15:04:05")
	sb.WriteString(lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("39")).Render("CAMBIOS EN REFRESH: " + date))
	sb.WriteString("\n\n")

	var diff apps.VSCodeDiff
	if err := json.Unmarshal([]byte(entry.DiffJSON), &diff); err == nil {
		if !diff.HasAnyChange {
			sb.WriteString("Sin cambios detectados en este refresco.\n")
		} else {
			if diff.VersionChanged {
				sb.WriteString(lipgloss.NewStyle().Bold(true).Render("• Versión de VSCode:") + "\n")
				sb.WriteString(fmt.Sprintf("  %s → %s\n\n", diff.OldVersion, diff.NewVersion))
			}
			if len(diff.PathsAdded) > 0 {
				sb.WriteString(lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("34")).Render("• Paths Añadidos:") + "\n")
				for _, p := range diff.PathsAdded {
					sb.WriteString(fmt.Sprintf("  + %s\n", p))
				}
				sb.WriteString("\n")
			}
			if len(diff.PathsRemoved) > 0 {
				sb.WriteString(lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("196")).Render("• Paths Eliminados:") + "\n")
				for _, p := range diff.PathsRemoved {
					sb.WriteString(fmt.Sprintf("  - %s\n", p))
				}
				sb.WriteString("\n")
			}
			if len(diff.ProfilesAdded) > 0 {
				sb.WriteString(lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("34")).Render("• Perfiles Añadidos:") + "\n")
				for _, p := range diff.ProfilesAdded {
					sb.WriteString(fmt.Sprintf("  + %s\n", p))
				}
				sb.WriteString("\n")
			}
			if len(diff.ProfilesRemoved) > 0 {
				sb.WriteString(lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("196")).Render("• Perfiles Eliminados:") + "\n")
				for _, p := range diff.ProfilesRemoved {
					sb.WriteString(fmt.Sprintf("  - %s\n", p))
				}
				sb.WriteString("\n")
			}
			if len(diff.ExtChanges) > 0 {
				sb.WriteString(lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("39")).Render("• Cambio en Extensiones:") + "\n")
				for _, line := range diff.ExtChanges {
					sb.WriteString(fmt.Sprintf("  * %s\n", line))
				}
				sb.WriteString("\n")
			}
		}
	} else {
		sb.WriteString("Error decodificando diferencias.\n")
	}

	const leftWidth = 28
	previewWidth := m.width - leftWidth - 4
	if previewWidth < 25 {
		previewWidth = 25
	}
	m.vscodeHistoryDetailVP.SetWidth(previewWidth - 2)
	m.vscodeHistoryDetailVP.SetHeight(m.height - 12)

	wrapped := WordWrap(sb.String(), previewWidth-4)
	m.vscodeHistoryDetailVP.SetContent(wrapped)
	m.vscodeHistoryDetailVP.GotoTop()

	return m
}

func updateVSCodeDepsPreview(m Model) Model {
	if len(m.vscodeDepsFiltered) == 0 || m.vscodeDepsCursor < 0 || m.vscodeDepsCursor >= len(m.vscodeDepsFiltered) {
		m.vscodeDepsVP.SetContent("")
		return m
	}

	dep := m.vscodeDepsFiltered[m.vscodeDepsCursor]
	var sb strings.Builder

	sb.WriteString(lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("39")).Render("EXTENSION: " + dep.ID))
	sb.WriteString("\n")
	if dep.Description != "" {
		sb.WriteString(lipgloss.NewStyle().Italic(true).Foreground(lipgloss.Color("244")).Render(dep.Description))
		sb.WriteString("\n")
	}
	sb.WriteString("\n")

	if m.vscodeDepsShowLong {
		sb.WriteString(lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("212")).Render("📄 DESCRIPCIÓN OFICIAL (README):"))
		sb.WriteString("\n\n")
		if dep.LongDescription != "" {
			sb.WriteString(dep.LongDescription)
		} else {
			sb.WriteString(lipgloss.NewStyle().Italic(true).Foreground(lipgloss.Color("240")).Render("No official README description fetched from Marketplace."))
		}
	} else {
		sb.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("240")).Render("Perfiles que la tienen instalada:"))
		sb.WriteString("\n\n")

		for _, inst := range dep.Installs {
			sb.WriteString("  " + headerStyle.Render("●") + " " + warningStyle.Render(inst.ProfileName) + "\n")
			sb.WriteString("    Versión: " + inst.Version + "\n")
			if !inst.InstalledAt.IsZero() {
				sb.WriteString("    Instalada el: " + inst.InstalledAt.Local().Format("2006-01-02 15:04:05") + "\n")
			}
			if inst.InstallPath != "" {
				sb.WriteString("    Path: " + inst.InstallPath + "\n")
			}
			sb.WriteString("\n")
		}
	}

	const leftWidth = 35
	previewWidth := m.width - leftWidth - 4
	if previewWidth < 25 {
		previewWidth = 25
	}
	m.vscodeDepsVP.SetWidth(previewWidth - 2)
	m.vscodeDepsVP.SetHeight(m.height - 12)

	wrapped := WordWrap(sb.String(), previewWidth-4)
	m.vscodeDepsVP.SetContent(wrapped)
	m.vscodeDepsVP.GotoTop()

	return m
}

