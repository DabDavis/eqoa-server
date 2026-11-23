package database

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestNewServerDatabaseIsPrivate(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX file permissions")
	}
	path := filepath.Join(t.TempDir(), "accounts #1?.db")
	db, err := NewSQLite(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	for _, suffix := range []string{"", "-wal", "-shm"} {
		info, err := os.Stat(path + suffix)
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm()&0077 != 0 {
			t.Fatalf("database file %q has mode %o", suffix, info.Mode().Perm())
		}
	}
}
