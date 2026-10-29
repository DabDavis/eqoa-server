package entity

import (
	"fmt"
	"log"
	"sort"
	"strings"

	"github.com/eqoa/server/internal/session"
)

// buffline.go — only the best of a buff line counts on a player (the user,
// 2026-10-29: "Buff stacking: only the best of a line counts").
//
// Until then a buff took off only an earlier cast of the same spell
// (addEffectLocked), so every rank of a line stacked: a level 60 warrior
// with Call to Action, Arms, Combat, Battle, War and Victory and Bellow,
// Roar, Howl, Warcry and Battlecry on had 7,778 hit points for its 3,749
// (claude-notes/balance-20261029.md), and its self AC line (Tough, Iron,
// Steel, Mithril and Immortal Skin, Guard, Immortal Guard) gave the sum of
// seven.
//
// A spell's line (BuffLine) is what it does to its bearer while it lasts and
// to whom it is cast: the kind of each lasting effect, a stat's with the stat
// and which way it moves it (Reckless Force's +STR -AGI is not Furious
// Defense's +AGI -STR), and the ability's scope (self, target, group, pet):
// the ranks of a line are the same spell made stronger, and two lines that
// change the same stats the same way to the same bearers are one line. Its
// rank is the spell's level. A beneficial spell landing on a player:
//
//   - with an effect of its line of a higher rank on, does not take: the
//     caster is told in red (a spent cast, as the clarity line's weaker one,
//     enchanter.go);
//   - else takes off every effect of its line of its rank or below (a
//     recast of the same spell included, as before) and goes on.
//
// A spell's share on its own caster (lifetap.go: Pillar of Might's AC, cast
// at a creature) is held to its line the same way, untold.
//
// What has rules of its own keeps them: a heal over time (hot.go: the
// strongest tick), the clarity line (enchanter.go), illusions (illusion.go),
// potions and food (potion.go, consumable.go): a line is only read off the
// stat changes, shields, procs, speed, invisibility and power ceilings, so a
// spell that only ticks has none. Pets and creatures keep theirs as they
// were. An effect kept over a logout keeps its line (buffsave.go); one kept
// before 2026-10-29 has none and stacks until it ends. `balancesim -lines`
// prints every class's lines off a database.

// BuffLine is a spell's line, "" for one that has none: no lasting part but
// ticks, an illusion, a pet's summoning or a charm. (A spell that lands on a
// player is one cast on a friend or a self spell: Reckless Force's line reads
// as harmful, "Decrease AGI by 10", and is a buff.)
func BuffLine(s *CastSpell) string {
	d := &s.Def
	if d.FormKind != FormNone || d.Summons() || d.Charms() {
		return ""
	}
	seen := map[string]bool{}
	var keys []string
	for _, e := range d.Effects {
		var k string
		switch e.Kind {
		case EffectStat:
			sign := "+"
			if e.Amount < 0 {
				sign = "-"
			}
			k = sign + statLineNames[min(max(int(e.Stat), 0), len(statLineNames)-1)]
		case EffectShield, EffectDefProc, EffectProc, EffectSpeed, EffectInvis, EffectPowerCap:
			k = buffKindNames[e.Kind]
		default:
			continue
		}
		if e.Self {
			k += "@self"
		}
		if !seen[k] {
			seen[k] = true
			keys = append(keys, k)
		}
	}
	if len(keys) == 0 {
		return ""
	}
	sort.Strings(keys)
	return fmt.Sprintf("%s %s", scopeNames[min(max(s.Scope, 0), len(scopeNames)-1)], strings.Join(keys, " "))
}

// statLineNames are the stats as a line names them, by Stat.
var statLineNames = []string{"none", "STR", "STA", "AGI", "DEX", "WIS", "INT", "CHA", "AC", "maxHP", "maxPower",
	"FR", "CR", "PR", "DR", "LR", "AR", "?"}

var buffKindNames = map[EffectKind]string{EffectShield: "shield", EffectDefProc: "defproc", EffectProc: "proc",
	EffectSpeed: "speed", EffectInvis: "invis", EffectPowerCap: "powercap"}

// scopeNames are the ability scopes as CLIENT words them (VITargetWord); a
// scope past them is read as the last.
var scopeNames = []string{"self", "target", "group", "pet", "corpse", "other"}

const (
	lineStrongerSelf = "You already have %s, which is stronger."
	lineStronger     = "%s already has %s, which is stronger."
)

// lineTakesLocked marks a beneficial effect landing on a player with its
// spell's line and rank and reports whether it takes (the file's doc): not
// when an effect of its line of a higher rank is on, of which the caster is
// told. Must be called with the lock held.
func (w *World) lineTakesLocked(caster *Player, casterSess *session.Session, t *Player, spell *CastSpell, ae *ActiveEffect) bool {
	ae.Line, ae.Rank = BuffLine(spell), spell.Level
	if ae.Line == "" {
		return true
	}
	for _, e := range t.Effects {
		if e.Line != ae.Line || e.SpellID == ae.SpellID || e.Rank <= ae.Rank {
			continue
		}
		log.Printf("Cast: %s's %q (rank %d) does not take on %s: %q (rank %d) is on, line %s",
			caster.Name, spell.Name, ae.Rank, t.Name, e.Name, e.Rank, ae.Line)
		if casterSess != nil {
			text := fmt.Sprintf(lineStronger, t.Name, e.Name)
			if t == caster {
				text = fmt.Sprintf(lineStrongerSelf, e.Name)
			}
			w.sendColoredLocked(casterSess, text, castRefusalRGB)
		}
		return false
	}
	return true
}

// shareTakesLocked is lineTakesLocked for a spell's share on its own
// caster (lifetap.go: a taunt's AC, Pillar of Might's): it does not take
// under a higher rank of its line, and nobody is told (the spell's part on
// its target landed). Must be called with the lock held.
func (w *World) shareTakesLocked(caster *Player, ae *ActiveEffect) bool {
	if ae.Line == "" {
		return true
	}
	for _, e := range caster.Effects {
		if e.Line == ae.Line && e.SpellID != ae.SpellID && e.Rank > ae.Rank {
			log.Printf("Cast: %s's share of %q (rank %d) does not take: %q (rank %d) is on", caster.Name, ae.Name, ae.Rank, e.Name, e.Rank)
			return false
		}
	}
	return true
}

// dropLineLocked takes off the player's effects of ae's line (its rank or
// below: lineTakesLocked let it on) before ae goes on. Must be called with
// the lock held.
func (w *World) dropLineLocked(t *Player, ae *ActiveEffect) {
	if ae.Line == "" {
		return
	}
	var drop []int
	for _, e := range t.Effects {
		if e.Line == ae.Line && e.SpellID != ae.SpellID {
			drop = append(drop, e.SpellID)
		}
	}
	for _, id := range drop {
		w.removeEffectLocked(t, id)
	}
}
