package api

import (
	"database/sql"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/atretyak1985/swarmery/tools/swarmery/internal/store"
)

func patchTestServer(t *testing.T) (*httptest.Server, *sql.DB, int64) {
	t.Helper()
	db, err := store.Open(filepath.Join(t.TempDir(), "patch.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	if _, err := db.Exec(
		`INSERT INTO projects (path, slug, name, first_seen, last_activity)
		 VALUES ('/tmp/op', '-tmp-op', 'op', '2026-07-16T00:00:00Z', '2026-07-16T00:00:00Z')`); err != nil {
		t.Fatalf("insert project: %v", err)
	}
	res, err := db.Exec(
		`INSERT INTO sessions (project_id, session_uuid, status, started_at, source)
		 VALUES (1, 'u-outcome-1', 'completed', '2026-07-16T00:00:00Z', 'jsonl')`)
	if err != nil {
		t.Fatalf("insert session: %v", err)
	}
	id, _ := res.LastInsertId()

	h, err := NewServer(db, false)
	if err != nil {
		t.Fatalf("new server: %v", err)
	}
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	return srv, db, id
}

func TestPatchSessionOutcome(t *testing.T) {
	srv, db, id := patchTestServer(t)
	url := srv.URL + "/api/sessions/" + strconv.FormatInt(id, 10)

	doJSON(t, http.MethodPatch, url, map[string]any{"outcome": "success"}, http.StatusOK)

	var got sql.NullString
	if err := db.QueryRow(`SELECT outcome FROM sessions WHERE id = ?`, id).Scan(&got); err != nil {
		t.Fatal(err)
	}
	if !got.Valid || got.String != "success" {
		t.Errorf("outcome = %v, want success", got)
	}

	// The shared projection surfaces it (list + detail + WS use sessionSelect).
	var detail struct {
		Outcome *string `json:"outcome"`
	}
	getJSON(t, url, &detail)
	if detail.Outcome == nil || *detail.Outcome != "success" {
		t.Errorf("detail outcome = %v, want success", detail.Outcome)
	}
}

func TestPatchSessionOutcomeClear(t *testing.T) {
	srv, db, id := patchTestServer(t)
	url := srv.URL + "/api/sessions/" + strconv.FormatInt(id, 10)

	doJSON(t, http.MethodPatch, url, map[string]any{"outcome": "fail"}, http.StatusOK)
	doJSON(t, http.MethodPatch, url, map[string]any{"outcome": nil}, http.StatusOK)

	var got sql.NullString
	if err := db.QueryRow(`SELECT outcome FROM sessions WHERE id = ?`, id).Scan(&got); err != nil {
		t.Fatal(err)
	}
	if got.Valid {
		t.Errorf("outcome = %q, want NULL after clear", got.String)
	}
}

func TestPatchSessionOutcomeValidation(t *testing.T) {
	srv, _, id := patchTestServer(t)
	url := srv.URL + "/api/sessions/" + strconv.FormatInt(id, 10)

	doJSON(t, http.MethodPatch, url, map[string]any{"outcome": "meh"}, http.StatusBadRequest)
	doJSON(t, http.MethodPatch, url, map[string]any{}, http.StatusBadRequest)
	doJSON(t, http.MethodPatch, srv.URL+"/api/sessions/99999",
		map[string]any{"outcome": "success"}, http.StatusNotFound)
}

// The DELETE soft-hide contract must survive the PATCH addition untouched.
func TestPatchDoesNotBreakSoftHide(t *testing.T) {
	srv, _, id := patchTestServer(t)
	doJSON(t, http.MethodDelete, srv.URL+"/api/sessions/"+strconv.FormatInt(id, 10), nil, http.StatusOK)
	if n := sessionsListLen(t, srv.URL); n != 0 {
		t.Errorf("after hide: list len = %d, want 0", n)
	}
}
