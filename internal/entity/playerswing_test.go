package entity

import (
	"encoding/binary"
	"net"
	"testing"
	"time"

	"github.com/eqoa/server/internal/protocol"
)

// The GM Jibrish, watching two players fight: "im not seeing their auto attack
// animations fire off reliably". Everyone who sees a player is shown each round's swing
// (0x00B1, its effect at its target, as a creature's), the player itself none: its client
// plays its own. The control: one who does not see the player is sent nothing.
func TestAPlayersRoundsAreShownToThoseWhoSeeIt(t *testing.T) {
	f := newFight(t)
	see := f.w.SessionManager.CreateSession(&net.UDPAddr{IP: net.IPv4(10, 0, 0, 20), Port: 7020}, 20, 1, 1, 2, true)
	watcher := &Player{ServerID: 1020, Name: "Watcher", Level: 60, HP: 100, MaxHP: 100, X: 110, Y: 10, Z: 100}
	f.w.Players[watcher.ServerID] = watcher
	see.SetECSEntity(watcher)
	see.SetCharacterInWorld(true)
	see.SetPlayerObjectID(0x00AA0020)
	vs := GetOrCreateVisibility(see)
	vs.ChannelEntity[0], vs.ChannelObjectID[0] = f.p.ServerID, 0x0B0B0001
	vs.ChannelEntity[1], vs.ChannelObjectID[1] = f.npc.ServerID, 0x0B0B0002

	blind := f.w.SessionManager.CreateSession(&net.UDPAddr{IP: net.IPv4(10, 0, 0, 21), Port: 7021}, 21, 1, 1, 2, true)
	other := &Player{ServerID: 1021, Name: "Faraway", Level: 60, HP: 100, MaxHP: 100, X: 9000, Y: 10, Z: 9000}
	f.w.Players[other.ServerID] = other
	blind.SetECSEntity(other)
	blind.SetCharacterInWorld(true)
	blind.SetPlayerObjectID(0x00AA0021)

	f.p.AutoAttack = true
	swing := Swing{Kind: 5, Delay: 6 * time.Second, Reach: 4.5}
	if !f.w.EngageAttack(f.sess, f.npcID, swing, f.t0) {
		t.Fatal("not engaged")
	}
	casts := func(msgs [][]byte) [][]byte {
		var out [][]byte
		for _, m := range msgs {
			if opOfMsg(m) == protocol.OpcodeCastingCombat {
				out = append(out, m)
			}
		}
		return out
	}
	pending := func(q interface{ GetPending() []*protocol.Message }) [][]byte {
		var out [][]byte
		for _, m := range q.GetPending() {
			out = append(out, append([]byte(nil), m.Data()...))
		}
		return out
	}
	f.tick(300 * time.Millisecond) // the round begins
	got := casts(pending(see.Queue()))
	if len(got) != 1 {
		t.Fatalf("the watcher was sent %d swings, want the round's 1", len(got))
	}
	m := got[0]
	if c, tgt := binary.LittleEndian.Uint32(m[2:]), binary.LittleEndian.Uint32(m[6:]); c != 0x0B0B0001 || tgt != 0x0B0B0002 {
		t.Errorf("the swing from %#x at %#x, want the player %#x at the creature %#x", c, tgt, 0x0B0B0001, 0x0B0B0002)
	}
	// byte 10 the others' count, none; the swing's delay and effect after it
	if ms, fx := binary.LittleEndian.Uint32(m[11:]), binary.LittleEndian.Uint32(m[15:]); ms != 6000 || fx != HashResourceID("Attack4") {
		t.Errorf("the swing's delay %d and effect %#x, want 6000 and Attack4's %#x", ms, fx, HashResourceID("Attack4"))
	}
	if n := len(casts(pending(f.sess.Queue()))); n != 0 {
		t.Errorf("the swinger was sent %d swings; its client plays its own", n)
	}
	if n := len(casts(pending(blind.Queue()))); n != 0 {
		t.Errorf("one who does not see it was sent %d swings", n)
	}
	f.tick(3 * time.Second) // the same round's hits: no more swings
	if n := len(casts(pending(see.Queue()))); n != 0 {
		t.Errorf("the round's hits sent %d more swings", n)
	}
	f.tick(6100 * time.Millisecond) // the next round
	if n := len(casts(pending(see.Queue()))); n != 1 {
		t.Errorf("the next round sent %d swings, want 1", n)
	}
}
