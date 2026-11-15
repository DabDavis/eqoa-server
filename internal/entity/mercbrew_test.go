package entity

import (
	"testing"
	"time"
)

// An alchemist merc's supply (mercbrew.go): it makes its path's potion into
// its owner's stock at the brew rate up to the cap; it throws what it
// carries first, then the stock, then its path's own; it drinks a draught
// when its power is low, never below the health floor; it eats out of a
// fight when hurt.

const (
	testDraught = 50025
	testMeal    = 777001
)

func alcSupplyPotions(w *World) {
	alcPotions(w)
	d := NewPotionEffect(testDraught, "Draught", "Self, Decrease Hitpoints by 400, Increase Power per tic by 300, 18sec", "", 0, "", 0, 0)
	w.NotePotionEffect(d)
}

func (r *mercRig) alchemist(spells ...CastSpell) *mercState {
	mix := bookSpell(700085, "Volatile Mixture", 0, 2*time.Second, 3*time.Second, 20, "Self, Summon Volatile Potion x3")
	return r.hire(r.kit(14, 0, 0, append([]CastSpell{mix}, spells...)...), MercAssist)
}

func TestMercAlchemistBrewsIntoItsOwnersStock(t *testing.T) {
	r := newMercRig(t)
	alcSupplyPotions(r.w)
	r.w.SetMercBrew(10*time.Second, 3, -1)
	ms := r.alchemist()
	var brewed []MercNote
	r.w.OnMercNote(func(n MercNote) {
		if n.Kind == MercNoteBrewed {
			brewed = append(brewed, n)
		}
	})
	// no stock read for its owner yet: nothing made
	r.run(0, 15*time.Second)
	r.w.runMercNotes()
	if len(brewed) != 0 {
		t.Fatalf("made %d before its owner's stock was read", len(brewed))
	}
	r.w.SetMercStock(r.p.CharServerID, map[int]int{testVolatile: 1})
	r.run(15100*time.Millisecond, 60*time.Second) // 45 s: four or five at its rate, two past the cap of 3
	r.w.runMercNotes()
	st := r.w.MercStockOf(r.p.CharServerID)
	if st[testVolatile] != 3 || len(brewed) != 2 {
		t.Fatalf("stock %v after 45 s with %d made, want the cap's 3 (2 made)", st, len(brewed))
	}
	if brewed[0].PatternID != testVolatile || brewed[0].Owner != r.p {
		t.Errorf("made %+v", brewed[0])
	}
	_ = ms
	// a hire's stock is its owner's: another alchemist of his sees it
	if r.w.TakeMercStock(r.p.CharServerID, testVolatile, 2) != 2 || r.w.MercStockOf(r.p.CharServerID)[testVolatile] != 1 {
		t.Error("the owner's withdrawal")
	}
}

// What it throws: what it carries, then the stock (each one used up and
// told), then its path's own (never used up).
func TestMercAlchemistThrowsWhatItCarriesFirst(t *testing.T) {
	r := newMercRig(t)
	alcSupplyPotions(r.w)
	hot := NewPotionEffect(50013, "Icy Potion", "Target, Decrease Hitpoints by 10", "", 0, "", 0, 0) // a weaker one, given it
	r.w.NotePotionEffect(hot)
	ms := r.alchemist()
	ms.kit.Carried = []MercCarried{{ID: 91, PatternID: 50013, Count: 1, Name: "Icy Potion"}}
	r.w.SetMercStock(r.p.CharServerID, map[int]int{50013: 1})
	crab := r.creature("Crab", 28, 10)
	var used []MercNote
	r.w.OnMercNote(func(n MercNote) { used = append(used, n) })
	pick := func() mercThrowable {
		r.w.mu.Lock()
		defer r.w.mu.Unlock()
		c, _ := r.w.mercThrowPickLocked(ms, crab)
		return c
	}
	if c := pick(); c.carried != 91 {
		t.Fatalf("throws %+v, want the Icy Potion it carries", c.pe.ItemName)
	}
	r.w.mu.Lock()
	r.w.mercUsedLocked(ms, 91, false, 50013)
	r.w.mu.Unlock()
	if c := pick(); !c.stock || c.pe.PatternID != 50013 {
		t.Fatalf("its carried one used: throws %+v, want the stock's", c)
	}
	r.w.mu.Lock()
	r.w.mercUsedLocked(ms, 0, true, 50013)
	r.w.mu.Unlock()
	if c := pick(); c.stock || c.carried != 0 || c.pe.PatternID != testVolatile {
		t.Fatalf("none carried or in stock: throws %+v, want its path's Volatile Potion", c)
	}
	r.w.runMercNotes()
	if len(used) != 2 || used[0].Kind != MercNoteUsed || used[0].ItemID != 91 || used[1].Kind != MercNoteStockUsed {
		t.Fatalf("told %+v", used)
	}
	if len(ms.kit.Carried) != 0 {
		t.Error("its last carried potion is still carried")
	}
}

func TestMercAlchemistDrinksADraughtForPower(t *testing.T) {
	r := newMercRig(t)
	alcSupplyPotions(r.w)
	refine := bookSpell(50, "Refinement", 0, 2*time.Second, 3*time.Second, 10, "Self, Summon Draught x3")
	ms := r.alchemist(refine)
	m := ms.npc
	r.run(0, time.Second)
	saved := draughtIntn
	draughtIntn = func(int) int { return 9999 } // never resisted
	t.Cleanup(func() { draughtIntn = saved })
	r.w.mu.Lock()
	m.Power = m.MaxPower / 10
	hp, pw := m.HP, m.Power
	drank := r.w.mercDrinkTurnLocked(ms, r.at(2*time.Second))
	r.w.mu.Unlock()
	if !drank {
		t.Fatal("at a tenth of its power it drank nothing")
	}
	if m.HP != hp-400 {
		t.Errorf("health %d after the draught, want %d (its 400)", m.HP, hp-400)
	}
	r.run(2100*time.Millisecond, 15*time.Second)
	if m.Power < pw+300 {
		t.Errorf("power %d 13 s after the draught, from %d: none came", m.Power, pw)
	}
	// a second while the first works: none
	r.w.mu.Lock()
	m.Power = m.MaxPower / 10
	again := r.w.mercDrinkTurnLocked(ms, r.at(16*time.Second))
	r.w.mu.Unlock()
	if again {
		t.Error("drank a second draught while the first worked")
	}
}

func TestMercAlchemistDrinksNoDraughtBelowItsFloor(t *testing.T) {
	r := newMercRig(t)
	alcSupplyPotions(r.w)
	refine := bookSpell(50, "Refinement", 0, 2*time.Second, 3*time.Second, 10, "Self, Summon Draught x3")
	ms := r.alchemist(refine)
	m := ms.npc
	r.w.SetMercBrew(-1, -1, 50)
	r.w.mu.Lock()
	defer r.w.mu.Unlock()
	m.Power = 0
	m.HP = m.MaxHP/2 + 399 // 400 would take it under half
	if r.w.mercDrinkTurnLocked(ms, r.at(time.Second)) {
		t.Error("drank a draught that left it under its floor")
	}
	m.HP = m.MaxHP/2 + 401
	if !r.w.mercDrinkTurnLocked(ms, r.at(time.Second)) {
		t.Error("no draught with the health for it")
	}
}

func TestMercAlchemistEatsOutOfAFight(t *testing.T) {
	r := newMercRig(t)
	alcSupplyPotions(r.w)
	ms := r.alchemist()
	m := ms.npc
	r.run(0, time.Second)
	crab := r.creature("Crab", 28, 10)
	r.w.mu.Lock()
	m.HP = m.MaxHP / 2
	crab.CurrentEnemy = m.ServerID // a fight: no meal
	inFight := r.w.mercEatTurnLocked(ms, r.at(2*time.Second))
	crab.CurrentEnemy = 0
	ms.target, ms.ordered = 0, 0
	ms.kit.Carried = []MercCarried{{ID: 5, PatternID: testMeal, Count: 2, Name: "Spiced Jerky", Food: true, HoT: 40}}
	hp := m.HP
	ate := r.w.mercEatTurnLocked(ms, r.at(3*time.Second))
	r.w.mu.Unlock()
	if inFight {
		t.Error("ate in a fight")
	}
	if !ate || mercEating(m) == nil {
		t.Fatal("hurt out of a fight, it did not eat")
	}
	if m.HP != hp+40 {
		t.Errorf("health %d after the meal's first tick, want %d", m.HP, hp+40)
	}
	if ms.kit.Carried[0].Count != 1 {
		t.Error("the jerky was not eaten from its pack")
	}
	// a ration when it carries none
	r2 := newMercRig(t)
	alcSupplyPotions(r2.w)
	ms2 := r2.alchemist()
	r2.w.mu.Lock()
	ms2.npc.HP = ms2.npc.MaxHP / 3
	ok := r2.w.mercEatTurnLocked(ms2, r2.at(time.Second))
	r2.w.mu.Unlock()
	if ae := mercEating(ms2.npc); !ok || ae == nil || ae.Mods.HoT != max(ms2.body.MaxHPNow()/RationShare, ms2.body.Level) {
		t.Errorf("no ration: ate %v %+v", ok, ae)
	}
}

// A fight's throws and casts together: tested in TestMercAlchemistThrowsItsPotions
// (Range1 throws and Fire Spray casts in one fight).
