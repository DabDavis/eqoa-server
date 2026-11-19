package handlers

import (
	"testing"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
)

// Petition 2199 ("argot bittersteel is asking for the runed tablet. i
// already retrieved it but hes sending me back to get it again"): two
// tellings of one errand exclude each other (quests.excludes): one who has
// finished either, or has either under way, is not offered the other; the
// control, one who has done neither is.
func TestAnExcludedQuestIsNotOffered(t *testing.T) {
	f := newQuestFixture(t)
	f.exec(`UPDATE quests SET excludes = '10011' WHERE questid = 10010`)
	f.exec(`INSERT INTO quests (questid, name, enabled, giver, minlevel, maxlevel, offer, accept, decline, started, excludes)
		VALUES (10011, 'The same pincers', 1, 'Argot', 1, 10, 'Argot: Bring me pincers?', 'I will.', 'Not now.', 'Argot: Go.', '10010')`)
	f.exec(`INSERT INTO queststeps (questid, step, kind, target, journal) VALUES (10011, 1, 'talk', 'Kellina', 'Go speak to Kellina.')`)
	if err := f.qe.Reload(); err != nil {
		t.Fatal(err)
	}
	argot := f.tw.add(3, &entity.NPC{ServerID: 703, Name: "Argot", SpawnX: 25403.5, SpawnY: 54.1, SpawnZ: 15725.5})
	malsis := f.tw.add(1, &entity.NPC{ServerID: 701, Name: "Malsis", SpawnX: 25403.5, SpawnY: 54.1, SpawnZ: 15725.5})
	set := func(rows string) {
		t.Helper()
		f.exec(`DELETE FROM charquests WHERE serverid = ?`, f.sid)
		if rows != "" {
			f.exec(`INSERT INTO charquests (serverid, questid, step, done) VALUES `+rows, f.sid)
		}
		f.qe.ReloadCharacter(f.sid)
		f.msgs()
	}
	// neither done: each offered
	set("")
	f.tw.talk(argot)
	f.want(f.msgs(), "ask#*")
	f.tw.talk(malsis)
	f.want(f.msgs(), "ask#*")
	// 10010 finished: 10011 not offered
	set("(?, 10010, 0, 1)")
	f.tw.talk(argot)
	if m := f.msgs(); len(m) != 0 {
		t.Errorf("offered its twin after finishing the first: %q", m)
	}
	// 10010 under way: 10011 not offered
	set("(?, 10010, 1, 0)")
	f.tw.talk(argot)
	f.not(f.msgs(), "ask#*")
	// 10011 finished: 10010 not offered
	set("(?, 10011, 0, 1)")
	f.tw.talk(malsis)
	if m := f.msgs(); len(m) != 0 {
		t.Errorf("offered the first after finishing its twin: %q", m)
	}
	qs, err := database.NewQuestStore(f.db).LoadQuests()
	if err != nil {
		t.Fatal(err)
	}
	for _, q := range qs {
		if q.ID == 10010 && (len(q.Excludes) != 1 || q.Excludes[0] != 10011) {
			t.Errorf("10010's excludes read %v", q.Excludes)
		}
	}
}
