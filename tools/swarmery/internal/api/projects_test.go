package api

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/atretyak1985/swarmery/tools/swarmery/internal/store"
)

// projectsTestServer seeds two projects — a swarmery-managed one (with a real
// .claude/settings.json on disk, under an onboarding root) carrying two priced
// turns, and a telemetry-only one (no .claude) — plus an archived project. It
// wires onboardCfg.Roots to the temp parent so UnderOnboardRoot is exercised,
// and restores the global afterwards.
func projectsTestServer(t *testing.T) (*httptest.Server, *sql.DB) {
	t.Helper()
	db, err := store.Open(filepath.Join(t.TempDir(), "projects.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	root := t.TempDir()

	// Managed project: id 1, path under root, real settings.json enabling core.
	managedPath := filepath.Join(root, "managed")
	writeProjectSettings(t, managedPath, `{
		"extraKnownMarketplaces": {"swarmery": {"source": {"source": "github", "repo": "atretyak1985/swarmery"}}},
		"enabledPlugins": {"core@swarmery": true, "iot-pack@swarmery": true}
	}`)
	// One local agent so the detail component inventory is non-empty.
	if err := os.WriteFile(filepath.Join(managedPath, ".claude", "agents", "reviewer.md"), []byte("---\nname: reviewer\n---\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	execSQL(t, db, `INSERT INTO projects (id, path, slug, name, first_seen, last_activity, archived)
		VALUES (1, ?, 'managed', 'Managed', '2026-07-10T00:00:00Z', '2026-07-14T00:00:00Z', 0)`, managedPath)
	execSQL(t, db, `INSERT INTO sessions (id, project_id, session_uuid, status, started_at, source, model, title)
		VALUES (10, 1, 'u-managed-1', 'completed', '2026-07-14T00:00:00Z', 'jsonl', 'opus', 'first')`)
	execSQL(t, db, `INSERT INTO turns (session_id, seq, role, started_at, message_id, tokens_in, tokens_out, cost_usd)
		VALUES (10, 1, 'assistant', '2026-07-14T00:00:00Z', 'm1', 100, 50, 0.30)`)
	execSQL(t, db, `INSERT INTO turns (session_id, seq, role, started_at, message_id, tokens_in, tokens_out, cost_usd)
		VALUES (10, 2, 'assistant', '2026-07-14T00:01:00Z', 'm2', 200, 100, 0.70)`)
	// A hidden session must NOT appear in the detail recent list.
	execSQL(t, db, `INSERT INTO sessions (id, project_id, session_uuid, status, started_at, source, hidden)
		VALUES (11, 1, 'u-managed-hidden', 'completed', '2026-07-14T02:00:00Z', 'jsonl', 1)`)

	// Telemetry-only project: id 2, path with no .claude on disk.
	execSQL(t, db, `INSERT INTO projects (id, path, slug, name, first_seen, last_activity, archived)
		VALUES (2, '/tmp/telemetry-only-nonexistent', 'telemetry', 'Telemetry', '2026-07-11T00:00:00Z', '2026-07-13T00:00:00Z', 0)`)

	// Archived project: id 3 — hidden from the default list.
	execSQL(t, db, `INSERT INTO projects (id, path, slug, name, first_seen, last_activity, archived)
		VALUES (3, '/tmp/archived', 'archived', 'Archived', '2026-07-09T00:00:00Z', '2026-07-12T00:00:00Z', 1)`)

	prev := onboardCfg
	AttachOnboard(OnboardConfig{Roots: []string{root}})
	t.Cleanup(func() { onboardCfg = prev })

	h, err := NewServer(db, false)
	if err != nil {
		t.Fatalf("new server: %v", err)
	}
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	return srv, db
}

func writeProjectSettings(t *testing.T, projectDir, body string) {
	t.Helper()
	claude := filepath.Join(projectDir, ".claude")
	if err := os.MkdirAll(filepath.Join(claude, "agents"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(claude, "settings.json"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func execSQL(t *testing.T, db *sql.DB, q string, args ...any) {
	t.Helper()
	if _, err := db.Exec(q, args...); err != nil {
		t.Fatalf("exec: %v\nquery: %s", err, q)
	}
}

func getProjectsList(t *testing.T, url string) []projectDTO {
	t.Helper()
	resp, err := http.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET %s = %d, want 200", url, resp.StatusCode)
	}
	var list []projectDTO
	if err := json.NewDecoder(resp.Body).Decode(&list); err != nil {
		t.Fatalf("decode: %v", err)
	}
	return list
}

func TestListProjectsExcludesArchivedAndEnriches(t *testing.T) {
	srv, _ := projectsTestServer(t)
	list := getProjectsList(t, srv.URL+"/api/projects")

	if len(list) != 2 {
		t.Fatalf("default list len = %d, want 2 (archived hidden)", len(list))
	}
	byID := map[int64]projectDTO{}
	for _, p := range list {
		byID[p.ID] = p
	}

	managed := byID[1]
	if managed.Plugin == nil || !managed.Plugin.Managed {
		t.Errorf("project 1 plugin = %+v, want managed", managed.Plugin)
	}
	if managed.Plugin != nil && !managed.Plugin.UnderOnboardRoot {
		t.Error("project 1 should be under the onboarding root")
	}
	if managed.Tokens == nil || *managed.Tokens != 450 {
		t.Errorf("project 1 tokens = %v, want 450 (100+50+200+100)", managed.Tokens)
	}
	if managed.CostUSD == nil || *managed.CostUSD < 0.999 || *managed.CostUSD > 1.001 {
		t.Errorf("project 1 cost = %v, want ~1.00", managed.CostUSD)
	}

	telemetry := byID[2]
	if telemetry.Plugin != nil {
		t.Errorf("project 2 (no .claude) plugin = %+v, want nil", telemetry.Plugin)
	}
	if telemetry.Tokens != nil {
		t.Errorf("project 2 tokens = %v, want nil (no turns)", telemetry.Tokens)
	}
}

func TestListProjectsIncludeArchived(t *testing.T) {
	srv, _ := projectsTestServer(t)
	list := getProjectsList(t, srv.URL+"/api/projects?include=archived")
	if len(list) != 3 {
		t.Fatalf("include=archived list len = %d, want 3", len(list))
	}
}

func TestGetProjectDetail(t *testing.T) {
	srv, _ := projectsTestServer(t)
	resp, err := http.Get(srv.URL + "/api/projects/1")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	var d projectDetailDTO
	if err := json.NewDecoder(resp.Body).Decode(&d); err != nil {
		t.Fatalf("decode: %v", err)
	}

	if d.Project.ID != 1 || d.Project.Plugin == nil || !d.Project.Plugin.Managed {
		t.Errorf("detail project = %+v", d.Project)
	}
	if d.Components == nil || d.Components.Counts.Agents != 1 {
		t.Errorf("components = %+v, want 1 local agent", d.Components)
	}
	if d.Stats.Sessions != 2 {
		t.Errorf("stats.sessions = %d, want 2 (count includes the hidden session; only recent list excludes it)", d.Stats.Sessions)
	}
	// recent excludes the hidden session (10 shown, 11 hidden).
	if len(d.Stats.RecentSessions) != 1 || d.Stats.RecentSessions[0].ID != 10 {
		t.Errorf("recentSessions = %+v, want [session 10]", d.Stats.RecentSessions)
	}
}

func TestGetProjectNotFound(t *testing.T) {
	srv, _ := projectsTestServer(t)
	resp, err := http.Get(srv.URL + "/api/projects/9999")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("unknown id = %d, want 404", resp.StatusCode)
	}
}

func TestGetProjectBadID(t *testing.T) {
	srv, _ := projectsTestServer(t)
	resp, err := http.Get(srv.URL + "/api/projects/not-a-number")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("bad id = %d, want 400", resp.StatusCode)
	}
}

func TestListProjectsPinnedFirstWithTags(t *testing.T) {
	srv, db := projectsTestServer(t)
	// Project 2 has the OLDER last_activity but is pinned + tagged — it must lead.
	execSQL(t, db, `UPDATE projects SET pinned = 1, tags = '["billing","infra"]' WHERE id = 2`)

	list := getProjectsList(t, srv.URL+"/api/projects")
	if len(list) != 2 {
		t.Fatalf("list len = %d, want 2", len(list))
	}
	if list[0].ID != 2 {
		t.Errorf("first project = %d, want pinned project 2", list[0].ID)
	}
	if !list[0].Pinned {
		t.Error("project 2 should report pinned=true")
	}
	if len(list[0].Tags) != 2 || list[0].Tags[0] != "billing" || list[0].Tags[1] != "infra" {
		t.Errorf("project 2 tags = %v, want [billing infra]", list[0].Tags)
	}
	// Untagged projects must serialize tags as [], never null.
	if list[1].Tags == nil {
		t.Error("untagged project must have tags = [], not null")
	}
	if list[1].Pinned {
		t.Error("project 1 should report pinned=false")
	}
}
