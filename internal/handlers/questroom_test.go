package handlers

import (
	"fmt"
	"strings"
	"testing"

	"github.com/eqoa/server/internal/entity"
)

// roomQuest is the quest fixture with the pincers quest taken and its talk
// step done: Kellina wants two pincers for a Smoldering Aura (and 430
// experience). It returns her object id.
func roomQuest(t *testing.T) (*questFixture, uint32) {
	t.Helper()
	f := newQuestFixture(t)
	malsis := f.tw.add(1, &entity.NPC{ServerID: 701, Name: "Malsis", SpawnX: 25403.5, SpawnY: 54.1, SpawnZ: 15725.5})
	kellina := f.tw.add(2, &entity.NPC{ServerID: 702, Name: "Kellina", SpawnX: 25404, SpawnY: 54.1, SpawnZ: 15726})
	f.tw.talk(malsis)
	f.tw.pick(boxID(f.msgs()), 0)
	f.msgs()
	f.tw.talk(kellina)
	f.msgs()
	if s, _ := f.progress(10010); s != 2 {
		t.Fatalf("at step %d, want 2", s)
	}
	return f, kellina
}

// fill puts records of a filler item that does not stack in the pack until
// it holds n records.
func (f *questFixture) fill(n int) int {
	f.t.Helper()
	filler := f.pattern("cracked shell", 1)
	for {
		var have int
		f.db.QueryRow(`SELECT COUNT(*) FROM charinventory WHERE serverid = ? AND location = 0`, f.sid).Scan(&have)
		if have >= n {
			return filler
		}
		f.own(filler, 1)
	}
}

// drop takes one record of an item out of the pack (the player making room).
func (f *questFixture) drop(pattern int) {
	f.t.Helper()
	f.exec(`DELETE FROM charinventory WHERE itemid = (SELECT MIN(itemid) FROM charinventory WHERE serverid = ? AND patternid = ?)`, f.sid, pattern)
	rows, err := f.inv.GetCharacterItems(f.sid)
	if err != nil {
		f.t.Fatal(err)
	}
	f.tw.s.SetInventoryList(newInvList(rows))
}

// handIn answers Kellina's question with the pincers held and returns what
// was sent.
func (f *questFixture) handIn(kellina uint32) []string {
	f.t.Helper()
	f.tw.talk(kellina)
	m := f.msgs()
	if !strings.Contains(strings.Join(m, "\n"), "I have the pincers.") {
		f.t.Fatalf("Kellina asks no hand-in: %q", m)
	}
	f.tw.pick(boxID(m), 0)
	return f.msgs()
}

// A full pack: the giver says so and nothing changes hands. Three pincers
// (the hand-in takes two, the record stays) and 39 more records leave no room
// for the aura: no pincer taken, no aura, no experience, the step to do. The
// control: one record dropped, the same answer does the step, the aura in the
// last free record.
func TestAFullPackWaitsForRoom(t *testing.T) {
	f, kellina := roomQuest(t)
	f.own(f.pincer, 3)
	filler := f.fill(40)
	m := f.handIn(kellina)
	f.want(m, "box:Kellina: Your pack is too full for the Smoldering Aura. Make room and come back to me.")
	f.not(m, "xp", "item", "take:*", "box:"+questFinished)
	if f.held(f.pincer) != 3 || f.held(f.aura) != 0 {
		t.Fatalf("refused, yet pincers %d aura %d", f.held(f.pincer), f.held(f.aura))
	}
	if s, d := f.progress(10010); s != 2 || d != 0 {
		t.Fatalf("refused, yet step %d done %d", s, d)
	}

	f.drop(filler)
	m = f.handIn(kellina)
	f.want(m, "box:"+questFinished, "box:You have received Smoldering Aura.", "xp", "item")
	if f.held(f.pincer) != 1 || f.held(f.aura) != 1 {
		t.Errorf("done: pincers %d aura %d, want 1 1", f.held(f.pincer), f.held(f.aura))
	}
	if s, d := f.progress(10010); s != 0 || d != 1 {
		t.Errorf("done: step %d done %d, want 0 1", s, d)
	}
}

// What the hand-in takes counts: two pincers in one record, taken whole,
// leave the record the aura needs in a pack of 40. The control is
// TestAFullPackWaitsForRoom's three pincers, whose record stays.
func TestAHandInMakesItsOwnRoom(t *testing.T) {
	f, kellina := roomQuest(t)
	f.own(f.pincer, 2)
	f.fill(40)
	m := f.handIn(kellina)
	f.want(m, "box:"+questFinished, "item")
	if f.held(f.pincer) != 0 || f.held(f.aura) != 1 {
		t.Errorf("pincers %d aura %d, want 0 1", f.held(f.pincer), f.held(f.aura))
	}
}

// A reward that stacks goes onto the first record of its item, as CLIENT's
// AddItem files it: with Kellina giving three bone chips, a full pack whose
// bone chips stand at 5 of 20 takes them; the control: at 18 of 20 the three
// do not go in whole, and the step waits.
func TestAStackingRewardFitsItsStack(t *testing.T) {
	for _, tc := range []struct {
		have int
		done bool
	}{{5, true}, {18, false}} {
		t.Run(fmt.Sprint(tc.have), func(t *testing.T) {
			f, kellina := roomQuest(t)
			f.exec(`UPDATE queststeps SET rewarditems = ? WHERE questid = 10010 AND step = 2`, fmt.Sprintf("%d*3", f.bone))
			f.qe.ReloadQuests()
			f.own(f.bone, tc.have)
			f.own(f.pincer, 3)
			f.fill(40)
			m := f.handIn(kellina)
			if got := f.held(f.bone); tc.done && (got != tc.have+3 || !hasMsg(m, "box:"+questFinished)) {
				t.Errorf("bone chips %d, want %d, done (%q)", got, tc.have+3, m)
			} else if !tc.done && (got != tc.have || !hasMsg(m, "box:Kellina: Your pack is too full for 3 bone chips. Make room and come back to me.")) {
				t.Errorf("bone chips %d, want %d, refused (%q)", got, tc.have, m)
			}
		})
	}
}

// A choice's pick counts: with a full pack, picking the option that gives an
// item waits; the control: the option that gives none goes through.
func TestAPickedItemNeedsRoom(t *testing.T) {
	f, kellina := roomQuest(t)
	f.exec(`UPDATE queststeps SET answer = '', choices = ? WHERE questid = 10010 AND step = 2`,
		fmt.Sprintf(`{"ask":"Which?","options":[{"answer":"The aura","items":"%d"},{"answer":"Nothing more","xp":5}]}`, f.aura))
	f.exec(`UPDATE queststeps SET rewarditems = '' WHERE questid = 10010 AND step = 2`)
	f.qe.ReloadQuests()
	f.own(f.pincer, 3)
	f.fill(40)
	f.tw.talk(kellina)
	f.tw.pick(boxID(f.msgs()), 0)
	m := f.msgs()
	f.want(m, "box:Kellina: Your pack is too full for the Smoldering Aura. Make room and come back to me.")
	if s, _ := f.progress(10010); s != 2 || f.held(f.pincer) != 3 {
		t.Fatalf("refused, yet step %d pincers %d", s, f.held(f.pincer))
	}
	f.tw.talk(kellina)
	f.tw.pick(boxID(f.msgs()), 1)
	f.want(f.msgs(), "box:"+questFinished)
	if s, d := f.progress(10010); s != 0 || d != 1 || f.held(f.aura) != 0 {
		t.Errorf("the other pick: step %d done %d aura %d", s, d, f.held(f.aura))
	}
}

// A kill step's last kill with no room: the step waits at its count, the
// player told once; room made, the next look round does it. The control: no
// room, the look round leaves it.
func TestAKillStepWaitsForRoom(t *testing.T) {
	f := newQuestFixture(t)
	f.exec(`INSERT INTO quests (questid, name, enabled, giver, minlevel, maxlevel) VALUES (10020, 'Rats', 1, 'Malsis', 1, 10)`)
	f.exec(`INSERT INTO queststeps (questid, step, kind, target, count, journal, rewarditems) VALUES (10020, 1, 'kill', 'a rat', 2, 'Kill two rats.', ?)`,
		fmt.Sprint(f.aura))
	f.qe.ReloadQuests()
	if msg, err := f.qe.SetCharacterQuest(f.sid, 10020, QuestGive, 0); err != nil {
		t.Fatalf("%s %v", msg, err)
	}
	filler := f.fill(40)
	f.msgs()
	kill := entity.KillCredit{NPCName: "a rat", Players: []entity.KillPlayer{{ServerID: f.sid, Name: "Talker", Session: f.tw.s}}}
	f.qe.killed(kill)
	f.qe.killed(kill)
	m := f.msgs()
	f.want(m, `box:Your pack is too full for the Smoldering Aura. Make room to finish "Rats".`)
	if s, _ := f.progress(10020); s != 1 || f.held(f.aura) != 0 {
		t.Fatalf("no room, yet step %d aura %d", s, f.held(f.aura))
	}
	f.qe.checkWaiting()
	if s, _ := f.progress(10020); s != 1 || len(f.msgs()) != 0 {
		t.Fatal("the control: with no room the look round did the step or said it again")
	}
	f.drop(filler)
	f.qe.checkWaiting()
	f.want(f.msgs(), "box:"+questFinished, "item")
	if s, d := f.progress(10020); s != 0 || d != 1 || f.held(f.aura) != 1 {
		t.Errorf("room made: step %d done %d aura %d", s, d, f.held(f.aura))
	}
}

// Taking a quest whose acceptance hands over an item waits for room too: not
// taken with a full pack; the control: taken once there is a free record.
func TestTakingAQuestThatGivesNeedsRoom(t *testing.T) {
	f := newQuestFixture(t)
	malsis := f.tw.add(1, &entity.NPC{ServerID: 701, Name: "Malsis", SpawnX: 25403.5, SpawnY: 54.1, SpawnZ: 15725.5})
	f.exec(`UPDATE quests SET events = ? WHERE questid = 10010`, fmt.Sprintf(`[{"on":"accept","do":"give","items":"%d"}]`, f.aura))
	f.qe.ReloadQuests()
	filler := f.fill(40)
	f.tw.talk(malsis)
	f.tw.pick(boxID(f.msgs()), 0)
	m := f.msgs()
	f.want(m, "box:Malsis: Your pack is too full for the Smoldering Aura. Make room and come back to me.")
	if s, _ := f.progress(10010); s != -1 {
		t.Fatalf("taken with a full pack: step %d", s)
	}
	f.drop(filler)
	f.tw.talk(malsis)
	f.tw.pick(boxID(f.msgs()), 0)
	f.want(f.msgs(), "box:"+questReceived, "item")
	if s, _ := f.progress(10010); s != 1 || f.held(f.aura) != 1 {
		t.Errorf("room made: step %d aura %d", s, f.held(f.aura))
	}
}
