package handlers

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/eqoa/server/internal/ground"
	"github.com/eqoa/server/internal/protocol"
)

// Norach's trips to the Isle of Dread (the owner, 2026-11-15: "players are not
// always porting onto the raft from Norrach"). The first trip is his quest's
// hand-in (The Medallion of the Faceless), a quest teleport; every later one is
// his waysender trip (iodRaft). The hand-in landed in Graumord, 300 north of
// the raft, so a player's first trip never reached the raft.

// A teleport event with a facing lands with that heading, as a waysender's
// trip does; without one the player keeps its own. The control: the place is
// the event's in both.
func TestATeleportWithAFacing(t *testing.T) {
	f := newQuestFixture(t)
	f.exec(`INSERT INTO quests (questid, name, enabled, giver, events) VALUES (96, 'Raft', 1, 'Malsis',
		'[{"on":"accept","do":"teleport","world":0,"x":24578,"y":52.6,"z":3626,"facing":3.9}]')`)
	f.exec(`INSERT INTO quests (questid, name, enabled, giver, events) VALUES (97, 'Shore', 1, 'Malsis',
		'[{"on":"accept","do":"teleport","world":0,"x":24500,"y":54.1,"z":3330}]')`)
	f.exec(`INSERT INTO queststeps (questid, step, kind, target, journal) VALUES (96, 1, 'talk', 'Malsis', 'Talk.'), (97, 1, 'talk', 'Malsis', 'Talk.')`)
	f.qe.Reload()
	f.tw.p.Facing = 1.25
	if _, err := f.qe.SetCharacterQuest(f.sid, 97, QuestGive, 0); err != nil {
		t.Fatal(err)
	}
	f.want(f.msgs(), fmt.Sprintf("op:%04X", uint16(protocol.OpcodeTeleport)))
	if f.tw.p.X != 24500 || f.tw.p.Z != 3330 || f.tw.p.Facing != 1.25 {
		t.Fatalf("the control: no facing landed at %.0f, %.0f facing %.3f, want 24500, 3330 facing 1.25", f.tw.p.X, f.tw.p.Z, f.tw.p.Facing)
	}
	if _, err := f.qe.SetCharacterQuest(f.sid, 96, QuestGive, 0); err != nil {
		t.Fatal(err)
	}
	f.want(f.msgs(), fmt.Sprintf("op:%04X", uint16(protocol.OpcodeTeleport)))
	if f.tw.p.X != 24578 || f.tw.p.Z != 3626 || f.tw.p.Facing != 3.9 {
		t.Fatalf("landed at %.0f, %.0f facing %.3f, want 24578, 3626 facing 3.9", f.tw.p.X, f.tw.p.Z, f.tw.p.Facing)
	}
}

// The quest's source (tools/quests/data/portal.json, what import_quests.py
// writes into queststeps) lands its hand-in where Norach's trip and the
// Teleporter's Isle of Dread land: the raft, facing the pier. Its unlock is
// Norach's trip, the one with that landing.
func TestNorachsHandInLandsOnTheRaft(t *testing.T) {
	b, err := os.ReadFile(filepath.Join("..", "..", "tools", "quests", "data", "portal.json"))
	if err != nil {
		t.Fatal(err)
	}
	var src struct {
		Quests []struct {
			Name    string `json:"name"`
			Giver   string `json:"giver"`
			Unlocks []struct {
				NPC string `json:"npc"`
				To  string `json:"to"`
			} `json:"unlocks"`
			Steps []struct {
				Kind   string `json:"kind"`
				Target string `json:"target"`
				Events []struct {
					On     string  `json:"on"`
					Do     string  `json:"do"`
					World  int     `json:"world"`
					X      float32 `json:"x"`
					Y      float32 `json:"y"`
					Z      float32 `json:"z"`
					Facing float32 `json:"facing"`
				} `json:"events"`
			} `json:"steps"`
		} `json:"quests"`
	}
	if err := json.Unmarshal(b, &src); err != nil {
		t.Fatal(err)
	}
	var trip *wayRoute
	for i := range wayRoutes["Norach"] {
		if wayRoutes["Norach"][i].To == "the Isle of Dread" {
			trip = &wayRoutes["Norach"][i]
		}
	}
	if trip == nil || trip.At == nil || *trip.At != iodRaft {
		t.Fatalf("Norach's trip to the Isle of Dread is %+v, want one landing on iodRaft", trip)
	}
	found := false
	for _, pl := range telePlaces {
		if pl.label == "Isle of Dread" {
			found = true
			if pl.at != iodRaft {
				t.Errorf("the Teleporter's Isle of Dread lands at %+v, Norach's trip at %+v", pl.at, iodRaft)
			}
		}
	}
	if !found {
		t.Error("the Teleporter has no Isle of Dread")
	}
	teleports := 0
	for _, q := range src.Quests {
		if q.Name != "The Medallion of the Faceless" {
			continue
		}
		if q.Giver != "Norach" || len(q.Unlocks) != 1 || q.Unlocks[0].NPC != "Norach" || q.Unlocks[0].To != trip.To {
			t.Errorf("the quest is %q's, unlocking %+v; want Norach's, unlocking his trip to %s", q.Giver, q.Unlocks, trip.To)
		}
		for _, st := range q.Steps {
			for _, e := range st.Events {
				if e.Do != "teleport" {
					continue
				}
				teleports++
				if st.Kind != "give" || st.Target != "Norach" || e.On != "done" {
					t.Errorf("a teleport on %s of a %s step at %s: want the hand-in's, on done", e.On, st.Kind, st.Target)
				}
				got := wayPlace{World: uint8(e.World), X: e.X, Y: e.Y, Z: e.Z, Facing: e.Facing}
				if got != iodRaft {
					t.Errorf("the hand-in lands at %+v, Norach's trip at %+v", got, iodRaft)
				}
			}
		}
	}
	if teleports != 1 {
		t.Fatalf("%d teleports in The Medallion of the Faceless, want the hand-in's one", teleports)
	}
}

// The raft over the baked ground (EQOA_GROUND_DIR, world 0's files; skipped
// without): the landing is 0.2 or less over the deck, a placed floor, and the
// deck is under every point within 2 of it in x and z (the raft lies at a
// slant: its north-east edge is 3 off), CLIENT's Respawn's step of 1 in x and z
// off its first hit (viclient.cpp Respawn) among them; the sea floor is well
// under the deck beside it.
func TestTheRaftIsUnderItsLanding(t *testing.T) {
	dir := os.Getenv("EQOA_GROUND_DIR")
	if dir == "" {
		t.Skip("EQOA_GROUND_DIR names a baked ground")
	}
	g, err := ground.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(g.Close)
	r := iodRaft
	deck, ok := g.PlacedSupport(r.World, r.X, r.Y, r.Z, 0.5, 3)
	if !ok || r.Y-deck < 0 || r.Y-deck > 0.2 {
		t.Fatalf("the landing's height %.2f over a placed floor at %.2f (%v): want 0 to 0.2 over the deck", r.Y, deck, ok)
	}
	for dx := float32(-2); dx <= 2; dx++ {
		for dz := float32(-2); dz <= 2; dz++ {
			if h, ok := g.PlacedSupport(r.World, r.X+dx, r.Y, r.Z+dz, 0.5, 3); !ok || h < deck-0.3 || h > deck+0.3 {
				t.Errorf("at %+.0f, %+.0f off the landing the deck is %.2f (%v), at the landing %.2f", dx, dz, h, ok, deck)
			}
		}
	}
	if sea, ok := g.Support(r.World, r.X+20, r.Y, r.Z, 0.5, 60); !ok || sea > deck-10 {
		t.Errorf("20 east of the raft the floor is at %.2f (%v): want the sea floor, well under the deck", sea, ok)
	}
}
