package marketplace

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
)

func writeManifest(t *testing.T, claudeDir, name, body string) {
	t.Helper()
	dir := filepath.Join(claudeDir, "plugins", "marketplaces", name, ".claude-plugin")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "marketplace.json"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestReadHappyPath(t *testing.T) {
	dir := t.TempDir()
	writeManifest(t, dir, "swarmery", `{
		"name": "swarmery",
		"metadata": {"version": "1.13.0"},
		"plugins": [
			{"name": "core", "source": "./plugins/core", "description": "the core"},
			{"name": "lsp-pack", "source": "./plugins/lsp-pack", "description": "Serena LSP"}
		]
	}`)
	cat, err := Read(dir, "swarmery")
	if err != nil {
		t.Fatal(err)
	}
	if cat.Version != "1.13.0" {
		t.Errorf("version = %q, want 1.13.0", cat.Version)
	}
	if len(cat.Plugins) != 2 || cat.Plugins[0].Name != "core" || cat.Plugins[1].Name != "lsp-pack" {
		t.Errorf("plugins = %+v", cat.Plugins)
	}
	if cat.Plugins[1].Description != "Serena LSP" {
		t.Errorf("description = %q", cat.Plugins[1].Description)
	}
}

func TestReadMissingClone(t *testing.T) {
	_, err := Read(t.TempDir(), "swarmery")
	if !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("err = %v, want fs.ErrNotExist", err)
	}
}

func TestReadMalformedManifest(t *testing.T) {
	dir := t.TempDir()
	writeManifest(t, dir, "swarmery", `{not json`)
	if _, err := Read(dir, "swarmery"); err == nil || errors.Is(err, fs.ErrNotExist) {
		t.Errorf("err = %v, want parse error", err)
	}
}
