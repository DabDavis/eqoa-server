package database

import (
	"path/filepath"
	"testing"
)

// A new database's raidbosslist holds the seed (source seed): the bosses the owner named
// (Bertoxxulous, Poxathixius, Uzrathixius, Lord Nagafen) and none of the raid trash the
// loot's rule took for a raid boss, each name once; a GM's row is read as its key
// whatever its case; a database that has the table keeps what is in it, rows a GM took
// out included; its names are none a snapshot refuses.
func TestRaidBossListIsMadeWithItsBosses(t *testing.T) {
	path := filepath.Join(t.TempDir(), "raidbosses.db")
	db, err := NewSQLite(path)
	if err != nil {
		t.Fatal(err)
	}
	var n int
	if err := db.QueryRow(`SELECT count(*) FROM raidbosslist WHERE source = 'seed'`).Scan(&n); err != nil || n != len(raidBossListSeed) {
		t.Fatalf("a new database's seed: %d rows (%v), want %d", n, err, len(raidBossListSeed))
	}
	names, err := db.RaidBossList()
	if err != nil {
		t.Fatal(err)
	}
	listed := map[string]bool{}
	for _, k := range names {
		listed[k] = true
	}
	if len(listed) != len(raidBossListSeed) {
		t.Errorf("%d names read, %d distinct, the seed %d: a name twice", len(names), len(listed), len(raidBossListSeed))
	}
	for _, boss := range []string{"bertoxxulous", "poxathixius", "uzrathixius", "lord nagafen", "queen qy'tha", "cazicthule", "mr. moogie"} {
		if !listed[boss] {
			t.Errorf("%q is not listed", boss)
		}
	}
	// the loot's rule's raid trash and nameds, each on Heirloom's 2026-11-22 list
	for _, trash := range []string{"welahi-nai serpent", "reed cobra", "khal assault infantry", "rallosian captain",
		"bloodletter", "construct of frost", "aviak elder", "death mist", "wrath of the mother", "efreeti lord djarn",
		"graal the leviathan", "terrorantula"} {
		if listed[trash] {
			t.Errorf("%q is listed: raid trash or a named, not a boss", trash)
		}
	}

	if _, err := db.Exec(`INSERT INTO raidbosslist (npc_name, note) VALUES ('  Some Dragon$ ', 'a GM''s')`); err != nil {
		t.Fatal(err)
	}
	if names, err = db.RaidBossList(); err != nil {
		t.Fatal(err)
	}
	found := false
	for _, k := range names {
		found = found || k == "some dragon"
	}
	if !found {
		t.Errorf("a GM's row in capitals is not read as its key: %q", names)
	}

	if _, err := db.Exec(`DELETE FROM raidbosslist`); err != nil {
		t.Fatal(err)
	}
	db.Close()
	if db, err = NewSQLite(path); err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := db.QueryRow(`SELECT count(*) FROM raidbosslist`).Scan(&n); err != nil || n != 0 {
		t.Errorf("the bosses a GM took out came back: %d (%v)", n, err)
	}
	for _, name := range []string{"raidbosslist", "npc_name", "note", "source"} {
		if SecretLikeName(name) {
			t.Errorf("%q is named like a secret: every snapshot would be refused", name)
		}
	}
}
