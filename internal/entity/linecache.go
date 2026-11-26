package entity

import (
	"slices"
	"sync"
	"sync/atomic"
)

// linecache.go — a buff line read once, not every tick (the owner 2026-11-26:
// "cache the other items like mercs"; pprof of the Heirloom shard, ~68
// players: mercSystem 11% of its CPU, of it parseLine 1.6%, strings.Fields
// 1.4%, EffectLine 0.9%, BuffLine 0.8%, sameLine 0.8%, sumEffectMods 2.1%).
//
// Every merc's turn weighs each buff of its book against the effects on
// itself, its owner and its mates (buffOn: EffectLine, sameLine, lineVerdict),
// and every tick sums its effects (syncMercBody: sumEffectMods). Each of those
// read lines again from their text (parseLine: strings.Fields, an Atoi, a new
// slice) and made the book spell's line again (BuffLine: a map, a sort, a
// Sprintf), though a line's text and a spell's figures change only when the
// spell does. Two caches since:
//
//   - parseLine's reading of a line, by the line's text. A line is a string
//     and parseLine a function of it alone, so a text read once reads the same
//     every time: nothing invalidates it. Its keys are shared by every caller
//     and clipped (cap = len), so an append copies; no caller writes into them
//     (they are compared and searched: categoryKeys, keysWithin, lineStrength,
//     sumEffectMods). A reading keeps its keys' category too (whether they
//     are resists alone, and the keys without their resists: categoryParts),
//     so two lines weighed against each other make no new slice. Bounded at
//     lineCacheMax texts, past which a text is read as before, uncached (the
//     lines are the spells', a few thousand).
//   - EffectLine and BuffLine of a spell, by its *CastSpell, with a copy of
//     every figure BuffLine reads (spellLineInputs): a memo is used only when
//     the spell still has exactly those figures. The World keeps no spell
//     catalog to reload (a spell is made from its rows, SpellOfRow, whenever
//     a book, a kit or a cast is made), so rather than hang the memo on a
//     reload it checks the spell itself: a spell made again from edited rows
//     (the GM panel's), one edited in place (a potion's or an area's copy
//     changed after it was made) or a kit's made again is read again at its
//     next use, as if nothing had been kept. Bounded at spellMemoMax spells, the
//     whole memo dropped past it: a cast's temporary copies of its spell (a
//     potion's, a share's) come and go, and a kept one keeps its ~1.5 KB
//     alive until the drop (4,096 of them ~6 MB at most); the mercs' books
//     are a few thousand spells at a shard's busiest.

// lineCacheMax bounds the read lines (the doc above): texts of a few dozen
// bytes.
const lineCacheMax = 1 << 14

// spellMemoMax bounds the spells' kept lines (the doc above).
const spellMemoMax = 1 << 12

// parsedLine is a line's parseLine, kept.
type parsedLine struct {
	lp lineParts
	ok bool
}

var (
	parsedLines     sync.Map // string -> parsedLine
	parsedLineCount atomic.Int64
)

// parseLine reads a line; ok is false for "" or a line too short. A text read
// before is its first reading (the file's doc).
func parseLine(line string) (lp lineParts, ok bool) {
	if line == "" {
		return lp, false // parseLineText's answer for no text, without the lookup
	}
	if lineCachesOff {
		return parseLineText(line)
	}
	if v, hit := parsedLines.Load(line); hit {
		r := v.(parsedLine)
		return r.lp, r.ok
	}
	lp, ok = parseLineText(line)
	lp.keys, lp.nonResist = slices.Clip(lp.keys), slices.Clip(lp.nonResist)
	if parsedLineCount.Load() < lineCacheMax {
		if _, loaded := parsedLines.LoadOrStore(line, parsedLine{lp, ok}); !loaded {
			parsedLineCount.Add(1)
		}
	}
	return lp, ok
}

// spellLineInputs is every figure of a spell BuffLine and EffectLine read
// (the file's doc): BuffLine's own, Summons' and Charms' effect kinds, and
// lineKeys' parts of each effect.
type spellLineInputs struct {
	spellID     int
	name        string
	recastGroup string
	scope       int
	formKind    FormKind
	altersLook  bool
	workbook    bool
	stack       int
	effects     []lineEffectInputs
}

// lineEffectInputs is what of an effect lineKeys, Summons and Charms read.
type lineEffectInputs struct {
	kind     EffectKind
	negative bool // Amount < 0: a stat's sign
	stat     Stat
	sowPct   int
	self     bool
}

// matches reports s's figures those of in.
func (in *spellLineInputs) matches(s *CastSpell) bool {
	d := &s.Def
	if in.spellID != s.SpellID || in.scope != s.Scope || in.name != s.Name || in.recastGroup != s.RecastGroup ||
		in.formKind != d.FormKind || in.altersLook != d.AltersLook || in.workbook != d.Workbook || in.stack != d.Stack ||
		len(in.effects) != len(d.Effects) {
		return false
	}
	for i := range d.Effects {
		e := &d.Effects[i]
		if in.effects[i] != (lineEffectInputs{e.Kind, e.Amount < 0, e.Stat, e.SowPct, e.Self}) {
			return false
		}
	}
	return true
}

// spellLineInputsOf is s's figures, copied.
func spellLineInputsOf(s *CastSpell) spellLineInputs {
	d := &s.Def
	in := spellLineInputs{spellID: s.SpellID, name: s.Name, recastGroup: s.RecastGroup, scope: s.Scope,
		formKind: d.FormKind, altersLook: d.AltersLook, workbook: d.Workbook, stack: d.Stack,
		effects: make([]lineEffectInputs, len(d.Effects))}
	for i := range d.Effects {
		e := &d.Effects[i]
		in.effects[i] = lineEffectInputs{e.Kind, e.Amount < 0, e.Stat, e.SowPct, e.Self}
	}
	return in
}

// spellLines is a spell's BuffLine and EffectLine, with the figures they were
// made of.
type spellLines struct {
	in           spellLineInputs
	buff, effect string
}

var spellLineMemo struct {
	mu sync.Mutex
	m  map[*CastSpell]*spellLines
}

// spellLinesOf is s's lines: kept ones while s's figures are still theirs,
// else made again (the file's doc).
func spellLinesOf(s *CastSpell) *spellLines {
	if lineCachesOff {
		buff := buffLineOf(s)
		return &spellLines{buff: buff, effect: effectLineOf(s, buff)}
	}
	spellLineMemo.mu.Lock()
	l := spellLineMemo.m[s]
	spellLineMemo.mu.Unlock()
	if l != nil && (spellMemoTrusted || l.in.matches(s)) {
		return l
	}
	buff := buffLineOf(s)
	l = &spellLines{in: spellLineInputsOf(s), buff: buff, effect: effectLineOf(s, buff)}
	spellLineMemo.mu.Lock()
	if spellLineMemo.m == nil || len(spellLineMemo.m) >= spellMemoMax {
		spellLineMemo.m = make(map[*CastSpell]*spellLines)
	}
	spellLineMemo.m[s] = l
	spellLineMemo.mu.Unlock()
	return l
}

// effectModsMemo is a bearer's sumEffectMods kept with what it was summed of
// (syncMercBody's, every merc every tick: pprof 2.1% of a shard's CPU): each
// effect, its Mods and its Line, the only figures sumEffectMods reads. A sum
// is used again only while the bearer has the same effects, in the same
// order, with the same Mods and Lines, so any change (an effect on or off, a
// recast's new Mods, an effect replaced in place) sums again.
type effectModsMemo struct {
	valid   bool
	effects []*ActiveEffect
	mods    []StatMods
	lines   []string
	out     StatMods
}

// sum is sumEffectMods(effects), kept (the doc above).
func (c *effectModsMemo) sum(effects []*ActiveEffect) StatMods {
	if lineCachesOff {
		return sumEffectMods(effects)
	}
	if c.valid && modsMemoTrusted {
		return c.out
	}
	if c.valid && len(c.effects) == len(effects) {
		same := true
		for i, e := range effects {
			if c.effects[i] != e || c.mods[i] != e.Mods || c.lines[i] != e.Line {
				same = false
				break
			}
		}
		if same {
			return c.out
		}
	}
	c.out = sumEffectMods(effects)
	c.effects, c.mods, c.lines = c.effects[:0], c.mods[:0], c.lines[:0]
	for _, e := range effects {
		c.effects = append(c.effects, e)
		c.mods = append(c.mods, e.Mods)
		c.lines = append(c.lines, e.Line)
	}
	c.valid = true
	return c.out
}

// lineCachesOff has every line read and summed again at each use, as before
// the caches; spellMemoTrusted and modsMemoTrusted have the spell memo and
// the sum memo used without their checks. All are the tests' alone
// (linecache_test.go: the old path to hold the caches to, and the controls
// that show a memo left unchecked goes stale).
var lineCachesOff, spellMemoTrusted, modsMemoTrusted bool
