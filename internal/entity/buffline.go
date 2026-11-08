package entity

import (
	"fmt"
	"log"
	"slices"
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
// Defense's +AGI -STR), and whether a player or a pet bears it (scope 3),
// as the balance's simulator grouped them (balancesim.go): the ranks of a
// line are the same spell made stronger, and a spell's single, self and
// group forms one line (Courage and Group Courage, the warrior's Iron Skin
// and Iron Will). With every scope a line of its own, a cleric or shaman of
// 45-60 wore both forms and won a red three up every time with 70% of its
// hit points left (claude-notes/balance-20261029.md). Its rank is the
// spell's level. A beneficial spell landing on a player:
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
// spell that only ticks has none. A pet keeps only the best of a line too
// (pet.go's putOnPetLocked, since 2026-10-30: its AC, AGI, STR and DEX count
// in its fights), untold when it is its own cast; a creature keeps every one. An effect kept over a logout keeps its line (buffsave.go); one kept
// before 2026-10-29 has none and stacks until it ends. `balancesim -lines`
// prints every class's lines off a database.

// A spell read from EQOAEmu's workbook (spelljson.go) goes by the workbook's
// rule instead, read from its formula (2026-11-01: every StackType of its
// Spells sheet is one formula of the spell's scope, 1 single target, 2 self,
// 3 group and the areas, 4 the illusions, so a stack type is the class of
// whom it lands on) and its legend ("Used to overwrite lesser buffs of the
// same stack type and statistic effect"; an effect's Level "May be used to
// override lower level buffs that have the same stack type"): its line is its
// stack type, its bearer and its stats, and two such lines are one when their
// stack types and bearers are and they share a stat (sameLine); its rank is
// the workbook's level. So a self buff and a group buff of one stat both
// count, where a line's forms are one line. The rule is the same: a lower
// rank under a higher is refused, a higher takes the lower off, whole (the
// shared stat's spell is one effect). A workbook spell with no stack type has
// no line: nothing it stacks with is refused or taken off (a recast of the
// same spell still replaces itself, addEffectLocked).

// Nested lines (the user, 2026-11-08: "cleric resist buff and the pr/dr buff
// should stack", then "the stronger pr/dr over rides the weaker resist buff,
// but keeps the rest of the resist buffs from the resist buff"): two workbook
// stack lines of one stack type and bearer, not the same stats, the one's all
// within the other's (the cleric's PR/DR line, Endure Ailment to Consume
// Affliction, within its all-resist line, Elemental Ward to Mega's Clutch
// Ward; their group forms the same), are not one line. Both stay on, and each
// stat they share counts the stronger of the two once (sumEffectMods), the
// rest of each as it is. One that would add nothing (no stat of it stronger
// than the other's) is refused, as a lower rank is; one that gives all the
// other did, as strongly or more, takes it off, as a higher rank does. Two
// lines that share a stat but neither holds the other (+STR +DEX against +STR
// +AGI) are one line, as before; so are the same stats (a line's ranks).

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
	bearer := "player"
	if s.Scope == 3 {
		bearer = "pet"
	}
	if d.Workbook {
		if d.Stack <= 0 {
			return "" // no stack type: no line (the file's doc)
		}
		return fmt.Sprintf("%s%d %s %s", stackLinePrefix, d.Stack, bearer, strings.Join(keys, " "))
	}
	return bearer + " " + strings.Join(keys, " ")
}

// stackLinePrefix begins a workbook stack type's line.
const stackLinePrefix = "stack"

// sameLine reports whether two effects' lines are one (the file's doc): the
// same line, or two workbook stack lines of one stack type and bearer that
// share a stat.
func sameLine(a, b string) bool {
	if a == "" || b == "" {
		return false
	}
	if a == b {
		return true
	}
	if !strings.HasPrefix(a, stackLinePrefix) || !strings.HasPrefix(b, stackLinePrefix) {
		return false
	}
	af, bf := strings.Fields(a), strings.Fields(b)
	if len(af) < 3 || len(bf) < 3 || af[0] != bf[0] || af[1] != bf[1] {
		return false
	}
	for _, k := range af[2:] {
		for _, j := range bf[2:] {
			if k == j {
				return true
			}
		}
	}
	return false
}

// stackLineParts is a workbook stack line's stack type and bearer ("stack1
// player") and its stats' keys; ok is false for any other line.
func stackLineParts(line string) (family string, keys []string, ok bool) {
	if !strings.HasPrefix(line, stackLinePrefix) {
		return "", nil, false
	}
	f := strings.Fields(line)
	if len(f) < 3 {
		return "", nil, false
	}
	return f[0] + " " + f[1], f[2:], true
}

// nestedLines reports two lines nested (the doc above): workbook stack lines
// of one stack type and bearer, not the same, the one's keys all among the
// other's.
func nestedLines(a, b string) bool {
	fa, ka, oka := stackLineParts(a)
	fb, kb, okb := stackLineParts(b)
	if !oka || !okb || fa != fb || a == b {
		return false
	}
	return keysWithin(ka, kb) || keysWithin(kb, ka)
}

// keysWithin reports every key of a among b's.
func keysWithin(a, b []string) bool {
	for _, k := range a {
		if !slices.Contains(b, k) {
			return false
		}
	}
	return true
}

// covers reports whether a gives every stat b raises, as much or more: b would
// add nothing to a.
func covers(a, b StatMods) bool {
	for st := Stat(1); int(st) < len(statLineNames)-1; st++ {
		if st != StatRanged && b.get(st) > 0 && a.get(st) < b.get(st) {
			return false
		}
	}
	return true
}

// sumEffectMods is what a bearer's effects add to its stats: each one's, but a
// stat nested lines share (nestedLines) counts its largest once.
func sumEffectMods(effects []*ActiveEffect) StatMods {
	var m StatMods
	for _, e := range effects {
		m.sum(e.Mods)
	}
	type shared struct {
		family string
		st     Stat
	}
	raised := map[shared][]int{}
	for _, e := range effects {
		fam, keys, ok := stackLineParts(e.Line)
		if !ok {
			continue
		}
		for st := Stat(1); int(st) < len(statLineNames)-1; st++ {
			if v := e.Mods.get(st); st != StatRanged && v > 0 && slices.Contains(keys, "+"+statLineNames[st]) {
				k := shared{fam, st}
				raised[k] = append(raised[k], v)
			}
		}
	}
	for k, vs := range raised {
		if len(vs) < 2 {
			continue // one effect raises it: as it is
		}
		total, top := 0, 0
		for _, v := range vs {
			total += v
			top = max(top, v)
		}
		m.add(k.st, top-total)
	}
	return m
}

// lineRank is a spell's rank on its line: the workbook's level for a spell
// of a stack type, else its record's.
func lineRank(s *CastSpell) int {
	if s.Def.Workbook && s.Def.Stack > 0 && s.Def.StackLevel > 0 {
		return s.Def.StackLevel
	}
	return s.Level
}

// statLineNames are the stats as a line names them, by Stat.
var statLineNames = []string{"none", "STR", "STA", "AGI", "DEX", "WIS", "INT", "CHA", "AC", "maxHP", "maxPower",
	"FR", "CR", "PR", "DR", "LR", "AR", "damage", "HoT", "PoT", "pierce", "blunt", "slash", "ranged", "hpfactor", "offmod",
	"defmod", "?"}

var buffKindNames = map[EffectKind]string{EffectShield: "shield", EffectDefProc: "defproc", EffectProc: "proc",
	EffectSpeed: "speed", EffectInvis: "invis", EffectPowerCap: "powercap"}

const (
	lineStrongerSelf = "You already have %s, which is stronger."
	lineStronger     = "%s already has %s, which is stronger."
)

// lineTakesLocked marks a beneficial effect landing on a player with its
// spell's line and rank and reports whether it takes (the file's doc): not
// when an effect of its line of a higher rank is on, of which the caster is
// told. Must be called with the lock held.
func (w *World) lineTakesLocked(caster *Player, casterSess *session.Session, t *Player, spell *CastSpell, ae *ActiveEffect) bool {
	ae.Line, ae.Rank = BuffLine(spell), lineRank(spell)
	if ae.Line == "" {
		return true
	}
	for _, e := range t.Effects {
		if e.SpellID == ae.SpellID {
			continue
		}
		if nestedLines(e.Line, ae.Line) {
			if !covers(e.Mods, ae.Mods) {
				continue // both on, each shared stat its stronger once (the doc above)
			}
		} else if !sameLine(e.Line, ae.Line) || e.Rank <= ae.Rank {
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
		if e.SpellID == ae.SpellID {
			continue
		}
		if nested := nestedLines(e.Line, ae.Line); nested && covers(e.Mods, ae.Mods) || !nested && sameLine(e.Line, ae.Line) && e.Rank > ae.Rank {
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
		if e.SpellID == ae.SpellID {
			continue
		}
		if nestedLines(e.Line, ae.Line) {
			if covers(ae.Mods, e.Mods) {
				drop = append(drop, e.SpellID) // it gives all the other did, as strongly or more
			}
			continue
		}
		if sameLine(e.Line, ae.Line) {
			drop = append(drop, e.SpellID)
		}
	}
	for _, id := range drop {
		w.removeEffectLocked(t, id)
	}
}
