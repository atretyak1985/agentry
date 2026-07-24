package dispatch

import (
	"database/sql"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/atretyak1985/swarmery/tools/swarmery/internal/playbooks"
)

// setTaskPlaybook stamps a playbook name on a task row (the board write surface
// does this in production; here we set it directly for the dispatcher to read).
func setTaskPlaybook(t *testing.T, db *sql.DB, id int64, name string) {
	t.Helper()
	if _, err := db.Exec(`UPDATE tasks SET playbook=? WHERE id=?`, name, id); err != nil {
		t.Fatalf("set playbook: %v", err)
	}
}

// writeProjectPlaybook writes a project-local playbook file and points project 1
// at that root (so the registry's project overlay finds it).
func writeProjectPlaybook(t *testing.T, db *sql.DB, projectRoot, name, content string) {
	t.Helper()
	dir := playbooks.ProjectDir(projectRoot)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dir+"/"+name, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE projects SET path=? WHERE id=1`, projectRoot); err != nil {
		t.Fatalf("point project at root: %v", err)
	}
}

func newRegistry(t *testing.T) *playbooks.Registry {
	t.Helper()
	r, err := playbooks.New()
	if err != nil {
		t.Fatalf("playbooks.New: %v", err)
	}
	return r
}

// A review-heavy task runs TWO sequential stages in ONE worktree, both sessions
// linked to the task (acceptance criterion #1).
func TestPlaybook_ReviewHeavyRunsTwoLinkedStages(t *testing.T) {
	db := testDB(t)
	wt := &stubWt{}
	// Each stage ingests a distinct assistant reply (no sentinel) and exits 0.
	var mu sync.Mutex
	seen := map[string]bool{}
	r := &stubRunner{run: func(spec RunSpec) (*Run, error) {
		mu.Lock()
		seen[spec.SessionUUID] = true
		mu.Unlock()
		ingestSession(t, db, spec.SessionUUID, "stage output for "+spec.SessionUUID)
		return &Run{SessionUUID: spec.SessionUUID, ExitCode: 0}, nil
	}}
	s := newTestService(t, db, r, wt)
	s.Playbooks = newRegistry(t)
	id := insertTask(t, db, "T-rh", taskOpts{})
	setTaskPlaybook(t, db, id, "review-heavy")

	s.Schedule()

	// Two stages → two runs → two linked sessions, one worktree acquired.
	if r.count() != 2 {
		t.Fatalf("runner started %d times, want 2 (implement + self-review)", r.count())
	}
	if wt.acquiredCount() != 1 {
		t.Fatalf("worktree acquired %d times, want 1 (both stages share it)", wt.acquiredCount())
	}
	var links int
	if err := db.QueryRow(`SELECT COUNT(*) FROM task_sessions WHERE task_id=?`, id).Scan(&links); err != nil {
		t.Fatalf("count links: %v", err)
	}
	if links != 2 {
		t.Fatalf("task_sessions links = %d, want 2 (both stage sessions linked)", links)
	}
	// Both stage runs shared the same cwd (the one worktree).
	cwds := map[string]bool{}
	for _, spec := range r.specs {
		cwds[spec.Cwd] = true
	}
	if len(cwds) != 1 {
		t.Fatalf("stages ran in %d distinct cwds, want 1 worktree", len(cwds))
	}
	if got := column(t, db, id); got != "in_review" {
		t.Errorf("column after 2-stage run = %q, want in_review", got)
	}
}

// plan-first stage 2 receives stage 1's reply via {previous_stage_output}
// (acceptance criterion #1, second half).
func TestPlaybook_PlanFirstInjectsPreviousOutput(t *testing.T) {
	db := testDB(t)
	const planReply = "PLAN: 1. do X  2. do Y"
	var stage2Prompt string
	var call int
	r := &stubRunner{run: func(spec RunSpec) (*Run, error) {
		call++
		if call == 1 {
			// Stage 1 (plan): reply with the plan text, exit 0.
			ingestSession(t, db, spec.SessionUUID, planReply)
		} else {
			// Stage 2 (implement): capture the prompt it received.
			stage2Prompt = spec.Prompt
			ingestSession(t, db, spec.SessionUUID, "implemented per plan")
		}
		return &Run{SessionUUID: spec.SessionUUID, ExitCode: 0}, nil
	}}
	s := newTestService(t, db, r, &stubWt{})
	s.Playbooks = newRegistry(t)
	id := insertTask(t, db, "T-pf", taskOpts{})
	setTaskPlaybook(t, db, id, "plan-first")

	s.Schedule()

	if call != 2 {
		t.Fatalf("plan-first ran %d stages, want 2", call)
	}
	if !strings.Contains(stage2Prompt, planReply) {
		t.Fatalf("stage 2 prompt did not inject stage 1's plan via {previous_stage_output}:\n%s", stage2Prompt)
	}
}

// A non-final stage that exits nonzero STOPS the chain with a stage-scoped
// dispatch_error; the later stage never runs.
func TestPlaybook_Stage1FailureStopsChain(t *testing.T) {
	db := testDB(t)
	var call int
	r := &stubRunner{run: func(spec RunSpec) (*Run, error) {
		call++
		// Stage 1 fails (nonzero exit, no sentinel).
		return &Run{SessionUUID: spec.SessionUUID, ExitCode: 3, Stderr: "compile error"}, nil
	}}
	s := newTestService(t, db, r, &stubWt{})
	s.Playbooks = newRegistry(t)
	id := insertTask(t, db, "T-fail", taskOpts{})
	setTaskPlaybook(t, db, id, "review-heavy")

	s.Schedule()

	if call != 1 {
		t.Fatalf("chain ran %d stages, want 1 (stage-1 failure must stop it)", call)
	}
	if got := column(t, db, id); got != "in_review" {
		t.Errorf("column = %q, want in_review", got)
	}
	e := taskField(t, db, id, "dispatch_error")
	if !e.Valid || !strings.Contains(e.String, "stage implement") {
		t.Errorf("dispatch_error = %q, want a 'stage implement failed' message", e.String)
	}
}

// A sentinel on the FIRST stage is authoritative and stops the chain (an honest
// PREMISE STALE means later stages are pointless).
func TestPlaybook_SentinelOnStage1StopsChain(t *testing.T) {
	db := testDB(t)
	wt := &stubWt{}
	var call int
	r := &stubRunner{run: func(spec RunSpec) (*Run, error) {
		call++
		ingestSession(t, db, spec.SessionUUID, "PREMISE STALE: already implemented on HEAD")
		return &Run{SessionUUID: spec.SessionUUID, ExitCode: 0}, nil
	}}
	s := newTestService(t, db, r, wt)
	s.Playbooks = newRegistry(t)
	id := insertTask(t, db, "T-stale2", taskOpts{})
	setTaskPlaybook(t, db, id, "review-heavy")

	s.Schedule()

	if call != 1 {
		t.Fatalf("sentinel on stage 1 should stop the chain; ran %d stages", call)
	}
	if got := column(t, db, id); got != "done" {
		t.Errorf("column = %q, want done (PREMISE STALE sentinel)", got)
	}
}

// The default (NULL) playbook keeps the classic single-stage flow even with a
// registry attached — one run, one link, in_review.
func TestPlaybook_NullFallsBackToSingleStage(t *testing.T) {
	db := testDB(t)
	r := &stubRunner{run: func(spec RunSpec) (*Run, error) {
		ingestSession(t, db, spec.SessionUUID, "done")
		return &Run{SessionUUID: spec.SessionUUID, ExitCode: 0}, nil
	}}
	s := newTestService(t, db, r, &stubWt{})
	s.Playbooks = newRegistry(t)
	id := insertTask(t, db, "T-null", taskOpts{}) // no playbook set

	s.Schedule()

	if r.count() != 1 {
		t.Fatalf("null playbook ran %d stages, want 1 (standard/default)", r.count())
	}
	if got := column(t, db, id); got != "in_review" {
		t.Errorf("column = %q, want in_review", got)
	}
}

// A project-local playbook overriding a built-in name is honored by the
// dispatcher (the registry resolves project → built-in).
func TestPlaybook_ProjectOverrideResolvedByDispatcher(t *testing.T) {
	db := testDB(t)
	root := t.TempDir()
	// Override 'standard' with a TWO-stage project recipe.
	writeProjectPlaybook(t, db, root, "standard.md", `---
name: standard
verify: normal
---
## Stage: one
{task_prompt}
## Stage: two
follow up: {previous_stage_output}
`)
	r := &stubRunner{run: func(spec RunSpec) (*Run, error) {
		ingestSession(t, db, spec.SessionUUID, "reply "+spec.SessionUUID)
		return &Run{SessionUUID: spec.SessionUUID, ExitCode: 0}, nil
	}}
	s := newTestService(t, db, r, &stubWt{})
	s.Playbooks = newRegistry(t)
	id := insertTask(t, db, "T-ovr", taskOpts{})
	setTaskPlaybook(t, db, id, "standard")

	s.Schedule()

	if r.count() != 2 {
		t.Fatalf("project override 'standard' ran %d stages, want 2", r.count())
	}
}
