package entity

import (
	"testing"
	"time"
)

// Petition 1491 (the shard's GM Jibrish): "Correct it so mercenarys and healbot
// did not assist you in duels, pvp, or for mr moogies challenge". Each fight
// that is a player's own (ownFightLocked) by turns: in it Healbot heals
// nothing, a healer merc begins no heal, a heal of its begun before lands
// nothing, and its heal over time already on its owner ticks nothing; the
// control after each, the fight over: Healbot's 600 and the merc's heal.

// ownFights are the three fights: how each begins and ends on the rig's owner.
func ownFights(r *mercRig) []struct {
	name       string
	begin, end func()
} {
	_, foe := r.player("Foe", 0, 0)
	return []struct {
		name       string
		begin, end func()
	}{
		{"a duel", func() {
			r.w.duels.add(&Duel{ID: 1, A: r.p.ServerID, B: foe.ServerID, who: [2]*Player{r.p, foe}, Phase: duelOn})
		}, func() { r.w.duels.remove(r.w.duels.of[r.p.ServerID]) }},
		{"the PvP flag", func() { r.p.pvp.on = true }, func() { r.p.pvp.on = false }},
		{"Mr. Moogie's challenge", func() {
			r.w.metal = &metalDuel{npc: 999999, name: MetalMoogieName, c: &npcChallenge{player: r.p.ServerID, who: r.p, name: r.p.Name},
				began: r.at(0)}
		}, func() { r.w.metal = nil }},
	}
}

func TestHealbotStaysOutOfItsOwnersOwnFights(t *testing.T) {
	for i, f := range ownFights(newMercRig(t)) {
		r := newMercRig(t)
		f = ownFights(r)[i]
		r.callFamiliar(0)
		top := r.p.MaxHPNow()
		r.p.HP = top / 2
		f.begin()
		r.run(5*time.Second, 20*time.Second)
		if r.p.HP != top/2 {
			t.Errorf("%s: Healbot healed its owner %d", f.name, r.p.HP-top/2)
		}
		f.end()
		r.tick(20*time.Second + 100*time.Millisecond)
		if r.p.HP != min(top/2+600, top) {
			t.Errorf("%s over: Healbot did not heal (HP %d of %d)", f.name, r.p.HP, top)
		}
	}
}

func TestAMercStaysOutOfItsOwnersOwnFights(t *testing.T) {
	heal := bookSpell(700187, "Field Dress", 1, 3*time.Second, 19*time.Second, 180, "Target, Increase Hitpoints by 125 ; Increase 25% based on CHA")
	for i, f := range ownFights(newMercRig(t)) {
		r := newMercRig(t)
		f = ownFights(r)[i]
		ms := r.hire(r.kit(9, 0, 0, heal), MercAssist)
		top := r.p.MaxHPNow()
		r.p.HP = top / 2
		f.begin()
		r.run(0, 15*time.Second)
		if r.p.HP != top/2 || len(ms.ready) != 0 {
			t.Errorf("%s: the merc healed its owner %d (%d spells cast)", f.name, r.p.HP-top/2, len(ms.ready))
		}
		f.end()
		r.run(15*time.Second+100*time.Millisecond, 25*time.Second)
		if r.p.HP <= top/2 {
			t.Errorf("%s over: the merc did not heal its owner", f.name)
		}
	}
}

// A heal the merc began before the fight lands nothing in it.
func TestAMercsHealOnItsWayLandsNothingInAnOwnFight(t *testing.T) {
	heal := bookSpell(700187, "Field Dress", 1, 3*time.Second, 19*time.Second, 180, "Target, Increase Hitpoints by 125 ; Increase 25% based on CHA")
	for i, f := range ownFights(newMercRig(t)) {
		r := newMercRig(t)
		f = ownFights(r)[i]
		ms := r.hire(r.kit(9, 0, 0, heal), MercAssist)
		top := r.p.MaxHPNow()
		r.p.HP = top / 2
		d := time.Duration(0)
		for ; d < 5*time.Second && len(r.w.mercCasts) == 0; d += 100 * time.Millisecond {
			r.tick(d)
		}
		if len(r.w.mercCasts) != 1 || r.p.HP != top/2 {
			t.Fatalf("%s: no heal on its way before the fight (%d casts, %d spells cast)", f.name, len(r.w.mercCasts), len(ms.ready))
		}
		f.begin()
		r.run(d, d+8*time.Second)
		if len(r.w.mercCasts) != 0 || r.p.HP != top/2 {
			t.Errorf("%s: the heal on its way landed %d (%d casts left)", f.name, r.p.HP-top/2, len(r.w.mercCasts))
		}
	}
}

// A merc's heal over time put on before the fight ticks nothing in it, and
// goes on after; the control, the owner's own heal over time, ticks through.
func TestAMercsHealOverTimeWaitsOutAnOwnFight(t *testing.T) {
	hot := bookSpell(464, "Healing", 1, 3*time.Second, time.Second, 200, "Target, Increase HP per tic by 125 ; Increase 25% based on CHA, 60sec")
	for i, f := range ownFights(newMercRig(t)) {
		for _, own := range []bool{false, true} {
			r := newMercRig(t)
			f = ownFights(r)[i]
			ms := r.hire(r.kit(9, 0, 0), MercPassive)
			top := r.p.MaxHPNow()
			r.p.HP = top / 4
			r.w.mu.Lock()
			if own {
				cs := &castState{spell: hot, caster: r.p.ServerID, targets: []EntityID{r.p.ServerID}}
				r.w.landOnPlayerLocked(cs, r.p, r.sess, r.p, r.at(0))
			} else {
				r.w.mercHelpLocked(ms, &hot, r.p.ServerID, r.at(0))
			}
			r.w.mu.Unlock()
			if len(r.p.Effects) != 1 {
				t.Fatalf("%s: the heal over time is not on the owner (%d effects)", f.name, len(r.p.Effects))
			}
			f.begin()
			hp := r.p.HP
			r.run(100*time.Millisecond, 20*time.Second)
			if healed := r.p.HP > hp; healed != own {
				t.Errorf("%s, the owner's own cast %v: healed %d in the fight", f.name, own, r.p.HP-hp)
			}
			f.end()
			hp = r.p.HP
			r.run(20*time.Second+100*time.Millisecond, 40*time.Second)
			if r.p.HP <= hp {
				t.Errorf("%s over, the owner's own cast %v: the heal over time ticked nothing", f.name, own)
			}
		}
	}
}

// A merc of another of the group leaves a dueling member out of its heals too.
func TestAGroupMatesMercLeavesTheDuelistOut(t *testing.T) {
	heal := bookSpell(700187, "Field Dress", 1, 3*time.Second, 19*time.Second, 180, "Target, Increase Hitpoints by 125 ; Increase 25% based on CHA")
	r := newMercRig(t)
	_, mate := r.player("Mate", 0, 0)
	r.group(mate)
	ms := r.hire(r.kit(9, 0, 0, heal), MercAssist)
	_, foe := r.player("Foe", 0, 0)
	r.w.duels.add(&Duel{ID: 1, A: mate.ServerID, B: foe.ServerID, who: [2]*Player{mate, foe}, Phase: duelOn})
	top := mate.MaxHPNow()
	mate.HP = top / 2
	r.run(0, 15*time.Second)
	if mate.HP != top/2 || len(ms.ready) != 0 {
		t.Errorf("the owner's merc healed its dueling group mate %d", mate.HP-top/2)
	}
}
