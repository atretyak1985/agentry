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

// installEnvKeys maps each install flag to the SWARMERY_* var it bakes into the
// plist. launchd does not inherit the installing shell's environment, so these
// are the only way to configure the daemon under launchd.
var installEnvKeys = []struct{ flag, env string }{
	{"onboard-roots", "SWARMERY_ONBOARD_ROOTS"},
	{"workspace-root", "SWARMERY_WORKSPACE_ROOT"},
	{"statusline-src", "SWARMERY_STATUSLINE_SRC"},
}

// CmdInstall implements
//
//	swarmery install [--port <n>] [--onboard-roots <dirs>]
//	                 [--workspace-root <dir>] [--statusline-src <dir>]
//
// Anything the daemon needs at runtime is baked into the plist's
// EnvironmentVariables. Because Install rewrites the whole plist, a bare
// reinstall would otherwise WIPE previously-baked vars (silently disabling
// onboarding); to prevent that, any var not re-supplied on this run is
// PRESERVED from the existing plist. Precedence per var: explicit flag > shell
// env > existing plist. --onboard-roots enables POST /api/projects/onboard (and
// the dashboard "new project" button); clear it deliberately with --flag "".
func CmdInstall(args []string) error {
	fs := flag.NewFlagSet("install", flag.ExitOnError)
	// Defaults are empty (NOT the env): resolution — flag > env > prev plist —
	// happens in mergeInstallEnv so preservation works. Explicitness is read
	// from fs.Visit, so an unset flag can fall back instead of clobbering.
	port := fs.Int("port", -1, "daemon HTTP port baked into the plist (env: SWARMERY_PORT; default keeps the existing/daemon value)")
	onboardRoots := fs.String("onboard-roots", "",
		"comma-separated allow-list of parent dirs enabling project onboarding (env: SWARMERY_ONBOARD_ROOTS)")
	workspaceRoot := fs.String("workspace-root", "",
		"shared workspace repo root baked into the plist (env: SWARMERY_WORKSPACE_ROOT)")
	statuslineSrc := fs.String("statusline-src", "",
		"plugins/core/statusline dir onboarding copies from (env: SWARMERY_STATUSLINE_SRC)")
	fs.Parse(args)
	if *port != -1 && (*port < 0 || *port > 65535) {
		return fmt.Errorf("invalid port %d", *port)
	}

	set := map[string]bool{}
	fs.Visit(func(f *flag.Flag) { set[f.Name] = true })

	sys, err := realSystem()
	if err != nil {
		return err
	}
	prev := sys.ExistingPlistEnv()

	env, preserved := mergeInstallEnv(prev, set, map[string]string{
		"onboard-roots":  *onboardRoots,
		"workspace-root": *workspaceRoot,
		"statusline-src": *statuslineSrc,
	}, os.LookupEnv)
	for _, k := range preserved {
		fmt.Fprintf(os.Stdout, "  preserving %s from existing plist\n", k)
	}
	resolvedPort := resolveInstallPort(prev, set["port"], *port)

	sourceBin, err := os.Executable()
	if err != nil {
		return fmt.Errorf("locate current binary: %w", err)
	}
	return sys.Install(sourceBin, resolvedPort, env...)
}

// mergeInstallEnv resolves the plist EnvironmentVariables for an install run.
// Per var the precedence is explicit flag > shell env > existing plist (prev),
// so a bare reinstall preserves what was baked before instead of wiping it.
// preserved lists the vars carried over from prev (nothing was re-supplied) so
// the caller can report them. Explicitly passing an empty flag clears the var.
func mergeInstallEnv(
	prev map[string]string,
	set map[string]bool,
	flags map[string]string,
	getenv func(string) (string, bool),
) (env []EnvVar, preserved []string) {
	for _, k := range installEnvKeys {
		var val string
		switch {
		case set[k.flag]:
			val = strings.TrimSpace(flags[k.flag])
		default:
			if v, ok := getenv(k.env); ok {
				val = strings.TrimSpace(v)
			} else if p, ok := prev[k.env]; ok {
				val = p
				if p != "" {
					preserved = append(preserved, k.env)
				}
			}
		}
		if val != "" {
			env = append(env, EnvVar{Key: k.env, Value: val})
		}
	}
	return env, preserved
}

// resolveInstallPort mirrors mergeInstallEnv for the special-cased port: an
// explicit --port wins, else SWARMERY_PORT, else the port baked into the
// existing plist, else 0 (daemon default).
func resolveInstallPort(prev map[string]string, explicit bool, flagVal int) int {
	if explicit {
		return flagVal
	}
	if p := envPort(); p != 0 {
		return p
	}
	if v, ok := prev["SWARMERY_PORT"]; ok {
		if p, err := strconv.Atoi(v); err == nil && p > 0 && p < 65536 {
			return p
		}
	}
	return 0
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
