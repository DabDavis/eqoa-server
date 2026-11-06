package handlers

import (
	"testing"
	"time"

	"github.com/eqoa/pkg/pkg/buffer"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/protocol"
)

// Petition 1112 through the handlers: a 0x0010 naming a creature reads the hands
// the player holds now, with no 0x000F ever coming; the button up then fights it
// with them. The control: with no target named, the button up starts nothing.
func TestATargetChangeReadsTheHandsForAPressThatSendsNothing(t *testing.T) {
	f := newSwitchFixture(t, ClassWarrior)
	f.own(f.pattern("Test Blade", 14, 1, 0), 14)
	f.loadList()
	h := NewTargetHandler(nil)
	h.SetGameWorld(f.w)
	h.SetInventoryStore(f.inv)
	target := func(objectID uint32) {
		t.Helper()
		msg := protocol.NewMessage(protocol.MessageTypeReliable, protocol.OpcodeTarget)
		msg.SetData(build(protocol.OpcodeTarget, 16, func(w *buffer.Writer) {
			w.WriteUint32(1)
			w.WriteUint32(objectID)
		})[2:])
		if err := h.HandleTarget(f.sess, msg); err != nil {
			t.Fatal(err)
		}
	}
	button := func() {
		f.w.ApplyClientUpdate(f.player.ServerID, entity.ClientUpdate{X: f.player.X, Y: f.player.Y, Z: f.player.Z, HasAutoAttack: true,
			AutoAttack: true, At: time.Now().Add(-time.Minute)})
	}

	target(0xFFFFFFFF)
	button()
	f.w.TickCount = 0
	f.w.Tick(time.Now())
	if f.player.AttackTarget != 0 || f.player.Swing.Delay != 0 {
		t.Fatalf("no target: fighting %d, swing %+v; want neither", f.player.AttackTarget, f.player.Swing)
	}

	target(f.npcO)
	want := swingOf(f.w, f.inv, f.sess, f.player)
	if want.Delay <= 0 || f.player.Swing != want {
		t.Fatalf("after the 0x0010: swing %+v, want the hands' %+v", f.player.Swing, want)
	}
	button()
	f.w.TickCount = 0
	f.w.Tick(time.Now())
	if f.player.AttackTarget != f.npc.ServerID {
		t.Errorf("the button up on the creature: fighting %d, want %d", f.player.AttackTarget, f.npc.ServerID)
	}
}
