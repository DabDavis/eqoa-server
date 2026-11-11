package database

import (
	"database/sql"
	"testing"
)

// Petition 1535: an npcaggro made before leash gets it at the next start, its
// rows kept and reading 0 (the world's leash); a row's leash and an
// aggressive of -1 (no word on aggro) read back. Made-up names.
func TestNPCAggroGainsLeash(t *testing.T) {
	path := t.TempDir() + "/old.db"
	raw, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := raw.Exec(`CREATE TABLE npcaggro (npc_name TEXT PRIMARY KEY, aggressive INTEGER NOT NULL DEFAULT 1,
		radius REAL NOT NULL DEFAULT 0, note TEXT NOT NULL DEFAULT '', source TEXT NOT NULL DEFAULT 'gm');
		INSERT INTO npcaggro (npc_name, aggressive, radius) VALUES ('test bandit', 1, 40)`); err != nil {
		t.Fatal(err)
	}
	raw.Close()
	db, err := NewSQLite(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	rows, err := db.NPCAggroRules()
	if err != nil || len(rows) != 1 || !rows[0].Aggressive || rows[0].NoWord || rows[0].Radius != 40 || rows[0].Leash != 0 {
		t.Fatalf("the old row after the start: %+v (%v)", rows, err)
	}
	if _, err := db.Exec(`INSERT INTO npcaggro (npc_name, aggressive, leash) VALUES ('test phantom', -1, 600)`); err != nil {
		t.Fatal(err)
	}
	rows, _ = db.NPCAggroRules()
	for _, r := range rows {
		if r.Name == "test phantom" && (r.Aggressive || !r.NoWord || r.Leash != 600) {
			t.Errorf("the leash row: %+v", r)
		}
	}
	if len(rows) != 2 {
		t.Errorf("%d rows, want 2", len(rows))
	}
}
