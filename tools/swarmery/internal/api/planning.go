package api

// Planning Mode endpoints (fusion phase 8): turn a rough idea into a structured
// plan. POST /api/projects/{id}/planning spawns a headless planner run (through
// internal/planning) in the project directory; GET returns its live status. The
// planner asks clarifying questions as reply TEXT (the phase-8 spike proved
// AskUserQuestion does NOT fire the permission hook under `claude -p`) which the
// Planning page surfaces from the ingested transcript; the user answers via the
// EXISTING session-resume chat (POST /api/sessions/{id}/message), and the run
// writes a plan into the private workspace that wsingest surfaces as a workspace
// task row to activate into board tasks.
//
// The service is attached once at daemon startup (AttachPlanning) — the same
// package-var idiom as dispatchSvc/approvalsSvc — so httptest handlers built with
// &Handler{DB: db} stay hermetic (planningSvc nil ⇒ endpoints 503, no spawn).
// Writes carry the D4 requireLocalOrigin hardening; the POST also 409s when a
// run is already active for the project.

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/atretyak1985/swarmery/tools/swarmery/internal/planning"
)

// planningSvc is attached once at daemon startup (nil ⇒ planning endpoints 503
// and the notify adapter is never wired).
var planningSvc *planning.Service

// AttachPlanning wires the planning service into the api layer and gives it the
// api-owned session_updated emitter, keyed by project: on a planner-run edge the
// adapter resolves the in-flight session row (once ingest/the hook mints it) and
// republishes it over the FROZEN session_updated frame — no new WS type. The
// emitter closes over the SERVICE (never the planningSvc package var) so a
// run-goroutine Notify firing during a test's AttachPlanning(nil) teardown does
// not race the package-var write. Called from cmd/swarmery after the service is
// constructed. Left nil in unit tests so board/session writes never trigger a
// real headless spawn.
func AttachPlanning(s *planning.Service) {
	if s != nil {
		s.Notify = func(projectID int64) {
			// The planner session's numeric row is minted by ingest AFTER spawn,
			// so at the start edge there may be no row yet — then this is a no-op
			// and the page's reconcile poll (60s WS net + its own settle poll)
			// catches up. Reads the in-flight uuid from the service snapshot.
			if st := s.Snapshot(projectID); st.SessionID != nil {
				publishSessionUpdated(*st.SessionID)
			}
		}
	}
	planningSvc = s
}

// GET /api/projects/{id}/planning — the planner status snapshot for a project
// (active, sessionUuid, sessionId once resolvable, startedAt). 503 when the
// planning service is not attached (serve --no-ingest, or a test handler).
func (h *Handler) getPlanning(w http.ResponseWriter, r *http.Request) {
	if planningSvc == nil {
		writeClientErr(w, http.StatusServiceUnavailable, "planning not attached")
		return
	}
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeClientErr(w, http.StatusBadRequest, "invalid project id")
		return
	}
	writeJSON(w, planningSvc.Snapshot(id), nil)
}

// POST /api/projects/{id}/planning {idea} → 202 {sessionUuid}. requireLocalOrigin.
// 400 invalid id / empty idea; 404 unknown project; 409 a run is already active;
// 503 the service is not attached.
func (h *Handler) startPlanning(w http.ResponseWriter, r *http.Request) {
	if planningSvc == nil {
		writeClientErr(w, http.StatusServiceUnavailable, "planning not attached")
		return
	}
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeClientErr(w, http.StatusBadRequest, "invalid project id")
		return
	}
	var body struct {
		Idea string `json:"idea"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&body); err != nil {
		writeClientErr(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if len(body.Idea) > maxPlanningIdeaLen {
		writeClientErr(w, http.StatusRequestEntityTooLarge, "idea too long")
		return
	}
	if strings.TrimSpace(body.Idea) == "" {
		writeClientErr(w, http.StatusBadRequest, "idea is required")
		return
	}

	uuid, err := planningSvc.Start(id, body.Idea)
	switch {
	case errors.Is(err, planning.ErrProjectNotFound):
		writeClientErr(w, http.StatusNotFound, "project not found")
		return
	case errors.Is(err, planning.ErrNoPath):
		writeClientErr(w, http.StatusConflict, "project has no known path to plan in")
		return
	case errors.Is(err, planning.ErrActive):
		writeJSONStatus(w, http.StatusConflict, map[string]any{
			"error":       "a planning run is already active for this project",
			"sessionUuid": planningSvc.Snapshot(id).SessionUUID,
		})
		return
	case err != nil:
		writeErr(w, err)
		return
	}
	writeJSONStatus(w, http.StatusAccepted, map[string]string{"sessionUuid": uuid})
}

// POST /api/projects/{id}/planning/cancel — abort the in-flight planner run.
// requireLocalOrigin. 409 when nothing is in flight; 503 when not attached.
func (h *Handler) cancelPlanning(w http.ResponseWriter, r *http.Request) {
	if planningSvc == nil {
		writeClientErr(w, http.StatusServiceUnavailable, "planning not attached")
		return
	}
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeClientErr(w, http.StatusBadRequest, "invalid project id")
		return
	}
	if !planningSvc.Cancel(id) {
		writeClientErr(w, http.StatusConflict, "no planning run is active for this project")
		return
	}
	writeJSONStatus(w, http.StatusAccepted, map[string]any{"status": "cancelling", "projectId": id})
}

// maxPlanningIdeaLen bounds the idea payload (a paragraph or three, not a file).
const maxPlanningIdeaLen = 8000
