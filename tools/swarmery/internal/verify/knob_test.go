package verify

import (
	"context"
	"strings"
	"testing"
)

// knobStub scripts the PlaybookVerify seam: it maps a playbook name to its
// verify knob, mirroring what the real registry would resolve — so these tests
// stay decoupled from internal/playbooks.
func knobStub(m map[string]string) func(string, string) string {
	return func(_ /*projectPath*/, name string) string {
		if v, ok := m[name]; ok {
			return v
		}
		return "normal"
	}
}

// A verify:off playbook spawns NO verification run and stamps NO verdict
// (acceptance criterion: verify knob honored — off → no run) — stub-asserted via
// the Runner call count.
func TestKnob_OffSkipsRun(t *testing.T) {
	db := testDB(t)
	r := &stubRunner{out: "VERDICT: PASS"} // would PASS if it ever ran
	s := newTestService(t, db, r, stubTrees{hash: "tree-off"})
	s.PlaybookVerify = knobStub(map[string]string{"quiet": "off"})
	id := insertTask(t, db, taskOpts{playbook: "quiet"})

	if err := s.VerifyTask(context.Background(), id); err != nil {
		t.Fatalf("VerifyTask(off): %v", err)
	}

	if r.count() != 0 {
		t.Fatalf("verify:off spawned %d runs, want 0", r.count())
	}
	if v := verdictOf(t, db, id); v != "" {
		t.Fatalf("verify:off stamped a verdict %q, want none", v)
	}
	// No run row was created either (nothing to reap/finalize).
	var runs int
	if err := db.QueryRow(`SELECT COUNT(*) FROM verification_runs WHERE task_id=?`, id).Scan(&runs); err != nil {
		t.Fatal(err)
	}
	if runs != 0 {
		t.Fatalf("verify:off created %d run rows, want 0", runs)
	}
}

// The auto trigger (Poke) also honors off: no run spawned.
func TestKnob_OffSkipsAutoPoke(t *testing.T) {
	db := testDB(t)
	r := &stubRunner{out: "VERDICT: PASS"}
	s := newTestService(t, db, r, stubTrees{hash: "tree-off2"})
	s.PlaybookVerify = knobStub(map[string]string{"quiet": "off"})
	id := insertTask(t, db, taskOpts{playbook: "quiet"})

	s.Poke(id) // inline Go seam → runs VerifyTask synchronously

	if r.count() != 0 {
		t.Fatalf("verify:off auto-poke spawned %d runs, want 0", r.count())
	}
}

// A verify:strict playbook runs the verifier with the STRICT clause in the
// prompt (the knob moves the bar end-to-end).
func TestKnob_StrictThreadsIntoPrompt(t *testing.T) {
	db := testDB(t)
	var prompt string
	r := &stubRunner{outFn: func(spec RunSpec) *Run {
		prompt = spec.Prompt
		return &Run{Output: "VERDICT: PASS", ExitCode: 0}
	}}
	s := newTestService(t, db, r, stubTrees{hash: "tree-strict"})
	s.PlaybookVerify = knobStub(map[string]string{"review-heavy": "strict"})
	id := insertTask(t, db, taskOpts{playbook: "review-heavy"})

	if err := s.VerifyTask(context.Background(), id); err != nil {
		t.Fatalf("VerifyTask(strict): %v", err)
	}
	if r.count() != 1 {
		t.Fatalf("strict ran %d verifier runs, want 1", r.count())
	}
	if !strings.Contains(prompt, "STRICT REVIEW") {
		t.Fatalf("strict knob did not inject the strict clause into the prompt:\n%s", prompt)
	}
	if v := verdictOf(t, db, id); v != "pass" {
		t.Fatalf("strict verdict = %q, want pass", v)
	}
}

// A verify:normal playbook (and a NULL playbook with no seam) run at the normal
// bar — no strict clause.
func TestKnob_NormalNoStrictClause(t *testing.T) {
	db := testDB(t)
	var prompt string
	r := &stubRunner{outFn: func(spec RunSpec) *Run {
		prompt = spec.Prompt
		return &Run{Output: "VERDICT: PASS", ExitCode: 0}
	}}
	s := newTestService(t, db, r, stubTrees{hash: "tree-normal"})
	s.PlaybookVerify = knobStub(map[string]string{"quick-fix": "normal"})
	id := insertTask(t, db, taskOpts{playbook: "quick-fix"})

	if err := s.VerifyTask(context.Background(), id); err != nil {
		t.Fatalf("VerifyTask(normal): %v", err)
	}
	if strings.Contains(prompt, "STRICT REVIEW") {
		t.Fatalf("normal knob injected the strict clause:\n%s", prompt)
	}
}

// With NO PlaybookVerify seam attached (pre-playbook wiring), verification runs
// at the normal bar regardless of the task's playbook column — the seam is the
// only path that reads the knob, keeping verify decoupled.
func TestKnob_NilSeamDefaultsNormal(t *testing.T) {
	db := testDB(t)
	var prompt string
	r := &stubRunner{outFn: func(spec RunSpec) *Run {
		prompt = spec.Prompt
		return &Run{Output: "VERDICT: PASS", ExitCode: 0}
	}}
	s := newTestService(t, db, r, stubTrees{hash: "tree-nilseam"})
	// s.PlaybookVerify left nil.
	id := insertTask(t, db, taskOpts{playbook: "review-heavy"}) // would be strict IF a seam resolved it

	if err := s.VerifyTask(context.Background(), id); err != nil {
		t.Fatalf("VerifyTask(nil seam): %v", err)
	}
	if r.count() != 1 {
		t.Fatalf("nil seam ran %d runs, want 1 (normal)", r.count())
	}
	if strings.Contains(prompt, "STRICT REVIEW") {
		t.Fatalf("nil seam should default to normal, but injected strict:\n%s", prompt)
	}
}
