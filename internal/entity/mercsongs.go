package entity

import "time"

// mercsongs.go — a bard merc's songs and an enchanter merc's gifts to its
// team (the owner, 2026-11-11: "enchanter cast procs on party members and
// power. bard merc keeps buffs up since they are a lower cooldown").
//
// A bard's book's group songs (its anthems, dirges, Concerto to Harmony, the
// Power Jig line: two minutes and less, mostly, which a merc's book left out
// as a fight's buffs, mercBuffLasts) are kept up on its team, in a fight and
// out of one: each, the best rank of its line, cast again as it is about to
// run out on the bard (mercSongLead before its end, or gone), the first of
// them that is ready by its level, so they come round in turn on their short
// recasts, while it has the power for them (its power's return is what it
// sings with). Its arts only above half its power (mercBardSpare): until then its songs
// went once and its arts took the rest, its songs gone two minutes on.
//
// An enchanter's procs (the Chaotic line: a melee proc) and its power over
// time (the Clarity line, Empathy) go on each player of its team that lacks
// them, its owner first, in reach, in a fight or out of one, while half its
// power or more is left (mercBuffSpare): a proc on a player alone (a merc's
// swings run no procs), power on any with power. In a fight its power bursts
// (Power Flow to Power Flood: 24 s of power) go on the one of its team lowest
// in power, under half of it.
//
// Its team for these is its owner's own alone (merccast.go's doc,
// mercOwnTeamLocked: itself, its owner, its owner's other mercs; the owner,
// 2026-11-13): a bard's group song lands on no other player of its owner's
// group or raid, nor their mercs.

// mercBardSpare is the share of its power a bard keeps for its songs: its
// arts only above it.
const mercBardSpare = 50

// mercSongLead is how long before a song runs out a bard sings it again.
const mercSongLead = 6 * time.Second

// mercPowerLow is the share of its power under which a team mate is given an
// enchanter's power burst in a fight.
const mercPowerLow = 50

// classBard is the bard's class id.
const classBard = 5

// mercKeepsShort is whether a class's merc keeps a buff of under
// mercBuffLasts in its book: a bard's songs of its team (scope 1 or 2), an
// enchanter's power bursts (scope 1, power over time).
func mercKeepsShort(class int, s *CastSpell) bool {
	if s.Def.Duration <= 0 {
		return false
	}
	switch class {
	case classBard:
		return s.Scope == 1 || s.Scope == 2
	case classEnchanter:
		return s.Scope == 1 && mercGivesPower(s)
	}
	return false
}

// mercGivesPower is whether a spell gives power over time; mercGivesProc
// whether it puts a melee proc on its target.
func mercGivesPower(s *CastSpell) bool {
	for _, e := range s.Def.Effects {
		if !e.Self && (e.Kind == EffectStat && e.Stat == StatPoT && e.Amount > 0 || e.Kind == EffectPowerTick && e.Amount > 0) {
			return true
		}
	}
	return false
}

func mercGivesProc(s *CastSpell) bool {
	for _, e := range s.Def.Effects {
		if !e.Self && e.Kind == EffectProc {
			return true
		}
	}
	return false
}

// mercBuffUp is whether s (or a rank as good of its line) is on effects with
// more than lead to run.
func mercBuffUp(effects []*ActiveEffect, s *CastSpell, now time.Time, lead time.Duration) bool {
	line := BuffLine(s)
	for _, e := range effects {
		if e.SpellID == s.SpellID || sameLine(e.Line, line) && e.Rank >= lineRank(s) {
			if e.Until.IsZero() || e.Until.Sub(now) > lead {
				return true
			}
		}
	}
	return false
}

// mercCastingLocked is whether the merc has s on its way.
func (w *World) mercCastingLocked(ms *mercState, s *CastSpell) bool {
	for _, c := range w.mercCasts {
		if c.ms == ms && c.life == ms.life && c.spell.SpellID == s.SpellID {
			return true
		}
	}
	return false
}

// mercSongTurnLocked is a bard's song, if one is due (the file's doc). It
// reports a cast begun. Must be called with the lock held.
func (w *World) mercSongTurnLocked(ms *mercState, now time.Time) bool {
	m := ms.npc
	if ms.kit.Class != classBard || m.MaxPower <= 0 || w.ownFightLocked(ms.owner) {
		return false
	}
	for _, sp := range ms.mercBook() {
		s := sp.s
		if sp.use != useBuff || s.Scope != 2 && s.Scope != 1 || !ms.mercReady(s, now) || w.mercCastingLocked(ms, s) {
			continue
		}
		if mercBuffUp(m.Effects, s, now, s.CastTime+castLand+mercSongLead) {
			continue
		}
		if s.Scope == 2 {
			w.mercCastLocked(ms, s, m.ServerID, true, now)
		} else {
			w.mercCastLocked(ms, s, m.ServerID, false, now)
		}
		return true
	}
	return false
}

// mercTeamBuffTurnLocked is an enchanter's proc or power on one of its team,
// or its power burst in a fight (the file's doc). It reports a cast begun.
// Must be called with the lock held.
func (w *World) mercTeamBuffTurnLocked(ms *mercState, now time.Time) bool {
	m := ms.npc
	if ms.kit.Class != classEnchanter || m.MaxPower <= 0 || w.ownFightLocked(ms.owner) {
		return false
	}
	team := w.mercOwnTeamLocked(ms) // its owner's own alone (merccast.go's doc)
	book := ms.mercBook()
	reaches := func(s *CastSpell, mm mercMate) bool {
		x, y, z := mm.place()
		return mercReaches(m, s, x, y, z, 0)
	}
	// its power bursts in a fight: the lowest in power of its team
	if w.mercInFightLocked(ms) {
		var low *mercMate
		lowShare := float64(mercPowerLow) / 100
		for i := range team {
			mm := team[i]
			pw, top := mm.power()
			if top <= 0 {
				continue
			}
			if sh := float64(pw) / float64(top); sh < lowShare {
				low, lowShare = &team[i], sh
			}
		}
		if low != nil {
			for _, sp := range book {
				s := sp.s
				if sp.use != useBuff || !sp.short || !mercGivesPower(s) || !ms.mercReady(s, now) || w.mercCastingLocked(ms, s) ||
					!reaches(s, *low) || buffOn(low.effects(), s) {
					continue
				}
				w.mercCastLocked(ms, s, low.id(), false, now)
				return true
			}
		}
	}
	if m.Power*100 < m.MaxPower*mercBuffSpare {
		return false
	}
	for _, sp := range book {
		s := sp.s
		if sp.use != useBuff || sp.short || s.Scope != 1 || !ms.mercReady(s, now) || w.mercCastingLocked(ms, s) {
			continue
		}
		proc, power := mercGivesProc(s), mercGivesPower(s)
		if !proc && !power {
			continue
		}
		for _, mm := range team {
			if proc && mm.p == nil {
				continue // a merc's swings run no procs
			}
			if _, top := mm.power(); power && !proc && top <= 0 {
				continue
			}
			if mm.p != nil && s.Def.MinLevel > 0 && mm.p.Level < s.Def.MinLevel {
				continue
			}
			if buffOn(mm.effects(), s) || !reaches(s, mm) {
				continue
			}
			w.mercCastLocked(ms, s, mm.id(), false, now)
			return true
		}
	}
	return false
}

// power is a team mate's power and its most.
func (mm mercMate) power() (pw, top int) {
	if mm.p != nil {
		return mm.p.Power, mm.p.MaxPowerNow()
	}
	return mm.n.Power, mm.n.MaxPower
}
