package api

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// attachTestServer is projectsTestServer plus the WorkspaceRoot the attach
// endpoint requires (detach only falls back to it, so the shared fixture
// leaves it empty).
func attachTestServer(t *testing.T) (srvURL, projectDir string) {
	t.Helper()
	srv, _ := projectsTestServer(t)
	projectDir = projectPath(t, srv.URL, "1")
	AttachOnboard(OnboardConfig{
		Roots:         onboardCfg.Roots,
		WorkspaceRoot: t.TempDir(),
	})
	return srv.URL, projectDir
}

// Detach project 1 in full, then attach brings it back: settings merged,
// project.json restored from the backup, hooks reinstalled.
func TestAttachProjectHappyPath(t *testing.T) {
	srvURL, path := attachTestServer(t)
	claude := filepath.Join(path, ".claude")
	if err := os.WriteFile(filepath.Join(claude, "project.json"), []byte(`{"name": "managed"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	doJSON(t, http.MethodPost, srvURL+"/api/projects/1/detach?full=1", nil, http.StatusOK)
	if body := readDisk(t, filepath.Join(claude, "settings.json")); strings.Contains(body, "core@swarmery") {
		t.Fatal("fixture: detach did not strip settings.json")
	}

	out := doJSON(t, http.MethodPost, srvURL+"/api/projects/1/attach", nil, http.StatusOK)
	if out["attached"] != true {
		t.Errorf("attached = %v, want true (steps: %v)", out["attached"], out["steps"])
	}
	if out["backup"] != ".claude/settings.json.bak" {
		t.Errorf("backup = %v, want .claude/settings.json.bak", out["backup"])
	}

	settings := readDisk(t, filepath.Join(claude, "settings.json"))
	if !strings.Contains(settings, "core@swarmery") {
		t.Error("settings.json does not re-enable core@swarmery")
	}
	if body := readDisk(t, filepath.Join(claude, "project.json")); !strings.Contains(body, "managed") {
		t.Error("project.json not restored from its backup")
	}
	if body := readDisk(t, filepath.Join(claude, "settings.local.json")); !strings.Contains(body, "swarmery hook") {
		t.Error("swarmery hooks not installed in settings.local.json")
	}
}

func TestAttachProjectDryRun(t *testing.T) {
	srvURL, path := attachTestServer(t)
	claude := filepath.Join(path, ".claude")
	doJSON(t, http.MethodPost, srvURL+"/api/projects/1/detach?full=1", nil, http.StatusOK)
	settings := filepath.Join(claude, "settings.json")
	before := readDisk(t, settings)

	out := doJSON(t, http.MethodPost, srvURL+"/api/projects/1/attach?dryRun=1", nil, http.StatusOK)
	if out["attached"] != true || out["dryRun"] != true {
		t.Errorf("dry run body = %v, want attached=true dryRun=true", out)
	}
	if _, ok := out["backup"]; ok {
		t.Errorf("dry run must not report a backup, got %v", out["backup"])
	}
	if readDisk(t, settings) != before {
		t.Error("dry run modified settings.json on disk")
	}
	if _, err := os.Stat(filepath.Join(claude, "settings.local.json")); !os.IsNotExist(err) {
		t.Error("dry run must not install hooks")
	}
}

func TestAttachProjectDisabledWhenNoRoots(t *testing.T) {
	srv, _ := projectsTestServer(t)
	AttachOnboard(OnboardConfig{})
	doJSON(t, http.MethodPost, srv.URL+"/api/projects/1/attach", nil, http.StatusForbidden)
}

func TestAttachProjectNeedsWorkspaceRoot(t *testing.T) {
	srv, _ := projectsTestServer(t)
	// The shared fixture sets roots but no workspace root → clear 403, not a 500.
	doJSON(t, http.MethodPost, srv.URL+"/api/projects/1/attach", nil, http.StatusForbidden)
}

func TestAttachProjectOutsideRoots(t *testing.T) {
	srvURL, _ := attachTestServer(t)
	// Project 2's path is NOT under the onboarding root → fenced.
	doJSON(t, http.MethodPost, srvURL+"/api/projects/2/attach", nil, http.StatusForbidden)
}

func TestAttachProjectNotFound(t *testing.T) {
	srvURL, _ := attachTestServer(t)
	doJSON(t, http.MethodPost, srvURL+"/api/projects/9999/attach", nil, http.StatusNotFound)
}

func TestAttachProjectRejectsForeignOrigin(t *testing.T) {
	srvURL, _ := attachTestServer(t)
	req, err := http.NewRequest(http.MethodPost, srvURL+"/api/projects/1/attach", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Origin", "https://evil.example.com")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("cross-origin attach = %d, want 403", resp.StatusCode)
	}
}
