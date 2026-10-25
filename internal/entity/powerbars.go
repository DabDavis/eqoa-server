package entity

import (
	"encoding/binary"

	"github.com/eqoa/pkg/pkg/buffer"
	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// Others' power, for the bars a client draws under their health
// (protocol.OpcodePowerBars, handlers/powerbars.go). The game shows no one's
// power but the player's own: the HUD's gauge reads the stat channel (0x42),
// the entity channel carries health alone (+0x19), and CLIENT's group bars
// and target bar draw health only. A client that draws more
// (eqoa-rust-client-vanilla's Mana Bars) asks for it with 0x7E01, and from
// then on, every 5 ticks after the group's channel, is sent the power of the
// players its bars show: each other member of its group (by the id its
// roster names them, a stand-in for one out of sight), its target and its
// target's target (by the id its entity channels carry). Only players have
// power on this server: an NPC keeps none and casts nothing, so a creature
// has no bar. A set goes out only when it differs from the last one sent,
// and each is the whole set: an id it leaves out has no bar.

// powerBar is one bar: the id the client knows the player by, and its power
// of 255.
type powerBar struct {
	id    uint32
	power byte
}

// SetPowerBars makes sess's player's client have power bars (on) or stop
// (off); on, the set goes out at the next pass whatever was sent before.
// False when sess has no player.
func (w *World) SetPowerBars(sess *session.Session, on bool) bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	p := w.sessionPlayerLocked(sess)
	if p == nil {
		return false
	}
	p.PowerBars = on
	p.powerBarsSent, p.powerBarsDue = "", on
	return true
}

// powerOf255 is p's power of 255; false for a player with no power at all.
func powerOf255(p *Player) (byte, bool) {
	top := p.MaxPowerNow()
	if top <= 0 {
		return 0, false
	}
	return byte(max(0, min(255, p.Power*255/top))), true
}

// powerBarsLocked is the set me's client's bars show: each other group
// member (as the roster names it, groupSyncSystem), the target and the
// target's target (as the entity channels name it, putChannelTargetLocked),
// the players among them, each once. Must be called with the lock held.
func (w *World) powerBarsLocked(sess *session.Session, me *Player) []powerBar {
	var out []powerBar
	add := func(id uint32, e Entity) {
		p, ok := e.(*Player)
		if !ok || id == 0 || id == NoAssist {
			return
		}
		for _, b := range out {
			if b.id == id {
				return
			}
		}
		if v, ok := powerOf255(p); ok {
			out = append(out, powerBar{id, v})
		}
	}
	if g := w.groups[me.GroupID]; me.GroupID != 0 && g != nil {
		for _, mid := range g.Members {
			if m := w.Players[mid]; m != nil && m != me {
				add(w.idSeenByLocked(sess, m), m)
			}
		}
	}
	if t := w.entitySeenByLocked(sess, me.TargetObjectID); t != nil {
		add(me.TargetObjectID, t)
		if tt := w.targetOfLocked(t); tt != nil {
			add(w.objectIDSeenByLocked(sess, tt.GetEntityID()), tt)
		}
	}
	return out
}

// encodePowerBars is OpcodePowerBarsData's body: a u8 count, then each bar's
// u32 id and u8 power (the first 255 bars).
func encodePowerBars(bars []powerBar) []byte {
	if len(bars) > 255 {
		bars = bars[:255]
	}
	out := make([]byte, 1, 1+5*len(bars))
	out[0] = byte(len(bars))
	for _, b := range bars {
		out = binary.LittleEndian.AppendUint32(out, b.id)
		out = append(out, b.power)
	}
	return out
}

// powerSyncSystem sends each client that asked the set its bars show when
// it changed (powerBarsLocked). Every 5 ticks, after groupSyncSystem, so a
// member's id is the one its roster was just sent. Must be called with the
// lock held.
func (w *World) powerSyncSystem() {
	if w.TickCount%5 != 0 || w.SessionManager == nil {
		return
	}
	w.SessionManager.ForEach(func(s *session.Session) bool {
		p := w.sessionPlayerLocked(s)
		if p == nil || !p.PowerBars {
			return true
		}
		body := encodePowerBars(w.powerBarsLocked(s, p))
		if !p.powerBarsDue && string(body) == p.powerBarsSent {
			return true
		}
		p.powerBarsSent, p.powerBarsDue = string(body), false
		sendGroupMsg(s, protocol.OpcodePowerBarsData, func(b *buffer.Writer) { b.WriteBytes(body) })
		return true
	})
}
