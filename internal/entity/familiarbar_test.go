package entity

import (
	"encoding/binary"
	"fmt"
	"testing"
	"time"

	"github.com/eqoa/server/internal/protocol"
)

// A familiar on its owner's pet bar (familiar.go; jibrish, 2026-11-21: "add a
// way to dismiss healbot on heirloom, bind it to dismiss pet"): CLIENT sends a
// pet command only with m_petId set, so Healbot is told as the pet (0x007E)
// while no pet is out, and Dismiss Pet (0x0041 5) sends him away; a pet out
// owns the bar, and Healbot takes it again once the pet goes.

// barFamiliar calls Healbot on the pet rig's magician at at, landed.
func (r *petRig) barFamiliar(t *testing.T, at time.Duration) *familiarState {
	t.Helper()
	d := healbot()
	if !r.w.StartCast(r.sess, FamiliarCast(d), 0xFFFFFFFF, r.at(at)) {
		t.Fatal("the familiar's cast did not begin")
	}
	r.w.mu.Lock()
	r.w.castSystem(r.at(at + d.Cast + castLand))
	r.w.mu.Unlock()
	fs := r.w.familiars[r.p.ServerID]
	if fs == nil {
		t.Fatal("no familiar after its cast landed")
	}
	return fs
}

// barTick runs the pets' and the familiars' systems, in the tick's order.
func (r *petRig) barTick(d time.Duration) {
	r.w.mu.Lock()
	defer r.w.mu.Unlock()
	r.w.TickCount++
	r.w.petSystem(r.at(d))
	r.w.familiarSystem(r.at(d))
}

// setPetIDs is the id each 0x007E in msgs names.
func setPetIDs(msgs [][]byte) []uint32 {
	var out []uint32
	for _, m := range msgs {
		if opOfMsg(m) == protocol.OpcodeSetPet && len(m) >= 6 {
			out = append(out, binary.LittleEndian.Uint32(m[2:]))
		}
	}
	return out
}

func TestHealbotIsOnThePetBarAndDismissPetSendsHimAway(t *testing.T) {
	r := newPetRig(t)
	r.sent()
	fs := r.barFamiliar(t, 0)
	healbotID := r.w.objectIDForEntityLocked(r.sess, fs.npc.ServerID)
	msgs := r.sent()
	if ids := setPetIDs(msgs); healbotID == 0 || len(ids) != 1 || ids[0] != healbotID || fs.told != healbotID {
		t.Fatalf("Healbot with no pet out: 0x007E %v, want his id %#x once (told %#x)", ids, healbotID, fs.told)
	}
	r.barTick(3 * time.Second)
	if ids := setPetIDs(r.sent()); len(ids) != 0 {
		t.Errorf("a tick told the bar again %v", ids)
	}

	// any command but Dismiss Pet: a line, Healbot stays, the bar kept
	for _, cmd := range []byte{byte(petPassive), byte(petAggressive), petCmdBackOff, petCmdAttack} {
		r.w.PetCommand(r.sess, cmd, r.me, r.at(4*time.Second))
		msgs := r.sent()
		if lines := linesOf(msgs); len(lines) != 1 || lines[0] != fmt.Sprintf(familiarNoFight, "Healbot") ||
			opIndex(msgs, protocol.OpcodeClearPet) >= 0 || r.w.familiars[r.p.ServerID] != fs {
			t.Errorf("pet command %d to Healbot: lines %q, 0x007F at %d, Healbot out %v", cmd, lines,
				opIndex(msgs, protocol.OpcodeClearPet), r.w.familiars[r.p.ServerID] == fs)
		}
	}
	// 0 and 6 are no command
	r.w.PetCommand(r.sess, 0, 0, r.at(4*time.Second))
	r.w.PetCommand(r.sess, 6, 0, r.at(4*time.Second))
	if msgs := r.sent(); len(msgs) != 0 || r.w.familiars[r.p.ServerID] != fs {
		t.Errorf("no command answered %x, or Healbot went", msgs)
	}

	// a summoned pet takes the bar: Healbot stays out, beside it
	pet, msgs := r.summon(t, fireElemental(), 5*time.Second)
	petID := r.w.objectIDForEntityLocked(r.sess, pet.ServerID)
	if ids := setPetIDs(msgs); len(ids) != 1 || ids[0] != petID || opIndex(msgs, protocol.OpcodeClearPet) >= 0 {
		t.Fatalf("the summoning with Healbot on the bar: 0x007E %v (the pet's %#x), 0x007F at %d", ids, petID,
			opIndex(msgs, protocol.OpcodeClearPet))
	}
	r.barTick(14 * time.Second)
	if ids := setPetIDs(r.sent()); len(ids) != 0 || fs.told != 0 || r.w.familiars[r.p.ServerID] != fs {
		t.Fatalf("with a pet out Healbot told %v (told %#x), out %v", ids, fs.told, r.w.familiars[r.p.ServerID] == fs)
	}

	// Dismiss Pet with the pet out: the pet goes, Healbot stays, and keeps
	// off the bar familiarBarGrace (a client's auto-cast summons the pet again
	// meanwhile: petition 2791), then takes it
	r.w.PetCommand(r.sess, petCmdDismiss, 0, r.at(15*time.Second))
	msgs = r.sent()
	if opIndex(msgs, protocol.OpcodeClearPet) < 0 || r.w.petOfLocked(r.p) != nil || r.w.familiars[r.p.ServerID] != fs {
		t.Fatalf("Dismiss Pet with a pet out: 0x007F at %d, pet %v, Healbot out %v", opIndex(msgs, protocol.OpcodeClearPet),
			r.w.petOfLocked(r.p), r.w.familiars[r.p.ServerID] == fs)
	}
	r.barTick(15100 * time.Millisecond)
	if ids := setPetIDs(r.sent()); len(ids) != 0 || fs.told != 0 {
		t.Fatalf("the pet just gone: 0x007E %v (told %#x), want none for %v", ids, fs.told, familiarBarGrace)
	}
	r.barTick(15*time.Second + familiarBarGrace - 100*time.Millisecond)
	if ids := setPetIDs(r.sent()); len(ids) != 0 {
		t.Fatalf("within the grace: 0x007E %v", ids)
	}
	r.barTick(15*time.Second + familiarBarGrace + 200*time.Millisecond)
	healbotID = r.w.objectIDForEntityLocked(r.sess, fs.npc.ServerID)
	if ids := setPetIDs(r.sent()); len(ids) != 1 || ids[0] != healbotID || fs.told != healbotID {
		t.Fatalf("the pet gone: 0x007E %v, want Healbot's %#x", ids, healbotID)
	}

	// Dismiss Pet with Healbot on the bar: he goes, off the bar
	r.w.PetCommand(r.sess, petCmdDismiss, 0, r.at(16*time.Second+familiarBarGrace))
	msgs = r.sent()
	if lines := linesOf(msgs); len(lines) != 1 || lines[0] != fmt.Sprintf(familiarDismissed, "Healbot") ||
		opIndex(msgs, protocol.OpcodeClearPet) < 0 || opIndex(msgs, protocol.OpcodeNoCollideRemove) < 0 {
		t.Errorf("Dismiss Pet to Healbot: lines %q, 0x007F at %d, 0x1004 at %d", lines, opIndex(msgs, protocol.OpcodeClearPet),
			opIndex(msgs, protocol.OpcodeNoCollideRemove))
	}
	if r.w.familiars[r.p.ServerID] != nil || r.w.NPCs[fs.npc.ServerID] != nil {
		t.Fatal("Healbot stayed after Dismiss Pet")
	}
	// the control: nothing out, a command is CLIENT's own refusal
	r.w.PetCommand(r.sess, petCmdDismiss, 0, r.at(17*time.Second+familiarBarGrace))
	if lines := linesOf(r.sent()); len(lines) != 1 || lines[0] != castNoPet {
		t.Errorf("Dismiss Pet with nothing out: %q, want %q", lines, castNoPet)
	}
}

// /familiar dismiss with a pet out leaves the pet's bar alone (no 0x007F); a
// MemoryDump, which clears CLIENT's m_petId, has Healbot told again.
func TestHealbotLeavesAPetsBarAlone(t *testing.T) {
	r := newPetRig(t)
	fs := r.barFamiliar(t, 0)
	r.sent()
	r.w.PetDumped(r.sess)
	r.barTick(3 * time.Second)
	if ids := setPetIDs(r.sent()); len(ids) != 1 || ids[0] != fs.told || fs.told == 0 {
		t.Fatalf("after a MemoryDump Healbot told %v (told %#x), want once", ids, fs.told)
	}
	r.summon(t, fireElemental(), 5*time.Second)
	if name, ok := r.w.DismissFamiliar(r.sess); !ok || name != "Healbot" {
		t.Fatalf("/familiar dismiss: %q %v", name, ok)
	}
	if msgs := r.sent(); opIndex(msgs, protocol.OpcodeClearPet) >= 0 {
		t.Errorf("/familiar dismiss with a pet out cleared the pet's bar: % X", msgs)
	}
	if r.w.petOfLocked(r.p) == nil {
		t.Error("the pet went with Healbot")
	}
}

// A pet's death with Healbot out leaves the pet bar clear (Heirloom petition
// 2791: a client's auto-cast saw Healbot as the pet and never summoned it
// again), so a summoning in the grace takes the bar and Healbot is never told.
func TestHealbotLeavesTheBarClearForAPetsRecast(t *testing.T) {
	r := newPetRig(t)
	fs := r.barFamiliar(t, 0)
	pet, _ := r.summon(t, fireElemental(), 5*time.Second)
	r.barTick(14 * time.Second)
	r.sent()
	r.w.mu.Lock()
	r.w.killNPC(pet)
	r.w.mu.Unlock()
	gone := time.Duration(0)
	for at := 15 * time.Second; at < 120*time.Second && gone == 0; at += time.Second {
		r.barTick(at)
		if r.w.pets[r.p.ServerID] == nil {
			gone = at
		}
	}
	if gone == 0 {
		t.Fatal("the dead pet never left")
	}
	r.barTick(gone + 5*time.Second)
	if ids := setPetIDs(r.sent()); len(ids) != 0 || fs.told != 0 {
		t.Fatalf("the pet dead: Healbot told %v (told %#x), want the bar left clear", ids, fs.told)
	}
	r.sent()
	r.p.Power = r.p.MaxPower
	if !r.w.StartCast(r.sess, fireElemental(), 0xFFFFFFFF, r.at(gone+10*time.Second)) {
		t.Fatalf("the recast did not begin (the pet gone at %v): %q", gone, linesOf(r.sent()))
	}
	r.w.mu.Lock()
	r.w.castSystem(r.at(gone + 10*time.Second + fireElemental().CastTime + castLand))
	r.w.mu.Unlock()
	again, msgs := r.w.petOfLocked(r.p), r.sent()
	againID := r.w.objectIDForEntityLocked(r.sess, again.ServerID)
	if ids := setPetIDs(msgs); len(ids) != 1 || ids[0] != againID {
		t.Fatalf("the recast: 0x007E %v, want the new pet's %#x", ids, againID)
	}
	r.barTick(gone + 20*time.Second + familiarBarGrace)
	if ids := setPetIDs(r.sent()); len(ids) != 0 || fs.told != 0 {
		t.Errorf("with the new pet out Healbot told %v", ids)
	}
}
