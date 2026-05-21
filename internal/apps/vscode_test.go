package apps

import (
	"os"
	"path/filepath"
	"testing"

	"maximus-cli/internal/db"
)

func TestVSCodeIntegration(t *testing.T) {
	// Create a temporary database file
	tempDir, err := os.MkdirTemp("", "maximus-vscode-test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	dbPath := filepath.Join(tempDir, "maximus.db")
	database, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("failed to open test database: %v", err)
	}
	defer database.Close()

	// 1. Test ScanVSCode
	diff, err := ScanVSCode(database)
	if err != nil {
		t.Fatalf("ScanVSCode failed: %v", err)
	}

	t.Logf("ScanVSCode success. HasChanges: %v, NewVersion: %s", diff.HasAnyChange, diff.NewVersion)

	// 2. Test LoadVSCodeSummary
	summary, err := LoadVSCodeSummary(database)
	if err != nil {
		t.Fatalf("LoadVSCodeSummary failed: %v", err)
	}

	if summary.Version == "" {
		t.Errorf("expected version to be populated, got empty")
	}

	if len(summary.Paths) == 0 {
		t.Errorf("expected paths to be configured, got 0")
	}

	// 3. Test LoadVSCodeProfiles
	profiles, err := LoadVSCodeProfiles(database, true)
	if err != nil {
		t.Fatalf("LoadVSCodeProfiles failed: %v", err)
	}

	if len(profiles) == 0 {
		t.Errorf("expected profiles to have at least one entry (Default), got 0")
	}

	for _, p := range profiles {
		t.Logf("Profile: %s (Default: %v), path: %s", p.Name, p.IsDefault, p.ProfilePath)
		t.Logf("  Extensions count: %d", len(p.Extensions))
		t.Logf("  Projects count: %d", len(p.Projects))
	}
}
