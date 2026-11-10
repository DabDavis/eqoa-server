package handlers

import (
	"testing"
	"time"

	"github.com/eqoa/server/internal/protocol"
)

// Petition 1435 (Afk, a wizard holding Swampstench, punched for 1; the GM
// Jibrish: hitting with a held item is "supposed to be no"): hands holding a
// held item and no weapon never swing, and the press is refused with a red
// line, said once however fast the presses come. The control: the same orb
// with a blade in the primary hand fights.
func TestAHeldItemAloneCannotFight(t *testing.T) {
	f := newSwitchFixture(t, ClassWizard)
	f.own(f.pattern("Test Orb", 18, 0, 0), equipHeldOff)
	f.loadList()
	s := swingOf(f.w, f.inv, f.sess, f.player)
	if !s.Never || s.Held != "Test Orb" {
		t.Fatalf("the orb alone: swing %+v, want it never in reach with the orb named", s)
	}
	f.sess.Queue().GetPending()
	now := time.Now()
	if f.w.EngageAttack(f.sess, f.npcO, s, now) || f.player.AttackTarget != 0 {
		t.Fatalf("the press engaged (fighting %d) with an orb alone", f.player.AttackTarget)
	}
	if got := heard(t, f.sess); len(got) != 1 || got[0].op != protocol.OpcodeColoredChat || got[0].text != "You can't fight while holding Test Orb." {
		t.Fatalf("told %+v", got)
	}
	f.sess.Queue().GetPending()
	f.w.EngageAttack(f.sess, f.npcO, s, now.Add(time.Second))
	if got := heard(t, f.sess); len(got) != 0 {
		t.Fatalf("a second press a second on was told again: %+v", got)
	}

	// the control: a blade in the primary hand beside the orb fights
	f.own(f.pattern("Test Blade", 14, 1, 0), equipPrimary)
	f.loadList()
	s = swingOf(f.w, f.inv, f.sess, f.player)
	if s.Never || s.Held != "" {
		t.Fatalf("a blade and the orb: swing %+v, want a blade's", s)
	}
	if !f.w.EngageAttack(f.sess, f.npcO, s, now.Add(3*time.Second)) || f.player.AttackTarget != f.npc.ServerID {
		t.Fatalf("a blade and the orb did not engage (fighting %d)", f.player.AttackTarget)
	}
}
