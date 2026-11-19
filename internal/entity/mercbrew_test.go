package entity

import (
	"slices"
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

// brewNotes counts the merc's brews as told (each a batch).
func (r *mercRig) brewNotes() *[]MercNote {
	var brewed []MercNote
	r.w.OnMercNote(func(n MercNote) {
		if n.Kind == MercNoteBrewed {
			brewed = append(brewed, n)
		}
	})
	return &brewed
}

func TestMercAlchemistBrewsIntoItsOwnersStock(t *testing.T) {
	r := newMercRig(t)
	alcSupplyPotions(r.w)
	r.w.SetMercBrew(10*time.Second, 3, -1)
	r.w.SetMercBrewBatch(1, 0, 0)
	ms := r.alchemist()
	brewed := r.brewNotes()
	// no stock read for its owner yet: nothing made
	r.run(0, 15*time.Second)
	r.w.runMercNotes()
	if len(*brewed) != 0 {
		t.Fatalf("made %d before its owner's stock was read", len(*brewed))
	}
	r.w.SetMercStock(r.p.CharServerID, map[int]int{testVolatile: 1})
	r.run(15100*time.Millisecond, 60*time.Second) // 45 s at ease: four at its rate, two past the ceiling of 3
	r.w.runMercNotes()
	st := r.w.MercStockOf(r.p.CharServerID)
	if st[testVolatile] != 3 || len(*brewed) != 2 {
		t.Fatalf("stock %v after 45 s with %d made, want the ceiling's 3 (2 made)", st, len(*brewed))
	}
	if (*brewed)[0].PatternID != testVolatile || (*brewed)[0].Owner != r.p {
		t.Errorf("made %+v", (*brewed)[0])
	}
	_ = ms
	// a hire's stock is its owner's: another alchemist of his sees it
	if r.w.TakeMercStock(r.p.CharServerID, testVolatile, 2) != 2 || r.w.MercStockOf(r.p.CharServerID)[testVolatile] != 1 {
		t.Error("the owner's withdrawal")
	}
	// a cap of 0: none brewed
	r.w.SetMercBrew(10*time.Second, 0, -1)
	r.run(60100*time.Millisecond, 90*time.Second)
	if st := r.w.MercStockOf(r.p.CharServerID); st[testVolatile] != 1 {
		t.Errorf("a cap of 0 brewed: %v", st)
	}
}

// It brews only at ease: its owner's walk pauses it (it follows), a fight
// pauses it, and once both stand the settle and its rate start again.
func TestMercAlchemistBrewsOnlyAtEase(t *testing.T) {
	r := newMercRig(t)
	alcSupplyPotions(r.w)
	r.w.SetMercBrew(5*time.Second, -1, -1)
	r.w.SetMercBrewBatch(1, 3*time.Second, 0)
	ms := r.alchemist()
	r.w.SetMercStock(r.p.CharServerID, map[int]int{})
	brewed := r.brewNotes()
	count := func() int {
		r.w.runMercNotes()
		return r.w.MercStockOf(r.p.CharServerID)[testVolatile]
	}
	// standing: its first 3 s settle, then one every 5 s (8, 13, 18 s)
	r.run(0, 7900*time.Millisecond)
	if n := count(); n != 0 {
		t.Fatalf("%d brewed in its settle and first 5 s", n)
	}
	r.run(8*time.Second, 19*time.Second)
	if n := count(); n != 3 {
		t.Fatalf("%d brewed standing 19 s, want 3 (8, 13, 18 s)", n)
	}
	// its owner walks for 20 s: it follows, and brews nothing
	walkFrom := 19100 * time.Millisecond
	for d := walkFrom; d <= 39*time.Second; d += 100 * time.Millisecond {
		r.w.mu.Lock()
		r.p.X += 0.4 // 4 a second, a walk
		r.w.SpatialIndex.Update(r.p.ServerID, r.p.X-0.4, r.p.Z, r.p.X, r.p.Z)
		r.w.mu.Unlock()
		r.tick(d)
	}
	if n := count(); n != 3 {
		t.Fatalf("%d brewed while its owner walked, want still 3", n)
	}
	if d := distXZ(ms.npc.X, ms.npc.Z, r.p.X, r.p.Z); d > 12 {
		t.Fatalf("it did not follow: %.1f off its owner", d)
	}
	// both stand again: it catches up, settles 3 s, and brews 5 s after
	r.run(39100*time.Millisecond, 60*time.Second)
	if n := count(); n < 4 {
		t.Fatalf("%d after standing again 21 s: brewing did not resume", n)
	}
	if ms.npc.moving() && !ms.idle.walking {
		t.Error("it still follows") // a stroll at ease is its idling
	}
	// a fight pauses it: a crab on its owner
	before := count()
	crab := r.creature("Crab", 28, 30)
	r.w.mu.Lock()
	crab.CurrentEnemy = r.p.ServerID
	r.w.wakeNPCLocked(crab)
	r.w.mu.Unlock()
	r.run(60100*time.Millisecond, 75*time.Second)
	if n := count(); n != before {
		t.Errorf("%d brewed with a creature on its owner", n-before)
	}
	r.w.mu.Lock()
	crab.CurrentEnemy = 0
	r.w.wakeNPCLocked(crab)
	r.w.mu.Unlock()
	r.run(75100*time.Millisecond, 90*time.Second)
	if n := count(); n <= before {
		t.Errorf("none brewed after the fight ended (%d)", n)
	}
	_ = brewed
}

// It fills a full stack of each potion it uses, its single-target potion,
// its cocktail and its draught, the least full first, and then stops.
func TestMercAlchemistBrewsFullStacks(t *testing.T) {
	r := newMercRig(t)
	vol := NewPotionEffect(testVolatile, "Volatile Potion", "Target, Decrease Hitpoints by 300", "", 0, "", 0, 0)
	vol.MaxStack = 60
	ck := NewPotionEffect(testCocktail, "Klik'Anon Cocktail", "Target, Decrease Hitpoints by 214", "", 0, "", 0, 10)
	ck.MaxStack = 40
	weak := NewPotionEffect(50013, "Icy Potion", "Target, Decrease Hitpoints by 10", "", 0, "", 0, 0) // a weaker single, never brewed
	weak.MaxStack = 2000
	d := NewPotionEffect(testDraught, "Draught", "Self, Decrease Hitpoints by 400, Increase Power per tic by 300, 18sec", "", 0, "", 0, 0)
	d.MaxStack = 25
	r.w.SetPotionEffects([]PotionEffect{vol, ck, weak, d})
	r.w.SetMercBrew(time.Second, -1, -1)
	r.w.SetMercBrewBatch(5, time.Second, 0)
	cocktail := bookSpell(700195, "Klik'Anon Cocktail", 0, 2*time.Second, 3*time.Second, 20, "Self, Summon Klik'Anon Cocktail x3")
	icy := bookSpell(700086, "Icy Mixture", 0, 2*time.Second, 3*time.Second, 20, "Self, Summon Icy Potion x3")
	refine := bookSpell(50, "Refinement", 0, 2*time.Second, 3*time.Second, 10, "Self, Summon Draught x3")
	k := r.kit(14, 0, 0, bookSpell(700085, "Volatile Mixture", 0, 2*time.Second, 3*time.Second, 20, "Self, Summon Volatile Potion x3"),
		cocktail, icy, refine)
	k.Cocktail = true
	ms := r.hire(k, MercAssist)
	r.w.SetMercStock(r.p.CharServerID, map[int]int{testVolatile: 58})
	brewed := r.brewNotes()
	r.w.mu.Lock()
	wants := r.w.mercBrewWantsLocked(ms)
	r.w.mu.Unlock()
	if len(wants) != 3 || wants[0].pe.PatternID != testVolatile || wants[1].pe.PatternID != testCocktail || wants[2].pe.PatternID != testDraught {
		t.Fatalf("keeps stacks of %+v, want its Volatile Potion, its cocktail and its draught", wants)
	}
	// (60-58) + 40 + 25 = 67 to brew, 5 a second: within 20 s at ease
	r.run(0, 30*time.Second)
	r.w.runMercNotes()
	st := r.w.MercStockOf(r.p.CharServerID)
	if st[testVolatile] != 60 || st[testCocktail] != 40 || st[testDraught] != 25 || st[50013] != 0 {
		t.Fatalf("stock %v, want full stacks of 60, 40 and 25 and no Icy Potion", st)
	}
	// the least full first: the nearly full Volatile Potion's 2 last
	if (*brewed)[0].PatternID == testVolatile {
		t.Errorf("brewed its nearly full stack first: %+v", (*brewed)[0])
	}
	last := len(*brewed)
	r.run(30100*time.Millisecond, 50*time.Second)
	r.w.runMercNotes()
	if len(*brewed) != last {
		t.Errorf("brewed %d more with every stack full", len(*brewed)-last)
	}
	// a throw from the stock opens a place, brewed again at ease
	r.w.mu.Lock()
	r.w.mercUsedLocked(ms, 0, true, testCocktail)
	r.w.mu.Unlock()
	r.run(50100*time.Millisecond, 55*time.Second)
	r.w.runMercNotes()
	if st := r.w.MercStockOf(r.p.CharServerID); st[testCocktail] != 40 {
		t.Errorf("the thrown cocktail not brewed again: %v", st)
	}
	// world.merc_brew_stack holds a stack lower
	r.w.SetMercBrewBatch(5, time.Second, 10)
	r.w.mu.Lock()
	for _, w := range r.w.mercBrewWantsLocked(ms) {
		if w.most != 10 {
			t.Errorf("%q's stack %d with merc_brew_stack 10", w.pe.ItemName, w.most)
		}
	}
	r.w.mu.Unlock()
}

// The stock is its owner's: a death, a dismissal and a new hire keep it,
// and the new hire brews on it and throws from it.
func TestMercAlchemistStockOutlivesDeathAndDismissal(t *testing.T) {
	r := newMercRig(t)
	alcSupplyPotions(r.w)
	r.w.SetMercBrew(time.Second, -1, -1)
	r.w.SetMercBrewBatch(3, time.Second, 0)
	ms := r.alchemist()
	r.w.SetMercStock(r.p.CharServerID, map[int]int{testCocktail: 7})
	r.run(0, 5*time.Second)
	had := r.w.MercStockOf(r.p.CharServerID)
	if had[testVolatile] == 0 || had[testCocktail] != 7 {
		t.Fatalf("stock %v after 5 s at ease", had)
	}
	// its death: the stock stays
	r.w.mu.Lock()
	ms.npc.HP, ms.npc.Dead = 0, true
	r.w.wakeNPCLocked(ms.npc)
	r.w.mu.Unlock()
	r.run(5100*time.Millisecond, 7*time.Second)
	if r.w.merc1(r.p.ServerID) != nil && !r.w.merc1(r.p.ServerID).npc.Dead {
		t.Fatal("its death did not take")
	}
	after := r.w.MercStockOf(r.p.CharServerID)
	if after[testVolatile] < had[testVolatile] || after[testCocktail] != 7 {
		t.Fatalf("its death took from the stock: %v, had %v", after, had)
	}
	// dismissed (or gone at its death): a new hire finds the stock as it was
	if r.w.merc1(r.p.ServerID) != nil {
		if _, ok := r.w.DismissMerc(r.sess, 1, r.at(7*time.Second)); !ok {
			t.Fatal("not dismissed")
		}
	}
	if st := r.w.MercStockOf(r.p.CharServerID); st[testVolatile] != after[testVolatile] || st[testCocktail] != 7 {
		t.Fatalf("its dismissal took from the stock: %v", st)
	}
	k := r.kit(14, 0, 0, bookSpell(700085, "Volatile Mixture", 0, 2*time.Second, 3*time.Second, 20, "Self, Summon Volatile Potion x3"))
	if _, why := r.w.HireMerc(r.sess, k, MercAssist, r.at(8*time.Second)); why != "" {
		t.Fatalf("the new hire refused: %s", why)
	}
	ms2 := r.w.merc1(r.p.ServerID)
	crab := r.creature("Crab", 28, 10)
	r.w.mu.Lock()
	c, ok := r.w.mercThrowPickLocked(ms2, crab)
	r.w.mu.Unlock()
	if !ok || !c.stock || c.pe.PatternID != testVolatile {
		t.Fatalf("the new hire throws %+v, want its owner's stock's Volatile Potion", c)
	}
	// an entity id used again by another login is not its stock
	r.w.mu.Lock()
	r.p.CharServerID += 1000
	r.w.mu.Unlock()
	if r.w.MercStockKnown(r.p.CharServerID) {
		t.Error("another character on the same entity id has the stock")
	}
}

// Both of the toggle's modes keep throwing while the stock holds potions,
// down to its last; then its path's own.
func TestMercAlchemistThrowsFromStockBothModes(t *testing.T) {
	for _, aoe := range []bool{false, true} {
		r := newMercRig(t)
		alcPotions(r.w)
		ck := bookSpell(700195, "Klik'Anon Cocktail", 0, 2*time.Second, 3*time.Second, 20, "Self, Summon Klik'Anon Cocktail x3")
		mix := bookSpell(700085, "Volatile Mixture", 0, 2*time.Second, 3*time.Second, 20, "Self, Summon Volatile Potion x3")
		k := r.kit(14, 0, 0, ck, mix)
		k.Cocktail, k.AoE = true, aoe
		ms := r.hire(k, MercAssist)
		r.w.SetMercStock(r.p.CharServerID, map[int]int{testVolatile: 3, testCocktail: 3})
		crab := r.creature("Crab", 28, 10)
		crab.HP, crab.MaxHP = 900000, 900000
		other := r.creature("Crab Two", 28, 12)
		other.HP, other.MaxHP = 900000, 900000
		crab.CurrentEnemy, other.CurrentEnemy = r.p.ServerID, r.p.ServerID // fighting its team: a cocktail may go
		want := testVolatile
		if aoe {
			want = testCocktail
		}
		for i := range 3 {
			r.w.mu.Lock()
			c, ok := r.w.mercThrowPickLocked(ms, crab)
			if ok && c.stock {
				r.w.mercUsedLocked(ms, 0, true, c.pe.PatternID)
			}
			r.w.mu.Unlock()
			if !ok || !c.stock || c.pe.PatternID != want {
				t.Fatalf("aoe %v, throw %d: %+v, want the stock's %d", aoe, i, c, want)
			}
		}
		// its own kind's stock spent: its path's own of its kind, never a failure
		r.w.mu.Lock()
		c, ok := r.w.mercThrowPickLocked(ms, crab)
		r.w.mu.Unlock()
		if !ok || c.stock || c.pe.PatternID != want {
			t.Fatalf("aoe %v, own kind's stock spent: %+v ok %v", aoe, c, ok)
		}
		// and in a real fight the stock goes down as it throws
		r.seeRig(ms.npc.ServerID, crab.ServerID)
		r.w.SetMercStock(r.p.CharServerID, map[int]int{want: 4})
		r.run(0, time.Second)
		ms.ordered = crab.ServerID
		r.run(1100*time.Millisecond, 12*time.Second)
		if left := r.w.MercStockOf(r.p.CharServerID)[want]; left >= 4 {
			t.Errorf("aoe %v: the fight threw none of the stock (%d left)", aoe, left)
		}
		if crab.HP >= crab.MaxHP {
			t.Errorf("aoe %v: nothing landed", aoe)
		}
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

// draughtRig is an alchemist with a refinement's draughts in a fight with a
// crab, a cleric merc beside it when healer; its draughts never resisted.
func draughtRig(t *testing.T, healer bool) (*mercRig, *mercState, *NPC) {
	r := newMercRig(t)
	alcSupplyPotions(r.w)
	saved := draughtIntn
	draughtIntn = func(int) int { return 9999 } // never resisted
	t.Cleanup(func() { draughtIntn = saved })
	refine := bookSpell(50, "Refinement", 0, 2*time.Second, 3*time.Second, 10, "Self, Summon Draught x3")
	ms := r.alchemist(refine)
	if healer {
		k := r.kit(9, 0, 0)
		k.Name = "Mirela"
		if _, why, _ := r.w.HireMercAt(r.sess, 2, k, MercPassive, r.at(0)); why != "" {
			t.Fatalf("the cleric's hire: %s", why)
		}
		r.w.mu.Lock()
		_, up := r.w.invites[r.p.ServerID][r.p.ServerID]
		r.w.mu.Unlock()
		if up {
			r.w.GroupAccept(r.sess, r.sess.PlayerObjectID())
		}
		r.w.mu.Lock()
		near := r.w.mercHealerNearLocked(ms)
		r.w.mu.Unlock()
		if !near {
			t.Fatal("the cleric merc is no healer near it")
		}
	}
	crab := r.creature("Crab", 28, 6)
	crab.HP, crab.MaxHP = 900000, 900000
	r.w.mu.Lock()
	crab.CurrentEnemy = ms.npc.ServerID
	r.w.wakeNPCLocked(crab)
	r.w.mu.Unlock()
	return r, ms, crab
}

// In a fight, low on power and hurt past the floor, it drinks with a healer
// in its team; never past the hard floor.
func TestMercAlchemistDrinksInAFightWithAHealer(t *testing.T) {
	r, ms, crab := draughtRig(t, true)
	m := ms.npc
	r.w.mu.Lock()
	defer r.w.mu.Unlock()
	if !r.w.mercInFightLocked(ms) || crab.CurrentEnemy != m.ServerID {
		t.Fatal("no fight")
	}
	m.Power = m.MaxPower / 10
	m.HP = m.MaxHP/2 + 100 // the draught's 400 takes it under half
	if !r.w.mercDrinkTurnLocked(ms, r.at(time.Second)) {
		t.Fatal("with a cleric beside it, it drank nothing in the fight")
	}
	if m.HP != m.MaxHP/2-300 {
		t.Errorf("health %d after the draught, want %d", m.HP, m.MaxHP/2-300)
	}
	r.w.dropNPCEffect(m, PotionEffectID(testDraught))
	m.HP = m.MaxHP*mercDraughtHardFloor/100 + 399 // 400 takes it under the hard floor
	if r.w.mercDrinkTurnLocked(ms, r.at(2*time.Second)) {
		t.Error("drank past the hard floor")
	}
}

// With no healer its floor holds, in a fight too.
func TestMercAlchemistDrinksNoDraughtPastItsFloorWithoutAHealer(t *testing.T) {
	r, ms, _ := draughtRig(t, false)
	m := ms.npc
	r.w.mu.Lock()
	defer r.w.mu.Unlock()
	m.Power = m.MaxPower / 10
	m.HP = m.MaxHP/2 + 100
	if r.w.mercDrinkTurnLocked(ms, r.at(time.Second)) {
		t.Error("no healer near: drank under its floor")
	}
	m.HP = m.MaxHP/2 + 401
	if !r.w.mercDrinkTurnLocked(ms, r.at(time.Second)) {
		t.Error("no draught with the health for it")
	}
}

// Its power drained faster than a draught gives: it drinks again as its
// power and what its draught has still to give fall low, not once a draught's
// whole time.
func TestMercAlchemistDrinksAgainAsPowerDrops(t *testing.T) {
	r, ms, _ := draughtRig(t, true)
	m := ms.npc
	var at []time.Duration
	for d := time.Duration(0); d <= 60*time.Second; d += 100 * time.Millisecond {
		r.w.mu.Lock()
		m.Power = max(m.Power-15, 0) // 150 a second: its spells and throws
		m.HP = m.MaxHP               // its cleric's heals
		if r.w.mercDrinkTurnLocked(ms, r.at(d)) {
			at = append(at, d)
		}
		r.w.mu.Unlock()
		r.tick(d)
	}
	// a draught gives 1,200 over its 18 s (300 a tick), short of the drain:
	// the next follows before the last's time is out, as its power runs low
	if len(at) < 5 {
		t.Fatalf("%d draughts in a minute (at %v), want more than one a draught's 18 s", len(at), at)
	}
	for i := 1; i < len(at); i++ {
		if gap := at[i] - at[i-1]; gap >= 18*time.Second {
			t.Errorf("draught %d came %v after the last: it waited out the draught's whole time", i, gap)
		}
	}
}

// Petition 2195 ("when you use a alc merc and change its class MC
// they dont make any of thore potions"): with its ten spell slots set (the
// window's, which never lists a refinement) an alchemist still brews its
// refinements' potions and throws them; a potion spell is still thrown only
// from its slots.
func TestMercAlchemistBrewsWithItsSlotsSet(t *testing.T) {
	r := newMercRig(t)
	alcSupplyPotions(r.w)
	r.w.SetMercBrew(time.Second, -1, -1)
	r.w.SetMercBrewBatch(5, time.Second, 0)
	refine := bookSpell(50, "Refinement", 0, 2*time.Second, 3*time.Second, 10, "Self, Summon Draught x3")
	fire := bookSpell(700606, "Freeport Fire Potion", 1, 2*time.Second, 3*time.Second, 20, "Target, Decrease Hitpoints by 900")
	frost := bookSpell(700607, "Frost Potion", 1, 2*time.Second, 3*time.Second, 20, "Target, Decrease Hitpoints by 800")
	k := r.kit(14, 0, 0, bookSpell(700085, "Volatile Mixture", 0, 2*time.Second, 3*time.Second, 20, "Self, Summon Volatile Potion x3"),
		refine, fire, frost)
	k.Loadout = []int{700606, 0, 0, 0, 0, 0, 0, 0, 0, 0} // Fire on a slot, Frost and the refinements on none
	ms := r.hire(k, MercAssist)
	r.w.SetMercStock(r.p.CharServerID, map[int]int{})
	r.w.mu.Lock()
	var names []string
	for _, c := range r.w.mercThrowablesLocked(ms) {
		names = append(names, c.pe.ItemName)
	}
	wants := r.w.mercBrewWantsLocked(ms)
	r.w.mu.Unlock()
	if !slices.Contains(names, "Volatile Potion") || !slices.Contains(names, "Freeport Fire Potion") || slices.Contains(names, "Frost Potion") {
		t.Fatalf("throws %v: want its Volatile Potion and its slotted Fire Potion, not the unslotted Frost Potion", names)
	}
	if len(wants) != 2 || wants[0].pe.PatternID != testVolatile || wants[1].pe.PatternID != testDraught {
		t.Fatalf("keeps stacks of %+v, want its Volatile Potion and its draught", wants)
	}
	r.run(0, 20*time.Second)
	r.w.runMercNotes()
	if st := r.w.MercStockOf(r.p.CharServerID); st[testVolatile] == 0 || st[testDraught] == 0 {
		t.Fatalf("stock %v with its slots set: want both its potion and its draught brewed", st)
	}
	// a refinement turned off in its menu is still not brewed
	r.w.mu.Lock()
	ms.kit.SpellsOff = []int{700085}
	wants = r.w.mercBrewWantsLocked(ms)
	r.w.mu.Unlock()
	if len(wants) != 1 || wants[0].pe.PatternID != testDraught {
		t.Errorf("with its Volatile Mixture turned off it keeps %+v", wants)
	}
}
