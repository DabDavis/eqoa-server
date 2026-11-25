package entity

import (
	"encoding/binary"

	"github.com/eqoa/pkg/pkg/buffer"
	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// statfigures.go — the real offence and defence figures for a client's own stat
// window (vanilla's Stat Window; petition 2925: "Would it be possible to also
// include OFF MODS, DEF MODS"; the owner, 2026-11-24: "Yes, real figures"). The
// player's OffMod and DefMod are the old fitted placeholders no roll reads
// (statsreport.go), so what goes is what #stats states of the rolls: the
// player's hit on a creature of its own level (offence.go) and such a creature's
// hit on it (defence.go), the same calls #stats makes (playerFigures).
//
// A client that draws them answers the offer (protocol.OpcodeStatFigures,
// handlers/statfigures.go) 1 while its window is open and is sent the set
// (OpcodeStatFiguresData) at once and, every statFiguresTicks, whenever it
// changed: gear, effects, level, stats, masteries, whatever moves a roll. A PS2
// never answers and is sent nothing.

// The figures' ids on the wire (protocol.OpcodeStatFiguresData).
const (
	StatFigMeleeLow    = 1
	StatFigMeleeHigh   = 2
	StatFigRangedLow   = 3
	StatFigRangedHigh  = 4
	StatFigDefenceLow  = 5
	StatFigDefenceHigh = 6
)

// statFiguresTicks is how often the system looks: half a second, as the buff
// window's.
const statFiguresTicks = 5

// StatFigure is one figure of the set: its id (StatFig*) and its value.
type StatFigure struct {
	ID    uint8
	Value int32
}

// playerFigures is what #stats states of p's rolls: on a creature of its own
// level, its hit (an arrow's when ranged: a ranged weapon worn) low and high
// with its floor applied, and that creature's hit on it low and high.
func playerFigures(p *Player) (ranged bool, lo, hi, floor, dlo, dhi int) {
	foe := &NPC{Level: p.Level}
	var low, high int
	if p.Gear.WeaponSlot == 18 { // a ranged weapon's arrow: DEX its top, WIS its bottom (offence.go)
		ranged = true
		floor, low, high = playerRangedRoll(p, foe)
	} else {
		floor, low, high = playerMeleeRoll(p, foe)
	}
	dlo, dhi = hitRangeOn(creatureRoll(p.Level), p)
	return ranged, max(low, floor), max(high, floor), floor, dlo, dhi
}

// StatFiguresOf is p's set, in the ids' order.
func StatFiguresOf(p *Player) []StatFigure {
	ranged, lo, hi, _, dlo, dhi := playerFigures(p)
	idLo, idHi := uint8(StatFigMeleeLow), uint8(StatFigMeleeHigh)
	if ranged {
		idLo, idHi = StatFigRangedLow, StatFigRangedHigh
	}
	return []StatFigure{{idLo, int32(lo)}, {idHi, int32(hi)}, {StatFigDefenceLow, int32(dlo)}, {StatFigDefenceHigh, int32(dhi)}}
}

// EncodeStatFigures is OpcodeStatFiguresData's body: a u8 count, then each a
// u8 id and an i32 value, little-endian.
func EncodeStatFigures(figs []StatFigure) []byte {
	out := []byte{byte(len(figs))}
	for _, f := range figs {
		out = append(out, f.ID)
		out = binary.LittleEndian.AppendUint32(out, uint32(f.Value))
	}
	return out
}

// SetStatFigures makes sess's player's client be sent its figures (on) or stop
// (off); on, the set goes out at the next pass whatever was sent before. False
// when sess has no player.
func (w *World) SetStatFigures(sess *session.Session, on bool) bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	p := w.sessionPlayerLocked(sess)
	if p == nil {
		return false
	}
	p.StatFigures = on
	p.statFiguresSent, p.statFiguresDue = "", on
	return true
}

// statFiguresSystem sends each client that asked its figures when they
// changed, and at once after the ask. Every statFiguresTicks. Must be called
// with the lock held.
func (w *World) statFiguresSystem() {
	if w.TickCount%statFiguresTicks != 0 || w.SessionManager == nil {
		return
	}
	w.SessionManager.ForEach(func(s *session.Session) bool {
		if p := w.sessionPlayerLocked(s); p != nil && p.StatFigures {
			w.sendStatFiguresLocked(s, p)
		}
		return true
	})
}

// sendStatFiguresLocked sends p's figures to s when they changed or were just
// asked for.
func (w *World) sendStatFiguresLocked(s *session.Session, p *Player) {
	body := EncodeStatFigures(StatFiguresOf(p))
	if !p.statFiguresDue && string(body) == p.statFiguresSent {
		return
	}
	p.statFiguresSent, p.statFiguresDue = string(body), false
	sendGroupMsg(s, protocol.OpcodeStatFiguresData, func(b *buffer.Writer) { b.WriteBytes(body) })
}
