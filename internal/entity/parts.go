package entity

import (
	"log"
	"slices"
	"time"
)

// parts.go — an effect's parts that end apart. The workbook marks each part of
// a spell to end at a hit (dispel_on_contact) or at its bearer's cast
// (dispel_on_cast), and its bearer's own action (a cast, a swing, a throw:
// invis.go) ends its invisibility (a hide's slow with it, below); until
// 2026-11-01 each of these ended the whole effect, so a hit took Group Wolf
// Form's form and stats with its speed, a swing Spirit Walk's speed with its
// invisibility. The user, 2026-11-01: "yes only end the marked part". An effect
// whose parts may end apart keeps what each of its lasting parts puts on
// (effectPart, as lastingParts builds it from that part alone, out of its
// caster's figures as it landed), and an end takes off the parts it ends alone,
// the rest staying to its time; one whose every lasting part ends ends whole,
// as before. What a part's end does is what the whole effect's end does for
// what that part gave: on a player the stats summed again and a new run speed
// told (effectsChangedLocked), and the body, a hold, a slow and the
// invisibility read from what is left (Body, SpeedNow, Invisible); on a
// creature, what is left read the same way. The parts are kept over a logout
// (buffsave.go), so an ended part comes back on no one.

// effectPart is what one lasting part of an effect puts on its bearer, what
// ends it alone (Contact, Cast, Invisibility) and whether it has ended.
type effectPart struct {
	HPTick       int      `json:",omitempty"`
	PowerTick    int      `json:",omitempty"`
	TickDrain    DrainTo  `json:",omitempty"`
	TickRange    float32  `json:",omitempty"`
	Mods         StatMods `json:",omitempty"`
	Root         bool     `json:",omitempty"`
	Snare        bool     `json:",omitempty"`
	Speed        bool     `json:",omitempty"`
	Invis        bool     `json:",omitempty"`
	Proc         int      `json:",omitempty"`
	ProcMax      int      `json:",omitempty"`
	ProcScale    Stat     `json:",omitempty"`
	ProcScalePct int      `json:",omitempty"`
	ProcCap      int      `json:",omitempty"`
	ProcUndead   bool     `json:",omitempty"`
	Shield       *Shield  `json:",omitempty"`
	PowerCap     int      `json:",omitempty"`
	PowerCapped  bool     `json:",omitempty"`
	// the workbook's own (workbook.go's EffectRules)
	SpeedPct         int  `json:",omitempty"`
	MitigatePct      int  `json:",omitempty"`
	MitigateLeft     int  `json:",omitempty"`
	MitigateCapped   bool `json:",omitempty"`
	Lull             bool `json:",omitempty"`
	Breath           bool `json:",omitempty"`
	Hide             bool `json:",omitempty"`
	Disguise         bool `json:",omitempty"`
	ProcHeal         int  `json:",omitempty"`
	ProcHealDef      bool `json:",omitempty"`
	ProcHealScale    Stat `json:",omitempty"`
	ProcHealScalePct int  `json:",omitempty"`
	ProcHealChance   int  `json:",omitempty"`
	ProcHealUndead   bool `json:",omitempty"`
	// Form: an illusion (its body is the effect's, illusion.go); SetStat: a
	// stat set (setStatsLocked measures it on the bearer)
	Form    bool `json:",omitempty"`
	SetStat bool `json:",omitempty"`
	// what ends it alone: a hit (the workbook's dispel_on_contact), its
	// bearer's cast (dispel_on_cast), its bearer's own action (an
	// invisibility: invis.go)
	Contact      bool `json:",omitempty"`
	Cast         bool `json:",omitempty"`
	Invisibility bool `json:",omitempty"`
	Ended        bool `json:",omitempty"`
}

// The ends a part may have alone. A beneficial cast ends no run speed
// (dispelOnCastLocked, petition 1067), so it ends what a cast ends but that.
func partOnContact(p *effectPart) bool        { return p.Contact }
func partOnCast(p *effectPart) bool           { return p.Cast }
func partOnBeneficialCast(p *effectPart) bool { return p.Cast && !p.Speed }
func partInvisible(p *effectPart) bool        { return p.Invisibility }

// partOf is what a's one part puts on, its marks e's.
func partOf(a *ActiveEffect, e SpellEffect) *effectPart {
	r := a.Rules
	return &effectPart{HPTick: a.HPTick, PowerTick: a.PowerTick, TickDrain: a.TickDrain, TickRange: a.TickRange, Mods: a.Mods,
		Root: a.Root, Snare: a.Snare, Speed: a.Speed, Invis: a.Invis, Proc: a.Proc, ProcMax: a.ProcMax, ProcScale: a.ProcScale,
		ProcScalePct: a.ProcScalePct, ProcCap: a.ProcCap, ProcUndead: a.ProcUndead, Shield: a.Shield, PowerCap: a.PowerCap,
		PowerCapped: a.PowerCapped, SpeedPct: r.SpeedPct, MitigatePct: r.MitigatePct, MitigateLeft: r.MitigateLeft,
		MitigateCapped: r.MitigateCapped, Lull: r.Lull, Breath: r.Breath, Hide: r.Hide, Disguise: r.Disguise, ProcHeal: r.ProcHeal,
		ProcHealDef: r.ProcHealDef, ProcHealScale: r.ProcHealScale, ProcHealScalePct: r.ProcHealScalePct,
		ProcHealChance: r.ProcHealChance, ProcHealUndead: r.ProcHealUndead,
		Form: e.Kind == EffectIllusion, SetStat: e.Kind == EffectStat && e.Set,
		Contact: e.EndsOnContact, Cast: e.EndsOnCast, Invisibility: a.Invis}
}

// gives reports whether the part puts anything on, its body aside.
func (p *effectPart) gives() bool {
	return p.HPTick != 0 || p.PowerTick != 0 || p.Mods != (StatMods{}) || p.Root || p.Snare || p.Speed || p.Invis ||
		p.Proc != 0 || p.ProcMax != 0 || p.Shield != nil || p.PowerCapped || p.SpeedPct != 0 || p.MitigatePct != 0 ||
		p.Lull || p.Breath || p.Hide || p.Disguise || p.ProcHeal != 0 || p.SetStat
}

// mergeParts is what parts put on together, as lasting sums them.
func mergeParts(ps []*effectPart) effectPart {
	var m effectPart
	for _, p := range ps {
		m.HPTick += p.HPTick
		m.PowerTick += p.PowerTick
		if p.TickDrain != DrainNone && m.TickDrain == DrainNone {
			m.TickDrain, m.TickRange = p.TickDrain, p.TickRange
		}
		m.Mods.sum(p.Mods)
		m.Root, m.Snare, m.Speed, m.Invis = m.Root || p.Root, m.Snare || p.Snare, m.Speed || p.Speed, m.Invis || p.Invis
		if (p.Proc != 0 || p.ProcMax != 0) && m.Proc == 0 && m.ProcMax == 0 {
			m.Proc, m.ProcMax, m.ProcScale, m.ProcScalePct, m.ProcCap, m.ProcUndead =
				p.Proc, p.ProcMax, p.ProcScale, p.ProcScalePct, p.ProcCap, p.ProcUndead
		}
		if p.Shield != nil && m.Shield == nil {
			m.Shield = p.Shield
		}
		if p.PowerCapped && (!m.PowerCapped || p.PowerCap < m.PowerCap) {
			m.PowerCap, m.PowerCapped = p.PowerCap, true
		}
		m.SpeedPct = min(m.SpeedPct, p.SpeedPct)
		if p.MitigatePct > m.MitigatePct {
			m.MitigatePct, m.MitigateLeft, m.MitigateCapped = p.MitigatePct, p.MitigateLeft, p.MitigateCapped
		}
		m.Lull, m.Breath, m.Hide, m.Disguise = m.Lull || p.Lull, m.Breath || p.Breath, m.Hide || p.Hide, m.Disguise || p.Disguise
		if p.ProcHeal != 0 && m.ProcHeal == 0 {
			m.ProcHeal, m.ProcHealDef, m.ProcHealScale = p.ProcHeal, p.ProcHealDef, p.ProcHealScale
			m.ProcHealScalePct, m.ProcHealChance, m.ProcHealUndead = p.ProcHealScalePct, p.ProcHealChance, p.ProcHealUndead
		}
		m.Form, m.SetStat = m.Form || p.Form, m.SetStat || p.SetStat
	}
	return m
}

// partsOf is what each lasting part of a spell puts on, for an effect of
// dur whose parts may end apart: for a hit, a cast or its bearer's action,
// one part ending while another lasting part does not; nil for any other
// (it ends whole, as it did) and for one that lasts until a hit, its time
// the hit's (untilContact).
func partsOf(cs *castState, caster *Player, now time.Time, dur time.Duration) []*effectPart {
	d := &cs.spell.Def
	if d.Duration == untilContact || dur <= 0 {
		return nil
	}
	var out []*effectPart
	for _, e := range d.Effects {
		c := *cs
		c.spell.Def.Effects, c.spell.Def.Duration = []SpellEffect{e}, dur
		a := lastingParts(&c, caster, now)
		if a == nil {
			continue
		}
		if p := partOf(a, e); p.gives() || p.Form {
			out = append(out, p)
		}
	}
	// a rogue's hide slows its bearer while it hides, and the slow is the
	// hiding's own: it ends with the hiding at its bearer's action, though
	// the workbook marks it to end only at a hit or a cast (the user,
	// 2026-11-01: "end the slow"). A slow beside any other invisibility is no
	// hide's and ends as its marks say (the workbook has none)
	if slices.ContainsFunc(out, func(p *effectPart) bool { return p.Hide }) {
		for _, p := range out {
			if p.SpeedPct < 0 {
				p.Invisibility = true
			}
		}
	}
	for _, match := range []func(*effectPart) bool{partOnContact, partOnCast, partInvisible} {
		ends, stays := false, false
		for _, p := range out {
			if match(p) {
				ends = true
			} else {
				stays = true
			}
		}
		if ends && stays {
			return out
		}
	}
	// a run speed a cast ends ends only at a harmful one (dispelOnCastLocked:
	// petition 1067), so an effect with one keeps its parts to tell it from
	// what a beneficial cast ends, though every part of it ends at a cast
	// (Spirit Circle's lone speed, a haste beside a stat a cast ends)
	if slices.ContainsFunc(out, func(p *effectPart) bool { return p.Speed && p.Cast }) {
		return out
	}
	return nil
}

// endParts ends the effect's parts that end on what happened (match), the
// others staying as they are now: what the ending parts put on is taken off
// (takeOff), they are marked ended, and what ends at a hit or a cast is
// what is left of it. It reports matched when a part ended, and whole when
// nothing that lasts would be left beside them (or none is kept: the caller
// ends the effect whole, as before).
func (ae *ActiveEffect) endParts(match func(*effectPart) bool) (matched, whole bool) {
	if ae.parts == nil {
		return true, true
	}
	var ending, staying []*effectPart
	for _, p := range ae.parts {
		switch {
		case p.Ended:
		case match(p):
			ending = append(ending, p)
		default:
			staying = append(staying, p)
		}
	}
	if len(ending) == 0 {
		return false, false
	}
	rest := mergeParts(staying)
	if !rest.gives() && !(rest.Form && ae.Form != 0) {
		return true, true // nothing of it lasts beside them (an illusion of no body is nothing)
	}
	ae.takeOff(mergeParts(ending), rest)
	for _, p := range ending {
		p.Ended = true
	}
	ae.Rules.DispelOnContact, ae.Rules.DispelOnCast = false, false
	for _, p := range staying {
		ae.Rules.DispelOnContact = ae.Rules.DispelOnContact || p.Contact
		ae.Rules.DispelOnCast = ae.Rules.DispelOnCast || p.Cast
	}
	return true, false
}

// takeOff takes what the ending parts put on (m) off the effect, what the
// staying parts give (r) staying as it is now (a heal's ticks left, a
// mitigation's spent share): their stats and ticks taken back, a hold, a
// slow, a speed or an invisibility gone unless a staying part gives it too,
// an illusion's body with its part.
func (ae *ActiveEffect) takeOff(m, r effectPart) {
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
	if m.SpeedPct != 0 {
		ae.Rules.SpeedPct = r.SpeedPct
	}
	if m.MitigatePct != 0 {
		ae.Rules.MitigatePct, ae.Rules.MitigateLeft, ae.Rules.MitigateCapped = r.MitigatePct, r.MitigateLeft, r.MitigateCapped
	}
	if m.ProcHeal != 0 {
		ae.Rules.ProcHeal, ae.Rules.ProcHealDef, ae.Rules.ProcHealScale = r.ProcHeal, r.ProcHealDef, r.ProcHealScale
		ae.Rules.ProcHealScalePct, ae.Rules.ProcHealChance, ae.Rules.ProcHealUndead = r.ProcHealScalePct, r.ProcHealChance, r.ProcHealUndead
	}
	ae.Rules.Lull = ae.Rules.Lull && (!m.Lull || r.Lull)
	ae.Rules.Breath = ae.Rules.Breath && (!m.Breath || r.Breath)
	ae.Rules.Hide = ae.Rules.Hide && (!m.Hide || r.Hide)
	ae.Rules.Disguise = ae.Rules.Disguise && (!m.Disguise || r.Disguise)
	if m.Form && !r.Form {
		ae.Form, ae.FormSize = 0, 0
	}
}

// endOnPlayerLocked ends what of a player's effect ends on what happened
// (why, for the log): its matching parts alone when something else of it
// lasts, the stats summed again and a new run speed told as a whole end
// does (effectsChangedLocked); else the whole effect (removeEffectLocked).
func (w *World) endOnPlayerLocked(p *Player, e *ActiveEffect, match func(*effectPart) bool, why string) {
	speed := p.SpeedNow()
	matched, whole := e.endParts(match)
	switch {
	case !matched:
	case whole:
		log.Printf("Cast: %q ends on %s: %s", e.Name, p.Name, why)
		w.removeEffectLocked(p, e.SpellID)
	default:
		log.Printf("Cast: %q's parts that end at %s end on %s, the rest stays", e.Name, why, p.Name)
		w.effectsChangedLocked(p, speed)
	}
}

// endOnNPC is endOnPlayerLocked for a creature's effect: it reports whether
// the effect stays (its matching parts alone ended, or none matched).
func endOnNPC(npc *NPC, e *ActiveEffect, match func(*effectPart) bool, why string) bool {
	matched, whole := e.endParts(match)
	switch {
	case !matched:
		return true
	case whole:
		log.Printf("Cast: %q ends on %s: %s", e.Name, npc.Name, why)
		return false
	}
	log.Printf("Cast: %q's parts that end at %s end on %s, the rest stays", e.Name, why, npc.Name)
	return true
}
