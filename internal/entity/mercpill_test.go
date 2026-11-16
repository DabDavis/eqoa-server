package entity

import (
	"testing"
	"time"
)

// An alchemist merc's Master Class keepsake (mercbrew.go's mercKeepsLocked;
// the owner, 2026-11-16: "alchemist mercs should also make the transformative
// ogre pills if they are the right level", then "and they will only make if
// they dont have one so player can take it from their inventory"): a
// Transmuter at the second rank's level brews ONE Transformative Pill into its
// owner's stock when it holds none, no more while one waits, and the next
// once its owner took it; never below the rank's level, never without the
// Master Class; it never throws or drinks one.

const (
	testPill      = 709301
	testPillSpell = 700772
)

func pillPotions(w *World) {
	alcSupplyPotions(w)
	pill := NewPotionEffect(testPill, "Transformative Pill",
		"Self, Increase Hitpoints by 1250, Increase STR by 150, Increase STA by 150, Increase Maximum HP by 1250, 30min",
		"Illusion: Ogre", 1507275072, "", 0, 0)
	pill.SetBody(2223010960)
	pill.MaxStack = 9999
	w.NotePotionEffect(pill)
}

// transmuterRanks are the Transmuter's first two ranks as cmcatalog has them
// (7401 at 55, 7402 Creative Transmutation at 60 adding Create Transformative).
func transmuterRanks() []*CMRank {
	return []*CMRank{
		{ID: 7401, Category: 4, Classes: 1 << 14, Mastery: "Transmuter", Rank: 1, Name: "Transmuter", Level: 55},
		{ID: 7402, Category: 4, Classes: 1 << 14, Mastery: "Transmuter", Rank: 2, Name: "Creative Transmutation", Parent: 7401, Level: 60,
			Adds: []CMAbility{{Name: "Creative Transmutation", Level: 60, SpellID: testPillSpell}}},
	}
}

// pillKit is an alchemist of the level with its book's Volatile Mixture, its
// refinement and Create Transformative (which classspells puts in every
// level-60 alchemist's book), a Transmuter's ranks up to its level when
// master.
func (r *mercRig) pillKit(level int, master bool) MercKit {
	r.p.Level = level
	create := bookSpell(testPillSpell, "Create Transformative", 0, 2*time.Second, 3*time.Second, 20, "Self, Summon Transformative Pill x3")
	create.Level = 60
	refine := bookSpell(50, "Refinement", 0, 2*time.Second, 3*time.Second, 10, "Self, Summon Draught x3")
	k := r.kit(14, 0, 0, bookSpell(700085, "Volatile Mixture", 0, 2*time.Second, 3*time.Second, 20, "Self, Summon Volatile Potion x3"),
		refine, create)
	if master {
		k.Master = "Transmuter"
		for _, rk := range transmuterRanks() {
			if rk.Level <= level {
				k.MasterRanks = append(k.MasterRanks, rk)
			}
		}
	}
	return k
}

func pillWant(ms *mercState, w *World) (mercBrewWant, bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	for _, c := range w.mercBrewWantsLocked(ms) {
		if c.pe.PatternID == testPill {
			return c, true
		}
	}
	return mercBrewWant{}, false
}

func TestMercTransmuterBrewsOnePillAtATime(t *testing.T) {
	r := newMercRig(t)
	pillPotions(r.w)
	r.w.SetMercBrew(time.Second, -1, -1)
	r.w.SetMercBrewBatch(20, time.Second, 0)
	ms := r.hire(r.pillKit(60, true), MercAssist)
	// its other stacks full, so only the pill is left to brew
	r.w.SetMercStock(r.p.CharServerID, map[int]int{testVolatile: 20, testDraught: 20})
	r.w.SetMercBrewBatch(20, time.Second, 20)
	c, ok := pillWant(ms, r.w)
	if !ok || c.most != 1 {
		t.Fatalf("a level 60 Transmuter keeps %+v (found %v), want one Transformative Pill", c, ok)
	}
	brewed := r.brewNotes()
	// none held: it brews one
	r.run(0, 10*time.Second)
	r.w.runMercNotes()
	if st := r.w.MercStockOf(r.p.CharServerID); st[testPill] != 1 {
		t.Fatalf("stock %v after 10 s at ease, want one pill", st)
	}
	if len(*brewed) != 1 || (*brewed)[0].PatternID != testPill || (*brewed)[0].Count != 1 {
		t.Fatalf("brewed %+v, want a batch of one pill", *brewed)
	}
	// one held: no more, however long it rests
	r.run(10100*time.Millisecond, 60*time.Second)
	r.w.runMercNotes()
	if st := r.w.MercStockOf(r.p.CharServerID); st[testPill] != 1 || len(*brewed) != 1 {
		t.Fatalf("stock %v with %d brews while one waited, want still one", st, len(*brewed))
	}
	// its owner takes it (the Stock tab's take): it brews the next at its rate
	if r.w.TakeMercStock(r.p.CharServerID, testPill, 5) != 1 {
		t.Fatal("the owner could not take his pill")
	}
	r.run(60100*time.Millisecond, 70*time.Second)
	r.w.runMercNotes()
	if st := r.w.MercStockOf(r.p.CharServerID); st[testPill] != 1 || len(*brewed) != 2 {
		t.Fatalf("after the take: stock %v, %d brews; want one pill again", st, len(*brewed))
	}
	// one it carries counts as held
	r.w.TakeMercStock(r.p.CharServerID, testPill, 1)
	r.w.mu.Lock()
	ms.kit.Carried = append(ms.kit.Carried, MercCarried{ID: 1, PatternID: testPill, Count: 1})
	r.w.mu.Unlock()
	if c, _ := pillWant(ms, r.w); c.have != 1 {
		t.Errorf("one carried: have %d", c.have)
	}
	// it never throws or drinks one, from its stock or its path's own
	r.w.mu.Lock()
	defer r.w.mu.Unlock()
	r.w.mercSupply.stock[r.p.ServerID][testPill] = 1
	for _, c := range r.w.mercThrowablesLocked(ms) {
		if c.pe.PatternID == testPill {
			t.Error("throws its pill (its path's)")
		}
	}
	carried, stock := r.w.mercCarriedThrowsLocked(ms)
	for _, c := range append(carried, stock...) {
		if c.pe.PatternID == testPill {
			t.Error("throws its pill (stock or carried)")
		}
	}
	for _, d := range r.w.mercDraughtsLocked(ms) {
		if d.pe.PatternID == testPill {
			t.Error("drinks its pill as a draught")
		}
	}
}

// The gate: the Transmuter's Creative Transmutation, at 60. Below it (a
// Transmuter of 55 to 59 has the first rank alone) none; at and above it one;
// a level 60 alchemist without the Master Class (Create Transformative in its
// book by classspells) none, as a player alchemist has the spell only by the
// rank; a turned-off Create Transformative none.
func TestMercTransmuterPillLevelGate(t *testing.T) {
	for _, tc := range []struct {
		level  int
		master bool
		want   bool
	}{{55, true, false}, {59, true, false}, {60, true, true}, {61, true, true}, {60, false, false}} {
		r := newMercRig(t)
		pillPotions(r.w)
		r.w.SetMercBrew(time.Second, -1, -1)
		r.w.SetMercBrewBatch(20, time.Second, 20)
		ms := r.hire(r.pillKit(tc.level, tc.master), MercAssist)
		r.w.SetMercStock(r.p.CharServerID, map[int]int{testVolatile: 20, testDraught: 20})
		if _, ok := pillWant(ms, r.w); ok != tc.want {
			t.Errorf("level %d master %v: keeps a pill %v, want %v", tc.level, tc.master, ok, tc.want)
		}
		r.run(0, 15*time.Second)
		r.w.runMercNotes()
		if got := r.w.MercStockOf(r.p.CharServerID)[testPill]; (got == 1) != tc.want || got > 1 {
			t.Errorf("level %d master %v: %d pills brewed", tc.level, tc.master, got)
		}
		if tc.level == 60 && tc.master {
			r.w.mu.Lock()
			ms.kit.SpellsOff = []int{testPillSpell}
			r.w.mu.Unlock()
			if _, ok := pillWant(ms, r.w); ok {
				t.Error("keeps a pill with Create Transformative turned off")
			}
		}
	}
}

// The other stacks are as they were: the pill is one more want, after them.
func TestMercTransmuterPillLeavesItsStacks(t *testing.T) {
	r := newMercRig(t)
	pillPotions(r.w)
	ms := r.hire(r.pillKit(60, true), MercAssist)
	r.w.SetMercStock(r.p.CharServerID, map[int]int{})
	r.w.mu.Lock()
	wants := r.w.mercBrewWantsLocked(ms)
	r.w.mu.Unlock()
	if len(wants) != 3 || wants[0].pe.PatternID != testVolatile || wants[1].pe.PatternID != testDraught || wants[2].pe.PatternID != testPill {
		t.Fatalf("keeps %+v, want its Volatile Potion, its draught, then the pill", wants)
	}
	if wants[0].most == 1 || wants[1].most == 1 {
		t.Errorf("its potions' stacks held to one: %+v", wants)
	}
}
