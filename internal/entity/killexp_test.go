package entity

import (
	"encoding/binary"
	"net"
	"testing"

	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// The game's own experience model (killexp.go, CLIENT viexperience.cpp):
// Sandstorm's one measured kill is exactly it (a level 2's solo kill of a
// level 2 Orc Pawn: 0x0020 from 6 to 380, 374 = LevelExp(2)); the control is
// the formula this server had, a creature's level x 100 (200 there). The
// level table is CLIENT's ExpForLevel, its upper branch from level 51 (the
// C# table's 189,027,675 at 50 was the lower one's).
func TestAKillIsWorthTheGamesOwnExperience(t *testing.T) {
	if got := groupExp(2, 2, []int{2}); got != 374 {
		t.Fatalf("a level 2's solo kill of a level 2: %d, want Sandstorm's 374", got)
	}
	if old := 2 * 100; old == groupExp(2, 2, []int{2}) {
		t.Fatal("the control: the old level x 100 would match Sandstorm too")
	}
	if ExpForLevel(2) != 424 || CharXPDict[1] != 424 {
		t.Errorf("level 1 to 2: ExpForLevel %d, table %d, want 424", ExpForLevel(2), CharXPDict[1])
	}
	if CharXPDict[50] != 181731870 || CharXPDict[49] != ExpForLevel(50) || CharXPDict[49] != 174625000 {
		t.Errorf("levels 49 and 50 take %d and %d, want 174,625,000 and 181,731,870 (51's upper branch)", CharXPDict[49], CharXPDict[50])
	}
	for l := 1; l < 60; l++ {
		if CharXPDict[l] != ExpForLevel(l+1) {
			t.Errorf("level %d takes %d, CLIENT's bar %d", l, CharXPDict[l], ExpForLevel(l+1))
		}
	}

	// a level 60 on a level 63 alone (jib's report): LevelExp(63) x 1.15,
	// three mastery points' worth in one kill
	if got := groupExp(63, 60, []int{60}); got != 425820 {
		t.Errorf("a level 60's kill of a level 63: %d, want 425,820", got)
	}
	// grey: below three quarters of the highest level, nothing
	if groupExp(44, 60, []int{60}) != 0 || groupExp(45, 60, []int{60}) == 0 {
		t.Errorf("a level 60 on 44 and 45: %d and %d, want none and some", groupExp(44, 60, []int{60}), groupExp(45, 60, []int{60}))
	}
	// at most three times a member's own level's worth alone: a level 10 on
	// a level 20, 37,044 by the kill, held to 3 x 5,808
	if got := groupExp(20, 10, []int{10}); got != 17424 {
		t.Errorf("a level 10's kill of a level 20: %d, want the cap 17,424", got)
	}
	// a group: the kill worth 1.8 for two, shared by level squared; a member
	// below three quarters of the highest (six under, when nearer) gets none
	if a, b := groupExp(60, 60, []int{60, 60}), groupExp(60, 60, []int{60}); a != 294703 || b != 327448 {
		t.Errorf("two level 60s on a level 60: %d each, alone %d; want 294,703 and 327,448", a, b)
	}
	if got := groupExp(60, 30, []int{60, 30}); got != 0 {
		t.Errorf("a level 30 grouped with a level 60: %d, want none", got)
	}
}

// killWorld is a world with players in a group, each with its session.
type killWorld struct {
	w       *World
	players []*Player
	sess    []*session.Session
}

func newKillWorld(t *testing.T, levels ...int) *killWorld {
	t.Helper()
	k := &killWorld{w: NewWorld()}
	mgr := session.NewManager()
	k.w.SessionManager = mgr
	g := &Group{ID: 9}
	for i, l := range levels {
		s := mgr.CreateSession(&net.UDPAddr{IP: net.IPv4(10, 0, 1, byte(i+1)), Port: 7200 + i}, uint32(i+1), 1, 1, 2, true)
		p := &Player{ServerID: EntityID(1001 + i), Name: "Sharer", Level: l, HP: 100, MaxHP: 100, X: 1000, Z: 1000}
		if len(levels) > 1 {
			p.GroupID = g.ID
			g.Members = append(g.Members, p.ServerID)
		}
		k.w.Players[p.ServerID] = p
		s.SetECSEntity(p)
		s.SetCharacterInWorld(true)
		k.players = append(k.players, p)
		k.sess = append(k.sess, s)
	}
	if len(levels) > 1 {
		if k.w.groups == nil {
			k.w.groups = map[uint32]*Group{}
		}
		k.w.groups[g.ID] = g
	}
	return k
}

// sentOf is what a session was sent: its chat lines and whether a 0x1402
// (mastery points) went.
func sentOf(s *session.Session) (lines []string, points bool) {
	for _, m := range s.Queue().GetPending() {
		d := m.Data()
		switch protocol.GameOpcode(binary.LittleEndian.Uint16(d)) {
		case protocol.OpcodeClientMessage:
			lines = append(lines, chatText(d))
		case protocol.OpcodeClassMasteryServer:
			points = true
		}
	}
	return lines, points
}

// A kill's experience goes to the killer and the group's members in its
// world within KillShareRange, each GroupExp's share of the members there;
// one away (or in another world) gets none and counts for none.
func TestAKillIsSharedWithTheGroupNear(t *testing.T) {
	k := newKillWorld(t, 60, 60, 60)
	near, far := k.players[1], k.players[2]
	far.X += KillShareRange + 50
	npc := &NPC{ServerID: 5001, Name: "Froglok", Level: 60, X: 1000, Z: 1000}

	k.w.awardKillXPLocked(k.sess[0], k.players[0], npc)
	want := groupExp(60, 60, []int{60, 60})
	if k.players[0].TotalXP != want || near.TotalXP != want {
		t.Fatalf("killer %d, member near %d, want %d each (two sharing)", k.players[0].TotalXP, near.TotalXP, want)
	}
	if far.TotalXP != 0 {
		t.Errorf("the member %v off got %d", KillShareRange+50, far.TotalXP)
	}

	// the control: alone, the whole of it
	solo := newKillWorld(t, 60)
	solo.w.awardKillXPLocked(solo.sess[0], solo.players[0], npc)
	if solo.players[0].TotalXP != groupExp(60, 60, []int{60}) || solo.players[0].TotalXP == want {
		t.Errorf("alone %d, grouped %d", solo.players[0].TotalXP, want)
	}
}

// jib's report: a level 60 with the whole share on masteries gets three
// points from one kill of a level 63 (425,820: the first three cost
// 125,000, 125,763 and 126,530), told by 0x1402 and Sandstorm's one line,
// "You received N class mastery XP."; the control is no share: the level's
// line and no point.
func TestALevel60KillMakesAMasteryPoint(t *testing.T) {
	k := newKillWorld(t, 60)
	p := k.players[0]
	p.Mastery = Mastery{Percent: 100}
	npc := &NPC{ServerID: 5002, Name: "Froglok Slayer", Level: 63, X: 1000, Z: 1000}

	k.w.awardKillXPLocked(k.sess[0], p, npc)
	lines, told := sentOf(k.sess[0])
	left := 425820 - CMPointXP(1) - CMPointXP(2) - CMPointXP(3)
	if p.Mastery.Points != 3 || p.Mastery.XP != left || left != 48527 || !told {
		t.Fatalf("mastery %+v, told %v; want three points and 48,527 toward the fourth", p.Mastery, told)
	}
	if len(lines) != 1 || lines[0] != "You received 425820 class mastery XP." {
		t.Errorf("the chat: %q, want Sandstorm's one line", lines)
	}

	p.Mastery = Mastery{}
	k.w.awardKillXPLocked(k.sess[0], p, npc)
	lines, told = sentOf(k.sess[0])
	if told || p.Mastery.Points != 0 || len(lines) != 1 || lines[0] != "You receive 425820 XP." {
		t.Errorf("the control, no share: %q, told %v, mastery %+v", lines, told, p.Mastery)
	}
}
