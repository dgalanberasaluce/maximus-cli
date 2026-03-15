// Package db manages the SQLite3 database for maximux-cli history and settings.
package db

import (
	"database/sql"
	"fmt"
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
	}
	for _, q := range queries {
		if _, err := d.conn.Exec(q); err != nil {
			return fmt.Errorf("migration failed: %w", err)
		}
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

// Stats holds database statistics for the self-doctor.
type Stats struct {
	Path         string
	SettingCount int
	HistoryCount int
	UpgradeCount int
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
	return s, nil
}
