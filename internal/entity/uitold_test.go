package entity

import (
	"bytes"
	"encoding/binary"
	"testing"
	"time"

	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// uitold.go and groupinvites.go (the GM Jibrish's petitions 1475, 1476 and
// 1488, 2026-11-11), over the mercs' rig: what a client that asked is told of
// the mercs and of its group search standing, once a second and only what
// changed; a client that never asked (a PS2) is told nothing; and a group's
// leader's "Allow anyone in my group to invite" decides a member's invite in
// a regular group, never a raid group's.

// uiTold runs the system at d past the rig's start and returns what s was
// sent of op since the last look, each body past the opcode.
func (r *mercRig) uiTold(d time.Duration, s *session.Session, op protocol.GameOpcode) [][]byte {
	r.w.mu.Lock()
	r.w.uiToldSystem(r.at(d))
	r.w.mu.Unlock()
	var out [][]byte
	for _, m := range s.Queue().GetPending() {
		if d := m.Data(); opOfMsg(d) == op {
			out = append(out, append([]byte(nil), d[2:]...))
		}
	}
	return out
}

// mercsRead reads OpcodeMercsData's body as vanilla does: its own merc out,
// and the names; it must end at the body's end.
func mercsRead(t *testing.T, b []byte) (bool, []string) {
	t.Helper()
	if len(b) < 4 || b[0] != 1 {
		t.Fatalf("OpcodeMercsData % x", b)
	}
	n, at := int(binary.LittleEndian.Uint16(b[2:])), 4
	var names []string
	for range n {
		l := int(b[at])
		names = append(names, string(b[at+1:at+1+l]))
		at += 1 + l
	}
	if at != len(b) {
		t.Fatalf("OpcodeMercsData read %d of %d bytes", at, len(b))
	}
	return b[1]&1 != 0, names
}

// The mercs told: once asked, the empty set and no merc of one's own; a
// hire (the owner's: own out, its name; another player's: its name alone, in
// its world), told within the second and not again while nothing changes;
// a dismissal; the control, a session that never asked, is sent nothing.
func TestMercsToldToTheClientsThatAsked(t *testing.T) {
	r := newMercRig(t)
	other, _ := r.player("Other", 1, 0)
	ps2, _ := r.player("Console", 2, 0)
	r.sess.SetMercsTold(true)
	other.SetMercsTold(true)
	got := r.uiTold(0, r.sess, protocol.OpcodeMercsData)
	if len(got) != 1 {
		t.Fatalf("at the answer: %d messages", len(got))
	}
	if own, names := mercsRead(t, got[0]); own || len(names) != 0 {
		t.Errorf("before a hire: own %v names %v", own, names)
	}
	r.hire(r.kit(0, 0, 0), MercAssist)
	if got := r.uiTold(500*time.Millisecond, r.sess, protocol.OpcodeMercsData); len(got) != 0 {
		t.Errorf("within the second of the last look: %d messages", len(got))
	}
	got = r.uiTold(time.Second, r.sess, protocol.OpcodeMercsData)
	if len(got) != 1 {
		t.Fatalf("after the hire: %d messages", len(got))
	}
	if own, names := mercsRead(t, got[0]); !own || len(names) != 1 || names[0] != "Ralthas" {
		t.Errorf("the owner after the hire: own %v names %v", own, names)
	}
	got = r.uiTold(time.Second, other, protocol.OpcodeMercsData)
	if len(got) != 2 {
		t.Fatalf("the other player: %d messages, want the set before and after", len(got))
	}
	if own, names := mercsRead(t, got[1]); own || len(names) != 1 || names[0] != "Ralthas" {
		t.Errorf("another player after the hire: own %v names %v", own, names)
	}
	if got := r.uiTold(2*time.Second, r.sess, protocol.OpcodeMercsData); len(got) != 0 {
		t.Errorf("nothing changed: %d messages", len(got))
	}
	if _, ok := r.w.DismissMerc(r.sess, r.at(3*time.Second)); !ok {
		t.Fatal("no dismissal")
	}
	got = r.uiTold(3*time.Second, r.sess, protocol.OpcodeMercsData)
	if len(got) != 1 {
		t.Fatalf("after the dismissal: %d messages", len(got))
	}
	if own, names := mercsRead(t, got[0]); own || len(names) != 0 {
		t.Errorf("after the dismissal: own %v names %v", own, names)
	}
	// the control: never asked, never told, of either
	for _, op := range []protocol.GameOpcode{protocol.OpcodeMercsData, protocol.OpcodeGroupRegState} {
		if got := r.uiTold(4*time.Second, ps2, op); len(got) != 0 {
			t.Errorf("a session that never asked was sent %d of %#x", len(got), op)
		}
	}
}

// A merc in another world is not told: the names are the player's world's.
func TestMercsToldAreThePlayersWorlds(t *testing.T) {
	r := newMercRig(t)
	away, pa := r.player("Away", 1, 0)
	pa.World = 3
	away.SetMercsTold(true)
	r.hire(r.kit(0, 0, 0), MercAssist)
	got := r.uiTold(0, away, protocol.OpcodeMercsData)
	if len(got) != 1 {
		t.Fatalf("%d messages", len(got))
	}
	if _, names := mercsRead(t, got[0]); len(names) != 0 {
		t.Errorf("a player in world 3 told %v", names)
	}
}

// The standing told: registered (bit 0), its own setting (bit 1, on unless
// turned off), its group's members invite (bit 2); a member's bit 2 follows
// its leader's setting at once, a raid group's members invite whatever it
// says.
func TestGroupRegStateTold(t *testing.T) {
	r := newMercRig(t)
	ms, pm := r.player("Member", 1, 0)
	r.sess.SetGroupReg(true)
	ms.SetGroupReg(true)
	flags := func(d time.Duration, s *session.Session) int {
		t.Helper()
		got := r.uiTold(d, s, protocol.OpcodeGroupRegState)
		if len(got) == 0 {
			return -1 // nothing new
		}
		if len(got) != 1 || len(got[0]) != 2 || got[0][0] != 1 {
			t.Fatalf("OpcodeGroupRegState %x", got)
		}
		return int(got[0][1])
	}
	if f := flags(0, r.sess); f != 0b110 {
		t.Errorf("at the answer: %03b, want own on and members invite", f)
	}
	r.p.LFG = true
	if f := flags(time.Second, r.sess); f != 0b111 {
		t.Errorf("registered: %03b", f)
	}
	r.group(pm)
	flags(time.Second, ms)
	if _, leads := r.w.SetGroupInvitesOpen(r.sess, false); !leads {
		t.Error("the setter leads a regular group")
	}
	if f := flags(time.Second, ms); f != 0b010 {
		t.Errorf("the member after its leader's no: %03b, want its own on, members not inviting", f)
	}
	if f := flags(time.Second, r.sess); f != 0b001 {
		t.Errorf("the leader after its no: %03b, want registered alone (its members do not invite; it does)", f)
	}
	r.w.mu.Lock()
	r.w.groups[r.p.GroupID].flagged = true
	r.w.mu.Unlock()
	if f := flags(2*time.Second, ms); f != 0b110 {
		t.Errorf("the member of a raid group: %03b, want members inviting", f)
	}
}

// A member's invite with the leader's setting off is refused, the member
// told who invites; the leader's own goes; the setting on again, the
// member's goes. In a raid group a member's goes whatever the leader set.
func TestLeadersSettingDecidesMembersInvites(t *testing.T) {
	r := newMercRig(t)
	ms, pm := r.player("Member", 1, 0)
	ts, pt := r.player("Target", 2, 0)
	_, pt2 := r.player("Second", 3, 0)
	r.group(pm)
	asked := func(p *Player) bool {
		r.w.mu.Lock()
		defer r.w.mu.Unlock()
		return len(r.w.invites[p.ServerID]) > 0
	}
	r.w.SetGroupInvitesOpen(r.sess, false)
	ms.Queue().GetPending()
	r.w.GroupInvite(ms, r.w.IDSeenBy(ms, pt))
	if asked(pt) {
		t.Error("a member's invite went with the leader's setting off")
	}
	line := []byte("Only Owner, your group's leader, invites into this group.")
	wide := make([]byte, 0, 2*len(line))
	for _, c := range line {
		wide = append(wide, c, 0)
	}
	told := false
	for _, m := range ms.Queue().GetPending() {
		told = told || bytes.Contains(m.Data(), wide)
	}
	if !told {
		t.Error("the member was not told who invites")
	}
	r.w.GroupInvite(r.sess, r.w.IDSeenBy(r.sess, pt))
	if !asked(pt) {
		t.Error("the leader's own invite did not go")
	}
	// the control: the setting on, the member's goes
	r.w.SetGroupInvitesOpen(r.sess, true)
	r.w.GroupInvite(ms, r.w.IDSeenBy(ms, pt2))
	if !asked(pt2) {
		t.Error("a member's invite did not go with the setting on")
	}
	// a raid group: the setting off changes nothing
	r.w.mu.Lock()
	delete(r.w.invites, pt2.ServerID)
	r.w.groups[r.p.GroupID].flagged = true
	r.w.mu.Unlock()
	r.w.SetGroupInvitesOpen(r.sess, false)
	r.w.GroupInvite(ms, r.w.IDSeenBy(ms, pt2))
	if !asked(pt2) {
		t.Error("a raid group's member's invite was refused")
	}
	_ = ts
}

// Layout 2 (the owner, 2026-11-11: a merc's "<Owner's Mercenary>" tag): a
// client that answered 2 is told each merc's name and its hirer's; one that
// answered 1, beside it, the names alone as before.
func TestMercsToldWithTheirHirers(t *testing.T) {
	r := newMercRig(t)
	old, _ := r.player("Old", 1, 0)
	r.sess.SetMercsLayout(2)
	old.SetMercsLayout(1)
	r.hire(r.kit(0, 0, 0), MercAssist)
	got := r.uiTold(0, r.sess, protocol.OpcodeMercsData)
	if len(got) != 1 {
		t.Fatalf("%d messages", len(got))
	}
	want := []byte{2, 1, 1, 0, 7, 'R', 'a', 'l', 't', 'h', 'a', 's', 5, 'O', 'w', 'n', 'e', 'r'}
	if !bytes.Equal(got[0], want) {
		t.Errorf("layout 2: % x, want % x", got[0], want)
	}
	got = r.uiTold(0, old, protocol.OpcodeMercsData)
	if len(got) != 1 {
		t.Fatalf("layout 1: %d messages", len(got))
	}
	if own, names := mercsRead(t, got[0]); own || len(names) != 1 || names[0] != "Ralthas" {
		t.Errorf("layout 1: own %v names %v", own, names)
	}
}
