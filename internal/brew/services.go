package brew

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Service represents a Homebrew managed service.
type Service struct {
	Name       string
	Status     string // "started", "stopped", "none", "error"
	User       string
	File       string
	ExitCode   int
	Version    string
	Path       string
	Desc       string
	Homepage   string
	LogPath    string
	ErrLogPath string
}

type serviceJSON struct {
	Name     string  `json:"name"`
	Status   string  `json:"status"`
	User     *string `json:"user"`
	File     *string `json:"file"`
	ExitCode *int    `json:"exit_code"`
}

type servicesBrewInfoJSON struct {
	Formulae []servicesFormulaJSON `json:"formulae"`
	Casks    []servicesCaskJSON    `json:"casks"`
}

type servicesFormulaJSON struct {
	Name      string                    `json:"name"`
	Desc      string                    `json:"desc"`
	Homepage  string                    `json:"homepage"`
	Installed []servicesFormulaInstalled `json:"installed"`
	LinkedKeg string                    `json:"linked_keg"`
	Service   *servicesFormulaService   `json:"service"`
}

type servicesFormulaInstalled struct {
	Version string `json:"version"`
}

type servicesFormulaService struct {
	LogPath      string `json:"log_path"`
	ErrorLogPath string `json:"error_log_path"`
}

type servicesCaskJSON struct {
	Token     string `json:"token"`
	Installed string `json:"installed"`
	Desc      string `json:"desc"`
	Homepage  string `json:"homepage"`
}

// ListServices lists all Homebrew services and enriches them with formula details.
func ListServices() ([]Service, error) {
	// 1. Run brew services list --json
	cmd := exec.Command("brew", "services", "list", "--json")
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to run brew services list: %w", err)
	}

	var rawServices []serviceJSON
	if err := json.Unmarshal(out, &rawServices); err != nil {
		return nil, fmt.Errorf("failed to parse brew services list JSON: %w", err)
	}

	if len(rawServices) == 0 {
		return nil, nil
	}

	// 2. Extract names for batch brew info query
	var names []string
	servicesMap := make(map[string]*Service, len(rawServices))
	var services []Service

	for _, rs := range rawServices {
		s := Service{
			Name:   rs.Name,
			Status: rs.Status,
		}
		if rs.User != nil {
			s.User = *rs.User
		}
		if rs.File != nil {
			s.File = *rs.File
			if s.File != "" {
				s.Path = filepath.Dir(filepath.Dir(s.File))
			}
		}
		if s.Path == "" {
			s.Path = "/opt/homebrew/opt/" + s.Name
		}
		if rs.ExitCode != nil {
			s.ExitCode = *rs.ExitCode
		}
		servicesMap[rs.Name] = &s
		names = append(names, rs.Name)
	}

	// 3. Batch query brew info --json=v2 to get formula details
	infoCmd := exec.Command("brew", append([]string{"info", "--json=v2"}, names...)...)
	infoOut, err := infoCmd.Output()
	if err == nil {
		var info servicesBrewInfoJSON
		if err := json.Unmarshal(infoOut, &info); err == nil {
			// Map formulae
			for _, f := range info.Formulae {
				if s, ok := servicesMap[f.Name]; ok {
					s.Desc = f.Desc
					s.Homepage = f.Homepage
					if f.LinkedKeg != "" {
						s.Version = f.LinkedKeg
					} else if len(f.Installed) > 0 {
						s.Version = f.Installed[0].Version
					}
					if f.Service != nil {
						s.LogPath = f.Service.LogPath
						s.ErrLogPath = f.Service.ErrorLogPath
					}
				}
			}
			// Map casks
			for _, c := range info.Casks {
				if s, ok := servicesMap[c.Token]; ok {
					s.Desc = c.Desc
					s.Homepage = c.Homepage
					s.Version = c.Installed
				}
			}
		}
	}

	// Convert map back to slice in the original order
	for _, name := range names {
		if s, ok := servicesMap[name]; ok {
			services = append(services, *s)
		}
	}

	return services, nil
}

// ServiceAction executes a brew services subcommand on a specific service.
func ServiceAction(name, action string) (string, error) {
	// action can be: start, stop, restart, kill
	cmd := exec.Command("brew", "services", action, name)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("failed to %s service %s: %w\n%s", action, name, err, string(out))
	}
	return strings.TrimSpace(string(out)), nil
}

// logCandidates builds the deduplicated list of candidate log file paths for a service.
func logCandidates(s Service) []string {
	raw := []string{
		s.LogPath,
		s.ErrLogPath,
		filepath.Join("/opt/homebrew/var/log", s.Name+".log"),
		filepath.Join("/opt/homebrew/var/log", s.Name+".err.log"),
		filepath.Join("/usr/local/var/log", s.Name+".log"),
		filepath.Join("/usr/local/var/log", s.Name+".err.log"),
		filepath.Join(os.Getenv("HOME"), "Library/Logs", s.Name+".log"),
		filepath.Join(os.Getenv("HOME"), "Library/Logs", s.Name+".err.log"),
	}
	seen := make(map[string]bool)
	var out []string
	for _, p := range raw {
		if p == "" || seen[p] {
			continue
		}
		seen[p] = true
		out = append(out, p)
	}
	return out
}

// ServiceLogPaths returns the paths of log files that actually exist on disk.
func ServiceLogPaths(s Service) []string {
	var existing []string
	for _, p := range logCandidates(s) {
		if _, err := os.Stat(p); err == nil {
			existing = append(existing, p)
		}
	}
	return existing
}

// ServiceLogFile returns the first existing log file path suitable for streaming,
// or an empty string if none found.
func ServiceLogFile(s Service) string {
	for _, p := range logCandidates(s) {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return ""
}

// ServiceLogSize returns the total size of all existing log files in bytes and
// as a human-readable string (e.g. "1.2 MB").
func ServiceLogSize(s Service) (int64, string) {
	var total int64
	for _, p := range ServiceLogPaths(s) {
		if info, err := os.Stat(p); err == nil {
			total += info.Size()
		}
	}
	return total, humanSize(total)
}

// humanSize converts a byte count to a human-readable string.
func humanSize(b int64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(b)/float64(div), "KMGTPE"[exp])
}

// ServiceLogs reads the tail of all existing log files for a service.
func ServiceLogs(s Service, lines int) string {
	paths := ServiceLogPaths(s)
	if len(paths) == 0 {
		checked := logCandidates(s)
		return "No logs found or service has not written any logs yet.\nLocations checked:\n  - " + strings.Join(checked, "\n  - ")
	}

	var sb strings.Builder
	for _, p := range paths {
		if out, err := readTail(p, lines); err == nil && strings.TrimSpace(out) != "" {
			if sb.Len() > 0 {
				sb.WriteString("\n")
			}
			sb.WriteString(fmt.Sprintf("--- Log: %s ---\n", filepath.Base(p)))
			sb.WriteString(out)
		}
	}

	if sb.Len() == 0 {
		return "Log files exist but are empty."
	}
	return sb.String()
}

func readTail(path string, lines int) (string, error) {
	if _, err := os.Stat(path); err != nil {
		return "", err
	}
	cmd := exec.Command("tail", "-n", fmt.Sprintf("%d", lines), path)
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return string(out), nil
}
