// Package db manages the SQLite3 database for maximus-cli history and settings.
package db

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

// DB wraps the sql.DB connection with application-level helpers.
type DB struct {
	conn *sql.DB
	path string
}

// Open opens (or creates) the SQLite3 database at the given path.
// It runs all required migrations before returning.
func Open(path string) (*DB, error) {
	conn, err := sql.Open("sqlite3", path)
	if err != nil {
		return nil, fmt.Errorf("db: open %s: %w", path, err)
	}

	conn.SetMaxOpenConns(1) // SQLite doesn't support concurrent writes.

	d := &DB{conn: conn, path: path}
	if err := d.migrate(); err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("db: migrate: %w", err)
	}
	return d, nil
}

// Close closes the database connection.
func (d *DB) Close() error {
	return d.conn.Close()
}

// migrate creates the required tables if they don't exist.
func (d *DB) migrate() error {
	queries := []string{
		`CREATE TABLE IF NOT EXISTS settings (
			key   TEXT PRIMARY KEY,
			value TEXT NOT NULL,
			updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS history (
			id         INTEGER PRIMARY KEY AUTOINCREMENT,
			command    TEXT NOT NULL,
			output     TEXT,
			executed_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS upgrade_log (
			id           INTEGER PRIMARY KEY AUTOINCREMENT,
			package_name TEXT NOT NULL,
			old_version  TEXT NOT NULL,
			new_version  TEXT NOT NULL,
			upgraded_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE INDEX IF NOT EXISTS idx_upgrade_log_package ON upgrade_log(package_name)`,
		`CREATE TABLE IF NOT EXISTS package_addition_log (
			id           INTEGER PRIMARY KEY AUTOINCREMENT,
			package_name TEXT NOT NULL,
			kind         TEXT NOT NULL,
			version      TEXT NOT NULL DEFAULT '',
			added_at     DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE INDEX IF NOT EXISTS idx_addition_log_package ON package_addition_log(package_name)`,
		`CREATE TABLE IF NOT EXISTS dotfiles (
			id          INTEGER PRIMARY KEY AUTOINCREMENT,
			name        TEXT NOT NULL UNIQUE,
			is_dir      BOOLEAN NOT NULL DEFAULT 0,
			tool        TEXT NOT NULL DEFAULT '',
			tool_manual BOOLEAN NOT NULL DEFAULT 0,
			modified_at DATETIME NOT NULL,
			created_at  DATETIME NOT NULL,
			scanned_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE INDEX IF NOT EXISTS idx_dotfiles_name ON dotfiles(name)`,
	}
	for _, q := range queries {
		if _, err := d.conn.Exec(q); err != nil {
			return fmt.Errorf("migration failed: %w", err)
		}
	}

	// Dynamic migration for existing databases:
	_, _ = d.conn.Exec(`ALTER TABLE dotfiles ADD COLUMN tool_manual BOOLEAN NOT NULL DEFAULT 0`)

	if err := d.migrateVSCode(); err != nil {
		return err
	}

	return nil
}

// SetSetting upserts a key-value setting.
func (d *DB) SetSetting(key, value string) error {
	_, err := d.conn.Exec(
		`INSERT INTO settings(key, value, updated_at) VALUES (?, ?, ?)
		 ON CONFLICT(key) DO UPDATE SET value=excluded.value, updated_at=excluded.updated_at`,
		key, value, time.Now().UTC().Format(time.RFC3339),
	)
	if err != nil {
		return fmt.Errorf("db: set setting %q: %w", key, err)
	}
	return nil
}

// GetSetting retrieves a setting by key. Returns ("", nil) if not found.
func (d *DB) GetSetting(key string) (string, error) {
	var value string
	err := d.conn.QueryRow(`SELECT value FROM settings WHERE key = ?`, key).Scan(&value)
	if err == sql.ErrNoRows {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("db: get setting %q: %w", key, err)
	}
	return value, nil
}

// AllSettings returns all stored settings as a map.
func (d *DB) AllSettings() (map[string]string, error) {
	rows, err := d.conn.Query(`SELECT key, value FROM settings ORDER BY key`)
	if err != nil {
		return nil, fmt.Errorf("db: list settings: %w", err)
	}
	defer rows.Close()

	settings := make(map[string]string)
	for rows.Next() {
		var k, v string
		if err := rows.Scan(&k, &v); err != nil {
			return nil, fmt.Errorf("db: scan setting: %w", err)
		}
		settings[k] = v
	}
	return settings, rows.Err()
}

// LogHistory records a command execution to the history table.
func (d *DB) LogHistory(command, output string) error {
	_, err := d.conn.Exec(
		`INSERT INTO history(command, output, executed_at) VALUES (?, ?, ?)`,
		command, output, time.Now().UTC().Format(time.RFC3339),
	)
	if err != nil {
		return fmt.Errorf("db: log history: %w", err)
	}
	return nil
}

// UpgradeLog is a single entry in the upgrade log.
type UpgradeLog struct {
	ID          int64
	PackageName string
	OldVersion  string
	NewVersion  string
	UpgradedAt  time.Time
}

// LogUpgrade records a single package upgrade event.
func (d *DB) LogUpgrade(packageName, oldVersion, newVersion string) error {
	_, err := d.conn.Exec(
		`INSERT INTO upgrade_log(package_name, old_version, new_version, upgraded_at) VALUES (?, ?, ?, ?)`,
		packageName, oldVersion, newVersion, time.Now().UTC().Format(time.RFC3339),
	)
	if err != nil {
		return fmt.Errorf("db: log upgrade %q: %w", packageName, err)
	}
	return nil
}

// GetUpgradeLogs returns upgrade log entries with optional package filter,
// newest first. Pass filter="" to return all packages.
// Max limit is capped at 100. Use offset for pagination.
func (d *DB) GetUpgradeLogs(filter string, limit, offset int) ([]UpgradeLog, error) {
	if limit > 100 {
		limit = 100
	}

	var rows *sql.Rows
	var err error
	if filter != "" {
		rows, err = d.conn.Query(
			`SELECT id, package_name, old_version, new_version, upgraded_at
			   FROM upgrade_log
			  WHERE package_name LIKE ?
			  ORDER BY upgraded_at DESC
			  LIMIT ? OFFSET ?`,
			"%"+filter+"%", limit, offset,
		)
	} else {
		rows, err = d.conn.Query(
			`SELECT id, package_name, old_version, new_version, upgraded_at
			   FROM upgrade_log
			  ORDER BY upgraded_at DESC
			  LIMIT ? OFFSET ?`,
			limit, offset,
		)
	}
	if err != nil {
		return nil, fmt.Errorf("db: get upgrade logs: %w", err)
	}
	defer rows.Close()

	var entries []UpgradeLog
	for rows.Next() {
		var e UpgradeLog
		var ts string
		if err := rows.Scan(&e.ID, &e.PackageName, &e.OldVersion, &e.NewVersion, &ts); err != nil {
			return nil, fmt.Errorf("db: scan upgrade log: %w", err)
		}
		e.UpgradedAt, _ = time.Parse(time.RFC3339, ts)
		entries = append(entries, e)
	}
	return entries, rows.Err()
}

// CountUpgradeLogs returns the total number of log entries matching the filter.
func (d *DB) CountUpgradeLogs(filter string) (int, error) {
	var count int
	var err error
	if filter != "" {
		err = d.conn.QueryRow(
			`SELECT COUNT(*) FROM upgrade_log WHERE package_name LIKE ?`,
			"%"+filter+"%",
		).Scan(&count)
	} else {
		err = d.conn.QueryRow(`SELECT COUNT(*) FROM upgrade_log`).Scan(&count)
	}
	if err != nil {
		return 0, fmt.Errorf("db: count upgrade logs: %w", err)
	}
	return count, nil
}

// AdditionLog is a single entry in the package addition log.
type AdditionLog struct {
	ID          int64
	PackageName string
	Kind        string
	Version     string
	AddedAt     time.Time
}

// LogAddition records that a package was added to the Brewfile from the
// unstaged list. version may be empty if the installed version is unknown.
func (d *DB) LogAddition(packageName, kind, version string) error {
	_, err := d.conn.Exec(
		`INSERT INTO package_addition_log(package_name, kind, version, added_at) VALUES (?, ?, ?, ?)`,
		packageName, kind, version, time.Now().UTC().Format(time.RFC3339),
	)
	if err != nil {
		return fmt.Errorf("db: log addition %q: %w", packageName, err)
	}
	return nil
}

// GetAdditionLogs returns addition log entries with optional package filter,
// newest first. Pass filter="" to return all. Use limit/offset for pagination.
func (d *DB) GetAdditionLogs(filter string, limit, offset int) ([]AdditionLog, error) {
	if limit > 100 {
		limit = 100
	}
	var base string
	var args []any
	if filter != "" {
		base = `SELECT id, package_name, kind, version, added_at
			   FROM package_addition_log
			  WHERE package_name LIKE ?
			  ORDER BY added_at DESC LIMIT ? OFFSET ?`
		args = []any{"%" + filter + "%", limit, offset}
	} else {
		base = `SELECT id, package_name, kind, version, added_at
			   FROM package_addition_log
			  ORDER BY added_at DESC LIMIT ? OFFSET ?`
		args = []any{limit, offset}
	}
	rows, err := d.conn.Query(base, args...)
	if err != nil {
		return nil, fmt.Errorf("db: get addition logs: %w", err)
	}
	defer rows.Close()

	var entries []AdditionLog
	for rows.Next() {
		var e AdditionLog
		var ts string
		if err := rows.Scan(&e.ID, &e.PackageName, &e.Kind, &e.Version, &ts); err != nil {
			return nil, fmt.Errorf("db: scan addition log: %w", err)
		}
		e.AddedAt, _ = time.Parse(time.RFC3339, ts)
		entries = append(entries, e)
	}
	return entries, rows.Err()
}

// CountAdditionLogs returns the total number of addition log entries matching filter.
func (d *DB) CountAdditionLogs(filter string) (int, error) {
	var count int
	var err error
	if filter != "" {
		err = d.conn.QueryRow(
			`SELECT COUNT(*) FROM package_addition_log WHERE package_name LIKE ?`,
			"%"+filter+"%",
		).Scan(&count)
	} else {
		err = d.conn.QueryRow(`SELECT COUNT(*) FROM package_addition_log`).Scan(&count)
	}
	if err != nil {
		return 0, fmt.Errorf("db: count addition logs: %w", err)
	}
	return count, nil
}

// GetInstallDate returns the date on which the given version of a package was
// first recorded in the database. It queries upgrade_log first (new_version
// column), then package_addition_log (version column). Returns a zero time
// and nil error when no record is found.
func (d *DB) GetInstallDate(packageName, version string) (time.Time, error) {
	var ts string
	err := d.conn.QueryRow(
		`SELECT upgraded_at FROM upgrade_log
		  WHERE package_name = ? AND new_version = ?
		  ORDER BY upgraded_at ASC
		  LIMIT 1`,
		packageName, version,
	).Scan(&ts)
	if err != nil && err != sql.ErrNoRows {
		return time.Time{}, fmt.Errorf("db: get install date from upgrade_log %q: %w", packageName, err)
	}
	if err == nil && ts != "" {
		t, _ := time.Parse(time.RFC3339, ts)
		return t, nil
	}

	// Fall back to package_addition_log.
	err = d.conn.QueryRow(
		`SELECT added_at FROM package_addition_log
		  WHERE package_name = ? AND version = ?
		  ORDER BY added_at ASC
		  LIMIT 1`,
		packageName, version,
	).Scan(&ts)
	if err != nil && err != sql.ErrNoRows {
		return time.Time{}, fmt.Errorf("db: get install date from addition_log %q: %w", packageName, err)
	}
	if err == nil && ts != "" {
		t, _ := time.Parse(time.RFC3339, ts)
		return t, nil
	}

	return time.Time{}, nil
}

// Stats holds database statistics for the self-doctor.
type Stats struct {
	Path          string
	SettingCount  int
	HistoryCount  int
	UpgradeCount  int
	AdditionCount int
	DotfileCount  int
}

// Doctor returns diagnostic information about the database.
func (d *DB) Doctor() (*Stats, error) {
	s := &Stats{Path: d.path}

	if err := d.conn.QueryRow(`SELECT COUNT(*) FROM settings`).Scan(&s.SettingCount); err != nil {
		return nil, fmt.Errorf("db: doctor settings count: %w", err)
	}
	if err := d.conn.QueryRow(`SELECT COUNT(*) FROM history`).Scan(&s.HistoryCount); err != nil {
		return nil, fmt.Errorf("db: doctor history count: %w", err)
	}
	if err := d.conn.QueryRow(`SELECT COUNT(*) FROM upgrade_log`).Scan(&s.UpgradeCount); err != nil {
		return nil, fmt.Errorf("db: doctor upgrade count: %w", err)
	}
	if err := d.conn.QueryRow(`SELECT COUNT(*) FROM package_addition_log`).Scan(&s.AdditionCount); err != nil {
		return nil, fmt.Errorf("db: doctor addition count: %w", err)
	}
	if err := d.conn.QueryRow(`SELECT COUNT(*) FROM dotfiles`).Scan(&s.DotfileCount); err != nil {
		return nil, fmt.Errorf("db: doctor dotfiles count: %w", err)
	}
	return s, nil
}

// DotfileEntry represents a scanned dotfile or dotfolder in $HOME.
type DotfileEntry struct {
	ID         int64
	Name       string
	IsDir      bool
	Tool       string
	ToolManual bool
	ModifiedAt time.Time
	CreatedAt  time.Time
	ScannedAt  time.Time
}

// UpsertDotfile inserts or updates a dotfile entry.
func (d *DB) UpsertDotfile(name string, isDir bool, tool string, modifiedAt, createdAt time.Time) error {
	_, err := d.conn.Exec(
		`INSERT INTO dotfiles(name, is_dir, tool, tool_manual, modified_at, created_at, scanned_at)
		 VALUES (?, ?, ?, 0, ?, ?, ?)
		 ON CONFLICT(name) DO UPDATE SET
		 	is_dir=excluded.is_dir,
		 	tool=CASE WHEN dotfiles.tool_manual=1 THEN dotfiles.tool ELSE excluded.tool END,
		 	modified_at=excluded.modified_at,
		 	created_at=excluded.created_at,
		 	scanned_at=excluded.scanned_at`,
		name, isDir, tool,
		modifiedAt.UTC().Format(time.RFC3339),
		createdAt.UTC().Format(time.RFC3339),
		time.Now().UTC().Format(time.RFC3339),
	)
	if err != nil {
		return fmt.Errorf("db: upsert dotfile %q: %w", name, err)
	}
	return nil
}

// UpdateDotfileTool updates only the tool field and marks tool_manual as true for a named dotfile entry.
// This allows users to customise the inferred tool description in-place.
func (d *DB) UpdateDotfileTool(name string, tool string) error {
	_, err := d.conn.Exec(
		`UPDATE dotfiles SET tool=?, tool_manual=1 WHERE name=?`,
		tool, name,
	)
	if err != nil {
		return fmt.Errorf("db: update tool for %q: %w", name, err)
	}
	return nil
}

// ClearDotfiles deletes all entries in the dotfiles table.
func (d *DB) ClearDotfiles() error {
	_, err := d.conn.Exec(`DELETE FROM dotfiles`)
	if err != nil {
		return fmt.Errorf("db: clear dotfiles: %w", err)
	}
	return nil
}

// DeleteDotfile deletes a single dotfile entry by its name.
func (d *DB) DeleteDotfile(name string) error {
	_, err := d.conn.Exec(`DELETE FROM dotfiles WHERE name=?`, name)
	if err != nil {
		return fmt.Errorf("db: delete dotfile %q: %w", name, err)
	}
	return nil
}

// GetDotfiles retrieves dotfile entries with filtering, sorting, and pagination.
// limit <= 0 means no limit (represented as -1).
func (d *DB) GetDotfiles(filter string, sortBy string, asc bool, limit, offset int) ([]DotfileEntry, error) {
	var query strings.Builder
	query.WriteString(`SELECT id, name, is_dir, tool, tool_manual, modified_at, created_at, scanned_at FROM dotfiles`)

	var args []any
	if filter != "" {
		query.WriteString(` WHERE name LIKE ?`)
		args = append(args, "%"+filter+"%")
	}

	// Validate and build sorting
	validSortColumns := map[string]string{
		"name":        "name",
		"is_dir":      "is_dir",
		"tool":        "tool",
		"modified_at": "modified_at",
		"created_at":  "created_at",
	}
	sortCol, ok := validSortColumns[strings.ToLower(sortBy)]
	if !ok {
		sortCol = "name"
	}

	direction := "ASC"
	if !asc {
		direction = "DESC"
	}
	query.WriteString(fmt.Sprintf(` ORDER BY %s %s`, sortCol, direction))

	if limit > 0 {
		query.WriteString(` LIMIT ? OFFSET ?`)
		args = append(args, limit, offset)
	}

	rows, err := d.conn.Query(query.String(), args...)
	if err != nil {
		return nil, fmt.Errorf("db: query dotfiles: %w", err)
	}
	defer rows.Close()

	var entries []DotfileEntry
	for rows.Next() {
		var e DotfileEntry
		var modStr, creStr, scaStr string
		if err := rows.Scan(&e.ID, &e.Name, &e.IsDir, &e.Tool, &e.ToolManual, &modStr, &creStr, &scaStr); err != nil {
			return nil, fmt.Errorf("db: scan dotfile: %w", err)
		}
		e.ModifiedAt, _ = time.Parse(time.RFC3339, modStr)
		e.CreatedAt, _ = time.Parse(time.RFC3339, creStr)
		e.ScannedAt, _ = time.Parse(time.RFC3339, scaStr)
		entries = append(entries, e)
	}
	return entries, rows.Err()
}

// CountDotfiles returns total count of dotfiles matching the filter.
func (d *DB) CountDotfiles(filter string) (int, error) {
	var count int
	var err error
	if filter != "" {
		err = d.conn.QueryRow(
			`SELECT COUNT(*) FROM dotfiles WHERE name LIKE ?`,
			"%"+filter+"%",
		).Scan(&count)
	} else {
		err = d.conn.QueryRow(`SELECT COUNT(*) FROM dotfiles`).Scan(&count)
	}
	if err != nil {
		return 0, fmt.Errorf("db: count dotfiles: %w", err)
	}
	return count, nil
}
