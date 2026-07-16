package approvals

import (
	"encoding/json"
	"testing"
)

func TestArgOf(t *testing.T) {
	cases := []struct {
		tool  string
		input string
		want  string
		ok    bool
	}{
		{"Bash", `{"command":"git push origin","description":"d"}`, "git push origin", true},
		{"Read", `{"file_path":"/etc/hosts"}`, "/etc/hosts", true},
		{"Write", `{"file_path":"/tmp/x","content":"…"}`, "/tmp/x", true},
		{"Edit", `{"file_path":"a.go","old_string":"x","new_string":"y"}`, "a.go", true},
		{"WebFetch", `{"url":"https://x.dev","prompt":"p"}`, "https://x.dev", true},
		{"Glob", `{"pattern":"**/*.ts"}`, "**/*.ts", true},
		{"Grep", `{"pattern":"TODO"}`, "TODO", true},
		{"Bash", `{"description":"no command"}`, "", false}, // field absent
		{"Bash", ``, "", false},                             // empty input
		{"Bash", `not json`, "", false},                     // malformed
		{"Task", `{"prompt":"x"}`, "", false},               // unmapped tool → deny-by-default
	}
	for _, c := range cases {
		got, ok := argOf(c.tool, json.RawMessage(c.input))
		if got != c.want || ok != c.ok {
			t.Errorf("argOf(%s, %s) = (%q, %v), want (%q, %v)", c.tool, c.input, got, ok, c.want, c.ok)
		}
	}
}

func TestTruncate(t *testing.T) {
	if got := truncate("  short  ", 10); got != "short" {
		t.Errorf("truncate short = %q", got)
	}
	if got := truncate("aaaaaaaaaa", 4); got != "aaaa…" {
		t.Errorf("truncate long = %q", got)
	}
}
