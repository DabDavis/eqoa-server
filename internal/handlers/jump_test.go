package handlers

import (
	"bytes"
	"encoding/json"
	"net"
	"testing"
	"time"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// setJumps sets world.jump's offer for one test.
func setJumps(t *testing.T, on bool) {
	t.Helper()
	was := jumpsOn.Load()
	t.Cleanup(func() { jumpsOn.Store(was) })
	SetJumps(on)
}

// The jump is offered (0x7E28, a u32 1) only to a client that answered the bug
// report offer, never at the world entry itself, so a PS2 (which answers no
// offer) is never sent it; an answer of 0 brings none, and a server without
// jumps offers none (the control: the same answer on a server with them does).
func TestJumpOfferedOnlyAfterTheBugReportAnswer(t *testing.T) {
	setJumps(t, true)
	h := &CharacterHandler{}
	in := protocol.NewMessage(protocol.MessageTypeReliable, protocol.OpcodeCharacterInWorld)
	sess := entitySlotsSession()
	if err := h.HandleCharacterInWorld(sess, in); err != nil {
		t.Fatal(err)
	}
	if got := queued(sess, protocol.OpcodeJump); len(got) != 0 {
		t.Fatalf("the world entry offered the jump to whatever entered: %x", got)
	}
	if got := queued(sess, protocol.OpcodeBugReportOffer); len(got) != 1 {
		t.Fatalf("no bug report offer at the world entry: %x", got)
	}

	_, sh, _, as, _ := bugWorld(t)
	answer := func(on byte) [][]byte {
		as.Queue().GetPending()
		if err := sh.handleBugReportOffer(as, incoming(protocol.OpcodeBugReportOffer, []byte{on, 0, 0, 0})); err != nil {
			t.Fatal(err)
		}
		return queued(as, protocol.OpcodeJump)
	}
	if got, want := answer(1), []byte{0x28, 0x7E, 1, 0, 0, 0}; len(got) != 1 || !bytes.Equal(got[0], want) {
		t.Fatalf("the answer brought %x, want one %x", got, want)
	}
	if got := answer(0); len(got) != 0 {
		t.Fatalf("an answer of 0 brought %x", got)
	}
	SetJumps(false)
	if got := answer(1); len(got) != 0 {
		t.Fatalf("a server without jumps offered %x", got)
	}
}

// The client's answer is its player's, on a server with jumps alone: 1 jumps, 0
// does not, a short one changes nothing.
func TestHandleJump(t *testing.T) {
	c := newChatWorld(t)
	sa, pa := c.join(1, "Alice", 100, 100, 0)
	h := NewJumpHandler(c.gw)
	if err := h.HandleJump(sa, incoming(protocol.OpcodeJump, []byte{1, 0, 0, 0})); err != nil || pa.Jumps {
		t.Fatalf("taken on a server without jumps: %v %v", pa.Jumps, err)
	}
	c.gw.SetJumps(true)
	h.HandleJump(sa, incoming(protocol.OpcodeJump, []byte{1, 0, 0}))
	if pa.Jumps {
		t.Fatal("a short answer was taken")
	}
	h.HandleJump(sa, incoming(protocol.OpcodeJump, []byte{1, 0, 0, 0}))
	if !pa.Jumps {
		t.Fatal("its 1 was not taken")
	}
	h.HandleJump(sa, incoming(protocol.OpcodeJump, []byte{0, 0, 0, 0}))
	if pa.Jumps {
		t.Fatal("its 0 was not taken")
	}
}

// A character whose client jumped, carried through a restart: on a server with
// jumps it jumps on and nothing is said; into one without them its client is
// told 0 (it would jump on, and be judged as rising in the air).
func TestAResumedJumperIsToldWhenJumpsAreGone(t *testing.T) {
	for _, after := range []bool{true, false} {
		db := keepDB(t)
		_, gw, sm := keepProcess(t, db)
		gw.SetJumps(true)
		sess := sm.CreateSession(&net.UDPAddr{IP: net.IPv4(203, 0, 113, 8), Port: 5008}, 8, 8, 8, 1, true)
		sess.SetAccountID(3)
		sess.SetPlayerObjectID(500000008)
		sess.SetState(session.StateInGame)
		sess.SetCharacterInWorld(true)
		rec, err := database.NewSQLiteCharacterStore(db).GetByServerID(7)
		if err != nil {
			t.Fatal(err)
		}
		if p := keepChars.Load().enterFromRecord(sess, rec); p == nil {
			t.Fatal("no player")
		}
		if !gw.SetJumpClient(sess, true) {
			t.Fatal("the answer not taken")
		}
		kp, _ := gw.KeepPlayer(sess, time.Now())
		b, _ := json.Marshal(struct {
			S session.Kept
			P entity.KeptPlayer
			E KeptExtras
		}{sess.Keep(), kp, KeepExtras(sess, gw.SessionPlayer(sess))})

		_, gw2, sm2 := keepProcess(t, db)
		gw2.SetJumps(after)
		var back struct {
			S session.Kept
			P entity.KeptPlayer
			E KeptExtras
		}
		if err := json.Unmarshal(b, &back); err != nil {
			t.Fatal(err)
		}
		rs, err := sm2.Resume(back.S)
		if err != nil {
			t.Fatal(err)
		}
		p, err := ResumeCharacter(rs, back.E, back.P, time.Now())
		if err != nil {
			t.Fatal(err)
		}
		told := queued(rs, protocol.OpcodeJump)
		if after {
			if !p.Jumps || len(told) != 0 {
				t.Fatalf("into a server with jumps: jumps %v, told %x", p.Jumps, told)
			}
			continue
		}
		if p.Jumps || len(told) != 1 || !bytes.Equal(told[0], []byte{0x28, 0x7E, 0, 0, 0, 0}) {
			t.Fatalf("into a server without jumps: jumps %v, told %x", p.Jumps, told)
		}
	}
}
