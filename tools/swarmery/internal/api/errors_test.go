package api

import (
	"database/sql"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/atretyak1985/swarmery/tools/swarmery/internal/store"
)

// errorsServer seeds the two real error-payload shapes the ingester writes
// (ingest.go): system api_error → payload {"error":{...}}; tool failure →
// payload {"input":…, "result": "<error text>"}. Two api_errors differ only
// in the request id and must fold to ONE group.
func errorsServer(t *testing.T) (*httptest.Server, [3]string) {
	t.Helper()
	db, err := store.Open(filepath.Join(t.TempDir(), "errors.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	const tsFmt = "2006-01-02T15:04:05.000Z"
	now := time.Now()
	todayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	at := func(h int) string { return todayStart.Add(time.Duration(h) * time.Hour).UTC().Format(tsFmt) }
	ts10, ts11, ts12 := at(10), at(11), at(12)
	day20 := todayStart.AddDate(0, 0, -20).Add(12 * time.Hour).UTC().Format(tsFmt)

	mustExec := func(q string, args ...any) {
		t.Helper()
		if _, err := db.Exec(q, args...); err != nil {
			t.Fatalf("exec: %v\n%s", err, q)
		}
	}

	mustExec(`INSERT INTO projects (id, path, slug, name, first_seen) VALUES
		(1, '/work/alpha', '-work-alpha', 'Alpha', ?),
		(2, '/work/beta',  '-work-beta',  NULL,    ?)`, day20, day20)
	mustExec(`INSERT INTO sessions (id, project_id, session_uuid, model, status, started_at, title) VALUES
		(1, 1, 'u1', 'claude-fable-5', 'active',    ?, 'Fix login flow'),
		(2, 2, 'u2', 'claude-fable-5', 'completed', ?, NULL)`, ts10, ts10)

	mustExec(`INSERT INTO events (session_id, ts, type, tool_name, status, payload, dedup_key) VALUES
		(1, ?, 'error', NULL, 'error',
		 '{"error":{"message":"API Error 529 overloaded (request id req_011abc)"}}', 'e1'),
		(2, ?, 'error', NULL, 'error',
		 '{"error":{"message":"API Error 529 overloaded (request id req_022xyz)"}}', 'e2'),
		(1, ?, 'tool_call', 'Bash', 'error',
		 '{"input":{"command":"npm test"},"result":"Error: ENOENT: no such file or directory, open ''/tmp/build-4821/out.log''"}', 'e3'),
		(1, ?, 'error', NULL, 'error',
		 '{"error":{"message":"API Error 529 overloaded (request id req_099old)"}}', 'e4')`,
		ts10, ts11, ts12, day20)

	h, err := NewServer(db, false)
	if err != nil {
		t.Fatalf("new server: %v", err)
	}
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	return srv, [3]string{ts10, ts11, ts12}
}

func TestNormalizeErrKey(t *testing.T) {
	longCyrillic := strings.Repeat("ошибка соединения ", 10) // 180 runes, multibyte
	tests := []struct {
		name, in, want string
	}{
		{
			name: "digit tokens fold to # so ids and status codes share a group",
			in:   "API Error 529 overloaded (request id req_011abc)",
			want: "api error # overloaded (request id #)",
		},
		{
			name: "hex ids, ports and path segments with digits fold too",
			in:   "ECONNREFUSED 127.0.0.1:8080 in /tmp/build-4821/out.log",
			want: "econnrefused #.#.#.#:# in /tmp/build-#/out.log",
		},
		{
			name: "whitespace folds and trims",
			in:   "  connection\t\treset\n by peer  ",
			want: "connection reset by peer",
		},
		{
			name: "80-rune truncation counts runes, not bytes, on multibyte input",
			in:   longCyrillic,
			want: string([]rune(strings.TrimSpace(longCyrillic))[:80]),
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := normalizeErrKey(tc.in); got != tc.want {
				t.Errorf("normalizeErrKey(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
	if got := normalizeErrKey(longCyrillic); len([]rune(got)) != 80 {
		t.Errorf("multibyte key length = %d runes, want 80", len([]rune(got)))
	}
}

func TestExtractErrMsg(t *testing.T) {
	str := func(s string) sql.NullString { return sql.NullString{String: s, Valid: true} }
	none := sql.NullString{}
	tests := []struct {
		name     string
		typ      string
		toolName sql.NullString
		payload  sql.NullString
		want     string
	}{
		{
			name: "api error with a plain string payload.error",
			typ:  "error", toolName: none,
			payload: str(`{"error":"stream disconnected"}`),
			want:    "stream disconnected",
		},
		{
			name: "api error with error.message object",
			typ:  "error", toolName: none,
			payload: str(`{"error":{"message":"overloaded"}}`),
			want:    "overloaded",
		},
		{
			name: "api error with one-level nested error.error.message",
			typ:  "error", toolName: none,
			payload: str(`{"error":{"type":"api_error","error":{"message":"rate limited"}}}`),
			want:    "rate limited",
		},
		{
			name: "api error object without message falls back to its JSON marshal",
			typ:  "error", toolName: none,
			payload: str(`{"error":{"code":529,"type":"overloaded_error"}}`),
			want:    `{"code":529,"type":"overloaded_error"}`,
		},
		{
			name: "api error with empty payload falls back to the generic label",
			typ:  "error", toolName: none, payload: none,
			want: "api error",
		},
		{
			name: "tool failure with a string result",
			typ:  "tool_call", toolName: str("Bash"),
			payload: str(`{"input":{"command":"npm test"},"result":"exit status 1"}`),
			want:    "exit status 1",
		},
		{
			name: "tool failure with a result.message object",
			typ:  "tool_call", toolName: str("Edit"),
			payload: str(`{"result":{"message":"old_string not found"}}`),
			want:    "old_string not found",
		},
		{
			name: "tool failure with an object result without message marshals it",
			typ:  "tool_call", toolName: str("Edit"),
			payload: str(`{"result":{"interrupted":true}}`),
			want:    `{"interrupted":true}`,
		},
		{
			name: "empty payload falls back to the tool name",
			typ:  "tool_call", toolName: str("Bash"), payload: none,
			want: "Bash error",
		},
		{
			name: "no payload and no tool name falls back to the generic label",
			typ:  "tool_call", toolName: none, payload: none,
			want: "error",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := extractErrMsg(tc.typ, tc.toolName, tc.payload); got != tc.want {
				t.Errorf("extractErrMsg(%q, %v, %v) = %q, want %q",
					tc.typ, tc.toolName, tc.payload, got, tc.want)
			}
		})
	}
}

func TestStatsErrors(t *testing.T) {
	srv, ts := errorsServer(t)

	var out errorsDTO
	getJSON(t, srv.URL+"/api/stats/errors", &out)
	if len(out.Groups) != 2 { // day20 row out of range
		t.Fatalf("groups = %d (%+v), want 2", len(out.Groups), out.Groups)
	}

	api := out.Groups[0] // count 2 ranks first
	if api.Key != "api error # overloaded (request id #)" {
		t.Errorf("api key = %q", api.Key)
	}
	if api.Count != 2 {
		t.Errorf("api count = %d, want 2", api.Count)
	}
	if api.LastTs != ts[1] { // newest of the group (ts11)
		t.Errorf("api last_ts = %q, want %q", api.LastTs, ts[1])
	}
	if len(api.Samples) != 2 {
		t.Fatalf("api samples = %+v, want 2 distinct sessions", api.Samples)
	}
	// ts DESC → session 2 (untitled) first, then session 1 with its title.
	if api.Samples[0].SessionID != 2 || api.Samples[0].Title != nil {
		t.Errorf("sample[0] = %+v, want session 2 untitled", api.Samples[0])
	}
	if api.Samples[1].SessionID != 1 || api.Samples[1].Title == nil || *api.Samples[1].Title != "Fix login flow" {
		t.Errorf("sample[1] = %+v, want session 1 'Fix login flow'", api.Samples[1])
	}

	tool := out.Groups[1]
	if tool.Key != "error: enoent: no such file or directory, open '/tmp/build-#/out.log'" {
		t.Errorf("tool key = %q", tool.Key)
	}
	if tool.Count != 1 || tool.LastTs != ts[2] || len(tool.Samples) != 1 || tool.Samples[0].SessionID != 1 {
		t.Errorf("tool group = %+v", tool)
	}
	if tool.Example != "Error: ENOENT: no such file or directory, open '/tmp/build-4821/out.log'" {
		t.Errorf("tool example = %q", tool.Example)
	}

	// ?project= filter: only alpha's one api_error remains in that group.
	var alpha errorsDTO
	getJSON(t, srv.URL+"/api/stats/errors?project=-work-alpha", &alpha)
	for _, g := range alpha.Groups {
		if g.Key == "api error # overloaded (request id #)" && g.Count != 1 {
			t.Errorf("filtered api count = %d, want 1", g.Count)
		}
	}
}
