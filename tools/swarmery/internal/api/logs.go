package api

// Fusion phase 9 (Console/DX): GET /api/logs exposes the in-memory structured
// log ring (internal/logbuf) so the `swarmery console` TUI and `swarmery status`
// can render a live, filtered feed without touching disk. Read-only and
// localhost-only like every other endpoint; the ring is attached once at daemon
// startup via AttachLogRing (the same package-var idiom as wsBus/approvalsSvc),
// so httptest handlers built without it simply return an empty snapshot.

import (
	"net/http"
	"strconv"

	"github.com/atretyak1985/swarmery/tools/swarmery/internal/logbuf"
)

// logRing is attached once at daemon startup (nil ⇒ /api/logs returns an empty
// snapshot rather than 503 — an empty ring is a valid, harmless answer).
var logRing *logbuf.Ring

// AttachLogRing wires the daemon's log ring into the /api/logs endpoint.
func AttachLogRing(r *logbuf.Ring) { logRing = r }

// logsDTO is the /api/logs response: the matching entries plus the ring's
// current lastId so a poller can pass it back as sinceId next time.
type logsDTO struct {
	Entries []logbuf.Entry `json:"entries"`
	LastID  int64          `json:"lastId"`
}

// GET /api/logs?sinceId=&tag=&limit=
//
// sinceId: return only entries with id > sinceId (default 0 = from the oldest
// retained). tag: filter to a single subsystem tag (empty = all). limit: cap the
// result to the newest N (default: the ring's own capacity via no cap).
func (h *Handler) logs(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()

	var sinceID int64
	if s := q.Get("sinceId"); s != "" {
		v, err := strconv.ParseInt(s, 10, 64)
		if err != nil || v < 0 {
			writeClientErr(w, http.StatusBadRequest, "invalid sinceId")
			return
		}
		sinceID = v
	}

	var limit int
	if s := q.Get("limit"); s != "" {
		v, err := strconv.Atoi(s)
		if err != nil || v < 1 {
			writeClientErr(w, http.StatusBadRequest, "invalid limit")
			return
		}
		limit = v
	}

	tag := q.Get("tag")

	dto := logsDTO{Entries: []logbuf.Entry{}}
	if logRing != nil {
		entries, last := logRing.Snapshot(sinceID, tag, limit)
		dto.Entries = entries
		dto.LastID = last
	}
	writeJSON(w, dto, nil)
}
