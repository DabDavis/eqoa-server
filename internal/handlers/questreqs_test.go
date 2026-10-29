package handlers

import (
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/eqoa/server/internal/entity"
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
