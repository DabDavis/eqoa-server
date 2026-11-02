package database

import "testing"

// npccharm is made with the schema, empty, and read keyed as npcclass keys a
// name (any case, a trailing space or $ off), its resist held to 0-100 and its
// template keyed as classtemplates keys one.
func TestNPCCharmRules(t *testing.T) {
	db, err := NewSQLite(t.TempDir() + "/npccharm.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	got, err := db.NPCCharmRules()
	if err != nil || len(got) != 0 {
		t.Fatalf("an empty table: %v, %v", got, err)
	}
	if _, err := db.Exec(`INSERT INTO npccharm (name, charmable, resist_pct, template, source) VALUES
		('Lady Vox ', 0, 0, '', 'seed'), ('hand of thule$', 1, 185, '', 'seed'), ('hollow', 1, 0, ' Charm Tree ', 'gm'),
		('ebony recluse', 1, -5, '', 'gm')`); err != nil {
		t.Fatal(err)
	}
	got, err = db.NPCCharmRules()
	if err != nil || len(got) != 4 {
		t.Fatalf("rules %v, %v", got, err)
	}
	for name, want := range map[string]CharmRule{
		"lady vox":      {Charmable: false},
		"hand of thule": {Charmable: true, ResistPct: 100},
		"hollow":        {Charmable: true, Template: "charm tree"},
		"ebony recluse": {Charmable: true},
	} {
		if got[name] != want {
			t.Errorf("%q: %+v, want %+v", name, got[name], want)
		}
	}
}
