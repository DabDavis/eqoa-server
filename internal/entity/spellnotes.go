package entity

import (
	"regexp"
	"strings"
	"time"
)

// spellnotes.go — what EQOA-Data's lines say beside what a spell does, and
// what some of them leave out (F3 20261025-222444: jib's Courage, which did
// nothing, and the cleric spells past level 20 that tools/import_eqoadata_spells.py
// brings in with their lines as the lists word them).

// reStatList is a run of stat names with commas between them before one
// "by": Ice Ward's "buff DR, PR, FR, CR by 100", which the clause split
// would leave as four parts with one number.
var reStatList = regexp.MustCompile(`\b(?:str|sta|agi|dex|wis|int|cha|ac|fr|cr|pr|dr|lr|ar)(?:\s*,\s*(?:str|sta|agi|dex|wis|int|cha|ac|fr|cr|pr|dr|lr|ar))+\s+by\b`)

// joinStatLists writes such a run as one subject, "dr/pr/fr/cr by", which
// parsePart splits into its stats.
func joinStatLists(text string) string {
	return reStatList.ReplaceAllStringFunc(text, func(run string) string {
		run = strings.TrimSuffix(run, " by")
		var names []string
		for _, n := range strings.Split(run, ",") {
			names = append(names, strings.TrimSpace(n))
		}
		return strings.Join(names, "/") + " by"
	})
}

// reNote is a part of a line that is read and left alone (Ignored): a
// group heal's radius ("15rad", "Radius 15"), the hate a buff or heal makes
// ("218 hate", "52hate", "hate 30", "no hate"), which here is no creature's
// hate for anyone (no creature is told of a heal), and the epic lists' and a
// few others' power cost, cast and recast, which the import reads into the
// ability record instead (tools/import_eqoadata_spells.py). "+100 hate", a
// taunt's, is an effect (reHate), and so is "Increase Hate by 750".
var reNote = regexp.MustCompile(`^(?:` +
	`(?:(?:\d+\s*rad(?:ius)?|radius\s*\d+|no hate|\d+\s*hate|hate\s*\d+)\s*)+` +
	`|level 60 epic .*|(?:power cost|cast|recast)\s*:\s*\d+.*` +
	`|\d+\s*(?:second\s+)?recast|faster recast` +
	`)$`)

func spellNote(part string) bool { return reNote.MatchString(part) }

// buffDuration is how long a line that raises stats and names no duration
// lasts: the lists leave it off some ranks of lines whose other ranks say
// 30min (Courage of the cleric's Courage, Center, Daring, Bravery, Valor;
// Resolution and Order, which say none; Consume Affliction beside Resist and
// Absorb Affliction; Ice and Lightning Ward beside Flame Ward). A stat
// change lasting nothing did nothing at all (jib: Courage "currently doesnt
// buff max hp or AC").
const buffDuration = 30 * time.Minute

// lineDefaults fills in a buff's duration (buffDuration) for a line that
// raises stats, names no time and does no harm.
func (d *SpellDef) lineDefaults() {
	if d.Harmful || d.Duration != 0 {
		return
	}
	for _, e := range d.Effects {
		if e.Kind == EffectStat && e.Amount > 0 {
			d.Duration = buffDuration
			return
		}
	}
}

// clericHPLine is the cleric's hit point and armour line, by name: each rank
// raises maximum hit points and AC, and jib (F3 20261025-222444) says it
// "should also heal for the amount of max Hp it buffs". The lists never say
// so for it, where every other line that heals says how much (Rockskin's
// "MaxHP by 30, HP by 30", a warrior's shouts), so the rule goes by name: a
// paladin's Hero Guard or a wizard's Minor Shielding raise both too and are
// left as their lines have them.
var clericHPLine = map[string]bool{
	"courage": true, "center": true, "daring": true, "bravery": true, "valor": true,
	"resolution": true, "order": true, "united faith": true,
}

// healWithMaxHP adds clericHPLine's heal: as much as the line raises the
// maximum, on the same scale, when it says nothing of hit points now.
func (d *SpellDef) healWithMaxHP(name string) {
	n := strings.ToLower(strings.TrimSpace(name))
	if !clericHPLine[strings.TrimPrefix(n, "group ")] {
		return
	}
	var maxHP *SpellEffect
	for i := range d.Effects {
		switch e := &d.Effects[i]; {
		case e.Kind == EffectHP:
			return
		case e.Kind == EffectStat && e.Stat == StatMaxHP && e.Amount > 0:
			maxHP = e
		}
	}
	if maxHP != nil {
		h := *maxHP
		h.Kind, h.Stat = EffectHP, StatNone
		d.Effects = append(d.Effects, h)
	}
}

// RaisesMaxHP reports whether the spell raises its target's maximum hit
// points: its heal then lands after the raise (cast.go), or a target at full
// health would get none of it.
func (d *SpellDef) RaisesMaxHP() bool {
	for _, e := range d.Effects {
		if e.Kind == EffectStat && e.Stat == StatMaxHP && e.Amount > 0 {
			return true
		}
	}
	return false
}
