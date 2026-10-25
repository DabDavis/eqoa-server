package database

import "testing"

// npcundead is made with the schema, empty, and read keyed as npcclass keys a
// name, whatever case or trailing space a GM typed.
func TestNPCUndeadTags(t *testing.T) {
	db, err := NewSQLite(t.TempDir() + "/npcundead.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	got, err := db.NPCUndeadTags()
	if err != nil || len(got) != 0 {
		t.Fatalf("an empty table: %v, %v", got, err)
	}
	if _, err := db.Exec(`INSERT INTO npcundead (name, undead, source) VALUES ('hexbone', 1, 'gm'), ('Rotting Flesh ', 0, 'gm'), ('skeleton', 1, 'seed')`); err != nil {
		t.Fatal(err)
	}
	got, err = db.NPCUndeadTags()
	if err != nil || len(got) != 3 || !got["hexbone"] || got["rotting flesh"] || !got["skeleton"] {
		t.Errorf("tags %v, %v", got, err)
	}
	if u, ok := got["rotting flesh"]; !ok || u {
		t.Errorf("the not-undead row: %v %v", u, ok)
	}
}
