package entity

import (
	"encoding/binary"
	"net"
	"testing"

	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// An emote goes to the players who can see the sender, as 0x0775 under the
// id each of them knows it by; the sender, who played it already, gets none.
func TestShowEmoteToViewers(t *testing.T) {
	w := NewWorld()
	mgr := session.NewManager()
	w.SessionManager = mgr

	mk := func(port int, id EntityID) (*session.Session, *Player) {
		s := mgr.CreateSession(&net.UDPAddr{IP: net.IPv4(10, 0, 0, 7), Port: port}, uint32(port), uint32(port), 1, 2, true)
		p := &Player{ServerID: id, Name: "P"}
		w.Players[id] = p
		s.SetECSEntity(p)
		s.SetCharacterInWorld(true)
		return s, p
	}
	me, mine := mk(7001, 1001)
	viewer, _ := mk(7002, 1002)
	stranger, _ := mk(7003, 1003)

	// The viewer has me on a channel as 0x00BB0007; the stranger does not see me.
	vs := GetOrCreateVisibility(viewer)
	vs.ChannelEntity[3] = mine.ServerID
	vs.ChannelObjectID[3] = 0x00BB0007
	GetOrCreateVisibility(stranger)

	if n := w.ShowEmote(me, ActionWave); n != 1 {
		t.Fatalf("shown to %d, want 1", n)
	}
	got := actions(viewer)
	if len(got) != 1 || got[0][0] != 0x00BB0007 || got[0][1] != ActionWave {
		t.Errorf("viewer got %v, want one 0x0775 (BB0007, %d)", got, ActionWave)
	}
	if len(actions(me)) != 0 || len(actions(stranger)) != 0 {
		t.Error("the sender or a player who cannot see it got the emote")
	}
}

func actions(s *session.Session) [][2]uint32 {
	var out [][2]uint32
	for _, m := range s.Queue().PeekPending() {
		d := m.Data()
		if len(d) == 7 && protocol.GameOpcode(binary.LittleEndian.Uint16(d)) == protocol.OpcodeEntityAction {
			out = append(out, [2]uint32{binary.LittleEndian.Uint32(d[2:]), uint32(d[6])})
		}
	}
	return out
}
