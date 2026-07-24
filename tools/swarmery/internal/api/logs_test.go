package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/atretyak1985/swarmery/tools/swarmery/internal/logbuf"
	"github.com/atretyak1985/swarmery/tools/swarmery/internal/store"
)

// newTestHandler builds a bare Handler over a fresh migrated DB (no full server
// wiring). Enough for the logs + phase-9 health field tests.
func newLogsHandler(t *testing.T) *Handler {
	t.Helper()
	db, err := store.Open(filepath.Join(t.TempDir(), "logs.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return &Handler{DB: db}
}

func TestLogsEndpointFilterAndSince(t *testing.T) {
	ring := logbuf.New(100)
	AttachLogRing(ring)
	t.Cleanup(func() { AttachLogRing(nil) })

	ring.Append("info", "ingest", "a")
	ring.Append("warn", "dispatch", "b")
	ring.Append("info", "ingest", "c")

	h := newLogsHandler(t)
	srv := httptest.NewServer(http.HandlerFunc(h.logs))
	t.Cleanup(srv.Close)

	// All entries.
	var all logsDTO
	getJSON(t, srv.URL+"/api/logs", &all)
	if len(all.Entries) != 3 {
		t.Fatalf("all: len = %d, want 3", len(all.Entries))
	}
	if all.LastID != 3 {
		t.Errorf("lastId = %d, want 3", all.LastID)
	}

	// Tag filter.
	var disp logsDTO
	getJSON(t, srv.URL+"/api/logs?tag=dispatch", &disp)
	if len(disp.Entries) != 1 || disp.Entries[0].Msg != "b" {
		t.Fatalf("tag=dispatch = %+v, want [b]", disp.Entries)
	}

	// sinceId returns only newer ids.
	var since logsDTO
	getJSON(t, srv.URL+"/api/logs?sinceId=2", &since)
	if len(since.Entries) != 1 || since.Entries[0].ID != 3 {
		t.Fatalf("sinceId=2 = %+v, want [id 3]", since.Entries)
	}

	// limit keeps the newest.
	var lim logsDTO
	getJSON(t, srv.URL+"/api/logs?limit=1", &lim)
	if len(lim.Entries) != 1 || lim.Entries[0].ID != 3 {
		t.Fatalf("limit=1 = %+v, want newest [id 3]", lim.Entries)
	}
}

func TestLogsEndpointBadParams(t *testing.T) {
	h := newLogsHandler(t)
	srv := httptest.NewServer(http.HandlerFunc(h.logs))
	t.Cleanup(srv.Close)

	for _, url := range []string{"/api/logs?sinceId=abc", "/api/logs?limit=0", "/api/logs?limit=-3"} {
		resp, err := http.Get(srv.URL + url)
		if err != nil {
			t.Fatalf("GET %s: %v", url, err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusBadRequest {
			t.Errorf("%s status = %d, want 400", url, resp.StatusCode)
		}
	}
}

func TestLogsEndpointEmptyWhenNoRing(t *testing.T) {
	AttachLogRing(nil)
	h := newLogsHandler(t)
	srv := httptest.NewServer(http.HandlerFunc(h.logs))
	t.Cleanup(srv.Close)

	var dto logsDTO
	getJSON(t, srv.URL+"/api/logs", &dto)
	if len(dto.Entries) != 0 {
		t.Errorf("no ring: entries = %d, want 0", len(dto.Entries))
	}
}

// TestHealthPhase9Fields exercises the additive operational fields directly on
// the handler (dispatcher zero-value when unattached, migration version > 0,
// ingest lag null on a fresh DB, uptime after AttachUptime).
func TestHealthPhase9Fields(t *testing.T) {
	// No dispatcher / no bus / no ring attached → all degrade gracefully.
	AttachLogRing(nil)
	h := newLogsHandler(t)

	AttachUptime(time.Now().Add(-90 * time.Second))
	t.Cleanup(func() { processStart = time.Time{} })

	rr := httptest.NewRecorder()
	h.health(rr, httptest.NewRequest(http.MethodGet, "/api/health", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rr.Code)
	}

	var dto struct {
		DBSizeBytes      int64 `json:"db_size_bytes"`
		DBSizeBytesCamel int64 `json:"dbSizeBytes"`
		UptimeSec        int64 `json:"uptimeSec"`
		MigrationVersion int   `json:"migrationVersion"`
		WSClients        int   `json:"wsClients"`
		IngestLagSec     *int64 `json:"ingestLagSec"`
		Dispatch         struct {
			Active int  `json:"active"`
			Paused bool `json:"paused"`
		} `json:"dispatch"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &dto); err != nil {
		t.Fatalf("decode: %v", err)
	}

	if dto.MigrationVersion <= 0 {
		t.Errorf("migrationVersion = %d, want > 0 (migrated schema)", dto.MigrationVersion)
	}
	if dto.DBSizeBytes != dto.DBSizeBytesCamel {
		t.Errorf("dbSizeBytes camel/snake mismatch: %d vs %d", dto.DBSizeBytesCamel, dto.DBSizeBytes)
	}
	if dto.UptimeSec < 80 {
		t.Errorf("uptimeSec = %d, want ~90", dto.UptimeSec)
	}
	if dto.IngestLagSec != nil {
		t.Errorf("ingestLagSec = %v, want null on a fresh DB", *dto.IngestLagSec)
	}
	if dto.WSClients != 0 {
		t.Errorf("wsClients = %d, want 0 (no bus)", dto.WSClients)
	}
	if dto.Dispatch.Active != 0 || dto.Dispatch.Paused {
		t.Errorf("dispatch = %+v, want zero-value (no dispatcher)", dto.Dispatch)
	}
}
