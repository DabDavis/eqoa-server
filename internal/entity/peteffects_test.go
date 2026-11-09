package entity

import (
	"encoding/binary"
	"strings"
	"testing"
	"time"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/protocol"
)

// The pet's effects (peteffects.go): on the pet, never on its owner's effects
// window, and to a client that asked, whenever they change.

// Spells of a necromancer's pet's kind (the user, 2026-11-09: "jibcro is
// showing pets buffs as if on him"): the pet's own buff on itself, its owner's
// on it (scope 3, on the pet whatever is targeted), and the owner's on itself.
var (
	petOwnBuff = CastSpell{SpellID: 9301, Level: 55, Scope: 0, Name: "Undead Proc", Icon: 0xBDD60F01,
		Def: ParseSpellText("Self, Increase STR by 20, 30min")}
	ownerOnPet = CastSpell{SpellID: 9302, Level: 51, Scope: 3, Name: "Soul Mastery", Icon: 0xBDD60F02, CastTime: time.Second,
		Def: ParseSpellText("Pet, Increase STR by 45, Increase DEX by 45, 10min")}
	ownerOnPet2 = CastSpell{SpellID: 9303, Level: 60, Scope: 3, Name: "Largesse of Plague", Icon: 0xA3F1BB2A, CastTime: time.Second,
		Def: ParseSpellText("Pet, Increase AC by 40, 5min")}
	ownerOnSelf = CastSpell{SpellID: 9304, Level: 40, Scope: 0, Name: "Dark Ward", Icon: 0xBDD60F04, CastTime: time.Second,
		Def: ParseSpellText("Self, Increase AC by 50, 10min")}
)

// petBuffsItself lands sp as a workbook pet's buff ability on itself does
// (petLandLocked's AbilityBuff aimed at the pet).
func (r *petRig) petBuffsItself(sp CastSpell, at time.Duration) {
	r.w.mu.Lock()
	defer r.w.mu.Unlock()
	ps := r.w.pets[r.p.ServerID]
	a := &templateAbility{TemplateAbility: database.TemplateAbility{Kind: database.AbilityBuff}}
	r.w.petLandLocked(&petCast{ps: ps, a: a, spell: &sp, target: ps.npc.ServerID}, r.at(at))
}

// castAt casts sp at target (as the client names it) and lands it.
func (r *petRig) castAt(t *testing.T, sp CastSpell, target uint32, at time.Duration) {
	t.Helper()
	r.p.Power = r.p.MaxPower
	if !r.w.StartCast(r.sess, sp, target, r.at(at)) {
		t.Fatalf("%s not begun", sp.Name)
	}
	r.w.mu.Lock()
	r.w.castSystem(r.at(at + sp.CastTime + castLand))
	r.w.mu.Unlock()
}

// effectsWindow is the owner's effects channel (0x43) as its client reads it,
// after the system sent it.
func (r *petRig) effectsWindow(t *testing.T) []string {
	t.Helper()
	r.w.mu.Lock()
	r.p.StatsDirty = true
	r.w.buffChannelSyncSystem()
	r.w.mu.Unlock()
	_, names, short := channel43(r.sess.Connection().GetClientBuffUpdate().GetData(), EffectRecordSize)
	if short {
		t.Fatal("the effects channel reads short")
	}
	return names
}

// The owner's effects window (channel 0x43, every client's, a PS2's too) is its
// own effects alone: its pet's buff on itself and the owner's buff on its pet
// are on the pet, and not on the owner's channel (from petition 1041 the
// channel carried them after the owner's: run against that, both checks fail).
// The control: the owner's buff on itself is on it.
func TestTheOwnersEffectsWindowIsHisOwn(t *testing.T) {
	r := newPetRig(t)
	pet, _ := r.summon(t, fireElemental(), 0)
	r.petBuffsItself(petOwnBuff, 10*time.Second)
	r.castAt(t, ownerOnPet, r.w.objectIDForEntityLocked(r.sess, pet.ServerID), 20*time.Second)
	var onPet []string
	for _, e := range pet.Effects {
		onPet = append(onPet, e.Name)
	}
	if strings.Join(onPet, ", ") != "Undead Proc, Soul Mastery" {
		t.Fatalf("on the pet: %q, want its own buff and its owner's", onPet)
	}
	if len(r.p.Effects) != 0 || len(r.p.Buffs) != 0 {
		t.Fatalf("on the owner: %d effects, %d slots; want none", len(r.p.Effects), len(r.p.Buffs))
	}
	if got := r.effectsWindow(t); len(got) != 0 {
		t.Errorf("the owner's effects window shows %q, want nothing: both are the pet's", got)
	}
	r.castAt(t, ownerOnSelf, r.me, 30*time.Second)
	if got := r.effectsWindow(t); strings.Join(got, ", ") != "Dark Ward" {
		t.Errorf("the control, the owner's own buff: its window shows %q, want Dark Ward alone", got)
	}
}

// petEffectsSent is the bodies of every OpcodePetEffectsData queued since the
// last look.
func (r *petRig) petEffectsSent() [][]byte {
	var out [][]byte
	for _, m := range r.sent() {
		if opOfMsg(m) == protocol.OpcodePetEffectsData {
			out = append(out, m[2:])
		}
	}
	return out
}

// petEffect is one effect of a set as a client reads it.
type petEffect struct {
	icon uint32
	left uint32
	name string
}

// readPetEffects reads a set's body; ok false when it is short or has bytes
// past its end.
func readPetEffects(b []byte) (fx []petEffect, ok bool) {
	if len(b) < 1 {
		return nil, false
	}
	n, pos := int(b[0]), 1
	for range n {
		if len(b)-pos < 9 {
			return nil, false
		}
		e := petEffect{icon: binary.LittleEndian.Uint32(b[pos:]), left: binary.LittleEndian.Uint32(b[pos+4:])}
		k := int(b[pos+8])
		pos += 9
		if len(b)-pos < k {
			return nil, false
		}
		e.name, pos = string(b[pos:pos+k]), pos+k
		fx = append(fx, e)
	}
	return fx, pos == len(b)
}

// look runs the pet's tick and the set's pass at d, a pass's tick.
func (r *petRig) look(d time.Duration) {
	r.w.mu.Lock()
	defer r.w.mu.Unlock()
	r.w.petSystem(r.at(d))
	r.w.TickCount = petEffectsTicks
	r.w.petEffectsSystem(r.at(d))
}

func names(fx []petEffect) string {
	var s []string
	for _, e := range fx {
		s = append(s, e.name)
	}
	return strings.Join(s, ", ")
}

// The pet's effects go to a client that asked (0x7E32 1), whenever they
// change: none to one that never asked (a PS2), the set at once after the ask
// (each effect's icon, its time left, its name, in the order they went on),
// nothing while it stays the same, again when an effect goes on, is cast again
// (it goes last) or runs out, an empty set once as the pet goes, and nothing
// once the client stopped.
func TestThePetsEffectsGoToAClientThatAsked(t *testing.T) {
	r := newPetRig(t)
	pet, _ := r.summon(t, fireElemental(), 0)
	petID := r.w.objectIDForEntityLocked(r.sess, pet.ServerID)
	r.petBuffsItself(petOwnBuff, 10*time.Second)
	r.castAt(t, ownerOnPet, petID, 20*time.Second) // lands at 22.4 s
	r.sent()
	r.look(30 * time.Second)
	if got := r.petEffectsSent(); len(got) != 0 {
		t.Fatalf("a client that never asked (a PS2) was sent %x", got)
	}

	if !r.w.SetPetEffects(r.sess, true) {
		t.Fatal("the ask found no player")
	}
	r.look(30 * time.Second)
	got := r.petEffectsSent()
	if len(got) != 1 {
		t.Fatalf("after the ask: %d sets, want one", len(got))
	}
	fx, ok := readPetEffects(got[0])
	if !ok || names(fx) != "Undead Proc, Soul Mastery" {
		t.Fatalf("the set %x reads %v (ok %v), want the pet's own buff and its owner's", got[0], fx, ok)
	}
	if fx[0].icon != petOwnBuff.Icon || fx[1].icon != ownerOnPet.Icon {
		t.Errorf("icons %#x, %#x; want %#x, %#x", fx[0].icon, fx[1].icon, petOwnBuff.Icon, ownerOnPet.Icon)
	}
	// its own 30 minutes from 10 s, the owner's 10 minutes from 22.4 s: as of 30 s
	if want := uint32((30*time.Minute - 20*time.Second).Milliseconds()); fx[0].left != want {
		t.Errorf("its own buff's time left %d ms, want %d", fx[0].left, want)
	}
	if want := uint32((10*time.Minute - 7600*time.Millisecond).Milliseconds()); fx[1].left != want {
		t.Errorf("the owner's buff's time left %d ms, want %d", fx[1].left, want)
	}

	r.look(31 * time.Second)
	if got := r.petEffectsSent(); len(got) != 0 {
		t.Errorf("sent again with nothing changed: %x", got)
	}

	r.castAt(t, ownerOnPet2, petID, 40*time.Second)
	r.look(43 * time.Second)
	if got := r.petEffectsSent(); len(got) != 1 {
		t.Errorf("another buff on the pet: %d sets, want one", len(got))
	} else if fx, _ := readPetEffects(got[0]); names(fx) != "Undead Proc, Soul Mastery, Largesse of Plague" {
		t.Errorf("another buff on the pet: %v", fx)
	}

	r.castAt(t, ownerOnPet, petID, 50*time.Second) // cast again: last, its time whole
	r.look(53 * time.Second)
	if got := r.petEffectsSent(); len(got) != 1 {
		t.Errorf("a recast: %d sets, want one", len(got))
	} else if fx, _ := readPetEffects(got[0]); names(fx) != "Undead Proc, Largesse of Plague, Soul Mastery" ||
		fx[2].left != uint32((10*time.Minute-600*time.Millisecond).Milliseconds()) {
		t.Errorf("a recast: %v, want Soul Mastery last with its time whole", fx)
	}

	r.look(42*time.Second + 5*time.Minute + time.Second) // Largesse of Plague ran out (5 min from 42.4 s)
	if got := r.petEffectsSent(); len(got) != 1 {
		t.Errorf("an effect ran out: %d sets, want one", len(got))
	} else if fx, _ := readPetEffects(got[0]); names(fx) != "Undead Proc, Soul Mastery" {
		t.Errorf("an effect ran out: %v", fx)
	}

	r.p.Power = r.p.MaxPower
	r.w.PetCommand(r.sess, 5, 0, r.at(6*time.Minute)) // dismissed
	r.sent()
	r.look(6*time.Minute + time.Second)
	if got := r.petEffectsSent(); len(got) != 1 {
		t.Errorf("the pet gone: %d sets, want one", len(got))
	} else if fx, ok := readPetEffects(got[0]); !ok || len(fx) != 0 {
		t.Errorf("the pet gone: %x, want an empty set", got[0])
	}
	r.look(6*time.Minute + 2*time.Second)
	if got := r.petEffectsSent(); len(got) != 0 {
		t.Errorf("no pet, nothing changed: sent %x", got)
	}

	// stopped: nothing, though a new pet comes with a buff on it
	r.w.SetPetEffects(r.sess, false)
	pet2, _ := r.summon(t, fireElemental(), 7*time.Minute)
	r.castAt(t, ownerOnPet, r.w.objectIDForEntityLocked(r.sess, pet2.ServerID), 8*time.Minute)
	r.look(8*time.Minute + 5*time.Second)
	if got := r.petEffectsSent(); len(got) != 0 {
		t.Errorf("a client that stopped was sent %x", got)
	}
}

// A name goes as its first 31 characters, at most 63 bytes, on a character's
// edge; a set holds at most PetEffectsMost; and a body of the most effects of
// the longest names stays under a message.
func TestThePetsEffectsBody(t *testing.T) {
	if got := petEffectName(strings.Repeat("a", 40)); got != strings.Repeat("a", MaxEffectName) {
		t.Errorf("40 letters: %q, want the first %d", got, MaxEffectName)
	}
	if got := petEffectName(strings.Repeat("é", 40)); got != strings.Repeat("é", 31) || len(got) != 62 {
		t.Errorf("40 two-byte letters: %q (%d bytes), want 31 of them", got, len(got))
	}
	if got := petEffectName(strings.Repeat("中", 40)); got != strings.Repeat("中", 21) || len(got) != 63 {
		t.Errorf("40 three-byte letters: %q (%d bytes), want 21 of them, 63 bytes", got, len(got))
	}
	now := time.Unix(1_800_000_000, 0)
	var fx []*ActiveEffect
	for i := range PetEffectsMost + 4 {
		fx = append(fx, &ActiveEffect{Icon: uint32(i), Name: strings.Repeat("中", 40), Until: now.Add(time.Minute)})
	}
	body := encodePetEffects(fx, now)
	got, ok := readPetEffects(body)
	if !ok || len(got) != PetEffectsMost || got[0].left != 60000 {
		t.Fatalf("%d effects: read %d (ok %v), the first's time %d", len(fx), len(got), ok, got[0].left)
	}
	if len(body)+2 > 1200 {
		t.Errorf("the most body is %d bytes with its opcode, over 1200", len(body)+2)
	}
	if got, ok := readPetEffects(encodePetEffects([]*ActiveEffect{{Name: "x", Until: now.Add(-time.Second)}}, now)); !ok || got[0].left != 0 {
		t.Errorf("one past its end: %v, want 0 ms left", got)
	}
}
