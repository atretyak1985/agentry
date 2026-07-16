// Package projectscan is the read-only counterpart to internal/onboard: given a
// consumer project's directory it reports whether the swarmery plugin is enabled
// (its .claude/settings.json) and enumerates the project-LOCAL agent/skill/
// command/hook files under .claude/. It never writes and never errors the whole
// listing on a single unreadable project — a missing or malformed settings.json
// simply yields a nil PluginState so the caller renders the project as
// telemetry-only rather than failing the request.
//
// Scope boundary: Components resolves ONLY the files a project ships locally.
// Components provided by the enabled plugins (core@swarmery + packs) live in the
// plugin cache (~/.claude/plugins/cache) and are deliberately NOT enumerated
// here — that is a later stretch step. The enabled packs are still reported by
// name via PluginState.Packs.
package projectscan

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// PluginState is the swarmery-plugin view of a project, derived from its
// .claude/settings.json. The zero value is meaningful: Managed=false with empty
// packs describes a project whose settings.json exists but does not enable
// swarmery.
type PluginState struct {
	// Managed reports enabledPlugins["core@swarmery"] === true.
	Managed bool `json:"managed"`
	// Packs are the other "<pack>@swarmery" entries enabled alongside core,
	// with the "@swarmery" suffix stripped, sorted for a stable response.
	Packs []string `json:"packs"`
	// Marketplace is extraKnownMarketplaces.swarmery.source.repo, "" when absent.
	Marketplace string `json:"marketplace"`
	// UnderOnboardRoot reports whether the project path is under one of the
	// daemon's onboarding roots — i.e. whether the (fenced) detach endpoint may
	// operate on it. Purely advisory for the UI; the write path re-checks.
	UnderOnboardRoot bool `json:"underOnboardRoot"`
}

// marketplaceSuffix is the "@<marketplace>" tag every swarmery plugin key
// carries in enabledPlugins (e.g. "core@swarmery").
const marketplaceSuffix = "@swarmery"

// settingsShape is the subset of .claude/settings.json projectscan reads.
type settingsShape struct {
	EnabledPlugins         map[string]bool `json:"enabledPlugins"`
	ExtraKnownMarketplaces map[string]struct {
		Source struct {
			Repo string `json:"repo"`
		} `json:"source"`
	} `json:"extraKnownMarketplaces"`
}

// PluginState reads <projectPath>/.claude/settings.json and reports the swarmery
// plugin state. A missing or malformed file returns (nil, nil): the project is
// simply not swarmery-managed as far as we can tell, which must not fail the
// list. roots is the daemon's onboarding allow-list, used only to compute
// UnderOnboardRoot (pass nil to skip that hint).
func ReadPluginState(projectPath string, roots []string) (*PluginState, error) {
	raw, err := os.ReadFile(filepath.Join(projectPath, ".claude", "settings.json"))
	if err != nil {
		return nil, nil //nolint:nilerr // absent settings = not managed, not an error
	}
	var s settingsShape
	if err := json.Unmarshal(raw, &s); err != nil {
		return nil, nil //nolint:nilerr // malformed settings = treat as not managed
	}

	st := &PluginState{
		UnderOnboardRoot: underAnyRoot(projectPath, roots),
	}
	for key, on := range s.EnabledPlugins {
		if !on || !strings.HasSuffix(key, marketplaceSuffix) {
			continue
		}
		name := strings.TrimSuffix(key, marketplaceSuffix)
		if name == "core" {
			st.Managed = true
			continue
		}
		st.Packs = append(st.Packs, name)
	}
	sort.Strings(st.Packs)
	if mp, ok := s.ExtraKnownMarketplaces["swarmery"]; ok {
		st.Marketplace = mp.Source.Repo
	}
	return st, nil
}

// Component is one project-local registry entry (agent, skill, command or hook).
type Component struct {
	Name string `json:"name"`
	// Source is always "local" here; plugin-provided components (a later step)
	// will carry "core@swarmery" / "<pack>@swarmery".
	Source string `json:"source"`
}

// ComponentCounts is the at-a-glance tally rendered on the list/detail header.
type ComponentCounts struct {
	Agents   int `json:"agents"`
	Skills   int `json:"skills"`
	Commands int `json:"commands"`
	Hooks    int `json:"hooks"`
}

// Components is the project-local registry inventory.
type Components struct {
	Agents   []Component     `json:"agents"`
	Skills   []Component     `json:"skills"`
	Commands []Component     `json:"commands"`
	Hooks    []Component     `json:"hooks"`
	Counts   ComponentCounts `json:"counts"`
}

// Components enumerates the project-local .claude/{agents,skills,commands,hooks}
// entries. Agents/commands are *.md files; skills are directories (each holds a
// SKILL.md); hooks are the files under .claude/hooks/. Missing directories yield
// empty slices, never an error — the returned Components is always non-nil.
func ReadComponents(projectPath string) (*Components, error) {
	claudeDir := filepath.Join(projectPath, ".claude")
	c := &Components{
		Agents:   markdownComponents(filepath.Join(claudeDir, "agents")),
		Skills:   dirComponents(filepath.Join(claudeDir, "skills")),
		Commands: markdownComponents(filepath.Join(claudeDir, "commands")),
		Hooks:    fileComponents(filepath.Join(claudeDir, "hooks")),
	}
	c.Counts = ComponentCounts{
		Agents:   len(c.Agents),
		Skills:   len(c.Skills),
		Commands: len(c.Commands),
		Hooks:    len(c.Hooks),
	}
	return c, nil
}

// markdownComponents lists *.md files in dir, named without the extension.
func markdownComponents(dir string) []Component {
	return scanDir(dir, func(e os.DirEntry) (string, bool) {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".md") || strings.HasPrefix(name, ".") {
			return "", false
		}
		return strings.TrimSuffix(name, ".md"), true
	})
}

// dirComponents lists subdirectories of dir (skills: one dir per skill).
func dirComponents(dir string) []Component {
	return scanDir(dir, func(e os.DirEntry) (string, bool) {
		if !e.IsDir() || strings.HasPrefix(e.Name(), ".") {
			return "", false
		}
		return e.Name(), true
	})
}

// fileComponents lists regular files in dir (hooks: scripts / hooks.json).
func fileComponents(dir string) []Component {
	return scanDir(dir, func(e os.DirEntry) (string, bool) {
		if e.IsDir() || strings.HasPrefix(e.Name(), ".") {
			return "", false
		}
		return e.Name(), true
	})
}

// scanDir reads dir and returns a sorted []Component for entries pick accepts.
// A missing/unreadable dir yields an empty slice (never nil), so JSON renders
// `[]` not `null`.
func scanDir(dir string, pick func(os.DirEntry) (string, bool)) []Component {
	out := []Component{}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return out
	}
	for _, e := range entries {
		if name, ok := pick(e); ok {
			out = append(out, Component{Name: name, Source: "local"})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// underAnyRoot reports whether path is one of roots or nested inside it. It is a
// lexical check on cleaned absolute paths — enough for the advisory UI hint; the
// detach write path performs the symlink-safe fence (api.resolveUnderRoots).
func underAnyRoot(path string, roots []string) bool {
	abs, err := filepath.Abs(path)
	if err != nil {
		return false
	}
	abs = filepath.Clean(abs)
	for _, root := range roots {
		rootAbs, err := filepath.Abs(root)
		if err != nil {
			continue
		}
		rel, err := filepath.Rel(filepath.Clean(rootAbs), abs)
		if err != nil {
			continue
		}
		if rel == "." || (!strings.HasPrefix(rel, ".."+string(filepath.Separator)) && rel != ".." && !filepath.IsAbs(rel)) {
			return true
		}
	}
	return false
}
