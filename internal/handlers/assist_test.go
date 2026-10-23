package handlers

import (
	"bytes"
	"net"
	"testing"

	"github.com/eqoa/pkg/pkg/buffer"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// 0x00CB and 0x007B answered with a 0x007B, and the target they read is the
// player's last 0x0010, -1 included: the leader letting its target go (a
// 0x0010 of -1, which names no entity) leaves nothing to assist on.
func TestAssistAnswersWithTheTargetsTarget(t *testing.T) {
	w := entity.NewWorld()
	mgr := session.NewManager()
	w.SessionManager = mgr
	player := func(port byte, p *entity.Player, self uint32) *session.Session {
		s := mgr.CreateSession(&net.UDPAddr{IP: net.IPv4(10, 0, 0, port), Port: 7000 + int(port)}, uint32(port), 1, 1, 2, true)
		w.Players[p.ServerID] = p
		s.SetECSEntity(p)
		s.SetCharacterInWorld(true)
		s.SetPlayerObjectID(self)
		return s
	}
	asker := player(8, &entity.Player{ServerID: 1001, Name: "Asker", Level: 20}, 0x1424A1D0)
	leader := player(9, &entity.Player{ServerID: 1002, Name: "Leader", Level: 20}, 0x14330D1E)
	orc := &entity.NPC{ServerID: 2001, Name: "an orc", Level: 20, HP: 100, MaxHP: 100}
	w.NPCs[orc.ServerID] = orc
	see := func(s *session.Session, ch int, id entity.EntityID, obj uint32) {
		vs := entity.GetOrCreateVisibility(s)
		vs.ChannelEntity[ch], vs.ChannelObjectID[ch] = id, obj
	}
	see(asker, 0, 1002, 100001)
	see(asker, 1, orc.ServerID, 100002)
	see(leader, 0, 1001, 100001)
	see(leader, 1, orc.ServerID, 100003)

	h := NewTargetHandler(mgr)
	h.SetGameWorld(w)
	r := NewRegistry()
	h.RegisterHandlers(r)
	send := func(s *session.Session, op protocol.GameOpcode, body func(*buffer.Writer)) [][]byte {
		t.Helper()
		msg := protocol.NewMessage(protocol.MessageTypeReliable, op)
		msg.SetData(build(op, 16, body)[2:])
		fn, ok := r.Get(op)
		if !ok {
			t.Fatalf("no handler for %#04x", uint16(op))
		}
		if err := fn(s, msg); err != nil {
			t.Fatal(err)
		}
		var out [][]byte
		for _, m := range s.Queue().GetPending() {
			out = append(out, append([]byte(nil), m.Data()...))
		}
		return out
	}
	target := func(s *session.Session, id uint32) {
		send(s, protocol.OpcodeClassicTargetInfo, func(wr *buffer.Writer) { wr.WriteUint32(0); wr.WriteUint32(id) })
	}
	assist := func(op protocol.GameOpcode, id uint32) []byte {
		t.Helper()
		out := send(asker, op, func(wr *buffer.Writer) { wr.WriteUint32(id) })
		if len(out) != 1 {
			t.Fatalf("%#04x: %d messages, want one 0x007B", uint16(op), len(out))
		}
		return out[0]
	}

	target(leader, 100003) // the orc, as the leader knows it
	if got, want := assist(protocol.OpcodeAssist, 100001), []byte{0x7B, 0x00, 0xA2, 0x86, 0x01, 0x00}; !bytes.Equal(got, want) {
		t.Fatalf("Assist Target on the leader: % x, want % x (the orc as the asker knows it, 100002)", got, want)
	}
	target(leader, 0xFFFFFFFF)
	if got, want := assist(protocol.OpcodeAssist, 100001), []byte{0x7B, 0x00, 0xFF, 0xFF, 0xFF, 0xFF}; !bytes.Equal(got, want) {
		t.Fatalf("after the leader let its target go: % x, want % x", got, want)
	}
	// out of a group, Assist Leader is none
	if got, want := assist(protocol.OpcodeAssistTarget, 100002), []byte{0x7B, 0x00, 0xFF, 0xFF, 0xFF, 0xFF}; !bytes.Equal(got, want) {
		t.Fatalf("Assist Leader out of a group: % x, want % x", got, want)
	}
	// a session with no player is not answered
	stray := mgr.CreateSession(&net.UDPAddr{IP: net.IPv4(10, 0, 0, 3), Port: 7003}, 3, 1, 1, 2, true)
	if out := send(stray, protocol.OpcodeAssist, func(wr *buffer.Writer) { wr.WriteUint32(100001) }); len(out) != 0 {
		t.Fatalf("a session with no player was answered: % x", out)
	}
}
