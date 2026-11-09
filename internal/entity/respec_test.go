package entity

import (
	"reflect"
	"testing"

	"github.com/eqoa/server/internal/database"
)

// fitTraining: all a template asks when the points are there; else each stat
// its share, rounded down, the points left to the stats the rounding cut most
// (a tie to the first), never more than a stat asks and every point spent.
func TestFitTrainingShares(t *testing.T) {
	for _, c := range []struct {
		want  [7]int
		avail int
		got   [7]int
		short bool
	}{
		{[7]int{50, 20}, 35, [7]int{25, 10}, true},
		{[7]int{50, 20}, 70, [7]int{50, 20}, false},
		{[7]int{50, 20}, 100, [7]int{50, 20}, false},
		{[7]int{1, 1, 1}, 2, [7]int{1, 1, 0}, true},
		{[7]int{3, 0, 0, 4}, 5, [7]int{2, 0, 0, 3}, true},
		{[7]int{50, 20}, 0, [7]int{}, true},
		{[7]int{50, 20}, -4, [7]int{}, true},
		{[7]int{}, 10, [7]int{}, false},
	} {
		got, short := fitTraining(c.want, c.avail)
		if got != c.got || short != c.short {
			t.Errorf("%v of %d: %v %v, want %v %v", c.want, c.avail, got, short, c.got, c.short)
		}
	}
	// every point spent, none past what a stat asks, on any mix
	seed := uint32(1322)
	next := func(n int) int {
		seed = seed*1664525 + 1013904223
		return int(seed>>8) % n
	}
	for range 2000 {
		var want [7]int
		sum := 0
		for i := range want {
			want[i] = next(60)
			sum += want[i]
		}
		avail := next(sum + 30)
		got, _ := fitTraining(want, avail)
		spent := 0
		for i, v := range got {
			if v < 0 || v > want[i] {
				t.Fatalf("%v of %d: %v takes past what stat %d asks", want, avail, got, i)
			}
			spent += v
		}
		if spent != min(avail, sum) {
			t.Fatalf("%v of %d: %v spends %d, want %d", want, avail, got, spent, min(avail, sum))
		}
	}
}

// respecCatalog: A, then B under it, and C, which needs 20 points spent.
func respecCatalog() []database.CMRankRow {
	all, races := 32767, 2047
	return []database.CMRankRow{
		{ID: 1, Classes: all, Races: races, Name: "A", Cost: 10, Level: 1, Enabled: true},
		{ID: 2, Classes: all, Races: races, Name: "B", Parent: 1, Cost: 15, Level: 1, Enabled: true},
		{ID: 3, Classes: all, Races: races, Name: "C", Cost: 5, Level: 1, Spent: 20, Enabled: true},
	}
}

// A template's ranks are bought in passes: listed children first (C needing
// points spent, B its parent), every one is bought once what it needs is, in
// the order they came to be bought; an id twice counts once (its cost too).
// The plan changes nothing of the player. The control: C alone is left out,
// with the purchase's words.
func TestPlanRespecBuysInPasses(t *testing.T) {
	w := NewWorld()
	w.SetCMCatalog(respecCatalog())
	p := &Player{Level: 60, CMs: map[int]int{}, Mastery: Mastery{Points: 100, Percent: 40}}
	ranks := []CMTemplateRank{{ID: 3, Cost: 5}, {ID: 2, Cost: 15}, {ID: 1, Cost: 10}, {ID: 1, Cost: 10}}
	pl, s := w.planRespecLocked(p, [7]int{}, [7]int{}, ranks, CMKnows{Book: map[int]bool{}})
	if !reflect.DeepEqual(pl.Bought, []CMTemplateRank{{1, 10}, {2, 15}, {3, 5}}) || pl.Paid != 30 || len(pl.Skipped) != 0 {
		t.Errorf("bought %v for %d, skipped %v", pl.Bought, pl.Paid, pl.Skipped)
	}
	if pl.After != (Mastery{Points: 70, Spent: 30, Percent: 40}) || !reflect.DeepEqual(pl.Added, []int{1, 2, 3}) {
		t.Errorf("after: %+v, added %v", pl.After, pl.Added)
	}
	if len(s.CMs) != 3 || len(p.CMs) != 0 || p.Mastery.Points != 100 {
		t.Errorf("the scratch %v, the player %v %+v", s.CMs, p.CMs, p.Mastery)
	}
	pl, _ = w.planRespecLocked(p, [7]int{}, [7]int{}, []CMTemplateRank{{ID: 3, Cost: 5}}, CMKnows{})
	if len(pl.Bought) != 0 || len(pl.Skipped) != 1 || pl.Skipped[0].Why != "You must have spent 20 mastery points to buy C." {
		t.Errorf("the control: %+v", pl)
	}
}

// What the player holds is given back first, its ranks' abilities out of the
// book and those they replaced back, so a rank needing the replaced ability
// is bought; a rank held before and after is kept, one not in the template
// removed. The training: the base as born plus what the template asks, under
// the level's cap (a stat past it held there), every other point unspent.
func TestPlanRespecGivesBackFirst(t *testing.T) {
	w := NewWorld()
	all, races := 32767, 2047
	w.SetCMCatalog(append(respecCatalog(),
		database.CMRankRow{ID: 10, Classes: all, Races: races, Name: "Cry", Cost: 20, Level: 1, Requires: "Taunt@1",
			RequireSpells: "500", Adds: "Battle Cry@20", AddSpells: "501", Removes: "Taunt@1", RemoveSpells: "500", Enabled: true},
		database.CMRankRow{ID: 11, Classes: all, Races: races, Name: "Other", Cost: 40, Level: 1, Enabled: true},
	))
	bs := LookupBaseStats(0, 0)
	p := &Player{Level: 30, CMs: map[int]int{10: 20, 11: 40}, Mastery: Mastery{Points: 40, Spent: 60}, UnusedTP: 200, TotalTP: 300}
	p.BaseStr, p.BaseSta, p.BaseAgi, p.BaseDex, p.BaseWis, p.BaseInt, p.BaseCha = bs.Str+100, bs.Sta, bs.Agi, bs.Dex, bs.Wis, bs.Int, bs.Cha
	// the book holds Battle Cry, not Taunt (Cry took it)
	k := CMKnows{Book: map[int]bool{501: true}}
	want := [7]int{300, 10}
	pl, s := w.planRespecLocked(p, [7]int{100}, want, []CMTemplateRank{{ID: 10, Cost: 20}, {ID: 1, Cost: 10}}, k)
	if !reflect.DeepEqual(pl.Bought, []CMTemplateRank{{10, 20}, {1, 10}}) || !reflect.DeepEqual(pl.Kept, []int{10}) ||
		!reflect.DeepEqual(pl.Removed, []int{11}) || !reflect.DeepEqual(pl.Added, []int{1}) {
		t.Errorf("bought %v kept %v removed %v added %v (skipped %v)", pl.Bought, pl.Kept, pl.Removed, pl.Added, pl.Skipped)
	}
	if pl.Returned != 60 || pl.After != (Mastery{Points: 70, Spent: 30}) {
		t.Errorf("returned %d, after %+v", pl.Returned, pl.After)
	}
	room := StatCapBase(30) - bs.Str
	if !pl.TPCapped || pl.TPShort || pl.TPWant != [7]int{room, 10} || pl.TPTo != [7]int{room, 10} || pl.TPAfter != 300-room-10 ||
		s.BaseStr != StatCapBase(30) || s.BaseSta != bs.Sta+10 {
		t.Errorf("the training: want %v to %v, %d unspent, capped %v, STR %d STA %d", pl.TPWant, pl.TPTo, pl.TPAfter, pl.TPCapped,
			s.BaseStr, s.BaseSta)
	}
	if !pl.Changes() {
		t.Errorf("no change")
	}
	// the control: a template of what it holds, at what it holds, changes nothing
	p2 := &Player{Level: 30, CMs: map[int]int{1: 10}, Mastery: Mastery{Points: 5, Spent: 10}, UnusedTP: 0, TotalTP: 20}
	p2.BaseStr, p2.BaseSta, p2.BaseAgi, p2.BaseDex, p2.BaseWis, p2.BaseInt, p2.BaseCha = bs.Str+20, bs.Sta, bs.Agi, bs.Dex, bs.Wis, bs.Int, bs.Cha
	pl, _ = w.planRespecLocked(p2, [7]int{20}, [7]int{20}, []CMTemplateRank{{ID: 1, Cost: 10}}, CMKnows{})
	if pl.Changes() || !reflect.DeepEqual(pl.Kept, []int{1}) {
		t.Errorf("the control changes: %+v", pl)
	}
}
