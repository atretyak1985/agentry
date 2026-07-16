package api

// phase: projects — POST /api/projects/{id}/detach "cleans" a project by
// removing the swarmery-owned entries from its .claude/settings.json (the
// inverse of onboarding). This edits a caller-supplied filesystem path outside
// ~/.claude, so it is fenced exactly like onboardProject: requireLocalOrigin at
// the route, the SWARMERY_ONBOARD_ROOTS allow-list here (empty ⇒ 403 disabled),
// and the symlink-safe resolveUnderRoots check before any write. The actual
// pruning lives in onboard.Detach — conservative, idempotent, backed up to
// settings.json.bak, and dry-runnable.

import (
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"

	"github.com/atretyak1985/swarmery/tools/swarmery/internal/onboard"
)

type detachRequest struct {
	// DryRun previews the plan without writing (also accepted as ?dryRun=1).
	DryRun bool `json:"dryRun"`
}

type detachResponse struct {
	Detached bool     `json:"detached"`
	DryRun   bool     `json:"dryRun"`
	Steps    []string `json:"steps"`
	// Backup is the relative path of the pre-change copy, set only on a real
	// (non-dry-run) write that changed something.
	Backup string `json:"backup,omitempty"`
}

// detachProject handles POST /api/projects/{id}/detach.
func (h *Handler) detachProject(w http.ResponseWriter, r *http.Request) {
	if len(onboardCfg.Roots) == 0 {
		writeJSONStatus(w, http.StatusForbidden, map[string]string{
			"error": "project detach is disabled — start the daemon with SWARMERY_ONBOARD_ROOTS set to the allowed parent directories",
		})
		return
	}
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.Error(w, `{"error":"invalid project id"}`, http.StatusBadRequest)
		return
	}

	// dryRun from ?dryRun=1|true or the JSON body {dryRun:true}.
	q := r.URL.Query().Get("dryRun")
	dryRun := q == "1" || q == "true"
	if !dryRun && r.Body != nil {
		var req detachRequest
		if err := json.NewDecoder(io.LimitReader(r.Body, 1<<16)).Decode(&req); err == nil {
			dryRun = req.DryRun
		}
	}

	var path, slug string
	err = h.DB.QueryRow(`SELECT path, slug FROM projects WHERE id = ?`, id).Scan(&path, &slug)
	if errors.Is(err, sql.ErrNoRows) {
		http.Error(w, `{"error":"project not found"}`, http.StatusNotFound)
		return
	}
	if err != nil {
		writeErr(w, err)
		return
	}

	// Fence: the project path MUST resolve under an allowed onboarding root —
	// the same symlink-safe check the onboarding write uses.
	target, err := resolveUnderRoots(path, onboardCfg.Roots)
	if err != nil {
		writeJSONStatus(w, http.StatusForbidden, map[string]string{"error": err.Error()})
		return
	}

	res, err := onboard.Detach(onboard.DetachConfig{
		ProjectDir:    target,
		Slug:          slug,
		WorkspaceRoot: onboardCfg.WorkspaceRoot,
		DryRun:        dryRun,
	})
	if err != nil {
		writeErr(w, err)
		return
	}

	resp := detachResponse{Detached: res.Detached, DryRun: dryRun, Steps: res.Steps}
	if res.Detached && !dryRun {
		resp.Backup = ".claude/settings.json.bak"
	}
	writeJSON(w, resp, nil)
}
