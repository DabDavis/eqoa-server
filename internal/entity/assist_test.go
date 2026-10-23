package entity

import (
	"net"
	"testing"

	"github.com/eqoa/server/internal/session"
)

// Assist Target and Assist Leader (assist.go) across two sessions whose
// visibility tables number the same NPCs the other way round: the orc is
// 100002 to the asker and 100003 to the leader, the rat the reverse. An
// answer read through the wrong session's table names the other NPC.

type assistWorld struct {
	w              *World
	asker, leader  *session.Session
	a, b           *Player
	orc, rat, wolf *NPC
}

const (
	askerSelf  = 0x1424A1D0
	leaderSelf = 0x14330D1E
	// the asker's ids
	aLeader, aOrc, aRat = 100001, 100002, 100003
	// the leader's ids
	bAsker, bRat, bOrc, bWolf = 100001, 100002, 100003, 100004
)

func newAssistWorld(t *testing.T) *assistWorld {
	t.Helper()
	w := NewWorld()
	mgr := session.NewManager()
	w.SessionManager = mgr
	mk := func(port int, p *Player, self uint32) *session.Session {
		s := mgr.CreateSession(&net.UDPAddr{IP: net.IPv4(10, 0, 0, byte(port)), Port: 7000 + port}, uint32(port), 1, 1, 2, true)
		w.Players[p.ServerID] = p
		s.SetECSEntity(p)
		s.SetCharacterInWorld(true)
		s.SetPlayerObjectID(self)
		return s
	}
	a := &Player{ServerID: 1001, Name: "Asker", Level: 20, X: 100, Z: 100}
	b := &Player{ServerID: 1002, Name: "Leader", Level: 20, X: 102, Z: 100}
	aw := &assistWorld{w: w, a: a, b: b}
	aw.asker = mk(8, a, askerSelf)
	aw.leader = mk(9, b, leaderSelf)
	aw.orc = &NPC{ServerID: 2001, Name: "an orc", Level: 20, HP: 100, MaxHP: 100}
	aw.rat = &NPC{ServerID: 2002, Name: "a rat", Level: 2, HP: 10, MaxHP: 10}
	aw.wolf = &NPC{ServerID: 2003, Name: "a wolf", Level: 5, HP: 30, MaxHP: 30}
	for _, n := range []*NPC{aw.orc, aw.rat, aw.wolf} {
		w.NPCs[n.ServerID] = n
	}
	see := func(s *session.Session, ids ...any) {
		vs := GetOrCreateVisibility(s)
		for i := 0; i < len(ids); i += 2 {
			vs.ChannelEntity[i/2] = ids[i].(EntityID)
			vs.ChannelObjectID[i/2] = uint32(ids[i+1].(int))
		}
	}
	see(aw.asker, b.ServerID, aLeader, aw.orc.ServerID, aOrc, aw.rat.ServerID, aRat)
	see(aw.leader, a.ServerID, bAsker, aw.rat.ServerID, bRat, aw.orc.ServerID, bOrc, aw.wolf.ServerID, bWolf)
	return aw
}

// group puts the leader at the head of a group with the asker.
func (aw *assistWorld) group() {
	aw.w.groups = map[uint32]*Group{7: {ID: 7, Members: []EntityID{aw.b.ServerID, aw.a.ServerID}}}
	aw.a.GroupID, aw.b.GroupID = 7, 7
}

func (aw *assistWorld) target(t *testing.T, who *session.Session, id uint32) uint32 {
	t.Helper()
	got, ok := aw.w.AssistTarget(who, id)
	if !ok {
		t.Fatal("no player for the session")
	}
	return got
}

func (aw *assistWorld) lead(t *testing.T, who *session.Session) uint32 {
	t.Helper()
	got, ok := aw.w.AssistLeader(who)
	if !ok {
		t.Fatal("no player for the session")
	}
	return got
}

// A player's target is read in its own session's table and answered in the
// asker's: the leader's 100003 is the orc, which the asker calls 100002.
func TestAssistTargetOfAPlayerIsInTheAskersIDs(t *testing.T) {
	aw := newAssistWorld(t)
	aw.b.TargetObjectID = bOrc
	if got := aw.target(t, aw.asker, aLeader); got != aOrc {
		t.Fatalf("assist on the leader: %d, want the orc as the asker knows it (%d)", got, aOrc)
	}
	// the control: the leader's own number, read in the asker's table, is the rat
	if e := aw.w.entitySeenByLocked(aw.asker, bOrc); e != aw.rat {
		t.Fatalf("control: the asker's %d should be the rat, is %v", bOrc, e)
	}
	// a group stand-in names the same player
	if got := aw.target(t, aw.asker, groupFarID(aw.b.ServerID)); got != aOrc {
		t.Fatalf("assist through the stand-in: %d, want %d", got, aOrc)
	}
}

// An NPC's target is its enemy: the asker itself comes back as its own id.
func TestAssistTargetOfAnNPCIsItsEnemy(t *testing.T) {
	aw := newAssistWorld(t)
	aw.orc.CurrentEnemy = aw.b.ServerID
	if got := aw.target(t, aw.asker, aOrc); got != aLeader {
		t.Fatalf("orc fighting the leader: %d, want %d", got, aLeader)
	}
	aw.orc.CurrentEnemy = aw.a.ServerID
	if got := aw.target(t, aw.asker, aOrc); got != askerSelf {
		t.Fatalf("orc fighting the asker: %#x, want the asker's own %#x", got, askerSelf)
	}
	// a dummy's player is DummyTarget, its CurrentEnemy staying 0
	aw.rat.Dummy, aw.rat.DummyTarget = TrainingDummy, aw.b.ServerID
	if got := aw.target(t, aw.asker, aRat); got != aLeader {
		t.Fatalf("dummy fighting the leader: %d, want %d", got, aLeader)
	}
	// a corpse has none
	aw.orc.Dead = true
	if got := aw.target(t, aw.asker, aOrc); got != NoAssist {
		t.Fatalf("a corpse's target: %d, want none", got)
	}
}

// None: no target, a target with none, one the asker cannot see.
func TestAssistTargetNone(t *testing.T) {
	aw := newAssistWorld(t)
	if got := aw.target(t, aw.asker, NoAssist); got != NoAssist {
		t.Fatalf("no target: %d", got)
	}
	if got := aw.target(t, aw.asker, aRat); got != NoAssist {
		t.Fatalf("a rat fighting no one: %d", got)
	}
	aw.b.TargetObjectID = bWolf // the asker has no channel for the wolf
	if got := aw.target(t, aw.asker, aLeader); got != NoAssist {
		t.Fatalf("a target the asker cannot see: %d", got)
	}
	aw.b.TargetObjectID = NoAssist
	if got := aw.target(t, aw.asker, aLeader); got != NoAssist {
		t.Fatalf("the leader let its target go: %d", got)
	}
	if _, ok := aw.w.AssistTarget(session.NewManager().CreateSession(&net.UDPAddr{IP: net.IPv4(10, 0, 0, 3), Port: 7003}, 3, 1, 1, 2, true), aOrc); ok {
		t.Fatal("a session with no player was answered")
	}
}

// Assist Leader: the group's first member's target; none out of a group,
// and the leader gets its own back. It reads a target both ways round (the
// leader's for the asker, then the asker's for the leader), so a lookup over
// every session's table (findEntityByObjectIDLocked), which takes whichever
// session the manager's map gives first, fails it in either order.
func TestAssistLeader(t *testing.T) {
	aw := newAssistWorld(t)
	aw.b.TargetObjectID = bOrc
	if got := aw.lead(t, aw.asker); got != NoAssist {
		t.Fatalf("out of a group: %d", got)
	}
	aw.group()
	if got := aw.lead(t, aw.asker); got != aOrc {
		t.Fatalf("assist the leader: %d, want %d", got, aOrc)
	}
	if got := aw.lead(t, aw.leader); got != bOrc {
		t.Fatalf("the leader asking: %d, want its own %d", got, bOrc)
	}
	// the control: the asker leading instead gives the asker's target
	aw.w.groups[7].Members = []EntityID{aw.a.ServerID, aw.b.ServerID}
	aw.a.TargetObjectID = aRat
	if got := aw.lead(t, aw.leader); got != bRat {
		t.Fatalf("the asker leading: %d, want the rat as the leader knows it (%d)", got, bRat)
	}
}
