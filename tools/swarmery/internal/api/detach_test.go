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

// Full offboard (?full=1): onboarding artifacts (project.json, statusline
// scripts) and the settings.local.json swarmery hooks all go; user files stay.
func TestDetachProjectFull(t *testing.T) {
	srv, _ := projectsTestServer(t)
	path := projectPath(t, srv.URL, "1")
	claude := filepath.Join(path, ".claude")
	pj := filepath.Join(claude, "project.json")
	if err := os.WriteFile(pj, []byte(`{"name": "managed"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	sl := filepath.Join(claude, "statusline", "statusline.sh")
	if err := os.MkdirAll(filepath.Dir(sl), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(sl, []byte("#!/bin/bash\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	local := filepath.Join(claude, "settings.local.json")
	if err := os.WriteFile(local, []byte(`{"hooks": {"Stop": [{"hooks": [
		{"type": "command", "command": "/home/u/.swarmery/bin/swarmery hook stop"}]}]},
		"permissions": {"allow": ["Bash(ls:*)"]}}`), 0o644); err != nil {
		t.Fatal(err)
	}

	// Dry run lists the artifacts and hooks, removes nothing.
	out := doJSON(t, http.MethodPost, srv.URL+"/api/projects/1/detach?dryRun=1&full=1", nil, http.StatusOK)
	if out["detached"] != true {
		t.Fatalf("dry run detached = %v, want true", out["detached"])
	}
	if _, err := os.Stat(pj); err != nil {
		t.Fatal("dry run removed project.json")
	}
	if body := readDisk(t, local); !strings.Contains(body, "swarmery hook stop") {
		t.Fatal("dry run modified settings.local.json")
	}

	out = doJSON(t, http.MethodPost, srv.URL+"/api/projects/1/detach?full=1", nil, http.StatusOK)
	if out["detached"] != true {
		t.Fatalf("detached = %v, want true", out["detached"])
	}
	if _, err := os.Stat(pj); !os.IsNotExist(err) {
		t.Error("project.json not removed")
	}
	if _, err := os.Stat(sl); !os.IsNotExist(err) {
		t.Error("statusline script not removed")
	}
	body := readDisk(t, local)
	if strings.Contains(body, "swarmery hook") {
		t.Error("settings.local.json still carries swarmery hooks")
	}
	if !strings.Contains(body, "Bash(ls:*)") {
		t.Error("user permissions in settings.local.json must survive")
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
