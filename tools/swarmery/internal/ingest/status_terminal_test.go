package ingest

// A 'killed' session is terminal: the operator ended it via the kill endpoint
// (prockill.go sets status='killed' + proc_state='dead' and documents the
// invariant "procwatch/ingest never revert a 'killed' row"). A killed Claude
// process commonly flushes its final JSONL lines *after* death; tailing them
// must NOT re-upsert the session back to a live status — otherwise the card
// shows a green "active" dot next to a "dead"/"exited" process badge.

import (
	"path/filepath"
	"testing"
)

func TestUpsertPreservesKilledStatus(t *testing.T) {
	db := testDB(t)
	path := filepath.Join(fixtures, "simple-session.jsonl")

	if _, err := db.Exec(
		`INSERT INTO projects (path, slug, name, first_seen)
		 VALUES ('/tmp/demo', '-tmp-demo', 'demo', '2026-07-13T00:00:00.000Z')`); err != nil {
		t.Fatal(err)
	}
	// The kill endpoint's terminal state: status='killed', process dead.
	if _, err := db.Exec(
		`INSERT INTO sessions (project_id, session_uuid, cwd, status, proc_state, started_at, source)
		 VALUES (1, '0f1e2d3c-4b5a-4968-8776-655443322110', '/tmp/demo', 'killed', 'dead',
		         '2026-07-13T00:00:00.000Z', 'both')`); err != nil {
		t.Fatal(err)
	}

	// Death-rattle JSONL flush arrives and gets tailed.
	if _, err := File(db, path); err != nil {
		t.Fatalf("ingest: %v", err)
	}

	var status string
	if err := db.QueryRow(
		`SELECT status FROM sessions WHERE session_uuid = '0f1e2d3c-4b5a-4968-8776-655443322110'`).
		Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != "killed" {
		t.Errorf("status = %q — ingest must not revert a terminal 'killed' session", status)
	}
}
