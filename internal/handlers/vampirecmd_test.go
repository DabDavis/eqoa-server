package handlers

import (
	"slices"
	"testing"
	"time"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// fakeVampires is charvampire in memory.
type fakeVampires struct {
	rows  map[uint32]bool
	calls int
}

func (f *fakeVampires) SetVampire(serverID uint32, on bool, _ database.GMEdit, _ string) (int64, error) {
	f.calls++
	if f.rows[serverID] == on {
		return 0, nil
	}
	f.rows[serverID] = on
	return int64(f.calls), nil
}

// #vampire is a GM's: a player's is answered as an unknown command and makes
// no one a vampire; the GM's makes its character one, kept and in force where
// it stands, says what the sun does to it at the hour its client shows (noon:
// the open sky burns it), and takes it back with off.
func TestVampireIsAGMsAndMakesOne(t *testing.T) {
	c, gs, gp, ps, pp := gmWorld(t)
	store := &fakeVampires{rows: map[uint32]bool{}}
	c.h.SetVampireStore(store)
	noon := int64(session.GameDay)*472_000 + session.GameDay/2
	gs.SetGameClock(noon, time.Now())

	c.say(ps, "#vampire on")
	if got := heard(t, ps); len(got) != 1 || got[0].op != protocol.OpcodeErrorMessage || got[0].text != `Unrecognized command "#vampire".` {
		t.Fatalf("a player's #vampire: %+v", got)
	}
	if pp.Vampire || store.calls != 0 {
		t.Fatal("a player's #vampire made a vampire")
	}

	c.say(gs, "#vampire on")
	want := "[GM] Gm is a vampire under the open sky: the sun takes 50 every 6s (its client shows 12:00, day)"
	if got := texts(heard(t, gs)); !slices.Contains(got, want) {
		t.Fatalf("the GM's #vampire on: %q", got)
	}
	if !gp.Vampire || !store.rows[gp.CharServerID] {
		t.Fatalf("not made a vampire: in the world %v, kept %v", gp.Vampire, store.rows[gp.CharServerID])
	}
	gs.Queue().GetPending()
	c.say(gs, "/vampire")
	if got := texts(heard(t, gs)); len(got) != 1 || got[0] != want {
		t.Fatalf("the GM's /vampire: %q", got)
	}
	gs.Queue().GetPending()
	c.say(gs, "#vampire off")
	if got := texts(heard(t, gs)); !slices.Contains(got, "[GM] Gm is no vampire (its client shows 12:00, day)") {
		t.Fatalf("the GM's #vampire off: %q", got)
	}
	if gp.Vampire || store.rows[gp.CharServerID] {
		t.Fatal("still a vampire")
	}
}
