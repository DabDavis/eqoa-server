package entity

import (
	"bytes"
	"reflect"
	"testing"
	"time"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/protocol"
)

// What a logout keeps of a player's lockouts (charlockouts): each still
// running, by spell, when it ends; a spell locked out twice (a recast in the
// tick its last lockout ended, before castSystem let that one go) once, with
// the later end; one of a spell the book has no more too (bookchange.go keeps
// it). One over, or over at that very moment, is not kept. No player: nothing.
func TestSavedLockoutsAreTheOnesStillRunning(t *testing.T) {
	c := newCaster(t)
	now := c.at(0)
	c.p.Lockouts = []castLockout{
		{spellID: 901, displayIndex: 3, until: now.Add(10 * time.Minute)},
		{spellID: 902, displayIndex: 4, until: now.Add(-time.Second)},
		{spellID: 903, displayIndex: -1, until: now.Add(time.Minute)},
		{spellID: 901, displayIndex: 3, until: now.Add(2 * time.Minute)},
		{spellID: 904, displayIndex: 5, until: now},
	}
	rows, ok := c.w.SavedLockouts(c.p.ServerID, now)
	want := []database.LockoutRow{{SpellID: 901, Until: now.Add(10 * time.Minute)}, {SpellID: 903, Until: now.Add(time.Minute)}}
	if !ok || !reflect.DeepEqual(rows, want) {
		t.Fatalf("kept %+v (%v), want %+v", rows, ok, want)
	}
	if rows, ok := c.w.SavedLockouts(4242, now); ok || rows != nil {
		t.Fatalf("no player kept %+v (%v)", rows, ok)
	}
}

// A login's lockouts (RestoreLockouts): one still running refuses its spell
// with what is left of it (the cast's own answer, "Recovery time for that
// ability has not been met (599.0 s)" a second in) and is told after the dump
// on the ability the book has its spell at now (SendLockouts: 0x00CD, 600000
// ms), and its end is said there; one of a spell the book lacks refuses its
// spell, is told on nothing and ends saying nothing; one over by the login is
// not put back, and its spell is cast (the control).
func TestRestoredLockoutsRefuseAndShowOnTheBook(t *testing.T) {
	c := newCaster(t)
	now := c.at(0)
	heal, burst := minorHeal(), strike()
	gone := heal
	gone.SpellID = 777 // a spell the book has no more
	n := c.w.RestoreLockouts(c.sess, []database.LockoutRow{
		{SpellID: heal.SpellID, Until: now.Add(10 * time.Minute)},
		{SpellID: burst.SpellID, Until: now.Add(-time.Second)},
		{SpellID: gone.SpellID, Until: now.Add(time.Minute)},
	}, map[int]int{heal.SpellID: 5, burst.SpellID: 12}, now)
	if n != 2 || len(c.p.Lockouts) != 2 {
		t.Fatalf("put back %d, the player has %+v; want the heal's and 777's", n, c.p.Lockouts)
	}

	// after the dump: the heal's badge on ability 5, 600000 ms; none for 777
	if told := c.w.SendLockouts(c.sess, now); told != 1 {
		t.Fatalf("%d lockouts told, want 1", told)
	}
	msgs := c.sent()
	wantOps(t, msgs, protocol.OpcodeCooldownTimer)
	r := &vims{b: msgs[0][7:]}
	if msgs[0][2] != 5 || r.signed() != 600000 {
		t.Fatalf("0x00CD % X: want ability 5 with 600000 ms", msgs[0])
	}

	// the heal and 777 refused with what is left, nothing taken
	power := c.p.Power
	if c.w.StartCast(c.sess, heal, c.me, now.Add(time.Second)) {
		t.Fatal("the heal was cast inside the lockout its login brought back")
	}
	msgs = c.sent()
	wantOps(t, msgs, protocol.OpcodeColoredChat, protocol.OpcodeCastingCombat)
	if !bytes.Contains(msgs[0], utf16le(" (599.0 s)")) {
		t.Errorf("the refusal % X, want the 599.0 s left", msgs[0])
	}
	if c.w.StartCast(c.sess, gone, c.me, now.Add(3*time.Second)) {
		t.Fatal("a spell out of the book was cast inside its lockout")
	}
	c.sent()
	if c.p.Power != power {
		t.Fatalf("a refused cast took power: %d -> %d", power, c.p.Power)
	}
	// the control: the strike, whose lockout was over by the login, is cast
	if !c.w.StartCast(c.sess, burst, c.npcO, now.Add(4*time.Second)) {
		t.Fatal("a spell whose lockout ran out before the login was refused")
	}
	c.sent()

	// 777's end is said to no ability; the heal's on ability 5
	c.tick(time.Minute + time.Second)
	for _, m := range c.sent() {
		if opOfMsg(m) == protocol.OpcodeCooldownTimer && m[2] != 12 { // the strike's own end, 5.3 s on, is ability 12's
			t.Errorf("a lockout of no ability said its end: % X", m)
		}
	}
	c.tick(10*time.Minute + time.Second)
	var ends []byte
	for _, m := range c.sent() {
		if opOfMsg(m) == protocol.OpcodeCooldownTimer {
			ends = append(ends, m[2])
			if r := (&vims{b: m[7:]}); r.signed() != 0 {
				t.Errorf("the end % X is no 0", m)
			}
		}
	}
	if !bytes.Equal(ends, []byte{5}) || len(c.p.Lockouts) != 0 {
		t.Fatalf("ends said on abilities %v, %d lockouts left; want ability 5's alone", ends, len(c.p.Lockouts))
	}
}
