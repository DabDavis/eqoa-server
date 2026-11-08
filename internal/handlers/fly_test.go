package handlers

import (
	"bytes"
	"encoding/binary"
	"strings"
	"testing"
	"time"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// flyMessages are the 0x7E1Bs queued on s: each its u32.
func flyMessages(s *session.Session) []uint32 {
	var out []uint32
	for _, m := range s.Queue().PeekPending() {
		d := m.Data()
		if len(d) >= 6 && protocol.GameOpcode(binary.LittleEndian.Uint16(d)) == protocol.OpcodeFly {
			out = append(out, binary.LittleEndian.Uint32(d[2:]))
		}
	}
	return out
}

func saysFly(t *testing.T, s *session.Session, want string) {
	t.Helper()
	got := heard(t, s)
	for _, l := range got {
		if strings.Contains(l.text, want) {
			return
		}
	}
	t.Fatalf("no line with %q: %+v", want, got)
}

// A GM's #fly turns it over, on and off say which, either spelling, and every
// answer says where it stands; the client is told each. Nobody else has it: a
// player's is answered as an unknown command, in either spelling, and neither
// flies nor is sent anything, though its client says it can (the control: the
// same client's GM flies).
func TestFlyIsAGMsToggle(t *testing.T) {
	c, gs, gp, ps, pp := gmWorld(t)
	gs.SetFlyClient(true)
	ps.SetFlyClient(true)

	c.say(gs, "#fly")
	if !gp.Flying {
		t.Fatal("#fly did not turn it on")
	}
	if got := flyMessages(gs); len(got) != 1 || got[0] != 1 {
		t.Fatalf("the client was told %v, want one 1", got)
	}
	saysFly(t, gs, flyOnLine)
	gs.Queue().GetPending()

	c.say(gs, "#fly")
	if gp.Flying {
		t.Fatal("#fly again did not turn it off")
	}
	if got := flyMessages(gs); len(got) != 1 || got[0] != 0 {
		t.Fatalf("the client was told %v, want one 0", got)
	}
	saysFly(t, gs, flyOffLine)
	gs.Queue().GetPending()

	for _, step := range []struct {
		line string
		on   bool
	}{{"#fly on", true}, {"#fly on", true}, {"#FLY OFF", false}, {"/fly on", true}, {"/fly", false}} {
		c.say(gs, step.line)
		if gp.Flying != step.on {
			t.Fatalf("%s: flying %v", step.line, gp.Flying)
		}
		want := uint32(0)
		if step.on {
			want = 1
		}
		if got := flyMessages(gs); len(got) != 1 || got[0] != want {
			t.Fatalf("%s: the client was told %v", step.line, got)
		}
		gs.Queue().GetPending()
	}

	c.say(gs, "#fly sideways")
	saysFly(t, gs, "Usage: #fly")
	if gp.Flying || len(flyMessages(gs)) != 0 {
		t.Fatalf("a bad word changed something: flying %v, %v", gp.Flying, flyMessages(gs))
	}

	for _, spelled := range []string{"#fly", "/fly", "#fly on"} {
		ps.Queue().GetPending()
		c.say(ps, spelled)
		if got := heard(t, ps); len(got) != 1 || !strings.HasPrefix(got[0].text, "Unrecognized command") {
			t.Fatalf("a player's %s: %+v", spelled, got)
		}
		if pp.Flying || len(flyMessages(ps)) != 0 {
			t.Fatalf("a player's %s: flying %v, %v", spelled, pp.Flying, flyMessages(ps))
		}
	}
}

// A GM on a client that never said it flies (a PS2, an older build) is told so
// in a plain line and sent nothing: no 0x7E1B, on or off, whatever it types. The
// control: once it has said so, the same GM flies.
func TestFlyOnAClientThatCannot(t *testing.T) {
	c, gs, gp, _, _ := gmWorld(t)
	if SendFlying(gs, true) || len(flyMessages(gs)) != 0 {
		t.Fatalf("SendFlying told a client that cannot fly: %v", flyMessages(gs))
	}
	for _, spelled := range []string{"#fly", "#fly on", "#fly off", "/fly"} {
		gs.Queue().GetPending()
		c.say(gs, spelled)
		saysFly(t, gs, flyCannotLine)
		if got := flyMessages(gs); len(got) != 0 {
			t.Fatalf("%s: sent %v to a client that cannot fly", spelled, got)
		}
		if gp.Flying {
			t.Fatalf("%s: a client that cannot fly is flying", spelled)
		}
	}
	gs.Queue().GetPending()

	h := NewFlyHandler()
	if err := h.HandleFly(gs, incoming(protocol.OpcodeFly, []byte{1, 0, 0})); err != nil || gs.FlyClient() {
		t.Fatalf("a short word was taken: %v", err)
	}
	if err := h.HandleFly(gs, incoming(protocol.OpcodeFly, []byte{1, 0, 0, 0})); err != nil || !gs.FlyClient() {
		t.Fatalf("its word was not kept: %v", err)
	}
	// its word alone turns nothing on (a modified client's could say anything)
	if gp.Flying || len(flyMessages(gs)) != 0 {
		t.Fatalf("its word alone: flying %v, %v", gp.Flying, flyMessages(gs))
	}
	c.say(gs, "#fly")
	if !gp.Flying || len(flyMessages(gs)) != 1 {
		t.Fatalf("the control: flying %v, %v", gp.Flying, flyMessages(gs))
	}
}

// The wire: the opcode and a u32, the bytes vanilla reads.
func TestTheFlyMessage(t *testing.T) {
	if got, want := buildFly(true), []byte{0x1B, 0x7E, 1, 0, 0, 0}; !bytes.Equal(got, want) {
		t.Errorf("on % x, want % x", got, want)
	}
	if got, want := buildFly(false), []byte{0x1B, 0x7E, 0, 0, 0, 0}; !bytes.Equal(got, want) {
		t.Errorf("off % x, want % x", got, want)
	}
}

// A flying GM through a restart that keeps the players: the new process has it
// flying where it flew (its row says where it last stood, and a hover sends no
// 0x40 to correct that), its dump goes THERE, and the client is told it flies
// after the dump, with the client's word kept. The row keeps the place it stood.
// The control: one that does not fly gets no 0x7E1B, and stands where its row
// says.
func TestAFlyingGMStaysFlyingThroughARestart(t *testing.T) {
	const stoodY, flewY = float32(54.1), float32(300.5)
	db := keepDB(t)
	for _, flying := range []bool{true, false} {
		_, gw, sm := keepProcess(t, db)
		k := session.Kept{Addr: "203.0.113.7:5007", InstanceID: 7, SessionID: 500000007, ClientEndpoint: 7, ServerEndpoint: 1,
			State: session.StateInGame, HasInstance: true, DidServerInitiate: true, CharacterInWorld: true, InGame: true,
			AccountID: 3, CharServerID: 7, PlayerObjectID: 500000007, FlyClient: true}
		rs, err := sm.Resume(k)
		if err != nil {
			t.Fatal(err)
		}
		if !rs.FlyClient() {
			t.Fatal("the client's word was not kept")
		}
		kp := entity.KeptPlayer{CharServerID: 7, Name: "Hero", Flying: flying, Place: [3]float32{25340, flewY, 15730}}
		p, err := ResumeCharacter(rs, KeptExtras{}, kp, time.Now())
		if err != nil {
			t.Fatal(err)
		}
		if p.Flying != flying {
			t.Fatalf("flying %v, want %v", p.Flying, flying)
		}
		if flying && (p.Y != flewY || p.X != 25340) {
			t.Fatalf("back at (%.1f, %.1f), it flew at (25340, %.1f)", p.X, p.Y, flewY)
		}
		if !flying && p.Y != stoodY {
			t.Fatalf("the control: at y %.1f, its row's %.1f", p.Y, stoodY)
		}
		if len(flyMessages(rs)) != 0 {
			t.Fatal("told before its dump")
		}
		if !ResumeDump(rs, false) {
			t.Fatal("no dump")
		}
		var ops []protocol.GameOpcode
		for _, m := range rs.Queue().PeekPending() {
			if d := m.Data(); len(d) >= 2 {
				ops = append(ops, protocol.GameOpcode(binary.LittleEndian.Uint16(d)))
			}
		}
		dump, fly := -1, -1
		for i, op := range ops {
			if op == protocol.OpcodeMemoryDump && dump < 0 {
				dump = i
			}
			if op == protocol.OpcodeFly {
				fly = i
			}
		}
		if flying && (dump < 0 || fly < dump) {
			t.Fatalf("the dump at %d, the flight at %d: %v", dump, fly, ops)
		}
		if !flying && fly >= 0 {
			t.Fatalf("the control was told it flies: %v", ops)
		}
		if got, ok := gw.PlayerSnapshot(p.ServerID); !ok || (flying && got.Y != flewY) {
			t.Fatalf("the dump's place: %v, %v", got.Y, ok)
		}
		rec, err := database.NewSQLiteCharacterStore(db).GetByServerID(7)
		if err != nil || rec.Y != stoodY {
			t.Fatalf("the row's height %v (%v), it stood at %.1f", rec.Y, err, stoodY)
		}
	}
}

// subs is levels with the sub GMs (a tester's row marked subgm).
type subs struct {
	levels
	sub map[int]bool
}

func (g subs) SubGM(id int) (bool, error) { return g.sub[id], nil }

// A sub GM (the user, 2026-11-08: "make his account a sub gm new category above
// tester than can fly"): its world entry offers its client the flight (0x7E07),
// never the GM channel (0x0700); /fly and #fly turn it over; no other GM command
// (/goto, #pos) is its. The control: an account that is no sub GM is offered
// nothing and its /fly is an unknown command.
func TestASubGMFlies(t *testing.T) {
	c := newChatWorld(t)
	c.h.SetAccessLevels(subs{levels{1: 0, 2: 0}, map[int]bool{1: true}})
	fs, fp := c.join(1, "Neo", 25000, 15000, 0)
	fs.SetAccountID(1)
	ps, pp := c.join(2, "Player", 25010, 15010, 0)
	ps.SetAccountID(2)
	ops := func(s *session.Session) (offer, channel int) {
		for _, m := range s.Queue().PeekPending() {
			d := m.Data()
			if len(d) < 2 {
				continue
			}
			switch protocol.GameOpcode(binary.LittleEndian.Uint16(d)) {
			case protocol.OpcodeGMTriggers:
				offer++
			case protocol.OpcodeAdminCode:
				channel++
			}
		}
		return
	}
	fs.Queue().GetPending()
	ps.Queue().GetPending()
	c.h.SendGMChannel(fs)
	c.h.SendGMChannel(ps)
	if offer, channel := ops(fs); offer != 1 || channel != 0 {
		t.Fatalf("the sub GM's world entry: %d offers, %d GM channels; want 1 and 0", offer, channel)
	}
	if offer, channel := ops(ps); offer != 0 || channel != 0 {
		t.Fatalf("the control's world entry: %d offers, %d GM channels; want none", offer, channel)
	}
	fs.Queue().GetPending()
	fs.SetFlyClient(true) // its answer to the offer
	ps.SetFlyClient(true)

	c.say(fs, "/fly")
	if !fp.Flying {
		t.Fatal("the sub GM's /fly did not turn it on")
	}
	if got := flyMessages(fs); len(got) != 1 || got[0] != 1 {
		t.Fatalf("its client was told %v, want one 1", got)
	}
	saysFly(t, fs, flyOnLine)
	fs.Queue().GetPending()
	c.say(fs, "#fly off")
	if fp.Flying {
		t.Fatal("#fly off did not turn it off")
	}
	fs.Queue().GetPending()
	for _, other := range []string{"/goto 1 2 3", "#pos"} {
		c.say(fs, other)
		if got := heard(t, fs); len(got) != 1 || !strings.HasPrefix(got[0].text, "Unrecognized command") {
			t.Fatalf("the sub GM's %s, a GM's command: %+v", other, got)
		}
		if _, _, _, moved := teleported(fs); moved {
			t.Fatalf("the sub GM's %s moved it", other)
		}
		fs.Queue().GetPending()
	}

	c.say(ps, "/fly")
	if got := heard(t, ps); len(got) != 1 || !strings.HasPrefix(got[0].text, "Unrecognized command") {
		t.Fatalf("the control's /fly: %+v", got)
	}
	if pp.Flying || len(flyMessages(ps)) != 0 {
		t.Fatalf("the control flies: %v, %v", pp.Flying, flyMessages(ps))
	}
}
