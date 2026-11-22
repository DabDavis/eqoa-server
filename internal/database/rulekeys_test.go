package database

import (
	"slices"
	"testing"
)

// A rule row stored with capitals counts (board 743): an npcclass row
// "Acolyte Satyr" class 3 classes creatures named "acolyte satyr" and "Acolyte
// Satyr", read whole (NPCClasses, by NPCClassKey) and one at a time
// (NPCClassOf); where a row stored as its key meets it, that row wins. A
// lootcreature row of profile "boss" in capitals makes a raid boss of both
// spellings, and RaidBossNames gives its key. The control: a name with no row
// is a warrior and no boss.
func TestRuleRowsInCapitalsCount(t *testing.T) {
	db := heroesDB(t)
	for _, q := range []string{
		`INSERT INTO npcclass (name, class, source) VALUES ('Acolyte Satyr', 3, 'sandstorm-census'), ('Grey Wolf', 1, 'x'), ('grey wolf', 7, 'gm'), ('Dread Bat$', 12, 'x')`,
		`INSERT INTO lootcreature (npc_name, region, profile) VALUES ('Lady Vox', 'permafrost', 'boss')`,
	} {
		if _, err := db.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	s := NewNPCStore(db)
	classes, err := s.NPCClasses()
	if err != nil {
		t.Fatal(err)
	}
	if classes["acolyte satyr"] != 3 || classes["grey wolf"] != 7 || classes["dread bat"] != 12 || len(classes) != 3 {
		t.Errorf("NPCClasses: %v", classes)
	}
	for name, want := range map[string]int{"acolyte satyr": 3, "Acolyte Satyr": 3, " ACOLYTE SATYR$": 3, "Grey Wolf": 7, "dread bat": 12, "a rat": 0} {
		if got, err := s.NPCClassOf(name); err != nil || got != want {
			t.Errorf("NPCClassOf(%q) = %d (%v), want %d", name, got, err, want)
		}
	}
	inv := NewInventoryStore(db)
	for name, want := range map[string]bool{"lady vox": true, "Lady Vox": true, "a rat": false} {
		if boss, err := inv.RaidBoss(name); err != nil || boss != want {
			t.Errorf("RaidBoss(%q) = %v (%v)", name, boss, err)
		}
	}
	if names, err := db.RaidBossNames(); err != nil || !slices.Equal(names, []string{"lady vox"}) {
		t.Errorf("RaidBossNames %q (%v)", names, err)
	}
}
