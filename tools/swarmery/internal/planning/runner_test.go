package planning

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestClaudeBin_ExplicitOverride(t *testing.T) {
	t.Setenv("SWARMERY_CLAUDE_BIN", "/custom/claude")
	got, err := ClaudeBin()
	if err != nil {
		t.Fatalf("ClaudeBin: %v", err)
	}
	if got != "/custom/claude" {
		t.Errorf("ClaudeBin = %q, want /custom/claude", got)
	}
}

func TestClaudeBin_PathLookup(t *testing.T) {
	// Put a fake executable `claude` on PATH and clear the override.
	t.Setenv("SWARMERY_CLAUDE_BIN", "")
	dir := t.TempDir()
	bin := filepath.Join(dir, "claude")
	if err := os.WriteFile(bin, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
	got, err := ClaudeBin()
	if err != nil {
		t.Fatalf("ClaudeBin: %v", err)
	}
	if got != bin {
		t.Errorf("ClaudeBin = %q, want %q", got, bin)
	}
}

func TestClaudeBin_HomeCandidate(t *testing.T) {
	// No override, empty PATH, but a claude at $HOME/.local/bin/claude → resolved
	// via the candidate probe. Deterministic regardless of what is installed on
	// the machine (this candidate is checked before the machine's real claude
	// would matter only if PATH found it, which we've emptied).
	t.Setenv("SWARMERY_CLAUDE_BIN", "")
	t.Setenv("PATH", t.TempDir())
	home := t.TempDir()
	t.Setenv("HOME", home)
	bin := filepath.Join(home, ".local", "bin", "claude")
	if err := os.MkdirAll(filepath.Dir(bin), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(bin, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	got, err := ClaudeBin()
	if err != nil {
		t.Fatalf("ClaudeBin: %v", err)
	}
	// On a machine WITHOUT /opt/homebrew/bin/claude or /usr/local/bin/claude the
	// home candidate wins; on a machine WITH one of those absolute installs that
	// wins first (they precede the home candidate in the probe order). Either is a
	// valid resolution — assert we got a real claude path, not an error.
	if got == "" {
		t.Fatal("ClaudeBin returned empty path")
	}
	if got != bin && got != "/opt/homebrew/bin/claude" && got != "/usr/local/bin/claude" {
		t.Errorf("ClaudeBin = %q, want the home candidate or an absolute install", got)
	}
}

func TestClaudeRunner_Start_FakeClaude(t *testing.T) {
	// Point the runner at a fake `claude` shell script so Start's exec path,
	// exit-code handling, and stderr tail are exercised without a real claude.
	script := filepath.Join(t.TempDir(), "fakeclaude.sh")
	body := "#!/bin/sh\necho on-stdout\necho on-stderr 1>&2\nexit 0\n"
	if err := os.WriteFile(script, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("SWARMERY_CLAUDE_BIN", script)

	r := ClaudeRunner{Timeout: 30 * time.Second}
	run, err := r.Start(context.Background(), RunSpec{
		Prompt: "plan it", SessionUUID: "u1", Cwd: t.TempDir(),
	})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	if run.ExitCode != 0 {
		t.Errorf("ExitCode = %d, want 0", run.ExitCode)
	}
	if run.TimedOut {
		t.Error("unexpected TimedOut")
	}
	if run.SessionUUID != "u1" {
		t.Errorf("SessionUUID = %q", run.SessionUUID)
	}
}

func TestClaudeRunner_Start_NonzeroExit(t *testing.T) {
	script := filepath.Join(t.TempDir(), "fail.sh")
	if err := os.WriteFile(script, []byte("#!/bin/sh\necho bad 1>&2\nexit 7\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("SWARMERY_CLAUDE_BIN", script)
	r := ClaudeRunner{Timeout: 30 * time.Second}
	run, err := r.Start(context.Background(), RunSpec{Prompt: "p", SessionUUID: "u2", Cwd: t.TempDir()})
	if err != nil {
		t.Fatalf("Start returned error for nonzero exit (should be an outcome): %v", err)
	}
	if run.ExitCode != 7 {
		t.Errorf("ExitCode = %d, want 7", run.ExitCode)
	}
	if run.Stderr != "bad" {
		t.Errorf("Stderr = %q, want bad", run.Stderr)
	}
}

func TestClaudeRunner_Start_BinMissing(t *testing.T) {
	t.Setenv("SWARMERY_CLAUDE_BIN", filepath.Join(t.TempDir(), "does-not-exist"))
	r := ClaudeRunner{Timeout: 5 * time.Second}
	run, err := r.Start(context.Background(), RunSpec{Prompt: "p", SessionUUID: "u3", Cwd: t.TempDir()})
	if err == nil {
		t.Fatal("expected a start error for a missing binary")
	}
	if run == nil || run.ExitCode != -1 {
		t.Errorf("run = %+v, want ExitCode -1", run)
	}
}

func TestNewUUID(t *testing.T) {
	a, b := newUUID(), newUUID()
	if a == b {
		t.Error("newUUID returned identical values")
	}
	// RFC-4122 v4 shape: 8-4-4-4-12 hex, version nibble 4.
	if len(a) != 36 || a[14] != '4' {
		t.Errorf("newUUID = %q, not a v4 uuid", a)
	}
}

func TestTail(t *testing.T) {
	if got := tail("  hello world  ", 5); got != "world" {
		t.Errorf("tail = %q, want world", got)
	}
	if got := tail("short", 100); got != "short" {
		t.Errorf("tail = %q, want short", got)
	}
}
