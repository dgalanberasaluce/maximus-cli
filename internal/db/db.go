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

// Stats holds database statistics for the self-doctor.
type Stats struct {
	Path         string
	SettingCount int
	HistoryCount int
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
	return s, nil
}
