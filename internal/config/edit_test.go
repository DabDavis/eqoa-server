package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestConfigEditsPreserveSeparatePrivateBackups(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	const original = "# original\nworld:\n  start_level: 1\n"
	if err := os.WriteFile(path, []byte(original), 0o644); err != nil {
		t.Fatal(err)
	}
	first, err := SetInFile(path, map[string]any{"world.start_level": 2})
	if err != nil {
		t.Fatal(err)
	}
	second, err := SetInFile(path, map[string]any{"world.start_level": 3})
	if err != nil {
		t.Fatal(err)
	}
	if first == second {
		t.Fatal("edits reused a backup path")
	}
	prev, err := os.ReadFile(first)
	if err != nil || string(prev) != original {
		t.Fatalf("first backup %q, %v", prev, err)
	}
	if fi, err := os.Stat(second); err != nil || fi.Mode().Perm()&0o077 != 0 {
		t.Fatalf("backup should be private: %v, %v", fi, err)
	}
}
