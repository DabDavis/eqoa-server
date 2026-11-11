package handlers

import (
	"testing"
	"time"

	"github.com/eqoa/server/internal/entity"
)

// A survive step counts its seconds, keeps its kept spawns up (one whose
// corpse is gone comes again; a corpse lying holds its place), says when the
// time is up, and is done only once none of them is alive.
func TestASurviveStepKeepsItsSpawnsUpForItsTime(t *testing.T) {
	f := newQuestFixture(t)
	f.tw.p.HP, f.tw.p.MaxHP = 100, 100
	f.tw.add(5, &entity.NPC{ServerID: 707, Name: "thoughtbleeder", MaxHP: 100, SpawnX: 26000, SpawnY: 54.1, SpawnZ: 15000})
	f.exec(`INSERT INTO quests (questid, name, enabled, giver) VALUES (81, 'Endurance', 1, 'Malsis')`)
	f.exec(`INSERT INTO queststeps (questid, step, kind, count, journal, events) VALUES (81, 1, 'survive', 5, 'Survive.',
		'[{"on":"start","do":"spawn","npc":"thoughtbleeder","hostile":true,"count":2,"keep":true,"minutes":30}]')`)
	f.exec(`INSERT INTO queststeps (questid, step, kind, target, journal) VALUES (81, 2, 'talk', 'Malsis', 'Return to Malsis.')`)
	f.qe.Reload()
	live := func() (alive []*entity.NPC) {
		for _, n := range f.tw.w.NPCs {
			if n.QuestSpawn && !n.Dead {
				alive = append(alive, n)
			}
		}
		return alive
	}
	gone := func(n *entity.NPC) { // dead, its corpse gone
		n.Dead, n.DeathTime, n.CorpseGone = true, time.Now(), time.Now()
	}
	if _, err := f.qe.SetCharacterQuest(f.sid, 81, QuestGive, 0); err != nil {
		t.Fatal(err)
	}
	if a := live(); len(a) != 2 || a[0].CurrentEnemy != f.tw.p.ServerID {
		t.Fatalf("%d bleeders at the start, want 2 at the player", len(a))
	}
	gone(live()[0])
	f.qe.checkSurvive() // 1 s: one comes again
	if a := live(); len(a) != 2 {
		t.Fatalf("one gone, %d alive after a second, want 2", len(a))
	}
	// a corpse lying holds its place
	a := live()
	a[0].Dead, a[0].DeathTime = true, time.Now()
	f.qe.checkSurvive() // 2 s
	if n := len(live()); n != 1 {
		t.Fatalf("a corpse lying: %d alive, want 1", n)
	}
	for i := 0; i < 3; i++ {
		f.qe.checkSurvive() // 3..5 s: the time is up
	}
	if s, _ := f.progress(81); s != 1 {
		t.Fatalf("the time up with a bleeder alive, the step is %d", s)
	}
	for _, n := range live() {
		gone(n)
	}
	f.qe.checkSurvive()
	if s, _ := f.progress(81); s != 2 {
		t.Fatalf("the time up and every bleeder dead, the step is %d", s)
	}
	if n := len(live()); n != 0 {
		t.Fatalf("%d bleeders after the step", n)
	}
}
