package gmweb

import (
	"fmt"
	"strings"
	"testing"

	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/handlers"
)

// The later guides' fields in the panel (tools/quests/FORMAT.md): a quest's
// standings asked, a step's rewards by class and the trips a quest unlocks,
// saved, checked and read again by the game; the quest list's status and
// city of the later guides' quests. The names and numbers are the tests'.

// aFaction is a faction the fixture's database has.
func (f *fixture) aFaction() string {
	f.t.Helper()
	var name string
	if err := f.db.QueryRow(`SELECT name FROM factions ORDER BY name LIMIT 1`).Scan(&name); err != nil {
		f.t.Fatalf("no faction in the fixture: %v", err)
	}
	return name
}

// A quest saved with its standings, rewards by class and trips: each written
// (the trips as questunlocks rows, logged), each problem named (a faction this
// server lacks, a requirement that cannot be read, a class there is not, a
// class's item and spell that do not exist, a trip's NPC no one is, a trip no
// route is); the controls name none. Saved again, a trip left out is
// deleted, and left alone when the save names none; the game bars the trip
// until the quest is finished; deleting the quest takes its trips.
func TestAQuestSavesItsStandingsClassRewardsAndTrips(t *testing.T) {
	f := newFixture(t)
	q := f.questEngine()
	c := f.browser()
	c.login("gm")
	f.exec(`INSERT INTO npcs (id, npc_name, world, x_coord, y_coord, z_coord, facing, modelid, size, npc_level, npc_type, race)
		VALUES (503, 'Waysender Ti` + "`" + `Thox', 0, 25020, 50, 15020, 0, 1893243078, 1.0, 30, 128, '0')`)
	faction := f.aFaction()
	code, out := c.op("quest_save", map[string]any{
		"quest": map[string]any{"name": "The Writ", "enabled": 1, "giver": "Guard Dahvel",
			"factionreq": faction + ":100, Nobody's Friends:0..50, broken"},
		"steps": []map[string]any{{"kind": "talk", "target": "Guard Dahvel", "journal": "Go.",
			"byclass": `{"0": {"items": "10, 999", "spells": "100, 777", "xp": 50}, "15": {}}`}},
		"unlocks": []map[string]any{{"npc": "Waysender Ti`Thox", "dest": "the grobb"}, {"npc": "Ferryman Olaf", "dest": "Isle"}},
	})
	if code != 200 {
		t.Fatalf("quest_save: %d %v", code, out)
	}
	ps := problemsOf(out)
	for _, want := range []string{`the faction requirement "broken"`, `faction "Nobody's Friends", which this server does not have`,
		`rewards for class "15": no such class`, "step 1 gives class 0 item 999, which does not exist",
		"step 1 gives class 0 spell 777, which does not exist", `no NPC is named "Ferryman Olaf"`,
		"Ferryman Olaf's trip to Isle, which is no route"} {
		if !strings.Contains(ps, want) {
			t.Errorf("problems lack %q:\n%s", want, ps)
		}
	}
	for _, not := range []string{fmt.Sprintf("%q", faction), "item 10,", "spell 100,", "Thox's trip to the grobb"} {
		if strings.Contains(ps, not) {
			t.Errorf("the control %q has a problem:\n%s", not, ps)
		}
	}
	id, _ := asFloat(out["quest"].(map[string]any)["questid"])
	var req, byclass string
	f.db.QueryRow(`SELECT factionreq FROM quests WHERE questid = ?`, int64(id)).Scan(&req)
	f.db.QueryRow(`SELECT byclass FROM queststeps WHERE questid = ?`, int64(id)).Scan(&byclass)
	if !strings.HasPrefix(req, faction+":100") || !strings.Contains(byclass, `"items": "10, 999"`) {
		t.Errorf("written: factionreq %q byclass %q", req, byclass)
	}
	routes, _ := out["routes"].([]any)
	if len(routes) < 10 || len(out["unlocks"].([]any)) != 2 {
		t.Errorf("quest_get's routes (%d) and unlocks %v", len(routes), out["unlocks"])
	}
	var n int
	f.db.QueryRow(`SELECT COUNT(*) FROM gm_audit WHERE tbl = 'questunlocks' AND action = 'insert'`).Scan(&n)
	if n != 2 {
		t.Errorf("%d trips logged, want 2", n)
	}

	// the game: the trip is barred until the quest is finished
	hero := &entity.Player{Name: "Hero", CharServerID: 7, Class: 0}
	if name, barred := q.TripBarred(hero, "Waysender Ti`Thox", "Grobb"); !barred || name != "The Writ" {
		t.Errorf("before: barred %v by %q, want barred by The Writ", barred, name)
	}
	if _, err := q.SetCharacterQuest(7, int(id), handlers.QuestComplete, 0); err != nil {
		t.Fatal(err)
	}
	if _, barred := q.TripBarred(hero, "Waysender Ti`Thox", "Grobb"); barred {
		t.Error("finished, the trip is still barred")
	}

	// saved again: a trip left out goes; a save that names none leaves them
	code, out = c.op("quest_save", map[string]any{"quest": map[string]any{"questid": id, "name": "The Writ"},
		"steps":   []map[string]any{{"kind": "talk", "target": "Guard Dahvel", "journal": "Go."}},
		"unlocks": []map[string]any{{"npc": "Waysender Ti`Thox", "dest": "the grobb"}}})
	if code != 200 {
		t.Fatalf("quest_save again: %d %v", code, out)
	}
	f.db.QueryRow(`SELECT COUNT(*) FROM questunlocks WHERE questid = ?`, int64(id)).Scan(&n)
	if n != 1 {
		t.Errorf("%d trips after saving one, want 1", n)
	}
	if code, _ := c.op("quest_save", map[string]any{"quest": map[string]any{"questid": id, "name": "The Writ II"},
		"steps": []map[string]any{{"kind": "talk", "target": "Guard Dahvel", "journal": "Go."}}}); code != 200 {
		t.Fatalf("quest_save without unlocks: %d", code)
	}
	f.db.QueryRow(`SELECT COUNT(*) FROM questunlocks WHERE questid = ?`, int64(id)).Scan(&n)
	if n != 1 {
		t.Errorf("a save naming no trips changed them: %d", n)
	}
	if code, out := c.op("quest_save", map[string]any{"quest": map[string]any{"questid": id, "name": "x"},
		"steps": []map[string]any{}, "unlocks": []map[string]any{{"npc": "Waysender Ti`Thox"}}}); code != 400 {
		t.Errorf("a trip without its place was taken: %d %v", code, out)
	}
	if code, out := c.op("quest_delete", map[string]any{"quest": id}); code != 200 {
		t.Fatalf("quest_delete: %d %v", code, out)
	}
	f.db.QueryRow(`SELECT COUNT(*) FROM questunlocks`).Scan(&n)
	if n != 0 {
		t.Errorf("%d trips left after the quest was deleted", n)
	}
}

// A questunlocks row changed in the table page is the game's at once (live).
func TestAnUnlockRowIsLive(t *testing.T) {
	f := newFixture(t)
	q := f.questEngine()
	c := f.browser()
	c.login("gm")
	f.exec(`INSERT INTO quests (questid, name, enabled, giver) VALUES (40, 'Pass', 1, 'Guard Dahvel')`)
	f.exec(`INSERT INTO queststeps (questid, step, kind, target, journal) VALUES (40, 1, 'talk', 'Guard Dahvel', 'Go.')`)
	q.Reload()
	hero := &entity.Player{Name: "Hero", CharServerID: 7}
	if _, barred := q.TripBarred(hero, "Waysender Abodai", "Neriak"); barred {
		t.Fatal("barred before any unlock")
	}
	code, out := c.op("insert_row", map[string]any{"table": "questunlocks",
		"values": map[string]any{"questid": 40, "npc": "Waysender Abodai", "dest": "Neriak"}})
	if code != 200 {
		t.Fatalf("insert_row: %d %v", code, out)
	}
	if _, barred := q.TripBarred(hero, "Waysender Abodai", "Neriak"); !barred {
		t.Errorf("the row is not the game's: not barred (live: %v)", out["live"])
	}
}

// The list over the later guides' quests: their city is their category (the
// key's folder as the content file's city names it), a draft whose standing
// names a faction this server lacks, whose class reward is an item not here
// or whose trip is no route is not ready, with its reasons; the controls: the
// same draft with its names here is ready, and a content note naming a spell
// or a faction that is here holds nothing back.
func TestTheQuestListKnowsTheLaterFields(t *testing.T) {
	f := newFixture(t)
	f.questEngine()
	c := f.browser()
	c.login("gm")
	faction := f.aFaction()
	src := func(key string) string { return "EQOA-Data " + key + " (tools/import_quests.py)" }
	f.exec(`INSERT INTO quests (questid, name, enabled, giver, source, factionreq, notes) VALUES
		(1, 'Good', 0, 'Guard Dahvel', ?, ?, 'A draft until these are here: the file''s unresolved ''Minor Healing''; the file''s unresolved ''`+faction+`''.'),
		(2, 'No faction', 0, 'Guard Dahvel', ?, 'Nobody''s Friends:10', ''),
		(3, 'No item', 0, 'Guard Dahvel', ?, '', ''),
		(4, 'No route', 0, 'Guard Dahvel', ?, '', '')`,
		src("Misc/kratas/p1"), faction+":10..20", src("30,40,49/group-1-good/p1"), src("TankExemplar/warrior/p2"), src("HuntAndGather/crazy-larry/p1"))
	for id := 1; id <= 4; id++ {
		by := `{"0": {"items": "10"}}`
		if id == 3 {
			by = `{"0": {"items": "999"}}`
		}
		f.exec(`INSERT INTO queststeps (questid, step, kind, target, journal, byclass) VALUES (?, 1, 'talk', 'Guard Dahvel', 'Go.', ?)`, id, by)
	}
	f.exec(`INSERT INTO npcs (id, npc_name, world, x_coord, y_coord, z_coord, facing, modelid, size, npc_level, npc_type, race)
		VALUES (504, 'Waysender Abodai', 0, 25030, 50, 15030, 0, 1893243078, 1.0, 30, 128, '0')`)
	f.exec(`INSERT INTO questunlocks VALUES (1, 'Waysender Abodai', 'Neriak'), (4, 'Waysender Abodai', 'Atlantis')`)
	code, out := c.op("quest_list", map[string]any{})
	if code != 200 {
		t.Fatalf("quest_list: %d %v", code, out)
	}
	rows := map[float64]map[string]any{}
	for _, r := range out["quests"].([]any) {
		m := r.(map[string]any)
		id, _ := asFloat(m["questid"])
		rows[id] = m
	}
	why := func(id float64) string { return fmt.Sprint(rows[id]["why"]) }
	if rows[1]["status"] != questReady || rows[1]["city"] != "misc" {
		t.Errorf("the control: quest 1 is %v in %v (%s), want ready in misc", rows[1]["status"], rows[1]["city"], why(1))
	}
	for id, want := range map[float64]string{2: `no faction "Nobody's Friends"`, 3: "no item 999 (step 1's reward for class 0)",
		4: "Abodai's trip to Atlantis, which is no route"} {
		if rows[id]["status"] != questDraft || !strings.Contains(why(id), want) {
			t.Errorf("quest %v: %v, %s; want a draft for %q", id, rows[id]["status"], why(id), want)
		}
	}
	for id, city := range map[float64]string{2: "30-40-49", 3: "tank-exemplar", 4: "hunt-and-gather"} {
		if rows[id]["city"] != city {
			t.Errorf("quest %v's city %v, want %s", id, rows[id]["city"], city)
		}
	}
	if ids := c.questIDs(t, map[string]any{"city": "misc"}); len(ids) != 1 || ids[0] != 1 {
		t.Errorf("city misc: %v, want quest 1", ids)
	}
}

// The old one-step drafts are not made of a later guide the content import
// made any part of; the control: a guide it did not make is.
func TestEQOADataDraftsSkipLaterGuidesTheImportMade(t *testing.T) {
	f := newFixture(t)
	f.questEngine()
	c := f.browser()
	c.login("gm")
	f.exec(`INSERT INTO quests (questid, name, enabled, giver, source) VALUES
		(9, 'Kratas, part 2', 1, 'Guard Dahvel', 'EQOA-Data Misc/kratas/p2 (tools/import_quests.py)')`)
	guide := `{"key": "Kratas", "guide": "**Quest NPC:** Guard Dahvel  \nKill Kratas.  \n**1000 XP**\n"}`
	code, out := c.op("quest_import_eqoadata", map[string]any{"files": []map[string]any{
		{"path": "Quests/Misc/kratas.json", "text": guide}, {"path": "Quests/Misc/glonkal.json", "text": guide}}})
	if code != 200 || fmt.Sprint(out["made"]) != "1" || fmt.Sprint(out["skipped"]) != "1" {
		t.Errorf("import: %d made %v skipped %v, want 1 and 1 (kratas imported in parts)", code, out["made"], out["skipped"])
	}
}

// questCity names the later guides' categories as their files do.
func TestTheLaterGuidesCities(t *testing.T) {
	for key, want := range map[string]string{"1-20/freeport/warrior/L3": "freeport", "17/freeport/p1": "17",
		"23-47/23/p1": "23-47", "30,40,49/group-4-evil/p2": "30-40-49", "60/good-tank/ep2-3": "60",
		"HuntAndGather/hunt-and-gather/p1": "hunt-and-gather", "TankExemplar/paladin/p1": "tank-exemplar",
		"Weres/vampire/p1": "weres", "Misc/lady-vox/p1": "misc", "Portal/plane-of-sky/p1": "portal", "gm-made": ""} {
		if got := questCity(key); got != want {
			t.Errorf("questCity(%q) = %q, want %q", key, got, want)
		}
	}
}
