package brew

import "fmt"

// Cleanup runs `brew cleanup` followed by `brew autoremove` to remove
// stale formulae and unused dependencies.
// Returns the combined output of both commands.
func Cleanup() (string, error) {
	cleanupOut, err := runCommand("brew", "cleanup")
	if err != nil {
		return "", fmt.Errorf("brew cleanup: %w", err)
	}

	autoremoveOut, err := runCommand("brew", "autoremove")
	if err != nil {
		return "", fmt.Errorf("brew autoremove: %w", err)
	}

	combined := cleanupOut
	if autoremoveOut != "" {
		combined += "\n\n" + autoremoveOut
	}
	return combined, nil
}
