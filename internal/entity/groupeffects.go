package entity

import (
	"encoding/binary"
	"fmt"
	"strings"
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
// effects that have not run out. Version 1 (a client that answered 1) is icons only;
// version 2 (one that answered 2 or more: petition 2962, Graga, 2026-11-25: "the buff
// icons of group members should reveal the buff when mouse hovering and have timers like
// the new buff timers") carries each icon's time left and name as well, the client
// counting the time down between sets. A member in another zone or out of sight has its
// icons as well: the server holds them. A PS2 never asks, and is sent none.

// GroupEffectsMost is the most icons a member carries: its own buff window's most.
const GroupEffectsMost = BuffWindowPlayerMost

// GroupEffectsNewest is the newest version of OpcodeGroupEffectsData the server sends,
// what it offers: 1 icons only, 2 each icon's time left and name too.
const GroupEffectsNewest = 2

// groupEffectsTicks is how often the system looks: half a second, as the power bars'.
const groupEffectsTicks = 5

// memberEffect is one effect on a member: its icon, its time left in milliseconds
// (buffNoEnd: no end the server keeps) and its name, as its buff window's line.
type memberEffect struct {
	icon   uint32
	leftMS uint32
	name   string
}

// memberEffects is one member's effects, by the id the receiving client knows it.
type memberEffects struct {
	id      uint32
	effects []memberEffect
}

// SetGroupEffects makes sess's player's client be sent its group members' effects in
// version 1 (on) or stop (off): SetGroupEffectsVersion's 1 or 0.
func (w *World) SetGroupEffects(sess *session.Session, on bool) bool {
	v := uint32(0)
	if on {
		v = 1
	}
	return w.SetGroupEffectsVersion(sess, v)
}

// SetGroupEffectsVersion makes sess's player's client be sent its group members' effects
// in version v (at most GroupEffectsNewest; a client asking for a newer one reads the
// newest the server sends, as it offered) or stop (0); asked, the set goes out at the next
// pass whatever was sent before. False when sess has no player.
func (w *World) SetGroupEffectsVersion(sess *session.Session, v uint32) bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	p := w.sessionPlayerLocked(sess)
	if p == nil {
		return false
	}
	on := v != 0
	p.GroupEffects, p.GroupEffectsV2 = on, v >= 2
	p.groupEffectsSent, p.groupEffectsDue = "", on
	return true
}

// memberEffectsLocked is the effects on group member id at now: a player's as its own
// buff window shows them (each line's time left and name with it), a merc's named effects
// that have not run out, at most GroupEffectsMost; none for anything else. Must be called
// with the lock held.
func (w *World) memberEffectsLocked(id EntityID, now time.Time) []memberEffect {
	var out []memberEffect
	if p := w.Players[id]; p != nil {
		for _, l := range w.buffWindowSetLocked(p, now) {
			if l.Whose == BuffOnPlayer && len(out) < GroupEffectsMost {
				out = append(out, memberEffect{icon: l.Icon, leftMS: l.LeftMS, name: l.Name})
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
				left := uint32(buffNoEnd)
				if e.Until.Sub(now) <= noEndShown { // a were form's: no end shown, as the buff window's
					left = uint32(max(0, min(e.Until.Sub(now).Milliseconds(), buffNoEnd-1)))
				}
				out = append(out, memberEffect{icon: e.Icon, leftMS: left, name: petEffectName(e.Name)})
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
		out = append(out, memberEffects{id: w.groupIDSeenByLocked(sess, v.id), effects: w.memberEffectsLocked(v.id, now)})
	}
	return out
}

// encodeGroupEffects is OpcodeGroupEffectsData's body: a u8 count of members, then each
// member's u32 id, u8 count of effects and each effect's u32 icon; in version 2 (v2) each
// icon is followed by its u32 milliseconds left (0: no end) and its name (a u8 byte count
// and UTF-8).
func encodeGroupEffects(set []memberEffects, v2 bool) []byte {
	if len(set) > 255 {
		set = set[:255]
	}
	out := []byte{byte(len(set))}
	for _, m := range set {
		fx := m.effects
		if len(fx) > GroupEffectsMost {
			fx = fx[:GroupEffectsMost]
		}
		out = binary.LittleEndian.AppendUint32(out, m.id)
		out = append(out, byte(len(fx)))
		for _, e := range fx {
			out = binary.LittleEndian.AppendUint32(out, e.icon)
			if !v2 {
				continue
			}
			left := e.leftMS
			switch {
			case left == buffNoEnd:
				left = 0
			case left == 0:
				left = 1 // an end, however near, is not "no end"
			}
			name := petEffectName(e.name)
			out = binary.LittleEndian.AppendUint32(out, left)
			out = append(out, byte(len(name)))
			out = append(out, name...)
		}
	}
	return out
}

// groupEffectsKey is what makes a set: in version 1 its body; in version 2 each member's
// id and each effect's icon, name and end, not its time left (less at every look), so a
// set goes out only when an effect goes on, runs out, is cast again or the group changes.
func groupEffectsKey(set []memberEffects, v2 bool, now time.Time) string {
	if !v2 {
		return string(encodeGroupEffects(set, false))
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%d\x00", len(set))
	for _, m := range set {
		fmt.Fprintf(&b, "%d\x01", m.id)
		for i, e := range m.effects {
			if i == GroupEffectsMost {
				break
			}
			end := int64(-1)
			if e.leftMS != buffNoEnd {
				// to the second: the same end read at two looks rounds alike
				end = (now.UnixMilli() + int64(e.leftMS) + 500) / 1000
			}
			fmt.Fprintf(&b, "%d|%d|%s\x00", e.icon, end, e.name)
		}
	}
	return b.String()
}

// groupEffectsSystem sends each client that asked its members' effects when they changed
// (groupEffectsKey: version 1's body carries no time left, so it is its own key), and at
// once after the ask. Every
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
		set := w.groupEffectsLocked(s, p, now)
		key := groupEffectsKey(set, p.GroupEffectsV2, now)
		if !p.groupEffectsDue && key == p.groupEffectsSent {
			return true
		}
		p.groupEffectsSent, p.groupEffectsDue = key, false
		body := encodeGroupEffects(set, p.GroupEffectsV2)
		sendGroupMsg(s, protocol.OpcodeGroupEffectsData, func(b *buffer.Writer) { b.WriteBytes(body) })
		return true
	})
}
