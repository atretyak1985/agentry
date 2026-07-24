package console

import (
	"context"
	"fmt"
	"io"
)

// StatusResult is the outcome of RunStatus: the rendered block and whether the
// daemon was reachable (the CLI maps unreachable → exit 1).
type StatusResult struct {
	Text      string
	Reachable bool
}

// RunStatus fetches a one-shot snapshot from the daemon at baseURL, augments it
// with the dispatcher slot total when available, renders the status block, and
// writes it to out. It never returns an error for an unreachable daemon — that
// is a normal, script-friendly outcome reported via StatusResult.Reachable
// (false) so the caller exits nonzero with the down banner already printed.
//
// Split from the CLI wiring so it unit-tests against a stub or an httptest
// daemon without spawning a process.
func RunStatus(ctx context.Context, client Client, out io.Writer) (StatusResult, error) {
	snap, err := client.Snapshot(ctx)
	snap = snap.WithURL(client.BaseURL())

	// Best-effort dispatcher slot total for the "N running / M slots" line.
	if d, ok := client.(dispatchReader); ok && snap.Reachable {
		if st, derr := d.DispatchStatus(ctx); derr == nil {
			snap = snap.WithMaxSlots(st.FreeSlots + st.ActiveRuns)
		}
	}

	text := RenderStatus(snap)
	if _, werr := fmt.Fprintln(out, text); werr != nil {
		return StatusResult{}, werr
	}
	// err (from Snapshot) is only the unreachable signal; surface it as
	// Reachable=false, not as a hard error.
	_ = err
	return StatusResult{Text: text, Reachable: snap.Reachable}, nil
}

// dispatchReader is the optional capability RunStatus uses to enrich the slots
// figure. The HTTPClient implements it; a plain stub need not.
type dispatchReader interface {
	DispatchStatus(ctx context.Context) (Dispatch, error)
}
