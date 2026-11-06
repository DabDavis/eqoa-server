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

// A worn item is traded (the user, 2026-11-06: "lets allow equipped to be
// banked, traded"): offered, and as the trade completes taken off first (the
// giver told 0x0040 its slot), then moved, arriving unworn. The control: a
// window handler wired without the take-off refuses the worn item's offer.
func TestAWornItemIsTraded(t *testing.T) {
	run := func(takeOff bool) (sword int, giverSess *session.Session, inv *database.InventoryStore, h *TradeHandler) {
		db, err := database.NewSQLite(t.TempDir() + "/trade.db")
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { db.Close() })
		inv = database.NewInventoryStore(db)
		sword = 900100
		if err := database.NewItemPatternStore(db).UpsertItemPattern(&database.ItemPatternRecord{PatternID: sword, ItemName: "keen sword",
			EquipSlot: 14, MaxHP: 100, MaxStack: 1, ClassUse: 32767, RaceUse: 32767, Rent: 1, WeaponDamage: 50, AttackType: 1}); err != nil {
			t.Fatal(err)
		}
		id, err := inv.AddCharItem(11, sword, 1, 100, 0, 0)
		if err != nil {
			t.Fatal(err)
		}
		if err := inv.SetEquipLoc(11, id, 14); err != nil {
			t.Fatal(err)
		}
		w := entity.NewWorld()
		sm := session.NewManager()
		w.SessionManager = sm
		mk := func(port int, account int) *session.Session {
			s := sm.CreateSession(&net.UDPAddr{IP: net.IPv4(198, 51, 100, 21), Port: port}, uint32(port), uint32(port), uint16(port), 2, true)
			s.SetAccountID(account)
			s.SetInGame(true)
			s.SetCharacterInWorld(true)
			s.UpdateActivity()
			return s
		}
		aSess, bSess := mk(7101, 1), mk(7102, 2)
		w.CreatePlayer(aSess, &database.CharacterRecord{ServerID: 11, CharName: "Giver", AccountID: 1, Level: 10, X: 100, Y: 54, Z: 100})
		taker := w.CreatePlayer(bSess, &database.CharacterRecord{ServerID: 12, CharName: "Taker", AccountID: 2, Level: 10, X: 102, Y: 54, Z: 100})
		for _, s := range []struct {
			sess *session.Session
			sid  uint32
		}{{aSess, 11}, {bSess, 12}} {
			rows, err := inv.GetCharacterItems(s.sid)
			if err != nil {
				t.Fatal(err)
			}
			s.sess.SetInventoryList(newInvList(rows))
		}
		items := &NPCWindowHandler{inv: inv}
		if takeOff {
			items.takeOff = NewEquipHandler(nil, inv, nil, nil).TakeOff
		}
		h = NewTradeHandler(sm, items)
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
		ui(bSess, 2, nil)                                                                     // yes
		ui(aSess, 3, func(w *buffer.Writer) { w.WriteUint32(0); w.Write7BitEncodedInt64(1) }) // the worn sword
		ui(aSess, 6, nil)
		ui(bSess, 6, nil)
		return sword, aSess, inv, h
	}

	sword, aSess, inv, _ := run(true)
	took := false
	for _, m := range aSess.Queue().PeekPending() {
		if d := m.Data(); len(d) >= 3 && protocol.GameOpcode(uint16(d[0])|uint16(d[1])<<8) == protocol.OpcodeUnequipItem && d[2] == 14 {
			took = true
		}
	}
	if !took {
		t.Error("the giver was not told the sword came off (0x0040 14)")
	}
	mine, _ := inv.GetCharacterItems(11)
	theirs, _ := inv.GetCharacterItems(12)
	if len(mine) != 0 || len(theirs) != 1 || theirs[0].Pattern.PatternID != sword || theirs[0].EquipLoc != -1 {
		t.Fatalf("after the trade: the giver holds %d, the taker %+v", len(mine), theirs)
	}

	// the control: no take-off, the worn sword stays its wearer's
	_, _, inv, _ = run(false)
	if mine, _ := inv.GetCharacterItems(11); len(mine) != 1 || mine[0].EquipLoc != 14 {
		t.Fatalf("the control: the giver holds %+v", mine)
	}
}
