package entity

import (
	"testing"
	"time"
)

// Petition 1639 (the magician Waldorf, 2026-11-11): "Ghilan Noble doesn't seem
// to be casting her giant heal anymore. If she is, it's frequency was greatly
// reduced." Her tide (target group, below 100) is wanted whenever anyone of her
// side is a little hurt; her Lay Hands and Mending (either, below 50 and 60)
// only when her owner or she is low. A single heal wanted now goes before the
// tide whatever their order in the row, and with world.pet_heal_group a group
// mate's low hit points want it too.

// ghilanTurn is the Ghilan Noble's turn with every heal of hers due: the spell of
// what she began, 0 for nothing.
func ghilanTurn(t *testing.T, on, tideFirst bool, ownerHP, mateHP int) int {
	t.Helper()
	r := pets1110Rig(t)
	pet, _ := r.summon(t, summonGhilanNoble, 0)
	ps := r.w.pets[r.p.ServerID]
	if tideFirst {
		ab := ps.tmpl.tmpl.abilities
		ps.tmpl.tmpl.abilities = append([]templateAbility{ab[6]}, ab[:6]...)
	}
	r.w.SetPetHealGroup(on)
	mate := &Player{ServerID: 2001, Name: "Mate", Level: 60, HP: mateHP, MaxHP: 4000, X: r.p.X + 5, Y: r.p.Y, Z: r.p.Z,
		World: r.p.World}
	r.w.Players[mate.ServerID] = mate
	r.w.SpatialIndex.Insert(mate.ServerID, mate.X, mate.Z)
	if r.w.groups == nil {
		r.w.groups = map[uint32]*Group{}
	}
	r.w.groups[9] = &Group{ID: 9, Members: []EntityID{r.p.ServerID, mate.ServerID}}
	r.p.GroupID, mate.GroupID = 9, 9
	r.p.MaxHP, r.p.HP = 4000, ownerHP
	pet.HP = pet.MaxHP
	now := r.at(120 * time.Second)
	r.w.mu.Lock()
	defer r.w.mu.Unlock()
	f := ps.petAbilityClocks(0, now)
	for i, a := range ps.petAbilities() {
		f.next[i] = now.Add(time.Hour) // her buff and nukes not due: her heals alone
		if a.Kind == "heal" {
			f.next[i] = now.Add(-time.Second)
		}
	}
	f.busy, f.chain = time.Time{}, 0
	r.w.petCasts = nil
	r.w.petTemplateTurnLocked(ps, nil, now)
	if len(r.w.petCasts) == 0 {
		return 0
	}
	return int(r.w.petCasts[0].spell.SpellID)
}

const (
	spellLayHands = 905457
	spellMending  = 905456
	spellTide     = 905495
)

// Her owner at 30%, the group a little hurt: Lay Hands, not the tide, either
// order of the row, the switch on or off.
func TestGhilanNoblesBigHealGoesBeforeHerTide(t *testing.T) {
	for _, on := range []bool{false, true} {
		for _, first := range []bool{false, true} {
			if got := ghilanTurn(t, on, first, 1200, 3900); got != spellLayHands {
				t.Errorf("switch %v, tide first %v: she began %d, want Lay Hands (%d)", on, first, got, spellLayHands)
			}
		}
	}
}

// Everyone a little hurt: the tide.
func TestGhilanNoblesTideWhenNoOneIsLow(t *testing.T) {
	for _, on := range []bool{false, true} {
		if got := ghilanTurn(t, on, false, 3500, 3900); got != spellTide {
			t.Errorf("switch %v: she began %d, want the tide (%d)", on, got, spellTide)
		}
	}
}

// A group mate at 30%, her owner at 90%: with the switch her Lay Hands goes to
// the mate; without it only the tide is hers to give (her single heals are her
// owner's and her own).
func TestGhilanNoblesBigHealOnALowGroupMate(t *testing.T) {
	if got := ghilanTurn(t, true, false, 3600, 1200); got != spellLayHands {
		t.Errorf("switch on: she began %d, want Lay Hands (%d)", got, spellLayHands)
	}
	if got := ghilanTurn(t, false, false, 3600, 1200); got != spellTide {
		t.Errorf("switch off: she began %d, want the tide (%d)", got, spellTide)
	}
}
