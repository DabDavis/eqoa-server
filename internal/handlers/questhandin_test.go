package handlers

import (
	"fmt"
	"strings"
	"testing"

	"github.com/eqoa/server/internal/entity"
)

// handInFixture is a giver of three repeatable quests, a piece each, every
// one of them a single give step at the giver asking its own part and the
// one talisman: as the Plane of Sky's armourers are.
type handInFixture struct {
	*questFixture
	giver                                                 uint32
	talisman, part1, part2, part3, piece1, piece2, piece3 int
}

const (
	handIn1 = 20001
	handIn2 = 20002
	handIn3 = 20003
)

func newHandInFixture(t *testing.T) *handInFixture {
	t.Helper()
	f := &handInFixture{questFixture: newQuestFixture(t)}
	f.giver = f.tw.add(1, &entity.NPC{ServerID: 701, Name: "Stark", SpawnX: 25403.5, SpawnY: 54.1, SpawnZ: 15725.5})
	f.talisman = f.pattern("talisman of the test", 20)
	f.part1, f.part2, f.part3 = f.pattern("part of the helm", 1), f.pattern("part of the tunic", 1), f.pattern("part of the boots", 1)
	f.piece1, f.piece2, f.piece3 = f.pattern("the helm", 1), f.pattern("the tunic", 1), f.pattern("the boots", 1)
	for i, q := range []struct{ id, part, piece int }{{handIn1, f.part1, f.piece1}, {handIn2, f.part2, f.piece2}, {handIn3, f.part3, f.piece3}} {
		name := fmt.Sprintf("Piece %d", i+1)
		f.exec(`INSERT INTO quests (questid, name, enabled, giver, minlevel, maxlevel, classes, repeatable, offer, accept, decline, declined, started)
			VALUES (?, ?, 1, 'Stark', 1, 10, '10', 1, ?, 'I will bring them.', 'Not now.', 'Then not.', 'Bring them.')`,
			q.id, name, "The parts of "+name+"."+"\nWill you make "+name+"?")
		f.exec(`INSERT INTO queststeps (questid, step, kind, target, items, journal, says, reminder, rewarditems)
			VALUES (?, 1, 'give', 'Stark', ?, ?, 'There.', 'Not all of it yet.', ?)`,
			q.id, fmt.Sprintf("%d, %d", f.talisman, q.part), "Bring Stark the parts of "+name+".", fmt.Sprint(q.piece))
	}
	if err := f.qe.Reload(); err != nil {
		t.Fatal(err)
	}
	return f
}

// talk talks to the giver and returns what its client was told.
func (f *handInFixture) talk() []string {
	f.t.Helper()
	f.msgs()
	f.tw.talk(f.giver)
	return f.msgs()
}

// asks is whether what the client was told asks that question.
func asks(msgs []string, question string) bool {
	return strings.Contains(strings.Join(msgs, "\n"), ":"+question+"|")
}

// The piece the player has the parts of is the one offered, whichever it is,
// and doing it hands in those parts and gives that piece, and only those.
func TestAHandInThePlayerHoldsIsOfferedFirst(t *testing.T) {
	f := newHandInFixture(t)
	f.own(f.talisman, 1)
	f.own(f.part3, 1)
	m := f.talk()
	if !asks(m, "Will you make Piece 3?") {
		t.Fatalf("not offered the third piece, whose parts it holds: %q", m)
	}
	f.tw.pick(boxID(m), 0)
	f.msgs()
	if step, _ := f.progress(handIn3); step != 1 {
		t.Fatalf("the third piece's quest is at step %d", step)
	}
	f.want(f.talk(), "box:Stark: There.", "box:"+questFinished)
	if f.held(f.piece3) != 1 || f.held(f.part3) != 0 || f.held(f.talisman) != 0 {
		t.Errorf("held: piece %d, part %d, talisman %d", f.held(f.piece3), f.held(f.part3), f.held(f.talisman))
	}
	if f.held(f.piece1) != 0 || f.held(f.piece2) != 0 {
		t.Error("another piece was given")
	}
}

// A step waiting on its items does not stand in the way of a hand-in the
// player holds, or of another quest's step it can do now.
func TestAStepThatLacksStandsInNoOnesWay(t *testing.T) {
	f := newHandInFixture(t)
	f.own(f.talisman, 1)
	// nothing ready: the first offered; no to it, the second offered and taken
	m := f.talk()
	if !asks(m, "Will you make Piece 1?") {
		t.Fatalf("not offered the first: %q", m)
	}
	f.tw.pick(boxID(m), 1)
	m = f.talk()
	if !asks(m, "Will you make Piece 2?") {
		t.Fatalf("not offered the second: %q", m)
	}
	f.tw.pick(boxID(m), 0)
	f.msgs()
	if step, _ := f.progress(handIn2); step != 1 {
		t.Fatalf("the second piece's quest is at step %d", step)
	}
	// nothing it can do: the reminder, nothing taken
	f.want(f.talk(), "box:Stark: Not all of it yet.")
	if f.held(f.talisman) != 1 {
		t.Error("a talisman was taken for a step that lacks")
	}
	// the third's part now: offered past the step waiting
	f.own(f.part3, 1)
	m = f.talk()
	if !asks(m, "Will you make Piece 3?") {
		t.Fatalf("not offered the third past the step waiting: %q", m)
	}
	f.tw.pick(boxID(m), 0)
	f.msgs()
	// and done past it, the second still in the journal, waiting
	f.want(f.talk(), "box:Stark: There.")
	if f.held(f.piece3) != 1 {
		t.Error("the third piece was not given past the second's step")
	}
	if step, _ := f.progress(handIn2); step != 1 {
		t.Errorf("the second piece's quest is at step %d", step)
	}
}

// A no moves on to the giver's next quest, and round again after the last, so
// a player can read every piece's offer and journal the one it wants; a giver
// of one quest goes on offering it, as ever.
func TestANoMovesOnToTheNextQuest(t *testing.T) {
	f := newHandInFixture(t)
	for _, want := range []string{"Piece 1", "Piece 2", "Piece 3", "Piece 1"} {
		m := f.talk()
		if !asks(m, "Will you make "+want+"?") {
			t.Fatalf("not offered %s: %q", want, m)
		}
		f.tw.pick(boxID(m), 1)
		f.want(f.msgs(), "box:Then not.")
	}
	g := newQuestFixture(t)
	malsis := g.tw.add(1, &entity.NPC{ServerID: 702, Name: "Malsis", SpawnX: 25403.5, SpawnY: 54.1, SpawnZ: 15725.5})
	for i := 0; i < 2; i++ {
		g.msgs()
		g.tw.talk(malsis)
		m := g.msgs()
		if !asks(m, "Will you prove your worth?") {
			t.Fatalf("offer %d: %q", i, m)
		}
		g.tw.pick(boxID(m), 1)
	}
}

// With two pieces' parts and one talisman the player chooses: a no at the
// first offers the second, and a login forgets the no.
func TestANoAtAReadyHandInOffersTheNextReadyOne(t *testing.T) {
	f := newHandInFixture(t)
	f.own(f.talisman, 1)
	f.own(f.part1, 1)
	f.own(f.part3, 1)
	m := f.talk()
	if !asks(m, "Will you make Piece 1?") {
		t.Fatalf("not offered the first: %q", m)
	}
	f.tw.pick(boxID(m), 1)
	m = f.talk()
	if !asks(m, "Will you make Piece 3?") {
		t.Fatalf("not offered the third, past the second it has no part of: %q", m)
	}
	f.tw.pick(boxID(m), 1)
	f.qe.LoggedIn(f.sid)
	if m = f.talk(); !asks(m, "Will you make Piece 1?") {
		t.Fatalf("a login forgets a no: %q", m)
	}
}

// A replacement: a piece made before is offered and made again from its parts.
func TestAPieceIsMadeAgain(t *testing.T) {
	f := newHandInFixture(t)
	for i := 1; i <= 2; i++ {
		f.own(f.talisman, 1)
		f.own(f.part2, 1)
		m := f.talk()
		if !asks(m, "Will you make Piece 2?") {
			t.Fatalf("round %d, not offered: %q", i, m)
		}
		f.tw.pick(boxID(m), 0)
		f.msgs()
		f.want(f.talk(), "box:Stark: There.")
		if _, done := f.progress(handIn2); done != i {
			t.Fatalf("finished %d times, want %d", done, i)
		}
		if f.held(f.piece2) != i {
			t.Fatalf("holds %d of the piece, want %d", f.held(f.piece2), i)
		}
	}
}

// Another class's quest is offered to no one holding its parts, and nothing
// is taken; a step that gives only tunar is no hand-in, so it waits its turn.
func TestAHandInIsItsOwnClasssAndOfItems(t *testing.T) {
	f := newHandInFixture(t)
	f.own(f.talisman, 1)
	f.own(f.part1, 1)
	f.exec(`UPDATE quests SET classes = '0' WHERE questid IN (20001, 20002, 20003)`)
	f.exec(`INSERT INTO quests (questid, name, enabled, giver, minlevel, maxlevel, classes, repeatable, offer, accept, decline)
		VALUES (20004, 'A key', 1, 'Stark', 1, 10, '10', 0, 'Buy a key?', 'Yes.', 'No.')`)
	f.exec(`INSERT INTO queststeps (questid, step, kind, target, tunar, journal) VALUES (20004, 1, 'give', 'Stark', 10, 'Pay Stark.')`)
	f.exec(`INSERT INTO quests (questid, name, enabled, giver, minlevel, maxlevel, classes, repeatable, offer, accept, decline)
		VALUES (20005, 'A hand-in', 1, 'Stark', 1, 10, '10', 0, 'Hand it in?', 'Yes.', 'No.')`)
	f.exec(`INSERT INTO queststeps (questid, step, kind, target, items, journal) VALUES (20005, 1, 'give', 'Stark', ?, 'Hand in the talisman.')`, fmt.Sprint(f.talisman))
	if err := f.qe.Reload(); err != nil {
		t.Fatal(err)
	}
	m := f.talk()
	if asks(m, "Will you make Piece 1?") {
		t.Fatalf("offered another class's quest: %q", m)
	}
	if !asks(m, "Hand it in?") {
		t.Fatalf("the hand-in it holds comes before the key bought for tunar: %q", m)
	}
	if f.held(f.talisman) != 1 || f.held(f.part1) != 1 {
		t.Error("something was taken")
	}
}
