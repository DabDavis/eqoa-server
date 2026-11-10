package entity

import (
	"encoding/binary"
	"testing"
	"time"

	"github.com/eqoa/server/internal/protocol"
)

// Petition 1351 (Jibrish, a GM's rule): "All charm lines and pet lines need to
// be able to be out and not lose the pet while they summon the genie in their
// inventory. This is an example where dual pets are allowed but only for that
// sort of thing. Dont make it so they can summon their spell pet and charm at
// the same time. Its only for the vendor stones." A genie (genie.go: a lamp's
// vendor, GenieOf, never Owner) comes, stands, opens its windows, waits and
// goes beside a pet, summoned or charmed, through the whole tick: the pet stays
// its owner's, out in the world on the slot and id it had, its charm held, and
// its owner is sent nothing of it (no 0x007F, which takes CLIENT's pet bar
// away, no 0x007E for anything else, no line). The control: a second summoning
// replaces the first pet, and the same watch sees its 0x007F.

// genieTick runs the whole tick at d (the genie's system, the pets', the
// view's), the view's pass due.
func genieTick(r *petRig, d time.Duration) {
	r.w.mu.Lock()
	r.w.TickCount = VisibilityTickInterval - 1 // tickLocked counts it up to its pass
	r.w.mu.Unlock()
	r.w.Tick(r.at(d))
}

// petUntouched fails the test when the pet is no longer the rig's player's as
// it was, or when msgs tell its owner of a pet.
func petUntouched(t *testing.T, r *petRig, pet *NPC, id uint32, msgs [][]byte, when string) {
	t.Helper()
	if got := r.w.PetOf(r.p); got != pet.ServerID {
		t.Fatalf("%s: the pet is %d, want %d (%q)", when, got, pet.ServerID, pet.Name)
	}
	if n, ok := r.w.NPCs[pet.ServerID]; !ok || n != pet || pet.Dead || pet.Owner != r.p.ServerID {
		t.Fatalf("%s: the pet is gone from the world or not its owner's (in world %v, owner %d)", when, ok, pet.Owner)
	}
	if got := r.w.objectIDForEntityLocked(r.sess, pet.ServerID); got != id {
		t.Errorf("%s: the pet's id %#x, want %#x", when, got, id)
	}
	for _, m := range msgs {
		switch op := opOfMsg(m); {
		case op == protocol.OpcodeClearPet, op == protocol.OpcodeNoCollideRemove:
			t.Errorf("%s: sent %#04x: %x", when, uint16(op), m)
		case op == protocol.OpcodeSetPet && binary.LittleEndian.Uint32(m[2:]) != id:
			t.Errorf("%s: 0x007E names %#x, not the pet's %#x", when, binary.LittleEndian.Uint32(m[2:]), id)
		}
	}
	for owner, ps := range r.w.pets {
		if ps.npc.GenieOf != 0 || ps.npc.Owner != owner {
			t.Errorf("%s: a genie (%d) on the pets' list of %d", when, ps.npc.ServerID, owner)
		}
	}
}

func TestTheGenieNeverTakesAPet(t *testing.T) {
	defer func(f func(int) int) { charmIntn = f }(charmIntn)
	charmIntn = dice(99)
	for _, kind := range []string{"summoned", "charmed"} {
		t.Run(kind, func(t *testing.T) {
			r, _, _ := charmRig(t)
			var pet *NPC
			if kind == "summoned" {
				pet, _ = r.summon(t, fireElemental(), 0)
			} else {
				if began, _ := castCharm(t, r, captivation(), crabID, 0); !began || r.w.petOfLocked(r.p) != r.enemy {
					t.Fatal("the crab was not charmed")
				}
				pet = r.enemy
			}
			genieTick(r, 20*time.Second)
			id := r.w.objectIDForEntityLocked(r.sess, pet.ServerID)
			r.sent()
			petUntouched(t, r, pet, id, nil, "before the genie")

			// it comes and stands, with the pet beside
			gid, err := r.w.SummonGenie(r.sess, 5002, r.at(21*time.Second))
			if err != nil {
				t.Fatal(err)
			}
			for d := 21 * time.Second; d <= 30*time.Second; d += 500 * time.Millisecond {
				genieTick(r, d)
			}
			petUntouched(t, r, pet, id, r.sent(), "the genie standing")
			if r.w.objectIDForEntityLocked(r.sess, gid) == 0 {
				t.Error("the genie is on none of its master's slots: the view did not run")
			}
			// its bank: the genie leaves, the window stays its master's
			if !r.w.GenieOpened(r.sess, GenieBank, 0, r.at(31*time.Second)) {
				t.Fatal("no genie to open the bank")
			}
			if _, ok := r.w.GenieWindowNPC(r.sess, GenieBank, 42, r.at(32*time.Second)); !ok {
				t.Fatal("the bank's request was not the genie's")
			}
			for d := 32 * time.Second; d <= 40*time.Second; d += 500 * time.Millisecond {
				genieTick(r, d)
			}
			petUntouched(t, r, pet, id, r.sent(), "the genie's bank")
			// summoned again and sent away; then again, left to wait out its time
			r.w.SummonGenie(r.sess, 5002, r.at(41*time.Second))
			genieTick(r, 41*time.Second)
			r.w.DismissGenie(r.sess)
			genieTick(r, 42*time.Second)
			r.w.SummonGenie(r.sess, 5002, r.at(43*time.Second))
			for d := 43 * time.Second; d <= 43*time.Second+GenieWait+10*time.Second; d += 5 * time.Second {
				genieTick(r, d)
			}
			if len(genies(r.w)) != 0 {
				t.Errorf("a genie outlived its wait: %d", len(genies(r.w)))
			}
			petUntouched(t, r, pet, id, r.sent(), "the genie gone")
			if kind == "charmed" && (pet.Charm == nil || pet.NPCType != charmNPCType) {
				t.Errorf("the charm let go: hold %v, npc flags %#x", pet.Charm, pet.NPCType)
			}

			// the control: a summoning replaces the pet, seen by the same watch
			r.p.Power = r.p.MaxPower
			_, msgs := r.summon(t, fireElemental(), 5*time.Minute)
			if opIndex(msgs, protocol.OpcodeClearPet) < 0 || r.w.PetOf(r.p) == pet.ServerID {
				t.Errorf("the control: a new summoning kept the %s pet (sent %x)", kind, msgs)
			}
		})
	}
}

// Nor does a genie count as a pet (petition 1351: "dual pets are allowed but
// only for that sort of thing"): with only a genie out a charm is cast and
// takes its creature, a summoning makes its pet, a pet command is answered
// "You do not have a pet." and commands nothing, and a pet's spell (scope 3)
// is refused the same way; the genie stands through all of it, never on the
// pets' list. The control: with the charmed pet out, a second charm is still
// refused, the genie standing or not (TestACharmIsNotCastWithAPetOut).
func TestTheGenieIsNoPet(t *testing.T) {
	defer func(f func(int) int) { charmIntn = f }(charmIntn)
	charmIntn = dice(99)
	r, _, _ := charmRig(t)
	gid, err := r.w.SummonGenie(r.sess, 5002, r.at(0))
	if err != nil {
		t.Fatal(err)
	}
	genie := r.w.NPCs[gid]
	r.sent()

	r.w.PetCommand(r.sess, petCmdDismiss, 0, r.at(time.Second))
	if msgs := r.sent(); len(msgs) != 1 || chatText(msgs[0]) != castNoPet || r.w.NPCs[gid] != genie {
		t.Errorf("a pet command with only the genie out: %x, genie standing %v", msgs, r.w.NPCs[gid] == genie)
	}
	petHeal := CastSpell{SpellID: 620, DisplayIndex: 10, Level: 20, Scope: 3, Name: "Mend Pet", CastTime: time.Second,
		Recast: time.Second, PowerCost: 10, Range: 30, Def: ParseSpellText("Pet, Increase Hitpoints by 100")}
	if began, msgs := castCharm(t, r, petHeal, crabID, 2*time.Second); began || len(msgs) != 1 || !containsUTF16(msgs[0], castNoPet) {
		t.Errorf("a pet's spell with only the genie out: began %v, sent %x", began, msgs)
	}

	if began, msgs := castCharm(t, r, captivation(), crabID, 10*time.Second); !began || !castsAt(msgs) ||
		r.w.petOfLocked(r.p) != r.enemy || r.w.NPCs[gid] != genie {
		t.Fatalf("a charm with the genie out: began %v, pet %v, genie standing %v", began, r.w.petOfLocked(r.p), r.w.NPCs[gid] == genie)
	}
	for owner, ps := range r.w.pets {
		if ps.npc == genie {
			t.Errorf("the genie on the pets' list of %d", owner)
		}
	}

	r2 := newPetRig(t)
	gid2, _ := r2.w.SummonGenie(r2.sess, 5002, r2.at(0))
	pet, _ := r2.summon(t, fireElemental(), time.Second)
	if pet == nil || pet.GenieOf != 0 || r2.w.NPCs[gid2] == nil || r2.w.NPCs[gid2].GenieOf != r2.p.ServerID {
		t.Errorf("a summoning with the genie out: pet %v, genie standing %v", pet, r2.w.NPCs[gid2] != nil)
	}

	// the control: the charmed crab out, a second charm is refused, the genie by
	if began, msgs := castCharm(t, r, enticement(), 0x01312E97, 20*time.Second); began || len(msgs) != 1 {
		t.Errorf("the control: a second charm with the charmed crab out: began %v, sent %x", began, msgs)
	}
}

// containsUTF16 is whether m carries s as UTF-16 (a line's words, whatever its
// opcode).
func containsUTF16(m []byte, s string) bool {
	w := utf16le(s)
	for i := 0; i+len(w) <= len(m); i++ {
		if string(m[i:i+len(w)]) == string(w) {
			return true
		}
	}
	return false
}
