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
