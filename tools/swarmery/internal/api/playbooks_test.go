package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/atretyak1985/swarmery/tools/swarmery/internal/playbooks"
	"github.com/atretyak1985/swarmery/tools/swarmery/internal/store"
)

func itoa64(n int64) string { return strconv.FormatInt(n, 10) }

// playbookServer builds a minimal httptest server backed by a fresh DB with one
// project rooted at projectRoot, and attaches a real playbook registry. It
// restores the package registry var on cleanup so it never leaks into sibling
// tests. Returns the server URL, the DB, and the project id.
func playbookServer(t *testing.T, projectRoot string) (*httptest.Server, int64) {
	t.Helper()
	db, err := store.Open(filepath.Join(t.TempDir(), "pb.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	res, err := db.Exec(
		`INSERT INTO projects(path, slug, first_seen) VALUES(?, 'p', '2026-01-01T00:00:00Z')`, projectRoot)
	if err != nil {
		t.Fatal(err)
	}
	pid, _ := res.LastInsertId()

	reg, err := playbooks.New()
	if err != nil {
		t.Fatalf("playbooks.New: %v", err)
	}
	prev := playbookReg
	AttachPlaybooks(reg)
	t.Cleanup(func() { playbookReg = prev })

	mux := http.NewServeMux()
	Routes(mux, &Handler{DB: db})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv, pid
}

func TestListPlaybooks_BuiltinsAndOverride(t *testing.T) {
	root := t.TempDir()
	// A project override of 'standard'.
	pdir := playbooks.ProjectDir(root)
	if err := os.MkdirAll(pdir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pdir, "standard.md"), []byte(`---
name: standard
description: PROJECT OVERRIDE
verify: strict
---
## Stage: implement
{task_prompt}
`), 0o644); err != nil {
		t.Fatal(err)
	}
	srv, pid := playbookServer(t, root)

	// Built-ins only (no projectId).
	var builtins []playbookDTO
	getJSON(t, srv.URL+"/api/playbooks", &builtins)
	if len(builtins) != 4 {
		t.Fatalf("built-in list = %d, want 4", len(builtins))
	}
	for _, p := range builtins {
		if p.Source != "builtin" {
			t.Errorf("%s source = %q, want builtin", p.Name, p.Source)
		}
	}

	// Scoped to the project: 'standard' now comes from the project (override).
	var scoped []playbookDTO
	getJSON(t, srv.URL+"/api/playbooks?projectId="+itoa64(pid), &scoped)
	if len(scoped) != 4 {
		t.Fatalf("scoped list = %d, want 4", len(scoped))
	}
	var found bool
	for _, p := range scoped {
		if p.Name == "standard" {
			found = true
			if p.Source != "project" || p.Description != "PROJECT OVERRIDE" || p.Verify != "strict" {
				t.Errorf("override not applied: %+v", p)
			}
			if p.Path == "" {
				t.Error("project playbook missing path")
			}
		}
	}
	if !found {
		t.Fatal("standard missing from scoped list")
	}
}

func TestListPlaybooks_StageShape(t *testing.T) {
	srv, _ := playbookServer(t, t.TempDir())
	var list []playbookDTO
	getJSON(t, srv.URL+"/api/playbooks", &list)
	byName := map[string]playbookDTO{}
	for _, p := range list {
		byName[p.Name] = p
	}
	rh, ok := byName["review-heavy"]
	if !ok {
		t.Fatal("review-heavy missing")
	}
	if len(rh.Stages) != 2 || rh.Stages[0].Name != "implement" || rh.Stages[1].Name != "self-review" {
		t.Fatalf("review-heavy stages = %+v", rh.Stages)
	}
	if rh.Verify != "strict" {
		t.Errorf("review-heavy verify = %q, want strict", rh.Verify)
	}
}

func TestDuplicatePlaybook_WritesOnceThen409(t *testing.T) {
	root := t.TempDir()
	srv, pid := playbookServer(t, root)
	url := srv.URL + "/api/projects/" + itoa64(pid) + "/playbooks/review-heavy/duplicate"

	// First duplicate → 201 + the file is written verbatim.
	resp, err := http.Post(url, "application/json", nil)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("first duplicate = %d, want 201", resp.StatusCode)
	}
	var body struct {
		Name, Path, Hint string
	}
	json.NewDecoder(resp.Body).Decode(&body)
	resp.Body.Close()
	if body.Name != "review-heavy" {
		t.Errorf("duplicate name = %q", body.Name)
	}
	dest := filepath.Join(playbooks.ProjectDir(root), "review-heavy.md")
	if body.Path != dest {
		t.Errorf("duplicate path = %q, want %q", body.Path, dest)
	}
	written, err := os.ReadFile(dest)
	if err != nil {
		t.Fatalf("read written file: %v", err)
	}
	if !bytes.Contains(written, []byte("name: review-heavy")) || !bytes.Contains(written, []byte("## Stage: self-review")) {
		t.Errorf("written markdown missing expected content:\n%s", written)
	}

	// Second duplicate → 409 (O_EXCL, never overwrite a customization).
	resp2, err := http.Post(url, "application/json", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp2.Body.Close()
	if resp2.StatusCode != http.StatusConflict {
		t.Fatalf("repeat duplicate = %d, want 409", resp2.StatusCode)
	}
}

func TestDuplicatePlaybook_UnknownBuiltinAndProject(t *testing.T) {
	root := t.TempDir()
	srv, pid := playbookServer(t, root)

	// Unknown built-in → 404.
	resp, err := http.Post(srv.URL+"/api/projects/"+itoa64(pid)+"/playbooks/nope/duplicate", "application/json", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("unknown built-in = %d, want 404", resp.StatusCode)
	}

	// Unknown project → 404.
	resp2, err := http.Post(srv.URL+"/api/projects/9999/playbooks/standard/duplicate", "application/json", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp2.Body.Close()
	if resp2.StatusCode != http.StatusNotFound {
		t.Errorf("unknown project = %d, want 404", resp2.StatusCode)
	}
}

// A path-traversal-ish name is rejected before any filesystem access (400).
func TestDuplicatePlaybook_RejectsUnsafeName(t *testing.T) {
	root := t.TempDir()
	srv, pid := playbookServer(t, root)
	// The router will not match a name containing a slash to {name}, so test the
	// safePlaybookName gate directly with an encoded dot-dot segment that DOES
	// reach the handler as a single path value.
	resp, err := http.Post(srv.URL+"/api/projects/"+itoa64(pid)+"/playbooks/..evil/duplicate", "application/json", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("unsafe name = %d, want 400", resp.StatusCode)
	}
}

// The board create/patch surface accepts a valid playbook name, stores it,
// clears it on a blank patch, and rejects an unknown name with 400 (registry
// attached).
func TestBoardTask_PlaybookField(t *testing.T) {
	srv, pid := playbookServer(t, t.TempDir())

	// Create with a valid built-in playbook.
	resp, err := http.Post(srv.URL+"/api/board/tasks", "application/json",
		bytes.NewReader([]byte(`{"projectId":`+itoa64(pid)+`,"title":"t","prompt":"p","playbook":"review-heavy"}`)))
	if err != nil {
		t.Fatal(err)
	}
	var created boardTaskDTO
	json.NewDecoder(resp.Body).Decode(&created)
	resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create with playbook = %d, want 201", resp.StatusCode)
	}
	if created.Playbook == nil || *created.Playbook != "review-heavy" {
		t.Fatalf("created playbook = %v, want review-heavy", created.Playbook)
	}

	// Create with an UNKNOWN playbook → 400.
	resp2, err := http.Post(srv.URL+"/api/board/tasks", "application/json",
		bytes.NewReader([]byte(`{"projectId":`+itoa64(pid)+`,"title":"t","prompt":"p","playbook":"bogus"}`)))
	if err != nil {
		t.Fatal(err)
	}
	resp2.Body.Close()
	if resp2.StatusCode != http.StatusBadRequest {
		t.Fatalf("create with unknown playbook = %d, want 400", resp2.StatusCode)
	}

	// PATCH to another valid playbook.
	req, _ := http.NewRequest(http.MethodPatch,
		srv.URL+"/api/board/tasks/"+itoa64(created.ID), bytes.NewReader([]byte(`{"playbook":"quick-fix"}`)))
	req.Header.Set("Content-Type", "application/json")
	presp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	var patched boardTaskDTO
	json.NewDecoder(presp.Body).Decode(&patched)
	presp.Body.Close()
	if presp.StatusCode != http.StatusOK || patched.Playbook == nil || *patched.Playbook != "quick-fix" {
		t.Fatalf("patch playbook = %d / %v", presp.StatusCode, patched.Playbook)
	}

	// PATCH with a blank string clears back to default (null).
	req2, _ := http.NewRequest(http.MethodPatch,
		srv.URL+"/api/board/tasks/"+itoa64(created.ID), bytes.NewReader([]byte(`{"playbook":""}`)))
	req2.Header.Set("Content-Type", "application/json")
	presp2, err := http.DefaultClient.Do(req2)
	if err != nil {
		t.Fatal(err)
	}
	var cleared boardTaskDTO
	json.NewDecoder(presp2.Body).Decode(&cleared)
	presp2.Body.Close()
	if cleared.Playbook != nil {
		t.Fatalf("cleared playbook = %v, want null", cleared.Playbook)
	}
}

func TestListPlaybooks_InvalidAndUnknownProject(t *testing.T) {
	srv, _ := playbookServer(t, t.TempDir())

	// Malformed projectId → 400.
	resp, err := http.Get(srv.URL + "/api/playbooks?projectId=notanumber")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("malformed projectId = %d, want 400", resp.StatusCode)
	}

	// Unknown projectId → 400.
	resp2, err := http.Get(srv.URL + "/api/playbooks?projectId=99999")
	if err != nil {
		t.Fatal(err)
	}
	resp2.Body.Close()
	if resp2.StatusCode != http.StatusBadRequest {
		t.Errorf("unknown projectId = %d, want 400", resp2.StatusCode)
	}
}

func TestDuplicatePlaybook_NotAttached503(t *testing.T) {
	prev := playbookReg
	playbookReg = nil
	t.Cleanup(func() { playbookReg = prev })

	mux := http.NewServeMux()
	Routes(mux, &Handler{})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	resp, err := http.Post(srv.URL+"/api/projects/1/playbooks/standard/duplicate", "application/json", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Errorf("duplicate without registry = %d, want 503", resp.StatusCode)
	}
}

func TestListPlaybooks_NotAttached503(t *testing.T) {
	// A handler with no registry attached answers 503.
	prev := playbookReg
	playbookReg = nil
	t.Cleanup(func() { playbookReg = prev })

	mux := http.NewServeMux()
	Routes(mux, &Handler{})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	resp, err := http.Get(srv.URL + "/api/playbooks")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Errorf("list without registry = %d, want 503", resp.StatusCode)
	}
}
