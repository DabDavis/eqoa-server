package entity

import (
	"log"
	"regexp"
	"strconv"
	"time"
)

// lull.go — Lull, Calm and their kin: a creature made to notice no one but
// who all but touches it (the owner's ruling of 2026-10-29: "lull and calm
// reduced the agro radius of a monster within or below the spells level
// range. Making it so you could nearly walk up and touch it.").
//
// The lines, EQOA-Data's as spelleffect holds them: Lull's "Target, Decreases
// Aggro Range, 15AE", Calm's "Target, Decrease NPC Aggro Range, 12sec",
// Calming Verse's "Target, Decrease NPC Aggro Range, 30sec", Soothe's "Lowers
// the awareness range of the target", Dumbstruck's "Target, Lull ; Increase
// 10% based on CHA, 18sec"; each record's description is "Reduces Aggression
// towards you." Until now these were prose, read nowhere: cast at a creature
// (the reader's old "aggro"/"lull" words made them harmful), they did nothing
// but anger it, and Soothe was refused at one.
//
// The words (a line's clause, any target word before it): "Decrease Aggro
// Range", "Decreases Aggro Range", "Decrease NPC Aggro Range", "Lowers the
// awareness range" (of the target), "Lull"; each may end "up to level N", or
// be followed by that as a clause of its own: the highest level the lull
// takes, as a charm's (charm.go). A scale after it (Dumbstruck's CHA) is
// read as its own and does nothing. Lull's "15AE" stays as area.go reads it:
// no area (its word is no area word there).
//
// The rules (ours where it says so):
//   - it is cast at a creature, as a harmful spell is (refused at a player or
//     a pet, and across the water's edge), but its landing is no attack: no
//     hate, no fight, no call to the creature's helpers (socialaggro.go), the
//     caster not put in a fight; casting it shows an invisible caster as any
//     cast does (invis.go), nothing more;
//   - it calms a creature of a level up to its band: its words' "up to level
//     N", else the spell's level and LullLevelBand. One above the band, or
//     one already fighting (ours), is not calmed, which is logged;
//   - while it lasts the calmed creature notices a player only within
//     LullRadius (aggroRadiusLocked), whatever its own radius or a GM's row
//     for its name says, when that is wider; it still fights back when hit,
//     and still comes to its own's help (a lull lowers its notice of players,
//     not its call to help: ours);
//   - a line with no time (Lull's, Soothe's) lasts lullDuration.

// The lull's numbers, ours.
const (
	// LullRadius is how near a calmed creature notices a player: just over
	// CLIENT's melee reach, 4.5 (reach.go), so a player can nearly walk up
	// and touch it.
	LullRadius float32 = 5
	// LullLevelBand is how far above the spell's own level a lull still
	// calms a creature when its words name no level: the ranks come at
	// levels 1, 24 and 44, some 20 apart, so each takes the creatures up to
	// the next rank's band.
	LullLevelBand = 20
	// lullDuration is how long a lull whose line gives no time lasts:
	// Calm's 12 seconds, the next rank's.
	lullDuration = 12 * time.Second
)

var (
	reLull      = regexp.MustCompile(`^(?:(?:decreases?|lowers?)\s+(?:the\s+)?(?:npc\s+)?(?:aggro|awareness)\s+range(?:\s+of\s+(?:the\s+)?target)?|lulls?)(?:\s+up\s+to\s+level\s+(\d+))?$`)
	reLullLevel = regexp.MustCompile(`^up\s+to\s+level\s+(\d+)$`)
)

// lullPart reads a lull's clause: Amount the level its words name, 0 for
// none.
func lullPart(part string) (SpellEffect, bool) {
	m := reLull.FindStringSubmatch(part)
	if m == nil {
		return SpellEffect{}, false
	}
	n, _ := strconv.Atoi(m[1])
	return SpellEffect{Kind: EffectLull, Amount: n, AmountMax: n}, true
}

// lullLevelPart reads "up to level N" as its own clause after a lull: the
// lull's level, reporting whether it was one.
func (d *SpellDef) lullLevelPart(part string) bool {
	m := reLullLevel.FindStringSubmatch(part)
	if m == nil || len(d.Effects) == 0 || d.Effects[len(d.Effects)-1].Kind != EffectLull {
		return false
	}
	n, _ := strconv.Atoi(m[1])
	e := &d.Effects[len(d.Effects)-1]
	e.Amount, e.AmountMax = n, n
	return true
}

// lullsOnly reports a line that is a lull and nothing else.
func (d *SpellDef) lullsOnly() bool {
	for _, e := range d.Effects {
		if e.Kind != EffectLull {
			return false
		}
	}
	return len(d.Effects) > 0
}

// lullDefaults gives a lull whose line names no time its lullDuration.
func (d *SpellDef) lullDefaults() {
	if d.Duration == 0 && d.lullsOnly() {
		d.Duration = lullDuration
	}
}

// lullMaxLevel is the highest level the spell's lull calms.
func lullMaxLevel(spell *CastSpell) int {
	for _, e := range spell.Def.Effects {
		if e.Kind == EffectLull && e.Amount > 0 {
			return e.Amount
		}
	}
	return spell.Level + LullLevelBand
}

// lulled reports whether a lull calms the creature now.
func (n *NPC) lulled(now time.Time) bool {
	for _, e := range n.Effects {
		if e.Lull && now.Before(e.Until) {
			return true
		}
	}
	return false
}

// lullLandLocked is a lull landing on a creature (the file's rules): no
// attack, and the creature calmed for the line's time when it is in the
// lull's band and not fighting. Must be called with the lock held.
func (w *World) lullLandLocked(cs *castState, caster *Player, npc *NPC, now time.Time) {
	top := lullMaxLevel(&cs.spell)
	if npc.CurrentEnemy != 0 {
		log.Printf("Lull: %s's %q on %s: it is fighting, not calmed", caster.Name, cs.spell.Name, npc.Name)
		return
	}
	if npc.Level > top {
		log.Printf("Lull: %s's %q on %s: level %d is above its %d, not calmed", caster.Name, cs.spell.Name, npc.Name, npc.Level, top)
		return
	}
	ae := lasting(cs, caster, now)
	if ae == nil {
		return
	}
	for i, old := range npc.Effects {
		if old.SpellID == ae.SpellID {
			npc.Effects = append(npc.Effects[:i], npc.Effects[i+1:]...)
			break
		}
	}
	npc.Effects = append(npc.Effects, ae)
	if w.affected == nil {
		w.affected = make(map[EntityID]*NPC)
	}
	w.affected[npc.ServerID] = npc
	log.Printf("Lull: %s's %q calms %s (level %d, up to %d): it notices a player within %.0f for %v",
		caster.Name, cs.spell.Name, npc.Name, npc.Level, top, LullRadius, ae.Until.Sub(now))
}
