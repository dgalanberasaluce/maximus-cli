// Package tui provides the Bubble Tea terminal user interface for maximux-cli.
package tui

import (
	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// viewState represents which screen is currently displayed.
type viewState int

const (
	stateMainMenu viewState = iota
	stateBrewMenu
	stateLoading
	stateResult
)

// menuItem is a selectable list entry with a title and description.
type menuItem struct {
	title, desc string
}

func (m menuItem) Title() string       { return m.title }
func (m menuItem) Description() string { return m.desc }
func (m menuItem) FilterValue() string { return m.title }

// resultMsg carries the output of a background command back to the model.
type resultMsg struct {
	content string
}

// errMsg carries an error back to the model.
type errMsg error

// Model is the root Bubble Tea model for the application.
type Model struct {
	state       viewState
	mainList    list.Model
	brewList    list.Model
	spinner     spinner.Model
	result      string
	loadingText string
	width       int
	height      int
	brewfile    string // path passed from config
}

// styles
var (
	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("212")).
			PaddingLeft(2)

	helpStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("240")).
			PaddingLeft(2)
)

// New creates and initialises the root model.
// brewfilePath is the Brewfile path for brew commands.
func New(brewfilePath string) Model {
	mainItems := []list.Item{
		menuItem{title: "Brew", desc: "Manage Homebrew packages"},
	}
	ml := list.New(mainItems, list.NewDefaultDelegate(), 0, 0)
	ml.Title = "Maximus CLI"
	ml.SetShowStatusBar(false)

	brewItems := []list.Item{
		menuItem{title: "Update",     desc: "Refresh the Homebrew package database (brew update)"},
		menuItem{title: "Upgrade All", desc: "Install all packages from Brewfile (brew bundle install)"},
		menuItem{title: "Cleanup",    desc: "Remove stale packages and unused dependencies (brew cleanup + autoremove)"},
		menuItem{title: "Diff",       desc: "Compare Brewfile with system — show available upgrades (Smart Diff)"},
		menuItem{title: "Unstaged",   desc: "Show packages installed but not in Brewfile (brew bundle cleanup --dry-run)"},
		menuItem{title: "Remove",     desc: "⚠  Remove packages not in Brewfile (brew bundle cleanup --force)"},
		menuItem{title: "Cheatsheet", desc: "Quick reference for Homebrew commands"},
	}
	bl := list.New(brewItems, list.NewDefaultDelegate(), 0, 0)
	bl.Title = "Brew"
	bl.SetShowStatusBar(false)

	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("205"))

	return Model{
		state:    stateMainMenu,
		mainList: ml,
		brewList: bl,
		spinner:  s,
		brewfile: brewfilePath,
	}
}

// Init starts the initial Bubble Tea command.
func (m Model) Init() tea.Cmd {
	return nil
}
