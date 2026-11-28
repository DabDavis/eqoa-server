package database

import (
	"errors"
	"testing"
)

func TestHeroicMarksRedemption(t *testing.T) {
	db, chars := newCharDB(t)
	set := HeroicMarksFor("mythic_nagafen")
	const godTouched = "FR:25,CR:25,LR:25,AR:25,PR:25,DR:25"
	if _, err := db.Exec(`INSERT INTO cmcatalog (id,name,mastery,rank,cost,level,spent,effects)
		VALUES (?, 'God Touched', 'God Touched', 1, 75, 50, 500, ?)`, set.Mastery.Base, godTouched); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if err := db.InstallHeroicMarks(); err != nil {
			t.Fatal(err)
		}
	}
	var name, effects string
	var rank, parent, cost, level, spent int
	if err := db.QueryRow(`SELECT name,rank,parent,cost,level,spent,effects FROM cmcatalog WHERE id=?`, set.Mastery.ID).
		Scan(&name, &rank, &parent, &cost, &level, &spent, &effects); err != nil {
		t.Fatal(err)
	}
	if name != "God Touched II" || rank != 2 || parent != set.Mastery.Base || cost != 75 || level != 50 || spent != 500 || effects != godTouched {
		t.Fatalf("bad second rank: %q rank %d parent %d cost %d level %d spent %d effects %q", name, rank, parent, cost, level, spent, effects)
	}
	a := &CharacterRecord{AccountID: 1, CharName: "Ember"}
	b := &CharacterRecord{AccountID: 1, CharName: "Ash"}
	for _, c := range []*CharacterRecord{a, b} {
		if err := chars.Create(c); err != nil {
			t.Fatal(err)
		}
	}
	inv := NewInventoryStore(db)
	role, wrong := set.RoleFor(7), set.RoleFor(0)
	put := func(pat, qty, loc int) {
		t.Helper()
		if _, err := inv.AddCharItem(a.ServerID, pat, qty, 0, 0, loc); err != nil {
			t.Fatal(err)
		}
	}
	spend := func(reward HeroicReward, limit int, want error) HeroicRedemption {
		t.Helper()
		got, err := inv.RedeemHeroicMarks(a.ServerID, 7, limit, set.Boss, reward)
		if !errors.Is(err, want) {
			t.Fatalf("redeem %d: got %v, want %v", reward, err, want)
		}
		return got
	}
	put(wrong.MarkID, 99, 0)
	put(role.MarkID, 99, 1)
	spend(HeroicUnlock, 40, ErrHeroicShort)
	put(role.MarkID, 1, 0)
	put(role.MarkID, 2, 0)
	if got := spend(HeroicUnlock, 40, nil); len(got.Taken) != 2 {
		t.Fatalf("unlock should use two stacks: %+v", got.Taken)
	}
	spend(HeroicUnlock, 40, ErrHeroicClaimed)
	for _, c := range []*CharacterRecord{a, b} {
		flags, err := chars.HeroicMasteryUnlocks(c.ServerID)
		if err != nil || flags[set.Boss] != (c == a) {
			t.Fatalf("%s unlocks %v: %v", c.CharName, flags, err)
		}
	}
	put(role.MarkID, 7, 0)
	spend(HeroicRelic, 2, ErrHeroicPackFull)
	ring := spend(HeroicRelic, 40, nil).Item
	if ring.Pattern.PatternID != role.RelicID || ring.Pattern.Trade != 1 || ring.Pattern.Lore != 1 {
		t.Fatalf("bad relic: %+v", ring)
	}
	if err := inv.DeleteCharItem(a.ServerID, ring.ItemID); err != nil {
		t.Fatal(err)
	}
	spend(HeroicRelic, 40, ErrHeroicClaimed)
	for range 2 {
		stone := spend(HeroicTeleport, 40, nil).Item
		if stone.Charges != 50 || stone.Pattern.PatternID != set.Teleport.ID {
			t.Fatalf("bad teleport item: %+v", stone)
		}
	}
	spend(HeroicTeleport, 40, ErrHeroicShort)
	spell, err := NewSpellbookStore(db).SpellPattern(set.Teleport.Spell)
	if err != nil || spell.CastTime != 2 {
		t.Fatalf("teleport definition: %+v, %v", spell, err)
	}
}
