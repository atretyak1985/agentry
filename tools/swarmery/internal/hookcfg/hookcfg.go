// Package hookcfg manages the Claude Code hook entries that wire projects to
// the swarmery approvals channel: `swarmery hooks install|uninstall|status`.
//
// Placement is per-project `.claude/settings.local.json` (D2 — the
// not-shared/gitignored tier; hook commands carry a machine-local binary
// path). Surgery rules:
//
//   - read-modify-write via map[string]any, mutating ONLY the
//     hooks.PermissionRequest / hooks.Stop arrays — every foreign key and
//     foreign hook survives;
//   - our entries are recognized by the "swarmery hook" command substring;
//   - unparseable JSON aborts WITHOUT writing;
//   - the original file is copied to .bak before the first write;
//   - idempotent: a second install produces no diff;
//   - uninstall removes ONLY swarmery entries (and the empty containers the
//     removal leaves behind).
//
// The user-level ~/.claude/settings.json tier is deliberately NOT supported
// in this iteration (no --user flag).
package hookcfg

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// marker identifies swarmery-managed hook command entries.
const marker = "swarmery hook"

// hookTimeout is the installed PermissionRequest per-hook "timeout" (seconds):
// approval_timeout (120 s) + margin, so Claude Code never kills the shim
// mid-poll (frozen: docs/hooks-protocol.md §Timing, spike E6).
const hookTimeout = 130

// System bundles the environment the hooks manager operates on; tests use a
// temp Home and an in-memory Out.
type System struct {
	Home string
	Out  io.Writer
}

// InstalledBin returns the path baked into hook commands: the launchd-managed
// daemon binary (~/.swarmery/bin/swarmery), NOT the invoking binary — hook
// entries must survive rebuilds of a dev checkout.
func (s *System) InstalledBin() string {
	return filepath.Join(s.Home, ".swarmery", "bin", "swarmery")
}

// SettingsPath returns <project>/.claude/settings.local.json.
func SettingsPath(project string) string {
	return filepath.Join(project, ".claude", "settings.local.json")
}

// command renders one hook command line. A non-zero port is baked in as an
// env prefix so hooked projects can target a non-default daemon port.
func (s *System) command(event string, port int) string {
	cmd := s.InstalledBin() + " hook " + event
	if port > 0 {
		cmd = fmt.Sprintf("SWARMERY_PORT=%d %s", port, cmd)
	}
	return cmd
}

// ── install ──────────────────────────────────────────────────────────────────

// Install writes (or refreshes) the swarmery PermissionRequest + Stop hook
// entries into the project's settings.local.json. Idempotent: when the file
// already contains exactly the desired entries nothing is written.
func (s *System) Install(project string, port int) error {
	path := SettingsPath(project)
	raw, root, existed, err := readSettings(path)
	if err != nil {
		return err
	}

	// Refresh semantics: strip any stale swarmery entries first, then append
	// the current ones — a changed binary path or timeout self-heals.
	stripOurs(root)
	hooks := ensureMap(root, "hooks")
	hooks["PermissionRequest"] = append(sliceOf(hooks["PermissionRequest"]), map[string]any{
		"matcher": "*",
		"hooks": []any{map[string]any{
			"type":    "command",
			"command": s.command("permission-request", port),
			"timeout": hookTimeout,
		}},
	})
	hooks["Stop"] = append(sliceOf(hooks["Stop"]), map[string]any{
		"hooks": []any{map[string]any{
			"type":    "command",
			"command": s.command("stop", port),
		}},
	})

	changed, err := writeSettings(path, raw, root, existed)
	if err != nil {
		return err
	}
	if changed {
		fmt.Fprintf(s.Out, "%s: hooks installed (%s)\n", project, path)
	} else {
		fmt.Fprintf(s.Out, "%s: already installed — no changes\n", project)
	}
	return nil
}

// Uninstall removes ONLY swarmery hook entries; foreign hooks and every other
// setting survive. A file that never contained swarmery entries is untouched.
func (s *System) Uninstall(project string) error {
	path := SettingsPath(project)
	raw, root, existed, err := readSettings(path)
	if err != nil {
		return err
	}
	if !existed {
		fmt.Fprintf(s.Out, "%s: not installed (no %s)\n", project, path)
		return nil
	}
	// A file without swarmery entries is left completely untouched — not
	// even re-formatted.
	if !stripOurs(root) {
		fmt.Fprintf(s.Out, "%s: not installed — no changes\n", project)
		return nil
	}

	if _, err := writeSettings(path, raw, root, existed); err != nil {
		return err
	}
	fmt.Fprintf(s.Out, "%s: swarmery hooks removed\n", project)
	return nil
}

// ── status ───────────────────────────────────────────────────────────────────

// State classifies one project's hook installation.
type State string

const (
	StateInstalled    State = "installed"
	StateStale        State = "stale" // present but binary path/shape drifted
	StateNotInstalled State = "not installed"
	StateBroken       State = "broken json"
)

// Inspect reports the installation state of one project.
func (s *System) Inspect(project string, port int) State {
	raw, err := os.ReadFile(SettingsPath(project))
	if os.IsNotExist(err) {
		return StateNotInstalled
	}
	if err != nil {
		return StateBroken
	}
	var root map[string]any
	if err := json.Unmarshal(raw, &root); err != nil {
		return StateBroken
	}
	found := map[string]bool{}
	current := 0
	for _, event := range []string{"PermissionRequest", "Stop"} {
		hooks, _ := root["hooks"].(map[string]any)
		for _, g := range sliceOf(hooks[event]) {
			group, _ := g.(map[string]any)
			for _, h := range sliceOf(group["hooks"]) {
				entry, _ := h.(map[string]any)
				cmd, _ := entry["command"].(string)
				if !strings.Contains(cmd, marker) {
					continue
				}
				found[event] = true
				want := s.command(map[string]string{
					"PermissionRequest": "permission-request", "Stop": "stop",
				}[event], port)
				if cmd == want {
					current++
				}
			}
		}
	}
	switch {
	case len(found) == 0:
		return StateNotInstalled
	case found["PermissionRequest"] && found["Stop"] && current == 2:
		return StateInstalled
	default:
		return StateStale
	}
}

// Status prints a project → state table.
func (s *System) Status(projects []string, port int) error {
	for _, p := range projects {
		fmt.Fprintf(s.Out, "%-14s %s\n", s.Inspect(p, port), p)
	}
	return nil
}

// ── settings surgery helpers ─────────────────────────────────────────────────

// readSettings loads and parses the settings file. A missing file yields an
// empty root; a parse failure aborts (never write over a file we cannot read).
func readSettings(path string) (raw []byte, root map[string]any, existed bool, err error) {
	raw, err = os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil, map[string]any{}, false, nil
	}
	if err != nil {
		return nil, nil, false, fmt.Errorf("read %s: %w", path, err)
	}
	if err := json.Unmarshal(raw, &root); err != nil {
		return nil, nil, true, fmt.Errorf(
			"%s is not valid JSON (%v) — aborting without writing; fix or remove the file and retry", path, err)
	}
	return raw, root, true, nil
}

// writeSettings marshals root (2-space indent, trailing newline) and writes
// it if it differs from the original bytes. The original is preserved as
// .bak before the FIRST swarmery write.
func writeSettings(path string, raw []byte, root map[string]any, existed bool) (changed bool, err error) {
	out, err := json.MarshalIndent(root, "", "  ")
	if err != nil {
		return false, err
	}
	out = append(out, '\n')
	if existed && bytes.Equal(out, raw) {
		return false, nil
	}
	if existed {
		bak := path + ".bak"
		if _, err := os.Stat(bak); os.IsNotExist(err) {
			if err := os.WriteFile(bak, raw, 0o644); err != nil {
				return false, fmt.Errorf("write backup %s: %w", bak, err)
			}
		}
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return false, err
	}
	if err := os.WriteFile(path, out, 0o644); err != nil {
		return false, fmt.Errorf("write %s: %w", path, err)
	}
	return true, nil
}

// stripOurs removes every hook command entry containing the swarmery marker,
// dropping matcher groups / event arrays / the hooks object itself when the
// removal leaves them empty. Foreign entries are never touched. Reports
// whether anything was removed.
func stripOurs(root map[string]any) bool {
	hooks, _ := root["hooks"].(map[string]any)
	if hooks == nil {
		return false
	}
	removed := false
	for _, event := range []string{"PermissionRequest", "Stop"} {
		groups := sliceOf(hooks[event])
		if groups == nil {
			continue
		}
		var keptGroups []any
		for _, g := range groups {
			group, ok := g.(map[string]any)
			if !ok {
				keptGroups = append(keptGroups, g)
				continue
			}
			var keptHooks []any
			for _, h := range sliceOf(group["hooks"]) {
				entry, ok := h.(map[string]any)
				cmd, _ := entry["command"].(string)
				if ok && strings.Contains(cmd, marker) {
					removed = true
					continue // ours — drop
				}
				keptHooks = append(keptHooks, h)
			}
			if len(keptHooks) == 0 {
				continue // group existed only for our entries — drop it
			}
			group["hooks"] = keptHooks
			keptGroups = append(keptGroups, group)
		}
		if len(keptGroups) == 0 {
			delete(hooks, event)
		} else {
			hooks[event] = keptGroups
		}
	}
	if len(hooks) == 0 {
		delete(root, "hooks")
	}
	return removed
}

func ensureMap(root map[string]any, key string) map[string]any {
	if m, ok := root[key].(map[string]any); ok {
		return m
	}
	m := map[string]any{}
	root[key] = m
	return m
}

func sliceOf(v any) []any {
	s, _ := v.([]any)
	return s
}

// ProjectsFromDB lists the non-archived project paths known to the daemon DB
// (for --all).
func ProjectsFromDB(db *sql.DB) ([]string, error) {
	rows, err := db.Query(`SELECT path FROM projects WHERE archived = 0 ORDER BY path`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var p string
		if err := rows.Scan(&p); err != nil {
			return nil, err
		}
		if strings.HasPrefix(p, "/") { // skip the '(unknown)' placeholder rows
			out = append(out, p)
		}
	}
	return out, rows.Err()
}
