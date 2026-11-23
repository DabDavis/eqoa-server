package database

import (
	"database/sql"
	"path/filepath"
	"strings"
	"testing"
)

// campspawns' weights (placeholder.go): a table made before them is given the
// column, its rows read at even odds; /ph's writes are audited, a placeholder's
// row is camp 0, a camp's spawn's keeps its camp, and a clear deletes the one
// and gives the other its own kind alone.
func TestPlaceholderRows(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ph.db")
	old, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := old.Exec(`CREATE TABLE campspawns (npcid INTEGER PRIMARY KEY, camp INTEGER NOT NULL DEFAULT 0, kinds TEXT NOT NULL DEFAULT '');
		INSERT INTO campspawns VALUES (7, 3, 'wolf, pariah of Xrag')`); err != nil {
		t.Fatal(err)
	}
	old.Close()
	db, err := NewSQLite(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	r, has, err := db.CampSpawnOf(7)
	if err != nil || !has || r.Camp != 3 || len(r.Kinds) != 2 || r.Weights != nil {
		t.Fatalf("the old row: %+v %v %v", r, has, err)
	}
	who := GMEdit{AccountID: 1, Username: "a GM", Via: "game"}

	// a placeholder: inserted at camp 0, then changed, then unchanged
	if a, err := db.SetPlaceholderAudited(9, []string{"wolf", "pariah of Xrag"}, []int{90, 10}, who, "/ph"); err != nil || a != "insert" {
		t.Fatalf("insert: %q %v", a, err)
	}
	if a, err := db.SetPlaceholderAudited(9, []string{"wolf", "pariah of Xrag"}, nil, who, "/ph"); err != nil || a != "update" {
		t.Fatalf("update: %q %v", a, err)
	}
	if a, err := db.SetPlaceholderAudited(9, []string{"wolf", "pariah of Xrag"}, nil, who, "/ph"); err != nil || a != "unchanged" {
		t.Fatalf("unchanged: %q %v", a, err)
	}
	if _, err := db.SetPlaceholderAudited(9, []string{"a", "b", "c", "d", "e", "f"}, nil, who, "/ph"); err == nil {
		t.Fatal("six kinds taken")
	}
	if r, _, _ := db.CampSpawnOf(9); r.Camp != 0 || r.Weights != nil {
		t.Fatalf("the placeholder: %+v", r)
	}
	// the camp's spawn: its camp kept
	if a, err := db.SetPlaceholderAudited(7, []string{"wolf", "pariah of Xrag"}, []int{60, 40}, who, "/ph"); err != nil || a != "update" {
		t.Fatalf("the camp's spawn: %q %v", a, err)
	}
	if r, _, _ := db.CampSpawnOf(7); r.Camp != 3 || len(r.Weights) != 2 || r.Weights[0] != 60 {
		t.Fatalf("the camp's spawn: %+v", r)
	}
	// clears
	if a, err := db.ClearPlaceholderAudited(9, "wolf", who, "/ph clear"); err != nil || a != "delete" {
		t.Fatalf("clear: %q %v", a, err)
	}
	if _, has, _ := db.CampSpawnOf(9); has {
		t.Fatal("the placeholder's row is still there")
	}
	if a, err := db.ClearPlaceholderAudited(9, "wolf", who, "/ph clear"); err != nil || a != "" {
		t.Fatalf("clear again: %q %v", a, err)
	}
	if a, err := db.ClearPlaceholderAudited(7, "wolf", who, "/ph clear"); err != nil || a != "update" {
		t.Fatalf("clear the camp's: %q %v", a, err)
	}
	if r, _, _ := db.CampSpawnOf(7); r.Camp != 3 || strings.Join(r.Kinds, "|") != "wolf" || r.Weights != nil {
		t.Fatalf("the camp's spawn cleared: %+v", r)
	}
	var lines []string
	rows, err := db.Query(`SELECT action || ' ' || detail FROM gm_audit WHERE tbl = 'campspawns' ORDER BY id`)
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var l string
		rows.Scan(&l)
		lines = append(lines, l)
	}
	rows.Close()
	if got := strings.Join(lines, "; "); got != "insert /ph; update /ph; update /ph; delete /ph clear; update /ph clear" {
		t.Fatalf("audit lines: %s", got)
	}
}
