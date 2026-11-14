package handlers

import (
	"testing"

	"github.com/eqoa/server/internal/entity"
)

// A reach step with a height is done only on its floor (queststeps.height:
// the kappa fortress's floors stand over one another, quest 101026's room by
// the wooden spiral ramp): a player at its x and z a floor below is not
// there; on the floor, within the height of its y, the step is done and its
// done events spawn the foe beside the player. The control: a reach step with
// no height is done at any height, as before the column.
func TestAReachStepWithAHeightWantsItsFloor(t *testing.T) {
	f := newQuestFixture(t)
	f.tw.p.HP, f.tw.p.MaxHP = 100, 100 // a dead player stands nowhere
	f.tw.add(5, &entity.NPC{ServerID: 707, Name: "Shrixiv", MaxHP: 100, SpawnX: 26000, SpawnY: 54.1, SpawnZ: 15000})
	f.exec(`INSERT INTO quests (questid, name, enabled, giver) VALUES (87, 'The Fortress', 1, 'Malsis'), (88, 'The Hill', 1, 'Malsis')`)
	f.exec(`INSERT INTO queststeps (questid, step, kind, world, x, y, z, radius, height, journal, events) VALUES
		(87, 1, 'reach', 0, 25403, 84, 15725, 25, 6, 'Climb to the room of the spiral ramp.', '[{"on":"done","do":"spawn","npc":"Shrixiv","hostile":true}]'),
		(88, 1, 'reach', 0, 25403, 84, 15725, 25, 0, 'Stand anywhere there.', '')`)
	f.exec(`INSERT INTO queststeps (questid, step, kind, target, journal) VALUES (87, 2, 'kill', 'Shrixiv', 'Defeat Shrixiv.')`)
	f.qe.Reload()
	for _, q := range []int{87, 88} {
		if _, err := f.qe.SetCharacterQuest(f.sid, q, QuestGive, 0); err != nil {
			t.Fatal(err)
		}
	}
	f.tw.w.UpdatePosition(f.tw.p.ServerID, 25403, 61, 15725, 0) // two floors down
	f.qe.checkReach()
	if s, _ := f.progress(87); s != 1 {
		t.Fatalf("a floor below: step %d, want still 1", s)
	}
	if s, d := f.progress(88); s != 0 || d != 1 {
		t.Errorf("no height: step %d done %d, want done at any height", s, d)
	}
	f.tw.w.UpdatePosition(f.tw.p.ServerID, 25410, 84.5, 15730, 0) // on the floor
	f.qe.checkReach()
	if s, _ := f.progress(87); s != 2 {
		t.Fatalf("on the floor: step %d, want 2", s)
	}
	spawned := 0
	for _, n := range f.tw.w.NPCs {
		if n.QuestSpawn && n.Name == "Shrixiv" {
			spawned++
		}
	}
	if spawned != 1 {
		t.Errorf("%d copies of Shrixiv beside the player, want 1", spawned)
	}
}
