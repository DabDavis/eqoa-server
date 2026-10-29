package entity

import (
	"encoding/json"
	"time"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/session"
)

// buffsave.go — a player's spell effects kept over a logout.
//
// Sandstorm keeps them, and their time runs only while the character is in
// the world (2026-10-21, claude-notes/casting-sandstorm-20261021.md §6):
// Rockskin and Nature's Wisdom, 30 minutes each, were still on a druid 54
// minutes after they were cast, about 5 of them spent online, and came back
// at the login with their stats (AC 150, WIS 87, max HP 536, max power 1025;
// the base fields unbuffed). A Minor Healing cast 1.5 s before a logout was
// on again at the next login, 40 s later, ticked at once and wore off 12.4 s
// after: its time had run some 10 s past the logout, as though the character
// stayed in the world that long. Here the time stops at the logout.
//
// What is kept is the effect as it landed (its ticks and stat changes worked
// out from its caster's stats then), what is left of it, and of its next tick.

// The flags a kept effect's row carries.
const (
	buffRoot = 1 << iota
	buffSnare
	buffSpeed
	buffInvis
)

// SavedEffects is a player's spell effects as they are kept, what is left of
// each now; ok is false when id is no player in the world.
func (w *World) SavedEffects(id EntityID, now time.Time) (rows []database.BuffRow, ok bool) {
	w.mu.RLock()
	defer w.mu.RUnlock()
	p := w.Players[id]
	if p == nil {
		return nil, false
	}
	return savedEffects(p, now), true
}

// savedMods is an effect's mods column: its stat changes and, beside them, a
// proc's damage (proc.go), which has no column of its own; a row kept before
// procs reads as none.
type savedMods struct {
	StatMods
	Proc    int `json:",omitempty"`
	ProcMax int `json:",omitempty"`
	// a proc scaled by its bearer's stat, and a proc's cap (proc.go)
	ProcScale    Stat `json:",omitempty"`
	ProcScalePct int  `json:",omitempty"`
	ProcCap      int  `json:",omitempty"`
	// an illusion's body and its scale (illusion.go); a row kept before
	// illusions reads as none
	Form     uint32  `json:",omitempty"`
	FormSize float32 `json:",omitempty"`
	// a heal over time's ticks left (hot.go); a row kept before reads as
	// none, an effect that ticks until it ends
	Ticks int `json:",omitempty"`
	// a damage shield or defensive proc (dshield.go); a row kept before
	// reads as none
	Shield *Shield `json:",omitempty"`
	// a ceiling on the maximum power (powercap.go); a row kept before reads
	// as none
	PowerCap *int `json:",omitempty"`
}

func savedEffects(p *Player, now time.Time) []database.BuffRow {
	var rows []database.BuffRow
	for _, e := range p.Effects {
		left := e.Until.Sub(now)
		if left <= 0 || e.duelFoe != 0 { // a duel's harm ends with it (duel.go)
			continue
		}
		tick := int64(-1)
		if !e.NextTick.IsZero() && (e.HPTick != 0 || e.PowerTick != 0) {
			tick = max(0, e.NextTick.Sub(now).Milliseconds())
		}
		sm := savedMods{StatMods: e.Mods, Proc: e.Proc, ProcMax: e.ProcMax, ProcScale: e.ProcScale, ProcScalePct: e.ProcScalePct,
			ProcCap: e.ProcCap, Form: e.Form, FormSize: e.FormSize, Ticks: e.Ticks, Shield: e.Shield}
		if e.PowerCapped {
			top := e.PowerCap
			sm.PowerCap = &top
		}
		mods, _ := json.Marshal(sm)
		flags := 0
		for _, f := range []struct {
			on  bool
			bit int
		}{{e.Root, buffRoot}, {e.Snare, buffSnare}, {e.Speed, buffSpeed}, {e.Invis, buffInvis}} {
			if f.on {
				flags |= f.bit
			}
		}
		rows = append(rows, database.BuffRow{SpellID: e.SpellID, Name: e.Name, Icon: e.Icon, Caster: uint32(e.Caster),
			LeftMS: left.Milliseconds(), TickMS: tick, HPTick: e.HPTick, PowerTick: e.PowerTick, Mods: string(mods), Flags: flags})
	}
	return rows
}

// RestoreEffects puts kept effects back on a session's player, their time
// running again from now, as it logs in; the stat and effects channels carry
// them from the next tick, and the world entry's 0x00F8 a speed one. It
// returns how many it put back.
func (w *World) RestoreEffects(sess *session.Session, rows []database.BuffRow, now time.Time) int {
	w.mu.Lock()
	defer w.mu.Unlock()
	p := w.sessionPlayerLocked(sess)
	if p == nil {
		return 0
	}
	n := 0
	for _, r := range rows {
		if r.LeftMS <= 0 || p.hasEffect(r.SpellID) {
			continue
		}
		ae := &ActiveEffect{SpellID: r.SpellID, Name: r.Name, Icon: r.Icon, Caster: EntityID(r.Caster),
			Until: now.Add(time.Duration(r.LeftMS) * time.Millisecond), HPTick: r.HPTick, PowerTick: r.PowerTick,
			Root: r.Flags&buffRoot != 0, Snare: r.Flags&buffSnare != 0, Speed: r.Flags&buffSpeed != 0, Invis: r.Flags&buffInvis != 0}
		if r.TickMS >= 0 {
			ae.NextTick = now.Add(time.Duration(r.TickMS) * time.Millisecond)
		}
		if r.Mods != "" {
			var sm savedMods
			_ = json.Unmarshal([]byte(r.Mods), &sm)
			ae.Mods, ae.Proc, ae.ProcMax = sm.StatMods, sm.Proc, sm.ProcMax
			ae.ProcScale, ae.ProcScalePct, ae.ProcCap = sm.ProcScale, sm.ProcScalePct, sm.ProcCap
			ae.Form, ae.FormSize = sm.Form, sm.FormSize
			ae.Ticks = sm.Ticks
			ae.Shield = sm.Shield
			if sm.PowerCap != nil {
				ae.PowerCap, ae.PowerCapped = *sm.PowerCap, true
			}
		}
		p.Effects = append(p.Effects, ae)
		if ae.Name != "" && len(p.Buffs) < MaxEffects {
			p.Buffs = append(p.Buffs, BuffSlot{Icon: ae.Icon, Name: ae.Name})
		}
		n++
	}
	var m StatMods
	for _, e := range p.Effects {
		m.sum(e.Mods)
	}
	p.Mods = m
	if top, ok := p.powerCap(); ok && p.Power > top { // powercap.go
		p.Power = max(top, 0)
	}
	p.StatsDirty = true
	return n
}

func (p *Player) hasEffect(spellID int) bool {
	for _, e := range p.Effects {
		if e.SpellID == spellID {
			return true
		}
	}
	return false
}

// PlayerSpeed is the session's player's run speed with its effects, for the
// world entry's 0x00F8; def with no player.
func (w *World) PlayerSpeed(sess *session.Session, def float32) float32 {
	w.mu.RLock()
	defer w.mu.RUnlock()
	if p := w.sessionPlayerLocked(sess); p != nil {
		return p.SpeedNow()
	}
	return def
}
