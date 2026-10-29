package entity

import (
	"log"
	"regexp"
	"strings"
)

// cure.go — spells that take harmful effects off their target: the druid's,
// shaman's and cleric's antidotes (poison) and curatives (disease, the lists'
// "Infection"), the monk's Purity line (both), the "Dispel Effect" of the
// shaman's Savage and Tribal Healing and of the alchemist's Restorative, a
// spell; the owner's ruling of 2026-10-29: cures take off "poisons and
// diseases of matching or similar level or lower with top ranks removing 2 at
// a time, unless otherwise specific on the debuff". Until now their lines
// were unread, and a cast of one did nothing.
//
// The words (a line's clause, any target word before it; "This spell" may
// lead it), as EQOA-Data's lists and this server's spelleffect rows put them:
//   - "Removes Minor Infection", "This spell removes Minor Poison", "Removes
//     Lesser Poison", "removes Greater Infection", "remove Major Poison",
//     "removes poison", "Removes Infection": poison, or disease; the rank word
//     (minor, lesser, greater, major, vital) is the cure's name for its
//     level, which is its record's;
//   - "Removes poison and disease" and the monk's "Cures Posions" (the lists'
//     spelling): both, or poison;
//   - "Dispel Effect" (and "Dispel Effects", "Cure Effect", "Remove Harmful
//     Effects"), with " xN" or not: any harmful effect, of a kind or none;
//   - a GM may write "Cure Poison", "Cure Disease", "Removes Poison x2".
//
// What kind an effect is (ours): what its spell's name and line say, in whole
// words: poison, venom (envenomed, venomous), toxic, toxin, sting -> poison;
// disease, plague, infection (infected), rot, rotting, sickness, sick, fever
// -> disease; a spell may be both. A resist named in the line ("Debuff Resist
// Poison by 10") says what it resists, not what the effect is, and counts for
// nothing. An effect that is neither is taken off by "Dispel Effect" alone.
//
// The rules (ours where it says so):
//   - a cure is no harmful spell: it lands on whom its line and record say, a
//     player or a pet (cast.go landOnPlayerLocked, pet.go landOnPetLocked);
//   - it takes off the harmful effects of its kind (an effect that takes hit
//     points each tick, holds, slows or lowers a stat: what a potion's cure
//     took off until now) whose own spell's level is at most the cure's level
//     plus CureLevelMargin (ours, of "matching or similar level or lower"),
//     the oldest first;
//   - as many as its count: the line's xN, else CureTopCount for a cure of
//     CureTopLevel or more (ours, of "top ranks": the Major and Vital ranks,
//     levels 44 and 55, and the shaman's level 49 heals' Dispel Effect), else
//     one;
//   - "unless otherwise specific on the debuff": a harmful line saying it
//     "cannot be cured" (or "can't be cured", "incurable", "uncurable") is
//     taken off by no cure; no line here says so yet;
//   - a potion's cure (potion.go: a corrective's or a restorative's "Dispel
//     Effect xN") is these rules but for the level: an itemeffect row names
//     none, so it takes off effects of any level, as it always did;
//   - an effect keeps its spell's level and kind over a logout (buffsave.go);
//     a row kept before reads as level 0 and no kind.
//
// No creature casts a poison or a disease on a player yet (creatures cast
// nothing at players but the sparring dummy's element strike, an instant hit:
// sparring.go), so a cure finds little to take off: a player's own harmful
// effects (a form's loss each tick, lifetap.go), which "Dispel Effect" takes
// as a potion's cure always did, and on a pet nothing. It is ready for when
// creatures cast; the potions' cure, players' own harmful effects and pets
// follow these same rules.

const (
	// CureLevelMargin is how far above a cure's level an effect it takes off
	// may be (ours: see the file's comment).
	CureLevelMargin = 5
	// CureTopLevel is the level from which a cure takes off CureTopCount
	// effects at once (ours: the top ranks).
	CureTopLevel = 44
	CureTopCount = 2
)

// CureKind is what kind of harm a cure takes off, and what kind an effect is:
// poison, disease or both; CureAny (a cure's alone) any harmful effect.
type CureKind uint8

const (
	CurePoison CureKind = 1 << iota
	CureDisease
	CureAny
)

// The words of a cure's clause, lower case, and of a debuff no cure takes
// off.
var (
	reCure   = regexp.MustCompile(`^(?:this\s+spell\s+)?(?:removes?|cures?)\s+(?:(?:minor|lesser|greater|major|vital)\s+)?(poison|posion|disease|infection)s?(?:\s+(?:and|&)\s+(poison|posion|disease|infection)s?)?(?:\s+x\s*(\d+))?$`)
	reDispel = regexp.MustCompile(`^(?:dispels?|cures?|removes?)\s+(?:harmful\s+)?effects?(?:\s+x\s*(\d+))?$`)
	reNoCure = regexp.MustCompile(`^(?:cannot|can't|can not)\s+be\s+(?:cured|dispelled)$|^(?:incurable|uncurable)$`)
)

// curePart reads a cure's clause: Cure what it takes off, Amount its count
// (0: by its level, cureOf).
func curePart(part string) (SpellEffect, bool) {
	if m := reDispel.FindStringSubmatch(part); m != nil {
		n := atoi(m[1])
		return SpellEffect{Kind: EffectCure, Cure: CureAny, Amount: n, AmountMax: n}, true
	}
	m := reCure.FindStringSubmatch(part)
	if m == nil {
		return SpellEffect{}, false
	}
	n := atoi(m[3])
	return SpellEffect{Kind: EffectCure, Cure: cureWord(m[1]) | cureWord(m[2]), Amount: n, AmountMax: n}, true
}

func cureWord(w string) CureKind {
	switch w {
	case "poison", "posion":
		return CurePoison
	case "disease", "infection":
		return CureDisease
	}
	return 0
}

// The words that make an effect a poison or a disease (see the file's
// comment), and a resist's name, which makes it neither.
var (
	rePoisonWord  = regexp.MustCompile(`\b(?:poison\w*|posion\w*|\w*venom\w*|toxi[cn]\w*|stings?|stinging)\b`)
	reDiseaseWord = regexp.MustCompile(`\b(?:disease\w*|plague\w*|infect\w*|rot|rots|rotting|sickness|sick|fever\w*)\b`)
	reResistWord  = regexp.MustCompile(`\bresist\s+(?:poison|disease)\b|\b(?:poison|disease)\s+resist\w*\b`)
)

// readAffliction is what kind of harm the spell does, by its name's and its
// line's words.
func (d *SpellDef) readAffliction(name, line string) {
	text := strings.ToLower(name) + " " + reResistWord.ReplaceAllString(strings.ToLower(line), "")
	if rePoisonWord.MatchString(text) {
		d.Afflicts |= CurePoison
	}
	if reDiseaseWord.MatchString(text) {
		d.Afflicts |= CureDisease
	}
}

// harmful reports whether an effect does its bearer harm: takes hit points
// each tick, holds or slows it, or lowers a stat.
func (ae *ActiveEffect) harmful() bool {
	return ae.HPTick < 0 || ae.Root || ae.Snare || ae.Mods.harmful()
}

// cure is one cure as it lands: what it takes off, how many, and the highest
// level of effect it reaches (anyLevel: a potion's, any).
type cure struct {
	kind     CureKind
	count    int
	reach    int
	anyLevel bool
}

// cureOf is a cure effect of a spell of that level as it lands.
func cureOf(e SpellEffect, level int) cure {
	n := e.Amount
	if n <= 0 {
		n = 1
		if level >= CureTopLevel {
			n = CureTopCount
		}
	}
	return cure{kind: e.Cure, count: n, reach: level + CureLevelMargin}
}

// takes reports whether the cure takes ae off.
func (c cure) takes(ae *ActiveEffect) bool {
	if !ae.harmful() || ae.NoCure || (!c.anyLevel && ae.Level > c.reach) {
		return false
	}
	return c.kind&CureAny != 0 || c.kind&ae.Afflicts != 0
}

// cured is what the cure takes off a bearer's effects: up to its count of
// those it takes, the oldest first.
func (c cure) cured(effects []*ActiveEffect) []*ActiveEffect {
	var out []*ActiveEffect
	for _, ae := range effects {
		if len(out) >= c.count {
			break
		}
		if c.takes(ae) {
			out = append(out, ae)
		}
	}
	return out
}

// curePlayerLocked takes off a player what the cure takes. by names it for
// the log.
func (w *World) curePlayerLocked(p *Player, c cure, by string) {
	for _, ae := range c.cured(p.Effects) {
		w.removeEffectLocked(p, ae.SpellID)
		log.Printf("Cure: %q takes %q off %s", by, ae.Name, p.Name)
	}
}

// curePetLocked takes off a pet what the cure takes.
func (w *World) curePetLocked(pet *NPC, c cure, by string) {
	for _, ae := range c.cured(pet.Effects) {
		dropNPCEffect(pet, ae)
		log.Printf("Cure: %q takes %q off the pet %q", by, ae.Name, pet.Name)
	}
}

// dropNPCEffect takes one effect off an NPC's list.
func dropNPCEffect(n *NPC, ae *ActiveEffect) {
	for i, e := range n.Effects {
		if e == ae {
			n.Effects = append(n.Effects[:i], n.Effects[i+1:]...)
			return
		}
	}
}
