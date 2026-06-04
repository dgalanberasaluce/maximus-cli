package db

import (
	"database/sql"
	"fmt"
	"time"
)

// VSCodeSummaryRow representa el resumen de instalación almacenado.
type VSCodeSummaryRow struct {
	Version   string
	Installed bool
	PathsJSON string
	ScannedAt time.Time
}

// VSCodeProfileRow representa un perfil de VSCode.
type VSCodeProfileRow struct {
	LocationID  string
	Name        string
	Icon        string
	IsDefault   bool
	ProfilePath string
	DirMtime    time.Time
	ScannedAt   time.Time
}

// VSCodeExtRow representa un registro de extensión en la BD.
type VSCodeExtRow struct {
	ProfileID       string
	ExtID           string
	Version         string
	InstalledAt     time.Time
	InstallPath     string
	Description     string
	LongDescription string
}

// VSCodeProjectRow representa un proyecto asociado a un perfil.
type VSCodeProjectRow struct {
	ProfileID    string
	ProjectPath  string
	ExistsOnDisk bool
}

// VSCodeRefreshLogRow representa una entrada en el historial de refrescos con cambios.
type VSCodeRefreshLogRow struct {
	ID          int64
	RefreshedAt time.Time
	HasChanges  bool
	DiffJSON    string
}

// migrateVSCode crea las tablas necesarias para VSCode si no existen.
func (d *DB) migrateVSCode() error {
	queries := []string{
		`CREATE TABLE IF NOT EXISTS vscode_summary (
			id          INTEGER PRIMARY KEY CHECK (id = 1),
			version     TEXT NOT NULL DEFAULT '',
			installed   BOOLEAN NOT NULL DEFAULT 0,
			paths_json  TEXT NOT NULL DEFAULT '[]',
			scanned_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS vscode_profiles (
			location_id  TEXT PRIMARY KEY,
			name         TEXT NOT NULL,
			icon         TEXT NOT NULL DEFAULT '',
			is_default   BOOLEAN NOT NULL DEFAULT 0,
			profile_path TEXT NOT NULL DEFAULT '',
			dir_mtime    DATETIME,
			scanned_at   DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS vscode_profile_extensions (
			profile_id       TEXT NOT NULL,
			ext_id           TEXT NOT NULL,
			version          TEXT NOT NULL DEFAULT '',
			installed_at     DATETIME,
			install_path     TEXT NOT NULL DEFAULT '',
			description      TEXT NOT NULL DEFAULT '',
			long_description TEXT NOT NULL DEFAULT '',
			PRIMARY KEY (profile_id, ext_id),
			FOREIGN KEY (profile_id) REFERENCES vscode_profiles(location_id) ON DELETE CASCADE
		)`,
		`CREATE TABLE IF NOT EXISTS vscode_profile_projects (
			profile_id     TEXT NOT NULL,
			project_path   TEXT NOT NULL,
			exists_on_disk BOOLEAN NOT NULL DEFAULT 1,
			PRIMARY KEY (profile_id, project_path),
			FOREIGN KEY (profile_id) REFERENCES vscode_profiles(location_id) ON DELETE CASCADE
		)`,
		`CREATE TABLE IF NOT EXISTS vscode_refresh_log (
			id           INTEGER PRIMARY KEY AUTOINCREMENT,
			refreshed_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			has_changes  BOOLEAN NOT NULL DEFAULT 0,
			diff_json    TEXT NOT NULL DEFAULT '{}'
		)`,
	}

	for _, q := range queries {
		if _, err := d.conn.Exec(q); err != nil {
			return fmt.Errorf("vscode migration failed: %w", err)
		}
	}

	// Safe alter table migrations for existing databases
	alterQueries := []string{
		`ALTER TABLE vscode_profile_extensions ADD COLUMN install_path TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE vscode_profile_extensions ADD COLUMN description TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE vscode_profile_extensions ADD COLUMN long_description TEXT NOT NULL DEFAULT ''`,
	}
	for _, aq := range alterQueries {
		_, _ = d.conn.Exec(aq) // Ignore error if columns already exist
	}

	return nil
}

// UpsertVSCodeSummary actualiza el resumen global de instalación de VSCode.
func (d *DB) UpsertVSCodeSummary(version string, installed bool, pathsJSON string) error {
	_, err := d.conn.Exec(
		`INSERT INTO vscode_summary(id, version, installed, paths_json, scanned_at)
		 VALUES (1, ?, ?, ?, ?)
		 ON CONFLICT(id) DO UPDATE SET
		 	version=excluded.version,
		 	installed=excluded.installed,
		 	paths_json=excluded.paths_json,
		 	scanned_at=excluded.scanned_at`,
		version, installed, pathsJSON, time.Now().UTC().Format(time.RFC3339),
	)
	if err != nil {
		return fmt.Errorf("db: upsert vscode summary: %w", err)
	}
	return nil
}

// GetVSCodeSummary obtiene el resumen actual de la base de datos.
// Retorna (row, exists, error).
func (d *DB) GetVSCodeSummary() (VSCodeSummaryRow, bool, error) {
	var row VSCodeSummaryRow
	var scannedStr string
	err := d.conn.QueryRow(`SELECT version, installed, paths_json, scanned_at FROM vscode_summary WHERE id = 1`).Scan(
		&row.Version, &row.Installed, &row.PathsJSON, &scannedStr,
	)
	if err == sql.ErrNoRows {
		return row, false, nil
	}
	if err != nil {
		return row, false, fmt.Errorf("db: get vscode summary: %w", err)
	}

	row.ScannedAt, _ = time.Parse(time.RFC3339, scannedStr)
	return row, true, nil
}

// ClearVSCodeProfiles limpia perfiles, extensiones y proyectos antes de refrescar.
func (d *DB) ClearVSCodeProfiles() error {
	tx, err := d.conn.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.Exec(`DELETE FROM vscode_profile_extensions`); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM vscode_profile_projects`); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM vscode_profiles`); err != nil {
		return err
	}

	return tx.Commit()
}

// UpsertVSCodeProfile inserta o actualiza un perfil.
func (d *DB) UpsertVSCodeProfile(p VSCodeProfileRow) error {
	var mtimeStr *string
	if !p.DirMtime.IsZero() {
		s := p.DirMtime.UTC().Format(time.RFC3339)
		mtimeStr = &s
	}

	_, err := d.conn.Exec(
		`INSERT INTO vscode_profiles(location_id, name, icon, is_default, profile_path, dir_mtime, scanned_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(location_id) DO UPDATE SET
		 	name=excluded.name,
		 	icon=excluded.icon,
		 	is_default=excluded.is_default,
		 	profile_path=excluded.profile_path,
		 	dir_mtime=excluded.dir_mtime,
		 	scanned_at=excluded.scanned_at`,
		p.LocationID, p.Name, p.Icon, p.IsDefault, p.ProfilePath, mtimeStr, time.Now().UTC().Format(time.RFC3339),
	)
	if err != nil {
		return fmt.Errorf("db: upsert vscode profile %s: %w", p.LocationID, err)
	}
	return nil
}

// GetVSCodeProfiles obtiene todos los perfiles de VSCode.
func (d *DB) GetVSCodeProfiles() ([]VSCodeProfileRow, error) {
	rows, err := d.conn.Query(`SELECT location_id, name, icon, is_default, profile_path, dir_mtime, scanned_at FROM vscode_profiles ORDER BY is_default DESC, name ASC`)
	if err != nil {
		return nil, fmt.Errorf("db: query vscode profiles: %w", err)
	}
	defer rows.Close()

	var list []VSCodeProfileRow
	for rows.Next() {
		var r VSCodeProfileRow
		var mtimeStr sql.NullString
		var scannedStr string
		err := rows.Scan(&r.LocationID, &r.Name, &r.Icon, &r.IsDefault, &r.ProfilePath, &mtimeStr, &scannedStr)
		if err != nil {
			return nil, fmt.Errorf("db: scan vscode profile: %w", err)
		}
		if mtimeStr.Valid {
			r.DirMtime, _ = time.Parse(time.RFC3339, mtimeStr.String)
		}
		r.ScannedAt, _ = time.Parse(time.RFC3339, scannedStr)
		list = append(list, r)
	}
	return list, nil
}

// UpsertVSCodeExtensions inserta todas las extensiones de un perfil en batch.
func (d *DB) UpsertVSCodeExtensions(profileID string, exts []VSCodeExtRow) error {
	tx, err := d.conn.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare(`
		INSERT INTO vscode_profile_extensions(profile_id, ext_id, version, installed_at, install_path, description, long_description)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(profile_id, ext_id) DO UPDATE SET
			version=excluded.version,
			installed_at=excluded.installed_at,
			install_path=excluded.install_path,
			description=excluded.description,
			long_description=excluded.long_description
	`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, ext := range exts {
		var instStr *string
		if !ext.InstalledAt.IsZero() {
			s := ext.InstalledAt.UTC().Format(time.RFC3339)
			instStr = &s
		}
		if _, err := stmt.Exec(profileID, ext.ExtID, ext.Version, instStr, ext.InstallPath, ext.Description, ext.LongDescription); err != nil {
			return err
		}
	}

	return tx.Commit()
}

// GetVSCodeExtensions obtiene las extensiones instaladas para un perfil.
func (d *DB) GetVSCodeExtensions(profileID string) ([]VSCodeExtRow, error) {
	rows, err := d.conn.Query(`SELECT ext_id, version, installed_at, install_path, description, long_description FROM vscode_profile_extensions WHERE profile_id = ? ORDER BY ext_id ASC`, profileID)
	if err != nil {
		return nil, fmt.Errorf("db: query vscode extensions: %w", err)
	}
	defer rows.Close()

	var list []VSCodeExtRow
	for rows.Next() {
		var r VSCodeExtRow
		r.ProfileID = profileID
		var instStr sql.NullString
		if err := rows.Scan(&r.ExtID, &r.Version, &instStr, &r.InstallPath, &r.Description, &r.LongDescription); err != nil {
			return nil, fmt.Errorf("db: scan vscode extension: %w", err)
		}
		if instStr.Valid {
			r.InstalledAt, _ = time.Parse(time.RFC3339, instStr.String)
		}
		list = append(list, r)
	}
	return list, nil
}

// UpsertVSCodeProjects inserta todos los proyectos asociados a un perfil.
func (d *DB) UpsertVSCodeProjects(profileID string, projects []VSCodeProjectRow) error {
	tx, err := d.conn.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare(`
		INSERT INTO vscode_profile_projects(profile_id, project_path, exists_on_disk)
		VALUES (?, ?, ?)
		ON CONFLICT(profile_id, project_path) DO UPDATE SET
			exists_on_disk=excluded.exists_on_disk
	`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, p := range projects {
		if _, err := stmt.Exec(profileID, p.ProjectPath, p.ExistsOnDisk); err != nil {
			return err
		}
	}

	return tx.Commit()
}

// GetVSCodeProjects obtiene los proyectos de un perfil.
// Si includeArchived es falso, sólo devuelve aquellos proyectos que existen en disco (exists_on_disk = 1).
func (d *DB) GetVSCodeProjects(profileID string, includeArchived bool) ([]VSCodeProjectRow, error) {
	var rows *sql.Rows
	var err error

	if includeArchived {
		rows, err = d.conn.Query(`SELECT project_path, exists_on_disk FROM vscode_profile_projects WHERE profile_id = ? ORDER BY exists_on_disk DESC, project_path ASC`, profileID)
	} else {
		rows, err = d.conn.Query(`SELECT project_path, exists_on_disk FROM vscode_profile_projects WHERE profile_id = ? AND exists_on_disk = 1 ORDER BY project_path ASC`, profileID)
	}

	if err != nil {
		return nil, fmt.Errorf("db: query vscode projects: %w", err)
	}
	defer rows.Close()

	var list []VSCodeProjectRow
	for rows.Next() {
		var r VSCodeProjectRow
		r.ProfileID = profileID
		if err := rows.Scan(&r.ProjectPath, &r.ExistsOnDisk); err != nil {
			return nil, fmt.Errorf("db: scan vscode project: %w", err)
		}
		list = append(list, r)
	}
	return list, nil
}

// InsertVSCodeRefreshLog inserta una nueva entrada de refresco.
func (d *DB) InsertVSCodeRefreshLog(hasChanges bool, diffJSON string) error {
	_, err := d.conn.Exec(
		`INSERT INTO vscode_refresh_log(refreshed_at, has_changes, diff_json)
		 VALUES (?, ?, ?)`,
		time.Now().UTC().Format(time.RFC3339), hasChanges, diffJSON,
	)
	if err != nil {
		return fmt.Errorf("db: insert vscode refresh log: %w", err)
	}
	return nil
}

// GetVSCodeRefreshLogs obtiene las últimas entradas de refresco.
// Si onlyWithChanges es verdadero, filtra por has_changes = 1.
func (d *DB) GetVSCodeRefreshLogs(onlyWithChanges bool, limit, offset int) ([]VSCodeRefreshLogRow, error) {
	var rows *sql.Rows
	var err error

	if onlyWithChanges {
		rows, err = d.conn.Query(
			`SELECT id, refreshed_at, has_changes, diff_json
			 FROM vscode_refresh_log
			 WHERE has_changes = 1
			 ORDER BY refreshed_at DESC
			 LIMIT ? OFFSET ?`,
			limit, offset,
		)
	} else {
		rows, err = d.conn.Query(
			`SELECT id, refreshed_at, has_changes, diff_json
			 FROM vscode_refresh_log
			 ORDER BY refreshed_at DESC
			 LIMIT ? OFFSET ?`,
			limit, offset,
		)
	}

	if err != nil {
		return nil, fmt.Errorf("db: get vscode refresh logs: %w", err)
	}
	defer rows.Close()

	var list []VSCodeRefreshLogRow
	for rows.Next() {
		var r VSCodeRefreshLogRow
		var refStr string
		if err := rows.Scan(&r.ID, &refStr, &r.HasChanges, &r.DiffJSON); err != nil {
			return nil, fmt.Errorf("db: scan vscode refresh log: %w", err)
		}
		r.RefreshedAt, _ = time.Parse(time.RFC3339, refStr)
		list = append(list, r)
	}
	return list, nil
}

// GetLastVSCodeRefreshAt obtiene la fecha del último refresco exitoso (sin importar si tuvo cambios o no).
func (d *DB) GetLastVSCodeRefreshAt() (time.Time, error) {
	var ts string
	err := d.conn.QueryRow(`SELECT refreshed_at FROM vscode_refresh_log ORDER BY refreshed_at DESC LIMIT 1`).Scan(&ts)
	if err == sql.ErrNoRows {
		return time.Time{}, nil
	}
	if err != nil {
		return time.Time{}, fmt.Errorf("db: get last vscode refresh at: %w", err)
	}
	t, _ := time.Parse(time.RFC3339, ts)
	return t, nil
}

type VSCodeExtAggRow struct {
	ExtID           string
	ProfileName     string
	Version         string
	InstalledAt     time.Time
	InstallPath     string
	Description     string
	LongDescription string
}

// GetVSCodeDependenciesAgg obtiene la lista de todas las extensiones agregadas por ID con sus perfiles y versiones.
func (d *DB) GetVSCodeDependenciesAgg() ([]VSCodeExtAggRow, error) {
	rows, err := d.conn.Query(`
		SELECT pe.ext_id, p.name, pe.version, pe.installed_at, pe.install_path, pe.description, pe.long_description
		FROM vscode_profile_extensions pe
		JOIN vscode_profiles p ON pe.profile_id = p.location_id
		ORDER BY pe.ext_id ASC, p.name ASC
	`)
	if err != nil {
		return nil, fmt.Errorf("db: query aggregated vscode dependencies: %w", err)
	}
	defer rows.Close()

	var list []VSCodeExtAggRow
	for rows.Next() {
		var r VSCodeExtAggRow
		var instStr sql.NullString
		if err := rows.Scan(&r.ExtID, &r.ProfileName, &r.Version, &instStr, &r.InstallPath, &r.Description, &r.LongDescription); err != nil {
			return nil, fmt.Errorf("db: scan aggregated vscode dependency: %w", err)
		}
		if instStr.Valid && instStr.String != "" {
			r.InstalledAt, _ = time.Parse(time.RFC3339, instStr.String)
		}
		list = append(list, r)
	}
	return list, nil
}
