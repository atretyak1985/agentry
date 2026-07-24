package console

import (
	"fmt"
	"strings"
	"time"
)

// RenderStatus formats the three-line status block from a Snapshot, matching the
// normative layout in phase-9-console.md:
//
//	swarmery v0.1.0   up 3h12m   db 84 MB   migrations 0028   ingest lag 2s
//	today: 14 sessions · $4.12 · 1.2M tok    dispatch: 1 running / 2 slots (active)
//	approvals pending: 2   ws clients: 1     url http://localhost:7777
//
// Pure function (no I/O) so it unit-tests exactly. When snap.Reachable is false
// it returns the single-line down banner instead (see RenderDown).
func RenderStatus(snap Snapshot) string {
	if !snap.Reachable {
		return RenderDown(snap)
	}
	h := snap.Health
	var b strings.Builder

	fmt.Fprintf(&b, "swarmery v%s   up %s   db %s   migrations %04d   ingest lag %s\n",
		h.Version,
		humanUptime(time.Duration(h.UptimeSec)*time.Second),
		humanBytes(h.DBSizeBytes),
		h.MigrationVersion,
		humanLag(h.IngestLagSec),
	)

	dispatchState := "idle"
	if h.Dispatch.Paused {
		dispatchState = "paused"
	} else if h.Dispatch.Active > 0 {
		dispatchState = "active"
	}
	slots := snap.Stats // alias for readability below
	fmt.Fprintf(&b, "today: %d sessions · %s · %s tok    dispatch: %d running / %d slots (%s)\n",
		slots.Sessions,
		humanCost(slots.CostUSD),
		humanTokens(slots.TotalTokens()),
		h.Dispatch.Active,
		maxSlots(snap),
		dispatchState,
	)

	fmt.Fprintf(&b, "approvals pending: %d   ws clients: %d     url %s",
		countPending(snap.Approvals),
		h.WSClients,
		snap.dashboardURL(),
	)
	return b.String()
}

// RenderDown is the single red-worthy line `swarmery status` prints (and the
// TUI header shows) when the daemon can't be reached.
func RenderDown(snap Snapshot) string {
	url := snap.dashboardURL()
	return fmt.Sprintf("swarmery daemon unreachable at %s — is it running? (`swarmery service-status`)", url)
}

// dashboardURL is where the daemon serves; the CLI passes it through (WithURL)
// so the URL shown always matches the client's target.
func (s Snapshot) dashboardURL() string {
	if s.url != "" {
		return s.url
	}
	return "http://localhost:7777"
}

// maxSlots derives the total dispatcher slots. The health endpoint reports only
// active; free slots come from GET /api/dispatch when the console has it, else
// active is the floor we can prove — but the status block prefers the dispatch
// snapshot's MaxConcurrent when present via snap.maxSlots.
func maxSlots(snap Snapshot) int {
	if snap.maxSlots > 0 {
		return snap.maxSlots
	}
	// Fall back to active (never report fewer slots than are running).
	return snap.Health.Dispatch.Active
}

func countPending(as []Approval) int {
	n := 0
	for _, a := range as {
		if a.Status == "" || a.Status == "pending" {
			n++
		}
	}
	return n
}

// ── human formatting helpers ──

func humanUptime(d time.Duration) string {
	if d <= 0 {
		return "0m"
	}
	h := int(d.Hours())
	m := int(d.Minutes()) % 60
	switch {
	case h >= 24:
		return fmt.Sprintf("%dd%dh", h/24, h%24)
	case h > 0:
		return fmt.Sprintf("%dh%02dm", h, m)
	default:
		return fmt.Sprintf("%dm", m)
	}
}

func humanBytes(n int64) string {
	const unit = 1000 // MB as the block shows (84 MB), decimal for readability
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(unit), 0
	for x := n / unit; x >= unit; x /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.0f %cB", float64(n)/float64(div), "kMGTPE"[exp])
}

func humanTokens(n int64) string {
	switch {
	case n >= 1_000_000:
		return fmt.Sprintf("%.1fM", float64(n)/1_000_000)
	case n >= 1_000:
		return fmt.Sprintf("%.1fK", float64(n)/1_000)
	default:
		return fmt.Sprintf("%d", n)
	}
}

func humanCost(c *float64) string {
	if c == nil {
		return "$0.00"
	}
	return fmt.Sprintf("$%.2f", *c)
}

func humanLag(lag *int64) string {
	if lag == nil {
		return "—"
	}
	d := time.Duration(*lag) * time.Second
	switch {
	case d >= time.Hour:
		return fmt.Sprintf("%dh", int(d.Hours()))
	case d >= time.Minute:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	default:
		return fmt.Sprintf("%ds", *lag)
	}
}
