package entity

import (
	"encoding/binary"
	"testing"

	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// relog is member i logging in again on a new client, which enters the world
// (rejoinGroupLocked keeps it in its group); its new session.
func (r *raidRig) relog(i, port int) *session.Session {
	r.t.Helper()
	ns := r.open(port)
	ns.SetCharacterInWorld(false)
	np := r.w.CreatePlayer(ns, r.record(i, 30))
	np.HP, np.MaxHP = 500, 500
	r.sess[i], r.p[i] = ns, np
	return ns
}

func rejoinBoxes(msgs [][]byte) int {
	n := 0
	for _, m := range msgs {
		if opOfMsg(m) == protocol.OpcodeGroupInviteReceived {
			n++
		}
	}
	return n
}

// Petitions 2338/2348/2351: a member back on a new client is asked, once its
// client is in the world, to show its group; its yes is sent the roster, its
// answer 2 (its client holds the group) the roster again, its no takes it out.
func TestANewClientIsAskedToShowItsGroup(t *testing.T) {
	for _, tc := range []struct {
		name   string
		answer func(r *raidRig, ns *session.Session)
		in     bool
	}{
		{"yes", func(r *raidRig, ns *session.Session) { r.w.GroupAccept(ns, r.w.IDSeenBy(ns, r.p[2])) }, true},
		{"held already", func(r *raidRig, ns *session.Session) { r.w.GroupDecline(ns, r.w.IDSeenBy(ns, r.p[2]), 2) }, true},
		{"no", func(r *raidRig, ns *session.Session) { r.w.GroupDecline(ns, r.w.IDSeenBy(ns, r.p[2]), 1) }, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := newRaidRig(t, 3, 30)
			r.form(3)
			r.sync()
			r.drainAll()
			ns := r.relog(2, 40)
			r.sync()
			if n := rejoinBoxes(drain(ns)); n != 0 {
				t.Fatalf("%d boxes before its world", n)
			}
			ns.SetCharacterInWorld(true)
			r.sync()
			msgs := drain(ns)
			if n := rejoinBoxes(msgs); n != 1 || !hasLine(linesOf(msgs), groupRejoinLine) {
				t.Fatalf("in the world: %d boxes, lines %q", n, linesOf(msgs))
			}
			r.sync()
			if n := rejoinBoxes(drain(ns)); n != 0 {
				t.Fatalf("asked again: %d boxes", n)
			}
			tc.answer(r, ns)
			msgs = drain(ns)
			if tc.in {
				if r.p[2].GroupID == 0 || r.p[2].GroupID != r.p[0].GroupID {
					t.Fatalf("out of the group after %s", tc.name)
				}
				if rs := rostersIn(t, msgs); len(rs) != 1 || len(rs[0]) != 3 {
					t.Errorf("its roster after %s: %v", tc.name, rs)
				}
				return
			}
			if r.p[2].GroupID != 0 || len(r.w.GroupMembers(r.p[0])) != 2 {
				t.Errorf("after its no: group %d, %d left", r.p[2].GroupID, len(r.w.GroupMembers(r.p[0])))
			}
		})
	}
}

// The leader back on a new client is asked too, its box naming another member;
// a member whose group is gone by its answer is told so; one whose group is
// only itself (its mercs' probe is mercgroup.go's) is not asked.
func TestALeaderIsAskedAndAGoneGroupIsTold(t *testing.T) {
	r := newRaidRig(t, 2, 30)
	r.form(2)
	r.sync()
	r.drainAll()
	ns := r.relog(0, 41)
	ns.SetCharacterInWorld(true)
	r.sync()
	if n := rejoinBoxes(drain(ns)); n != 1 {
		t.Fatalf("the leader: %d boxes", n)
	}
	r.w.GroupLeave(r.sess[1]) // the group of two is gone
	r.drainAll()
	r.w.GroupAccept(ns, r.w.IDSeenBy(ns, r.p[0]))
	got := sentOn(ns)
	if got[protocol.OpcodeRemoveGroupMember] != 1 || r.p[0].GroupID != 0 {
		t.Errorf("a yes to a group gone: %v, group %d", got, r.p[0].GroupID)
	}

	r = newRaidRig(t, 1, 30)
	r.w.mu.Lock()
	g := r.w.formGroupLocked(r.p[0])
	r.w.mu.Unlock()
	if g == nil {
		t.Fatal("no group")
	}
	ns = r.relog(0, 42)
	ns.SetCharacterInWorld(true)
	r.sync()
	if n := rejoinBoxes(drain(ns)); n != 0 {
		t.Errorf("alone in its group: %d boxes", n)
	}
}

// withoutRejoinLine is lines without groupRejoinLine, which a new client in a
// group of others hears with its box (grouprejoin.go).
func withoutRejoinLine(lines []string) []string {
	var out []string
	for _, l := range lines {
		if l != groupRejoinLine {
			out = append(out, l)
		}
	}
	return out
}

// takeOver is member i's character taken over by a new client of its own (a
// reload: TakeOverPlayer), which enters the world; its new session.
func (r *raidRig) takeOver(i, port int) *session.Session {
	r.t.Helper()
	ns := r.open(port)
	ns.SetCharacterInWorld(false)
	if p, _ := r.w.TakeOverPlayer(ns, uint32(raidID(i))); p != r.p[i] {
		r.t.Fatalf("member %d not taken over", i)
	}
	r.sess[i] = ns
	return ns
}

// Live petition 5533: both members of a group reload (an update) one after the
// other, each answering yes to its box; each client's roster and group channel
// name the other.
func TestTwoReloadsOneAfterTheOtherBothShowTheGroup(t *testing.T) {
	r := newRaidRig(t, 2, 30)
	r.form(2)
	r.sync()
	r.drainAll()
	a := r.takeOver(0, 50)
	a.SetCharacterInWorld(true)
	r.sync()
	if n := rejoinBoxes(drain(a)); n != 1 {
		t.Fatalf("the first reload: %d boxes", n)
	}
	r.w.GroupAccept(a, r.w.IDSeenBy(a, r.p[0]))
	r.sync()
	b := r.takeOver(1, 51)
	b.SetCharacterInWorld(true)
	r.sync()
	if n := rejoinBoxes(drain(b)); n != 1 {
		t.Fatalf("the second reload: %d boxes", n)
	}
	r.drainAll()
	r.w.GroupAccept(b, r.w.IDSeenBy(b, r.p[1]))
	r.sync()
	for i := 0; i < 2; i++ {
		ids, names := r.lastRoster(i)
		t.Logf("member %d: roster %v %v", i, ids, names)
		if len(names) != 2 {
			t.Errorf("member %d's roster after both yeses: %v", i, names)
		}
		other := r.w.IDSeenBy(r.sess[i], r.p[1-i])
		data := r.sess[i].Connection().GetServerGroupUpdate().GetData()
		var recs []uint32
		for k := 0; k+13 <= len(data); k += 13 {
			if id := binary.LittleEndian.Uint32(data[k:]); id != 0 {
				recs = append(recs, id)
			}
		}
		if len(recs) != 1 || recs[0] != other {
			t.Errorf("member %d's group channel: %v, want the other's %d", i, recs, other)
		}
	}
}
