package handlers

import (
	"encoding/binary"
	"net"
	"testing"

	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// Players in sessions, driven end to end through the handlers. Ids are per
// session here: a client names another player by the id its own session knows
// it by (World.IDSeenBy), which seen is.

type groupFixture struct {
	t     *testing.T
	sm    *session.Manager
	world *entity.World
	sess  map[string]*session.Session
	p     map[string]*entity.Player
}

func newGroupFixture(t *testing.T, names ...string) *groupFixture {
	f := &groupFixture{t: t, sm: session.NewManager(), world: entity.NewWorld(),
		sess: map[string]*session.Session{}, p: map[string]*entity.Player{}}
	f.world.SessionManager = f.sm
	for i, name := range names {
		s := f.sm.CreateSession(&net.UDPAddr{IP: net.IPv4(198, 51, 100, byte(i+1)), Port: 6000 + i},
			uint32(i+1), uint32(i+1), uint16(i+1), uint16(i+1), true)
		s.SetPlayerObjectID(uint32(500000001 + i))
		s.SetCharacterInWorld(true)
		p := &entity.Player{ServerID: entity.EntityID(9001 + i), Name: name, HP: 100, MaxHP: 100}
		f.world.Players[p.ServerID] = p
		s.SetECSEntity(p)
		f.sess[name], f.p[name] = s, p
	}
	return f
}

// seen is the id viewer's client knows who by.
func (f *groupFixture) seen(viewer, who string) uint32 {
	return f.world.IDSeenBy(f.sess[viewer], f.p[who])
}

// groupMsg is a request as the dispatcher hands it over: Data() is the
// payload after the opcode.
func groupMsg(op protocol.GameOpcode, body ...[]byte) *protocol.Message {
	var b []byte
	for _, x := range body {
		b = append(b, x...)
	}
	m := protocol.NewMessage(protocol.MessageTypeReliable, op)
	m.SetData(b)
	return m
}

func u32b(v uint32) []byte { return binary.LittleEndian.AppendUint32(nil, v) }
func str8b(s string) []byte {
	return append(binary.LittleEndian.AppendUint32(nil, uint32(len(s))), s...)
}

// take returns and clears what name's session was sent, opcode first.
func (f *groupFixture) take(name string) [][]byte {
	var out [][]byte
	for _, m := range f.sess[name].Queue().GetPending() {
		out = append(out, append([]byte(nil), m.Data()...))
	}
	return out
}

func opsOf(msgs [][]byte) []protocol.GameOpcode {
	var ops []protocol.GameOpcode
	for _, m := range msgs {
		ops = append(ops, protocol.GameOpcode(binary.LittleEndian.Uint16(m)))
	}
	return ops
}

// lastOf is the last message of op in msgs, opcode first, or nil.
func lastOf(msgs [][]byte, op protocol.GameOpcode) []byte {
	var last []byte
	for _, m := range msgs {
		if len(m) >= 2 && protocol.GameOpcode(binary.LittleEndian.Uint16(m)) == op {
			last = m
		}
	}
	return last
}
