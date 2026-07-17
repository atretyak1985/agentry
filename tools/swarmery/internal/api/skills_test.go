package api

import (
	"database/sql"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/atretyak1985/swarmery/tools/swarmery/internal/store"
)

// skillsServer seeds skill_use events across two projects: brainstorming runs
// on the main transcript plus one inside a debugger sidechain (parented to its
// subagent_start), a denied run on beta, and a run outside the 14-day range.
func skillsServer(t *testing.T) (*httptest.Server, *sql.DB) {
	t.Helper()
	db, err := store.Open(filepath.Join(t.TempDir(), "skills.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	const tsFmt = "2006-01-02T15:04:05.000Z"
	now := time.Now()
	todayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	today := todayStart.Add(12 * time.Hour).UTC().Format(tsFmt)
	day20 := todayStart.AddDate(0, 0, -20).Add(12 * time.Hour).UTC().Format(tsFmt)

	mustExec := func(q string, args ...any) {
		t.Helper()
		if _, err := db.Exec(q, args...); err != nil {
			t.Fatalf("exec: %v\n%s", err, q)
		}
	}

	mustExec(`INSERT INTO projects (id, path, slug, name, first_seen) VALUES
		(1, '/work/alpha', '-work-alpha', 'Alpha', ?),
		(2, '/work/beta',  '-work-beta',  NULL,    ?)`, day20, day20)
	mustExec(`INSERT INTO sessions (id, project_id, session_uuid, model, status, started_at) VALUES
		(1, 1, 'u1', 'claude-fable-5', 'active',    ?),
		(2, 2, 'u2', 'claude-fable-5', 'completed', ?)`, today, today)

	// The debugger's subagent_start: sidechain skill events are parented to it.
	mustExec(`INSERT INTO events (id, session_id, ts, type, tool_name, status, duration_ms, payload, dedup_key) VALUES
		(10, 1, ?, 'subagent_start', 'Agent', 'ok', 60000, '{"subagent_type":"core:debugger"}', 'sub1')`, today)

	// brainstorming: 3 ok on main, 1 error inside the debugger, 1 denied on beta (no duration).
	mustExec(`INSERT INTO events (session_id, ts, type, status, duration_ms, parent_event_id, payload, dedup_key) VALUES
		(1, ?, 'skill_use', 'ok',     100,  NULL, '{"input":{"skill":"brainstorming"}}', 'k1'),
		(1, ?, 'skill_use', 'ok',     200,  NULL, '{"input":{"skill":"brainstorming"}}', 'k2'),
		(1, ?, 'skill_use', 'ok',     1000, NULL, '{"input":{"skill":"brainstorming"}}', 'k3'),
		(1, ?, 'skill_use', 'error',  400,  10,   '{"input":{"skill":"brainstorming"}}', 'k4'),
		(2, ?, 'skill_use', 'denied', NULL, NULL, '{"input":{"skill":"brainstorming"}}', 'k5')`,
		today, today, today, today, today)

	// systematic-debugging: one in range, one 20 days back (excluded).
	mustExec(`INSERT INTO events (session_id, ts, type, status, duration_ms, payload, dedup_key) VALUES
		(1, ?, 'skill_use', 'ok', 50, '{"input":{"skill":"systematic-debugging"}}', 's1'),
		(1, ?, 'skill_use', 'ok', 70, '{"input":{"skill":"systematic-debugging"}}', 's2')`, today, day20)

	h, err := NewServer(db, false)
	if err != nil {
		t.Fatalf("new server: %v", err)
	}
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	return srv, db
}

func TestStatsSkills(t *testing.T) {
	srv, db := skillsServer(t)

	var out skillsDTO
	getJSON(t, srv.URL+"/api/stats/skills", &out)
	bySkill := map[string]skillStatDTO{}
	for _, s := range out.Skills {
		bySkill[s.Skill] = s
	}

	brain := bySkill["brainstorming"]
	if brain.Calls != 5 || brain.Errors != 1 || brain.Denied != 1 {
		t.Fatalf("brainstorming = %+v, want 5 calls / 1 error / 1 denied", brain)
	}
	// durations [100,200,400,1000] (denied carried none): avg 425, p95 = 4th → 1000
	if brain.AvgMs == nil || *brain.AvgMs != 425 {
		t.Errorf("brainstorming avg = %v, want 425", brain.AvgMs)
	}
	if brain.P95Ms == nil || *brain.P95Ms != 1000 {
		t.Errorf("brainstorming p95 = %v, want 1000", brain.P95Ms)
	}
	agents := map[string]toolAgentDTO{}
	for _, a := range brain.Agents {
		agents[a.Agent] = a
	}
	if a := agents["main"]; a.Calls != 4 || a.Errors != 0 {
		t.Errorf("main split = %+v, want 4 calls 0 errors", a)
	}
	if a := agents["debugger"]; a.Calls != 1 || a.Errors != 1 { // "core:debugger" folded
		t.Errorf("debugger split = %+v, want 1 call 1 error", a)
	}

	if bySkill["systematic-debugging"].Calls != 1 { // the day20 row is out of range
		t.Errorf("systematic-debugging calls = %d, want 1", bySkill["systematic-debugging"].Calls)
	}
	if out.Skills[0].Skill != "brainstorming" { // ranked by calls desc
		t.Errorf("first skill = %q, want brainstorming", out.Skills[0].Skill)
	}

	// ?project= filters by slug: beta's denied run disappears.
	var alpha skillsDTO
	getJSON(t, srv.URL+"/api/stats/skills?project=-work-alpha", &alpha)
	for _, s := range alpha.Skills {
		if s.Skill == "brainstorming" && (s.Calls != 4 || s.Denied != 0) {
			t.Errorf("filtered brainstorming = %+v, want 4 calls 0 denied", s)
		}
	}

	// approx honesty: no rollup in range → false; a daily_rollups row inside the
	// range → true.
	if out.Approx {
		t.Error("approx = true with no rolled-up days, want false")
	}
	prunedDay := time.Now().AddDate(0, 0, -5).Format("2006-01-02")
	if _, err := db.Exec(`INSERT INTO daily_rollups
		(day, project_id, agent_id, sessions, tasks_done, tasks_reverted,
		 tool_calls, errors, tokens_in, tokens_out, cost_usd, wait_minutes)
		VALUES (?, 1, NULL, 3, 0, 0, 40, 2, 1000, 400, 2.0, 0)`, prunedDay); err != nil {
		t.Fatalf("seed rollup: %v", err)
	}
	var rolled skillsDTO
	getJSON(t, srv.URL+"/api/stats/skills", &rolled)
	if !rolled.Approx {
		t.Error("approx = false over a range overlapping a rolled-up day, want true")
	}
}
