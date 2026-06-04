# SQLite Database Cheatsheet — Maximus CLI

**Database:** `~/.config/maximus-cli/maximus.db`

---

## Quick Reference

```bash
# Open interactive shell
sqlite3 ~/.config/maximus-cli/maximus.db

# Run a query and exit
sqlite3 ~/.config/maximus-cli/maximus.db "SELECT * FROM settings;"

# Export table(s) to CSV
sqlite3 -header -csv ~/.config/maximus-cli/maximus.db "SELECT * FROM dotfiles;" > dotfiles.csv

# Dump entire database
sqlite3 ~/.config/maximus-cli/maximus.db .dump

# Show schema for all tables
sqlite3 ~/.config/maximus-cli/maximus.db .schema

# Show schema for a specific table
sqlite3 ~/.config/maximus-cli/maximus.db ".schema dotfiles"

# Get database info
sqlite3 ~/.config/maximus-cli/maximus.db ".dbinfo"
```

---

## Useful Meta Commands (inside sqlite3)

| Command | Description |
|---------|-------------|
| `.tables` | List all tables |
| `.schema` | Show full schema |
| `.schema <table>` | Schema for one table |
| `.indexes` | List all indexes |
| `.indexes <table>` | Indexes for one table |
| `.mode column` | Column-aligned output |
| `.mode markdown` | Markdown table output |
| `.headers on` | Show column headers |
| `.stats on` | Show query stats |
| `.timer on` | Show query timing |
| `.quit` | Exit sqlite3 |

---

## Tables Overview

| Table | Purpose | Rows Check |
|-------|---------|------------|
| `settings` | Key-value application settings | `SELECT COUNT(*) FROM settings;` |
| `history` | Command execution history | `SELECT COUNT(*) FROM history;` |
| `upgrade_log` | Package upgrade events | `SELECT COUNT(*) FROM upgrade_log;` |
| `package_addition_log` | Packages added to Brewfile | `SELECT COUNT(*) FROM package_addition_log;` |
| `dotfiles` | Scanned dotfiles in $HOME | `SELECT COUNT(*) FROM dotfiles;` |
| `vscode_summary` | VSCode installation summary (singleton) | `SELECT * FROM vscode_summary;` |
| `vscode_profiles` | VSCode user data profiles | `SELECT COUNT(*) FROM vscode_profiles;` |
| `vscode_profile_extensions` | Extensions per VSCode profile | `SELECT COUNT(*) FROM vscode_profile_extensions;` |
| `vscode_profile_projects` | Projects per VSCode profile | `SELECT COUNT(*) FROM vscode_profile_projects;` |
| `vscode_refresh_log` | VSCode scan history | `SELECT COUNT(*) FROM vscode_refresh_log;` |
| `github_repos` | Tracked GitHub repositories | `SELECT COUNT(*) FROM github_repos;` |

---

## Schema Details

### `settings`
```sql
key         TEXT PRIMARY KEY,
value       TEXT NOT NULL,
updated_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
```

### `history`
```sql
id           INTEGER PRIMARY KEY AUTOINCREMENT,
command      TEXT NOT NULL,
output       TEXT,
executed_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
```

### `upgrade_log`
```sql
id            INTEGER PRIMARY KEY AUTOINCREMENT,
package_name  TEXT NOT NULL,
old_version   TEXT NOT NULL,
new_version   TEXT NOT NULL,
upgraded_at   DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
```
Index: `idx_upgrade_log_package ON upgrade_log(package_name)`

### `package_addition_log`
```sql
id            INTEGER PRIMARY KEY AUTOINCREMENT,
package_name  TEXT NOT NULL,
kind          TEXT NOT NULL,
version       TEXT NOT NULL DEFAULT '',
added_at      DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
```
Index: `idx_addition_log_package ON package_addition_log(package_name)`

### `dotfiles`
```sql
id           INTEGER PRIMARY KEY AUTOINCREMENT,
name         TEXT NOT NULL UNIQUE,
is_dir       BOOLEAN NOT NULL DEFAULT 0,
tool         TEXT NOT NULL DEFAULT '',
tool_manual  BOOLEAN NOT NULL DEFAULT 0,
modified_at  DATETIME NOT NULL,
created_at   DATETIME NOT NULL,
scanned_at   DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
size_bytes   INTEGER NOT NULL DEFAULT 0
```
Index: `idx_dotfiles_name ON dotfiles(name)`

### `vscode_summary`
```sql
id           INTEGER PRIMARY KEY CHECK (id = 1),
version      TEXT NOT NULL DEFAULT '',
installed    BOOLEAN NOT NULL DEFAULT 0,
paths_json   TEXT NOT NULL DEFAULT '[]',
scanned_at   DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
```
Check constraint ensures only one row (id always 1).

### `vscode_profiles`
```sql
location_id   TEXT PRIMARY KEY,
name          TEXT NOT NULL,
icon          TEXT NOT NULL DEFAULT '',
is_default    BOOLEAN NOT NULL DEFAULT 0,
profile_path  TEXT NOT NULL DEFAULT '',
dir_mtime     DATETIME,
scanned_at    DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
```

### `vscode_profile_extensions`
```sql
profile_id       TEXT NOT NULL,
ext_id           TEXT NOT NULL,
version          TEXT NOT NULL DEFAULT '',
installed_at     DATETIME,
install_path     TEXT NOT NULL DEFAULT '',
description      TEXT NOT NULL DEFAULT '',
long_description TEXT NOT NULL DEFAULT '',
PRIMARY KEY (profile_id, ext_id),
FOREIGN KEY (profile_id) REFERENCES vscode_profiles(location_id) ON DELETE CASCADE
```

### `vscode_profile_projects`
```sql
profile_id     TEXT NOT NULL,
project_path   TEXT NOT NULL,
exists_on_disk BOOLEAN NOT NULL DEFAULT 1,
PRIMARY KEY (profile_id, project_path),
FOREIGN KEY (profile_id) REFERENCES vscode_profiles(location_id) ON DELETE CASCADE
```

### `vscode_refresh_log`
```sql
id            INTEGER PRIMARY KEY AUTOINCREMENT,
refreshed_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
has_changes   BOOLEAN NOT NULL DEFAULT 0,
diff_json     TEXT NOT NULL DEFAULT '{}'
```

### `github_repos`
```sql
id            INTEGER PRIMARY KEY AUTOINCREMENT,
url           TEXT NOT NULL UNIQUE,
name          TEXT NOT NULL,
organization  TEXT NOT NULL DEFAULT '',
description   TEXT NOT NULL DEFAULT '',
language      TEXT NOT NULL DEFAULT '',
stars         INTEGER NOT NULL DEFAULT 0,
updated_at    DATETIME NOT NULL,
first_commit  DATETIME,
size_bytes    INTEGER NOT NULL DEFAULT 0,
category      TEXT NOT NULL DEFAULT '',
source        TEXT NOT NULL DEFAULT 'manual',
notes         TEXT NOT NULL DEFAULT '',
added_at      DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
```
Indexes: `idx_github_repos_name(name)`, `idx_github_repos_category(category)`, `idx_github_repos_source(source)`

---

## Common Queries

### Diagnostics & Health
```sql
-- Row counts for all tables (same as CLI `maximus doctor`)
SELECT 'settings' AS tbl, COUNT(*) FROM settings
UNION ALL SELECT 'history', COUNT(*) FROM history
UNION ALL SELECT 'upgrade_log', COUNT(*) FROM upgrade_log
UNION ALL SELECT 'package_addition_log', COUNT(*) FROM package_addition_log
UNION ALL SELECT 'dotfiles', COUNT(*) FROM dotfiles
UNION ALL SELECT 'vscode_summary', COUNT(*) FROM vscode_summary
UNION ALL SELECT 'vscode_profiles', COUNT(*) FROM vscode_profiles
UNION ALL SELECT 'vscode_profile_extensions', COUNT(*) FROM vscode_profile_extensions
UNION ALL SELECT 'vscode_profile_projects', COUNT(*) FROM vscode_profile_projects
UNION ALL SELECT 'vscode_refresh_log', COUNT(*) FROM vscode_refresh_log
UNION ALL SELECT 'github_repos', COUNT(*) FROM github_repos
ORDER BY tbl;

-- Database file size
SELECT page_count * page_size AS size_bytes
  FROM pragma_page_count, pragma_page_size;
```

### Settings
```sql
-- All settings
SELECT * FROM settings ORDER BY key;

-- Get a specific setting
SELECT value FROM settings WHERE key = 'last_vscode_scan_ref';

-- Upsert a setting
INSERT INTO settings (key, value) VALUES ('my_key', 'my_value')
  ON CONFLICT(key) DO UPDATE SET value = excluded.value,
                                  updated_at = CURRENT_TIMESTAMP;
```

### History
```sql
-- Recent command history
SELECT id, substr(command, 1, 80) AS command_short,
       substr(executed_at, 1, 19) AS executed_at
  FROM history
 ORDER BY id DESC LIMIT 20;

-- Count commands by date
SELECT date(executed_at) AS day, COUNT(*) AS cmds
  FROM history
 GROUP BY day
 ORDER BY day DESC LIMIT 14;
```

### Upgrades
```sql
-- Recent upgrades
SELECT * FROM upgrade_log ORDER BY upgraded_at DESC LIMIT 10;

-- Upgrades for a specific package
SELECT * FROM upgrade_log
 WHERE package_name LIKE '%node%'
 ORDER BY upgraded_at DESC;

-- Most upgraded packages
SELECT package_name, COUNT(*) AS times
  FROM upgrade_log
 GROUP BY package_name
 ORDER BY times DESC LIMIT 10;
```

### Package Additions
```sql
-- Recent additions
SELECT * FROM package_addition_log ORDER BY added_at DESC LIMIT 10;

-- Additions by kind
SELECT kind, COUNT(*) AS count
  FROM package_addition_log
 GROUP BY kind ORDER BY count DESC;
```

### Dotfiles
```sql
-- All dotfiles
SELECT * FROM dotfiles ORDER BY name;

-- Dotfiles with a tool assigned
SELECT name, tool FROM dotfiles WHERE tool != '' ORDER BY name;

-- Dotfiles without a tool
SELECT name, is_dir, size_bytes FROM dotfiles WHERE tool = '' ORDER BY name;

-- Dotfiles by size (largest first)
SELECT name, is_dir, size_bytes, modified_at
  FROM dotfiles
 ORDER BY size_bytes DESC LIMIT 20;

-- Dotfiles manually assigned a tool
SELECT name, tool FROM dotfiles WHERE tool_manual = 1 ORDER BY name;

-- Directories only
SELECT name, tool FROM dotfiles WHERE is_dir = 1 ORDER BY name;
```

### VSCode
```sql
-- VSCode summary
SELECT * FROM vscode_summary WHERE id = 1;

-- All profiles
SELECT * FROM vscode_profiles ORDER BY is_default DESC, name;

-- Extensions for a specific profile (by profile name)
SELECT pe.ext_id, pe.version, pe.description
  FROM vscode_profile_extensions pe
  JOIN vscode_profiles p ON pe.profile_id = p.location_id
 WHERE p.name = 'Default'
 ORDER BY pe.ext_id;

-- Extension dependency aggregation (all profiles)
SELECT pe.ext_id,
       group_concat(p.name, ', ') AS profiles,
       group_concat(pe.version, ', ') AS versions
  FROM vscode_profile_extensions pe
  JOIN vscode_profiles p ON pe.profile_id = p.location_id
 GROUP BY pe.ext_id
 ORDER BY pe.ext_id;

-- Projects for a specific profile
SELECT pp.*
  FROM vscode_profile_projects pp
  JOIN vscode_profiles p ON pp.profile_id = p.location_id
 WHERE p.name = 'Default' AND pp.exists_on_disk = 1;

-- Refresh history (with changes)
SELECT * FROM vscode_refresh_log
 WHERE has_changes = 1
 ORDER BY refreshed_at DESC LIMIT 10;

-- Last refresh timestamp
SELECT refreshed_at FROM vscode_refresh_log
 ORDER BY refreshed_at DESC LIMIT 1;
```

### GitHub Repos
```sql
-- All repos
SELECT * FROM github_repos ORDER BY name;

-- Repos by category
SELECT * FROM github_repos WHERE category = 'tooling' ORDER BY name;

-- Repos by source
SELECT source, COUNT(*) FROM github_repos GROUP BY source;

-- Top starred repos
SELECT name, organization, stars, language
  FROM github_repos
 ORDER BY stars DESC LIMIT 20;

-- Repos by language
SELECT language, COUNT(*) AS count
  FROM github_repos
 GROUP BY language
 ORDER BY count DESC;

-- Most recently updated
SELECT name, updated_at FROM github_repos
 ORDER BY updated_at DESC LIMIT 10;

-- Search repos
SELECT name, organization, description
  FROM github_repos
 WHERE name        LIKE '%terraform%'
    OR organization LIKE '%hashicorp%'
    OR description  LIKE '%infra%'
    OR category     LIKE '%iac%'
 ORDER BY stars DESC;

-- Bulk insert (INSERT OR IGNORE skips duplicates by unique url)
INSERT OR IGNORE INTO github_repos
    (url, name, organization, description, language, stars, updated_at, category, source)
 VALUES
    ('https://github.com/user/repo', 'repo', 'user', 'desc', 'Go', 10, '2025-01-01', 'dev', 'manual');
```

### Cleanup / Maintenance
```sql
-- Clear dotfiles (re-scan needed)
DELETE FROM dotfiles;

-- Delete a single dotfile
DELETE FROM dotfiles WHERE name = '.zshrc';

-- Clear all VSCode profile data (re-scan needed)
DELETE FROM vscode_profile_extensions;
DELETE FROM vscode_profile_projects;
DELETE FROM vscode_profiles;

-- Delete a GitHub repo by ID
DELETE FROM github_repos WHERE id = 42;

-- Purge history older than 90 days
DELETE FROM history WHERE executed_at < datetime('now', '-90 days');

-- Purge upgrade logs older than 1 year
DELETE FROM upgrade_log WHERE upgraded_at < datetime('now', '-1 year');

-- Vacuum to reclaim space (after deletes)
VACUUM;

-- Reindex all indexes
REINDEX;
```

---

## Connection Details (from Go code)

- **Driver:** `github.com/mattn/go-sqlite3`
- **Max open connections:** 1 (SQLite does not support concurrent writes)
- **Migrations:** Happen automatically on `db.Open()` — tables created if not existing, columns added via safe `ALTER TABLE` for backward compatibility
- **Config path:** `~/.config/maximus-cli/` (created automatically if missing)
- **Database path:** `~/.config/maximus-cli/maximus.db`

---

## Check Database Size

```bash
# Check page count and page size (logical SQLite size)
sqlite3 ~/.config/maximus-cli/maximus.db \
  "SELECT page_count * page_size AS size_bytes FROM pragma_page_count, pragma_page_size;"

# Human readable (MB)
sqlite3 ~/.config/maximus-cli/maximus.db \
  "SELECT printf('%.2f MB', page_count * page_size / 1048576.0) AS size FROM pragma_page_count, pragma_page_size;"

# File size on disk (OS level)
ls -lh ~/.config/maximus-cli/maximus.db
```

| Method | Description |
|--------|-------------|
| `pragma_page_count * pragma_page_size` | Logical SQLite size (pages × page size) |
| `ls -lh` | Actual file size on disk |

They're usually the same unless the file has been vacuumed or has free pages from deletions.