package db

import (
	"testing"
	"time"
)

func TestGitHubRepoStarsHistory(t *testing.T) {
	// 1. Open database in memory
	d, err := Open(":memory:")
	if err != nil {
		t.Fatalf("Failed to open memory db: %v", err)
	}
	defer d.Close()

	// 2. Insert mock repo
	repo := GitHubRepo{
		URL:          "https://github.com/test-owner/test-repo",
		Name:         "test-repo",
		Organization: "test-owner",
		Description:  "A test repo",
		Language:     "Go",
		Stars:        42,
		UpdatedAt:    time.Now().Add(-24 * time.Hour),
		FirstCommit:  time.Now().Add(-30 * 24 * time.Hour),
		SizeBytes:    1024,
		Category:     "test",
		Source:       "manual",
		AddedAt:      time.Now(),
	}

	if err := d.UpsertGitHubRepo(repo); err != nil {
		t.Fatalf("Failed to upsert repo: %v", err)
	}

	// 3. Verify repo refs
	refs, err := d.GetRepoRefs()
	if err != nil {
		t.Fatalf("Failed to get repo refs: %v", err)
	}
	if len(refs) != 1 {
		t.Fatalf("Expected 1 repo ref, got %d", len(refs))
	}
	if refs[0].URL != repo.URL {
		t.Errorf("Expected URL %q, got %q", repo.URL, refs[0].URL)
	}

	repoID := refs[0].ID

	// 4. Test snapshot insertion
	sampledAt1 := time.Now().Add(-1 * time.Hour).Truncate(time.Second)
	if err := d.InsertStarSnapshot(repoID, 40, sampledAt1); err != nil {
		t.Fatalf("Failed to insert star snapshot: %v", err)
	}

	sampledAt2 := time.Now().Truncate(time.Second)
	if err := d.InsertStarSnapshot(repoID, 42, sampledAt2); err != nil {
		t.Fatalf("Failed to insert star snapshot: %v", err)
	}

	// 5. Query and verify star history (ordered newest first)
	history, err := d.GetStarHistory(repoID, 10)
	if err != nil {
		t.Fatalf("Failed to get star history: %v", err)
	}

	if len(history) != 2 {
		t.Fatalf("Expected 2 history records, got %d", len(history))
	}

	// Check sorting (newest first)
	if history[0].Stars != 42 {
		t.Errorf("Expected first snapshot stars to be 42, got %d", history[0].Stars)
	}
	if history[1].Stars != 40 {
		t.Errorf("Expected second snapshot stars to be 40, got %d", history[1].Stars)
	}

	// 6. Test settings / cursor
	lastID := int64(123)
	lastAt := time.Now().Truncate(time.Second)
	if err := d.SetRefreshCursor(lastID, lastAt); err != nil {
		t.Fatalf("Failed to set refresh cursor: %v", err)
	}

	cursorID, cursorAt, err := d.GetRefreshCursor()
	if err != nil {
		t.Fatalf("Failed to get refresh cursor: %v", err)
	}

	if cursorID != lastID {
		t.Errorf("Expected cursor ID %d, got %d", lastID, cursorID)
	}
	if !cursorAt.UTC().Equal(lastAt.UTC()) {
		t.Errorf("Expected cursor timestamp %v, got %v", lastAt.UTC(), cursorAt.UTC())
	}
}
