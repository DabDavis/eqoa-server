package handlers

import (
	"fmt"
	"testing"
	"time"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
)

// Live petition 5663 (the owner, 2026-11-26: "fix these"): the Were-Hunter
// Trials' clocks, wards and prices as eqoa.forumotion.com t235 has them
// (questtrials.go). The quests here are the tests' own, shaped as the trials'.

const trialFaction = "Werehunters of Tunaria"

// trialQuest is a quest of four steps: 1 a talk to the Delegate, 2 a give of
// the letter to Nivaan (met without it: -250 and back to 1, to be earned
// back), 3 a talk to Iruko whose doing gives the totem and starts a clock of
// ten minutes to step 5 (-75 and back to 3), 4 Iruko's question (the wrong
// answer -75 and back to 3, earned back; the right one on), 5 a talk to Urtoe.
func trialQuest(t *testing.T) (f *questFixture, delegate, nivaan, iruko, urtoe uint32, letter, totem int) {
	t.Helper()
	f = newQuestFixture(t)
	letter, totem = f.pattern("sealed letter", 1), f.pattern("Concentration Totem", 1)
	delegate = f.tw.add(3, &entity.NPC{ServerID: 801, Name: "Delegate Tenrathe", SpawnX: 25403, SpawnY: 54.1, SpawnZ: 15726})
	nivaan = f.tw.add(4, &entity.NPC{ServerID: 802, Name: "Nivaan D'Tarin", SpawnX: 25404, SpawnY: 54.1, SpawnZ: 15726})
	iruko = f.tw.add(5, &entity.NPC{ServerID: 803, Name: "Trainer Iruko", SpawnX: 25403, SpawnY: 54.1, SpawnZ: 15724})
	urtoe = f.tw.add(6, &entity.NPC{ServerID: 804, Name: "Assistant Urtoe", SpawnX: 25402, SpawnY: 54.1, SpawnZ: 15724})
	f.exec(`INSERT INTO quests (questid, name, enabled, giver) VALUES (5663, 'The WereHunter Trials', 1, 'Delegate Tenrathe')`)
	f.exec(`INSERT INTO queststeps (questid, step, kind, target, journal, says, events) VALUES (5663, 1, 'talk', 'Delegate Tenrathe',
		'See the Delegate.', 'Take this letter.', ?)`, fmt.Sprintf(`[{"on":"done","do":"give","items":"%d"}]`, letter))
	f.exec(`INSERT INTO queststeps (questid, step, kind, target, items, journal, notyet, says, events) VALUES (5663, 2, 'give', 'Nivaan D''Tarin', ?,
		'Bring Nivaan the letter.', 'Where is the Delegate''s sealed letter?', 'Remember the warm air.', ?)`, fmt.Sprint(letter),
		`[{"on":"lacking","do":"back","back":1,"factions":"Werehunters of Tunaria:-250","regain":true,"npc":"Nivaan D'Tarin","text":"Go back to the Delegate."}]`)
	f.exec(`INSERT INTO queststeps (questid, step, kind, target, journal, says, events) VALUES (5663, 3, 'talk', 'Trainer Iruko',
		'Speak with Iruko when ready.', 'Your ten minutes begin now.', ?)`, fmt.Sprintf(
		`[{"on":"done","do":"give","items":"%d"},{"on":"done","do":"clock","minutes":10,"until":5,"back":3,"factions":"Werehunters of Tunaria:-75","regain":true,"text":"Too slow. Begin again."}]`, totem))
	f.exec(`INSERT INTO queststeps (questid, step, kind, target, journal, choices) VALUES (5663, 4, 'talk', 'Trainer Iruko', 'Answer Iruko.', ?)`,
		`{"ask":"Where stood the lionwere?","options":[{"answer":"The first.","says":"No. Begin again.","back":3,"factions":"Werehunters of Tunaria:-75","regain":true},{"answer":"The third.","says":"That is so."}]}`)
	f.exec(`INSERT INTO queststeps (questid, step, kind, target, journal, says) VALUES (5663, 5, 'talk', 'Assistant Urtoe', 'See Urtoe.', 'Well hunted!')`)
	f.qe.ReloadQuests()
	f.standWith(trialFaction, 1000)
	if msg, err := f.qe.SetCharacterQuest(f.sid, 5663, QuestGive, 0); err != nil {
		t.Fatalf("%s %v", msg, err)
	}
	f.msgs()
	return
}

// A wrong answer costs 75, puts the quest back to Iruko's first step and Iruko will not begin again until
// the 75 is earned back; the control: the standing back, the step is done; and
// the right answer goes on with nothing lost.
func TestATrialsWrongAnswerCostsStandingUntilEarnedBack(t *testing.T) {
	f, delegate, _, iruko, _, _, _ := trialQuest(t)
	f.tw.talk(delegate)
	f.msgs()
	f.exec(`UPDATE queststeps SET events = '' WHERE questid = 5663 AND step = 3`) // no clock in this test
	f.qe.ReloadQuests()
	f.exec(`UPDATE charquests SET step = 3 WHERE serverid = ? AND questid = 5663`, f.sid)
	f.qe.ReloadCharacter(f.sid)
	f.tw.talk(iruko)
	f.msgs()
	if s, _ := f.progress(5663); s != 4 {
		t.Fatalf("Iruko's first step: step %d, want 4", s)
	}
	f.tw.talk(iruko)
	f.tw.pick(boxID(f.msgs()), 0) // the first: wrong
	m := f.msgs()
	f.want(m, "box:Trainer Iruko: No. Begin again.")
	if s, _ := f.progress(5663); s != 3 {
		t.Fatalf("a wrong answer: step %d, want 3", s)
	}
	if v := f.tw.p.Factions[trialFaction]; v != 925 {
		t.Fatalf("a wrong answer: standing %d, want 925", v)
	}
	// owed: Iruko will not begin again
	f.tw.talk(iruko)
	f.want(f.msgs(), "box:Trainer Iruko: You have lost the trust of the Werehunters of Tunaria. Earn it back, then come to me again.")
	if s, _ := f.progress(5663); s != 3 {
		t.Fatalf("owing: step %d, want 3", s)
	}
	// earned back: he does
	f.standWith(trialFaction, 1000)
	f.tw.talk(iruko)
	f.msgs()
	if s, _ := f.progress(5663); s != 4 {
		t.Fatalf("earned back: step %d, want 4", s)
	}
	// the right answer: on, nothing lost
	f.tw.talk(iruko)
	f.tw.pick(boxID(f.msgs()), 1)
	f.want(f.msgs(), "box:Trainer Iruko: That is so.")
	if s, _ := f.progress(5663); s != 5 || f.tw.p.Factions[trialFaction] != 1000 {
		t.Fatalf("the right answer: step %d standing %d, want 5 and 1000", s, f.tw.p.Factions[trialFaction])
	}
}

// Nivaan met without the letter: his words, -250 and back to the Delegate
// (who gives a letter again), to be earned back; the control: with the
// letter, the step is done and nothing lost.
func TestATrialsMissingItemCostsStandingAndGoesBack(t *testing.T) {
	f, delegate, nivaan, _, _, letter, _ := trialQuest(t)
	f.tw.talk(delegate)
	f.msgs()
	if s, _ := f.progress(5663); s != 2 || f.held(letter) != 1 {
		t.Fatalf("the Delegate: step %d letters %d", s, f.held(letter))
	}
	f.exec(`DELETE FROM charinventory WHERE serverid = ? AND patternid = ?`, f.sid, letter) // lost on the way
	f.tw.s.SetInventoryList(newInvList(nil))
	f.tw.talk(nivaan)
	m := f.msgs()
	f.want(m, "box:Nivaan D'Tarin: Where is the Delegate's sealed letter?", "box:Nivaan D'Tarin: Go back to the Delegate.")
	if s, _ := f.progress(5663); s != 1 || f.tw.p.Factions[trialFaction] != 750 {
		t.Fatalf("without the letter: step %d standing %d, want 1 and 750", s, f.tw.p.Factions[trialFaction])
	}
	f.tw.talk(delegate)
	f.want(f.msgs(), "box:Delegate Tenrathe: You have lost the trust of the Werehunters of Tunaria. Earn it back, then come to me again.")
	f.standWith(trialFaction, 1000)
	f.tw.talk(delegate)
	f.msgs()
	if s, _ := f.progress(5663); s != 2 || f.held(letter) != 1 {
		t.Fatalf("earned back, the Delegate again: step %d letters %d", s, f.held(letter))
	}
	f.tw.talk(nivaan)
	m = f.msgs()
	f.want(m, "box:Nivaan D'Tarin: Remember the warm air.")
	if s, _ := f.progress(5663); s != 3 || f.tw.p.Factions[trialFaction] != 1000 {
		t.Fatalf("with the letter: step %d standing %d, want 3 and 1000", s, f.tw.p.Factions[trialFaction])
	}
}

// A clock: its warnings at five minutes and at one, and its time up short of
// its step is the setback (words, -75, back, the totem taken back); the
// control: its step reached in time stops it, nothing lost.
func TestATrialsClockRunsOutShortOfItsStep(t *testing.T) {
	f, _, _, iruko, urtoe, _, totem := trialQuest(t)
	f.exec(`UPDATE charquests SET step = 3 WHERE serverid = ? AND questid = 5663`, f.sid)
	f.qe.ReloadCharacter(f.sid)
	f.tw.talk(iruko) // the totem, and the clock
	f.msgs()
	if s, _ := f.progress(5663); s != 4 || f.held(totem) != 1 {
		t.Fatalf("Iruko: step %d totems %d", s, f.held(totem))
	}
	now := time.Now()
	f.qe.checkTrials(now.Add(2 * time.Minute))
	f.not(f.msgs(), "chat:*")
	f.qe.checkTrials(now.Add(5*time.Minute + 30*time.Second))
	f.want(f.msgs(), "chat:The WereHunter Trials: 5 minutes remain.")
	f.qe.checkTrials(now.Add(9*time.Minute + 30*time.Second))
	f.want(f.msgs(), "chat:The WereHunter Trials: one minute remains.")
	f.qe.checkTrials(now.Add(10*time.Minute + time.Second))
	m := f.msgs()
	f.want(m, "box:Too slow. Begin again.")
	if s, _ := f.progress(5663); s != 3 || f.tw.p.Factions[trialFaction] != 925 || f.held(totem) != 0 {
		t.Fatalf("time up: step %d standing %d totems %d, want 3, 925, 0", s, f.tw.p.Factions[trialFaction], f.held(totem))
	}
	var rows int
	f.db.QueryRow(`SELECT COUNT(*) FROM charquesttrials WHERE serverid = ? AND questid = 5663 AND deadline = 0 AND owes <> ''`, f.sid).Scan(&rows)
	if rows != 1 {
		t.Fatalf("time up: %d rows of no clock and a standing owed, want 1", rows)
	}

	// the control: in time
	f.standWith(trialFaction, 1000)
	f.tw.talk(iruko)
	f.msgs()
	start := time.Now()
	f.tw.talk(iruko)
	f.tw.pick(boxID(f.msgs()), 1)
	f.tw.talk(urtoe) // step 5, the clock's until, reached: and done
	f.msgs()
	f.qe.checkTrials(start.Add(11 * time.Minute))
	if m := f.msgs(); hasMsg(m, "box:Too slow. Begin again.") {
		t.Fatalf("done in time, yet the setback: %q", m)
	}
	if _, done := f.progress(5663); done != 1 || f.tw.p.Factions[trialFaction] != 1000 {
		t.Fatalf("in time: done %d standing %d", done, f.tw.p.Factions[trialFaction])
	}
}

// A clock runs over a restart: the engine that reads the character again
// still has it, and runs it out.
func TestATrialsClockIsKeptOverARestart(t *testing.T) {
	f, _, _, iruko, _, _, _ := trialQuest(t)
	f.exec(`UPDATE charquests SET step = 3 WHERE serverid = ? AND questid = 5663`, f.sid)
	f.qe.ReloadCharacter(f.sid)
	f.tw.talk(iruko)
	f.msgs()
	f.qe.ReloadCharacter(f.sid) // the character read again, as at a login after a restart
	f.qe.checkTrials(time.Now().Add(11 * time.Minute))
	f.want(f.msgs(), "box:Too slow. Begin again.")
	if s, _ := f.progress(5663); s != 3 {
		t.Fatalf("read again and the time up: step %d, want 3", s)
	}
}

// A ward: the player coming into its place while on its step is the setback,
// and the steps passed back over are undone (what a give took handed back,
// what its doing gave taken back: Irlynn Poe's ring); the control: outside it,
// nothing.
func TestATrialsWardScaresTheBeastOff(t *testing.T) {
	f := newQuestFixture(t)
	ring, radiant := f.pattern("Arcane WH Band", 1), f.pattern("Radiant Arcane Band", 1)
	f.tw.add(3, &entity.NPC{ServerID: 801, Name: "Trainer Iruko", SpawnX: 25403, SpawnY: 54.1, SpawnZ: 15726})
	poe := f.tw.add(4, &entity.NPC{ServerID: 802, Name: "Irlynn Poe", SpawnX: 25404, SpawnY: 54.1, SpawnZ: 15726})
	f.exec(`INSERT INTO quests (questid, name, enabled, giver) VALUES (5664, 'The WereHunter Trials', 1, 'Trainer Iruko')`)
	f.exec(`INSERT INTO queststeps (questid, step, kind, target, journal) VALUES (5664, 1, 'talk', 'Trainer Iruko', 'The first trial.')`)
	f.exec(`INSERT INTO queststeps (questid, step, kind, target, items, journal, events) VALUES (5664, 2, 'give', 'Irlynn Poe', ?, 'Bring Poe your band.', ?)`,
		fmt.Sprint(ring), fmt.Sprintf(`[{"on":"done","do":"give","items":"%d"}]`, radiant))
	f.exec(`INSERT INTO queststeps (questid, step, kind, target, journal, events) VALUES (5664, 3, 'kill', 'vicious wolfwere', 'Kill the beast.', ?)`,
		`[{"on":"start","do":"ward","world":0,"x":26000,"z":15000,"radius":20,"back":1,"text":"The were-beast catches your scent and flees. Begin the first trial again."}]`)
	f.qe.ReloadQuests()
	f.exec(`INSERT INTO charquests (serverid, questid, step) VALUES (?, 5664, 2)`, f.sid)
	f.qe.ReloadCharacter(f.sid)
	f.own(ring, 1)
	f.tw.talk(poe)
	f.msgs()
	if s, _ := f.progress(5664); s != 3 || f.held(ring) != 0 || f.held(radiant) != 1 {
		t.Fatalf("Poe: step %d rings %d radiant %d", s, f.held(ring), f.held(radiant))
	}
	f.tw.p.HP, f.tw.p.MaxHP = 100, 100 // alive: a dead player is in no ward
	f.tw.p.X, f.tw.p.Z = 26030, 15000  // outside the ward
	f.qe.checkTrials(time.Now())
	if s, _ := f.progress(5664); s != 3 {
		t.Fatalf("outside the ward: step %d, want 3", s)
	}
	f.tw.p.X, f.tw.p.Z = 26010, 15005 // in it
	f.qe.checkTrials(time.Now())
	m := f.msgs()
	f.want(m, "box:The were-beast catches your scent and flees. Begin the first trial again.", "box:Radiant Arcane Band is taken back.")
	if s, _ := f.progress(5664); s != 1 || f.held(ring) != 1 || f.held(radiant) != 0 {
		t.Fatalf("in the ward: step %d rings %d radiant %d, want 1, 1, 0", s, f.held(ring), f.held(radiant))
	}
}

// The trials' events and options are checked as the panel saves them.
func TestATrialsEventsAreChecked(t *testing.T) {
	good := `[{"on":"done","do":"clock","minutes":10,"until":15,"back":4,"factions":"Werehunters of Tunaria:-75","regain":true},
		{"on":"start","do":"ward","world":1,"x":7015,"z":2935,"radius":20,"back":4},
		{"on":"lacking","do":"back","back":1,"factions":"Werehunters of Tunaria:-250"}]`
	if p := database.EventProblems(good, true); len(p) != 0 {
		t.Fatalf("good events: %q", p)
	}
	for _, bad := range []string{
		`[{"on":"done","do":"clock","minutes":10,"until":3,"back":4}]`,  // until not after back
		`[{"on":"done","do":"ward","x":1,"z":1,"radius":5,"back":1}]`,   // a ward on done
		`[{"on":"start","do":"ward","radius":5,"back":1}]`,              // no place
		`[{"on":"done","do":"back","back":1}]`,                          // a back not on lacking
		`[{"on":"lacking","do":"back","back":1,"factions":"nonsense"}]`, // factions unread
		`[{"on":"accept","do":"clock","minutes":5,"until":3,"back":1}]`, // a quest's
	} {
		if p := database.EventProblems(bad, true); len(p) == 0 {
			t.Errorf("no problem found in %s", bad)
		}
	}
	if p := database.ChoiceProblems(`{"ask":"?","options":[{"answer":"No.","back":3,"stay":true}]}`, "talk"); len(p) == 0 {
		t.Error("an option that puts back and stays: no problem found")
	}
}
