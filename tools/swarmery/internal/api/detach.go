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
	"os"
	"strconv"

	"github.com/atretyak1985/swarmery/tools/swarmery/internal/hookcfg"
	"github.com/atretyak1985/swarmery/tools/swarmery/internal/onboard"
)

type detachRequest struct {
	// DryRun previews the plan without writing (also accepted as ?dryRun=1).
	DryRun bool `json:"dryRun"`
	// Full removes every onboarding artifact (project.json, statusline scripts),
	// not just the settings.json entries. Also accepted as ?full=1.
	Full bool `json:"full"`
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

	// dryRun / full from ?dryRun=1|true / ?full=1|true or the JSON body.
	q := r.URL.Query().Get("dryRun")
	dryRun := q == "1" || q == "true"
	fq := r.URL.Query().Get("full")
	full := fq == "1" || fq == "true"
	if r.Body != nil {
		var req detachRequest
		if err := json.NewDecoder(io.LimitReader(r.Body, 1<<16)).Decode(&req); err == nil {
			dryRun = dryRun || req.DryRun
			full = full || req.Full
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
		Full:          full,
		DryRun:        dryRun,
	})
	if err != nil {
		writeErr(w, err)
		return
	}

	// Full offboard also removes the swarmery approvals/liveness hooks from the
	// project's .claude/settings.local.json (hookcfg surgery: only our entries,
	// foreign hooks survive). Non-fatal — a hook failure never voids the
	// settings.json detach that already happened.
	if full {
		if home, herr := os.UserHomeDir(); herr == nil {
			hooks := &hookcfg.System{Home: home, Out: io.Discard}
			if st := hooks.Inspect(target, 0); st == hookcfg.StateInstalled || st == hookcfg.StateStale {
				if dryRun {
					res.Steps = append(res.Steps, "- .claude/settings.local.json swarmery hooks")
					res.Detached = true
				} else if uerr := hooks.Uninstall(target); uerr != nil {
					res.Steps = append(res.Steps, "! hooks uninstall failed: "+uerr.Error())
				} else {
					res.Steps = append(res.Steps, "✓ .claude/settings.local.json swarmery hooks removed")
					res.Detached = true
				}
			}
		}
	}

	resp := detachResponse{Detached: res.Detached, DryRun: dryRun, Steps: res.Steps}
	if res.Detached && !dryRun {
		resp.Backup = ".claude/settings.json.bak"
	}
	writeJSON(w, resp, nil)
}
