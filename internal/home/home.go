package home

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"maximus-cli/internal/db"
)

// dirSizeShallow returns the sum of sizes of immediate (non-recursive) children
// of the given directory. Symbolic links are counted by their own size.
// Returns 0 on error.
func dirSizeShallow(path string) int64 {
	entries, err := os.ReadDir(path)
	if err != nil {
		return 0
	}
	var total int64
	for _, e := range entries {
		info, err := e.Info()
		if err == nil {
			total += info.Size()
		}
	}
	return total
}

type ScanResult struct {
	Entries  []db.DotfileEntry
	Added    []string
	Updated  []string
	Deleted  []string
	LastScan time.Time
}

// ScanDotfiles scans the user's home directory for dotfiles/dotdirectories,
// populates the SQLite3 database, and returns the scan result.
func ScanDotfiles(database *db.DB) (ScanResult, error) {
	var result ScanResult

	homeDir, err := os.UserHomeDir()
	if err != nil {
		return result, fmt.Errorf("home: get home dir: %w", err)
	}

	// 1. Fetch old entries before the scan
	oldEntries, err := database.GetDotfiles("", "name", true, -1, 0)
	if err != nil {
		return result, fmt.Errorf("home: fetch existing dotfiles: %w", err)
	}

	// 2. Identify the last scan time and build a map of old entries for fast lookups
	oldMap := make(map[string]db.DotfileEntry)
	for _, entry := range oldEntries {
		oldMap[entry.Name] = entry
		if entry.ScannedAt.After(result.LastScan) {
			result.LastScan = entry.ScannedAt
		}
	}

	entries, err := os.ReadDir(homeDir)
	if err != nil {
		return result, fmt.Errorf("home: read home dir: %w", err)
	}

	seen := make(map[string]bool)

	for _, entry := range entries {
		name := entry.Name()
		// We only want files/folders starting with a dot, excluding '.' and '..'
		if !strings.HasPrefix(name, ".") || name == "." || name == ".." {
			continue
		}

		fullPath := filepath.Join(homeDir, name)
		info, err := os.Lstat(fullPath)
		if err != nil {
			// Skip files we can't stat (e.g. permission denied)
			continue
		}

		isDir := info.IsDir()
		tool := InferTool(name)
		modifiedAt := info.ModTime()
		createdAt := getCreationTime(info)

		// Compute size: files use their direct size; directories use a shallow walk.
		var sizeBytes int64
		if isDir {
			sizeBytes = dirSizeShallow(fullPath)
		} else {
			sizeBytes = info.Size()
		}

		// 3. Compare with existing state
		if old, exists := oldMap[name]; exists {
			// Compare modification times (Unix timestamp to ignore timezone/nanosecond diffs)
			if modifiedAt.Unix() != old.ModifiedAt.Unix() {
				result.Updated = append(result.Updated, name)
			}
		} else {
			result.Added = append(result.Added, name)
		}

		seen[name] = true

		// Save to DB
		err = database.UpsertDotfile(name, isDir, tool, modifiedAt, createdAt, sizeBytes)
		if err != nil {
			return result, fmt.Errorf("home: save dotfile %q: %w", name, err)
		}
	}

	// 4. Identify deleted files
	for name := range oldMap {
		if !seen[name] {
			result.Deleted = append(result.Deleted, name)
			if err := database.DeleteDotfile(name); err != nil {
				return result, fmt.Errorf("home: delete missing dotfile %q: %w", name, err)
			}
		}
	}

	// 5. Fetch back all stored dotfiles from DB (sorted by name by default)
	dbEntries, err := database.GetDotfiles("", "name", true, -1, 0)
	if err != nil {
		return result, fmt.Errorf("home: fetch dotfiles: %w", err)
	}
	result.Entries = dbEntries

	return result, nil
}

// getCreationTime extracts creation time of a file on macOS (using syscall.Stat_t.Birthtimespec)
// and falls back to modification time if birthtime is unavailable or not on macOS.
func getCreationTime(fi os.FileInfo) time.Time {
	if stat, ok := fi.Sys().(*syscall.Stat_t); ok {
		return time.Unix(stat.Birthtimespec.Sec, stat.Birthtimespec.Nsec)
	}
	return fi.ModTime()
}
