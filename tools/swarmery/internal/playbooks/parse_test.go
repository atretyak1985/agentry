package playbooks

import (
	"strings"
	"testing"
)

func TestParse_ValidSingleStage(t *testing.T) {
	src := `---
name: quick-fix
description: one pass
verify: normal
---
## Stage: implement
{task_prompt}
`
	pb, err := Parse(src, SourceBuiltin)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if pb.Name != "quick-fix" || pb.Verify != "normal" || pb.Source != SourceBuiltin {
		t.Fatalf("unexpected header: %+v", pb)
	}
	if len(pb.Stages) != 1 || pb.Stages[0].Name != "implement" {
		t.Fatalf("want 1 stage 'implement', got %+v", pb.Stages)
	}
	if strings.TrimSpace(pb.Stages[0].Body) != "{task_prompt}" {
		t.Fatalf("stage body = %q", pb.Stages[0].Body)
	}
}

func TestParse_MultiStageOrderPreserved(t *testing.T) {
	src := `---
name: plan-first
verify: normal
---
## Stage: plan
plan it: {task_prompt}
## Stage: implement
do it: {previous_stage_output}
`
	pb, err := Parse(src, SourceProject)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(pb.Stages) != 2 {
		t.Fatalf("want 2 stages, got %d", len(pb.Stages))
	}
	if pb.Stages[0].Name != "plan" || pb.Stages[1].Name != "implement" {
		t.Fatalf("stage order wrong: %+v", pb.Stages)
	}
}

func TestParse_VerifyDefaultsToNormal(t *testing.T) {
	src := `---
name: nolevel
---
## Stage: implement
{task_prompt}
`
	pb, err := Parse(src, SourceBuiltin)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if pb.Verify != "normal" {
		t.Fatalf("verify default = %q, want normal", pb.Verify)
	}
}

func TestParse_ModelAndCommentStripping(t *testing.T) {
	src := `---
name: withmodel
model: opus   # inline comment stripped
verify: strict
---
## Stage: implement
{task_prompt}
`
	pb, err := Parse(src, SourceBuiltin)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if pb.Model != "opus" {
		t.Fatalf("model = %q, want opus (comment stripped)", pb.Model)
	}
	if pb.Verify != "strict" {
		t.Fatalf("verify = %q", pb.Verify)
	}
}

// The validation matrix: each case is a distinct rejection reason.
func TestParse_ValidationMatrix(t *testing.T) {
	cases := []struct {
		name    string
		src     string
		wantErr string
	}{
		{
			name:    "no frontmatter fence",
			src:     "## Stage: implement\n{task_prompt}\n",
			wantErr: "missing frontmatter",
		},
		{
			name:    "unclosed frontmatter",
			src:     "---\nname: x\n## Stage: implement\nbody\n",
			wantErr: "not closed",
		},
		{
			name:    "missing name",
			src:     "---\nverify: normal\n---\n## Stage: implement\n{task_prompt}\n",
			wantErr: "missing a name",
		},
		{
			name:    "no stages",
			src:     "---\nname: empty\n---\njust prose, no stage headers\n",
			wantErr: "no stages",
		},
		{
			name:    "bad verify value",
			src:     "---\nname: x\nverify: paranoid\n---\n## Stage: s\n{task_prompt}\n",
			wantErr: "invalid verify",
		},
		{
			name:    "unknown template var",
			src:     "---\nname: x\n---\n## Stage: s\nhello {not_a_var}\n",
			wantErr: "unknown variable",
		},
		{
			name:    "empty stage body",
			src:     "---\nname: x\n---\n## Stage: s\n## Stage: t\n{task_prompt}\n",
			wantErr: "empty body",
		},
		{
			name:    "stage without name",
			src:     "---\nname: x\n---\n## Stage:\n{task_prompt}\n",
			wantErr: "no name",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Parse(tc.src, SourceProject)
			if err == nil {
				t.Fatalf("expected error containing %q, got nil", tc.wantErr)
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("error %q does not contain %q", err.Error(), tc.wantErr)
			}
		})
	}
}

func TestParse_LiteralBracesAreNotVars(t *testing.T) {
	// A JSON-ish brace and a spaced brace must not be treated as placeholders,
	// so they do not trip the unknown-var validation.
	src := `---
name: braces
---
## Stage: s
Return JSON like {"ok": true} and a set { a, b } but resolve {task_prompt}.
`
	pb, err := Parse(src, SourceProject)
	if err != nil {
		t.Fatalf("Parse rejected literal braces: %v", err)
	}
	if len(pb.Stages) != 1 {
		t.Fatalf("want 1 stage")
	}
}

func TestParse_CRLFNormalized(t *testing.T) {
	src := "---\r\nname: crlf\r\nverify: off\r\n---\r\n## Stage: s\r\n{task_prompt}\r\n"
	pb, err := Parse(src, SourceProject)
	if err != nil {
		t.Fatalf("Parse CRLF: %v", err)
	}
	if pb.Verify != "off" || len(pb.Stages) != 1 {
		t.Fatalf("CRLF parse wrong: %+v", pb)
	}
}

func TestUnknownVar_PreviousStageOutputAllowed(t *testing.T) {
	if bad, ok := unknownVar("use {previous_stage_output} here"); ok {
		t.Fatalf("previous_stage_output flagged as unknown: %q", bad)
	}
	if bad, ok := unknownVar("all five {task_prompt}{start_point}{branch}{task_id}{file_scope}"); ok {
		t.Fatalf("known var flagged: %q", bad)
	}
	if _, ok := unknownVar("{bogus}"); !ok {
		t.Fatal("bogus var not flagged")
	}
}
