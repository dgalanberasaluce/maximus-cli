package db

import (
	"database/sql"
	"fmt"
	"strings"
	"time"
)

type GitHubRepo struct {
	ID           int64
	URL          string
	Name         string
	Organization string
	Description  string
	Language     string
	Stars        int
	UpdatedAt    time.Time
	FirstCommit  time.Time
	SizeBytes    int
	Category     string
	Source       string
	Notes        string
	AddedAt      time.Time
}

type StarSnapshot struct {
	ID        int64
	RepoID    int64
	Stars     int
	SampledAt time.Time
}

func (d *DB) migrateGitHubRepos() error {
	q := `CREATE TABLE IF NOT EXISTS github_repos (
		id           INTEGER PRIMARY KEY AUTOINCREMENT,
		url          TEXT NOT NULL UNIQUE,
		name         TEXT NOT NULL,
		organization TEXT NOT NULL DEFAULT '',
		description  TEXT NOT NULL DEFAULT '',
		language     TEXT NOT NULL DEFAULT '',
		stars        INTEGER NOT NULL DEFAULT 0,
		updated_at   DATETIME NOT NULL,
		first_commit DATETIME,
		size_bytes   INTEGER NOT NULL DEFAULT 0,
		category     TEXT NOT NULL DEFAULT '',
		source       TEXT NOT NULL DEFAULT 'manual',
		notes        TEXT NOT NULL DEFAULT '',
		added_at     DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
	)`
	if _, err := d.conn.Exec(q); err != nil {
		return fmt.Errorf("migration github_repos failed: %w", err)
	}

	indexes := []string{
		`CREATE INDEX IF NOT EXISTS idx_github_repos_name ON github_repos(name)`,
		`CREATE INDEX IF NOT EXISTS idx_github_repos_category ON github_repos(category)`,
		`CREATE INDEX IF NOT EXISTS idx_github_repos_source ON github_repos(source)`,
	}
	for _, idx := range indexes {
		if _, err := d.conn.Exec(idx); err != nil {
			return fmt.Errorf("migration github_repos index failed: %w", err)
		}
	}
	return nil
}

type RepoRef struct {
	ID  int64
	URL string
}

func (d *DB) GetRepoRefs() ([]RepoRef, error) {
	rows, err := d.conn.Query(`SELECT id, url FROM github_repos ORDER BY id ASC`)
	if err != nil {
		return nil, fmt.Errorf("db: get repo refs: %w", err)
	}
	defer rows.Close()
	var refs []RepoRef
	for rows.Next() {
		var ref RepoRef
		if err := rows.Scan(&ref.ID, &ref.URL); err != nil {
			return nil, fmt.Errorf("db: scan repo ref %d: %w", ref.ID, err)
		}
		refs = append(refs, ref)
	}
	return refs, rows.Err()
}

func (d *DB) UpsertGitHubRepo(r GitHubRepo) error {
	_, err := d.conn.Exec(
		`INSERT INTO github_repos(url, name, organization, description, language, stars, updated_at, first_commit, size_bytes, category, source, notes, added_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(url) DO UPDATE SET
		 	name=excluded.name,
		 	organization=excluded.organization,
		 	description=excluded.description,
		 	language=excluded.language,
		 	stars=excluded.stars,
		 	updated_at=excluded.updated_at,
		 	first_commit=excluded.first_commit,
		 	size_bytes=excluded.size_bytes,
		 	category=CASE WHEN excluded.category != '' THEN excluded.category ELSE github_repos.category END,
		 	source=CASE WHEN excluded.source != '' AND excluded.source != 'manual' THEN excluded.source ELSE github_repos.source END,
		 	notes=CASE WHEN excluded.notes != '' THEN excluded.notes ELSE github_repos.notes END`,
		r.URL, r.Name, r.Organization, r.Description, r.Language, r.Stars,
		r.UpdatedAt.UTC().Format(time.RFC3339),
		r.FirstCommit.UTC().Format(time.RFC3339),
		r.SizeBytes, r.Category, r.Source, r.Notes,
		r.AddedAt.UTC().Format(time.RFC3339),
	)
	if err != nil {
		return fmt.Errorf("db: upsert github_repo %q: %w", r.URL, err)
	}
	return nil
}

func (d *DB) GetGitHubRepos(filter string, sortBy string, asc bool, limit, offset int) ([]GitHubRepo, error) {
	var query strings.Builder
	query.WriteString(`SELECT id, url, name, organization, description, language, stars, updated_at, first_commit, size_bytes, category, source, notes, added_at FROM github_repos`)

	var args []any
	if filter != "" {
		query.WriteString(` WHERE name LIKE ? OR organization LIKE ? OR category LIKE ? OR description LIKE ?`)
		f := "%" + filter + "%"
		args = append(args, f, f, f, f)
	}

	validSort := map[string]string{
		"name":         "name",
		"organization": "organization",
		"category":     "category",
		"stars":        "stars",
		"language":     "language",
		"updated_at":   "updated_at",
		"added_at":     "added_at",
	}
	sc, ok := validSort[strings.ToLower(sortBy)]
	if !ok {
		sc = "name"
	}

	dir := "ASC"
	if !asc {
		dir = "DESC"
	}
	query.WriteString(fmt.Sprintf(` ORDER BY %s %s`, sc, dir))

	if limit > 0 {
		query.WriteString(` LIMIT ? OFFSET ?`)
		args = append(args, limit, offset)
	}

	rows, err := d.conn.Query(query.String(), args...)
	if err != nil {
		return nil, fmt.Errorf("db: query github_repos: %w", err)
	}
	defer rows.Close()

	var repos []GitHubRepo
	for rows.Next() {
		var r GitHubRepo
		var upStr, fcStr, adStr sql.NullString
		if err := rows.Scan(&r.ID, &r.URL, &r.Name, &r.Organization, &r.Description, &r.Language, &r.Stars, &upStr, &fcStr, &r.SizeBytes, &r.Category, &r.Source, &r.Notes, &adStr); err != nil {
			return nil, fmt.Errorf("db: scan github_repo: %w", err)
		}
		if upStr.Valid {
			r.UpdatedAt, _ = time.Parse(time.RFC3339, upStr.String)
		}
		if fcStr.Valid {
			r.FirstCommit, _ = time.Parse(time.RFC3339, fcStr.String)
		}
		if adStr.Valid {
			r.AddedAt, _ = time.Parse(time.RFC3339, adStr.String)
		}
		repos = append(repos, r)
	}
	return repos, rows.Err()
}

func (d *DB) CountGitHubRepos(filter string) (int, error) {
	var count int
	var err error
	if filter != "" {
		err = d.conn.QueryRow(
			`SELECT COUNT(*) FROM github_repos WHERE name LIKE ? OR organization LIKE ? OR category LIKE ? OR description LIKE ?`,
			"%"+filter+"%", "%"+filter+"%", "%"+filter+"%", "%"+filter+"%",
		).Scan(&count)
	} else {
		err = d.conn.QueryRow(`SELECT COUNT(*) FROM github_repos`).Scan(&count)
	}
	if err != nil {
		return 0, fmt.Errorf("db: count github_repos: %w", err)
	}
	return count, nil
}

func (d *DB) DeleteGitHubRepo(id int64) error {
	_, err := d.conn.Exec(`DELETE FROM github_repos WHERE id=?`, id)
	if err != nil {
		return fmt.Errorf("db: delete github_repo %d: %w", id, err)
	}
	return nil
}

func (d *DB) BulkInsertGitHubRepos(repos []GitHubRepo) error {
	tx, err := d.conn.Begin()
	if err != nil {
		return fmt.Errorf("db: begin tx for bulk insert: %w", err)
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare(
		`INSERT OR IGNORE INTO github_repos(url, name, organization, description, language, stars, updated_at, first_commit, size_bytes, category, source, notes, added_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
	)
	if err != nil {
		return fmt.Errorf("db: prepare bulk insert: %w", err)
	}
	defer stmt.Close()

	for _, r := range repos {
		_, err := stmt.Exec(r.URL, r.Name, r.Organization, r.Description, r.Language, r.Stars,
			r.UpdatedAt.UTC().Format(time.RFC3339),
			r.FirstCommit.UTC().Format(time.RFC3339),
			r.SizeBytes, r.Category, r.Source, r.Notes,
			r.AddedAt.UTC().Format(time.RFC3339),
		)
		if err != nil {
			return fmt.Errorf("db: bulk insert repo %q: %w", r.URL, err)
		}
	}

	return tx.Commit()
}

// UpdateGitHubRepoCategory updates only the category field for a given repo ID.
func (d *DB) UpdateGitHubRepoCategory(id int64, category string) error {
	_, err := d.conn.Exec(`UPDATE github_repos SET category=? WHERE id=?`, category, id)
	if err != nil {
		return fmt.Errorf("db: update category for repo %d: %w", id, err)
	}
	return nil
}

// GetGitHubRepoCategories returns all distinct non-empty categories sorted alphabetically.
func (d *DB) GetGitHubRepoCategories() ([]string, error) {
	rows, err := d.conn.Query(`SELECT DISTINCT category FROM github_repos WHERE category != '' ORDER BY category ASC`)
	if err != nil {
		return nil, fmt.Errorf("db: get categories: %w", err)
	}
	defer rows.Close()
	var cats []string
	for rows.Next() {
		var c string
		if err := rows.Scan(&c); err != nil {
			return nil, err
		}
		cats = append(cats, c)
	}
	return cats, rows.Err()
}

func (d *DB) migrateStarHistory() error {
	q := `CREATE TABLE IF NOT EXISTS github_repo_stars_history (
		id         INTEGER PRIMARY KEY AUTOINCREMENT,
		repo_id    INTEGER NOT NULL REFERENCES github_repos(id) ON DELETE CASCADE,
		stars      INTEGER NOT NULL,
		sampled_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
		UNIQUE(repo_id, sampled_at)
	)`
	if _, err := d.conn.Exec(q); err != nil {
		return fmt.Errorf("migration github_repo_stars_history failed: %w", err)
	}

	if _, err := d.conn.Exec(`CREATE INDEX IF NOT EXISTS idx_stars_history_repo_id ON github_repo_stars_history(repo_id)`); err != nil {
		return fmt.Errorf("migration idx_stars_history_repo_id failed: %w", err)
	}
	return nil
}

func (d *DB) InsertStarSnapshot(repoID int64, stars int, sampledAt time.Time) error {
	_, err := d.conn.Exec(
		`INSERT INTO github_repo_stars_history(repo_id, stars, sampled_at)
		 VALUES (?, ?, ?)
		 ON CONFLICT(repo_id, sampled_at) DO UPDATE SET stars=excluded.stars`,
		repoID, stars, sampledAt.UTC().Format(time.RFC3339),
	)
	if err != nil {
		return fmt.Errorf("db: insert star snapshot for repo %d: %w", repoID, err)
	}
	return nil
}

func (d *DB) GetStarHistory(repoID int64, limit int) ([]StarSnapshot, error) {
	query := `SELECT id, repo_id, stars, sampled_at FROM github_repo_stars_history WHERE repo_id = ? ORDER BY sampled_at DESC`
	var rows *sql.Rows
	var err error
	if limit > 0 {
		query += " LIMIT ?"
		rows, err = d.conn.Query(query, repoID, limit)
	} else {
		rows, err = d.conn.Query(query, repoID)
	}
	if err != nil {
		return nil, fmt.Errorf("db: query star history for repo %d: %w", repoID, err)
	}
	defer rows.Close()

	var history []StarSnapshot
	for rows.Next() {
		var s StarSnapshot
		var ts string
		if err := rows.Scan(&s.ID, &s.RepoID, &s.Stars, &ts); err != nil {
			return nil, fmt.Errorf("db: scan star history: %w", err)
		}
		s.SampledAt, _ = time.Parse(time.RFC3339, ts)
		history = append(history, s)
	}
	return history, rows.Err()
}

func (d *DB) GetAllRepoIDsOrdered() ([]int64, error) {
	rows, err := d.conn.Query(`SELECT id FROM github_repos ORDER BY id ASC`)
	if err != nil {
		return nil, fmt.Errorf("db: get all repo ids: %w", err)
	}
	defer rows.Close()

	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func (d *DB) GetRefreshCursor() (lastID int64, lastAt time.Time, err error) {
	idStr, err := d.GetSetting("refresh_last_repo_id")
	if err != nil {
		return 0, time.Time{}, err
	}
	if idStr != "" {
		fmt.Sscanf(idStr, "%d", &lastID)
	}

	atStr, err := d.GetSetting("refresh_last_at")
	if err != nil {
		return lastID, time.Time{}, err
	}
	if atStr != "" {
		lastAt, _ = time.Parse(time.RFC3339, atStr)
	}
	return lastID, lastAt, nil
}

func (d *DB) SetRefreshCursor(lastID int64, at time.Time) error {
	if err := d.SetSetting("refresh_last_repo_id", fmt.Sprintf("%d", lastID)); err != nil {
		return err
	}
	if err := d.SetSetting("refresh_last_at", at.UTC().Format(time.RFC3339)); err != nil {
		return err
	}
	return nil
}
