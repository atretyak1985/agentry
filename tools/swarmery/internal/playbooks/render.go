package playbooks

import "strings"

// Vars is the substitution payload for rendering a stage body. All fields are
// plain strings; an unset field renders empty (stage 1 has no
// PreviousStageOutput, so {previous_stage_output} there collapses to "").
type Vars struct {
	TaskPrompt          string // the board task's own prompt (step-doc / card body)
	StartPoint          string // base ref the work forks from (for "diff vs …")
	Branch              string // swarm/<id> worktree branch
	TaskID              string // external card id (T-xxxxxx)
	FileScope           string // human-rendered file scope line
	PreviousStageOutput string // last assistant text of the prior stage ("" for stage 1)
}

// varMap turns Vars into the placeholder→value table. Kept as a method so the
// key names are defined in exactly one place and stay in lockstep with
// knownVars (validated at parse time).
func (v Vars) varMap() map[string]string {
	return map[string]string{
		"task_prompt":           v.TaskPrompt,
		"start_point":           v.StartPoint,
		"branch":                v.Branch,
		"task_id":               v.TaskID,
		"file_scope":            v.FileScope,
		"previous_stage_output": v.PreviousStageOutput,
	}
}

// Render substitutes {var} placeholders in a stage body with the values in v.
// Only well-formed {known_var} tokens are replaced (the same recognition rule
// as validation); literal braces (JSON, code) pass through untouched. Rendering
// never fails — an unknown placeholder cannot occur (parse-time validation
// rejects it), and an empty value is a valid substitution.
func Render(body string, v Vars) string {
	vars := v.varMap()
	var b strings.Builder
	b.Grow(len(body))
	for i := 0; i < len(body); {
		if body[i] != '{' {
			b.WriteByte(body[i])
			i++
			continue
		}
		close := strings.IndexByte(body[i+1:], '}')
		if close < 0 {
			b.WriteString(body[i:]) // no closing brace — rest is literal
			break
		}
		name := body[i+1 : i+1+close]
		if val, ok := vars[name]; ok && isBareIdent(name) {
			b.WriteString(val)
			i += close + 2 // skip "{name}"
			continue
		}
		// Not a recognized placeholder — emit the '{' literally and continue past
		// it (the inner text is re-scanned, so a nested placeholder still renders).
		b.WriteByte('{')
		i++
	}
	return b.String()
}

// RenderStage renders the Nth stage (0-based) of a playbook with v. Returns the
// rendered prompt and false when the index is out of range.
func (p Playbook) RenderStage(idx int, v Vars) (string, bool) {
	if idx < 0 || idx >= len(p.Stages) {
		return "", false
	}
	return Render(p.Stages[idx].Body, v), true
}
