package api

// GET /api/projects/health tests. The fixture uses RELATIVE timestamps
// (time.Now-based) because the endpoint's windows are rolling 7/14-day spans.

import (
	"fmt"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/atretyak1985/swarmery/tools/swarmery/internal/store"
)

// healthTestServer seeds:
//   - alpha (id 1): this week — one 30-minute session with a $1.50 turn and
//     3 tool_calls (1 error); prev week — one session with a $3.00 turn.
//   - zed (id 2): archived — must not appear.
//   - quiet (id 3): no activity in either window — all health fields null.
func healthTestServer(t *testing.T) *httptest.Server {
	t.Helper()
	db, err := store.Open(filepath.Join(t.TempDir(), "health.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	now := time.Now().UTC()
	ts := func(ago time.Duration) string { return now.Add(-ago).Format(time.RFC3339) }
	mustExec := func(q string, args ...any) {
		t.Helper()
		if _, err := db.Exec(q, args...); err != nil {
			t.Fatalf("exec: %v\n%s", err, q)
		}
	}

	mustExec(`INSERT INTO projects (id, path, slug, name, first_seen, last_activity, archived) VALUES
		(1, '/tmp/alpha', 'alpha', 'Alpha', ?, ?, 0),
		(2, '/tmp/zed',   'zed',   'Zed',   ?, ?, 1),
		(3, '/tmp/quiet', 'quiet', 'Quiet', ?, ?, 0)`,
		ts(20*24*time.Hour), ts(2*time.Hour),
		ts(20*24*time.Hour), ts(3*time.Hour),
		ts(30*24*time.Hour), ts(20*24*time.Hour))

	// This week: 30-minute session (started 24h ago, ended 23.5h ago).
	mustExec(`INSERT INTO sessions (id, project_id, session_uuid, status, started_at, ended_at, source) VALUES
		(10, 1, 'u-a-1', 'completed', ?, ?, 'jsonl')`,
		ts(24*time.Hour), ts(24*time.Hour-30*time.Minute))
	mustExec(`INSERT INTO turns (session_id, seq, role, started_at, message_id, cost_usd) VALUES
		(10, 1, 'assistant', ?, 'm1', 1.5)`, ts(24*time.Hour))
	for i, status := range []string{"ok", "ok", "error"} {
		mustExec(`INSERT INTO events (session_id, ts, type, tool_name, status, dedup_key) VALUES
			(10, ?, 'tool_call', 'Bash', ?, ?)`, ts(24*time.Hour), status, fmt.Sprintf("e-a-%d", i))
	}

	// Previous week: one session, one $3.00 turn (10 days ago).
	mustExec(`INSERT INTO sessions (id, project_id, session_uuid, status, started_at, ended_at, source) VALUES
		(11, 1, 'u-a-2', 'completed', ?, ?, 'jsonl')`,
		ts(10*24*time.Hour), ts(10*24*time.Hour-time.Hour))
	mustExec(`INSERT INTO turns (session_id, seq, role, started_at, message_id, cost_usd) VALUES
		(11, 1, 'assistant', ?, 'm2', 3.0)`, ts(10*24*time.Hour))

	h, err := NewServer(db, false)
	if err != nil {
		t.Fatalf("new server: %v", err)
	}
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	return srv
}

func TestProjectsHealth(t *testing.T) {
	srv := healthTestServer(t)

	var rows []projectHealthDTO
	getJSON(t, srv.URL+"/api/projects/health", &rows)

	if len(rows) != 2 {
		t.Fatalf("rows = %d, want 2 (archived zed excluded)", len(rows))
	}
	byslug := map[string]projectHealthDTO{}
	for _, r := range rows {
		byslug[r.Slug] = r
	}

	alpha := byslug["alpha"]
	if alpha.CostWeekUSD == nil || *alpha.CostWeekUSD < 1.499 || *alpha.CostWeekUSD > 1.501 {
		t.Errorf("alpha costWeekUsd = %v, want ~1.50", alpha.CostWeekUSD)
	}
	if alpha.CostPrevWeekUSD == nil || *alpha.CostPrevWeekUSD < 2.999 || *alpha.CostPrevWeekUSD > 3.001 {
		t.Errorf("alpha costPrevWeekUsd = %v, want ~3.00", alpha.CostPrevWeekUSD)
	}
	if alpha.ErrorRate == nil || *alpha.ErrorRate < 0.333 || *alpha.ErrorRate > 0.334 {
		t.Errorf("alpha errorRate = %v, want ~0.333 (1 of 3 tool_calls)", alpha.ErrorRate)
	}
	// One ended session this week: 30 minutes = 1_800_000 ms (±2s slack).
	if alpha.AvgSessionMs == nil || *alpha.AvgSessionMs < 1_798_000 || *alpha.AvgSessionMs > 1_802_000 {
		t.Errorf("alpha avgSessionMs = %v, want ~1800000", alpha.AvgSessionMs)
	}
	if alpha.LastActivity == nil {
		t.Error("alpha lastActivity = nil, want set")
	}
	if alpha.Tags == nil {
		t.Error("alpha tags must be [], not null")
	}

	quiet := byslug["quiet"]
	if quiet.CostWeekUSD != nil || quiet.CostPrevWeekUSD != nil ||
		quiet.ErrorRate != nil || quiet.AvgSessionMs != nil {
		t.Errorf("quiet health fields = %+v, want all null (no activity)", quiet)
	}
}
