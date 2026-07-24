package playbooks

import (
	"strings"
	"testing"
)

func TestRender_AllVars(t *testing.T) {
	body := "task={task_prompt} start={start_point} branch={branch} id={task_id} scope={file_scope} prev={previous_stage_output}"
	got := Render(body, Vars{
		TaskPrompt:          "DO THE THING",
		StartPoint:          "main",
		Branch:              "swarm/T-abc",
		TaskID:              "T-abc",
		FileScope:           "web/, api/",
		PreviousStageOutput: "PLAN123",
	})
	want := "task=DO THE THING start=main branch=swarm/T-abc id=T-abc scope=web/, api/ prev=PLAN123"
	if got != want {
		t.Fatalf("Render:\n got %q\nwant %q", got, want)
	}
}

func TestRender_Stage1EmptyPreviousOutput(t *testing.T) {
	// In stage 1 there is no previous output — the placeholder collapses to "".
	got := Render("before[{previous_stage_output}]after", Vars{})
	if got != "before[]after" {
		t.Fatalf("empty previous_stage_output render = %q", got)
	}
}

func TestRender_LiteralBracesPassThrough(t *testing.T) {
	body := `emit {"json": 1} and { spaced } then {task_prompt}`
	got := Render(body, Vars{TaskPrompt: "X"})
	want := `emit {"json": 1} and { spaced } then X`
	if got != want {
		t.Fatalf("literal braces:\n got %q\nwant %q", got, want)
	}
}

func TestRender_UnterminatedBrace(t *testing.T) {
	got := Render("trailing {task_prompt and {open", Vars{TaskPrompt: "X"})
	// {task_prompt and {open — the first '{' has a later '}'? No '}' at all →
	// whole tail is literal from the first '{'.
	if !strings.Contains(got, "{task_prompt and {open") {
		t.Fatalf("unterminated brace mangled: %q", got)
	}
}

func TestRenderStage_Indexing(t *testing.T) {
	pb := Playbook{Stages: []Stage{
		{Name: "a", Body: "first {task_prompt}"},
		{Name: "b", Body: "second {previous_stage_output}"},
	}}
	if s, ok := pb.RenderStage(0, Vars{TaskPrompt: "P"}); !ok || s != "first P" {
		t.Fatalf("stage 0 render = %q ok=%v", s, ok)
	}
	if s, ok := pb.RenderStage(1, Vars{PreviousStageOutput: "Q"}); !ok || s != "second Q" {
		t.Fatalf("stage 1 render = %q ok=%v", s, ok)
	}
	if _, ok := pb.RenderStage(2, Vars{}); ok {
		t.Fatal("out-of-range stage returned ok=true")
	}
	if _, ok := pb.RenderStage(-1, Vars{}); ok {
		t.Fatal("negative index returned ok=true")
	}
}
