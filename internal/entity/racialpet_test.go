package entity

import (
	"testing"

	"github.com/eqoa/server/internal/database"
)

// A racial ability's pet has the same hit points whatever its owner's class
// (racialpet.go, the owner 2026-11-13): a magician's racial pet takes no
// share of the magician's pet_hp_pct (175% at 50 here), a magician's own pet
// still does; and which ranks are a race's: the racial category, a master
// class of some races (the gnome's Construct, the Mermaid's Siren Royal), not
// a rank every base race may buy.
func TestARacialPetsHitPointsAreTheSameForEveryClass(t *testing.T) {
	w := NewWorld()
	const classMagician = 10
	mag := &Player{ServerID: 1001, Class: classMagician, Level: 50, HP: 900, MaxHP: 900}
	w.Players[mag.ServerID] = mag
	own := &NPC{ServerID: 3000, Name: "Air Elemental", Level: 45, Owner: mag.ServerID}
	racial := &NPC{ServerID: 3001, Name: "Construct", Level: 45, Owner: mag.ServerID, racialPet: true}
	for _, n := range []*NPC{own, racial} {
		n.HP, n.MaxHP = 1, 1
		w.NPCs[n.ServerID] = n
	}
	b := testBalance().WithClassLevels([]database.BalanceClassLevel{
		{Class: classMagician, Level: 40, MeleeTakenPct: 100, PetHPPct: 150},
		{Class: classMagician, Level: 60, MeleeTakenPct: 100, PetHPPct: 200}})
	w.SetBalance(b)
	plain := pct(NPCMaxHP(45), b.Level(50).PetHPPct)
	if want := pct(plain, 175); own.MaxHP != want {
		t.Errorf("the magician's own pet: %d hit points, want %d", own.MaxHP, want)
	}
	if racial.MaxHP != plain {
		t.Errorf("the magician's racial pet: %d hit points, want the level's %d whatever the class", racial.MaxHP, plain)
	}

	w.SetCMCatalog([]database.CMRankRow{
		{ID: 1542, Category: CMCategoryMaster, Classes: 32767, Races: 8, Name: "Construct", AddSpells: "901835", Adds: "Construct@1", Enabled: true},
		{ID: 1911, Category: 1, Classes: 32767, Races: 32, Name: "Swamp Friend 1", AddSpells: "901796", Adds: "Swamp Friend@1", Enabled: true},
		{ID: 2181, Category: 1, Classes: 32767, Races: 1 << 32, Name: "Aquatic Birthright 5", AddSpells: "760184", Adds: "Summon Ghilan@55", Enabled: true},
		{ID: 271, Category: 0, Classes: 32767, Races: 2047, Name: "First Aid 1", AddSpells: "901780", Adds: "First Aid@1", Enabled: true},
		{ID: 9001, Category: CMCategoryMaster, Classes: 1 << 10, Races: 0, Name: "A class master", AddSpells: "9901", Adds: "Pet@60", Enabled: true},
	})
	w.mu.RLock()
	defer w.mu.RUnlock()
	for spell, want := range map[int]bool{901835: true, 901796: true, 760184: true, 901780: false, 9901: false} {
		if got := w.racialSpellLocked(spell); got != want {
			t.Errorf("spell %d racial %v, want %v", spell, got, want)
		}
	}
}
