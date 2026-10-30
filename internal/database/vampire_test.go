package database

import "testing"

// A vampire is a character's row, written with its gm_audit line as the
// panel writes a row edit (so the panel lists and reverts it): on makes one
// and says so once, off takes it back, and a deleted character's goes with
// it. The controls: another character's stays, and nothing of the table is
// named like a secret, which would refuse every database snapshot
// (snapshot.go).
func TestAVampireIsKeptAndAudited(t *testing.T) {
	db, err := NewSQLite(t.TempDir() + "/vampire.db")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	who := GMEdit{AccountID: 1, Username: "dab", Via: "game"}
	if v, err := db.IsVampire(7); err != nil || v {
		t.Fatalf("no row: %v (%v)", v, err)
	}
	audit, err := db.SetVampire(7, true, who, "#vampire in the game")
	if err != nil || audit == 0 {
		t.Fatalf("made one: audit %d (%v)", audit, err)
	}
	if v, _ := db.IsVampire(7); !v {
		t.Fatal("not kept")
	}
	if again, err := db.SetVampire(7, true, who, "again"); err != nil || again != 0 {
		t.Errorf("made one again: audit %d (%v), want nothing changed", again, err)
	}
	var action, tbl, key, after, note string
	db.QueryRow(`SELECT action, tbl, rowkey, after FROM gm_audit WHERE id = ?`, audit).Scan(&action, &tbl, &key, &after)
	db.QueryRow(`SELECT note FROM charvampire WHERE serverid = 7`).Scan(&note)
	if action != "insert" || tbl != "charvampire" || key != `{"serverid":7}` || after == "" || note != "made a vampire by GM dab in the game" {
		t.Errorf("the audit line: %s %s %s %s; note %q", action, tbl, key, after, note)
	}
	db.SetVampire(9, true, who, "")
	if off, err := db.SetVampire(7, false, who, "#vampire off"); err != nil || off == 0 {
		t.Fatalf("taken back: audit %d (%v)", off, err)
	}
	if v, _ := db.IsVampire(7); v {
		t.Error("still a vampire")
	}
	if n, _ := db.Vampires(); n != 1 {
		t.Errorf("%d vampires, want 9 alone", n)
	}

	s := NewSQLiteCharacterStore(db)
	c := &CharacterRecord{CharName: "Bitten", AccountID: 1}
	if err := s.Create(c); err != nil {
		t.Fatal(err)
	}
	db.SetVampire(c.ServerID, true, who, "")
	if err := s.Delete(c.ServerID); err != nil {
		t.Fatal(err)
	}
	if v, _ := db.IsVampire(c.ServerID); v {
		t.Errorf("deleted character %d is still a vampire", c.ServerID)
	}
	if v, _ := db.IsVampire(9); !v {
		t.Error("the control: character 9 is no longer one")
	}
	for _, name := range []string{"charvampire", "serverid", "since", "note", "source"} {
		if SecretLikeName(name) {
			t.Errorf("%q reads like a secret: every snapshot would be refused", name)
		}
	}
}
