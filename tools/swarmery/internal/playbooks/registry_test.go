package playbooks

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func newRegistry(t *testing.T) *Registry {
	t.Helper()
	r, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return r
}

// The four built-ins ship in the binary; assert their shape matches the spec so
// a broken embed or a renamed file fails loudly.
func TestBuiltins_ShapeMatchesSpec(t *testing.T) {
	r := newRegistry(t)
	want := map[string]struct {
		stages int
		verify string
	}{
		"quick-fix":    {1, "normal"},
		"standard":     {1, "normal"},
		"review-heavy": {2, "strict"},
		"plan-first":   {2, "normal"},
	}
	got := r.List("")
	if len(got) != len(want) {
		t.Fatalf("built-in count = %d, want %d (%+v)", len(got), len(want), got)
	}
	for _, pb := range got {
		w, ok := want[pb.Name]
		if !ok {
			t.Fatalf("unexpected built-in %q", pb.Name)
		}
		if len(pb.Stages) != w.stages {
			t.Errorf("%s: stages = %d, want %d", pb.Name, len(pb.Stages), w.stages)
		}
		if pb.Verify != w.verify {
			t.Errorf("%s: verify = %q, want %q", pb.Name, pb.Verify, w.verify)
		}
		if pb.Source != SourceBuiltin {
			t.Errorf("%s: source = %q, want builtin", pb.Name, pb.Source)
		}
	}
}

// plan-first stage 2 must reference {previous_stage_output} — the contract that
// carries stage 1's reply forward (asserted here so the built-in stays honest).
func TestBuiltin_PlanFirstWiresPreviousOutput(t *testing.T) {
	r := newRegistry(t)
	pb, ok := r.Get("", "plan-first")
	if !ok {
		t.Fatal("plan-first missing")
	}
	if len(pb.Stages) != 2 {
		t.Fatalf("plan-first stages = %d", len(pb.Stages))
	}
	rendered, _ := pb.RenderStage(1, Vars{PreviousStageOutput: "SENTINEL_PLAN"})
	if !contains(rendered, "SENTINEL_PLAN") {
		t.Fatalf("plan-first stage 2 did not inject previous output:\n%s", rendered)
	}
}

func TestBuiltin_ReviewHeavyIsTwoStageStrict(t *testing.T) {
	r := newRegistry(t)
	pb, ok := r.Get("", "review-heavy")
	if !ok {
		t.Fatal("review-heavy missing")
	}
	if len(pb.Stages) != 2 || pb.Verify != "strict" {
		t.Fatalf("review-heavy = %d stages / verify %q, want 2 / strict", len(pb.Stages), pb.Verify)
	}
	if pb.Stages[0].Name != "implement" || pb.Stages[1].Name != "self-review" {
		t.Fatalf("review-heavy stage names = [%s, %s]", pb.Stages[0].Name, pb.Stages[1].Name)
	}
	// Stage 1 injects the task; stage 2 diffs against {start_point}.
	s0, _ := pb.RenderStage(0, Vars{TaskPrompt: "MY_TASK"})
	if !contains(s0, "MY_TASK") {
		t.Fatalf("review-heavy stage 1 missing task prompt")
	}
	s1, _ := pb.RenderStage(1, Vars{StartPoint: "origin/main"})
	if !contains(s1, "origin/main") {
		t.Fatalf("review-heavy stage 2 missing start point")
	}
}

func TestGet_EmptyNameResolvesDefault(t *testing.T) {
	r := newRegistry(t)
	pb, ok := r.Get("", "")
	if !ok || pb.Name != DefaultName {
		t.Fatalf("empty name → %q ok=%v, want %q", pb.Name, ok, DefaultName)
	}
}

func TestGet_UnknownNameNotFound(t *testing.T) {
	r := newRegistry(t)
	if _, ok := r.Get("", "does-not-exist"); ok {
		t.Fatal("unknown playbook resolved ok=true")
	}
}

// A project file with the same name as a built-in must WIN (graduation rule).
func TestProjectOverride_WinsOnCollision(t *testing.T) {
	dir := t.TempDir()
	writePlaybook(t, dir, "standard.md", `---
name: standard
description: PROJECT OVERRIDE
verify: strict
---
## Stage: implement
overridden {task_prompt}
`)
	r := newRegistry(t)

	pb, ok := r.Get(dir, "standard")
	if !ok {
		t.Fatal("standard missing after override")
	}
	if pb.Source != SourceProject {
		t.Fatalf("override source = %q, want project", pb.Source)
	}
	if pb.Description != "PROJECT OVERRIDE" || pb.Verify != "strict" {
		t.Fatalf("override not applied: %+v", pb)
	}
	if pb.Path == "" {
		t.Fatal("project playbook has no Path")
	}
	// The other built-ins are still visible alongside the override.
	if _, ok := r.Get(dir, "review-heavy"); !ok {
		t.Fatal("built-in review-heavy hidden by an unrelated override")
	}
}

// A project can ADD a brand-new playbook not shipped as a built-in.
func TestProjectAddsNewPlaybook(t *testing.T) {
	dir := t.TempDir()
	writePlaybook(t, dir, "hotfix.md", `---
name: hotfix
verify: off
---
## Stage: implement
{task_prompt}
`)
	r := newRegistry(t)
	list := r.List(dir)
	if len(list) != 5 { // 4 built-ins + hotfix
		t.Fatalf("List after add = %d, want 5", len(list))
	}
	pb, ok := r.Get(dir, "hotfix")
	if !ok || pb.Verify != "off" {
		t.Fatalf("hotfix not resolved: %+v ok=%v", pb, ok)
	}
}

// A malformed project file is skipped, never breaking the registry.
func TestProjectMalformedFileSkipped(t *testing.T) {
	dir := t.TempDir()
	writePlaybook(t, dir, "broken.md", "this is not a playbook\n")
	writePlaybook(t, dir, "good.md", `---
name: good
---
## Stage: implement
{task_prompt}
`)
	r := newRegistry(t)
	if _, ok := r.Get(dir, "good"); !ok {
		t.Fatal("good playbook hidden by a sibling malformed file")
	}
	// Still all four built-ins present.
	if _, ok := r.Get(dir, "standard"); !ok {
		t.Fatal("built-in dropped due to malformed project file")
	}
	list := r.List(dir)
	if len(list) != 5 { // 4 built-ins + good (broken skipped)
		t.Fatalf("List with a malformed file = %d, want 5", len(list))
	}
}

// The registry rescans a project dir when its mtime advances (lazy staleness).
func TestProjectRescan_OnMtimeChange(t *testing.T) {
	dir := t.TempDir()
	pdir := ProjectDir(dir)
	if err := os.MkdirAll(pdir, 0o755); err != nil {
		t.Fatal(err)
	}
	r := newRegistry(t)

	// Initially no project files.
	if _, ok := r.Get(dir, "late"); ok {
		t.Fatal("late playbook present before it was written")
	}

	// Add a file and bump the dir mtime forward so the cache is invalidated
	// deterministically (mtime granularity can be coarse on some filesystems).
	writeFileRaw(t, filepath.Join(pdir, "late.md"), `---
name: late
---
## Stage: implement
{task_prompt}
`)
	future := time.Now().Add(2 * time.Second)
	if err := os.Chtimes(pdir, future, future); err != nil {
		t.Fatal(err)
	}

	if _, ok := r.Get(dir, "late"); !ok {
		t.Fatal("registry did not rescan after mtime change")
	}
}

// A cached scan is reused while the mtime is unchanged (no rescan thrash). We
// prove reuse by deleting the file WITHOUT touching the dir mtime and asserting
// the cached view still resolves it.
func TestProjectRescan_CacheReusedWhileMtimeStable(t *testing.T) {
	dir := t.TempDir()
	pdir := ProjectDir(dir)
	if err := os.MkdirAll(pdir, 0o755); err != nil {
		t.Fatal(err)
	}
	fpath := filepath.Join(pdir, "cached.md")
	writeFileRaw(t, fpath, `---
name: cached
---
## Stage: implement
{task_prompt}
`)
	// Pin a stable mtime and prime the cache.
	stamp := time.Now().Add(-1 * time.Hour)
	if err := os.Chtimes(pdir, stamp, stamp); err != nil {
		t.Fatal(err)
	}
	r := newRegistry(t)
	if _, ok := r.Get(dir, "cached"); !ok {
		t.Fatal("cached playbook not primed")
	}

	// Remove the file but restore the dir mtime so the registry sees no change.
	if err := os.Remove(fpath); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(pdir, stamp, stamp); err != nil {
		t.Fatal(err)
	}
	if _, ok := r.Get(dir, "cached"); !ok {
		t.Fatal("cached view was not reused (rescanned despite stable mtime)")
	}
}

func TestList_EmptyProjectPathBuiltinsOnly(t *testing.T) {
	r := newRegistry(t)
	if got := len(r.List("")); got != 4 {
		t.Fatalf("built-in-only List = %d, want 4", got)
	}
	// A non-existent project path also yields built-ins only (no panic).
	if got := len(r.List("/no/such/dir")); got != 4 {
		t.Fatalf("missing-dir List = %d, want 4", got)
	}
}

// ── helpers ──

func writePlaybook(t *testing.T, projectRoot, name, content string) {
	t.Helper()
	pdir := ProjectDir(projectRoot)
	if err := os.MkdirAll(pdir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeFileRaw(t, filepath.Join(pdir, name), content)
}

func writeFileRaw(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func contains(haystack, needle string) bool {
	return len(needle) == 0 || (len(haystack) >= len(needle) && indexOf(haystack, needle) >= 0)
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
