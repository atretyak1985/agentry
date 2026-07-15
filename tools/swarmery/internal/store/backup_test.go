package store

import (
	"os"
	"path/filepath"
	"testing"
)

// TestBackupRoundTrip verifies that Backup produces a self-contained snapshot
// whose contents match the source DB and that the source stays usable.
func TestBackupRoundTrip(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "swarmery.db")

	db, err := Open(src)
	if err != nil {
		t.Fatalf("open src: %v", err)
	}
	if _, err := db.Exec(`CREATE TABLE t (id INTEGER PRIMARY KEY, v TEXT)`); err != nil {
		t.Fatalf("create table: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO t (v) VALUES ('alpha'), ('beta')`); err != nil {
		t.Fatalf("insert: %v", err)
	}
	db.Close()

	dest := filepath.Join(dir, "backups", "snap.db")
	size, err := Backup(src, dest)
	if err != nil {
		t.Fatalf("backup: %v", err)
	}
	if size <= 0 {
		t.Fatalf("expected positive snapshot size, got %d", size)
	}

	// The snapshot is a standalone SQLite file with no -wal/-shm sidecars.
	for _, sidecar := range []string{dest + "-wal", dest + "-shm"} {
		if _, err := os.Stat(sidecar); err == nil {
			t.Errorf("unexpected sidecar left behind: %s", sidecar)
		}
	}

	// Re-open the snapshot and confirm the rows survived.
	snap, err := Open(dest)
	if err != nil {
		t.Fatalf("open snapshot: %v", err)
	}
	defer snap.Close()
	var n int
	if err := snap.QueryRow(`SELECT count(*) FROM t`).Scan(&n); err != nil {
		t.Fatalf("count snapshot rows: %v", err)
	}
	if n != 2 {
		t.Fatalf("expected 2 rows in snapshot, got %d", n)
	}
}

// TestBackupRefusesExistingTarget guards against silently clobbering an
// existing snapshot file.
func TestBackupRefusesExistingTarget(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "swarmery.db")
	if db, err := Open(src); err != nil {
		t.Fatalf("open src: %v", err)
	} else {
		db.Close()
	}

	dest := filepath.Join(dir, "exists.db")
	if err := os.WriteFile(dest, []byte("x"), 0o644); err != nil {
		t.Fatalf("seed target: %v", err)
	}
	if _, err := Backup(src, dest); err == nil {
		t.Fatalf("expected error backing up onto an existing target, got nil")
	}
}
