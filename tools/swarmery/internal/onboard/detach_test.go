package onboard

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// managedSettings is a realistic onboarded settings.json plus two user-owned
// extras (a foreign plugin, a hand-added env var) that detach MUST preserve.
const managedSettings = `{
  "enabledPlugins": {"core@swarmery": true, "iot-pack@swarmery": true, "other@elsewhere": true},
  "extraKnownMarketplaces": {
    "swarmery": {"source": {"source": "github", "repo": "atretyak1985/swarmery"}},
    "elsewhere": {"source": {"source": "github", "repo": "someone/else"}}
  },
  "env": {"AGENT_PROJECT": "demo", "AGENT_WORKSPACE_ROOT": "/ws", "MY_OWN": "keep-me"},
  "statusLine": {"type": "command", "command": "bash $CLAUDE_PROJECT_DIR/.claude/statusline/statusline.sh"},
  "permissions": {
    "deny": ["Read(./.env)"],
    "additionalDirectories": ["/ws", "/some/other/dir"]
  }
}`

func writeTestSettings(t *testing.T, dir, body string) string {
	t.Helper()
	claude := filepath.Join(dir, ".claude")
	if err := os.MkdirAll(claude, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(claude, "settings.json")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func readSettings(t *testing.T, path string) map[string]any {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}
	return m
}

func TestDetachPrunesSwarmeryOnly(t *testing.T) {
	dir := t.TempDir()
	path := writeTestSettings(t, dir, managedSettings)

	res, err := Detach(DetachConfig{ProjectDir: dir, Slug: "demo", WorkspaceRoot: "/ws"})
	if err != nil {
		t.Fatalf("detach: %v", err)
	}
	if !res.Detached {
		t.Fatal("want Detached=true")
	}

	s := readSettings(t, path)

	// enabledPlugins: swarmery gone, foreign plugin kept.
	ep := s["enabledPlugins"].(map[string]any)
	if _, ok := ep["core@swarmery"]; ok {
		t.Error("core@swarmery not removed")
	}
	if _, ok := ep["iot-pack@swarmery"]; ok {
		t.Error("iot-pack@swarmery not removed")
	}
	if _, ok := ep["other@elsewhere"]; !ok {
		t.Error("foreign plugin other@elsewhere must be preserved")
	}

	// extraKnownMarketplaces: swarmery gone, elsewhere kept.
	mk := s["extraKnownMarketplaces"].(map[string]any)
	if _, ok := mk["swarmery"]; ok {
		t.Error("extraKnownMarketplaces.swarmery not removed")
	}
	if _, ok := mk["elsewhere"]; !ok {
		t.Error("foreign marketplace must be preserved")
	}

	// env: both AGENT_* gone, user var kept.
	env := s["env"].(map[string]any)
	if _, ok := env["AGENT_PROJECT"]; ok {
		t.Error("env.AGENT_PROJECT not removed")
	}
	if _, ok := env["AGENT_WORKSPACE_ROOT"]; ok {
		t.Error("env.AGENT_WORKSPACE_ROOT not removed")
	}
	if env["MY_OWN"] != "keep-me" {
		t.Error("user env var MY_OWN must be preserved")
	}

	// statusLine removed entirely.
	if _, ok := s["statusLine"]; ok {
		t.Error("swarmery statusLine not removed")
	}

	// permissions: workspace dir dropped, other dir + deny list kept.
	perms := s["permissions"].(map[string]any)
	dirs := perms["additionalDirectories"].([]any)
	if len(dirs) != 1 || dirs[0] != "/some/other/dir" {
		t.Errorf("additionalDirectories = %v, want [/some/other/dir]", dirs)
	}
	if _, ok := perms["deny"]; !ok {
		t.Error("permissions.deny (generic .env protection) must be preserved")
	}
}

func TestDetachWritesBackup(t *testing.T) {
	dir := t.TempDir()
	path := writeTestSettings(t, dir, managedSettings)

	if _, err := Detach(DetachConfig{ProjectDir: dir, Slug: "demo", WorkspaceRoot: "/ws"}); err != nil {
		t.Fatal(err)
	}
	bak, err := os.ReadFile(path + ".bak")
	if err != nil {
		t.Fatalf("backup not written: %v", err)
	}
	if string(bak) != managedSettings {
		t.Error("backup must be a verbatim copy of the pre-change file")
	}
}

func TestDetachIdempotent(t *testing.T) {
	dir := t.TempDir()
	writeTestSettings(t, dir, managedSettings)

	if _, err := Detach(DetachConfig{ProjectDir: dir, Slug: "demo", WorkspaceRoot: "/ws"}); err != nil {
		t.Fatal(err)
	}
	res, err := Detach(DetachConfig{ProjectDir: dir, Slug: "demo", WorkspaceRoot: "/ws"})
	if err != nil {
		t.Fatal(err)
	}
	if res.Detached {
		t.Error("second detach should find nothing to remove (Detached=false)")
	}
}

func TestDetachDryRunTouchesNothing(t *testing.T) {
	dir := t.TempDir()
	path := writeTestSettings(t, dir, managedSettings)

	res, err := Detach(DetachConfig{ProjectDir: dir, Slug: "demo", WorkspaceRoot: "/ws", DryRun: true})
	if err != nil {
		t.Fatal(err)
	}
	if !res.Detached {
		t.Error("dry run should still report Detached=true (there WAS something to remove)")
	}
	// File unchanged.
	raw, _ := os.ReadFile(path)
	if string(raw) != managedSettings {
		t.Error("dry run must not modify settings.json")
	}
	// No backup on dry run.
	if _, err := os.Stat(path + ".bak"); !os.IsNotExist(err) {
		t.Error("dry run must not write a backup")
	}
}

func TestDetachMissingSettingsIsNoop(t *testing.T) {
	res, err := Detach(DetachConfig{ProjectDir: t.TempDir(), Slug: "demo"})
	if err != nil {
		t.Fatalf("missing settings must not error: %v", err)
	}
	if res.Detached {
		t.Error("missing settings → Detached=false")
	}
}

func TestDetachGuardsForeignAgentProject(t *testing.T) {
	dir := t.TempDir()
	// AGENT_PROJECT value differs from the slug we pass → must be preserved.
	writeTestSettings(t, dir, `{"env": {"AGENT_PROJECT": "someone-elses"}}`)

	res, err := Detach(DetachConfig{ProjectDir: dir, Slug: "demo"})
	if err != nil {
		t.Fatal(err)
	}
	if res.Detached {
		t.Error("AGENT_PROJECT with a non-matching value must not be removed")
	}
}
