package api

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// seedPluginCatalog writes a marketplace.json fixture into a temp claudeDir
// (mirroring writeManifest in internal/marketplace/marketplace_test.go), points
// the project-plugins endpoints at it via AttachPluginCatalog, and restores the
// global on cleanup.
func seedPluginCatalog(t *testing.T, body string) {
	t.Helper()
	claudeDir := t.TempDir()
	dir := filepath.Join(claudeDir, "plugins", "marketplaces", "swarmery", ".claude-plugin")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "marketplace.json"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	AttachPluginCatalog(claudeDir)
	t.Cleanup(func() { AttachPluginCatalog("") })
}

const threePackManifest = `{
	"name": "swarmery",
	"metadata": {"version": "1.13.0"},
	"plugins": [
		{"name": "core", "source": "./plugins/core", "description": "the core plugin"},
		{"name": "uav-pack", "source": "./plugins/uav-pack", "description": "UAV domain pack"},
		{"name": "lsp-pack", "source": "./plugins/lsp-pack", "description": "LSP pack"}
	]
}`

func getPluginsResponse(t *testing.T, srvURL, projectID string) projectPluginsResponse {
	t.Helper()
	var resp projectPluginsResponse
	getJSON(t, srvURL+"/api/projects/"+projectID+"/plugins", &resp)
	return resp
}

func TestProjectPluginsMergesCatalogAndState(t *testing.T) {
	srv, _ := projectsTestServer(t)
	seedPluginCatalog(t, threePackManifest)
	// Overwrite the seeded settings: enable core + lsp-pack only.
	writeProjectSettings(t, projectPath(t, srv.URL, "1"), `{
		"enabledPlugins": {"core@swarmery": true, "lsp-pack@swarmery": true}
	}`)

	resp := getPluginsResponse(t, srv.URL, "1")
	if resp.MarketplaceVersion != "1.13.0" {
		t.Errorf("marketplaceVersion = %q, want 1.13.0", resp.MarketplaceVersion)
	}
	want := []projectPluginDTO{
		{Name: "core", Description: "the core plugin", Enabled: true, Locked: true},
		{Name: "uav-pack", Description: "UAV domain pack", Enabled: false, Locked: false},
		{Name: "lsp-pack", Description: "LSP pack", Enabled: true, Locked: false},
	}
	if len(resp.Plugins) != len(want) {
		t.Fatalf("plugins len = %d, want %d (%+v)", len(resp.Plugins), len(want), resp.Plugins)
	}
	for i, w := range want {
		if resp.Plugins[i] != w {
			t.Errorf("plugins[%d] = %+v, want %+v (manifest order)", i, resp.Plugins[i], w)
		}
	}
}

func TestProjectPluginsCanWriteFollowsFence(t *testing.T) {
	srv, _ := projectsTestServer(t)
	seedPluginCatalog(t, threePackManifest)
	path := projectPath(t, srv.URL, "1")
	t.Cleanup(func() { AttachOnboard(OnboardConfig{}) })

	// No onboarding roots → the write fence would reject, canWrite=false.
	AttachOnboard(OnboardConfig{})
	if resp := getPluginsResponse(t, srv.URL, "1"); resp.CanWrite {
		t.Error("canWrite = true without onboarding roots, want false")
	}

	// Project path under an allowed root → canWrite=true.
	AttachOnboard(OnboardConfig{Roots: []string{filepath.Dir(path)}, WorkspaceRoot: t.TempDir()})
	if resp := getPluginsResponse(t, srv.URL, "1"); !resp.CanWrite {
		t.Error("canWrite = false with the project under an onboarding root, want true")
	}
}

func TestProjectPluginsStaleCloneKeepsEnabledPack(t *testing.T) {
	srv, _ := projectsTestServer(t)
	// Stale clone: manifest only knows core.
	seedPluginCatalog(t, `{
		"name": "swarmery",
		"metadata": {"version": "1.13.0"},
		"plugins": [{"name": "core", "source": "./plugins/core", "description": "the core plugin"}]
	}`)
	writeProjectSettings(t, projectPath(t, srv.URL, "1"), `{
		"enabledPlugins": {"core@swarmery": true, "web-pack@swarmery": true}
	}`)

	resp := getPluginsResponse(t, srv.URL, "1")
	if len(resp.Plugins) != 2 {
		t.Fatalf("plugins len = %d, want 2 (%+v)", len(resp.Plugins), resp.Plugins)
	}
	last := resp.Plugins[1]
	if last.Name != "web-pack" || !last.Enabled || last.Locked {
		t.Errorf("appended row = %+v, want web-pack enabled unlocked", last)
	}
	if !strings.Contains(last.Description, "missing from the local marketplace clone") {
		t.Errorf("description = %q, want a stale-clone note", last.Description)
	}
}

func TestProjectPluginsNoMarketplace(t *testing.T) {
	srv, _ := projectsTestServer(t)
	// A claudeDir with no marketplaces/ clone at all.
	AttachPluginCatalog(t.TempDir())
	t.Cleanup(func() { AttachPluginCatalog("") })

	out := doJSON(t, "GET", srv.URL+"/api/projects/1/plugins", nil, 404)
	msg, _ := out["error"].(string)
	if !strings.Contains(msg, "marketplace is not installed") {
		t.Errorf("error = %q, want the marketplace-not-installed message", msg)
	}
}

func TestProjectPluginsUnknownProject(t *testing.T) {
	srv, _ := projectsTestServer(t)
	seedPluginCatalog(t, threePackManifest)

	out := doJSON(t, "GET", srv.URL+"/api/projects/9999/plugins", nil, 404)
	if msg, _ := out["error"].(string); msg != "project not found" {
		t.Errorf("error = %q, want \"project not found\"", msg)
	}
}
