package notify

import (
	"database/sql"
	"path/filepath"
	"strings"
	"testing"

	"github.com/atretyak1985/swarmery/tools/swarmery/internal/store"
)

func sessionTestDB(t *testing.T) (*sql.DB, int64) {
	t.Helper()
	db, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	if _, err := db.Exec(
		`INSERT INTO projects (path, slug, name, first_seen)
		 VALUES ('/tmp/proj', '-tmp-proj', 'proj', '2026-07-16T09:00:00.000Z')`); err != nil {
		t.Fatal(err)
	}
	res, err := db.Exec(
		`INSERT INTO sessions (project_id, session_uuid, cwd, status, started_at, title, source)
		 VALUES (1, 'uuid-1', '/tmp/proj', 'completed', '2026-07-16T09:00:00.000Z', 'fix the build', 'jsonl')`)
	if err != nil {
		t.Fatal(err)
	}
	id, _ := res.LastInsertId()
	return db, id
}

func TestSessionEventCompleted(t *testing.T) {
	db, id := sessionTestDB(t)
	e, err := SessionEvent(db, id, 0)
	if err != nil {
		t.Fatal(err)
	}
	if e.Type != EventSessionCompleted || e.SessionID != id || e.Project != "proj" {
		t.Errorf("event = %+v", e)
	}
	if !strings.Contains(e.Body, "fix the build") {
		t.Errorf("body = %q, want the session title", e.Body)
	}
}

func TestSessionEventWithErrors(t *testing.T) {
	db, id := sessionTestDB(t)
	e, err := SessionEvent(db, id, 3)
	if err != nil {
		t.Fatal(err)
	}
	if e.Type != EventSessionError {
		t.Errorf("type = %s, want %s", e.Type, EventSessionError)
	}
	if !strings.Contains(e.Body, "3 error(s)") {
		t.Errorf("body = %q, want the error count", e.Body)
	}
}
