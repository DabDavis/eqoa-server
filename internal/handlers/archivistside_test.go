package handlers

import (
	"fmt"
	"strings"
	"testing"
)

// The quests' side spells in "Grant basic spells" (petition 1361) against a
// database and the quests running: a human wizard of 30 beside the Spell
// Archivist, the quests made up as the import keys them.

// sideQuest adds a quest by its import key, enabled or not, for the classes
// ("" any) from the level, with one talk step: its own spells, its reward
// items (patternid*count), its rewards by class (byclass's JSON) and its
// choice (choices' JSON).
func (f *archivistFixture) sideQuest(id int, key string, enabled bool, classes string, minlevel int, spells, items, byclass, choices string) {
	f.t.Helper()
	on := 0
	if enabled {
		on = 1
	}
	if _, err := f.db.Exec(`INSERT INTO quests (questid, name, enabled, giver, minlevel, maxlevel, classes, source)
		VALUES (?, ?, ?, 'Guildmaster Elra', ?, 60, ?, ?)`, id, "Quest "+key, on, minlevel, classes,
		"EQOA-Data "+key+" (tools/import_quests.py)"); err != nil {
		f.t.Fatal(err)
	}
	if _, err := f.db.Exec(`INSERT INTO queststeps (questid, step, kind, target, journal, spells, rewarditems, byclass, choices)
		VALUES (?, 1, 'talk', 'Guildmaster Elra', 'Talk.', ?, ?, ?, ?)`, id, spells, items, byclass, choices); err != nil {
		f.t.Fatal(err)
	}
}

// sideRig is the wizard of 30 with the quests running over its quests: the
// class quests' spells at 2 to 23 (own, a scroll handed over, the ability
// quests' class reward), and, not side spells, the level 20 pick's two path
// scrolls and a city's level 20 that kept one path, a path reward, a vampire
// quest's and a Tank Exemplar's, a disabled quest's, one of a quest of level
// 31, another class's, an epic's, a were's; Fire Bolt a quest's and a
// merchant's both; Known Ward a quest's the book holds, as it holds Return
// Home, a path's level 20 and its 30.
func newSideRig(t *testing.T) *archivistFixture {
	t.Helper()
	f := newArchivistFixture(t, nil)
	const wiz = 1 << archivistWizard
	for _, s := range []struct {
		id    int
		name  string
		level int
	}{
		{1623, "Return Home", 1}, {301, "Minor Shielding", 3}, {302, "Arcane Bindings", 4}, {3021, "Arcane Bindings", 4},
		{303, "Mind Over Matter", 23}, {304, "Cleric's Gift", 23}, {305, "Blazing Clash", 20}, {306, "Mental Focus", 20},
		{307, "Static Clash", 30}, {3071, "Mental Surge", 30}, {309, "High Bolt", 35}, {310, "Draft Bolt", 5},
		{311, "Djinni Ward", 11}, {312, "Fire Bolt", 5}, {313, "Known Ward", 7}, {314, "Exemplar Bolt", 20},
		{315, "Vampiric Kiss", 10}, {316, "Quest High", 5}, {317, "Cleric Mend", 5}, {318, "Common Lore", 2},
		{319, "Wererat Bite", 9},
	} {
		f.spell(s.id, s.name, s.level, "")
	}
	f.classSpell(archivistWizard, 302, "dump")
	f.classSpell(12, 3021, "dump") // the enchanter's record of the name
	f.classSpell(archivistWizard, 311, "eqoa-data epic")
	f.shop(SpellArchivistName, 0x81, archivistShop)
	f.shop("Scroll Merchant", 0x81, 41, f.scroll("Fire Bolt", wiz, 5))
	bindings := f.scroll("Arcane Bindings", wiz|1<<12, 3)
	blazing, focus := f.scroll("Blazing Clash", wiz, 20), f.scroll("Mental Focus", wiz, 20)
	pick := fmt.Sprintf(`{"ask":"Which do you take?","options":[{"answer":"Blazing Clash","items":"%d"},{"answer":"Mental Focus","items":"%d"}]}`,
		blazing, focus)
	f.sideQuest(200001, "1-20/qeynos/wizard/L3", true, "13", 3, "301", "", "", "")
	f.sideQuest(200002, "1-20/qeynos/wizard/L5", true, "13", 5, "", fmt.Sprintf("%d*1", bindings), "", "")
	f.sideQuest(200003, "1-20/qeynos/wizard/L20", true, "13", 20, "", "", "", pick)
	f.sideQuest(200004, "1-20/fayspire/wizard/L20", true, "13", 20, "", fmt.Sprintf("%d*1", blazing), "", "")
	f.sideQuest(200005, "1-20/qeynos/wizard/L7", false, "13", 7, "310", "", "", "")
	f.sideQuest(200006, "1-20/qeynos/wizard/L9", true, "13", 31, "316", "", "", "")
	f.sideQuest(200007, "1-20/qeynos/cleric/L5", true, "9", 5, "317", "", "", "")
	f.sideQuest(200010, "23-47/23/abilities", true, "", 23, "", "", `{"13":{"spells":"303"},"9":{"spells":"304"}}`, "")
	f.sideQuest(200011, "23-47/35/abilities", true, "", 35, "", "", `{"13":{"spells":"309"}}`, "")
	f.sideQuest(200020, "30,40,49/group-1-good/L30-wizard", true, "13", 30, "", "", "",
		choice("Static Clash=307:", "Mental Surge=3071:"))
	f.sideQuest(200030, "Weres/vampire/p1", true, "", 1, "315", "", "", "")
	f.sideQuest(200040, "TankExemplar/wizard/strike", true, "13", 20, "314", "", "", "")
	f.sideQuest(200050, "1-20/qeynos/wizard/L11", true, "13", 11, "311", "", "", "")
	f.sideQuest(200051, "1-20/qeynos/wizard/L13", true, "13", 13, "312", "", "", "")
	f.sideQuest(200052, "1-20/qeynos/wizard/L15", true, "13", 15, "313", "", "", "")
	f.sideQuest(200053, "Misc/lore/p1", true, "", 1, "318", "", "", "")
	f.sideQuest(200054, "1-20/qeynos/wizard/L17", true, "13", 17, "319", "", "", "")
	qe, err := NewQuestEngine(f.db, f.w, f.mgr, f.inv, f.spells)
	if err != nil {
		t.Fatal(err)
	}
	f.a.quests = func() *QuestEngine { return qe }
	f.know(1623, 0, 0)
	f.know(313, 1, -1)
	f.know(306, 2, -1) // a path's level 20 (Mental Focus, the Mental Surge line's)
	f.know(307, 3, 4)  // and the other path's 30, on the bar
	return f
}

// "Grant basic spells" writes the quests' side spells with the merchants'
// (the notes of archivistside.go): for the wizard of 30, every class's quest's
// Common Lore, the class quests' Minor Shielding (their own) and Arcane
// Bindings (a scroll handed over, the wizard's record of the name), the
// merchants' Fire Bolt once though a quest teaches it too, and the ability
// quest's Mind Over Matter (the wizard's class reward), in level order after
// the book's last index, then one box. The controls: the level 20 pick's two
// path scrolls and the city's single-path level 20 (the path rule), the path
// reward, the vampire's and the Tank Exemplar's, a disabled quest's, the
// cleric's two, the epic's and the were's are not written, nor the level 31
// quest's nor High Bolt (35) at 30; Known Ward, held, not twice; nothing comes
// out of the book, the other paths' spells and the bar slot kept. At 35 the
// two of level 31 and 35 are written; then nothing.
func TestTheArchivistGrantsTheQuestsSideSpells(t *testing.T) {
	f := newSideRig(t)
	before := f.book()
	f.wantMsgs("the grant at 30", f.grant(),
		"add:318@4 flag 0", "add:301@5 flag 0", "add:302@6 flag 0", "add:312@7 flag 0", "add:303@8 flag 0",
		"box:Spell Archivist: "+fmt.Sprintf(archivistGranted, spellCount(5), 30))
	want := append(before, "318@4", "301@5", "302@6", "312@7", "303@8")
	if got := f.book(); strings.Join(got, " ") != strings.Join(want, " ") {
		t.Errorf("the book after the grant at 30: %v, want %v", got, want)
	}

	f.p.Level = 35
	f.wantMsgs("the grant at 35", f.grant(), "add:316@9 flag 0", "add:309@10 flag 0",
		"box:Spell Archivist: "+fmt.Sprintf(archivistGranted, spellCount(2), 35))
	f.wantMsgs("a second grant", f.grant(), "box:Spell Archivist: "+fmt.Sprintf(archivistHasAll, 35))
	if got := f.book(); strings.Join(got[:len(before)], " ") != strings.Join(before, " ") {
		t.Errorf("the book's rows from before: %v, were %v", got[:len(before)], before)
	}
}

// With no quests running the grant is the merchants' alone, as before petition
// 1361: the control of the side spells' source.
func TestTheArchivistGrantsTheMerchantsAloneWithNoQuests(t *testing.T) {
	f := newSideRig(t)
	f.a.quests = func() *QuestEngine { return nil }
	f.wantMsgs("the grant", f.grant(), "add:312@4 flag 0", "box:Spell Archivist: "+fmt.Sprintf(archivistGranted, spellCount(1), 30))
}
