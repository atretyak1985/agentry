package api

import (
	"database/sql"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/atretyak1985/swarmery/tools/swarmery/internal/store"
)

// toolsServer seeds tool_call events across two projects: Bash calls on the
// main transcript and one inside a debugger sidechain (parented to its
// subagent_start — the attribution the ingester actually stores), a denied
// call on beta, and a Read call outside the default 14-day range.
func toolsServer(t *testing.T) (*httptest.Server, *sql.DB) {
	t.Helper()
	db, err := store.Open(filepath.Join(t.TempDir(), "tools.db"))
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

	// The debugger's subagent_start: sidechain tool events are parented to it.
	mustExec(`INSERT INTO events (id, session_id, ts, type, tool_name, status, duration_ms, payload, dedup_key) VALUES
		(10, 1, ?, 'subagent_start', 'Agent', 'ok', 60000, '{"subagent_type":"core:debugger"}', 'sub1')`, today)

	// Bash: 3 ok on main, 1 error inside the debugger, 1 denied on beta (no duration).
	mustExec(`INSERT INTO events (session_id, ts, type, tool_name, status, duration_ms, parent_event_id, dedup_key) VALUES
		(1, ?, 'tool_call', 'Bash', 'ok',     100,  NULL, 'b1'),
		(1, ?, 'tool_call', 'Bash', 'ok',     200,  NULL, 'b2'),
		(1, ?, 'tool_call', 'Bash', 'ok',     1000, NULL, 'b3'),
		(1, ?, 'tool_call', 'Bash', 'error',  400,  10,   'b4'),
		(2, ?, 'tool_call', 'Bash', 'denied', NULL, NULL, 'b5')`,
		today, today, today, today, today)

	// Read: one in range, one 20 days back (excluded).
	mustExec(`INSERT INTO events (session_id, ts, type, tool_name, status, duration_ms, dedup_key) VALUES
		(1, ?, 'tool_call', 'Read', 'ok', 50, 'r1'),
		(1, ?, 'tool_call', 'Read', 'ok', 70, 'r2')`, today, day20)

	h, err := NewServer(db, false)
	if err != nil {
		t.Fatalf("new server: %v", err)
	}
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	return srv, db
}

func TestStatsTools(t *testing.T) {
	srv, db := toolsServer(t)

	var out toolsDTO
	getJSON(t, srv.URL+"/api/stats/tools", &out)
	byTool := map[string]toolStatDTO{}
	for _, tl := range out.Tools {
		byTool[tl.Tool] = tl
	}

	bash := byTool["Bash"]
	if bash.Calls != 5 || bash.Errors != 1 || bash.Denied != 1 {
		t.Fatalf("Bash = %+v, want 5 calls / 1 error / 1 denied", bash)
	}
	// durations [100,200,400,1000] (denied carried none): avg 425, p95 = ceil(0.95×4)=4th → 1000
	if bash.AvgMs == nil || *bash.AvgMs != 425 {
		t.Errorf("Bash avg = %v, want 425", bash.AvgMs)
	}
	if bash.P95Ms == nil || *bash.P95Ms != 1000 {
		t.Errorf("Bash p95 = %v, want 1000", bash.P95Ms)
	}
	agents := map[string]toolAgentDTO{}
	for _, a := range bash.Agents {
		agents[a.Agent] = a
	}
	if a := agents["main"]; a.Calls != 4 || a.Errors != 0 {
		t.Errorf("main split = %+v, want 4 calls 0 errors", a)
	}
	if a := agents["debugger"]; a.Calls != 1 || a.Errors != 1 { // "core:debugger" folded
		t.Errorf("debugger split = %+v, want 1 call 1 error", a)
	}

	if byTool["Read"].Calls != 1 { // the day20 row is out of range
		t.Errorf("Read calls = %d, want 1", byTool["Read"].Calls)
	}
	if byTool["Agent"].Calls != 1 { // subagent_start counts as an Agent tool call
		t.Errorf("Agent calls = %d, want 1", byTool["Agent"].Calls)
	}
	if out.Tools[0].Tool != "Bash" { // ranked by calls desc
		t.Errorf("first tool = %q, want Bash", out.Tools[0].Tool)
	}

	// ?project= filters by slug (global scope predicate): beta's denied Bash
	// call disappears.
	var alpha toolsDTO
	getJSON(t, srv.URL+"/api/stats/tools?project=-work-alpha", &alpha)
	for _, tl := range alpha.Tools {
		if tl.Tool == "Bash" && (tl.Calls != 4 || tl.Denied != 0) {
			t.Errorf("filtered Bash = %+v, want 4 calls 0 denied", tl)
		}
	}

	// approx honesty: no rollup in range → false; a daily_rollups row inside
	// the range (pruned history the query cannot see) → true.
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
	var rolled toolsDTO
	getJSON(t, srv.URL+"/api/stats/tools", &rolled)
	if !rolled.Approx {
		t.Error("approx = false over a range overlapping a rolled-up day, want true")
	}

	// agent option list: full attributed set regardless of any filter.
	wantAgents := map[string]bool{"main": true, "debugger": true}
	got := map[string]bool{}
	for _, a := range out.Agents {
		got[a] = true
	}
	for a := range wantAgents {
		if !got[a] {
			t.Errorf("agents %v missing %q", out.Agents, a)
		}
	}
	if len(out.Agents) == 0 || out.Agents[0] != "main" {
		t.Errorf("agents[0] = %v, want main first", out.Agents)
	}

	// ?agent= narrows every row + column to that agent's events. The debugger
	// ran exactly one Bash call (the error inside its sidechain).
	var dbg toolsDTO
	getJSON(t, srv.URL+"/api/stats/tools?agent=debugger", &dbg)
	byToolDbg := map[string]toolStatDTO{}
	for _, tl := range dbg.Tools {
		byToolDbg[tl.Tool] = tl
	}
	if b := byToolDbg["Bash"]; b.Calls != 1 || b.Errors != 1 {
		t.Errorf("agent=debugger Bash = %+v, want 1 call 1 error", b)
	}
	if _, ok := byToolDbg["Read"]; ok {
		t.Error("agent=debugger should exclude Read (main-attributed)")
	}
	// The full agent list is still returned under a filter (dropdown stays populated).
	if len(dbg.Agents) < 2 {
		t.Errorf("filtered agents = %v, want the full set", dbg.Agents)
	}
}
