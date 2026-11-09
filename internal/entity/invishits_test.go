package entity

import (
	"testing"
	"time"

	"github.com/eqoa/server/internal/database"
)

// The user, 2026-11-02: "potions should still break it, hits too". A potion
// drunk ends its drinker's invisibility, before its own effects land (an
// invisibility potion leaves its drinker invisible: a control) and not when
// it is refused (the other); and every hit on an invisible player ends it,
// whatever gave it: here an invisibility as an EQOA-Data line lands (Sneak's
// "Self, Invis, 60sec": no workbook rules, so the workbook's ends-at-a-hit
// rule leaves it), through each of the six ways a player is hit.
func TestAPotionOrAHitEndsInvisibility(t *testing.T) {
	sneak := func(c *caster) {
		c.p.Effects = append(c.p.Effects, &ActiveEffect{SpellID: 700013, Name: "Sneak", Invis: true, Until: c.at(time.Minute)})
	}
	sneaking := func(c *caster) bool {
		for _, e := range c.p.Effects {
			if e.Name == "Sneak" {
				return true
			}
		}
		return false
	}

	heal := NewPotionEffect(950001, "Healing Potion", "Self, Increase HP by 50", "Minor Healing", 0, "", -1, 0)
	sublimation := NewPotionEffect(950002, "Sublimation", "Self, Invis, 10min", "Sublimation", 0, "attack,damage", -1, 0)
	price := NewPotionEffect(950003, "Blood Draught", "Self, Decrease HP by 100", "Blood Draught", 0, "", -1, 0)
	c := newCaster(t)
	sneak(c)
	if refusal := c.use(t, heal, 0, time.Second); refusal != "" || c.p.Invisible() {
		t.Errorf("a healing potion drunk: refusal %q, invisible %v; want used, seen", refusal, c.p.Invisible())
	}
	c = newCaster(t)
	sneak(c)
	if refusal := c.use(t, sublimation, 0, time.Second); refusal != "" || sneaking(c) || !c.p.Invisible() {
		t.Errorf("an invisibility potion drunk: refusal %q, Sneak still on %v, invisible %v; want Sneak ended, the potion's own on",
			refusal, sneaking(c), c.p.Invisible())
	}
	c = newCaster(t)
	sneak(c)
	c.p.HP = 10
	if refusal := c.use(t, price, 0, time.Second); refusal == "" || !sneaking(c) {
		t.Errorf("a potion refused (its price above the drinker's health): refusal %q, Sneak still on %v; want refused, still on", refusal, sneaking(c))
	}

	orc := func(c *caster) *NPC {
		npc := &NPC{ServerID: 2003, Name: "a test orc", Level: 10, HP: 500, MaxHP: 500, X: 101, Y: 10, Z: 100}
		c.w.NPCs[npc.ServerID] = npc
		return npc
	}
	for _, h := range []struct {
		what string
		hit  func(c *caster)
	}{
		{"a creature's swing", func(c *caster) { c.w.npcAttackPlayer(orc(c), c.p.ServerID, c.at(time.Second)) }},
		{"a creature's spell", func(c *caster) {
			cast := &npcCast{npc: orc(c), spell: &CastSpell{Name: "Frost Bolt"}, pct: 100}
			c.w.templateHitLocked(cast, c.p.ServerID, []SpellEffect{{Kind: EffectHP, Amount: -40, AmountMax: -40}}, c.at(time.Second))
		}},
		{"a creature's special", func(c *caster) {
			npc := orc(c)
			npc.tmpl = &ClassTemplate{ClassTemplate: database.ClassTemplate{SpecialDamage: 1}}
			c.w.templateSpecialHitLocked(npc, c.p.ServerID, c.at(time.Second))
		}},
	} {
		c := newCaster(t)
		c.p.HP, c.p.MaxHP = 5000, 5000
		sneak(c)
		c.w.mu.Lock()
		h.hit(c)
		c.w.mu.Unlock()
		if c.p.Invisible() {
			t.Errorf("%s left the player invisible", h.what)
		}
	}
	d := newDummy(t, "Training Dummy Lv35", 35)
	d.p.HP, d.p.MaxHP = 5000, 5000
	sneak(d)
	d.w.mu.Lock()
	d.w.dummyHitLocked(d.sess, d.npc, d.p, d.at(time.Second))
	d.w.mu.Unlock()
	if d.p.Invisible() {
		t.Error("a training dummy's swing left the player invisible")
	}
	s := newDummy(t, "Sparring Dummy", 30)
	s.p.HP, s.p.MaxHP = 5000, 5000
	s.npc.DummyTarget = s.p.ServerID
	sneak(s)
	s.w.mu.Lock()
	s.w.sparHitLocked(s.sess, s.npc, s.p, 120, "", s.at(time.Second))
	s.w.mu.Unlock()
	if s.p.Invisible() {
		t.Error("a sparring dummy's strike left the player invisible")
	}
	ds := newDuelists(t)
	duel := ds.start(t)
	ds.b.Effects = append(ds.b.Effects, &ActiveEffect{SpellID: 700013, Name: "Sneak", Invis: true, Until: ds.at(time.Minute)})
	ds.w.mu.Lock()
	ds.w.duelHurtLocked(duel, ds.a, ds.a.ServerID, ds.b, 300, ds.at(2*time.Second), numberMelee)
	ds.w.mu.Unlock()
	if ds.b.Invisible() {
		t.Error("a duel's hit left the duelist invisible")
	}
}
