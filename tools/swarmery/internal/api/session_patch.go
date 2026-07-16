package api

// PATCH /api/sessions/{id} — partial session updates. Today the only
// patchable field is `outcome` ('success'|'fail'|'abandoned'|null to clear;
// migration 0014). The DELETE soft-hide contract (session_hide.go) is
// deliberately untouched — PATCH lives alongside it, with the same D4
// requireLocalOrigin hardening.

import (
	"encoding/json"
	"net/http"
	"strconv"
)

// validOutcomes mirrors the CHECK constraint in migration 0014.
var validOutcomes = map[string]bool{"success": true, "fail": true, "abandoned": true}

// patchSession handles PATCH /api/sessions/{id}.
func (h *Handler) patchSession(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.Error(w, `{"error":"invalid session id"}`, http.StatusBadRequest)
		return
	}
	// map[string]*string distinguishes {"outcome": null} (clear) from an
	// absent key (400) and rejects non-string values at decode time.
	var body map[string]*string
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, `{"error":"invalid JSON body"}`, http.StatusBadRequest)
		return
	}
	outcome, ok := body["outcome"]
	if !ok {
		http.Error(w, `{"error":"outcome is required"}`, http.StatusBadRequest)
		return
	}
	if outcome != nil && !validOutcomes[*outcome] {
		http.Error(w, `{"error":"outcome must be success|fail|abandoned|null"}`, http.StatusBadRequest)
		return
	}

	res, err := h.DB.Exec(`UPDATE sessions SET outcome = ? WHERE id = ?`, outcome, id)
	if err != nil {
		writeErr(w, err)
		return
	}
	if n, _ := res.RowsAffected(); n == 0 {
		http.Error(w, `{"error":"session not found"}`, http.StatusNotFound)
		return
	}
	writeJSON(w, map[string]any{"id": id, "outcome": outcome}, nil)
}
