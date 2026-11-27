package handlers

import (
	"bytes"
	"encoding/binary"
	"net"
	"testing"

	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// dpsMeterOffers is every OpcodeDPSMeterOffer queued for the session's client.
func dpsMeterOffers(sess *session.Session) [][]byte {
	var out [][]byte
	for _, m := range sess.Queue().PeekPending() {
		if d := m.Data(); len(d) >= 2 && protocol.GameOpcode(binary.LittleEndian.Uint16(d)) == protocol.OpcodeDPSMeterOffer {
			out = append(out, d)
		}
	}
	return out
}

// The first 0x0014 offers the meter, with the newest layout after the opcode,
// once; the second 0x0014 offers nothing more.
func TestDPSMeterOfferedAtWorldEntry(t *testing.T) {
	h := &CharacterHandler{}
	in := protocol.NewMessage(protocol.MessageTypeReliable, protocol.OpcodeCharacterInWorld)
	sess := entitySlotsSession()
	for range 2 {
		if err := h.HandleCharacterInWorld(sess, in); err != nil {
			t.Fatal(err)
		}
	}
	if got, want := dpsMeterOffers(sess), []byte{0x3A, 0x7E, entity.DPSMeterLayoutNewest}; len(got) != 1 || !bytes.Equal(got[0], want) {
		t.Fatalf("world entry offered %x, want one %x", got, want)
	}
}

// The client's answer is its player's: a u8 1 has the meter (layout 1), 2 in
// layout 2, 3 in layout 3, one past the newest the newest, 0 stops it, a
// u32's 1 and 0 the same by their first byte; an empty one, or a session with
// no player, changes nothing. A reset is taken from a client with a meter, and
// the registry routes both.
func TestHandleDPSMeter(t *testing.T) {
	w := entity.NewWorld()
	mgr := session.NewManager()
	w.SessionManager = mgr
	sess := mgr.CreateSession(&net.UDPAddr{IP: net.IPv4(198, 51, 100, 58), Port: 6058}, 58, 58, 1, 2, true)
	p := &entity.Player{ServerID: 1058, Name: "Meterer"}
	w.Players[p.ServerID] = p
	sess.SetECSEntity(p)
	sess.SetCharacterInWorld(true)
	h := NewDPSMeterHandler(w)
	send := func(s *session.Session, op protocol.GameOpcode, data []byte) {
		m := protocol.NewMessage(protocol.MessageTypeReliable, op)
		m.SetData(data)
		var err error
		if op == protocol.OpcodeDPSMeterReset {
			err = h.HandleDPSMeterReset(s, m)
		} else {
			err = h.HandleDPSMeter(s, m)
		}
		if err != nil {
			t.Fatal(err)
		}
	}
	send(sess, protocol.OpcodeDPSMeterAnswer, []byte{1})
	if !w.DPSMeter(sess) {
		t.Fatal("answered 1: no meter")
	}
	if got := w.DPSMeterLayout(sess); got != 1 {
		t.Fatalf("answered 1 (every vanilla before layout 2): layout %d", got)
	}
	send(sess, protocol.OpcodeDPSMeterAnswer, []byte{2})
	if got := w.DPSMeterLayout(sess); got != 2 {
		t.Fatalf("answered 2: layout %d", got)
	}
	send(sess, protocol.OpcodeDPSMeterAnswer, []byte{3})
	if got := w.DPSMeterLayout(sess); got != 3 {
		t.Fatalf("answered 3 (mercs' lines apart): layout %d", got)
	}
	send(sess, protocol.OpcodeDPSMeterAnswer, []byte{9})
	if got := w.DPSMeterLayout(sess); got != entity.DPSMeterLayoutNewest {
		t.Fatalf("answered past the newest: layout %d, want %d", got, entity.DPSMeterLayoutNewest)
	}
	send(sess, protocol.OpcodeDPSMeterAnswer, []byte{1})
	if got := w.DPSMeterLayout(sess); got != 1 {
		t.Fatalf("back to 1: layout %d", got)
	}
	send(sess, protocol.OpcodeDPSMeterAnswer, nil)
	if !w.DPSMeter(sess) {
		t.Fatal("an empty answer stopped it")
	}
	send(sess, protocol.OpcodeDPSMeterReset, []byte{1})
	if !w.DPSMeter(sess) {
		t.Fatal("a reset turned it off")
	}
	send(sess, protocol.OpcodeDPSMeterAnswer, []byte{0})
	if w.DPSMeter(sess) {
		t.Fatal("answered 0: still on")
	}
	send(sess, protocol.OpcodeDPSMeterAnswer, []byte{1, 0, 0, 0})
	if !w.DPSMeter(sess) {
		t.Fatal("a u32 1: no meter")
	}
	send(sess, protocol.OpcodeDPSMeterAnswer, []byte{0, 0, 0, 0})
	if w.DPSMeter(sess) {
		t.Fatal("a u32 0: still on")
	}
	nobody := mgr.CreateSession(&net.UDPAddr{IP: net.IPv4(198, 51, 100, 59), Port: 6059}, 59, 59, 1, 2, true)
	send(nobody, protocol.OpcodeDPSMeterAnswer, []byte{1})
	send(nobody, protocol.OpcodeDPSMeterReset, []byte{1})
	if w.DPSMeter(nobody) {
		t.Fatal("a session with no player has a meter")
	}
	// the registry routes 0x7E3B and 0x7E3D to it
	r := NewRegistry()
	h.RegisterHandlers(r)
	for _, op := range []protocol.GameOpcode{protocol.OpcodeDPSMeterAnswer, protocol.OpcodeDPSMeterReset} {
		m := protocol.NewMessage(protocol.MessageTypeReliable, op)
		m.SetData([]byte{1})
		if err := r.Handle(sess, m); err != nil {
			t.Fatalf("%v through the registry: %v", op, err)
		}
	}
	if !w.DPSMeter(sess) {
		t.Fatal("through the registry: no meter")
	}
}
