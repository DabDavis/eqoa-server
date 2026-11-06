package entity

import (
	"math"
	"net"
	"testing"
	"time"

	"github.com/eqoa/server/internal/session"
)

func newGenieWorld(t *testing.T) (*World, *session.Session, *Player, time.Time) {
	t.Helper()
	w := NewWorld()
	mgr := session.NewManager()
	w.SessionManager = mgr
	sess := mgr.CreateSession(&net.UDPAddr{IP: net.IPv4(10, 0, 0, 7), Port: 7007}, 1, 1, 1, 2, true)
	p := &Player{ServerID: 1007, Name: "Master", X: 100, Y: 54, Z: 100, Facing: 0, Level: 30, HP: 100, MaxHP: 100}
	w.Players[p.ServerID] = p
	sess.SetECSEntity(p)
	return w, sess, p, time.Unix(1_800_000_000, 0)
}

func genies(w *World) []*NPC {
	var out []*NPC
	for _, n := range w.NPCs {
		if n.GenieOf != 0 {
			out = append(out, n)
		}
	}
	return out
}

// A lamp's genie: GenieAhead in front of its master (facing 0 looks down +z)
// and facing it, the Djinn's body, the NPC bit alone (talked to, never a
// window's flags), never a pet; its master's and no one else's; one at a
// time, a second summons replacing the first.
func TestASummonedGenieStandsBeforeItsMaster(t *testing.T) {
	w, sess, p, now := newGenieWorld(t)
	id, err := w.SummonGenie(sess, 5002, now)
	if err != nil {
		t.Fatal(err)
	}
	g := w.NPCs[id]
	if g == nil || g.GenieOf != p.ServerID || g.Owner != 0 || g.NPCType != 0x80 || g.ModelID != GenieModel || g.NPCID != 5002 ||
		g.Name != GenieName || !g.Temporary {
		t.Fatalf("the genie: %+v", g)
	}
	if math.Abs(float64(g.X-100)) > 1e-4 || math.Abs(float64(g.Z-102.5)) > 1e-4 || math.Abs(float64(g.Facing)-math.Pi) > 1e-4 {
		t.Errorf("at (%.2f, %.2f) facing %.3f; want (100, 102.5) facing pi", g.X, g.Z, g.Facing)
	}
	if genie, mine := w.GenieOf(sess, g); !genie || !mine {
		t.Errorf("GenieOf its master: %v %v", genie, mine)
	}
	other := w.SessionManager.CreateSession(&net.UDPAddr{IP: net.IPv4(10, 0, 0, 8), Port: 7008}, 2, 1, 1, 2, true)
	q := &Player{ServerID: 1008, Name: "Other", X: 100, Z: 100, HP: 1, MaxHP: 1}
	w.Players[q.ServerID] = q
	other.SetECSEntity(q)
	if genie, mine := w.GenieOf(other, g); !genie || mine {
		t.Errorf("GenieOf another: %v %v", genie, mine)
	}
	if genie, _ := w.GenieOf(sess, &NPC{ServerID: 1}); genie {
		t.Error("a plain NPC taken for a genie")
	}
	p.Facing = math.Pi / 2 // now looking down +x
	id2, _ := w.SummonGenie(sess, 5002, now)
	if gs := genies(w); len(gs) != 1 || gs[0].ServerID != id2 || id2 == id {
		t.Fatalf("after a second summons: %d genies", len(gs))
	}
	if g2 := w.NPCs[id2]; math.Abs(float64(g2.X-102.5)) > 1e-4 || math.Abs(float64(g2.Z-100)) > 1e-4 {
		t.Errorf("the second at (%.2f, %.2f); want (102.5, 100)", g2.X, g2.Z)
	}
	// none for the dead
	p.HP = 0
	if _, err := w.SummonGenie(sess, 5002, now); err != ErrGenieNoPlayer {
		t.Errorf("a dead master's summons: %v", err)
	}
}

// A window it opened: the genie leaves the world, and the window is the
// master's: the bank's with any id, the shop's and the forge's with theirs,
// never another kind's; it ends GenieReach from where the genie stood, in
// another world, dead, idle GenieWindowIdle, or closed (the forge).
func TestAGenieWindowIsItsMastersWhileItLasts(t *testing.T) {
	w, sess, p, now := newGenieWorld(t)
	open := func(kind GenieWindow) uint32 {
		t.Helper()
		id, err := w.SummonGenie(sess, 5002, now)
		if err != nil {
			t.Fatal(err)
		}
		wid := GenieWindowID(id)
		if kind == GenieBank {
			wid = 0
		}
		if !w.GenieOpened(sess, kind, wid, now) {
			t.Fatal("not opened")
		}
		if len(genies(w)) != 0 {
			t.Fatal("the genie stays in the world once its window is open")
		}
		return GenieWindowID(id)
	}
	ok := func(kind GenieWindow, id uint32, at time.Time) bool {
		_, ok := w.GenieWindowNPC(sess, kind, id, at)
		return ok
	}
	open(GenieBank)
	if npc, ok := w.GenieWindowNPC(sess, GenieBank, 12345, now); !ok || npc.Name != GenieName || npc.NPCID != 5002 {
		t.Fatalf("the bank's request with CLIENT's target: %+v %v", npc, ok)
	}
	if ok(GenieShop, 12345, now) {
		t.Error("a shop's request to the bank's window")
	}
	// used within the idle time, it holds; past it from the last use, not
	if !ok(GenieBank, 1, now.Add(GenieWindowIdle-time.Second)) || !ok(GenieBank, 1, now.Add(2*GenieWindowIdle-2*time.Second)) {
		t.Error("the bank's window lapsed while used")
	}
	if ok(GenieBank, 1, now.Add(4*GenieWindowIdle)) || p.genie != nil {
		t.Error("the bank's window held past its idle time")
	}

	shop := open(GenieShop)
	if !ok(GenieShop, shop, now) || ok(GenieShop, shop+1, now) || ok(GenieBank, 1, now) {
		t.Error("the shop's window by its id alone")
	}
	p.X += GenieReach + 1
	if ok(GenieShop, shop, now) || p.genie != nil {
		t.Error("the shop's window held away from where the genie stood")
	}
	p.X -= GenieReach + 1

	smith := open(GenieSmith)
	if !ok(GenieSmith, smith, now) {
		t.Fatal("the forge's window")
	}
	w.EndGenieWindow(sess, GenieShop) // another kind's close: kept
	if !ok(GenieSmith, smith, now) {
		t.Fatal("the forge ended by another window's close")
	}
	w.EndGenieWindow(sess, GenieSmith)
	if ok(GenieSmith, smith, now) {
		t.Error("the forge held past its close")
	}

	open(GenieBank)
	p.World = 1
	if ok(GenieBank, 1, now) {
		t.Error("the bank's window held in another world")
	}
	p.World = 0
	open(GenieBank)
	p.HP = 0
	if ok(GenieBank, 1, now) {
		t.Error("the bank's window held for the dead")
	}
}

// A genie no box was answered for waits GenieWait to be talked to, then
// goes (genieSystem); one whose master leaves the world goes with it; one
// whose master is gone any other way is taken away at the next tick.
func TestAGenieWaitsThenGoes(t *testing.T) {
	w, sess, p, now := newGenieWorld(t)
	if _, err := w.SummonGenie(sess, 0, now); err != nil {
		t.Fatal(err)
	}
	w.mu.Lock()
	w.genieSystem(now.Add(GenieWait - time.Second))
	w.mu.Unlock()
	if len(genies(w)) != 1 {
		t.Fatal("gone before its wait")
	}
	w.mu.Lock()
	w.genieSystem(now.Add(GenieWait + time.Second))
	w.mu.Unlock()
	if len(genies(w)) != 0 || p.genie != nil {
		t.Fatal("still there past its wait")
	}
	// its master leaves
	if _, err := w.SummonGenie(sess, 0, now); err != nil {
		t.Fatal(err)
	}
	w.RemovePlayer(p.ServerID)
	if len(genies(w)) != 0 {
		t.Fatal("the genie outlived its master's leaving")
	}
	// a master gone from the map by other means: the tick takes it away
	w.Players[p.ServerID] = p
	if _, err := w.SummonGenie(sess, 0, now); err != nil {
		t.Fatal(err)
	}
	delete(w.Players, p.ServerID)
	w.mu.Lock()
	w.genieSystem(now)
	w.mu.Unlock()
	if len(genies(w)) != 0 || len(w.genies) != 0 {
		t.Fatal("an orphaned genie kept")
	}
}
