package console

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// stubClient is the in-memory Client for model + status tests (no daemon, no TTY).
type stubClient struct {
	mu           sync.Mutex
	snap         Snapshot
	snapErr      error
	logs         []LogEntry
	resolveErr   error
	pauseErr     error
	resolveCalls []resolveCall
	pauseCalls   []bool
	base         string
}

type resolveCall struct {
	id     int64
	action string
}

func (s *stubClient) Snapshot(ctx context.Context) (Snapshot, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.snap, s.snapErr
}
func (s *stubClient) Logs(ctx context.Context, sinceID int64, tag string, limit int) ([]LogEntry, int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var last int64
	if len(s.logs) > 0 {
		last = s.logs[len(s.logs)-1].ID
	}
	return s.logs, last, nil
}
func (s *stubClient) ResolveApproval(ctx context.Context, id int64, action string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.resolveCalls = append(s.resolveCalls, resolveCall{id, action})
	return s.resolveErr
}
func (s *stubClient) PauseDispatch(ctx context.Context, paused bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pauseCalls = append(s.pauseCalls, paused)
	return s.pauseErr
}
func (s *stubClient) BaseURL() string {
	if s.base == "" {
		return "http://localhost:7777"
	}
	return s.base
}

// key builds a tea.KeyMsg for a single rune (drives handleKey in tests).
func key(r rune) tea.KeyMsg { return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}} }

// runCmd runs a Cmd synchronously and returns the produced Msg (nil-safe).
func runCmd(cmd tea.Cmd) tea.Msg {
	if cmd == nil {
		return nil
	}
	return cmd()
}

// step feeds one msg and returns the concrete Model back.
func step(m Model, msg tea.Msg) Model {
	next, _ := m.Update(msg)
	return next.(Model)
}

func TestSnapshotMsgPopulatesHeaderAndApprovals(t *testing.T) {
	m := NewModel(&stubClient{})
	snap := Snapshot{Reachable: true, Approvals: []Approval{{ID: 5, ToolName: "Bash", Status: "pending"}}}
	m = step(m, snapshotMsg{snap: snap})
	if m.StatusError() != "" {
		t.Errorf("statusErr = %q, want empty", m.StatusError())
	}
	if m.PendingCount() != 1 {
		t.Errorf("pending = %d, want 1", m.PendingCount())
	}
}

func TestSnapshotErrorSetsDownState(t *testing.T) {
	m := NewModel(&stubClient{})
	m = step(m, snapshotMsg{err: "daemon unreachable"})
	if m.StatusError() == "" {
		t.Errorf("statusErr must be set on a failed snapshot")
	}
	// headerText renders the down banner.
	if got := m.headerText(); got == "" || !contains(got, "unreachable") {
		t.Errorf("down header = %q, want the unreachable banner", got)
	}
}

func TestLogsMsgAppendsFeed(t *testing.T) {
	m := NewModel(&stubClient{})
	m = step(m, logsMsg{entries: []LogEntry{
		{ID: 1, Tag: "ingest", Msg: "a"},
		{ID: 2, Tag: "dispatch", Msg: "b"},
	}, lastID: 2})
	if m.FeedLen() != 2 {
		t.Fatalf("feed len = %d, want 2", m.FeedLen())
	}
	if m.lastLogID != 2 {
		t.Errorf("lastLogID = %d, want 2", m.lastLogID)
	}
}

func TestFeedTagFilterCycle(t *testing.T) {
	m := NewModel(&stubClient{})
	m = step(m, logsMsg{entries: []LogEntry{
		{ID: 1, Tag: "ingest", Msg: "a"},
		{ID: 2, Tag: "dispatch", Msg: "b"},
		{ID: 3, Tag: "ingest", Msg: "c"},
	}})
	// All visible initially.
	if len(m.visibleFeed()) != 3 {
		t.Fatalf("unfiltered visible = %d, want 3", len(m.visibleFeed()))
	}
	// [f] cycles "" → ingest (first knownTag).
	m = step(m, key('f'))
	if m.TagFilter() != "ingest" {
		t.Fatalf("after 1×f filter = %q, want ingest", m.TagFilter())
	}
	if len(m.visibleFeed()) != 2 {
		t.Errorf("ingest-filtered visible = %d, want 2", len(m.visibleFeed()))
	}
}

func TestWrapToggle(t *testing.T) {
	m := NewModel(&stubClient{})
	if !m.Wrap() {
		t.Fatalf("wrap default = false, want true")
	}
	m = step(m, key('w'))
	if m.Wrap() {
		t.Errorf("after w, wrap = true, want false")
	}
	m = step(m, key('w'))
	if !m.Wrap() {
		t.Errorf("after 2×w, wrap = false, want true")
	}
}

func TestApprovalSelectionMovesWithinPending(t *testing.T) {
	m := NewModel(&stubClient{})
	m = step(m, snapshotMsg{snap: Snapshot{Reachable: true, Approvals: []Approval{
		{ID: 10, Status: "pending"}, {ID: 11, Status: "pending"}, {ID: 12, Status: "pending"},
	}}})
	// pending() sorts newest-first: 12, 11, 10. Selection starts at 0 (id 12).
	if a, _ := m.SelectedApproval(); a.ID != 12 {
		t.Fatalf("initial selected = %d, want 12", a.ID)
	}
	m = step(m, tea.KeyMsg{Type: tea.KeyDown})
	if a, _ := m.SelectedApproval(); a.ID != 11 {
		t.Errorf("after ↓ selected = %d, want 11", a.ID)
	}
	// Clamp at the bottom.
	m = step(m, tea.KeyMsg{Type: tea.KeyDown})
	m = step(m, tea.KeyMsg{Type: tea.KeyDown})
	if a, _ := m.SelectedApproval(); a.ID != 10 {
		t.Errorf("clamped selected = %d, want 10 (bottom)", a.ID)
	}
	// Up returns toward the top.
	m = step(m, tea.KeyMsg{Type: tea.KeyUp})
	if a, _ := m.SelectedApproval(); a.ID != 11 {
		t.Errorf("after ↑ selected = %d, want 11", a.ID)
	}
}

func TestApproveResolvesSelectedEndToEnd(t *testing.T) {
	sc := &stubClient{}
	m := NewModel(sc)
	m = step(m, snapshotMsg{snap: Snapshot{Reachable: true, Approvals: []Approval{
		{ID: 42, ToolName: "Bash", Status: "pending"},
	}}})

	// Press 'y' → Cmd calls ResolveApproval(42, "approve").
	next, cmd := m.Update(key('y'))
	m = next.(Model)
	msg := runCmd(cmd)
	if len(sc.resolveCalls) != 1 || sc.resolveCalls[0] != (resolveCall{42, "approve"}) {
		t.Fatalf("resolve calls = %+v, want [{42 approve}]", sc.resolveCalls)
	}
	// Folding the actionMsg optimistically removes the approval + flashes.
	m = step(m, msg)
	if m.PendingCount() != 0 {
		t.Errorf("pending after approve = %d, want 0", m.PendingCount())
	}
	if m.Flash() != "approved #42" {
		t.Errorf("flash = %q, want 'approved #42'", m.Flash())
	}
}

func TestDenyResolvesSelected(t *testing.T) {
	sc := &stubClient{}
	m := NewModel(sc)
	m = step(m, snapshotMsg{snap: Snapshot{Reachable: true, Approvals: []Approval{{ID: 7, Status: "pending"}}}})
	_, cmd := m.Update(key('n'))
	msg := runCmd(cmd)
	if len(sc.resolveCalls) != 1 || sc.resolveCalls[0].action != "deny" {
		t.Fatalf("resolve calls = %+v, want a deny", sc.resolveCalls)
	}
	m = step(m, msg)
	if m.Flash() != "denied #7" {
		t.Errorf("flash = %q, want 'denied #7'", m.Flash())
	}
}

func TestApproveFailureKeepsApprovalAndFlashesError(t *testing.T) {
	sc := &stubClient{resolveErr: errors.New("boom")}
	m := NewModel(sc)
	m = step(m, snapshotMsg{snap: Snapshot{Reachable: true, Approvals: []Approval{{ID: 9, Status: "pending"}}}})
	_, cmd := m.Update(key('y'))
	m = step(m, runCmd(cmd))
	if m.PendingCount() != 1 {
		t.Errorf("failed approve should keep the approval; pending = %d, want 1", m.PendingCount())
	}
	if got := m.Flash(); !contains(got, "failed to approve") {
		t.Errorf("flash = %q, want a failure message", got)
	}
}

func TestApproveNoSelectionIsNoop(t *testing.T) {
	sc := &stubClient{}
	m := NewModel(sc)
	// No approvals loaded → 'y' produces no Cmd.
	_, cmd := m.Update(key('y'))
	if cmd != nil {
		if msg := runCmd(cmd); msg != nil {
			t.Errorf("approve with no selection produced %T, want no-op", msg)
		}
	}
	if len(sc.resolveCalls) != 0 {
		t.Errorf("resolve calls = %d, want 0", len(sc.resolveCalls))
	}
}

func TestPauseToggleFlipsHeaderChip(t *testing.T) {
	sc := &stubClient{}
	m := NewModel(sc)
	// Start unpaused.
	m = step(m, snapshotMsg{snap: Snapshot{Reachable: true}})
	if m.snap.Health.Dispatch.Paused {
		t.Fatalf("precondition: expected unpaused")
	}
	// 'p' → PauseDispatch(true).
	_, cmd := m.Update(key('p'))
	msg := runCmd(cmd)
	if len(sc.pauseCalls) != 1 || sc.pauseCalls[0] != true {
		t.Fatalf("pause calls = %+v, want [true]", sc.pauseCalls)
	}
	m = step(m, msg)
	if !m.snap.Health.Dispatch.Paused {
		t.Errorf("after pause, header chip should show paused")
	}
	if m.Flash() != "dispatcher paused" {
		t.Errorf("flash = %q", m.Flash())
	}
	// Toggling again requests resume.
	_, cmd = m.Update(key('p'))
	_ = runCmd(cmd)
	if sc.pauseCalls[1] != false {
		t.Errorf("second pause call = %v, want false (resume)", sc.pauseCalls[1])
	}
}

func TestReconnectState(t *testing.T) {
	m := NewModel(&stubClient{})
	if m.WSConnected() {
		t.Fatalf("initial WSConnected = true, want false")
	}
	m = step(m, wsConnMsg{connected: true})
	if !m.WSConnected() {
		t.Errorf("after connect, WSConnected = false")
	}
	// A drop flips it back → the View shows "reconnecting…".
	m = step(m, wsConnMsg{connected: false})
	if m.WSConnected() {
		t.Errorf("after drop, WSConnected = true, want false")
	}
}

func TestWSPermissionEventsUpdateApprovals(t *testing.T) {
	m := NewModel(&stubClient{})
	// A permission_requested WS frame surfaces a new pending approval.
	req, _ := json.Marshal(Approval{ID: 100, ToolName: "Write", Status: "pending"})
	m = step(m, wsMsg{evt: WSEvent{Type: "permission_requested", Payload: req}})
	if m.PendingCount() != 1 {
		t.Fatalf("after permission_requested, pending = %d, want 1", m.PendingCount())
	}
	// A permission_resolved frame removes it.
	res, _ := json.Marshal(Approval{ID: 100})
	m = step(m, wsMsg{evt: WSEvent{Type: "permission_resolved", Payload: res}})
	if m.PendingCount() != 0 {
		t.Errorf("after permission_resolved, pending = %d, want 0", m.PendingCount())
	}
	// Both events also left feed lines.
	if m.FeedLen() < 2 {
		t.Errorf("feed len = %d, want >= 2", m.FeedLen())
	}
}

func TestQuitKey(t *testing.T) {
	m := NewModel(&stubClient{})
	next, cmd := m.Update(key('q'))
	m = next.(Model)
	if !m.Quitting() {
		t.Errorf("q should set quitting")
	}
	// The Cmd is tea.Quit — executing it yields tea.QuitMsg.
	if _, ok := runCmd(cmd).(tea.QuitMsg); !ok {
		t.Errorf("q Cmd = %T, want tea.QuitMsg", runCmd(cmd))
	}
}

func TestViewRendersWithoutPanic(t *testing.T) {
	// The View has no logic but must not panic across states (down + live).
	m := NewModel(&stubClient{})
	m = step(m, tea.WindowSizeMsg{Width: 100, Height: 30})
	_ = m.View()
	m = step(m, snapshotMsg{err: "down"})
	_ = m.View()
	m = step(m, snapshotMsg{snap: Snapshot{Reachable: true, Approvals: []Approval{{ID: 1, ToolName: "Bash", RequestJSON: `{"cwd":"/x"}`, Status: "pending"}}}})
	m = step(m, wsConnMsg{connected: true})
	if out := m.View(); out == "" {
		t.Errorf("live View rendered empty")
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || indexOf(s, sub) >= 0)
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
