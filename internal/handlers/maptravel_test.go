package handlers

import (
	"encoding/binary"
	"encoding/hex"
	"math"
	"testing"

	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// mapGround is a ground whose terrain is a floor at 60 over Tunaria's
// (24000..27000, 14000..17000), and none anywhere else.
type mapGround struct{}

func (mapGround) Floor(world uint8, x, y, z float32) (float32, bool) {
	if world == 0 && x >= 24000 && x <= 27000 && z >= 14000 && z <= 17000 && 60 <= y+2 {
		return 60, true
	}
	return y, false
}

// mapTravel is a client's OpcodeMapTravelTo for the spot.
func mapTravel(world uint32, x, z float32) *protocol.Message {
	b := make([]byte, 12)
	binary.LittleEndian.PutUint32(b, world)
	binary.LittleEndian.PutUint32(b[4:], math.Float32bits(x))
	binary.LittleEndian.PutUint32(b[8:], math.Float32bits(z))
	return incoming(protocol.OpcodeMapTravelTo, b)
}

// teleportedY is the height of the 0x07F6 queued for s, if one is.
func teleportedY(s *session.Session) (float32, bool) {
	for _, m := range s.Queue().PeekPending() {
		d := m.Data()
		if len(d) >= 15 && protocol.GameOpcode(binary.LittleEndian.Uint16(d)) == protocol.OpcodeTeleport {
			return f32(d[7:]), true
		}
	}
	return 0, false
}

// Travel by the map is a GM's alone: its client is offered it with the GM's
// channel (a u32 1) and a player's is not; the GM's ask moves its character to
// the spot, on the ground's floor there, told in a [GM] line. A player's ask
// moves no one and is answered with nothing, whatever it asks: a spot with
// ground, a world past the last, a body too short. The GM's own asks for a
// spot the ground knows no floor at, one past every world's edge and a world
// past the last are told so, and move it not. The control for each refusal:
// the GM's same ask at a spot with ground moves it.
func TestMapTravelIsAGMs(t *testing.T) {
	c, gs, gp, ps, pp := gmWorld(t)
	c.gw.SetGround(mapGround{})
	c.h.SendGMChannel(gs)
	c.h.SendGMChannel(ps)
	if got := sentAll(gs, protocol.OpcodeMapTravel); len(got) != 1 || hex.EncodeToString(got[0]) != "01000000" {
		t.Fatalf("the GM's offer: %x", got)
	}
	if got := sentAll(ps, protocol.OpcodeMapTravel); len(got) != 0 {
		t.Fatalf("the player's offer: %x", got)
	}
	gs.Queue().GetPending()
	ps.Queue().GetPending()

	travel := func(s *session.Session, m *protocol.Message) {
		t.Helper()
		if err := c.h.HandleMapTravelTo(s, m); err != nil {
			t.Fatal(err)
		}
	}

	// the GM's: there, on the floor
	travel(gs, mapTravel(0, 25500, 15600))
	x, z, w, ok := teleported(gs)
	y, _ := teleportedY(gs)
	if !ok || x != 25500 || y != 60 || z != 15600 || w != 0 {
		t.Fatalf("the GM's travel: (%v, %v, %v) world %d, sent %v", x, y, z, w, ok)
	}
	if gp.X != 25500 || gp.Y != 60 || gp.Z != 15600 {
		t.Fatalf("the GM's character at (%v, %v, %v)", gp.X, gp.Y, gp.Z)
	}
	if lines := gmReplies(t, gs); !hasLine(lines, "Travelled to (25500, 60, 15600) world=Tunaria") {
		t.Fatalf("the GM was told %q", lines)
	}

	// a player's: nothing, whatever it sends
	short := incoming(protocol.OpcodeMapTravelTo, []byte{0, 0, 0, 0, 0, 0})
	for name, m := range map[string]*protocol.Message{
		"a spot with ground": mapTravel(0, 25500, 15600),
		"a world past last":  mapTravel(99, 25500, 15600),
		"a short body":       short,
	} {
		travel(ps, m)
		if _, _, _, moved := teleported(ps); moved {
			t.Fatalf("a player's ask for %s sent it a teleport", name)
		}
		if got := heard(t, ps); len(got) != 0 {
			t.Fatalf("a player's ask for %s was answered: %+v", name, got)
		}
		if pp.X != 25010 || pp.Z != 15010 {
			t.Fatalf("a player's ask for %s moved it to (%v, %v)", name, pp.X, pp.Z)
		}
	}

	// the GM's refused: no ground, past every edge, no such world
	for _, c := range []struct {
		name string
		m    *protocol.Message
		says string
	}{
		{"no ground there", mapTravel(0, 1000, 1000), "no ground is known at (1000, 1000) in Tunaria"},
		{"past every edge", mapTravel(0, 3e6, 15600), "is no place to go"},
		{"NaN", mapTravel(0, float32(math.NaN()), 15600), "is no place to go"},
		{"no such world", mapTravel(99, 25500, 15600), "is no place to go"},
	} {
		travel(gs, c.m)
		if _, _, _, moved := teleported(gs); moved {
			t.Fatalf("the GM's ask for %s moved it", c.name)
		}
		if lines := gmReplies(t, gs); !hasLine(lines, c.says) {
			t.Fatalf("the GM's ask for %s: told %q, want %q", c.name, lines, c.says)
		}
	}
	travel(gs, mapTravel(0, 24500, 14500))
	if x, z, _, ok := teleported(gs); !ok || x != 24500 || z != 14500 {
		t.Fatalf("the GM's last travel: (%v, %v), sent %v", x, z, ok)
	}

	// a GM no more (its level taken away): refused as a player's
	gs.Queue().GetPending()
	c.h.SetAccessLevels(levels{1: 0, 2: 0})
	travel(gs, mapTravel(0, 25000, 15000))
	if _, _, _, moved := teleported(gs); moved {
		t.Fatal("an account no longer a GM's travelled")
	}
}
