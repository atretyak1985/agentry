package planning

import "testing"

func TestBuildPrompt(t *testing.T) {
	p := BuildPrompt("  add a dark mode toggle  ")

	// Idea is interpolated verbatim (trimmed).
	if want := "The user's idea:\nadd a dark mode toggle"; !contains(p, want) {
		t.Errorf("prompt missing trimmed idea; got tail:\n%s", tailStr(p, 200))
	}
	// Fallback-path invariants the spike proved are load-bearing.
	for _, must := range []string{
		"do NOT call the AskUserQuestion tool", // spike: hook does not fire under -p
		"NUMBERED clarifying questions",        // ask as text, then stop
		"PRIVATE WORKSPACE",                    // plan lands in the workspace, not a repo
		"PLAN SAVED:",                          // unambiguous completion signal
		"@task-planner",                        // agent disambiguation
		"@implementation-planner",
	} {
		if !contains(p, must) {
			t.Errorf("prompt missing required instruction %q", must)
		}
	}
}

func TestBuildPromptEmptyIdea(t *testing.T) {
	// An empty idea still renders a well-formed prompt (the api layer rejects
	// empties before calling, but BuildPrompt must not panic or drop the frame).
	p := BuildPrompt("")
	if !contains(p, "The user's idea:") {
		t.Fatalf("empty idea dropped the frame:\n%s", p)
	}
}

// contains / tailStr are tiny local helpers to keep the test dependency-free.
func contains(s, sub string) bool {
	return len(sub) == 0 || indexOf(s, sub) >= 0
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}

func tailStr(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[len(s)-n:]
}
