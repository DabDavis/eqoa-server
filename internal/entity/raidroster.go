package entity

import (
	"encoding/binary"

	"github.com/eqoa/pkg/pkg/buffer"
	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// raidroster.go — the whole of a raid group, for a client that can show it (the user,
// 2026-11-07: "show names and hp in a way to accomodate"). CLIENT holds four
// (raid.go), and every client keeps that window, a PS2's the only one it has. The
// server offers more (0x7E18, handlers/group.go) as it offers others' power (0x7E01):
// a client that shows it answers 0x7E19 {1} and is sent 0x7E1A (protocol/opcodes.go
// has the format) when it changes and at most once a second, the whole set each time,
// in as many messages as it takes; 0 members when its group is no raid group.

// raidRosterTicks is how often the system looks: a second.
const raidRosterTicks = 10

// raidRosterPageBytes is the most records' bytes in one message, under a message's
// buffer (protocol.DefaultMessageSize) with its opcode and head.
const raidRosterPageBytes = 1200

// SetRaidRoster makes sess's player's client be sent the roster (on) or stop (off);
// on, it goes out at the next pass, whatever was sent before. False when sess has no
// player.
func (w *World) SetRaidRoster(sess *session.Session, on bool) bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	p := w.sessionPlayerLocked(sess)
	if p == nil {
		return false
	}
	p.RaidRoster, p.raidRosterSent = on, ""
	return true
}

// raidRosterRecord is one member's record for sess's client.
func (w *World) raidRosterRecord(sess *session.Session, o *Player) []byte {
	name := []byte(o.Name)
	if len(name) > 23 {
		name = name[:23]
	}
	hp := 0
	if top := o.MaxHPNow(); top > 0 {
		hp = max(0, min(255, int(healthBar(o.HP, top))))
	}
	power, _ := powerOf255(o)
	b := binary.LittleEndian.AppendUint32(make([]byte, 0, 9+len(name)), w.idSeenByLocked(sess, o))
	b = append(b, byte(len(name)))
	b = append(b, name...)
	return append(b, byte(o.Class), byte(max(0, min(255, o.Level))), byte(hp), power)
}

// raidRosterLocked is every member's record for sess's client, in the order they
// joined; none when me's group is no raid group. Must be called with the lock held.
func (w *World) raidRosterLocked(sess *session.Session, me *Player) [][]byte {
	g := w.groups[me.GroupID]
	if me.GroupID == 0 || g == nil || !g.isRaid() {
		return nil
	}
	out := make([][]byte, 0, len(g.Members))
	for _, id := range g.Members {
		if o := w.Players[id]; o != nil {
			out = append(out, w.raidRosterRecord(sess, o))
		}
	}
	return out
}

// raidRosterPages is the records as message bodies, each past its opcode: u16 members
// in all, u16 its first, u8 how many, the records. No records: one body saying 0.
func raidRosterPages(recs [][]byte) [][]byte {
	head := func(first, count int) []byte {
		b := binary.LittleEndian.AppendUint16(nil, uint16(len(recs)))
		b = binary.LittleEndian.AppendUint16(b, uint16(first))
		return append(b, byte(count))
	}
	var pages [][]byte
	first, size := 0, 0
	flush := func(end int) {
		page := head(first, end-first)
		for _, r := range recs[first:end] {
			page = append(page, r...)
		}
		pages = append(pages, page)
		first, size = end, 0
	}
	for i, r := range recs {
		if size+len(r) > raidRosterPageBytes || i-first == 255 {
			flush(i)
		}
		size += len(r)
	}
	if first < len(recs) || len(recs) == 0 {
		flush(len(recs))
	}
	return pages
}

// raidRosterSystem sends each client that asked its roster when it changed. Must be
// called with the lock held.
func (w *World) raidRosterSystem() {
	if w.TickCount%raidRosterTicks != 0 || w.SessionManager == nil {
		return
	}
	w.SessionManager.ForEach(func(s *session.Session) bool {
		p := w.sessionPlayerLocked(s)
		if p == nil || !p.RaidRoster {
			return true
		}
		recs := w.raidRosterLocked(s, p)
		var all []byte
		for _, r := range recs {
			all = append(all, r...)
		}
		if string(all) == p.raidRosterSent {
			return true
		}
		p.raidRosterSent = string(all)
		for _, page := range raidRosterPages(recs) {
			sendGroupMsg(s, protocol.OpcodeRaidRoster, func(b *buffer.Writer) { b.WriteBytes(page) })
		}
		return true
	})
}
