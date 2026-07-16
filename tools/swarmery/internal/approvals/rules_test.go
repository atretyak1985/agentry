package approvals

import (
	"encoding/json"
	"testing"
)

func TestParseRulePattern(t *testing.T) {
	cases := []struct {
		in       string
		tool     string
		inner    string
		hasInner bool
		ok       bool
	}{
		{"Read", "Read", "", false, true},
		{"  Bash(git *)  ", "Bash", "git *", true, true},
		{"Bash(npm run test*)", "Bash", "npm run test*", true, true},
		{"mcp__ide__getDiagnostics", "mcp__ide__getDiagnostics", "", false, true}, // MCP tool names are legal tool_names
		{"WebFetch(https://docs.*/*)", "WebFetch", "https://docs.*/*", true, true},
		{"", "", "", false, false},
		{"*", "", "", false, false},               // wildcard tool part forbidden
		{"Ba*sh(x)", "", "", false, false},        // wildcard inside tool part forbidden
		{"Bash()", "", "", false, false},          // empty inner forbidden
		{"(x)", "", "", false, false},             // missing tool part
		{"AskUserQuestion", "", "", false, false}, // never auto-approvable (E12d)
		{"AskUserQuestion(*)", "", "", false, false},
	}
	for _, c := range cases {
		got, err := ParseRulePattern(c.in)
		if (err == nil) != c.ok {
			t.Errorf("ParseRulePattern(%q) err = %v, want ok=%v", c.in, err, c.ok)
			continue
		}
		if c.ok && (got.Tool != c.tool || got.Inner != c.inner || got.HasInner != c.hasInner) {
			t.Errorf("ParseRulePattern(%q) = %+v", c.in, got)
		}
	}
}

func TestRulePatternMatches(t *testing.T) {
	mustParse := func(s string) RulePattern {
		p, err := ParseRulePattern(s)
		if err != nil {
			t.Fatalf("parse %q: %v", s, err)
		}
		return p
	}
	cases := []struct {
		pattern string
		tool    string
		input   string
		want    bool
	}{
		// bare tool: any input
		{"Read", "Read", `{"file_path":"/anything"}`, true},
		{"Read", "Write", `{"file_path":"/x"}`, false},
		// Bash prefix semantics
		{"Bash(git *)", "Bash", `{"command":"git status"}`, true},
		{"Bash(git *)", "Bash", `{"command":"git status && rm -rf /"}`, true}, // documented caveat
		{"Bash(git *)", "Bash", `{"command":"gitk"}`, false},
		{"Bash(git *)", "Bash", `{"command":"git"}`, false}, // needs the space
		{"Bash(git *)", "Bash", `{"command":"sudo git push"}`, false},
		// '*' crosses '/' and spaces (custom glob, NOT path.Match)
		{"Bash(cat *)", "Bash", `{"command":"cat /etc/hosts"}`, true},
		{"Read(/workspace/*)", "Read", `{"file_path":"/workspace/a/b/c.go"}`, true},
		{"Read(/workspace/*)", "Read", `{"file_path":"/etc/passwd"}`, false},
		// exact inner (no '*')
		{"Bash(make test)", "Bash", `{"command":"make test"}`, true},
		{"Bash(make test)", "Bash", `{"command":"make test-e2e"}`, false},
		// middle + suffix segments
		{"WebFetch(https://*.ntfy.sh/*)", "WebFetch", `{"url":"https://docs.ntfy.sh/publish"}`, true},
		{"Bash(git * --force)", "Bash", `{"command":"git push --force"}`, true},
		{"Bash(git * --force)", "Bash", `{"command":"git push"}`, false},
		// deny-by-default: unmapped tool with an inner pattern never matches
		{"Task(deploy*)", "Task", `{"prompt":"deploy prod"}`, false},
		// missing / malformed input never matches an inner pattern
		{"Bash(git *)", "Bash", `{}`, false},
		{"Bash(git *)", "Bash", ``, false},
	}
	for _, c := range cases {
		p := mustParse(c.pattern)
		if got := p.Matches(c.tool, json.RawMessage(c.input)); got != c.want {
			t.Errorf("%q.Matches(%s, %s) = %v, want %v", c.pattern, c.tool, c.input, got, c.want)
		}
	}
}
