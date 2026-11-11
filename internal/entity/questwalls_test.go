package entity

import (
	"testing"

	"github.com/eqoa/server/internal/database"
)

// The Cazic-Thule trials' world parts (questwalls.go, npcrow.go).

// A wall is in a player's view, so solid, until the quest engine says the
// character passes it; then it is in no view. Near, it is pinned before nearer
// entities when the slots are full.
func TestAQuestWallLeavesTheViewOfThoseWhoPassIt(t *testing.T) {
	f := newAggroFix(t, 60)
	wall := f.npc(3001, "", 1, BlockerBody, 1050, 1000)
	wall.NPCType = 0x80
	f.w.SetQuestWalls([]database.QuestWall{{NPCID: 3001, QuestID: 101312, Step: 2}})
	if !f.inView(wall) {
		t.Fatal("the wall is not in the view of a character who does not pass it")
	}
	f.w.SetQuestState(f.p.CharServerID, map[EntityID]bool{3001: true}, nil)
	if f.inView(wall) {
		t.Fatal("the wall is in the view of a character who passes it")
	}
	f.w.ForgetQuestState(f.p.CharServerID)
	if !f.inView(wall) {
		t.Fatal("the wall is not back once the character is forgotten")
	}
	// a crowd nearer than the wall: 40 creatures 5 off; the wall, 50 off, is kept
	for i := 0; i < 40; i++ {
		f.npc(EntityID(4000+i), "rat", 1, creationBody, 1005, 1000+float32(i)*0.1)
	}
	if !f.inView(wall) {
		t.Fatal("a crowd pushed the wall out of the view")
	}
}

// An npcaggro rule with an item attacks the players holding it alone.
func TestAHoldingRuleAttacksTheHolderAlone(t *testing.T) {
	f := newAggroFix(t, 60)
	g := f.npc(3002, "The Guardian", 62, creationBody, 1010, 1000)
	f.w.SetAggroRules(map[string]AggroRule{"The Guardian": {Aggressive: true, Holding: 1231}})
	if got := f.w.WatchedItems(); len(got) != 1 || got[0] != 1231 {
		t.Fatalf("watched items %v", got)
	}
	if !f.w.NearHoldingRule(f.p) {
		t.Fatal("the player 10 off the Guardian is not near a holding rule")
	}
	f.look()
	if f.attacks(g) {
		t.Fatal("the Guardian attacks a player without the scent")
	}
	f.w.SetQuestState(f.p.CharServerID, nil, map[int]bool{1231: true})
	f.look()
	if !f.attacks(g) {
		t.Fatal("the Guardian leaves the scent's holder alone")
	}
}

// "#n" names an npcs row: a spawn copies that row, npcspawnonly holds it
// alone, npctemplate gives it (and its copies) a template before its name's.
func TestARowKeyNamesOneRow(t *testing.T) {
	if id, ok := ParseRowKey(" #118877 "); !ok || id != 118877 {
		t.Fatalf("ParseRowKey: %d %v", id, ok)
	}
	if _, ok := ParseRowKey("Contagion"); ok {
		t.Fatal("a name read as a row")
	}
	f := newAggroFix(t, 60)
	a := f.npc(3010, "Contagion", 55, creationBody, 2000, 2000)
	b := f.npc(3011, "Contagion", 63, creationBody, 2100, 2000)
	f.w.SetSpawnOnly(map[string]bool{RowKey(3011): true})
	if a.SpawnOnly || !b.SpawnOnly {
		t.Fatalf("spawn-only: the outdoor %v, the row %v", a.SpawnOnly, b.SpawnOnly)
	}
	id, _, ok := f.w.SpawnNPCCopy("#3011", 1000, 10, 1010, 0, 0)
	if !ok {
		t.Fatal("no copy of #3011")
	}
	c := f.w.NPCs[id]
	if c.Level != 63 || c.SpawnOnly || c.CopyOf != 3011 || c.row() != 3011 {
		t.Fatalf("the copy: level %d spawn-only %v of %d", c.Level, c.SpawnOnly, c.CopyOf)
	}
	if _, _, ok := f.w.SpawnNPCCopy("#9999", 1000, 10, 1010, 0, 0); ok {
		t.Fatal("a copy of a row that is not there")
	}
	f.w.templates.use = map[string]string{RowKey(3011): "iod red", "contagion": "iod contagion"}
	f.w.templates.byName = map[string]*ClassTemplate{"iod red": {ClassTemplate: database.ClassTemplate{Name: "iod red"}}, "iod contagion": {ClassTemplate: database.ClassTemplate{Name: "iod contagion"}}}
	if tp := f.w.templateForLocked(c); tp == nil || tp.Name != "iod red" {
		t.Fatalf("the copy's template %+v", tp)
	}
	if tp := f.w.templateForLocked(a); tp == nil || tp.Name != "iod contagion" {
		t.Fatalf("the outdoor one's template %+v", tp)
	}
}
