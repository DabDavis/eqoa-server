package database

import "testing"

// A pet's dyes (petdyes.go): each area set apart, by the body, read back as
// written; each change one gm_audit line of the row before and after; a row
// left with no dye deleted; a same colour again nothing; alpha 0 and an area
// past 2 refused. The controls: another body's and another character's rows
// untouched.
func TestPetDyes(t *testing.T) {
	db, err := NewSQLite(t.TempDir() + "/petdyes.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	who := GMEdit{Username: "zerkziee", Via: "game"}
	const look, other = 0x15764DA8, 0x9CF72381
	if _, a, err := db.SetPetDye(747, look, 0, 0xD10000FF, who, "the Pet Groomer"); err != nil || a == 0 {
		t.Fatalf("area 1: audit %d, %v", a, err)
	}
	if _, a, err := db.SetPetDye(747, look, 1, 0x008000FF, who, "the Pet Groomer"); err != nil || a == 0 {
		t.Fatalf("area 2: audit %d, %v", a, err)
	}
	if _, _, err := db.SetPetDye(747, other, 0, 0x0000FFFF, who, "x"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := db.SetPetDye(748, look, 0, 0xFFFF00FF, who, "x"); err != nil {
		t.Fatal(err)
	}
	got, err := db.PetDyesOf(747)
	if err != nil || len(got) != 2 || got[look] != [PetDyeAreas]uint32{0xD10000FF, 0x008000FF} || got[other] != [PetDyeAreas]uint32{0x0000FFFF, 0} {
		t.Fatalf("747's: %x, %v", got, err)
	}
	if before, a, err := db.SetPetDye(747, look, 1, 0x008000FF, who, "x"); err != nil || a != 0 || before != 0x008000FF {
		t.Fatalf("the same again: before %08X audit %d %v, want nothing written", before, a, err)
	}
	var n int
	if err := db.QueryRow(`SELECT count(*) FROM gm_audit WHERE tbl = 'petdyes'`).Scan(&n); err != nil || n != 4 {
		t.Fatalf("audit lines %d, %v: want 4", n, err)
	}
	for area := range 2 {
		if _, _, err := db.SetPetDye(747, look, area, 0, who, "x"); err != nil {
			t.Fatal(err)
		}
	}
	if err := db.QueryRow(`SELECT count(*) FROM petdyes WHERE serverid = 747 AND look = ?`, look).Scan(&n); err != nil || n != 0 {
		t.Fatalf("both off: %d rows, %v; want the row gone", n, err)
	}
	var action string
	if err := db.QueryRow(`SELECT action FROM gm_audit WHERE tbl = 'petdyes' ORDER BY id DESC LIMIT 1`).Scan(&action); err != nil || action != "delete" {
		t.Fatalf("the last line %q, %v: want a delete", action, err)
	}
	if got, _ := db.PetDyesOf(748); got[look] != [PetDyeAreas]uint32{0xFFFF00FF, 0} {
		t.Errorf("the control: another character's %x", got)
	}
	if _, _, err := db.SetPetDye(747, look, 0, 0xD1000000, who, "x"); err == nil {
		t.Error("alpha 0 taken")
	}
	if _, _, err := db.SetPetDye(747, look, 2, 0xD10000FF, who, "x"); err == nil {
		t.Error("a third area taken")
	}
}

// A body's areas read by its key, area 2's materials a mask; a list it
// cannot read left out and said (the control: the good row read).
func TestPetDyeAreas(t *testing.T) {
	db, err := NewSQLite(t.TempDir() + "/petdyeareas.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if _, err := db.Exec(`INSERT INTO petdyeareas (look, area1, area2, materials) VALUES
		(360074664, 'Torso', 'Spider body', '1, 3;63'), (2633457281, 'Body', 'Gears', '64')`); err != nil {
		t.Fatal(err)
	}
	got, bad, err := db.PetDyeAreaRows()
	if err != nil || len(bad) != 1 || len(got) != 1 {
		t.Fatalf("%v bad %v %v: want one read, one left out", got, bad, err)
	}
	a := got[0x15764DA8]
	if a.Area1 != "Torso" || a.Area2 != "Spider body" || a.Mask != 1<<1|1<<3|1<<63 || !a.TwoAreas() {
		t.Errorf("%+v", a)
	}
	if FormatPetDyeMaterials(a.Mask) != "1,3,63" {
		t.Errorf("formatted %q", FormatPetDyeMaterials(a.Mask))
	}
	if (PetDyeArea{Area2: "x"}).TwoAreas() || (PetDyeArea{Mask: 1}).TwoAreas() {
		t.Error("a second area with no materials or no name")
	}
}
