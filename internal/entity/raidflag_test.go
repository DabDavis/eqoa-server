package entity

import (
	"encoding/binary"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/eqoa/server/internal/protocol"
)

// The raid flag (raidflag.go) and the roster for a client that asks (raidroster.go).
// Each rule beside its control.

// boxesIn is who each 0x0773 in msgs names: its id and its words, as CLIENT's Msg0773
// reads them.
func boxesIn(msgs [][]byte) (out []string) {
	for _, m := range msgs {
		if opOfMsg(m) == protocol.OpcodeGroupInviteReceived {
			n := binary.LittleEndian.Uint32(m[10:])
			out = append(out, string(m[14:14+n]))
		}
	}
	return out
}

func refusedFull(msgs [][]byte) bool {
	for _, m := range msgs {
		if opOfMsg(m) == protocol.OpcodeRemoveGroupMember && m[2] == 4 {
			return true
		}
	}
	return false
}

// A flag asks every player within 60 in its world in no group, in the box a group
// invite shows, named as its leader's raid, and the leader hears whom. Controls: past
// 60, up in the air past it, another world, another group, a client still loading, a
// GM hidden from players (named in no one's line, and asked once it is not).
func TestARaidFlagAsksThoseInReach(t *testing.T) {
	r := newRaidRig(t, 10, 30)
	r.join(0, 1) // Bob: in the leader's group
	r.join(2, 3) // Cal and Dee: a group of their own
	r.drainAll()
	r.w.UpdatePosition(r.p[4].ServerID, 1000+59.5, 10, 1000, 0)  // Eve, in reach
	r.w.UpdatePosition(r.p[5].ServerID, 1000+60.5, 10, 1000, 0)  // Fay, past it
	r.w.UpdatePosition(r.p[6].ServerID, 1000+10, 10+70, 1000, 0) // Gus, overhead past it
	r.p[7].World = 2                                             // Hal, in another world
	r.sess[8].SetCharacterInWorld(false)                         // Ivy, loading
	r.p[9].GMHidden = true
	r.w.RaidCall(r.sess[0])
	for i, want := range map[int][]string{1: nil, 2: nil, 3: nil, 4: {"Ann's raid"}, 5: nil, 6: nil, 7: nil, 8: nil, 9: nil} {
		msgs := drain(r.sess[i])
		if got := boxesIn(msgs); fmt.Sprint(got) != fmt.Sprint(want) {
			t.Errorf("%s's boxes %q, want %q", r.p[i].Name, got, want)
		}
		if i == 4 && !hasLine(linesOf(msgs), "Ann raises a raid flag. Past 4 members a group takes 80% less XP from kills.") {
			t.Errorf("Eve is not told what it means: %q", linesOf(msgs))
		}
	}
	if got := r.heard(0); !hasLine(got, "Raid flag: asked Eve.") {
		t.Errorf("the leader heard %q", got)
	}
	r.p[9].GMHidden = false
	r.w.RaidCall(r.sess[0])
	if got := boxesIn(drain(r.sess[9])); fmt.Sprint(got) != "[Ann's raid]" {
		t.Errorf("the GM seen again: boxes %q", got)
	}
	if b := raidAskedLine(nil, 2); b != "Raid flag: no one new within 60 in no group. 2 still to answer." {
		t.Errorf("no one: %q", b)
	}
	names := make([]string, 14)
	for i := range names {
		names[i] = fmt.Sprintf("N%02d", i)
	}
	if b := raidAskedLine(names, 0); !strings.HasSuffix(b, "N11 and 2 more.") || strings.Contains(b, "N12") {
		t.Errorf("many: %q", b)
	}
}

// An invite stops at four, the inviter told what does more; a flag's yes does not, and
// a group past four stays so until it shrinks, its invites refused still. Below four
// an invite is as ever. One who said yes to an invite while the group filled is refused
// at its answer, 0x069A 4, and both told.
func TestAnInviteStopsAtFourAndAFlagDoesNot(t *testing.T) {
	r := newRaidRig(t, 8, 30)
	r.drainAll()
	for i := 1; i < 4; i++ {
		r.w.GroupInvite(r.sess[0], r.w.IDSeenBy(r.sess[0], r.p[i]))
		if got := boxesIn(drain(r.sess[i])); len(got) != 1 || got[0] != "Ann" {
			t.Fatalf("%s's boxes at %d members: %q", r.p[i].Name, i, got)
		}
		r.w.GroupAccept(r.sess[i], r.w.IDSeenBy(r.sess[i], r.p[0]))
	}
	r.drainAll()
	// a fifth by an invite: no box, a word to the inviter, whoever invites
	for _, by := range []int{0, 2} {
		r.w.GroupInvite(r.sess[by], r.w.IDSeenBy(r.sess[by], r.p[4]))
		if got := boxesIn(drain(r.sess[4])); len(got) != 0 {
			t.Errorf("a box past four: %q", got)
		}
		if got := r.heard(by); len(got) != 1 || !strings.Contains(got[0], "/raid") || !strings.Contains(got[0], "/t raid") {
			t.Errorf("the inviter heard %q", got)
		}
	}
	if r.p[4].GroupID != 0 || len(r.w.GroupMembers(r.p[0])) != 4 {
		t.Fatalf("a fifth joined: %d", len(r.w.GroupMembers(r.p[0])))
	}
	// by the flag: the box, the join
	r.w.RaidCall(r.sess[0])
	if got := boxesIn(drain(r.sess[4])); len(got) != 1 || got[0] != "Ann's raid" {
		t.Fatalf("the flag's box: %q", got)
	}
	r.w.GroupAccept(r.sess[4], r.w.IDSeenBy(r.sess[4], r.p[0]))
	if len(r.w.GroupMembers(r.p[0])) != 5 {
		t.Fatal("the flag's yes did not join")
	}
	// past four an invite is refused still
	r.drainAll()
	r.w.GroupInvite(r.sess[1], r.w.IDSeenBy(r.sess[1], r.p[6]))
	if got := boxesIn(drain(r.sess[6])); len(got) != 0 {
		t.Errorf("a box past four, in a raid group: %q", got)
	}
	// back at four, still; at three it opens again
	r.w.GroupLeave(r.sess[4])
	r.w.GroupInvite(r.sess[0], r.w.IDSeenBy(r.sess[0], r.p[6]))
	if got := boxesIn(drain(r.sess[6])); len(got) != 0 {
		t.Errorf("a box at four: %q", got)
	}
	r.w.GroupLeave(r.sess[3])
	r.w.GroupInvite(r.sess[0], r.w.IDSeenBy(r.sess[0], r.p[6]))
	r.w.GroupInvite(r.sess[0], r.w.IDSeenBy(r.sess[0], r.p[7]))
	r.drainAll()
	// two asked at three: the first yes fills it, the second is refused at its answer
	r.w.GroupAccept(r.sess[6], r.w.IDSeenBy(r.sess[6], r.p[0]))
	r.w.GroupAccept(r.sess[7], r.w.IDSeenBy(r.sess[7], r.p[0]))
	msgs := drain(r.sess[7])
	if !refusedFull(msgs) || r.p[7].GroupID != 0 || !strings.Contains(strings.Join(linesOf(msgs), " "), "/raid") {
		t.Errorf("the late yes: refused %v, group %d, lines %q", refusedFull(msgs), r.p[7].GroupID, linesOf(msgs))
	}
	if got := r.heard(0); len(got) == 0 || !strings.Contains(got[len(got)-1], "/raid") {
		t.Errorf("the inviter of the late yes heard %q", got)
	}
}

// A second flag asks whoever came into reach since, no one with a box unanswered, no
// one who said no in the last minute, and past that minute asks a refuser again.
func TestASecondFlagAsksOnlyWhoIsNew(t *testing.T) {
	r := newRaidRig(t, 4, 30)
	r.join(0, 1)
	r.w.UpdatePosition(r.p[3].ServerID, 1000+100, 10, 1000, 0) // Dee, out of reach
	r.drainAll()
	r.w.RaidCall(r.sess[0])
	if got := boxesIn(drain(r.sess[2])); len(got) != 1 {
		t.Fatalf("Cal's boxes: %q", got)
	}
	if got := boxesIn(drain(r.sess[3])); len(got) != 0 {
		t.Fatalf("Dee's boxes, out of reach: %q", got)
	}
	r.drainAll()
	r.w.UpdatePosition(r.p[3].ServerID, 1000+20, 10, 1000, 0)
	r.w.RaidCall(r.sess[0])
	if got := boxesIn(drain(r.sess[2])); len(got) != 0 {
		t.Errorf("Cal asked again, a box up: %q", got)
	}
	if got := boxesIn(drain(r.sess[3])); len(got) != 1 {
		t.Errorf("Dee, in reach now: %q", got)
	}
	if got := r.heard(0); !hasLine(got, "Raid flag: asked Dee. 1 still to answer.") {
		t.Errorf("the leader heard %q", got)
	}
	// Cal says no: not asked again at once, and the leader told as of an invite
	r.w.GroupDecline(r.sess[2], r.w.IDSeenBy(r.sess[2], r.p[0]), 1)
	r.drainAll()
	r.w.RaidCall(r.sess[0])
	if got := boxesIn(drain(r.sess[2])); len(got) != 0 {
		t.Errorf("Cal asked again at once: %q", got)
	}
	// a no keeps no box: its answer finds nothing to take
	r.w.GroupAccept(r.sess[2], r.w.IDSeenBy(r.sess[2], r.p[0]))
	if r.p[2].GroupID != 0 {
		t.Error("a no let a later yes in")
	}
	// a minute on it does
	r.drainAll()
	ask := r.w.invites[r.p[2].ServerID][r.p[0].ServerID]
	ask.declined = time.Now().Add(-raidAskAgain - time.Second)
	r.w.invites[r.p[2].ServerID][r.p[0].ServerID] = ask
	r.w.RaidCall(r.sess[0])
	if got := boxesIn(drain(r.sess[2])); len(got) != 1 {
		t.Errorf("Cal, a minute on: %q", got)
	}
	// a client's own "already grouped" or a death's refusal is no one's no
	r.w.GroupDecline(r.sess[2], r.w.IDSeenBy(r.sess[2], r.p[0]), 3)
	r.drainAll()
	r.w.RaidCall(r.sess[0])
	if got := boxesIn(drain(r.sess[2])); len(got) != 1 {
		t.Errorf("Cal, after a refusal of 3: %q", got)
	}
}

// Only a leader raises one; a member is told, and no one is asked.
func TestOnlyALeaderRaisesAFlag(t *testing.T) {
	r := newRaidRig(t, 3, 30)
	r.join(0, 1)
	r.drainAll()
	r.w.RaidCall(r.sess[1])
	if got := r.heard(1); len(got) != 1 || !strings.Contains(got[0], "leader") {
		t.Errorf("the member heard %q", got)
	}
	if got := boxesIn(drain(r.sess[2])); len(got) != 0 {
		t.Errorf("a member's flag asked: %q", got)
	}
}

// One in no group is asked first, in the box, for its client takes a group only from a
// yes (m_inGroup); no one else until then. Its yes makes it a group of one under its own
// roster, member 0, and the flag goes up. Its no leaves it in none; a second /raid sends
// no second box. A client that says it is in a group already gets the flag at once.
func TestACallerInNoGroupIsAskedFirst(t *testing.T) {
	r := newRaidRig(t, 3, 30)
	r.drainAll()
	r.w.RaidCall(r.sess[0])
	msgs := drain(r.sess[0])
	if got := boxesIn(msgs); len(got) != 1 || got[0] != raidOwnBoxWords {
		t.Fatalf("its boxes: %q", got)
	}
	if id := binary.LittleEndian.Uint32(msgs[0][2:]); id != r.sess[0].PlayerObjectID() {
		t.Errorf("its box names %#x, want its own id %#x", id, r.sess[0].PlayerObjectID())
	}
	if r.p[0].GroupID != 0 {
		t.Error("in a group before its yes")
	}
	if got := boxesIn(drain(r.sess[1])); len(got) != 0 {
		t.Errorf("another asked before its yes: %q", got)
	}
	r.w.RaidCall(r.sess[0])
	if got := boxesIn(drain(r.sess[0])); len(got) != 0 {
		t.Errorf("a second box: %q", got)
	}
	// no: nothing, and it may call again
	r.w.GroupDecline(r.sess[0], r.sess[0].PlayerObjectID(), 1)
	if r.p[0].GroupID != 0 {
		t.Fatal("in a group after its no")
	}
	if got := boxesIn(drain(r.sess[1])); len(got) != 0 {
		t.Errorf("another asked after a no: %q", got)
	}
	r.drainAll()
	r.w.RaidCall(r.sess[0])
	if got := boxesIn(drain(r.sess[0])); len(got) != 1 {
		t.Fatalf("after a no, its boxes: %q", got)
	}
	// yes: a group of one, its roster, and the others asked
	r.w.GroupAccept(r.sess[0], r.sess[0].PlayerObjectID())
	if r.p[0].GroupID == 0 {
		t.Fatal("no group after its yes")
	}
	ids, names := r.lastRoster(0)
	if len(ids) != 1 || ids[0] != r.sess[0].PlayerObjectID() || names[0] != "Ann" {
		t.Errorf("its roster: %v %v", ids, names)
	}
	for _, i := range []int{1, 2} {
		if got := boxesIn(drain(r.sess[i])); len(got) != 1 || got[0] != "Ann's raid" {
			t.Errorf("%s's boxes: %q", r.p[i].Name, got)
		}
	}
	// and they join it
	r.w.GroupAccept(r.sess[1], r.w.IDSeenBy(r.sess[1], r.p[0]))
	if got := r.w.GroupMembers(r.p[0]); len(got) != 2 || got[0] != r.p[0] {
		t.Errorf("the group: %v", got)
	}

	// Msg0773's own answer: already in a group, as it sees it
	a := newRaidRig(t, 2, 30)
	a.drainAll()
	a.w.RaidCall(a.sess[0])
	a.w.GroupDecline(a.sess[0], a.sess[0].PlayerObjectID(), 2)
	if a.p[0].GroupID == 0 {
		t.Error("answer 2: no group")
	}
	if got := boxesIn(drain(a.sess[1])); len(got) != 1 {
		t.Errorf("answer 2: the other's boxes %q", got)
	}
}

// What a leader asked goes with it: its boxes answer "not invited" once it has left, and
// a new leader's flag asks them anew. A login that replaces a session or takes one over
// has no box up, so it can be asked.
func TestAFlagsBoxesGoWithTheirLeader(t *testing.T) {
	r := newRaidRig(t, 4, 30)
	r.join(0, 1)
	r.join(0, 2)
	r.drainAll()
	r.w.RaidCall(r.sess[0]) // asks Dee
	r.w.GroupLeave(r.sess[0])
	r.drainAll()
	r.w.GroupAccept(r.sess[3], r.w.IDSeenBy(r.sess[3], r.p[0]))
	msgs := drain(r.sess[3])
	if r.p[3].GroupID != 0 {
		t.Error("joined a group its leader left")
	}
	removed := false
	for _, m := range msgs {
		removed = removed || (opOfMsg(m) == protocol.OpcodeRemoveGroupMember && m[2] == 3)
	}
	if !removed {
		t.Errorf("no 0x069A 3: % x", msgs)
	}
	r.w.RaidCall(r.sess[1]) // Bob leads now
	if got := boxesIn(drain(r.sess[3])); len(got) != 1 || got[0] != "Bob's raid" {
		t.Errorf("the new leader's box: %q", got)
	}

	// a replaced login
	l := newRaidRig(t, 3, 30)
	l.join(0, 1)
	l.drainAll()
	l.w.RaidCall(l.sess[0])
	drain(l.sess[2])
	ns := l.open(20)
	np := l.w.CreatePlayer(ns, l.record(2, 30))
	l.sess[2], l.p[2] = ns, np
	l.w.RaidCall(l.sess[0])
	if got := boxesIn(drain(ns)); len(got) != 1 {
		t.Errorf("a new login's boxes: %q", got)
	}
}

// readRaidRoster reads the 0x7E1A messages in msgs into one roster (the last message
// with a first of 0 begins it); ok is whether any came. Each page's head says what it
// holds, and a page is within a message's buffer.
type raidRow struct {
	id                      uint32
	name                    string
	class, level, hp, power byte
}

func readRaidRoster(t *testing.T, msgs [][]byte) (rows []raidRow, total int, sent int) {
	t.Helper()
	for _, m := range msgs {
		if opOfMsg(m) != protocol.OpcodeRaidRoster {
			continue
		}
		sent++
		if len(m) > protocol.DefaultMessageSize {
			t.Fatalf("a page of %d bytes", len(m))
		}
		total = int(binary.LittleEndian.Uint16(m[2:]))
		first := int(binary.LittleEndian.Uint16(m[4:]))
		count := int(m[6])
		if first == 0 {
			rows = nil
		}
		if first != len(rows) {
			t.Fatalf("a page from %d, %d read", first, len(rows))
		}
		b := m[7:]
		for i := 0; i < count; i++ {
			n := int(b[4])
			rows = append(rows, raidRow{id: binary.LittleEndian.Uint32(b), name: string(b[5 : 5+n]), class: b[5+n], level: b[6+n], hp: b[7+n], power: b[8+n]})
			b = b[9+n:]
		}
		if len(b) != 0 {
			t.Fatalf("%d bytes past a page's records", len(b))
		}
	}
	return rows, total, sent
}

func (r *raidRig) rosterPass() {
	r.w.mu.Lock()
	defer r.w.mu.Unlock()
	r.w.TickCount += raidRosterTicks - r.w.TickCount%raidRosterTicks
	r.w.raidRosterSystem()
}

// A client that shows the whole roster is not told what bars hold; one with its window
// of four is, when the group becomes a raid group and when it joins one.
func TestARaidGroupsLineFitsWhatItsClientShows(t *testing.T) {
	r := newRaidRig(t, 7, 30)
	r.form(4)
	r.w.SetRaidRoster(r.sess[1], true)
	r.w.SetRaidRoster(r.sess[5], true)
	r.drainAll()
	r.join(0, 4)
	if got := r.heard(1); !hasLine(got, raidNowWords) || hasLine(got, raidNowLine) {
		t.Errorf("a client with the roster heard %q", got)
	}
	if got := r.heard(2); !hasLine(got, raidNowLine) {
		t.Errorf("a client with its window heard %q", got)
	}
	r.drainAll()
	r.join(0, 5)
	if got := r.heard(5); !hasLine(got, raidJoinedWords) || hasLine(got, raidJoinedLine) {
		t.Errorf("a newcomer with the roster heard %q", got)
	}
	r.drainAll()
	r.join(0, 6)
	if got := r.heard(6); !hasLine(got, raidJoinedLine) {
		t.Errorf("a newcomer with its window heard %q", got)
	}
}

// A client that asked gets the whole raid group, each member's id as it knows it, name,
// class, level, HP and power; a client that did not, a PS2's, nothing and its window of
// four. In a group of four, nothing. On a change alone, at most once a second: the
// system looks every ten ticks and at no other. Back at four, 0 members once. Off, no
// more. A keep restart keeps the asking; a takeover's new client asks again.
func TestTheRaidRosterGoesToWhoAsks(t *testing.T) {
	r := newRaidRig(t, 6, 30)
	r.form(4)
	for i, p := range r.p {
		p.Class, p.MaxHP, p.HP = i, 400, 400
		p.Power, p.MaxPower = 100, 200
	}
	if !r.w.SetRaidRoster(r.sess[5], true) || !r.w.SetRaidRoster(r.sess[0], true) {
		t.Fatal("no player")
	}
	r.drainAll()
	r.rosterPass()
	if _, _, sent := readRaidRoster(t, drain(r.sess[0])); sent != 0 {
		t.Errorf("in a group of four: %d messages", sent)
	}
	r.join(0, 4)
	r.join(0, 5)
	vs := GetOrCreateVisibility(r.sess[5])
	vs.ChannelEntity[0], vs.ChannelObjectID[0] = r.p[1].ServerID, 100009
	r.w.mu.Lock()
	r.p[2].HP = 200
	r.p[3].Power = 50
	r.w.mu.Unlock()
	r.drainAll()
	r.rosterPass()
	rows, total, sent := readRaidRoster(t, drain(r.sess[5]))
	if sent != 1 || total != 6 || len(rows) != 6 {
		t.Fatalf("%d messages, %d in all, rows %+v", sent, total, rows)
	}
	want := []string{"Ann", "Bob", "Cal", "Dee", "Eve", "Fay"}
	for i, row := range rows {
		if row.name != want[i] || row.class != byte(i) || row.level != 30 {
			t.Errorf("row %d: %+v", i, row)
		}
	}
	if rows[5].id != r.sess[5].PlayerObjectID() || rows[1].id != 100009 || rows[0].id != groupFarID(r.p[0].ServerID) {
		t.Errorf("ids as it knows them: %#x %#x %#x", rows[5].id, rows[1].id, rows[0].id)
	}
	if rows[0].hp != 255 || rows[2].hp < 126 || rows[2].hp > 128 || rows[0].power != 127 || rows[3].power != 63 {
		t.Errorf("hp and power: %+v", rows)
	}
	// the others in the raid who did not ask: nothing, their rosters of four
	for _, i := range []int{1, 2, 3, 4} {
		if _, _, sent := readRaidRoster(t, drain(r.sess[i])); sent != 0 {
			t.Errorf("%s, who did not ask: %d messages", r.p[i].Name, sent)
		}
	}
	// nothing changed: nothing goes
	drain(r.sess[0])
	r.rosterPass()
	if _, _, sent := readRaidRoster(t, drain(r.sess[5])); sent != 0 {
		t.Errorf("no change: %d messages", sent)
	}
	// a change goes at the next look, not between looks
	r.p[1].HP = 10
	r.w.mu.Lock()
	for k := 0; k < raidRosterTicks-1; k++ {
		r.w.TickCount++
		if r.w.TickCount%raidRosterTicks == 0 {
			r.w.TickCount++
		}
		r.w.raidRosterSystem()
	}
	r.w.mu.Unlock()
	if _, _, sent := readRaidRoster(t, drain(r.sess[5])); sent != 0 {
		t.Errorf("between looks: %d messages", sent)
	}
	r.rosterPass()
	if rows, _, sent := readRaidRoster(t, drain(r.sess[5])); sent != 1 || rows[1].hp > 10 {
		t.Errorf("at the look: %d messages, %+v", sent, rows)
	}
	// a keep restart keeps the asking and sends it again
	kp, ok := r.w.KeepPlayer(r.sess[5], time.Now())
	if !ok || !kp.RaidRoster {
		t.Errorf("kept %+v", kp)
	}
	r.w.RestoreKeptPlayer(r.sess[5], kp, time.Now())
	r.rosterPass()
	if _, _, sent := readRaidRoster(t, drain(r.sess[5])); sent != 1 {
		t.Errorf("after a restore: %d messages", sent)
	}
	// back at four: 0 members, once
	r.w.GroupLeave(r.sess[4])
	r.w.GroupLeave(r.sess[3])
	r.drainAll()
	r.rosterPass()
	rows, total, sent = readRaidRoster(t, drain(r.sess[5]))
	if sent != 1 || total != 0 || len(rows) != 0 {
		t.Errorf("back at four: %d messages, %d in all, %d rows", sent, total, len(rows))
	}
	r.rosterPass()
	if _, _, sent := readRaidRoster(t, drain(r.sess[5])); sent != 0 {
		t.Errorf("back at four, again: %d messages", sent)
	}
	// off: no more, in a raid again
	r.w.SetRaidRoster(r.sess[5], false)
	r.join(0, 3)
	r.join(0, 4)
	r.drainAll()
	r.rosterPass()
	if _, _, sent := readRaidRoster(t, drain(r.sess[5])); sent != 0 {
		t.Errorf("off: %d messages", sent)
	}
	if rows, _, sent := readRaidRoster(t, drain(r.sess[0])); sent != 1 || len(rows) != 6 {
		t.Errorf("the other that asked: %d messages, %d rows", sent, len(rows))
	}
	// a takeover's new client has not asked
	r.p[0].CharServerID = 777
	r.sess[0].SetCharacterServerID(777)
	ns := r.open(30)
	got, _ := r.w.TakeOverPlayer(ns, 777)
	if got != r.p[0] || got.RaidRoster {
		t.Errorf("taken over: RaidRoster %v", got.RaidRoster)
	}
}

// However many, it comes in messages that fit, in order, whole.
func TestAVeryLargeRaidRosterIsPaged(t *testing.T) {
	recs := make([][]byte, 300)
	for i := range recs {
		name := strings.Repeat("x", 23)
		b := binary.LittleEndian.AppendUint32(nil, uint32(i+1))
		b = append(b, byte(len(name)))
		b = append(b, name...)
		recs[i] = append(b, 1, 60, 255, 255)
	}
	pages := raidRosterPages(recs)
	if len(pages) < 2 {
		t.Fatalf("%d pages", len(pages))
	}
	var msgs [][]byte
	for _, p := range pages {
		msgs = append(msgs, append([]byte{0x1A, 0x7E}, p...))
	}
	rows, total, sent := readRaidRoster(t, msgs)
	if total != 300 || len(rows) != 300 || sent != len(pages) || rows[299].id != 300 {
		t.Errorf("%d in all, %d rows, %d messages", total, len(rows), sent)
	}
	if got := raidRosterPages(nil); len(got) != 1 || len(got[0]) != 5 || got[0][0] != 0 || got[0][4] != 0 {
		t.Errorf("no records: % x", got)
	}
}
