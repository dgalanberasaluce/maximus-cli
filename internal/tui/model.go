// Package tui provides the Bubble Tea terminal user interface for maximus-cli.
package tui

import (
	"maximus-cli/internal/brew"
	"maximus-cli/internal/db"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
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
	stateBrewLogs
	stateUnstaged // interactive unstaged packages screen
)

const (
	logPageSize = 20 // default entries per page
	logMaxLimit = 100
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

// logsMsg carries the result of a log query back to the model.
type logsMsg struct {
	entries []db.UpgradeLog
	total   int
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
	database    *db.DB // database connection

	// Log viewer state
	logEntries   []db.UpgradeLog
	logTotal     int
	logPage      int    // 0-indexed current page
	logFilter    string // active filter (package name substring)
	logInput     textinput.Model
	logInputMode bool // whether the filter text input is active

	// Unstaged packages state
	unstagedPackages []brew.UnstagedPackage
	unstagedCursor   int          // index of currently highlighted row
	unstagedSelected map[int]bool // selected package indices
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

	headerStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("39"))

	warningStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("208")).
			Bold(true)
)

// New creates and initialises the root model.
// brewfilePath is the Brewfile path for brew commands.
// database is the open SQLite3 connection.
func New(brewfilePath string, database *db.DB) Model {
	mainItems := []list.Item{
		menuItem{title: "Brew", desc: "Manage Homebrew packages"},
	}
	ml := list.New(mainItems, list.NewDefaultDelegate(), 0, 0)
	ml.Title = "Maximus CLI"
	ml.SetShowStatusBar(false)

	brewItems := []list.Item{
		menuItem{title: "Update", desc: "Refresh the Homebrew package database (brew update)"},
		menuItem{title: "Upgrade All", desc: "Install all packages from Brewfile (brew bundle install)"},
		menuItem{title: "Cleanup", desc: "Remove stale packages and unused dependencies (brew cleanup + autoremove)"},
		menuItem{title: "Diff", desc: "Compare Brewfile with system — show available upgrades (Smart Diff)"},
		menuItem{title: "Unstaged", desc: "Show packages installed but not in Brewfile (brew bundle cleanup --dry-run)"},
		menuItem{title: "Remove", desc: "⚠  Remove packages not in Brewfile (brew bundle cleanup --force)"},
		menuItem{title: "Logs", desc: "View upgrade history (last 20 entries, filterable)"},
		menuItem{title: "Cheatsheet", desc: "Quick reference for Homebrew commands"},
	}
	bl := list.New(brewItems, list.NewDefaultDelegate(), 0, 0)
	bl.Title = "Brew"
	bl.SetShowStatusBar(false)

	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("205"))

	ti := textinput.New()
	ti.Placeholder = "package name..."
	ti.CharLimit = 60
	ti.Width = 40

	return Model{
		state:    stateMainMenu,
		mainList: ml,
		brewList: bl,
		spinner:  s,
		brewfile: brewfilePath,
		database: database,
		logInput: ti,
	}
}

// Init starts the initial Bubble Tea command.
func (m Model) Init() tea.Cmd {
	return nil
}
