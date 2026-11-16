package handlers

import (
	"strings"
	"testing"

	"github.com/eqoa/server/internal/entity"
)

// A reach step whose doing fires a trigger of a lock held is not done while it is held (the Kratas quest's
// last step at the whale bones raises Kratas, one at a time): the player is told, the step stays; the lock
// free, the step is done there and the trigger fires. The control: the first player, the lock free, is done
// at once and raises the creature. Before, the second player's step was done and the trigger did not fire.
func TestAReachStepWaitsForItsTriggersLock(t *testing.T) {
	f := newQuestFixture(t)
	f.tw.p.HP, f.tw.p.MaxHP = 100, 100 // a dead player stands nowhere
	f.tw.w.NPCs[705] = &entity.NPC{ServerID: 705, Name: "Kratas", HP: 100, MaxHP: 100, Level: 57, X: 13476, Y: 23, Z: 31456, World: 0}
	e := NewTriggerEngine(f.db, f.tw.w, f.tw.mgr, f.inv)
	triggerEngine.Store(e)
	t.Cleanup(func() { triggerEngine.CompareAndSwap(e, nil) })
	f.exec(`INSERT INTO gmtriggers (slot, name, world, x, y, z, steps) VALUES (23, 'Kratas Rises', 0, 13476, 23, 31456,
		'[{"do":"lock","lock":"kratas"},{"do":"clock","seconds":3600,"empty":600},{"do":"spawn","npc":"Kratas","hostile":true,"minutes":60},{"do":"awaitdead","npc":"Kratas"},{"do":"end"},{"do":"despawn"}]')`)
	if err := e.Reload(); err != nil {
		t.Fatal(err)
	}
	f.exec(`INSERT INTO quests (questid, name, enabled, giver) VALUES (89, 'Kratas', 1, 'Julaech')`)
	f.exec(`INSERT INTO queststeps (questid, step, kind, world, x, y, z, radius, journal, events) VALUES
		(89, 1, 'reach', 0, 13476, 34, 31456, 40, 'Swim to the whale bones.',
		'[{"on":"done","do":"trigger","trigger":"Kratas Rises","text":"Kratas already stirs among the bones."}]')`)
	f.qe.Reload()

	// the lock held by another's run
	if _, err := e.FireNamed("Kratas Rises", TriggerFirer{Name: "Other", Quiet: true}); err != nil {
		t.Fatal(err)
	}
	if !e.BusyNamed("Kratas Rises") {
		t.Fatal("the first run holds no lock")
	}
	if _, err := f.qe.SetCharacterQuest(f.sid, 89, QuestGive, 0); err != nil {
		t.Fatal(err)
	}
	f.msgs()
	f.tw.w.UpdatePosition(f.tw.p.ServerID, 13480, 30, 31460, 0)
	f.qe.checkReach()
	f.qe.checkReach()
	if s, d := f.progress(89); s != 1 || d != 0 {
		t.Fatalf("the lock held: step %d done %d, want still on step 1", s, d)
	}
	told := 0
	for _, m := range f.msgs() {
		if strings.Contains(m, "Kratas already stirs among the bones.") {
			told++
		}
	}
	if told != 1 {
		t.Errorf("told %d times in two looks, want once", told)
	}

	// the lock free: done there, and its trigger fires for this player
	e.StopRuns(23)
	if e.BusyNamed("Kratas Rises") {
		t.Fatal("the lock still held after its run ended")
	}
	f.qe.checkReach()
	if s, d := f.progress(89); s != 0 || d != 1 {
		t.Fatalf("the lock free: step %d done %d, want the quest done", s, d)
	}
	if !e.BusyNamed("Kratas Rises") {
		t.Error("the step's trigger did not fire: no run holds its lock")
	}
}
