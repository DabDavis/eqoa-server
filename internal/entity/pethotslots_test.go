package entity

import (
	"testing"
	"time"
)

// Heirloom petition 3364 (a shaman on her mercs): "When I cast Celestial Heal, and then Radiance, Radiance will
// replace Celestial healing entirely". A pet's (or a merc's) heals over time go by slots as a player's: a group's
// heal ticks beside a single target's, and within a slot a stronger one holds a weaker one, which ticks again once
// the stronger has ended. Negative control: on the old rule (one heal over time a pet, the others taken off) the
// group's heal took the single target's off and the held one was gone.
func TestAPetsHoTsGoBySlots(t *testing.T) {
	r := newTagRig(t)
	r.w.mu.Lock()
	defer r.w.mu.Unlock()
	end := r.t0.Add(24 * time.Second)
	single := &ActiveEffect{SpellID: 700771, Name: "Celestial Healing", Caster: r.p.ServerID, Until: end, HPTick: 775, HoTSlot: 1}
	group := &ActiveEffect{SpellID: 900491, Name: "Radiance", Caster: r.p.ServerID, Until: end, HPTick: 900, HoTSlot: 3}
	if !r.w.placePetHoTLocked(r.pet, single, r.t0) {
		t.Fatal("the single target's heal did not go on")
	}
	r.pet.Effects = append(r.pet.Effects, single)
	if !r.w.placePetHoTLocked(r.pet, group, r.t0) {
		t.Fatal("the group's heal did not go on")
	}
	r.pet.Effects = append(r.pet.Effects, group)
	if len(r.pet.Effects) != 2 {
		t.Fatalf("effects after the group's heal: %d, want both heals", len(r.pet.Effects))
	}
	r.pet.MaxHP, r.pet.HP = 10000, 100
	r.w.petEffectsLocked(r.ps, r.t0)
	if r.pet.HP != 100+775+900 {
		t.Errorf("both ticked: HP %d, want %d", r.pet.HP, 100+775+900)
	}

	// within a slot: a weaker one is held, not taken off, and ticks again after the stronger
	weak := &ActiveEffect{SpellID: 700589, Name: "Divine Healing", Caster: r.p.ServerID, Until: r.t0.Add(time.Minute), HPTick: 600, HoTSlot: 1, NextTick: r.t0}
	r.pet.Effects = []*ActiveEffect{weak}
	strong := &ActiveEffect{SpellID: 700771, Name: "Celestial Healing", Caster: r.p.ServerID, Until: r.t0.Add(12 * time.Second), HPTick: 775, HoTSlot: 1}
	if !r.w.placePetHoTLocked(r.pet, strong, r.t0) {
		t.Fatal("the stronger heal did not go on")
	}
	r.pet.Effects = append(r.pet.Effects, strong)
	if len(r.pet.Effects) != 2 {
		t.Fatalf("the weaker one was taken off: %d effects", len(r.pet.Effects))
	}
	r.pet.HP = 100
	at := strong.NextTick
	r.w.petEffectsLocked(r.ps, at)
	if r.pet.HP != 100+775 {
		t.Errorf("held: HP %d, want the stronger's tick alone (%d)", r.pet.HP, 100+775)
	}
	r.pet.HP = 100
	later := strong.Until.Add(time.Second)
	for i := 0; i < 4 && r.pet.HP == 100; i++ {
		r.w.petEffectsLocked(r.ps, later.Add(time.Duration(i)*effectTick))
	}
	if r.pet.HP != 100+600 {
		t.Errorf("after the stronger ended: HP %d, want the weaker's tick (%d)", r.pet.HP, 100+600)
	}
}

// Heirloom petition 3364: "Tribal Ward Shaman MC spell doesn't seem to be triggering on mercs". A defensive proc's
// heal over time on its bearer goes on a pet or a merc at a creature's hit, as on a player, under the proc's id.
func TestAWardsHealProcsOnAPet(t *testing.T) {
	r := newTagRig(t)
	defer func(f func(int) int) { procIntn = f }(procIntn)
	procIntn = dice(0, 99)
	own := SpellEffect{Kind: EffectHPTick, Amount: 550, AmountMax: 550}
	own.Self = true
	r.w.mu.Lock()
	defer r.w.mu.Unlock()
	r.pet.MaxHP, r.pet.HP = 5000, 1000
	r.pet.Effects = []*ActiveEffect{{SpellID: 701246, Name: "Tribal Ward", Caster: r.p.ServerID, Until: r.later,
		ProcSpells: []ProcSpell{{Defense: true, Chance: 100, Duration: 18 * time.Second, Effects: []SpellEffect{own}}}}}
	r.w.npcDefenseHealsLocked(r.pet, r.t0)
	var proc *ActiveEffect
	for _, e := range r.pet.Effects {
		if e.SpellID == procSpellID(701246, 0) {
			proc = e
		}
	}
	if proc == nil || proc.HPTick != 550 {
		t.Fatalf("the ward's heal on the pet: %+v, want a heal over time of 550 under the proc's id", proc)
	}
	n := len(r.pet.Effects)
	r.w.npcDefenseHealsLocked(r.pet, r.t0.Add(time.Second)) // another hit renews it
	if len(r.pet.Effects) != n {
		t.Errorf("a second hit added an effect: %d, want %d", len(r.pet.Effects), n)
	}
	// no caster in the world: nothing (its amounts are its caster's)
	r.pet.Effects = r.pet.Effects[:1]
	r.pet.Effects[0].Caster = 4242
	r.w.npcDefenseHealsLocked(r.pet, r.t0)
	if len(r.pet.Effects) != 1 {
		t.Errorf("a ward of a caster gone fired: %d effects", len(r.pet.Effects))
	}
}
