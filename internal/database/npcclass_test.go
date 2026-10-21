package database

import "testing"

// npcclass keys a name as the server looks it up: trimmed, any case, no
// trailing '$'.
func TestNPCClasses(t *testing.T) {
	for in, want := range map[string]string{"Crag Wisp": "crag wisp", "crag wisp$": "crag wisp", "Gharial ": "gharial", " Guard Findel ": "guard findel"} {
		if got := NPCClassKey(in); got != want {
			t.Errorf("NPCClassKey(%q) = %q, want %q", in, got, want)
		}
	}
	db, err := NewSQLite(t.TempDir() + "/npcclass.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	got, err := NewNPCStore(db).NPCClasses()
	if err != nil || len(got) != 0 {
		t.Fatalf("an empty table: %v, %v", got, err)
	}
	if _, err := db.Exec(`INSERT INTO npcclass (name, class, source) VALUES ('crag wisp', 10, 'sandstorm'), ('guard findel', 3, 'sandstorm')`); err != nil {
		t.Fatal(err)
	}
	got, err = NewNPCStore(db).NPCClasses()
	if err != nil || got["crag wisp"] != 10 || got["guard findel"] != 3 || len(got) != 2 {
		t.Errorf("classes %v, %v", got, err)
	}
}
