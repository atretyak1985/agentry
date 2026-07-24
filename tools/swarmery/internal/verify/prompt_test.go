package verify

import (
	"strings"
	"testing"
)

func TestBuildPrompt_ContainsContract(t *testing.T) {
	p := BuildPrompt("Add waypoint editing", "Criteria:\n- editable list", "swarm/T-abc", StrictnessNormal)
	for _, want := range []string{
		"read-only verification agent",
		"Add waypoint editing",
		"editable list",
		"READ ONLY",
		"INCONCLUSIVE — not FAIL",
		"VERDICT: PASS | FAIL | INCONCLUSIVE",
		"swarm/T-abc", // startPoint interpolated into the diff instruction
	} {
		if !strings.Contains(p, want) {
			t.Errorf("prompt missing %q\n---\n%s", want, p)
		}
	}
}

func TestBuildPrompt_EmptyStartPointFallback(t *testing.T) {
	p := BuildPrompt("t", "c", "", StrictnessNormal)
	if !strings.Contains(p, "the base branch") {
		t.Errorf("empty startPoint should fall back to a neutral phrase; got:\n%s", p)
	}
}

// The verify knob (fusion phase 13) moves ONLY the bar: normal omits the strict
// clause, strict injects it, and the fixed verdict vocabulary is present in both.
func TestBuildPrompt_StrictnessKnob(t *testing.T) {
	normal := BuildPrompt("t", "c", "main", StrictnessNormal)
	strict := BuildPrompt("t", "c", "main", StrictnessStrict)

	if strings.Contains(normal, "STRICT REVIEW") {
		t.Errorf("normal prompt should NOT contain the strict clause:\n%s", normal)
	}
	if !strings.Contains(strict, "STRICT REVIEW") {
		t.Errorf("strict prompt should contain the strict clause:\n%s", strict)
	}
	// The verdict contract is invariant across the knob (only the BAR moves).
	for _, p := range []string{normal, strict} {
		if !strings.Contains(p, "VERDICT: PASS | FAIL | INCONCLUSIVE") {
			t.Errorf("verdict line missing regardless of strictness:\n%s", p)
		}
		if !strings.Contains(p, "READ ONLY") {
			t.Errorf("read-only contract missing regardless of strictness:\n%s", p)
		}
	}
}
