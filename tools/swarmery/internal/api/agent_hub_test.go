package api

import (
	"database/sql"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/atretyak1985/swarmery/tools/swarmery/internal/store"
)

// agentHubServer seeds a corpus exercising every Agent Hub aggregation edge:
//
//   - TWO registry agents: id=1 stored plugin-qualified "core:tech-lead" (the
//     registry-id ↔ normalised-name BRIDGE case — its runs are recorded as bare
//     "tech-lead"), and id=2 "debugger" (a run, no turns).
//   - subagent_start runs across two projects (one archived-scoping case) + a
//     subagent_stop error, so runs/activity/failedShare have data.
//   - subagent turns in both notations ("core:tech-lead" + "tech-lead") folding
//     to one $ figure, plus a NULL-agent "main" turn that must NOT surface.
//   - a judged session (outcome) so successRate is non-null.
//   - a board task + delegation ledger rows naming the agent (Tasks tab).
//   - a recommendation (target_kind=agent), a change proposal, and a lesson
//     attributable via the ledger (Insights tab).
//
// Numbers are chosen so the roster row can be asserted against the Retro
// scorecard for the same agent+window (parity by shared helper).
func agentHubServer(t *testing.T) (*httptest.Server, *sql.DB) {
	t.Helper()
	db, err := store.Open(filepath.Join(t.TempDir(), "hub.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	today := retroDay(t, 1) // inside a 30d window, not today's boundary
	older := retroDay(t, 5) // a different local day for the sparkline
	ancient := retroDay(t, 400)

	exec := func(q string, args ...any) {
		t.Helper()
		if _, err := db.Exec(q, args...); err != nil {
			t.Fatalf("exec: %v\n%s", err, q)
		}
	}

	// Two projects; alpha active, beta archived (scope-exclusion case).
	exec(`INSERT INTO projects (id, path, slug, name, first_seen, archived) VALUES
		(1, '/work/alpha', 'alpha', 'Alpha', ?, 0),
		(2, '/work/beta',  'beta',  'Beta',  ?, 1)`, older, older)
	exec(`INSERT INTO sessions (id, project_id, session_uuid, title, status, started_at, outcome) VALUES
		(1, 1, 'u1', 'Sess one', 'completed', ?, 'success'),
		(2, 1, 'u2', 'Sess two', 'completed', ?, 'fail'),
		(3, 2, 'u3', 'Archived', 'completed', ?, NULL)`, today, older, today)

	// Turns: tech-lead 4.0/$200 over both notations; main 0.5 (must not appear
	// as an agent); debugger has no turns.
	exec(`INSERT INTO turns (id, session_id, seq, role, started_at, tokens_in, tokens_out, cost_usd, agent_name) VALUES
		(1, 1, 0, 'assistant', ?, 10, 50,  0.5, NULL),
		(2, 1, 1, 'assistant', ?, 10, 120, 2.5, 'core:tech-lead'),
		(3, 1, 2, 'assistant', ?, 10, 80,  1.5, 'tech-lead'),
		(4, 2, 0, 'assistant', ?, 10, 20,  0.5, 'tech-lead')`, today, today, today, older)

	// Runs (subagent_start): tech-lead ×2 today (a1 ok, a2 error) + ×1 older,
	// debugger ×1 today; one archived-project run that scope must drop.
	exec(`INSERT INTO events (session_id, ts, type, status, payload, duration_ms, dedup_key) VALUES
		(1, ?, 'subagent_start', 'ok',    '{"subagent_type":"core:tech-lead","description":"plan it"}', 1000, 'a1'),
		(1, ?, 'subagent_start', 'error', '{"subagent_type":"tech-lead","description":"broke"}',        3000, 'a2'),
		(2, ?, 'subagent_start', 'ok',    '{"subagent_type":"tech-lead","description":"older run"}',    2000, 'a3'),
		(1, ?, 'subagent_start', 'ok',    '{"subagent_type":"debugger","description":"rca"}',           500,  'a4'),
		(3, ?, 'subagent_start', 'ok',    '{"subagent_type":"tech-lead","description":"in archived"}',  700,  'a5')`,
		today, today, older, today, today)

	// A subagent_stop error naming its own agentType → a behavior-failed run for
	// tech-lead (Activity + failedShare).
	exec(`INSERT INTO events (session_id, parent_event_id, ts, type, tool_name, status, payload, dedup_key) VALUES
		(1, 1, ?, 'tool_call',     'Bash',  'error', '{"result":"boom"}', 'e1'),
		(1, 2, ?, 'subagent_stop', 'Agent', 'error', '{"agentType":"core:tech-lead","status":"failed"}', 'e2')`,
		today, today)

	// Board task + delegation ledger (Tasks tab). Agent stored lowercased.
	exec(`INSERT INTO tasks (id, project_id, title, prompt, status, created_at, started_at, source, external_id) VALUES
		(1, 1, 'Ship the hub', 'goal', 'done', ?, ?, 'queue', '2026-07-24-hub')`, today, today)
	exec(`INSERT INTO task_delegations (task_id, seq, agent, phase, verdict, artifact) VALUES
		(1, 1, 'tech-lead', '4', 'OK', 'phases/04.md'),
		(1, 2, 'tech-lead', '5', 'RE-DISPATCH', 'phases/05.md')`)

	// Registry rows: id=1 plugin-qualified, id=2 bare. A deleted row must be
	// excluded from the roster.
	exec(`INSERT INTO agents (id, name, scope, project_id, file_path, model, origin, plugin_name, description, current_version_id, deleted) VALUES
		(1, 'core:tech-lead', 'global', NULL, '/plugins/core/agents/tech-lead.md', 'opus', 'plugin', 'core', 'Orchestrator', 1, 0),
		(2, 'debugger',       'global', NULL, '/agents/debugger.md',                'sonnet','local', NULL,   'RCA',          NULL, 0),
		(3, 'ghost',          'global', NULL, '/agents/ghost.md',                   NULL,    'local', NULL,   NULL,           NULL, 1)`)
	exec(`INSERT INTO agent_versions (id, agent_id, content_hash, content, created_at) VALUES
		(1, 1, 'h1', '---\nname: tech-lead\n---\nbody', ?)`, today)

	// Insights: a recommendation (agent target), a proposal, and a lesson via
	// the ledger. dedup_key + updated_at are required.
	exec(`INSERT INTO recommendations (id, rule, target_kind, target, title, detail, evidence, status, dedup_key, created_at, updated_at) VALUES
		(1, 'R2', 'agent', 'tech-lead', 'High failed-run share', 'detail', '{"session_ids":["u1"]}', 'proposed', 'R2:tech-lead', ?, ?),
		(2, 'R1', 'tool',  'Bash',      'Add auto-approve',      'detail', '{}',                     'proposed', 'R1:Bash',      ?, ?)`,
		today, today, today, today)
	exec(`INSERT INTO agent_change_proposals (id, agent, agent_path, base_sha256, diff, rationale, status, created_at) VALUES
		(1, 'tech-lead', '/plugins/core/agents/tech-lead.md', 'sha', '@@ diff @@', 'why', 'proposed', ?)`, today)
	exec(`INSERT INTO task_retros (id, task_id, ingested_at) VALUES (1, 1, ?)`, today)
	exec(`INSERT INTO retro_lessons (id, retro_id, seq, title, body, action) VALUES
		(1, 1, 1, 'Read before edit', 'body', 'always Read first')`)

	// Ancient row that a 30d window must ignore entirely.
	exec(`INSERT INTO sessions (id, project_id, session_uuid, status, started_at) VALUES (9, 1, 'u9', 'completed', ?)`, ancient)
	exec(`INSERT INTO events (session_id, ts, type, status, payload, dedup_key) VALUES
		(9, ?, 'subagent_start', 'ok', '{"subagent_type":"tech-lead"}', 'aOld')`, ancient)

	h, err := NewServer(db, false)
	if err != nil {
		t.Fatalf("new server: %v", err)
	}
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	return srv, db
}

func findRoster(rows []agentRosterRow, name string) *agentRosterRow {
	for i := range rows {
		if rows[i].Name == name {
			return &rows[i]
		}
	}
	return nil
}

func TestAgentsHubRoster(t *testing.T) {
	srv, _ := agentHubServer(t)
	var out agentRosterDTO
	getJSON(t, srv.URL+"/api/agents/hub", &out)

	t.Run("lists live registry agents, excludes deleted", func(t *testing.T) {
		if len(out.Agents) != 2 {
			t.Fatalf("roster = %d agents, want 2 (%+v)", len(out.Agents), out.Agents)
		}
		if findRoster(out.Agents, "ghost") != nil {
			t.Errorf("deleted agent 'ghost' leaked into the roster")
		}
	})

	t.Run("registry-id ↔ normalised-name bridge (plugin-qualified)", func(t *testing.T) {
		// id=1 is stored "core:tech-lead" but its runs/turns are recorded bare
		// "tech-lead"; the rollup must still attach via normAgentType.
		tl := findRoster(out.Agents, "core:tech-lead")
		if tl == nil {
			t.Fatalf("core:tech-lead missing from roster")
		}
		// Runs today+older (a1,a2,a3) = 3; the archived a5 and the ancient aOld
		// must be excluded.
		if tl.Runs30d != 3 {
			t.Errorf("tech-lead runs30d = %d, want 3 (archived + ancient excluded)", tl.Runs30d)
		}
		if tl.Cost30d != 4.5 {
			t.Errorf("tech-lead cost30d = %v, want 4.5 (2.5+1.5+0.5, main's 0.5 excluded)", tl.Cost30d)
		}
		// Judged sessions: u1 success, u2 fail → 0.5.
		if tl.SuccessRate == nil || *tl.SuccessRate != 0.5 {
			t.Errorf("tech-lead successRate = %v, want 0.5", tl.SuccessRate)
		}
		// Two behavior-failed runs of 3 → ~0.667: a1 (its Bash tool_call e1
		// errored) and a2 (its subagent_stop e2 failed). e1 marks a1's RUN
		// failed even though a1's start status is 'ok' — the same behavior-
		// failed-run share the Retro scorecard reports (asserted for parity in
		// TestAgentsHubParityWithRetro).
		if !almostEq(tl.FailedShare, 2.0/3.0) {
			t.Errorf("tech-lead failedShare = %v, want 2/3", tl.FailedShare)
		}
		if tl.LastActiveAt == nil || *tl.LastActiveAt == "" {
			t.Errorf("tech-lead lastActiveAt is empty")
		}
		if tl.Model == nil || *tl.Model != "opus" {
			t.Errorf("tech-lead model = %v, want opus", tl.Model)
		}
		if tl.Origin != "plugin" || tl.PluginName == nil || *tl.PluginName != "core" {
			t.Errorf("tech-lead origin/plugin = %v/%v, want plugin/core", tl.Origin, tl.PluginName)
		}
	})

	t.Run("agent with a run but no turns still lists (zeros for $)", func(t *testing.T) {
		dbg := findRoster(out.Agents, "debugger")
		if dbg == nil {
			t.Fatalf("debugger missing from roster")
		}
		if dbg.Runs30d != 1 {
			t.Errorf("debugger runs30d = %d, want 1", dbg.Runs30d)
		}
		if dbg.Cost30d != 0 {
			t.Errorf("debugger cost30d = %v, want 0 (no turns)", dbg.Cost30d)
		}
	})

	t.Run("busiest agent sorts first", func(t *testing.T) {
		if out.Agents[0].Name != "core:tech-lead" {
			t.Errorf("first roster row = %q, want core:tech-lead (3 runs > 1)", out.Agents[0].Name)
		}
	})
}

// TestAgentsHubParityWithRetro pins the structural guarantee: because the roster
// and the Retro scorecard call the SAME retroAgentWindow + agentOutcomeRates
// helpers over the same window, their per-agent numbers agree.
func TestAgentsHubParityWithRetro(t *testing.T) {
	srv, _ := agentHubServer(t)

	var roster agentRosterDTO
	getJSON(t, srv.URL+"/api/agents/hub", &roster)
	var retro retroAgentsDTO
	getJSON(t, srv.URL+"/api/retro/agents?"+retroRange(30), &retro)

	tlRoster := findRoster(roster.Agents, "core:tech-lead")
	if tlRoster == nil {
		t.Fatal("tech-lead missing from roster")
	}
	var tlRetro *retroAgentDTO
	for i := range retro.Agents {
		if retro.Agents[i].Agent == "tech-lead" {
			tlRetro = &retro.Agents[i]
		}
	}
	if tlRetro == nil {
		t.Fatal("tech-lead missing from retro scorecards")
	}
	if tlRoster.Runs30d != tlRetro.Runs {
		t.Errorf("runs mismatch: roster %d vs retro %d", tlRoster.Runs30d, tlRetro.Runs)
	}
	if tlRoster.Cost30d != tlRetro.CostUSD {
		t.Errorf("cost mismatch: roster %v vs retro %v", tlRoster.Cost30d, tlRetro.CostUSD)
	}
	if !almostEq(tlRoster.FailedShare, tlRetro.ErrorRate) {
		t.Errorf("failedShare mismatch: roster %v vs retro error_rate %v", tlRoster.FailedShare, tlRetro.ErrorRate)
	}
	if (tlRoster.SuccessRate == nil) != (tlRetro.SuccessRate == nil) {
		t.Fatalf("successRate nullability mismatch: roster %v vs retro %v", tlRoster.SuccessRate, tlRetro.SuccessRate)
	}
	if tlRoster.SuccessRate != nil && *tlRoster.SuccessRate != *tlRetro.SuccessRate {
		t.Errorf("successRate mismatch: roster %v vs retro %v", *tlRoster.SuccessRate, *tlRetro.SuccessRate)
	}
}

func TestAgentsHubRosterProjectScope(t *testing.T) {
	srv, _ := agentHubServer(t)
	// Scope to beta (archived) → its runs are still archived-excluded by the
	// helper, so tech-lead shows zero runs but is STILL listed (registry-whole).
	var out agentRosterDTO
	getJSON(t, srv.URL+"/api/agents/hub?projectId=alpha", &out)
	tl := findRoster(out.Agents, "core:tech-lead")
	if tl == nil {
		t.Fatalf("tech-lead missing under alpha scope")
	}
	// Under alpha, a1/a2 (session 1) count; a3 is session 2 which is also alpha,
	// so runs stay 3. The point: scoping is accepted and does not 500.
	if tl.Runs30d == 0 {
		t.Errorf("tech-lead runs30d = 0 under alpha scope, want > 0")
	}

	// A bogus project id yields the registry rows with zero rollups, never 500.
	var bogus agentRosterDTO
	getJSON(t, srv.URL+"/api/agents/hub?projectId=does-not-exist", &bogus)
	if len(bogus.Agents) != 2 {
		t.Errorf("bogus scope roster = %d, want 2 registry rows", len(bogus.Agents))
	}
	if b := findRoster(bogus.Agents, "core:tech-lead"); b == nil || b.Runs30d != 0 {
		t.Errorf("bogus scope tech-lead = %+v, want runs30d 0", b)
	}
}

func TestAgentHubProfile(t *testing.T) {
	srv, _ := agentHubServer(t)
	var out agentProfileDTO
	getJSON(t, srv.URL+"/api/agents/1/hub", &out)

	t.Run("identity + overview", func(t *testing.T) {
		if out.Name != "core:tech-lead" || out.ID != 1 {
			t.Errorf("identity = %d/%q, want 1/core:tech-lead", out.ID, out.Name)
		}
		if out.Overview.Runs30d != 3 {
			t.Errorf("overview runs30d = %d, want 3", out.Overview.Runs30d)
		}
		if out.Overview.Cost30d != 4.5 {
			t.Errorf("overview cost30d = %v, want 4.5", out.Overview.Cost30d)
		}
		if len(out.Overview.RunsByDay) != hubWindowDays {
			t.Errorf("runsByDay = %d buckets, want %d (zero-filled window)", len(out.Overview.RunsByDay), hubWindowDays)
		}
		var spark int64
		for _, d := range out.Overview.RunsByDay {
			spark += d.Runs
		}
		if spark != 3 {
			t.Errorf("runsByDay total = %d, want 3", spark)
		}
	})

	t.Run("runs tab lists real sessions with cost/duration", func(t *testing.T) {
		if len(out.Runs) != 3 {
			t.Fatalf("runs = %d, want 3 (%+v)", len(out.Runs), out.Runs)
		}
		// Newest first; every run carries a session uuid + status.
		for _, r := range out.Runs {
			if r.SessionUUID == "" {
				t.Errorf("run missing sessionUuid: %+v", r)
			}
		}
		if out.Runs[0].Ts < out.Runs[len(out.Runs)-1].Ts {
			t.Errorf("runs not newest-first")
		}
	})

	t.Run("activity tab shows agent-attributed events", func(t *testing.T) {
		if len(out.Activity) == 0 {
			t.Fatalf("activity empty, want subagent_start/stop + parented tool errors")
		}
		// The subagent_stop error (e2) must be attributed to tech-lead.
		var sawStop bool
		for _, a := range out.Activity {
			if a.Type == "subagent_stop" {
				sawStop = true
				if a.Status == nil || *a.Status != "error" {
					t.Errorf("subagent_stop status = %v, want error", a.Status)
				}
			}
		}
		if !sawStop {
			t.Errorf("activity missing the subagent_stop event")
		}
	})

	t.Run("tasks tab shows delegation ledger rows", func(t *testing.T) {
		if len(out.Tasks) != 2 {
			t.Fatalf("tasks = %d, want 2 delegation rows (%+v)", len(out.Tasks), out.Tasks)
		}
		for _, tk := range out.Tasks {
			if tk.Source != "delegation" || tk.ExternalID != "2026-07-24-hub" {
				t.Errorf("task row = %+v, want delegation on 2026-07-24-hub", tk)
			}
		}
	})

	t.Run("insights tab filters to the agent", func(t *testing.T) {
		if len(out.Insights.Recommendations) != 1 {
			t.Errorf("recommendations = %d, want 1 (agent target only; Bash tool rec excluded)", len(out.Insights.Recommendations))
		}
		if len(out.Insights.Recommendations) == 1 && out.Insights.Recommendations[0].Rule != "R2" {
			t.Errorf("recommendation rule = %q, want R2", out.Insights.Recommendations[0].Rule)
		}
		if len(out.Insights.Proposals) != 1 {
			t.Errorf("proposals = %d, want 1", len(out.Insights.Proposals))
		}
		if len(out.Insights.Lessons) != 1 {
			t.Errorf("lessons = %d, want 1 (via the ledger)", len(out.Insights.Lessons))
		}
	})
}

func TestAgentHubProfileUnknown404(t *testing.T) {
	srv, _ := agentHubServer(t)

	res, err := http.Get(srv.URL + "/api/agents/999/hub")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusNotFound {
		t.Errorf("unknown agent status = %d, want 404", res.StatusCode)
	}

	// A soft-deleted agent (id=3 'ghost') is also a 404 — not a live registry row.
	res2, err := http.Get(srv.URL + "/api/agents/3/hub")
	if err != nil {
		t.Fatalf("get deleted: %v", err)
	}
	defer res2.Body.Close()
	if res2.StatusCode != http.StatusNotFound {
		t.Errorf("deleted agent status = %d, want 404", res2.StatusCode)
	}

	// A non-numeric id is a 400 (systemItemID contract).
	res3, err := http.Get(srv.URL + "/api/agents/not-a-number/hub")
	if err != nil {
		t.Fatalf("get non-numeric: %v", err)
	}
	defer res3.Body.Close()
	if res3.StatusCode != http.StatusBadRequest {
		t.Errorf("non-numeric id status = %d, want 400", res3.StatusCode)
	}
}

// TestAgentHubAddsNoWritePaths asserts the phase invariant: the Hub added
// exactly two GET routes and NO mutating route. The daemon mounts a SPA
// catch-all (server.go: mux.Handle("/", spaHandler)) that answers ANY unclaimed
// path/method with index.html (200, text/html) — so a real API write handler is
// distinguished from the fallback by its CONTENT TYPE + JSON body, not the
// status code. Here the two GETs must return JSON, and every write method on the
// hub paths must fall through to the HTML SPA (i.e. no JSON API handler claims
// it) — the robust proxy for "no write route exists".
func TestAgentHubAddsNoWritePaths(t *testing.T) {
	srv, _ := agentHubServer(t)

	isJSON := func(res *http.Response) bool {
		return strings.HasPrefix(res.Header.Get("Content-Type"), "application/json")
	}

	// The two GETs are real JSON API routes.
	for _, path := range []string{"/api/agents/hub", "/api/agents/1/hub"} {
		res, err := http.Get(srv.URL + path)
		if err != nil {
			t.Fatalf("GET %s: %v", path, err)
		}
		gotJSON := isJSON(res)
		res.Body.Close()
		if res.StatusCode != http.StatusOK || !gotJSON {
			t.Errorf("GET %s = %d %q, want 200 application/json", path, res.StatusCode, res.Header.Get("Content-Type"))
		}
	}

	// Every write method on the hub paths must NOT be claimed by a JSON API
	// handler — it falls through to the SPA (text/html), proving no write route
	// was registered for these paths.
	for _, method := range []string{http.MethodPost, http.MethodPut, http.MethodDelete, http.MethodPatch} {
		for _, path := range []string{"/api/agents/hub", "/api/agents/1/hub"} {
			req, err := http.NewRequest(method, srv.URL+path, nil)
			if err != nil {
				t.Fatalf("new %s %s: %v", method, path, err)
			}
			res, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatalf("%s %s: %v", method, path, err)
			}
			claimed := isJSON(res)
			res.Body.Close()
			if claimed {
				t.Errorf("%s %s hit a JSON API handler — the Hub must expose NO write path", method, path)
			}
		}
	}
}
