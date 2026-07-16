package api

// phase: projects — POST /api/projects/{id}/attach re-enables swarmery for a
// detached (or never-onboarded) project: the inverse of detachProject. The
// heavy lifting is onboard.Attach — merge-only settings.json surgery,
// project.json restore from its .bak, statusline redeploy — plus a hookcfg
// reinstall of the approvals/liveness hooks. Fenced exactly like detach:
// requireLocalOrigin at the route, the SWARMERY_ONBOARD_ROOTS allow-list here
// (empty ⇒ 403 disabled), and the symlink-safe resolveUnderRoots check before
// any write.

import (
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"

	"github.com/atretyak1985/swarmery/tools/swarmery/internal/hookcfg"
	"github.com/atretyak1985/swarmery/tools/swarmery/internal/onboard"
)

type attachRequest struct {
	// DryRun previews the plan without writing (also accepted as ?dryRun=1).
	DryRun bool `json:"dryRun"`
}

type attachResponse struct {
	Attached bool     `json:"attached"`
	DryRun   bool     `json:"dryRun"`
	Steps    []string `json:"steps"`
	// Backup is the relative path of the pre-merge settings.json copy, set only
	// when a real run rewrote an existing file.
	Backup string `json:"backup,omitempty"`
}

// attachProject handles POST /api/projects/{id}/attach.
func (h *Handler) attachProject(w http.ResponseWriter, r *http.Request) {
	if len(onboardCfg.Roots) == 0 {
		writeJSONStatus(w, http.StatusForbidden, map[string]string{
			"error": "project attach is disabled — start the daemon with SWARMERY_ONBOARD_ROOTS set to the allowed parent directories",
		})
		return
	}
	if onboardCfg.WorkspaceRoot == "" {
		writeJSONStatus(w, http.StatusForbidden, map[string]string{
			"error": "project attach needs a workspace root — start the daemon with SWARMERY_WORKSPACE_ROOT (or `swarmery install --workspace-root`)",
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
	if r.Body != nil {
		var req attachRequest
		if err := json.NewDecoder(io.LimitReader(r.Body, 1<<16)).Decode(&req); err == nil {
			dryRun = dryRun || req.DryRun
		}
	}

	var path string
	err = h.DB.QueryRow(`SELECT path FROM projects WHERE id = ?`, id).Scan(&path)
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
	if info, statErr := os.Stat(target); statErr != nil || !info.IsDir() {
		writeJSONStatus(w, http.StatusBadRequest, map[string]string{
			"error": "project path does not exist or is not a directory: " + target,
		})
		return
	}

	res, err := onboard.Attach(onboard.AttachConfig{
		ProjectDir:    target,
		WorkspaceRoot: onboardCfg.WorkspaceRoot,
		StatuslineSrc: onboardCfg.StatuslineSrc,
		DryRun:        dryRun,
	})
	if err != nil {
		writeErr(w, err)
		return
	}

	// Reinstall the swarmery approvals/liveness hooks in settings.local.json
	// (hookcfg surgery: refresh only our entries, foreign hooks survive).
	// Non-fatal — a hook failure never voids the settings attach that already
	// happened.
	if home, herr := os.UserHomeDir(); herr == nil {
		hooks := &hookcfg.System{Home: home, Out: io.Discard}
		if st := hooks.Inspect(target, 0); st != hookcfg.StateInstalled {
			if dryRun {
				res.Steps = append(res.Steps, "+ .claude/settings.local.json swarmery hooks")
				res.Attached = true
			} else if ierr := hooks.Install(target, 0); ierr != nil {
				res.Steps = append(res.Steps, "! hooks install failed: "+ierr.Error())
			} else {
				res.Steps = append(res.Steps, "✓ .claude/settings.local.json swarmery hooks installed")
				res.Attached = true
			}
		}
	}

	resp := attachResponse{Attached: res.Attached, DryRun: dryRun, Steps: res.Steps}
	if !dryRun {
		for _, s := range res.Steps {
			if strings.Contains(s, "settings.json merged") {
				resp.Backup = ".claude/settings.json.bak"
			}
		}
	}
	writeJSON(w, resp, nil)
}
