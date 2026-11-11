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
//     updateAggro). Either, the parts the workbook marks alone (the user,
//     2026-11-01: "yes only end the marked part"): the rest of the effect
//     stays and runs to its time (parts.go), one whose every lasting part is
//     marked ending whole. A run speed on a player ends at neither as such:
//     only at a harmful cast (petition 1067), and at its bearer's own hit in
//     place of a hit taken (petition 1109).
//   - Mitigation (and the invulnerables): that share of each melee hit on a
//     player is not taken, up to its most over the effect's time; the
//     workbook says it "has no effect on spells".
//   - Lull: a lulled creature notices no one (aggro.go) and comes to no one's
//     help (socialaggro.go) while it lasts; its landing angers nothing.
//   - Enduring breath: a player under water loses no breath while it lasts.
//   - Hide: unseen as invisibility is (invis.go), and ends as it says; its
//     slow ends with the hiding at its bearer's cast or attack too (parts.go).
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
	// Stance marks a warrior's stance (SpellDef.Stance): another stance's
	// cure takes it off
	Stance bool `json:",omitempty"`
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
	// NoCure: no cure takes it off, a spell's or a potion's (a template's
	// debuff: classtemplate.go)
	NoCure bool `json:",omitempty"`
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
	// ProcChance is its proc's chance a hit in percent, 0 by its bearer's INT
	// (proc.go); ProcRound the most its proc does in one of its bearer's
	// rounds, 0 no limit (procsOfRound). A form's (weaponproc.go).
	ProcChance int `json:",omitempty"`
	ProcRound  int `json:",omitempty"`
	// MarkHeal: a mark on a creature, each weapon hit landed on it healing its
	// hitter this much (markheal.go: Mark of Faith)
	MarkHeal int `json:",omitempty"`
	// Grow is how many percent bigger (smaller, negative) it shows its bearer
	// (EffectSize: Player.Body)
	Grow int `json:",omitempty"`
	// Kept: it ends only at its time or its bearer's death: a strip, Mr.
	// Moogie's "Clear my buffs" and a cure leave it on, as they leave the towns'
	// Boon (Mr. Moogie's Duelist's Favor, metalmoogie.go); a GM's #clearbuffs
	// ends it
	Kept bool `json:",omitempty"`
	// Raiment: a Wisp in its own form shown in all it wears (EffectRaiment,
	// wispform.go); ended as an illusion is
	Raiment bool `json:",omitempty"`
}

// rulesOf is a cast's rules for what lasts of it.
func rulesOf(d *SpellDef) EffectRules {
	r := EffectRules{DispelOnCast: d.DispelOnCast, DispelOnContact: d.DispelOnContact, Harm: d.Harmful, Twice: d.Workbook, Stance: d.Stance,
		ProcChance: d.ProcChance, ProcRound: d.ProcRound}
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
	case EffectRaiment:
		ae.Rules.Raiment = true
	case EffectMarkHeal:
		ae.Rules.MarkHeal = max(ae.Rules.MarkHeal, e.Amount) // markheal.go
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

// lullChaPct is a lull's share of its caster's CHA in seconds when its effect
// names none of CHA: charm's usual (charm.go, "Increase 25% based on CHA").
const lullChaPct = 25

// lullBonus is what a caster's CHA adds to a lull's time: the beta's stat
// sheet's (betaformula.webp, adopted by the owner 2026-11-11: "Spells that
// modify an NPCs behavior (Charm, Lull, Etc) will have a CHA mod on them that
// increases their duration"), as charmDuration's: its effect's CHA share
// (scale_pct of cha: Soothe's 10%) of the CHA in seconds, else lullChaPct's
// (Soothing Verse's DEX 15% is not CHA: 25% of the CHA, its DEX nothing); 0 for
// a spell that does not lull. Charm has its own (charmDuration); no other kind
// of ours changes a creature's behaviour by itself (a root and a snare hold it
// by the con's rolls, root.go; a hate or forget lasts nothing), so the sheet's
// "Etc" adds nothing more. Until then a lull lasted its line's time, its CHA
// and DEX shares read and unused.
func lullBonus(d *SpellDef, cha int) time.Duration {
	for _, e := range d.Effects {
		if e.Kind != EffectLull {
			continue
		}
		pct := lullChaPct
		if e.Scale == StatCha && e.ScalePct > 0 {
			pct = e.ScalePct
		}
		return time.Duration(pct*max(cha, 0)) * time.Second / 100
	}
	return 0
}

// resistOutcomeWords is the caster's line for a spell its target resisted
// whole (EverQuest's words: no recording of EQOA's exists).
const spellResisted = "Your target resisted the %s spell."

// spellResistedLeast is the caster's line when its target resisted a spell whose hit
// still lands at its least (leastHarm).
const spellResistedLeast = "Your target resisted the %s spell: it does the least it can."

// leastHarm is a damage effect (a hit or a tick on hit points) at the least it does: the
// figure of its range nearest nothing, no caster's stat or level added. A creature's
// resist never takes all of a spell's or a proc's damage (the user, 2026-11-07: "in eqoa
// mobs cant 100% negate a spell or attack with resists, it will hit for minimum damage
// that it can only"); what is no damage (a debuff, a lull, a hate) it still resists whole.
func leastHarm(e SpellEffect) (SpellEffect, bool) {
	if (e.Kind != EffectHP && e.Kind != EffectHPTick) || e.Amount >= 0 {
		return e, false
	}
	lo := e.Amount
	if e.AmountMax < 0 && e.AmountMax > lo {
		lo = e.AmountMax
	}
	e.Amount, e.AmountMax = lo, lo
	e.Scale, e.ScalePct, e.LevelPct = StatNone, 0, 0
	return e, true
}

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
	off := npcResistDebuff(npc, st, now)
	if off == 0 {
		return 0
	}
	return int(ResistShare(off)*100 + 0.5)
}

// npcResistDebuff is how far a creature's resist of a kind is lowered now:
// the largest debuff of it on the creature (debuffs of a kind do not add), 0
// for none.
func npcResistDebuff(npc *NPC, st Stat, now time.Time) int {
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
	return off
}

// debuffScaled is a hit n (negative) of an effect that grows with its
// target's debuff (SpellEffect.DebuffStat), as hard as the debuff on npc
// makes it; any other effect's as it is.
func debuffScaled(e SpellEffect, npc *NPC, n int, now time.Time) int {
	if e.DebuffPct == 0 || e.DebuffStat == StatNone || npc == nil {
		return n
	}
	off := npcResistDebuff(npc, e.DebuffStat, now)
	if off <= 0 {
		return n
	}
	return int(math.Round(float64(n) * (1 + e.DebuffPct*float64(off)/100)))
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
	harm, kept, least := 0, 0, 0
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
		} else if l, ok := leastHarm(e); ok {
			effects = append(effects, l)
			least++
		}
	}
	if harm == 0 || kept == harm {
		return cs, true
	}
	log.Printf("Cast: %s resists %d of %s's %q's %d harmful effects (%d of them land at their least)", npc.Name, harm-kept, caster.Name, cs.spell.Name, harm, least)
	if kept == 0 && casterSess != nil {
		line := spellResisted
		if least > 0 {
			line = spellResistedLeast
		}
		w.sendColoredLocked(casterSess, fmt.Sprintf(line, cs.spell.Name), castRefusalRGB)
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

// dispelOnCastLocked ends what lasts on a player that ends when it casts:
// the parts the workbook marks dispel_on_cast alone, when something else of
// the effect lasts (parts.go), else the whole effect. A run speed so marked
// (Spirit of the Wolf's, Spirit Circle's, a song's or a wolf form's) ends
// only at a harmful cast (harmful: the cast's), never at a buff or another
// beneficial spell (petition 1067: "SoW is being removed when casting buffs
// and beneficial spells"), as the boards' Spirit of the Wolf ends only at a
// hostile act (boardwolf.go); the rest of what a cast ends ends at any.
func (w *World) dispelOnCastLocked(p *Player, harmful bool) {
	match := partOnCast
	if !harmful {
		match = partOnBeneficialCast
	}
	for _, e := range append([]*ActiveEffect(nil), p.Effects...) {
		if e.Rules.DispelOnCast {
			w.endOnPlayerLocked(p, e, match, "a cast")
		}
	}
}

// contactPlayerLocked ends what lasts on a player that ends when it is hit
// or harmed: the parts the workbook marks dispel_on_contact alone, when
// something else of the effect lasts (parts.go), else the whole effect. A run
// speed so marked (Spirit of the Wolf's, Spirit Circle's, a song's or a wolf
// form's) stays, to end at its bearer's own hit (ownHitLocked; petition 1109:
// "Spirit of the wolf is dropped when hit, it should only drop when player
// hits mob").
func (w *World) contactPlayerLocked(p *Player) {
	for _, e := range append([]*ActiveEffect(nil), p.Effects...) {
		if e.Rules.DispelOnContact {
			w.endOnPlayerLocked(p, e, partOnPlayerHit, "a hit")
		}
	}
}

// ownHitLocked ends a run speed the workbook marks dispel_on_contact at its
// bearer's own hit on a creature, a swing or a throw (why says which), as
// contactPlayerLocked no longer does at a hit taken; a harmful cast ends it as
// a cast (dispelOnCastLocked). What else of the effect lasts stays.
func (w *World) ownHitLocked(p *Player, why string) {
	for _, e := range append([]*ActiveEffect(nil), p.Effects...) {
		if e.Rules.DispelOnContact && e.Speed {
			w.endOnPlayerLocked(p, e, partOnOwnHit, why)
		}
	}
}

// untilContact is how long a harmful effect stays that the workbook gives no
// time but ends at contact (Enshrouding darkness: the user's "until hit"):
// a day, so a hit ends it, or its bearer's death, respawn or a restart.
const untilContact = 24 * time.Hour

// contactNPCLocked ends what lasts on a creature that ends when it is hit
// or harmed (a lull, a snare): the parts the workbook marks alone, when
// something else of the effect lasts (parts.go: a creature's slows, holds,
// body and invisibility are read from what is left), else the whole effect.
func contactNPCLocked(npc *NPC) {
	if len(npc.Effects) == 0 {
		return
	}
	left := npc.Effects[:0]
	for _, e := range npc.Effects {
		// its own tick is no contact for itself
		if e.Rules.DispelOnContact && !e.ticking && !endOnNPC(npc, e, partOnContact, "a hit") {
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

// noLowestLevelUpTo is the highest spell level with no lowest level to receive it
// (petition 1130, the GM Jibrish: "All level 50 or below buffs should work on all
// players below level 50, including level 1"). The workbook's lowest levels hold for
// the spells above it. A resurrection has none at any level ("Resurrect spells should
// work on all levels, including level 60 res on level 1").
const noLowestLevelUpTo = 50

// tooLowLocked reports a player below the spell's lowest level to receive it,
// telling its caster. Must be called with the lock held.
func (w *World) tooLowLocked(caster *Player, casterSess *session.Session, t *Player, s *CastSpell) bool {
	d := &s.Def
	if d.MinLevel <= 0 || t.Level >= d.MinLevel || t == caster || s.Level <= noLowestLevelUpTo || d.Resurrects() > 0 {
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
		if most := cs.spell.Def.CureMax; most > 0 && n >= most {
			break // a cure of so many (CureMax)
		}
		if ae.SpellID == cs.spell.SpellID || ae.Rules.NoCure {
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
	case w == "mitigation":
		// a barrier: an effect that mitigates melee hits (the paladin's
		// Clay to Talimus's Barriers; their hold is part of the effect)
		return ae.Rules.MitigatePct > 0
	case w == "stance":
		// a stance's cure: the bearer's other stances (its own spell is
		// never cured by itself: cureLocked)
		return ae.Rules.Stance
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
		if !defense && ae.Proc > 0 {
			continue // its heal comes with its hit, one proc (procTapLocked)
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
		w.procHealLocked(p, w.findSessionByEntityIDLocked(p.ServerID), p, n)
	}
}
