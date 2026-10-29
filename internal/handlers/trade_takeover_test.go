package handlers

import (
	"net"
	"testing"

	"github.com/eqoa/pkg/pkg/buffer"
	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// A trade whose side's character is taken over by another window
// (entity/takeover.go) is called off at the other side's next move, which is
// told it was cancelled: the accept the dropped client gave would otherwise
// move the live character's tunar behind its new client's back (and its
// items, off the old client's inventory list). The control: the same trade
// with no takeover completes, the money moving.
func TestATradeEndsWithATakeover(t *testing.T) {
	run := func(takeOver bool) (giver, taker *entity.Player, bSess *session.Session, h *TradeHandler) {
		w := entity.NewWorld()
		sm := session.NewManager()
		w.SessionManager = sm
		mk := func(port int, account int) *session.Session {
			s := sm.CreateSession(&net.UDPAddr{IP: net.IPv4(198, 51, 100, 20), Port: port}, uint32(port), uint32(port), uint16(port), 2, true)
			s.SetAccountID(account)
			s.SetInGame(true)
			s.SetCharacterInWorld(true)
			s.UpdateActivity()
			return s
		}
		aSess, bSess := mk(7001, 1), mk(7002, 2)
		giver = w.CreatePlayer(aSess, &database.CharacterRecord{ServerID: 11, CharName: "Giver", AccountID: 1, Level: 10, X: 100, Y: 54, Z: 100})
		taker = w.CreatePlayer(bSess, &database.CharacterRecord{ServerID: 12, CharName: "Taker", AccountID: 2, Level: 10, X: 102, Y: 54, Z: 100})
		giver.Tunar, taker.Tunar = 500, 0
		h = NewTradeHandler(sm, &NPCWindowHandler{})
		h.SetGameWorld(w)
		ui := func(s *session.Session, sub byte, fill func(w *buffer.Writer)) {
			t.Helper()
			msg := protocol.NewMessage(protocol.MessageTypeReliable, protocol.OpcodeTradeUI)
			b := make([]byte, 64)
			wr := buffer.NewWriter(b)
			wr.WriteByte(sub)
			wr.WriteUint32(0)
			if fill != nil {
				fill(wr)
			}
			msg.SetData(b[:wr.Position()])
			if err := h.HandleTradeUI(s, msg); err != nil {
				t.Fatal(err)
			}
		}
		ask := protocol.NewMessage(protocol.MessageTypeReliable, protocol.OpcodeTradeRequest)
		ask.SetData(u32b(0x7F000000 | uint32(taker.ServerID)))
		if err := h.HandleTradeRequest(aSess, ask); err != nil {
			t.Fatal(err)
		}
		ui(bSess, 2, nil)                                                     // yes
		ui(aSess, 5, func(w *buffer.Writer) { w.Write7BitEncodedInt64(100) }) // 100 tunar
		ui(aSess, 6, nil)                                                     // accepted, and the client drops
		if takeOver {
			later := mk(7003, 1)
			if p, _ := w.TakeOverPlayer(later, 11); p != giver {
				t.Fatal("setup: not taken over")
			}
		}
		ui(bSess, 6, nil) // the other side accepts
		return giver, taker, bSess, h
	}

	giver, taker, bSess, h := run(true)
	if giver.Tunar != 500 || taker.Tunar != 0 {
		t.Fatalf("the taken-over character's trade went through: %d and %d tunar", giver.Tunar, taker.Tunar)
	}
	if len(h.trades) != 0 {
		t.Fatalf("%d trades left open", len(h.trades))
	}
	cancelled := false
	for _, m := range bSess.Queue().PeekPending() {
		if d := m.Data(); len(d) >= 3 && opcode(d) == protocol.OpcodeTradeUI && d[2] == 1 {
			cancelled = true
		}
	}
	if !cancelled {
		t.Fatal("the other side was not told the trade was off")
	}

	// the control
	giver, taker, _, h = run(false)
	if giver.Tunar != 400 || taker.Tunar != 100 || len(h.trades) != 0 {
		t.Fatalf("the control: a trade with no takeover did not complete (%d and %d tunar, %d open)", giver.Tunar, taker.Tunar, len(h.trades))
	}
}
