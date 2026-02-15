package main

import (
	"fmt"
	"os"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
)

type status int

const (
	statusIdle status = iota
	statusBrew
)

type model struct {
	state  status
	list   list.Model
	choice string
	width  int
	height int
}

// Define our list items (we can move this to a separate file later)
type item struct {
	title, desc string
}

func (i item) Title() string       { return i.title }
func (i item) Description() string { return i.desc }
func (i item) FilterValue() string { return i.title }

func initialModel() model {
	// Create the 4 options you requested
	items := []list.Item{
		item{title: "Install", desc: "Install a new package"},
		item{title: "Update", desc: "Update existing packages"},
		item{title: "Doctor", desc: "Check system health"},
		item{title: "Cleanup", desc: "Remove old cache files"},
	}

	// Initialize the list component
	// 0, 0 = default width/height (will be resized by the window msg)
	l := list.New(items, list.NewDefaultDelegate(), 0, 0)
	l.Title = "Maximus Brew"

	return model{
		state: statusBrew, // Defaulting to brew state for this demo
		list:  l,
	}
}

func (m model) Init() tea.Cmd {
	return nil
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd

	switch msg := msg.(type) {

	// Handle window resizing (Crucial for TUIs!)
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.list.SetSize(msg.Width, msg.Height)

	// Handle Key Presses
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q":
			return m, tea.Quit

		case "enter":
			// Capture the selection and exit
			i, ok := m.list.SelectedItem().(item)
			if ok {
				m.choice = i.title
			}
			return m, tea.Quit
		}
	}

	// Pass all other events to the list component so it handles
	// scrolling, filtering, etc.
	m.list, cmd = m.list.Update(msg)
	return m, cmd
}

func (m model) View() string {
	// If the user made a choice, show it and stop rendering the list
	if m.choice != "" {
		return fmt.Sprintf("\n  You selected: %s\n\n", m.choice)
	}

	// Otherwise, render the list
	return m.list.View()
}

func main() {
	// Simple CLI Routing
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "help":
			printHelp()
			return
		case "brew":
			// Continue to run the TUI below
		default:
			fmt.Printf("Unknown command: %s\nRun 'maximus help' for usage.\n", os.Args[1])
			os.Exit(1)
		}
	} else {
		// Default behavior if just "maximus" is run
		printHelp()
		return
	}

	// Run the Bubble Tea Program
	p := tea.NewProgram(initialModel(), tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Printf("Error starting maximus: %v", err)
		os.Exit(1)
	}
}

// Static Help Output
func printHelp() {
	fmt.Println("\nMaximus CLI - v0.1.0")
	fmt.Println("--------------------")
	fmt.Println("Usage:")
	fmt.Println("  maximus brew   : Open the interactive package manager")
	fmt.Println("  maximus help   : Show this help message")
	fmt.Println("")
}
