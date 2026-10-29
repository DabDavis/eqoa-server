package entity

import (
	"encoding/binary"
	"net"
	"testing"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// The wiki's brackets, each level's points given as it is left; the totals a
// level is due with the create screen's 20. The control: "reaching a level
// gives its bracket's" is another table (60: 493).
func TestTrainingPointsByLevel(t *testing.T) {
	for l, want := range map[int]int{1: 3, 14: 3, 15: 5, 29: 5, 30: 9, 44: 9, 45: 14, 59: 14, 60: 0} {
		if got := TrainingPerLevel(l); got != want {
			t.Errorf("leaving level %d: %d, want %d", l, got, want)
		}
	}
	for level, want := range map[int]int{1: 20, 2: 23, 15: 62, 16: 67, 30: 137, 45: 272, 60: 482} {
		if got := TrainingPointsDue(level); got != want {
			t.Errorf("level %d is due %d, want %d", level, got, want)
		}
	}
	if reached := CreationTrainingPoints + trainingReached(60); reached != 493 || reached == TrainingPointsDue(60) {
		t.Fatalf("the control: the other reading gives %d, must be 493", reached)
	}
}

// trainingReached is the other reading: reaching level l gives l's bracket's.
func trainingReached(level int) int {
	n := 0
	for l := 2; l <= level; l++ {
		switch {
		case l <= 14:
			n += 3
		case l <= 29:
			n += 5
		case l <= 44:
			n += 9
		default:
			n += 14
		}
	}
	return n
}

// SOE's own characters, with nothing unspent: base stats 465 (Corsten, 60)
// and 256 (Lostologist, 45) over their creation. The levels-left reading
// (462, 252) is nearer than the levels-reached one (473, 263).
func TestTrainingPointsAgreeWithSOEsCharacters(t *testing.T) {
	for _, c := range []struct{ level, soe int }{{60, 465}, {45, 256}} {
		left := TrainingPointsDue(c.level) - CreationTrainingPoints
		if abs(left-c.soe) >= abs(trainingReached(c.level)-c.soe) {
			t.Errorf("level %d: left %d, reached %d, SOE's %d: left must be nearer", c.level, left, trainingReached(c.level), c.soe)
		}
	}
}

func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

// A character is made up to its level's points once: made at 50, set down
// to 10 (it keeps them), back up to 11 (nothing again), then on to 51 (only
// 50's 14). A row that holds none counts as having had the create screen's.
func TestTrainingPointsAreNeverGivenTwice(t *testing.T) {
	unused, total, give := TopUpTraining(50, 20, 20)
	if want := TrainingPointsDue(50) - 20; give != want || unused != 20+want || total != TrainingPointsDue(50) {
		t.Fatalf("made at 50: gave %d (%d unspent, %d in all), want %d", give, unused, total, want)
	}
	unused -= 100 // spent
	for _, level := range []int{10, 11, 50} {
		u, tot, g := TopUpTraining(level, unused, total)
		if g != 0 || u != unused || tot != total {
			t.Errorf("at %d: gave %d", level, g)
		}
	}
	if _, tot, g := TopUpTraining(51, unused, total); g != 14 || tot != TrainingPointsDue(51) {
		t.Errorf("on to 51: gave %d, want 50's 14", g)
	}
	if u, tot, g := TopUpTraining(1, 0, 0); g != 0 || u != 0 || tot != 20 {
		t.Errorf("a level 1 row of none: %d unspent, %d in all, gave %d", u, tot, g)
	}
}

// A level gained gives the level it left's points, told as CLIENT reads
// them (0x0036: a byte, the unspent points zigzag). The control: a kill
// that gains no level gives none.
func TestALevelGainedGivesItsTrainingPoints(t *testing.T) {
	w := NewWorld()
	mgr := session.NewManager()
	w.SessionManager = mgr
	s := mgr.CreateSession(&net.UDPAddr{IP: net.IPv4(10, 0, 0, 7), Port: 7102}, 1, 1, 1, 2, true)
	p := &Player{ServerID: 1002, Name: "Trainee", Level: 14, XPInLevel: CharXPDict[14] - 10,
		UnusedTP: 5, TotalTP: TrainingPointsDue(14)}
	w.Players[p.ServerID] = p
	s.SetECSEntity(p)
	s.SetCharacterInWorld(true)

	told := func() (int, bool) {
		for _, m := range s.Queue().PeekPending() {
			d := m.Data()
			if protocol.GameOpcode(binary.LittleEndian.Uint16(d)) == protocol.OpcodeTrainingPoints {
				v, n := binary.Varint(d[3:])
				return int(v), n > 0
			}
		}
		return 0, false
	}
	w.awardXP(s, p, 100)
	if p.Level != 15 || p.UnusedTP != 8 || p.TotalTP != TrainingPointsDue(15) {
		t.Fatalf("level %d, %d unspent, %d in all; want 15, 8, %d", p.Level, p.UnusedTP, p.TotalTP, TrainingPointsDue(15))
	}
	if v, ok := told(); !ok || v != 8 {
		t.Errorf("0x0036: %d (%v), want 8", v, ok)
	}
	s.Queue().GetPending()
	w.awardXP(s, p, 10)
	if _, ok := told(); ok || p.UnusedTP != 8 {
		t.Error("the control: no level, no training points")
	}
}

// A training reset of a player in the world takes the points off its base
// stats and adds them to its unspent ones, the total never below them; a
// base the tp columns would take below its race's and class's (a hand's edit
// of the columns) stops there, as its next login would make it. The control:
// a character not in the world is not found.
func TestTakeBackTraining(t *testing.T) {
	w := NewWorld()
	p := w.CreatePlayer(nil, &database.CharacterRecord{ServerID: 9, CharName: "Tess", Race: 3, TClass: 13, Level: 30,
		TPStrength: 4, TPWisdom: 6, UnusedTP: 2, TotalTP: 12})
	bs := LookupBaseStats(3, 13)
	if p.BaseStr != bs.Str+4 || p.BaseWis != bs.Wis+6 {
		t.Fatalf("the login's base: STR %d WIS %d", p.BaseStr, p.BaseWis)
	}
	before, after, total, ok := w.TakeBackTraining(9, [7]int{4, 0, 0, 0, 9, 0, 0})
	if !ok || before != 2 || after != 15 || total != 15 {
		t.Fatalf("unspent %d -> %d of %d, %v", before, after, total, ok)
	}
	if p.BaseStr != bs.Str || p.BaseWis != bs.Wis || p.UnusedTP != 15 || !p.StatsDirty {
		t.Errorf("after: STR %d (want %d), WIS %d (want %d), unspent %d", p.BaseStr, bs.Str, p.BaseWis, bs.Wis, p.UnusedTP)
	}
	if _, _, _, ok := w.TakeBackTraining(10, [7]int{1}); ok {
		t.Error("the control: character 10 is in no world")
	}
}
