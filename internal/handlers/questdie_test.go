package handlers

import (
	"testing"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
)

// A step's die event (Live petition 5638: Delwin Stitchfinger plays no death
// when he eats Ilenar's poisoned chocolates) lays the step's target nearest the
// player down dead: its death played, a corpse of nothing, no one's kill. The
// controls: a Delwin 200 off and an NPC of another name beside the player stay
// alive; a die on a quest (not a step) and a misspelled action are refused.
func TestADieEventKillsTheNearestTarget(t *testing.T) {
	f := newQuestFixture(t)
	f.tw.p.HP, f.tw.p.MaxHP = 100, 100
	f.tw.add(5, &entity.NPC{ServerID: 801, Name: "Delwin Stitchfinger", NPCType: 0x80, Level: 10, MaxHP: 100, HP: 100, SpawnX: 25405, SpawnY: 54.1, SpawnZ: 15727})
	f.tw.add(6, &entity.NPC{ServerID: 802, Name: "Delwin Stitchfinger", NPCType: 0x80, Level: 10, MaxHP: 100, HP: 100, SpawnX: 25600, SpawnY: 54.1, SpawnZ: 15727})
	f.tw.add(7, &entity.NPC{ServerID: 803, Name: "Fritz Belgor", NPCType: 0x80, Level: 10, MaxHP: 100, HP: 100, SpawnX: 25406, SpawnY: 54.1, SpawnZ: 15727})
	f.exec(`INSERT INTO quests (questid, name, enabled, giver) VALUES (82, 'Assassination', 1, 'Ilenar Crelwin')`)
	f.exec(`INSERT INTO queststeps (questid, step, kind, target, journal, events) VALUES (82, 1, 'talk', 'Delwin Stitchfinger', 'Chocolates.',
		'[{"on":"done","do":"die"}]')`)
	f.exec(`INSERT INTO queststeps (questid, step, kind, target, journal) VALUES (82, 2, 'talk', 'Ilenar Crelwin', 'Back.')`)
	f.qe.Reload()
	if _, err := f.qe.SetCharacterQuest(f.sid, 82, QuestGive, 0); err != nil {
		t.Fatal(err)
	}
	if f.tw.w.NPCs[801].Dead {
		t.Fatal("Delwin dead before the step was done")
	}
	f.tw.talk(f.tw.at(5, 801)) // the step done before him
	d := f.tw.w.NPCs[801]
	if !d.Dead || d.HP != 0 {
		t.Fatalf("Delwin beside the player: dead %v, HP %d", d.Dead, d.HP)
	}
	if !d.CorpseOfNothing() {
		t.Error("his corpse carries loot")
	}
	if f.tw.w.NPCs[802].Dead {
		t.Error("the control: a Delwin 200 off died too")
	}
	if f.tw.w.NPCs[803].Dead {
		t.Error("the control: Fritz beside him died")
	}

	if p := database.EventProblems(`[{"on":"finish","do":"die","npc":"Delwin Stitchfinger"}]`, false); len(p) == 0 {
		t.Error("a quest's die was taken")
	}
	if p := database.EventProblems(`[{"on":"done","do":"die"}]`, true); len(p) != 0 {
		t.Errorf("a step's die refused: %v", p)
	}
	if p := database.EventProblems(`[{"on":"done","do":"dies"}]`, true); len(p) == 0 {
		t.Error("the control: an unknown action was taken")
	}
}
