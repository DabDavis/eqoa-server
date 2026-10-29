package entity

import (
	"net"
	"testing"
	"time"

	"github.com/eqoa/server/internal/session"
)

// The cleric's marks and brands (mark.go): their words, and a mark's answer
// to the players' and pets' hits on its creature. The spells' names, levels
// and figures are made up; the words are the lists'.

// markSpell is a made-up mark of that level and line, cast at a target.
func markSpell(id int, name string, level int, line string) CastSpell {
	return CastSpell{SpellID: id, DisplayIndex: 6, Level: level, Scope: 1, Name: name, Icon: 0x5A5B5C5D,
		CastTime: time.Second, Recast: time.Second, PowerCost: 20, Range: 30, Def: ParseSpellNamed(name, line, "")}
}

// markEffect is a mark on a creature until the time given.
func markEffect(id int, name string, lo, hi int, heal bool, caster EntityID, until time.Time) *ActiveEffect {
	return &ActiveEffect{SpellID: id, Name: name, Caster: caster, Until: until, Mark: &Mark{Min: lo, Max: hi, Heal: heal}}
}

// addWarrior is a second player beside the caster's creature, in its own
// session, that sees the creature as 0x01312E99.
func (c *caster) addWarrior(t *testing.T) (*Player, *session.Session) {
	t.Helper()
	sess := c.w.SessionManager.CreateSession(&net.UDPAddr{IP: net.IPv4(10, 0, 0, 9), Port: 7001}, 2, 2, 2, 3, true)
	p := &Player{ServerID: 1003, Name: "Warrior", Class: 0, Level: 30, HP: 400, MaxHP: 900, Str: 90, Sta: 90, Agi: 80, Dex: 80,
		X: 103, Y: 10, Z: 100}
	c.w.Players[p.ServerID] = p
	sess.SetECSEntity(p)
	sess.SetCharacterInWorld(true)
	sess.SetPlayerObjectID(0x1424A1D1)
	vs := GetOrCreateVisibility(sess)
	vs.ChannelEntity[0] = c.npc.ServerID
	vs.ChannelObjectID[0] = 0x01312E99
	return p, sess
}

// Every way the lines say a mark is read as one: cast at an enemy, its
// figure the line's or 0 for the default, nothing unread ("Add Offensive
// Proc" before a mark is its own words). The controls read as they did.
func TestTheMarkWordsAreRead(t *testing.T) {
	for _, c := range []struct {
		line   string
		kind   EffectKind
		lo, hi int
		dur    time.Duration
	}{
		{"Target, damages target when it hits, 40sec", EffectMark, 0, 0, 40 * time.Second},
		{"Target, Add Offensive Proc, Damage Target when they hit , 40sec", EffectMark, 0, 0, 40 * time.Second},
		{"Target, 11-13 Heal when Mob hits, 20sec", EffectMarkHeal, 11, 13, 20 * time.Second},
		// a GM's
		{"Target, Damage Target by 30 when they hit, 1min", EffectMark, 30, 30, time.Minute},
		{"Target, Damage Target for 20 to 24 when it hits, 1min", EffectMark, 20, 24, time.Minute},
		{"Target, Heal when Mob hits, 1min", EffectMarkHeal, 0, 0, time.Minute},
	} {
		d := ParseSpellNamed("Mark of Thistles", c.line, "")
		if len(d.Effects) != 1 || d.Effects[0].Kind != c.kind || d.Effects[0].Amount != c.lo || d.Effects[0].AmountMax != c.hi ||
			!d.Harmful || d.Target != TargetSingle || d.Duration != c.dur || len(d.Unread) != 0 {
			t.Errorf("%q: %+v", c.line, d)
		}
	}
	for _, c := range []struct{ line, why string }{
		{"Target, Inflicts Damage on mob when it hits, 60sec", "a druid's curse, read nowhere"},
		{"Target, approx. 27 Damage to Mob when it hits, 10min", "a damage shield"},
		{"Pet, Increase INT by 25, Add Offensive Proc:, Add DS, 10min", "a pet's mark: a shield and an unread proc"},
		{"Target, Decrease Hitpoints by 40", "a nuke"},
	} {
		for _, e := range ParseSpellNamed("Mark of Thistles", c.line, "").Effects {
			if e.Kind == EffectMark || e.Kind == EffectMarkHeal {
				t.Errorf("%q (%s) reads as a mark", c.line, c.why)
			}
		}
	}

	// cast at a friend it is refused, as any harmful spell
	cc := newCaster(t)
	if cc.w.StartCast(cc.sess, markSpell(9701, "Mark of Thistles", 30, "Target, damages target when it hits, 40sec"), cc.me, cc.at(0)) {
		t.Error("a mark began at its caster")
	}
}

// The figure: the line's own; else half the spell's level, at least 2, and
// twice that for a "Brand of ..." (ours).
func TestTheMarksFigure(t *testing.T) {
	c := newCaster(t)
	now := c.at(0)
	for _, x := range []struct {
		name  string
		level int
		line  string
		want  Mark
	}{
		{"Mark of Thistles", 30, "Target, damages target when it hits, 40sec", Mark{Min: 15, Max: 15}},
		{"Brand of Thistles", 30, "Target, damages target when it hits, 40sec", Mark{Min: 30, Max: 30}},
		{"Mark of Thistles", 3, "Target, damages target when it hits, 40sec", Mark{Min: 2, Max: 2}},
		{"Brand of Thistles", 3, "Target, damages target when it hits, 40sec", Mark{Min: 4, Max: 4}},
		{"Brand of Thistles", 30, "Target, Damage Target by 7 when they hit, 40sec", Mark{Min: 7, Max: 7}},
		{"Mark of Mercy", 40, "Target, 12-15 Heal when Mob hits, 20sec", Mark{Min: 12, Max: 15, Heal: true}},
		{"Mark of Mercy", 40, "Target, Heal when Mob hits, 20sec", Mark{Min: 20, Max: 20, Heal: true}},
	} {
		cs := &castState{spell: markSpell(9701, x.name, x.level, x.line), caster: c.p.ServerID}
		ae := lasting(cs, c.p, now)
		if ae == nil || ae.Mark == nil || *ae.Mark != x.want || ae.Until != now.Add(cs.spell.Def.Duration) {
			t.Errorf("%s (L%d) %q: %+v, want %+v", x.name, x.level, x.line, ae, x.want)
		}
	}
}

// A damage mark landed on a creature adds its figure to every player's hit
// on it, as that player's hit: its 0x00DB from the attacker, the creature's
// hate for the attacker, the kill and its XP the attacker's.
func TestADamageMarkAddsToEachHit(t *testing.T) {
	c := newCaster(t)
	c.npc.Name, c.npc.Level, c.npc.HP, c.npc.MaxHP = "Sand Beetle", 30, 5000, 5000
	c.land(t, markSpell(9701, "Mark of Thistles", 30, "Target, damages target when it hits, 40sec"), c.npcO, 0)
	if len(c.npc.Effects) != 1 || c.npc.Effects[0].Mark == nil || *c.npc.Effects[0].Mark != (Mark{Min: 15, Max: 15}) ||
		c.npc.HP != 5000 {
		t.Fatalf("on the creature: %+v, HP %d", c.npc.Effects, c.npc.HP)
	}
	// an auto-attack's hit is on the clock's time, not the test's: the mark
	// held on past both
	c.npc.Effects[0].Until = time.Now().Add(100 * 365 * 24 * time.Hour)

	// the caster's own hits, and another player's
	war, wsess := c.addWarrior(t)
	for _, a := range []struct {
		p    *Player
		sess *session.Session
		npcO uint32
	}{{c.p, c.sess, c.npcO}, {war, wsess, 0x01312E99}} {
		for i := 0; i < 2; i++ {
			hp, hate := c.npc.HP, c.npc.Threats[a.p.ServerID]
			c.w.mu.Lock()
			killed := c.w.playerHitLocked(a.sess, a.p, c.npc, a.npcO)
			c.w.mu.Unlock()
			figures := damageIndications(drain(a.sess))
			me := int64(a.sess.PlayerObjectID())
			if killed || len(figures) != 2 || figures[0][0] != me || figures[0][2] != int64(a.npcO) ||
				figures[1] != [3]int64{me, -15, int64(a.npcO)} {
				t.Fatalf("%s's hit %d: killed %v, sent %v", a.p.Name, i, killed, figures)
			}
			melee := -int(figures[0][1])
			if c.npc.HP != hp-melee-15 || c.npc.Threats[a.p.ServerID] != hate+int64(melee+15) {
				t.Errorf("%s's hit %d of %d: creature %d HP (was %d), hate %d (was %d)", a.p.Name, i, melee, c.npc.HP, hp,
					c.npc.Threats[a.p.ServerID], hate)
			}
		}
	}
	c.sent()

	// the mark's answer kills: the warrior's kill and XP
	c.npc.HP = 10
	xp := war.TotalXP
	c.w.mu.Lock()
	killed := c.w.playerMarkLocked(wsess, war, c.npc, c.at(10*time.Second))
	c.w.mu.Unlock()
	if !killed || !c.npc.Dead || war.TotalXP <= xp || c.p.TotalXP != 0 {
		t.Errorf("the mark's kill: killed %v, dead %v, the warrior's XP %d (was %d), the caster's %d", killed, c.npc.Dead, war.TotalXP, xp, c.p.TotalXP)
	}

	// the control: with no mark on, a hit is the player's alone
	c2 := newCaster(t)
	c2.npc.HP, c2.npc.MaxHP, c2.npc.Level = 5000, 5000, 30
	c2.w.mu.Lock()
	c2.w.playerHitLocked(c2.sess, c2.p, c2.npc, c2.npcO)
	c2.w.mu.Unlock()
	if got := damageIndications(c2.sent()); len(got) != 1 {
		t.Errorf("no mark: sent %v", got)
	}
}

// A heal mark heals the attacker its figure, a range rolled within it, at
// most to its maximum: told from the mark's caster, or from the attacker once
// the caster is gone. The creature loses nothing more.
func TestAHealMarkHealsTheAttacker(t *testing.T) {
	defer func(f func(int) int) { procIntn = f }(procIntn)
	procIntn = dice(2)
	c := newCaster(t)
	c.npc.Name, c.npc.Level, c.npc.HP, c.npc.MaxHP = "Sand Beetle", 30, 5000, 5000
	c.land(t, markSpell(9702, "Mark of Mercy", 40, "Target, 12-15 Heal when Mob hits, 20sec"), c.npcO, 0)
	if len(c.npc.Effects) != 1 || c.npc.Effects[0].Mark == nil || *c.npc.Effects[0].Mark != (Mark{Min: 12, Max: 15, Heal: true}) {
		t.Fatalf("on the creature: %+v", c.npc.Effects)
	}
	c.npc.Effects[0].Until = time.Now().Add(100 * 365 * 24 * time.Hour) // the hit's time is the clock's
	c.p.HP = 100
	hp := c.npc.HP
	c.w.mu.Lock()
	c.w.playerHitLocked(c.sess, c.p, c.npc, c.npcO)
	c.w.mu.Unlock()
	figures := damageIndications(c.sent())
	if c.p.HP != 114 || len(figures) != 2 || figures[1] != [3]int64{int64(c.me), 14, int64(c.me)} ||
		c.npc.HP != hp+int(figures[0][1]) {
		t.Errorf("the hit: the caster %d HP, want 114; creature %d (was %d); sent %v", c.p.HP, c.npc.HP, hp, figures)
	}
	// at most to its maximum
	top := c.p.MaxHPNow()
	c.p.HP = top - 3
	c.w.mu.Lock()
	c.w.playerMarkLocked(c.sess, c.p, c.npc, c.at(5*time.Second))
	c.w.mu.Unlock()
	if c.p.HP != top {
		t.Errorf("3 short of %d: %d", top, c.p.HP)
	}
	c.sent()

	// another player's hit heals that player, told from the mark's caster
	war, wsess := c.addWarrior(t)
	c.w.mu.Lock()
	c.w.playerMarkLocked(wsess, war, c.npc, c.at(6*time.Second))
	c.w.mu.Unlock()
	if war.HP != 414 {
		t.Errorf("the warrior: %d HP, want 414", war.HP)
	}
	drain(wsess)

	// the mark's caster gone: the heal is told as the attacker's own
	c.npc.Effects = []*ActiveEffect{markEffect(9702, "Mark of Mercy", 12, 15, true, 4242, c.at(time.Minute))}
	c.p.HP = 100
	c.w.mu.Lock()
	c.w.playerMarkLocked(c.sess, c.p, c.npc, c.at(7*time.Second))
	c.w.mu.Unlock()
	if got := damageIndications(c.sent()); c.p.HP != 114 || len(got) != 1 || got[0] != [3]int64{int64(c.me), 14, int64(c.me)} {
		t.Errorf("no caster: %d HP, sent %v", c.p.HP, got)
	}
}

// One mark on a creature counts, the strongest by its top (ours), whether it
// heals or hurts; none answers past its time.
func TestOneMarkCounts(t *testing.T) {
	c := newCaster(t)
	c.npc.HP, c.npc.MaxHP, c.npc.Level = 5000, 5000, 30
	now := c.at(time.Minute)
	until := now.Add(time.Minute)
	c.npc.Effects = []*ActiveEffect{
		markEffect(9701, "Mark of Thistles", 10, 10, false, c.p.ServerID, until),
		markEffect(9703, "Brand of Thistles", 25, 25, false, c.p.ServerID, until),
		markEffect(9704, "Mark of Burrs", 20, 20, false, c.p.ServerID, until),
	}
	c.w.mu.Lock()
	c.w.playerMarkLocked(c.sess, c.p, c.npc, now)
	c.w.mu.Unlock()
	if got := damageIndications(c.sent()); c.npc.HP != 4975 || len(got) != 1 || got[0][1] != -25 {
		t.Errorf("three marks: creature %d HP, sent %v; want the brand's 25 alone", c.npc.HP, got)
	}
	// a stronger heal mark answers in their place
	c.npc.Effects = append(c.npc.Effects, markEffect(9702, "Mark of Mercy", 30, 30, true, c.p.ServerID, until))
	c.p.HP = 100
	c.w.mu.Lock()
	c.w.playerMarkLocked(c.sess, c.p, c.npc, now)
	c.w.mu.Unlock()
	if c.npc.HP != 4975 || c.p.HP != 130 {
		t.Errorf("a heal of 30 over a brand of 25: creature %d HP, the caster %d", c.npc.HP, c.p.HP)
	}
	c.sent()
	// past their time, none
	c.w.mu.Lock()
	c.w.playerMarkLocked(c.sess, c.p, c.npc, until)
	c.w.mu.Unlock()
	if got := c.sent(); len(got) != 0 || c.npc.HP != 4975 || c.p.HP != 130 {
		t.Errorf("past their time: sent %x, creature %d, caster %d", got, c.npc.HP, c.p.HP)
	}
}

// A pet's hit on a marked creature: a damage mark's figure is the pet's, its
// owner's as the pet's hits are (the creature's hate for the pet); a heal
// mark heals the pet, at most to its maximum.
func TestAMarkAnswersAPetsHit(t *testing.T) {
	r := newPetRig(t)
	pet, _ := r.summon(t, fireElemental(), 0)
	ps := r.w.pets[pet.Owner]
	if ps == nil || ps.npc != pet {
		t.Fatal("no pet state")
	}
	now := r.at(30 * time.Second)
	r.enemy.Effects = []*ActiveEffect{markEffect(9701, "Mark of Thistles", 25, 25, false, r.p.ServerID, now.Add(time.Minute))}
	hp, hate := r.enemy.HP, r.enemy.Threats[pet.ServerID]
	r.w.mu.Lock()
	killed := r.w.petHitLocked(ps, r.enemy, now)
	r.w.mu.Unlock()
	figures := damageIndications(r.sent())
	if killed || len(figures) != 2 || figures[1] != [3]int64{int64(r.me), -25, 0x0133E8A0} {
		t.Fatalf("the pet's hit: killed %v, sent %v", killed, figures)
	}
	melee := -int(figures[0][1])
	if r.enemy.HP != hp-melee-25 || r.enemy.Threats[pet.ServerID] != hate+int64(melee+25) {
		t.Errorf("the crab %d HP (was %d, the hit %d), its hate for the pet %d (was %d)", r.enemy.HP, hp, melee, r.enemy.Threats[pet.ServerID], hate)
	}

	r.enemy.Effects = []*ActiveEffect{markEffect(9702, "Mark of Mercy", 30, 30, true, r.p.ServerID, now.Add(time.Minute))}
	pet.HP = pet.MaxHP - 50
	hp = r.enemy.HP
	r.w.mu.Lock()
	r.w.petMarkLocked(ps, r.enemy, now)
	r.w.mu.Unlock()
	if pet.HP != pet.MaxHP-20 || r.enemy.HP != hp {
		t.Errorf("the heal mark: the pet %d of %d, the crab %d (was %d)", pet.HP, pet.MaxHP, r.enemy.HP, hp)
	}
	pet.HP = pet.MaxHP - 10
	r.w.mu.Lock()
	r.w.petMarkLocked(ps, r.enemy, now)
	r.w.mu.Unlock()
	if pet.HP != pet.MaxHP {
		t.Errorf("10 short: the pet %d of %d", pet.HP, pet.MaxHP)
	}
}
