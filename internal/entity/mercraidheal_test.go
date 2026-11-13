package entity

import (
	"testing"
	"time"
)

// A healer merc in a raid (merccast.go, the owner, 2026-11-12: "let mercs heal raid
// members"): it heals the worst off of the whole raid its heals reach and it sees, its
// groups of four aside, and the next worst off when one is out of reach or sight; one out
// of reach or sight is never healed; a plain group is healed as before.

// mercHealRaid is the rig's owner with a healer merc of the spells given, leading a raid
// of it and five more players: the owner, A, B and C its group 1, D and E group 2.
func mercHealRaid(t *testing.T, spells ...CastSpell) (*mercRig, *mercState, *Group, []*Player) {
	t.Helper()
	r := newMercRig(t)
	ms := r.hire(r.kit(9, 0, 0, spells...), MercAssist)
	var ps []*Player
	for _, n := range []string{"A", "B", "C", "D", "E"} {
		_, p := r.player(n, 0, 0)
		ps = append(ps, p)
	}
	g := r.group(ps...)
	g.flagged = true
	r.w.mu.Lock()
	subs := r.w.subgroupsLocked(g)
	r.w.mu.Unlock()
	if !g.isRaid() || subs[r.p.ServerID] != 1 || subs[ps[3].ServerID] != 2 || subs[ps[4].ServerID] != 2 {
		t.Fatalf("the raid %v, groups %v", g.Members, subs)
	}
	return r, ms, g, ps
}

func mercHealSpell() CastSpell {
	return bookSpell(700187, "Field Dress", 1, 3*time.Second, 19*time.Second, 180, "Target, Increase Hitpoints by 125 ; Increase 25% based on CHA")
}

func mercGroupHealSpell() CastSpell {
	return bookSpell(700190, "Word of Health", 2, 3*time.Second, 19*time.Second, 300, "Group, Increase Hitpoints by 300, Radius 30")
}

// firstMercCast ticks from a to b until a merc's cast is on its way, and returns it.
func (r *mercRig) firstMercCast(a, b time.Duration) *mercCast {
	for d := a; d <= b; d += 100 * time.Millisecond {
		r.tick(d)
		if len(r.w.mercCasts) > 0 {
			return r.w.mercCasts[0]
		}
	}
	return nil
}

func hurtTo(p *Player, pct int) int {
	top := p.MaxHPNow()
	p.HP = top * pct / 100
	return top
}

// (a) The raid's worst off across the zone, out of its heal's reach, held the merc's heals
// from everyone: now it heals the worst off it reaches, of the other group of four, and
// a group heal goes up for two hurt in its reach though the worst off is out of it.
func TestMercHealsRaidPastItsGroup(t *testing.T) {
	r, ms, _, ps := mercHealRaid(t, mercHealSpell())
	far, e := ps[0], ps[4]
	far.X = 1500 // the raid's worst off, out of reach
	hurtTo(far, 10)
	top := hurtTo(e, 50)
	c := r.firstMercCast(0, 2*time.Second)
	if c == nil || c.target != e.ServerID {
		t.Fatalf("the merc's cast %+v: want its heal on E, of group 2 (the worst off out of reach)", c)
	}
	r.run(2*time.Second, 6*time.Second)
	if e.HP <= top/2 {
		t.Errorf("E was not healed: %d/%d", e.HP, top)
	}
	if far.HP != far.MaxHPNow()*10/100 {
		t.Errorf("the one out of reach was healed: %d", far.HP)
	}
	if len(ms.ready) == 0 {
		t.Error("no heal was cast")
	}

	// the group heal: two hurt in its reach, the worst off out of it
	gh := mercGroupHealSpell()
	if mercUseOf(&gh) != useGroupHeal {
		t.Fatalf("the group heal reads as %v", mercUseOf(&gh))
	}
	r2, _, _, ps2 := mercHealRaid(t, gh)
	ps2[0].X = 1500
	hurtTo(ps2[0], 10)
	hurtTo(ps2[3], 50)
	hurtTo(ps2[4], 50)
	c = r2.firstMercCast(0, 2*time.Second)
	if c == nil || !c.group {
		t.Fatalf("the merc's cast %+v: want its group heal", c)
	}
}

// (b) Its priority holds among those it reaches and sees: the worst off first (one hidden
// behind a wall passed over), one below mercEmergencyPct with its biggest heal.
func TestMercRaidHealKeepsItsPriority(t *testing.T) {
	small := mercHealSpell()
	big := bookSpell(700188, "Greater Healing", 1, 3*time.Second, 19*time.Second, 300, "Target, Increase Hitpoints by 600")
	r, _, _, ps := mercHealRaid(t, small, big)
	r.w.SetGround(sightGround{wallAt(990)})
	hidden := ps[0]
	hidden.X = 985 // the worst off, behind the wall
	hurtTo(hidden, 5)
	hurtTo(ps[2], 60)
	hurtTo(ps[4], 30) // group 2, the worst off it sees: an emergency
	c := r.firstMercCast(0, 2*time.Second)
	if c == nil || c.target != ps[4].ServerID {
		t.Fatalf("the merc's cast %+v: want its heal on E, the worst off it sees", c)
	}
	if c.spell.SpellID != big.SpellID {
		t.Errorf("an emergency's heal is %q, want its biggest", c.spell.Name)
	}
}

// (c) One out of reach or out of sight is not healed: with no other hurt the merc casts
// nothing; one that gets behind a wall while the heal is on its way is spared.
func TestMercRaidHealNeedsReachAndSight(t *testing.T) {
	r, _, _, ps := mercHealRaid(t, mercHealSpell())
	ps[3].X = 1500
	hurtTo(ps[3], 30)
	if c := r.firstMercCast(0, 2*time.Second); c != nil {
		t.Fatalf("a heal at one out of reach: %+v", c)
	}

	r, _, _, ps = mercHealRaid(t, mercHealSpell())
	r.w.SetGround(sightGround{wallAt(990)})
	ps[3].X = 985
	hurtTo(ps[3], 30)
	if c := r.firstMercCast(0, 2*time.Second); c != nil {
		t.Fatalf("a heal at one out of sight: %+v", c)
	}

	// in sight as it begins, behind the wall as it lands
	r, _, _, ps = mercHealRaid(t, mercHealSpell())
	r.w.SetGround(sightGround{wallAt(990)})
	e := ps[4]
	top := hurtTo(e, 30)
	c := r.firstMercCast(0, 2*time.Second)
	if c == nil || c.target != e.ServerID {
		t.Fatalf("the merc's cast %+v: want its heal on E", c)
	}
	e.X = 985
	r.run(2*time.Second, 6*time.Second)
	if e.HP != top*30/100 {
		t.Errorf("E behind the wall was healed: %d/%d", e.HP, top)
	}
}

// (d) A plain group of four is healed as before: the worst off first, then the next; two
// hurt, its group heal.
func TestMercPlainGroupHealUnchanged(t *testing.T) {
	r := newMercRig(t)
	_, a := r.player("A", 0, 0)
	_, b := r.player("B", 0, 0)
	g := r.group(a, b)
	ms := r.hire(r.kit(9, 0, 0, mercHealSpell()), MercAssist)
	if g.isRaid() || !groupHas(g, ms.npc.ServerID) || len(g.Members) != 4 {
		t.Fatalf("the group %v", g.Members)
	}
	hurtTo(a, 60)
	topB := hurtTo(b, 30)
	c := r.firstMercCast(0, 2*time.Second)
	if c == nil || c.target != b.ServerID {
		t.Fatalf("the merc's cast %+v: want its heal on B, the worst off", c)
	}
	r.run(2*time.Second, 6*time.Second)
	if b.HP <= topB*30/100 {
		t.Errorf("B was not healed: %d/%d", b.HP, topB)
	}

	r2 := newMercRig(t)
	_, a2 := r2.player("A", 0, 0)
	_, b2 := r2.player("B", 0, 0)
	r2.group(a2, b2)
	r2.hire(r2.kit(9, 0, 0, mercGroupHealSpell()), MercAssist)
	hurtTo(a2, 50)
	hurtTo(b2, 50)
	if c := r2.firstMercCast(0, 2*time.Second); c == nil || !c.group {
		t.Fatalf("the merc's cast %+v: want its group heal", c)
	}
}
