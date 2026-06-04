// Package tui provides the Bubble Tea terminal user interface for maximus-cli.
package tui

import (
	"os"
	"time"

	"maximus-cli/internal/apps"
	"maximus-cli/internal/brew"
	"maximus-cli/internal/db"

	"charm.land/bubbles/v2/list"
	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/textinput"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// viewState represents which screen is currently displayed.
type viewState int

const (
	stateMainMenu viewState = iota
	stateBrewMenu
	stateLoading
	stateResult
	stateBrewLogs
	stateUnstaged       // interactive unstaged packages screen
	stateBrewVersion    // installed package version table
	stateUpgradePkgs    // select and upgrade specific packages screen
	stateHomeMenu       // home sub-menu (Explain / Reload)
	stateHomeDotfiles   // home dotfiles table
	stateBrewServices   // services management panel
	stateAppsMenu       // apps sub-menu
	stateVSCodeInfo     // vscode installation summary screen
	stateVSCodeMenu     // vscode sub-menu (Summary/Profiles/History/Refresh)
	stateVSCodeProfiles // vscode profiles interactive panel
	stateVSCodeHistory  // vscode refresh history diffs panel
	stateVSCodeDeps     // vscode aggregated dependencies screen
	stateGitHubRepos    // github repo tracker table
)

// versionSortField identifies which column is used for sorting the version table.
type versionSortField int

const (
	sortByName versionSortField = iota
	sortByKind
	sortByMetaDate
	sortByInstallDate
)

// dotfileSortField identifies which column is used for sorting the dotfiles table.
type dotfileSortField int

const (
	sortDFByName dotfileSortField = iota
	sortDFByType
	sortDFByTool
	sortDFByModified
	sortDFByCreated
)

// githubRepoSortField identifies which column is used for sorting the GitHub repos table.
type githubRepoSortField int

const (
	sortRepoByName githubRepoSortField = iota
	sortRepoByCategory
	sortRepoByLanguage
	sortRepoByStars
	sortRepoByUpdated
)

const (
	logPageSize = 20 // default entries per page
	logMaxLimit = 100
)

// typeFilter restricts the dotfiles table to files, directories, or all entries.
type typeFilter int

const (
	typeFilterAll   typeFilter = iota // show all
	typeFilterFiles                   // files only
	typeFilterDirs                    // directories only
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
	viewport    viewport.Model

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

	// Upgrade packages state
	upgradePkgs     []brew.DiffResult
	upgradeCursor   int          // index of currently highlighted row
	upgradeSelected map[int]bool // selected package indices

	// Version table state
	versionItems     []brew.PackageVersion // all items (unfiltered)
	versionFiltered  []brew.PackageVersion // items after applying filter
	versionCursor    int                   // highlighted row index
	versionFilter    string                // active filter string
	versionSortField versionSortField      // current sort column
	versionSortAsc   bool                  // true = ascending
	versionInput     textinput.Model
	versionInputMode bool // true when filter text input is focused

	// Home sub-menu & dotfiles state
	homeList              list.Model
	dotfileItems          []db.DotfileEntry // all items (unfiltered)
	dotfileFiltered       []db.DotfileEntry // items after applying filter
	dotfileCursor         int               // highlighted row index
	dotfileFilter         string            // active text filter string
	dotfileTypeFilter     typeFilter        // file/dir/all filter
	dotfileSortField      dotfileSortField  // current sort column
	dotfileSortAsc        bool              // true = ascending
	dotfileInput          textinput.Model
	dotfileInputMode      bool // true when text filter input is focused
	dotfileToolInput      textinput.Model
	dotfileToolEditMode   bool            // true when editing a tool description
	returnState           viewState       // the state to return to from stateResult
	dotfilePreviewFocused bool            // true when the preview panel has focus
	dotfileDeleteMode     bool            // true when confirming deletion of a dotfile
	dotfileDeleteInput    textinput.Model // input for typing file name to delete
	previewViewport       viewport.Model  // viewport for scrollable preview pane

	// Services state
	servicesItems       []brew.Service
	servicesCursor      int
	servicesInfoVP      viewport.Model
	servicesLogsVP      viewport.Model
	servicesFocusPanel  int    // 0=list, 1=info, 2=logs
	servicesLogsContent string // cached full (unfiltered) log content
	// Confirmation popup
	servicesConfirmMode   bool   // true = popup visible
	servicesConfirmAction string // "start"|"stop"|"restart"|"kill"
	// Log streaming
	servicesStreamProc    *os.Process // running tail -f process, nil if stopped
	servicesLogsStreaming bool        // true when streaming is active
	// Log filter
	servicesLogsFilter     string
	servicesLogsFilterMode bool
	servicesLogsInput      textinput.Model
	// Log size (computed on demand via Z key)
	servicesLogSize string // human-readable, e.g. "1.2 MB", empty until computed
	// Log paths (cached from last fetch)
	servicesLogPaths []string

	// Apps state
	appsList                list.Model
	vscodeSummary           apps.VSCodeSummary
	vscodeList              list.Model
	vscodeProfiles          []apps.VSCodeProfile
	vscodeProfileCursor     int
	vscodeProfileVP         viewport.Model
	vscodeProfileFocusPanel bool
	vscodeShowArchived      bool
	vscodeRefreshHistory    []db.VSCodeRefreshLogRow
	vscodeHistoryCursor     int
	vscodeHistoryExpanded   bool
	vscodeHistoryDetailVP   viewport.Model
	vscodeLastRefreshAt     time.Time
	vscodeDeps              []apps.VSCodeExtAgg
	vscodeDepsFiltered      []apps.VSCodeExtAgg
	vscodeDepsCursor        int
	vscodeDepsFocusPanel    bool
	vscodeDepsVP            viewport.Model
	vscodeDepsInput         textinput.Model
	vscodeDepsInputMode     bool
	vscodeDepsShowLong      bool

	// GitHub Repo Tracker state
	githubRepoItems        []db.GitHubRepo
	githubRepoFiltered     []db.GitHubRepo
	githubRepoCursor       int
	githubRepoFilter       string
	githubRepoSortField    githubRepoSortField
	githubRepoSortAsc      bool
	githubRepoInput        textinput.Model
	githubRepoInputMode    bool
	githubRepoShowAddedCol bool
	githubRepoPreviewVP    viewport.Model
	githubRepoPreviewFocus bool
	// Add repo overlay
	githubRepoAddMode      bool
	githubRepoAddOwner     string
	githubRepoAddRepo      string
	githubRepoAddInput     textinput.Model
	githubRepoAddInputMode bool
	githubRepoAddInputStep int // 0=owner, 1=repo, 2=done
	githubRepoAddMsg       string
	githubRepoAddMsgType   string // "loading", "success", "error"
	githubRepoAddSpinner   spinner.Model
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
		menuItem{title: "Home", desc: "Manage and explain dotfiles in $HOME"},
		menuItem{title: "Brew", desc: "Manage Homebrew packages"},
		menuItem{title: "Applications", desc: "Manage and view configurations of applications (VSCode, etc.)"},
	}
	ml := list.New(mainItems, list.NewDefaultDelegate(), 0, 0)
	ml.Title = "Maximus CLI"
	ml.SetShowStatusBar(false)

	appsItems := []list.Item{
		menuItem{title: "VSCode", desc: "Manage and view configurations of VSCode"},
		menuItem{title: "Git Repo Tracker", desc: "Track and manage Git repositories"},
	}
	al := list.New(appsItems, list.NewDefaultDelegate(), 0, 0)
	al.Title = "Applications"
	al.SetShowStatusBar(false)

	vscodeItems := []list.Item{
		menuItem{title: "Summary", desc: "Show installation summary"},
		menuItem{title: "Profiles", desc: "Manage and view interactive profiles"},
		menuItem{title: "Dependencies", desc: "View all installed extensions and their profiles"},
		menuItem{title: "History", desc: "View history of changes"},
		menuItem{title: "Refresh", desc: "Scan and refresh database data"},
	}
	vl := list.New(vscodeItems, list.NewDefaultDelegate(), 0, 0)
	vl.Title = "VSCode Options"
	vl.SetShowStatusBar(false)

	homeItems := []list.Item{
		menuItem{title: "Explain", desc: "Show dotfiles/folders table in $HOME with tool explanations"},
		menuItem{title: "Reload", desc: "Rescan $HOME directory and refresh the database"},
	}
	hl := list.New(homeItems, list.NewDefaultDelegate(), 0, 0)
	hl.Title = "Home Options"
	hl.SetShowStatusBar(false)

	brewItems := []list.Item{
		menuItem{title: "Update", desc: "Refresh the Homebrew package database (brew update)"},
		menuItem{title: "Upgrade All", desc: "Install all packages from Brewfile (brew bundle install)"},
		menuItem{title: "Upgrade Package(s)", desc: "Select and upgrade specific packages"},
		menuItem{title: "Cleanup", desc: "Remove stale packages and unused dependencies (brew cleanup + autoremove)"},
		menuItem{title: "Diff", desc: "Compare Brewfile with system — show available upgrades (Smart Diff)"},
		menuItem{title: "Unstaged", desc: "Show packages installed but not in Brewfile (brew bundle cleanup --dry-run)"},
		menuItem{title: "Remove", desc: "⚠  Remove packages not in Brewfile (brew bundle cleanup --force)"},
		menuItem{title: "Version", desc: "Show installed versions of all Brewfile packages (sortable & filterable)"},
		menuItem{title: "Services", desc: "View and manage Homebrew services (start/stop/restart/kill)"},
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
	ti.SetWidth(40)

	vi := textinput.New()
	vi.Placeholder = "filter packages..."
	vi.CharLimit = 60
	vi.SetWidth(40)

	di := textinput.New()
	di.Placeholder = "filter dotfiles..."
	di.CharLimit = 60
	di.SetWidth(40)

	eti := textinput.New()
	eti.Placeholder = "tool name..."
	eti.CharLimit = 80
	eti.SetWidth(40)

	vp := viewport.New()
	vp.Style = lipgloss.NewStyle().Padding(1, 2)

	dti := textinput.New()
	dti.Placeholder = "type name to confirm..."
	dti.CharLimit = 80
	dti.SetWidth(40)

	pv := viewport.New()

	sip := viewport.New()
	sip.Style = lipgloss.NewStyle().Padding(0, 1)

	slp := viewport.New()
	slp.Style = lipgloss.NewStyle().Padding(0, 1)

	sli := textinput.New()
	sli.Placeholder = "filter logs..."
	sli.CharLimit = 120
	sli.SetWidth(40)

	vdi := textinput.New()
	vdi.Placeholder = "filter extensions..."
	vdi.CharLimit = 60
	vdi.SetWidth(30)

	gri := textinput.New()
	gri.Placeholder = "filter repos..."
	gri.CharLimit = 60
	gri.SetWidth(40)

	grai := textinput.New()
	grai.Placeholder = "owner or repo..."
	grai.CharLimit = 80
	grai.SetWidth(30)

	grs := spinner.New()
	grs.Spinner = spinner.Dot
	grs.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("205"))

	grpv := viewport.New()

	return Model{
		state:                 stateMainMenu,
		returnState:           stateMainMenu,
		mainList:              ml,
		homeList:              hl,
		brewList:              bl,
		spinner:               s,
		brewfile:              brewfilePath,
		database:              database,
		logInput:              ti,
		versionInput:          vi,
		versionSortField:      sortByName,
		versionSortAsc:        true,
		viewport:              vp,
		dotfileInput:          di,
		dotfileSortField:      sortDFByName,
		dotfileSortAsc:        true,
		dotfileToolInput:      eti,
		dotfileDeleteInput:    dti,
		previewViewport:       pv,
		dotfilePreviewFocused: false,
		dotfileDeleteMode:     false,
		servicesInfoVP:        sip,
		servicesLogsVP:        slp,
		servicesFocusPanel:    0,
		servicesLogsInput:     sli,
		appsList:              al,
		vscodeList:            vl,
		vscodeShowArchived:    false,
		vscodeProfileVP:       viewport.New(),
		vscodeHistoryDetailVP: viewport.New(),
		vscodeDepsVP:          viewport.New(),
		vscodeDepsInput:       vdi,
		githubRepoInput:       gri,
		githubRepoSortField:   sortRepoByName,
		githubRepoSortAsc:     true,
		githubRepoPreviewVP:   grpv,
		githubRepoAddInput:    grai,
		githubRepoAddSpinner:  grs,
	}
}

// Init starts the initial Bubble Tea command.
func (m Model) Init() tea.Cmd {
	return nil
}
