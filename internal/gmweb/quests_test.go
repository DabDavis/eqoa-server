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
	for _, want := range []string{"step 2 wants item 999, which does not exist", "step 4's place is 0, 0"} {
		if !strings.Contains(ps, want) {
			t.Errorf("problems lack %q:\n%s", want, ps)
		}
	}
	if strings.Contains(ps, "step 3") || strings.Contains(ps, "step 1") {
		t.Errorf("the control: step 3 (two skeletons, which spawn here) or step 1 (a merchant, done at its window) has a problem:\n%s", ps)
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

// importedQuests is a small imported city (freeport's warrior chain, two
// qeynos magician drafts) and a GM's own quest, as tools/import_quests.py and
// the panel write them: a quest in the game, a ready draft, a draft lacking
// an NPC, one after it, one whose creature cannot be attacked, one the
// content's own notes hold back.
func (f *fixture) importedQuests() {
	f.t.Helper()
	src := func(key string) string { return "EQOA-Data " + key + " (tools/import_quests.py)" }
	f.exec(`INSERT INTO quests (questid, name, enabled, giver, minlevel, maxlevel, classes, requires, source, notes) VALUES
		(7, 'GM made', 1, 'Guard Dahvel', 1, 60, '', '', 'gm', ''),
		(100001, 'Bread one', 1, 'Guard Dahvel', 1, 10, '0', '', ?, ''),
		(100002, 'Skeletons', 0, 'Guard Dahvel', 3, 60, '0', '100001', ?, ''),
		(100003, 'Lost', 0, 'Maryan', 5, 60, '0', '100002', ?, ''),
		(100004, 'After lost', 0, 'Guard Dahvel', 7, 60, '0', '100003', ?, ''),
		(100005, 'Mage one', 0, 'Guard Dahvel', 1, 60, '10', '', ?, ''),
		(100006, 'Notes', 0, 'Guard Dahvel', 3, 60, '10', '', ?, 'A draft until these are here: the file''s unresolved ''Smithy Cragis''.')`,
		src("1-20/freeport/warrior/L1"), src("1-20/freeport/warrior/L3"), src("1-20/freeport/warrior/L5"),
		src("1-20/freeport/warrior/L7"), src("1-20/qeynos/magician/L1"), src("1-20/qeynos/magician/L3"))
	f.exec(`INSERT INTO queststeps (questid, step, kind, target, count, journal) VALUES
		(7, 1, 'talk', 'Guard Dahvel', 1, 'Talk.'), (100001, 1, 'talk', 'Guard Dahvel', 1, 'Talk.'),
		(100002, 1, 'kill', 'a decaying skeleton', 2, 'Kill two.'), (100003, 1, 'talk', 'Guard Dahvel', 1, 'Talk.'),
		(100004, 1, 'talk', 'Guard Dahvel', 1, 'Talk.'), (100005, 1, 'kill', 'Guard Dahvel', 1, 'Kill him.'),
		(100006, 1, 'talk', 'Guard Dahvel', 1, 'Talk.')`)
}

func questIDs(out map[string]any) []int64 {
	var ids []int64
	list, _ := out["quests"].([]any)
	for _, q := range list {
		id, _ := asFloat(q.(map[string]any)["questid"])
		ids = append(ids, int64(id))
	}
	return ids
}

func (c *client) questIDs(t *testing.T, filter map[string]any) []int64 {
	t.Helper()
	code, out := c.op("quest_list", filter)
	if code != 200 {
		t.Fatalf("quest_list %v: %d %v", filter, code, out)
	}
	return questIDs(out)
}

// The list over many imported quests: each quest's status and why worked out
// from the database as it is (a missing NPC, a draft it requires, a creature no
// one can attack, the content's own note), the filters, the cities and what
// the drafts lack; the control: the missing NPC placed, its draft and the one
// after it are ready.
func TestTheQuestListFiltersImportedQuests(t *testing.T) {
	f := newFixture(t)
	f.questEngine()
	f.importedQuests()
	c := f.browser()
	c.login("gm")
	code, out := c.op("quest_list", map[string]any{})
	if code != 200 {
		t.Fatalf("quest_list: %d %v", code, out)
	}
	if fmt.Sprint(out["total"]) != "7" {
		t.Fatalf("total %v, want 7", out["total"])
	}
	counts := out["counts"].(map[string]any)
	if fmt.Sprint(counts[questInGame], counts[questReady], counts[questDraft]) != "2 1 4" {
		t.Errorf("counts %v, want 2 in the game, 1 ready, 4 drafts", counts)
	}
	why := map[int64]string{}
	status := map[int64]string{}
	for _, q := range out["quests"].([]any) {
		m := q.(map[string]any)
		id, _ := asFloat(m["questid"])
		status[int64(id)], _ = m["status"].(string)
		if w, ok := m["why"].([]any); ok {
			why[int64(id)] = fmt.Sprint(w...)
		}
	}
	for id, want := range map[int64]string{100001: questInGame, 100002: questReady, 100003: questDraft, 100004: questDraft, 100005: questDraft, 100006: questDraft} {
		if status[id] != want {
			t.Errorf("quest %d is %q, want %q (why: %s)", id, status[id], want, why[id])
		}
	}
	for id, want := range map[int64]string{100003: `no NPC "Maryan" (its giver)`, 100004: "it requires quest 100003 (Lost), a draft that is not ready",
		100005: "carries the NPC bit, so none can be attacked", 100006: `the content's note: no "Smithy Cragis"`} {
		if !strings.Contains(why[id], want) {
			t.Errorf("quest %d's why %q lacks %q", id, why[id], want)
		}
	}
	if why[100002] != "" || why[100001] != "" {
		t.Errorf("the control: whole quests have reasons: %q, %q", why[100001], why[100002])
	}
	cities := fmt.Sprint(out["cities"])
	if !strings.Contains(cities, "city:freeport") || !strings.Contains(cities, "city:qeynos") {
		t.Errorf("cities: %s", cities)
	}
	if m := fmt.Sprint(out["missing"]); !strings.Contains(m, "name:Maryan") || !strings.Contains(m, "name:Smithy Cragis") {
		t.Errorf("missing: %s", m)
	}
	// a name lacked two ways (a step's creature and the content's note) is one entry
	f.exec(`UPDATE quests SET notes = 'A draft until these are here: the file''s unresolved ''Guard Dahvel (attackable)''.' WHERE questid = 100005`)
	_, out2 := c.op("quest_list", map[string]any{"missing": "Guard Dahvel"})
	if m := fmt.Sprint(out2["missing"]); strings.Count(m, "name:Guard Dahvel") != 1 || !strings.Contains(m, "NPC/creature") {
		t.Errorf("a name lacked two ways: %s", m)
	}
	f.exec(`UPDATE quests SET notes = '' WHERE questid = 100005`)

	for _, tc := range []struct {
		filter map[string]any
		want   string
	}{
		{map[string]any{"status": questReady}, "[100002]"},
		{map[string]any{"status": "blocked"}, "[100003 100004 100005 100006]"},
		{map[string]any{"status": "draft"}, "[100002 100003 100004 100005 100006]"},
		{map[string]any{"status": questInGame}, "[7 100001]"},
		{map[string]any{"city": "freeport"}, "[100001 100002 100003 100004]"},
		{map[string]any{"city": "none"}, "[7]"},
		{map[string]any{"class": 10}, "[7 100005 100006]"},
		{map[string]any{"level": 4}, "[7 100001 100002 100005 100006]"},
		{map[string]any{"minLevel": 11, "maxLevel": 5}, "[7 100002 100003 100005 100006]"},
		{map[string]any{"minLevel": 11}, "[7 100002 100003 100004 100005 100006]"},
		{map[string]any{"giver": "mary"}, "[100003]"},
		{map[string]any{"missing": "maryan"}, "[100003]"},
		{map[string]any{"search": "warrior/L7"}, "[100004]"},
	} {
		if got := fmt.Sprint(c.questIDs(t, tc.filter)); got != tc.want {
			t.Errorf("%v: %s, want %s", tc.filter, got, tc.want)
		}
	}

	// past a thousand steps (rowsOf's cap) every quest still has its own
	for i := 2; i <= 1100; i++ {
		f.exec(`INSERT INTO queststeps (questid, step, kind, target, count, journal) VALUES (7, ?, 'talk', 'Guard Dahvel', 1, 'Talk.')`, i)
	}
	code, out = c.op("quest_list", map[string]any{"search": "Mage one"})
	if code != 200 || strings.Contains(fmt.Sprint(out["quests"]), "it has no steps") {
		t.Errorf("past a thousand steps, a quest lost its own: %d %v", code, out["quests"])
	}

	// Maryan placed: her quest and the one after it are ready
	f.exec(`INSERT INTO npcs (id, npc_name, world, x_coord, y_coord, z_coord, facing, modelid, size, npc_level, npc_type, race)
		VALUES (503, 'Maryan', 0, 25005, 50, 15005, 0, 1893243078, 1.0, 20, 128, '0')`)
	if got := fmt.Sprint(c.questIDs(t, map[string]any{"status": questReady})); got != "[100002 100003 100004]" {
		t.Errorf("Maryan placed, the ready ones are %s", got)
	}

	// an NPC whose name has a trailing space is the name all the same, as the
	// engine matches it (623 of the live npcs rows): no problem on its quest page
	f.exec(`UPDATE npcs SET npc_name = 'Guard Dahvel ' WHERE id = 500`)
	code, out = c.op("quest_get", map[string]any{"quest": 100001})
	if code != 200 || strings.Contains(problemsOf(out), "no NPC") {
		t.Errorf("a name with a trailing space: %d %s", code, problemsOf(out))
	}
	f.exec(`UPDATE npcs SET npc_name = 'Guard Dahvel' WHERE id = 500`)

	// a merchant offers its quest at its window and a step at one is done there
	// (the game asks for its window, never talks to it): both ready; the
	// control, a giver no NPC is, a draft
	f.exec(`INSERT INTO quests (questid, name, enabled, giver, minlevel, maxlevel, classes, source) VALUES
		(100007, 'Shop', 0, 'Merchant Shadehand', 1, 60, '', 'gm'), (100008, 'Shop step', 0, 'Guard Dahvel', 1, 60, '', 'gm'),
		(100009, 'Shop nobody', 0, 'Merchant Nobody', 1, 60, '', 'gm')`)
	f.exec(`INSERT INTO queststeps (questid, step, kind, target, count, journal) VALUES
		(100007, 1, 'talk', 'Guard Dahvel', 1, 'Talk.'), (100008, 1, 'talk', 'Merchant Shadehand', 1, 'See the merchant.'),
		(100009, 1, 'talk', 'Guard Dahvel', 1, 'Talk.')`)
	code, out = c.op("quest_list", map[string]any{"search": "shop"})
	if code != 200 || strings.Contains(fmt.Sprint(out["quests"]), "offers nothing") {
		t.Errorf("a merchant giver: %d %v", code, out["quests"])
	}
	if got := fmt.Sprint(c.questIDs(t, map[string]any{"search": "shop", "status": questReady})); got != "[100007 100008]" {
		t.Errorf("a merchant's quest and a step at a merchant should be ready: %s", got)
	}
}

// Putting quests in the game: a dry run first, then the ready ones a filter
// takes, each an audited row; a draft that is not ready left out with its
// reasons, unless forced; a quest's ready prerequisites brought in with it;
// taking quests out; and nothing at all asked for refused.
func TestQuestsArePutInTheGameAndTakenOut(t *testing.T) {
	f := newFixture(t)
	f.questEngine()
	f.importedQuests()
	f.exec(`INSERT INTO npcs (id, npc_name, world, x_coord, y_coord, z_coord, facing, modelid, size, npc_level, npc_type, race)
		VALUES (503, 'Maryan', 0, 25005, 50, 15005, 0, 1893243078, 1.0, 20, 128, '0')`)
	c := f.browser()
	c.login("gm")
	enabled := func(id int64) int {
		var n int
		f.db.QueryRow(`SELECT enabled FROM quests WHERE questid = ?`, id).Scan(&n)
		return n
	}
	audits := func() int {
		var n int
		f.db.QueryRow(`SELECT COUNT(*) FROM gm_audit WHERE tbl = 'quests' AND action = 'update'`).Scan(&n)
		return n
	}
	if code, _ := c.op("quest_set_enabled", map[string]any{"enabled": true}); code == 200 {
		t.Error("turning on every quest with no filter was taken")
	}
	// the quest at the chain's end alone: its ready prerequisites come too (dry)
	code, out := c.op("quest_set_enabled", map[string]any{"quests": []int64{100004}, "enabled": true, "dryRun": true})
	if code != 200 {
		t.Fatalf("dry run: %d %v", code, out)
	}
	if got := fmt.Sprint(out["changed"]); !strings.Contains(got, "questid:100004") || !strings.Contains(got, "questid:100003") ||
		!strings.Contains(got, "questid:100002") || !strings.Contains(got, "required:true") {
		t.Errorf("the chain's end brought in: %s", got)
	}
	if enabled(100004) != 0 || audits() != 0 {
		t.Fatal("a dry run changed something")
	}
	// the ready ones of freeport, for real
	code, out = c.op("quest_set_enabled", map[string]any{"filter": map[string]any{"city": "freeport", "status": questReady}, "enabled": true})
	if code != 200 {
		t.Fatalf("set enabled: %d %v", code, out)
	}
	if enabled(100002) != 1 || enabled(100003) != 1 || enabled(100004) != 1 || audits() != 3 {
		t.Errorf("after: %d %d %d, %d audit lines; want 1 1 1, 3", enabled(100002), enabled(100003), enabled(100004), audits())
	}
	// a draft that is not ready is left out with its reasons; forced it goes in
	code, out = c.op("quest_set_enabled", map[string]any{"quests": []int64{100005}, "enabled": true})
	if code != 200 || enabled(100005) != 0 || !strings.Contains(fmt.Sprint(out["skipped"]), "NPC bit") {
		t.Errorf("a blocked draft: %d %v, enabled %d", code, out, enabled(100005))
	}
	code, out = c.op("quest_set_enabled", map[string]any{"quests": []int64{100005}, "enabled": true, "force": true})
	if code != 200 || enabled(100005) != 1 || !strings.Contains(fmt.Sprint(out["changed"]), "forced") {
		t.Errorf("forced: %d %v, enabled %d", code, out, enabled(100005))
	}
	// out of the game: qeynos's, only those in it change
	code, out = c.op("quest_set_enabled", map[string]any{"filter": map[string]any{"city": "qeynos"}, "enabled": false})
	if code != 200 || enabled(100005) != 0 || len(out["changed"].([]any)) != 1 {
		t.Errorf("qeynos out: %d %v", code, out)
	}
	// the quest page says where a quest stands in its chain
	code, out = c.op("quest_get", map[string]any{"quest": 100003})
	if code != 200 || out["status"] != questInGame || !strings.Contains(fmt.Sprint(out["before"]), "questid:100002") ||
		!strings.Contains(fmt.Sprint(out["after"]), "questid:100004") {
		t.Errorf("quest_get's chain: %d status %v before %v after %v", code, out["status"], out["before"], out["after"])
	}
}

// The old one-step drafts are not made of a section the content import made
// whole; the control: a section it did not make is.
func TestEQOADataDraftsSkipImportedSections(t *testing.T) {
	f := newFixture(t)
	f.questEngine()
	f.importedQuests()
	c := f.browser()
	c.login("gm")
	guide := `{"key": "Warrior", "guide": "# Level 1\n\n**Quest NPC:** Guard Dahvel  \nBread.  \n**430 XP**\n\n---\n\n# Level 2\n\n**Quest NPC:** Guard Dahvel  \nMore bread.  \n**900 XP**\n"}`
	code, out := c.op("quest_import_eqoadata", map[string]any{"files": []map[string]any{{"path": "Quests/1-20/freeport/warrior.json", "text": guide}}})
	if code != 200 || fmt.Sprint(out["made"]) != "1" || fmt.Sprint(out["skipped"]) != "1" {
		t.Errorf("import: %d made %v skipped %v, want 1 and 1 (level 1 imported whole)", code, out["made"], out["skipped"])
	}
}

// A quest that requires a group ("a|b", any one of them) is ready while one of
// the group is in the game, and kept out only when every one of them is
// missing or a draft that is not ready; put in the game, it brings in none of
// the group when one is in already. The control: the same two apart ("a, b")
// keep it out while either is out.
func TestAQuestRequiringAGroupIsReadyOnAnyOfIt(t *testing.T) {
	f := newFixture(t)
	f.questEngine()
	f.importedQuests()
	src := func(key string) string { return "EQOA-Data " + key + " (tools/import_quests.py)" }
	// 100003 (Lost) is a draft that is not ready (no Maryan), 100001 in the game
	f.exec(`INSERT INTO quests (questid, name, enabled, giver, minlevel, maxlevel, classes, requires, source, notes) VALUES
		(100011, 'Either', 0, 'Guard Dahvel', 1, 60, '0', '100003|100001', ?, ''),
		(100012, 'Both', 0, 'Guard Dahvel', 1, 60, '0', '100003, 100001', ?, ''),
		(100013, 'Neither', 0, 'Guard Dahvel', 1, 60, '0', '100003|100004', ?, '')`,
		src("60/good-healer/ep1-1"), src("60/good-healer/ep1-2"), src("60/good-healer/ep1-3"))
	f.exec(`INSERT INTO queststeps (questid, step, kind, target, count, journal) VALUES
		(100011, 1, 'talk', 'Guard Dahvel', 1, 'Talk.'), (100012, 1, 'talk', 'Guard Dahvel', 1, 'Talk.'),
		(100013, 1, 'talk', 'Guard Dahvel', 1, 'Talk.')`)
	c := f.browser()
	c.login("gm")
	code, out := c.op("quest_list", map[string]any{})
	if code != 200 {
		t.Fatalf("quest_list: %d %v", code, out)
	}
	status, why := map[int64]string{}, map[int64]string{}
	for _, q := range out["quests"].([]any) {
		m := q.(map[string]any)
		id, _ := asFloat(m["questid"])
		status[int64(id)], _ = m["status"].(string)
		if w, ok := m["why"].([]any); ok {
			why[int64(id)] = fmt.Sprint(w...)
		}
	}
	for id, want := range map[int64]string{100011: questReady, 100012: questDraft, 100013: questDraft} {
		if status[id] != want {
			t.Errorf("quest %d is %q, want %q (why: %s)", id, status[id], want, why[id])
		}
	}
	if !strings.Contains(why[100012], "it requires quest 100003 (Lost)") || strings.Contains(why[100011], "100003") {
		t.Errorf("why: Both %q, Either %q", why[100012], why[100011])
	}
	// put in the game: Either brings in nothing of its group (100001 is in)
	code, out = c.op("quest_set_enabled", map[string]any{"quests": []int64{100011}, "enabled": true, "dryRun": true})
	if code != 200 {
		t.Fatalf("dry run: %d %v", code, out)
	}
	if got := fmt.Sprint(out["changed"]); !strings.Contains(got, "questid:100011") || strings.Contains(got, "required:true") {
		t.Errorf("Either put in brought in: %s", got)
	}
}
