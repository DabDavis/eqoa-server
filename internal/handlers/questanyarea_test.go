package handlers

import (
	"strconv"
	"testing"
	"time"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
)

// killAt is a kill of name at (x, z) in world credited to the fixture's character.
func (f *questFixture) killAt(name string, npc entity.EntityID, world uint8, x, z float32) {
	f.qe.killed(entity.KillCredit{NPCName: name, NPCSpawn: npc, NPCID: npc, Died: time.Now(), World: world, X: x, Z: z,
		Players: []entity.KillPlayer{{Session: f.tw.s, ServerID: f.sid, Name: "Talker"}}})
}

// The owner, 2026-11-17, Lord Valkanith's emerald: "10% chance to drop in this city only". A loot
// step whose target is AnyCreature takes a creature of any name that died within its area (World,
// X/Z, Radius) and none outside it or in another world, at the step's chance; before the quest is
// taken nothing drops (no pre-looting for an area's every kill).
func TestAnAnyCreatureLootStepDropsInItsAreaAlone(t *testing.T) {
	f := newQuestFixture(t)
	gem := f.pattern("Tal'Thex Emerald", 1)
	f.exec(`INSERT INTO quests (questid, name, enabled, giver) VALUES (70, 'Gems', 1, 'Malsis')`)
	f.exec(`INSERT INTO queststeps (questid, step, kind, target, items, count, chance, world, x, z, radius, journal)
		VALUES (70, 1, 'loot', '*', '` + strconv.Itoa(gem) + `', 1, 0.1, 0, 1000, 1000, 100, 'The emerald, in the city.')`)
	f.exec(`INSERT INTO queststeps (questid, step, kind, target, journal) VALUES (70, 2, 'talk', 'Malsis', 'Back.')`)
	f.qe.Reload()
	var drops []lootDrop
	f.qe.drop = func(npc entity.EntityID, _ time.Time, rec database.CharInventoryEntry, ahead bool) {
		drops = append(drops, lootDrop{npc, ahead, rec.Pattern.PatternID})
	}
	f.qe.roll = func() float64 { return 0 } // every roll a drop: only the area decides

	f.killAt("chaos hound", 901, 0, 1010, 990) // in the area, the quest not taken: none
	if len(drops) != 0 {
		t.Fatalf("before the quest: drops %v", drops)
	}
	if _, err := f.qe.SetCharacterQuest(f.sid, 70, QuestGive, 0); err != nil {
		t.Fatal(err)
	}
	f.killAt("chaos hound", 902, 0, 1101, 1000) // just outside the radius
	f.killAt("chaos hound", 903, 1, 1000, 1000) // its place in another world
	if len(drops) != 0 {
		t.Fatalf("outside the area: drops %v", drops)
	}
	f.killAt("cursed armor", 904, 0, 1060, 1070) // inside (92 off): any name
	if len(drops) != 1 || drops[0] != (lootDrop{904, false, gem}) {
		t.Fatalf("inside the area: drops %v, want the emerald on 904", drops)
	}

	// another creature the step names (queststeptargets) counts wherever it dies
	f.exec(`INSERT INTO queststeptargets (questid, step, target) VALUES (70, 1, 'Teir''Dal knight')`)
	f.qe.Reload()
	f.own(gem, 0)
	drops = nil
	f.killAt("chaos hound", 905, 0, 5000, 5000) // far outside: none
	f.killAt("Teir'Dal knight", 906, 0, 5000, 5000)
	if len(drops) != 1 || drops[0] != (lootDrop{906, false, gem}) {
		t.Fatalf("the step's other creature outside the area: drops %v, want the gem on 906", drops)
	}

	st := database.QuestStep{Kind: database.StepLoot, Target: database.AnyCreature, Radius: 0}
	if st.TakesKillAt(0, 0, 0) || st.AnyInArea() {
		t.Error("an AnyCreature step with no radius takes a kill")
	}
	st.Radius = 10
	if len(st.Targets()) != 0 || st.TargetWords() != "creatures" {
		t.Errorf("targets %q, words %q", st.Targets(), st.TargetWords())
	}
	named := database.QuestStep{Kind: database.StepLoot, Target: "chaos hound", Radius: 15}
	if named.TakesKillAt(0, 0, 0) {
		t.Error("a named step's radius takes any kill")
	}
}
