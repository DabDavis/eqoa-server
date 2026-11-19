package entity

import (
	"testing"
	"time"

	"github.com/eqoa/server/internal/session"
)

// A foe's pets are fought as the foe is (pvp.go, the owner 2026-11-18: "[a player] and I are Pvp
// flagged but pets dont get flagged so i cant attack").

const (
	aSeesPet uint32 = 0x00018830
	cSeesPet uint32 = 0x00018831
	bSeesPet uint32 = 0x00018832
)

// pvpPetOf gives B a pet beside A, seen by all three, in the spatial index.
func (d *duelists) pvpPetOf(t *testing.T, owner *Player) (*NPC, *petState) {
	t.Helper()
	pet := &NPC{ServerID: 4005, Name: "Zerk's Pet", Level: 30, HP: 4000, MaxHP: 4000, Owner: owner.ServerID, World: owner.World,
		X: 101, Y: 10, Z: 100}
	d.w.NPCs[pet.ServerID] = pet
	d.w.SpatialIndex.Insert(pet.ServerID, pet.X, pet.Z)
	if d.w.pets == nil {
		d.w.pets = make(map[EntityID]*petState)
	}
	ps := &petState{owner: owner, npc: pet, stance: petPassive, last: d.at(0)}
	d.w.pets[owner.ServerID] = ps
	for s, obj := range map[*session.Session]uint32{d.sa: aSeesPet, d.sc: cSeesPet, d.sb: bSeesPet} {
		vs := GetOrCreateVisibility(s)
		vs.ChannelEntity[5], vs.ChannelObjectID[5] = pet.ServerID, obj
	}
	return pet, ps
}

func (d *duelists) petBytes(s *session.Session, pet *NPC, obj uint32) (b142, b146 byte) {
	d.w.mu.Lock()
	defer d.w.mu.Unlock()
	b := d.w.serializeEntityForChannel(s, pet.ServerID, obj)
	return b[142], b[146]
}

// The control: with its owner unflagged (or A unflagged), a pet stays a pet to everyone: no
// engage, no harmful spell, byte 142 a pet's 3, the players' face.
func TestPvPPetOfUnflaggedOwnerUntouchable(t *testing.T) {
	d := newPvP(t)
	pet, _ := d.pvpPetOf(t, d.b)
	d.flag(t, d.sa, 0)
	if d.w.EngageAttack(d.sa, aSeesPet, pvpSwing, d.at(time.Second)) || d.a.AttackTarget != 0 {
		t.Fatal("engaged an unflagged player's pet")
	}
	if b, _ := d.petBytes(d.sa, pet, aSeesPet); b != 3 {
		t.Errorf("byte 142 %d, want a pet's 3", b)
	}
	if f := d.w.TargetFace(d.a, pet); f != FaceAlly {
		t.Errorf("face %d, want the players' %d", f, FaceAlly)
	}
	if d.w.StartCast(d.sa, strike(), aSeesPet, d.at(time.Second)) {
		t.Fatal("a strike at an unflagged player's pet was cast")
	}
	if got := duelLines(duelDrain(d.sa)); len(got) != 1 || got[0] != castFriendly {
		t.Fatalf("the refusal %q", got)
	}
	// its owner flagged, A not: still nothing for A
	d.flag(t, d.sb, 0)
	d.a.pvp.hold = d.at(0)
	if d.w.PvPCommand(d.sa, d.at(2*time.Minute)) != d.a {
		t.Fatal("A's flag did not come off")
	}
	duelDrain(d.sa)
	if d.w.EngageAttack(d.sa, aSeesPet, pvpSwing, d.at(2*time.Minute)) {
		t.Fatal("an unflagged player engaged a flagged one's pet")
	}
	if pet.HP != 4000 {
		t.Fatalf("the pet took %d", 4000-pet.HP)
	}
}

// Both flagged: A engages B's pet and its rounds land at the PvP share, both flags held, B itself
// untouched; to A the pet's byte 142 is pvpFoeByte and its face red; to B (its owner) and Gamma (no
// flag) it stays a pet. B's flag down: a pet again at once, the fight let go.
func TestPvPFoePetAttackable(t *testing.T) {
	d := newPvP(t)
	pet, _ := d.pvpPetOf(t, d.b)
	d.flag(t, d.sa, 0)
	d.flag(t, d.sb, 0)
	if b, _ := d.petBytes(d.sa, pet, aSeesPet); b != pvpFoeByte {
		t.Errorf("to the foe: byte 142 %d, want %d", b, pvpFoeByte)
	}
	if b, _ := d.petBytes(d.sb, pet, bSeesPet); b != 3 {
		t.Errorf("to its owner: byte 142 %d, want 3", b)
	}
	if b, _ := d.petBytes(d.sc, pet, cSeesPet); b != 3 {
		t.Errorf("to the unflagged third: byte 142 %d, want 3", b)
	}
	if f := d.w.TargetFace(d.a, pet); f != FaceHostile {
		t.Errorf("the foe's face %d, want red", f)
	}
	if f := d.w.TargetFace(d.c, pet); f != FaceAlly {
		t.Errorf("the third's face %d, want the players'", f)
	}
	if !d.w.EngageAttack(d.sa, aSeesPet, pvpSwing, d.at(time.Second)) || d.a.AttackTarget != pet.ServerID {
		t.Fatal("a foe's pet not engaged")
	}
	d.a.Gear.WeaponDamage, d.a.Gear.WeaponCond = 40, 1
	d.a.AutoAttack = true
	bBefore := d.b.HP
	d.attackFor(time.Second, 8*time.Second)
	if pet.HP >= 4000 || pet.Dead {
		t.Fatalf("the pet's HP %d, dead %v", pet.HP, pet.Dead)
	}
	if d.b.HP != bBefore || d.a.TotalXP != 0 {
		t.Fatalf("B's HP %d of %d, A's XP %d", d.b.HP, bBefore, d.a.TotalXP)
	}
	if d.b.pvp.hold.Before(d.at(time.Second+PvPHoldTime)) || d.b.pvp.struckBy != d.a.ServerID {
		t.Fatalf("B's hold %v, struck by %d", d.b.pvp.hold, d.b.pvp.struckBy)
	}
	// B's flag off: a pet again, the fight let go
	d.b.pvp.hold = d.at(9 * time.Second)
	if d.w.PvPCommand(d.sb, d.at(10*time.Second)) != d.b {
		t.Fatal("B's flag did not come off")
	}
	if b, _ := d.petBytes(d.sa, pet, aSeesPet); b != 3 {
		t.Errorf("flag down: byte 142 %d, want 3", b)
	}
	if f := d.w.TargetFace(d.a, pet); f != FaceAlly {
		t.Errorf("flag down: face %d", f)
	}
	d.attackFor(10*time.Second, 11*time.Second)
	if d.a.AttackTarget != 0 {
		t.Fatal("still engaged on the pet of one with no flag")
	}
}

// Grouped, the two are no foes: the pet is a pet.
func TestPvPGroupedPetNoFoe(t *testing.T) {
	d := newPvP(t)
	pet, _ := d.pvpPetOf(t, d.b)
	d.flag(t, d.sa, 0)
	d.flag(t, d.sb, 0)
	d.w.groups = map[uint32]*Group{9: {ID: 9, Members: []EntityID{d.a.ServerID, d.b.ServerID}}}
	d.a.GroupID, d.b.GroupID = 9, 9
	if d.w.EngageAttack(d.sa, aSeesPet, pvpSwing, d.at(time.Second)) {
		t.Fatal("engaged a group mate's pet")
	}
	if b, _ := d.petBytes(d.sa, pet, aSeesPet); b != 3 {
		t.Errorf("byte 142 %d, want 3", b)
	}
}

// A hit the pet cannot take kills it: a pet's death, A let go, nothing to B, no experience.
func TestPvPFoePetKilled(t *testing.T) {
	d := newPvP(t)
	pet, _ := d.pvpPetOf(t, d.b)
	d.flag(t, d.sa, 0)
	d.flag(t, d.sb, 0)
	if !d.w.EngageAttack(d.sa, aSeesPet, pvpSwing, d.at(time.Second)) {
		t.Fatal("not engaged")
	}
	pet.HP = 5
	d.w.mu.Lock()
	o := d.w.pvpFoePetLocked(d.a, pet)
	d.w.duelHurtPetLocked(d.w.pvpFightLocked(d.a, o), d.a, pet, 5000, d.at(2*time.Second), numberMelee)
	d.w.mu.Unlock()
	if !pet.Dead || pet.HP != 0 {
		t.Fatalf("the pet: HP %d, dead %v", pet.HP, pet.Dead)
	}
	if d.a.AttackTarget != 0 || d.a.TotalXP != 0 || d.b.IsDead() || d.b.respawnPending() {
		t.Fatalf("A's target %d, XP %d; B dead %v", d.a.AttackTarget, d.a.TotalXP, d.b.IsDead())
	}
	if got := duelLines(duelDrain(d.sa)); len(got) != 1 || got[0] != "You have slain Zerk's Pet." {
		t.Fatalf("told A %q", got)
	}
	d.w.mu.Lock()
	dead := d.w.pvpFoePetLocked(d.a, pet)
	d.w.mu.Unlock()
	if dead != nil {
		t.Fatal("a dead pet is still a foe's")
	}
}

// Spells: a strike at the foe's pet lands at the share; an area at a creature beside it splashes it.
func TestPvPFoePetSpells(t *testing.T) {
	d := newPvP(t)
	pet, _ := d.pvpPetOf(t, d.b)
	d.flag(t, d.sa, 0)
	d.flag(t, d.sb, 0)
	sp := strike()
	if !d.w.StartCast(d.sa, sp, aSeesPet, d.at(time.Second)) {
		t.Fatalf("a strike at a foe's pet was refused: %q", duelLines(duelDrain(d.sa)))
	}
	d.w.mu.Lock()
	d.w.castSystem(d.at(time.Second + sp.CastTime + castLand))
	d.w.mu.Unlock()
	if taken := 4000 - pet.HP; taken <= 0 || taken > 30 {
		t.Fatalf("the strike took %d of the pet, want about half of 26+", taken)
	}

	run := func(flagB bool) int {
		d := newPvP(t)
		pet, _ := d.pvpPetOf(t, d.b)
		d.flag(t, d.sa, 0)
		if flagB {
			d.flag(t, d.sb, 0)
		}
		rat := &NPC{ServerID: 2003, Name: "rat", Level: 1, HP: 5000, MaxHP: 5000, X: 101, Y: 10, Z: 101}
		d.w.NPCs[rat.ServerID] = rat
		d.w.SpatialIndex.Insert(rat.ServerID, rat.X, rat.Z)
		vs := GetOrCreateVisibility(d.sa)
		vs.ChannelEntity[6], vs.ChannelObjectID[6] = rat.ServerID, 0x00018833
		storm := pvpStorm(t)
		cast := d.at(2 * time.Minute)
		if !d.w.StartCast(d.sa, storm, 0x00018833, cast) {
			t.Fatalf("the storm was refused: %q", duelLines(duelDrain(d.sa)))
		}
		d.w.mu.Lock()
		d.w.castSystem(cast.Add(storm.CastTime + castLand))
		d.w.mu.Unlock()
		if rat.HP >= 5000 {
			t.Fatal("the storm missed the rat")
		}
		return 4000 - pet.HP
	}
	if got := run(true); got != 45 {
		t.Errorf("a foe's pet in the splash took %d, want 90 halved", got)
	}
	if got := run(false); got != 0 {
		t.Errorf("an unflagged owner's pet in the splash took %d", got)
	}
}

// A charmed pet (npc flags 0x80) and a merc: the foe's view clears the flag ToggleAutoAttack refuses,
// and a merc is engaged as a pet is.
func TestPvPFoeCharmAndMerc(t *testing.T) {
	d := newPvP(t)
	pet, ps := d.pvpPetOf(t, d.b)
	pet.NPCType = charmNPCType
	pet.Charm = &charmHold{npcType: 0}
	d.flag(t, d.sa, 0)
	if _, b := d.petBytes(d.sa, pet, aSeesPet); b&0x80 == 0 {
		t.Errorf("not a foe yet: byte 146 %#x, want the charm's 0x80", b)
	}
	d.flag(t, d.sb, 0)
	if b142, b146 := d.petBytes(d.sa, pet, aSeesPet); b142 != pvpFoeByte || b146&0x80 != 0 {
		t.Errorf("to the foe: bytes 142 %d, 146 %#x", b142, b146)
	}
	if _, b := d.petBytes(d.sc, pet, cSeesPet); b&0x80 == 0 {
		t.Errorf("to the third: byte 146 %#x, want the charm's 0x80", b)
	}
	if !d.w.EngageAttack(d.sa, aSeesPet, pvpSwing, d.at(time.Second)) {
		t.Fatal("a foe's charmed pet not engaged")
	}
	// the same NPC as a merc in place of a pet
	delete(d.w.pets, d.b.ServerID)
	_ = ps
	pet.Charm, pet.NPCType = nil, 0
	pet.merc = &mercState{owner: d.b, npc: pet}
	d.a.disengage()
	if !d.w.EngageAttack(d.sa, aSeesPet, pvpSwing, d.at(2*time.Second)) || d.a.AttackTarget != pet.ServerID {
		t.Fatal("a foe's merc not engaged")
	}
	// no pet state and no merc: nothing a player fights with
	pet.merc = nil
	d.a.disengage()
	if d.w.EngageAttack(d.sa, aSeesPet, pvpSwing, d.at(3*time.Second)) {
		t.Fatal("engaged an owned NPC that is nobody's pet")
	}
}

// A's pet, told to, goes for B's pet; and a flag's change names the pet's targeters.
func TestPvPPetOnFoePetAndTargeters(t *testing.T) {
	d := newPvP(t)
	petB, _ := d.pvpPetOf(t, d.b)
	d.flag(t, d.sa, 0)
	d.flag(t, d.sb, 0)
	petA := &NPC{ServerID: 4006, Name: "Alpha's Pet", Level: 30, HP: 400, MaxHP: 400, Owner: d.a.ServerID, X: 101, Y: 10, Z: 101}
	d.w.NPCs[petA.ServerID] = petA
	psA := &petState{owner: d.a, npc: petA, stance: petPassive, last: d.at(0)}
	d.w.pets[d.a.ServerID] = psA
	d.w.mu.Lock()
	if !d.w.pvpPetCommandLocked(psA, petB) {
		d.w.mu.Unlock()
		t.Fatal("told to go for a foe's pet, it did not")
	}
	for x := time.Second; x < 8*time.Second; x += 100 * time.Millisecond {
		d.w.pvpPetLocked(psA, 0.1, d.at(x))
	}
	d.w.mu.Unlock()
	if petB.HP >= 4000 {
		t.Fatal("A's pet did not hurt B's")
	}
	d.a.TargetObjectID, d.a.targetToken = aSeesPet, 7
	got := d.w.PetTargetersOf(d.b)
	if len(got) != 1 || got[0].Viewer != d.a || got[0].Pet != petB || got[0].Token != 7 {
		t.Fatalf("B's pets' targeters %+v", got)
	}
}

// A merc's pet is no duel's: a duelist may not fight a stranger's (duelFoeLocked read `|| isMercPet`
// as any merc pet anywhere).
func TestDuelMercPetOfStrangerNoFoe(t *testing.T) {
	d := newDuelists(t)
	d.start(t)
	mp := &NPC{ServerID: 4007, Name: "stray", Owner: d.c.ServerID, mercPet: &mercState{owner: d.c}}
	d.w.NPCs[mp.ServerID] = mp
	d.w.mu.Lock()
	defer d.w.mu.Unlock()
	if d.w.duelFoeLocked(d.a, mp.ServerID) {
		t.Fatal("a stranger's merc's pet is a duel's foe")
	}
}
