package api

// phase: projects — GET /api/projects/{id}/plugins merges the swarmery
// marketplace catalog (the clone under ~/.claude/plugins/marketplaces/swarmery,
// read via internal/marketplace) with the project's enabledPlugins state
// (projectscan.ReadPluginState). Read-only and unfenced; the canWrite flag
// tells the UI whether the PUT fence (step 03, same file) would admit a write.

import (
	"database/sql"
	"errors"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strconv"

	"github.com/atretyak1985/swarmery/tools/swarmery/internal/marketplace"
	"github.com/atretyak1985/swarmery/tools/swarmery/internal/projectscan"
)

// pluginMarketplace is the only marketplace this surface manages — matches
// projectscan's marketplaceSuffix ("@swarmery") view of enabledPlugins.
const pluginMarketplace = "swarmery"

// pluginCatalogDir is attached once at startup (or per-test); empty ⇒ resolve
// ~/.claude at request time. Mirrors AttachOnboard (onboard.go:41).
var pluginCatalogDir string

// AttachPluginCatalog points the project-plugins endpoints at the directory
// holding plugins/marketplaces/ (production: ~/.claude; tests: a temp dir).
func AttachPluginCatalog(claudeDir string) { pluginCatalogDir = claudeDir }

func catalogDir() (string, error) {
	if pluginCatalogDir != "" {
		return pluginCatalogDir, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".claude"), nil
}

type projectPluginDTO struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Enabled     bool   `json:"enabled"`
	// Locked marks plugins this surface refuses to toggle: core's lifecycle is
	// attach/detach (hooks + statusline + project.json travel with it).
	Locked bool `json:"locked"`
}

type projectPluginsResponse struct {
	MarketplaceVersion string             `json:"marketplaceVersion"`
	CanWrite           bool               `json:"canWrite"`
	Plugins            []projectPluginDTO `json:"plugins"`
}

// projectPlugins handles GET /api/projects/{id}/plugins.
func (h *Handler) projectPlugins(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.Error(w, `{"error":"invalid project id"}`, http.StatusBadRequest)
		return
	}
	var path string
	err = h.DB.QueryRow(`SELECT path FROM projects WHERE id = ?`, id).Scan(&path)
	if errors.Is(err, sql.ErrNoRows) {
		http.Error(w, `{"error":"project not found"}`, http.StatusNotFound)
		return
	}
	if err != nil {
		writeErr(w, err)
		return
	}

	cdir, err := catalogDir()
	if err != nil {
		writeErr(w, err)
		return
	}
	cat, err := marketplace.Read(cdir, pluginMarketplace)
	if errors.Is(err, fs.ErrNotExist) {
		writeJSONStatus(w, http.StatusNotFound, map[string]string{
			"error": "swarmery marketplace is not installed on this machine — run a Claude Code marketplace update",
		})
		return
	}
	if err != nil {
		writeErr(w, err)
		return
	}

	// Enabled state: Managed covers core, Packs the domain packs. A nil state
	// (telemetry-only project, unreadable settings) renders everything off.
	enabledCore, enabledPacks := false, []string{}
	if st, serr := projectscan.ReadPluginState(path, nil); serr == nil && st != nil {
		enabledCore = st.Managed
		enabledPacks = st.Packs
	}

	// canWrite mirrors the attach/detach fence (attach.go:42-87): roots must be
	// configured AND the project path must resolve under one of them.
	canWrite := false
	if len(onboardCfg.Roots) > 0 {
		if _, ferr := resolveUnderRoots(path, onboardCfg.Roots); ferr == nil {
			canWrite = true
		}
	}

	resp := projectPluginsResponse{MarketplaceVersion: cat.Version, CanWrite: canWrite, Plugins: []projectPluginDTO{}}
	seen := map[string]bool{}
	for _, p := range cat.Plugins {
		seen[p.Name] = true
		enabled := p.Name == "core" && enabledCore || slices.Contains(enabledPacks, p.Name)
		resp.Plugins = append(resp.Plugins, projectPluginDTO{
			Name: p.Name, Description: p.Description,
			Enabled: enabled, Locked: p.Name == "core",
		})
	}
	// Enabled-but-unknown packs (stale clone) must stay visible.
	for _, name := range enabledPacks {
		if !seen[name] {
			resp.Plugins = append(resp.Plugins, projectPluginDTO{
				Name:        name,
				Description: "(enabled here, but missing from the local marketplace clone — refresh marketplaces)",
				Enabled:     true,
			})
		}
	}
	writeJSON(w, resp, nil)
}
