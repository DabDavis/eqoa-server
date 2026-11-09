package entity

import (
	"testing"
	"time"

	"github.com/eqoa/server/internal/protocol"
)

// A pet that goes alive dies where it stands (Contintious' petition 1398:
// "whenever you dismiss a pet or recast a pet, the pet should show dying it
// currently just vanishes"): its channel shows a corpse as a killed creature's
// does (byte 58 Die, 26 a corpse, health 0), which CLIENT's UpdateCreatures and
// vanilla play as its death, and its body goes once that has played
// (petDismissedLies); no one's kill, no experience. The control: a pet killed
// in a fight lies its corpse's time (petCorpseTime), told "Your pet has died.",
// as before.

// deadOnChannel is whether the owner's channel shows the pet as a corpse: the
// death's animation, the corpse bit, no health.
func deadOnChannel(r *petRig, pet *NPC) bool {
	r.w.mu.Lock()
	defer r.w.mu.Unlock()
	id := r.w.objectIDForEntityLocked(r.sess, pet.ServerID)
	buf := r.w.serializeEntityForChannel(r.sess, pet.ServerID, id)
	return id != 0 && buf != nil && buf[58] == 0x0e && buf[26] == 3 && buf[27] == 0
}

// corpses runs the corpses' system at d.
func (r *petRig) corpses(d time.Duration) {
	r.w.mu.Lock()
	defer r.w.mu.Unlock()
	r.w.corpseSystem(r.at(d))
}

// sentOp is whether a message of the opcode went to the rig's session.
func sentOp(msgs [][]byte, op protocol.GameOpcode) bool {
	for _, m := range msgs {
		if opOfMsg(m) == op {
			return true
		}
	}
	return false
}

// Dismissed, the pet dies where it stands and its body lies petDismissedLies,
// then goes; the owner is told (0x007F, the line) and gains nothing.
func TestADismissedPetDiesWhereItStands(t *testing.T) {
	r := newPetRig(t)
	pet, _ := r.summon(t, fireElemental(), 0)
	xp := r.p.TotalXP
	r.w.PetCommand(r.sess, petCmdDismiss, 0, r.at(20*time.Second))
	msgs := r.sent()
	if !pet.Dead || pet.HP != 0 || r.w.NPCs[pet.ServerID] != pet || r.w.petOfLocked(r.p) != nil {
		t.Fatalf("dismissed: dead %v, HP %d, in the world %v, still its owner's %v: want dead where it stands, its owner's no more",
			pet.Dead, pet.HP, r.w.NPCs[pet.ServerID] == pet, r.w.petOfLocked(r.p) != nil)
	}
	if !deadOnChannel(r, pet) {
		t.Error("its channel does not show it dead (byte 58 Die, 26 a corpse, health 0)")
	}
	if !sentOp(msgs, protocol.OpcodeClearPet) || !hasLine(linesOf(msgs), "Your pet has been dismissed.") {
		t.Errorf("the owner told %d messages: want 0x007F and the dismissal's line", len(msgs))
	}
	r.corpses(20*time.Second + petDismissedLies - 100*time.Millisecond)
	if r.w.NPCs[pet.ServerID] != pet {
		t.Fatalf("its body went before its death had played (%v)", petDismissedLies)
	}
	r.corpses(20*time.Second + petDismissedLies)
	if r.w.NPCs[pet.ServerID] != nil {
		t.Errorf("its body still lies %v after its dismissal", petDismissedLies)
	}
	if r.p.TotalXP != xp {
		t.Errorf("its death gave its owner %d experience", r.p.TotalXP-xp)
	}
}

// Summoned again, the old pet dies where it stands while the new one comes as
// it always did (alive, its owner's, its 0x007E), and the old one's body goes
// after its death has played, the new one staying.
func TestARecastPetsOldOneDies(t *testing.T) {
	r := newPetRig(t)
	old, _ := r.summon(t, fireElemental(), 0)
	r.p.Power = r.p.MaxPower
	pet, msgs := r.summon(t, fireElemental(), 20*time.Second)
	if pet == old || pet.Dead || r.w.petOfLocked(r.p) != pet || !sentOp(msgs, protocol.OpcodeSetPet) {
		t.Fatalf("the new pet: a new one %v, alive %v, its owner's %v, announced %v", pet != old, !pet.Dead, r.w.petOfLocked(r.p) == pet,
			sentOp(msgs, protocol.OpcodeSetPet))
	}
	if !old.Dead || old.HP != 0 || r.w.NPCs[old.ServerID] != old || !deadOnChannel(r, old) {
		t.Fatalf("the old pet: dead %v, HP %d, in the world %v, dead on the channel %v: want it dying where it stood",
			old.Dead, old.HP, r.w.NPCs[old.ServerID] == old, deadOnChannel(r, old))
	}
	landed := 20*time.Second + fireElemental().CastTime + castLand
	r.corpses(landed + petDismissedLies)
	if r.w.NPCs[old.ServerID] != nil || r.w.NPCs[pet.ServerID] != pet || pet.Dead {
		t.Errorf("after its death: the old body there %v, the new pet there %v and alive %v", r.w.NPCs[old.ServerID] != nil,
			r.w.NPCs[pet.ServerID] == pet, !pet.Dead)
	}
}

// Its owner gone from the world, the pet dies where it stands too, and goes.
func TestAPetDiesWhenItsOwnerLeaves(t *testing.T) {
	r := newPetRig(t)
	pet, _ := r.summon(t, fireElemental(), 0)
	r.w.mu.Lock()
	delete(r.w.Players, r.p.ServerID)
	r.w.petSystem(r.at(20 * time.Second))
	r.w.mu.Unlock()
	if !pet.Dead || r.w.NPCs[pet.ServerID] != pet {
		t.Fatalf("its owner gone: dead %v, in the world %v", pet.Dead, r.w.NPCs[pet.ServerID] == pet)
	}
	r.corpses(20*time.Second + petDismissedLies)
	if r.w.NPCs[pet.ServerID] != nil {
		t.Error("its body still lies after its death")
	}
}

// The control: killed in a fight, the pet lies its corpse's time, its owner
// told it died, as before.
func TestAPetKilledInAFightLiesItsTime(t *testing.T) {
	r := newPetRig(t)
	pet, _ := r.summon(t, fireElemental(), 0)
	r.w.mu.Lock()
	pet.HP = 0
	r.w.killNPC(pet)
	pet.DeathTime = r.at(20 * time.Second)
	r.w.petSystem(r.at(20 * time.Second))
	r.w.mu.Unlock()
	msgs := r.sent()
	if !hasLine(linesOf(msgs), "Your pet has died.") || r.w.petOfLocked(r.p) != nil || pet.CorpseDecay != petCorpseTime {
		t.Fatalf("killed: told it died %v, its owner's %v, its corpse's time %v", hasLine(linesOf(msgs), "Your pet has died."),
			r.w.petOfLocked(r.p) != nil, pet.CorpseDecay)
	}
	r.corpses(20*time.Second + petDismissedLies)
	if r.w.NPCs[pet.ServerID] != pet || !deadOnChannel(r, pet) {
		t.Fatalf("killed, its body went at %v (a dismissed one's time), not its corpse's %v", petDismissedLies, petCorpseTime)
	}
	r.corpses(20*time.Second + petCorpseTime)
	if r.w.NPCs[pet.ServerID] != nil {
		t.Error("killed, its body still lies past its corpse's time")
	}
}

// The attack command says why it is refused (Waldorf's petition 1399): a
// creature the pet could fight but too far from its owner is "too far away";
// one it may not fight at all (here a creature of npc flags 0x80), near or
// far, keeps "Your pet cannot attack that."; a near one is attacked.
func TestThePetsAttackRefusals(t *testing.T) {
	r := newPetRig(t)
	pet, _ := r.summon(t, fireElemental(), 0)
	ps := r.w.pets[r.p.ServerID]
	vs := GetOrCreateVisibility(r.sess)
	put := func(slot int, id EntityID, name string, flags int, dx float32) uint32 {
		n := &NPC{ServerID: id, Name: name, Level: 30, HP: 1000, MaxHP: 1000, NPCType: flags, X: r.p.X + dx, Y: r.p.Y, Z: r.p.Z,
			World: r.p.World}
		r.w.NPCs[id] = n
		r.w.SpatialIndex.Insert(id, n.X, n.Z)
		vs.ChannelEntity[slot], vs.ChannelObjectID[slot] = id, 0x01400000+uint32(id)
		return vs.ChannelObjectID[slot]
	}
	far := put(5, 7301, "Distant Crab", 0x0100, petLeash+20)
	merchant := put(6, 7302, "Town Merchant", 0x0180, 5)
	farMerchant := put(7, 7303, "Far Merchant", 0x0180, petLeash+20)
	near := put(8, 7304, "Near Crab", 0x0100, 5)
	for _, c := range []struct {
		name   string
		object uint32
		line   string
		target EntityID
	}{
		{"a crab too far off", far, petTargetTooFar, 0},
		{"the control, a merchant", merchant, petCannotAttack, 0},
		{"a merchant too far off too", farMerchant, petCannotAttack, 0},
		{"a crab near", near, "Your pet attacks Near Crab.", 7304},
	} {
		ps.target = 0
		r.w.PetCommand(r.sess, petCmdAttack, c.object, r.at(20*time.Second))
		lines := linesOf(r.sent())
		if len(lines) != 1 || lines[0] != c.line || ps.target != c.target {
			t.Errorf("%s: %q, target %d; want %q, target %d", c.name, lines, ps.target, c.line, c.target)
		}
	}
	if pet.Dead {
		t.Error("the pet died")
	}
}
