package api

// Playbooks API (fusion phase 13 — selectable workflows): the read surface for
// the playbook registry (built-ins overlaid by a project's own
// .claude/playbooks/*.md) and the one write — duplicate-to-project, which copies
// a built-in's markdown into the project so its prompts become editable (the
// graduation rule; structure stays read-only in the UI). The registry itself
// lives in internal/playbooks; these handlers are the thin HTTP surface. The
// registry is attached once at daemon startup (AttachPlaybooks) — the same
// package-var idiom as dispatchSvc/verifySvc — so httptest handlers built with
// &Handler{DB: db} stay hermetic (playbookReg nil ⇒ the endpoints answer 503).
//
// The duplicate write carries the same D4 requireLocalOrigin hardening as every
// other mutating endpoint, plus O_EXCL so a repeat is a clean 409 (never a
// silent overwrite of an already-customized project file).

import (
	"database/sql"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/atretyak1985/swarmery/tools/swarmery/internal/playbooks"
)

// playbookReg is attached once at daemon startup (nil ⇒ playbook endpoints 503).
// Mirrors dispatchSvc/verifySvc.
var playbookReg *playbooks.Registry

// AttachPlaybooks wires the playbook registry into the api layer. Called from
// cmd/swarmery after the registry is constructed. Left nil in unit tests so the
// endpoints answer 503 without touching the filesystem.
func AttachPlaybooks(r *playbooks.Registry) { playbookReg = r }

// playbookStageDTO is one stage in the API shape (name + raw body). camelCase
// JSON, mirrored in web/src/api/types.ts.
type playbookStageDTO struct {
	Name string `json:"name"`
	Body string `json:"body"`
}

// playbookDTO is the full playbook shape for GET /api/playbooks. Mirrors
// playbooks.Playbook minus the on-disk Path (surfaced only as a hint string).
type playbookDTO struct {
	Name        string             `json:"name"`
	Description string             `json:"description"`
	Model       string             `json:"model"`
	Verify      string             `json:"verify"` // strict | normal | off
	Source      string             `json:"source"` // builtin | project
	Stages      []playbookStageDTO `json:"stages"`
	// Path is the on-disk path of a project playbook ("" for a built-in) — the UI
	// shows it in the "edit <path>" hint after a duplicate.
	Path string `json:"path"`
}

func toPlaybookDTO(p playbooks.Playbook) playbookDTO {
	stages := make([]playbookStageDTO, 0, len(p.Stages))
	for _, st := range p.Stages {
		stages = append(stages, playbookStageDTO{Name: st.Name, Body: st.Body})
	}
	return playbookDTO{
		Name: p.Name, Description: p.Description, Model: p.Model,
		Verify: p.Verify, Source: p.Source, Stages: stages, Path: p.Path,
	}
}

// listPlaybooks — GET /api/playbooks?projectId= : the playbooks visible to a
// project (built-ins overlaid by the project's own files, sorted by name). The
// projectId is optional — omitted ⇒ built-ins only. 503 when the registry is
// not attached; 400 for a malformed/unknown projectId.
func (h *Handler) listPlaybooks(w http.ResponseWriter, r *http.Request) {
	if playbookReg == nil {
		writeClientErr(w, http.StatusServiceUnavailable, "playbooks not attached")
		return
	}
	projectPath := ""
	if pid := strings.TrimSpace(r.URL.Query().Get("projectId")); pid != "" {
		id, err := strconv.ParseInt(pid, 10, 64)
		if err != nil {
			writeClientErr(w, http.StatusBadRequest, "invalid projectId")
			return
		}
		path, ok, err := h.projectPath(id)
		if err != nil {
			writeErr(w, err)
			return
		}
		if !ok {
			writeClientErr(w, http.StatusBadRequest, "unknown project id")
			return
		}
		projectPath = path
	}
	list := playbookReg.List(projectPath)
	out := make([]playbookDTO, 0, len(list))
	for _, p := range list {
		out = append(out, toPlaybookDTO(p))
	}
	writeJSON(w, out, nil)
}

// duplicatePlaybook — POST /api/projects/{id}/playbooks/{name}/duplicate : copy
// a built-in's markdown into <project>/.claude/playbooks/<name>.md so its
// prompts become editable (the graduation override). 201 with the written path
// on success; 404 unknown project or unknown built-in; 409 the project file
// already exists (O_EXCL, never overwrite a customization); 503 registry not
// attached. requireLocalOrigin.
func (h *Handler) duplicatePlaybook(w http.ResponseWriter, r *http.Request) {
	if playbookReg == nil {
		writeClientErr(w, http.StatusServiceUnavailable, "playbooks not attached")
		return
	}
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeClientErr(w, http.StatusBadRequest, "invalid project id")
		return
	}
	name := strings.TrimSpace(r.PathValue("name"))
	if name == "" || !safePlaybookName(name) {
		writeClientErr(w, http.StatusBadRequest, "invalid playbook name")
		return
	}
	projectPath, ok, err := h.projectPath(id)
	if err != nil {
		writeErr(w, err)
		return
	}
	if !ok {
		writeClientErr(w, http.StatusNotFound, "project not found")
		return
	}

	md, ok := playbookReg.BuiltinMarkdown(name)
	if !ok {
		writeClientErr(w, http.StatusNotFound, "no built-in playbook named "+name)
		return
	}

	dir := playbooks.ProjectDir(projectPath)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		writeErr(w, err)
		return
	}
	dest := filepath.Join(dir, name+".md")
	// O_EXCL: a second duplicate of an already-customized file is a clean 409, not
	// a silent overwrite of the user's edits.
	f, err := os.OpenFile(dest, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		if errors.Is(err, os.ErrExist) {
			writeClientErr(w, http.StatusConflict, "project playbook already exists: "+dest)
			return
		}
		writeErr(w, err)
		return
	}
	if _, werr := f.WriteString(md); werr != nil {
		f.Close()
		writeErr(w, werr)
		return
	}
	if cerr := f.Close(); cerr != nil {
		writeErr(w, cerr)
		return
	}
	writeJSONStatus(w, http.StatusCreated, map[string]any{
		"name": name,
		"path": dest,
		"hint": "edit " + dest + " — project files override built-ins",
	})
}

// resolvePlaybookName validates a board task's optional playbook selection for a
// project. It returns the storage value (an `any` — a name string, or nil to
// store NULL) and ok=true; on an unresolvable name it writes a 400 and returns
// ok=false. A nil pointer (field absent from the patch) is a no-op that returns
// (nil, true) — but the CALLER only invokes this when the field is present, so
// in practice: a blank string clears the selection (nil → default 'standard'),
// a non-blank name must resolve in the registry (built-in or project-local).
// When the registry is not attached (unit tests), the name is accepted as-is so
// board tests stay hermetic.
func (h *Handler) resolvePlaybookName(w http.ResponseWriter, projectID int64, name *string) (any, bool) {
	if name == nil {
		return nil, true
	}
	trimmed := strings.TrimSpace(*name)
	if trimmed == "" {
		return nil, true // clear → default recipe
	}
	if playbookReg == nil {
		return trimmed, true // hermetic: no registry to validate against
	}
	projectPath, ok, err := h.projectPath(projectID)
	if err != nil {
		writeErr(w, err)
		return nil, false
	}
	if !ok {
		// The caller already verified the project exists; a race here is a 400.
		writeClientErr(w, http.StatusBadRequest, "unknown project id")
		return nil, false
	}
	if _, found := playbookReg.Get(projectPath, trimmed); !found {
		writeClientErr(w, http.StatusBadRequest, "unknown playbook: "+trimmed)
		return nil, false
	}
	return trimmed, true
}

// projectPath resolves a project's on-disk path by id. ok=false on ErrNoRows.
// (A non-writing sibling of projectPathByID, which writes HTTP errors itself.)
func (h *Handler) projectPath(id int64) (string, bool, error) {
	var path string
	err := h.DB.QueryRow(`SELECT path FROM projects WHERE id = ?`, id).Scan(&path)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return path, true, nil
}

// safePlaybookName rejects a name that could escape the playbooks dir or embed
// a path separator — the {name} path segment is otherwise attacker-influenced.
// Names are the frontmatter identifiers (lowercase kebab), so this is strict.
func safePlaybookName(name string) bool {
	if name == "" || strings.ContainsAny(name, `/\`) || strings.Contains(name, "..") {
		return false
	}
	for _, c := range name {
		if !(c >= 'a' && c <= 'z') && !(c >= '0' && c <= '9') && c != '-' && c != '_' {
			return false
		}
	}
	return true
}
