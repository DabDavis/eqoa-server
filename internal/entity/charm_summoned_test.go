package entity

import (
	"encoding/binary"
	"testing"
	"time"

	"github.com/eqoa/server/internal/protocol"
)

// Petition 1348 (Battaryhog, an enchanter): "when tring to charm a pet with
// binding grasp, if i alrdy got swarming blades out tanking it to charm, i get
// red message saying already got a pet". With a summoned pet out the charm is
// cast while the pet holds the creature; when it takes hold the summoned pet
// goes first (0x007F, its non-collide entry off, its line), then the creature
// is the pet (0x007E with its own id, its name on the list), so the bar takes
// the new name and there is never a spell pet and a charmed one at once
// (petition 1351, Jibrish: "Dont make it so they can summon their spell pet
// and charm at the same time"). Resisted, the summoned pet stays, still the
// creature's enemy. The controls: with no pet out no 0x007F goes; a charmed
// pet out still refuses the charm (TestACharmIsNotCastWithAPetOut).

// heldBy has the rig's summoned pet take on the crab and fight it a few
// seconds, so the crab's enemy is the pet.
func heldBy(t *testing.T, r *petRig, pet *NPC, from time.Duration) {
	t.Helper()
	r.w.PetCommand(r.sess, petCmdAttack, crabID, r.at(from))
	for d := from; d <= from+6*time.Second; d += 100 * time.Millisecond {
		r.tick(d)
	}
	r.sent()
	if r.enemy.CurrentEnemy != pet.ServerID || r.enemy.Threats[pet.ServerID] == 0 {
		t.Fatalf("the crab is not held by the pet: enemy %d, threats %v", r.enemy.CurrentEnemy, r.enemy.Threats)
	}
}

// opIndex is the index of the first message of the opcode in msgs, -1 none.
func opIndex(msgs [][]byte, op protocol.GameOpcode) int {
	for i, m := range msgs {
		if opOfMsg(m) == op {
			return i
		}
	}
	return -1
}

func TestACharmTakesThePlaceOfASummonedPet(t *testing.T) {
	defer func(f func(int) int) { charmIntn = f }(charmIntn)
	charmIntn = dice(99)
	r, _, _ := charmRig(t)
	crab := r.enemy
	pet, _ := r.summon(t, fireElemental(), 0)
	heldBy(t, r, pet, 10*time.Second)
	hp := crab.HP // what the pet left it: the charm takes it as it is

	began, msgs := castCharm(t, r, captivation(), crabID, 20*time.Second)
	if !began || !castsAt(msgs) {
		t.Fatalf("the charm with a summoned pet out: began %v, sent %x", began, msgs)
	}
	if got := r.w.petOfLocked(r.p); got != crab || crab.Owner != r.p.ServerID || crab.Charm == nil {
		t.Fatalf("the crab is not the pet: pet %v, owner %d", got, crab.Owner)
	}
	if _, ok := r.w.NPCs[pet.ServerID]; ok || len(r.w.pets) != 1 {
		t.Errorf("the summoned pet is still in the world (%v), pets %d", ok, len(r.w.pets))
	}
	clear, set := opIndex(msgs, protocol.OpcodeClearPet), opIndex(msgs, protocol.OpcodeSetPet)
	off, on := opIndex(msgs, protocol.OpcodeNoCollideRemove), opIndex(msgs, protocol.OpcodeNoCollideAdd)
	if clear < 0 || set < 0 || clear > set || off < 0 || on < 0 || off > on {
		t.Errorf("0x007F at %d, 0x007E at %d, 0x1004 at %d, 0x1003 at %d: the old pet's go first", clear, set, off, on)
	}
	if set >= 0 && binary.LittleEndian.Uint32(msgs[set][2:]) != crabID {
		t.Errorf("0x007E names %#x, want the crab's %#x", binary.LittleEndian.Uint32(msgs[set][2:]), crabID)
	}
	said := false
	for _, m := range msgs {
		said = said || chatText(m) == "Fire Elemental"+charmPetLeaves
	}
	if !said {
		t.Errorf("no %q line in %x", "Fire Elemental"+charmPetLeaves, msgs)
	}
	if crab.HP != hp || hp == crab.MaxHP || len(crab.Threats) != 0 {
		t.Errorf("the charmed crab: HP %d (held at %d of %d), threats %v", crab.HP, hp, crab.MaxHP, crab.Threats)
	}

	// resisted: the summoned pet stays, the crab its enemy still
	charmIntn = dice(0)
	r2, _, _ := charmRig(t)
	pet2, _ := r2.summon(t, fireElemental(), 0)
	heldBy(t, r2, pet2, 10*time.Second)
	began2, msgs2 := castCharm(t, r2, captivation(), crabID, 20*time.Second)
	if !began2 || opIndex(msgs2, protocol.OpcodeClearPet) >= 0 || r2.w.petOfLocked(r2.p) != pet2 || r2.enemy.Owner != 0 {
		t.Errorf("resisted: began %v, 0x007F at %d, pet %v, the crab's owner %d", began2,
			opIndex(msgs2, protocol.OpcodeClearPet), r2.w.petOfLocked(r2.p), r2.enemy.Owner)
	}
	if r2.enemy.CurrentEnemy != pet2.ServerID {
		t.Errorf("resisted: the crab turned on %d, want the pet %d still", r2.enemy.CurrentEnemy, pet2.ServerID)
	}

	// the control: no pet out, nothing to clear
	charmIntn = dice(99)
	r3, _, _ := charmRig(t)
	if began, msgs := castCharm(t, r3, captivation(), crabID, 0); !began || opIndex(msgs, protocol.OpcodeClearPet) >= 0 ||
		opIndex(msgs, protocol.OpcodeSetPet) < 0 {
		t.Errorf("no pet out: began %v, 0x007F at %d, 0x007E at %d", began, opIndex(msgs, protocol.OpcodeClearPet), opIndex(msgs, protocol.OpcodeSetPet))
	}
}

// A summoned pet that died while the charm was cast lies its corpse's time and
// its owner is not told it "departs"; the charm still takes the creature, the
// dead pet's 0x007F before the creature's 0x007E. The control: a live one is
// told (above).
func TestACharmAfterTheSummonedPetDied(t *testing.T) {
	defer func(f func(int) int) { charmIntn = f }(charmIntn)
	charmIntn = dice(99)
	r, _, _ := charmRig(t)
	pet, _ := r.summon(t, fireElemental(), 0)
	r.sent()
	if !r.w.StartCast(r.sess, captivation(), crabID, r.at(10*time.Second)) {
		t.Fatal("the charm did not begin")
	}
	r.w.mu.Lock()
	r.w.killNPC(pet) // dies in the cast, before petSystem sees it
	r.w.castSystem(r.at(10*time.Second + captivation().CastTime + castLand))
	r.w.mu.Unlock()
	msgs := r.sent()
	if r.w.petOfLocked(r.p) != r.enemy || len(r.w.pets) != 1 {
		t.Fatalf("the crab is not the pet: %v", r.w.petOfLocked(r.p))
	}
	for _, m := range msgs {
		if chatText(m) == "Fire Elemental"+charmPetLeaves {
			t.Errorf("a dead pet was said to depart: %x", msgs)
		}
	}
	if clear, set := opIndex(msgs, protocol.OpcodeClearPet), opIndex(msgs, protocol.OpcodeSetPet); clear < 0 || set < clear {
		t.Errorf("0x007F at %d, 0x007E at %d", clear, set)
	}
	if _, ok := r.w.NPCs[pet.ServerID]; !ok {
		t.Error("the dead pet's corpse was taken away at once")
	}
}
