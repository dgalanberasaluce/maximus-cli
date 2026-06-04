# Plan: File Upload (CSV/JSON) for GitHub Repo Tracker

## What exists today
- **`apps/git.go:112`** — `BulkImportCSV()` already parses CSV files with a flexible header-based mapping. It uses `INSERT OR IGNORE` via `database.BulkInsertGitHubRepos()`.
- **No UI for file upload** — the CSV function exists but is never called from the TUI. There's no file picker or drag-and-drop in the app.

---

## What needs to change

Since Bubbletea/TUI doesn't have native file pickers, the approach will use **paste-based file path input** (matching the existing `n` add-repo overlay pattern):

### 1. `internal/apps/git.go` — Add JSON import + shared validation
- Add `BulkImportJSON(filepath string, database *db.DB) (int, error)` that:
  - Reads a JSON array of objects with the same field mapping as CSV (`url` required, `name`, `organization`, `description`, `language`, `category`, `notes`, `stars` optional)
  - Supports both full URLs (`https://github.com/owner/repo`) and shorthand (`owner/repo`)
  - Sets `Source: "import"` and `AddedAt: time.Now()`
  - Uses `database.BulkInsertGitHubRepos()` for batch insert
- Extract shared repo validation/normalization into a helper so both CSV and JSON use the same URL parsing and field mapping logic

### 2. `internal/tui/model.go` — Add import overlay state
- Add fields to the `Model` struct:
  - `githubRepoImportMode bool` — whether the import overlay is visible
  - `githubRepoImportInput textinput.Model` — file path input
  - `githubRepoImportInputMode bool` — input focused
  - `githubRepoImportMsg string` — status message
  - `githubRepoImportMsgType string` — `"loading"`, `"success"`, `"error"`
  - `githubRepoImportSpinner spinner.Model` — loading indicator

### 3. `internal/tui/model.go:New()` — Initialize import components
- Initialize `githubRepoImportInput` and `githubRepoImportSpinner` in the constructor

### 4. `internal/tui/update.go` — Key handling + message handling + command
- Add `importRepoDoneMsg` message type (same shape as `addRepoDoneMsg`)
- In the GitHub repos key handler (`Update`), add a new case for key `"i"`:
  - Opens the import overlay with a file path input
- Handle input: when user types a path and presses enter:
  - Detect file format by extension (`.csv` or `.json`)
  - Validate file exists
  - Run background cmd that calls `BulkImportCSV` or `BulkImportJSON`
  - Show spinner during import
  - On success: show count of imported repos, refresh table
  - On error: show error message
- Add `importGitHubReposCmd(filepath string, format string, database *db.DB)` tea.Cmd

### 5. `internal/tui/view.go` — Render import overlay
- Add `renderGitHubRepoImportOverlay()` — centered modal panel (similar to `renderGitHubRepoAddOverlay()`):
  - Header: "Import Repositories"
  - Shows file path text input
  - Format hint: "Supports .csv and .json files (url column required)"
  - Loading state with spinner
  - Success/error message with color coding
  - Key hints: enter to import, esc to cancel

### 6. `internal/tui/view.go:renderGitHubRepos()` — Update key hints
- Append `· i import file` to the footer key hints line

---

## Supported file formats

### CSV (existing logic, enhanced)
```csv
url,name,organization,language,stars,category,notes
https://github.com/golang/go,Go,Go,Go,98000,language,Main Go repo
```

### JSON
```json
[
  {
    "url": "https://github.com/golang/go",
    "name": "Go",
    "language": "Go",
    "stars": 98000,
    "category": "language",
    "notes": "Main Go repo"
  }
]
```

---

## Files to modify

| File | Changes |
|------|---------|
| `internal/apps/git.go` | Add `BulkImportJSON()`, extract shared normalization |
| `internal/tui/model.go` | Add import overlay state fields, init in `New()` |
| `internal/tui/update.go` | Add `importRepoDoneMsg`, key handler for `i`, `importGitHubReposCmd()` |
| `internal/tui/view.go` | Add `renderGitHubRepoImportOverlay()`, update key hints |

---

## Questions for consideration

1. **File input method**: Since there's no native file picker in TUI, the user will paste a file path. Should we also support **drag-and-drop** of the file onto the terminal (which passes the path via environment variable in some terminals)? This would require adding a `drop` key handler.

2. **Import behavior on duplicate URLs**: The existing `BulkInsertGitHubRepos` uses `INSERT OR IGNORE`. Should import **skip** duplicates (current behavior), **update** them (use `UPSERT`), or offer a choice?

3. **JSON format flexibility**: Should JSON support both a flat array of repo objects AND a format where each object has a `repos` key wrapping the array (like `{"repos": [...]}`)?
