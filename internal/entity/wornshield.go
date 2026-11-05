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
// only" (the three level 58 chest pieces of 401 AC); then a sheet of the
// named armour's procs (the user's photo of "Named/Unique Armor — Proc
// Verbiage (crowd-sourced, ungraded)", players' testimony), whose figures
// the rest are, and whose formula the Anointed's is (the user's choice:
// "procs 15 + 10% cha, so at 600 cha procs for 75"). This server holds no
// record of any of these effects (no itemeffect row, weaponproc 0); the
// rules are ours:
//   - each answers every melee hit a creature lands on its wearer while the
//     wearer stands after it (playerShieldsLocked's callers: a creature's
//     round, a class template's special, a dummy's hit), beside any spell
//     shield on it (an item's own effect, not one of a buff line); the
//     sheet's "proc" read as every hit, as the user read it ("25 heal on
//     contact"), and its element (AR, CR, FR, PR) as nothing: no shield's
//     damage is resisted here;
//   - its damage is the wearer's, as a damage shield's answer is (the 0x00B1
//     at the creature with the shields' visual, then spellHitLocked: the
//     figure, the hate, a kill's XP and faction), on undead alone for one
//     that says so (undeadLocked: npcundead, else the name); a range ("Ds
//     39-40") rolled each hit;
//   - its heal is the wearer's own hit points back (told with 0x00DB, whole,
//     as a heal is, when the wearer was short of its most), its power the
//     wearer's own power back ("mana proc 15 every hit"; no figure is told:
//     the stat channel carries it); neither draws a creature on (no
//     healAggroLocked: a spell's heal sets every red creature near on its
//     caster, which at every hit taken would pull every one in reach into a
//     tank's fight);
//   - a heal by a stat's percent (the Anointed's 15 + 10% of CHA, at most 75)
//     takes the stat as the client shows it (StatNow: within its cap, 400 at
//     level 46 and up, more with the cap masteries); one in a line (the
//     Seraphic Planar's, the user's choice: 75 at none of its INT to 225 at
//     the wearer's INT cap, its damage the same) goes from its figure at none
//     of the stat to its most at the cap.
// The sheet's rows with no figure (Imperial Breastplate, Shield of the Isle,
// Sandals of the Burning Sands), with no effect of these kinds (Tobrin's
// Eyepatch's anti-aggro, the Desecrated breastplate's arcane resist debuff,
// Martyr's Breastplate's extra damage taken) or with a figure of nothing
// said ("Imperial Forged: 120") are not read; nor are its items this server
// has no record of (Deathwall Shroud, Serrated breastplate, Sanctified
// Chestguard, Spiked chestguard, Isleguard Tunic, Thorned Belt).

// WornShield is what a worn item does at each creature's melee hit landed on
// its wearer. Item is the record's name, for the log.
type WornShield struct {
	Item string
	// Damage to the creature: Damage, or a roll from Damage to DamageTo; on
	// undead alone when Undead
	Damage, DamageTo int
	Undead           bool
	// Heal is hit points back to the wearer, Power power back
	Heal, Power int
	// Scale is the stat that raises them: with Line, Damage and Heal go in a
	// line to DamageMax and HealMax at the wearer's cap of the stat; else
	// HealPct percent of the stat is added to the heal, never past HealMax
	Scale              Stat
	Line               bool
	DamageMax, HealMax int
	HealPct            int
}

// wornShields are the items that do so, by their record's name in any case
// (both of the nightshade gloves' records: 3203 any class, 1214 a caster's;
// the Seraphic Planar's record is "seraphic planar chestplate", 8527, the
// Worldbreaker's "worldbreaker chestguard", 1019, and the grundleskin chest
// piece "Grundleskin Guard", 6513, each with the user's word for it beside
// it); wornShieldsByPattern are those keyed by their record, a name another
// record shares (the named Mithril Breastplate, 2251, and the crafted
// "mithril breastplate", 703150, a smith's, of no proc).
var wornShields = map[string]WornShield{
	"breastplate of the anointed": {Heal: 15, HealPct: 10, HealMax: 75, Scale: StatCha},
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
	// the sheet's
	"baleful hide chestguard":   {Damage: 23, DamageTo: 24},
	"coat of jagged rust":       {Damage: 39, DamageTo: 40},
	"golden spiked boots":       {Damage: 30},
	"guard of the deep":         {Damage: 20},
	"rune crested gloves":       {Damage: 30},
	"stinging kite shield":      {Damage: 20},
	"thornlink":                 {Damage: 40},
	"positronian energy shield": {Damage: 200, Undead: true},
	"marauder breastplate":      {Damage: 10, Heal: 5},
	"tower of woe":              {Damage: 90, Heal: 45},
	"mithron chestplate":        {Heal: 10, Power: 10},
	"coralium chestguard":       {Power: 15},
	"coral tunic":               {Power: 25},
	"fathomsteel guard":         {Power: 15},
	"festertwig":                {Power: 15},
}

var wornShieldsByPattern = map[int]WornShield{
	2251: {Power: 15}, // Mithril Breastplate
}

// seraphicPlanar is the Plane of Sky's shadow knight breastplate's.
var seraphicPlanar = WornShield{Damage: 75, DamageMax: 225, Heal: 75, HealMax: 225, Scale: StatInt, Line: true}

// WornShieldOf is the item's, by its record (patternid), else by its name;
// false for an item that does nothing at a hit.
func WornShieldOf(patternID int, itemName string) (WornShield, bool) {
	s, ok := wornShieldsByPattern[patternID]
	if !ok {
		s, ok = wornShields[strings.ToLower(strings.TrimSpace(itemName))]
	}
	if ok {
		s.Item = itemName
	}
	return s, ok
}

// lineScaled is a figure from lo at none of the stat to hi at its cap, v the
// wearer's (lo with no more than lo to go to).
func lineScaled(lo, hi, v, cap int) int {
	if hi <= lo || cap <= 0 {
		return lo
	}
	v = max(0, min(v, cap))
	return lo + (hi-lo)*v/cap
}

// forWearer is its damage, heal and power on p at a hit, its stat as the
// client shows it, a range rolled with intn.
func (s WornShield) forWearer(p *Player, intn func(int) int) (damage, heal, power int) {
	damage, heal, power = s.Damage, s.Heal, s.Power
	if s.DamageTo > s.Damage {
		damage += intn(s.DamageTo - s.Damage + 1)
	}
	if s.Scale == StatNone {
		return damage, heal, power
	}
	v := p.StatNow(s.Scale)
	if s.Line {
		cap := p.StatCapOf(s.Scale)
		return lineScaled(s.Damage, s.DamageMax, v, cap), lineScaled(s.Heal, s.HealMax, v, cap), power
	}
	heal += max(v, 0) * s.HealPct / 100
	if s.HealMax > 0 {
		heal = min(heal, s.HealMax)
	}
	return damage, heal, power
}

// wornShieldsLocked answers a creature's melee hit on a player with what its
// worn items do (Gear.Shields, handlers' wornGear): each one's damage, then
// its heal and power. It reports the creature's death. Must be called with
// the lock held, the player standing.
func (w *World) wornShieldsLocked(sess *session.Session, p *Player, npc *NPC) bool {
	died := false
	for _, s := range p.Gear.Shields {
		damage, n, power := s.forWearer(p, procIntn)
		if damage > 0 && !npc.Dead && (!s.Undead || w.undeadLocked(npc)) {
			w.broadcastCastLocked(p, npc.ServerID, 0, fxShieldHit)
			log.Printf("Shield: %s's %q answers %s for %d", p.Name, s.Item, npc.Name, damage)
			died = w.spellHitLocked(p, sess, npc, damage) || died
		}
		if p.IsDead() {
			continue
		}
		if top := p.MaxHPNow(); n > 0 && p.HP < top {
			p.HP = min(p.HP+n, top)
			p.StatsDirty = true
			w.tellAmountLocked(p, sess, p.ServerID, n)
			log.Printf("Shield: %s's %q heals it for %d (%d/%d)", p.Name, s.Item, n, p.HP, top)
		}
		if top := p.MaxPowerNow(); power > 0 && p.Power < top {
			p.Power = min(p.Power+power, top)
			p.StatsDirty = true
			log.Printf("Shield: %s's %q gives it %d power (%d/%d)", p.Name, s.Item, power, p.Power, top)
		}
	}
	return died
}
