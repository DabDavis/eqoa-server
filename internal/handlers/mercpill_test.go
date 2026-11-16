package handlers

import (
	"os"
	"slices"
	"testing"
	"time"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
)

// The alchemist's Master Class keepsakes (entity's mercKeepsLocked): the
// Transmuter's Transformative Pill (itempattern 709301, Create Transformative
// 700772, Creative Transmutation, cmcatalog 7402, level 60) and Transmutive
// Potion (50106, Transmutive Mixture 700827, Transmutative Mixture 7403, 60);
// the Metabolist's Miraculous Draught (1203, Miraculous Mixture 700802,
// Miraculous Mix 7412, 60).
const (
	realTransformativePill = 709301
	realTransmutivePotion  = 50106
	realMiraculousDraught  = 1203
)

var realKeepsakes = map[string][]int{
	"Transmuter": {realTransformativePill, realTransmutivePotion},
	"Metabolist": {realMiraculousDraught},
}

// On a real database (EQOA_REAL_DB, a copy): an alchemist merc keeps one of
// each of its Master Class's potions for its owner only at 60 (the ranks'
// level), of either path and any race: a Transmuter its pill and Transmutive
// Potion, a Metabolist its Miraculous Draught; one of 55 or 59, and one with
// no Master Class of 60 (whose book has the mixtures by classspells), none.
// Its other stacks are as before, the keepsakes after them.
func TestRealMercMasterKeepsakes(t *testing.T) {
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
	all := []int{realTransformativePill, realTransmutivePotion, realMiraculousDraught}
	for _, race := range []int{0, 3, 9} {
		for _, path := range []int{0, 1} {
			plain, err := m.kit(database.MercRow{Name: "Real", Class: 14, Race: race, Path: path}, 60)
			if err != nil {
				t.Fatal(err)
			}
			base := gw.MercBrewsFor(plain)
			for _, id := range all {
				if slices.Contains(base, id) {
					t.Errorf("race %d path %d: one with no Master Class keeps %d: %v", race, path, id, base)
				}
			}
			for _, master := range []string{"Transmuter", "Metabolist"} {
				for _, level := range []int{55, 59, 60} {
					k, err := m.kit(database.MercRow{Name: "Real", Class: 14, Race: race, Path: path, Master: master}, level)
					if err != nil {
						t.Fatal(err)
					}
					got := gw.MercBrewsFor(k)
					var want []int
					if level == 60 {
						want = realKeepsakes[master]
					}
					var kept []int
					for _, id := range got {
						if slices.Contains(all, id) {
							kept = append(kept, id)
						}
					}
					if !slices.Equal(kept, want) {
						t.Errorf("race %d path %d %s of %d keeps %v (of %v), want %v", race, path, master, level, kept, got, want)
					}
					if len(want) > 0 && !slices.Equal(got[len(got)-len(want):], want) {
						t.Errorf("race %d path %d %s: its keepsakes not after its stacks: %v", race, path, master, got)
					}
				}
			}
		}
	}
}

// The keepsakes as the stock keeps them: a brew of one of each is kept
// (charmercstock), shown in the Stock tab, read again by a new World's hire
// (a restart), and taken by its owner out of the stock into his pack, the
// World's copy with it (so the merc brews the next).
func TestMercKeepsakesInTheStock(t *testing.T) {
	r, m, db := newMercFixture(t)
	invItems(t, db)
	names := map[int]string{realTransformativePill: "Transformative Pill", realTransmutivePotion: "Transmutive Potion", realMiraculousDraught: "Miraculous Draught"}
	ids := []int{realTransformativePill, realTransmutivePotion, realMiraculousDraught}
	for _, id := range ids {
		putItem(t, db, database.ItemPatternRecord{PatternID: id, ItemName: names[id], EquipSlot: -1, LevelReq: 1, MaxStack: 9999})
	}
	now := m.now()
	m.now = func() time.Time { return now }
	char := r.pa.CharServerID
	m.hireWith(r.anna, r.pa, 14, 0, 0, 0)
	for _, id := range ids {
		m.note(entity.MercNote{Kind: entity.MercNoteBrewed, Owner: r.pa, Slot: 1, PatternID: id, Count: 1})
	}
	if st, _ := db.MercStock(char); st[ids[0]] != 1 || st[ids[1]] != 1 || st[ids[2]] != 1 {
		t.Fatalf("kept %v, want one of each", st)
	}
	// a restart: a new World's hire reads them
	r2 := newPortalRig(t)
	r2.gw.SetRaceModels(map[[2]int]uint32{{0, 0}: 1893243078, {0, 1}: 223572789, {2, 0}: 2128125354, {2, 1}: 2358987577})
	heir := NewHeirlooms(db)
	heir.Start(nil, nil)
	m2 := NewMercs(r2.gw, db, heir)
	r2.ih.mercs, r2.h.mercs = m2, m2
	r2.pa.Level, r2.pa.Race, r2.pa.Class = 30, 0, 0
	m.Command(r.anna, r.pa, "dismiss")
	m2.hireWith(r2.anna, r2.pa, 14, 0, 0, 0)
	if st := r2.gw.MercStockOf(char); st[ids[0]] != 1 || st[ids[1]] != 1 || st[ids[2]] != 1 {
		t.Fatalf("after a restart the World's stock %v, want one of each", st)
	}
	// the Stock tab shows each and takes each
	m2.now = func() time.Time { return now }
	r2.relist(db)
	r2.anna.SetMercWindow(1)
	h := NewMercWindowHandler(r2.gw, m2)
	r2.drain()
	winAsk(t, h, r2.anna, 1, mercAskOpen)
	st, ok := lastMercWinState(t, r2.anna)
	for _, id := range ids {
		if !ok || !slices.ContainsFunc(st.recs, func(x mercWinRec) bool {
			return x.kind == mercRecStock && x.item.pattern == int64(id) && x.item.stack == 1
		}) {
			t.Fatalf("the Stock tab lacks one %s: %v %+v", names[id], ok, st.recs)
		}
	}
	for _, id := range ids {
		winAsk(t, h, r2.anna, 1, mercAskStock, u32b(uint32(id)))
		if packHas(t, db, char, id) != 1 {
			t.Fatalf("the pack holds %d of %s", packHas(t, db, char, id), names[id])
		}
		if st, _ := db.MercStock(char); st[id] != 0 {
			t.Errorf("kept %v after taking the %s", st, names[id])
		}
		if st := r2.gw.MercStockOf(char); st[id] != 0 {
			t.Errorf("the World's copy %v after taking the %s", st, names[id])
		}
	}
}
