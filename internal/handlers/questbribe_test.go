package handlers

import (
	"testing"

	"github.com/eqoa/server/internal/entity"
)

// bribeQuest is the rat were quest's Cogwhisker as Heirloom petition 2487
// asked for him: a talk step that asks pay or fight; paying takes the tunar
// and passes over the fight (step 2), fighting goes to it; step 3 is the
// return either way.
func bribeQuest(t *testing.T) (*questFixture, uint32) {
	t.Helper()
	f := newQuestFixture(t)
	cog := f.tw.add(3, &entity.NPC{ServerID: 703, Name: "Cogwhisker", SpawnX: 25404, SpawnY: 54.1, SpawnZ: 15726})
	f.exec(`INSERT INTO quests (questid, name, enabled, giver, minlevel, maxlevel) VALUES (10030, 'Debts', 1, 'Malsis', 1, 60)`)
	f.exec(`INSERT INTO queststeps (questid, step, kind, target, journal, choices) VALUES (10030, 1, 'talk', 'Cogwhisker', 'Settle it.', ?)`,
		`{"ask":"Pay or fight?","options":[{"answer":"Pay his debt","cost":500,"skip":1,"says":"Paid. Off with you."},{"answer":"Fight","says":"Then die!"}]}`)
	f.exec(`INSERT INTO queststeps (questid, step, kind, target, journal) VALUES (10030, 2, 'kill', 'Cogwhisker', 'Kill him.')`)
	f.exec(`INSERT INTO queststeps (questid, step, kind, target, journal) VALUES (10030, 3, 'talk', 'Malsis', 'Return.')`)
	f.qe.ReloadQuests()
	if msg, err := f.qe.SetCharacterQuest(f.sid, 10030, QuestGive, 0); err != nil {
		t.Fatalf("%s %v", msg, err)
	}
	f.msgs()
	return f, cog
}

// Paying with the tunar takes it and goes past the fight to the return; the
// control: without the tunar the player is told and the step stays, nothing
// taken.
func TestABribePassesOverTheFight(t *testing.T) {
	f, cog := bribeQuest(t)
	f.tw.p.Tunar = 499
	f.tw.talk(cog)
	f.tw.pick(boxID(f.msgs()), 0)
	f.want(f.msgs(), "box:Cogwhisker: You do not have the 500 tunar.")
	if s, _ := f.progress(10030); s != 1 || f.tw.p.Tunar != 499 {
		t.Fatalf("short of the bribe, yet step %d tunar %d", s, f.tw.p.Tunar)
	}
	f.tw.p.Tunar = 700
	f.tw.talk(cog)
	f.tw.pick(boxID(f.msgs()), 0)
	m := f.msgs()
	f.want(m, "box:Cogwhisker: Paid. Off with you.")
	f.want(m, "box:You have given away 500 tunar.")
	if s, _ := f.progress(10030); s != 3 || f.tw.p.Tunar != 200 {
		t.Errorf("paid: step %d tunar %d, want step 3 tunar 200", s, f.tw.p.Tunar)
	}
}

// Fighting takes nothing and goes to the fight.
func TestRefusingTheBribeGoesToTheFight(t *testing.T) {
	f, cog := bribeQuest(t)
	f.tw.p.Tunar = 700
	f.tw.talk(cog)
	f.tw.pick(boxID(f.msgs()), 1)
	f.want(f.msgs(), "box:Cogwhisker: Then die!")
	if s, _ := f.progress(10030); s != 2 || f.tw.p.Tunar != 700 {
		t.Errorf("fight: step %d tunar %d, want step 2 tunar 700", s, f.tw.p.Tunar)
	}
}
