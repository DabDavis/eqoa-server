package handlers

import (
	"fmt"
	"strings"
	"testing"

	"github.com/eqoa/pkg/pkg/buffer"
	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// The lines the "not yet" tests' table holds, one a city and one of no city,
// so each test knows the words it should hear.
const (
	nyNeriak   = "The weak are of no use to a House. Return when you are stronger."
	nyOggok    = "You too puny. Come back when you big and strong."
	nyFreeport = "Freeport pays for results. Come back when you are stronger."
	nyMoradhim = "Soft iron breaks. Come back when you are stronger."
	nyPlain    = "Come back when you are stronger."
)

// Dust's chain (the user's own case, 2026-10-30: "dust is level 6 and cannot
// take level 7 q yet"): Neriak's shadow knight guide, "New Gear" at level 4
// (here another giver's), then Sythonis U`Dedne's "The Fire Bats" at level 5
// and "Frog Legs for Grobb" at level 7.
const (
	nyNewGear  = 480
	nyFireBats = 481
	nyFrogLegs = 482
	nySythonis = "Sythonis U`Dedne"
)

// newNotYetFixture is the quest fixture's character made Dust: a level 6 dark
// elf shadow knight who has finished "New Gear" and "The Fire Bats", with
// Sythonis standing by and questnotyet holding the tests' lines. It returns
// the object id the session knows Sythonis by.
func newNotYetFixture(t *testing.T) (*questFixture, uint32) {
	t.Helper()
	f := newQuestFixture(t)
	f.tw.p.Class, f.tw.p.Race, f.tw.p.Level = 3, 2, 6
	f.quest(nyNewGear, "New Gear", "Zethril Do`Vexis", 4, 0, "EQOA-Data 1-20/neriak-nektulos/shadowknight/L4 (tools/import_quests.py)")
	f.quest(nyFireBats, "The Fire Bats", nySythonis, 5, nyNewGear, "EQOA-Data 1-20/neriak-nektulos/shadowknight/L5 (tools/import_quests.py)")
	f.quest(nyFrogLegs, "Frog Legs for Grobb", nySythonis, 7, nyFireBats, "EQOA-Data 1-20/neriak-nektulos/shadowknight/L7 (tools/import_quests.py)")
	f.notYetLines([2]string{"Neriak", nyNeriak}, [2]string{"Oggok", nyOggok}, [2]string{"Freeport", nyFreeport},
		[2]string{"Moradhim", nyMoradhim}, [2]string{"", nyPlain})
	f.reload()
	f.finish(nyNewGear)
	f.finish(nyFireBats)
	sythonis := f.tw.add(3, &entity.NPC{ServerID: 703, Name: nySythonis, SpawnX: 25403.5, SpawnY: 54.1, SpawnZ: 15725.5})
	return f, sythonis
}

// quest adds an enabled quest of one talk step (at Kellina) for shadow
// knights: its id, name, giver, lowest level, the quest finished first (0
// none) and its source.
func (f *questFixture) quest(id int, name, giver string, minLevel, requires int, source string) {
	f.t.Helper()
	req := ""
	if requires > 0 {
		req = fmt.Sprint(requires)
	}
	f.exec(`INSERT INTO quests (questid, name, enabled, giver, minlevel, maxlevel, classes, requires, offer, accept, decline, started, source)
		VALUES (?, ?, 1, ?, ?, 60, '3', ?, ?, 'I will.', 'Not now.', ?, ?)`,
		id, name, giver, minLevel, req, "Will you do this for me?", giver+": Go, then.", source)
	f.exec(`INSERT INTO queststeps (questid, step, kind, target, journal) VALUES (?, 1, 'talk', 'Kellina', ?)`, id, "Speak with Kellina about "+name+".")
}

// notYetLines makes questnotyet hold these (city, line) rows alone.
func (f *questFixture) notYetLines(rows ...[2]string) {
	f.t.Helper()
	f.exec(`DELETE FROM questnotyet`)
	for _, r := range rows {
		f.exec(`INSERT INTO questnotyet (city, line) VALUES (?, ?)`, r[0], r[1])
	}
	database.InvalidateQuestNotYet()
}

func (f *questFixture) reload() {
	f.t.Helper()
	if err := f.qe.Reload(); err != nil {
		f.t.Fatal(err)
	}
}

// finish marks a quest finished for the character (a GM's complete: no
// rewards), the messages it sends taken.
func (f *questFixture) finish(quest int) {
	f.t.Helper()
	if _, err := f.qe.SetCharacterQuest(f.sid, quest, QuestComplete, 0); err != nil {
		f.t.Fatal(err)
	}
	f.msgs()
}

// heardOnly fails unless the messages are exactly the one box.
func (f *questFixture) heardOnly(msgs []string, box string) {
	f.t.Helper()
	if len(msgs) != 1 || msgs[0] != "box:"+box {
		f.t.Fatalf("heard %q, want only %q", msgs, "box:"+box)
	}
}

// A player who has done the quests before a giver's next one, of its class
// and race, but a level short of it, hears the giver say to come back when
// stronger in its city's words, its name before them, and nothing else (no
// offer, no town's words). The city is the quest's import key's (Dust's
// Neriak chain), else the giver's town by its faction, else by where it
// stands; a giver of no town says the line of no city. A city's lines are
// never said twice running when it has another.
func TestNotYetOneLevelShortHearsItsCitysLine(t *testing.T) {
	f, sythonis := newNotYetFixture(t)

	f.tw.talk(sythonis)
	f.heardOnly(f.msgs(), nySythonis+": "+nyNeriak)

	// another city's chain at its own giver: Oggok's words (the key's city,
	// though this giver stands in Freeport)
	f.quest(601, "Rat Tails", "Greenblood Yurgat", 5, 0, "EQOA-Data 1-20/oggok/shadowknight/L5 (tools/import_quests.py)")
	f.quest(602, "Rotwood", "Greenblood Yurgat", 7, 601, "EQOA-Data 1-20/oggok/shadowknight/L7 (tools/import_quests.py)")
	f.reload()
	f.finish(601)
	yurgat := f.tw.add(4, &entity.NPC{ServerID: 704, Name: "Greenblood Yurgat", SpawnX: 25404, SpawnY: 54.1, SpawnZ: 15726})
	f.tw.talk(yurgat)
	f.heardOnly(f.msgs(), "Greenblood Yurgat: "+nyOggok)

	// a later guide's quest (no starting city in its key): the giver's town,
	// by its faction
	f.quest(603, "Side Work", "Scribe Cathassa", 1, 0, "EQOA-Data 17/side-quests/p0 (tools/import_quests.py)")
	f.quest(604, "The Scribe's Errand", "Scribe Cathassa", 17, 603, "EQOA-Data 17/side-quests/p1 (tools/import_quests.py)")
	f.reload()
	f.finish(603)
	f.tw.w.SetNPCFactions([]database.NPCFactionRow{{Name: "scribe cathassa", World: -1, Faction: "Citizens of Freeport"}})
	cathassa := &entity.NPC{ServerID: 705, Name: "Scribe Cathassa", SpawnX: 15352, SpawnZ: 9262} // standing at Moradhim's mark
	if !f.qe.NotYet(f.tw.s, f.tw.p, cathassa) {
		t.Fatal("Scribe Cathassa said nothing")
	}
	f.heardOnly(f.msgs(), "Scribe Cathassa: "+nyFreeport)

	// no faction: the town it stands in (the coach map's)
	f.tw.w.SetNPCFactions(nil)
	if !f.qe.NotYet(f.tw.s, f.tw.p, cathassa) {
		t.Fatal("Scribe Cathassa said nothing")
	}
	f.heardOnly(f.msgs(), "Scribe Cathassa: "+nyMoradhim)

	// in no town at all: the line of no city
	cathassa.SpawnX, cathassa.SpawnZ = 2000, 2000
	if !f.qe.NotYet(f.tw.s, f.tw.p, cathassa) {
		t.Fatal("Scribe Cathassa said nothing")
	}
	f.heardOnly(f.msgs(), "Scribe Cathassa: "+nyPlain)

	// two lines of Neriak: never the same twice running
	f.notYetLines([2]string{"neriak-nektulos", "First."}, [2]string{"Neriak", "Second."})
	f.tw.talk(sythonis)
	first := f.msgs()
	f.tw.talk(sythonis)
	second := f.msgs()
	if len(first) != 1 || len(second) != 1 || first[0] == second[0] {
		t.Errorf("two talks heard %q then %q: want each line once", first, second)
	}
}

// At the quest's level the giver offers it, as before, and says no "not yet";
// a level short (the baseline) it says it. The off-by-one control: the
// quest's own level is enough.
func TestNotYetAtTheLevelIsOfferedTheQuest(t *testing.T) {
	f, sythonis := newNotYetFixture(t)
	f.tw.talk(sythonis)
	f.heardOnly(f.msgs(), nySythonis+": "+nyNeriak)

	f.tw.p.Level = 7
	f.tw.talk(sythonis)
	m := f.msgs()
	if !strings.Contains(strings.Join(m, "\n"), ":Will you do this for me?|I will.|Not now.") {
		t.Fatalf("at level 7: %q, want Frog Legs for Grobb offered", m)
	}
	f.not(m, "box:"+nySythonis+": "+nyNeriak)
}

// A player who has not finished the quests before the giver's next one
// hears no "come back when you are stronger": the level is not all it lacks
// (the baseline, with them finished, hears it). With "The Fire Bats"
// forgotten Sythonis offers it, the one to take now; with "New Gear" (another
// giver's) forgotten too it has nothing for the player and says nothing. Nor
// does a player of a class the chain is not for hear it.
func TestNotYetNotForAPlayerMissingAnEarlierQuest(t *testing.T) {
	f, sythonis := newNotYetFixture(t)
	f.tw.talk(sythonis)
	f.heardOnly(f.msgs(), nySythonis+": "+nyNeriak)

	f.tw.p.Class = 10 // a magician: the chain is the shadow knights'
	f.tw.talk(sythonis)
	if m := f.msgs(); len(m) != 0 {
		t.Errorf("a magician heard %q", m)
	}
	f.tw.p.Class = 3

	forget := func(quest int) {
		t.Helper()
		if _, err := f.qe.SetCharacterQuest(f.sid, quest, QuestReset, 0); err != nil {
			t.Fatal(err)
		}
		f.msgs()
	}
	forget(nyFireBats)
	f.tw.talk(sythonis)
	m := f.msgs()
	if !strings.Contains(strings.Join(m, "\n"), ":Will you do this for me?|I will.|Not now.") {
		t.Fatalf("with The Fire Bats forgotten: %q, want it offered", m)
	}
	f.not(m, "box:"+nySythonis+": "+nyNeriak)

	forget(nyNewGear)
	f.tw.talk(sythonis)
	if m := f.msgs(); len(m) != 0 {
		t.Errorf("with New Gear forgotten too: %q, want nothing", m)
	}
}

// A giver with a step of the player's to do does the step, not the "not
// yet" (the baseline, with none, says it).
func TestNotYetAGiverWithAStepToDoDoesTheStep(t *testing.T) {
	f, sythonis := newNotYetFixture(t)
	f.tw.talk(sythonis)
	f.heardOnly(f.msgs(), nySythonis+": "+nyNeriak)

	f.exec(`INSERT INTO quests (questid, name, enabled, giver, minlevel) VALUES (605, 'A Word from the Dominary', 1, ?, 1)`, "Dominary K`Jartan")
	f.exec(`INSERT INTO queststeps (questid, step, kind, target, journal, says) VALUES (605, 1, 'talk', ?, 'Speak with Sythonis.', ?)`,
		nySythonis, nySythonis+": So the Dominary sent you.")
	f.reload()
	if _, err := f.qe.SetCharacterQuest(f.sid, 605, QuestGive, 0); err != nil {
		t.Fatal(err)
	}
	f.msgs()
	f.tw.talk(sythonis)
	m := f.msgs()
	f.want(m, "box:"+nySythonis+": So the Dominary sent you.", "box:"+questFinished)
	f.not(m, "box:"+nySythonis+": "+nyNeriak)
}

// A giver's own business comes before its "not yet": a coachman who gives a
// quest the player is too low for still lists its routes (as the Portal
// guides' waysenders still send), where a giver with no business of its own
// (the baseline) says it.
func TestNotYetAfterTheGiversOwnBusiness(t *testing.T) {
	f, sythonis := newNotYetFixture(t)
	f.tw.talk(sythonis)
	f.heardOnly(f.msgs(), nySythonis+": "+nyNeriak)

	f.quest(608, "The Stable Ledger", "Coachman Lothi", 7, nyFireBats, "EQOA-Data 1-20/neriak-nektulos/shadowknight/L7b (tools/import_quests.py)")
	f.reload()
	lothi := f.tw.add(5, &entity.NPC{ServerID: 706, Name: "Coachman Lothi", SpawnX: 25404, SpawnY: 54.1, SpawnZ: 15726})
	f.tw.add(6, &entity.NPC{ServerID: 710, Name: "Coachman Billfer", SpawnX: 4829, SpawnY: 54.1, SpawnZ: 17154})
	f.tw.talk(lothi)
	m := f.msgs()
	if !strings.Contains(strings.Join(m, "\n"), "Where would you like to go?") {
		t.Fatalf("Coachman Lothi: %q, want its routes", m)
	}
	f.not(m, "box:Coachman Lothi: "+nyNeriak)
}

// A merchant giver (CLIENT asks for its window, never talks to it) says its
// "not yet" in place of the window once a session, as it offers a quest:
// the next request opens the shop. The control: before the quest that comes
// first is done, the shop opens at once.
func TestNotYetAtAShopWindowOnceASession(t *testing.T) {
	f, _ := newNotYetFixture(t)
	ulla := &entity.NPC{ServerID: 709, Name: "Spellsmith Ulla", NPCID: 709, NPCType: 0x81, SpawnX: 25403, SpawnY: 54.1, SpawnZ: 15726}
	id := f.tw.add(8, ulla)
	f.quest(606, "Foreman's Ore", "Foreman Druza", 5, 0, "EQOA-Data 1-20/moradhim/shadowknight/L5 (tools/import_quests.py)")
	f.quest(607, "The Spellsmith's Order", "Spellsmith Ulla", 7, 606, "EQOA-Data 1-20/moradhim/shadowknight/L7 (tools/import_quests.py)")
	f.reload()
	wh := NewNPCWindowHandler(f.tw.mgr, f.inv)
	wh.playerOf = func(*session.Session) *entity.Player { return f.tw.p }
	wh.find = func(_ *session.Session, oid uint32) (entity.Entity, bool) { return ulla, oid == id }
	open := func() []string {
		t.Helper()
		msg := protocol.NewMessage(protocol.MessageTypeReliable, protocol.OpcodeMerchantDiag)
		msg.SetData(build(protocol.OpcodeMerchantDiag, 8, func(w *buffer.Writer) { w.WriteUint32(id) })[2:])
		if err := wh.HandleMerchantOpen(f.tw.s, msg); err != nil {
			t.Fatal(err)
		}
		return f.msgs()
	}
	shop := fmt.Sprintf("op:%04X", uint16(protocol.OpcodeMerchantBox))

	f.want(open(), shop) // the control: Foreman's Ore not done, nothing to say
	f.finish(606)
	m := open()
	f.not(m, shop)
	f.want(m, fmt.Sprintf("op:%04X", uint16(protocol.OpcodeCancelPending)), "box:Spellsmith Ulla: "+nyMoradhim)
	f.want(open(), shop) // said once this session: the shop now
}

// What a quest's source names as its city: a starting city's guide (the
// content import's key or the panel's draft), Weres for a were guide, none
// for a later guide or a GM's own quest.
func TestNotYetImportCity(t *testing.T) {
	for source, want := range map[string]string{
		"EQOA-Data 1-20/neriak-nektulos/shadowknight/L7 (tools/import_quests.py)": "neriak-nektulos",
		"EQOA-Data Quests/1-20/freeport/warrior.json level 3":                     "freeport",
		"EQOA-Data 1-20/klick-anon/wizard.json level 10":                          "klick-anon",
		"EQOA-Data Weres/wolfwere/p3 (tools/import_quests.py)":                    "Weres",
		"EQOA-Data Misc/kratas/p1 (tools/import_quests.py)":                       "",
		"EQOA-Data 30,40,49/group-1-good/p2 (tools/import_quests.py)":             "",
		"EQOA-Data 1-20 (tools/import_quests.py)":                                 "",
		"a GM's own": "",
		"":           "",
	} {
		if got := importCity(source); got != want {
			t.Errorf("importCity(%q) = %q, want %q", source, got, want)
		}
	}
}
