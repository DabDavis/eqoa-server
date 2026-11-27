package main

import (
	"database/sql"
	"os"
	"path/filepath"
	"testing"
)

func TestImportPublishesOnlyCompleteNewDatabase(t *testing.T) {
	dir := t.TempDir()
	input := filepath.Join(dir, "dump.sql")
	dest := filepath.Join(dir, "output.db")
	const good = "CREATE TABLE records (\n  id int(11) PRIMARY KEY\n);\nINSERT INTO records VALUES (42);\n"
	for _, dump := range []string{"", "CREATE TABLE incomplete (\n", good + "INSERT INTO records VALUES (42);\n"} {
		if err := os.WriteFile(input, []byte(dump), 0600); err != nil {
			t.Fatal(err)
		}
		if err := importFile(input, dest); err == nil {
			t.Fatal("invalid import succeeded")
		}
		files, err := os.ReadDir(dir)
		if err != nil || len(files) != 1 || files[0].Name() != "dump.sql" {
			t.Fatalf("failed import left files: %v (%v)", files, err)
		}
	}
	if err := os.WriteFile(input, []byte(good), 0600); err != nil {
		t.Fatal(err)
	}
	if err := importFile(input, dest); err != nil {
		t.Fatal(err)
	}
	if err := importFile(input, dest); err == nil {
		t.Fatal("replaced existing database")
	}
	db, err := sql.Open("sqlite", dest)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var id int
	if err := db.QueryRow("SELECT id FROM records").Scan(&id); err != nil || id != 42 {
		t.Fatalf("stored row = %d (%v)", id, err)
	}
}
