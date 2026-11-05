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
// a hit, 15 heal no scale. also nightshade gloves are the same". This server
// holds no record of any of these effects (no itemeffect row, weaponproc 0):
// the figures are the user's, the rest ours:
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
//   - a heal by CHA goes in a line from Heal at none to HealMax at the
//     wearer's CHA cap (StatCapOf: 400 at level 46 and up, more with the cap
//     masteries), CHA as the client shows it (StatNow).

// WornShield is what a worn item does at each creature's melee hit landed on
// its wearer: Damage to the creature, Heal hit points to the wearer (by CHA
// up to HealMax when HealByCha). Item is the record's name, for the log.
type WornShield struct {
	Item      string
	Damage    int
	Heal      int
	HealMax   int
	HealByCha bool
}

// wornShields are the items that do so, by their record's name in any case
// (both of the nightshade gloves' records: 3203 any class, 1214 a caster's)
var wornShields = map[string]WornShield{
	"breastplate of the anointed": {Heal: 25, HealMax: 75, HealByCha: true},
	"splendor":                    {Damage: 30, Heal: 15},
	"crown of the jaguar":         {Damage: 30, Heal: 15},
	"nightshade gloves":           {Damage: 30, Heal: 15},
}

// WornShieldOf is the item's, by its record's name; false for an item that
// does nothing at a hit.
func WornShieldOf(itemName string) (WornShield, bool) {
	s, ok := wornShields[strings.ToLower(strings.TrimSpace(itemName))]
	if ok {
		s.Item = itemName
	}
	return s, ok
}

// healFor is what it heals a wearer of cha CHA whose cap is cap.
func (s WornShield) healFor(cha, cap int) int {
	if !s.HealByCha || s.HealMax <= s.Heal || cap <= 0 {
		return s.Heal
	}
	cha = max(0, min(cha, cap))
	return s.Heal + (s.HealMax-s.Heal)*cha/cap
}

// wornShieldsLocked answers a creature's melee hit on a player with what its
// worn items do (Gear.Shields, handlers' wornGear): each one's damage, then
// its heal. It reports the creature's death. Must be called with the lock
// held, the player standing.
func (w *World) wornShieldsLocked(sess *session.Session, p *Player, npc *NPC) bool {
	died := false
	for _, s := range p.Gear.Shields {
		if s.Damage > 0 && !npc.Dead {
			w.broadcastCastLocked(p, npc.ServerID, 0, fxShieldHit)
			log.Printf("Shield: %s's %q answers %s for %d", p.Name, s.Item, npc.Name, s.Damage)
			died = w.spellHitLocked(p, sess, npc, s.Damage) || died
		}
		n := s.healFor(p.StatNow(StatCha), p.StatCapOf(StatCha))
		if top := p.MaxHPNow(); n > 0 && p.HP < top && !p.IsDead() {
			p.HP = min(p.HP+n, top)
			p.StatsDirty = true
			w.tellAmountLocked(p, sess, p.ServerID, n)
			log.Printf("Shield: %s's %q heals it for %d (%d/%d)", p.Name, s.Item, n, p.HP, top)
		}
	}
	return died
}
