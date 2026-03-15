package tui

import (
	"fmt"

	"maximus-cli/internal/brew"

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
		m.result = msg.content + "\n\n(Press q or esc to go back)"
		m.state = stateResult
		return m, nil

	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c":
			return m, tea.Quit

		case "esc", "q":
			switch m.state {
			case stateResult:
				// Return to the brew menu after viewing a result.
				m.result = ""
				m.state = stateBrewMenu
				return m, nil
			case stateBrewMenu:
				// Return to main menu from brew menu.
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

	// Delegate navigation updates to the active list.
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

// dispatchBrewCmd maps a selected brew menu item to the appropriate background command.
func (m Model) dispatchBrewCmd(title string) (tea.Model, tea.Cmd) {
	m.state = stateLoading
	brewfile := m.brewfile

	var bgCmd tea.Cmd
	switch title {
	case "Update":
		m.loadingText = "Running brew update..."
		bgCmd = func() tea.Msg {
			out, err := brew.Update()
			if err != nil {
				return errMsg(err)
			}
			return resultMsg{content: out}
		}

	case "Upgrade All":
		m.loadingText = "Running brew bundle install..."
		bgCmd = func() tea.Msg {
			out, err := brew.Upgrade(brewfile)
			if err != nil {
				return errMsg(err)
			}
			return resultMsg{content: out}
		}

	case "Cleanup":
		m.loadingText = "Running brew cleanup + autoremove..."
		bgCmd = func() tea.Msg {
			out, err := brew.Cleanup()
			if err != nil {
				return errMsg(err)
			}
			return resultMsg{content: out}
		}

	case "Diff":
		m.loadingText = "Computing Smart Diff..."
		bgCmd = func() tea.Msg {
			results, err := brew.SmartDiff(brewfile)
			if err != nil {
				return errMsg(err)
			}
			return resultMsg{content: brew.FormatDiffResults(results)}
		}

	case "Unstaged":
		m.loadingText = "Checking unstaged packages..."
		bgCmd = func() tea.Msg {
			out, err := brew.Unstaged(brewfile)
			if err != nil {
				return errMsg(err)
			}
			if out == "" {
				out = "✓ No packages found outside your Brewfile."
			}
			return resultMsg{content: out}
		}

	case "Remove":
		m.loadingText = "Running brew bundle cleanup --force..."
		bgCmd = func() tea.Msg {
			out, err := brew.Remove(brewfile)
			if err != nil {
				return errMsg(err)
			}
			return resultMsg{content: out}
		}

	case "Cheatsheet":
		m.loadingText = "Loading cheatsheet..."
		bgCmd = func() tea.Msg {
			out, err := brew.Cheatsheet()
			if err != nil {
				return errMsg(err)
			}
			return resultMsg{content: out}
		}

	default:
		m.result = fmt.Sprintf("Unknown command: %q", title)
		m.state = stateResult
		return m, nil
	}

	return m, tea.Batch(m.spinner.Tick, bgCmd)
}
