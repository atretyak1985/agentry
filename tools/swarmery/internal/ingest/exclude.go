package ingest

// Project exclusion: throwaway working directories (spike/e2e runs under
// /tmp) must not pollute the dashboards. An ExcludeList of path globs is
// honored by BOTH tracking channels:
//
//   - the JSONL scanner skips matching project dirs on scan and tail (the
//     flattened ~/.claude/projects dir name is matched against the flattened
//     glob), and the stub heal never CREATES a project row for an excluded
//     cwd;
//   - the approvals hook path serves requests from excluded cwds normally —
//     the fail-open decision flow is untouched — but persists no
//     session/project rows (the API answers 204, the shim falls back to the
//     native dialog).
//
// Exclusion gates row CREATION only: existing DB rows are never deleted by
// code — removing already-ingested data is a deliberate one-off operation
// for the owner.

import (
	"flag"
	"path/filepath"
	"strings"
)

// DefaultExclude is the default --exclude-projects / SWARMERY_EXCLUDE value.
const DefaultExclude = "/tmp/*,/private/tmp/*"

// ExcludeList is a set of path globs (filepath.Match syntax per element). A
// path is excluded when a pattern matches the path itself OR any ancestor
// directory — so '/tmp/*' covers /tmp/x and everything below it.
type ExcludeList []string

// ParseExcludeList splits a comma-separated glob list, trimming blanks.
func ParseExcludeList(s string) ExcludeList {
	var e ExcludeList
	for _, part := range strings.Split(s, ",") {
		if p := strings.TrimSpace(part); p != "" {
			e = append(e, p)
		}
	}
	return e
}

// String implements flag.Value.
func (e *ExcludeList) String() string { return strings.Join(*e, ",") }

// Set implements flag.Value (replaces the current list).
func (e *ExcludeList) Set(s string) error {
	*e = ParseExcludeList(s)
	return nil
}

var _ flag.Value = (*ExcludeList)(nil)

// MatchPath reports whether path (a cwd) is excluded: some pattern matches
// the cleaned path or one of its ancestor directories.
func (e ExcludeList) MatchPath(path string) bool {
	if len(e) == 0 || path == "" {
		return false
	}
	path = filepath.Clean(path)
	for _, pat := range e {
		for p := path; ; p = filepath.Dir(p) {
			if ok, _ := filepath.Match(pat, p); ok {
				return true
			}
			if p == "/" || p == "." || p == filepath.Dir(p) {
				break
			}
		}
	}
	return false
}

// MatchProjectDir reports whether a ~/.claude/projects entry (the FLATTENED
// project dir name, e.g. '-private-tmp-p2-live-proj') belongs to an excluded
// path: each glob is flattened the same way Claude Code flattens cwds and
// matched against the dir name. Flattened names contain no '/', so '*'
// spans the whole remainder.
func (e ExcludeList) MatchProjectDir(dirName string) bool {
	if len(e) == 0 || dirName == "" {
		return false
	}
	for _, pat := range e {
		if ok, _ := filepath.Match(flattenPattern(pat), dirName); ok {
			return true
		}
	}
	return false
}

// flattenPattern applies Claude Code's project-dir flattening to a glob:
// '/', '.', '_' and spaces become '-'.
func flattenPattern(p string) string {
	return strings.NewReplacer("/", "-", ".", "-", "_", "-", " ", "-").Replace(p)
}
