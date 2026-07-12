package hookcfg

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/atretyak1985/swarmery/tools/swarmery/internal/store"
)

// Cmd implements `swarmery hooks <install|uninstall|status>
// [--project <path>] [--all] [--port <n>] [--db <path>]`.
func Cmd(args []string) error {
	if len(args) < 1 {
		return usageErr()
	}
	sub := args[0]

	fs := flag.NewFlagSet("hooks "+sub, flag.ExitOnError)
	project := fs.String("project", "", "project directory (default: current directory)")
	all := fs.Bool("all", false, "target every non-archived project in the daemon DB")
	port := fs.Int("port", 0, "bake SWARMERY_PORT=<n> into the hook commands (0 = daemon default)")
	dbPath := fs.String("db", "", "daemon DB path for --all (default: ~/.swarmery/swarmery.db)")
	fs.Parse(args[1:])
	if *port < 0 || *port > 65535 {
		return fmt.Errorf("invalid port %d", *port)
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("resolve home dir: %w", err)
	}
	sys := &System{Home: home, Out: os.Stdout}

	projects, err := resolveTargets(*project, *all, *dbPath)
	if err != nil {
		return err
	}

	switch sub {
	case "install":
		for _, p := range projects {
			if err := sys.Install(p, *port); err != nil {
				return err
			}
		}
		return nil
	case "uninstall":
		for _, p := range projects {
			if err := sys.Uninstall(p); err != nil {
				return err
			}
		}
		return nil
	case "status":
		return sys.Status(projects, *port)
	default:
		return usageErr()
	}
}

func usageErr() error {
	return fmt.Errorf("usage: swarmery hooks <install|uninstall|status> [--project <path>] [--all] [--port <n>] [--db <path>]")
}

// resolveTargets picks the project list: --all → daemon DB paths;
// --project <path> → that path; default → the current directory.
func resolveTargets(project string, all bool, dbPath string) ([]string, error) {
	if all && project != "" {
		return nil, fmt.Errorf("--all and --project are mutually exclusive")
	}
	if all {
		if dbPath == "" {
			var err error
			dbPath, err = store.DefaultDBPath()
			if err != nil {
				return nil, err
			}
		}
		if _, err := os.Stat(dbPath); err != nil {
			return nil, fmt.Errorf("daemon DB: %w", err)
		}
		db, err := store.Open(dbPath)
		if err != nil {
			return nil, err
		}
		defer db.Close()
		projects, err := ProjectsFromDB(db)
		if err != nil {
			return nil, err
		}
		if len(projects) == 0 {
			return nil, fmt.Errorf("no projects found in %s", dbPath)
		}
		return projects, nil
	}
	if project == "" {
		var err error
		project, err = os.Getwd()
		if err != nil {
			return nil, err
		}
	}
	abs, err := filepath.Abs(project)
	if err != nil {
		return nil, err
	}
	if fi, err := os.Stat(abs); err != nil || !fi.IsDir() {
		return nil, fmt.Errorf("project %s is not a directory", abs)
	}
	return []string{abs}, nil
}
