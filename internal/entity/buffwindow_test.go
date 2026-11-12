package entity

import (
	"encoding/binary"
	"strings"
	"testing"
	"time"

	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// The buff window (buffwindow.go, the GM Jibrish's petition 1761).

// bwLine is one line of a set as a client reads it.
type bwLine struct {
	whose, flags byte
	id, icon     uint32
	left         uint32
	name         string
}

// readBuffWindow reads a set's body; ok false when it is short or has bytes past its end.
func readBuffWindow(b []byte) (out []bwLine, ok bool) {
	if len(b) < 1 {
		return nil, false
	}
	n, pos := int(b[0]), 1
	for range n {
		if len(b)-pos < 15 {
			return nil, false
		}
		l := bwLine{whose: b[pos], flags: b[pos+1], id: binary.LittleEndian.Uint32(b[pos+2:]),
			icon: binary.LittleEndian.Uint32(b[pos+6:]), left: binary.LittleEndian.Uint32(b[pos+10:])}
		k := int(b[pos+14])
		pos += 15
		if len(b)-pos < k {
			return nil, false
		}
		l.name, pos = string(b[pos:pos+k]), pos+k
		out = append(out, l)
	}
	return out, pos == len(b)
}

// buffSets is the bodies of every OpcodeBuffWindowData in msgs.
func buffSets(msgs [][]byte) [][]byte {
	var out [][]byte
	for _, m := range msgs {
		if opOfMsg(m) == protocol.OpcodeBuffWindowData {
			out = append(out, m[2:])
		}
	}
	return out
}

func bwNames(ls []bwLine) string {
	var s []string
	for _, l := range ls {
		w := "me"
		if l.whose == BuffOnPet {
			w = "pet"
		}
		if l.flags&1 != 0 {
			w += "+"
		}
		s = append(s, w+":"+l.name)
	}
	return strings.Join(s, ", ")
}

// lookBW runs the effect systems and the set's pass at d.
func (r *petRig) lookBW(d time.Duration) {
	r.w.mu.Lock()
	defer r.w.mu.Unlock()
	r.w.effectSystem(r.at(d))
	r.w.petSystem(r.at(d))
	r.w.TickCount = buffWindowTicks
	r.w.buffWindowSystem(r.at(d))
}

// The set goes to a client that asked, whenever it changes: none to one that never asked (a
// PS2), at once after the ask the player's effects (its own buff, a harmful one not to be
// taken off) then the pet's (its own buff and its owner's on it, both to be taken off), each
// with its time left; nothing while it stays the same; again when one goes on.
func TestTheBuffWindowGoesToAClientThatAsked(t *testing.T) {
	r := newPetRig(t)
	pet, _ := r.summon(t, fireElemental(), 0)
	petID := r.w.objectIDForEntityLocked(r.sess, pet.ServerID)
	r.petBuffsItself(petOwnBuff, 10*time.Second)
	r.castAt(t, ownerOnPet, petID, 20*time.Second)
	r.castAt(t, ownerOnSelf, r.me, 30*time.Second) // lands at 32.4 s
	r.w.mu.Lock()
	r.p.Effects = append(r.p.Effects, &ActiveEffect{SpellID: 9399, Name: "Plague", Icon: 77, Until: r.at(10 * time.Minute), HPTick: -5})
	r.w.mu.Unlock()
	r.sent()
	r.lookBW(40 * time.Second)
	if got := buffSets(r.sent()); len(got) != 0 {
		t.Fatalf("a client that never asked (a PS2) was sent %x", got)
	}
	if !r.w.SetBuffWindow(r.sess, true) {
		t.Fatal("the ask found no player")
	}
	r.lookBW(40 * time.Second)
	got := buffSets(r.sent())
	if len(got) != 1 {
		t.Fatalf("after the ask: %d sets, want one", len(got))
	}
	ls, ok := readBuffWindow(got[0])
	if want := "me+:Dark Ward, me:Plague, pet+:Undead Proc, pet+:Soul Mastery"; !ok || bwNames(ls) != want {
		t.Fatalf("the set reads %q (ok %v), want %q", bwNames(ls), ok, want)
	}
	if ls[0].id != uint32(ownerOnSelf.SpellID) || ls[0].icon != ownerOnSelf.Icon {
		t.Errorf("the first line's id %d icon %#x, want %d %#x", ls[0].id, ls[0].icon, ownerOnSelf.SpellID, ownerOnSelf.Icon)
	}
	if want := uint32((10*time.Minute - 7600*time.Millisecond).Milliseconds()); ls[0].left != want {
		t.Errorf("Dark Ward's time left %d ms, want %d", ls[0].left, want)
	}
	r.lookBW(41 * time.Second)
	if got := buffSets(r.sent()); len(got) != 0 {
		t.Errorf("sent again with nothing changed: %x", got)
	}
	r.castAt(t, ownerOnPet2, petID, 50*time.Second)
	r.lookBW(53 * time.Second)
	if got := buffSets(r.sent()); len(got) != 1 {
		t.Errorf("another buff on the pet: %d sets, want one", len(got))
	} else if ls, _ := readBuffWindow(got[0]); !strings.HasSuffix(bwNames(ls), "pet+:Largesse of Plague") {
		t.Errorf("another buff on the pet: %q", bwNames(ls))
	}
	r.w.SetBuffWindow(r.sess, false)
	r.castAt(t, ownerOnSelf, r.me, 60*time.Second)
	r.lookBW(65 * time.Second)
	if got := buffSets(r.sent()); len(got) != 0 {
		t.Errorf("a client that stopped was sent %x", got)
	}
}

// More than channel 0x43's eight: the set carries the player's effects past the eighth (they
// land with no slot there), up to two rows of eight; and a slot no effect backs (a GM's
// #effect) after them, with no id and no end, not to be taken off.
func TestTheBuffWindowCarriesMoreThanEight(t *testing.T) {
	r := newPetRig(t)
	now := r.at(time.Minute)
	r.w.mu.Lock()
	for i := range 20 {
		name := "Buff " + string(rune('A'+i))
		r.p.Effects = append(r.p.Effects, &ActiveEffect{SpellID: 100 + i, Name: name, Icon: uint32(i + 1), Until: now.Add(time.Hour), Mods: StatMods{Str: 5}})
		if len(r.p.Buffs) < MaxEffects {
			r.p.Buffs = append(r.p.Buffs, BuffSlot{Icon: uint32(i + 1), Name: name})
		}
	}
	ls := r.w.buffWindowSetLocked(r.p, now)
	r.w.mu.Unlock()
	if len(ls) != BuffWindowPlayerMost || ls[15].Name != "Buff P" || !ls[15].Removable {
		t.Fatalf("20 effects: %d lines (last %+v), want the first 16", len(ls), ls[len(ls)-1])
	}
	r2 := newPetRig(t)
	r2.w.mu.Lock()
	r2.p.Effects = []*ActiveEffect{{SpellID: 5, Name: "Real", Icon: 9, Until: now.Add(time.Hour), Mods: StatMods{Str: 5}}}
	r2.p.Buffs = []BuffSlot{{Icon: 9, Name: "Real"}, {Icon: 4, Name: "GM slot"}}
	ls = r2.w.buffWindowSetLocked(r2.p, now)
	r2.w.mu.Unlock()
	body := EncodeBuffWindow(ls)
	got, ok := readBuffWindow(body)
	if !ok || bwNames(got) != "me+:Real, me:GM slot" || got[1].id != 0 || got[1].left != 0xFFFFFFFF {
		t.Fatalf("a backed slot and a GM's: %q %+v (ok %v)", bwNames(got), got, ok)
	}
}

// A removal: the player's own beneficial buff goes as its time running out ends it (off the
// effects, off channel 0x43's slots, its stat gone), the player told and the set sent at
// once; a harmful one stays, refused in a line; one already gone, or a wrong icon, is
// nothing; the pet's buff goes off the pet. The control: the others stay.
func TestABuffIsTakenOffOnRequest(t *testing.T) {
	r := newPetRig(t)
	pet, _ := r.summon(t, fireElemental(), 0)
	petID := r.w.objectIDForEntityLocked(r.sess, pet.ServerID)
	r.castAt(t, ownerOnPet, petID, 10*time.Second)
	r.castAt(t, ownerOnSelf, r.me, 20*time.Second)
	r.w.mu.Lock()
	r.p.Effects = append(r.p.Effects, &ActiveEffect{SpellID: 9399, Name: "Plague", Icon: 77, Until: r.at(10 * time.Minute), HPTick: -5})
	r.w.mu.Unlock()
	r.w.SetBuffWindow(r.sess, true)
	r.sent()
	ac := r.p.Mods.AC
	if ac < 50 || len(r.p.Buffs) != 1 {
		t.Fatalf("Dark Ward not on: AC mod %d, %d slots", ac, len(r.p.Buffs))
	}
	at := 30 * time.Second
	if r.w.RemoveBuffAt(r.sess, BuffOnPlayer, uint32(ownerOnSelf.SpellID), ownerOnSelf.Icon+1, r.at(at)) {
		t.Error("a wrong icon took it off")
	}
	r.sent() // the set again, unchanged
	if !r.w.RemoveBuffAt(r.sess, BuffOnPlayer, uint32(ownerOnSelf.SpellID), ownerOnSelf.Icon, r.at(at)) {
		t.Fatal("Dark Ward not taken off")
	}
	if r.p.Mods.AC != ac-50 || len(r.p.Buffs) != 0 {
		t.Errorf("after: AC mod %d (was %d), %d slots on channel 0x43", r.p.Mods.AC, ac, len(r.p.Buffs))
	}
	if l := lines(r.sess); len(l) == 0 || l[len(l)-1] != "You remove Dark Ward." {
		t.Errorf("told %q", l)
	}
	if got := buffSets(r.sent()); len(got) != 1 {
		t.Errorf("the set at once: %d sent", len(got))
	} else if ls, _ := readBuffWindow(got[0]); bwNames(ls) != "me:Plague, pet+:Soul Mastery" {
		t.Errorf("the set after: %q", bwNames(ls))
	}
	if r.w.RemoveBuffAt(r.sess, BuffOnPlayer, 9399, 77, r.at(at)) {
		t.Error("a harmful effect was taken off")
	}
	if l := lines(r.sess); len(l) == 0 || l[len(l)-1] != "Plague cannot be removed." {
		t.Errorf("the refusal told %q", l)
	}
	if len(r.p.Effects) != 1 {
		t.Errorf("the harmful one: %d effects left, want it", len(r.p.Effects))
	}
	if r.w.RemoveBuffAt(r.sess, BuffOnPlayer, uint32(ownerOnSelf.SpellID), ownerOnSelf.Icon, r.at(at)) {
		t.Error("one already gone was taken off again")
	}
	r.sent()
	if !r.w.RemoveBuffAt(r.sess, BuffOnPet, uint32(ownerOnPet.SpellID), ownerOnPet.Icon, r.at(at)) {
		t.Fatal("the pet's buff not taken off")
	}
	if len(pet.Effects) != 0 {
		t.Errorf("on the pet after: %d effects", len(pet.Effects))
	}
	if l := lines(r.sess); len(l) == 0 || !strings.HasPrefix(l[len(l)-1], "You remove Soul Mastery from ") {
		t.Errorf("told %q", l)
	}
}

// The Tester Pro (petition 1725) comes off on request: its speed, its ceiling on the hit
// points and its pin on the stats go with it, as at its end.
func TestTheTesterProComesOff(t *testing.T) {
	c := newCaster(t)
	p := c.p
	sow, ownMax := p.Speed, p.MaxHPNow()
	if r := c.use(t, testerPro(), 0xFFFFFFFF, 0); r != "" {
		t.Fatalf("refused: %q", r)
	}
	if p.MaxHPNow() != 1 || len(p.Effects) != 1 {
		t.Fatalf("not on: max HP %d, %d effects", p.MaxHPNow(), len(p.Effects))
	}
	e := p.Effects[0]
	c.w.mu.Lock()
	ls := c.w.buffWindowSetLocked(p, c.at(time.Second))
	c.w.mu.Unlock()
	if len(ls) != 1 || !ls[0].Removable {
		t.Fatalf("the set: %+v, want the Tester Pro to be taken off", ls)
	}
	if !c.w.RemoveBuffAt(c.sess, BuffOnPlayer, uint32(e.SpellID), e.Icon, c.at(time.Second)) {
		t.Fatal("not taken off")
	}
	if p.SpeedNow() != sow || p.MaxHPNow() != ownMax || len(p.Effects) != 0 {
		t.Errorf("after: speed %v (own %v), max HP %d (own %d), %d effects", p.SpeedNow(), sow, p.MaxHPNow(), ownMax, len(p.Effects))
	}
}

// No player: nothing, and nothing panics.
func TestABuffRemovalWithNoPlayer(t *testing.T) {
	w := NewWorld()
	mgr := session.NewManager()
	w.SessionManager = mgr
	if w.SetBuffWindow(nil, true) || w.RemoveBuffAt(nil, 0, 1, 1, time.Now()) {
		t.Error("no session: something done")
	}
}
