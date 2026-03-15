// Package brew provides wrappers for Homebrew command-line operations.
package brew

import (
	"bufio"
	"bytes"
	"fmt"
	"os/exec"
	"strings"
)

// runCommand executes a shell command and returns its combined stdout output.
func runCommand(name string, args ...string) (string, error) {
	cmd := exec.Command(name, args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("command %q failed: %w\n%s", name+" "+strings.Join(args, " "), err, string(out))
	}
	return strings.TrimSpace(string(out)), nil
}

// runCommandLines executes a shell command and returns output split by lines.
func runCommandLines(name string, args ...string) ([]string, error) {
	cmd := exec.Command(name, args...)
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("command %q failed: %w", name+" "+strings.Join(args, " "), err)
	}
	var lines []string
	scanner := bufio.NewScanner(bytes.NewReader(out))
	for scanner.Scan() {
		if t := strings.TrimSpace(scanner.Text()); t != "" {
			lines = append(lines, t)
		}
	}
	return lines, scanner.Err()
}

// sortedSet converts a string slice to a sorted, deduplicated map for O(1) lookups.
func sortedSet(items []string) map[string]struct{} {
	m := make(map[string]struct{}, len(items))
	for _, item := range items {
		m[item] = struct{}{}
	}
	return m
}
