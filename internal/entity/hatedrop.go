package entity

import "log"

// A hate-reducer cast at a friendly player (petition 3258, Mightyeagle: "almost 100% jolt could be
// cast on other players. i think concussion, avoid, anything with no dmg or player type debuff
// could"): a spell whose every part takes hate away (Jolt, Concussion, Avoidance, the rogues'
// Evade and Evasion, Clash, Impact) may be aimed at another player who is no foe, and then takes
// that player's hate, by the caster's amount, off every creature fighting him. The floor of 0
// holds (hatefloor.go): a creature that holds none for him loses nothing and starts no fight. The
// caster draws no hate of his own by it, and nothing else lands. A spell with a hit or a stat
// taken (the elemental clashes, Cat Scratch Fever) stays an enemy's alone.

// dropsHateOnly reports a spell whose every part takes hate away from its target, and nothing
// else: one that may be cast at a friendly player (hatedrop.go).
func (d *SpellDef) dropsHateOnly() bool {
	if len(d.Effects) == 0 || d.Charms() || d.Target == TargetUndead {
		return false
	}
	for _, e := range d.Effects {
		if e.Kind != EffectHate || e.Self || e.Amount >= 0 || e.AmountMax > 0 {
			return false
		}
	}
	return true
}

// hateDropAtFriend reports a cast of spell at another player that is a hate-reducer's on a
// friend: one target, no area.
func hateDropAtFriend(spell *CastSpell) bool {
	return spell.Def.dropsHateOnly() && spell.Area.Centre == AreaNone
}

// landHateDropLocked lands a hate-reducer cast at friend t: every live creature holding hate for
// t holds that much less (never below 0). Must be called with the lock held.
func (w *World) landHateDropLocked(cs *castState, caster *Player, t *Player) {
	amount := 0
	for _, e := range cs.spell.Def.Effects {
		amount += spellAmount(e, caster)
	}
	if amount >= 0 {
		return
	}
	n := 0
	for _, npc := range w.NPCs {
		if npc.Dead || npc.World != t.World || npc.Threats[t.ServerID] <= 0 {
			continue
		}
		w.addThreatLocked(npc, t.ServerID, amount)
		n++
	}
	log.Printf("Cast: %s's %q on %s takes %d hate off %d creatures fighting him (petition 3258)",
		caster.Name, cs.spell.Name, t.Name, -amount, n)
}
