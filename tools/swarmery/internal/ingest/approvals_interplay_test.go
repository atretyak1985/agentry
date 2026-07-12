package ingest

// Phase 2 (approvals) ↔ ingest interplay:
//   - a session first minted by the hooks channel (source='hook') is promoted
//     to 'both' when its JSONL transcript shows up — never overwritten to
//     'jsonl';
//   - status='waiting_approval' is owned by the approvals layer and must
//     survive a JSONL tail re-upsert.

import (
	"path/filepath"
	"testing"
)

func TestUpsertPromotesHookSourceToBoth(t *testing.T) {
	db := testDB(t)
	path := filepath.Join(fixtures, "simple-session.jsonl")

	// First contact came through the hooks channel (as internal/approvals
	// creates it): project by cwd + session with source='hook'.
	if _, err := db.Exec(
		`INSERT INTO projects (path, slug, name, first_seen) VALUES ('/tmp/demo', '-tmp-demo', 'demo', '2026-07-13T00:00:00.000Z')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(
		`INSERT INTO sessions (project_id, session_uuid, cwd, status, started_at, source)
		 VALUES (1, '0f1e2d3c-4b5a-4968-8776-655443322110', '/tmp/demo', 'waiting_approval',
		         '2026-07-13T00:00:00.000Z', 'hook')`); err != nil {
		t.Fatal(err)
	}

	if _, err := File(db, path); err != nil {
		t.Fatalf("ingest: %v", err)
	}

	var source, status string
	if err := db.QueryRow(
		`SELECT source, status FROM sessions WHERE session_uuid = '0f1e2d3c-4b5a-4968-8776-655443322110'`).
		Scan(&source, &status); err != nil {
		t.Fatal(err)
	}
	if source != "both" {
		t.Errorf("source = %q, want 'both' (hook + jsonl)", source)
	}
	if status != "waiting_approval" {
		t.Errorf("status = %q — ingest must not steal waiting_approval from the approvals layer", status)
	}

	// Plain jsonl sessions stay 'jsonl' on re-ingest.
	if _, err := File(db, path); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(
		`SELECT source FROM sessions WHERE session_uuid = '0f1e2d3c-4b5a-4968-8776-655443322110'`).
		Scan(&source); err != nil {
		t.Fatal(err)
	}
	if source != "both" {
		t.Errorf("source after second ingest = %q, want 'both' (stable)", source)
	}
}
