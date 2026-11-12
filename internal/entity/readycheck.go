package entity

import (
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/eqoa/pkg/pkg/buffer"
	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// readycheck.go — /readycheck (the GM Jibrish's petition 1711, 2026-11-12): "Add
// /readycheck that only raid leaders or co raid leaders can use, or anyone in a regular 4
// man non raid group. This pops up a window on their screen with "Are you ready?" and Yes
// or No. It does not stop them from moving while the pop up is up. It expires after 30
// seconds. If the player hits no, or it expires, it reports back to the command issuer who
// responded no or did not respond. If they hit yes that name does not go back to the
// issuer. If everyone hits ready, it informs the issuer "Everyone is ready". If a few hit
// no, most hit yes, it only reports "Raid not ready" and lists the names of who is not
// ready."
//
//   - Who may ask: in a raid (raidingLocked: flagged, or past four) its leader or its
//     helper (the co-leader, raidrules.go); in a plain group any member. One check at a
//     time a group; the issuer is not asked (it asked).
//   - Who is asked: every PLAYER member but the issuer (a merc is an NPC member,
//     mercgroup.go: never asked, never named).
//   - How: a client that answered the offer (protocol.OpcodeReadyCheckOffer..) gets its
//     window (OpcodeReadyCheck: no mode of the game's, so nothing stops its walk); every
//     other (a PS2: CLIENT's only server boxes, 0x0034 and the invitation's, put the game
//     window in a mode that stops the walk, the decomp's mode_locks) is told in a line to
//     answer with /ready or /notready, "/t ready" on a PS2 (handlers/raidcmds.go's
//     spellings), which any client may type.
//   - The end: everyone asked answered, or ReadyCheckSeconds passed: the issuer is told
//     "Everyone is ready", else "Raid not ready" (or "Group not ready") and who said no
//     and who did not answer; the windows still open are closed. A member gone from the
//     group by then is neither; the issuer gone from it ends the check unsaid.

// ReadyCheckSeconds is how long a ready check waits (the petition's 30 seconds).
const ReadyCheckSeconds = 30

// The answers a ready check keeps.
const (
	readyNone int8 = iota // not answered (yet)
	readyYes
	readyNo
)

// readyCheck is one group's check.
type readyCheck struct {
	id      uint32
	issuer  EntityID
	raid    bool
	expires time.Time
	asked   []EntityID // in the group's order
	answers map[EntityID]int8
}

// readyChecks is the World's checks by group id, and the last check id given.
type readyChecks struct {
	byGroup map[uint32]*readyCheck
	lastID  uint32
}

// SetReadyWindows is sess's client's answer to the ready check's offer
// (protocol.OpcodeReadyCheckAnswer): on, it is sent the check's window; off, the line.
// False when sess has no player.
func (w *World) SetReadyWindows(sess *session.Session, on bool) bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	p := w.sessionPlayerLocked(sess)
	if p == nil {
		return false
	}
	p.ReadyWindows = on
	return true
}

// ReadyCheck is /readycheck from sess's player (the file's doc).
func (w *World) ReadyCheck(sess *session.Session) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.readyCheckLocked(sess, time.Now())
}

func (w *World) readyCheckLocked(sess *session.Session, now time.Time) {
	p := w.sessionPlayerLocked(sess)
	if p == nil {
		return
	}
	g := w.groups[p.GroupID]
	if p.GroupID == 0 || g == nil {
		w.refuseLocked(sess, "You are not in a group.")
		return
	}
	raid := raidingLocked(g)
	if raid && w.raidCommandLocked(sess, p, true, "start a ready check") == nil {
		return
	}
	if c := w.ready.byGroup[g.ID]; c != nil {
		left := int(c.expires.Sub(now).Seconds() + 0.999)
		w.refuseLocked(sess, fmt.Sprintf("A ready check is already under way (%d seconds left).", max(left, 1)))
		return
	}
	c := &readyCheck{issuer: p.ServerID, raid: raid, expires: now.Add(ReadyCheckSeconds * time.Second), answers: map[EntityID]int8{}}
	for _, id := range g.Members {
		if m := w.Players[id]; m != nil && m != p {
			c.asked = append(c.asked, id)
		}
	}
	if len(c.asked) == 0 {
		w.refuseLocked(sess, "There is no one else in your group to ask.")
		return
	}
	w.ready.lastID++
	if w.ready.lastID == 0 {
		w.ready.lastID = 1
	}
	c.id = w.ready.lastID
	if w.ready.byGroup == nil {
		w.ready.byGroup = map[uint32]*readyCheck{}
	}
	w.ready.byGroup[g.ID] = c
	for _, id := range c.asked {
		m := w.Players[id]
		ms := w.sessionOfLocked(m)
		if ms == nil {
			continue
		}
		if m.ReadyWindows {
			sendReadyWindow(ms, c.id, ReadyCheckSeconds, p.Name)
		} else {
			w.raidLineLocked(ms, fmt.Sprintf("%s asks: Are you ready? Answer within %d seconds with /ready or /notready (on a PS2: /t ready or /t notready).", p.Name, ReadyCheckSeconds))
		}
	}
	what := "group"
	if raid {
		what = "raid"
	}
	w.raidLineLocked(sess, fmt.Sprintf("Ready check sent to your %s (%d asked, %d seconds).", what, len(c.asked), ReadyCheckSeconds))
	log.Printf("ReadyCheck %d: %s asks %d of group %d (raid %v)", c.id, p.Name, len(c.asked), g.ID, raid)
}

// sendReadyWindow is protocol.OpcodeReadyCheck: u32 check id, u8 seconds left (0
// closes it), the issuer's name (u8 count and its bytes).
func sendReadyWindow(sess *session.Session, id uint32, seconds int, issuer string) {
	sendGroupMsg(sess, protocol.OpcodeReadyCheck, func(b *buffer.Writer) {
		b.WriteUint32(id)
		b.WriteByte(byte(seconds))
		n := []byte(issuer)
		if len(n) > 64 {
			n = n[:64]
		}
		b.WriteByte(byte(len(n)))
		b.WriteBytes(n)
	})
}

// ReadyAnswer is sess's player's answer to its group's check: id the check's (0: whichever
// is open, a typed /ready), yes or no. typed: from a line, answered with a line. False
// when it had no check to answer.
func (w *World) ReadyAnswer(sess *session.Session, id uint32, yes, typed bool) bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.readyAnswerLocked(sess, id, yes, typed, time.Now())
}

func (w *World) readyAnswerLocked(sess *session.Session, id uint32, yes, typed bool, now time.Time) bool {
	p := w.sessionPlayerLocked(sess)
	c := w.pendingReadyLocked(p)
	if c == nil || id != 0 && id != c.id {
		if typed {
			w.refuseLocked(sess, "There is no ready check to answer.")
		}
		return false
	}
	if yes {
		c.answers[p.ServerID] = readyYes
	} else {
		c.answers[p.ServerID] = readyNo
	}
	if p.ReadyWindows {
		sendReadyWindow(sess, c.id, 0, "") // a typed answer closes the window too
	}
	if typed {
		if yes {
			w.raidLineLocked(sess, "You answered: ready.")
		} else {
			w.raidLineLocked(sess, "You answered: not ready.")
		}
	}
	w.readyMaybeDoneLocked(p.GroupID, c, now)
	return true
}

// HasReadyCheck is whether sess's player has a check to answer (a PS2's "/t ready" is
// one only then, else a tell).
func (w *World) HasReadyCheck(sess *session.Session) bool {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.pendingReadyLocked(w.sessionPlayerLocked(sess)) != nil
}

// pendingReadyLocked is p's group's check when p is asked by it, or nil. Must be called
// with the lock held.
func (w *World) pendingReadyLocked(p *Player) *readyCheck {
	if p == nil || p.GroupID == 0 {
		return nil
	}
	c := w.ready.byGroup[p.GroupID]
	if c == nil {
		return nil
	}
	for _, id := range c.asked {
		if id == p.ServerID {
			return c
		}
	}
	return nil
}

// readySystem ends the checks whose time is up, or whose group or issuer is gone. Must be
// called with the lock held.
func (w *World) readySystem(now time.Time) {
	for gid, c := range w.ready.byGroup {
		w.readyMaybeDoneLocked(gid, c, now)
	}
}

// readyMaybeDoneLocked ends c (group gid's) when everyone still in the group answered, its
// time is up, or its issuer or group is gone. Must be called with the lock held.
func (w *World) readyMaybeDoneLocked(gid uint32, c *readyCheck, now time.Time) {
	g := w.groups[gid]
	issuer := w.Players[c.issuer]
	in := func(id EntityID) *Player {
		if m := w.Players[id]; m != nil && g != nil && m.GroupID == g.ID {
			return m
		}
		return nil
	}
	if g == nil || issuer == nil || issuer.GroupID != gid {
		w.readyEndLocked(gid, c)
		log.Printf("ReadyCheck %d: its issuer or group is gone", c.id)
		return
	}
	var no, silent []string
	for _, id := range c.asked {
		m := in(id)
		if m == nil {
			continue
		}
		switch c.answers[id] {
		case readyNo:
			no = append(no, m.Name)
		case readyNone:
			silent = append(silent, m.Name)
		}
	}
	if len(silent) > 0 && now.Before(c.expires) {
		return
	}
	w.readyEndLocked(gid, c)
	is := w.sessionOfLocked(issuer)
	log.Printf("ReadyCheck %d: no %v, no answer %v", c.id, no, silent)
	if is == nil {
		return
	}
	if len(no) == 0 && len(silent) == 0 {
		w.raidLineLocked(is, "Everyone is ready")
		return
	}
	head := "Group not ready"
	if c.raid {
		head = "Raid not ready"
	}
	w.raidLineLocked(is, head)
	for _, l := range readyNameLines("Not ready: ", no) {
		w.raidLineLocked(is, l)
	}
	for _, l := range readyNameLines("No answer: ", silent) {
		w.raidLineLocked(is, l)
	}
}

// readyEndLocked drops group gid's check and closes the windows still open. Must be
// called with the lock held.
func (w *World) readyEndLocked(gid uint32, c *readyCheck) {
	delete(w.ready.byGroup, gid)
	for _, id := range c.asked {
		if c.answers[id] != readyNone {
			continue
		}
		if m := w.Players[id]; m != nil && m.ReadyWindows {
			sendReadyWindow(w.sessionOfLocked(m), c.id, 0, "")
		}
	}
}

// readyNameLinesWidth is about a PS2's chat line's width.
const readyNameLinesWidth = 60

// readyNameLines is names after head, comma-separated, in lines a PS2's width or so (the
// later ones indented); none for no names.
func readyNameLines(head string, names []string) []string {
	if len(names) == 0 {
		return nil
	}
	var out []string
	line := head
	for i, n := range names {
		part := n
		if i < len(names)-1 {
			part += ","
		}
		if len(line)+len(part) > readyNameLinesWidth && line != head && line != "  " {
			out = append(out, strings.TrimRight(line, " "))
			line = "  "
		}
		line += part + " "
	}
	return append(out, strings.TrimRight(line, " "))
}
