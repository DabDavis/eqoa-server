package entity

import (
	"slices"
	"testing"
	"time"
)

// Petition 2216 (mercselfpotion.go): an alchemist merc's Master Class
// mixtures are used. A Transmuter takes its Transformative Pill (its stats,
// not its ogre's look) and throws its Transmutive Potion first at a creature
// fighting its team; a Metabolist drinks its Miraculous Draught when hurt;
// an alchemist with the same mixtures in its book and no Master Class (every
// alchemist's book holds them by 60) uses none of them.

const (
	testPill        = 709301
	testTransmutive = 50106
	testMiraculous  = 1203
)

func alcMasterPotions(w *World) {
	alcPotions(w)
	pill := NewPotionEffect(testPill, "Transformative Pill", "Self, Increase Hitpoints by 1250, Increase STR by 150, Increase STA by 150, Increase Maximum HP by 1250, 30min", "", 0, "", 0, 0)
	pill.Def.Form, pill.Def.FormKind = "ogre", FormSelf
	pill.Def.Effects = append(pill.Def.Effects, SpellEffect{Kind: EffectIllusion})
	tm := NewPotionEffect(testTransmutive, "Transmutive Potion", "Target, Decrease STR by 125, Decrease STA by 125, Decrease AC by 300, 24sec", "", 0, "", 0, 0)
	mir := NewPotionEffect(testMiraculous, "Miraculous Draught", "Self, Increase HP per tic by 650, 24sec", "", 0, "", 0, 0)
	for _, pe := range []PotionEffect{pill, tm, mir} {
		w.NotePotionEffect(pe)
	}
}

var (
	testCreative     = bookSpell(700772, "Create Transformative", 0, 3*time.Second, 3*time.Second, 500, "Self, Summon Transformative Pill x3")
	testTransmixture = bookSpell(700827, "Transmutive Mixture", 0, 2*time.Second, 3*time.Second, 500, "Self, Summon Transmutive Potion")
	testMiracleMix   = bookSpell(700802, "Miraculous Mixture", 0, 2*time.Second, 3*time.Second, 0, "Self, Summon Miraculous Draught x3")
)

// masterAlchemist hires an alchemist with the three mixtures in its book (as
// classspells gives them) and the ranks that add those given.
func (r *mercRig) masterAlchemist(master string, adds ...int) *mercState {
	mix := bookSpell(700085, "Volatile Mixture", 0, 2*time.Second, 3*time.Second, 20, "Self, Summon Volatile Potion x3")
	k := r.kit(14, 0, 0, mix, testCreative, testTransmixture, testMiracleMix)
	if master != "" {
		k.Master = master
		first := &CMRank{ID: 1, Name: master, Category: CMCategoryMaster}
		k.MasterRanks = []*CMRank{first}
		for i, id := range adds {
			k.MasterRanks = append(k.MasterRanks, &CMRank{ID: 2 + i, Parent: 1, Category: CMCategoryMaster, Adds: []CMAbility{{SpellID: id}}})
		}
	}
	return r.hire(k, MercAssist)
}

func TestMercMasterMixturesOnlyItsMasterClasss(t *testing.T) {
	r := newMercRig(t)
	alcMasterPotions(r.w)
	for _, c := range []struct {
		master                string
		adds                  []int
		buffs, heals, debuffs []int
	}{
		{"", nil, nil, nil, nil},
		{"Sage", []int{900783}, nil, nil, nil},
		{"Transmuter", []int{700772, 700827}, []int{testPill}, nil, []int{testTransmutive}},
		{"Metabolist", []int{700802}, nil, []int{testMiraculous}, nil},
	} {
		k := r.kit(14, 0, 0, testCreative, testTransmixture, testMiracleMix)
		for i, id := range c.adds {
			k.MasterRanks = append(k.MasterRanks, &CMRank{ID: 2 + i, Adds: []CMAbility{{SpellID: id}}})
		}
		b, h, d := r.w.MercPotionUsesFor(k)
		if !slices.Equal(b, c.buffs) || !slices.Equal(h, c.heals) || !slices.Equal(d, c.debuffs) {
			t.Errorf("%q: buffs %v heals %v debuffs %v, want %v %v %v", c.master, b, h, d, c.buffs, c.heals, c.debuffs)
		}
		// turned off in its menu: none
		k.SpellsOff = []int{700772, 700827, 700802}
		if b, h, d := r.w.MercPotionUsesFor(k); len(b)+len(h)+len(d) != 0 {
			t.Errorf("%q with its mixtures off: %v %v %v", c.master, b, h, d)
		}
	}
	// another class with the ranks: none
	k := r.kit(13, 0, 0, testCreative, testTransmixture)
	k.MasterRanks = []*CMRank{{ID: 2, Adds: []CMAbility{{SpellID: 700772}, {SpellID: 700827}}}}
	if b, h, d := r.w.MercPotionUsesFor(k); len(b)+len(h)+len(d) != 0 {
		t.Errorf("a wizard uses an alchemist's mixtures: %v %v %v", b, h, d)
	}
}

func TestMercTransmuterTakesItsPill(t *testing.T) {
	r := newMercRig(t)
	alcMasterPotions(r.w)
	ms := r.masterAlchemist("Transmuter", 700772, 700827)
	m := ms.npc
	r.w.mu.Lock()
	defer r.w.mu.Unlock()
	top, str := m.MaxHP, ms.body.StatNow(StatStr)
	look := m.ModelID
	if !r.w.mercSelfPotionTurnLocked(ms, r.at(time.Second)) {
		t.Fatal("it took no pill")
	}
	if !npcHasEffect(m, PotionEffectID(testPill)) {
		t.Fatal("the pill is not on it")
	}
	if m.MaxHP < top+1250 {
		t.Errorf("its most health %d after the pill, from %d: want 1250 more", m.MaxHP, top)
	}
	if got := ms.body.StatNow(StatStr); got < str+150 {
		t.Errorf("STR %d after the pill, from %d", got, str)
	}
	if m.ModelID != look {
		t.Errorf("its look changed: model %#x, was %#x", m.ModelID, look)
	}
	for _, e := range m.Effects {
		if e.SpellID == PotionEffectID(testPill) && e.Form != 0 {
			t.Errorf("the pill put its look %#x on it", e.Form)
		}
	}
	if r.w.mercSelfPotionTurnLocked(ms, r.at(2*time.Second)) {
		t.Error("a second pill while the first is on it")
	}
	// one with no Master Class takes none
	r2 := newMercRig(t)
	alcMasterPotions(r2.w)
	plain := r2.masterAlchemist("")
	r2.w.mu.Lock()
	defer r2.w.mu.Unlock()
	if r2.w.mercSelfPotionTurnLocked(plain, r2.at(time.Second)) {
		t.Error("an alchemist with no Master Class took a Master Class's potion")
	}
}

func TestMercTransmuterThrowsItsDebuffFirst(t *testing.T) {
	r := newMercRig(t)
	alcMasterPotions(r.w)
	ms := r.masterAlchemist("Transmuter", 700772, 700827)
	crab := r.creature("Crab", 28, 10)
	r.w.mu.Lock()
	defer r.w.mu.Unlock()
	now := r.at(time.Second)
	// not fighting its team: it pulls nothing with a debuff
	if c, ok := r.w.mercThrowPickAtLocked(ms, crab, now); !ok || c.pe.PatternID != testVolatile {
		t.Fatalf("at a creature fighting no one it throws %q, want the Volatile Potion", c.pe.ItemName)
	}
	crab.CurrentEnemy = r.p.ServerID
	c, ok := r.w.mercThrowPickAtLocked(ms, crab, now)
	if !ok || c.pe.PatternID != testTransmutive || !c.debuff {
		t.Fatalf("at a creature fighting its owner it throws %q, want the Transmutive Potion first", c.pe.ItemName)
	}
	// it lands its debuff, then its throws are its nukes'
	s := c.pe.castSpell()
	r.w.mercHarmLocked(ms, &s, crab, now)
	if !npcHasEffect(crab, PotionEffectID(testTransmutive)) {
		t.Fatal("the debuff did not stay on the crab")
	}
	if c, _ := r.w.mercThrowPickAtLocked(ms, crab, now); c.pe.PatternID != testVolatile {
		t.Errorf("with the debuff on it throws %q, want the Volatile Potion", c.pe.ItemName)
	}
	// thrown a moment ago (the crab took none of it): not again so soon
	crab.Effects = nil
	ms.ready[PotionEffectID(testTransmutive)] = now.Add(10 * time.Second)
	if c, _ := r.w.mercThrowPickAtLocked(ms, crab, now.Add(5*time.Second)); c.pe.PatternID != testVolatile {
		t.Errorf("5 s after a debuff it throws %q again", c.pe.ItemName)
	}
	if c, _ := r.w.mercThrowPickAtLocked(ms, crab, now.Add(11*time.Second)); c.pe.PatternID != testTransmutive {
		t.Errorf("11 s after it throws %q, want the debuff again", c.pe.ItemName)
	}
}

func TestMercMetabolistDrinksItsDraughtWhenHurt(t *testing.T) {
	r := newMercRig(t)
	alcMasterPotions(r.w)
	ms := r.masterAlchemist("Metabolist", 700802)
	m := ms.npc
	r.w.mu.Lock()
	if r.w.mercSelfPotionTurnLocked(ms, r.at(time.Second)) {
		t.Fatal("drank its heal at full health")
	}
	m.HP = m.MaxHP / 2
	if !r.w.mercSelfPotionTurnLocked(ms, r.at(time.Second)) {
		t.Fatal("drank no heal at half its health")
	}
	if !npcHasEffect(m, PotionEffectID(testMiraculous)) {
		t.Fatal("the draught is not on it")
	}
	if r.w.mercSelfPotionTurnLocked(ms, r.at(2*time.Second)) {
		t.Error("a second draught while the first works")
	}
	hp := m.HP
	r.w.mu.Unlock()
	r.run(2*time.Second, 14*time.Second)
	if m.HP <= hp {
		t.Errorf("health %d after 12 s of the draught, from %d", m.HP, hp)
	}
}
