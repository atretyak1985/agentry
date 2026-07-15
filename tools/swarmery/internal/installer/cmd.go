package installer

import (
	"flag"
	"fmt"
	"os"
	"os/user"
	"runtime"
	"strconv"
	"strings"

	"github.com/atretyak1985/swarmery/tools/swarmery/internal/version"
)

// Version is the CLI version reported by `swarmery status` — re-exported from
// the single shared source in internal/version (also served by /api/health).
const Version = version.Version

// CmdInstall implements
//
//	swarmery install [--port <n>] [--onboard-roots <dirs>]
//	                 [--workspace-root <dir>] [--statusline-src <dir>]
//
// launchd does not inherit the installing shell's environment, so anything the
// daemon needs at runtime is baked into the plist's EnvironmentVariables here.
// Each flag defaults to its matching SWARMERY_* env var; empty values are
// omitted. --onboard-roots is what enables POST /api/projects/onboard (and the
// dashboard "new project" button) — without it that endpoint stays disabled.
func CmdInstall(args []string) error {
	fs := flag.NewFlagSet("install", flag.ExitOnError)
	port := fs.Int("port", envPort(), "daemon HTTP port baked into the plist (env: SWARMERY_PORT; 0 = daemon default)")
	onboardRoots := fs.String("onboard-roots", os.Getenv("SWARMERY_ONBOARD_ROOTS"),
		"comma-separated allow-list of parent dirs enabling project onboarding (env: SWARMERY_ONBOARD_ROOTS)")
	workspaceRoot := fs.String("workspace-root", os.Getenv("SWARMERY_WORKSPACE_ROOT"),
		"shared workspace repo root baked into the plist (env: SWARMERY_WORKSPACE_ROOT)")
	statuslineSrc := fs.String("statusline-src", os.Getenv("SWARMERY_STATUSLINE_SRC"),
		"plugins/core/statusline dir onboarding copies from (env: SWARMERY_STATUSLINE_SRC)")
	fs.Parse(args)
	if *port < 0 || *port > 65535 {
		return fmt.Errorf("invalid port %d", *port)
	}

	var env []EnvVar
	for _, e := range []EnvVar{
		{Key: "SWARMERY_ONBOARD_ROOTS", Value: strings.TrimSpace(*onboardRoots)},
		{Key: "SWARMERY_WORKSPACE_ROOT", Value: strings.TrimSpace(*workspaceRoot)},
		{Key: "SWARMERY_STATUSLINE_SRC", Value: strings.TrimSpace(*statuslineSrc)},
	} {
		if e.Value != "" {
			env = append(env, e)
		}
	}

	sys, err := realSystem()
	if err != nil {
		return err
	}
	sourceBin, err := os.Executable()
	if err != nil {
		return fmt.Errorf("locate current binary: %w", err)
	}
	return sys.Install(sourceBin, *port, env...)
}

// CmdUninstall implements `swarmery uninstall`.
func CmdUninstall(args []string) error {
	fs := flag.NewFlagSet("uninstall", flag.ExitOnError)
	fs.Parse(args)
	sys, err := realSystem()
	if err != nil {
		return err
	}
	return sys.Uninstall()
}

// CmdStatus implements `swarmery status`.
func CmdStatus(args []string) error {
	fs := flag.NewFlagSet("status", flag.ExitOnError)
	fs.Parse(args)
	sys, err := realSystem()
	if err != nil {
		return err
	}
	return sys.Status()
}

// realSystem wires a System against the actual host environment.
func realSystem() (*System, error) {
	if runtime.GOOS != "darwin" {
		return nil, fmt.Errorf("install/uninstall/status use launchd and are macOS-only (got %s)", runtime.GOOS)
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("resolve home dir: %w", err)
	}
	u, err := user.Current()
	if err != nil {
		return nil, fmt.Errorf("resolve current user: %w", err)
	}
	return &System{Home: home, UID: u.Uid, Run: ExecRunner{}, Out: os.Stdout}, nil
}

// envPort mirrors the serve command's SWARMERY_PORT handling, but returns 0
// (meaning "not configured") when the variable is absent or invalid.
func envPort() int {
	if v := os.Getenv("SWARMERY_PORT"); v != "" {
		if p, err := strconv.Atoi(v); err == nil && p > 0 && p < 65536 {
			return p
		}
	}
	return 0
}
