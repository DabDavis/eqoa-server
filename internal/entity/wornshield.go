package entity

import (
	"log"
	"strings"

	"github.com/eqoa/server/internal/session"
)

// wornshield.go — worn items that answer a creature's melee hit on their
// wearer, as a damage shield does (dshield.go). The user, 2026-11-05:
// "breastplate of the anointed isnt using its effect yet. it is like a damage
// shield, but heals on contact base starting at 25, max at 75 cha scales it",
// then "IoD Splendor paladin plate does 30 damage and heals 15 per hit. no
// scaling" and "Crown of the Jaguar all class all race head piece. 30 damage
// a hit, 15 heal no scale. also nightshade gloves are the same", and
// "Seraphic Planar Breatplate PoS SK breastplate 75 heal and damage. scales
// with int higher, one of the very best in the game", and "traitorous
// breastplate, worldbreaker breastplate, grundleskin 25 heal on contact
// only" (the three level 58 chest pieces of 401 AC). This server holds no
// record of any of these effects (no itemeffect row, weaponproc 0): the
// figures are the user's, the rest ours:
//   - each answers every melee hit a creature lands on its wearer while the
//     wearer stands after it (playerShieldsLocked's callers: a creature's
//     round, a class template's special, a dummy's hit), beside any spell
//     shield on it (an item's own effect, not one of a buff line);
//   - its damage is the wearer's, as a damage shield's answer is (the 0x00B1
//     at the creature with the shields' visual, then spellHitLocked: the
//     figure, the hate, a kill's XP and faction);
//   - its heal is the wearer's own hit points back (told with 0x00DB, whole,
//     as a heal is, when the wearer was short of its most), and it draws no
//     creature on (no healAggroLocked: a spell's heal sets every red creature
//     near on its caster, which at every hit taken would pull every one in
//     reach into a tank's fight);
//   - one that scales (Scale: the Anointed's CHA, the Seraphic Planar's INT)
//     goes in a line from its figure at none of the stat to its most at the
//     wearer's cap of it (StatCapOf: 400 at level 46 and up, more with the cap
//     masteries), the stat as the client shows it (StatNow); the Seraphic
//     Planar's most is 225 at the cap (the user's choice: the Anointed's 3x).

// WornShield is what a worn item does at each creature's melee hit landed on
// its wearer: Damage to the creature, Heal hit points to the wearer; with a
// Scale, each up to its Max at the wearer's cap of that stat. Item is the
// record's name, for the log.
type WornShield struct {
	Item      string
	Damage    int
	DamageMax int
	Heal      int
	HealMax   int
	Scale     Stat
}

// wornShields are the items that do so, by their record's name in any case
// (both of the nightshade gloves' records: 3203 any class, 1214 a caster's;
// the Seraphic Planar's record is "seraphic planar chestplate", 8527, the
// Worldbreaker's "worldbreaker chestguard", 1019, and the grundleskin chest
// piece "Grundleskin Guard", 6513, each with the user's word for it beside it)
var wornShields = map[string]WornShield{
	"breastplate of the anointed": {Heal: 25, HealMax: 75, Scale: StatCha},
	"splendor":                    {Damage: 30, Heal: 15},
	"crown of the jaguar":         {Damage: 30, Heal: 15},
	"nightshade gloves":           {Damage: 30, Heal: 15},
	"seraphic planar chestplate":  seraphicPlanar,
	"seraphic planar breastplate": seraphicPlanar,
	"traitorous breastplate":      {Heal: 25},
	"worldbreaker chestguard":     {Heal: 25},
	"worldbreaker breastplate":    {Heal: 25},
	"grundleskin guard":           {Heal: 25},
	"grundleskin breastplate":     {Heal: 25},
}

// seraphicPlanar is the Plane of Sky's shadow knight breastplate's.
var seraphicPlanar = WornShield{Damage: 75, DamageMax: 225, Heal: 75, HealMax: 225, Scale: StatInt}

// WornShieldOf is the item's, by its record's name; false for an item that
// does nothing at a hit.
func WornShieldOf(itemName string) (WornShield, bool) {
	s, ok := wornShields[strings.ToLower(strings.TrimSpace(itemName))]
	if ok {
		s.Item = itemName
	}
	return s, ok
}

// scaled is a figure from lo at none of the stat to hi at its cap, v the
// wearer's (lo with no Scale, or no more than lo to go to).
func (s WornShield) scaled(lo, hi, v, cap int) int {
	if s.Scale == StatNone || hi <= lo || cap <= 0 {
		return lo
	}
	v = max(0, min(v, cap))
	return lo + (hi-lo)*v/cap
}

// forWearer is its damage and heal on p, its stat as the client shows it.
func (s WornShield) forWearer(p *Player) (damage, heal int) {
	v, cap := 0, 0
	if s.Scale != StatNone {
		v, cap = p.StatNow(s.Scale), p.StatCapOf(s.Scale)
	}
	return s.scaled(s.Damage, s.DamageMax, v, cap), s.scaled(s.Heal, s.HealMax, v, cap)
}

// wornShieldsLocked answers a creature's melee hit on a player with what its
// worn items do (Gear.Shields, handlers' wornGear): each one's damage, then
// its heal. It reports the creature's death. Must be called with the lock
// held, the player standing.
func (w *World) wornShieldsLocked(sess *session.Session, p *Player, npc *NPC) bool {
	died := false
	for _, s := range p.Gear.Shields {
		damage, n := s.forWearer(p)
		if damage > 0 && !npc.Dead {
			w.broadcastCastLocked(p, npc.ServerID, 0, fxShieldHit)
			log.Printf("Shield: %s's %q answers %s for %d", p.Name, s.Item, npc.Name, damage)
			died = w.spellHitLocked(p, sess, npc, damage) || died
		}
		if top := p.MaxHPNow(); n > 0 && p.HP < top && !p.IsDead() {
			p.HP = min(p.HP+n, top)
			p.StatsDirty = true
			w.tellAmountLocked(p, sess, p.ServerID, n)
			log.Printf("Shield: %s's %q heals it for %d (%d/%d)", p.Name, s.Item, n, p.HP, top)
		}
	}
	return died
}
