package brew

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// UnstagedPackage represents a package installed on the system but absent from
// the Brewfile.
type UnstagedPackage struct {
	// Name is the formula, cask, or tap name.
	Name string
	// Kind is "brew", "cask", or "tap".
	Kind string
}

// Unstaged returns the packages installed on the system that are NOT listed in
// the Brewfile. It uses `brew bundle cleanup` (without --force, which is the
// default dry-run mode) and never returns an error just because there are
// unstaged packages; an error is only returned for genuine failures (e.g.
// brew not found, permission denied, Brewfile path wrong).
func Unstaged(brewfilePath string) ([]UnstagedPackage, error) {
	cmd := exec.Command("brew", "bundle", "cleanup", "--file", brewfilePath)
	out, err := cmd.CombinedOutput()
	if err != nil {
		// brew bundle cleanup exits 1 when it finds packages to remove — that
		// is expected and NOT an error we should surface.  Only propagate
		// errors that are NOT a plain non-zero exit (e.g. binary not found).
		var exitErr *exec.ExitError
		if !errors.As(err, &exitErr) {
			return nil, fmt.Errorf("brew bundle cleanup: %w", err)
		}
		// exitErr just means "brew found packages to remove" — continue with
		// the output we already captured.
	}
	return parseUnstagedOutput(out), nil
}

// parseUnstagedOutput converts the raw output of `brew bundle cleanup` into a
// slice of UnstagedPackage.
//
// Actual brew output format:
//
//	Would uninstall casks:
//	iterm2
//	visual-studio-code
//	Would uninstall formulae:
//	bat
//	fzf
//	Would untap:
//	some/tap
//	Run `brew bundle cleanup --force` to make these changes.
func parseUnstagedOutput(data []byte) []UnstagedPackage {
	var pkgs []UnstagedPackage
	var currentKind string

	scanner := bufio.NewScanner(bytes.NewReader(data))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		// Detect section headers.
		switch {
		case strings.HasPrefix(line, "Would uninstall cask"):
			currentKind = "cask"
			continue
		case strings.HasPrefix(line, "Would uninstall formula"):
			currentKind = "brew"
			continue
		case strings.HasPrefix(line, "Would untap"):
			currentKind = "tap"
			continue
		}

		// Skip the trailing instruction line and any other non-package lines.
		if strings.HasPrefix(line, "Run `brew") ||
			strings.HasPrefix(line, "Error:") ||
			strings.HasPrefix(line, "Warning:") {
			continue
		}
		pkgs = append(pkgs, UnstagedPackage{Name: line, Kind: currentKind})
	}
	return pkgs
}

// FormatUnstagedResults produces a human-readable summary of unstaged packages.
func FormatUnstagedResults(pkgs []UnstagedPackage) string {
	if len(pkgs) == 0 {
		return "✓ No packages found outside your Brewfile."
	}
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("⚠  %d package(s) installed but not in Brewfile:\n\n", len(pkgs)))
	for _, p := range pkgs {
		sb.WriteString(fmt.Sprintf("  [%s] %s\n", p.Kind, p.Name))
	}
	return sb.String()
}

// AddPackagesToBrewfile appends each package to the Brewfile using the correct
// Homebrew Bundle directive (brew "…" or cask "…").
// It writes the section header "# Added by maximus-cli" only on the first call;
// subsequent calls detect the header and append packages directly below it.
func AddPackagesToBrewfile(brewfilePath string, pkgs []UnstagedPackage) error {
	if len(pkgs) == 0 {
		return nil
	}

	const header = "# Added by maximus-cli"

	// Read existing content to check whether the header is already present.
	existing, err := os.ReadFile(brewfilePath)
	if err != nil {
		return fmt.Errorf("read brewfile %s: %w", brewfilePath, err)
	}
	hasHeader := strings.Contains(string(existing), header)

	f, err := os.OpenFile(brewfilePath, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("open brewfile %s: %w", brewfilePath, err)
	}
	defer f.Close()

	if !hasHeader {
		_, _ = fmt.Fprintln(f, "")
		_, _ = fmt.Fprintln(f, header)
	}

	for _, p := range pkgs {
		var line string
		switch p.Kind {
		case "tap":
			line = fmt.Sprintf("tap %q\n", p.Name)
		default:
			line = fmt.Sprintf("%s %q\n", p.Kind, p.Name)
		}
		if _, err := f.WriteString(line); err != nil {
			return fmt.Errorf("write %q to brewfile: %w", p.Name, err)
		}
	}
	return nil
}

// InfoVersion returns the installed version of formula/cask name by calling
// `brew info --json=v2 <name>`. Returns ("", nil) when not installed or on
// any non-critical error.
func InfoVersion(name string) (string, error) {
	cmd := exec.Command("brew", "info", "--json=v2", name)
	out, err := cmd.Output()
	if err != nil {
		return "", nil // non-zero exit is expected when not installed
	}

	// Minimal JSON structures we care about.
	var result struct {
		Formulae []struct {
			Installed []struct {
				Version string `json:"version"`
			} `json:"installed"`
		} `json:"formulae"`
		Casks []struct {
			Installed string `json:"installed"`
		} `json:"casks"`
	}
	if err := json.Unmarshal(out, &result); err != nil {
		return "", nil
	}
	for _, f := range result.Formulae {
		if len(f.Installed) > 0 {
			return f.Installed[0].Version, nil
		}
	}
	for _, c := range result.Casks {
		if c.Installed != "" {
			return c.Installed, nil
		}
	}
	return "", nil
}
