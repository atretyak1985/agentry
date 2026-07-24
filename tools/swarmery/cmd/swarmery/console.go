package main

// fusion phase 9 (Console/DX): the `swarmery status` and `swarmery console`
// subcommands. Both attach to a running daemon over its localhost HTTP+WS API
// via internal/console; the decision logic + rendering are unit-tested in that
// package, so these wrappers only parse flags, build the client, and map results
// to process exit codes (kept out of the CI coverage denominator per policy).

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/atretyak1985/swarmery/tools/swarmery/internal/console"
)

// consoleTarget resolves the daemon base URL from --url (explicit) or --port
// (else SWARMERY_PORT / default), shared by both subcommands.
func consoleTarget(fs *flag.FlagSet, args []string) string {
	url := fs.String("url", "", "daemon base URL (default http://127.0.0.1:<port>)")
	port := fs.Int("port", envPort(), "daemon port (env: SWARMERY_PORT)")
	fs.Parse(args)
	if *url != "" {
		return *url
	}
	return fmt.Sprintf("http://127.0.0.1:%d", *port)
}

// cmdStatus prints the one-shot daemon snapshot. Returns the process exit code:
// 0 when the daemon answered, 1 when it was unreachable (script-friendly).
func cmdStatus(args []string) int {
	fs := flag.NewFlagSet("status", flag.ExitOnError)
	base := consoleTarget(fs, args)

	client := console.NewHTTPClient(base)
	res, err := console.RunStatus(context.Background(), client, os.Stdout)
	if err != nil {
		fmt.Fprintf(os.Stderr, "status: %v\n", err)
		return 1
	}
	if !res.Reachable {
		return 1
	}
	return 0
}

// cmdConsole runs the interactive TUI until the user quits or a signal arrives.
func cmdConsole(args []string) error {
	fs := flag.NewFlagSet("console", flag.ExitOnError)
	base := consoleTarget(fs, args)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	client := console.NewHTTPClient(base)
	return console.Run(ctx, client)
}
