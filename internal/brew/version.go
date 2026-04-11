package brew

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"maximus-cli/internal/db"
)

// PackageVersion holds version and date information for a single Brewfile entry.
type PackageVersion struct {
	// Name is the formula or cask token.
	Name string
	// Kind is "brew" or "cask".
	Kind string
	// Version is the currently installed version string.
	Version string
	// MetadataDate is the date reported by Homebrew for the installed revision
	// (installed_time for casks; the installed.time for formulae).
	MetadataDate time.Time
	// InstallDate is the date recorded in the internal database for this
	// version being installed via maximus-cli. Zero when not found.
	InstallDate time.Time
}

// brewInfoJSON is the top-level structure of `brew info --json=v2 --installed`.
type brewInfoJSON struct {
	Formulae []formulaJSON `json:"formulae"`
	Casks    []caskJSON    `json:"casks"`
}

type formulaJSON struct {
	Name      string          `json:"name"`
	FullName  string          `json:"full_name"`
	Installed []formulaInstalled `json:"installed"`
}

type formulaInstalled struct {
	Version string `json:"version"`
	Time    int64  `json:"time"` // Unix timestamp
}

type caskJSON struct {
	Token         string `json:"token"`
	Installed     string `json:"installed"`
	InstalledTime int64  `json:"installed_time"` // Unix timestamp
}

// ListVersions returns a slice of PackageVersion for every package listed in
// the Brewfile that is also currently installed. It enriches each entry with
// an install date from the internal database when available.
func ListVersions(brewfilePath string, database *db.DB) ([]PackageVersion, error) {
	// 1. Collect names from the Brewfile.
	brewfileEntries, err := brewfilePackages(brewfilePath)
	if err != nil {
		return nil, fmt.Errorf("version: list brewfile packages: %w", err)
	}
	if len(brewfileEntries) == 0 {
		return nil, nil
	}

	// 2. Fetch full info for all installed packages.
	info, err := installedInfo()
	if err != nil {
		return nil, fmt.Errorf("version: fetch installed info: %w", err)
	}

	// 3. Build a lookup set from brewfile entries (base name → kind).
	type entry struct{ name, kind string }
	lookup := make(map[string]string, len(brewfileEntries)) // name -> kind
	for _, e := range brewfileEntries {
		lookup[e.name] = e.kind
	}

	// 4. Intersect installed packages with the Brewfile set.
	var results []PackageVersion

	for _, f := range info.Formulae {
		if len(f.Installed) == 0 {
			continue
		}
		base := baseName(f.Name)
		kind, inBrewfile := lookup[f.Name]
		if !inBrewfile {
			kind, inBrewfile = lookup[base]
		}
		if !inBrewfile {
			kind = "brew"
			_, inBrewfile = lookup[f.FullName]
		}
		if !inBrewfile {
			continue
		}
		inst := f.Installed[0]
		pv := PackageVersion{
			Name:    base,
			Kind:    kind,
			Version: inst.Version,
		}
		if inst.Time > 0 {
			pv.MetadataDate = time.Unix(inst.Time, 0)
		}
		if database != nil {
			if t, err := database.GetInstallDate(base, inst.Version); err == nil {
				pv.InstallDate = t
			}
		}
		results = append(results, pv)
	}

	for _, c := range info.Casks {
		if c.Installed == "" {
			continue
		}
		kind, inBrewfile := lookup[c.Token]
		if !inBrewfile {
			continue
		}
		pv := PackageVersion{
			Name:    c.Token,
			Kind:    kind,
			Version: c.Installed,
		}
		if c.InstalledTime > 0 {
			pv.MetadataDate = time.Unix(c.InstalledTime, 0)
		}
		if database != nil {
			if t, err := database.GetInstallDate(c.Token, c.Installed); err == nil {
				pv.InstallDate = t
			}
		}
		results = append(results, pv)
	}

	return results, nil
}

// brewfileEntry is a package name and kind parsed from `brew bundle list`.
type brewfileEntry struct{ name, kind string }

// brewfilePackages returns the set of packages listed in the Brewfile via
// `brew bundle list --all`. Each entry carries the inferred kind.
func brewfilePackages(brewfilePath string) ([]brewfileEntry, error) {
	// brew bundle list --all outputs lines like:
	//   formulaname
	//   homebrew/cask/caskname   (when scoped)
	// We cannot distinguish brew vs cask from the plain list, so we query
	// the formulae and casks separately.
	formulaeLines, err := runCommandLines("brew", "bundle", "list", "--brews", "--file", brewfilePath)
	if err != nil {
		// Non-fatal: may be an empty section.
		formulaeLines = nil
	}
	caskLines, err := runCommandLines("brew", "bundle", "list", "--casks", "--file", brewfilePath)
	if err != nil {
		caskLines = nil
	}

	var entries []brewfileEntry
	for _, line := range formulaeLines {
		entries = append(entries, brewfileEntry{name: strings.TrimSpace(line), kind: "brew"})
	}
	for _, line := range caskLines {
		entries = append(entries, brewfileEntry{name: strings.TrimSpace(line), kind: "cask"})
	}
	return entries, nil
}

// installedInfo calls `brew info --json=v2 --installed` and parses its output.
func installedInfo() (*brewInfoJSON, error) {
	cmd := exec.Command("brew", "info", "--json=v2", "--installed")
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("brew info: %w", err)
	}
	var info brewInfoJSON
	if err := json.Unmarshal(out, &info); err != nil {
		return nil, fmt.Errorf("brew info: parse json: %w", err)
	}
	return &info, nil
}
