package entity

import (
	"testing"
	"time"
)

// A merc helps its owner's own alone (merccast.go, the owner, 2026-11-13: "the mercs that
// belong to a player only can effect that player and their mercs, not the whole raid"):
// its heals, its group heal and its group buffs land on itself, its owner and its owner's
// other mercs, never another player of the group or raid nor their mercs. A player's own
// group spell still reaches the others' mercs (petition 1909, groupMercsLocked).

func mercHealSpell() CastSpell {
	return bookSpell(700187, "Field Dress", 1, 3*time.Second, 19*time.Second, 180, "Target, Increase Hitpoints by 125 ; Increase 25% based on CHA")
}

func mercGroupHealSpell() CastSpell {
	return bookSpell(700190, "Word of Health", 2, 3*time.Second, 19*time.Second, 300, "Group, Increase Hitpoints by 300, Radius 30")
}

func mercGroupBuffSpell() CastSpell {
	s := bookSpell(700396, "Group Resist Affliction", 2, 3*time.Second, 0, 60, "Group, Increase PR by 45, Increase DR by 45, Radius 30, 30min")
	s.Level = mercOwnerLevel
	return s
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

func hurtNPCTo(n *NPC, pct int) int {
	n.HP = n.MaxHP * pct / 100
	return n.HP
}

// mercScope is the rig's owner with a cleric merc of the spells given (slot 1) and a
// warrior merc with none (slot 2), leading a raid of them and five more players: A, B, C
// and D, E; A with a merc of its own in the raid.
type mercScope struct {
	*mercRig
	healer, second, theirs *mercState
	g                      *Group
	ps                     []*Player
}

func newMercScope(t *testing.T, spells ...CastSpell) *mercScope {
	t.Helper()
	r := newMercRig(t)
	s := &mercScope{mercRig: r}
	for slot, k := range []MercKit{r.kit(9, 0, 0, spells...), r.kit(0, 0, 0)} {
		k.Name = []string{"Healer", "Second"}[slot]
		if _, why, _ := r.w.HireMercAt(r.sess, slot+1, k, MercAssist, r.at(0)); why != "" {
			t.Fatalf("slot %d: %s", slot+1, why)
		}
		r.w.mu.Lock()
		_, up := r.w.invites[r.p.ServerID][r.p.ServerID]
		r.w.mu.Unlock()
		if up {
			r.w.GroupAccept(r.sess, r.sess.PlayerObjectID())
		}
	}
	r.w.mu.Lock()
	for _, ms := range r.w.mercsOfLocked(r.p) {
		if ms.slot == 1 {
			s.healer = ms
		} else {
			s.second = ms
		}
	}
	r.w.mu.Unlock()
	if s.healer == nil || s.second == nil {
		t.Fatal("the owner's two mercs are not out")
	}
	var sessA = r.sess
	for i, n := range []string{"A", "B", "C", "D", "E"} {
		sess, p := r.player(n, 0, 0)
		if i == 0 {
			sessA = sess
		}
		s.ps = append(s.ps, p)
	}
	s.g = r.group(s.ps...)
	s.g.flagged = true
	k := r.kit(9, 0, 0)
	k.Name = "Theirs"
	if _, why := r.w.HireMerc(sessA, k, MercPassive, r.at(0)); why != "" {
		t.Fatalf("A's hire: %s", why)
	}
	s.theirs = r.w.merc1(s.ps[0].ServerID)
	if s.theirs == nil || !s.g.isRaid() || !groupHas(s.g, s.theirs.npc.ServerID) || !groupHas(s.g, s.second.npc.ServerID) {
		t.Fatalf("the raid %v", s.g.Members)
	}
	return s
}

// A healer merc in a raid heals its owner and its owner's other merc alone: the raid's
// players and another player's merc, however hurt, are never healed.
func TestMercHealsOnlyItsOwnersOwn(t *testing.T) {
	s := newMercScope(t, mercHealSpell())
	a, e, theirs := s.ps[0], s.ps[4], s.theirs.npc
	hurtTo(a, 20)
	hurtTo(e, 20)
	theirsHP := hurtNPCTo(theirs, 20)
	if c := s.firstMercCast(0, 2*time.Second); c != nil {
		t.Fatalf("a heal with only others hurt: %+v on %d", c.spell.Name, c.target)
	}

	second := s.second.npc
	before := hurtNPCTo(second, 60)
	c := s.firstMercCast(2*time.Second, 4*time.Second)
	if c == nil || c.target != second.ServerID {
		t.Fatalf("the merc's cast %+v: want its heal on its owner's other merc", c)
	}
	s.run(4*time.Second, 9*time.Second)
	if second.HP <= before {
		t.Errorf("the owner's other merc was not healed: %d", second.HP)
	}
	if a.HP != a.MaxHPNow()*20/100 || e.HP != e.MaxHPNow()*20/100 || theirs.HP != theirsHP {
		t.Errorf("another was healed: A %d, E %d, A's merc %d", a.HP, e.HP, theirs.HP)
	}

	// the owner, less hurt than A, before A
	s2 := newMercScope(t, mercHealSpell())
	hurtTo(s2.ps[0], 10)
	hurtTo(s2.p, 50)
	if c := s2.firstMercCast(0, 2*time.Second); c == nil || c.target != s2.p.ServerID {
		t.Fatalf("the merc's cast %+v: want its heal on its owner", c)
	}
}

// Its priority holds among its own: one below mercEmergencyPct first, with its biggest heal.
func TestMercOwnHealKeepsItsPriority(t *testing.T) {
	small := mercHealSpell()
	big := bookSpell(700188, "Greater Healing", 1, 3*time.Second, 19*time.Second, 300, "Target, Increase Hitpoints by 600")
	s := newMercScope(t, small, big)
	hurtTo(s.p, 60)
	hurtNPCTo(s.second.npc, 30)
	hurtTo(s.ps[1], 5)
	c := s.firstMercCast(0, 2*time.Second)
	if c == nil || c.target != s.second.npc.ServerID {
		t.Fatalf("the merc's cast %+v: want its heal on its owner's other merc, the worst off of its own", c)
	}
	if c.spell.SpellID != big.SpellID {
		t.Errorf("an emergency's heal is %q, want its biggest", c.spell.Name)
	}
}

// Its group heal goes up for two of its own hurt, never for others alone, and lands on its
// own alone.
func TestMercGroupHealSkipsOthers(t *testing.T) {
	s := newMercScope(t, mercGroupHealSpell())
	hurtTo(s.ps[0], 50)
	hurtTo(s.ps[1], 50)
	hurtNPCTo(s.theirs.npc, 50)
	if c := s.firstMercCast(0, 2*time.Second); c != nil {
		t.Fatalf("a group heal with only others hurt: %+v", c)
	}
	owner := hurtTo(s.p, 50) * 50 / 100
	second := hurtNPCTo(s.second.npc, 50)
	c := s.firstMercCast(2*time.Second, 4*time.Second)
	if c == nil || !c.group {
		t.Fatalf("the merc's cast %+v: want its group heal", c)
	}
	s.run(4*time.Second, 9*time.Second)
	if s.p.HP <= owner || s.second.npc.HP <= second {
		t.Errorf("its own not healed: owner %d (was %d), its other merc %d (was %d)", s.p.HP, owner, s.second.npc.HP, second)
	}
	for _, p := range s.ps[:2] {
		if p.HP != p.MaxHPNow()*50/100 {
			t.Errorf("%s was healed: %d", p.Name, p.HP)
		}
	}
	if n := s.theirs.npc; n.HP != n.MaxHP*50/100 {
		t.Errorf("A's merc was healed: %d", n.HP)
	}
}

func hasEffect(effects []*ActiveEffect, id int) bool {
	for _, ae := range effects {
		if ae.SpellID == id {
			return true
		}
	}
	return false
}

// A merc's group buff lands on its owner and its owner's mercs, not on the raid's other
// players nor their mercs; a cast at another lands on no one.
func TestMercGroupBuffSkipsOthers(t *testing.T) {
	gra := mercGroupBuffSpell()
	s := newMercScope(t, gra)
	s.run(0, 10*time.Second)
	if !hasEffect(s.p.Effects, gra.SpellID) || !hasEffect(s.healer.npc.Effects, gra.SpellID) || !hasEffect(s.second.npc.Effects, gra.SpellID) {
		t.Fatalf("its group buff is not on its own: owner %v, itself %v, its other merc %v", hasEffect(s.p.Effects, gra.SpellID),
			hasEffect(s.healer.npc.Effects, gra.SpellID), hasEffect(s.second.npc.Effects, gra.SpellID))
	}
	for _, p := range s.ps {
		if hasEffect(p.Effects, gra.SpellID) {
			t.Errorf("its group buff is on %s", p.Name)
		}
	}
	if hasEffect(s.theirs.npc.Effects, gra.SpellID) {
		t.Error("its group buff is on A's merc")
	}

	// a cast at another, landing: nothing
	single := bookSpell(700805, "Order", 1, 5*time.Second, 0, 60, "Target, Increase AC by 650, Increase Maximum HP by 650, 30min")
	s.w.mu.Lock()
	s.w.mercHelpLocked(s.healer, &single, s.ps[1].ServerID, s.at(11*time.Second))
	s.w.mercHelpLocked(s.healer, &single, s.theirs.npc.ServerID, s.at(11*time.Second))
	s.w.mercHelpLocked(s.healer, &single, s.second.npc.ServerID, s.at(11*time.Second))
	s.w.mu.Unlock()
	if hasEffect(s.ps[1].Effects, single.SpellID) || hasEffect(s.theirs.npc.Effects, single.SpellID) {
		t.Error("a merc's buff landed on another than its own")
	}
	if !hasEffect(s.second.npc.Effects, single.SpellID) {
		t.Error("a merc's buff did not land on its owner's other merc")
	}
}

// A player's own group spell still reaches the mercs of its group, another's too
// (unchanged: petition 1909).
func TestPlayerGroupSpellStillReachesOthersMercs(t *testing.T) {
	r := newMercRig(t)
	ms := r.hire(r.kit(0, 0, 0), MercPassive)
	sessB, b := r.player("B", 0, 0)
	r.group(b)
	gra := mercGroupBuffSpell()
	b.Power = b.MaxPowerNow()
	r.run(0, time.Second)
	if !r.w.StartCast(sessB, gra, 0, r.at(2*time.Second)) {
		t.Fatal("B's cast was refused")
	}
	r.run(2*time.Second, 7*time.Second)
	if !hasEffect(b.Effects, gra.SpellID) {
		t.Fatal("B's group buff is not on B")
	}
	if !hasEffect(ms.npc.Effects, gra.SpellID) {
		t.Error("B's group buff did not reach the owner's merc")
	}
	if !hasEffect(r.p.Effects, gra.SpellID) {
		t.Error("B's group buff did not reach the owner")
	}
}

// Its fights are the raid's still (the owner, 2026-11-13: "they can attack when raid
// engages"): a creature fighting a player of the other group of four is taken on and hit,
// and an aggressive merc takes on what a raid member attacks, though it heals and buffs
// its owner's own alone.
func TestMercStillFightsWhatTheRaidEngages(t *testing.T) {
	s := newMercScope(t)
	e := s.ps[4]
	crab := s.creature("Crab", 28, 4)
	crab.HP, crab.MaxHP = 900000, 900000
	crab.CurrentEnemy = e.ServerID
	s.run(0, 5*time.Second)
	if s.second.target != crab.ServerID {
		t.Fatalf("the merc's target %d: want the crab fighting E, of the raid's other group", s.second.target)
	}
	if crab.HP >= crab.MaxHP {
		t.Error("the merc did not hit the crab the raid fights")
	}

	s2 := newMercScope(t)
	d := s2.ps[3]
	rat := s2.creature("Rat", 28, 4)
	rat.HP, rat.MaxHP = 900000, 900000
	s2.second.stance = MercAggressive
	d.AttackTarget, d.AutoAttack = rat.ServerID, true
	s2.run(0, 5*time.Second)
	if s2.second.target != rat.ServerID {
		t.Fatalf("the aggressive merc's target %d: want the rat D attacks", s2.second.target)
	}
}
