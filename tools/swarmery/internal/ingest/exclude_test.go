package ingest

import (
	"os"
	"path/filepath"
	"testing"
)

func TestExcludeListMatchPath(t *testing.T) {
	e := ParseExcludeList(DefaultExclude)
	cases := []struct {
		path string
		want bool
	}{
		{"/tmp/proj", true},
		{"/tmp/swarmery-spike/proj", true}, // ancestor /tmp/swarmery-spike matches /tmp/*
		{"/private/tmp/p2-live/proj", true},
		{"/Volumes/Work/swarmery", false},
		{"/tmpx/proj", false}, // no partial-segment match
		{"", false},
	}
	for _, c := range cases {
		if got := e.MatchPath(c.path); got != c.want {
			t.Errorf("MatchPath(%q) = %v, want %v", c.path, got, c.want)
		}
	}
	if (ExcludeList)(nil).MatchPath("/tmp/x") {
		t.Error("nil exclude list must match nothing")
	}
}

func TestExcludeListMatchProjectDir(t *testing.T) {
	e := ParseExcludeList(DefaultExclude)
	cases := []struct {
		dir  string
		want bool
	}{
		{"-private-tmp-swarmery-spike-proj", true},
		{"-private-tmp-p2-live-proj", true},
		{"-tmp-proj", true},
		{"-Volumes-Work-swarmery", false},
	}
	for _, c := range cases {
		if got := e.MatchProjectDir(c.dir); got != c.want {
			t.Errorf("MatchProjectDir(%q) = %v, want %v", c.dir, got, c.want)
		}
	}
}

// TestDiscoverSkipsExcludedProjects: excluded project dirs never reach the
// scanner, so deleted tmp data cannot rescan itself back into the DB.
func TestDiscoverSkipsExcludedProjects(t *testing.T) {
	db := testDB(t)
	root := t.TempDir()
	for _, dir := range []string{"-private-tmp-p2-live-proj", "-Volumes-Work-app"} {
		if err := os.MkdirAll(filepath.Join(root, dir), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(
			filepath.Join(root, dir, "s.jsonl"), []byte("{}\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	p := NewPipeline(db, Config{
		ProjectsRoot: root,
		Exclude:      ParseExcludeList(DefaultExclude),
	}, nil)
	files := p.discover()
	if len(files) != 1 || filepath.Base(filepath.Dir(files[0])) != "-Volumes-Work-app" {
		t.Errorf("discover = %v, want only the -Volumes-Work-app transcript", files)
	}

	// tailOne must also refuse directly-delivered excluded paths (fsnotify).
	excludedFile := filepath.Join(root, "-private-tmp-p2-live-proj", "s.jsonl")
	p.tailOne(excludedFile, false)
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM file_offsets`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Errorf("file_offsets = %d after tailing an excluded path, want 0", n)
	}
}
