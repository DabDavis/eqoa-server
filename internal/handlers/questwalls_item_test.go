package handlers

import (
	"encoding/binary"
	"fmt"
	"testing"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// errorLines counts the chat error lines (0x00CE) queued since the last read.
func errorLines(s *session.Session) int {
	n := 0
	for _, m := range s.Queue().GetPending() {
		if d := m.Data(); len(d) >= 2 && protocol.GameOpcode(binary.LittleEndian.Uint16(d)) == protocol.OpcodeErrorMessage {
			n++
		}
	}
	return n
}

// The Last House's ward (2026-11-13): a wall of an item is passed by a
// character carrying it, the engine says so to the World; one without it,
// come up against it, is told its line once, again only after it went away.
func TestTheEngineOpensAnItemWallToItsHolder(t *testing.T) {
	f := newQuestFixture(t)
	suit := f.pattern("zombie flesh suit", 1)
	px, pz := f.tw.p.X, f.tw.p.Z
	f.tw.add(5, &entity.NPC{ServerID: 3001, Name: "invisible wall", NPCType: 0x80, ModelID: int64(entity.BlockerBody), SpawnX: px - 6, SpawnY: f.tw.p.Y, SpawnZ: pz})
	f.tw.w.SetQuestWalls([]database.QuestWall{{NPCID: 3001, Item: suit, Text: "The ward's stench turns you back."}})
	f.tw.s.Queue().GetPending()

	f.qe.syncQuestState()
	if got := f.qe.chars[f.sid].qstate.sent; got != "+" {
		t.Fatalf("without the suit the World was told %q, want no wall", got)
	}
	if n := errorLines(f.tw.s); n != 1 {
		t.Fatalf("at the wall without the suit: %d lines, want its line once", n)
	}
	f.qe.syncQuestState()
	if n := errorLines(f.tw.s); n != 0 {
		t.Fatalf("the line said again (%d) while the player stays at the wall", n)
	}
	// gone off and back: told again
	f.tw.w.UpdatePosition(f.tw.p.ServerID, px+40, f.tw.p.Y, pz, 0)
	f.qe.syncQuestState()
	f.tw.w.UpdatePosition(f.tw.p.ServerID, px, f.tw.p.Y, pz, 0)
	f.qe.syncQuestState()
	if n := errorLines(f.tw.s); n != 1 {
		t.Fatalf("back at the wall after going away: %d lines, want 1", n)
	}

	f.own(suit, 1)
	f.qe.syncQuestState()
	if got := f.qe.chars[f.sid].qstate.sent; got != fmt.Sprintf("+i%d,w3001", suit) {
		t.Fatalf("with the suit the World was told %q, want the wall and the item", got)
	}
	if n := errorLines(f.tw.s); n != 0 {
		t.Fatalf("the suit's holder was told %d lines", n)
	}
	if !f.qe.Holds(f.tw.p, suit) || f.qe.Holds(f.tw.p, suit+1) {
		t.Fatal("Holds does not read the pack")
	}
}

// The Oozing Portal (the wiki's Last House portal, 2026-11-13) takes a player
// of level 51 on to the Plane of Disease only with the zombie flesh suit: one
// without it is told so and offered nothing.
func TestTheOozingPortalAsksForTheSuit(t *testing.T) {
	f := newQuestFixture(t)
	f.exec(`UPDATE itempattern SET patternid = ? WHERE patternid = ?`, zombieFleshSuit, f.pattern("zombie flesh suit", 1))
	f.tw.p.Level = podLevel
	portal := f.tw.add(6, &entity.NPC{ServerID: 3002, Name: "Oozing Portal", NPCType: 0x80, SpawnX: f.tw.p.X + 3, SpawnY: f.tw.p.Y, SpawnZ: f.tw.p.Z})
	f.tw.s.Queue().GetPending()
	f.tw.talk(portal)
	if box := boxSaid(f.tw.s); box != wayRoutes["Oozing Portal"][0].Lacks {
		t.Fatalf("without the suit the portal said %q", box)
	}
	f.tw.s.Queue().GetPending()
	f.own(zombieFleshSuit, 1)
	f.tw.talk(portal)
	if got := optionBox(t, f.tw.s); len(got) != 2 || got[0] != "Yes" {
		t.Fatalf("with the suit the portal offered %q, want Yes and No", got)
	}
	f.tw.pick(portal, 0)
	if x, z, world, moved := teleported(f.tw.s); !moved || world != 5 || x != 5063 || z != 3866 {
		t.Fatalf("Yes sent the player to (%.1f, %.1f) world %d (%v)", x, z, world, moved)
	}
}

// The Dead Hills' way into the Plane of Disease (the owner, 2026-11-13: "players
// cannot enter without the flesh suit and completing the quest"): Zizclank and
// the Mausoleum Portal send a player of level 51 on only with the zombie flesh
// suit; one without it is told so and offered nothing.
func TestTheDeadHillsWayToPoDAsksForTheSuit(t *testing.T) {
	for i, name := range []string{"Mausoleum Portal", "Necrologist Zizclank"} {
		f := newQuestFixture(t)
		f.exec(`UPDATE itempattern SET patternid = ? WHERE patternid = ?`, zombieFleshSuit, f.pattern("zombie flesh suit", 1))
		f.tw.p.Level = podLevel
		way := f.tw.add(6, &entity.NPC{ServerID: entity.EntityID(3010 + i), Name: name, NPCType: 0x80, SpawnX: f.tw.p.X + 3, SpawnY: f.tw.p.Y, SpawnZ: f.tw.p.Z})
		f.tw.s.Queue().GetPending()
		f.tw.talk(way)
		if box := boxSaid(f.tw.s); box != wayRoutes[name][0].Lacks {
			t.Fatalf("%s without the suit said %q", name, box)
		}
		f.tw.s.Queue().GetPending()
		f.own(zombieFleshSuit, 1)
		f.tw.talk(way)
		if got := optionBox(t, f.tw.s); len(got) != 2 || got[0] != "Yes" {
			t.Fatalf("%s with the suit offered %q, want Yes and No", name, got)
		}
	}
}

// The Last House Warden (a stone golem) at the Last House's ward (the owner, 2026-11-13) sets
// a player with the zombie flesh suit past the ward; one without it is told so
// and offered nothing.
func TestTheLastHouseWardenLetsTheSuitPass(t *testing.T) {
	f := newQuestFixture(t)
	f.exec(`UPDATE itempattern SET patternid = ? WHERE patternid = ?`, zombieFleshSuit, f.pattern("zombie flesh suit", 1))
	golem := f.tw.add(6, &entity.NPC{ServerID: 3020, Name: "Last House Warden", NPCType: 0x180, SpawnX: f.tw.p.X + 3, SpawnY: f.tw.p.Y, SpawnZ: f.tw.p.Z})
	f.tw.s.Queue().GetPending()
	f.tw.talk(golem)
	if box := boxSaid(f.tw.s); box != wayRoutes["Last House Warden"][0].Lacks {
		t.Fatalf("without the suit the golem said %q", box)
	}
	f.tw.s.Queue().GetPending()
	f.own(zombieFleshSuit, 1)
	f.tw.talk(golem)
	if got := optionBox(t, f.tw.s); len(got) != 2 || got[0] != "Yes" {
		t.Fatalf("with the suit the golem offered %q, want Yes and No", got)
	}
	f.tw.pick(golem, 0)
	if x, z, world, moved := teleported(f.tw.s); !moved || world != 5 || x != 4614 || z != 3604 {
		t.Fatalf("Yes set the player at (%.1f, %.1f) world %d (%v)", x, z, world, moved)
	}
}
