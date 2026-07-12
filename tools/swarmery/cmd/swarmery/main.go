// Command swarmery is the control-plane daemon CLI:
//
//	swarmery ingest <file.jsonl>   parse one transcript into the local DB
//	swarmery serve                 serve the REST API + embedded SPA
//	swarmery recost                recompute cost_usd for all turns
package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/atretyak1985/swarmery/tools/swarmery/internal/api"
	"github.com/atretyak1985/swarmery/tools/swarmery/internal/cost"
	"github.com/atretyak1985/swarmery/tools/swarmery/internal/ingest"
	"github.com/atretyak1985/swarmery/tools/swarmery/internal/store"
)

const defaultPort = 7777

func main() {
	log.SetFlags(0)
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	var err error
	switch os.Args[1] {
	case "ingest":
		err = cmdIngest(os.Args[2:])
	case "serve":
		err = cmdServe(os.Args[2:])
	case "recost":
		err = cmdRecost(os.Args[2:])
	case "-h", "--help", "help":
		usage()
	default:
		fmt.Fprintf(os.Stderr, "unknown command %q\n", os.Args[1])
		usage()
		os.Exit(2)
	}
	if err != nil {
		log.Fatalf("error: %v", err)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, `usage:
  swarmery ingest [--db <path>] <file.jsonl>
  swarmery serve  [--db <path>] [--port <n>]   (env: SWARMERY_PORT)
  swarmery recost [--db <path>]                (env: SWARMERY_PRICING overrides embedded pricing)`)
}

func dbFlag(fs *flag.FlagSet) *string {
	def, err := store.DefaultDBPath()
	if err != nil {
		def = "swarmery.db"
	}
	return fs.String("db", def, "path to the SQLite database")
}

func cmdIngest(args []string) error {
	fs := flag.NewFlagSet("ingest", flag.ExitOnError)
	dbPath := dbFlag(fs)
	fs.Parse(args)
	if fs.NArg() != 1 {
		return fmt.Errorf("usage: swarmery ingest [--db <path>] <file.jsonl>")
	}

	db, err := store.Open(*dbPath)
	if err != nil {
		return err
	}
	defer db.Close()

	stats, err := ingest.File(db, fs.Arg(0))
	if err != nil {
		return err
	}
	fmt.Printf("ingested %s\n  projects: %d created\n  sessions: %d created\n  turns: %d created\n  events: %d created\n  file_changes: %d created\n  skipped lines: %d\n",
		fs.Arg(0), stats.Projects, stats.Sessions, stats.Turns, stats.Events, stats.FileChanges, stats.SkippedLines)
	return nil
}

// cmdRecost recomputes turns.cost_usd for every turn from stored usage and
// the current pricing table — run it after changing config/pricing.json.
// Idempotent: converges to the same values on every run.
func cmdRecost(args []string) error {
	fs := flag.NewFlagSet("recost", flag.ExitOnError)
	dbPath := dbFlag(fs)
	fs.Parse(args)
	if fs.NArg() != 0 {
		return fmt.Errorf("usage: swarmery recost [--db <path>]")
	}

	if port := envPort(); daemonRunning(port) {
		log.Printf("warn: a swarmery daemon appears to be running on port %d — recost writes to the same WAL; concurrent ingest may interleave (busy_timeout handles locking, but consider stopping the daemon first)", port)
	}

	db, err := store.Open(*dbPath)
	if err != nil {
		return err
	}
	defer db.Close()

	stats, err := cost.Recost(db, cost.Default())
	if err != nil {
		return err
	}
	fmt.Printf("recost %s\n  turns examined: %d\n  priced: %d\n  unpriced (unknown model → NULL): %d\n  no usage (user turns → NULL): %d\n",
		*dbPath, stats.Total, stats.Priced, stats.Unpriced, stats.NoUsage)
	return nil
}

// daemonRunning probes the local API port to detect a live daemon.
func daemonRunning(port int) bool {
	client := http.Client{Timeout: 500 * time.Millisecond}
	resp, err := client.Get(fmt.Sprintf("http://localhost:%d/api/projects", port))
	if err != nil {
		return false
	}
	resp.Body.Close()
	return true
}

func cmdServe(args []string) error {
	fs := flag.NewFlagSet("serve", flag.ExitOnError)
	dbPath := dbFlag(fs)
	port := fs.Int("port", envPort(), "HTTP port (env: SWARMERY_PORT)")
	fs.Parse(args)

	db, err := store.Open(*dbPath)
	if err != nil {
		return err
	}
	defer db.Close()

	handler, err := api.NewServer(db)
	if err != nil {
		return err
	}
	addr := fmt.Sprintf(":%d", *port)
	log.Printf("swarmery serving on http://localhost%s (db: %s)", addr, *dbPath)
	return http.ListenAndServe(addr, handler)
}

func envPort() int {
	if v := os.Getenv("SWARMERY_PORT"); v != "" {
		if p, err := strconv.Atoi(v); err == nil && p > 0 && p < 65536 {
			return p
		}
		log.Printf("warn: ignoring invalid SWARMERY_PORT=%q", v)
	}
	return defaultPort
}
