package brew

import (
	"errors"
	"fmt"
	"strings"
)

// DiffResult holds information about a single package that is in the Brewfile
// and has an available upgrade.
type DiffResult struct {
	Name           string
	CurrentVersion string
	LatestVersion  string
}

// SmartDiff is a native Go implementation of the equivalent shell pipeline:
//
//	comm -12 <(brew bundle list --all --file <brewfile> | sort) \
//	         <(brew outdated --quiet | sort) \
//	| xargs brew outdated
//
// It returns a list of DiffResult entries, showing which packages from the
// Brewfile have a newer version available, along with current and future versions.
func SmartDiff(brewfilePath string) ([]DiffResult, error) {
	// Step 1: get packages listed in the Brewfile.
	bundleLines, err := runCommandLines("brew", "bundle", "list", "--all", "--file", brewfilePath)
	if err != nil {
		return nil, fmt.Errorf("smart diff: read brewfile %q: %w", brewfilePath, err)
	}
	if len(bundleLines) == 0 {
		return nil, errors.New("smart diff: Brewfile appears to be empty or not found")
	}
	brewfileSet := sortedSet(bundleLines)

	// Step 2: get ALL outdated packages with version info in one shot.
	// `brew outdated` (no flags) returns lines like:
	//   name (currentVersion) < latestVersion
	// We avoid passing explicit package names to brew to prevent exit status 1
	// for casks or tap-scoped formulae.
	verboseLines, err := runCommandLines("brew", "outdated")
	if err != nil {
		return nil, fmt.Errorf("smart diff: check outdated packages: %w", err)
	}

	// Step 3: parse the verbose output and keep only entries present in the Brewfile.
	all := parseDiffLines(verboseLines)
	var results []DiffResult
	for _, r := range all {
		// Match by the base name (strip any tap prefix, e.g. "homebrew/cask/foo" -> "foo").
		base := baseName(r.Name)
		if _, ok := brewfileSet[r.Name]; ok {
			results = append(results, r)
		} else if _, ok := brewfileSet[base]; ok {
			results = append(results, r)
		}
	}

	return results, nil
}

// baseName extracts the last component of a slash-separated package name.
// e.g. "homebrew/cask/lazydocker" -> "lazydocker"
func baseName(name string) string {
	parts := strings.Split(name, "/")
	return parts[len(parts)-1]
}

// parseDiffLines converts lines from `brew outdated` into DiffResult structs.
// Expected format: "name (currentVersion) < latestVersion"
func parseDiffLines(lines []string) []DiffResult {
	results := make([]DiffResult, 0, len(lines))
	for _, line := range lines {
		// Split on " < " to get left and right.
		parts := strings.SplitN(line, " < ", 2)
		if len(parts) != 2 {
			continue
		}
		latest := strings.TrimSpace(parts[1])
		left := strings.TrimSpace(parts[0]) // e.g. "name (current)"

		// Extract name and current version from "name (current)".
		parenOpen := strings.LastIndex(left, " (")
		if parenOpen < 0 {
			continue
		}
		name := strings.TrimSpace(left[:parenOpen])
		current := strings.Trim(strings.TrimSpace(left[parenOpen+1:]), "()")

		results = append(results, DiffResult{
			Name:           name,
			CurrentVersion: current,
			LatestVersion:  latest,
		})
	}
	return results
}

// FormatDiffResults renders a slice of DiffResult as a display-ready string.
func FormatDiffResults(results []DiffResult) string {
	if len(results) == 0 {
		return "✓ Everything in your Brewfile is up to date!"
	}
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("%-30s  %-20s  %s\n", "PACKAGE", "CURRENT", "AVAILABLE"))
	sb.WriteString(strings.Repeat("─", 70) + "\n")
	for _, r := range results {
		sb.WriteString(fmt.Sprintf("%-30s  %-20s  %s\n", r.Name, r.CurrentVersion, r.LatestVersion))
	}
	return sb.String()
}
