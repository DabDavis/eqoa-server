package handlers

import (
	"testing"

	"github.com/eqoa/server/internal/database"
)

func TestSkyTwinsCountDistribution(t *testing.T) {
	rolls := []database.CategoryRoll{{Category: "sky twins ascended", Counts: []float64{0, 30, 70}, Choices: pool(980900, 32, 1)}}
	counts := map[int]int{}
	for i := 0; i < 10000; i++ {
		got := rollCategories(rolls, func() float64 { return float64(i) / 10000 }, func(int) int { return 0 })
		counts[len(got)]++
	}
	if len(counts) != 2 || counts[1] != 3000 || counts[2] != 7000 {
		t.Fatalf("one/two distribution %v", counts)
	}
}

func TestSkyTwinsGuaranteedGearForAnyOpener(t *testing.T) {
	for _, category := range []string{"sky twins ascended", "uncommon"} {
		for _, roll := range []float64{0, 0.299999, 0.3, 0.999999} {
			f := newCorpseFixture(t, 0)
			f.npc.Name, f.npc.Level, f.player.Level = "Talon's Veeshan", 70, 1
			for _, q := range []struct {
				sql  string
				args []any
			}{
				{`UPDATE itempattern SET levelreq = 60 WHERE patternid = ?`, []any{f.sword}},
				{`INSERT INTO lootpreset (name, active) VALUES ('jib', 1)`, nil},
				{`INSERT INTO lootcreature (npc_name, region, profile) VALUES ('talon''s veeshan', 'plane of sky', 'sky twins ascended')`, nil},
				{`INSERT INTO lootrate (preset, profile, category, counts) VALUES ('jib', 'sky twins ascended', ?, '0,30,70')`, []any{category}},
				{`INSERT INTO lootpool (category, region, patternid, minlevel, maxlevel) VALUES (?, 'plane of sky', ?, 1, 100)`, []any{category, f.sword}},
			} {
				if _, err := f.db.Exec(q.sql, q.args...); err != nil {
					t.Fatal(err)
				}
			}
			f.h.lootFloat = func() float64 { return roll }
			f.h.lootIntn = func(int) int { return 0 }
			want := int64(1)
			if roll >= 0.3 {
				want = 2
			}
			if category != "sky twins ascended" {
				want = 0
			}
			for opening := 0; opening < 2; opening++ {
				f.h.OpenCorpse(f.sess, f.npc)
				sent := f.sent()
				if got := lootCount(t, sent[len(sent)-1]); got != want {
					t.Fatalf("category %s roll %v opening %d got %d want %d", category, roll, opening, got, want)
				}
				f.h.lootFloat = func() float64 { t.Fatal("corpse rerolled on reopen"); return 0 }
			}
		}
	}
}
