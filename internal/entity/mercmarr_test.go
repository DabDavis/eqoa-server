package entity

import (
	"testing"
	"time"
)

// Heirloom petition 2660: a bard merc holding Marr's Blessing (a CM spell: a
// group's heal over time and +200 AC for two minutes) never cast it; read as
// a group heal it waited for its team under mercEmergencyPct. A heal over
// time that carries a lasting effect is a buff kept up (mercUseOf): a bard
// sings it as a song; a merc that keeps no two-minute buff casts it as the
// group heal it was (mercHealUseOf); a merc without it casts nothing of it.

func marrsBlessing() CastSpell {
	s := bookSpell(760121, "Marr's Blessing", 2, time.Second, 60*time.Second, 500,
		"Group, Increase HP per tic by 150 ; Increase 30% based on WIS, Increase AC by 200, Radius 30, 2min")
	s.Def.Effects = []SpellEffect{{Kind: EffectHPTick, Amount: 150, Scale: StatWis, ScalePct: 30}, {Kind: EffectStat, Stat: StatAC, Amount: 200}}
	s.Def.Duration = 2 * time.Minute
	return s
}

func TestMercKeepsMarrsBlessingUp(t *testing.T) {
	marr := marrsBlessing()
	if u := mercUseOf(&marr); u != useBuff {
		t.Fatalf("Marr's Blessing read as %q, want a buff", u.name())
	}
	// a bard holding it: on itself and its owner, all at full health
	r := newMercRig(t)
	ms := r.hire(r.kit(classBard, 10, 0, marr), MercAssist)
	r.run(0, 20*time.Second)
	if !buffOn(r.p.Effects, &marr) || !buffOn(ms.npc.Effects, &marr) {
		t.Fatalf("a bard holding Marr's Blessing: owner %v, merc %v", buffOn(r.p.Effects, &marr), buffOn(ms.npc.Effects, &marr))
	}
	// kept up past its two minutes
	r.run(20*time.Second+100*time.Millisecond, 3*time.Minute)
	if !buffOn(r.p.Effects, &marr) {
		t.Error("Marr's Blessing ran out on the owner: not kept up")
	}

	// the control: a bard without it puts none of it on anyone
	r2 := newMercRig(t)
	anthem := bookSpell(800, "Anthem of Drive", 2, 2*time.Second, 5*time.Second, 50, "Group, Increase STR by 45 ; Increase STA by 45, 30sec")
	ms2 := r2.hire(r2.kit(classBard, 10, 0, anthem), MercAssist)
	r2.run(0, 20*time.Second)
	if buffOn(r2.p.Effects, &marr) || buffOn(ms2.npc.Effects, &marr) {
		t.Error("a bard without Marr's Blessing put it up")
	}
	if !buffOn(r2.p.Effects, &anthem) {
		t.Error("the control bard sang nothing: the rig is not casting")
	}

	// a paladin keeps no two-minute buff: its group heal still, as before
	r3 := newMercRig(t)
	ms3 := r3.hire(r3.kit(2, 10, 0, marr), MercAssist)
	var use mercUse
	r3.w.mu.Lock()
	for _, sp := range ms3.mercBook() {
		if sp.s.SpellID == marr.SpellID {
			use = sp.use
		}
	}
	r3.w.mu.Unlock()
	if use != useGroupHeal {
		t.Errorf("a paladin's book holds Marr's Blessing as %q, want a group heal", use.name())
	}
}
