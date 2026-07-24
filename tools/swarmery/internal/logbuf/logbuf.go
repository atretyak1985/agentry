// Package logbuf is the daemon's in-memory structured-log ring (fusion phase 9,
// Console/DX). It keeps the last N log entries — {id, ts, level, tag, msg} — so
// `GET /api/logs` and the `swarmery console` TUI can render a live, filtered
// feed without touching disk. It exposes two writers into the ring:
//
//   - Handler: an slog.Handler that tees every structured record (with its
//     group/attr context flattened into the message) into the ring while still
//     forwarding to a wrapped handler, so stderr/launchd output is unchanged.
//   - Writer: an io.Writer adapter so the stdlib `log` package (used pervasively
//     across the daemon) is captured too, tagged with a caller-supplied subsystem.
//
// Everything is safe for concurrent use: Append and Snapshot take the same
// mutex, and ids increase monotonically so a poller can ask for "everything
// after id X".
package logbuf

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"sync"
	"time"
)

// DefaultCapacity is the ring size mandated by the phase-9 spec (1000 entries).
const DefaultCapacity = 1000

// Entry is one line in the ring. Field names are the /api/logs JSON contract.
type Entry struct {
	ID    int64  `json:"id"`
	TS    string `json:"ts"`    // RFC3339 UTC
	Level string `json:"level"` // debug | info | warn | error
	Tag   string `json:"tag"`   // subsystem: ingest, approvals, dispatch, …
	Msg   string `json:"msg"`
}

// Ring is a fixed-capacity, id-stamped, concurrency-safe log buffer.
type Ring struct {
	mu     sync.Mutex
	buf    []Entry
	next   int   // write cursor into buf (circular)
	filled bool  // whether buf has wrapped at least once
	lastID int64 // last id handed out (monotonic)
	now    func() time.Time
}

// New returns a ring holding up to capacity entries (capacity <= 0 → default).
func New(capacity int) *Ring {
	if capacity <= 0 {
		capacity = DefaultCapacity
	}
	return &Ring{buf: make([]Entry, capacity), now: time.Now}
}

// Append records one entry, assigns it the next id, and returns that id. The
// ts/level/tag are normalised (blank tag → "general", blank level → "info").
func (r *Ring) Append(level, tag, msg string) int64 {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.lastID++
	e := Entry{
		ID:    r.lastID,
		TS:    r.now().UTC().Format(time.RFC3339),
		Level: normLevel(level),
		Tag:   normTag(tag),
		Msg:   msg,
	}
	r.buf[r.next] = e
	r.next = (r.next + 1) % len(r.buf)
	if r.next == 0 {
		r.filled = true
	}
	return e.ID
}

// Snapshot returns entries in chronological (oldest→newest) order, optionally
// filtered: only ids strictly greater than sinceID, only the given tag (empty =
// all tags), and at most limit entries (limit <= 0 = no cap; when capped the
// NEWEST limit entries are returned). lastID is the highest id currently in the
// ring so a poller can pass it back as the next sinceID.
func (r *Ring) Snapshot(sinceID int64, tag string, limit int) (entries []Entry, lastID int64) {
	r.mu.Lock()
	defer r.mu.Unlock()

	all := r.orderedLocked()
	out := make([]Entry, 0, len(all))
	for _, e := range all {
		if e.ID <= sinceID {
			continue
		}
		if tag != "" && e.Tag != tag {
			continue
		}
		out = append(out, e)
	}
	if limit > 0 && len(out) > limit {
		out = out[len(out)-limit:] // keep the newest `limit`
	}
	return out, r.lastID
}

// LastID reports the highest id handed out so far (0 before the first Append).
func (r *Ring) LastID() int64 {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.lastID
}

// orderedLocked returns the live entries oldest-first. Caller holds r.mu.
func (r *Ring) orderedLocked() []Entry {
	if !r.filled {
		return append([]Entry(nil), r.buf[:r.next]...)
	}
	out := make([]Entry, 0, len(r.buf))
	out = append(out, r.buf[r.next:]...)
	out = append(out, r.buf[:r.next]...)
	return out
}

func normLevel(l string) string {
	if l == "" {
		return "info"
	}
	return strings.ToLower(l)
}

func normTag(t string) string {
	if t == "" {
		return "general"
	}
	return t
}

// ── slog.Handler tee ─────────────────────────────────────────────────────────

// tagKey is the slog attribute name that sets an entry's subsystem tag. Boot
// code logs e.g. slog.Info("listening", "tag", "api") or uses Tagged().
const tagKey = "tag"

// Handler is an slog.Handler that appends every record to a Ring (tagged by the
// record's "tag" attribute or an inherited group tag) and then forwards to a
// wrapped handler so existing stderr/launchd output is preserved.
type Handler struct {
	ring  *Ring
	inner slog.Handler
	tag   string      // inherited tag from WithAttrs/WithGroup
	attrs []slog.Attr // accumulated attrs (rendered into the ring message)
}

// NewHandler wraps inner so records tee into ring. If inner is nil the tee is
// terminal (records only go to the ring).
func NewHandler(ring *Ring, inner slog.Handler) *Handler {
	return &Handler{ring: ring, inner: inner}
}

// Enabled reports whether the level is handled. The ring wants everything; a
// stricter wrapped handler still gets only what it enables (checked in Handle),
// so returning true keeps the ring complete.
func (h *Handler) Enabled(_ context.Context, _ slog.Level) bool { return true }

// Handle appends to the ring, then forwards to the inner handler.
func (h *Handler) Handle(ctx context.Context, r slog.Record) error {
	tag := h.tag
	var extra []string
	// Render inherited attrs first, then record attrs; a "tag" attr sets the tag.
	for _, a := range h.attrs {
		if a.Key == tagKey {
			tag = a.Value.String()
			continue
		}
		extra = append(extra, a.Key+"="+a.Value.String())
	}
	r.Attrs(func(a slog.Attr) bool {
		if a.Key == tagKey {
			tag = a.Value.String()
			return true
		}
		extra = append(extra, a.Key+"="+a.Value.String())
		return true
	})
	msg := r.Message
	if len(extra) > 0 {
		msg = msg + " " + strings.Join(extra, " ")
	}
	h.ring.Append(r.Level.String(), tag, msg)

	if h.inner != nil && h.inner.Enabled(ctx, r.Level) {
		return h.inner.Handle(ctx, r)
	}
	return nil
}

// WithAttrs returns a handler that carries attrs (a "tag" attr updates the tag).
func (h *Handler) WithAttrs(attrs []slog.Attr) slog.Handler {
	nh := h.clone()
	for _, a := range attrs {
		if a.Key == tagKey {
			nh.tag = a.Value.String()
			continue
		}
		nh.attrs = append(nh.attrs, a)
	}
	if h.inner != nil {
		nh.inner = h.inner.WithAttrs(attrs)
	}
	return nh
}

// WithGroup treats the group name as the subsystem tag (its natural use in the
// boot path: slog.Default().WithGroup("dispatch")).
func (h *Handler) WithGroup(name string) slog.Handler {
	nh := h.clone()
	if name != "" {
		nh.tag = name
	}
	if h.inner != nil {
		nh.inner = h.inner.WithGroup(name)
	}
	return nh
}

func (h *Handler) clone() *Handler {
	cp := *h
	cp.attrs = append([]slog.Attr(nil), h.attrs...)
	return &cp
}

// Tagged returns an *slog.Logger whose records land under tag in the ring —
// sugar for slog.New(handler).With("tag", tag).
func Tagged(base *slog.Logger, tag string) *slog.Logger {
	return base.With(slog.String(tagKey, tag))
}

// ── io.Writer adapter for the stdlib log package ─────────────────────────────

// Writer adapts a Ring to io.Writer so `log.SetOutput` tees the daemon's many
// `log.Printf` lines into the ring under a fixed tag. It parses the leading
// "warn:"/"error:"/"warning:" convention the codebase already uses to set the
// level, and forwards the raw bytes to mirror (usually os.Stderr) so console and
// launchd logs both keep flowing.
type Writer struct {
	ring   *Ring
	tag    string
	mirror io.Writer
}

// NewWriter builds a log-package sink. mirror may be nil (ring only).
func NewWriter(ring *Ring, tag string, mirror io.Writer) *Writer {
	return &Writer{ring: ring, tag: tag, mirror: mirror}
}

func (w *Writer) Write(p []byte) (int, error) {
	line := strings.TrimRight(string(p), "\n")
	if line != "" {
		level := "info"
		switch {
		case strings.HasPrefix(line, "error:"):
			level = "error"
		case strings.HasPrefix(line, "warn:"), strings.HasPrefix(line, "warning:"):
			level = "warn"
		}
		w.ring.Append(level, w.tag, line)
	}
	if w.mirror != nil {
		return w.mirror.Write(p)
	}
	return len(p), nil
}

// ── startup-phase line formatters (exact spec text) ──────────────────────────
//
// These are PURE formatters — they do not touch the ring. The boot path logs
// their result through the tagged slog logger (logbuf.Tagged(…, "boot")), so a
// single slog call tees the line into both the ring and stderr/launchd. Kept as
// package funcs so the exact wording lives in one place and is unit-tested.

// Phasef renders a startup-phase line ("startup phase store.migrate: 12ms").
func Phasef(phase string, d time.Duration) string {
	return fmt.Sprintf("startup phase %s: %s", phase, roundDur(d))
}

// Readyf renders the final "ready in 356ms" boot line.
func Readyf(d time.Duration) string {
	return fmt.Sprintf("ready in %s", roundDur(d))
}

// roundDur renders a duration the way the spec lines do: whole ms under a
// second (12ms, 340ms), else Go's default rounded form (1.2s).
func roundDur(d time.Duration) string {
	if d < time.Second {
		return fmt.Sprintf("%dms", d.Round(time.Millisecond)/time.Millisecond)
	}
	return d.Round(time.Millisecond).String()
}
