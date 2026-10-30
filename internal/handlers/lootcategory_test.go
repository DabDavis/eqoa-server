package handlers

import (
	"math"
	"math/rand"
	"testing"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/protocol"
)

// The loot categories (lootcategory.go): each category's own rolls at its own
// rate, an item drawn from the creature's pool; the pool's size never changes
// how often a category drops.

// pool is n items of weight 1, pattern ids from base.
func pool(base, n, maxStack int) []database.LootChoice {
	out := make([]database.LootChoice, n)
	for i := range out {
		out[i] = database.LootChoice{Weight: 1, Item: database.CharInventoryEntry{
			Pattern: database.ItemPatternRecord{PatternID: base + i, MaxStack: maxStack}}}
	}
	return out
}

// within reports whether got is within k standard deviations of n draws at p.
func within(got, n int, p, k float64) bool {
	mean := float64(n) * p
	return math.Abs(float64(got)-mean) <= k*math.Sqrt(float64(n)*p*(1-p))
}

// A category drops at its rate whatever its pool's size: one item, ten or
// five hundred, over 300,000 seeded kills, each within five standard
// deviations of the rate; and the big pool's items come evenly. The control:
// a rate twice as high is told apart from it.
func TestACategorysRateHoldsWhateverItsPool(t *testing.T) {
	const kills = 300000
	for _, rate := range []float64{1.0 / 30, 1.0 / 240, 1.0 / 1200} {
		var counts []int
		for _, size := range []int{1, 10, 500} {
			rng := rand.New(rand.NewSource(int64(size) * 7919))
			rolls := []database.CategoryRoll{{Category: "uncommon", Rate: rate, Choices: pool(1000, size, 1)}}
			n := 0
			per := map[int]int{}
			for k := 0; k < kills; k++ {
				for _, it := range rollCategories(rolls, rng.Float64, rng.Intn) {
					n++
					per[it.Pattern.PatternID]++
				}
			}
			if !within(n, kills, rate, 5) {
				t.Errorf("rate %.5f, a pool of %d: %d drops in %d kills, want about %.0f", rate, size, n, kills, rate*kills)
			}
			if size == 500 && rate == 1.0/30 {
				// 10,000 drops over 500 items: each about 20; none above 60 or missing many
				missing, top := 0, 0
				for id := 1000; id < 1500; id++ {
					missing += map[bool]int{true: 1}[per[id] == 0]
					top = max(top, per[id])
				}
				if top > 60 || missing > 5 {
					t.Errorf("the big pool's draw is uneven: the most of one item %d, items never drawn %d", top, missing)
				}
			}
			counts = append(counts, n)
		}
		// the control: twice the rate is outside the band
		if within(counts[0], kills, 2*rate, 5) {
			t.Errorf("rate %.5f: %d drops also look like twice the rate; the test cannot tell rates apart", rate, counts[0])
		}
	}
}

// The trash's counts: jib's chances of 0 to 6 items a kill, each item drawn
// from the creature's two on its own and gathered into stacks; 1.475 a
// kill. Every count comes at its chance within five standard deviations.
func TestTrashCountsComeAtTheirChances(t *testing.T) {
	counts := []float64{25, 31, 25, 12, 5, 1.5, 0.5}
	rolls := []database.CategoryRoll{{Category: "trash", Counts: counts, Choices: pool(1, 2, 99)}}
	rng := rand.New(rand.NewSource(42))
	const kills = 200000
	seen := make([]int, len(counts))
	items, perItem := 0, map[int]int{}
	for k := 0; k < kills; k++ {
		got := rollCategories(rolls, rng.Float64, rng.Intn)
		n := 0
		for _, it := range got {
			n += it.StackLeft
			perItem[it.Pattern.PatternID] += it.StackLeft
		}
		if len(got) > 2 {
			t.Fatalf("%d stacks from two items: the same item's were not gathered", len(got))
		}
		seen[n]++
		items += n
	}
	for c, want := range counts {
		if !within(seen[c], kills, want/100, 5) {
			t.Errorf("count %d: %d kills in %d, want about %.0f", c, seen[c], kills, want/100*kills)
		}
	}
	if mean := float64(items) / kills; math.Abs(mean-1.475) > 0.02 {
		t.Errorf("%.3f items a kill, want 1.475 (jib's table: 0.31+0.5+0.36+0.2+0.075+0.03)", mean)
	}
	if !within(perItem[1], items, 0.5, 5) {
		t.Errorf("the two items %d and %d: not an even pick", perItem[1], perItem[2])
	}
}

// A roll at a rate gives a stackable item minstack..maxstack at a time, no
// more than its own maxstack, and one of anything else; an empty pool, or a
// miss, nothing.
func TestARollsStackAndItsMisses(t *testing.T) {
	rng := rand.New(rand.NewSource(1))
	stack := []database.CategoryRoll{{Category: "stacks", Rate: 1, MinStack: 1, MaxStack: 3, Choices: pool(1, 1, 99)}}
	seen := map[int]bool{}
	for k := 0; k < 1000; k++ {
		got := rollCategories(stack, rng.Float64, rng.Intn)
		if len(got) != 1 || got[0].StackLeft < 1 || got[0].StackLeft > 3 {
			t.Fatalf("got %+v", got)
		}
		seen[got[0].StackLeft] = true
	}
	if len(seen) != 3 {
		t.Errorf("stacks of %v, want 1, 2 and 3", seen)
	}
	small := []database.CategoryRoll{{Category: "stacks", Rate: 1, MinStack: 5, MaxStack: 9, Choices: pool(1, 1, 2)}}
	if got := rollCategories(small, rng.Float64, rng.Intn); got[0].StackLeft != 2 {
		t.Errorf("an item that stacks to 2 came %d at a time", got[0].StackLeft)
	}
	single := []database.CategoryRoll{{Category: "uncommon", Rate: 1, MinStack: 1, MaxStack: 3, Choices: pool(1, 1, 1)}}
	if got := rollCategories(single, rng.Float64, rng.Intn); got[0].StackLeft != 1 {
		t.Errorf("an unstackable item came %d at a time", got[0].StackLeft)
	}
	empty := []database.CategoryRoll{{Category: "rare", Rate: 1}}
	if got := rollCategories(empty, rng.Float64, rng.Intn); len(got) != 0 {
		t.Errorf("an empty pool gave %+v", got)
	}
	miss := []database.CategoryRoll{{Category: "rare", Rate: 0.5, Choices: pool(1, 1, 1)}}
	if got := rollCategories(miss, func() float64 { return 0.5 }, rng.Intn); len(got) != 0 {
		t.Errorf("a roll at the rate itself landed: %+v", got)
	}
}

// Pets and training dummies roll no category (nor a charmed creature, by the
// same rule as lootNames'); a kill that gives no experience rolls them as any
// other.
func TestWhoRollsCategories(t *testing.T) {
	if !rollsCategories(&entity.NPC{Level: 5}) {
		t.Error("a creature rolls none")
	}
	if rollsCategories(&entity.NPC{Owner: 7}) || rollsCategories(&entity.NPC{Dummy: entity.TrainingDummy}) {
		t.Error("a pet or a dummy rolls them")
	}
}

// seedCategories writes a preset, its rates and the pools: "jib" active with
// an uncommon at 1 in 2 and the trash counts; "user" with an uncommon at 1;
// the sword in the plains' uncommon pool for levels 10-20, the bait in every
// region's, and the bait the beetle's own trash.
func seedCategories(t *testing.T, f *corpseFixture) {
	t.Helper()
	for _, q := range []struct {
		sql  string
		args []any
	}{
		{`INSERT INTO lootpreset (name, active) VALUES ('jib', 1), ('user', 0)`, nil},
		{`INSERT INTO lootrate (preset, profile, category, rate, position) VALUES ('jib', 'xp', 'uncommon', 0.5, 2)`, nil},
		{`INSERT INTO lootrate (preset, profile, category, counts, position) VALUES ('jib', 'xp', 'trash', '0,0,1', 1)`, nil},
		{`INSERT INTO lootrate (preset, profile, category, rate) VALUES ('jib', 'named', 'uncommon', 0.9)`, nil},
		{`INSERT INTO lootrate (preset, profile, category, rate) VALUES ('user', 'xp', 'uncommon', 1)`, nil},
		{`INSERT INTO lootpool (category, region, patternid, minlevel, maxlevel) VALUES ('uncommon', 'plains', ?, 10, 20)`, []any{f.sword}},
		{`INSERT INTO lootpool (category, region, patternid, minlevel, maxlevel) VALUES ('uncommon', '*', ?, 10, 20)`, []any{f.bait}},
		{`INSERT INTO lootpool (category, region, patternid, minlevel, maxlevel) VALUES ('trash', 'plains', ?, 1, 100)`, []any{f.sword}},
		{`INSERT INTO lootpool (category, npc_name, patternid) VALUES ('trash', 'beetle', ?)`, []any{f.bait}},
		{`INSERT INTO lootcreature (npc_name, region) VALUES ('beetle', 'plains')`, nil},
		{`INSERT INTO lootcreature (npc_name, region, profile) VALUES ('old red', 'plains', 'named')`, nil},
		{`INSERT INTO lootcreature (npc_name, region, profile) VALUES ('ghost', 'plains', 'boss')`, nil},
		{`INSERT INTO lootcreature (npc_name, region) VALUES ('kindred spirit', '-')`, nil},
	} {
		if _, err := f.db.Exec(q.sql, q.args...); err != nil {
			t.Fatal(err)
		}
	}
}

func patterns(cs []database.LootChoice) []int {
	var out []int
	for _, c := range cs {
		out = append(out, c.Item.Pattern.PatternID)
	}
	return out
}

// A creature's rolls: the active preset's rates for its profile (the
// default's when the preset has none for it), its region's pool and every
// region's, its own rows first; the controls: a level no row covers, region
// "-", no active preset.
func TestCategoryRollsByPresetProfileRegionAndLevel(t *testing.T) {
	f := newCorpseFixture(t, 0)
	seedCategories(t, f)
	rolls, err := f.inv.CategoryRolls("Beetle", 15)
	if err != nil {
		t.Fatal(err)
	}
	if len(rolls) != 2 || rolls[0].Category != "trash" || len(rolls[0].Counts) != 3 || rolls[1].Category != "uncommon" ||
		rolls[1].Rate != 0.5 {
		t.Fatalf("the beetle's rolls %+v", rolls)
	}
	if got := patterns(rolls[1].Choices); len(got) != 2 || got[0] != f.sword || got[1] != f.bait {
		t.Errorf("its uncommon pool %v: want the plains' sword and every region's bait", got)
	}
	if got := patterns(rolls[0].Choices); len(got) != 1 || got[0] != f.bait {
		t.Errorf("its trash %v: want its own bait, not the region's sword", got)
	}
	if rolls, _ := f.inv.CategoryRolls("beetle", 30); len(rolls[1].Choices) != 0 {
		t.Errorf("level 30, no row covers it: %v", patterns(rolls[1].Choices))
	}
	if rolls, _ := f.inv.CategoryRolls("old red", 15); len(rolls) != 1 || rolls[0].Rate != 0.9 {
		t.Errorf("a named creature: %+v", rolls)
	}
	if rolls, _ := f.inv.CategoryRolls("ghost", 15); len(rolls) != 2 || rolls[1].Rate != 0.5 {
		t.Errorf("a profile the preset lacks takes the default's: %+v", rolls)
	}
	if rolls, _ := f.inv.CategoryRolls("wasp", 15); len(rolls) != 2 || len(patterns(rolls[1].Choices)) != 1 {
		t.Errorf("a creature with no row: every region's rows alone, got %+v", rolls)
	}
	if rolls, _ := f.inv.CategoryRolls("kindred spirit", 15); len(rolls) != 0 {
		t.Errorf("region -: %+v", rolls)
	}
	if _, err := f.db.Exec(`UPDATE lootpreset SET active = (name = 'user')`); err != nil {
		t.Fatal(err)
	}
	if rolls, _ := f.inv.CategoryRolls("beetle", 15); len(rolls) != 1 || rolls[0].Rate != 1 {
		t.Errorf("the user preset made active: %+v", rolls)
	}
	if _, err := f.db.Exec(`UPDATE lootpreset SET active = 0`); err != nil {
		t.Fatal(err)
	}
	if rolls, _ := f.inv.CategoryRolls("beetle", 15); len(rolls) != 0 {
		t.Errorf("no active preset: %+v", rolls)
	}
}

// A corpse holds what its categories rolled, beside its loot tables', rolled
// once a death.
func TestACorpseHoldsWhatItsCategoriesRolled(t *testing.T) {
	f := newCorpseFixture(t, 0)
	f.npc.Level = 15
	seedCategories(t, f)
	f.h.lootFloat = func() float64 { return 0 } // every roll lands, the trash's count is 2
	f.h.lootIntn = func(int) int { return 0 }
	f.h.OpenCorpse(f.sess, f.npc)
	got := f.sent()
	mustOps(t, got, protocol.OpcodePlayerTunar, protocol.OpcodeClientMessage, protocol.OpcodeLoot)
	// 0 on the counts "0,0,1" is count 2: two bait gathered in one stack, and the uncommon's sword
	if n := lootCount(t, got[2]); n != 2 {
		t.Fatalf("%d records, want the trash's stack and the uncommon", n)
	}
	// the control: no active preset, nothing
	g := newCorpseFixture(t, 0)
	g.npc.Level = 15
	seedCategories(t, g)
	if _, err := g.db.Exec(`UPDATE lootpreset SET active = 0`); err != nil {
		t.Fatal(err)
	}
	g.h.lootFloat = func() float64 { return 0 }
	g.h.OpenCorpse(g.sess, g.npc)
	got = g.sent()
	if n := lootCount(t, got[len(got)-1]); n != 0 {
		t.Fatalf("no active preset: %d records", n)
	}
}
