package api

// Soft-archive a project from the dashboard list. DELETE /api/projects/{id}
// flips projects.archived = 1; POST /api/projects/{id}/restore flips it back.
// Nothing is destroyed — the project row, its sessions and their .jsonl
// transcripts are preserved, the project stays reachable by direct id
// (GET /api/projects/{id}), and it reappears in the list under
// ?include=archived — so an archive is fully reversible. Same D4 origin
// hardening as the session-hide and other write endpoints. The `archived`
// column ships in the initial schema (0001_init.sql), so no migration is needed.

import (
	"net/http"
	"strconv"
)

// hideProject handles DELETE /api/projects/{id} — soft-archive.
func (h *Handler) hideProject(w http.ResponseWriter, r *http.Request) {
	h.setProjectArchived(w, r, true)
}

// restoreProject handles POST /api/projects/{id}/restore — un-archive.
func (h *Handler) restoreProject(w http.ResponseWriter, r *http.Request) {
	h.setProjectArchived(w, r, false)
}

// setProjectArchived flips projects.archived and replies {"archived": <v>};
// 400 on a bad id, 404 when no row matched (parity with hideSession).
func (h *Handler) setProjectArchived(w http.ResponseWriter, r *http.Request, archived bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.Error(w, `{"error":"invalid project id"}`, http.StatusBadRequest)
		return
	}
	flag := 0
	if archived {
		flag = 1
	}
	res, err := h.DB.Exec(`UPDATE projects SET archived = ? WHERE id = ?`, flag, id)
	if err != nil {
		writeErr(w, err)
		return
	}
	if n, _ := res.RowsAffected(); n == 0 {
		http.Error(w, `{"error":"project not found"}`, http.StatusNotFound)
		return
	}
	writeJSON(w, map[string]bool{"archived": archived}, nil)
}
