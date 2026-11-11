package handlers

import (
	"encoding/binary"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// What the later guides need of the engine (tools/quests/FORMAT.md): a
// quest's faction requirements, a repeatable quest, a step's rewards by
// class and the trips a quest unlocks. The words and numbers are the tests'
// own.

// standWith sets the player's own standing with a faction (charfactions'),
// which World.Standing reads before any starting one.
func (f *questFixture) standWith(faction string, v int) {
	if f.tw.p.Factions == nil {
		f.tw.p.Factions = map[string]int{}
	}
	f.tw.p.Factions[faction] = v
}

// takeMarks is the givers the marks sent now show a quest to take at; the
// marks are off again after (the engine's worker sends no more).
func (f *questFixture) takeMarks() []string {
	f.t.Helper()
	f.qe.SetMarkers(f.tw.s, f.tw.p, true)
	marks, _ := sentMarks(f.t, f.tw.s)
	f.qe.SetMarkers(f.tw.s, f.tw.p, false)
	var out []string
	for _, m := range marks {
		if m.kind == markTake {
			out = append(out, m.npc)
		}
	}
	return out
}

// A quest that asks a standing with a faction is offered (and marked) only
// while the player has it; once taken, a step at its giver waits on it too,
// with a line that says why, while a step at another NPC goes on. A highest
// standing bars one too well regarded. The controls: at the standing it
// asks, offered and done.
func TestAQuestWaitsOnAFactionStanding(t *testing.T) {
	f := newQuestFixture(t)
	f.tw.p.HP = 100
	holt := f.tw.add(1, &entity.NPC{ServerID: 711, Name: "Warden Holt", SpawnX: 25403.5, SpawnY: 54.1, SpawnZ: 15725.5})
	kellina := f.tw.add(2, &entity.NPC{ServerID: 702, Name: "Kellina", SpawnX: 25404, SpawnY: 54.1, SpawnZ: 15726})
	f.exec(`UPDATE quests SET enabled = 0 WHERE questid = 10010`) // Malsis's: not in the way
	f.exec(`INSERT INTO quests (questid, name, enabled, giver, offer, accept, factionreq)
		VALUES (50, 'Watch work', 1, 'Warden Holt', 'The Watch needs a hand.', 'I will.', 'Testmoor Watch:100')`)
	f.exec(`INSERT INTO queststeps (questid, step, kind, target, journal) VALUES (50, 1, 'talk', 'Kellina', 'See Kellina.')`)
	f.exec(`INSERT INTO queststeps (questid, step, kind, target, journal, says, xp) VALUES (50, 2, 'talk', 'Warden Holt', 'Back to Holt.', 'Well done.', 100)`)
	if err := f.qe.Reload(); err != nil {
		t.Fatal(err)
	}
	f.msgs()

	f.standWith("Testmoor Watch", 50)
	f.tw.talk(holt)
	if m := f.msgs(); len(m) != 0 {
		t.Fatalf("offered at a standing of 50 of the 100 it asks: %q", m)
	}
	if got := f.takeMarks(); len(got) != 0 {
		t.Fatalf("marked at 50: %q", got)
	}

	f.standWith("Testmoor Watch", 100)
	if got := f.takeMarks(); !slices.Equal(got, []string{"Warden Holt"}) {
		t.Fatalf("the control: at 100 the marks are %q, want Warden Holt's", got)
	}
	f.tw.talk(holt)
	m := f.msgs()
	f.want(m, "ask#*")
	f.tw.pick(boxID(m), 0)
	f.want(f.msgs(), "add:See Kellina.", "box:"+questReceived)

	// the step at Kellina is no giver's: done at any standing
	f.standWith("Testmoor Watch", -300)
	f.tw.talk(kellina)
	f.want(f.msgs(), "add:Back to Holt.")
	f.tw.talk(holt)
	m = f.msgs()
	if len(m) != 1 || m[0] != "box:Warden Holt will not deal with you: your standing with Testmoor Watch is too low." {
		t.Fatalf("at the giver below its standing: %q", m)
	}
	if s, _ := f.progress(50); s != 2 {
		t.Fatalf("the giver's step was done at -300 (step %d)", s)
	}
	f.standWith("Testmoor Watch", 400)
	f.tw.talk(holt)
	f.want(f.msgs(), "box:Warden Holt: Well done.", "box:"+questFinished, "chat:You receive 100 XP.")
	if s, d := f.progress(50); s != 0 || d != 1 {
		t.Errorf("finished: step %d done %d", s, d)
	}

	// a highest standing: one too well regarded is not offered it
	f.exec(`INSERT INTO quests (questid, name, enabled, giver, factionreq) VALUES (51, 'Dark work', 1, 'Warden Holt', 'Lockjaw:-2000..-100')`)
	f.exec(`INSERT INTO queststeps (questid, step, kind, target, journal) VALUES (51, 1, 'talk', 'Kellina', 'See Kellina again.')`)
	if err := f.qe.Reload(); err != nil {
		t.Fatal(err)
	}
	f.standWith("Lockjaw", 0)
	f.tw.talk(holt)
	if m := f.msgs(); len(m) != 0 {
		t.Fatalf("offered at 0 with Lockjaw, which it wants at -100 or below: %q", m)
	}
	f.standWith("Lockjaw", -100)
	f.tw.talk(holt)
	f.want(f.msgs(), "add:See Kellina again.")
}

// A repeatable quest is offered again once finished, and its rewards given
// again; the control: the same quest not repeatable is not.
func TestARepeatableQuestIsTakenAgain(t *testing.T) {
	f := newQuestFixture(t)
	f.tw.p.HP = 100
	f.exec(`UPDATE quests SET enabled = 0 WHERE questid = 10010`)
	jorn := f.tw.add(1, &entity.NPC{ServerID: 721, Name: "Jorn", SpawnX: 25403.5, SpawnY: 54.1, SpawnZ: 15725.5})
	f.exec(`INSERT INTO quests (questid, name, enabled, giver, repeatable, offer) VALUES (60, 'Pelts', 1, 'Jorn', 1, 'Bring me a pelt.')`)
	f.exec(fmt.Sprintf(`INSERT INTO queststeps (questid, step, kind, target, items, journal, rewardtunar)
		VALUES (60, 1, 'give', 'Jorn', '%d', 'Bring Jorn a pelt.', 7)`, f.bone))
	if err := f.qe.Reload(); err != nil {
		t.Fatal(err)
	}
	for round := 1; round <= 2; round++ {
		f.tw.talk(jorn) // taken at once: no answer to take it
		f.want(f.msgs(), "add:Bring Jorn a pelt.")
		f.own(f.bone, 1)
		f.tw.talk(jorn)
		f.want(f.msgs(), "box:You have received 7 tunar.", "box:"+questFinished)
		if s, d := f.progress(60); s != 0 || d != round {
			t.Fatalf("round %d: step %d done %d", round, s, d)
		}
	}
	f.exec(`UPDATE quests SET repeatable = 0 WHERE questid = 60`)
	if err := f.qe.Reload(); err != nil {
		t.Fatal(err)
	}
	f.tw.talk(jorn)
	if m := f.msgs(); len(m) != 0 {
		t.Errorf("the control: not repeatable, a finished quest was offered again: %q", m)
	}
}

// A step's rewards by class are given with its own to a player of that class
// (items, a spell taught as a scribe teaches it, experience, tunar) and said
// in the quest's marks for that class; another class gets its own, and a
// class with none the step's alone.
func TestClassRewardsGoWithAStep(t *testing.T) {
	f := newQuestFixture(t)
	f.tw.p.HP = 100
	f.exec(`UPDATE quests SET enabled = 0 WHERE questid = 10010`)
	for _, q := range []string{
		`INSERT INTO spellpattern (spellid, abilitylvl, unk2, unk3, range, casttime, power, iconColor, icon, scope, recast, eqprequire, spellname, spelldesc)
		 VALUES (650, 3, 0, 0, 16880, 2, 21, 1, 2, 1, 3, -1, 'Ember Lash', '')`,
	} {
		f.exec(q)
	}
	staff := f.pattern("Ash Staff", 1)
	axe := f.pattern("Iron Axe", 1)
	brin := f.tw.add(1, &entity.NPC{ServerID: 731, Name: "Brin", SpawnX: 25403.5, SpawnY: 54.1, SpawnZ: 15725.5})
	f.exec(`INSERT INTO quests (questid, name, enabled, giver, offer) VALUES (70, 'Proving', 1, 'Brin', 'Prove yourself.')`)
	f.exec(fmt.Sprintf(`INSERT INTO queststeps (questid, step, kind, target, journal, xp, rewardtunar, rewarditems, byclass)
		VALUES (70, 1, 'talk', 'Kellina', 'See Kellina.', 100, 5, '%d',
		'{"10": {"xp": 50, "tunar": 20, "items": "%d", "spells": "650"}, "0": {"items": "%d"}}')`, f.aura, staff, axe))
	if err := f.qe.Reload(); err != nil {
		t.Fatal(err)
	}
	kellina := f.tw.add(2, &entity.NPC{ServerID: 702, Name: "Kellina", SpawnX: 25404, SpawnY: 54.1, SpawnZ: 15726})
	f.msgs()

	// the marks name the magician's rewards, and a warrior's its own
	f.qe.SetMarkers(f.tw.s, f.tw.p, true)
	marks, _ := sentMarks(t, f.tw.s)
	if len(marks) != 1 || marks[0].quests[0].detail != "150 XP, 25 tunar, Smoldering Aura, Ash Staff, Ember Lash" {
		t.Fatalf("a magician's marks: %+v", marks)
	}
	f.tw.p.Class = 0
	f.qe.syncMarks(time.Now())
	marks, _ = sentMarks(t, f.tw.s)
	if len(marks) != 1 || marks[0].quests[0].detail != "100 XP, 5 tunar, Smoldering Aura, Iron Axe" {
		t.Fatalf("a warrior's marks: %+v", marks)
	}
	f.tw.p.Class = 4
	f.qe.syncMarks(time.Now())
	marks, _ = sentMarks(t, f.tw.s)
	if len(marks) != 1 || marks[0].quests[0].detail != "100 XP, 5 tunar, Smoldering Aura" {
		t.Fatalf("the control, a monk's (no class rewards): %+v", marks)
	}

	f.tw.p.Class = 10
	f.qe.SetMarkers(f.tw.s, f.tw.p, false)
	f.tw.talk(brin)
	f.msgs()
	f.tw.talk(kellina)
	m := f.msgs()
	f.want(m, "box:You have received 25 tunar.", "box:You have received Smoldering Aura.", "box:You have received Ash Staff.",
		"box:You have learned Ember Lash.", "chat:You receive 150 XP.")
	f.not(m, "box:You have received Iron Axe.")
	if f.held(staff) != 1 || f.held(axe) != 0 || f.held(f.aura) != 1 {
		t.Errorf("held: staff %d axe %d aura %d, want 1 0 1", f.held(staff), f.held(axe), f.held(f.aura))
	}
	var n int
	f.db.QueryRow(`SELECT COUNT(*) FROM spells WHERE serverid = ? AND spellid = 650`, f.sid).Scan(&n)
	if n != 1 {
		t.Errorf("Ember Lash is in the book %d times, want once", n)
	}
}

// thoxWorld is the quest fixture with Waysender Ti`Thox (Freeport for 100,
// Grobb for 225) and the two waysenders his trips land at.
func thoxWorld(t *testing.T) (*questFixture, uint32) {
	f := newQuestFixture(t)
	thox := f.tw.add(1, &entity.NPC{ServerID: 2616, Name: "Waysender Ti`Thox", SpawnX: 25403.5, SpawnY: 54.1, SpawnZ: 15725.5})
	f.tw.w.NPCs[695] = &entity.NPC{ServerID: 695, Name: "Waysender Abodai", SpawnX: 25405.4, SpawnY: 54.1, SpawnZ: 15781}
	f.tw.w.NPCs[101655] = &entity.NPC{ServerID: 101655, Name: "Waysender Zapgit", SpawnX: 25436.3, SpawnY: 63.8, SpawnZ: 31347.3}
	f.exec(`UPDATE quests SET enabled = 0 WHERE questid = 10010`)
	return f, thox
}

// A trip a quest unlocks is not offered to a player who has not finished it:
// Ti`Thox's Grobb waits on "The Grobb Writ", so a player without it is asked
// of Freeport alone; once it is finished, both. Every trip so closed, the
// waysender says which quest to finish. The controls: a draft's unlock bars no
// one, and an unlock is matched to its route's label by case and a "the".
func TestAQuestUnlocksATrip(t *testing.T) {
	f, thox := thoxWorld(t)
	f.exec(`INSERT INTO quests (questid, name, enabled, giver) VALUES (80, 'The Grobb Writ', 1, 'Nobody Here')`)
	f.exec(`INSERT INTO queststeps (questid, step, kind, target, journal) VALUES (80, 1, 'talk', 'Kellina', 'See Kellina.')`)
	f.exec("INSERT INTO questunlocks VALUES (80, 'waysender ti`thox', 'the GROBB')")
	if err := f.qe.Reload(); err != nil {
		t.Fatal(err)
	}
	f.msgs()

	f.tw.talk(thox)
	if got := optionBox(t, f.tw.s); !slices.Equal(got, []string{"Yes", "No"}) {
		t.Fatalf("without the writ Ti`Thox asked %q, want Freeport alone (Yes, No)", got)
	}
	if _, err := f.qe.SetCharacterQuest(f.sid, 80, QuestComplete, 0); err != nil {
		t.Fatal(err)
	}
	f.tw.talk(thox)
	if got := optionBox(t, f.tw.s); !slices.Equal(got, []string{"Freeport (100 tunar)", "Grobb (225 tunar)"}) {
		t.Fatalf("with the writ finished Ti`Thox offered %q", got)
	}

	// both trips closed: the waysender says what to finish
	f.exec(`INSERT INTO quests (questid, name, enabled, giver) VALUES (81, 'The Freeport Pass', 1, 'Nobody Here')`)
	f.exec(`INSERT INTO queststeps (questid, step, kind, target, journal) VALUES (81, 1, 'talk', 'Kellina', 'x')`)
	f.exec("INSERT INTO questunlocks VALUES (81, 'Waysender Ti`Thox', 'Freeport')")
	if _, err := f.qe.SetCharacterQuest(f.sid, 80, QuestReset, 0); err != nil {
		t.Fatal(err)
	}
	if err := f.qe.Reload(); err != nil {
		t.Fatal(err)
	}
	f.msgs()
	f.tw.talk(thox)
	got := said(f.tw)
	if len(got) != 0 {
		t.Logf("lines: %q", got)
	}
	boxes := dialogueBoxes(f.tw.s)
	if !slices.Equal(boxes, []string{"You may not travel to Freeport until you have finished The Freeport Pass."}) {
		t.Fatalf("with every trip closed Ti`Thox said %q", boxes)
	}

	// a draft's unlock bars no one
	f.exec(`UPDATE quests SET enabled = 0 WHERE questid IN (80, 81)`)
	if err := f.qe.Reload(); err != nil {
		t.Fatal(err)
	}
	f.tw.talk(thox)
	if got := optionBox(t, f.tw.s); !slices.Equal(got, []string{"Freeport (100 tunar)", "Grobb (225 tunar)"}) {
		t.Fatalf("the control: the unlocking quests drafts, Ti`Thox offered %q", got)
	}
}

// An unlock's route is found by its NPC and place as routeKey matches them;
// a place spelled another way is no route.
func TestAnUnlockFindsItsRoute(t *testing.T) {
	if r, ok := RouteOf("necrologist zizclank", "Plane of Disease"); !ok || r.To != "the Plane of Disease" {
		t.Errorf("Zizclank's trip: %+v %v", r, ok)
	}
	if _, ok := RouteOf("Waysender Windfoot", "Klick'Anon"); ok {
		t.Error("Klick'Anon is not Windfoot's Klik'Anon: a letter is another place")
	}
	if _, ok := RouteOf("Waysender Windfoot", "klik`anon"); !ok {
		t.Error("the control: klik`anon is Klik'Anon")
	}
	routes := TravelRoutes()
	if len(routes) < 10 || !slices.Contains(routes, TravelRoute{NPC: "Waysender Brindel", To: "Lavastorm"}) {
		t.Errorf("the routes: %+v", routes)
	}
}

// dialogueBoxes is the 0x0046 boxes queued for the session, their words.
func dialogueBoxes(s *session.Session) []string {
	var out []string
	for _, m := range questMsgs(s) {
		if strings.HasPrefix(m, "box:") {
			out = append(out, strings.TrimPrefix(m, "box:"))
		}
	}
	return out
}

// A step's choice (the guides' "choose your epic weapon"): done at its NPC,
// the step asks its question with an answer an option; an option that stays
// says its words and does nothing (talked to again, it asks again); a picked
// one says its words, gives its rewards besides the step's own and does the
// step, the other options' rewards not given. The marks name the choice. A
// step with an answer of its own asks that first.
func TestAStepsChoiceGivesThePickedOption(t *testing.T) {
	f := newQuestFixture(t)
	f.tw.p.HP = 100
	f.exec(`UPDATE quests SET enabled = 0 WHERE questid = 10010`)
	blade, staff := f.pattern("Ash Blade", 1), f.pattern("Ash Staff", 1)
	orinas := f.tw.add(1, &entity.NPC{ServerID: 741, Name: "Orinas", SpawnX: 25403.5, SpawnY: 54.1, SpawnZ: 15725.5})
	f.exec(`INSERT INTO quests (questid, name, enabled, giver, offer) VALUES (90, 'The Epic', 1, 'Orinas', 'Your weapon awaits.')`)
	f.exec(fmt.Sprintf(`INSERT INTO queststeps (questid, step, kind, target, journal, xp, choices) VALUES (90, 1, 'talk', 'Orinas',
		'Choose your weapon.', 100, '{"ask": "Which weapon?", "options": [{"answer": "The blade.", "says": "It is yours.", "items": "%d"},
		{"answer": "The staff.", "items": "%d", "xp": 10}, {"answer": "Neither.", "says": "Then think on it.", "stay": true}]}')`, blade, staff))
	if err := f.qe.Reload(); err != nil {
		t.Fatal(err)
	}
	f.msgs()
	f.qe.SetMarkers(f.tw.s, f.tw.p, true)
	marks, _ := sentMarks(t, f.tw.s)
	f.qe.SetMarkers(f.tw.s, f.tw.p, false)
	if len(marks) != 1 || marks[0].quests[0].detail != "100 XP, a choice of Ash Blade or Ash Staff, 10 XP" {
		t.Fatalf("the marks: %+v", marks)
	}

	f.tw.talk(orinas) // the offer, taken at once
	f.msgs()
	f.tw.talk(orinas)
	m := f.msgs()
	if len(m) != 1 || !strings.HasSuffix(m[0], ":Which weapon?|The blade.|The staff.|Neither.") {
		t.Fatalf("the choice: %q", m)
	}
	f.tw.pick(boxID(m), 2)
	if m := f.msgs(); len(m) != 1 || m[0] != "box:Orinas: Then think on it." {
		t.Fatalf("an option that stays: %q", m)
	}
	if s, _ := f.progress(90); s != 1 {
		t.Fatalf("an option that stays did the step (step %d)", s)
	}
	f.tw.talk(orinas)
	f.tw.pick(boxID(f.msgs()), 1)
	m = f.msgs()
	f.want(m, "box:You have received Ash Staff.", "box:"+questFinished, "chat:You receive 110 XP.")
	f.not(m, "box:You have received Ash Blade.")
	if f.held(staff) != 1 || f.held(blade) != 0 {
		t.Errorf("held: staff %d blade %d, want 1 0", f.held(staff), f.held(blade))
	}

	// an answer of the step's own first, then the choice
	f.exec(`INSERT INTO quests (questid, name, enabled, giver) VALUES (91, 'Again', 1, 'Nobody Here')`)
	f.exec(`INSERT INTO queststeps (questid, step, kind, target, journal, answer, waiting, choices) VALUES (91, 1, 'talk', 'Orinas',
		'x', 'I am ready.', 'You return.', '{"options": [{"answer": "Gold."}, {"answer": "Glory."}]}')`)
	if err := f.qe.Reload(); err != nil {
		t.Fatal(err)
	}
	if _, err := f.qe.SetCharacterQuest(f.sid, 91, QuestGive, 0); err != nil {
		t.Fatal(err)
	}
	f.msgs()
	f.tw.talk(orinas)
	m = f.msgs()
	if len(m) != 1 || !strings.HasSuffix(m[0], ":You return.|I am ready.") {
		t.Fatalf("the step's own answer first: %q", m)
	}
	f.tw.pick(boxID(m), 0)
	m = f.msgs()
	if len(m) != 1 || !strings.HasSuffix(m[0], ":"+questAskChoice+"|Gold.|Glory.") {
		t.Fatalf("then the choice, with ours for its question: %q", m)
	}
	f.tw.pick(boxID(m), 0)
	f.want(f.msgs(), "box:"+questFinished)
}

// A trip only a quest opens (the Portal guides': the portal guardian's
// Zaoran Plateau, for 5,000 tunar a trip): with no quest unlocking it, or only a
// draft, no one is offered it and the guardian says so; once a quest in the
// game unlocks it, he names that quest; finished, it is offered with its fee
// and lands on its place, not in front of an NPC. A player who finished the
// quest keeps the trip once the quest is a draft again.
func TestAQuestOnlyTripWaitsForItsQuest(t *testing.T) {
	f := newQuestFixture(t)
	f.exec(`UPDATE quests SET enabled = 0 WHERE questid = 10010`)
	guardian := f.tw.add(1, &entity.NPC{ServerID: 117983, Name: "portal guardian", SpawnX: 25403.5, SpawnY: 54.1, SpawnZ: 15725.5})
	f.tw.h.spend = func(_ *session.Session, p *entity.Player, n int) bool {
		if n > p.Tunar {
			return false
		}
		p.Tunar -= n
		return true
	}
	boxes := func() []string {
		f.tw.talk(guardian)
		return dialogueBoxes(f.tw.s)
	}
	if got := boxes(); !slices.Equal(got, []string{"You may not travel to Zaoran Plateau yet."}) {
		t.Fatalf("no quest unlocks it: %q", got)
	}
	f.exec(`INSERT INTO quests (questid, name, enabled, giver) VALUES (95, 'The Plateau', 0, 'Nobody Here')`)
	f.exec(`INSERT INTO queststeps (questid, step, kind, target, journal) VALUES (95, 1, 'talk', 'Kellina', 'x')`)
	f.exec(`INSERT INTO questunlocks VALUES (95, 'portal guardian', 'Zaoran Plateau')`)
	if err := f.qe.Reload(); err != nil {
		t.Fatal(err)
	}
	if got := boxes(); !slices.Equal(got, []string{"You may not travel to Zaoran Plateau yet."}) {
		t.Fatalf("a draft unlocks it: %q", got)
	}
	f.exec(`UPDATE quests SET enabled = 1 WHERE questid = 95`)
	if err := f.qe.Reload(); err != nil {
		t.Fatal(err)
	}
	if got := boxes(); !slices.Equal(got, []string{"You may not travel to Zaoran Plateau until you have finished The Plateau."}) {
		t.Fatalf("a quest in the game unlocks it: %q", got)
	}
	if _, err := f.qe.SetCharacterQuest(f.sid, 95, QuestComplete, 0); err != nil {
		t.Fatal(err)
	}
	f.exec(`UPDATE quests SET enabled = 0 WHERE questid = 95`) // a draft again: the finisher keeps it
	if err := f.qe.Reload(); err != nil {
		t.Fatal(err)
	}
	f.tw.p.Tunar = 5012
	f.tw.talk(guardian)
	if got := optionBox(t, f.tw.s); !slices.Equal(got, []string{"Yes", "No"}) {
		t.Fatalf("finished, the guardian asked %q", got)
	}
	f.tw.pick(guardian, 0)
	x, z, world, moved := teleported(f.tw.s)
	if !moved || world != 5 || x != 4899 || z != 7049 || f.tw.p.Tunar != 12 {
		t.Errorf("Yes: to (%.1f, %.1f) world %d (%v), tunar %d; want (4899, 7049) world 5 for 5000", x, z, world, moved, f.tw.p.Tunar)
	}
}

// Norach (the owner, 2026-11-11: "he will teleport you to Isle of Dread onto
// the little raft for 10k", then "Keep the medallion gate"): a player who has
// not finished The Medallion of the Faceless is refused; one who has is asked
// the trip for 10,000; short of it, told the fare and kept, tunar untouched;
// with it, pays and lands standing on the raft's deck in the cove south of
// Graumord, facing the pier. A finisher keeps the trip if the quest is later
// made a draft (QuestOnly), and no one else gets it then.
func TestNorachFerriesToTheRaftForTenThousand(t *testing.T) {
	f := newQuestFixture(t)
	f.exec(`UPDATE quests SET enabled = 0 WHERE questid = 10010`)
	f.exec(`INSERT INTO quests (questid, name, enabled, giver, minlevel) VALUES (101309, 'The Medallion of the Faceless', 1, 'Norach', 50)`)
	f.exec(`INSERT INTO queststeps (questid, step, kind, target, journal) VALUES (101309, 1, 'give', 'Norach', 'x')`)
	f.exec(`INSERT INTO questunlocks VALUES (101309, 'Norach', 'the Isle of Dread')`)
	if err := f.qe.Reload(); err != nil {
		t.Fatal(err)
	}
	norach := f.tw.add(1, &entity.NPC{ServerID: 117982, Name: "Norach", SpawnX: 25403.5, SpawnY: 54.1, SpawnZ: 15725.5})
	f.tw.h.spend = func(_ *session.Session, p *entity.Player, n int) bool {
		if n > p.Tunar {
			return false
		}
		p.Tunar -= n
		return true
	}
	f.tw.talk(norach)
	if got := dialogueBoxes(f.tw.s); !slices.Equal(got, []string{"You may not travel to the Isle of Dread until you have finished The Medallion of the Faceless."}) {
		t.Fatalf("not finished: %q", got)
	}
	if _, err := f.qe.SetCharacterQuest(f.sid, 101309, QuestComplete, 0); err != nil {
		t.Fatal(err)
	}
	f.tw.p.Tunar = 9999
	f.tw.talk(norach)
	if got := optionBox(t, f.tw.s); !slices.Equal(got, []string{"Yes", "No"}) {
		t.Fatalf("Norach asked %q", got)
	}
	f.tw.pick(norach, 0)
	if _, _, _, moved := teleported(f.tw.s); moved || f.tw.p.Tunar != 9999 {
		t.Fatalf("short of the fare: moved %v, tunar %d", moved, f.tw.p.Tunar)
	}
	if got := dialogueBoxes(f.tw.s); !slices.Equal(got, []string{"The trip to the Isle of Dread costs 10000 tunar."}) {
		t.Fatalf("short of the fare, told %q", got)
	}
	f.tw.p.Tunar = 12000
	f.tw.talk(norach)
	if got := optionBox(t, f.tw.s); !slices.Equal(got, []string{"Yes", "No"}) {
		t.Fatalf("Norach asked %q", got)
	}
	f.tw.pick(norach, 0)
	x, z, world, moved := teleported(f.tw.s)
	y, _ := teleportedY(f.tw.s)
	if !moved || world != 0 || x != 24578 || z != 3626 || y != 52.6 || f.tw.p.Tunar != 2000 {
		t.Errorf("Yes: to (%.1f, %.2f, %.1f) world %d (%v), tunar %d; want the raft (24578, 52.6, 3626) world 0 for 10000",
			x, y, z, world, moved, f.tw.p.Tunar)
	}
	for _, m := range f.tw.s.Queue().PeekPending() {
		if d := m.Data(); len(d) >= 19 && protocol.GameOpcode(binary.LittleEndian.Uint16(d)) == protocol.OpcodeTeleport {
			if face := f32(d[15:]); face != 3.9 {
				t.Errorf("landed facing %v, want 3.9 (the pier)", face)
			}
		}
	}
}

// The Plane of Sky's way back to the third isle of rain (the armourers' hut): any
// essence of rain, for a player the chain has brought there and no one else.
func TestTheWayBackToTheHutWaitsForTheChain(t *testing.T) {
	f := newQuestFixture(t)
	for _, q := range []struct {
		id             int
		name, requires string
	}{{101, "The Key of Rain", ""}, {102, "Sterie Glitterlight", "101"}, {103, "The Necrologist of Rain", "102"}} {
		f.exec(`INSERT INTO quests (questid, name, enabled, giver, requires) VALUES (?, ?, 1, 'Nobody Here', ?)`, q.id, q.name, q.requires)
		f.exec(`INSERT INTO queststeps (questid, step, kind, target, journal) VALUES (?, 1, 'talk', 'Kellina', 'x')`, q.id)
	}
	f.exec(`INSERT INTO questunlocks VALUES (102, 'essence of rain', 'Rain 3')`)
	f.exec(`INSERT INTO questunlocks VALUES (103, 'essence of rain', 'Rain 1')`)
	if err := f.qe.Reload(); err != nil {
		t.Fatal(err)
	}
	essence := f.tw.add(1, &entity.NPC{ServerID: 119510, Name: "essence of rain", SpawnX: 25403.5, SpawnY: 54.1, SpawnZ: 15725.5})
	const shut = "You may not travel to Rain 1 until you have finished The Necrologist of Rain."
	refused := func(when string) {
		t.Helper()
		f.tw.talk(essence)
		if got := dialogueBoxes(f.tw.s); !slices.Equal(got, []string{shut}) {
			t.Fatalf("%s: %q", when, got)
		}
		if _, _, _, moved := teleported(f.tw.s); moved {
			t.Fatalf("%s: it moved the player", when)
		}
	}
	done := func(id int) {
		t.Helper()
		if _, err := f.qe.SetCharacterQuest(f.sid, id, QuestComplete, 0); err != nil {
			t.Fatal(err)
		}
	}
	refused("none of the chain")
	done(101)
	refused("the first quest alone")

	done(102)
	f.tw.talk(essence)
	if got := optionBox(t, f.tw.s); !slices.Equal(got, []string{"Yes", "No"}) {
		t.Fatalf("the second quest done, the essence asked %q", got)
	}
	f.tw.pick(essence, 0)
	x, z, world, moved := teleported(f.tw.s)
	if !moved || world != 4 || x != 4614.2 || z != 4936.5 {
		t.Errorf("Yes: to (%.1f, %.1f) world %d (%v); want the hut (4614.2, 4936.5) in world 4", x, z, world, moved)
	}
	// again, whenever it likes: the quest done is all it asks, nothing is spent
	if dests, _, locked := wayDests(f.tw.h.gameWorld, f.tw.s, "essence of rain", f.tw.p); len(dests) != 1 || dests[0].Name != "Rain 3" || locked.place != "Rain 1" {
		t.Errorf("after the trip: %+v, locked %+v", dests, locked)
	}

	done(103)
	dests, _, locked := wayDests(f.tw.h.gameWorld, f.tw.s, "essence of rain", f.tw.p)
	if len(dests) != 2 || dests[0].Name != "Rain 1" || dests[1].Name != "Rain 3" || locked.place != "" {
		t.Errorf("the whole chain done: %+v, locked %+v", dests, locked)
	}
}
