package planning

import (
	"strings"
	"text/template"
)

// promptTemplate is the planner prompt appended to the user's idea. It is the
// FALLBACK path proven by the phase-8 spike: headless `claude -p` does NOT fire
// the AskUserQuestion permission hook (Claude auto-resolves it with no TTY), so
// clarifying questions are asked as plain reply TEXT — the Planning page shows
// that reply from the ingested transcript and the user answers via the existing
// session-resume chat (`POST /api/sessions/{id}/message`), continuing THIS
// session. Two-phase contract in one prompt:
//
//  1. FIRST turn: if anything material is ambiguous, ask 1-5 numbered
//     clarifying questions as prose and STOP (no plan yet). If everything is
//     clear, skip straight to step 2.
//  2. After the user replies (resume): pick the right planning agent
//     (@task-planner for < 1 week of work, @implementation-planner otherwise —
//     let the session decide), and save a plan to the private workspace per the
//     workspace convention the agents already know (CLAUDE.md §11 / core pack).
//     End the final turn with the exact line `PLAN SAVED: <absolute plan dir>`
//     so the page has an unambiguous completion signal (belt-and-braces on top
//     of the wsingest rescan that also surfaces the plan dir as a task row).
//
// text/template so {{.Idea}} interpolates without any prompt-side format bug;
// the surrounding wording is fixed.
var promptTemplate = template.Must(template.New("planner").Parse(
	`You are the swarmery planning assistant, running headlessly to turn a rough idea into a structured, executable plan for THIS project (your current working directory is the project repo).

IMPORTANT — how questions work here: you are NOT in an interactive terminal, so do NOT call the AskUserQuestion tool (it will not reach the user). Instead:

STEP 1 — Clarify (only if needed). If any requirement, scope boundary, or acceptance criterion is genuinely ambiguous, ask 1 to 5 NUMBERED clarifying questions as plain text and then STOP — do not write any files yet. Keep questions specific and answerable in a sentence each. If the idea is already unambiguous, skip to STEP 2 immediately.

STEP 2 — Plan. Once the requirements are clear (either straight away, or after the user replies to your questions in a follow-up message):
- Choose the planning agent that fits the scope: use the @task-planner approach for work under ~1 week / <=3 phases, or @implementation-planner for larger multi-phase / multi-repo work. Let the scope decide.
- Write the plan to the PRIVATE WORKSPACE, never into a code repo, following the workspace convention you already know (CLAUDE.md section 11 / core pack): a task dir under the workspace with a plan/README.md (objective, real file paths, phase/step sequencing, risks, Definition of Done) plus phase-N / step-NN docs, each with a self-contained copy-paste agent prompt and measurable acceptance criteria.
- Do NOT implement anything and do NOT create git branches — this is planning only.
- Finish your FINAL message with this exact line on its own (absolute path to the plan directory you created):
  PLAN SAVED: <absolute path to the plan dir>

The user's idea:
{{.Idea}}`))

// BuildPrompt renders the planner prompt for one idea. The idea is trimmed and
// interpolated verbatim; template execution on a fixed template with string
// data cannot fail, so the (unreachable) error is ignored — a failure would
// leave the idea absent, which the caller would still spawn (acceptable and
// unreachable).
func BuildPrompt(idea string) string {
	var b strings.Builder
	_ = promptTemplate.Execute(&b, struct{ Idea string }{Idea: strings.TrimSpace(idea)})
	return b.String()
}
