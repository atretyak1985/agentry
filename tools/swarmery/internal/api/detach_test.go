package api

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// projectPath fetches a seeded project's on-disk path via the detail endpoint.
func projectPath(t *testing.T, srv, projectID string) string {
	t.Helper()
	var d projectDetailDTO
	getJSON(t, srv+"/api/projects/"+projectID, &d)
	return d.Project.Path
}

func TestDetachProjectHappyPath(t *testing.T) {
	srv, _ := projectsTestServer(t)
	path := projectPath(t, srv.URL, "1")
	settings := filepath.Join(path, ".claude", "settings.json")

	out := doJSON(t, http.MethodPost, srv.URL+"/api/projects/1/detach", nil, http.StatusOK)
	if out["detached"] != true {
		t.Errorf("detached = %v, want true", out["detached"])
	}
	if out["backup"] != ".claude/settings.json.bak" {
		t.Errorf("backup = %v, want .claude/settings.json.bak", out["backup"])
	}

	// The swarmery entries are gone from the file on disk…
	if body := readDisk(t, settings); strings.Contains(body, "core@swarmery") {
		t.Error("settings.json still contains core@swarmery after detach")
	}
	// …and the backup preserves the pre-change file.
	if _, err := os.Stat(settings + ".bak"); err != nil {
		t.Errorf("backup not written: %v", err)
	}
}

func TestDetachProjectDryRun(t *testing.T) {
	srv, _ := projectsTestServer(t)
	path := projectPath(t, srv.URL, "1")
	settings := filepath.Join(path, ".claude", "settings.json")
	before := readDisk(t, settings)

	out := doJSON(t, http.MethodPost, srv.URL+"/api/projects/1/detach?dryRun=1", nil, http.StatusOK)
	if out["detached"] != true || out["dryRun"] != true {
		t.Errorf("dry run body = %v, want detached=true dryRun=true", out)
	}
	if _, ok := out["backup"]; ok {
		t.Errorf("dry run must not report a backup, got %v", out["backup"])
	}
	if readDisk(t, settings) != before {
		t.Error("dry run modified settings.json on disk")
	}
	if _, err := os.Stat(settings + ".bak"); !os.IsNotExist(err) {
		t.Error("dry run wrote a backup")
	}
}

func TestDetachProjectDisabledWhenNoRoots(t *testing.T) {
	srv, _ := projectsTestServer(t)
	// Disable the endpoint by clearing the roots (the server-helper cleanup
	// restores the original global afterwards).
	AttachOnboard(OnboardConfig{})
	doJSON(t, http.MethodPost, srv.URL+"/api/projects/1/detach", nil, http.StatusForbidden)
}

func TestDetachProjectOutsideRoots(t *testing.T) {
	srv, _ := projectsTestServer(t)
	// Project 2's path (/tmp/telemetry-only-nonexistent) is NOT under the
	// onboarding root → the symlink-safe fence rejects it before any write.
	doJSON(t, http.MethodPost, srv.URL+"/api/projects/2/detach", nil, http.StatusForbidden)
}

func TestDetachProjectNotFound(t *testing.T) {
	srv, _ := projectsTestServer(t)
	doJSON(t, http.MethodPost, srv.URL+"/api/projects/9999/detach", nil, http.StatusNotFound)
}

func TestDetachProjectRejectsForeignOrigin(t *testing.T) {
	srv, _ := projectsTestServer(t)
	req, err := http.NewRequest(http.MethodPost, srv.URL+"/api/projects/1/detach", nil)
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
		t.Errorf("cross-origin detach = %d, want 403", resp.StatusCode)
	}
}
