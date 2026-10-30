package entity

import (
	"testing"
	"time"
)

// Spiritform's two clauses (spiritform.go; the owner's ruling of 2026-10-29,
// "check for archives"): "Set Maximum HP to N" holds the maximum down while
// it lasts, and "Invulnerable" takes nothing of a creature's hit. The spells
// and figures here are made up.

// The words are read, whom they land on and how long, never harmful; a line
// as Spiritform's reads whole. The controls are not a ceiling.
func TestSpiritformWordsAreRead(t *testing.T) {
	d := ParseSpellNamed("Made-up Wraithform", "Self, Set Maximum HP to 0, Invulnerable, Form, 20sec", "")
	if d.Target != TargetSelf || d.Harmful || d.Duration != 20*time.Second || len(d.Unread) != 0 || !d.Appearance ||
		len(d.Effects) != 2 || d.Effects[0].Kind != EffectHPCap || d.Effects[0].Amount != 1 || d.Effects[1].Kind != EffectInvuln {
		t.Errorf("the whole line: %+v", d)
	}
	for _, c := range []struct {
		line   string
		target SpellTarget
		kind   EffectKind
		amount int
	}{
		{"Self, Set Max HP to 250, 1min", TargetSelf, EffectHPCap, 250},
		{"Target, Set MaxHP to 40, 10sec", TargetSingle, EffectHPCap, 40},
		{"Self, Set Maximum Hitpoints to 0, 5sec", TargetSelf, EffectHPCap, 1},
		{"Group, Invulnerability, 12sec", TargetGroup, EffectInvuln, 0},
		{"Pet, Invulnerable, 1min", TargetPet, EffectInvuln, 0},
	} {
		d := ParseSpellNamed("Made-up Stone", c.line, "")
		if d.Target != c.target || d.Harmful || len(d.Unread) != 0 || len(d.Effects) != 1 ||
			d.Effects[0].Kind != c.kind || d.Effects[0].Amount != c.amount {
			t.Errorf("%q: %+v", c.line, d)
		}
	}
	for _, line := range []string{"Self, Set Maximum HP to, 20sec", "Self, Invulnerable to fire, 20sec"} {
		d := ParseSpellNamed("Made-up Stone", line, "")
		if len(d.Effects) != 0 || len(d.Unread) != 1 {
			t.Errorf("%q: %+v", line, d)
		}
	}
	if d := ParseSpellNamed("Made-up Vigor", "Self, Increase Max HP by 50, 1min", ""); len(d.Effects) != 1 ||
		d.Effects[0].Kind != EffectStat || d.Effects[0].Stat != StatMaxHP || d.Effects[0].Amount != 50 {
		t.Errorf("a raise of the maximum is still a stat: %+v", d)
	}
}

// Invulnerable: the crab's hit (17) takes nothing while it lasts, the figure
// shown 0, and a ward on beside it cuts nothing; once it is over the hit is
// whole.
func TestInvulnerableTakesNoCreatureHit(t *testing.T) {
	c, crab := casterAndCrab(t)
	c.land(t, selfSpell(9601, "Made-up Stone", "Self, Invulnerable, 20sec"), c.me, 0)
	if len(c.p.Effects) != 1 || !c.p.Effects[0].Invulnerable || len(c.p.Buffs) != 1 {
		t.Fatalf("on the druid: %+v, buffs %+v", c.p.Effects, c.p.Buffs)
	}
	ward := wardEffect(9602, "Made-up Rune", Ward{Pct: 50, Cap: 100}, c.at(time.Hour))
	c.p.Effects = append(c.p.Effects, ward)
	hp := c.p.HP
	for i := range 3 {
		c.w.mu.Lock()
		c.w.npcAttackPlayer(crab, c.p.ServerID, c.at(10*time.Second+time.Duration(i)*time.Second))
		c.w.mu.Unlock()
		if got := c.sent(); !sameMsgs(got, [][]byte{damageMsg(t, c.npcO, 0, c.me)}) {
			t.Errorf("hit %d sent % X, want a 0", i, got)
		}
	}
	if c.p.HP != hp || ward.Ward.Taken != 0 {
		t.Errorf("three hits on the invulnerable: HP %d (was %d), the ward took %d", c.p.HP, hp, ward.Ward.Taken)
	}
	c.tick(30 * time.Second) // over
	c.sent()
	c.p.Effects = nil
	c.w.mu.Lock()
	c.w.npcAttackPlayer(crab, c.p.ServerID, c.at(31*time.Second))
	c.w.mu.Unlock()
	if c.p.HP != hp-17 {
		t.Errorf("the control, after it: HP %d, want %d", c.p.HP, hp-17)
	}
}

// A training dummy's hit and a creature's on a pet are a creature's hits:
// nothing taken by an invulnerable bearer.
func TestInvulnerableTakesNoDummyOrPetHit(t *testing.T) {
	c := newDummy(t, "Training Dummy Lv35", 35)
	c.p.AC, c.p.HP = 600, 100
	c.p.Effects = []*ActiveEffect{{SpellID: 9603, Name: "Made-up Stone", Until: c.at(time.Hour), Invulnerable: true}}
	c.w.mu.Lock()
	c.w.dummyHitLocked(c.sess, c.npc, c.p, c.at(time.Second))
	c.w.mu.Unlock()
	if got := c.sent(); len(got) == 0 || !sameMsgs(got[:1], [][]byte{damageMsg(t, c.npcO, 0, c.me)}) || c.p.HP < 100 {
		t.Errorf("a dummy's hit on the invulnerable: HP %d, sent % X", c.p.HP, got)
	}

	r := newPetRig(t)
	pet, _ := r.summon(t, fireElemental(), 0)
	pet.X, pet.Z = r.enemy.X-2, r.enemy.Z
	stone := CastSpell{SpellID: 9604, DisplayIndex: 22, Level: 20, Scope: 3, Name: "Made-up Pet Stone", CastTime: 2 * time.Second, PowerCost: 50, Range: 30,
		Def: ParseSpellNamed("Made-up Pet Stone", "Pet, Invulnerable, 1min", "")}
	if !r.w.StartCast(r.sess, stone, 0xFFFFFFFF, r.at(20*time.Second)) {
		t.Fatalf("the stone did not begin: %x", r.sent())
	}
	r.w.mu.Lock()
	r.w.castSystem(r.at(22*time.Second + castLand))
	r.w.npcFightPetLocked(r.enemy, pet, r.at(30*time.Second)) // the swing
	hp := pet.HP
	r.w.npcFightPetLocked(r.enemy, pet, r.at(30*time.Second+500*time.Millisecond))
	r.w.mu.Unlock()
	if len(pet.Effects) != 1 || !pet.Effects[0].Invulnerable || pet.HP != hp {
		t.Errorf("the crab's hit on the invulnerable pet: HP %d (was %d), effects %+v", pet.HP, hp, pet.Effects)
	}
}

// Set Maximum HP to 0 with Invulnerable, as Spiritform: the maximum 1 and the
// hit points 1 while it lasts, the crab's hits taking nothing and a heal
// giving nothing; at its end the maximum is the druid's own again and the hit
// points stay at 1, the druid alive.
func TestSetMaximumHPHoldsOneThenGivesTheMaximumBack(t *testing.T) {
	c, crab := casterAndCrab(t)
	top := c.p.MaxHPNow()
	c.land(t, selfSpell(9605, "Made-up Wraithform", "Self, Set Maximum HP to 0, Invulnerable, Form, 20sec"), c.me, 0)
	if c.p.MaxHPNow() != 1 || c.p.HP != 1 || c.p.IsDead() {
		t.Fatalf("under it: HP %d/%d (dead %v), want 1/1", c.p.HP, c.p.MaxHPNow(), c.p.IsDead())
	}
	c.w.mu.Lock()
	c.w.npcAttackPlayer(crab, c.p.ServerID, c.at(10*time.Second))
	c.w.healLocked(c.p, c.sess, c.p, 300)
	c.p.regenTick()
	c.w.mu.Unlock()
	if c.p.HP != 1 || c.p.MaxHPNow() != 1 || c.p.IsDead() {
		t.Errorf("a hit, a heal and a regen under it: HP %d/%d (dead %v), want 1/1", c.p.HP, c.p.MaxHPNow(), c.p.IsDead())
	}
	c.tick(25 * time.Second) // 3.4 s in, 20 s long: over
	if len(c.p.Effects) != 0 || c.p.MaxHPNow() != top || c.p.HP != 1 || c.p.IsDead() {
		t.Errorf("after it: HP %d/%d (dead %v), effects %+v, want 1/%d", c.p.HP, c.p.MaxHPNow(), c.p.IsDead(), c.p.Effects, top)
	}

	// lasting no time, the ceiling takes the hit points down at once, never
	// under 1; a higher ceiling leaves a lower figure alone
	n := newCaster(t)
	n.p.HP = 400
	n.land(t, selfSpell(9606, "Made-up Drain", "Self, Set Maximum HP to 0"), n.me, 0)
	if n.p.HP != 1 || n.p.IsDead() || len(n.p.Effects) != 0 {
		t.Errorf("lasting no time: HP %d (dead %v), effects %+v, want 1", n.p.HP, n.p.IsDead(), n.p.Effects)
	}
	n.land(t, selfSpell(9607, "Made-up Drain", "Self, Set Maximum HP to 300"), n.me, 10*time.Second)
	if n.p.HP != 1 {
		t.Errorf("a ceiling over the hit points: HP %d, want 1", n.p.HP)
	}
}

// Both are kept over a logout; the ceiling holds the hit points as it comes
// back.
func TestSpiritformIsKeptOverALogout(t *testing.T) {
	c := newCaster(t)
	c.p.Effects = []*ActiveEffect{{SpellID: 9608, Name: "Made-up Wraithform", Until: c.at(time.Minute), HPCap: 1, HPCapped: true, Invulnerable: true}}
	rows, ok := c.w.SavedEffects(c.p.ServerID, c.at(0))
	if !ok || len(rows) != 1 {
		t.Fatalf("kept %d (%v)", len(rows), ok)
	}
	d := newCaster(t)
	if n := d.w.RestoreEffects(d.sess, rows, d.at(time.Hour)); n != 1 {
		t.Fatalf("restored %d", n)
	}
	if e := d.p.Effects[0]; !e.Invulnerable || !e.HPCapped || e.HPCap != 1 || d.p.HP != 1 || d.p.MaxHPNow() != 1 {
		t.Errorf("came back as %+v, HP %d/%d", e, d.p.HP, d.p.MaxHPNow())
	}
}
