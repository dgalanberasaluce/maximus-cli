package home

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

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

		var sb strings.Builder
		for i, entry := range entries {
			if i >= 50 {
				sb.WriteString(fmt.Sprintf("\n... and %d more items", len(entries)-50))
				break
			}
			indicator := "📄"
			if entry.IsDir() {
				indicator = "📁"
			}
			sb.WriteString(fmt.Sprintf("%s %s\n", indicator, entry.Name()))
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

// isText returns true if data contains no null bytes.
func isText(data []byte) bool {
	for _, b := range data {
		if b == 0 {
			return false
		}
	}
	return true
}
