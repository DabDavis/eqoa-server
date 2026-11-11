package handlers

import (
	"fmt"
	"testing"

	"github.com/eqoa/server/internal/entity"
)

// A show step's tunar is a fare (the portal guardian's first passage to
// Zaoran Plateau: questmasters1.com's "With the five Ruins and 5,000 tunar"):
// short of it, the step waits and nothing is taken; with it, the runes are
// kept and the tunar is paid, said among what was handed in.
func TestAShowStepPaysItsTunar(t *testing.T) {
	f := newQuestFixture(t)
	f.exec(`INSERT INTO quests (questid, name, enabled, giver) VALUES (31, 'The passage', 1, 'Guardian Test')`)
	f.exec(fmt.Sprintf(`INSERT INTO queststeps (questid, step, kind, target, items, tunar, journal, says)
		VALUES (31, 1, 'show', 'Guardian Test', '%d', 5000, 'Show the rune.', 'Pass, then.')`, f.aura))
	f.qe.Reload()
	g := f.tw.add(3, &entity.NPC{ServerID: 704, Name: "Guardian Test", SpawnX: 25403, SpawnY: 54.1, SpawnZ: 15726})
	f.tw.talk(g)
	f.tw.pick(boxID(f.msgs()), 0) // taken
	f.own(f.aura, 1)
	f.tw.p.Tunar = 4999
	f.tw.talk(g)
	f.msgs()
	if s, _ := f.progress(31); s != 1 || f.tw.p.Tunar != 4999 || f.held(f.aura) != 1 {
		t.Fatalf("short of the fare: step %d, tunar %d, rune %d", s, f.tw.p.Tunar, f.held(f.aura))
	}
	f.tw.p.Tunar = 5200
	f.tw.talk(g)
	m := f.msgs()
	if s, d := f.progress(31); s != 0 || d != 1 {
		t.Fatalf("with the fare: step %d done %d (%q)", s, d, m)
	}
	if f.tw.p.Tunar != 200 || f.held(f.aura) != 1 {
		t.Errorf("paid: tunar %d (want 200), rune %d kept (want 1)", f.tw.p.Tunar, f.held(f.aura))
	}
	f.want(m, "box:You have given away 5000 tunar.")
}
