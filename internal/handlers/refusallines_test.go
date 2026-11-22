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

// A trade asked of a player twice InteractRange away is refused with the live
// game's red line and the other player is not asked (no 0x0057); the
// control: within reach the other player is asked, and no line is said.
func TestATradeFromOutOfReach(t *testing.T) {
	ask := func(dx float32) (asker, other *session.Session) {
		w := entity.NewWorld()
		sm := session.NewManager()
		w.SessionManager = sm
		mk := func(port int) *session.Session {
			s := sm.CreateSession(&net.UDPAddr{IP: net.IPv4(198, 51, 100, 21), Port: port}, uint32(port), uint32(port), uint16(port), 2, true)
			s.SetAccountID(port)
			s.SetInGame(true)
			s.SetCharacterInWorld(true)
			s.UpdateActivity()
			return s
		}
		asker, other = mk(7101), mk(7102)
		w.CreatePlayer(asker, &database.CharacterRecord{ServerID: 21, CharName: "Asker", AccountID: 7101, Level: 10, X: 100, Y: 54, Z: 100})
		them := w.CreatePlayer(other, &database.CharacterRecord{ServerID: 22, CharName: "Other", AccountID: 7102, Level: 10, X: 100 + dx, Y: 54, Z: 100})
		h := NewTradeHandler(sm, &NPCWindowHandler{})
		h.SetGameWorld(w)
		msg := protocol.NewMessage(protocol.MessageTypeReliable, protocol.OpcodeTradeRequest)
		msg.SetData(u32b(0x7F000000 | uint32(them.ServerID)))
		if err := h.HandleTradeRequest(asker, msg); err != nil {
			t.Fatal(err)
		}
		return asker, other
	}
	asker, other := ask(2 * entity.InteractRange)
	if got := chatLines(asker, protocol.OpcodeColoredChat); len(got) != 1 || got[0] != "You can not trade with your target from there." {
		t.Errorf("out of reach: the asker was told %q", got)
	}
	if n := len(queued(other, protocol.OpcodeTradeRequest)); n != 0 {
		t.Errorf("out of reach: the other player was asked (%d 0x0057)", n)
	}
	asker, other = ask(2)
	if got := chatLines(asker, protocol.OpcodeColoredChat); len(got) != 0 {
		t.Errorf("in reach: the asker was told %q", got)
	}
	if n := len(queued(other, protocol.OpcodeTradeRequest)); n != 1 {
		t.Errorf("in reach: the other player got %d 0x0057, want 1", n)
	}
}

// A purchase the purse cannot cover says "You can't afford that." in red and
// buys nothing; the control: with the tunar it is bought and nothing is said.
func TestAPurchaseYouCantAfford(t *testing.T) {
	f := newShopFixture(t)
	f.catalog(f.sword, 1, 100)
	f.loadLists()
	buy := func() {
		f.send(protocol.OpcodeMerchantDiag, func(w *buffer.Writer) { w.WriteUint32(shopMerchantID) })
		f.sess.Queue().GetPending()
		f.send(protocol.OpcodeMerchantBuy, func(w *buffer.Writer) {
			w.Write7BitEncodedInt64(0)
			w.Write7BitEncodedInt64(1)
			w.WriteUint32(shopMerchantID)
		})
	}
	f.player.Tunar = 1
	buy()
	if got := chatLines(f.sess, protocol.OpcodeColoredChat); len(got) != 1 || got[0] != "You can't afford that." {
		t.Errorf("short of tunar: told %q", got)
	}
	if n := len(queued(f.sess, protocol.OpcodeAddInvItem)); n != 0 || f.player.Tunar != 1 {
		t.Errorf("short of tunar: %d items given, purse %d", n, f.player.Tunar)
	}
	f.sess.Queue().GetPending()
	f.player.Tunar = 100000
	buy()
	if got := chatLines(f.sess, protocol.OpcodeColoredChat); len(got) != 0 {
		t.Errorf("with the tunar: told %q", got)
	}
	if n := len(queued(f.sess, protocol.OpcodeAddInvItem)); n != 1 {
		t.Errorf("with the tunar: %d items given, want 1", n)
	}
}
