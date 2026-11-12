package entity

import (
	"encoding/binary"
	"encoding/json"
	"testing"

	"github.com/eqoa/server/internal/protocol"
)

// A raid's groups inside it (raidsubgroups.go; the GM Jibrish's petition 1687): each
// beside its control.

// rosterGroups is the last 0x7E1A roster session i was queued (drained), each name's
// group number, read as a client that answered RaidRosterWithGroups reads it: a byte
// after each record (a page with any other length fails).
func (r *raidRig) rosterGroups(i int) (subs map[string]uint8, order []string) {
	r.t.Helper()
	for _, m := range drain(r.sess[i]) {
		if opOfMsg(m) != protocol.OpcodeRaidRoster {
			continue
		}
		first, count := int(binary.LittleEndian.Uint16(m[4:])), int(m[6])
		if first == 0 {
			subs, order = map[string]uint8{}, nil
		}
		b := m[7:]
		for k := 0; k < count; k++ {
			n := int(b[4])
			name := string(b[5 : 5+n])
			order = append(order, name)
			subs[name] = b[9+n]
			b = b[10+n:]
		}
		if len(b) != 0 {
			r.t.Fatalf("%d bytes past a page's records", len(b))
		}
	}
	return subs, order
}

// groupsOf is the World's own numbers for the rig's raid, by name.
func (r *raidRig) groupsOf() map[string]uint8 {
	r.w.mu.Lock()
	defer r.w.mu.Unlock()
	out := map[string]uint8{}
	for id, n := range r.w.subgroupsLocked(r.raidOf(0)) {
		out[r.w.Players[id].Name] = n
	}
	return out
}

func wantGroups(t *testing.T, got map[string]uint8, want map[string]uint8) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("groups %v, want %v", got, want)
	}
	for k, v := range want {
		if got[k] != v {
			t.Fatalf("groups %v, want %v (%s)", got, want, k)
		}
	}
}

// A raid of ten is groups of four in the order they joined (4, 4, 2), sent after each
// record to a client that answered 2; a client that answered 1 has its records as
// before (the control: no byte after them).
func TestARaidsGroupsFillInJoinOrderAndGoToAClientThatAsked(t *testing.T) {
	r := newRaidRig(t, 10, 30)
	r.form(10)
	r.w.SetRaidRosterAnswer(r.sess[0], RaidRosterWithGroups)
	r.w.SetRaidRosterAnswer(r.sess[1], 1)
	r.drainAll()
	r.rosterPass()
	subs, order := r.rosterGroups(0)
	if len(order) != 10 {
		t.Fatalf("Ann's roster: %v %v", order, subs)
	}
	want := map[string]uint8{"Ann": 1, "Bob": 1, "Cal": 1, "Dee": 1, "Eve": 2, "Fay": 2, "Gus": 2, "Hal": 2, "Ivy": 3, "Jon": 3}
	wantGroups(t, subs, want)
	rows, _, _ := readRaidRoster(t, drain(r.sess[1])) // fails on a byte past the records
	if len(rows) != 10 {
		t.Fatalf("Bob's roster as before: %d rows", len(rows))
	}
}

// The leader and its helper move a member to a group with room, the one moved is told
// and every roster goes at once; a full group refuses and names the swap; past the
// highest group refuses; a plain member is refused (the control).
func TestTheLeaderAndHelperMoveAMember(t *testing.T) {
	r := newRaidRig(t, 10, 30)
	r.form(10)
	r.w.SetRaidRosterAnswer(r.sess[0], RaidRosterWithGroups)
	r.w.RaidHelper(r.sess[0], "Bob", 0)
	r.drainAll()
	// full: group 1 holds four
	r.w.RaidGroupMove(r.sess[0], "Ivy", 0, 1)
	if !saidPart(r.heard(0), "Group 1 is full (4). Swap two members: /raidswap Ivy") {
		t.Fatal("a move to a full group was not refused with the swap")
	}
	// most: (10+3)/4+1 = 4
	r.w.RaidGroupMove(r.sess[0], "Ivy", 0, 5)
	if !saidPart(r.heard(0), "Groups go from 1 to 4") {
		t.Fatal("group 5 of a raid of ten was not refused")
	}
	// a plain member: the control
	r.w.RaidGroupMove(r.sess[3], "Ivy", 0, 4)
	if !saidPart(r.heard(3), "Only your raid's leader or its helper can move members") {
		t.Fatal("a plain member's move was not refused")
	}
	wantGroups(t, r.groupsOf(), map[string]uint8{"Ann": 1, "Bob": 1, "Cal": 1, "Dee": 1, "Eve": 2, "Fay": 2, "Gus": 2, "Hal": 2, "Ivy": 3, "Jon": 3})
	// the helper, by the id its client knows Ivy by, to a new group 4
	r.drainAll()
	r.w.RaidGroupMove(r.sess[1], "", r.w.IDSeenBy(r.sess[1], r.p[8]), 4)
	if !saidPart(r.heard(1), "Bob moves Ivy to group 4.") || !saidPart(r.heard(8), "Bob puts you in group 4 of the raid.") {
		t.Fatal("the move was not told")
	}
	subs, _ := r.rosterGroups(0) // sent at once, no pass
	if subs["Ivy"] != 4 || subs["Jon"] != 3 {
		t.Fatalf("Ann's roster after the move: %v", subs)
	}
	// the leader moves Dee out of group 1, then Jon in
	r.w.RaidGroupMove(r.sess[0], "dee", 0, 3)
	r.w.RaidGroupMove(r.sess[0], "Jon", 0, 1)
	wantGroups(t, r.groupsOf(), map[string]uint8{"Ann": 1, "Bob": 1, "Cal": 1, "Dee": 3, "Eve": 2, "Fay": 2, "Gus": 2, "Hal": 2, "Ivy": 4, "Jon": 1})
	// one not in the raid
	r.drainAll()
	r.w.RaidGroupMove(r.sess[0], "Zed", 0, 2)
	if !saidPart(r.heard(0), "Zed is not in your raid.") {
		t.Fatal("a name not in the raid was not said")
	}
}

// A swap trades two members' groups (full or not); two of one group refuse.
func TestASwapTradesTwoMembersGroups(t *testing.T) {
	r := newRaidRig(t, 8, 30)
	r.form(8)
	r.drainAll()
	r.w.RaidGroupSwap(r.sess[0], "Bob", "Cal", 0, 0)
	if !saidPart(r.heard(0), "Bob and Cal are both in group 1.") {
		t.Fatal("a swap within a group was not refused")
	}
	r.w.RaidGroupSwap(r.sess[0], "Bob", "Hal", 0, 0)
	if !saidPart(r.heard(0), "Ann swaps Bob (now group 2) and Hal (now group 1).") {
		t.Fatal("the swap was not told")
	}
	if !saidPart(r.heard(7), "Ann puts you in group 1 of the raid.") {
		t.Fatal("Hal was not told")
	}
	wantGroups(t, r.groupsOf(), map[string]uint8{"Ann": 1, "Bob": 2, "Cal": 1, "Dee": 1, "Eve": 2, "Fay": 2, "Gus": 2, "Hal": 1})
	// by ids (the window's)
	r.w.RaidGroupSwap(r.sess[0], "", "", r.w.IDSeenBy(r.sess[0], r.p[1]), r.w.IDSeenBy(r.sess[0], r.p[7]))
	wantGroups(t, r.groupsOf(), map[string]uint8{"Ann": 1, "Bob": 1, "Cal": 1, "Dee": 1, "Eve": 2, "Fay": 2, "Gus": 2, "Hal": 2})
}

// One leaving frees its place and moves no one; a newcomer takes the lowest group with
// room.
func TestALeaverFreesItsPlaceAndANewcomerTakesIt(t *testing.T) {
	r := newRaidRig(t, 10, 30)
	r.form(9)
	r.rosterPass()           // the numbers settled, whoever asked for them
	r.w.RaidLeave(r.sess[2]) // Cal, of group 1
	wantGroups(t, r.groupsOf(), map[string]uint8{"Ann": 1, "Bob": 1, "Dee": 1, "Eve": 2, "Fay": 2, "Gus": 2, "Hal": 2, "Ivy": 3})
	r.join(0, 9) // Jon
	wantGroups(t, r.groupsOf(), map[string]uint8{"Ann": 1, "Bob": 1, "Dee": 1, "Eve": 2, "Fay": 2, "Gus": 2, "Hal": 2, "Ivy": 3, "Jon": 1})
}

// /raidgroup alone lists the groups, a line each, for anyone in the raid (a PS2's way).
func TestRaidGroupsListsTheGroups(t *testing.T) {
	r := newRaidRig(t, 6, 30)
	r.form(6)
	r.drainAll()
	r.w.RaidGroups(r.sess[5])
	l := r.heard(5)
	if !saidPart(l, "Group 1: Ann, Bob, Cal, Dee") || !saidPart(l, "Group 2: Eve, Fay") || !saidPart(l, "You are in group 2.") {
		t.Fatalf("Fay heard %q", l)
	}
}

// A keep restart keeps the numbers; a snapshot from before them fills them again; a
// plain group keeps none.
func TestARaidsGroupsAreKeptOverARestart(t *testing.T) {
	r := newRaidRig(t, 7, 30)
	r.form(7)
	r.w.RaidGroupMove(r.sess[0], "Bob", 0, 2)
	b, err := json.Marshal(r.w.KeepGroups())
	if err != nil {
		t.Fatal(err)
	}
	var kept []KeptGroup
	if err := json.Unmarshal(b, &kept); err != nil {
		t.Fatal(err)
	}
	for _, p := range r.p {
		p.GroupID = 0
	}
	r.w.groups = nil
	if n, err := r.w.RestoreGroups(kept); n != 1 || err != nil {
		t.Fatalf("restored %d: %v", n, err)
	}
	want := map[string]uint8{"Ann": 1, "Bob": 2, "Cal": 1, "Dee": 1, "Eve": 2, "Fay": 2, "Gus": 2}
	wantGroups(t, r.groupsOf(), want)
	// before them: no subs, filled in join order
	kept[0].Subs = nil
	for _, p := range r.p {
		p.GroupID = 0
	}
	r.w.groups = nil
	r.w.RestoreGroups(kept)
	wantGroups(t, r.groupsOf(), map[string]uint8{"Ann": 1, "Bob": 1, "Cal": 1, "Dee": 1, "Eve": 2, "Fay": 2, "Gus": 2})
	r2 := newRaidRig(t, 3, 30)
	r2.form(3)
	if k := r2.w.KeepGroups(); len(k) != 1 || k[0].Subs != nil {
		t.Fatalf("a plain group kept %+v", k)
	}
}
