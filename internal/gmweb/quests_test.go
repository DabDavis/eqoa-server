package gmweb

import (
	"fmt"
	"net"
	"strings"
	"testing"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/handlers"
)

// questEngine starts the quest engine over the fixture's database and world.
func (f *fixture) questEngine() *handlers.QuestEngine {
	f.t.Helper()
	q, err := handlers.NewQuestEngine(f.db, f.world, f.s.d.Sessions, database.NewInventoryStore(f.db), database.NewSpellbookStore(f.db))
	if err != nil {
		f.t.Fatal(err)
	}
	q.Start()
	f.t.Cleanup(q.Stop)
	return q
}

func problemsOf(out map[string]any) string {
	var ps []string
	if list, ok := out["problems"].([]any); ok {
		for _, p := range list {
			ps = append(ps, fmt.Sprint(p))
		}
	}
	return strings.Join(ps, "\n")
}

// A quest saved with its steps: written as rows (each logged), its problems
// named (a merchant never talked to, a place at 0, 0, an item not on this
// server); saved again shorter, the steps past the last are deleted; a step
// change is reverted from the log.
func TestAQuestIsSavedWithItsSteps(t *testing.T) {
	f := newFixture(t)
	f.questEngine()
	c := f.browser()
	c.login("gm")
	steps := []map[string]any{
		{"kind": "talk", "target": "Merchant Shadehand", "journal": "Speak to the merchant."},
		{"kind": "give", "target": "Guard Dahvel", "items": "11*2, 999", "journal": "Bring the guard two loaves."},
		{"kind": "kill", "target": "a decaying skeleton", "count": 2, "journal": "Kill two skeletons."},
		{"kind": "reach", "journal": "Go somewhere."},
	}
	code, out := c.op("quest_save", map[string]any{
		"quest": map[string]any{"name": "Bread run", "enabled": 1, "giver": "Guard Dahvel", "accept": "Yes."},
		"steps": steps,
	})
	if code != 200 {
		t.Fatalf("quest_save: %d %v", code, out)
	}
	ps := problemsOf(out)
	for _, want := range []string{`step 1's NPC "Merchant Shadehand" opens a window`, "step 2 wants item 999, which does not exist", "step 4's place is 0, 0"} {
		if !strings.Contains(ps, want) {
			t.Errorf("problems lack %q:\n%s", want, ps)
		}
	}
	if strings.Contains(ps, "step 3") {
		t.Errorf("the control: step 3 (two skeletons, which spawn here) has a problem:\n%s", ps)
	}
	quest, _ := out["quest"].(map[string]any)
	id, _ := asFloat(quest["questid"])
	var n int
	f.db.QueryRow(`SELECT COUNT(*) FROM queststeps WHERE questid = ?`, int64(id)).Scan(&n)
	if n != 4 {
		t.Fatalf("%d steps written, want 4", n)
	}
	f.db.QueryRow(`SELECT COUNT(*) FROM gm_audit WHERE tbl IN ('quests', 'queststeps')`).Scan(&n)
	if n != 5 {
		t.Errorf("%d audit lines, want 5 (the quest and four steps)", n)
	}

	steps[1]["items"] = "11*3"
	code, out = c.op("quest_save", map[string]any{"quest": map[string]any{"questid": id, "name": "Bread run"}, "steps": steps[:2]})
	if code != 200 {
		t.Fatalf("quest_save again: %d %v", code, out)
	}
	f.db.QueryRow(`SELECT COUNT(*) FROM queststeps WHERE questid = ?`, int64(id)).Scan(&n)
	if n != 2 {
		t.Errorf("saved with two steps, %d left", n)
	}
	var items string
	f.db.QueryRow(`SELECT items FROM queststeps WHERE questid = ? AND step = 2`, int64(id)).Scan(&items)
	if items != "11*3" {
		t.Errorf("step 2's items %q, want 11*3", items)
	}
	var auditID int64
	f.db.QueryRow(`SELECT id FROM gm_audit WHERE tbl = 'queststeps' AND action = 'update' ORDER BY id DESC LIMIT 1`).Scan(&auditID)
	if code, out := c.op("revert_change", map[string]any{"id": auditID}); code != 200 {
		t.Fatalf("revert: %d %v", code, out)
	}
	f.db.QueryRow(`SELECT items FROM queststeps WHERE questid = ? AND step = 2`, int64(id)).Scan(&items)
	if items != "11*2, 999" {
		t.Errorf("reverted, step 2's items are %q", items)
	}
	if code, out := c.op("quest_list", map[string]any{"search": "bread"}); code != 200 || fmt.Sprint(out["total"]) != "1" {
		t.Errorf("quest_list: %d %v", code, out)
	}
}

// A character's quest set by a GM is logged as its row before and after, and
// reverting that takes the quest away again; a quest that does not exist is
// refused.
func TestACharactersQuestIsSetAndReverted(t *testing.T) {
	f := newFixture(t)
	q := f.questEngine()
	c := f.browser()
	c.login("gm")
	code, out := c.op("quest_save", map[string]any{
		"quest": map[string]any{"name": "Skeletons", "enabled": 1, "giver": "Guard Dahvel"},
		"steps": []map[string]any{{"kind": "kill", "target": "a decaying skeleton", "count": 3, "journal": "Kill three skeletons."}},
	})
	if code != 200 {
		t.Fatalf("quest_save: %d %v", code, out)
	}
	id, _ := asFloat(out["quest"].(map[string]any)["questid"])
	code, out = c.op("character_quest_set", map[string]any{"character": "Hero", "quest": id, "action": "give"})
	if code != 200 {
		t.Fatalf("give: %d %v", code, out)
	}
	if got := q.CharacterQuests(7); len(got) != 1 || got[0].Step != 1 || got[0].Journal != "Kill three skeletons." {
		t.Fatalf("Hero's quests: %+v", got)
	}
	var auditID int64
	f.db.QueryRow(`SELECT id FROM gm_audit WHERE tbl = 'charquests' AND action = 'insert'`).Scan(&auditID)
	if auditID == 0 {
		t.Fatal("the give was not logged as a charquests row")
	}
	if code, out := c.op("revert_change", map[string]any{"id": auditID}); code != 200 {
		t.Fatalf("revert: %d %v", code, out)
	}
	if got := q.CharacterQuests(7); len(got) != 0 {
		t.Errorf("after the revert Hero still has %+v", got)
	}
	if code, _ := c.op("character_quest_set", map[string]any{"character": "Hero", "quest": 999, "action": "give"}); code == 200 {
		t.Error("a quest that does not exist was given")
	}
	if code, _ := c.op("quest_try", map[string]any{"quest": id}); code == 200 {
		t.Error("quest_try worked for a GM with no character in the world")
	}

	// the GM in the world with a character of their own: tried, logged
	f.exec(`INSERT INTO characters (charName, accountid, serverid, modelid, tclass, race, humType, level,
		haircolor, hairlength, hairstyle, faceoption, sex, world, x, y, z, tunar)
		VALUES ('Gamemaster', 2, 8, 1, 0, 0, 1, 10, 0, 0, 0, 0, 0, 0, 25300, 54, 15700, 0)`)
	sess := f.s.d.Sessions.CreateSession(&net.UDPAddr{IP: net.IPv4(10, 0, 0, 8), Port: 7008}, 8, 1, 1, 2, true)
	sess.SetAccountID(2)
	p := &entity.Player{ServerID: 1008, CharServerID: 8, Name: "Gamemaster", Level: 10}
	f.world.Players[p.ServerID] = p
	sess.SetECSEntity(p)
	sess.SetCharacterInWorld(true)
	code, out = c.op("quest_try", map[string]any{"quest": id})
	if code != 200 {
		t.Fatalf("quest_try with a character in the world: %d %v", code, out)
	}
	if got := q.CharacterQuests(8); len(got) != 1 || got[0].Step != 1 {
		t.Errorf("the GM's character after quest_try: %+v", got)
	}
	var tries int
	f.db.QueryRow(`SELECT COUNT(*) FROM gm_audit WHERE tbl = 'charquests' AND rowkey LIKE '%"serverid":8%'`).Scan(&tries)
	if tries != 1 {
		t.Errorf("%d audit lines for the try, want 1", tries)
	}
}

func (f *fixture) exec(q string, args ...any) {
	f.t.Helper()
	mustExec(f.t, f.db, q, args...)
}

// A guide in EQOA-Data's shape: two level sections, each a draft with its
// giver, level, class, experience and reward; names this server has are
// resolved, others listed; a second import skips what the first made.
func TestEQOADataGuidesBecomeDrafts(t *testing.T) {
	guide := `{"key": "Warrior", "guide": "# Level 1\n\n**Quest NPC:** Guard Dahvel  \nSpeak to Guard Dahvel by the gate. He wants bread.  \n**430 XP** & **Bread**\n\n---\n\n# Level 3\n\n**Starting NPC:** Nobody Here  \nKill skeletons outside the gate.  \n**6,900 XP** & **Golden Sword**\n"}`
	drafts, err := parseEQOAData("Quests/1-20/freeport/warrior.json", guide)
	if err != nil {
		t.Fatal(err)
	}
	if len(drafts) != 2 {
		t.Fatalf("%d drafts, want 2", len(drafts))
	}
	d := drafts[0]
	if d.Name != "Freeport Warrior, level 1" || d.Giver != "Guard Dahvel" || d.Level != 1 || d.XP != 430 ||
		len(d.Classes) != 1 || d.Classes[0] != 0 || len(d.ItemNames) != 1 || d.ItemNames[0] != "Bread" ||
		d.Journal != "Speak to Guard Dahvel by the gate." {
		t.Errorf("the first draft: %+v", d)
	}
	if drafts[1].XP != 6900 || drafts[1].Giver != "Nobody Here" {
		t.Errorf("the second draft: %+v", drafts[1])
	}

	f := newFixture(t)
	f.questEngine()
	c := f.browser()
	c.login("gm")
	files := []map[string]any{{"path": "Quests/1-20/freeport/warrior.json", "text": guide}}
	code, out := c.op("quest_import_eqoadata", map[string]any{"files": files})
	if code != 200 || fmt.Sprint(out["made"]) != "2" {
		t.Fatalf("import: %d %v", code, out)
	}
	var enabled, n int
	var giver, rewards string
	f.db.QueryRow(`SELECT q.enabled, q.giver, s.rewarditems FROM quests q JOIN queststeps s ON s.questid = q.questid
		WHERE q.name = 'Freeport Warrior, level 1'`).Scan(&enabled, &giver, &rewards)
	if enabled != 0 || giver != "Guard Dahvel" || rewards != "11" {
		t.Errorf("the draft: enabled %d giver %q rewards %q, want 0, Guard Dahvel, 11 (Bread)", enabled, giver, rewards)
	}
	var notes string
	f.db.QueryRow(`SELECT notes FROM quests WHERE name = 'Freeport Warrior, level 3'`).Scan(&notes)
	if !strings.Contains(notes, "NPC Nobody Here") || !strings.Contains(notes, "item Golden Sword") {
		t.Errorf("the unresolved names are not in the notes: %q", notes)
	}
	code, out = c.op("quest_import_eqoadata", map[string]any{"files": files})
	if code != 200 || fmt.Sprint(out["made"]) != "0" || fmt.Sprint(out["skipped"]) != "2" {
		t.Errorf("a second import: %d made %v skipped %v", code, out["made"], out["skipped"])
	}
	f.db.QueryRow(`SELECT COUNT(*) FROM quests`).Scan(&n)
	if n != 2 {
		t.Errorf("%d quests after two imports, want 2", n)
	}
	code, out = c.op("quest_get", map[string]any{"quest": 1})
	if code != 200 || !strings.Contains(problemsOf(out), "an imported draft") {
		t.Errorf("an imported draft is not said to be one: %d %s", code, problemsOf(out))
	}
}

// A loot step and events saved through the panel: written as the editor sends
// them (chance a number, events a JSON list), read back by the engine; their
// problems named (an item not here, a chance past 1, an event the engine has
// not, a spawn of an NPC this server lacks); the control: right ones none.
func TestALootStepAndEventsAreSaved(t *testing.T) {
	f := newFixture(t)
	q := f.questEngine()
	c := f.browser()
	c.login("gm")
	good := `[{"on":"start","do":"popup","text":"Find the belts."},{"on":"done","do":"spawn","npc":"Guard Dahvel","hostile":true}]`
	code, out := c.op("quest_save", map[string]any{
		"quest": map[string]any{"name": "Belts", "enabled": 1, "giver": "Guard Dahvel",
			"events": `[{"on":"accept","do":"say","npc":"Guard Dahvel","text":"Go."}]`},
		"steps": []map[string]any{
			{"kind": "loot", "target": "a decaying skeleton", "items": "11", "count": 2, "chance": 0.5, "journal": "Bring two loaves.", "events": good},
		},
	})
	if code != 200 {
		t.Fatalf("quest_save: %d %v", code, out)
	}
	if ps := problemsOf(out); strings.Contains(ps, "step 1") || strings.Contains(ps, "event") {
		t.Errorf("the control: a right loot step and events have problems:\n%s", ps)
	}
	id, _ := asFloat(out["quest"].(map[string]any)["questid"])
	if got := q.CharacterQuests(7); len(got) != 0 {
		t.Fatalf("no one has it yet: %+v", got)
	}
	if code, out := c.op("character_quest_set", map[string]any{"character": "Hero", "quest": id, "action": "give"}); code != 200 {
		t.Fatalf("give: %d %v", code, out)
	}
	if got := q.CharacterQuests(7); len(got) != 1 || got[0].Journal != "Bring two loaves." {
		t.Fatalf("Hero's quests: %+v", got)
	}

	code, out = c.op("quest_save", map[string]any{
		"quest": map[string]any{"questid": id, "name": "Belts", "events": `[{"on":"start","do":"dance"}]`},
		"steps": []map[string]any{
			{"kind": "loot", "target": "a decaying skeleton", "items": "999", "count": 2, "chance": 1.5, "journal": "Bring two.",
				"events": `[{"on":"start","do":"spawn","npc":"Nobody At All"}, {"on":"done","do":"visual","fx":"Fade"}]`},
		},
	})
	if code != 200 {
		t.Fatalf("quest_save again: %d %v", code, out)
	}
	ps := problemsOf(out)
	for _, want := range []string{"step 1 drops item 999, which does not exist", "chance 1.5 is not between 0 and 1",
		`the quest's event 1: on "start" is not one of accept, finish`, `the quest's event 1: do "dance" is not one of`,
		`spawns "Nobody At All", which this server has no NPC of`, `fx "Fade" is not a DictID`} {
		if !strings.Contains(ps, want) {
			t.Errorf("problems lack %q:\n%s", want, ps)
		}
	}
	var chance float64
	var events string
	f.db.QueryRow(`SELECT chance, events FROM queststeps WHERE questid = ? AND step = 1`, int64(id)).Scan(&chance, &events)
	if chance != 1.5 || !strings.Contains(events, "Nobody At All") {
		t.Errorf("written: chance %v events %q", chance, events)
	}

	// a faction this server has moves without a problem; one it lacks has one
	var faction string
	if err := f.db.QueryRow(`SELECT name FROM factions ORDER BY name LIMIT 1`).Scan(&faction); err != nil {
		t.Fatalf("no faction in the fixture: %v", err)
	}
	code, out = c.op("quest_save", map[string]any{
		"quest": map[string]any{"questid": id, "name": "Belts"},
		"steps": []map[string]any{{"kind": "talk", "target": "Guard Dahvel", "journal": "Go.", "factions": faction + ":+10, Nobody's Friends:-5"}},
	})
	if code != 200 {
		t.Fatalf("quest_save with factions: %d %v", code, out)
	}
	ps = problemsOf(out)
	if strings.Contains(ps, fmt.Sprintf("%q", faction)) || !strings.Contains(ps, `"Nobody's Friends", which this server does not have`) {
		t.Errorf("the factions' problems:\n%s", ps)
	}
}
