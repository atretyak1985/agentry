package api

// Parity wave: daemon health endpoint for the dashboard header.
//
// The ORIGINAL parity contract (snake_case) is preserved verbatim so the web
// header keeps working:
//   {"status":"ok","version":"<semver>","db_size_bytes":<int>,"watching":<bool>}
//
// Fusion phase 9 (Console/DX) ADDS operational fields consumed by `swarmery
// status` / `swarmery console` (camelCase, additive — nothing above is renamed):
//   uptimeSec, migrationVersion, wsClients, ingestLagSec, dispatch{active,paused}
// dbSizeBytes duplicates db_size_bytes in camelCase for the new CLI without
// breaking the frozen snake_case reader.

import (
	"database/sql"
	"net/http"
	"time"

	"github.com/atretyak1985/swarmery/tools/swarmery/internal/version"
)

// processStart is stamped once at daemon startup (AttachUptime) so /api/health
// can report uptimeSec. Zero until attached ⇒ uptime is reported as 0.
var processStart time.Time

// AttachUptime records the daemon's start instant for the health uptime field.
func AttachUptime(t time.Time) { processStart = t }

type healthDTO struct {
	Status      string `json:"status"`
	Version     string `json:"version"`
	DBSizeBytes int64  `json:"db_size_bytes"`
	Watching    bool   `json:"watching"`
	// hooks_last_seen: ISO timestamp of the most recent POST /api/hooks/*
	// (phase 2 heartbeat, additive optional per the frozen HealthResponse).
	// Kept in-memory in the approvals service — absent until the first hook
	// checks in after daemon start.
	HooksLastSeen *string `json:"hooks_last_seen,omitempty"`

	// ── fusion phase 9 additive operational fields (camelCase) ──
	UptimeSec        int64          `json:"uptimeSec"`
	DBSizeBytesCamel int64          `json:"dbSizeBytes"` // camelCase mirror for the CLI
	MigrationVersion int            `json:"migrationVersion"`
	WSClients        int            `json:"wsClients"`
	IngestLagSec     *int64         `json:"ingestLagSec"` // null when no events ingested yet
	Dispatch         healthDispatch `json:"dispatch"`
}

// healthDispatch is the zero-valued-when-absent dispatcher summary (the spec's
// "dispatch: {active, paused} (zero-value if Phase 3 absent)").
type healthDispatch struct {
	Active int  `json:"active"` // live runs in this process
	Paused bool `json:"paused"` // global pause flag
}

// GET /api/health
//
// db_size_bytes is computed from the live connection (page_count ×
// page_size), so it needs no filesystem access to the DB path. watching is
// true when the ingest pipeline is attached (serve without --no-ingest). The
// fusion-phase-9 operational fields are best-effort: a failed sub-query leaves
// its field at the zero value rather than failing the whole endpoint.
func (h *Handler) health(w http.ResponseWriter, r *http.Request) {
	var size int64
	err := h.DB.QueryRow(
		`SELECT page_count * page_size FROM pragma_page_count(), pragma_page_size()`).Scan(&size)

	dto := healthDTO{
		Status:           "ok",
		Version:          version.Version,
		DBSizeBytes:      size,
		DBSizeBytesCamel: size,
		Watching:         h.Watching,
		MigrationVersion: h.migrationVersion(),
		WSClients:        wsClientCount(),
		IngestLagSec:     h.ingestLagSec(),
		Dispatch:         dispatchHealth(),
	}
	if !processStart.IsZero() {
		dto.UptimeSec = int64(time.Since(processStart).Seconds())
	}
	if approvalsSvc != nil {
		if t, ok := approvalsSvc.LastSeen(); ok {
			iso := t.UTC().Format(time.RFC3339)
			dto.HooksLastSeen = &iso
		}
	}
	writeJSON(w, dto, err)
}

// migrationVersion reads the highest applied schema-migration version. Best
// effort: 0 on any error (a health probe must never 500 over this).
func (h *Handler) migrationVersion() int {
	var v sql.NullInt64
	if err := h.DB.QueryRow(`SELECT MAX(version) FROM schema_migrations`).Scan(&v); err != nil {
		return 0
	}
	if !v.Valid {
		return 0
	}
	return int(v.Int64)
}

// ingestLagSec is (now − newest ingested event ts) in whole seconds, or nil
// when no events have been ingested yet (fresh DB) so the client can render
// "—" instead of a misleading 0. Negative skew is clamped to 0.
func (h *Handler) ingestLagSec() *int64 {
	var newest sql.NullString
	if err := h.DB.QueryRow(`SELECT MAX(ts) FROM events`).Scan(&newest); err != nil {
		return nil
	}
	if !newest.Valid || newest.String == "" {
		return nil
	}
	t, err := parseEventTS(newest.String)
	if err != nil {
		return nil
	}
	lag := int64(time.Since(t).Seconds())
	if lag < 0 {
		lag = 0
	}
	return &lag
}

// parseEventTS parses the ISO-8601 UTC timestamps events.ts stores. The ingest
// pipeline writes RFC3339 (optionally fractional); try both.
func parseEventTS(s string) (time.Time, error) {
	if t, err := time.Parse(time.RFC3339Nano, s); err == nil {
		return t, nil
	}
	return time.Parse(time.RFC3339, s)
}

// dispatchHealth returns the dispatcher summary, zero-valued when the dispatcher
// is not attached (serve --no-ingest, or a test handler) — the spec's "zero-value
// if Phase 3 absent".
func dispatchHealth() healthDispatch {
	if dispatchSvc == nil {
		return healthDispatch{}
	}
	st, err := dispatchSvc.Snapshot()
	if err != nil {
		return healthDispatch{}
	}
	return healthDispatch{Active: st.ActiveRuns, Paused: st.GlobalPaused}
}
