package handlers

import (
	"os"
	"slices"
	"testing"
	"time"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
)

// The Transformative Pill (itempattern 709301) of Create Transformative
// (700772), the Transmuter's Creative Transmutation (cmcatalog 7402, level 60).
const realTransformativePill = 709301

// On a real database (EQOA_REAL_DB, a copy): an alchemist merc keeps one
// Transformative Pill for its owner only as a Transmuter of 60 (the rank's
// level), of either path and any race; a Transmuter of 55 or 59, a Metabolist
// of 60 and one with no Master Class of 60 (whose book has Create
// Transformative by classspells) none. Its other stacks are as before.
func TestRealMercTransmuterPill(t *testing.T) {
	path := os.Getenv("EQOA_REAL_DB")
	if path == "" {
		t.Skip("EQOA_REAL_DB unset")
	}
	db, err := database.NewSQLite(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	gw := entity.NewWorld()
	if _, err := gw.LoadCMCatalog(db); err != nil {
		t.Fatal(err)
	}
	eh := NewEquipHandler(nil, database.NewInventoryStore(db), nil, nil)
	eh.gameWorld = gw
	eh.LoadPotions()
	m := NewMercs(gw, db, NewHeirlooms(db))
	for _, race := range []int{0, 3, 9} {
		for _, path := range []int{0, 1} {
			for _, tc := range []struct {
				master string
				level  int
				want   bool
			}{{"", 60, false}, {"Metabolist", 60, false}, {"Transmuter", 55, false}, {"Transmuter", 59, false}, {"Transmuter", 60, true}} {
				k, err := m.kit(database.MercRow{Name: "Real", Class: 14, Race: race, Path: path, Master: tc.master}, tc.level)
				if err != nil {
					t.Fatal(err)
				}
				got := gw.MercBrewsFor(k)
				if slices.Contains(got, realTransformativePill) != tc.want {
					t.Errorf("race %d path %d %q at %d keeps %v: a pill %v", race, path, tc.master, tc.level, got, tc.want)
				}
				if tc.want && got[len(got)-1] != realTransformativePill {
					t.Errorf("race %d path %d: the pill not after its stacks: %v", race, path, got)
				}
			}
		}
	}
}

// The pill as the stock keeps it: a brew of one is kept (charmercstock),
// shown in the Stock tab, read again by a new World's hire (a restart), and
// taken by its owner out of the stock into his pack, the World's copy with it
// (so the merc brews the next).
func TestMercPillInTheStock(t *testing.T) {
	r, m, db := newMercFixture(t)
	invItems(t, db)
	putItem(t, db, database.ItemPatternRecord{PatternID: realTransformativePill, ItemName: "Transformative Pill", EquipSlot: -1, LevelReq: 60, MaxStack: 9999})
	now := m.now()
	m.now = func() time.Time { return now }
	char := r.pa.CharServerID
	m.hireWith(r.anna, r.pa, 14, 0, 0, 0)
	r.relist(db)
	r.anna.SetMercWindow(1)
	m.note(entity.MercNote{Kind: entity.MercNoteBrewed, Owner: r.pa, Slot: 1, PatternID: realTransformativePill, Count: 1})
	if st, _ := db.MercStock(char); st[realTransformativePill] != 1 {
		t.Fatalf("kept %v, want the pill", st)
	}
	// a restart: a new World's hire reads it
	r2 := newPortalRig(t)
	r2.gw.SetRaceModels(map[[2]int]uint32{{0, 0}: 1893243078, {0, 1}: 223572789, {2, 0}: 2128125354, {2, 1}: 2358987577})
	heir := NewHeirlooms(db)
	heir.Start(nil, nil)
	m2 := NewMercs(r2.gw, db, heir)
	r2.ih.mercs, r2.h.mercs = m2, m2
	r2.pa.Level, r2.pa.Race, r2.pa.Class = 30, 0, 0
	m.Command(r.anna, r.pa, "dismiss")
	m2.hireWith(r2.anna, r2.pa, 14, 0, 0, 0)
	if st := r2.gw.MercStockOf(char); st[realTransformativePill] != 1 {
		t.Fatalf("after a restart the World's stock %v, want the pill", st)
	}
	// the Stock tab shows it and takes it
	m2.now = func() time.Time { return now }
	r2.relist(db)
	r2.anna.SetMercWindow(1)
	h := NewMercWindowHandler(r2.gw, m2)
	r2.drain()
	winAsk(t, h, r2.anna, 1, mercAskOpen)
	st, ok := lastMercWinState(t, r2.anna)
	if !ok || !slices.ContainsFunc(st.recs, func(x mercWinRec) bool {
		return x.kind == mercRecStock && x.item.pattern == realTransformativePill && x.item.stack == 1
	}) {
		t.Fatalf("the Stock tab: %v %+v", ok, st.recs)
	}
	winAsk(t, h, r2.anna, 1, mercAskStock, u32b(realTransformativePill))
	if packHas(t, db, char, realTransformativePill) != 1 {
		t.Fatalf("the pack holds %d pills", packHas(t, db, char, realTransformativePill))
	}
	if st, _ := db.MercStock(char); st[realTransformativePill] != 0 {
		t.Errorf("kept %v after the take", st)
	}
	if st := r2.gw.MercStockOf(char); st[realTransformativePill] != 0 {
		t.Errorf("the World's copy %v after the take", st)
	}
}
