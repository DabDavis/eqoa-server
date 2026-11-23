package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/eqoa/server/internal/database"
)

func TestJSONFilesShareOneImport(t *testing.T) {
	dir := t.TempDir()
	db, err := database.NewSQLite(filepath.Join(dir, "data.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	items, spells := filepath.Join(dir, "items.json"), filepath.Join(dir, "spells.json")
	if err := os.WriteFile(items, []byte(`[{"ItemID": 71, "Stats": {"12": 7}}]`), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(spells, []byte("["), 0600); err != nil {
		t.Fatal(err)
	}
	if err := importJSON(db, items, spells); err == nil {
		t.Fatal("accepted incomplete spell input")
	}
	patterns := database.NewItemPatternStore(db)
	if n, err := patterns.Count(); err != nil || n != 0 {
		t.Fatalf("items after rollback: %d, %v", n, err)
	}
	if err := os.Remove(spells); err != nil {
		t.Fatal(err)
	}
	if err := importJSON(db, items, spells); err != nil {
		t.Fatal(err)
	}
	rec, err := patterns.GetItemPattern(71)
	if err != nil || rec.HoT == nil || *rec.HoT != 7 || rec.PoT != nil {
		t.Fatalf("item-only import: %+v, %v", rec, err)
	}
}
