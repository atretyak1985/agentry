package api

// Soft-hide a session from the dashboard list. DELETE /api/sessions/{id} flips
// sessions.hidden = 1 — the row and its .jsonl transcript are preserved (a
// session row is inserted once and never rewritten by re-ingest, so the flag
// survives rescans), and the session stays reachable by direct id, so a hide is
// fully reversible. Same D4 origin hardening as the other write endpoints.

import (
	"net/http"
	"strconv"
)

// hideSession handles DELETE /api/sessions/{id}.
func (h *Handler) hideSession(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.Error(w, `{"error":"invalid session id"}`, http.StatusBadRequest)
		return
	}
	res, err := h.DB.Exec(`UPDATE sessions SET hidden = 1 WHERE id = ?`, id)
	if err != nil {
		writeErr(w, err)
		return
	}
	if n, _ := res.RowsAffected(); n == 0 {
		http.Error(w, `{"error":"session not found"}`, http.StatusNotFound)
		return
	}
	writeJSON(w, map[string]bool{"hidden": true}, nil)
}
