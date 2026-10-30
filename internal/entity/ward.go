package entity

import (
	"log"
	"math"
	"regexp"
	"time"
)

// ward.go — a spell that cuts the damage of the creatures' hits on its
// bearer (the owner's ruling of 2026-10-29 on the level-60 epics: write them
// from their notes' numbers, and for what the server lacks, "code them").
//
// The notes, EQOA-Data's epic lists as spelleffect holds them: the paladin's
// "1/2 Damage reducer and Root for 30 seconds", "over 1/3 damage reduced
// larger max damage cap. 30 second duration" and "2/3 Damage reduced for 30
// seconds. smaller damage cap."; the enchanter's Eldritch Rune, "3000 cap
// dmg. Everything that hits you while the buff is up does 1 damage." None
// names a figure past those, and no recording of one exists. Until now such a
// line was prose, read nowhere.
//
// The words (a line's clause, any target word before it):
//   - "Reduce Damage Taken by N%": each hit cut by N% (1 to 100), the cut
//     rounded down;
//   - "Reduce Damage Taken to 1": each hit does 1;
//   - either with " max M" after it, or ", max M" as the next clause (the
//     reader's cap, reCap): the ward ends once it has taken M off in all.
//
// The rules (ours where it says so):
//   - a ward cuts each melee hit a creature lands on its bearer, a player or a
//     pet, while it lasts: npcAttackPlayer's, the training and sparring
//     dummies' (dummy.go), a creature's on a pet (pet.go). The hit's figure,
//     the 0x00DB the client sees, is what is taken; a damage shield answers
//     the hit all the same (dshield.go). Nothing else is cut: drowning, a
//     pact's or a self line's price (lifetap.go), a potion's price, nor the
//     sparring dummy's strike, the one creature's spell here (no shield
//     answers it either);
//   - it lands as a buff, on whom its target word and its record's scope
//     say, and is never harmful; kept over a logout with what it has taken
//     (buffsave.go); a line naming no time lasts buffDuration (spellnotes.go,
//     ours: Eldritch Rune's note gives none);
//   - only the strongest ward on a bearer counts, as only the strongest
//     damage shield answers (dshield.go; ours: two do not add up): the
//     highest percent, a ward to 1 above any short of 100% (it lets through
//     no more than 99% does at any hit) and below 100%; then the most it has
//     left to take, one with no cap the most; then the first put on;
//   - a ward with a cap takes no more than it has left, and once it has
//     taken its cap it is gone: the effect it is part of ends with it unless
//     something else of it lasts (a root, a stat), which keeps its time.

// reWard is a ward's clause, lower case.
var reWard = regexp.MustCompile(`^reduce\s+damage\s+taken\s+(?:by\s+(\d+)\s*%|to\s+1)(?:\s+max\s+(\d+))?$`)

// wardPart reads a ward's clause: Amount its percent, or ToOne; Cap its max.
func wardPart(part string) (SpellEffect, bool) {
	m := reWard.FindStringSubmatch(part)
	if m == nil {
		return SpellEffect{}, false
	}
	e := SpellEffect{Kind: EffectWard, Cap: atoi(m[2])}
	if m[1] == "" {
		e.ToOne = true
		return e, true
	}
	pct := atoi(m[1])
	if pct < 1 || pct > 100 {
		return SpellEffect{}, false
	}
	e.Amount, e.AmountMax = pct, pct
	return e, true
}

// Ward is what an effect takes off each creature's melee hit on its bearer:
// Pct percent of it (rounded down), or all but 1 when One; Cap the most it
// takes in all (0 for no cap), Taken what it has so far. Kept over a logout
// in the mods column (buffsave.go).
type Ward struct {
	Pct   int  `json:",omitempty"`
	One   bool `json:",omitempty"`
	Cap   int  `json:",omitempty"`
	Taken int  `json:",omitempty"`
}

// with is the ward with one of the cast's ward effects added: the stronger
// of the two.
func (wd *Ward) with(e SpellEffect) *Ward {
	nw := &Ward{Pct: e.Amount, One: e.ToOne, Cap: max(e.Cap, 0)}
	if wd != nil && !nw.beats(wd) {
		return wd
	}
	return nw
}

// strength orders wards by what they cut: twice the percent, a ward to 1
// between 99% and 100% (see the file's comment).
func (wd *Ward) strength() int {
	if wd.One {
		return 199
	}
	return 2 * wd.Pct
}

// left is what the ward may still take, math.MaxInt with no cap.
func (wd *Ward) left() int {
	if wd.Cap <= 0 {
		return math.MaxInt
	}
	return max(wd.Cap-wd.Taken, 0)
}

// spent reports whether the ward has taken its cap.
func (wd *Ward) spent() bool { return wd.Cap > 0 && wd.Taken >= wd.Cap }

// beats reports whether wd counts over o: the stronger, then the more left.
func (wd *Ward) beats(o *Ward) bool {
	if a, b := wd.strength(), o.strength(); a != b {
		return a > b
	}
	return wd.left() > o.left()
}

// cut is what the ward takes off a hit of n, within what it has left.
func (wd *Ward) cut(n int) int {
	c := n * wd.Pct / 100
	if wd.One {
		c = n - 1
	}
	return min(max(c, 0), wd.left())
}

// wardHit is a creature's melee hit of n on the bearer of effects after the
// strongest ward on: what it takes, the effect whose ward cut it (nil for
// none), how much was cut, and whether that ward is now spent. The ward's
// Taken goes up by the cut.
func wardHit(effects []*ActiveEffect, n int, now time.Time) (taken int, ae *ActiveEffect, cut int, spent bool) {
	if n <= 0 {
		return n, nil, 0, false // nothing to cut: an invulnerable bearer's (spiritform.go)
	}
	for _, e := range effects {
		if e.Ward == nil || !now.Before(e.Until) || e.Ward.spent() {
			continue
		}
		if ae == nil || e.Ward.beats(ae.Ward) {
			ae = e
		}
	}
	if ae == nil {
		return n, nil, 0, false
	}
	cut = ae.Ward.cut(n)
	ae.Ward.Taken += cut
	return n - cut, ae, cut, ae.Ward.spent()
}

// idle reports whether an effect does nothing at all: a spent ward's, once
// the ward is off it, when nothing else of it lasts.
func (ae *ActiveEffect) idle() bool {
	return ae.Ward == nil && ae.HPTick == 0 && ae.PowerTick == 0 && ae.Mods == (StatMods{}) && !ae.Root && !ae.Snare &&
		!ae.Speed && !ae.Invis && ae.Proc == 0 && ae.ProcMax == 0 && ae.Form == 0 && !ae.Appearance &&
		ae.Shield == nil && !ae.PowerCapped && ae.Mark == nil && !ae.HPCapped && !ae.Invulnerable && !ae.Lull
}

// playerWardLocked is a creature's melee hit of n on a player after the
// player's ward: what the player takes. A spent ward comes off, and its
// effect with it when nothing else of it lasts.
func (w *World) playerWardLocked(p *Player, npc *NPC, n int, now time.Time) int {
	taken, ae, cut, spent := wardHit(p.Effects, n, now)
	if ae == nil {
		return n
	}
	log.Printf("Ward: %s's %q takes %d off %s's hit of %d (%d taken of %d)", p.Name, ae.Name, cut, npc.Name, n, ae.Ward.Taken, ae.Ward.Cap)
	if spent {
		ae.Ward = nil
		if ae.idle() {
			w.removeEffectLocked(p, ae.SpellID)
		}
		log.Printf("Ward: %s's %q is spent", p.Name, ae.Name)
	}
	return taken
}

// petWardLocked is a creature's melee hit of n on a pet after the pet's
// ward, as a player's (playerWardLocked).
func (w *World) petWardLocked(pet, npc *NPC, n int, now time.Time) int {
	taken, ae, cut, spent := wardHit(pet.Effects, n, now)
	if ae == nil {
		return n
	}
	log.Printf("Ward: the pet %q's %q takes %d off %s's hit of %d (%d taken of %d)", pet.Name, ae.Name, cut, npc.Name, n, ae.Ward.Taken, ae.Ward.Cap)
	if spent {
		ae.Ward = nil
		if ae.idle() {
			dropNPCEffect(pet, ae) // cure.go
		}
		log.Printf("Ward: the pet %q's %q is spent", pet.Name, ae.Name)
	}
	return taken
}
