package entity

import (
	"encoding/binary"
	"testing"

	"github.com/eqoa/server/internal/protocol"
)

// A pet's taunt is seen (the user, 2026-11-08: "i am not seeing the taunt fire
// though it may be"): everyone who sees the pet is sent its 0x00B1 on the
// target with the Taunt spells' visual; the control, a session that does not
// see it, is sent nothing.
func TestAPetsTauntIsSeen(t *testing.T) {
	c := newCaster(t)
	c.npc.Name = "Grand Animation"
	_, otherSess := c.addViewer(t)
	c.sent()
	drain(otherSess)
	c.w.mu.Lock()
	c.w.showPetTauntLocked(c.npc, c.p.ServerID)
	c.w.mu.Unlock()
	for who, got := range map[string][][]byte{"the player": c.sent(), "a bystander": drain(otherSess)} {
		if len(got) != 1 || opOfMsg(got[0]) != protocol.OpcodeCastingCombat {
			t.Fatalf("%s got %x, want one 0x00B1", who, got)
		}
		m := got[0]
		if fx := binary.LittleEndian.Uint32(m[len(m)-4:]); fx != petTauntFX {
			t.Errorf("%s: visual %08X, want the taunt's %08X", who, fx, petTauntFX)
		}
	}
	// the control: a pet no one sees shows no one anything
	far := &NPC{ServerID: 991234, Name: "Unseen Animation"}
	c.w.mu.Lock()
	c.w.showPetTauntLocked(far, c.p.ServerID)
	c.w.mu.Unlock()
	if got := c.sent(); len(got) != 0 {
		t.Errorf("a pet the player does not see showed %x", got)
	}
}
