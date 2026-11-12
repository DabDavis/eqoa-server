package entity

import (
	"fmt"
	"log"
	"slices"
	"sort"
	"strconv"
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

// Two procs (petition 1359, 2026-11-11): nested lines that both carry a
// proc (Graal's Final Strike's "stack2 player proc" within Stormblade's
// "stack2 player +INT proc") hold one proc slot between them, so they go as
// one line by rank: the higher rank (the workbook's level) wins, and of two
// of one rank the bigger proc. Until then only their stats were weighed:
// Graal's has none, so Stormblade (49) took Graal's (60) off and Graal's was
// refused under Stormblade, its rank and its bigger proc never looked at.

// A form's buff (petition 1529, 2026-11-11): a workbook spell of the sheet's
// "Alter Appearance" type that is not itself read as an illusion (Scorpion's
// Pestilence, the scorpion form's +DEX +INT +PoT; Form of the Master) has no
// line, as every form has none. Until then Pestilence, of the self stack type
// and sharing +PoT, took the wizard's epic power path buff (Aktrana's
// Cerebral Fortress, 20 minutes) off well before its recast came back.

// BuffLine is a spell's line, "" for one that has none: no lasting part but
// ticks, an illusion, a pet's summoning or a charm. (A spell that lands on a
// player is one cast on a friend or a self spell: Reckless Force's line reads
// as harmful, "Decrease AGI by 10", and is a buff.)
func BuffLine(s *CastSpell) string {
	d := &s.Def
	if d.FormKind != FormNone || d.AltersLook || d.Summons() || d.Charms() {
		return "" // a form's buff (AltersLook) is a form's: no line (petition 1529)
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
		case EffectShield, EffectDefProc, EffectProc, EffectSpeed, EffectInvis, EffectPowerCap, EffectHPCap, EffectStatPin:
			k = buffKindNames[e.Kind]
			if e.Kind == EffectSpeed && e.SowPct != 0 {
				k += fmt.Sprint(e.SowPct) // a speed of its own (hpcap.go): not the movement buff's line
			}
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

// Lines across the two kinds (the owner, 2026-11-12, of three mercs' buffs on
// the GM Jibrish: "buffs are stacking on Jibrish from mercs that normally
// shouldn't"). A spell made from EQOA-Data's words (no workbook stack type)
// has a plain line, and a plain line was only ever one with the same plain
// line: so the cleric's Blessing (+CHA, self) stacked with Minor Blessing, the
// workbook's +CHA self buff; its wards, each rank an element more (Disease
// Ward +DR, Poison Ward +DR +PR, Flame, Ice, Lightning Ward +CR +DR +FR +LR
// +PR), every one on at once; and Lightning Ward's +PR +DR counted in full on
// top of Consume Affliction's, where the user's rule of 2026-11-08 (nested
// lines, above) counts a shared resist the stronger once. Since then:
//
//   - a beneficial effect's line (EffectLine) carries, for a plain line, the
//     workbook stack type its scope would have ("^s2" self, "^s1" a single
//     target, "^s3" a group: the workbook's StackType is its scope's class,
//     the doc above). Two plain lines still ignore it (a plain line's single,
//     self and group forms are one line, as before); a plain line and a
//     workbook line are of one family when the plain line's stack type is
//     the workbook line's and their bearer one (lineFamily);
//   - a plain line and a workbook line of one family with the same stats are
//     one line (Blessing and Minor Blessing); two lines of one family, the
//     one's stats all within the other's, are nested whatever their kinds
//     (Ice Ward within Lightning Ward; Lightning Ward and Consume Affliction),
//     with the nested lines' rule: the one that adds nothing is refused, the
//     one that gives all the other did takes it off, else both stay and each
//     shared stat counts its stronger once (sumEffectMods);
//   - a pet (a merc) keeps its effects by the same rule as a player
//     (lineVerdict, lineDrops: putOnPetLocked).
//
// An effect kept from before (its plain line untagged) is of no workbook
// family until it ends; BuffLine itself (the balance simulator's key) is
// unchanged.

// EffectLine is the line a beneficial effect of the spell is kept under:
// BuffLine, a plain line tagged with its scope's workbook stack type (the doc
// above).
func EffectLine(s *CastSpell) string {
	l := BuffLine(s)
	if l == "" || strings.HasPrefix(l, stackLinePrefix) {
		return l
	}
	if st := scopeStack(s.Scope); st > 0 {
		return fmt.Sprintf("%s %s%d", l, plainStackTag, st)
	}
	return l
}

// plainStackTag marks a plain line's workbook stack type (EffectLine).
const plainStackTag = "^s"

// scopeStack is the workbook stack type of a scope (the doc above): 2 self,
// 1 a single target, 3 a group; 0 for any other (a pet's).
func scopeStack(scope int) int {
	switch scope {
	case 0:
		return 2
	case 1:
		return 1
	case 2:
		return 3
	}
	return 0
}

// lineParts is a line read: its workbook stack type (0 for a plain line), its
// bearer, its stats' keys (sorted, as BuffLine writes them) and, for a plain
// line, the stack type its scope would have (0 unknown).
type lineParts struct {
	stack  int
	bearer string
	keys   []string
	as     int
}

// parseLine reads a line; ok is false for "" or a line too short.
func parseLine(line string) (lp lineParts, ok bool) {
	f := strings.Fields(line)
	if len(f) < 2 {
		return lp, false
	}
	if strings.HasPrefix(f[0], stackLinePrefix) {
		if len(f) < 3 {
			return lp, false
		}
		n, err := strconv.Atoi(strings.TrimPrefix(f[0], stackLinePrefix))
		if err != nil || n <= 0 {
			return lp, false
		}
		lp.stack, lp.bearer, lp.keys = n, f[1], f[2:]
		return lp, true
	}
	lp.bearer = f[0]
	for _, k := range f[1:] {
		if strings.HasPrefix(k, plainStackTag) {
			lp.as, _ = strconv.Atoi(strings.TrimPrefix(k, plainStackTag))
			continue
		}
		lp.keys = append(lp.keys, k)
	}
	return lp, len(lp.keys) > 0
}

// lineFamily reports two read lines of one family (the doc above): one
// bearer, and the same workbook stack type, or both plain, or a plain line
// whose scope's stack type is the workbook line's.
func lineFamily(a, b lineParts) bool {
	if a.bearer != b.bearer {
		return false
	}
	switch {
	case a.stack > 0 && b.stack > 0:
		return a.stack == b.stack
	case a.stack == 0 && b.stack == 0:
		return true
	case a.stack == 0:
		return a.as == b.stack
	default:
		return b.as == a.stack
	}
}

// sameLine reports whether two effects' lines are one (the file's doc): two
// workbook stack lines of one stack type and bearer that share a stat; two
// plain lines, or a plain and a workbook line of one family, of the same
// stats.
func sameLine(a, b string) bool {
	if a == "" || b == "" {
		return false
	}
	if a == b {
		return true
	}
	pa, oka := parseLine(a)
	pb, okb := parseLine(b)
	if !oka || !okb || !lineFamily(pa, pb) {
		return false
	}
	if pa.stack > 0 && pb.stack > 0 {
		for _, k := range pa.keys {
			if slices.Contains(pb.keys, k) {
				return true
			}
		}
		return false
	}
	return slices.Equal(pa.keys, pb.keys)
}

// nestedLines reports two lines nested (the docs above): lines of one family,
// not of the same stats, the one's all among the other's.
func nestedLines(a, b string) bool {
	pa, oka := parseLine(a)
	pb, okb := parseLine(b)
	if !oka || !okb || !lineFamily(pa, pb) || slices.Equal(pa.keys, pb.keys) {
		return false
	}
	return keysWithin(pa.keys, pb.keys) || keysWithin(pb.keys, pa.keys)
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

// procRivals reports nested lines that both carry a proc (the doc above:
// they go as one line by rank).
func procRivals(a, b *ActiveEffect) bool {
	if !nestedLines(a.Line, b.Line) {
		return false
	}
	pa, _ := parseLine(a.Line)
	pb, _ := parseLine(b.Line)
	return hasProcKey(pa.keys) && hasProcKey(pb.keys)
}

// hasProcKey reports a line's keys holding an offensive proc.
func hasProcKey(keys []string) bool {
	return slices.Contains(keys, "proc") || slices.Contains(keys, "proc@self")
}

// procOutranks reports a proc rival above b: of a higher rank, or of one rank
// with the bigger proc.
func procOutranks(a, b *ActiveEffect) bool {
	if a.Rank != b.Rank {
		return a.Rank > b.Rank
	}
	return a.Proc+a.ProcMax > b.Proc+b.ProcMax
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
// stat nested lines share (nestedLines, linked one to another) counts its
// largest once.
func sumEffectMods(effects []*ActiveEffect) StatMods {
	var m StatMods
	for _, e := range effects {
		m.sum(e.Mods)
	}
	keys := make([][]string, len(effects))
	any := false
	for i, e := range effects {
		if lp, ok := parseLine(e.Line); ok {
			keys[i] = lp.keys
			any = true
		}
	}
	if !any {
		return m
	}
	for st := Stat(1); int(st) < len(statLineNames)-1; st++ {
		if st == StatRanged {
			continue
		}
		var on []int // the effects raising st, by their line
		for i, e := range effects {
			if e.Mods.get(st) > 0 && slices.Contains(keys[i], "+"+statLineNames[st]) {
				on = append(on, i)
			}
		}
		if len(on) < 2 {
			continue
		}
		// the nested ones linked: each group's st its largest once
		group := make([]int, len(on))
		for i := range group {
			group[i] = i
		}
		var root func(int) int
		root = func(i int) int {
			for group[i] != i {
				i = group[i]
			}
			return i
		}
		for i := range on {
			for j := i + 1; j < len(on); j++ {
				if nestedLines(effects[on[i]].Line, effects[on[j]].Line) {
					group[root(j)] = root(i)
				}
			}
		}
		total, top := map[int]int{}, map[int]int{}
		count := map[int]int{}
		for i, at := range on {
			r, v := root(i), effects[at].Mods.get(st)
			total[r] += v
			top[r] = max(top[r], v)
			count[r]++
		}
		for r, n := range count {
			if n > 1 {
				m.add(st, top[r]-total[r])
			}
		}
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
	EffectSpeed: "speed", EffectInvis: "invis", EffectPowerCap: "powercap", EffectHPCap: "hpcap", EffectStatPin: "statpin"}

const (
	lineStrongerSelf = "You already have %s, which is stronger."
	lineStronger     = "%s already has %s, which is stronger."
)

// lineTakesLocked marks a beneficial effect landing on a player with its
// spell's line and rank and reports whether it takes (the file's doc): not
// when an effect of its line of a higher rank is on, of which the caster is
// told. Must be called with the lock held.
func (w *World) lineTakesLocked(caster *Player, casterSess *session.Session, t *Player, spell *CastSpell, ae *ActiveEffect) bool {
	ae.Line, ae.Rank = EffectLine(spell), lineRank(spell)
	e := lineVerdict(t.Effects, ae)
	if e == nil {
		return true
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

// lineVerdict is the effect of effects that keeps ae off (the file's doc), nil
// when ae takes: a proc rival above it, a nested line that covers it, or its
// line's higher rank. ae's Line and Rank are set.
func lineVerdict(effects []*ActiveEffect, ae *ActiveEffect) *ActiveEffect {
	if ae.Line == "" {
		return nil
	}
	for _, e := range effects {
		if e.SpellID == ae.SpellID {
			continue
		}
		if procRivals(e, ae) {
			if !procOutranks(e, ae) {
				continue // the higher rank, or the bigger proc (petition 1359)
			}
		} else if nestedLines(e.Line, ae.Line) {
			if !covers(e.Mods, ae.Mods) {
				continue // both on, each shared stat its stronger once (the doc above)
			}
		} else if !sameLine(e.Line, ae.Line) || e.Rank <= ae.Rank {
			continue
		}
		return e
	}
	return nil
}

// lineDrops is the spell ids of effects ae takes off as it goes on
// (lineVerdict let it on): its line's, a proc rival, a nested line it covers.
func lineDrops(effects []*ActiveEffect, ae *ActiveEffect) []int {
	if ae.Line == "" {
		return nil
	}
	var drop []int
	for _, e := range effects {
		if e.SpellID == ae.SpellID {
			continue
		}
		if procRivals(e, ae) {
			drop = append(drop, e.SpellID) // one proc slot: lineTakesLocked let ae on (petition 1359)
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
	return drop
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
		refused := false
		switch {
		case procRivals(e, ae):
			refused = procOutranks(e, ae) // petition 1359
		case nestedLines(e.Line, ae.Line):
			refused = covers(e.Mods, ae.Mods)
		default:
			refused = sameLine(e.Line, ae.Line) && e.Rank > ae.Rank
		}
		if refused {
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
	for _, id := range lineDrops(t.Effects, ae) {
		w.removeEffectLocked(t, id)
	}
}
