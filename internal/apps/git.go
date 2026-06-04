package apps

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"
	"time"

	"maximus-cli/internal/db"
)

var urlRe = regexp.MustCompile(`github\.com[:/]([^/]+)/([^/]+?)(?:\.git)?$`)

type gitHubAPIResponse struct {
	Name            string `json:"name"`
	FullName        string `json:"full_name"`
	Description     string `json:"description"`
	Language        string `json:"language"`
	StargazersCount int    `json:"stargazers_count"`
	UpdatedAt       string `json:"pushed_at"`
	CreatedAt       string `json:"created_at"`
	Size            int    `json:"size"`
}

func ParseGitHubURL(raw string) (owner, repo string, ok bool) {
	raw = strings.TrimSpace(raw)
	m := urlRe.FindStringSubmatch(raw)
	if m == nil {
		return "", "", false
	}
	return m[1], m[2], true
}

func FetchRepoMetadata(rawURL string) (*db.GitHubRepo, error) {
	owner, repo, ok := ParseGitHubURL(rawURL)
	if !ok {
		return nil, fmt.Errorf("invalid GitHub URL: %s", rawURL)
	}

	apiURL := fmt.Sprintf("https://api.github.com/repos/%s/%s", owner, repo)
	req, err := http.NewRequest(http.MethodGet, apiURL, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Accept", "application/vnd.github.v3+json")
	if token := os.Getenv("GITHUB_TOKEN"); token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch %s: %w", apiURL, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == 403 {
		return nil, fmt.Errorf("rate limited by GitHub API (try setting GITHUB_TOKEN)")
	}
	if resp.StatusCode == 404 {
		return nil, fmt.Errorf("repository %s/%s not found", owner, repo)
	}
	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("GitHub API %d: %s", resp.StatusCode, string(body))
	}

	var data gitHubAPIResponse
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	r := &db.GitHubRepo{
		URL:          fmt.Sprintf("https://github.com/%s/%s", owner, repo),
		Name:         data.Name,
		Organization: owner,
		Description:  data.Description,
		Language:     data.Language,
		Stars:        data.StargazersCount,
		SizeBytes:    data.Size,
		Source:       "api",
		AddedAt:      time.Now(),
	}

	if data.UpdatedAt != "" {
		r.UpdatedAt, _ = time.Parse(time.RFC3339, data.UpdatedAt)
	}
	if data.CreatedAt != "" {
		r.FirstCommit, _ = time.Parse(time.RFC3339, data.CreatedAt)
	}

	return r, nil
}

type CSVRepo struct {
	URL          string
	Name         string
	Organization string
	Description  string
	Language     string
	Stars        int
	Category     string
	Notes        string
}

func BulkImportCSV(filepath string, database *db.DB) (int, error) {
	f, err := os.Open(filepath)
	if err != nil {
		return 0, fmt.Errorf("open file: %w", err)
	}
	defer f.Close()

	reader := csv.NewReader(f)
	reader.TrimLeadingSpace = true
	reader.FieldsPerRecord = -1

	records, err := reader.ReadAll()
	if err != nil {
		return 0, fmt.Errorf("read CSV: %w", err)
	}
	if len(records) < 2 {
		return 0, fmt.Errorf("CSV must have a header row and at least one data row")
	}

	headerIdx := make(map[string]int)
	for i, h := range records[0] {
		headerIdx[strings.ToLower(strings.TrimSpace(h))] = i
	}

	required := []string{"url"}
	for _, r := range required {
		if _, ok := headerIdx[r]; !ok {
			return 0, fmt.Errorf("CSV missing required column: %s", r)
		}
	}

	var repos []db.GitHubRepo
	for _, row := range records[1:] {
		r := db.GitHubRepo{
			URL:     strings.TrimSpace(row[headerIdx["url"]]),
			Source:  "import",
			AddedAt: time.Now(),
		}

		if idx, ok := headerIdx["name"]; ok && idx < len(row) {
			r.Name = strings.TrimSpace(row[idx])
		}
		if idx, ok := headerIdx["organization"]; ok && idx < len(row) {
			r.Organization = strings.TrimSpace(row[idx])
		}
		if idx, ok := headerIdx["description"]; ok && idx < len(row) {
			r.Description = strings.TrimSpace(row[idx])
		}
		if idx, ok := headerIdx["language"]; ok && idx < len(row) {
			r.Language = strings.TrimSpace(row[idx])
		}
		if idx, ok := headerIdx["category"]; ok && idx < len(row) {
			r.Category = strings.TrimSpace(row[idx])
		}
		if idx, ok := headerIdx["notes"]; ok && idx < len(row) {
			r.Notes = strings.TrimSpace(row[idx])
		}
		if idx, ok := headerIdx["stars"]; ok && idx < len(row) {
			fmt.Sscanf(strings.TrimSpace(row[idx]), "%d", &r.Stars)
		}

		if r.URL == "" {
			continue
		}

		// Normalize URL
		owner, repo, ok := ParseGitHubURL(r.URL)
		if ok {
			r.URL = fmt.Sprintf("https://github.com/%s/%s", owner, repo)
			if r.Name == "" {
				r.Name = repo
			}
			if r.Organization == "" {
				r.Organization = owner
			}
		}
		if r.Name == "" {
			r.Name = r.URL
		}

		repos = append(repos, r)
	}

	if len(repos) == 0 {
		return 0, fmt.Errorf("no valid repos found in CSV")
	}

	if err := database.BulkInsertGitHubRepos(repos); err != nil {
		return 0, err
	}

	return len(repos), nil
}

func NormalizeGitHubURL(raw string) string {
	raw = strings.TrimSpace(raw)
	u, err := url.Parse(raw)
	if err == nil && u.Host == "" {
		raw = "https://" + raw
	}
	owner, repo, ok := ParseGitHubURL(raw)
	if !ok {
		return raw
	}
	return fmt.Sprintf("https://github.com/%s/%s", owner, repo)
}

func RefreshStarsWithRateLimit(
	database *db.DB,
	lastRepoID int64,
	maxRequests int,
) (newLastID int64, wrapped bool, err error) {
	refs, err := database.GetRepoRefs()
	if err != nil {
		return lastRepoID, false, fmt.Errorf("fetch repo refs: %w", err)
	}
	if len(refs) == 0 {
		return 0, false, nil
	}

	startIndex := 0
	found := false
	for i, ref := range refs {
		if ref.ID == lastRepoID {
			startIndex = i + 1
			found = true
			break
		}
	}
	if startIndex >= len(refs) {
		startIndex = 0
		wrapped = true
	}
	if !found {
		startIndex = 0
	}

	now := time.Now()
	cursorIndex := startIndex
	processedCount := 0

	for processedCount < maxRequests {
		if processedCount > 0 && cursorIndex == startIndex {
			wrapped = true
			break
		}

		ref := refs[cursorIndex]

		meta, fetchErr := FetchRepoMetadata(ref.URL)
		if fetchErr != nil {
			errStr := fetchErr.Error()
			if strings.Contains(strings.ToLower(errStr), "rate limited") || strings.Contains(errStr, "403") || strings.Contains(errStr, "429") {
				return lastRepoID, wrapped, fetchErr
			}
			// Skip this repo for other kinds of errors (e.g. 404), advancing the cursor so we don't get stuck.
			lastRepoID = ref.ID
			cursorIndex = (cursorIndex + 1) % len(refs)
			processedCount++
			if cursorIndex == 0 {
				wrapped = true
			}
			continue
		}

		if err := database.UpsertGitHubRepo(*meta); err != nil {
			return lastRepoID, wrapped, fmt.Errorf("update repo %s: %w", ref.URL, err)
		}

		if err := database.InsertStarSnapshot(ref.ID, meta.Stars, now); err != nil {
			return lastRepoID, wrapped, fmt.Errorf("insert star snapshot for %s: %w", ref.URL, err)
		}

		lastRepoID = ref.ID
		cursorIndex = (cursorIndex + 1) % len(refs)
		processedCount++
		if cursorIndex == 0 {
			wrapped = true
		}
	}

	return lastRepoID, wrapped, nil
}
