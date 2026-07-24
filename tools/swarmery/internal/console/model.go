package console

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

// Model is the `swarmery console` bubbletea model. All state transitions live in
// Update and are driven by typed messages, so the whole thing is unit-testable
// by feeding synthetic tea.Msg values — no TTY, no daemon (the View is the only
// TTY-touching part and carries no logic). The Client seam is stubbed in tests.
//
// Panes: header (status block, refreshed 5s) · left Events feed (WS + logs,
// tag-colored, wrap/filter toggles) · right Approvals list (select + y/n).
// Hotkeys: y approve · n deny · p pause · o open dashboard · w wrap · f filter ·
// ↑/↓ select · q quit.
type Model struct {
	client Client

	// header/status
	snap        Snapshot
	statusErr   string // last snapshot error (daemon-down banner)
	wsConnected bool   // WS stream state → "live" vs "reconnecting…"

	// events feed (merged WS events + log ring), newest last
	events    []feedLine
	lastLogID int64
	wrap      bool
	tagFilter string // "" = all; cycles through knownTags

	// approvals
	approvals []Approval
	selected  int    // index into the filtered pending list
	flash     string // transient status line (e.g. "approved #12")

	// terminal geometry (from tea.WindowSizeMsg); 0 until the first resize
	width, height int

	quitting bool
}

// feedLine is one rendered line in the Events pane.
type feedLine struct {
	tag   string
	level string
	text  string
}

// knownTags is the filter cycle order for [f]. "" (all) is prepended at runtime.
var knownTags = []string{"ingest", "approvals", "dispatch", "verify", "routines", "provision", "api", "wsingest", "boot"}

// NewModel builds the initial model bound to client.
func NewModel(client Client) Model {
	return Model{client: client, wrap: true, selected: 0}
}

// ── messages ──

type (
	// tickMsg fires every refresh interval to repoll the header + approvals.
	tickMsg struct{}
	// snapshotMsg carries a fetched status snapshot (or an error string).
	snapshotMsg struct {
		snap Snapshot
		err  string
	}
	// logsMsg carries newly-fetched log-ring entries.
	logsMsg struct {
		entries []LogEntry
		lastID  int64
	}
	// wsMsg is a decoded WS envelope; the model turns notable ones into feed
	// lines and schedules a refetch of approvals/header.
	wsMsg struct{ evt WSEvent }
	// wsConnMsg reports the WS stream connecting/dropping (reconnect banner).
	wsConnMsg struct{ connected bool }
	// actionMsg reports the result of an approve/deny/pause action.
	actionMsg struct {
		kind string // "approve" | "deny" | "pause"
		id   int64
		ok   bool
		err  string
	}
)

// Init satisfies tea.Model. The real Cmds (poll ticker, WS stream) are wired in
// run.go; here Init is nil so tests drive Update directly.
func (m Model) Init() tea.Cmd { return nil }

// Update is the pure reducer. It returns the next model and an optional Cmd; in
// tests the Cmd is usually ignored (state assertions) or executed synchronously.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		return m, nil

	case tea.KeyMsg:
		return m.handleKey(msg)

	case snapshotMsg:
		m.snap = msg.snap
		m.statusErr = msg.err
		if msg.err == "" {
			m.approvals = msg.snap.Approvals
			m.clampSelection()
		}
		return m, nil

	case logsMsg:
		for _, e := range msg.entries {
			m.appendFeed(feedLine{tag: e.Tag, level: e.Level, text: e.Msg})
		}
		if msg.lastID > m.lastLogID {
			m.lastLogID = msg.lastID
		}
		return m, nil

	case wsMsg:
		m.ingestWSEvent(msg.evt)
		return m, nil

	case wsConnMsg:
		m.wsConnected = msg.connected
		return m, nil

	case actionMsg:
		m.applyActionResult(msg)
		return m, nil
	}
	return m, nil
}

// handleKey maps hotkeys to state changes / Cmds. Cmds that need the client are
// returned as closures (executed by the runtime, or by tests that want them).
func (m Model) handleKey(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch k.String() {
	case "q", "ctrl+c":
		m.quitting = true
		return m, tea.Quit
	case "up", "k":
		if m.selected > 0 {
			m.selected--
		}
		return m, nil
	case "down", "j":
		if m.selected < len(m.pending())-1 {
			m.selected++
		}
		return m, nil
	case "w":
		m.wrap = !m.wrap
		return m, nil
	case "f":
		m.cycleFilter()
		return m, nil
	case "y":
		return m, m.resolveSelected("approve")
	case "n":
		return m, m.resolveSelected("deny")
	case "p":
		return m, m.togglePauseCmd()
	case "o":
		return m, m.openDashboardCmd()
	}
	return m, nil
}

// ── approvals helpers ──

// pending returns the currently-pending approvals (status pending/empty).
func (m Model) pending() []Approval {
	out := make([]Approval, 0, len(m.approvals))
	for _, a := range m.approvals {
		if a.Status == "" || a.Status == "pending" {
			out = append(out, a)
		}
	}
	// Stable newest-first ordering by id keeps selection intuitive.
	sort.Slice(out, func(i, j int) bool { return out[i].ID > out[j].ID })
	return out
}

// SelectedApproval returns the approval under the cursor, ok=false when none.
func (m Model) SelectedApproval() (Approval, bool) {
	p := m.pending()
	if m.selected < 0 || m.selected >= len(p) {
		return Approval{}, false
	}
	return p[m.selected], true
}

func (m *Model) clampSelection() {
	if n := len(m.pending()); m.selected >= n {
		m.selected = maxInt(0, n-1)
	}
	if m.selected < 0 {
		m.selected = 0
	}
}

// resolveSelected builds the Cmd that approves/denies the selected request. When
// nothing is selected it returns nil (a no-op keypress).
func (m Model) resolveSelected(action string) tea.Cmd {
	a, ok := m.SelectedApproval()
	if !ok {
		return nil
	}
	client := m.client
	id := a.ID
	return func() tea.Msg {
		err := client.ResolveApproval(ctxBackground(), id, action)
		return actionMsg{kind: action, id: id, ok: err == nil, err: errString(err)}
	}
}

func (m Model) togglePauseCmd() tea.Cmd {
	client := m.client
	want := !m.snap.Health.Dispatch.Paused
	return func() tea.Msg {
		err := client.PauseDispatch(ctxBackground(), want)
		return actionMsg{kind: "pause", ok: err == nil, err: errString(err)}
	}
}

func (m Model) openDashboardCmd() tea.Cmd {
	url := m.client.BaseURL()
	return func() tea.Msg {
		_ = openBrowser(url) // best-effort; failure is silent (no feed spam)
		return nil
	}
}

// applyActionResult folds an action outcome into the model: optimistic removal
// of a resolved approval, a flash line, and an immediate dispatch-pause flip so
// the header chip reflects the toggle before the next 5s refresh.
func (m *Model) applyActionResult(msg actionMsg) {
	switch msg.kind {
	case "approve", "deny":
		if msg.ok {
			m.removeApproval(msg.id)
			if msg.kind == "approve" {
				m.flash = fmt.Sprintf("approved #%d", msg.id)
			} else {
				m.flash = fmt.Sprintf("denied #%d", msg.id)
			}
			m.clampSelection()
		} else {
			m.flash = fmt.Sprintf("failed to %s #%d: %s", msg.kind, msg.id, msg.err)
		}
	case "pause":
		if msg.ok {
			m.snap.Health.Dispatch.Paused = !m.snap.Health.Dispatch.Paused
			if m.snap.Health.Dispatch.Paused {
				m.flash = "dispatcher paused"
			} else {
				m.flash = "dispatcher resumed"
			}
		} else {
			m.flash = "pause failed: " + msg.err
		}
	}
}

func (m *Model) removeApproval(id int64) {
	out := m.approvals[:0]
	for _, a := range m.approvals {
		if a.ID != id {
			out = append(out, a)
		}
	}
	m.approvals = out
}

// ── events feed helpers ──

const maxFeed = 500 // cap the retained feed so a long session bounds memory

func (m *Model) appendFeed(l feedLine) {
	m.events = append(m.events, l)
	if len(m.events) > maxFeed {
		m.events = m.events[len(m.events)-maxFeed:]
	}
}

// ingestWSEvent turns a frozen WS envelope into a feed line. It never introduces
// a new message type — it reads only the type + a couple of known payload fields.
func (m *Model) ingestWSEvent(evt WSEvent) {
	switch evt.Type {
	case "permission_requested":
		// A new pending request: surface it and let the next poll hydrate the
		// full list (payload is the PermissionRequest DTO).
		var p Approval
		_ = json.Unmarshal(evt.Payload, &p)
		if p.ID != 0 {
			m.upsertApproval(p)
			m.clampSelection()
		}
		m.appendFeed(feedLine{tag: "approvals", level: "info", text: "permission requested: " + p.ToolName})
	case "permission_resolved":
		var p Approval
		_ = json.Unmarshal(evt.Payload, &p)
		if p.ID != 0 {
			m.removeApproval(p.ID)
			m.clampSelection()
		}
		m.appendFeed(feedLine{tag: "approvals", level: "info", text: fmt.Sprintf("permission resolved: #%d", p.ID)})
	case "task_updated":
		m.appendFeed(feedLine{tag: "dispatch", level: "info", text: "task updated"})
	case "session_started", "session_updated":
		m.appendFeed(feedLine{tag: "ingest", level: "info", text: evt.Type})
	case "event_appended":
		// High-volume; only note it when unfiltered-verbose would be desired.
		m.appendFeed(feedLine{tag: "ingest", level: "debug", text: "event appended"})
	}
}

// upsertApproval adds a approval if new, or updates it in place by id.
func (m *Model) upsertApproval(a Approval) {
	for i := range m.approvals {
		if m.approvals[i].ID == a.ID {
			m.approvals[i] = a
			return
		}
	}
	m.approvals = append(m.approvals, a)
}

// cycleFilter advances the tag filter through ["" , knownTags...] and wraps.
func (m *Model) cycleFilter() {
	if m.tagFilter == "" {
		m.tagFilter = knownTags[0]
		return
	}
	for i, t := range knownTags {
		if t == m.tagFilter {
			if i+1 >= len(knownTags) {
				m.tagFilter = "" // wrap back to all
			} else {
				m.tagFilter = knownTags[i+1]
			}
			return
		}
	}
	m.tagFilter = ""
}

// visibleFeed applies the tag filter to the retained feed.
func (m Model) visibleFeed() []feedLine {
	if m.tagFilter == "" {
		return m.events
	}
	out := make([]feedLine, 0, len(m.events))
	for _, l := range m.events {
		if l.tag == m.tagFilter {
			out = append(out, l)
		}
	}
	return out
}

// ── small utilities ──

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func errString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

// TagFilter / Wrap / WSConnected / Flash expose model state for tests + the View.
func (m Model) TagFilter() string   { return m.tagFilter }
func (m Model) Wrap() bool          { return m.wrap }
func (m Model) WSConnected() bool   { return m.wsConnected }
func (m Model) Flash() string       { return m.flash }
func (m Model) Quitting() bool      { return m.quitting }
func (m Model) FeedLen() int        { return len(m.events) }
func (m Model) PendingCount() int   { return len(m.pending()) }
func (m Model) Selected() int       { return m.selected }
func (m Model) StatusError() string { return m.statusErr }

// headerText renders the status block (or down banner) for the View.
func (m Model) headerText() string {
	if m.statusErr != "" {
		return RenderDown(m.snap)
	}
	return RenderStatus(m.snap)
}

// feedText renders the visible feed as plain lines (View wraps it in a pane).
func (m Model) feedText() string {
	var b strings.Builder
	for _, l := range m.visibleFeed() {
		fmt.Fprintf(&b, "[%s] %s\n", l.tag, l.text)
	}
	return b.String()
}
