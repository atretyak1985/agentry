package playbooks

import (
	"fmt"
	"strings"
)

// knownVars is the closed set of template variables a stage body may reference.
// Validation rejects any {unknown} placeholder so a typo surfaces at parse time
// (registry load / duplicate) rather than producing a literal {typo} in a live
// dispatched prompt. {previous_stage_output} is legal only from stage 2 on, but
// its presence is not an error in stage 1 (it simply renders empty there) — the
// spec's "unknown var" validation is about UNRECOGNIZED names, not positioning.
var knownVars = map[string]bool{
	"task_prompt":           true,
	"start_point":           true,
	"branch":                true,
	"task_id":               true,
	"file_scope":            true,
	"previous_stage_output": true,
}

// stageHeaderPrefix marks a stage section. A line whose trimmed form starts with
// this prefix opens a new stage; everything until the next such line (or EOF) is
// that stage's body.
const stageHeaderPrefix = "## Stage:"

// Parse turns a playbook markdown file into a validated Playbook. It parses the
// leading `---` frontmatter block (name/description/model/verify), splits the
// remainder into `## Stage:` sections, and validates: frontmatter present with a
// name, verify in {strict,normal,off} (default normal when omitted), at least
// one stage, every stage named and non-empty, and every {var} recognized.
func Parse(content, source string) (Playbook, error) {
	fm, rest, err := splitFrontmatter(content)
	if err != nil {
		return Playbook{}, err
	}

	pb := Playbook{Source: source, Verify: "normal"}
	for key, val := range fm {
		switch key {
		case "name":
			pb.Name = val
		case "description":
			pb.Description = val
		case "model":
			pb.Model = val
		case "verify":
			if val != "" {
				pb.Verify = val
			}
		}
	}

	if strings.TrimSpace(pb.Name) == "" {
		return Playbook{}, fmt.Errorf("frontmatter is missing a name")
	}
	if !verifyValues[pb.Verify] {
		return Playbook{}, fmt.Errorf("invalid verify %q (want strict|normal|off)", pb.Verify)
	}

	stages, err := parseStages(rest)
	if err != nil {
		return Playbook{}, err
	}
	if len(stages) == 0 {
		return Playbook{}, fmt.Errorf("playbook %q has no stages (need at least one `## Stage: <name>` section)", pb.Name)
	}
	for _, st := range stages {
		if bad, ok := unknownVar(st.Body); ok {
			return Playbook{}, fmt.Errorf("stage %q references unknown variable {%s}", st.Name, bad)
		}
	}
	pb.Stages = stages
	return pb, nil
}

// splitFrontmatter parses a leading `---`-fenced frontmatter block into a
// key→value map (values trimmed; quotes stripped) and returns the body after the
// closing fence. A file with no frontmatter is an error — every playbook needs
// at least a name. Keys are matched as `key: value` on their own line inside the
// fence; unrecognized keys are ignored by the caller.
func splitFrontmatter(content string) (map[string]string, string, error) {
	// Normalize CRLF so `---` fences and stage headers match regardless of the
	// editor that wrote the project file.
	content = strings.ReplaceAll(content, "\r\n", "\n")
	lines := strings.Split(content, "\n")
	if len(lines) == 0 || strings.TrimSpace(lines[0]) != "---" {
		return nil, "", fmt.Errorf("missing frontmatter (file must start with a `---` fence)")
	}
	fm := map[string]string{}
	i := 1
	closed := false
	for ; i < len(lines); i++ {
		if strings.TrimSpace(lines[i]) == "---" {
			closed = true
			i++
			break
		}
		line := lines[i]
		colon := strings.IndexByte(line, ':')
		if colon <= 0 {
			continue // comment / blank / non key:value line inside the fence
		}
		key := strings.TrimSpace(line[:colon])
		val := strings.TrimSpace(line[colon+1:])
		val = strings.Trim(val, `"'`)
		// Strip a trailing `# comment` from a scalar value (only when clearly a
		// comment — preceded by whitespace — so a `#` inside the value survives).
		if h := strings.Index(val, " #"); h >= 0 {
			val = strings.TrimSpace(val[:h])
		}
		fm[key] = val
	}
	if !closed {
		return nil, "", fmt.Errorf("frontmatter `---` fence is not closed")
	}
	body := strings.Join(lines[i:], "\n")
	return fm, body, nil
}

// parseStages splits a body into ordered stages on `## Stage: <name>` headers.
// The body of each stage is everything after its header up to the next header
// (or EOF), trimmed of surrounding blank lines. A stage with a blank name or an
// empty body is rejected.
func parseStages(body string) ([]Stage, error) {
	lines := strings.Split(body, "\n")
	var stages []Stage
	var curName string
	var curBody []string
	haveStage := false

	flush := func() error {
		if !haveStage {
			return nil
		}
		text := strings.TrimSpace(strings.Join(curBody, "\n"))
		if curName == "" {
			return fmt.Errorf("a `## Stage:` header has no name")
		}
		if text == "" {
			return fmt.Errorf("stage %q has an empty body", curName)
		}
		stages = append(stages, Stage{Name: curName, Body: text})
		return nil
	}

	for _, line := range lines {
		if trimmed := strings.TrimSpace(line); strings.HasPrefix(trimmed, stageHeaderPrefix) {
			if err := flush(); err != nil {
				return nil, err
			}
			curName = strings.TrimSpace(strings.TrimPrefix(trimmed, stageHeaderPrefix))
			curBody = nil
			haveStage = true
			continue
		}
		if haveStage {
			curBody = append(curBody, line)
		}
	}
	if err := flush(); err != nil {
		return nil, err
	}
	return stages, nil
}

// unknownVar scans a stage body for {placeholder} tokens and returns the first
// one whose name is not in knownVars. A `{` not followed by a valid bare
// identifier + `}` is ignored (it is literal prose, e.g. a JSON brace), so only
// well-formed placeholders are validated.
func unknownVar(body string) (string, bool) {
	for i := 0; i < len(body); i++ {
		if body[i] != '{' {
			continue
		}
		close := strings.IndexByte(body[i+1:], '}')
		if close < 0 {
			break // no more closing braces
		}
		name := body[i+1 : i+1+close]
		if isBareIdent(name) {
			if !knownVars[name] {
				return name, true
			}
			i += close + 1 // skip past this placeholder
		}
	}
	return "", false
}

// isBareIdent reports whether s is a non-empty [a-z0-9_] token — the shape of a
// template variable name. Anything else (spaces, punctuation, JSON) is treated
// as literal text, not a placeholder.
func isBareIdent(s string) bool {
	if s == "" {
		return false
	}
	for _, c := range s {
		if !(c >= 'a' && c <= 'z') && !(c >= '0' && c <= '9') && c != '_' {
			return false
		}
	}
	return true
}
