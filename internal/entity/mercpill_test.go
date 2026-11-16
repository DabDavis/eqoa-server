package entity

import (
	"testing"
	"time"
)

// An alchemist merc's Master Class keepsakes (mercbrew.go's mercKeepsLocked;
// the owner, 2026-11-16: "alchemist mercs should also make the transformative
// ogre pills if they are the right level", "and they will only make if they
// dont have one so player can take it from their inventory", then "make the
// transmutive potion and miraculous draught one at a time too"): each potion a
// mixture its own Master Class's ranks give makes, ONE at a time in its
// owner's stock: brewed when none is held, no more while one waits, the next
// once its owner took it; never below the rank's level, never without the
// Master Class; the stock's copy never used by the merc (its own uses are
// conjured: mercselfpotion.go).

// keepPotions are the master potions (mercselfpotion_test.go's), the path's
// and a draught, with a stack's most for each.
func keepPotions(w *World) {
	alcMasterPotions(w)
	d := NewPotionEffect(testDraught, "Draught", "Self, Decrease Hitpoints by 400, Increase Power per tic by 300, 18sec", "", 0, "", 0, 0)
	w.NotePotionEffect(d)
	w.mu.Lock()
	for _, pe := range w.potions.byPattern {
		pe.MaxStack = 9999
	}
	w.mu.Unlock()
}

// masterRanks are the alchemist's Master Classes' ranks as cmcatalog has them:
// the Transmuter's 7401 at 55, 7402 Creative Transmutation (Create
// Transformative) and 7403 Transmutative Mixture at 60; the Metabolist's 7411
// at 55 and 7412 Miraculous Mix at 60.
func masterRanks(master string) []*CMRank {
	rank := func(id, level int, name string, adds ...int) *CMRank {
		r := &CMRank{ID: id, Category: CMCategoryMaster, Classes: 1 << 14, Mastery: master, Name: name, Level: level}
		for _, a := range adds {
			r.Adds = append(r.Adds, CMAbility{Name: name, Level: level, SpellID: a})
		}
		return r
	}
	switch master {
	case "Transmuter":
		return []*CMRank{rank(7401, 55, "Transmuter"), rank(7402, 60, "Creative Transmutation", 700772), rank(7403, 60, "Transmutative Mixture", 700827)}
	case "Metabolist":
		return []*CMRank{rank(7411, 55, "Metabolist"), rank(7412, 60, "Miraculous Mix", 700802)}
	}
	return nil
}

// keepKit is an alchemist of the level with its book's Volatile Mixture, its
// refinement and the three Master Class mixtures (which classspells puts in
// every level-60 alchemist's book), the Master Class's ranks up to its level
// (as the handlers give them).
func (r *mercRig) keepKit(level int, master string) MercKit {
	r.p.Level = level
	refine := bookSpell(50, "Refinement", 0, 2*time.Second, 3*time.Second, 10, "Self, Summon Draught x3")
	k := r.kit(14, 0, 0, bookSpell(700085, "Volatile Mixture", 0, 2*time.Second, 3*time.Second, 20, "Self, Summon Volatile Potion x3"),
		refine, testCreative, testTransmixture, testMiracleMix)
	if master != "" {
		k.Master = master
		for _, rk := range masterRanks(master) {
			if rk.Level <= level {
				k.MasterRanks = append(k.MasterRanks, rk)
			}
		}
	}
	return k
}

// keeps is the keepsakes the merc keeps, by pattern: the stack's most.
func keeps(ms *mercState, w *World) map[int]int {
	w.mu.Lock()
	defer w.mu.Unlock()
	out := map[int]int{}
	for _, c := range w.mercBrewWantsLocked(ms) {
		switch c.pe.PatternID {
		case testPill, testTransmutive, testMiraculous:
			out[c.pe.PatternID] = c.most
		}
	}
	return out
}

// keepRig is a merc of the master class at the level, its other stacks full
// (so only its keepsakes are left to brew) and a brew a second.
func keepsakeRig(t *testing.T, level int, master string) (*mercRig, *mercState) {
	r := newMercRig(t)
	keepPotions(r.w)
	r.w.SetMercBrew(time.Second, -1, -1)
	r.w.SetMercBrewBatch(20, time.Second, 20)
	ms := r.hire(r.keepKit(level, master), MercAssist)
	r.w.SetMercStock(r.p.CharServerID, map[int]int{testVolatile: 20, testDraught: 20})
	return r, ms
}

func TestMercMasterKeepsakesOneAtATime(t *testing.T) {
	for _, tc := range []struct {
		master string
		want   []int
	}{
		{"Transmuter", []int{testPill, testTransmutive}},
		{"Metabolist", []int{testMiraculous}},
	} {
		r, ms := keepsakeRig(t, 60, tc.master)
		got := keeps(ms, r.w)
		if len(got) != len(tc.want) {
			t.Fatalf("%s of 60 keeps %v, want %v", tc.master, got, tc.want)
		}
		for _, id := range tc.want {
			if got[id] != 1 {
				t.Fatalf("%s of 60 keeps %v: want one of %d", tc.master, got, id)
			}
		}
		brewed := r.brewNotes()
		// none held: one of each
		r.run(0, 10*time.Second)
		r.w.runMercNotes()
		st := r.w.MercStockOf(r.p.CharServerID)
		for _, id := range tc.want {
			if st[id] != 1 {
				t.Fatalf("%s: stock %v after 10 s at ease, want one of %d", tc.master, st, id)
			}
		}
		if len(*brewed) != len(tc.want) {
			t.Fatalf("%s: %d brews, want %d of one each: %+v", tc.master, len(*brewed), len(tc.want), *brewed)
		}
		for _, b := range *brewed {
			if b.Count != 1 {
				t.Errorf("%s: a batch of %d of %d", tc.master, b.Count, b.PatternID)
			}
		}
		// one held: no more, however long it rests
		r.run(10100*time.Millisecond, 60*time.Second)
		r.w.runMercNotes()
		if len(*brewed) != len(tc.want) {
			t.Fatalf("%s: %d brews while one of each waited", tc.master, len(*brewed))
		}
		// its owner takes each (the Stock tab's take): the next is brewed
		at := 60 * time.Second
		for _, id := range tc.want {
			if r.w.TakeMercStock(r.p.CharServerID, id, 5) != 1 {
				t.Fatalf("%s: the owner could not take %d", tc.master, id)
			}
			r.run(at+100*time.Millisecond, at+10*time.Second)
			at += 10 * time.Second
			r.w.runMercNotes()
			if st := r.w.MercStockOf(r.p.CharServerID); st[id] != 1 {
				t.Fatalf("%s: after taking %d the stock %v, want one again", tc.master, id, st)
			}
		}
		if len(*brewed) != 2*len(tc.want) {
			t.Errorf("%s: %d brews, want %d", tc.master, len(*brewed), 2*len(tc.want))
		}
	}
}

// The gate: its Master Class's second and third ranks, at 60. A Transmuter or
// Metabolist of 55 to 59 has the first rank alone: none; at 60 and above its
// own; the other's never; with no Master Class (the mixtures in its book by
// classspells) none, as a player alchemist has them only by the ranks; a
// mixture turned off: not its potion.
func TestMercMasterKeepsakesLevelGate(t *testing.T) {
	for _, tc := range []struct {
		level  int
		master string
		want   []int
	}{
		{55, "Transmuter", nil}, {59, "Transmuter", nil}, {60, "Transmuter", []int{testPill, testTransmutive}}, {61, "Transmuter", []int{testPill, testTransmutive}},
		{55, "Metabolist", nil}, {59, "Metabolist", nil}, {60, "Metabolist", []int{testMiraculous}}, {61, "Metabolist", []int{testMiraculous}},
		{60, "", nil},
	} {
		r, ms := keepsakeRig(t, tc.level, tc.master)
		got := keeps(ms, r.w)
		if len(got) != len(tc.want) {
			t.Errorf("%q of %d keeps %v, want %v", tc.master, tc.level, got, tc.want)
		}
		r.run(0, 15*time.Second)
		r.w.runMercNotes()
		st := r.w.MercStockOf(r.p.CharServerID)
		for _, id := range []int{testPill, testTransmutive, testMiraculous} {
			want := 0
			for _, w := range tc.want {
				if w == id {
					want = 1
				}
			}
			if st[id] != want {
				t.Errorf("%q of %d: %d of %d brewed, want %d", tc.master, tc.level, st[id], id, want)
			}
		}
	}
	// a rank above its level given anyway is not counted
	r, ms := keepsakeRig(t, 59, "")
	r.w.mu.Lock()
	ms.kit.Master, ms.kit.MasterRanks = "Transmuter", masterRanks("Transmuter")
	r.w.mu.Unlock()
	if got := keeps(ms, r.w); len(got) != 0 {
		t.Errorf("a Transmuter of 59 handed its rank of 60 keeps %v", got)
	}
	// turned off: not its potion
	r, ms = keepsakeRig(t, 60, "Transmuter")
	r.w.mu.Lock()
	ms.kit.SpellsOff = []int{700772}
	r.w.mu.Unlock()
	if got := keeps(ms, r.w); len(got) != 1 || got[testTransmutive] != 1 {
		t.Errorf("with Create Transformative off it keeps %v, want the Transmutive Potion alone", got)
	}
}

// One it carries counts as held; the other stacks are as they were, the
// keepsakes after them.
func TestMercMasterKeepsakesBesideItsStacks(t *testing.T) {
	r, ms := keepsakeRig(t, 60, "Transmuter")
	r.w.SetMercStock(r.p.CharServerID, map[int]int{})
	r.w.mu.Lock()
	wants := r.w.mercBrewWantsLocked(ms)
	r.w.mu.Unlock()
	if len(wants) != 4 || wants[0].pe.PatternID != testVolatile || wants[1].pe.PatternID != testDraught ||
		wants[2].pe.PatternID != testPill || wants[3].pe.PatternID != testTransmutive {
		t.Fatalf("keeps %+v, want its Volatile Potion, its draught, then the pill and the Transmutive Potion", wants)
	}
	if wants[0].most == 1 || wants[1].most == 1 {
		t.Errorf("its potions' stacks held to one: %+v", wants)
	}
	r.w.mu.Lock()
	ms.kit.Carried = append(ms.kit.Carried, MercCarried{ID: 1, PatternID: testPill, Count: 1})
	r.w.mu.Unlock()
	for _, c := range func() []mercBrewWant { r.w.mu.Lock(); defer r.w.mu.Unlock(); return r.w.mercBrewWantsLocked(ms) }() {
		if c.pe.PatternID == testPill && c.have != 1 {
			t.Errorf("one carried: have %d", c.have)
		}
	}
}

// The stock's copies are its owner's: the merc's own pill, debuff and heal
// (mercselfpotion.go) are conjured, and none of its throws or drinks takes
// one from the stock.
func TestMercMasterKeepsakesLeftToItsOwner(t *testing.T) {
	r, ms := keepsakeRig(t, 60, "Transmuter")
	r.w.SetMercStock(r.p.CharServerID, map[int]int{testVolatile: 20, testDraught: 20, testPill: 1, testTransmutive: 1, testMiraculous: 1})
	crab := r.creature("Crab", 28, 10)
	r.w.mu.Lock()
	crab.CurrentEnemy = r.p.ServerID
	now := r.at(time.Second)
	if c, ok := r.w.mercThrowPickAtLocked(ms, crab, now); !ok || c.pe.PatternID != testTransmutive || c.stock || c.carried != 0 {
		t.Errorf("its debuff throw %+v: want its conjured Transmutive Potion", c)
	}
	if !r.w.mercSelfPotionTurnLocked(ms, now) {
		t.Error("it took no pill of its own")
	}
	carried, stock := r.w.mercCarriedThrowsLocked(ms)
	for _, c := range append(carried, stock...) {
		switch c.pe.PatternID {
		case testPill, testTransmutive, testMiraculous:
			t.Errorf("throws its owner's %q", c.pe.ItemName)
		}
	}
	for _, d := range r.w.mercDraughtsLocked(ms) {
		switch d.pe.PatternID {
		case testPill, testTransmutive, testMiraculous:
			t.Errorf("drinks its owner's %q as a draught", d.pe.ItemName)
		}
	}
	r.w.mu.Unlock()
	// a fight with the crab: the stock's keepsakes stay
	r.run(time.Second, 20*time.Second)
	r.w.runMercNotes()
	st := r.w.MercStockOf(r.p.CharServerID)
	if st[testPill] != 1 || st[testTransmutive] != 1 || st[testMiraculous] != 1 {
		t.Errorf("after a fight its owner's stock %v, want one of each kept", st)
	}
	// a Metabolist hurt drinks its own draught, not its owner's
	r2, ms2 := keepsakeRig(t, 60, "Metabolist")
	r2.w.SetMercStock(r2.p.CharServerID, map[int]int{testVolatile: 20, testDraught: 20, testMiraculous: 1})
	r2.w.mu.Lock()
	ms2.npc.HP = ms2.npc.MaxHP / 2
	if !r2.w.mercSelfPotionTurnLocked(ms2, r2.at(time.Second)) {
		t.Error("the Metabolist drank no heal of its own")
	}
	r2.w.mu.Unlock()
	if st := r2.w.MercStockOf(r2.p.CharServerID); st[testMiraculous] != 1 {
		t.Errorf("the Metabolist's heal took its owner's: %v", st)
	}
}
