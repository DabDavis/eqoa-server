package entity

import (
	"encoding/binary"
	"log"
	"sort"
	"time"
	"unicode/utf8"

	"github.com/eqoa/pkg/pkg/buffer"
	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// dpsmeter.go — a damage meter (the owner, 2026-11-10: "lets add dps meters,
// will work solo, group, raid"): who in a player's group did how much damage in
// the fight under way, and how fast, for a client that draws one
// (eqoa-rust-client-vanilla's DPS Meter; claude-notes/dps-meter-20261110.md).
// CLIENT has no meter, so it is offered as the number tags are
// (handlers/dpsmeter.go): OpcodeDPSMeterOffer once the player is in the world, a
// client that draws one answers OpcodeDPSMeterAnswer 1, and from then on it is
// sent OpcodeDPSMeterData (protocol/opcodes.go has the layout). A PS2 ignores the
// offer and is sent nothing; its player's damage still counts on the meters of
// its group's others.
//
// What counts is what the player is told it did, at the figure told: each 0x00DB
// of damage the server sends a player of its own or its pet's making
// (sendDamageIndication: a negative amount of a tagged numberKind, numbertags.go's
// table: a swing or a bow's shot, a spell's or an ability's hit, an area's
// others, a thrown potion, a damage over time's tick, a damage shield's answer, a
// proc's hit; its pet's the same, charmed or summoned), counted once, for the
// player the figure is told to, the pet's share kept apart within it. So a
// training dummy's hits count (the place a meter is wanted most), and so does a
// duel's or a PvP fight's damage on another player (halved as told). Heals do
// not, nor a figure on the player itself, nor one on its own pet (an alchemist's
// Destroy Contraption: friendlyFigureLocked), nor any figure another made (a
// creature's, the sun's, a price paid: all untagged). One exception is counted
// with no figure: the necromancer's zombie, mummy and spectre fight untold, as
// Sandstorm's did (petrotation.go), and their hits are counted where the figure
// would be told (petLandHitLocked), as the pet's share, or a necromancer's meter
// would leave its pet out.
//
// The scope is the group as the server holds it at each figure, and the fight
// is the group's (the owner, 2026-11-11: "drunkenfist came in the raid late and
// the dps meter was not the same as other players. we should sync it up per
// group and raid"): one fight per group, a raid group's too (Group.dps), and
// alone the player's own (Player.dpsSolo). A figure counts on its maker's group's
// fight (counted whatever its members' clients draw), and every member's meter
// is shown that same fight: the same number, length, lines and whole, each
// receiver's own line flagged. So a member who joins mid-fight is shown the
// group's fight under way at once, every line from its first figure; one who
// leaves counts no more on it, its line kept, marked, until the fight ends, and
// its own meter's view of it ends there (one last message, over: its client keeps
// it as the last fight) and goes on to its own fight (alone, or its new group's)
// at its next figure. A group formed or disbanded mid-fight is the same: its
// members' views of the fights they were shown end, and the new scope's fight
// starts at its next figure. A fight starts at the first figure counted on it and
// ends DPSMeterIdle after the last, longer than anything that keeps a fight going
// leaves quiet (a damage over time's six-second tick, effectTick; a round of six
// seconds; a long cast), or, while any member of the group (or its pet) is still
// on a living creature's hate list (dpsEngagedLocked), DPSMeterHeld after it: a
// boss fight's quiet stretch is the same fight. A reset is the client's own: it
// drops the fight under way from its view only (nothing more is sent of it to
// that client; the group's others go on being shown it) and it is shown the
// group's next fight; alone, the fight is its own and ends there.
// Its length is from its first figure to its latest, so a fight's quiet end adds
// nothing, and the client divides each line's damage by it (at least a second):
// every line over the same span, so DPS ranks as damage does and a healer's one
// strike is no burst. Every member of the group in the world has a line, of no
// damage too; at most DPSMeterLinesMost are sent, the most damage first, the
// receiver's own always among them, with the count of all and their whole.
// Nothing is kept over a restart but the client's answer (keep.go): a fight on
// then ends there, and the next figure starts another.

// DPSMeterLinesMost is the most lines a message carries: a raid's top, the
// receiver's own taking the last place when it is not among them. With names of
// at most dpsNameBytes the body stays under a message (protocol.DefaultMessageSize).
const DPSMeterLinesMost = 24

// DPSMeterIdle is how long a fight goes on with no damage in its group before it
// ends: past a damage over time's tick (effectTick, six seconds), a round of six
// seconds and a long cast with room to spare, and well short of the next pull.
const DPSMeterIdle = 10 * time.Second

// DPSMeterHeld is how long a fight goes on with no damage in its group while the
// group is still in it: a living creature has one of its members (or a member's
// pet) on its hate list (groupsInCombatLocked; alone, the player or its pet). A
// boss fight's quiet stretch (the caster dead, the tanks only taunting: Lord
// Nagafen, 2026-11-11, 11 s and 16 s of no damage while he fought on) is the same
// fight; a creature holding hate on a group that left it does not keep the fight
// open past this.
const DPSMeterHeld = 60 * time.Second

// dpsMeterEvery is the least time between two messages of a fight under way: a
// raid's meter changes at every hit, and its client is told a second's worth at a
// time. A fight's end is told at once.
const dpsMeterEvery = time.Second

// dpsMeterTicks is how often the system looks: half a second.
const dpsMeterTicks = 5

// dpsMeterLayout is OpcodeDPSMeterData's layout, the one the offer names.
const dpsMeterLayout = 1

// dpsNameBytes is the most bytes of a name sent, as a roster's (writeName8).
const dpsNameBytes = 23

// dpsMeterNow is the meter's clock, a test's own.
var dpsMeterNow = time.Now

// dpsLine is one maker's damage in a fight: its name and class as they were
// when it first counted (it may leave the world before the fight ends), its
// damage and its pet's share of it.
type dpsLine struct {
	name   string
	class  int
	damage int64
	pet    int64
}

// dpsFight is one fight of a group (a raid group's too) or of a player alone:
// its number, under way (live) from its first figure to its latest, and each
// maker's line. group: the group it is of, when it is one's (it may have been
// disbanded since); solo: else the player.
type dpsFight struct {
	number      uint32
	live        bool
	start, last time.Time
	lines       map[EntityID]*dpsLine
	group       *Group
	solo        *Player
}

// dpsMeter is a client's meter: the fight it is shown (view, its player's
// group's or its own), the number of that fight told and not yet told over
// (shown), the number of a fight the client reset (dropped: nothing more of it is
// sent), and what it was last sent of its view (due: send at the next pass, just
// asked).
type dpsMeter struct {
	view    *dpsFight
	shown   uint32
	dropped uint32
	sent    string
	sentAt  time.Time
	due     bool
}

// SetDPSMeter makes sess's player's client be sent the meter (on) or stop (off,
// its fight forgotten); on, a fight under way goes out at the next pass. False
// when sess has no player.
func (w *World) SetDPSMeter(sess *session.Session, on bool) bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	p := w.sessionPlayerLocked(sess)
	if p == nil {
		return false
	}
	switch {
	case !on:
		p.dps = nil
	case p.dps == nil:
		p.dps = &dpsMeter{}
	default:
		p.dps.due = true
	}
	return true
}

// DPSMeter is whether sess's player's client asked for a damage meter.
func (w *World) DPSMeter(sess *session.Session) bool {
	w.mu.RLock()
	defer w.mu.RUnlock()
	p := w.sessionPlayerLocked(sess)
	return p != nil && p.dps != nil
}

// ResetDPSMeter drops the fight under way from sess's player's meter: nothing
// more is sent of it to that client, and it is shown its group's next fight. The
// group's other members' meters go on with it; a player alone's fight is its own
// and ends there, so its next figure starts another under another number. False
// when sess has no player or its client never asked for a meter.
func (w *World) ResetDPSMeter(sess *session.Session) bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	p := w.sessionPlayerLocked(sess)
	if p == nil || p.dps == nil {
		return false
	}
	m := p.dps
	if f := *w.dpsScopeLocked(p); f != nil && f.live {
		if f.group == nil {
			f.live, f.lines = false, nil // the system drops it from dpsLive
		} else {
			m.dropped = f.number
		}
	}
	m.shown, m.sent, m.due = 0, "", false
	return true
}

// dpsScopeLocked is where p's fight is kept: its group's (a raid group's too)
// when it is in one, else its own. Must be called with the lock held.
func (w *World) dpsScopeLocked(p *Player) **dpsFight {
	if g := w.groups[p.GroupID]; p.GroupID != 0 && g != nil {
		return &g.dps
	}
	return &p.dpsSolo
}

// dpsMembersLocked is who a fight is of now, in the world or not: its group's
// members while the group stands, its player alone's, and the makers of its lines
// for a group disbanded since. Must be called with the lock held.
func (w *World) dpsMembersLocked(f *dpsFight) []EntityID {
	switch {
	case f.group != nil && w.groups[f.group.ID] == f.group:
		return f.group.Members
	case f.group == nil && f.solo != nil:
		return []EntityID{f.solo.ServerID}
	}
	out := make([]EntityID, 0, len(f.lines))
	for id := range f.lines {
		out = append(out, id)
	}
	return out
}

// dpsFigureLocked counts n of damage told to sess's player as kind's
// (sendDamageIndication) on target, the id sess's client knows it by: one that
// hurts something of the player's own (friendlyFigureLocked) does not count.
// Must be called with the lock held.
func (w *World) dpsFigureLocked(sess *session.Session, n int, kind numberKind, target uint32) {
	p := w.sessionPlayerLocked(sess)
	if p == nil || n <= 0 || w.friendlyFigureLocked(sess, p, target) {
		return
	}
	w.dpsCountLocked(p, n, kind.byHelper(), dpsMeterNow())
}

// friendlyFigureLocked is whether a figure on target (the id sess's client knows
// it by) hurts the player itself or its own pet: the alchemist's Destroy
// Contraption, -20000 on its own contraption (landOnPetLocked), is no damage a
// meter counts. Must be called with the lock held.
func (w *World) friendlyFigureLocked(sess *session.Session, p *Player, target uint32) bool {
	if target == sess.PlayerObjectID() {
		return true
	}
	vs, ok := sess.VisibilityState().(*VisibilityState)
	if !ok || vs == nil {
		return false
	}
	for i, id := range vs.ChannelObjectID {
		if id == target && i < len(vs.ChannelEntity) {
			npc := w.NPCs[vs.ChannelEntity[i]]
			return npc != nil && npc.Owner != 0 && npc.Owner == p.ServerID
		}
	}
	return false
}

// dpsCountLocked counts n of maker's damage (byPet: its pet's) at now on the
// fight of maker's group as it is at this figure (alone, maker's own): one with
// no fight under way starts one, under a number no other fight of this process
// had. Every member's meter is shown that fight. Must be called with the lock
// held.
func (w *World) dpsCountLocked(maker *Player, n int, byPet bool, now time.Time) {
	if maker == nil || n <= 0 {
		return
	}
	at := w.dpsScopeLocked(maker)
	f := *at
	if f == nil || !f.live {
		if w.dpsFight == 0 {
			// numbers from the clock's milliseconds: a restarted process's
			// differ from the last one's, so a client keeping a fight from
			// before a restart takes the first after it for another
			w.dpsFight = uint32(time.Now().UnixMilli())
		}
		w.dpsFight++
		if w.dpsFight == 0 {
			w.dpsFight++
		}
		f = &dpsFight{number: w.dpsFight, live: true, start: now, last: now, lines: map[EntityID]*dpsLine{}}
		if g := w.groups[maker.GroupID]; maker.GroupID != 0 && g != nil {
			f.group = g
		} else {
			f.solo = maker
		}
		*at = f
		w.dpsLive = append(w.dpsLive, f)
	}
	l := f.lines[maker.ServerID]
	if l == nil {
		l = &dpsLine{name: maker.Name, class: maker.Class}
		f.lines[maker.ServerID] = l
	}
	l.damage += int64(n)
	if byPet {
		l.pet += int64(n)
	}
	if now.After(f.last) {
		f.last = now
	}
}

// dpsRow is one line of a message: the maker, its line and its flags.
type dpsRow struct {
	id    EntityID
	line  dpsLine
	flags byte
}

// dpsRowsLocked is every line of fight f as p's meter is sent it, in the order
// sent (the most damage first, then by name): each maker counted, and each member
// of f's group in the world, none of damage too; p's own flagged 1, one no longer
// in f's group flagged 2. Every member is sent the same lines but for flag 1.
// Must be called with the lock held.
func (w *World) dpsRowsLocked(p *Player, m *dpsFight) []dpsRow {
	scope := map[EntityID]bool{}
	for _, id := range w.dpsMembersLocked(m) {
		if w.Players[id] != nil || id == p.ServerID {
			scope[id] = true
		}
	}
	rows := make([]dpsRow, 0, len(m.lines)+len(scope))
	for id, l := range m.lines {
		rows = append(rows, dpsRow{id: id, line: *l})
	}
	for id := range scope {
		if m.lines[id] == nil {
			o := w.Players[id]
			if id == p.ServerID {
				o = p
			}
			if o == nil {
				continue
			}
			rows = append(rows, dpsRow{id: id, line: dpsLine{name: o.Name, class: o.Class}})
		}
	}
	for i := range rows {
		if rows[i].id == p.ServerID {
			rows[i].flags |= 1
		}
		if !scope[rows[i].id] {
			rows[i].flags |= 2
		}
	}
	sort.Slice(rows, func(i, j int) bool {
		a, b := rows[i], rows[j]
		if a.line.damage != b.line.damage {
			return a.line.damage > b.line.damage
		}
		if a.line.name != b.line.name {
			return a.line.name < b.line.name
		}
		return a.id < b.id
	})
	return rows
}

// dpsName is a name as a line carries it: at most dpsNameBytes bytes, cut on a
// character's edge.
func dpsName(s string) string {
	end := 0
	for end < len(s) {
		_, size := utf8.DecodeRuneInString(s[end:])
		if end+size > dpsNameBytes {
			break
		}
		end += size
	}
	return s[:end]
}

// encodeDPSMeter is OpcodeDPSMeterData's body (past its opcode) for a fight of
// number fight, over or not, length long, of rows in their order: the first
// DPSMeterLinesMost, the receiver's own (flag 1) in the last place when it is
// not among them, its place among all of them said (0 for none).
func encodeDPSMeter(fight uint32, over bool, length time.Duration, rows []dpsRow) []byte {
	var total int64
	mine := -1
	for i, r := range rows {
		total += r.line.damage
		if r.flags&1 != 0 && mine < 0 {
			mine = i
		}
	}
	shown := rows
	if len(shown) > DPSMeterLinesMost {
		shown = append([]dpsRow(nil), rows[:DPSMeterLinesMost]...)
		if mine >= DPSMeterLinesMost {
			shown[DPSMeterLinesMost-1] = rows[mine]
		}
	}
	clamp := func(v int64) uint32 { return uint32(max(0, min(v, 0xFFFFFFFF))) }
	state := byte(0)
	if over {
		state = 1
	}
	out := []byte{dpsMeterLayout}
	out = binary.LittleEndian.AppendUint32(out, fight)
	out = append(out, state)
	out = binary.LittleEndian.AppendUint32(out, clamp(length.Milliseconds()))
	out = binary.LittleEndian.AppendUint32(out, clamp(total))
	out = binary.LittleEndian.AppendUint16(out, uint16(min(len(rows), 0xFFFF)))
	out = binary.LittleEndian.AppendUint16(out, uint16(min(mine+1, 0xFFFF)))
	out = append(out, byte(len(shown)))
	for _, r := range shown {
		name := dpsName(r.line.name)
		out = append(out, r.flags, byte(max(0, min(r.line.class, 255))))
		out = binary.LittleEndian.AppendUint32(out, clamp(r.line.damage))
		out = binary.LittleEndian.AppendUint32(out, clamp(r.line.pet))
		out = append(out, byte(len(name)))
		out = append(out, name...)
	}
	return out
}

// dpsMeterSystem ends the fights gone quiet, DPSMeterIdle after the last figure
// (DPSMeterHeld while one of its members is engaged), and tells each client with
// a meter its group's fight: as it goes, when something in it changed and at
// most every dpsMeterEvery (at once after the ask or a change of group), and once
// more as it ends; a client whose player left the fight's group is told its view
// is over. Every dpsMeterTicks; must be called with the lock held.
func (w *World) dpsMeterSystem(now time.Time) {
	if w.TickCount%dpsMeterTicks != 0 || w.SessionManager == nil {
		return
	}
	var engaged map[EntityID]bool // dpsEngagedLocked, once a pass and only if asked
	ending := map[*dpsFight]bool{}
	live := w.dpsLive[:0]
	for _, f := range w.dpsLive {
		if !f.live {
			continue
		}
		quiet := now.Sub(f.last)
		over := quiet >= DPSMeterIdle
		if over && quiet < DPSMeterHeld {
			if engaged == nil {
				engaged = w.dpsEngagedLocked()
			}
			over = true
			for _, id := range w.dpsMembersLocked(f) {
				if engaged[id] {
					over = false
					break
				}
			}
		}
		if over {
			ending[f] = true
		} else {
			live = append(live, f)
		}
	}
	clear(w.dpsLive[len(live):])
	w.dpsLive = live
	send := func(s *session.Session, m *dpsMeter, body []byte) {
		m.sent, m.sentAt, m.due = string(body), now, false
		sendGroupMsg(s, protocol.OpcodeDPSMeterData, func(b *buffer.Writer) { b.WriteBytes(body) })
	}
	w.SessionManager.ForEach(func(s *session.Session) bool {
		p := w.sessionPlayerLocked(s)
		if p == nil || p.dps == nil {
			return true
		}
		m := p.dps
		f := *w.dpsScopeLocked(p)
		if m.view != f {
			// another group, or none: the view of the last fight ends, told
			// once more as it stands, over, when it was being told
			if old := m.view; old != nil && old.live && m.shown == old.number {
				send(s, m, encodeDPSMeter(old.number, true, old.last.Sub(old.start), w.dpsRowsLocked(p, old)))
			}
			m.view, m.shown, m.sent, m.due = f, 0, "", true
		}
		if f == nil || !f.live || f.number == m.dropped {
			return true
		}
		over := ending[f]
		body := encodeDPSMeter(f.number, over, f.last.Sub(f.start), w.dpsRowsLocked(p, f))
		switch {
		case over:
			log.Printf("DPSMeter: %s's fight %d over (%d ms)", p.Name, f.number, f.last.Sub(f.start).Milliseconds())
		case m.due:
		case string(body) == m.sent || now.Sub(m.sentAt) < dpsMeterEvery:
			return true
		}
		send(s, m, body)
		m.shown = f.number
		if over {
			m.shown = 0
		}
		return true
	})
	for f := range ending {
		f.live, f.lines = false, nil
	}
}

// dpsEngagedLocked is every player still in a fight now: one on a living
// creature's hate list, or whose pet is, and every member of a group that is
// (groupsInCombatLocked's rule). Must be called with the lock held.
func (w *World) dpsEngagedLocked() map[EntityID]bool {
	w.syncNPCSetsLocked()
	out := map[EntityID]bool{}
	groups := map[uint32]bool{}
	for id, npc := range w.sets.ai.m {
		if npc = w.setMember(w.sets.ai.m, id, npc); npc == nil || npc.Dead || len(npc.Threats) == 0 {
			continue
		}
		for t := range npc.Threats {
			pid := t
			if n := w.NPCs[t]; n != nil && n.Owner != 0 {
				pid = n.Owner
			}
			if p := w.Players[pid]; p != nil {
				out[pid] = true
				if p.GroupID != 0 {
					groups[p.GroupID] = true
				}
			}
		}
	}
	for gid := range groups {
		if g := w.groups[gid]; g != nil {
			for _, id := range g.Members {
				out[id] = true
			}
		}
	}
	return out
}
