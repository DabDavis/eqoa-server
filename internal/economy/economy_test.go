package economy

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/eqoa/server/internal/database"
)

func testDB(t *testing.T) *database.SQLiteDB {
	t.Helper()
	db, err := database.NewSQLite(filepath.Join(t.TempDir(), "eqoa.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func at(l *Ledger, ts string) {
	tm, _ := time.Parse(time.RFC3339, ts)
	l.now = func() time.Time { return tm }
}

type row struct {
	count, amount int64
	kind          string
}

func ledgerRows(t *testing.T, db *database.SQLiteDB) map[[2]any]row {
	t.Helper()
	hs, err := database.TunarHours(db, time.Unix(0, 0), time.Unix(1<<40, 0))
	if err != nil {
		t.Fatal(err)
	}
	out := map[[2]any]row{}
	for _, r := range hs {
		out[[2]any{r.Hour, r.Source}] = row{r.Count, r.Amount, r.Kind}
	}
	return out
}

// The counters: by the hour and source, a character's sides by the day; a
// transfer counted once with both sides; nothing for 0 or less.
func TestLedgerCounts(t *testing.T) {
	l := New()
	at(l, "2026-11-14T10:15:00Z")
	l.Made(Loot, 7, "Hero", 30)
	l.Made(Loot, 7, "Hero", 12)
	l.Made(Loot, 8, "Other", 0)    // nothing
	l.Spent(Repair, 7, "Hero", -5) // nothing
	l.Spent(MerchantBuy, 7, "Hero", 100)
	l.Moved(Trade, 7, "Hero", 8, "Other", 500)
	at(l, "2026-11-14T11:01:00Z")
	l.Made(Loot, 8, "Other", 4)
	hours, chars := l.Pending()
	h10 := time.Date(2026, 11, 14, 10, 0, 0, 0, time.UTC).Unix()
	got := map[[2]any]database.TunarHourRow{}
	for _, r := range hours {
		got[[2]any{r.Hour, r.Source}] = r
	}
	if r := got[[2]any{h10, "loot"}]; r.Count != 2 || r.Amount != 42 || r.Kind != "faucet" {
		t.Errorf("loot at 10: %+v", r)
	}
	if r := got[[2]any{h10 + 3600, "loot"}]; r.Count != 1 || r.Amount != 4 {
		t.Errorf("loot at 11: %+v", r)
	}
	if r := got[[2]any{h10, "trade"}]; r.Count != 1 || r.Amount != 500 || r.Kind != "transfer" {
		t.Errorf("the trade: %+v", r)
	}
	if len(hours) != 4 {
		t.Errorf("%d hour rows: %+v", len(hours), hours)
	}
	day := time.Date(2026, 11, 14, 0, 0, 0, 0, time.UTC).Unix()
	cs := map[[2]any]database.TunarCharRow{}
	for _, r := range chars {
		if r.Day != day {
			t.Errorf("day %d", r.Day)
		}
		cs[[2]any{r.ServerID, r.Source}] = r
	}
	if r := cs[[2]any{uint32(7), "trade"}]; r.Paid != 500 || r.Got != 0 || r.Count != 1 {
		t.Errorf("Hero's trade: %+v", r)
	}
	if r := cs[[2]any{uint32(8), "trade"}]; r.Got != 500 || r.Paid != 0 || r.Name != "Other" {
		t.Errorf("Other's trade: %+v", r)
	}
	if r := cs[[2]any{uint32(7), "loot"}]; r.Got != 42 || r.Count != 2 {
		t.Errorf("Hero's loot: %+v", r)
	}
	if r := cs[[2]any{uint32(7), "merchant_buy"}]; r.Paid != 100 {
		t.Errorf("Hero's buy: %+v", r)
	}
}

// A flush writes and empties; a second adds to the rows; a held ledger and an
// empty one write nothing; a failed write keeps the counts for the next.
func TestLedgerFlush(t *testing.T) {
	db := testDB(t)
	l := New()
	at(l, "2026-11-14T10:15:00Z")
	l.Made(Loot, 7, "Hero", 30)
	l.Spent(Travel, 7, "Hero", 100)
	if err := l.Flush(db); err != nil {
		t.Fatal(err)
	}
	if h, c := l.Pending(); len(h) != 0 || len(c) != 0 {
		t.Fatalf("not emptied: %v %v", h, c)
	}
	l.Made(Loot, 7, "Hero", 5)
	l.Hold(true)
	if err := l.Flush(db); err != nil {
		t.Fatal(err)
	}
	h10 := time.Date(2026, 11, 14, 10, 0, 0, 0, time.UTC).Unix()
	if r := ledgerRows(t, db)[[2]any{h10, "loot"}]; r.amount != 30 {
		t.Errorf("a held flush wrote: %+v", r)
	}
	l.Hold(false)
	if err := l.Flush(db); err != nil {
		t.Fatal(err)
	}
	rows := ledgerRows(t, db)
	if r := rows[[2]any{h10, "loot"}]; r.count != 2 || r.amount != 35 || r.kind != "faucet" {
		t.Errorf("loot: %+v", r)
	}
	if r := rows[[2]any{h10, "travel"}]; r.count != 1 || r.amount != 100 || r.kind != "sink" {
		t.Errorf("travel: %+v", r)
	}
	cs, _ := database.TunarChars(db, time.Unix(0, 0), time.Unix(1<<40, 0))
	if len(cs) != 2 || cs[0].Got+cs[1].Got != 35 {
		t.Errorf("characters: %+v", cs)
	}
	// a failure: the counts stay, and reach the next database written
	l.Made(Pickpocket, 9, "Rogue", 7)
	db.Close()
	if err := l.Flush(db); err == nil {
		t.Fatal("a closed database took the write")
	}
	l.Made(Pickpocket, 9, "Rogue", 3)
	db2 := testDB(t)
	if err := l.Flush(db2); err != nil {
		t.Fatal(err)
	}
	if r := ledgerRows(t, db2)[[2]any{h10, "pickpocket"}]; r.count != 2 || r.amount != 10 {
		t.Errorf("kept over the failure: %+v", r)
	}
}

// Every source has a kind and a label, each once.
func TestSourcesListed(t *testing.T) {
	seen := map[Source]bool{}
	for _, i := range Sources {
		if seen[i.Source] || i.Label == "" || i.Phrase == "" || (i.Kind != Faucet && i.Kind != Sink && i.Kind != Transfer) {
			t.Errorf("%+v", i)
		}
		seen[i.Source] = true
	}
	if KindOf(AuctionHouse) != Sink || KindOf(DuelWager) != Transfer || KindOf(StartingPurse) != Faucet {
		t.Error("kinds")
	}
}

// The wealth: purses, banks and escrow; a GM's characters apart from the spread.
func TestMeasureWealth(t *testing.T) {
	db := testDB(t)
	for _, q := range []string{
		`INSERT INTO accountinfo (accountid, username, acctstatus, acctlevel) VALUES (1, 'p', 1, 0), (2, 'g', 1, 1)`,
		`INSERT INTO characters (charName, accountid, serverid, modelid, tclass, race, humType, level, haircolor, hairlength,
			hairstyle, faceoption, sex, world, x, y, z, tunar, bankTunar) VALUES
			('A', 1, 1, 1, 0, 0, 1, 10, 0, 0, 0, 0, 0, 0, 0, 0, 0, 100, 0),
			('B', 1, 2, 1, 0, 0, 1, 20, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1000, 2000000),
			('C', 1, 3, 1, 0, 0, 1, 30, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0),
			('Gm', 2, 4, 1, 0, 0, 1, 60, 0, 0, 0, 0, 0, 0, 0, 0, 0, 5000000, 0)`,
		`INSERT INTO auction_bids (lot, bidder, bidder_name, max_bid, escrow, placed_at) VALUES (1, 1, 'A', 50, 50, 0)`,
	} {
		if _, err := db.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	l := New()
	at(l, "2026-11-14T10:15:00Z")
	if err := l.Measure(db, 1, false); err != nil {
		t.Fatal(err)
	}
	ws, err := database.TunarWealthSince(db, time.Unix(0, 0))
	if err != nil || len(ws) != 1 {
		t.Fatalf("%v %v", ws, err)
	}
	w := ws[0]
	if w.Characters != 3 || w.Holders != 2 || w.Purse != 1100 || w.Bank != 2000000 || w.Escrow != 50 || w.Staff != 5000000 {
		t.Errorf("%+v", w)
	}
	if w.Median != 100 || w.Over1M != 1 || w.Over100k != 1 || w.Over10M != 0 || w.Top10 != 2001100 {
		t.Errorf("the spread: %+v", w)
	}
	if len(w.Top) != 4 || w.Top[0].Name != "Gm" || !w.Top[0].Staff || w.Top[1].Name != "B" {
		t.Errorf("the top: %+v", w.Top)
	}
	if w.Supply() != 7001150 {
		t.Errorf("supply %d", w.Supply())
	}
	// once an hour
	db.Exec(`UPDATE characters SET tunar = 0`)
	if err := l.Measure(db, 1, false); err != nil {
		t.Fatal(err)
	}
	if ws, _ := database.TunarWealthSince(db, time.Unix(0, 0)); ws[0].Purse != 1100 {
		t.Error("measured twice in the hour")
	}
}
