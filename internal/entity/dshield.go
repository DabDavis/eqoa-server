package entity

import (
	"log"
	"regexp"
	"strings"
	"time"

	"github.com/eqoa/server/internal/session"
)

// dshield.go — damage shields and defensive procs (jib's review, F3 pass
// 19): an effect on a player or a pet that answers a creature's melee hit on
// its bearer. A damage shield takes its damage back off the creature at
// every hit; a defensive proc does at some.
//
// What was recorded: Sandstorm's one shield, a level 50 shadow knight's
// Shroud of Hate fought by the training dummies
// (claude-notes/training-dummies-20261021.md, offence-sandstorm-20261022.md).
// At each of a dummy's hits on him, in the same millisecond, came a 0x00B1 of
// his own at the dummy (cast 0, visual 0x33FBB69A) and a 0x00DB of 52 or 53
// from him to it. Its ability record calls it a "Defensive Proc", EQOA-Data's
// racial masteries a "SELF damage shield"; it answered every hit.
//
// The words, as EQOA-Data's lists and this server's spelleffect rows put
// them (the counts are the lists' 1,693 lines; spelleffect holds the same
// lines by name, and the two records with no line are named below):
//
//   - a shield with its damage, 7: "Damage Shield Decrease Hitpoints by 2"
//     (Divine Protection), "Damage SHield ..." and "damage shield Decrease HP
//     by N" (the ranger's Thistlecoat 4, Barbcoat 8, Bramblecoat 12), "Damage
//     shield, Decrease Mob Hitpoints by 2" (Chilling Aura), "DS Decrease
//     Hitpoints by 10" (O'Keils Embers), "Debuff Hp by 6 Damage Shield"
//     (O'Keils Radiation);
//   - "approx. 27 Damage to Mob when it hits", 1 (the shaman's Spirit Shield);
//   - a shield and no damage, 26: "Damageshield", "Damage Shield",
//     "DamageShield", "Damage shield" (the druid's six thorn shields, five
//     Coats and Protection of Karana; the magician's seven Auras and seven
//     Cloaks, the last two "..., 8min, FR");
//   - a pet's, no damage, 5: "Add Damage Shield", "Add DS", "Add DS Proc" (the
//     magician's Frozen, Burning, Earthen, Shocking and Elemental Mark,
//     "Increase INT by N" beside it);
//   - the ranger's coats by name, 2: Spikecoat and Bladecoat say only
//     "Decrease Hitpoints by 22/30, Increase AC by 110/140", where the three
//     coats below them say the same as a damage shield (rangerCoat);
//   - two records with no line: Shroud of Hate (above) and the dump's
//     O'Keil's Radiation (an apostrophe from EQOA-Data's "O'Keils
//     Radiation"), given lines here (shieldLines).
//
// A GM can write any of them, and these too: "Damage Shield 30" or "Damage
// Shield of 30", "Reflect 30 damage", and a defensive proc, "Add Defensive
// Proc, Decrease Hitpoints by 50" or "Add Defensive Proc 50" (no line has one
// yet; one naming a spell and no damage stays unread, as "Add Offensive
// Proc:" does).
//
// Not read here, and why:
//   - the cleric's Marks and Brands ("damages target when it hits", "Add
//     Offensive Proc, Damage Target when they hit", 10; Inquisitor Brand
//     "Doubles Mark damage"), the druid's Storm Caller's Curse ("Inflicts
//     Damage on mob when it hits") and the cleric's Mark of Faith ("25-27
//     Heal when Mob hits"): put on the creature, they answer ITS hits on
//     anyone, the creature the bearer, and none but Mark of Faith names a
//     number;
//   - Crusader's Aura ("Increase Hitpoints by 5 when Crusader is hit") and
//     Tribal Ward ("waits to work till target is hit, Increase HP per tic by
//     550"): heals set off by a hit, not a shield;
//   - the epics' prose ("up to 125 damage shield", "a damage shield of 75",
//     "Off+Def Pet Proc LR"), read nowhere (spellnotes.go), and Aura Stone's
//     "Summons Aura Stone, 3 charges, damage shield", an item.
//
// The rules (ours where it says so):
//   - a shield answers each melee hit a creature lands on its bearer while
//     the bearer stands after it: a hit that kills the bearer ends its
//     effects (clearSpellsLocked). It answers with its whole damage, no
//     resist: Shroud of Hate's 52-53 came at every one of the dummies' hits
//     on its level 50 bearer;
//   - only the strongest shield on a bearer answers, by its top (ours: a
//     druid's shield over a ranger's coat does not add up);
//   - a defensive proc fires on defProcChancePct of the hits (ours: no list
//     or recording gives a rate, and the offensive procs' INT says nothing of
//     being hit), resisted all or nothing as an offensive proc is (proc.go's
//     procResistPct, less an arcane resist debuff: enchanter.go); each one on
//     the bearer rolls;
//   - either is the bearer's hit, as a spell's: a 0x00B1 of the bearer at
//     the creature (cast 0, Shroud of Hate's visual, fxShieldHit, the one
//     recorded), then the 0x00DB from the bearer, the creature's hate for
//     the bearer, and on a kill the bearer's XP and faction hits (a pet's
//     its owner's, as its hits are: pet.go);
//   - the damage is never the bearer's: the line's "Decrease Hitpoints" is
//     the shield's, read as no hit on anyone, so a shield cannot kill its
//     bearer (Spikecoat's and Bladecoat's lines read as a harmful hit until
//     now: Bladecoat, a Target spell, was refused on a friend);
//   - a line that names no damage does half its spell's level, at least 2
//     (ours: the ranger's coats, 4 at level 1, 8 at 12, 12 at 24, 22 at 44
//     and 30 at 55, are near it): a level 8 Thistleshield 4, a level 60
//     Searing Aura 30. A GM's number in the line is taken instead;
//   - Shroud of Hate does its bearer's level plus 2 to 3: Sandstorm's 52 and
//     53 on a level 50, the level ours (one measurement fixes no scale), and
//     lasts 5 minutes (ours: the other level 1 shields' time);
//   - a shield whose record says it hurts undead answers undead alone
//     (Divine Protection: "A protective aura injures all undead who come in
//     contact with it"; undead.go says which creatures are);
//   - a training dummy's hits are answered as a creature's, the figure told
//     and taken off nothing (dummy.go), as Sandstorm's dummies were by Shroud
//     of Hate; a sparring dummy's melee too, not its element strike (a
//     spell);
//   - a group shield lands on the members, not their pets (ours: no source
//     says; the magician's pet marks are the pets' own).

const (
	// shieldLevelDivisor and shieldLeast make a line's damage when it
	// names none: the spell's level over the divisor, at least the least
	// (ours: see the file's comment).
	shieldLevelDivisor = 2
	shieldLeast        = 2
	// defProcChancePct is how many hits in a hundred a defensive proc
	// answers (ours).
	defProcChancePct = 10
)

// fxShieldHit is the visual of a shield's 0x00B1: Shroud of Hate's on
// Sandstorm, at each hit it answered.
const fxShieldHit uint32 = 0x33FBB69A

// Shield is what an effect does when a creature's melee hit lands on its
// bearer: a damage shield's damage at every hit (Min to Max), a defensive
// proc's at some (ProcMin to ProcMax), undead attackers alone when Undead.
// Kept over a logout in the mods column (buffsave.go).
type Shield struct {
	Min     int  `json:",omitempty"`
	Max     int  `json:",omitempty"`
	ProcMin int  `json:",omitempty"`
	ProcMax int  `json:",omitempty"`
	Undead  bool `json:",omitempty"`
}

// shieldLines are the lines of the shields with no line of their own (a
// line elsewhere wins): Shroud of Hate's (the damage by levelShields, the 5
// minutes ours) and O'Keil's Radiation's, EQOA-Data's "O'Keils Radiation".
var shieldLines = map[string]string{
	"shroud of hate":     "Self, Damage Shield, 5min",
	"o'keil's radiation": "Self Debuff Hp by 6 Damage Shield 5min",
}

func init() {
	for name, line := range shieldLines {
		if defaultSpellLines[name] == "" {
			defaultSpellLines[name] = line
		}
	}
}

// levelShields are the shields whose damage goes with their bearer's level,
// its least and most above it, when the line names none: Shroud of Hate's
// 52-53 on Sandstorm's level 50 (the level ours).
var levelShields = map[string][2]int{
	"shroud of hate": {2, 3},
}

// The ways the lines say a shield (see the file's comment), all lower case;
// shieldWords writes each as "dshield [N [to M]]" or "defproc N [to M]",
// which shieldPart reads.
var (
	hpWords      = `(?:mob\s+|target\s+)?(?:hitpoints|hit points|hp|health)`
	reShieldHit  = regexp.MustCompile(`(?:\bdamage ?shield|\bds)\b[\s,]*(?:decrease|debuff|lower)\s+` + hpWords + `\s+by\s+(\d+)(?:\s+to\s+(\d+))?`)
	reHitShield  = regexp.MustCompile(`\b(?:decrease|debuff|lower)\s+` + hpWords + `\s+by\s+(\d+)(?:\s+to\s+(\d+))?\s+(?:damage ?shield|ds)\b`)
	reShieldOf   = regexp.MustCompile(`(?:\bdamage ?shield|\bds)\s+(?:of\s+)?(\d+)(?:\s+to\s+(\d+))?\b`)
	reReflect    = regexp.MustCompile(`\breflects?\s+(\d+)(?:\s+to\s+(\d+))?\s+(?:points?\s+of\s+)?damage\b`)
	reMobHits    = regexp.MustCompile(`(?:\bapprox\.?\s+|\babout\s+)?\b(\d+)(?:\s+to\s+(\d+))?\s+damage\s+to\s+(?:the\s+)?(?:mob|attacker)\s+when\s+it\s+hits\b`)
	reShieldBare = regexp.MustCompile(`(?:\badd\s+)?(?:\bdamage ?shield|\bds)\b(?:\s+proc\b)?`)
	reDefProcHit = regexp.MustCompile(`(?:\badd\s+)?\bdefensive\s+proc\b:?[\s,]*(?:decrease|debuff|lower)\s+` + hpWords + `\s+by\s+(\d+)(?:\s+to\s+(\d+))?`)
	reDefProcN   = regexp.MustCompile(`(?:\badd\s+)?\bdefensive\s+proc\b:?\s+(\d+)(?:\s+to\s+(\d+))?\b`)
	// what a shield's line may carry after it: its element ("8min, FR"; no
	// creature resists one here) and a note ("10min - Enhances Pet")
	reShieldElement = regexp.MustCompile(`,\s*(?:fr|cr|lr|ar|pr|dr)\s*$`)
	reShieldNote    = regexp.MustCompile(`\s+-\s+[a-z ]+$`)
	reShieldToken   = regexp.MustCompile(`^(dshield|defproc)(?:\s+(\d+)(?:\s+to\s+(\d+))?)?$`)
)

// shieldWords writes a lower-case line's shields in the words shieldPart
// reads. A summoning line (Aura Stone's item) and an epic's prose are left
// as they are.
func shieldWords(text string) string {
	if reSummons.MatchString(text) || strings.HasPrefix(text, "level 60 epic") {
		return text
	}
	token := func(word string) func(string, []string) string {
		return func(_ string, m []string) string {
			if m[2] != "" {
				return word + " " + m[1] + " to " + m[2]
			}
			return word + " " + m[1]
		}
	}
	for _, r := range []struct {
		re   *regexp.Regexp
		with func(string, []string) string
	}{
		{reDefProcHit, token("defproc")}, {reDefProcN, token("defproc")},
		{reShieldHit, token("dshield")}, {reHitShield, token("dshield")}, {reShieldOf, token("dshield")},
		{reReflect, token("dshield")}, {reMobHits, token("dshield")},
	} {
		text = replaceSubmatches(r.re, text, r.with)
	}
	text = reShieldBare.ReplaceAllString(text, "dshield")
	if strings.Contains(text, "dshield") || strings.Contains(text, "defproc") {
		text = reShieldNote.ReplaceAllString(reShieldElement.ReplaceAllString(text, ""), "")
	}
	return text
}

// replaceSubmatches is ReplaceAllStringFunc with each match's groups.
func replaceSubmatches(re *regexp.Regexp, s string, with func(string, []string) string) string {
	return re.ReplaceAllStringFunc(s, func(m string) string {
		return with(m, re.FindStringSubmatch(m))
	})
}

// shieldPart reads one part of a line shieldWords wrote: a damage shield or
// a defensive proc, its damage 0 when the line names none.
func shieldPart(part string) (SpellEffect, bool) {
	m := reShieldToken.FindStringSubmatch(part)
	if m == nil {
		return SpellEffect{}, false
	}
	e := SpellEffect{Kind: EffectShield, Amount: atoi(m[2])}
	if m[1] == "defproc" {
		e.Kind = EffectDefProc
	}
	e.AmountMax = max(atoi(m[3]), e.Amount)
	return e, true
}

// rangerCoat reports whether a spell of this name is one of the ranger's
// coats (Thistlecoat ... Bladecoat: one word ending in "coat"; the druid's
// Static Coat and the rest are two).
func rangerCoat(name string) bool {
	n := strings.ToLower(strings.TrimSpace(name))
	return strings.HasSuffix(n, "coat") && !strings.Contains(n, " ")
}

// readShield is what the spell's name and description say of its shield: a
// ranger's coat's hit is its damage shield, and a shield whose description
// says it hurts undead answers undead alone.
func (d *SpellDef) readShield(name, desc string) {
	if rangerCoat(name) && !d.hasShield() {
		for i, e := range d.Effects {
			if e.Kind == EffectHP && e.Amount < 0 && !e.Self {
				d.Effects = append([]SpellEffect(nil), d.Effects...)
				d.Effects[i] = SpellEffect{Kind: EffectShield, Amount: -e.AmountMax, AmountMax: -e.Amount,
					Scale: e.Scale, ScalePct: e.ScalePct, Cap: e.Cap}
				d.harmfulAgain()
				break
			}
		}
	}
	if strings.Contains(strings.ToLower(desc), "undead") {
		for i := range d.Effects {
			if k := d.Effects[i].Kind; k == EffectShield || k == EffectDefProc {
				d.Effects[i].Undead = true
			}
		}
	}
}

// hasShield reports whether the spell is a damage shield or a defensive
// proc.
func (d *SpellDef) hasShield() bool {
	for _, e := range d.Effects {
		if e.Kind == EffectShield || e.Kind == EffectDefProc {
			return true
		}
	}
	return false
}

// harmfulAgain works out whether the spell is cast at an enemy once one of
// its effects changed, from its effects and target as ParseSpellText does.
func (d *SpellDef) harmfulAgain() {
	d.Harmful = d.Target == TargetUndead
	for _, e := range d.Effects {
		switch e.Kind {
		case EffectHP, EffectHPTick, EffectStat:
			d.Harmful = d.Harmful || (e.Amount < 0 && !e.Self)
		case EffectRoot, EffectSnare, EffectHate, EffectCharm:
			d.Harmful = true
		}
	}
}

// with is the shield with one of the cast's shield effects added, its
// damage worked out from its caster at the landing: the line's, plus its
// scale ("; Increase N% based on WIS") and held to its cap; a line that
// names none, shieldDefault's.
func (s *Shield) with(e SpellEffect, cs *castState, caster *Player) *Shield {
	var out Shield
	if s != nil {
		out = *s
	}
	lo, hi := e.Amount, max(e.AmountMax, e.Amount)
	if lo <= 0 && e.Kind == EffectShield {
		lo, hi = shieldDefault(cs.spell.Name, cs.spell.Level, caster)
	}
	bonus := procBonus(e, caster)
	lo, hi = capAmount(lo+bonus, e.Cap), capAmount(hi+bonus, e.Cap)
	if e.Kind == EffectDefProc {
		out.ProcMin, out.ProcMax = lo, hi
	} else if hi > out.Max {
		out.Min, out.Max = lo, hi
	}
	out.Undead = out.Undead || e.Undead
	return &out
}

// shieldDefault is the damage of a shield whose line names none: its
// bearer's level and levelShields' for one of those (the caster's: they are
// self spells), else half the spell's level, at least shieldLeast (ours).
func shieldDefault(name string, level int, caster *Player) (int, int) {
	if r, ok := levelShields[strings.ToLower(strings.TrimSpace(name))]; ok && caster != nil {
		return caster.Level + r[0], caster.Level + r[1]
	}
	n := max(level/shieldLevelDivisor, shieldLeast)
	return n, n
}

// shieldHit is one answer to a creature's hit: its damage, the effect it is
// of, and whether a defensive proc made it.
type shieldHit struct {
	n    int
	name string
	proc bool
}

// shieldAnswers is what the bearer's effects answer one creature's melee hit
// with (see the file's comment): the strongest damage shield on, and each
// defensive proc on at its chance, less the ones the creature resists.
// undead says whether the creature is (asked only of a shield for undead
// alone); resistOff is what arcane resist debuffs take off its resist.
func shieldAnswers(effects []*ActiveEffect, bearerLevel, attackerLevel, resistOff int, undead func() bool,
	now time.Time, intn func(int) int) (hits []shieldHit, resisted int) {
	on := func(ae *ActiveEffect) bool {
		return ae.Shield != nil && now.Before(ae.Until) && (!ae.Shield.Undead || undead())
	}
	var best *ActiveEffect
	for _, ae := range effects {
		if ae.Shield != nil && ae.Shield.Max > 0 && (best == nil || ae.Shield.Max > best.Shield.Max) && on(ae) {
			best = ae
		}
	}
	roll := func(lo, hi int) int {
		if hi > lo {
			return lo + intn(hi-lo+1)
		}
		return lo
	}
	if best != nil {
		hits = append(hits, shieldHit{n: roll(best.Shield.Min, best.Shield.Max), name: best.Name})
	}
	for _, ae := range effects {
		if ae.Shield == nil || ae.Shield.ProcMax <= 0 || !on(ae) {
			continue
		}
		if intn(100) >= defProcChancePct {
			continue
		}
		if intn(100) < procResistPct(bearerLevel, attackerLevel)-resistOff {
			resisted++
			continue
		}
		hits = append(hits, shieldHit{n: roll(ae.Shield.ProcMin, ae.Shield.ProcMax), name: ae.Name, proc: true})
	}
	return hits, resisted
}

// playerShieldsLocked answers a creature's melee hit on a player with the
// player's shields: each answer a 0x00B1 of the player's at the creature and
// its damage as the player's spell's (spellHitLocked: the figure, the hate,
// a kill's XP and faction; a dummy's told, taken off nothing). Called after
// the hit, while the player stands. It reports the creature's death.
func (w *World) playerShieldsLocked(sess *session.Session, p *Player, npc *NPC, now time.Time) bool {
	if len(p.Effects) == 0 || p.IsDead() || npc.Dead {
		return false
	}
	hits, resisted := shieldAnswers(p.Effects, p.Level, npc.Level, npcResistOffPct(npc, now),
		func() bool { return w.undeadLocked(npc) }, now, procIntn)
	if resisted > 0 {
		log.Printf("Shield: %s's defensive proc resisted by %s", p.Name, npc.Name)
	}
	for _, h := range hits {
		w.broadcastCastLocked(p, npc.ServerID, 0, fxShieldHit)
		log.Printf("Shield: %s's %q answers %s for %d (proc %v)", p.Name, h.name, npc.Name, h.n, h.proc)
		if w.spellHitLocked(p, sess, npc, h.n) {
			return true
		}
	}
	return false
}

// petShieldsLocked answers a creature's melee hit on a pet with the pet's
// shields, as its owner's player's are: its 0x00B1 the pet's, the damage
// its owner's as the pet's hits are (petDamageLocked). Called after the
// hit, while the pet stands. It reports the creature's death.
func (w *World) petShieldsLocked(pet, npc *NPC, now time.Time) bool {
	ps := w.pets[pet.Owner]
	if ps == nil || ps.npc != pet || len(pet.Effects) == 0 || pet.Dead || npc.Dead {
		return false
	}
	hits, resisted := shieldAnswers(pet.Effects, pet.Level, npc.Level, npcResistOffPct(npc, now),
		func() bool { return w.undeadLocked(npc) }, now, procIntn)
	if resisted > 0 {
		log.Printf("Shield: the pet %q's defensive proc resisted by %s", pet.Name, npc.Name)
	}
	for _, h := range hits {
		w.broadcastNPCSwingLocked(pet, npc.ServerID, 0, fxShieldHit)
		log.Printf("Shield: the pet %q's %q answers %s for %d (proc %v)", pet.Name, h.name, npc.Name, h.n, h.proc)
		if w.petDamageLocked(ps, npc, h.n, now) {
			return true
		}
	}
	return false
}
