package entity

import (
	"testing"
	"time"
)

// A heal on a full target is told whole (petition 2016): the caster's own on
// itself, its heal on a full friend (to both), a heal on a full pet; nothing
// is healed past the maximum.
func TestHealOnAFullTargetIsToldWhole(t *testing.T) {
	r := newTagRig(t)
	r.p.HP, r.friend.HP, r.pet.HP = r.p.MaxHP, r.friend.MaxHP, r.pet.MaxHP
	heal := SpellEffect{Kind: EffectHP, Amount: 120, AmountMax: 120}
	r.w.mu.Lock()
	r.w.healLocked(r.p, r.sess, r.p, 120, numberHeal)
	r.w.mu.Unlock()
	if got := figTags(t, r.sess); len(got) != 1 || got[0].src != r.me || got[0].dst != r.me || got[0].amount != 120 || got[0].tag != 6 {
		t.Errorf("a heal on itself at full: told %v, want (me, 120, me) tag 6", got)
	}
	r.w.mu.Lock()
	r.w.healLocked(r.p, r.sess, r.friend, 120, numberHeal)
	r.w.mu.Unlock()
	if got := figTags(t, r.sess); len(got) != 1 || got[0].dst != r.fid || got[0].amount != 120 || got[0].tag != 6 {
		t.Errorf("a heal on a full friend: the healer was told %v, want (me, 120, friend) tag 6", got)
	}
	if got := figTags(t, r.fsess); len(got) != 1 || got[0].src != r.meF || got[0].amount != 120 || got[0].tag != -1 {
		t.Errorf("a heal on a full friend: the friend was told %v, want (healer, 120, it) untagged", got)
	}
	cs := &castState{spell: CastSpell{SpellID: 9400, Name: "Test Pet Mend", Level: 20, Def: SpellDef{Effects: []SpellEffect{heal}}},
		caster: r.p.ServerID, targets: []EntityID{r.pet.ServerID}}
	r.w.mu.Lock()
	r.w.landOnPetLocked(cs, r.p, r.sess, r.pet, r.t0)
	r.w.mu.Unlock()
	if got := figTags(t, r.sess); len(got) != 1 || got[0].dst != r.petO || got[0].amount != 120 || got[0].tag != 6 {
		t.Errorf("a heal on its full pet: told %v, want (me, 120, pet) tag 6", got)
	}
	if r.p.HP != r.p.MaxHP || r.friend.HP != r.friend.MaxHP || r.pet.HP != r.pet.MaxHP {
		t.Errorf("HP %d/%d, %d/%d, pet %d/%d: healed past the maximum", r.p.HP, r.p.MaxHP, r.friend.HP, r.friend.MaxHP, r.pet.HP, r.pet.MaxHP)
	}
}

// A heal over time on a pet (Soul Mastery's "Increase HP per tic by 75",
// petition 2142) is told to its owner each tick, at full health too, once a
// tick; another player's on it to that player (tagged) and to the owner from
// that player (untagged).
func TestPetHoTTickIsToldToItsOwner(t *testing.T) {
	r := newTagRig(t)
	ae := &ActiveEffect{SpellID: 700684, Name: "Soul Mastery", Caster: r.p.ServerID, Until: r.later, NextTick: r.t0, HPTick: 75}
	r.pet.Effects = []*ActiveEffect{ae}
	r.pet.HP = 790
	r.w.mu.Lock()
	r.w.petEffectsLocked(r.ps, r.t0)
	r.w.petEffectsLocked(r.ps, r.t0.Add(time.Second)) // no tick due
	r.w.mu.Unlock()
	if got := figTags(t, r.sess); len(got) != 1 || got[0].src != r.me || got[0].dst != r.petO || got[0].amount != 75 || got[0].tag != 7 {
		t.Errorf("its tick: the owner was told %v, want one (me, 75, pet) tag 7", got)
	}
	if r.pet.HP != r.pet.MaxHP {
		t.Errorf("pet HP %d, want %d", r.pet.HP, r.pet.MaxHP)
	}
	r.w.mu.Lock()
	r.w.petEffectsLocked(r.ps, r.t0.Add(effectTick)) // at full health
	r.w.mu.Unlock()
	if got := figTags(t, r.sess); len(got) != 1 || got[0].amount != 75 || got[0].tag != 7 {
		t.Errorf("its tick at full health: told %v, want (me, 75, pet) tag 7", got)
	}

	// the friend's heal over time on the pet
	fv := GetOrCreateVisibility(r.fsess)
	fv.ChannelEntity[2], fv.ChannelObjectID[2] = r.pet.ServerID, 0x0150CCC0
	ae.Caster = r.friend.ServerID
	r.w.mu.Lock()
	r.w.petEffectsLocked(r.ps, r.t0.Add(2*effectTick))
	r.w.mu.Unlock()
	if got := figTags(t, r.fsess); len(got) != 1 || got[0].dst != 0x0150CCC0 || got[0].amount != 75 || got[0].tag != 7 {
		t.Errorf("the friend's tick on the pet: the friend was told %v, want (it, 75, pet) tag 7", got)
	}
	if got := figTags(t, r.sess); len(got) != 1 || got[0].src != r.fid || got[0].dst != r.petO || got[0].tag != -1 {
		t.Errorf("the friend's tick on the pet: the owner was told %v, want (friend, 75, pet) untagged", got)
	}
}

// A spell's HoT stat is told at the regen tick, its part alone, on a player
// and on a pet; natural regeneration (a 50th, a meal's) shows nothing.
func TestStatHoTIsToldNaturalRegenIsNot(t *testing.T) {
	r := newTagRig(t)
	regen := func() {
		r.w.mu.Lock()
		r.w.TickCount = 0
		r.w.regenSystem(r.t0)
		r.w.npcRegenSystem(r.t0)
		r.w.mu.Unlock()
	}
	r.p.HP, r.pet.HP = 100, 100
	r.p.Effects = []*ActiveEffect{{SpellID: FoodEffectID, Name: "Test Meal", Caster: r.p.ServerID, Until: r.later, Mods: StatMods{HoT: 30}}}
	r.p.Mods = sumEffectMods(r.p.Effects)
	regen()
	if got := figTags(t, r.sess); len(got) != 0 {
		t.Errorf("natural regeneration and a meal: told %v, want nothing", got)
	}
	if r.p.HP <= 100 || r.pet.HP <= 100 {
		t.Fatalf("no regen ran: HP %d, pet %d", r.p.HP, r.pet.HP)
	}

	r.p.Effects = append(r.p.Effects, &ActiveEffect{SpellID: 9401, Name: "Test Regrowth", Caster: r.p.ServerID, Until: r.later, Mods: StatMods{HoT: 40}})
	r.p.Mods = sumEffectMods(r.p.Effects)
	r.pet.Effects = []*ActiveEffect{{SpellID: 9402, Name: "Test Pet Regrowth", Caster: r.p.ServerID, Until: r.later, Mods: StatMods{HoT: 50}}}
	r.p.HP = r.p.MaxHP // at full health too
	regen()
	got := figTags(t, r.sess)
	if len(got) != 2 {
		t.Fatalf("a spell's HoT on the player and the pet: told %v, want two", got)
	}
	if f := got[0]; f.src != r.me || f.dst != r.me || f.amount != 40 || f.tag != 7 {
		t.Errorf("the player's: %v, want (me, 40, me) tag 7", f)
	}
	if f := got[1]; f.src != r.me || f.dst != r.petO || f.amount != 50 || f.tag != 7 {
		t.Errorf("the pet's: %v, want (me, 50, pet) tag 7", f)
	}
}
