package entity

import (
	"testing"
	"time"

	"github.com/eqoa/server/internal/database"
)

// A merc's pet with a Master Class and its race's pets (mercmaster.go): a
// racial summoning (the gnome's Invention) only when its book has no other,
// a Master Class's (Tinker's Construct) before its class's book's (the
// magician's earth line, its usual), its hirer's own pick of a kind before
// the Master Class's.
func TestMercMasterPetChoice(t *testing.T) {
	sum := func(id int, name string, level int) CastSpell {
		s := bookSpell(id, name, 0, 3*time.Second, 5*time.Second, 50, "Summons a pet to aid you")
		s.Level = level
		return s
	}
	invention, construct := sum(901789, "Invention", 6), sum(901835, "Construct", 60)
	fire, badi := sum(717, "Fire Elementaling", 8), sum(701086, "Summon Badi Elder", 60)
	if !invention.Def.Summons() {
		t.Fatal("the rig's summoning does not summon")
	}
	pet := func(k MercKit) string {
		if s := MercPetFor(k); s != nil {
			return s.Name
		}
		return ""
	}
	for _, c := range []struct {
		name string
		kit  MercKit
		want string
	}{
		{"a gnome warrior", MercKit{Class: 0, Spells: []CastSpell{invention}, RacialPets: []int{901789}}, "Invention"},
		{"a gnome magician", MercKit{Class: 10, Spells: []CastSpell{invention, fire}, RacialPets: []int{901789}}, "Fire Elementaling"},
		{"a gnome warrior Tinker", MercKit{Class: 0, Spells: []CastSpell{invention, construct}, RacialPets: []int{901789},
			MasterPets: []int{901835}}, "Construct"},
		{"a gnome magician Tinker, its usual earth", MercKit{Class: 10, Spells: []CastSpell{invention, fire, badi, construct},
			RacialPets: []int{901789}, MasterPets: []int{901835}, PetPick: []string{"earth", "badi"}, PetUsual: true}, "Construct"},
		{"a gnome magician Tinker, its hirer's pick", MercKit{Class: 10, Spells: []CastSpell{invention, fire, badi, construct},
			RacialPets: []int{901789}, MasterPets: []int{901835}, PetPick: []string{"fire"}}, "Fire Elementaling"},
		{"a magician, no Master Class", MercKit{Class: 10, Spells: []CastSpell{fire, badi}, PetPick: []string{"earth", "badi"}, PetUsual: true},
			"Summon Badi Elder"},
	} {
		if got := pet(c.kit); got != c.want {
			t.Errorf("%s: %q, want %q", c.name, got, c.want)
		}
	}
}

// A Master Class's ranks are on a merc's body as a player's ranks are
// (refreshCMLocked): Tinker's first rank's 150 hit points and 150 power.
func TestMercMasterBodyStats(t *testing.T) {
	r := newMercRig(t)
	plain := r.kit(0, 3, 0)
	k := plain
	k.Master = "Tinker"
	k.MasterRanks = []*CMRank{ParseCMRank(database.CMRankRow{ID: 1541, Category: CMCategoryMaster, Classes: 32767, Races: 8,
		Mastery: "Tinker", Rank: 1, Name: "Tinker", Level: 55, Effects: "AGIMax:50,INTMax:50,HP:150,Pow:150,PoT:20", Enabled: true})}
	a, b := newMercBody(1, &plain), newMercBody(2, &k)
	if got := b.MaxHPNow() - a.MaxHPNow(); got != 150 {
		t.Errorf("Tinker's hit points: +%d, want +150", got)
	}
	if got := b.MaxPowerNow() - a.MaxPowerNow(); got != 150 {
		t.Errorf("Tinker's power: +%d, want +150", got)
	}
	if b.PoTNow()-a.PoTNow() < 20 { // its 20, and what its power adds to the base
		t.Errorf("Tinker's PoT: %d against %d", b.PoTNow(), a.PoTNow())
	}
}

// A Master Class's hit with an area round its target (CastSpell.MercSplash:
// the Sorcerer's Furies) lands on its target and splashes only what already
// fights the merc's team; a creature beside it fighting no one is spared
// (it pulls nothing). The control: the same spell with no MercSplash is one
// a merc never casts (a harmful area spell: mercSkips).
func TestMercMasterSplash(t *testing.T) {
	r := newMercRig(t)
	fury := bookSpell(701141, "Flaming Fury", 1, 2*time.Second, time.Second, 570, "Target, Decrease Hitpoints by 1455")
	fury.Area = SpellArea{Centre: AreaOnTarget, Effects: ParseSpellText("Target, Decrease Hitpoints by 856").Effects}
	if mercUseOf(&fury) != useNone {
		t.Fatal("a harmful area spell with no MercSplash is cast")
	}
	fury.MercSplash = true
	if mercUseOf(&fury) != useNuke {
		t.Fatalf("a Master Class's splash hit is %v, want a hit", mercUseOf(&fury))
	}
	ms := r.hire(r.kit(13, 0, 0, fury), MercAssist)
	target, mate, idle := r.creature("a crab", 28, 8), r.creature("a crab", 28, 10), r.creature("a crab", 28, 11)
	for _, c := range []*NPC{target, mate, idle} {
		c.HP, c.MaxHP = 900000, 900000
	}
	r.w.mu.Lock()
	target.CurrentEnemy, mate.CurrentEnemy = r.p.ServerID, r.p.ServerID
	r.w.mercLandLocked(&mercCast{ms: ms, life: ms.life, spell: fury, target: target.ServerID}, r.at(0))
	r.w.mu.Unlock()
	if target.HP >= 900000 || mate.HP >= 900000 {
		t.Fatalf("the target lost %d, the one fighting its owner %d", 900000-target.HP, 900000-mate.HP)
	}
	if 900000-mate.HP >= 900000-target.HP {
		t.Errorf("the splash (%d) as much as the target's (%d)", 900000-mate.HP, 900000-target.HP)
	}
	if idle.HP != 900000 || idle.CurrentEnemy != 0 {
		t.Errorf("the idle crab beside it: %d lost, fighting %d", 900000-idle.HP, idle.CurrentEnemy)
	}
}

// In a fight a Sorcerer merc casts its Master Class's Fury at what its owner
// fights, and the splash takes the other creature fighting its owner.
func TestMercMasterFuryInAFight(t *testing.T) {
	r := newMercRig(t)
	fury := bookSpell(701141, "Flaming Fury", 1, 2*time.Second, time.Second, 57, "Target, Decrease Hitpoints by 1455")
	fury.Area = SpellArea{Centre: AreaOnTarget, Effects: ParseSpellText("Target, Decrease Hitpoints by 856").Effects}
	fury.MercSplash = true
	k := r.kit(13, 0, 0, fury)
	k.Master = "Sorcerer"
	ms := r.hire(k, MercAssist)
	crab, other := r.creature("Desert Crab", 28, 10), r.creature("Desert Crab", 28, 12)
	for _, c := range []*NPC{crab, other} {
		c.HP, c.MaxHP = 900000, 900000
	}
	r.p.AttackTarget, r.p.AutoAttack = crab.ServerID, true
	r.w.mu.Lock()
	r.w.addThreatLocked(crab, r.p.ServerID, 400)
	r.w.addThreatLocked(other, r.p.ServerID, 400)
	r.w.mu.Unlock()
	r.run(0, 8*time.Second)
	if _, cast := ms.ready[fury.SpellID]; !cast {
		t.Fatal("the Sorcerer never cast its Fury")
	}
	if crab.HP >= 900000 || other.HP >= 900000 {
		t.Errorf("the target lost %d, the other fighting its owner %d", 900000-crab.HP, 900000-other.HP)
	}
}

// A Tinker merc summons its Construct (the clockwork gnome's body, a creature
// of its name) as a merc's pet, and its Repair heals it when it is hurt.
func TestMercMasterTinkerConstruct(t *testing.T) {
	r := newMercRig(t)
	r.w.mu.Lock()
	r.w.putNPCLocked(&NPC{ServerID: 4900, Name: "Construct", Level: 60, HP: 4000, MaxHP: 4000, ModelID: 0xFBABEB00, Size: 1,
		NPCType: 0x80, X: 20000, Z: 20000, World: 0})
	r.w.mu.Unlock()
	construct := bookSpell(901835, "Construct", 0, 3*time.Second, 3*time.Second, 50, "Summons a Construct to aid you")
	construct.Level = 60
	repair := bookSpell(901848, "Repair", 3, time.Second, 6*time.Second, 30, "Pet, Increase Hitpoints by 250")
	repair.Level = 60
	k := r.kit(0, 3, 0, construct, repair)
	k.Master, k.MasterPets = "Tinker", []int{901835}
	ms := r.hire(k, MercAssist)
	r.run(0, 6*time.Second)
	if ms.pet == nil || ms.pet.npc.Name != "Construct" || !ms.pet.npc.isMercPet() {
		t.Fatalf("no Construct out: %+v", ms.pet)
	}
	pet := ms.pet.npc
	r.w.mu.Lock()
	pet.HP = pet.MaxHP / 3
	hurt := pet.HP
	r.w.mu.Unlock()
	r.run(6100*time.Millisecond, 10*time.Second)
	if _, cast := ms.ready[repair.SpellID]; !cast || pet.HP <= hurt {
		t.Errorf("no Repair of the hurt Construct: cast %v, %d -> %d", cast, hurt, pet.HP)
	}
}
