package entity

import (
	"fmt"
	"log"
	"regexp"
	"strings"
	"time"

	"github.com/eqoa/server/internal/session"
)

// workbook.go — the mechanics EQOAEmu's spell workbook uses that the lines
// never did (spelljson.go reads them into a SpellDef; the user's "workbook
// wins", 2026-11-01). Each rule here acts on a spell read from the workbook's
// effects alone, so a spell read from its line is as it was.
//
//   - A resist: a harmful effect with a resist basis may be resisted by the
//     creature it lands on, wholly (effects of one workbook EffectID
//     together). No creature has resists of its own, so the chance is the one
//     the server's creatures already resist a proc by (proc.go): 5% against
//     a caster of its level, 3 more a level it is above the caster, at most
//     75%, less what resist debuffs of that kind on it take off (enchanter.go's
//     rule, by kind); a dummy never resists (dummy.go's measured hits), a
//     charm keeps its own roll (charm.go) and a hold or a slow the con's
//     (root.go). On a player (a templated creature's spell, classtemplate.go)
//     the player's resist of that kind takes its share (resist.go). Ours: the
//     workbook says only "Can resist up to a cap %".
//   - Ticks twice at the landing: the workbook's legend, "HoT and PoT tick
//     twice on first effect and every 6seconds after" (DoT the same); the
//     server's first tick was once at the landing (hot.go), so a workbook
//     spell's first tick lands twice, then they go on as the server's do (a
//     player's heal on the regen tick, a creature's damage every 6 s).
//   - Dispel on cast: what lasts ends when its bearer begins a cast. Dispel
//     on contact: when its bearer is hit in melee or a harmful spell lands on
//     it (a player by a creature's hit; a creature by any hit or hostile act,
//     updateAggro), the parts the workbook marks alone (the user,
//     2026-11-01: "yes only end the marked part"): the rest of the effect
//     stays and runs to its time (contactSplit), one whose every lasting
//     part is marked ending whole.
//   - Mitigation (and the invulnerables): that share of each melee hit on a
//     player is not taken, up to its most over the effect's time; the
//     workbook says it "has no effect on spells".
//   - Lull: a lulled creature notices no one (aggro.go) and comes to no one's
//     help (socialaggro.go) while it lasts; its landing angers nothing.
//   - Enduring breath: a player under water loses no breath while it lasts.
//   - Hide: unseen as invisibility is (invis.go), and ends as it says.
//   - A cure takes harmful effects off its target by the words it removes
//     (cureMatches); the workbook's own "ID?" (it does not know which) and no
//     words take every harmful effect, as the potions' cure does.
//   - The lowest level that may receive it: a player below it is not
//     affected, the caster told (ours: "<name> is too low a level for that
//     spell.").
//   - A snare's slow by its percent where a line's halves the pace, and on a
//     player a slow of its own beside the buff that carries it (a self buff's
//     AC with a slow).
//   - Proc heals: a proc that heals heals its bearer, an offensive one at the
//     proc chance of its hits (proc.go), a defensive one at every hit on it.

// EffectRules are what a workbook effect does beside its numbers, kept with
// it over a logout (buffsave.go).
type EffectRules struct {
	// SpeedPct is the bearer's run speed this many percent slower when
	// negative (a workbook snare or a buff's own slow)
	SpeedPct        int  `json:",omitempty"`
	DispelOnCast    bool `json:",omitempty"`
	DispelOnContact bool `json:",omitempty"`
	// MitigatePct of each melee hit on the bearer is not taken, at most
	// MitigateLeft more (0: no limit; MitigateCapped says there is one)
	MitigatePct    int  `json:",omitempty"`
	MitigateLeft   int  `json:",omitempty"`
	MitigateCapped bool `json:",omitempty"`
	Lull           bool `json:",omitempty"`
	Breath         bool `json:",omitempty"`
	Hide           bool `json:",omitempty"`
	// Resist is the resist its harm goes by and Harm whether it was a
	// harmful spell's (a cure's words go by them)
	Resist Stat `json:",omitempty"`
	Harm   bool `json:",omitempty"`
	// Twice: its first tick lands twice (the workbook's)
	Twice bool `json:",omitempty"`
	// a proc that heals its bearer (ProcHealDef: when hit), with its scale
	// on the bearer's stat at each proc and its chance (0: the proc's)
	ProcHeal         int  `json:",omitempty"`
	ProcHealDef      bool `json:",omitempty"`
	ProcHealScale    Stat `json:",omitempty"`
	ProcHealScalePct int  `json:",omitempty"`
	ProcHealChance   int  `json:",omitempty"`
	// ProcHealUndead: the proc heals only at a hit on, or by, an undead
	// creature (the workbook's "Enemy - Undead" proc; none heals so yet)
	ProcHealUndead bool `json:",omitempty"`
	// Disguise: the factions do not attack the bearer on sight
	// (workbookmore.go)
	Disguise bool `json:",omitempty"`
}

// rulesOf is a cast's rules for what lasts of it.
func rulesOf(d *SpellDef) EffectRules {
	r := EffectRules{DispelOnCast: d.DispelOnCast, DispelOnContact: d.DispelOnContact, Harm: d.Harmful, Twice: d.Workbook}
	for _, e := range d.Effects {
		if e.Resist != StatNone {
			r.Resist = e.Resist
			break
		}
	}
	return r
}

// workbookLasting puts one of the workbook's own kinds on what lasts, and
// reports whether it was one; anything else is lasting's.
func (ae *ActiveEffect) workbookLasting(e SpellEffect, d *SpellDef) bool {
	switch e.Kind {
	case EffectSnare:
		if e.Amount >= 0 || !d.Workbook {
			return false // a line's snare: lasting's, half the pace
		}
		ae.Rules.SpeedPct = min(ae.Rules.SpeedPct, e.Amount)
		ae.Snare = ae.Snare || d.Harmful
	case EffectMitigate:
		ae.Rules.MitigatePct = max(ae.Rules.MitigatePct, e.Amount)
		ae.Rules.MitigateLeft, ae.Rules.MitigateCapped = e.AmountMax, e.AmountMax > 0
	case EffectLull:
		ae.Rules.Lull = true
	case EffectBreath:
		ae.Rules.Breath = true
	case EffectHide:
		ae.Invis, ae.Rules.Hide = true, true
	case EffectDisguise:
		ae.Rules.Disguise = true
	case EffectProcHeal:
		ae.Rules.ProcHeal, ae.Rules.ProcHealDef = e.Amount, e.Defense
		ae.Rules.ProcHealScale, ae.Rules.ProcHealScalePct, ae.Rules.ProcHealChance = e.Scale, e.ScalePct, e.Chance
		ae.Rules.ProcHealUndead = e.Undead
	default:
		return false
	}
	return true
}

// snareFactor is what the creature's slows leave of its pace: a line's
// snare half, a workbook snare its percent less (the slowest one), 1 for
// none.
func (n *NPC) snareFactor(now time.Time) float32 {
	f := float32(1)
	for _, e := range n.Effects {
		if !now.Before(e.Until) {
			continue
		}
		switch {
		case e.Rules.SpeedPct < 0:
			f = min(f, float32(100+max(e.Rules.SpeedPct, -100))/100)
		case e.Snare:
			f = min(f, 0.5)
		}
	}
	return f
}

// slowFactor is what a player's own slows leave of its run speed (a buff's
// SpeedPct), 1 for none.
func (p *Player) slowFactor() float32 {
	f := float32(1)
	for _, e := range p.Effects {
		if e.duelFoe == 0 && e.Rules.SpeedPct < 0 {
			f = min(f, float32(100+max(e.Rules.SpeedPct, -100))/100)
		}
	}
	return f
}

// lulledAt reports a lull on the creature.
func (n *NPC) lulledAt(now time.Time) bool {
	for _, e := range n.Effects {
		if e.Rules.Lull && now.Before(e.Until) {
			return true
		}
	}
	return false
}

// holdsBreath reports an enduring breath on the player.
func (p *Player) holdsBreath() bool {
	for _, e := range p.Effects {
		if e.Rules.Breath {
			return true
		}
	}
	return false
}

// lulls reports whether the spell lulls.
func (d *SpellDef) lulls() bool {
	for _, e := range d.Effects {
		if e.Kind == EffectLull {
			return true
		}
	}
	return false
}

// resistOutcomeWords is the caster's line for a spell its target resisted
// whole (EverQuest's words: no recording of EQOA's exists).
const spellResisted = "Your target resisted the %s spell."

// resistible reports whether an effect is one a creature's resist turns:
// harm with a resist basis, not a charm's (its own roll) nor a hold's or a
// slow's (the con's, root.go).
func resistible(e SpellEffect) bool {
	if e.Resist == StatNone || e.Self {
		return false
	}
	switch e.Kind {
	case EffectCharm, EffectRoot, EffectSnare:
		return false
	case EffectHP, EffectHPTick, EffectPower, EffectPowerTick, EffectStat:
		return e.Amount < 0
	case EffectHate, EffectLull, EffectForget:
		return true
	}
	return false // what is not harm (a buff's part) is never resisted
}

// npcResistKindPct is how many points the resist debuffs of a kind on a
// creature take off its chance to resist, in percent: npcResistOffPct's rule
// for arcane, for each kind (the strongest one's).
func npcResistKindPct(npc *NPC, st Stat, now time.Time) int {
	if st == StatAR {
		return npcResistOffPct(npc, now)
	}
	off := 0
	for _, e := range npc.Effects {
		if !now.Before(e.Until) {
			continue
		}
		var v int
		switch st {
		case StatFR:
			v = e.Mods.FR
		case StatCR:
			v = e.Mods.CR
		case StatLR:
			v = e.Mods.LR
		case StatPR:
			v = e.Mods.PR
		case StatDR:
			v = e.Mods.DR
		case StatPierce, StatRanged: // melee resists (workbookmore.go)
			v = e.Mods.Pierce
		case StatBlunt:
			v = e.Mods.Blunt
		case StatSlash:
			v = e.Mods.Slash
		}
		off = max(off, -v)
	}
	if off == 0 {
		return 0
	}
	return int(ResistShare(off)*100 + 0.5)
}

// npcSpellResistPct is the chance, in percent, a creature resists a workbook
// effect of that resist from a caster of that level (the file's doc).
func npcSpellResistPct(casterLevel int, npc *NPC, st Stat, now time.Time) int {
	return max(procResistPct(casterLevel, npc.Level)-npcResistKindPct(npc, st, now), 0)
}

// resistLocked rolls a workbook spell's resists as it lands on a creature:
// the cast with what the creature resisted taken out, and false when it
// resisted all of the spell's harm, of which the caster is told. A spell
// with nothing to resist, or a line's, is the cast as it is.
func (w *World) resistLocked(cs *castState, caster *Player, casterSess *session.Session, npc *NPC, now time.Time) (*castState, bool) {
	d := &cs.spell.Def
	if !d.Workbook || npc.Dummy != NotDummy {
		return cs, true
	}
	rolled := map[string]bool{} // an EffectID's roll: resisted or not
	harm, kept := 0, 0
	var effects []SpellEffect
	for _, e := range d.Effects {
		if !resistible(e) {
			effects = append(effects, e)
			continue
		}
		harm++
		key := e.ID
		if key == "" {
			key = fmt.Sprintf("#%d", len(rolled))
		}
		resisted, ok := rolled[key]
		if !ok {
			resisted = procIntn(100) < npcSpellResistPct(caster.Level, npc, e.Resist, now)
			rolled[key] = resisted
		}
		if !resisted {
			effects = append(effects, e)
			kept++
		}
	}
	if harm == 0 || kept == harm {
		return cs, true
	}
	log.Printf("Cast: %s resists %d of %s's %q's %d harmful effects", npc.Name, harm-kept, caster.Name, cs.spell.Name, harm)
	if kept == 0 && casterSess != nil {
		w.sendColoredLocked(casterSess, fmt.Sprintf(spellResisted, cs.spell.Name), castRefusalRGB)
	}
	if len(effects) == 0 {
		return cs, false
	}
	c := *cs
	c.spell.Def.Effects = effects
	return &c, true
}

// workbookResist is the resist a templated creature's workbook spell's hit
// on a player goes by (classtemplate.go): its first hit's, a player's resist
// (FR to AR), else none.
func workbookResist(d *SpellDef) (Stat, bool) {
	if !d.Workbook {
		return StatNone, false
	}
	for _, e := range d.Effects {
		if e.Kind == EffectHP && e.Amount < 0 && e.Resist >= StatFR && e.Resist <= StatAR {
			return e.Resist, true
		}
	}
	return StatNone, true
}

// dispelOnCastLocked ends what lasts on a player that ends when it casts.
func (w *World) dispelOnCastLocked(p *Player) {
	for _, e := range append([]*ActiveEffect(nil), p.Effects...) {
		if e.Rules.DispelOnCast {
			log.Printf("Cast: %q ends on %s: a cast", e.Name, p.Name)
			w.removeEffectLocked(p, e.SpellID)
		}
	}
}

// contactPlayerLocked ends what lasts on a player that ends when it is hit
// or harmed: an effect whose parts the workbook marks only some of loses
// those (endContactParts), its new speed told and its stats summed again as
// a whole effect's end does them (effectsChangedLocked; its body and its
// invisibility are read from its parts as they are now); any other, whole.
func (w *World) contactPlayerLocked(p *Player) {
	for _, e := range append([]*ActiveEffect(nil), p.Effects...) {
		if !e.Rules.DispelOnContact {
			continue
		}
		if e.contact != nil {
			speed := p.SpeedNow()
			e.endContactParts()
			log.Printf("Cast: %q's parts that end at a hit end on %s, the rest stays", e.Name, p.Name)
			w.effectsChangedLocked(p, speed)
			continue
		}
		log.Printf("Cast: %q ends on %s: a hit", e.Name, p.Name)
		w.removeEffectLocked(p, e.SpellID)
	}
}

// contactSplit is what the parts of an effect a hit ends put on (Marked)
// beside what its other parts do (Rest), each as lasting builds it from
// those parts alone; Form: its illusion is among the marked parts alone.
type contactSplit struct {
	Marked, Rest *ActiveEffect
	Form         bool `json:",omitempty"`
}

// contactSplitOf is the split of a spell whose workbook marks only some of
// its lasting parts dispel_on_contact (SpellEffect.EndsOnContact), nil for
// one a hit ends whole: every lasting part marked (Arcane Shock's snare
// beside its instant hit), none marked, or one that lasts until a hit, its
// time the hit's (untilContact).
func contactSplitOf(cs *castState, caster *Player, now time.Time) *contactSplit {
	d := &cs.spell.Def
	if !d.DispelOnContact || d.Duration == untilContact {
		return nil
	}
	var marked, rest []SpellEffect
	markedForm, restForm := false, false
	for _, e := range d.Effects {
		if e.EndsOnContact {
			marked = append(marked, e)
			markedForm = markedForm || e.Kind == EffectIllusion
		} else {
			rest = append(rest, e)
			restForm = restForm || e.Kind == EffectIllusion
		}
	}
	if len(marked) == 0 || len(rest) == 0 {
		return nil
	}
	of := func(effects []SpellEffect) *ActiveEffect {
		c := *cs
		c.spell.Def.Effects = effects
		return lastingParts(&c, caster, now)
	}
	r := of(rest)
	if r == nil || !(r.lastsAnything() || restForm || hasSetStat(rest)) {
		return nil // only its marked parts last: a hit ends it whole
	}
	m := of(marked)
	if m == nil {
		return nil
	}
	return &contactSplit{Marked: m, Rest: r, Form: markedForm && !restForm}
}

// hasSetStat reports a stat set among the parts (setStatsLocked's).
func hasSetStat(effects []SpellEffect) bool {
	for _, e := range effects {
		if e.Kind == EffectStat && e.Set {
			return true
		}
	}
	return false
}

// endContactParts takes off what the effect's parts a hit ends put on
// (contact.Marked), what its other parts put on staying as it is now
// (contact.Rest: a heal's ticks left, a mitigation's spent share): their
// stats and ticks taken back, a hold, a slow, a speed or an invisibility
// gone unless another part gives it too, an illusion's body gone with its
// part. What stays ends at no hit more and runs to its time.
func (ae *ActiveEffect) endContactParts() {
	m, r := ae.contact.Marked, ae.contact.Rest
	ae.HPTick -= m.HPTick
	ae.PowerTick -= m.PowerTick
	if m.TickDrain != DrainNone && r.TickDrain == DrainNone {
		ae.TickDrain, ae.TickRange = DrainNone, 0
	}
	ae.Mods.take(m.Mods)
	ae.Root = ae.Root && (!m.Root || r.Root)
	ae.Snare = ae.Snare && (!m.Snare || r.Snare)
	ae.Speed = ae.Speed && (!m.Speed || r.Speed)
	ae.Invis = ae.Invis && (!m.Invis || r.Invis)
	if m.Proc != 0 || m.ProcMax != 0 {
		ae.Proc, ae.ProcMax, ae.ProcScale, ae.ProcScalePct, ae.ProcCap, ae.ProcUndead =
			r.Proc, r.ProcMax, r.ProcScale, r.ProcScalePct, r.ProcCap, r.ProcUndead
	}
	if m.Shield != nil {
		ae.Shield = r.Shield
	}
	if m.PowerCapped {
		ae.PowerCap, ae.PowerCapped = r.PowerCap, r.PowerCapped
	}
	mr, rr := m.Rules, r.Rules
	if mr.SpeedPct != 0 {
		ae.Rules.SpeedPct = rr.SpeedPct
	}
	if mr.MitigatePct != 0 {
		ae.Rules.MitigatePct, ae.Rules.MitigateLeft, ae.Rules.MitigateCapped = rr.MitigatePct, rr.MitigateLeft, rr.MitigateCapped
	}
	if mr.ProcHeal != 0 {
		ae.Rules.ProcHeal, ae.Rules.ProcHealDef, ae.Rules.ProcHealScale = rr.ProcHeal, rr.ProcHealDef, rr.ProcHealScale
		ae.Rules.ProcHealScalePct, ae.Rules.ProcHealChance, ae.Rules.ProcHealUndead = rr.ProcHealScalePct, rr.ProcHealChance, rr.ProcHealUndead
	}
	ae.Rules.Lull = ae.Rules.Lull && (!mr.Lull || rr.Lull)
	ae.Rules.Breath = ae.Rules.Breath && (!mr.Breath || rr.Breath)
	ae.Rules.Hide = ae.Rules.Hide && (!mr.Hide || rr.Hide)
	ae.Rules.Disguise = ae.Rules.Disguise && (!mr.Disguise || rr.Disguise)
	if ae.contact.Form {
		ae.Form, ae.FormSize = 0, 0
	}
	ae.Rules.DispelOnContact = false
	ae.contact = nil
}

// untilContact is how long a harmful effect stays that the workbook gives no
// time but ends at contact (Enshrouding darkness: the user's "until hit"):
// a day, so a hit ends it, or its bearer's death, respawn or a restart.
const untilContact = 24 * time.Hour

// contactNPCLocked ends what lasts on a creature that ends when it is hit
// or harmed (a lull, a snare): the marked parts alone of an effect whose
// workbook marks only some (endContactParts: a creature's slows, holds,
// body and invisibility are read from its parts as they are now), any other
// whole.
func contactNPCLocked(npc *NPC) {
	if len(npc.Effects) == 0 {
		return
	}
	left := npc.Effects[:0]
	for _, e := range npc.Effects {
		if e.Rules.DispelOnContact && !e.ticking { // its own tick is no contact for itself
			if e.contact != nil {
				e.endContactParts()
				log.Printf("Cast: %q's parts that end at a hit end on %s, the rest stays", e.Name, npc.Name)
				left = append(left, e)
				continue
			}
			log.Printf("Cast: %q ends on %s: a hit", e.Name, npc.Name)
			continue
		}
		left = append(left, e)
	}
	for i := len(left); i < len(npc.Effects); i++ {
		npc.Effects[i] = nil
	}
	npc.Effects = left
}

// mitigateLocked is a creature's melee hit of n on a player less what its
// mitigations take off (the strongest one with any left), their most spent
// as they go.
func mitigateLocked(p *Player, n int, now time.Time) int {
	var best *ActiveEffect
	for _, e := range p.Effects {
		if e.Rules.MitigatePct <= 0 || !now.Before(e.Until) || e.Rules.MitigateCapped && e.Rules.MitigateLeft <= 0 {
			continue
		}
		if best == nil || e.Rules.MitigatePct > best.Rules.MitigatePct {
			best = e
		}
	}
	if best == nil || n <= 0 {
		return n
	}
	off := n * best.Rules.MitigatePct / 100
	if best.Rules.MitigateCapped {
		off = min(off, best.Rules.MitigateLeft)
		best.Rules.MitigateLeft -= off
	}
	if off > 0 {
		log.Printf("Combat: %s's %q takes %d off a hit of %d", p.Name, best.Name, off, n)
	}
	return n - off
}

// tooLowLocked reports a player below the spell's lowest level to receive it,
// telling its caster. Must be called with the lock held.
func (w *World) tooLowLocked(caster *Player, casterSess *session.Session, t *Player, s *CastSpell) bool {
	d := &s.Def
	if d.MinLevel <= 0 || t.Level >= d.MinLevel || t == caster {
		return false
	}
	log.Printf("Cast: %s's %q does not take on %s: level %d, its lowest %d", caster.Name, s.Name, t.Name, t.Level, d.MinLevel)
	if casterSess != nil {
		w.sendColoredLocked(casterSess, fmt.Sprintf(tooLowWords, t.Name), castRefusalRGB)
	}
	return true
}

// tooLowWords tells a caster its target is below a spell's lowest level
// (ours: no recording has it).
const tooLowWords = "%s is too low a level for that spell."

// cureLocked takes off a player the effects a cure's words remove
// (cureMatches), its own spell's left on. Must be called with the lock held.
func (w *World) cureLocked(cs *castState, t *Player) int {
	n := 0
	for _, ae := range append([]*ActiveEffect(nil), t.Effects...) {
		if ae.SpellID == cs.spell.SpellID {
			continue
		}
		for _, words := range cs.spell.Def.Removes {
			if cureMatches(ae, words) {
				log.Printf("Cast: %s's %q cures %q off %s", w.entityNameLocked(cs.caster), cs.spell.Name, ae.Name, t.Name)
				w.removeEffectLocked(t, ae.SpellID)
				n++
				break
			}
		}
	}
	return n
}

// harmfulEffect reports an effect that harms its bearer: a harmful spell's,
// a damage tick, a hold or a slow, a stat taken.
func harmfulEffect(ae *ActiveEffect) bool {
	return ae.Rules.Harm || ae.duelFoe != 0 || ae.HPTick < 0 || ae.PowerTick < 0 || ae.Root || ae.Snare || ae.Mods.harmful()
}

// cureMatches reports whether a cure's words take the effect off: poison
// (PR, or a poison's or venom's name), disease (DR, a disease's or a
// plague's), curse (AR), slow or snare (a slow), speed (a speed buff, the
// workbook's "removing lesser speed boosts"), mask (an illusion: the rogues'
// masks), DoT (a harmful effect whose tick hurts), else an effect named by
// them; no words or the workbook's "ID?" any harmful one.
func cureMatches(ae *ActiveEffect, words string) bool {
	w := strings.ToLower(strings.TrimSpace(words))
	name := strings.ToLower(ae.Name)
	has := func(s ...string) bool {
		for _, x := range s {
			if strings.Contains(w, x) {
				return true
			}
		}
		return false
	}
	named := func(s ...string) bool {
		for _, x := range s {
			if strings.Contains(name, x) {
				return true
			}
		}
		return false
	}
	switch {
	case w == "" || w == "id?" || w == "all" || has("harmful", "everything"):
		return harmfulEffect(ae)
	case has("poison", "venom"):
		return harmfulEffect(ae) && (ae.Rules.Resist == StatPR || named("poison", "venom"))
	case has("disease", "plague") && !strings.Contains(name, w):
		return harmfulEffect(ae) && (ae.Rules.Resist == StatDR || named("disease", "plague"))
	case has("curse"):
		return harmfulEffect(ae) && (ae.Rules.Resist == StatAR || named("curse"))
	case has("slow", "snare"):
		return ae.Snare || ae.Rules.SpeedPct < 0
	case has("speed"):
		return ae.Speed
	case has("mask", "illusion"):
		return ae.Form != 0
	case reDoTWord.MatchString(w) || has("damage over time"):
		// a damage over time (Envaric Salve's "PoE DoT?": the user,
		// 2026-11-01, "damage over time"): a harmful effect whose tick hurts
		return harmfulEffect(ae) && ae.HPTick < 0
	}
	return w != "" && strings.Contains(name, w)
}

// reDoTWord is a cure's "DoT" as a word of its own ("antidote" holds one).
var reDoTWord = regexp.MustCompile(`\bdots?\b`)

// procHealsLocked heals a player by its proc heals: its offensive ones
// (defense false) after one of its melee hits at the proc chance, its
// defensive ones (true) after a hit on it at theirs; npc is the creature
// hit or hitting (one for the undead alone heals only by an undead one).
// Must be called with the lock held.
func (w *World) procHealsLocked(p *Player, npc *NPC, defense bool, now time.Time) {
	for _, ae := range p.Effects {
		r := ae.Rules
		if r.ProcHeal <= 0 || r.ProcHealDef != defense || !now.Before(ae.Until) {
			continue
		}
		if r.ProcHealUndead && (npc == nil || !w.undeadLocked(npc)) {
			continue
		}
		chance := r.ProcHealChance
		if chance <= 0 {
			chance = procChancePct(p.StatNow(StatInt))
		}
		if procIntn(100) >= chance {
			continue
		}
		n := r.ProcHeal
		if r.ProcHealScale != StatNone && r.ProcHealScalePct != 0 {
			n += p.StatNow(r.ProcHealScale) * r.ProcHealScalePct / 100
		}
		log.Printf("Proc: %s's %q heals it for %d", p.Name, ae.Name, n)
		w.healLocked(p, w.findSessionByEntityIDLocked(p.ServerID), p, n)
	}
}
