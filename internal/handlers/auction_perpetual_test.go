package handlers

import (
	"strings"
	"testing"

	"github.com/eqoa/pkg/pkg/buffer"
	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/protocol"
)

// The house's standing lots over the wire (auction_perpetual.go): found by a
// search under the seller "ilovetesters" with 72 hours left, bought by
// "Acquire now" (a bid at the price) into the bank, the lot still up after;
// with world.perpetual_auctions off, neither shown nor sold.
func TestAuctionPerpetualOverTheWire(t *testing.T) {
	f := newHouseFixture(t, "Buyer")
	c := f.chars["Buyer"]
	res, err := f.db.Exec(`INSERT INTO auction_lots (seller, seller_name, patternid, stack, remainhp, charges,
		min_bid, buyout, fee, listed_at, ends_at, per_item, perpetual)
		VALUES (0, 'ilovetesters', ?, 1, 100, 0, 100, 100, 0, ?, ?, 1, 1)`,
		f.gem, f.now.Unix(), database.AuctionPerpetualEnds)
	if err != nil {
		t.Fatal(err)
	}
	id, _ := res.LastInsertId()
	lot := int(id)
	search := func() wirePage {
		t.Helper()
		q := auctionQuery{seller: "ilovetesters", classes: 0xFFFFFFFF, races: 0xFFFFFFFF, minLevel: 1, maxLevel: 60,
			maxPrice: 999999999, wear: -2, attack: -1, open: true}
		f.request(c, 7, func(w *buffer.Writer) {
			w.Write7BitEncodedInt64(0)
			w.Write7BitEncodedInt64(-1)
			w.WriteByte(1)
			w.WriteByte(q.flags)
			writeString8(w, q.seller, 24)
			writeWideString(w, q.text, 128)
			w.WriteUint32(q.classes)
			w.WriteUint32(q.races)
			for _, v := range []int{q.minLevel, q.maxLevel, q.minPrice, q.maxPrice, q.wear, q.attack} {
				w.Write7BitEncodedInt64(int64(v))
			}
			w.WriteByte(1)
		})
		d, ok := findSent(f.drain(c), protocol.OpcodeAuctionStuff, 129)
		if !ok {
			t.Fatal("no page")
		}
		return readPage(t, d)
	}

	SetPerpetualAuctions(false)
	if p := search(); len(p.lots) != 0 {
		t.Fatalf("gate off: %d lots shown", len(p.lots))
	}
	f.bid(c, lot, 100)
	if ls := sentLines(f.drain(c)); len(ls) != 1 || !strings.Contains(ls[0], "closed auction") {
		t.Fatalf("gate off, a bid: %q", ls)
	}
	if f.dbBank(c) != 1000 {
		t.Fatalf("gate off, the bank %d", f.dbBank(c))
	}

	SetPerpetualAuctions(true)
	t.Cleanup(func() { SetPerpetualAuctions(false) })
	p := search()
	if len(p.lots) != 1 || p.lots[0].id != int64(lot) || p.lots[0].seller != "ilovetesters" || p.lots[0].buyout != 100 {
		t.Fatalf("page: %+v", p.lots)
	}
	if left := p.lots[0].end - p.minutes; left != database.AuctionMaxMinutes {
		t.Errorf("shown %d minutes left, want %d", left, database.AuctionMaxMinutes)
	}
	if p.cols[0][0] != 0 || p.cols[1][0] != -1 || p.cols[2][0] != 100 {
		t.Errorf("columns %v", p.cols)
	}
	for i := 1; i <= 2; i++ {
		f.bid(c, lot, 100)
		msgs := f.drain(c)
		if !sentClears(msgs) {
			t.Errorf("buy %d: nothing ends the bank's wait", i)
		}
		if n, _ := bankTunarOf(t, msgs); n != int64(1000-100*i) || c.player.BankTunar != 1000-100*i {
			t.Errorf("buy %d: bank %d / %d", i, n, c.player.BankTunar)
		}
		if _, ok := findSent(msgs, protocol.OpcodeAddBankItem, 0); !ok {
			t.Errorf("buy %d: no 0x1251", i)
		}
		if ls := sentLines(msgs); len(ls) != 1 || !strings.Contains(ls[0], "You bought Rough Gem from ilovetesters for 100") {
			t.Errorf("buy %d: lines %q", i, ls)
		}
	}
	if rows := f.bankRows(c); len(rows) != 1 || rows[0].StackLeft != 2 || rows[0].Pattern.PatternID != f.gem {
		t.Errorf("bank rows %+v", rows)
	}
	if got := sessionBankList(c.sess); got == nil {
		t.Error("no bank list copy")
	}
	// still up, as it was, after the sweep
	f.house.Sweep()
	if p := search(); len(p.lots) != 1 || p.lots[0].id != int64(lot) {
		t.Errorf("after the buys and a sweep: %+v", p.lots)
	}
	if problems, _ := f.db.AuctionAudit(); len(problems) != 0 {
		t.Errorf("audit: %v", problems)
	}
}
