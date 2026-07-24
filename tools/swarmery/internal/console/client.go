// Package console is the client half of the fusion phase-9 Console/DX feature:
// the data types + HTTP/WS client that `swarmery status` (one-shot snapshot) and
// `swarmery console` (bubbletea TUI) attach to the running daemon with. The
// bubbletea model (model.go) and status formatter (status.go) depend only on the
// Client interface, so both are unit-testable against a stub with no daemon and
// no TTY. The interactive terminal wiring lives in cmd/swarmery (kept out of the
// coverage denominator per CI policy).
package console

import (
	"context"
	"encoding/json"
)

// ── daemon JSON shapes (mirrors of the api-layer DTOs; field tags match) ──

// Health mirrors GET /api/health (the phase-9 superset). Only the fields the
// console renders are modelled; unknown fields are ignored by encoding/json.
type Health struct {
	Status           string `json:"status"`
	Version          string `json:"version"`
	DBSizeBytes      int64  `json:"dbSizeBytes"`
	UptimeSec        int64  `json:"uptimeSec"`
	MigrationVersion int    `json:"migrationVersion"`
	WSClients        int    `json:"wsClients"`
	IngestLagSec     *int64 `json:"ingestLagSec"`
	Dispatch         struct {
		Active int  `json:"active"`
		Paused bool `json:"paused"`
	} `json:"dispatch"`
}

// StatsToday mirrors GET /api/stats/today (the snake_case parity contract).
type StatsToday struct {
	Sessions  int64    `json:"sessions"`
	Active    int64    `json:"active"`
	TokensIn  int64    `json:"tokens_in"`
	TokensOut int64    `json:"tokens_out"`
	CostUSD   *float64 `json:"cost_usd"`
	Errors    int64    `json:"errors"`
}

// TotalTokens is in+out — the "tok" figure in the status block.
func (s StatsToday) TotalTokens() int64 { return s.TokensIn + s.TokensOut }

// Approval mirrors one permissionRequestDTO from GET /api/approvals. RequestJSON
// carries the gated tool's args/cwd (parsed lazily by the model for display).
type Approval struct {
	ID          int64  `json:"id"`
	SessionID   int64  `json:"sessionId"`
	ToolName    string `json:"toolName"`
	RequestJSON string `json:"requestJson"`
	Status      string `json:"status"`
	RequestedAt string `json:"requestedAt"`
}

// LogEntry mirrors one entry from GET /api/logs.
type LogEntry struct {
	ID    int64  `json:"id"`
	TS    string `json:"ts"`
	Level string `json:"level"`
	Tag   string `json:"tag"`
	Msg   string `json:"msg"`
}

// logsResponse is the /api/logs envelope.
type logsResponse struct {
	Entries []LogEntry `json:"entries"`
	LastID  int64      `json:"lastId"`
}

// Dispatch mirrors GET /api/dispatch (used by the pause toggle read-back).
type Dispatch struct {
	Enabled      bool     `json:"enabled"`
	GlobalPaused bool     `json:"globalPaused"`
	ActiveRuns   int      `json:"activeRuns"`
	FreeSlots    int      `json:"freeSlots"`
	PausedScopes []string `json:"pausedScopes"`
}

// WSEvent is one decoded frame off /api/ws: the envelope type plus its raw
// payload (the model only needs the type to decide whether to refetch, and the
// raw bytes for the log/event feed). No new WS message type is introduced —
// this consumes the frozen envelope verbatim.
type WSEvent struct {
	Type    string          `json:"type"`
	Payload json.RawMessage `json:"payload"`
}

// ── the client seam ──

// Client is the daemon-facing surface the status renderer and the TUI model use.
// A real implementation (httpClient) talks to the daemon; tests use a stub.
type Client interface {
	// Snapshot fetches the three read endpoints the status block needs in one
	// call. A partial failure still returns what succeeded plus err set, so the
	// caller can render a degraded block (the daemon-down path).
	Snapshot(ctx context.Context) (Snapshot, error)
	// Logs fetches ring entries newer than sinceID (0 = from the oldest).
	Logs(ctx context.Context, sinceID int64, tag string, limit int) ([]LogEntry, int64, error)
	// ResolveApproval approves or denies a pending request (action: approve|deny).
	ResolveApproval(ctx context.Context, id int64, action string) error
	// PauseDispatch sets the global dispatcher pause flag.
	PauseDispatch(ctx context.Context, paused bool) error
	// BaseURL is the dashboard URL shown in the header / opened by [o].
	BaseURL() string
}

// Snapshot is the aggregate the status block renders.
type Snapshot struct {
	Health    Health
	Stats     StatsToday
	Approvals []Approval
	// Reachable is false when the daemon could not be reached at all; the
	// renderer then prints the down banner and `swarmery status` exits nonzero.
	Reachable bool
	// url is the daemon base URL shown in the block (set by the CLI so the URL
	// always matches the client's target). Unexported: rendering detail only.
	url string
	// maxSlots is the dispatcher's MaxConcurrent when a GET /api/dispatch read
	// supplied it (0 = unknown → the renderer falls back to the active count).
	maxSlots int
}

// WithURL sets the dashboard URL rendered in the status block/header.
func (s Snapshot) WithURL(url string) Snapshot { s.url = url; return s }

// WithMaxSlots records the dispatcher slot total for the "N running / M slots"
// line (from GET /api/dispatch).
func (s Snapshot) WithMaxSlots(n int) Snapshot { s.maxSlots = n; return s }
