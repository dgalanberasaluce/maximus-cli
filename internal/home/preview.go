package home

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

// FormatSize returns a human-readable representation of a byte count.
// It uses the appropriate unit (B, KB, MB, GB) based on magnitude.
// Directories that use shallow sizing are prefixed with ~.
func FormatSize(bytes int64, shallow bool) string {
	prefix := ""
	if shallow {
		prefix = "~"
	}
	const (
		kb = 1024
		mb = 1024 * kb
		gb = 1024 * mb
	)
	switch {
	case bytes >= gb:
		return fmt.Sprintf("%s%.1f GB", prefix, float64(bytes)/float64(gb))
	case bytes >= mb:
		return fmt.Sprintf("%s%.1f MB", prefix, float64(bytes)/float64(mb))
	case bytes >= kb:
		return fmt.Sprintf("%s%.1f KB", prefix, float64(bytes)/float64(kb))
	default:
		return fmt.Sprintf("%s%d B", prefix, bytes)
	}
}

// GetPreview retrieves a printable preview of a dotfile or directory inside the user's home directory.
func GetPreview(name string, isDir bool) (title string, content string) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "Error", fmt.Sprintf("Could not retrieve home directory: %v", err)
	}

	fullPath := filepath.Join(homeDir, name)

	if isDir {
		title = fmt.Sprintf("Directory Contents: %s", name)
		entries, err := os.ReadDir(fullPath)
		if err != nil {
			return title, fmt.Sprintf("Error reading directory:\n%v", err)
		}

		if len(entries) == 0 {
			return title, " (empty directory)"
		}

		// Header for the enriched listing.
		var sb strings.Builder
		sb.WriteString(fmt.Sprintf("  %-28s  %-6s  %-10s  %-10s  %s\n", "NAME", "TYPE", "MODIFIED", "CREATED", "SIZE"))
		sb.WriteString("  " + strings.Repeat("─", 72) + "\n")

		for i, entry := range entries {
			if i >= 50 {
				sb.WriteString(fmt.Sprintf("\n  ... and %d more items", len(entries)-50))
				break
			}

			indicator := "File"
			if entry.IsDir() {
				indicator = "Dir"
			}

			info, err := entry.Info()
			var modStr, creStr, sizeStr string
			if err == nil {
				modStr = info.ModTime().Format("2006-01-02")
				creStr = entryCreationTime(info).Format("2006-01-02")
				sz := info.Size()
				sizeStr = FormatSize(sz, false)
			} else {
				modStr = "—"
				creStr = "—"
				sizeStr = "—"
			}

			entryName := entry.Name()
			if len(entryName) > 28 {
				entryName = entryName[:27] + "…"
			}

			sb.WriteString(fmt.Sprintf("  %-28s  %-6s  %-10s  %-10s  %s\n",
				entryName, indicator, modStr, creStr, sizeStr))
		}
		return title, sb.String()
	}

	// It's a file
	title = fmt.Sprintf("File Contents: %s", name)
	file, err := os.Open(fullPath)
	if err != nil {
		return title, fmt.Sprintf("Error opening file:\n%v", err)
	}
	defer file.Close()

	// Read first 4KB
	buf := make([]byte, 4096)
	n, err := file.Read(buf)
	if err != nil && err != io.EOF {
		return title, fmt.Sprintf("Error reading file:\n%v", err)
	}

	if n == 0 {
		return title, " (empty file)"
	}

	// Slice read bytes
	readBytes := buf[:n]

	// Basic binary file check: scan for null bytes
	if !isText(readBytes) {
		return title, " [Binary file preview not supported]"
	}

	content = string(readBytes)
	if n == 4096 {
		content += "\n... (preview truncated to 4KB)"
	}
	return title, content
}

// entryCreationTime extracts the birth time of a file on macOS, falling back
// to modification time on other platforms.
func entryCreationTime(fi os.FileInfo) time.Time {
	if stat, ok := fi.Sys().(*syscall.Stat_t); ok {
		return time.Unix(stat.Birthtimespec.Sec, stat.Birthtimespec.Nsec)
	}
	return fi.ModTime()
}

// isText returns true if data contains no null bytes.
func isText(data []byte) bool {
	for _, b := range data {
		if b == 0 {
			return false
		}
	}
	return true
}
