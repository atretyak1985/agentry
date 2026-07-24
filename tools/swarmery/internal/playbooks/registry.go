// Package playbooks is the selectable-workflow registry (fusion phase 13). A
// playbook is a markdown file (frontmatter + one or more `## Stage:` sections)
// that names an execution recipe a board task can select: how many sequential
// stages the dispatcher runs in the task's single worktree, and how strict the
// Phase 6 verifier grades the result. Four built-ins ship embedded in the
// daemon (quick-fix / standard / review-heavy / plan-first); a consumer project
// can override or add its own under `<project>/.claude/playbooks/*.md` (name
// collision → project wins, the graduation rule).
//
// The registry is a pure in-memory parser over an embedded FS + the project
// directory — no process spawn, no DB, no heavy deps (frontmatter parsing is
// hand-rolled in Go, mirroring internal/improve's frontmatterOK; the daemon
// never shells to yq/python). It is safe for concurrent use: reads take a
// read-lock; a lazy mtime-checked rescan of a project dir takes the write-lock.
package playbooks

import (
	"embed"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

//go:embed builtin/*.md
var builtinFS embed.FS

// Source tags where a resolved playbook came from (surfaced in the UI badge).
const (
	SourceBuiltin = "builtin"
	SourceProject = "project"
)

// verifyValues is the closed set of verify-knob strictness levels (phase-13
// spec). It maps directly onto Phase 6 behavior: strict keeps the verifier's
// behavioral-default-FAIL wording, normal is the default bar, off skips the
// verification run entirely (no verdict stamped).
var verifyValues = map[string]bool{"strict": true, "normal": true, "off": true}

// Stage is one step of a playbook: a name (from the `## Stage: <name>` header)
// and its prompt template body (verbatim markdown, template vars unresolved).
type Stage struct {
	Name string `json:"name"`
	Body string `json:"body"`
}

// Playbook is a parsed, validated recipe.
type Playbook struct {
	Name        string  `json:"name"`
	Description string  `json:"description"`
	Model       string  `json:"model"`  // optional --model override; "" = task/default
	Verify      string  `json:"verify"` // strict | normal | off
	Source      string  `json:"source"` // builtin | project
	Stages      []Stage `json:"stages"`
	// Path is the on-disk path for a project playbook ("" for a built-in). The
	// duplicate flow and the UI hint use it.
	Path string `json:"path,omitempty"`
}

// Registry resolves playbooks for a project. Built-ins are parsed once at
// construction (they can never change at runtime); project files are rescanned
// lazily per project dir when the directory's mtime advances.
type Registry struct {
	mu       sync.RWMutex
	builtins map[string]Playbook // name → built-in (immutable after New)
	// projects caches the last scan of each project dir keyed by absolute dir
	// path. A scan is reused until the dir's mtime changes (lazy staleness).
	projects map[string]*projectScan
}

type projectScan struct {
	mtime time.Time
	byName map[string]Playbook
}

// New parses the embedded built-ins and returns a ready Registry. A malformed
// built-in is a programming error (the files ship in the binary), so it is
// returned as an error to fail startup loudly rather than silently drop a
// recipe.
func New() (*Registry, error) {
	builtins, err := parseDir(builtinFS, "builtin", SourceBuiltin, "")
	if err != nil {
		return nil, fmt.Errorf("playbooks: parse built-ins: %w", err)
	}
	if _, ok := builtins[DefaultName]; !ok {
		return nil, fmt.Errorf("playbooks: built-in %q is required but missing", DefaultName)
	}
	return &Registry{
		builtins: builtins,
		projects: map[string]*projectScan{},
	}, nil
}

// DefaultName is the recipe used when a task's playbook column is NULL/empty.
const DefaultName = "standard"

// BuiltinMarkdown returns the raw embedded markdown source of a built-in
// playbook by name (the exact bytes shipped in the binary), for the
// duplicate-to-project flow which copies it verbatim into the project's
// .claude/playbooks/<name>.md. ok=false when name is not a built-in.
func (r *Registry) BuiltinMarkdown(name string) (string, bool) {
	name = strings.TrimSpace(name)
	if _, ok := r.builtins[name]; !ok {
		return "", false
	}
	body, err := builtinFS.ReadFile("builtin/" + name + ".md")
	if err != nil {
		return "", false
	}
	return string(body), true
}

// ProjectDir renders the playbooks directory for a project root.
func ProjectDir(projectPath string) string {
	return filepath.Join(projectPath, ".claude", "playbooks")
}

// List returns every playbook visible to a project (built-ins overlaid by the
// project's own files, project winning on a name collision), sorted by name.
// projectPath may be "" (built-ins only). A project-scan error is swallowed to
// the built-in view (a broken project dir must not hide the shipped recipes);
// per-file parse errors are already skipped inside the scan.
func (r *Registry) List(projectPath string) []Playbook {
	merged := r.resolve(projectPath)
	out := make([]Playbook, 0, len(merged))
	for _, p := range merged {
		out = append(out, p)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// Get resolves one playbook by name for a project. ok=false when no built-in or
// project file carries that name. An empty/whitespace name resolves to the
// default recipe (the dispatcher passes NULL through as "").
func (r *Registry) Get(projectPath, name string) (Playbook, bool) {
	name = strings.TrimSpace(name)
	if name == "" {
		name = DefaultName
	}
	merged := r.resolve(projectPath)
	p, ok := merged[name]
	return p, ok
}

// resolve builds the effective name→playbook map for a project: built-ins first,
// then project files overlaid (project wins). The built-in map is copied so the
// overlay never mutates the shared immutable set.
func (r *Registry) resolve(projectPath string) map[string]Playbook {
	merged := make(map[string]Playbook, len(r.builtins))
	r.mu.RLock()
	for k, v := range r.builtins {
		merged[k] = v
	}
	r.mu.RUnlock()

	for name, pb := range r.projectPlaybooks(projectPath) {
		if _, shadowed := merged[name]; shadowed {
			// A project file overriding a built-in is the intended graduation
			// override; leave a lint-style breadcrumb in the daemon log once per
			// scan is handled by the scanner (not here, to keep resolve pure).
		}
		merged[name] = pb
	}
	return merged
}

// projectPlaybooks returns the project's own playbooks (name→playbook), using a
// cached scan when the project dir's mtime is unchanged since the last scan.
// Returns an empty map for "" / a missing dir / a scan error.
func (r *Registry) projectPlaybooks(projectPath string) map[string]Playbook {
	if strings.TrimSpace(projectPath) == "" {
		return nil
	}
	dir := ProjectDir(projectPath)
	info, err := os.Stat(dir)
	if err != nil || !info.IsDir() {
		// No project playbooks dir (the common case) — drop any stale cache entry
		// so a later mkdir is picked up, and return empty.
		r.mu.Lock()
		delete(r.projects, dir)
		r.mu.Unlock()
		return nil
	}
	mtime := info.ModTime()

	// Fast path: a cached scan whose mtime matches the dir.
	r.mu.RLock()
	cached, ok := r.projects[dir]
	r.mu.RUnlock()
	if ok && cached.mtime.Equal(mtime) {
		return cached.byName
	}

	// Rescan under the write lock (double-check the mtime — another goroutine may
	// have rescanned while we waited).
	r.mu.Lock()
	defer r.mu.Unlock()
	if cur, ok := r.projects[dir]; ok && cur.mtime.Equal(mtime) {
		return cur.byName
	}
	byName, err := parseProjectDir(dir)
	if err != nil {
		// A scan failure caches an empty result at this mtime so we do not thrash
		// on every call; a dir edit (new mtime) retries.
		byName = map[string]Playbook{}
	}
	r.projects[dir] = &projectScan{mtime: mtime, byName: byName}
	return byName
}

// parseProjectDir reads every *.md in a project playbooks dir. Individual
// malformed files are skipped (a bad project file must not break the whole
// registry — it simply does not appear). Returns name→playbook.
func parseProjectDir(dir string) (map[string]Playbook, error) {
	return parseDir(os.DirFS(dir), ".", SourceProject, dir)
}

// parseDir parses every *.md at the root of fsys. source tags the origin;
// baseDir (non-empty for project scans) is prefixed onto each file's Path.
// Built-in parse failures propagate (they ship in the binary); project parse
// failures are skipped by the caller which passes SourceProject.
func parseDir(fsys fs.FS, root, source, baseDir string) (map[string]Playbook, error) {
	entries, err := fs.ReadDir(fsys, root)
	if err != nil {
		return nil, err
	}
	out := map[string]Playbook{}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		names = append(names, e.Name())
	}
	sort.Strings(names)
	for _, name := range names {
		p := name
		if root != "." {
			p = root + "/" + name
		}
		body, rerr := fs.ReadFile(fsys, p)
		if rerr != nil {
			if source == SourceBuiltin {
				return nil, rerr
			}
			continue
		}
		pb, perr := Parse(string(body), source)
		if perr != nil {
			if source == SourceBuiltin {
				return nil, fmt.Errorf("%s: %w", name, perr)
			}
			continue // skip a malformed project file
		}
		if baseDir != "" {
			pb.Path = filepath.Join(baseDir, name)
		}
		out[pb.Name] = pb
	}
	return out, nil
}
