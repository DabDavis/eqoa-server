package entity

import (
	"testing"
	"time"
)

// The owner's asks of 2026-11-11 for the enchanter and the bard (mercharm.go,
// mercsongs.go): /merc charm charms the hirer's target as the merc's pet, in
// place of its summoned one; its time out it is let go and comes at the
// merc; /merc charm off lets it go; a creature over the charm's cap is
// refused (the control). A bard keeps its short group songs up, cast again
// as they run out, and its arts wait under half its power. An enchanter puts
// its proc on its group's players (not on a merc) and its power over time on
// them, and in a fight its power burst on the one low in power.

// charmTarget makes the creature the owner's target, on its first channel.
func (r *mercRig) charmTarget(n *NPC) {
	vs := GetOrCreateVisibility(r.sess)
	vs.ChannelEntity[1], vs.ChannelObjectID[1] = n.ServerID, 0x01312E97
	r.p.TargetObjectID = 0x01312E97
}

func TestMercCharm(t *testing.T) {
	defer func(f func(int) int) { charmIntn = f }(charmIntn)
	charmIntn = func(int) int { return 99 } // never resisted
	r := newMercRig(t)
	r.p.Level = 49
	charm := entrancing("Entrancing Enticement", 30, "Target, Charm ; Increase 25% based on CHA , 1min")
	ms := r.hire(r.kit(classEnchanter, 0, 0, charm), MercAssist)
	m := ms.npc
	big := r.creature("Hill Giant", 45, 10)
	r.charmTarget(big)
	if _, why := r.w.MercCharm(r.sess, r.at(0)); why != charmTooHigh {
		t.Fatalf("a level 45 at a cap of 28: %q", why)
	}
	crab := r.creature("Desert Crab", 20, 10)
	r.charmTarget(crab)
	line, why := r.w.MercCharm(r.sess, r.at(0))
	if why != "" || line == "" {
		t.Fatalf("/merc charm: %q %q", line, why)
	}
	r.run(0, 8*time.Second)
	if ms.pet == nil || ms.pet.npc != crab || crab.Charm == nil || crab.Owner != r.p.ServerID || !crab.isMercPet() {
		t.Fatalf("after the cast: pet %+v, charm %v, owner %d", ms.pet, crab.Charm, crab.Owner)
	}
	if _, why := r.w.MercCharm(r.sess, r.at(9*time.Second)); why != mercCharmHasOne {
		t.Errorf("a second charm: %q", why)
	}
	// its time out (a minute and its CHA share): let go, at the merc
	r.run(8100*time.Millisecond, 3*time.Minute)
	if crab.Charm != nil || crab.Owner != 0 || crab.isMercPet() || ms.pet != nil && ms.pet.npc == crab {
		t.Fatalf("after its time: charm %v, owner %d, merc's pet %v", crab.Charm, crab.Owner, crab.isMercPet())
	}
	if _, angry := crab.Threats[m.ServerID]; !angry && crab.CurrentEnemy != m.ServerID {
		t.Error("let go, the crab did not turn on the merc")
	}
	// /merc charm off
	crab2 := r.creature("Sand Crab", 20, 12)
	r.charmTarget(crab2)
	ms.ready = map[int]time.Time{}
	ms.busy = time.Time{}
	if _, why := r.w.MercCharm(r.sess, r.at(3*time.Minute+time.Second)); why != "" {
		t.Fatalf("a second crab: %q", why)
	}
	r.run(3*time.Minute+1100*time.Millisecond, 3*time.Minute+9*time.Second)
	if ms.pet == nil || ms.pet.npc != crab2 {
		t.Fatal("the second crab was not charmed")
	}
	if _, why := r.w.MercCharmOff(r.sess); why != "" || crab2.Charm != nil || ms.pet != nil {
		t.Errorf("/merc charm off: %q, charm %v", why, crab2.Charm)
	}
	if _, why := r.w.MercCharmOff(r.sess); why != mercCharmNoneOut {
		t.Errorf("off with none: %q", why)
	}
}

func TestMercBardKeepsSongsUp(t *testing.T) {
	r := newMercRig(t)
	anthem := bookSpell(800, "Anthem of Drive", 2, 2*time.Second, 5*time.Second, 50, "Group, Increase STR by 45 ; Increase STA by 45, 30sec")
	strike := bookSpell(801, "Nimble Strike", 1, 0, 6*time.Second, 30, "Target, Decrease Hitpoints by 50")
	strike.Range = 30
	ms := r.hire(r.kit(classBard, 0, 0, anthem, strike), MercAssist)
	var sung []time.Duration
	last := time.Time{}
	for d := time.Duration(0); d <= 90*time.Second; d += 100 * time.Millisecond {
		r.tick(d)
		if u := ms.ready[anthem.SpellID]; !u.Equal(last) {
			last = u
			sung = append(sung, d)
		}
	}
	// 30 s songs, cast again mercSongLead and its cast before they end
	if len(sung) < 3 {
		t.Fatalf("the anthem sung at %v over 90 s: want it kept up", sung)
	}
	if !buffOn(r.p.Effects, &anthem) || !buffOn(ms.npc.Effects, &anthem) {
		t.Error("the anthem is not on the owner and the bard")
	}
	// under half its power, no art
	crab := r.creature("Crab", 28, 4)
	r.w.mu.Lock()
	ms.npc.Power = ms.npc.MaxPower * (mercBardSpare - 10) / 100
	ms.ready = map[int]time.Time{}
	ok := r.w.mercAttackTurnLocked(ms, crab, r.at(2*time.Minute))
	r.w.mu.Unlock()
	if ok {
		t.Error("a bard under half its power cast an art")
	}
}

func TestMercEnchanterGivesItsTeam(t *testing.T) {
	r := newMercRig(t)
	rune := bookSpell(900, "Chaotic Rune", 1, time.Second, time.Second, 65, "Target, proc 280 , 5min")
	rune.Def.Effects = []SpellEffect{{Kind: EffectProc, Amount: 280}}
	clarity := bookSpell(901, "Clarity", 1, 2*time.Second, 3*time.Second, 20, "Target, Increase Power per tic by 10 ; Increase 5% based on INT, 15min")
	flow := bookSpell(902, "Power Flow", 1, 3*time.Second, 36*time.Second, 150, "Target, Increase Power per tic by 70 ; Increase 20% based on CHA, 24sec")
	_, bea := r.player("Bea", 13, 0)
	r.group(bea)
	k := r.kit(classEnchanter, 0, 0, rune, clarity, flow)
	ms := r.hire(k, MercAssist)
	if !mercGivesProc(&rune) || !mercGivesPower(&clarity) || !mercKeepsShort(classEnchanter, &flow) {
		t.Fatalf("proc %v power %v burst %v", mercGivesProc(&rune), mercGivesPower(&clarity), mercKeepsShort(classEnchanter, &flow))
	}
	r.run(0, 30*time.Second)
	for _, p := range []*Player{r.p, bea} {
		if !buffOn(p.Effects, &rune) || !buffOn(p.Effects, &clarity) {
			t.Errorf("%s: rune %v clarity %v", p.Name, buffOn(p.Effects, &rune), buffOn(p.Effects, &clarity))
		}
	}
	for _, e := range ms.npc.Effects {
		if e.SpellID == rune.SpellID {
			t.Error("a proc on the merc")
		}
	}
	// a fight, Bea low on power: its burst on her
	crab := r.creature("Crab", 28, 8)
	crab.HP, crab.MaxHP = 900000, 900000
	r.p.AttackTarget, r.p.AutoAttack = crab.ServerID, true
	bea.Power = bea.MaxPowerNow() / 5
	r.run(30*time.Second+100*time.Millisecond, 40*time.Second)
	if !buffOn(bea.Effects, &flow) {
		t.Error("Bea, low on power in a fight, got no Power Flow")
	}
}
