package console

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"
)

func f64(v float64) *float64 { return &v }
func i64(v int64) *int64     { return &v }

func TestRenderStatusBlockLayout(t *testing.T) {
	var snap Snapshot
	snap.Reachable = true
	snap.Health.Version = "0.1.0"
	snap.Health.UptimeSec = 3*3600 + 12*60 // 3h12m
	snap.Health.DBSizeBytes = 84_000_000   // 84 MB
	snap.Health.MigrationVersion = 28
	snap.Health.IngestLagSec = i64(2)
	snap.Health.WSClients = 1
	snap.Health.Dispatch.Active = 1
	snap.Stats.Sessions = 14
	snap.Stats.CostUSD = f64(4.12)
	snap.Stats.TokensIn = 700_000
	snap.Stats.TokensOut = 500_000 // 1.2M total
	snap = snap.WithURL("http://localhost:7777").WithMaxSlots(2)

	out := RenderStatus(snap)
	lines := strings.Split(out, "\n")
	if len(lines) != 3 {
		t.Fatalf("status block has %d lines, want 3:\n%s", len(lines), out)
	}

	// Line 1: identity + uptime + db + migrations + lag.
	l1 := lines[0]
	for _, want := range []string{"swarmery v0.1.0", "up 3h12m", "db 84 MB", "migrations 0028", "ingest lag 2s"} {
		if !strings.Contains(l1, want) {
			t.Errorf("line1 missing %q:\n%s", want, l1)
		}
	}
	// Line 2: today + dispatch.
	l2 := lines[1]
	for _, want := range []string{"today: 14 sessions", "$4.12", "1.2M tok", "dispatch: 1 running / 2 slots (active)"} {
		if !strings.Contains(l2, want) {
			t.Errorf("line2 missing %q:\n%s", want, l2)
		}
	}
	// Line 3: approvals + ws clients + url.
	l3 := lines[2]
	for _, want := range []string{"approvals pending:", "ws clients: 1", "url http://localhost:7777"} {
		if !strings.Contains(l3, want) {
			t.Errorf("line3 missing %q:\n%s", want, l3)
		}
	}
}

func TestRenderStatusDispatchStates(t *testing.T) {
	base := Snapshot{Reachable: true}
	base.Health.Version = "0.1.0"

	// idle
	if got := RenderStatus(base); !strings.Contains(got, "(idle)") {
		t.Errorf("no dispatch → want (idle):\n%s", got)
	}
	// active
	act := base
	act.Health.Dispatch.Active = 2
	if got := RenderStatus(act); !strings.Contains(got, "(active)") {
		t.Errorf("active runs → want (active):\n%s", got)
	}
	// paused overrides active
	pau := act
	pau.Health.Dispatch.Paused = true
	if got := RenderStatus(pau); !strings.Contains(got, "(paused)") {
		t.Errorf("paused → want (paused):\n%s", got)
	}
}

func TestRenderStatusNilLagAndCost(t *testing.T) {
	snap := Snapshot{Reachable: true}
	snap.Health.Version = "0.1.0"
	// nil ingest lag → "—"; nil cost → "$0.00".
	out := RenderStatus(snap)
	if !strings.Contains(out, "ingest lag —") {
		t.Errorf("nil lag should render — :\n%s", out)
	}
	if !strings.Contains(out, "$0.00") {
		t.Errorf("nil cost should render $0.00:\n%s", out)
	}
}

func TestRenderDownBanner(t *testing.T) {
	snap := Snapshot{Reachable: false}
	snap = snap.WithURL("http://127.0.0.1:9999")
	out := RenderStatus(snap) // routes to RenderDown
	if !strings.Contains(out, "unreachable") || !strings.Contains(out, "127.0.0.1:9999") {
		t.Errorf("down banner = %q", out)
	}
	if strings.Contains(out, "\n") {
		t.Errorf("down banner should be a single line: %q", out)
	}
}

func TestHumanFormatters(t *testing.T) {
	// Uptime buckets.
	if got := humanUptime(0); got != "0m" {
		t.Errorf("zero uptime = %q, want 0m", got)
	}
	if got := humanUptime(dur(5 * 60)); got != "5m" {
		t.Errorf("5m uptime = %q", got)
	}
	if got := humanUptime(dur(3*3600 + 12*60)); got != "3h12m" {
		t.Errorf("3h12m uptime = %q", got)
	}
	if got := humanUptime(dur(25 * 3600)); got != "1d1h" {
		t.Errorf("25h uptime = %q, want 1d1h", got)
	}
	// Tokens.
	if got := humanTokens(1_200_000); got != "1.2M" {
		t.Errorf("tokens = %q", got)
	}
	if got := humanTokens(3400); got != "3.4K" {
		t.Errorf("tokens = %q", got)
	}
	if got := humanTokens(42); got != "42" {
		t.Errorf("tokens = %q", got)
	}
	// Bytes.
	if got := humanBytes(84_000_000); got != "84 MB" {
		t.Errorf("bytes = %q", got)
	}
	// Lag buckets.
	if got := humanLag(i64(2)); got != "2s" {
		t.Errorf("lag = %q", got)
	}
	if got := humanLag(i64(120)); got != "2m" {
		t.Errorf("lag = %q", got)
	}
	if got := humanLag(nil); got != "—" {
		t.Errorf("nil lag = %q", got)
	}
}

// ── RunStatus (the `swarmery status` command core) ──

func TestRunStatusReachablePrintsBlockExitZero(t *testing.T) {
	sc := &stubClient{snap: Snapshot{Reachable: true}}
	sc.snap.Health.Version = "0.1.0"
	var buf bytes.Buffer
	res, err := RunStatus(context.Background(), sc, &buf)
	if err != nil {
		t.Fatalf("RunStatus err = %v", err)
	}
	if !res.Reachable {
		t.Errorf("Reachable = false, want true")
	}
	if !strings.Contains(buf.String(), "swarmery v0.1.0") {
		t.Errorf("output missing the status block:\n%s", buf.String())
	}
}

func TestRunStatusUnreachablePrintsBannerExitOne(t *testing.T) {
	// Snapshot returns Reachable=false → the CLI maps this to exit 1.
	sc := &stubClient{snap: Snapshot{Reachable: false}}
	var buf bytes.Buffer
	res, err := RunStatus(context.Background(), sc, &buf)
	if err != nil {
		t.Fatalf("RunStatus err = %v (should not hard-error on down daemon)", err)
	}
	if res.Reachable {
		t.Errorf("Reachable = true, want false")
	}
	if !strings.Contains(buf.String(), "unreachable") {
		t.Errorf("output missing the down banner:\n%s", buf.String())
	}
}

// dur builds a time.Duration from whole seconds for the uptime cases.
func dur(seconds int) time.Duration { return time.Duration(seconds) * time.Second }
