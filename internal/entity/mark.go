package entity

import (
	"log"
	"regexp"
	"strings"
	"time"

	"github.com/eqoa/server/internal/session"
)

// mark.go — the cleric's marks and brands: a curse on a creature that
// answers the players' hits on it (the owner's ruling of 2026-10-29: "cleric
// marks and brands were reverse damage shields or heals. Eg; put it on a mob,
// if you attack the mob the effect happens (heals you) or the mob takes
// damage."). A damage shield answers a creature's hits on its bearer
// (dshield.go); a mark is on the creature and answers the hits on it.
//
// The words, as EQOA-Data's lists and this server's spelleffect rows put
// them (eleven spells; until now their lines were unread and the spells,
// read as helping no one, were refused at a creature):
//
//   - a damage mark naming no damage, 10: "Target, damages target when it
//     hits, 42sec" (the cleric's Mark and Brand of Pain, Agony, Anguish and
//     Sorrow) and "Target, Add Offensive Proc, Damage Target when they hit,
//     42sec" (Mark and Brand of Woe), its "Add Offensive Proc" the mark's
//     own words, read with it;
//   - a heal mark with its figure, 1: "Target, 25-27 Heal when Mob hits,
//     30sec" (Mark of Faith).
//
// A GM may write a figure into a damage mark, "Damage Target by 30 when they
// hit" (or "for 30", "30 to 40"), and leave a heal mark's out ("Heal when Mob
// hits").
//
// Not read here: the druid's Storm Caller's Curse ("Inflicts Damage on mob
// when it hits"), which is no cleric's (the ruling names the cleric's), and
// the cleric's Inquisitor Brand mastery, of which this server has no line
// (EQOA-Data's masteries list says it "Doubles Mark damage"; no live line is
// named for it).
//
// The rules (ours where it says so):
//   - the line is cast at an enemy (harmful: refused at a friend), and the
//     mark is on the creature for the line's time; its landing puts the
//     caster on the creature's hate list as a hold's does, with no hate, so
//     it starts no fight by itself (ours; cast.go landOnNPCLocked);
//   - while it lasts, each hit a player or a player's pet lands on the
//     creature that leaves it standing is answered, after the hit's procs
//     (proc.go): a player's auto-attack hits (a bow's shot too, as a proc
//     answers one; not a thrown potion's, a spell's or a tick's: attack.go),
//     a pet's hits and its taunt's (pet.go);
//   - a damage mark does its figure more to the creature as the attacker's
//     hit, as a proc's or a damage shield's answer is (spellHitLocked,
//     petDamageLocked): the 0x00DB from the attacker (a pet's owner), the
//     creature's hate for the attacker (the pet), and on a kill the
//     attacker's XP and faction hits; unresisted (ours), with no 0x00B1 of
//     its own (ours, as a proc's: no recording has a mark); a training
//     dummy is told and loses nothing (dummy.go);
//   - a heal mark heals the attacker (a pet too) its figure, a range rolled
//     within it, at most to its maximum, told as a heal over time is: from
//     the mark's caster while it is in the world, else from the attacker
//     (cast.go tickEffectLocked); it makes no hate (ours: no creature is told
//     of a heal here, spellnotes.go);
//   - the figure is the line's own number; else half the spell's level, at
//     least 2 (dshield.go's shieldLevelDivisor and shieldLeast, the rule for
//     a shield naming none: ours), and markBrandFactor times that for a
//     spell named "Brand of ..." (ours: the masteries list says the cleric's
//     Inquisitor Brand "Doubles Mark damage", and the brands are taken for
//     the doubled marks): a level 30 mark does 15, a level 30 brand 30;
//   - one mark on a creature counts, the strongest by its top (ours, as only
//     the strongest damage shield answers: dshield.go), whether it heals or
//     hurts; of equals the first put on;
//   - a creature's marks are not kept: they end with the line's time or the
//     creature's death.

// markBrandFactor is how many times a mark's figure a spell named "Brand of
// ..." does when its line names none (ours: see the file's comment).
const markBrandFactor = 2

// Mark is what a curse on a creature does at each player's or pet's hit on
// it: Min to Max more damage, or as much healing for the attacker when Heal.
type Mark struct {
	Min, Max int
	Heal     bool
}

// The ways the lines say a mark (see the file's comment), lower case;
// markWords writes each as "mark [N [to M]]" or "markheal [N [to M]]", which
// markPart reads.
var (
	reMarkHit   = regexp.MustCompile(`(?:\badd\s+offensive\s+proc\s*,\s*)?\bdamages?\s+target\s+(?:(?:for|by)\s+(\d+)(?:\s*(?:to|-)\s*(\d+))?\s+)?when\s+(?:it|they)\s+hits?\b`)
	reMarkHeal  = regexp.MustCompile(`(?:\b(\d+)(?:\s*(?:to|-)\s*(\d+))?\s+)?\bheals?\s+when\s+(?:the\s+)?mob\s+hits?\b`)
	reMarkToken = regexp.MustCompile(`^(mark|markheal)(?:\s+(\d+)(?:\s+to\s+(\d+))?)?$`)
)

// markWords writes a lower-case line's marks in the words markPart reads. An
// epic's prose is left as it is.
func markWords(text string) string {
	if strings.HasPrefix(text, "level 60 epic") {
		return text
	}
	token := func(word string) func(string, []string) string {
		return func(_ string, m []string) string {
			switch {
			case m[1] == "":
				return word
			case m[2] != "":
				return word + " " + m[1] + " to " + m[2]
			}
			return word + " " + m[1]
		}
	}
	text = replaceSubmatches(reMarkHit, text, token("mark"))
	return replaceSubmatches(reMarkHeal, text, token("markheal"))
}

// markPart reads one part of a line markWords wrote: a damage mark or a heal
// mark, its figure 0 when the line names none.
func markPart(part string) (SpellEffect, bool) {
	m := reMarkToken.FindStringSubmatch(part)
	if m == nil {
		return SpellEffect{}, false
	}
	e := SpellEffect{Kind: EffectMark, Amount: atoi(m[2])}
	if m[1] == "markheal" {
		e.Kind = EffectMarkHeal
	}
	e.AmountMax = max(atoi(m[3]), e.Amount)
	return e, true
}

// with is the mark with one of the cast's mark effects added, its figure
// worked out at the landing (markDefault for a line naming none): the
// stronger of the two.
func (mk *Mark) with(e SpellEffect, cs *castState) *Mark {
	lo, hi := e.Amount, max(e.AmountMax, e.Amount)
	if lo <= 0 {
		lo = markDefault(cs.spell.Name, cs.spell.Level)
		hi = max(hi, lo)
	}
	nm := &Mark{Min: lo, Max: hi, Heal: e.Kind == EffectMarkHeal}
	if mk != nil && mk.Max >= nm.Max {
		return mk
	}
	return nm
}

// markDefault is the figure of a mark whose line names none: half its
// spell's level, at least shieldLeast, a brand's markBrandFactor times that
// (ours).
func markDefault(name string, level int) int {
	n := max(level/shieldLevelDivisor, shieldLeast)
	if strings.HasPrefix(strings.ToLower(strings.TrimSpace(name)), "brand of ") {
		n *= markBrandFactor
	}
	return n
}

// roll is one answer's figure, within the mark's range.
func (mk *Mark) roll(intn func(int) int) int {
	if mk.Max > mk.Min {
		return mk.Min + intn(mk.Max-mk.Min+1)
	}
	return mk.Min
}

// strongestMark is the mark on a creature that answers a hit on it now: the
// one with the highest top, the first put on of equals; nil for none.
func strongestMark(effects []*ActiveEffect, now time.Time) *ActiveEffect {
	var best *ActiveEffect
	for _, ae := range effects {
		if ae.Mark != nil && ae.Mark.Max > 0 && now.Before(ae.Until) && (best == nil || ae.Mark.Max > best.Mark.Max) {
			best = ae
		}
	}
	return best
}

// markHealerLocked is whom a heal mark's 0x00DB comes from: the mark's caster
// while it is in the world, else the attacker.
func (w *World) markHealerLocked(ae *ActiveEffect, attacker *Player) (*Player, *session.Session) {
	h := w.Players[ae.Caster]
	if h == nil {
		h = attacker
	}
	return h, w.findSessionByEntityIDLocked(h.ServerID)
}

// playerMarkLocked answers a player's hit on a creature still standing with
// the creature's mark: its damage as the player's (spellHitLocked), or its
// heal for the player. It reports a kill.
func (w *World) playerMarkLocked(sess *session.Session, p *Player, npc *NPC, now time.Time) bool {
	ae := strongestMark(npc.Effects, now)
	if ae == nil || p.IsDead() || npc.Dead {
		return false
	}
	n := ae.Mark.roll(procIntn)
	if ae.Mark.Heal {
		healer, hs := w.markHealerLocked(ae, p)
		log.Printf("Mark: %s's hit on %s under %q heals it %d", p.Name, npc.Name, ae.Name, n)
		w.healLocked(healer, hs, p, n)
		return false
	}
	log.Printf("Mark: %s's hit on %s under %q does %d more", p.Name, npc.Name, ae.Name, n)
	return w.spellHitLocked(p, sess, npc, n)
}

// petMarkLocked answers a pet's hit on a creature still standing with the
// creature's mark: its damage as the pet's, its owner's (petDamageLocked), or
// its heal for the pet. It reports a kill.
func (w *World) petMarkLocked(ps *petState, t *NPC, now time.Time) bool {
	pet := ps.npc
	ae := strongestMark(t.Effects, now)
	if ae == nil || pet.Dead || t.Dead {
		return false
	}
	n := ae.Mark.roll(procIntn)
	if ae.Mark.Heal {
		log.Printf("Mark: the pet %q's hit on %s under %q heals it %d", pet.Name, t.Name, ae.Name, n)
		if pet.HP < pet.MaxHP {
			pet.HP = min(pet.HP+n, pet.MaxHP)
			healer, hs := w.markHealerLocked(ae, ps.owner)
			w.tellAmountLocked(healer, hs, pet.ServerID, n)
		}
		return false
	}
	log.Printf("Mark: the pet %q's hit on %s under %q does %d more", pet.Name, t.Name, ae.Name, n)
	return w.petDamageLocked(ps, t, n, now)
}
