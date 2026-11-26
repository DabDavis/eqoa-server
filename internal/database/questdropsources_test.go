package database

import "testing"

func TestDropSources(t *testing.T) {
	db, err := NewSQLite(t.TempDir() + "/d.db")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	for _, q := range []string{
		`INSERT INTO loottables (tablename, patternid) VALUES ('bandit sash', 701525), ('other', 9)`,
		`INSERT INTO npcloot (npc_name, tablename, chance) VALUES ('bandit', 'bandit sash', 30), ('bandit henchman', 'bandit sash', 30), ('rat', 'other', 5)`,
	} {
		if _, err := db.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	got, err := NewQuestStore(db).DropSources(map[int]bool{701525: true})
	if err != nil {
		t.Fatal(err)
	}
	if n := got[701525]; len(n) != 2 || n[0] != "bandit" || n[1] != "bandit henchman" || len(got) != 1 {
		t.Fatalf("sources %v", got)
	}
}
