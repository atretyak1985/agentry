package console

import (
	"context"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// refreshInterval is the header/approvals poll cadence (spec: 5s).
const refreshInterval = 5 * time.Second

// streamer is the subset of HTTPClient run.go needs beyond the Client interface:
// the live WS stream. The real client implements it; nil-safe callers skip the
// stream when absent (e.g. a Client stub without WS).
type streamer interface {
	StreamEvents(ctx context.Context, out chan<- WSEvent) error
}

// Run builds and runs the interactive `swarmery console` program against client.
// It wires two background feeds into the bubbletea runtime:
//   - a 5s poll of the status snapshot + log ring (tea tick Cmds),
//   - the WS event stream with reconnect/backoff (a goroutine that Sends frames).
//
// This function touches the TTY (tea.NewProgram with the alt-screen) and the
// process lifetime, so it is deliberately kept out of the unit-tested surface;
// all decision logic lives in Model.Update, which the tests drive directly.
func Run(ctx context.Context, client Client) error {
	m := NewModel(client)
	p := tea.NewProgram(m, tea.WithAltScreen(), tea.WithContext(ctx))

	// Initial snapshot + log fetch, then the recurring ticker, are driven by the
	// program via Cmds we inject on start using a tiny wrapper model would be
	// heavier; instead we push the first refresh + start the ticker goroutine.
	go pollLoop(ctx, client, p)

	if s, ok := client.(streamer); ok {
		go streamLoop(ctx, s, p)
	}

	_, err := p.Run()
	return err
}

// programSender is the piece of *tea.Program the loops use (Send). An interface
// so tests could substitute a recorder, though the loops themselves are not unit
// targets.
type programSender interface{ Send(tea.Msg) }

// pollLoop pushes an immediate refresh, then one every refreshInterval, until
// ctx is done. Each refresh fetches the snapshot and any new log entries.
func pollLoop(ctx context.Context, client Client, p programSender) {
	var lastLogID int64
	refresh := func() {
		snap, err := client.Snapshot(ctx)
		snap = snap.WithURL(client.BaseURL())
		p.Send(snapshotMsg{snap: snap, err: errString(err)})

		entries, last, lerr := client.Logs(ctx, lastLogID, "", 200)
		if lerr == nil && len(entries) > 0 {
			lastLogID = last
			p.Send(logsMsg{entries: entries, lastID: last})
		}
	}
	refresh()
	t := time.NewTicker(refreshInterval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			refresh()
		}
	}
}

// streamLoop maintains the WS stream: on drop it reports the disconnect (so the
// header chip flips to "reconnecting…") and retries with capped backoff until
// ctx is done. Frames are forwarded into the program as wsMsg.
func streamLoop(ctx context.Context, s streamer, p programSender) {
	backoff := 500 * time.Millisecond
	const maxBackoff = 10 * time.Second
	for {
		if ctx.Err() != nil {
			return
		}
		frames := make(chan WSEvent, 64)
		// Pump frames into the program while the stream is alive.
		streamCtx, cancel := context.WithCancel(ctx)
		go func() {
			for {
				select {
				case <-streamCtx.Done():
					return
				case evt, ok := <-frames:
					if !ok {
						return
					}
					p.Send(wsMsg{evt: evt})
				}
			}
		}()

		p.Send(wsConnMsg{connected: true})
		err := s.StreamEvents(streamCtx, frames)
		cancel()
		p.Send(wsConnMsg{connected: false})

		if err == nil || ctx.Err() != nil {
			return
		}
		// Backoff before reconnecting.
		select {
		case <-ctx.Done():
			return
		case <-time.After(backoff):
		}
		if backoff < maxBackoff {
			backoff *= 2
			if backoff > maxBackoff {
				backoff = maxBackoff
			}
		}
	}
}
