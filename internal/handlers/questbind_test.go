package handlers

import (
	"fmt"
	"testing"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/protocol"
)

// A teleport with bind binds the player where it lands, as a spiritmaster
// binds: the World's bind point (Return Home and a death go there) and the
// charbind row, by the event's NPC (else the quest's giver). A homeland's
// gatekeeper sends a new player to its city for good this way. The control: a
// teleport without bind leaves the bind point as it was.
func TestATeleportThatBinds(t *testing.T) {
	f := newQuestFixture(t)
	f.exec(`INSERT INTO quests (questid, name, enabled, giver, events) VALUES (91, 'North', 1, 'Malsis',
		'[{"on":"accept","do":"teleport","world":0,"x":25500,"y":60,"z":15800}]')`)
	f.exec(`INSERT INTO quests (questid, name, enabled, giver, events) VALUES (92, 'The Long Flight', 1, 'Gatekeeper Skyhame',
		'[{"on":"accept","do":"teleport","world":0,"x":13059.3,"y":54.4,"z":4289.6,"bind":true}]')`)
	f.exec(`INSERT INTO queststeps (questid, step, kind, target, journal) VALUES (91, 1, 'talk', 'Malsis', 'Talk.'), (92, 1, 'talk', 'Malsis', 'Talk.')`)
	f.qe.Reload()
	if _, err := f.qe.SetCharacterQuest(f.sid, 91, QuestGive, 0); err != nil {
		t.Fatal(err)
	}
	f.want(f.msgs(), fmt.Sprintf("op:%04X", uint16(protocol.OpcodeTeleport)))
	if _, bound := f.tw.w.BindOf(f.tw.p.ServerID); bound {
		t.Fatal("the control: a teleport without bind bound the player")
	}
	if _, err := f.qe.SetCharacterQuest(f.sid, 92, QuestGive, 0); err != nil {
		t.Fatal(err)
	}
	f.want(f.msgs(), fmt.Sprintf("op:%04X", uint16(protocol.OpcodeTeleport)))
	b, bound := f.tw.w.BindOf(f.tw.p.ServerID)
	if !bound || b.World != 0 || b.X != 13059.3 || b.Z != 4289.6 || b.By != "Gatekeeper Skyhame" {
		t.Fatalf("bound %v at %+v", bound, b)
	}
	row, ok, err := database.NewSQLiteCharacterStore(f.db).LoadBind(f.sid)
	if err != nil || !ok || row.X != 13059.3 || row.Spiritmaster != "Gatekeeper Skyhame" {
		t.Fatalf("kept %v %+v %v", ok, row, err)
	}
}

// A teleport to "mainland" sends the player to its race's spot on the disc's
// worlds, its start row for its class (MainlandPlace): a homeland's gatekeeper
// sends each class to its own guild. The controls: a to it does not know moves
// no one (never to the world's corner, 0, 0), nor does a race with no row.
func TestATeleportToTheMainland(t *testing.T) {
	f := newQuestFixture(t)
	f.exec(`INSERT INTO defaultcharacter (tclass, race, humType, level, tunar, bankTunar, unusedTP, totalTP, x, y, z, facing, world)
		VALUES (10, 0, 0, 1, 0, 0, 0, 0, 25290, 54.1, 15553, 0, 0), (0, 0, 0, 1, 0, 0, 0, 0, 25487, 61.4, 15547, 0, 0)`)
	f.exec(`INSERT INTO quests (questid, name, enabled, giver, events) VALUES (93, 'Out', 1, 'Gatekeeper Skyhame',
		'[{"on":"accept","do":"teleport","to":"mainland","bind":true}]')`)
	f.exec(`INSERT INTO queststeps (questid, step, kind, target, journal) VALUES (93, 1, 'talk', 'Malsis', 'Talk.')`)
	f.qe.Reload()
	if _, err := f.qe.SetCharacterQuest(f.sid, 93, QuestGive, 0); err != nil {
		t.Fatal(err)
	}
	f.want(f.msgs(), fmt.Sprintf("op:%04X", uint16(protocol.OpcodeTeleport)))
	if f.tw.p.X != 25290 || f.tw.p.Z != 15553 {
		t.Fatalf("a magician sent to %.0f, %.0f, not its guild's row", f.tw.p.X, f.tw.p.Z)
	}
	if b, bound := f.tw.w.BindOf(f.tw.p.ServerID); !bound || b.X != 25290 {
		t.Fatalf("bound %v at %+v", bound, b)
	}
	f.exec(`UPDATE quests SET events = '[{"on":"accept","do":"teleport","to":"elsewhere"}]' WHERE questid = 93`)
	f.qe.Reload()
	if _, err := f.qe.SetCharacterQuest(f.sid, 93, QuestReset, 0); err != nil {
		t.Fatal(err)
	}
	if _, err := f.qe.SetCharacterQuest(f.sid, 93, QuestGive, 0); err != nil {
		t.Fatal(err)
	}
	f.not(f.msgs(), fmt.Sprintf("op:%04X", uint16(protocol.OpcodeTeleport)))
	f.exec(`UPDATE quests SET events = '[{"on":"accept","do":"teleport","to":"mainland","bind":true}]' WHERE questid = 93`)
	f.qe.Reload()
	f.tw.p.Race = 5 // no row
	if _, err := f.qe.SetCharacterQuest(f.sid, 93, QuestReset, 0); err != nil {
		t.Fatal(err)
	}
	if _, err := f.qe.SetCharacterQuest(f.sid, 93, QuestGive, 0); err != nil {
		t.Fatal(err)
	}
	f.not(f.msgs(), fmt.Sprintf("op:%04X", uint16(protocol.OpcodeTeleport)))
}
