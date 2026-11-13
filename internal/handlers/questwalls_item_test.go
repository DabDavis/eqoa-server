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
}

// The ward is also The Last House's (the owner, 2026-11-13: "let flagged
// players through"): a character who finished its quest passes it without
// the item; one under way at its last step, or with no quest, is stopped.
func TestAFlaggedPlayerPassesTheWardWithoutTheSuit(t *testing.T) {
	f := newQuestFixture(t)
	suit := f.pattern("zombie flesh suit", 1)
	px, pz := f.tw.p.X, f.tw.p.Z
	f.tw.add(5, &entity.NPC{ServerID: 3001, Name: "invisible wall", NPCType: 0x80, ModelID: int64(entity.BlockerBody), SpawnX: px - 6, SpawnY: f.tw.p.Y, SpawnZ: pz})
	// quest 10010's last step is 2: past it is finished
	f.tw.w.SetQuestWalls([]database.QuestWall{{NPCID: 3001, QuestID: 10010, Step: 2, Item: suit, Text: "turned back"}})
	f.qe.syncQuestState()
	if got := f.qe.chars[f.sid].qstate.sent; got != "+" {
		t.Fatalf("an unflagged player without the suit: the World was told %q, want no wall passed", got)
	}
	if len(f.tw.w.ItemWallsStopping(f.tw.p, 8)) != 1 {
		t.Fatal("the ward does not stop an unflagged player without the suit")
	}
	if _, err := f.qe.SetCharacterQuest(f.sid, 10010, QuestGive, 2); err != nil {
		t.Fatal(err)
	}
	f.qe.syncQuestState()
	if got := f.qe.chars[f.sid].qstate.sent; got != "+" {
		t.Fatalf("at the quest's last step: the World was told %q, want no wall passed", got)
	}
	if _, err := f.qe.SetCharacterQuest(f.sid, 10010, QuestComplete, 0); err != nil {
		t.Fatal(err)
	}
	f.qe.syncQuestState()
	if got := f.qe.chars[f.sid].qstate.sent; got != "+w3001" {
		t.Fatalf("a flagged player without the suit: the World was told %q, want the ward passed", got)
	}
	if len(f.tw.w.ItemWallsStopping(f.tw.p, 8)) != 0 {
		t.Fatal("the ward stops a flagged player without the suit")
	}
}

// The Oozing Portal's trip on (for one who finished The Last House: its
// unlock) asks no suit (the owner, 2026-11-13: "let flagged players
// through"); at level 51.
func TestTheOozingPortalSendsWithoutTheSuit(t *testing.T) {
	f := newQuestFixture(t)
	portal := f.tw.add(6, &entity.NPC{ServerID: 3002, Name: "Oozing Portal", NPCType: 0x80, SpawnX: f.tw.p.X + 3, SpawnY: f.tw.p.Y, SpawnZ: f.tw.p.Z})
	f.tw.p.Level = podLevel - 1
	f.tw.s.Queue().GetPending()
	f.tw.talk(portal)
	if box := boxSaid(f.tw.s); box != tooWeak("the Plane of Disease") {
		t.Fatalf("a level %d was told %q", podLevel-1, box)
	}
	f.tw.s.Queue().GetPending()
	f.tw.p.Level = podLevel
	f.tw.talk(portal)
	if got := optionBox(t, f.tw.s); len(got) != 2 || got[0] != "Yes" {
		t.Fatalf("without the suit the portal offered %q, want Yes and No", got)
	}
	f.tw.pick(portal, 0)
	if x, z, world, moved := teleported(f.tw.s); !moved || world != 5 || x != 5063 || z != 3866 {
		t.Fatalf("Yes sent the player to (%.1f, %.1f) world %d (%v)", x, z, world, moved)
	}
}
