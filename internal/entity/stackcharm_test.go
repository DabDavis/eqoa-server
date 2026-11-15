package entity

import (
	"encoding/binary"
	"testing"
	"time"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/protocol"
)

// The Kerran's racial charm stacks (stackcharm.go, the owner 2026-11-15:
// "Looks like having more than 1 pet isnt working", "the kerran racial should
// stack only"): Cat Magnetism's animal comes beside whatever pet is out,
// summoned or charmed, never refused for it and never replacing it; a second
// Cat Magnetism lets only the first animal go; other charms keep their one-pet
// rules (charm_test.go, charm_summoned_test.go are the controls).

const (
	catMagnetismID = 760303
	// the second animal's object id on the rig's third channel
	jackalID = 0x01312E99
)

// catMagnetism is the Kerran's racial charm as spelleffect has it (its line:
// "Target Animal, Charm up to level 50, 45min").
func catMagnetism() CastSpell {
	s := CastSpell{SpellID: catMagnetismID, DisplayIndex: 9, Level: 60, Scope: 1, Name: "Cat Magnetism", CastTime: 5 * time.Second,
		Recast: time.Second, PowerCost: 400, Range: 30,
		Def: ParseSpell("Target, Charm up to level 50, 45min", "No beast of the wild can resist a kerran's charm.")}
	s.Def.TargetKind, s.Def.CharmMax, s.Def.Duration = "animal", 50, 45*time.Minute
	return s
}

// kerranRig is charmRig's enchanter made a Kerran with Cat Magnetism's rank
// in the catalog (racial: races 4096 alone), the crab and a jackal for its
// animals, the Rotting Flesh for an enchanter's charm, both made sturdy.
func kerranRig(t *testing.T) (*petRig, *NPC, *NPC) {
	t.Helper()
	r, flesh, _ := charmRig(t)
	r.p.Race = 12
	r.w.SetCMCatalog([]database.CMRankRow{{ID: 1972, Category: 4, Classes: 32767, Races: 4096, Name: "Cat Magnetism",
		AddSpells: "760303", Adds: "Cat Magnetism@60", Enabled: true}})
	jackal := &NPC{ServerID: 3400, Name: "jackal", Level: 20, HP: 900, MaxHP: 900, ModelID: 0x55432169, Size: 1,
		NPCType: 0x0100, X: 8638, Y: 75, Z: 5418, World: 2, SpawnX: 8638, SpawnY: 75, SpawnZ: 5418}
	r.w.NPCs[jackal.ServerID] = jackal
	r.w.SpatialIndex.Insert(jackal.ServerID, jackal.X, jackal.Z)
	vs := GetOrCreateVisibility(r.sess)
	vs.ChannelEntity[2], vs.ChannelObjectID[2] = jackal.ServerID, jackalID
	flesh.HP, flesh.MaxHP = 50000, 50000
	r.p.Power, r.p.MaxPower = 50000, 50000
	r.w.mu.Lock()
	if !r.w.stackingCharmLocked(&CastSpell{SpellID: catMagnetismID, Def: catMagnetism().Def}) {
		r.w.mu.Unlock()
		t.Fatal("Cat Magnetism is not a stacking charm")
	}
	r.w.mu.Unlock()
	return r, flesh, jackal
}

// noCollideAdds is the ids of the 0x1003s in msgs.
func noCollideAdds(msgs [][]byte) []uint32 {
	var out []uint32
	for _, m := range msgs {
		if opOfMsg(m) == protocol.OpcodeNoCollideAdd && len(m) >= 6 {
			out = append(out, binary.LittleEndian.Uint32(m[len(m)-4:]))
		}
	}
	return out
}

func TestCatMagnetismStacksWithASummonedPet(t *testing.T) {
	defer func(f func(int) int) { charmIntn = f }(charmIntn)
	charmIntn = dice(99)
	r, flesh, _ := kerranRig(t)
	crab := r.enemy
	pet, _ := r.summon(t, fireElemental(), 0)

	began, msgs := castCharm(t, r, catMagnetism(), crabID, 10*time.Second)
	if !began {
		t.Fatalf("Cat Magnetism with a summoned pet out was refused: %x", msgs)
	}
	if r.w.petOfLocked(r.p) != pet || pet.Dead {
		t.Fatalf("the summoned pet is not the pet still (dead %v)", pet.Dead)
	}
	side := r.w.sidePets[r.p.ServerID]
	if side == nil || side.npc != crab || crab.Owner != r.p.ServerID || crab.Charm == nil || !side.stacks || !side.side {
		t.Fatalf("the crab is not beside the pet: %+v, owner %d", side, crab.Owner)
	}
	if opIndex(msgs, protocol.OpcodeClearPet) >= 0 || opIndex(msgs, protocol.OpcodeSetPet) >= 0 {
		t.Errorf("the pet bar was touched: 0x007F at %d, 0x007E at %d", opIndex(msgs, protocol.OpcodeClearPet), opIndex(msgs, protocol.OpcodeSetPet))
	}
	if ids := noCollideAdds(msgs); len(ids) != 1 || ids[0] != sidePetNoCollideID {
		t.Errorf("non-collide entries %x, want the animal's %#x", ids, sidePetNoCollideID)
	}

	// both fight: the pet told to attack the flesh, the crab joins it
	r.w.PetCommand(r.sess, petCmdAttack, 0x01312E97, r.at(20*time.Second))
	for d := 20 * time.Second; d <= 30*time.Second; d += 100 * time.Millisecond {
		r.tick(d)
	}
	if flesh.Threats[pet.ServerID] == 0 || flesh.Threats[crab.ServerID] == 0 {
		t.Errorf("the flesh's threats %v: want the pet %d's and the crab %d's", flesh.Threats, pet.ServerID, crab.ServerID)
	}
	if r.w.sidePets[r.p.ServerID] != side || r.w.petOfLocked(r.p) != pet {
		t.Error("a pet went in the fight")
	}
}

func TestCatMagnetismStacksWithACharmedPet(t *testing.T) {
	defer func(f func(int) int) { charmIntn = f }(charmIntn)
	charmIntn = dice(99)
	r, flesh, _ := kerranRig(t)
	if began, _ := castCharm(t, r, captivation(), 0x01312E97, 0); !began || r.w.petOfLocked(r.p) != flesh {
		t.Fatal("the enchanter's charm did not take the flesh")
	}
	began, msgs := castCharm(t, r, catMagnetism(), crabID, 10*time.Second)
	if !began {
		t.Fatalf("Cat Magnetism with a charmed pet out was refused: %x", msgs)
	}
	if r.w.petOfLocked(r.p) != flesh || flesh.Charm == nil || r.w.sidePets[r.p.ServerID] == nil || r.w.sidePets[r.p.ServerID].npc != r.enemy {
		t.Fatalf("want the flesh the pet and the crab beside it: pet %v, side %+v", r.w.petOfLocked(r.p), r.w.sidePets[r.p.ServerID])
	}
	// the control: the enchanter's charm again is refused (one charmed pet), the crab beside it no matter
	if began, msgs := castCharm(t, r, captivation(), jackalID, 20*time.Second); began || len(msgs) != 1 {
		t.Errorf("a second enchanter's charm: began %v, %x", began, msgs)
	}
}

func TestASecondCatMagnetismReplacesOnlyTheFirstAnimal(t *testing.T) {
	defer func(f func(int) int) { charmIntn = f }(charmIntn)
	charmIntn = dice(99)
	r, _, jackal := kerranRig(t)
	crab := r.enemy
	pet, _ := r.summon(t, fireElemental(), 0)
	castCharm(t, r, catMagnetism(), crabID, 10*time.Second)
	began, msgs := castCharm(t, r, catMagnetism(), jackalID, 20*time.Second)
	if !began {
		t.Fatalf("the second Cat Magnetism was refused: %x", msgs)
	}
	if r.w.petOfLocked(r.p) != pet || pet.Dead {
		t.Error("the summoned pet went")
	}
	if side := r.w.sidePets[r.p.ServerID]; side == nil || side.npc != jackal || jackal.Owner != r.p.ServerID {
		t.Errorf("the jackal is not beside the pet: %+v", side)
	}
	if crab.Owner != 0 || crab.Charm != nil || crab.Threats[r.p.ServerID] != 0 {
		t.Errorf("the crab is not let go quietly: owner %d, charm %v, threats %v", crab.Owner, crab.Charm, crab.Threats)
	}
	said := false
	for _, m := range msgs {
		said = said || chatText(m) == crab.Name+stackFreed
	}
	if !said {
		t.Errorf("no %q line", crab.Name+stackFreed)
	}

	// alone, the animal is the pet; a second Cat Magnetism replaces it there
	r2, _, jackal2 := kerranRig(t)
	castCharm(t, r2, catMagnetism(), crabID, 0)
	if ps := r2.w.petStateLocked(r2.p); ps == nil || ps.npc != r2.enemy || !ps.stacks || r2.w.sidePets[r2.p.ServerID] != nil {
		t.Fatalf("alone, the crab is not the pet: %+v", ps)
	}
	castCharm(t, r2, catMagnetism(), jackalID, 10*time.Second)
	if r2.w.petOfLocked(r2.p) != jackal2 || r2.enemy.Owner != 0 || len(r2.w.sidePets) != 0 {
		t.Errorf("the jackal is not the pet in the crab's place: pet %v, crab's owner %d", r2.w.petOfLocked(r2.p), r2.enemy.Owner)
	}
}

// The animal alone is the pet (the bar, dismiss); a pet coming (a summoning,
// an enchanter's charm) puts it beside, and that pet going makes it the pet
// again.
func TestTheAnimalStepsAsideAndBack(t *testing.T) {
	defer func(f func(int) int) { charmIntn = f }(charmIntn)
	charmIntn = dice(99)
	r, flesh, _ := kerranRig(t)
	crab := r.enemy
	castCharm(t, r, catMagnetism(), crabID, 0)
	r.tick(time.Second)
	r.sent()
	pet, msgs := r.summon(t, fireElemental(), 2*time.Second)
	if r.w.petOfLocked(r.p) != pet || r.w.sidePets[r.p.ServerID] == nil || r.w.sidePets[r.p.ServerID].npc != crab || crab.Charm == nil {
		t.Fatalf("the summoning: pet %v, side %+v", r.w.petOfLocked(r.p), r.w.sidePets[r.p.ServerID])
	}
	_ = msgs
	r.w.PetCommand(r.sess, petCmdDismiss, 0, r.at(20*time.Second))
	r.tick(21 * time.Second)
	if r.w.petOfLocked(r.p) != crab || len(r.w.sidePets) != 0 {
		t.Fatalf("the summoned pet dismissed: pet %v, side %d", r.w.petOfLocked(r.p), len(r.w.sidePets))
	}
	r.tick(22 * time.Second)
	// an enchanter's charm with the animal the pet: not refused, the animal aside
	if began, msgs := castCharm(t, r, captivation(), 0x01312E97, 30*time.Second); !began {
		t.Fatalf("an enchanter's charm with the animal the pet was refused: %x", msgs)
	}
	if r.w.petOfLocked(r.p) != flesh || r.w.sidePets[r.p.ServerID] == nil || r.w.sidePets[r.p.ServerID].npc != crab {
		t.Errorf("the enchanter's charm: pet %v, side %+v", r.w.petOfLocked(r.p), r.w.sidePets[r.p.ServerID])
	}
}

// Not a Kerran's charm: one pet still, as before (the controls of
// charm_test.go): a summoned pet goes as it takes hold.
func TestOtherCharmsKeepOnePet(t *testing.T) {
	defer func(f func(int) int) { charmIntn = f }(charmIntn)
	charmIntn = dice(99)
	r, flesh, _ := kerranRig(t)
	pet, _ := r.summon(t, fireElemental(), 0)
	castCharm(t, r, captivation(), 0x01312E97, 10*time.Second)
	if r.w.petOfLocked(r.p) != flesh || !pet.Dead || len(r.w.sidePets) != 0 {
		t.Errorf("an enchanter's charm over a summoned pet: pet %v, summoned dead %v, side %d", r.w.petOfLocked(r.p), pet.Dead, len(r.w.sidePets))
	}
	// a racial charm only stacks: the same line, no racial rank adds it
	r.w.SetCMCatalog(nil)
	if began, _ := castCharm(t, r, catMagnetism(), crabID, 20*time.Second); began {
		t.Error("a charm no racial rank adds was cast with a charmed pet out")
	}
}

// The animal's charm ends as a charm does: its time (turned on its master),
// its master's logout (quietly); the pet beside it stays.
func TestTheAnimalsCharmEnds(t *testing.T) {
	defer func(f func(int) int) { charmIntn = f }(charmIntn)
	charmIntn = dice(99)
	r, _, _ := kerranRig(t)
	crab := r.enemy
	pet, _ := r.summon(t, fireElemental(), 0)
	castCharm(t, r, catMagnetism(), crabID, 10*time.Second)
	until := crab.Charm.until
	if want := 45 * time.Minute; until.Sub(r.at(10*time.Second+5*time.Second+castLand)) != want {
		t.Errorf("the charm holds %v, want %v", until.Sub(r.at(10*time.Second+5*time.Second+castLand)), want)
	}
	r.p.X, r.p.Z = crab.X, crab.Z // the crab beside its master: turned on it
	r.tick(until.Sub(r.t0) + time.Second)
	msgs := r.sent()
	if crab.Owner != 0 || crab.Charm != nil || len(r.w.sidePets) != 0 || crab.Threats[r.p.ServerID] == 0 {
		t.Errorf("worn off: owner %d, charm %v, side %d, threats %v", crab.Owner, crab.Charm, len(r.w.sidePets), crab.Threats)
	}
	if r.w.petOfLocked(r.p) != pet || opIndex(msgs, protocol.OpcodeClearPet) >= 0 {
		t.Errorf("the summoned pet: %v, 0x007F at %d", r.w.petOfLocked(r.p), opIndex(msgs, protocol.OpcodeClearPet))
	}

	r2, _, _ := kerranRig(t)
	r2.summon(t, fireElemental(), 0)
	castCharm(t, r2, catMagnetism(), crabID, 10*time.Second)
	r2.w.mu.Lock()
	delete(r2.w.Players, r2.p.ServerID)
	r2.w.mu.Unlock()
	r2.tick(20 * time.Second)
	if c := r2.enemy; c.Owner != 0 || c.Charm != nil || len(r2.w.sidePets) != 0 || len(c.Threats) != 0 {
		t.Errorf("logged out: owner %d, charm %v, side %d, threats %v", c.Owner, c.Charm, len(r2.w.sidePets), c.Threats)
	}
}

// A restart keeps the animal beside the pet (keeppet.go), and alone as the pet.
func TestARestartKeepsTheAnimal(t *testing.T) {
	defer func(f func(int) int) { charmIntn = f }(charmIntn)
	charmIntn = dice(99)
	r, _, _ := kerranRig(t)
	r.summon(t, fireElemental(), 0)
	castCharm(t, r, catMagnetism(), crabID, 10*time.Second)
	now := r.at(20 * time.Second)
	r.w.mu.Lock()
	main, side := r.w.keptPetLocked(r.p, now), r.w.keptSidePetLocked(r.p, now)
	r.w.mu.Unlock()
	if main == nil || main.Stacks || side == nil || !side.Stacks || side.NPC != uint32(r.enemy.ServerID) {
		t.Fatalf("kept: %+v, %+v", main, side)
	}

	// the new process: its pet back first, then the animal beside it
	r2, _, _ := kerranRig(t)
	r2.summon(t, fireElemental(), 0)
	if !r2.w.RestorePet(r2.sess, *side, nil, r2.at(time.Second)) {
		t.Fatal("the animal was not given back beside the pet")
	}
	if ps := r2.w.sidePets[r2.p.ServerID]; ps == nil || ps.npc != r2.enemy || r2.enemy.Charm == nil || r2.w.petOfLocked(r2.p) == r2.enemy {
		t.Errorf("restored: side %+v, pet %v", ps, r2.w.petOfLocked(r2.p))
	}
	// alone it comes back the pet, still a stacking charm's
	r3, _, _ := kerranRig(t)
	if !r3.w.RestorePet(r3.sess, *side, nil, r3.at(time.Second)) {
		t.Fatal("the animal was not given back alone")
	}
	if ps := r3.w.petStateLocked(r3.p); ps == nil || ps.npc != r3.enemy || !ps.stacks {
		t.Errorf("restored alone: %+v", ps)
	}
	// the control: a charm that does not stack is not given back beside a pet
	r4, _, _ := kerranRig(t)
	r4.summon(t, fireElemental(), 0)
	k := *side
	k.Stacks = false
	if r4.w.RestorePet(r4.sess, k, nil, r4.at(time.Second)) {
		t.Error("a kept charm that does not stack came back beside a pet")
	}
}
