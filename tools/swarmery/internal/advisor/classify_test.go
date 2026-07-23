package advisor

import "testing"

// TestClassify pins EVERY prefix of the classPrefixes table to its class —
// the table is normative (phase-1-error-classification plan doc). Keys are
// normalizeErrKey output: lowercased, digit runs folded to "#".
func TestClassify(t *testing.T) {
	cases := []struct {
		key  string
		want ErrClass
	}{
		// ── infra_noise ──
		{"connection error", InfraNoise},
		{`# {"type":"error","error":{"type":"overloaded_error"`, InfraNoise},
		{"request timed out", InfraNoise},
		// ── harness_recoverable ──
		{"error: file has not been read yet", HarnessRecoverable},
		{"error: file has been modified since read", HarnessRecoverable},
		{"error: file does not exist", HarnessRecoverable},
		{"error: permission for this action was denied by the claude code auto mode", HarnessRecoverable},
		{"error: this agent is isolated in the worktree", HarnessRecoverable},
		// ── behavior_fixable ──
		{"error: exit code #", BehaviorFixable},
		{"error: found # matches of the string to replace", BehaviorFixable},
		{"inputvalidationerror", BehaviorFixable},
		{"error: subagents should return findings as text", BehaviorFixable},
		{"bash error", BehaviorFixable},
		// ── longer keys still prefix-match ──
		{"connection error. please check your internet connection", InfraNoise},
		{"request timed out after # ms", InfraNoise},
		{"error: file has not been read yet. read it first before writing to it.", HarnessRecoverable},
		{"error: exit code # go test failed", BehaviorFixable},
		{"inputvalidationerror: missing required field", BehaviorFixable},
		// ── defaults: unknown / empty → behavior_fixable (conservative) ──
		{"agent flaky boom", BehaviorFixable},
		{"", BehaviorFixable},
		// ── case variations: Classify lowercases before matching ──
		{"Connection Error", InfraNoise},
		{"REQUEST TIMED OUT", InfraNoise},
		{"Error: File has not been read yet", HarnessRecoverable},
		{"InputValidationError", BehaviorFixable},
		{"Bash Error", BehaviorFixable},
	}
	for _, c := range cases {
		if got := Classify(c.key); got != c.want {
			t.Errorf("Classify(%q) = %q, want %q", c.key, got, c.want)
		}
	}
}
