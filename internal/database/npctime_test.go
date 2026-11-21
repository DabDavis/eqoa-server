package database

import (
	"path/filepath"
	"testing"
)

// npctime's rows read as keys to day or night: a name by NPCClassKey, a "#n"
// row key as it is, and a time other than day or night counts as no row.
func TestNPCTimesRead(t *testing.T) {
	db, err := NewSQLite(filepath.Join(t.TempDir(), "eqoa.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`INSERT INTO npctime (name, time) VALUES ('Deathfist Mercenary', 'Day'),
		(' skeletal watchman ', 'night'), ('#111141', ' NIGHT'), ('Grug Ironhead', 'dusk')`); err != nil {
		t.Fatal(err)
	}
	got, err := db.NPCTimes()
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{"deathfist mercenary": "day", "skeletal watchman": "night", "#111141": "night"}
	if len(got) != len(want) {
		t.Fatalf("read %v, want %v", got, want)
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("%q: %q, want %q", k, got[k], v)
		}
	}
}
