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
	v := uint32(0)
	if on {
		v = 1
	}
	return w.SetRaidRosterAnswer(sess, v)
}

// SetRaidRosterAnswer is a client's 0x7E19 answer v: 0 stop, RaidRosterWithGroups the
// roster with each record's group number (raidsubgroups.go), RaidRosterWithKinds that
// and each record's kind byte after it, any other the roster as before. False when
// sess has no player.
func (w *World) SetRaidRosterAnswer(sess *session.Session, v uint32) bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	p := w.sessionPlayerLocked(sess)
	if p == nil {
		return false
	}
	p.RaidRoster, p.raidRosterSent = v != 0, ""
	p.RaidRosterGroups, p.RaidRosterKinds = v == RaidRosterWithGroups || v == RaidRosterWithKinds, v == RaidRosterWithKinds
	return true
}

// RaidRosterWithKinds is the 0x7E18 offer's value and the 0x7E19 answer's for the
// roster with each record's group number and then its kind (raidRosterKindMerc for a
// mercenary, 0 a player) after it. Since 2026-11-12 (the owner: "im not able to target
// mercs in a raid group"): a raid's mercs are in its roster, so a client that draws the
// roster in place of the group's bars can target them; the kind keeps them out of its
// counts of each group's players (a merc takes no place in a raid's group,
// raidsubgroups.go). An older vanilla answers 2 to this offer and is sent the mercs'
// records with no kind.
const RaidRosterWithKinds = 3

// raidRosterKindMerc is a merc's kind byte (bit 0).
const raidRosterKindMerc = 1

// raidRosterRow is one record's fields: the id the receiving client knows it by.
type raidRosterRow struct {
	id               uint32
	name             string
	class, level, hp int
	power            byte
	sub              uint8
	merc, kinds      bool
}

// bytes is the row's record: sub, its group in the raid, after it when sub is not 0
// (a client that answered RaidRosterWithGroups), then its kind to a client that
// answered RaidRosterWithKinds.
func (r raidRosterRow) bytes() []byte {
	name := []byte(r.name)
	if len(name) > 23 {
		name = name[:23]
	}
	b := binary.LittleEndian.AppendUint32(make([]byte, 0, 11+len(name)), r.id)
	b = append(b, byte(len(name)))
	b = append(b, name...)
	b = append(b, byte(r.class), byte(max(0, min(255, r.level))), byte(max(0, min(255, r.hp))), r.power)
	if r.sub != 0 {
		b = append(b, r.sub)
	}
	if r.kinds {
		kind := byte(0)
		if r.merc {
			kind = raidRosterKindMerc
		}
		b = append(b, kind)
	}
	return b
}

// raidRosterRecord is one member's record for sess's client; sub, its group in the
// raid, after it when sub is not 0 (a client that answered RaidRosterWithGroups).
func (w *World) raidRosterRecord(sess *session.Session, o *Player, sub uint8) []byte {
	hp := 0
	if top := o.MaxHPNow(); top > 0 {
		hp = int(healthBar(o.HP, top))
	}
	power, _ := powerOf255(o)
	return raidRosterRow{id: w.idSeenByLocked(sess, o), name: o.Name, class: o.Class, level: o.Level, hp: hp, power: power, sub: sub}.bytes()
}

// raidRosterMercRecord is a living merc's record for sess's client, by the id that
// client knows it by (its channel's, else groupFarID's stand-in, as the group roster
// names it: mercgroup.go); sub, its owner's group in the raid. Must be called with the
// lock held.
func (w *World) raidRosterMercRecord(sess *session.Session, n *NPC, sub uint8, kinds bool) []byte {
	hp := 0
	if n.MaxHP > 0 {
		hp = int(healthBar(n.HP, n.MaxHP))
	}
	power, _ := w.barPowerLocked(n)
	return raidRosterRow{id: w.groupIDSeenByLocked(sess, n.ServerID), name: n.Name, class: n.Class, level: n.Level, hp: hp, power: power,
		sub: sub, merc: true, kinds: kinds}.bytes()
}

// raidRosterLocked is every member's record for sess's client, its mercs' too (each
// in its owner's group), in the order they joined; none when me's group is no raid's: one whose leader raised a raid flag,
// whatever its size, or past four (the user, 2026-11-08: "the raid ui doesnt show up
// for all players in raid, like maybe the early group still sees the group UI": a
// flagged group's first four saw their window of four until a fifth came). Must be
// called with the lock held.
func (w *World) raidRosterLocked(sess *session.Session, me *Player) [][]byte {
	g := w.groups[me.GroupID]
	if me.GroupID == 0 || g == nil || !(g.flagged || g.isRaid()) {
		return nil
	}
	var subs map[EntityID]uint8
	if me.RaidRosterGroups {
		subs = w.settleSubgroupsLocked(g)
	}
	out := make([][]byte, 0, len(g.Members))
	for _, id := range g.Members {
		if o := w.Players[id]; o != nil {
			rec := w.raidRosterRecord(sess, o, subs[id])
			if me.RaidRosterKinds {
				rec = append(rec, 0)
			}
			out = append(out, rec)
			continue
		}
		// a merc in the raid (its owner's group, mercgroup.go): in the roster too, in its
		// owner's group, so a client that draws the roster in place of the group's bars
		// can target it as it targets any member (the owner, 2026-11-12: "im not able to
		// target mercs in a raid group": before, a raid's mercs were in no window)
		if n := w.NPCs[id]; n.isMerc() && !n.Dead {
			out = append(out, w.raidRosterMercRecord(sess, n, subs[n.merc.owner.ServerID], me.RaidRosterKinds))
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
	// every raid's numbers settled, whoever shows them, so a member keeps its group
	// as others leave (raidsubgroups.go)
	for _, g := range w.groups {
		if raidingLocked(g) {
			w.settleSubgroupsLocked(g)
		}
	}
	w.SessionManager.ForEach(func(s *session.Session) bool {
		if p := w.sessionPlayerLocked(s); p != nil {
			w.sendRaidRosterLocked(s, p)
		}
		return true
	})
}

// sendRaidRosterLocked sends p's client its roster when it asked and it changed. Must
// be called with the lock held.
func (w *World) sendRaidRosterLocked(s *session.Session, p *Player) {
	if !p.RaidRoster {
		return
	}
	recs := w.raidRosterLocked(s, p)
	var all []byte
	for _, r := range recs {
		all = append(all, r...)
	}
	if string(all) == p.raidRosterSent {
		return
	}
	p.raidRosterSent = string(all)
	for _, page := range raidRosterPages(recs) {
		sendGroupMsg(s, protocol.OpcodeRaidRoster, func(b *buffer.Writer) { b.WriteBytes(page) })
	}
}

// raidRosterNowLocked sends g's members their rosters now (a move: raidsubgroups.go),
// not at the next pass. Must be called with the lock held.
func (w *World) raidRosterNowLocked(g *Group) {
	for _, id := range g.Members {
		if m := w.Players[id]; m != nil {
			if s := w.sessionOfLocked(m); s != nil {
				w.sendRaidRosterLocked(s, m)
			}
		}
	}
}
