package handlers

import (
	"fmt"
	"strings"
	"testing"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
)

// The races' homelands' starter quests (tools/quests/gen_homelands.py; the
// owner, 2026-11-10: "follow the same template as the other main race
// quests"): a homeland's quest of a level stands for its city's
// (classquest), it is its own race's alone (a quest's races by racedefs id),
// and its first gift is shown worn as well as carried.

const (
	homeL1     = 20001 // the Aviaks' level 1, at the Elder
	homeL3     = 20003
	halasWarL1 = 21001
	halasWarL3 = 21003
	halasWarL7 = 21007
	halasRgeL1 = 21101
	fpWarL1    = 21201
	qeyWarL1   = 21301
	halasSide  = 21401 // a quest of Halas's that is no class quest's key
)

// homesQuestFixture is the quest fixture with the Aviak (14, passing as a
// barbarian) and the Kerran (12, as a human) of racedefs, the Elder (the
// homeland's giver) and Marik (Halas's) beside the player, a homeland quest of
// levels 1 and 3 that end on their classquest, and the cities' class quests:
// Halas's warrior's 1, 3 and 7 in a chain and its rogue's 1, Freeport's and
// Qeynos's warrior's 1, and a quest of Marik's that is no class quest.
type homesQuestFixture struct {
	*questFixture
	elder, marik uint32
}

func newHomesQuestFixture(t *testing.T) *homesQuestFixture {
	t.Helper()
	f := &homesQuestFixture{questFixture: newQuestFixture(t)}
	entity.SetRaceDefs([]database.RaceDefRow{
		{ID: 14, Name: "Aviak", ShortName: "AVK", WireRace: 6, BodyMale: 0x3151DF13, BodyFemale: 0x3151DF13},
		{ID: 12, Name: "Kerran", ShortName: "KER", WireRace: 0, BodyMale: 0x22176258, BodyFemale: 0x22176258},
	})
	t.Cleanup(func() { entity.SetRaceDefs(nil) })
	f.exec(`UPDATE quests SET enabled = 0 WHERE questid = 10010`)
	f.elder = f.tw.add(1, &entity.NPC{ServerID: 801, Name: "Elder Kraawk", SpawnX: 25403.5, SpawnY: 54.1, SpawnZ: 15725.5})
	f.marik = f.tw.add(2, &entity.NPC{ServerID: 802, Name: "Marik McPherson", SpawnX: 25404, SpawnY: 54.1, SpawnZ: 15726})
	quest := func(id int, name, giver string, minLevel int, classes, races, requires, source, events string) {
		f.exec(`INSERT INTO quests (questid, name, enabled, giver, minlevel, maxlevel, classes, races, requires, offer, accept, decline,
			started, source, events) VALUES (?, ?, 1, ?, ?, 60, ?, ?, ?, ?, 'I will.', 'Not now.', ?, ?, ?)`,
			id, name, giver, minLevel, classes, races, requires, name+": will you?", giver+": Go, then.", source, events)
		f.exec(`INSERT INTO queststeps (questid, step, kind, target, journal, says, xp) VALUES (?, 1, 'talk', ?, ?, ?, 10)`,
			id, giver, "Speak with "+giver+" again.", giver+": Done.")
	}
	key := func(k string) string { return "EQOA-Data " + k + " (tools/import_quests.py)" }
	classquest := func(level int) string {
		return fmt.Sprintf(`[{"on":"finish","do":"classquest","level":%d}]`, level)
	}
	quest(homeL1, "A Fledgling's Wingband", "Elder Kraawk", 1, "", "14", "", key("homelands/aviak/L1"), classquest(1))
	quest(homeL3, "Rats Among the Seed", "Elder Kraawk", 1, "", "14", fmt.Sprint(homeL1), key("homelands/aviak/L3"), classquest(3))
	quest(halasWarL1, "A Warrior's Tunic", "Marik McPherson", 1, "0", "6", "", key("1-20/halas/warrior/L1"), "")
	quest(halasWarL3, "Rat Furs", "Marik McPherson", 3, "0", "6", fmt.Sprint(halasWarL1), key("1-20/halas/warrior/L3"), "")
	quest(halasWarL7, "The Seventh", "Marik McPherson", 7, "0", "6", fmt.Sprint(halasWarL3), key("1-20/halas/warrior/L7"), "")
	quest(halasRgeL1, "A Rogue's Tunic", "Juno Felligan", 1, "6", "6", "", key("1-20/halas/rogue/L1"), "")
	quest(fpWarL1, "The Militia Bracer", "Commander Nothard", 1, "0", "0, 10", "", key("1-20/freeport/warrior/L1"), "")
	quest(qeyWarL1, "The Guild's Gift", "Roger Stoutheart", 1, "0", "0", "", key("1-20/qeynos/warrior/L1"), "")
	quest(halasSide, "A Side Errand", "Marik McPherson", 1, "0", "6", fmt.Sprint(halasWarL7), key("17/halas/L1"), "")
	f.reload()
	f.tw.p.Race, f.tw.p.Class, f.tw.p.Level = 14, 0, 5
	return f
}

// do takes the quest the NPC offers and does its one step (the NPC talked to
// again); it returns the offer's box.
func (f *homesQuestFixture) do(npc uint32) string {
	f.t.Helper()
	f.tw.talk(npc)
	msgs := f.msgs()
	id := boxID(msgs)
	if id == 0 {
		f.t.Fatalf("no quest offered: %q", msgs)
	}
	f.tw.pick(id, 0)
	f.msgs()
	f.tw.talk(npc)
	f.msgs()
	for _, m := range msgs {
		if strings.HasPrefix(m, "ask#") {
			return m
		}
	}
	return ""
}

// offer is the question of the quest the NPC offers the player now, or ""
// when it offers none (whatever else it says).
func (f *homesQuestFixture) offer(npc uint32) string {
	f.t.Helper()
	f.tw.talk(npc)
	msgs := f.msgs()
	id := boxID(msgs)
	if id == 0 {
		return ""
	}
	f.tw.pick(id, 1) // "Not now."
	f.msgs()
	for _, m := range msgs {
		if pre := fmt.Sprintf("ask#%d:", id); strings.HasPrefix(m, pre) {
			return strings.TrimPrefix(m, pre)
		}
	}
	return ""
}

// A homeland's quest of a level finishes the character's own city class quest
// of that level with it: an Aviak warrior's (a barbarian on the wire) Halas
// warrior's, and no other class's, city's or quest's, with no reward and no
// word; so Halas's Marik offers the level 3 quest next, not the tunic, and
// after the homeland's level 3 the level 7. The rows are the character's for
// good (a fresh engine reads them). The controls: before the homeland's quest
// Marik offers the tunic.
func TestAHomelandsQuestFinishesTheCitysClassQuest(t *testing.T) {
	f := newHomesQuestFixture(t)
	if got := f.offer(f.marik); got != "A Warrior's Tunic: will you?|I will.|Not now." {
		t.Fatalf("the control: before the homeland's quest Marik offers %q, want the tunic", got)
	}
	f.qe.LoggedIn(f.sid) // a refusal is remembered for the session: a new one

	f.do(f.elder)
	if _, done := f.progress(homeL1); done != 1 {
		t.Fatalf("the homeland's level 1 is not finished (done %d)", done)
	}
	for quest, want := range map[int]int{halasWarL1: 1, halasWarL3: -1, halasWarL7: -1, halasRgeL1: -1, fpWarL1: -1, qeyWarL1: -1, halasSide: -1} {
		if _, done := f.progress(quest); done != want {
			t.Errorf("quest %d after the homeland's level 1: done %d, want %d (Halas's warrior's level 1 alone)", quest, done, want)
		}
	}
	if step, _ := f.progress(halasWarL1); step != 0 {
		t.Errorf("Halas's level 1 is at step %d, want finished (0)", step)
	}
	if _, lines := f.journal(); len(lines) != 0 {
		t.Errorf("the journal holds %q after it: a quest finished with another leaves no line", lines)
	}
	if got := f.offer(f.marik); got != "Rat Furs: will you?|I will.|Not now." {
		t.Errorf("after the homeland's level 1 Marik offers %q, want the level 3 quest (not the tunic)", got)
	}
	f.qe.LoggedIn(f.sid)

	f.do(f.elder) // the homeland's level 3
	if _, done := f.progress(halasWarL3); done != 1 {
		t.Fatalf("Halas's level 3 is not finished with the homeland's (done %d)", done)
	}
	f.tw.p.Level = 7
	if got := f.offer(f.marik); got != "The Seventh: will you?|I will.|Not now." {
		t.Errorf("after the homeland's level 3 Marik offers %q at level 7, want the level 7 quest", got)
	}

	// kept: a new engine on the same database (a restart, a relog) has them finished
	qe, err := NewQuestEngine(f.db, f.tw.w, f.tw.mgr, f.inv, database.NewSpellbookStore(f.db))
	if err != nil {
		t.Fatal(err)
	}
	fin := qe.FinishedBy(f.sid)
	if !fin[halasWarL1] || !fin[halasWarL3] || fin[halasWarL7] || fin[fpWarL1] {
		t.Errorf("a fresh engine reads %v finished, want Halas's warrior's 1 and 3 with the homeland's two", fin)
	}
}

// A class its race's cities have no chain for has none to finish: an Aviak
// druid's homeland quest writes no row but its own. A race that passes as a
// human could take Freeport's and Qeynos's alike: a Kerran warrior's finishes
// both. A city quest the character has a row in already is left as it is:
// under way, it stays under way.
func TestAClassQuestOfNoCityOfTwoAndOneUnderWay(t *testing.T) {
	f := newHomesQuestFixture(t)
	rows := func() int {
		var n int
		f.db.QueryRow(`SELECT count(*) FROM charquests WHERE serverid = ?`, f.sid).Scan(&n)
		return n
	}
	f.tw.p.Class = 7 // a druid: Halas trains none
	f.do(f.elder)
	if n := rows(); n != 1 {
		t.Errorf("an Aviak druid's homeland quest left %d rows, want its own alone", n)
	}

	// a Kerran (a human on the wire) warrior, its own homeland quest
	f.exec(`DELETE FROM charquests`)
	f.exec(`UPDATE quests SET races = '12' WHERE questid IN (?, ?)`, homeL1, homeL3)
	f.reload()
	f.qe.ReloadCharacter(f.sid)
	f.tw.p.Race, f.tw.p.Class = 12, 0
	f.do(f.elder)
	for quest, want := range map[int]int{fpWarL1: 1, qeyWarL1: 1, halasWarL1: -1} {
		if _, done := f.progress(quest); done != want {
			t.Errorf("a Kerran warrior's homeland quest: quest %d done %d, want %d (Freeport's and Qeynos's both, Halas's not)", quest, done, want)
		}
	}

	// an Aviak warrior with Halas's tunic quest under way: left under way
	f.exec(`DELETE FROM charquests`)
	f.exec(`UPDATE quests SET races = '14' WHERE questid IN (?, ?)`, homeL1, homeL3)
	f.reload()
	f.qe.ReloadCharacter(f.sid)
	f.tw.p.Race = 14
	f.tw.talk(f.marik)
	f.tw.pick(boxID(f.msgs()), 0)
	f.msgs()
	if step, done := f.progress(halasWarL1); step != 1 || done != 0 {
		t.Fatalf("the control: Halas's tunic quest taken is at step %d, done %d", step, done)
	}
	f.do(f.elder)
	if step, done := f.progress(halasWarL1); step != 1 || done != 0 {
		t.Errorf("Halas's quest under way is at step %d, done %d after the homeland's: want it left as it was (1, 0)", step, done)
	}
}

// A quest's races: the game's (0 to 10) are asked of the race a character
// passes as, a race of racedefs (11 and up) of the character's own. The
// Aviaks' quest is an Aviak's, and no barbarian's, human's or Kerran's; Halas's
// quest (barbarians') is a barbarian's and an Aviak's, and no human's or
// Kerran's; Qeynos's (humans') a human's and a Kerran's.
func TestAQuestsRacesNameARaceOfRacedefsByItsOwnID(t *testing.T) {
	f := newHomesQuestFixture(t)
	takes := func(quest, race int) bool {
		f.t.Helper()
		f.tw.p.Race = race
		f.qe.mu.Lock()
		defer f.qe.mu.Unlock()
		c := f.qe.charLocked(f.sid)
		why := f.qe.mayTakeLocked(f.qe.lookAt(f.tw.s), f.tw.p, c, f.qe.cat.byID[quest])
		if why != "" && why != "not for that race" {
			f.t.Fatalf("quest %d, race %d: %q", quest, race, why)
		}
		return why == ""
	}
	const (
		human, barbarian, kerran, aviak = 0, 6, 12, 14
	)
	for _, c := range []struct {
		quest, race int
		want        bool
	}{
		{homeL1, aviak, true}, {homeL1, barbarian, false}, {homeL1, human, false}, {homeL1, kerran, false},
		{halasWarL1, barbarian, true}, {halasWarL1, aviak, true}, {halasWarL1, human, false}, {halasWarL1, kerran, false},
		{qeyWarL1, human, true}, {qeyWarL1, kerran, true}, {qeyWarL1, aviak, false},
		{fpWarL1, kerran, true}, {fpWarL1, barbarian, false},
	} {
		if got := takes(c.quest, c.race); got != c.want {
			t.Errorf("quest %d for race %d: taken %v, want %v", c.quest, c.race, got, c.want)
		}
	}
	// a visitor at the homeland's giver is offered nothing of the home's
	f.tw.p.Race = barbarian
	if got := f.offer(f.elder); got != "" {
		t.Errorf("a barbarian at the Aviaks' elder is offered %q", got)
	}
	if !questRaceTakes([]int{6, 14}, 14) || questRaceTakes([]int{11}, 14) || !questRaceTakes([]int{10}, 10) || questRaceTakes(nil, 14) {
		t.Error("questRaceTakes: a list of both kinds, another race of racedefs, the froglok, or none")
	}
}

// A show step keeps its items, so one that is worn is shown as well as one
// in the pack: the homeland's first gift (and every city's) put on before it
// is shown. One in the bank is not shown; and a give step still takes only
// what is carried.
func TestAShowStepCountsWhatIsWorn(t *testing.T) {
	f := newQuestFixture(t)
	f.exec(`UPDATE quests SET enabled = 0 WHERE questid = 10010`)
	band := f.gearPattern("fledgling's wingband", 7, 4)
	elder := f.tw.add(1, &entity.NPC{ServerID: 801, Name: "Elder Kraawk", SpawnX: 25403.5, SpawnY: 54.1, SpawnZ: 15725.5})
	f.exec(`INSERT INTO quests (questid, name, enabled, giver, offer, accept, decline) VALUES (20010, 'The Wingband', 1, 'Elder Kraawk',
		'Show me your wingband.', '', '')`)
	f.exec(fmt.Sprintf(`INSERT INTO queststeps (questid, step, kind, target, items, journal, says, xp) VALUES
		(20010, 1, 'show', 'Elder Kraawk', '%d', 'Show the wingband to Elder Kraawk.', 'Elder Kraawk: It sits well on you.', 430)`, band))
	f.exec(fmt.Sprintf(`INSERT INTO queststeps (questid, step, kind, target, items, journal, says) VALUES
		(20010, 2, 'give', 'Elder Kraawk', '%d', 'Give the wingband to Elder Kraawk.', 'Elder Kraawk: Mine now.')`, band))
	f.reload()
	f.tw.talk(elder) // no answer to take it: given at once
	f.msgs()
	if step, _ := f.progress(20010); step != 1 {
		t.Fatalf("the quest is at step %d, want 1", step)
	}

	// in the bank: not shown
	item, err := f.inv.AddCharItem(f.sid, band, 1, 300, 0, database.LocBank)
	if err != nil {
		t.Fatal(err)
	}
	f.tw.talk(elder)
	if step, _ := f.progress(20010); step != 1 {
		t.Fatalf("a wingband in the bank was shown (step %d)", step)
	}
	f.msgs()

	// worn: shown, kept
	if err := f.inv.MoveCharItem(f.sid, item, database.LocPack); err != nil {
		t.Fatal(err)
	}
	if err := f.inv.SetEquipLoc(f.sid, item, 7); err != nil {
		t.Fatal(err)
	}
	f.tw.talk(elder)
	msgs := f.msgs()
	f.want(msgs, "box:Elder Kraawk: It sits well on you.")
	if step, _ := f.progress(20010); step != 2 {
		t.Fatalf("the wingband worn was not shown: step %d, %q", step, msgs)
	}
	if n := f.held(band); n != 1 {
		t.Errorf("a show step took the wingband: %d held", n)
	}

	// the give step after it: worn is not handed over; carried is
	f.tw.talk(elder)
	if step, _ := f.progress(20010); step != 2 {
		t.Errorf("a give step took a worn item (step %d)", step)
	}
	f.msgs()
	if err := f.inv.SetEquipLoc(f.sid, item, -1); err != nil {
		t.Fatal(err)
	}
	rows, _ := f.inv.GetCharacterItems(f.sid)
	f.tw.s.SetInventoryList(newInvList(rows))
	f.tw.talk(elder)
	if _, done := f.progress(20010); done != 1 || f.held(band) != 0 {
		t.Errorf("the wingband carried was not handed over: done %d, %d held, %q", done, f.held(band), f.msgs())
	}
}

// What a classquest event must be: a quest's, on its finish, with a level;
// and the level of a class quest is read off its import key alone.
func TestClassQuestEventAndLevel(t *testing.T) {
	for events, want := range map[string]int{
		`[{"on":"finish","do":"classquest","level":3}]`:  0,
		`[{"on":"accept","do":"classquest","level":3}]`:  1,
		`[{"on":"finish","do":"classquest"}]`:            1,
		`[{"on":"finish","do":"classquest","level":61}]`: 1,
	} {
		if got := database.EventProblems(events, false); len(got) != want {
			t.Errorf("%s as a quest's: %q, want %d problems", events, got, want)
		}
	}
	if got := database.EventProblems(`[{"on":"done","do":"classquest","level":3}]`, true); len(got) == 0 {
		t.Error("a step's classquest event has no problem")
	}
	for source, want := range map[string]int{
		"EQOA-Data 1-20/halas/warrior/L3 (tools/import_quests.py)":                 3,
		"EQOA-Data 1-20/neriak-nektulos/shadowknight/L20 (tools/import_quests.py)": 20,
		"EQOA-Data 1-20/halas/warrior/L20b (tools/import_quests.py)":               0,
		"EQOA-Data homelands/aviak/L3 (tools/import_quests.py)":                    0,
		"EQOA-Data 17/halas/L1 (tools/import_quests.py)":                           0,
		"EQOA-Data Quests/1-20/freeport/warrior.json level 3":                      0,
		"gm": 0,
		"":   0,
	} {
		if got := database.ClassQuestLevel(source); got != want {
			t.Errorf("ClassQuestLevel(%q) = %d, want %d", source, got, want)
		}
	}
}
