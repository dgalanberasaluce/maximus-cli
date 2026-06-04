// Package apps gathers information about external applications installed on the system.
package apps

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"maximus-cli/internal/db"
)

// PathInfo represents status of a configured path.
type PathInfo struct {
	Label  string `json:"label"`
	Path   string `json:"path"`
	Exists bool   `json:"exists"`
}

// VSCodeSummary holds gathered information about Visual Studio Code.
type VSCodeSummary struct {
	Version      string
	Installed    bool
	Paths        []PathInfo
	Dependencies []string
	ScannedAt    time.Time
}

// VSCodeProfile represents a rich profile of VSCode.
type VSCodeProfile struct {
	LocationID  string
	Name        string
	Icon        string
	IsDefault   bool
	ProfilePath string
	DirMtime    time.Time
	Extensions  []VSCodeExtension
	Projects    []VSCodeProject
}

// VSCodeExtension representa una extensión en un perfil.
type VSCodeExtension struct {
	ID              string
	Version         string
	InstalledAt     time.Time
	InstallPath     string
	Description     string
	LongDescription string
}

// VSCodeProject represents a project mapped to a profile.
type VSCodeProject struct {
	Path         string
	ExistsOnDisk bool
}

// VSCodeDiff represents differences between two scans.
type VSCodeDiff struct {
	VersionChanged  bool     `json:"version_changed"`
	OldVersion      string   `json:"old_version"`
	NewVersion      string   `json:"new_version"`
	PathsAdded      []string `json:"paths_added"`   // paths that started existing
	PathsRemoved    []string `json:"paths_removed"` // paths that stopped existing
	ProfilesAdded   []string `json:"profiles_added"`
	ProfilesRemoved []string `json:"profiles_removed"`
	ExtChanges      []string `json:"ext_changes"` // human readable diff lines for extensions
	HasAnyChange    bool     `json:"has_any_change"`
}

// brewCaskInfo is used to parse the JSON output of brew info.
type brewCaskInfo struct {
	Casks []struct {
		Token         string `json:"token"`
		Version       string `json:"version"`
		Installed     string `json:"installed"`
		InstalledTime int64  `json:"installed_time"`
		DependsOn     struct {
			Formula []string               `json:"formula"`
			Cask    []string               `json:"cask"`
			Macos   map[string]interface{} `json:"macos"`
		} `json:"depends_on"`
	} `json:"casks"`
}

// storageJSON represents the structure inside VSCode storage.json.
type storageJSON struct {
	UserDataProfiles    []profileEntry               `json:"userDataProfiles"`
	ProfileAssociations profileAssociationsStructure `json:"profileAssociations"`
}

type profileEntry struct {
	Location string `json:"location"`
	Name     string `json:"name"`
	Icon     string `json:"icon"`
}

type profileAssociationsStructure struct {
	Workspaces map[string]string `json:"workspaces"`
}

// extJSONEntry represents an entry in an extensions.json file.
type extJSONEntry struct {
	Identifier struct {
		ID string `json:"id"`
	} `json:"identifier"`
	Version  string `json:"version"`
	Location struct {
		Path string `json:"path"`
	} `json:"location"`
	Metadata struct {
		InstalledTimestamp int64 `json:"installedTimestamp"`
	} `json:"metadata"`
}

// ScanVSCode queries the live system for VSCode information, compares it with
// the current data in SQLite3, computes the difference, saves the new data,
// and logs the result.
func ScanVSCode(database *db.DB) (VSCodeDiff, error) {
	diff := VSCodeDiff{}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		return diff, fmt.Errorf("could not get user home dir: %w", err)
	}

	userDataDir := filepath.Join(homeDir, "Library/Application Support/Code/User")

	// 1. Gather live global info
	liveSummary := VSCodeSummary{
		Installed: false,
	}

	// Fetch info from brew cask
	cmd := exec.Command("brew", "info", "--cask", "--json=v2", "visual-studio-code")
	out, err := cmd.Output()
	var dependencies []string
	if err == nil {
		var info brewCaskInfo
		if err := json.Unmarshal(out, &info); err == nil && len(info.Casks) > 0 {
			c := info.Casks[0]
			if c.Installed != "" {
				liveSummary.Installed = true
				liveSummary.Version = c.Installed
			} else {
				liveSummary.Version = c.Version + " (not installed/detected via brew)"
			}

			// Extract dependencies
			for _, f := range c.DependsOn.Formula {
				dependencies = append(dependencies, fmt.Sprintf("formula: %s", f))
			}
			for _, cs := range c.DependsOn.Cask {
				dependencies = append(dependencies, fmt.Sprintf("cask: %s", cs))
			}
			if len(c.DependsOn.Macos) > 0 {
				for k, v := range c.DependsOn.Macos {
					dependencies = append(dependencies, fmt.Sprintf("macos %s: %v", k, v))
				}
			}
		}
	}

	if !liveSummary.Installed {
		// Fallback: Check if the app bundle is present in standard location
		appPath := "/Applications/Visual Studio Code.app"
		if fi, err := os.Stat(appPath); err == nil && fi.IsDir() {
			liveSummary.Installed = true
			liveSummary.Version = "Detected at " + appPath
		}
	}

	if len(dependencies) == 0 {
		dependencies = append(dependencies, "None")
	}
	liveSummary.Dependencies = dependencies

	// Paths verification
	pathsToVerify := []struct {
		label string
		path  string
	}{
		{"Executable (CLI)", "/Applications/Visual Studio Code.app/Contents/Resources/app/bin/code"},
		{"Symlink (Homebrew)", "/opt/homebrew/bin/code"},
		{"Symlink (System)", "/usr/local/bin/code"},
		{"Extensions Folder", filepath.Join(homeDir, ".vscode/extensions")},
		{"User Data Directory", userDataDir},
		{"Settings File", filepath.Join(userDataDir, "settings.json")},
		{"Global Storage", filepath.Join(userDataDir, "globalStorage")},
	}

	for _, p := range pathsToVerify {
		exists := false
		if _, err := os.Stat(p.path); err == nil {
			exists = true
		}
		liveSummary.Paths = append(liveSummary.Paths, PathInfo{
			Label:  p.label,
			Path:   p.path,
			Exists: exists,
		})
	}
	liveSummary.ScannedAt = time.Now()

	// 2. Load Old Data from DB to calculate diffs
	oldSummaryRow, oldExists, err := database.GetVSCodeSummary()
	if err == nil && oldExists {
		diff.OldVersion = oldSummaryRow.Version
		diff.NewVersion = liveSummary.Version
		if oldSummaryRow.Version != liveSummary.Version {
			diff.VersionChanged = true
		}

		// Compare paths
		var oldPaths []PathInfo
		if err := json.Unmarshal([]byte(oldSummaryRow.PathsJSON), &oldPaths); err == nil {
			oldMap := make(map[string]bool)
			for _, p := range oldPaths {
				oldMap[p.Label] = p.Exists
			}
			for _, p := range liveSummary.Paths {
				if oldExist, ok := oldMap[p.Label]; ok {
					if p.Exists && !oldExist {
						diff.PathsAdded = append(diff.PathsAdded, p.Label)
					} else if !p.Exists && oldExist {
						diff.PathsRemoved = append(diff.PathsRemoved, p.Label)
					}
				}
			}
		}
	} else {
		diff.OldVersion = "Ninguna"
		diff.NewVersion = liveSummary.Version
		diff.VersionChanged = true
	}

	// 3. Scan profiles, projects, and extensions
	var liveProfiles []VSCodeProfile

	storagePath := filepath.Join(userDataDir, "globalStorage/storage.json")
	var storage storageJSON

	if data, err := os.ReadFile(storagePath); err == nil {
		_ = json.Unmarshal(data, &storage)
	}

	// Parse projects from workspaces associations
	// Project path mapping: URL -> LocationID
	profileProjects := make(map[string][]VSCodeProject)
	for rawURL, loc := range storage.ProfileAssociations.Workspaces {
		pPath := strings.TrimPrefix(rawURL, "file://")
		if unescaped, err := url.PathUnescape(pPath); err == nil {
			pPath = unescaped
		}
		existsOnDisk := false
		if _, err := os.Stat(pPath); err == nil {
			existsOnDisk = true
		}

		proj := VSCodeProject{
			Path:         pPath,
			ExistsOnDisk: existsOnDisk,
		}

		profileKey := loc
		if loc == "__default__profile__" || loc == "" {
			profileKey = "__default__"
		}
		profileProjects[profileKey] = append(profileProjects[profileKey], proj)
	}

	// Helper to load extensions from an extensions.json file
	loadExtensions := func(extFilePath string) []VSCodeExtension {
		var list []VSCodeExtension
		if data, err := os.ReadFile(extFilePath); err == nil {
			var entries []extJSONEntry
			if err := json.Unmarshal(data, &entries); err == nil {
				for _, entry := range entries {
					var instTime time.Time
					if entry.Metadata.InstalledTimestamp > 0 {
						instTime = time.UnixMilli(entry.Metadata.InstalledTimestamp)
					}
					desc := ""
					if entry.Location.Path != "" {
						pkgJSONPath := filepath.Join(entry.Location.Path, "package.json")
						if pkgData, err := os.ReadFile(pkgJSONPath); err == nil {
							var pkg struct {
								Description string `json:"description"`
							}
							if err := json.Unmarshal(pkgData, &pkg); err == nil {
								desc = pkg.Description
							}
						}
					}
					list = append(list, VSCodeExtension{
						ID:          entry.Identifier.ID,
						Version:     entry.Version,
						InstalledAt: instTime,
						InstallPath: entry.Location.Path,
						Description: desc,
					})
				}
			}
		}
		return list
	}

	// Build Default profile
	defaultProfile := VSCodeProfile{
		LocationID:  "__default__",
		Name:        "Default",
		IsDefault:   true,
		ProfilePath: userDataDir,
	}
	if fi, err := os.Stat(userDataDir); err == nil {
		defaultProfile.DirMtime = fi.ModTime()
	}
	// Default profile extensions: located in ~/.vscode/extensions/extensions.json
	defaultExtPath := filepath.Join(homeDir, ".vscode/extensions/extensions.json")
	defaultProfile.Extensions = loadExtensions(defaultExtPath)
	defaultProfile.Projects = profileProjects["__default__"]

	liveProfiles = append(liveProfiles, defaultProfile)

	// Build custom profiles from storage.json
	for _, pe := range storage.UserDataProfiles {
		if pe.Location == "" || pe.Location == "__default__profile__" {
			continue
		}
		pPath := filepath.Join(userDataDir, "profiles", pe.Location)
		prof := VSCodeProfile{
			LocationID:  pe.Location,
			Name:        pe.Name,
			Icon:        pe.Icon,
			IsDefault:   false,
			ProfilePath: pPath,
		}
		if fi, err := os.Stat(pPath); err == nil {
			prof.DirMtime = fi.ModTime()
		}

		// Profile extensions are in its profile directory extensions.json
		profExtPath := filepath.Join(pPath, "extensions.json")
		prof.Extensions = loadExtensions(profExtPath)
		prof.Projects = profileProjects[pe.Location]

		liveProfiles = append(liveProfiles, prof)
	}

	// Fetch short descriptions and READMEs from VSCode Marketplace
	var allExtIDs []string
	for _, lp := range liveProfiles {
		for _, ext := range lp.Extensions {
			if ext.ID != "" {
				allExtIDs = append(allExtIDs, ext.ID)
			}
		}
	}
	if len(allExtIDs) > 0 {
		metas := fetchMarketplaceMetadata(allExtIDs)
		readmes := downloadREADMEs(metas)
		for i := range liveProfiles {
			for j := range liveProfiles[i].Extensions {
				extIDLower := strings.ToLower(liveProfiles[i].Extensions[j].ID)
				if meta, ok := metas[extIDLower]; ok {
					if meta.ShortDescription != "" {
						liveProfiles[i].Extensions[j].Description = meta.ShortDescription
					}
				}
				if readme, ok := readmes[extIDLower]; ok {
					liveProfiles[i].Extensions[j].LongDescription = readme
				}
			}
		}
	}

	// 4. Compare profiles and extensions with DB to build diffs
	oldProfiles, err := database.GetVSCodeProfiles()
	if err == nil {
		oldProfMap := make(map[string]db.VSCodeProfileRow)
		for _, op := range oldProfiles {
			oldProfMap[op.LocationID] = op
		}

		// Find added profiles
		for _, lp := range liveProfiles {
			if _, ok := oldProfMap[lp.LocationID]; !ok {
				diff.ProfilesAdded = append(diff.ProfilesAdded, lp.Name)
			}
		}
		// Find removed profiles
		liveProfMap := make(map[string]bool)
		for _, lp := range liveProfiles {
			liveProfMap[lp.LocationID] = true
		}
		for _, op := range oldProfiles {
			if !liveProfMap[op.LocationID] {
				diff.ProfilesRemoved = append(diff.ProfilesRemoved, op.Name)
			}
		}

		// Compare extensions for each profile
		for _, lp := range liveProfiles {
			oldExts, err := database.GetVSCodeExtensions(lp.LocationID)
			if err != nil {
				continue
			}
			oldExtMap := make(map[string]string) // ID -> Version
			for _, oe := range oldExts {
				oldExtMap[oe.ExtID] = oe.Version
			}

			// Check current vs old
			newExtMap := make(map[string]bool)
			for _, ne := range lp.Extensions {
				newExtMap[ne.ID] = true
				if oldVer, ok := oldExtMap[ne.ID]; ok {
					if oldVer != ne.Version {
						diff.ExtChanges = append(diff.ExtChanges, fmt.Sprintf("%s: actualizada %s (%s → %s)", lp.Name, ne.ID, oldVer, ne.Version))
					}
				} else {
					diff.ExtChanges = append(diff.ExtChanges, fmt.Sprintf("%s: añadida %s (%s)", lp.Name, ne.ID, ne.Version))
				}
			}

			for _, oe := range oldExts {
				if !newExtMap[oe.ExtID] {
					diff.ExtChanges = append(diff.ExtChanges, fmt.Sprintf("%s: eliminada %s", lp.Name, oe.ExtID))
				}
			}
		}
	}

	diff.HasAnyChange = diff.VersionChanged ||
		len(diff.PathsAdded) > 0 ||
		len(diff.PathsRemoved) > 0 ||
		len(diff.ProfilesAdded) > 0 ||
		len(diff.ProfilesRemoved) > 0 ||
		len(diff.ExtChanges) > 0

	// 5. Persist Everything to DB
	// Summary
	pathsJSON, _ := json.Marshal(liveSummary.Paths)
	_ = database.UpsertVSCodeSummary(liveSummary.Version, liveSummary.Installed, string(pathsJSON))

	// Upsert dependencies and other configs as settings in DB so they can be read back
	depsJSON, _ := json.Marshal(liveSummary.Dependencies)
	_ = database.SetSetting("vscode_dependencies", string(depsJSON))

	// Clear and write Profiles
	if err := database.ClearVSCodeProfiles(); err == nil {
		for _, lp := range liveProfiles {
			row := db.VSCodeProfileRow{
				LocationID:  lp.LocationID,
				Name:        lp.Name,
				Icon:        lp.Icon,
				IsDefault:   lp.IsDefault,
				ProfilePath: lp.ProfilePath,
				DirMtime:    lp.DirMtime,
			}
			_ = database.UpsertVSCodeProfile(row)

			// Extensions
			var exts []db.VSCodeExtRow
			for _, le := range lp.Extensions {
				exts = append(exts, db.VSCodeExtRow{
					ProfileID:       lp.LocationID,
					ExtID:           le.ID,
					Version:         le.Version,
					InstalledAt:     le.InstalledAt,
					InstallPath:     le.InstallPath,
					Description:     le.Description,
					LongDescription: le.LongDescription,
				})
			}
			_ = database.UpsertVSCodeExtensions(lp.LocationID, exts)

			// Projects
			var projs []db.VSCodeProjectRow
			for _, proj := range lp.Projects {
				projs = append(projs, db.VSCodeProjectRow{
					ProfileID:    lp.LocationID,
					ProjectPath:  proj.Path,
					ExistsOnDisk: proj.ExistsOnDisk,
				})
			}
			_ = database.UpsertVSCodeProjects(lp.LocationID, projs)
		}
	}

	// 6. Log Refresh in Database (Always do this to keep history)
	diffJSON, _ := json.Marshal(diff)
	_ = database.InsertVSCodeRefreshLog(diff.HasAnyChange, string(diffJSON))

	return diff, nil
}

// LoadVSCodeSummary loads VSCode global status from DB.
func LoadVSCodeSummary(database *db.DB) (VSCodeSummary, error) {
	summary := VSCodeSummary{}
	row, exists, err := database.GetVSCodeSummary()
	if err != nil {
		return summary, err
	}
	if !exists {
		return summary, fmt.Errorf("no vscode summary stored in database")
	}

	summary.Version = row.Version
	summary.Installed = row.Installed
	summary.ScannedAt = row.ScannedAt

	_ = json.Unmarshal([]byte(row.PathsJSON), &summary.Paths)

	// Fetch dependencies from settings
	depsVal, err := database.GetSetting("vscode_dependencies")
	if err == nil && depsVal != "" {
		_ = json.Unmarshal([]byte(depsVal), &summary.Dependencies)
	}

	return summary, nil
}

// LoadVSCodeProfiles loads VSCode profiles from DB.
func LoadVSCodeProfiles(database *db.DB, includeArchived bool) ([]VSCodeProfile, error) {
	rows, err := database.GetVSCodeProfiles()
	if err != nil {
		return nil, err
	}

	var list []VSCodeProfile
	for _, r := range rows {
		p := VSCodeProfile{
			LocationID:  r.LocationID,
			Name:        r.Name,
			Icon:        r.Icon,
			IsDefault:   r.IsDefault,
			ProfilePath: r.ProfilePath,
			DirMtime:    r.DirMtime,
		}

		// Load Extensions
		extRows, err := database.GetVSCodeExtensions(r.LocationID)
		if err == nil {
			for _, er := range extRows {
				p.Extensions = append(p.Extensions, VSCodeExtension{
					ID:              er.ExtID,
					Version:         er.Version,
					InstalledAt:     er.InstalledAt,
					InstallPath:     er.InstallPath,
					Description:     er.Description,
					LongDescription: er.LongDescription,
				})
			}
		}

		// Load Projects
		projRows, err := database.GetVSCodeProjects(r.LocationID, includeArchived)
		if err == nil {
			for _, pr := range projRows {
				p.Projects = append(p.Projects, VSCodeProject{
					Path:         pr.ProjectPath,
					ExistsOnDisk: pr.ExistsOnDisk,
				})
			}
		}

		list = append(list, p)
	}
	return list, nil
}

// VSCodeExtInstall represents an installation of an extension inside a specific profile.
type VSCodeExtInstall struct {
	ProfileName string
	Version     string
	InstalledAt time.Time
	InstallPath string
}

// VSCodeExtAgg representa una extensión agregada a través de perfiles.
type VSCodeExtAgg struct {
	ID              string
	Description     string
	LongDescription string
	Installs        []VSCodeExtInstall
}

// LoadVSCodeDependencies loads and aggregates VSCode extensions from the database.
func LoadVSCodeDependencies(database *db.DB) ([]VSCodeExtAgg, error) {
	rows, err := database.GetVSCodeDependenciesAgg()
	if err != nil {
		return nil, err
	}

	var list []VSCodeExtAgg
	var current *VSCodeExtAgg

	for _, r := range rows {
		if current == nil || current.ID != r.ExtID {
			if current != nil {
				list = append(list, *current)
			}
			current = &VSCodeExtAgg{
				ID:              r.ExtID,
				Description:     r.Description,
				LongDescription: r.LongDescription,
			}
		}
		current.Installs = append(current.Installs, VSCodeExtInstall{
			ProfileName: r.ProfileName,
			Version:     r.Version,
			InstalledAt: r.InstalledAt,
			InstallPath: r.InstallPath,
		})
	}
	if current != nil {
		list = append(list, *current)
	}

	return list, nil
}

// ── VSCode Marketplace Integration ──────────────────────────────────────────

type marketplaceMeta struct {
	ShortDescription string
	ReadmeURL        string
}

type marketplaceQueryRequest struct {
	Filters []marketplaceFilter `json:"filters"`
	Flags   int                 `json:"flags"`
}

type marketplaceFilter struct {
	Criteria   []marketplaceCriteria `json:"criteria"`
	PageSize   int                   `json:"pageSize"`
	PageNumber int                   `json:"pageNumber"`
}

type marketplaceCriteria struct {
	FilterType int    `json:"filterType"`
	Value      string `json:"value"`
}

type marketplaceQueryResponse struct {
	Results []marketplaceResult `json:"results"`
}

type marketplaceResult struct {
	Extensions []marketplaceExtension `json:"extensions"`
}

type marketplaceExtension struct {
	Publisher struct {
		PublisherName string `json:"publisherName"`
	} `json:"publisher"`
	ExtensionName    string `json:"extensionName"`
	ShortDescription string `json:"shortDescription"`
	Versions         []struct {
		Version string `json:"version"`
		Files   []struct {
			AssetType string `json:"assetType"`
			Source    string `json:"source"`
		} `json:"files"`
	} `json:"versions"`
}

func buildMarketplaceQueryBody(extIDs []string) marketplaceQueryRequest {
	criteria := make([]marketplaceCriteria, 0, len(extIDs))
	for _, id := range extIDs {
		criteria = append(criteria, marketplaceCriteria{
			FilterType: 7, // ExtensionName
			Value:      id,
		})
	}

	return marketplaceQueryRequest{
		Filters: []marketplaceFilter{
			{
				Criteria:   criteria,
				PageSize:   len(extIDs),
				PageNumber: 1,
			},
		},
		Flags: 914, // IncludeVersionProperties | IncludeFiles | IncludeStatistics | IncludeInstallationTargets | IncludeAssetUri
	}
}

func fetchMarketplaceMetadata(extIDs []string) map[string]marketplaceMeta {
	metas := make(map[string]marketplaceMeta)
	if len(extIDs) == 0 {
		return metas
	}

	// Remove duplicates
	uniqueIDs := make([]string, 0, len(extIDs))
	seen := make(map[string]bool)
	for _, id := range extIDs {
		lower := strings.ToLower(id)
		if !seen[lower] && id != "" {
			seen[lower] = true
			uniqueIDs = append(uniqueIDs, id)
		}
	}

	// Query in batches of 50
	const batchSize = 50
	client := &http.Client{Timeout: 10 * time.Second}

	for i := 0; i < len(uniqueIDs); i += batchSize {
		end := i + batchSize
		if end > len(uniqueIDs) {
			end = len(uniqueIDs)
		}
		batch := uniqueIDs[i:end]

		reqBody := buildMarketplaceQueryBody(batch)
		bodyBytes, err := json.Marshal(reqBody)
		if err != nil {
			continue
		}

		req, err := http.NewRequest("POST", "https://marketplace.visualstudio.com/_apis/public/gallery/extensionquery", bytes.NewBuffer(bodyBytes))
		if err != nil {
			continue
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json;api-version=3.0-preview.1")

		resp, err := client.Do(req)
		if err != nil {
			continue
		}

		var respData marketplaceQueryResponse
		if err := json.NewDecoder(resp.Body).Decode(&respData); err == nil {
			for _, result := range respData.Results {
				for _, ext := range result.Extensions {
					fullName := ext.Publisher.PublisherName + "." + ext.ExtensionName
					var readmeURL string
					if len(ext.Versions) > 0 {
						latestVersion := ext.Versions[0]
						for _, file := range latestVersion.Files {
							if file.AssetType == "Microsoft.VisualStudio.Services.Content.Details" {
								readmeURL = file.Source
								break
							}
						}
					}
					metas[strings.ToLower(fullName)] = marketplaceMeta{
						ShortDescription: ext.ShortDescription,
						ReadmeURL:        readmeURL,
					}
				}
			}
		}
		resp.Body.Close()
	}

	return metas
}

func downloadREADMEs(metas map[string]marketplaceMeta) map[string]string {
	readmes := make(map[string]string)
	if len(metas) == 0 {
		return readmes
	}

	type downloadJob struct {
		extID string
		url   string
	}
	type downloadResult struct {
		extID  string
		readme string
	}

	jobs := make(chan downloadJob, len(metas))
	results := make(chan downloadResult, len(metas))

	// Populate jobs
	for id, meta := range metas {
		if meta.ReadmeURL != "" {
			jobs <- downloadJob{extID: id, url: meta.ReadmeURL}
		}
	}
	close(jobs)

	// Worker pool size (max 8 concurrent downloads)
	const numWorkers = 8
	var wg sync.WaitGroup

	client := &http.Client{Timeout: 5 * time.Second}

	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for job := range jobs {
				resp, err := client.Get(job.url)
				if err != nil {
					continue
				}
				// Limit download to 16KB
				limitReader := io.LimitReader(resp.Body, 16384)
				data, err := io.ReadAll(limitReader)
				resp.Body.Close()
				if err == nil {
					results <- downloadResult{extID: job.extID, readme: string(data)}
				}
			}
		}()
	}

	go func() {
		wg.Wait()
		close(results)
	}()

	for res := range results {
		readmes[res.extID] = res.readme
	}

	return readmes
}
