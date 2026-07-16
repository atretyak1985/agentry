package projectscan

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

// writeSettings writes a .claude/settings.json under dir with body.
func writeSettings(t *testing.T, dir, body string) {
	t.Helper()
	claude := filepath.Join(dir, ".claude")
	if err := os.MkdirAll(claude, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(claude, "settings.json"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestPluginState_Managed(t *testing.T) {
	dir := t.TempDir()
	writeSettings(t, dir, `{
		"extraKnownMarketplaces": {"swarmery": {"source": {"source": "github", "repo": "atretyak1985/swarmery"}}},
		"enabledPlugins": {"core@swarmery": true, "iot-pack@swarmery": true, "web-pack@swarmery": false, "other@elsewhere": true}
	}`)

	st, err := ReadPluginState(dir, []string{filepath.Dir(dir)})
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if st == nil {
		t.Fatal("want non-nil state")
	}
	if !st.Managed {
		t.Error("want Managed=true")
	}
	if !reflect.DeepEqual(st.Packs, []string{"iot-pack"}) {
		t.Errorf("packs = %v, want [iot-pack] (disabled + non-swarmery excluded)", st.Packs)
	}
	if st.Marketplace != "atretyak1985/swarmery" {
		t.Errorf("marketplace = %q", st.Marketplace)
	}
	if !st.UnderOnboardRoot {
		t.Error("want UnderOnboardRoot=true (project is directly under the root)")
	}
}

func TestPluginState_NotManaged_NoSwarmery(t *testing.T) {
	dir := t.TempDir()
	writeSettings(t, dir, `{"enabledPlugins": {"other@elsewhere": true}}`)

	st, err := ReadPluginState(dir, nil)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if st == nil {
		t.Fatal("settings present but no swarmery → non-nil, Managed=false")
	}
	if st.Managed || len(st.Packs) != 0 {
		t.Errorf("want unmanaged empty, got %+v", st)
	}
	if st.UnderOnboardRoot {
		t.Error("nil roots → UnderOnboardRoot must be false")
	}
}

func TestPluginState_NoSettings(t *testing.T) {
	st, err := ReadPluginState(t.TempDir(), nil)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if st != nil {
		t.Errorf("missing settings.json → nil state, got %+v", st)
	}
}

func TestPluginState_MalformedJSON(t *testing.T) {
	dir := t.TempDir()
	writeSettings(t, dir, `{not valid json`)

	st, err := ReadPluginState(dir, nil)
	if err != nil {
		t.Fatalf("malformed settings must not error the list: %v", err)
	}
	if st != nil {
		t.Errorf("malformed settings → nil state, got %+v", st)
	}
}

func TestPluginState_OutsideRoot(t *testing.T) {
	dir := t.TempDir()
	writeSettings(t, dir, `{"enabledPlugins": {"core@swarmery": true}}`)

	st, err := ReadPluginState(dir, []string{"/some/unrelated/root"})
	if err != nil {
		t.Fatal(err)
	}
	if st.UnderOnboardRoot {
		t.Error("project not under the root → UnderOnboardRoot=false")
	}
}

func TestComponents(t *testing.T) {
	dir := t.TempDir()
	claude := filepath.Join(dir, ".claude")
	mkdir := func(p string) {
		if err := os.MkdirAll(filepath.Join(claude, p), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	write := func(p string) {
		if err := os.WriteFile(filepath.Join(claude, p), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	mkdir("agents")
	write("agents/reviewer.md")
	write("agents/planner.md")
	write("agents/notes.txt") // ignored: not .md
	mkdir("skills/deploy")     // skill = directory
	mkdir("skills/lint")
	mkdir("commands")
	write("commands/ship.md")
	mkdir("hooks")
	write("hooks/pretooluse.sh")

	c, err := ReadComponents(dir)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if c.Counts != (ComponentCounts{Agents: 2, Skills: 2, Commands: 1, Hooks: 1}) {
		t.Errorf("counts = %+v", c.Counts)
	}
	// sorted, extension stripped, source tagged local
	if len(c.Agents) != 2 || c.Agents[0].Name != "planner" || c.Agents[0].Source != "local" {
		t.Errorf("agents = %+v", c.Agents)
	}
	if c.Skills[0].Name != "deploy" {
		t.Errorf("skills = %+v", c.Skills)
	}
}

func TestComponents_MissingDirs(t *testing.T) {
	c, err := ReadComponents(t.TempDir())
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	// non-nil empty slices so JSON renders [] not null
	if c.Agents == nil || c.Skills == nil || c.Commands == nil || c.Hooks == nil {
		t.Errorf("want empty non-nil slices, got %+v", c)
	}
	if c.Counts != (ComponentCounts{}) {
		t.Errorf("counts = %+v, want all zero", c.Counts)
	}
}
