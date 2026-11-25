package entity

import (
	"encoding/binary"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/eqoa/pkg/pkg/buffer"
	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// npcdebuffs.go — the harmful effects a player has on the creatures its client draws, for
// the small icons a client draws under their name plates (Heirloom petition 2997,
// Cucumber, 2026-11-25: "Debuffs cast on evil NPCs (combat) give an icon under their ID
// plate with remaining time on the debuff (so you know when to refresh) or at the very
// least the icon so you know when the debuff is no longer active").
//
// CLIENT's name plate (VINameSprite) is a name alone, and no message of the game's carries
// a creature's effects. A client that draws them (eqoa-rust-client-vanilla's Debuff Icons)
// asks as for its group's effects (groupeffects.go): the server offers OpcodeNPCDebuffs at
// world entry (handlers/npcdebuffs.go), the client answers 1, and from then on, every
// npcDebuffsTicks after the entity channels were placed, it is sent the set
// (OpcodeNPCDebuffsData) when it changed: each creature on the client's slots (by the id
// its channel carries) with a harmful effect (harmfulEffect) the player put on it, its pet
// put on it (Caster its owner) or one of its mercs did (mercHiredByLocked), each with its
// icon, whose it is, its time left and its name. Others' effects are not the player's to
// refresh and are left out. Like the group's version 2 the change key is each effect's
// icon, name and end to the second, not its time left, so a set goes out only when an
// effect goes on, is cast again, runs out or is taken off, or a creature comes onto or
// leaves the client's slots; the client counts the time down between sets. A PS2 never
// asks, and is sent none.

// NPCDebuffsMost is the most effects a creature carries in a set.
const NPCDebuffsMost = 12

// npcDebuffsBudget is the most bytes of a set's body: creatures past it (the farthest) are
// left out, so a set always fits one message (protocol.DefaultMessageSize, the opcode and
// the message's own header besides).
const npcDebuffsBudget = 1200

// npcDebuffsTicks is how often the system looks: half a second, as the group's effects'.
const npcDebuffsTicks = 5

// Whose an effect on a creature is, as a set carries it.
const (
	npcDebuffMine byte = 0 // the player's own
	npcDebuffPet  byte = 1 // its pet's (Caster its owner, pet set)
	npcDebuffMerc byte = 2 // one of its mercs'
)

// npcDebuff is one of the player's effects on a creature.
type npcDebuff struct {
	icon   uint32
	whose  byte
	leftMS uint32 // buffNoEnd: no end the server keeps
	name   string
}

// npcDebuffs is one creature's: the id the client's channel carries, its own id (for the
// change key: a channel id may be another creature's later) and its effects.
type npcDebuffs struct {
	id      uint32
	npc     EntityID
	dist2   float32
	effects []npcDebuff
}

// SetNPCDebuffs makes sess's player's client be sent the effects it has on the creatures it
// draws (on) or stop (off); on, the set goes out at the next pass whatever was sent before.
// False when sess has no player.
func (w *World) SetNPCDebuffs(sess *session.Session, on bool) bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	p := w.sessionPlayerLocked(sess)
	if p == nil {
		return false
	}
	p.NPCDebuffs = on
	p.npcDebuffsSent, p.npcDebuffsDue = "", on
	return true
}

// npcDebuffWhoseLocked is whose of p's an effect on a creature is, false when it is none of
// p's: put on by p itself, by its pet (Caster p, pet set) or by one of p's mercs. Must be
// called with the lock held.
func (w *World) npcDebuffWhoseLocked(p *Player, e *ActiveEffect) (byte, bool) {
	switch {
	case e.merc != 0:
		return npcDebuffMerc, w.mercHiredByLocked(e.merc, p.ServerID)
	case e.Caster != p.ServerID:
		return 0, false
	case e.pet != 0:
		return npcDebuffPet, true
	}
	return npcDebuffMine, true
}

// npcDebuffsOnLocked is p's harmful effects on n at now that have a name and have not run
// out, in the order they went on, at most NPCDebuffsMost. Must be called with the lock held.
func (w *World) npcDebuffsOnLocked(p *Player, n *NPC, now time.Time) []npcDebuff {
	var out []npcDebuff
	for _, e := range n.Effects {
		if len(out) == NPCDebuffsMost {
			break
		}
		if e == nil || e.Name == "" || !now.Before(e.Until) || !harmfulEffect(e) {
			continue
		}
		whose, ok := w.npcDebuffWhoseLocked(p, e)
		if !ok {
			continue
		}
		left := uint32(buffNoEnd)
		if d := e.Until.Sub(now); d <= noEndShown {
			left = uint32(max(0, min(d.Milliseconds(), buffNoEnd-1)))
		}
		out = append(out, npcDebuff{icon: e.Icon, whose: whose, leftMS: left, name: petEffectName(e.Name)})
	}
	return out
}

// npcDebuffsLocked is the set me's client is sent: each living creature with an effect on
// it (w.affected) that its client has on a slot and that carries one of me's harmful
// effects; its target first, then the nearest. Must be called with the lock held.
func (w *World) npcDebuffsLocked(sess *session.Session, me *Player, now time.Time) []npcDebuffs {
	var out []npcDebuffs
	for id, n := range w.affected {
		if n == nil || n.Dead || len(n.Effects) == 0 || w.NPCs[id] != n {
			continue
		}
		fx := w.npcDebuffsOnLocked(me, n, now)
		if len(fx) == 0 {
			continue
		}
		oid := w.objectIDForEntityLocked(sess, id)
		if oid == 0 {
			continue // not on the client's slots: it draws no plate for it
		}
		dx, dz := n.X-me.X, n.Z-me.Z
		out = append(out, npcDebuffs{id: oid, npc: id, dist2: dx*dx + dz*dz, effects: fx})
	}
	sort.Slice(out, func(i, j int) bool {
		ti, tj := out[i].id == me.TargetObjectID, out[j].id == me.TargetObjectID
		if ti != tj {
			return ti
		}
		if out[i].dist2 != out[j].dist2 {
			return out[i].dist2 < out[j].dist2
		}
		return out[i].npc < out[j].npc
	})
	return out
}

// npcDebuffsSize is the bytes a creature takes in a set's body.
func npcDebuffsSize(c npcDebuffs) int {
	n := 5
	for _, e := range c.effects {
		n += 10 + len(petEffectName(e.name))
	}
	return n
}

// fitNPCDebuffs is set cut to what fits a message: as many creatures, in order, as fit
// npcDebuffsBudget, at most 255.
func fitNPCDebuffs(set []npcDebuffs) []npcDebuffs {
	size := 1
	for i, c := range set {
		if i == 255 {
			return set[:i]
		}
		if len(c.effects) > NPCDebuffsMost {
			set[i].effects = c.effects[:NPCDebuffsMost]
		}
		size += npcDebuffsSize(set[i])
		if size > npcDebuffsBudget {
			return set[:i]
		}
	}
	return set
}

// encodeNPCDebuffs is OpcodeNPCDebuffsData's body: a u8 count of creatures, then each
// one's u32 id, u8 count of effects, and per effect its u32 icon, u8 whose, u32
// milliseconds left (0: no end) and name (a u8 byte count and UTF-8). set is fitted
// (fitNPCDebuffs) first.
func encodeNPCDebuffs(set []npcDebuffs) []byte {
	set = fitNPCDebuffs(set)
	out := []byte{byte(len(set))}
	for _, c := range set {
		out = binary.LittleEndian.AppendUint32(out, c.id)
		out = append(out, byte(len(c.effects)))
		for _, e := range c.effects {
			left := e.leftMS
			switch {
			case left == buffNoEnd:
				left = 0
			case left == 0:
				left = 1 // an end, however near, is not "no end"
			}
			name := petEffectName(e.name)
			out = binary.LittleEndian.AppendUint32(out, e.icon)
			out = append(out, e.whose)
			out = binary.LittleEndian.AppendUint32(out, left)
			out = append(out, byte(len(name)))
			out = append(out, name...)
		}
	}
	return out
}

// npcDebuffsKey is what makes a set: each creature's ids and each effect's icon, whose,
// name and end to the second, not its time left (less at every look).
func npcDebuffsKey(set []npcDebuffs, now time.Time) string {
	set = fitNPCDebuffs(set)
	var b strings.Builder
	fmt.Fprintf(&b, "%d\x00", len(set))
	for _, c := range set {
		fmt.Fprintf(&b, "%d/%d\x01", c.id, c.npc)
		for _, e := range c.effects {
			end := int64(-1)
			if e.leftMS != buffNoEnd {
				// to the second: the same end read at two looks rounds alike
				end = (now.UnixMilli() + int64(e.leftMS) + 500) / 1000
			}
			fmt.Fprintf(&b, "%d|%d|%d|%s\x00", e.icon, e.whose, end, e.name)
		}
	}
	return b.String()
}

// npcDebuffsSystem sends each client that asked the effects it has on the creatures it
// draws when they changed (npcDebuffsKey), and at once after the ask. Every
// npcDebuffsTicks, after the visibility system placed the slots and effectSystem took off
// what ran out. Must be called with the lock held.
func (w *World) npcDebuffsSystem(now time.Time) {
	if w.TickCount%npcDebuffsTicks != 0 || w.SessionManager == nil {
		return
	}
	w.SessionManager.ForEach(func(s *session.Session) bool {
		p := w.sessionPlayerLocked(s)
		if p == nil || !p.NPCDebuffs {
			return true
		}
		set := fitNPCDebuffs(w.npcDebuffsLocked(s, p, now))
		key := npcDebuffsKey(set, now)
		if !p.npcDebuffsDue && key == p.npcDebuffsSent {
			return true
		}
		p.npcDebuffsSent, p.npcDebuffsDue = key, false
		body := encodeNPCDebuffs(set)
		sendGroupMsg(s, protocol.OpcodeNPCDebuffsData, func(b *buffer.Writer) { b.WriteBytes(body) })
		return true
	})
}
