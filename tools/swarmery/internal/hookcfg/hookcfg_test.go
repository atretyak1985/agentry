package hookcfg

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func testSystem(t *testing.T) (*System, string) {
	t.Helper()
	home := t.TempDir()
	project := t.TempDir()
	return &System{Home: home, Out: &bytes.Buffer{}}, project
}

func readFile(t *testing.T, path string) []byte {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return b
}

// foreignSettings is a realistic settings.local.json with unknown keys and a
// foreign hook that MUST survive install/uninstall byte-for-byte.
const foreignSettings = `{
  "hooks": {
    "PermissionRequest": [
      {
        "hooks": [
          {
            "command": "my-custom-audit.sh",
            "type": "command"
          }
        ],
        "matcher": "Bash"
      }
    ],
    "PostToolUse": [
      {
        "hooks": [
          {
            "command": "format-on-save.sh",
            "type": "command"
          }
        ],
        "matcher": "Edit|Write"
      }
    ]
  },
  "permissions": {
    "allow": [
      "Bash(npm test)"
    ]
  },
  "unknownFutureKey": {
    "nested": [
      1,
      2,
      3
    ]
  }
}
`

func TestInstallCreatesSettings(t *testing.T) {
	sys, project := testSystem(t)
	if err := sys.Install(project, 0); err != nil {
		t.Fatal(err)
	}
	raw := readFile(t, SettingsPath(project))
	var root map[string]any
	if err := json.Unmarshal(raw, &root); err != nil {
		t.Fatalf("written settings are not valid JSON: %v", err)
	}
	s := string(raw)
	wantBin := sys.InstalledBin()
	for _, want := range []string{
		wantBin + " hook permission-request",
		wantBin + " hook stop",
		`"matcher": "*"`,
		`"timeout": 130`,
	} {
		if !strings.Contains(s, want) {
			t.Errorf("settings missing %q:\n%s", want, s)
		}
	}
	// No .bak for a file we created from scratch.
	if _, err := os.Stat(SettingsPath(project) + ".bak"); !os.IsNotExist(err) {
		t.Error(".bak must not be created when the settings file did not exist")
	}
	if sys.Inspect(project, 0) != StateInstalled {
		t.Errorf("Inspect = %q, want installed", sys.Inspect(project, 0))
	}
}

// TestInstallIdempotent: the second run writes nothing (no diff).
func TestInstallIdempotent(t *testing.T) {
	sys, project := testSystem(t)
	if err := sys.Install(project, 0); err != nil {
		t.Fatal(err)
	}
	first := readFile(t, SettingsPath(project))
	fi1, _ := os.Stat(SettingsPath(project))

	var out bytes.Buffer
	sys.Out = &out
	if err := sys.Install(project, 0); err != nil {
		t.Fatal(err)
	}
	second := readFile(t, SettingsPath(project))
	if !bytes.Equal(first, second) {
		t.Errorf("second install changed the file:\n--- first\n%s\n--- second\n%s", first, second)
	}
	if !strings.Contains(out.String(), "already installed") {
		t.Errorf("second run output = %q, want 'already installed'", out.String())
	}
	fi2, _ := os.Stat(SettingsPath(project))
	if !fi1.ModTime().Equal(fi2.ModTime()) {
		t.Error("second install rewrote the file (mtime changed)")
	}
}

// TestInstallPreservesForeignSettings + uninstall removes ONLY ours.
func TestInstallUninstallRoundTrip(t *testing.T) {
	sys, project := testSystem(t)
	path := SettingsPath(project)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(foreignSettings), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := sys.Install(project, 0); err != nil {
		t.Fatal(err)
	}
	installed := string(readFile(t, path))
	for _, foreign := range []string{"my-custom-audit.sh", "format-on-save.sh",
		"unknownFutureKey", "Bash(npm test)", `"matcher": "Bash"`} {
		if !strings.Contains(installed, foreign) {
			t.Errorf("install dropped foreign content %q", foreign)
		}
	}
	if !strings.Contains(installed, "swarmery hook permission-request") {
		t.Error("install did not add the swarmery entry")
	}
	// .bak preserves the pre-swarmery original.
	if got := readFile(t, path+".bak"); string(got) != foreignSettings {
		t.Errorf(".bak = %q, want the original", got)
	}

	if err := sys.Uninstall(project); err != nil {
		t.Fatal(err)
	}
	after := readFile(t, path)
	if strings.Contains(string(after), "swarmery") {
		t.Errorf("uninstall left swarmery entries:\n%s", after)
	}
	if string(after) != foreignSettings {
		t.Errorf("uninstall must restore the foreign settings byte-for-byte:\n--- want\n%s\n--- got\n%s",
			foreignSettings, after)
	}
	if sys.Inspect(project, 0) != StateNotInstalled {
		t.Errorf("Inspect after uninstall = %q", sys.Inspect(project, 0))
	}
}

// TestBrokenJSONAborts: parse-fail = abort with message, file untouched,
// no .bak.
func TestBrokenJSONAborts(t *testing.T) {
	sys, project := testSystem(t)
	path := SettingsPath(project)
	os.MkdirAll(filepath.Dir(path), 0o755)
	broken := []byte(`{"hooks": [unclosed`)
	os.WriteFile(path, broken, 0o644)

	err := sys.Install(project, 0)
	if err == nil || !strings.Contains(err.Error(), "not valid JSON") {
		t.Fatalf("install on broken JSON: err = %v, want parse abort", err)
	}
	if got := readFile(t, path); !bytes.Equal(got, broken) {
		t.Error("broken file was modified")
	}
	if _, err := os.Stat(path + ".bak"); !os.IsNotExist(err) {
		t.Error("no .bak may be written on abort")
	}
	if err := sys.Uninstall(project); err == nil {
		t.Error("uninstall on broken JSON must abort too")
	}
}

// TestUninstallWhenNotInstalled: no file / foreign-only file → no-op.
func TestUninstallWhenNotInstalled(t *testing.T) {
	sys, project := testSystem(t)
	if err := sys.Uninstall(project); err != nil {
		t.Fatalf("uninstall without settings file: %v", err)
	}
	if _, err := os.Stat(SettingsPath(project)); !os.IsNotExist(err) {
		t.Error("uninstall must not create the settings file")
	}

	path := SettingsPath(project)
	os.MkdirAll(filepath.Dir(path), 0o755)
	os.WriteFile(path, []byte(foreignSettings), 0o644)
	if err := sys.Uninstall(project); err != nil {
		t.Fatal(err)
	}
	if got := readFile(t, path); string(got) != foreignSettings {
		t.Error("uninstall touched a file that has no swarmery entries")
	}
	if _, err := os.Stat(path + ".bak"); !os.IsNotExist(err) {
		t.Error("no-op uninstall must not write a .bak")
	}
}

// TestStaleDetectionAndRefresh: a moved binary path is reported stale and
// healed by re-install.
func TestStaleDetectionAndRefresh(t *testing.T) {
	sys, project := testSystem(t)
	if err := sys.Install(project, 0); err != nil {
		t.Fatal(err)
	}

	// Simulate an old install pointing at a different home.
	otherSys := &System{Home: filepath.Join(sys.Home, "elsewhere"), Out: &bytes.Buffer{}}
	if got := otherSys.Inspect(project, 0); got != StateStale {
		t.Errorf("Inspect with moved bin = %q, want stale", got)
	}

	if err := otherSys.Install(project, 0); err != nil {
		t.Fatal(err)
	}
	if got := otherSys.Inspect(project, 0); got != StateInstalled {
		t.Errorf("Inspect after refresh = %q, want installed", got)
	}
	raw := string(readFile(t, SettingsPath(project)))
	if strings.Contains(raw, sys.InstalledBin()+" hook") {
		t.Error("refresh left the stale binary path behind")
	}
	if c := strings.Count(raw, "hook permission-request"); c != 1 {
		t.Errorf("permission-request entries = %d, want exactly 1", c)
	}
}

// TestPortBaking: --port embeds SWARMERY_PORT into the commands.
func TestPortBaking(t *testing.T) {
	sys, project := testSystem(t)
	if err := sys.Install(project, 7799); err != nil {
		t.Fatal(err)
	}
	raw := string(readFile(t, SettingsPath(project)))
	if !strings.Contains(raw, "SWARMERY_PORT=7799 "+sys.InstalledBin()+" hook permission-request") {
		t.Errorf("port not baked into command:\n%s", raw)
	}
	if got := sys.Inspect(project, 7799); got != StateInstalled {
		t.Errorf("Inspect with port = %q, want installed", got)
	}
	if got := sys.Inspect(project, 0); got != StateStale {
		t.Errorf("Inspect with default port against port-baked install = %q, want stale", got)
	}
}
