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
// every session's table (findEntityByObjectIDLocked's until 2026-10-23), which
// takes whichever session the manager's map gives first, fails it in either
// order.
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

// FindEntityByObjectID reads the sender's own table: the asker's 100003 is
// the rat and the leader's the orc, asked both ways round so a search of
// every session's table fails in either order; a stand-in and the sender's
// own id resolve, another session's own id does not.
func TestFindEntityByObjectIDIsTheSendersTable(t *testing.T) {
	aw := newAssistWorld(t)
	for _, c := range []struct {
		who  *session.Session
		id   uint32
		want Entity
	}{
		{aw.asker, aRat, aw.rat},
		{aw.leader, bRat, aw.rat},
		{aw.asker, aOrc, aw.orc},
		{aw.leader, bOrc, aw.orc},
		{aw.asker, aLeader, aw.b},
		{aw.asker, askerSelf, aw.a},
		{aw.leader, groupFarID(aw.a.ServerID), aw.a},
	} {
		got, ok := aw.w.FindEntityByObjectID(c.who, c.id)
		if !ok || got != c.want {
			t.Errorf("%d: %v (found %v), want %v", c.id, got, ok, c.want)
		}
	}
	if got, ok := aw.w.FindEntityByObjectID(aw.asker, leaderSelf); ok {
		t.Errorf("the leader's own id resolved for the asker: %v", got)
	}
	if got, ok := aw.w.FindEntityByObjectID(aw.asker, bWolf); ok {
		t.Errorf("an id only the leader's table has resolved for the asker: %v", got)
	}
}

// A channel's target (bytes 59-62) is the id the session the channel is for
// knows the target by: the leader targets the orc, its own 100003, which is
// the rat to the asker; the asker's channel for the leader says 100002, the
// orc as the asker numbers it (the control: the leader's own id, raw, names
// the rat). An NPC's is its enemy, as each session knows it: the asker's own
// object id on its own channel for the orc, the asker's 100001 on the
// leader's. None: -1 for a player, 0 for an NPC.
func TestChannelTargetIsTheViewersID(t *testing.T) {
	aw := newAssistWorld(t)
	at := func(b []byte) uint32 { return uint32(b[59]) | uint32(b[60])<<8 | uint32(b[61])<<16 | uint32(b[62])<<24 }
	aw.b.TargetObjectID = bOrc
	if got := at(aw.w.serializeEntityForChannel(aw.asker, aw.b.ServerID, aLeader)); got != aOrc {
		t.Fatalf("the leader's target on the asker's channel: %d, want the orc's %d", got, aOrc)
	}
	if bOrc != aRat {
		t.Fatal("the control: the leader's own id for the orc is the rat's to the asker")
	}
	aw.b.TargetObjectID = 0
	if got := at(aw.w.serializeEntityForChannel(aw.asker, aw.b.ServerID, aLeader)); got != NoAssist {
		t.Fatalf("no target: %#x, want -1", got)
	}
	aw.orc.CurrentEnemy = aw.a.ServerID
	if got := at(aw.w.serializeEntityForChannel(aw.asker, aw.orc.ServerID, aOrc)); got != askerSelf {
		t.Fatalf("the orc's enemy on the asker's own channel: %#x, want the asker's own %#x", got, askerSelf)
	}
	if got := at(aw.w.serializeEntityForChannel(aw.leader, aw.orc.ServerID, bOrc)); got != bAsker {
		t.Fatalf("the orc's enemy on the leader's channel: %d, want %d", got, bAsker)
	}
	aw.orc.CurrentEnemy = 0
	if got := at(aw.w.serializeEntityForChannel(aw.asker, aw.orc.ServerID, aOrc)); got != 0 {
		t.Fatalf("an NPC with no enemy: %#x, want 0", got)
	}
}
