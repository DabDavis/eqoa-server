package database

import (
	"database/sql"
	"os"
	"path/filepath"
	"testing"
)

func TestOfflineDatabaseReadsWALWithoutMigratingSource(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "source.sqlite")
	src, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer src.Close()
	// Hold this connection open so the new row remains in the WAL.
	src.SetMaxOpenConns(1)
	if _, err := src.Exec("PRAGMA journal_mode=WAL; PRAGMA wal_autocheckpoint=0; CREATE TABLE sample(n); INSERT INTO sample VALUES (42)"); err != nil {
		t.Fatal(err)
	}
	inspect, err := OpenSQLiteReadOnly(path)
	if err != nil {
		t.Fatal(err)
	}
	defer inspect.Close()
	if _, err := inspect.Exec("INSERT INTO sample VALUES (99)"); err == nil {
		t.Fatal("read-only diagnostic handle accepted a write")
	}
	out := filepath.Join(dir, "copy.sqlite")
	if err := CopySQLiteFile(path, out); err != nil {
		t.Fatal(err)
	}
	if err := CopySQLiteFile(path, out); err == nil {
		t.Fatal("overwrote an existing backup")
	}
	copy, err := OpenSQLiteReadOnly(out)
	if err != nil {
		t.Fatal(err)
	}
	defer copy.Close()
	var n int
	if err := copy.QueryRow("SELECT n FROM sample").Scan(&n); err != nil || n != 42 {
		t.Fatalf("copied committed WAL row: %d, %v", n, err)
	}
	if err := inspect.QueryRow("SELECT COUNT(*) FROM sqlite_schema WHERE type='table'").Scan(&n); err != nil || n != 1 {
		t.Fatalf("source schema count: %d, %v", n, err)
	}
	if fi, err := os.Stat(out); err != nil || fi.Mode().Perm()&0o077 != 0 {
		t.Fatalf("copy is not private: %v, %v", fi, err)
	}
	missing := filepath.Join(dir, "missing")
	if _, err := OpenSQLiteReadOnly(missing); err == nil {
		t.Fatal("opened a missing database")
	}
	if _, err := os.Stat(missing); !os.IsNotExist(err) {
		t.Fatalf("diagnostic created the input: %v", err)
	}
}
