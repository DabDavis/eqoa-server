package handlers

import (
	"bytes"
	"testing"

	"github.com/eqoa/pkg/pkg/buffer"
	"github.com/eqoa/server/internal/protocol"
)

// Work board 880: two merchants sell the same leather strip (value 50); one has
// a merchantprice row of 40, the other none. The first's window carries 40 and
// its buy charges 40; the second's carries and charges the record's 50, as
// before (the negative control: with the row gone the first charges 50 too).
func TestAMerchantChargesItsOwnPrice(t *testing.T) {
	f := newShopFixture(t)
	strip := f.pattern("Leather Strip", 50, 20, 100, 21)
	f.catalog(strip, 1, 100)
	if _, err := f.db.Exec(`INSERT INTO merchantprice (merchant, patternid, price, source) VALUES ('tailor nissa', ?, 40, 'video-audit')`, strip); err != nil {
		t.Fatal(err)
	}
	npc := f.npcs[shopMerchantID]
	open := func(name string) (int, []byte) {
		t.Helper()
		npc.Name = name
		l := f.h.merchantList(f.player, npc, false)
		if len(l.wares) != 1 {
			t.Fatalf("%s lists %d wares", name, len(l.wares))
		}
		f.send(protocol.OpcodeMerchantDiag, func(w *buffer.Writer) { w.WriteUint32(shopMerchantID) })
		msgs := f.sent()
		mustOps(t, msgs, protocol.OpcodeMerchantBox)
		return l.wares[0].Pattern.ItemCost, msgs[0]
	}
	buy := func() int {
		t.Helper()
		before := f.player.Tunar
		f.send(protocol.OpcodeMerchantBuy, func(w *buffer.Writer) {
			w.Write7BitEncodedInt64(0)
			w.Write7BitEncodedInt64(1)
			w.WriteUint32(shopMerchantID)
		})
		mustOps(t, f.sent(), protocol.OpcodeAddInvItem, protocol.OpcodePlayerTunar)
		return before - f.player.Tunar
	}

	v, nissaBox := open("Tailor Nissa")
	if v != 40 {
		t.Fatalf("Tailor Nissa's window carries %d, want her 40", v)
	}
	if paid := buy(); paid != 40 {
		t.Fatalf("Tailor Nissa charged %d, want 40", paid)
	}
	v, efreyaBox := open("Merchant Efreya")
	if v != 50 {
		t.Fatalf("Merchant Efreya's window carries %d, want the record's 50", v)
	}
	if bytes.Equal(nissaBox, efreyaBox) {
		t.Fatal("the two windows' 0x00B7 are the same: the price is not in the record sent")
	}
	if paid := buy(); paid != 50 {
		t.Fatalf("Merchant Efreya charged %d, want 50", paid)
	}
	if rec, ok, _ := f.inv.ItemPatternByID(strip); !ok || rec.ItemCost != 50 {
		t.Fatalf("the shared record's value became %d", rec.ItemCost)
	}

	// the control: the row gone, Tailor Nissa charges the record's value
	if _, err := f.db.Exec(`DELETE FROM merchantprice`); err != nil {
		t.Fatal(err)
	}
	open("Tailor Nissa")
	if paid := buy(); paid != 50 {
		t.Fatalf("with no row Tailor Nissa charged %d, want 50", paid)
	}
}
