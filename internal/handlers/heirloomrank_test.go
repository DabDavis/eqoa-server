package handlers

import (
	"strings"
	"testing"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
)

// An Heirloom Rank raises the character's heirloom cap one level a rank past 49: at 60, rank 0 shows
// the best helm of 49, rank 3 the best of 52 that no raid gives (a boss table's helm of 51 and a
// record of 965000+ left out), and adds the AC table's figure to an armour piece, never a weapon.
// The control: with the marks off every rank reads 0.
func TestHeirloomRankRaisesTheCap(t *testing.T) {
	f := newHeirloomFixture(t)
	n := func(v int) *int { return &v }
	items := database.NewItemPatternStore(f.db)
	for _, r := range []database.ItemPatternRecord{
		{PatternID: 900049, ItemName: "helm 49", EquipSlot: 1, LevelReq: 49, AC: n(100)},
		{PatternID: 900051, ItemName: "boss helm 51", EquipSlot: 1, LevelReq: 51, AC: n(400)},
		{PatternID: 900052, ItemName: "helm 52", EquipSlot: 1, LevelReq: 52, AC: n(130)},
		{PatternID: 965500, ItemName: "raid range helm", EquipSlot: 1, LevelReq: 50, AC: n(500)},
		{PatternID: 900055, ItemName: "helm 55", EquipSlot: 1, LevelReq: 55, AC: n(160)},
	} {
		r.ClassUse, r.Rent, r.RaceUse, r.MaxStack = 1, 1, 32767, 1
		if err := items.UpsertItemPattern(&r); err != nil {
			t.Fatal(err)
		}
	}
	for _, q := range []string{
		`INSERT INTO loottables (tablename, patternid) VALUES ('boss nagafen', 900051)`,
		`INSERT INTO heirloomrankac (class, rank, ac) VALUES (0, 3, 101)`,
		`INSERT INTO charmarks (serverid, heirloom_rank) VALUES (?, 3)`,
	} {
		if _, err := f.db.Exec(q, f.sid); err != nil {
			t.Fatal(err)
		}
	}
	f.h.Note(f.sid, 0, 0, 60)
	got := f.held() // marks off: rank 0
	if h := got[hlHelm]; val(h.AC) != 100 {
		t.Fatalf("marks off at 60: helm AC %d, want 100 (the helm of 49)", val(h.AC))
	}
	SetHeirloomMarks(true)
	t.Cleanup(func() { SetHeirloomMarks(false) })
	f.h.ranks = nil
	got = f.held()
	if h := got[hlHelm]; val(h.AC) != 130+101 {
		t.Fatalf("rank 3 at 60: helm AC %d, want the helm of 52's 130 + 101", val(h.AC))
	}
	if b := got[hlBlade]; val(b.AC) != 0 {
		t.Errorf("the blade took rank AC: %d", val(b.AC))
	}
	if h := got[hlHelm]; val(h.Str) != heirloomStatTop {
		t.Errorf("the stats' share past 49: STR %d, want %d", val(h.Str), heirloomStatTop)
	}
	// a level 50 with rank 3: the cap is its level (the helm of 49 the best it may wear)
	f.h.Note(f.sid, 0, 0, 50)
	if h := f.held()[hlHelm]; val(h.AC) != 100+101 {
		t.Errorf("level 50 at rank 3: %d", val(h.AC))
	}
	// the description says the figures' level and the rank (Heirloom petitions 2910/2911): the record's
	// Level 1, its level to wear it, is left as it is
	got = f.held()
	if h := got[hlHelm]; !strings.HasSuffix(h.ItemDesc, "Now of level 50. Heirloom Rank 3: up to level 52, AC +101.") || h.LevelReq != 1 {
		t.Errorf("rank 3 at 50: helm %q level %d", h.ItemDesc, h.LevelReq)
	}
	if b := got[hlBlade]; !strings.HasSuffix(b.ItemDesc, "Now of level 50. Heirloom Rank 3: up to level 52.") {
		t.Errorf("rank 3 at 50: blade %q", b.ItemDesc)
	}
	if _, ok := entity.WeaponProcOf(hlBlade, got[hlBlade].ItemName, got[hlBlade].ItemDesc); ok {
		t.Errorf("the blade's words read as a proc: %q", got[hlBlade].ItemDesc)
	}
}

// The description's words: the control is rank 0 (the level alone); none of them is read as a weapon's
// proc, the longest description is cut to the wire's most.
func TestHeirloomDescSaysTheLevel(t *testing.T) {
	for _, c := range []struct {
		desc            string
		level, rank, ac int
		want            string
	}{
		{"Heirloom gear.", 20, 0, 0, "Heirloom gear. Now of level 20."},
		{"Heirloom gear.", 20, 1, 50, "Heirloom gear. Now of level 20. Heirloom Rank 1: up to level 50, AC +50."},
		{"Heirloom gear. ", 50, 1, 0, "Heirloom gear. Now of level 50. Heirloom Rank 1: up to level 50."},
		{"", 0, 0, 0, "Now of level 1."},
	} {
		got := heirloomDesc(c.desc, c.level, c.rank, c.ac)
		if got != c.want {
			t.Errorf("heirloomDesc(%q, %d, %d, %d) = %q, want %q", c.desc, c.level, c.rank, c.ac, got, c.want)
		}
		if _, ok := entity.WeaponProcFromText(got); ok {
			t.Errorf("%q reads as a proc", got)
		}
	}
	if got := heirloomDesc(strings.Repeat("x", 300), 60, 6, 213); len([]rune(got)) != itemDescMax {
		t.Errorf("a long description: %d runes", len([]rune(got)))
	}
}

// The Keeper's words at a rank bought: the AC at once where the class has a row, and the level the heirlooms
// grow to (a level 20) or are now (a level 60).
func TestKeeperRankWords(t *testing.T) {
	if got := keeperRankWords(20, 1, 50); !strings.HasPrefix(got, "Your heirloom armour gains 50 AC now. Your heirlooms grow with you up to level 50.") {
		t.Errorf("level 20: %q", got)
	}
	if got := keeperRankWords(60, 1, 50); !strings.HasPrefix(got, "Your heirloom armour gains 50 AC now. Your heirlooms are now of level 50.") {
		t.Errorf("level 60: %q", got)
	}
	if got := keeperRankWords(60, 2, 0); !strings.HasPrefix(got, "Your heirlooms are now of level 51.") || !strings.Contains(got, "next login") {
		t.Errorf("no AC row: %q", got)
	}
}
