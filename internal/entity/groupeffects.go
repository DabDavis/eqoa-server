package entity

import (
	"encoding/binary"
	"time"

	"github.com/eqoa/pkg/pkg/buffer"
	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// groupeffects.go — the effects on each of a player's group members, for the tiny icons a
// client draws beside their bars (jibrish, 2026-11-22: "add very small / tiny icon versions
// for the buffs group members have").
//
// CLIENT's group bars (VIWndGame::DrawGroup) show a member's name, its health and two
// status icons; no message of the game's carries another's effects (channel 0x43 is the
// player's own). A client that draws them (eqoa-rust-client-vanilla's Group Effects) asks
// as for others' power (powerbars.go): the server offers OpcodeGroupEffects at world entry
// (handlers/groupeffects.go), the client answers 1, and from then on, every
// groupEffectsTicks after groupSyncSystem (so a member's id is the one its roster was just
// sent), it is sent the set (OpcodeGroupEffectsData) when it changed: each other member its
// group window shows (shownViewsLocked: players and mercs), by the id its roster names it
// (groupIDSeenByLocked), with the icons of what the member's own buff window shows of it
// (buffWindowSetLocked's lines on the player: its named effects that have not run out, in
// the order they went on, then a channel 0x43 slot no effect backs), or a merc's named
// effects that have not run out. Icons only: a client's tiny icons need no name or time.
// A member in another zone or out of sight has its icons as well: the server holds them.
// A PS2 never asks, and is sent none.

// GroupEffectsMost is the most icons a member carries: its own buff window's most.
const GroupEffectsMost = BuffWindowPlayerMost

// groupEffectsTicks is how often the system looks: half a second, as the power bars'.
const groupEffectsTicks = 5

// memberEffects is one member's icons, by the id the receiving client knows it.
type memberEffects struct {
	id    uint32
	icons []uint32
}

// SetGroupEffects makes sess's player's client be sent its group members' effects (on) or
// stop (off); on, the set goes out at the next pass whatever was sent before. False when
// sess has no player.
func (w *World) SetGroupEffects(sess *session.Session, on bool) bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	p := w.sessionPlayerLocked(sess)
	if p == nil {
		return false
	}
	p.GroupEffects = on
	p.groupEffectsSent, p.groupEffectsDue = "", on
	return true
}

// memberIconsLocked is the icons of the effects on group member id at now: a player's as
// its own buff window shows them, a merc's named effects that have not run out, at most
// GroupEffectsMost; none for anything else. Must be called with the lock held.
func (w *World) memberIconsLocked(id EntityID, now time.Time) []uint32 {
	var out []uint32
	if p := w.Players[id]; p != nil {
		for _, l := range w.buffWindowSetLocked(p, now) {
			if l.Whose == BuffOnPlayer && len(out) < GroupEffectsMost {
				out = append(out, l.Icon)
			}
		}
		return out
	}
	if n := w.NPCs[id]; n != nil && !n.Dead {
		for _, e := range n.Effects {
			if len(out) == GroupEffectsMost {
				break
			}
			if e.Name != "" && now.Before(e.Until) {
				out = append(out, e.Icon)
			}
		}
	}
	return out
}

// groupEffectsLocked is the set me's client is sent: each other member its group window
// shows, by the id its roster names it, with its icons; none out of a group. Must be called
// with the lock held.
func (w *World) groupEffectsLocked(sess *session.Session, me *Player, now time.Time) []memberEffects {
	g := w.groups[me.GroupID]
	if me.GroupID == 0 || g == nil {
		return nil
	}
	var out []memberEffects
	for _, v := range w.shownViewsLocked(g, me) {
		if v.id == me.ServerID {
			continue
		}
		out = append(out, memberEffects{id: w.groupIDSeenByLocked(sess, v.id), icons: w.memberIconsLocked(v.id, now)})
	}
	return out
}

// encodeGroupEffects is OpcodeGroupEffectsData's body: a u8 count of members, then each
// member's u32 id, u8 count of icons and each u32 icon.
func encodeGroupEffects(set []memberEffects) []byte {
	if len(set) > 255 {
		set = set[:255]
	}
	out := []byte{byte(len(set))}
	for _, m := range set {
		icons := m.icons
		if len(icons) > GroupEffectsMost {
			icons = icons[:GroupEffectsMost]
		}
		out = binary.LittleEndian.AppendUint32(out, m.id)
		out = append(out, byte(len(icons)))
		for _, icon := range icons {
			out = binary.LittleEndian.AppendUint32(out, icon)
		}
	}
	return out
}

// groupEffectsSystem sends each client that asked its members' effects when they changed
// (the body carries no time left, so it is its own key), and at once after the ask. Every
// groupEffectsTicks, after groupSyncSystem and after effectSystem took off what ran out.
// Must be called with the lock held.
func (w *World) groupEffectsSystem(now time.Time) {
	if w.TickCount%groupEffectsTicks != 0 || w.SessionManager == nil {
		return
	}
	w.SessionManager.ForEach(func(s *session.Session) bool {
		p := w.sessionPlayerLocked(s)
		if p == nil || !p.GroupEffects {
			return true
		}
		body := encodeGroupEffects(w.groupEffectsLocked(s, p, now))
		if !p.groupEffectsDue && string(body) == p.groupEffectsSent {
			return true
		}
		p.groupEffectsSent, p.groupEffectsDue = string(body), false
		sendGroupMsg(s, protocol.OpcodeGroupEffectsData, func(b *buffer.Writer) { b.WriteBytes(body) })
		return true
	})
}
