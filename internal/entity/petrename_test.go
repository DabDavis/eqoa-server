package entity

import (
	"bytes"
	"testing"
	"time"

	"github.com/eqoa/server/internal/protocol"
)

// petrename.go: a summoned pet named by its owner is drawn by the name at
// once (the channel's 24 bytes at 116, every viewer's), its owner's pet bar
// told again a second later (0x007F, then 0x007E and its non-collide entry
// under the new name); the name is kept for its spell, so the next summoning
// of it comes by it, while what it is (its made-as name) stays the spell's. A
// charmed creature keeps its own name.
func TestARenamedPetIsDrawnByItsNameAndKeptForItsSpell(t *testing.T) {
	r := newPetRig(t)
	pet, _ := r.summon(t, fireElemental(), 0)
	id := r.w.objectIDForEntityLocked(r.sess, pet.ServerID)
	old, spell, why := r.w.RenamePet(r.sess, "Rennis", r.at(20*time.Second))
	if why != "" || old != "Fire Elemental" || spell != 717 || pet.Name != "Rennis" {
		t.Fatalf("RenamePet: %q %d %q, the pet %q", old, spell, why, pet.Name)
	}
	if name := string(bytes.TrimRight(SerializeNPCUpdate(id, pet)[116:140], "\x00")); name != "Rennis" {
		t.Errorf("the channel draws %q", name)
	}
	ops := func() []protocol.GameOpcode {
		var out []protocol.GameOpcode
		for _, m := range r.sent() {
			switch op := opOfMsg(m); op {
			case protocol.OpcodeClearPet, protocol.OpcodeSetPet, protocol.OpcodeNoCollideAdd, protocol.OpcodeNoCollideRemove:
				out = append(out, op)
				if op == protocol.OpcodeNoCollideAdd && !bytes.Contains(m, []byte("Rennis")) {
					t.Errorf("the non-collide entry % X is not under the new name", m)
				}
			}
		}
		return out
	}
	r.sent()
	r.tick(20*time.Second + 500*time.Millisecond)
	if got := ops(); len(got) != 0 {
		t.Errorf("the pet bar told again before its second: %v", got)
	}
	r.tick(21 * time.Second)
	got := ops()
	want := []protocol.GameOpcode{protocol.OpcodeClearPet, protocol.OpcodeNoCollideRemove, protocol.OpcodeSetPet, protocol.OpcodeNoCollideAdd}
	if len(got) != len(want) {
		t.Fatalf("the retell sent %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("the retell sent %v, want %v", got, want)
		}
	}
	r.tick(22 * time.Second)
	if got := ops(); len(got) != 0 {
		t.Errorf("told again a second time: %v", got)
	}
	if r.p.PetNames[717] != "Rennis" {
		t.Errorf("the name kept for its spell: %v", r.p.PetNames)
	}
	r.w.mu.Lock()
	ps := r.w.petStateLocked(r.p)
	if ps.madeAs() != "fire elemental" {
		t.Errorf("made as %q, want the spell's", ps.madeAs())
	}
	r.w.dismissPetLocked(ps, r.sess, "", r.at(23*time.Second))
	r.w.mu.Unlock()
	r.p.Power = 2000
	again, _ := r.summon(t, fireElemental(), 30*time.Second)
	if again == pet || again.Name != "Rennis" {
		t.Errorf("the next summoning is named %q", again.Name)
	}
	if n, ok := r.w.PetName(r.sess); !ok || n != "Rennis" {
		t.Errorf("PetName %q %v", n, ok)
	}
	// the control: another character's summoning of the spell is the spell's
	r.p.PetNames = nil
	r.w.mu.Lock()
	r.w.dismissPetLocked(r.w.petStateLocked(r.p), r.sess, "", r.at(50*time.Second))
	r.w.mu.Unlock()
	r.p.Power = 2000
	if plain, _ := r.summon(t, fireElemental(), 60*time.Second); plain.Name != "Fire Elemental" {
		t.Errorf("an unnamed summoning is %q", plain.Name)
	}
}

func TestACharmedCreatureKeepsItsName(t *testing.T) {
	r := newPetRig(t)
	if _, _, why := r.w.RenamePet(r.sess, "Rennis", r.t0); why != PetRenameNoPet {
		t.Errorf("no pet: %q", why)
	}
	pet, _ := r.summon(t, fireElemental(), 0)
	r.w.mu.Lock()
	ps := r.w.petStateLocked(r.p)
	ps.npc.Charm, ps.spell = &charmHold{}, 0 // as a charm holds a creature
	r.w.mu.Unlock()
	if _, _, why := r.w.RenamePet(r.sess, "Rennis", r.t0); why != PetRenameCharmed || pet.Name != "Fire Elemental" {
		t.Errorf("a charmed creature renamed: %q, %q", why, pet.Name)
	}
	if _, summoned := r.w.PetOutName(r.sess); summoned {
		t.Error("a charmed creature is offered a name")
	}
}

// A named individual of the world (a capital first, no one's) is one; a
// generic creature in lower case, a multi-word name and a pet are not.
func TestNPCIndividualNamed(t *testing.T) {
	r := newPetRig(t)
	r.w.NPCs[4000] = &NPC{ServerID: 4000, Name: "Dabrish"}
	r.w.NPCs[4001] = &NPC{ServerID: 4001, Name: "Pettyname", Owner: r.p.ServerID}
	for name, want := range map[string]bool{"dabrish": true, "DABRISH": true, "Zombie": false, "Fire": false, "Pettyname": false, "": false} {
		if got := r.w.NPCIndividualNamed(name); got != want {
			t.Errorf("NPCIndividualNamed(%q) = %v, want %v", name, got, want)
		}
	}
}
