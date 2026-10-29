package entity

import (
	"fmt"
	"log"
	"math"
	"regexp"
	"strings"
	"time"

	"github.com/eqoa/server/internal/session"
)

// lifetap.go — what a spell takes for its caster, and what it costs it.
//
// A life tap's damage heals its caster (F3 20261025-215541, jib: the
// necromancer's tap line "is supposed to heal you as well for the damage it
// deals"; -215810: "bloodleech and other lifetaps not draining hp"). The
// lines say so in several ways, all EQOA-Data's: "Decrease Drain HP by 156"
// (Spirit Tap), "Drain HP by 22 and transfer to caster" (Life Draw), "Drain
// HP by 500 give self; Increase 20% based on INT" (the shadow knight's
// Bloodleech), "Decrease HP by 76 and transfer to caster" (Lifespike); a
// stat or power drained is the caster's for as long as the target lacks it
// ("Drain STR by 10 and give to self, 60sec", the paladin's "Drain Power by
// 39 to self"); "Decrease Drain Group HP by 125" heals the caster's group.
// Until now the words after the hit were cut off and the tap was a plain
// nuke.
//
// A pact heals its target and costs its caster (F3 20261025-214751, jib:
// "damaging the necromancer, and healing the target. The damage is Arcane
// Resistance on the player and can be resisted with higher AR ... pacts
// should do more damage than the heal if unresisted ... the necro should be
// able to kill themselves with pacts if they overdo it"): "Target, Increase
// Hitpoints by 25, Decrease Hitpoints by 50 to Self" (Gloom Pact), "...
// by 90 Self Only", "Decrease 270 Hitpoints to Self", "Decrease Hitpoints
// 360 to Self". The price was unread, so a pact was a small heal. Every pact
// in the lists costs twice its heal and every covenant three times, but one:
// Infernal Pact's 1820 (tools/fix_taps_pacts.py makes it 1080).
//
// Here:
//   - an effect marked Self lands on the caster, once a cast, before the
//     targets (landOnCasterLocked): a price in hit points, less the share
//     the caster's arcane resist takes (resist.go, the one rule on this
//     server for a resist's share, as the sparring dummy's strikes take
//     theirs: 75% at the cap of 315, in proportion below it, and at the cap
//     a chance to fail wholly or in part); it can kill. A gain in hit points or power is the
//     caster's own; a stat changed lasts the line's duration as an effect of
//     the spell's name on the caster;
//   - a drained effect lands on its target as before, and then the caster
//     (or each of its group in range) gains what was taken: a tap's figure in
//     hit points, the power, or the stat for the line's duration
//     (drainLocked);
//   - a cast is harmful by what it does to its target: a price does not make
//     a pact one, so it is cast at a friend, oneself included, and on oneself
//     the price is paid first and the heal comes after.
//
// The resist's share below the cap is the server's own, not measured; so is healing the
// whole figure of a tap that kills (EverQuest's taps do).

// DrainTo is where a drained effect's take goes.
type DrainTo int

const (
	DrainNone   DrainTo = iota
	DrainCaster         // the caster: a life tap, a stat or power taken
	DrainGroup          // the caster's group in range: "Drain Group HP"
)

var (
	// the words that send the take to the caster or its group, longest
	// first where one ends another
	transferWords = []struct {
		words string
		to    DrainTo
	}{
		{"and transfer to caster", DrainCaster}, {"transfer to caster", DrainCaster},
		{"and gives to self", DrainCaster}, {"and give to self", DrainCaster},
		{"gives to self", DrainCaster}, {"give to self", DrainCaster},
		{"gives self", DrainCaster}, {"give self", DrainCaster},
		{"and gives to group", DrainGroup}, {"and give to group", DrainGroup},
		{"gives to group", DrainGroup}, {"give to group", DrainGroup},
	}
	// the words that put an effect on the caster instead of the target
	selfWords = []string{"self only", "to self", "to caster"}
	// "Increase Self AC by 500", "Increase Caster HP by 1160"
	reSelfSubject = regexp.MustCompile(`^(increase|decrease|buff|debuff|drain|raise|lower|heal)\s+(?:self|caster)\s+(.+)$`)
	// a price with no "by": "Decrease 270 Hitpoints", "Decrease Hitpoints 360"
	reNumBefore = regexp.MustCompile(`^(increase|decrease|drain)\s+(\d+)\s+(hitpoints|hit points|hp|health|power)$`)
	reNumAfter  = regexp.MustCompile(`^(increase|decrease|drain)\s+(hitpoints|hit points|hp|health|power)\s+(\d+)$`)
	// "Drain Group HP by 125", "Increase Group Power by 300"
	reGroupPool = regexp.MustCompile(`\bgroup\s+(hp|hitpoints|hit points|health|power)\b`)
)

// cutWords takes the first whole-word occurrence of words out of part,
// reporting whether there was one.
func cutWords(part, words string) (string, bool) {
	padded := " " + part + " "
	i := strings.Index(padded, " "+words+" ")
	if i < 0 {
		return part, false
	}
	return strings.Join(strings.Fields(padded[:i]+" "+padded[i+len(words)+2:]), " "), true
}

// splitTransfer takes a clause's words of transfer out of it: where what it
// takes goes, and whether it is the caster's own. What is left is read as
// any clause is; it is "" when the clause was those words alone ("Decrease
// STA by 30, give to self").
func splitTransfer(part string) (string, DrainTo, bool) {
	to, self := DrainNone, false
	for _, t := range transferWords {
		if rest, ok := cutWords(part, t.words); ok {
			part, to = rest, t.to
			break
		}
	}
	for _, w := range selfWords {
		if rest, ok := cutWords(part, w); ok {
			part, self = rest, true
		}
	}
	part = strings.TrimSpace(strings.TrimSuffix(part, " to target")) // Defile's other half
	if m := reSelfSubject.FindStringSubmatch(part); m != nil {
		part, self = m[1]+" "+m[2], true
	}
	if strings.HasSuffix(part, " self") {
		part, self = strings.TrimSuffix(part, " self"), true
	} else if part == "self" {
		part, self = "", true
	}
	if m := reGroupPool.FindStringSubmatch(part); m != nil {
		part = strings.Replace(part, m[0], m[1], 1)
		if to == DrainNone {
			to = DrainGroup
		}
	}
	// a number before or after its "hitpoints", with no "by"
	if m := reNumBefore.FindStringSubmatch(part); m != nil {
		part = m[1] + " " + m[3] + " by " + m[2]
	} else if m := reNumAfter.FindStringSubmatch(part); m != nil {
		part = m[1] + " " + m[2] + " by " + m[3]
	}
	return strings.TrimSpace(part), to, self
}

// resolveTransfer is what a clause's words of transfer make of one of its
// effects, by its verb and sign: a drain gives what it takes to the caster
// (or the group) whatever else it says ("Drain Power by 39 to self" is the
// paladin's power taken, not paid); a decrease that gives to the caster is a
// drain too ("Decrease STA by 15 and gives to self"); an increase that gives
// to self is the caster's own ("Increase AC by 85 give Self"), and one of
// the group's pool a plain heal where the spell lands ("Group, Increase
// Group HP by 500"); otherwise "to Self" puts it on the caster.
func resolveTransfer(verb string, sign int, to DrainTo, self bool) (DrainTo, bool) {
	if verb == "drain" {
		if to == DrainNone {
			to = DrainCaster
		}
		return to, false
	}
	if to != DrainNone {
		switch {
		case sign < 0:
			return to, false
		case to == DrainCaster:
			return DrainNone, true
		}
		return DrainNone, false
	}
	return DrainNone, self
}

// transferFrom gives the effects from index i on a clause's words of
// transfer that were read before them (a clause cut in two, or a clause of
// those words alone, which are the effect before it's).
func (d *SpellDef) transferFrom(i int, verb string, to DrainTo, self bool) {
	if to == DrainNone && !self {
		return
	}
	for j := max(i, 0); j < len(d.Effects); j++ {
		e := &d.Effects[j]
		if e.Drain != DrainNone || e.Self {
			continue
		}
		sign := 1
		if e.Amount < 0 {
			sign = -1
		}
		e.Drain, e.Self = resolveTransfer(verb, sign, to, self)
	}
}

// landOnCasterLocked lands what the cast does to its caster (its effects
// marked Self), once, before the targets, and returns the cast as its
// targets have it: those effects left out. A price the caster cannot pay
// kills it.
func (w *World) landOnCasterLocked(cs *castState, caster *Player, sess *session.Session, now time.Time) *castState {
	d := &cs.spell.Def
	var own []SpellEffect
	for _, e := range d.Effects {
		if e.Self {
			own = append(own, e)
		}
	}
	if len(own) == 0 {
		return cs
	}
	rest := make([]SpellEffect, 0, len(d.Effects)-len(own))
	for _, e := range d.Effects {
		if !e.Self {
			rest = append(rest, e)
		}
	}
	out := *cs
	out.spell.Def.Effects = rest

	var mods StatMods
	stats := false
	for _, e := range own {
		n := spellAmount(e, caster)
		switch e.Kind {
		case EffectHP:
			if n < 0 {
				w.payPriceLocked(cs, caster, sess, -n, now)
			} else {
				w.healLocked(caster, sess, caster, n)
			}
		case EffectPower:
			caster.Power = min(max(caster.Power+n, 0), caster.MaxPowerNow())
			caster.StatsDirty = true
		case EffectStat:
			mods.add(e.Stat, n)
			stats = true
		}
	}
	if stats && d.Duration > 0 && !caster.IsDead() {
		w.casterShareLocked(cs, caster, mods, now)
	}
	return &out
}

// payPriceLocked takes a price of n hit points off the caster, less its
// arcane resist's share: the figure on the caster, a line saying it, and a
// death when nothing is left.
func (w *World) payPriceLocked(cs *castState, caster *Player, sess *session.Session, n int, now time.Time) {
	share, how := RollResist(caster.StatNow(StatAR)) // resist.go
	taken := int(math.Round(float64(n) * (1 - share)))
	log.Printf("Cast: %s's %q costs %s %d of %d hit points (resist %.0f%%; HP %d)",
		caster.Name, cs.spell.Name, caster.Name, taken, n, share*100, caster.HP)
	if sess != nil {
		w.sendChatToSession(sess, fmt.Sprintf("%s costs you %d hit points (%s).", cs.spell.Name, taken, resistWords(share, how)))
	}
	if taken <= 0 {
		return
	}
	caster.HP = max(caster.HP-taken, 0)
	caster.StatsDirty = true
	w.tellAmountLocked(caster, sess, caster.ServerID, -taken)
	if caster.HP <= 0 {
		w.playerDiedLocked(sess, caster, now)
	}
}

// selfCostLocked takes a self line's own price of n hit points off its
// caster, in full: Cannibalize's, the ranger's blades', the monk's strikes',
// the alchemist's infusions' "Decrease Hitpoints by N" at the landing, and
// Lich's or Cursed Flesh's "Decrease HP per tic by N" each tick (tickEffectLocked).
// Until the test lanes' report of 2026-10-29 these were dropped; the user's
// ruling: "You could definitely kill yourself with lich form, pacts and so
// on. That was a drawback people had to actively manage with remove
// illusion." The figure on the bearer, a line saying it when say, and a
// death when nothing is left, which it reports. A pact's price (a clause "to
// Self") is payPriceLocked's, resisted as before.
func (w *World) selfCostLocked(name string, p *Player, sess *session.Session, n int, say bool, now time.Time) bool {
	log.Printf("Cast: %q costs %s %d hit points (HP %d)", name, p.Name, n, p.HP)
	if say && sess != nil {
		w.sendChatToSession(sess, fmt.Sprintf("%s costs you %d hit points.", name, n))
	}
	p.HP = max(p.HP-n, 0)
	p.StatsDirty = true
	w.tellAmountLocked(p, sess, p.ServerID, -n)
	if p.HP > 0 {
		return false
	}
	w.playerDiedLocked(sess, p, now)
	return true
}

// casterShareLocked puts a cast's stat changes on its caster for the line's
// duration: an effect of the spell's name (its id negated, so it is not the
// one the spell leaves on a target, oneself included). A share landed this
// same moment (a second stat of one line) is added to it.
func (w *World) casterShareLocked(cs *castState, caster *Player, mods StatMods, now time.Time) {
	until := now.Add(cs.spell.Def.Duration)
	id := -cs.spell.SpellID
	for _, e := range caster.Effects {
		if e.SpellID == id && e.Until.Equal(until) {
			speed := caster.SpeedNow()
			e.Mods.sum(mods)
			w.effectsChangedLocked(caster, speed)
			return
		}
	}
	ae := &ActiveEffect{SpellID: id, Name: cs.spell.Name, Icon: cs.spell.Icon,
		Caster: caster.ServerID, Until: until, Mods: mods, Line: BuffLine(&cs.spell), Rank: cs.spell.Level}
	if !w.shareTakesLocked(caster, ae) { // only the best of a line (buffline.go)
		return
	}
	w.dropLineLocked(caster, ae)
	w.addEffectLocked(caster, ae, now)
}

// drainLocked gives the caster (or its group) what one drained effect took
// from its target: n hit points (a tap's figure, whole, though it killed),
// n power, or n of the stat for the line's duration.
func (w *World) drainLocked(cs *castState, caster *Player, sess *session.Session, e SpellEffect, n int, now time.Time) {
	if e.Drain == DrainNone || n <= 0 || caster.IsDead() {
		return
	}
	switch e.Kind {
	case EffectHP:
		for _, p := range w.drainersLocked(cs, caster, e.Drain) {
			w.healLocked(caster, sess, p, n)
		}
		log.Printf("Cast: %s's %q takes %d hit points for %s", caster.Name, cs.spell.Name, n, drainName(e.Drain))
	case EffectPower:
		for _, p := range w.drainersLocked(cs, caster, e.Drain) {
			p.Power = min(p.Power+n, p.MaxPowerNow())
			p.StatsDirty = true
		}
	case EffectStat:
		if cs.spell.Def.Duration > 0 {
			var m StatMods
			m.add(e.Stat, n)
			w.casterShareLocked(cs, caster, m, now)
		}
	}
}

// drainersLocked is whom a take goes to: the caster, or the caster and each
// of its group alive in its world within the spell's range of it.
func (w *World) drainersLocked(cs *castState, caster *Player, to DrainTo) []*Player {
	if to != DrainGroup {
		return []*Player{caster}
	}
	out := []*Player{caster}
	for _, m := range w.groupMembersLocked(caster) {
		if m != caster && !m.IsDead() && m.World == caster.World && !outOfRange(caster, m.X, m.Y, m.Z, cs.spell.Range) {
			out = append(out, m)
		}
	}
	return out
}

func drainName(to DrainTo) string {
	if to == DrainGroup {
		return "the caster's group"
	}
	return "the caster"
}
