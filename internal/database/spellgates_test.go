package database

import "testing"

// spellgates is made with the schema, empty, and read keyed by the spell's
// name in lower case, whatever case or spaces a GM typed; a row the game
// cannot use (a kind that is none of the three, a world past the client's
// table, a negative radius) is left out and named, and the rest still read.
func TestSpellGatesRead(t *testing.T) {
	db, err := NewSQLite(t.TempDir() + "/spellgates.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	got, bad, err := db.SpellGates()
	if err != nil || len(got) != 0 || len(bad) != 0 {
		t.Fatalf("an empty table: %v %v %v", got, bad, err)
	}
	if _, err := db.Exec(`INSERT INTO spellgates (spell, kind, place, world, x, y, z, radius, source) VALUES
		(' Nerius Gate ', 'self', 'Kara Village', 0, 21000.5, 61.25, 7667, 0, 'seed'),
		('circle of rathe', 'Group', 'Lake Rathe', 1, 7000, 54, 5000, 20, 'seed'),
		('freeport evacuation', 'evac', 'Freeport', 0, 25270, 54.1, 15730, 0, 'gm'),
		('nowhere gate', 'portal', 'x', 0, 1, 2, 3, 0, 'gm'),
		('far gate', 'self', 'x', 30, 1, 2, 3, 0, 'gm'),
		('wide portal', 'group', 'x', 0, 1, 2, 3, -5, 'gm')`); err != nil {
		t.Fatal(err)
	}
	got, bad, err = db.SpellGates()
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 || len(bad) != 3 {
		t.Fatalf("read %d rows (%v), left out %v: want 3 and the 3 bad", len(got), got, bad)
	}
	g, ok := got["nerius gate"]
	if !ok || g.Kind != GateSelf || g.World != 0 || g.X != 21000.5 || g.Y != 61.25 || g.Z != 7667 || g.Place != "Kara Village" {
		t.Errorf("nerius gate: %+v %v", g, ok)
	}
	if g := got["circle of rathe"]; g.Kind != GateGroup || g.World != 1 || g.Radius != 20 {
		t.Errorf("circle of rathe: %+v (kind read in lower case)", g)
	}
	if g := got["freeport evacuation"]; g.Kind != GateEvac {
		t.Errorf("freeport evacuation: %+v", g)
	}
	for _, k := range []string{"nowhere gate", "far gate", "wide portal"} {
		if _, ok := got[k]; ok {
			t.Errorf("%q read, want it left out", k)
		}
	}
}

// SpellGateKey is the name as a cast finds it, and CheckSpellGate takes the
// three kinds and the client's thirty worlds only.
func TestSpellGateKeyAndCheck(t *testing.T) {
	if k := SpellGateKey("  Kelinar's Gate "); k != "kelinar's gate" {
		t.Errorf("key %q", k)
	}
	ok := SpellGate{Spell: "x", Kind: GateEvac}
	if err := CheckSpellGate(ok, 29); err != nil {
		t.Errorf("world 29: %v", err)
	}
	for _, c := range []struct {
		g     SpellGate
		world int
	}{
		{SpellGate{Spell: " ", Kind: GateSelf}, 0},
		{SpellGate{Spell: "x", Kind: "Self"}, 0}, // the reader folds case; the check takes the folded word
		{SpellGate{Spell: "x", Kind: GateSelf}, -1},
		{SpellGate{Spell: "x", Kind: GateGroup, Radius: -1}, 0},
	} {
		if err := CheckSpellGate(c.g, c.world); err == nil {
			t.Errorf("%+v in world %d passed", c.g, c.world)
		}
	}
}
