package logbuf

import (
	"bytes"
	"context"
	"log"
	"log/slog"
	"sync"
	"testing"
	"time"
)

func TestRingAppendAndSnapshotOrder(t *testing.T) {
	r := New(8)
	for i := 0; i < 5; i++ {
		r.Append("info", "ingest", "m")
	}
	got, last := r.Snapshot(0, "", 0)
	if len(got) != 5 {
		t.Fatalf("len = %d, want 5", len(got))
	}
	if last != 5 {
		t.Fatalf("lastID = %d, want 5", last)
	}
	// Oldest-first, ids monotonically increasing.
	for i, e := range got {
		if e.ID != int64(i+1) {
			t.Errorf("entry %d has id %d, want %d", i, e.ID, i+1)
		}
	}
}

func TestRingEviction(t *testing.T) {
	r := New(3)
	for i := 0; i < 10; i++ {
		r.Append("info", "t", "m")
	}
	got, last := r.Snapshot(0, "", 0)
	if len(got) != 3 {
		t.Fatalf("len = %d, want 3 (capacity)", len(got))
	}
	if last != 10 {
		t.Fatalf("lastID = %d, want 10", last)
	}
	// Only the newest 3 ids survive, oldest-first: 8, 9, 10.
	wantIDs := []int64{8, 9, 10}
	for i, e := range got {
		if e.ID != wantIDs[i] {
			t.Errorf("entry %d id = %d, want %d", i, e.ID, wantIDs[i])
		}
	}
}

func TestSnapshotSinceID(t *testing.T) {
	r := New(100)
	for i := 0; i < 10; i++ {
		r.Append("info", "t", "m")
	}
	got, _ := r.Snapshot(7, "", 0)
	if len(got) != 3 {
		t.Fatalf("since=7 len = %d, want 3 (ids 8,9,10)", len(got))
	}
	if got[0].ID != 8 {
		t.Errorf("first id = %d, want 8", got[0].ID)
	}
}

func TestSnapshotTagFilter(t *testing.T) {
	r := New(100)
	r.Append("info", "ingest", "a")
	r.Append("info", "dispatch", "b")
	r.Append("info", "ingest", "c")
	got, _ := r.Snapshot(0, "dispatch", 0)
	if len(got) != 1 || got[0].Msg != "b" {
		t.Fatalf("dispatch filter = %+v, want [b]", got)
	}
	got, _ = r.Snapshot(0, "ingest", 0)
	if len(got) != 2 {
		t.Fatalf("ingest filter len = %d, want 2", len(got))
	}
}

func TestSnapshotLimitKeepsNewest(t *testing.T) {
	r := New(100)
	for i := 0; i < 10; i++ {
		r.Append("info", "t", "m")
	}
	got, _ := r.Snapshot(0, "", 3)
	if len(got) != 3 {
		t.Fatalf("limit=3 len = %d, want 3", len(got))
	}
	// Newest three, oldest-first: 8,9,10.
	if got[0].ID != 8 || got[2].ID != 10 {
		t.Errorf("limited ids = %d..%d, want 8..10", got[0].ID, got[2].ID)
	}
}

func TestNormalisation(t *testing.T) {
	r := New(4)
	r.Append("", "", "m")
	got, _ := r.Snapshot(0, "", 0)
	if got[0].Level != "info" {
		t.Errorf("blank level → %q, want info", got[0].Level)
	}
	if got[0].Tag != "general" {
		t.Errorf("blank tag → %q, want general", got[0].Tag)
	}
	if got[0].TS == "" {
		t.Errorf("ts must be set")
	}
}

// TestConcurrentAppend is the -race guard: many goroutines appending while a
// reader snapshots must not race and must preserve id monotonicity.
func TestConcurrentAppend(t *testing.T) {
	r := New(256)
	var wg sync.WaitGroup
	for g := 0; g < 8; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 200; i++ {
				r.Append("info", "t", "m")
			}
		}()
	}
	// Concurrent reader.
	done := make(chan struct{})
	go func() {
		for {
			select {
			case <-done:
				return
			default:
				r.Snapshot(0, "", 10)
			}
		}
	}()
	wg.Wait()
	close(done)
	if r.LastID() != 8*200 {
		t.Errorf("lastID = %d, want %d", r.LastID(), 8*200)
	}
}

func TestHandlerTagsFromAttr(t *testing.T) {
	r := New(16)
	h := NewHandler(r, nil)
	logger := slog.New(h)
	logger.Info("hello", "tag", "dispatch", "k", "v")
	got, _ := r.Snapshot(0, "", 0)
	if len(got) != 1 {
		t.Fatalf("len = %d, want 1", len(got))
	}
	if got[0].Tag != "dispatch" {
		t.Errorf("tag = %q, want dispatch", got[0].Tag)
	}
	// slog's "INFO" is normalised to lowercase, matching the Writer path.
	if got[0].Level != "info" {
		t.Errorf("level = %q, want info", got[0].Level)
	}
	// Non-tag attrs are flattened into the message.
	if want := "hello k=v"; got[0].Msg != want {
		t.Errorf("msg = %q, want %q", got[0].Msg, want)
	}
}

func TestHandlerTagFromGroupAndTagged(t *testing.T) {
	r := New(16)
	base := slog.New(NewHandler(r, nil))

	base.WithGroup("ingest").Info("grouped")
	Tagged(base, "approvals").Warn("tagged")

	got, _ := r.Snapshot(0, "", 0)
	if len(got) != 2 {
		t.Fatalf("len = %d, want 2", len(got))
	}
	if got[0].Tag != "ingest" {
		t.Errorf("group tag = %q, want ingest", got[0].Tag)
	}
	if got[1].Tag != "approvals" {
		t.Errorf("Tagged tag = %q, want approvals", got[1].Tag)
	}
}

// TestHandlerForwardsToInner proves the tee still feeds a wrapped handler.
func TestHandlerForwardsToInner(t *testing.T) {
	r := New(16)
	var buf bytes.Buffer
	inner := slog.NewTextHandler(&buf, &slog.HandlerOptions{})
	logger := slog.New(NewHandler(r, inner))
	logger.Info("both")
	if buf.Len() == 0 {
		t.Errorf("inner handler received nothing")
	}
	if got, _ := r.Snapshot(0, "", 0); len(got) != 1 {
		t.Errorf("ring len = %d, want 1", len(got))
	}
	_ = context.Background()
}

func TestWriterLevelParsingAndMirror(t *testing.T) {
	r := New(16)
	var mirror bytes.Buffer
	w := NewWriter(r, "api", &mirror)
	lg := log.New(w, "", 0)

	lg.Println("plain line")
	lg.Println("warn: something")
	lg.Println("error: boom")
	lg.Println("warning: legacy")

	got, _ := r.Snapshot(0, "", 0)
	if len(got) != 4 {
		t.Fatalf("len = %d, want 4", len(got))
	}
	if got[0].Level != "info" || got[0].Tag != "api" {
		t.Errorf("line0 = %+v, want info/api", got[0])
	}
	if got[1].Level != "warn" {
		t.Errorf("warn line level = %q", got[1].Level)
	}
	if got[2].Level != "error" {
		t.Errorf("error line level = %q", got[2].Level)
	}
	if got[3].Level != "warn" {
		t.Errorf("warning line level = %q", got[3].Level)
	}
	// Mirror got the raw bytes too.
	if mirror.Len() == 0 {
		t.Errorf("mirror received nothing")
	}
}

func TestPhasefAndReadyfFormat(t *testing.T) {
	// Pure formatters — exact spec wording; they do NOT touch a ring.
	if s := Phasef("store.migrate", 12*time.Millisecond); s != "startup phase store.migrate: 12ms" {
		t.Errorf("Phasef = %q", s)
	}
	if s := Phasef("ingest.backfill", 340*time.Millisecond); s != "startup phase ingest.backfill: 340ms" {
		t.Errorf("Phasef = %q", s)
	}
	if s := Readyf(356 * time.Millisecond); s != "ready in 356ms" {
		t.Errorf("Readyf = %q", s)
	}
	// A second or more renders in Go's rounded form.
	if s := Readyf(1250 * time.Millisecond); s != "ready in 1.25s" {
		t.Errorf("Readyf(1.25s) = %q", s)
	}
}
