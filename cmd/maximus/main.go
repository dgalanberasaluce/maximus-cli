// Command maximus is the entry point for the Maximus CLI tool.
package main

import (
	"fmt"
	"os"

	"maximus-cli/internal/config"
	"maximus-cli/internal/db"
	"maximus-cli/internal/tui"

	tea "charm.land/bubbletea/v2"
)

func main() {
	// 1. Load configuration (creates ~/.config/maximus-cli if needed).
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "maximus: config error: %v\n", err)
		os.Exit(1)
	}

	// 2. Open (or create) the SQLite3 database.
	database, err := db.Open(cfg.DBPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "maximus: database error: %v\n", err)
		os.Exit(1)
	}
	defer database.Close()

	// 3. Start the TUI.
	m := tui.New(cfg.BrewfilePath, database)
	p := tea.NewProgram(m)
	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "maximus: TUI error: %v\n", err)
		os.Exit(1)
	}
}
