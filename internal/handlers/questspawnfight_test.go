package handlers

import (
	"testing"
	"time"

	"github.com/eqoa/server/internal/entity"
)

// A timed spawn past its time is never taken mid-fight (Live ticket 5308: a
// quest's raid-sized spawn went at its 30 minutes with a fifth of its health
// left): while it fights it stays, once out of the fight it stays
// spawnFightGrace more, then it goes. The control: a spawn past its time out
// of any fight goes at once.
func TestATimedSpawnWaitsOutItsFight(t *testing.T) {
	f := newQuestFixture(t)
	f.tw.p.HP, f.tw.p.MaxHP = 100, 100
	f.tw.add(5, &entity.NPC{ServerID: 706, Name: "Delvin", NPCType: 0x80, MaxHP: 100, SpawnX: 26000, SpawnY: 54.1, SpawnZ: 15000})
	f.exec(`INSERT INTO quests (questid, name, enabled, giver) VALUES (81, 'Delvin', 1, 'Malsis')`)
	f.exec(`INSERT INTO queststeps (questid, step, kind, target, journal, events) VALUES (81, 1, 'kill', 'Delvin', 'Kill Delvin.',
		'[{"on":"start","do":"spawn","npc":"Delvin","hostile":true,"minutes":30}]')`)
	f.exec(`INSERT INTO queststeps (questid, step, kind, target, journal) VALUES (81, 2, 'talk', 'Malsis', 'Again.')`)
	f.qe.Reload()
	if _, err := f.qe.SetCharacterQuest(f.sid, 81, QuestGive, 0); err != nil {
		t.Fatal(err)
	}
	spawn := func() *entity.NPC {
		for _, n := range f.tw.w.NPCs {
			if n.QuestSpawn && !n.Dead {
				return n
			}
		}
		return nil
	}
	n := spawn()
	if n == nil || n.CurrentEnemy == 0 {
		t.Fatalf("no hostile spawn fighting: %v", n)
	}
	// its time up: put its end in the past
	setUntil := func(d time.Duration) {
		f.qe.mu.Lock()
		for id := range f.qe.spawns {
			for i := range f.qe.spawns[id] {
				f.qe.spawns[id][i].until = time.Now().Add(d)
			}
		}
		f.qe.mu.Unlock()
	}
	setUntil(-time.Minute)
	f.qe.expireSpawns()
	if spawn() == nil {
		t.Fatal("a spawn past its time was taken mid-fight")
	}
	// the fight over: it stays the grace
	n.CurrentEnemy, n.Threats = 0, nil
	f.qe.expireSpawns()
	if spawn() == nil {
		t.Fatal("a spawn went the moment its fight ended, before the grace")
	}
	// the grace over: it goes
	f.qe.mu.Lock()
	for id := range f.qe.spawns {
		for i := range f.qe.spawns[id] {
			f.qe.spawns[id][i].fought = time.Now().Add(-spawnFightGrace - time.Second)
		}
	}
	f.qe.mu.Unlock()
	f.qe.expireSpawns()
	if spawn() != nil {
		t.Fatal("a spawn past its time stayed after its fight and the grace")
	}
}

// The control: a spawn past its time that never fought goes at once.
func TestATimedSpawnOutOfAFightGoes(t *testing.T) {
	f := newQuestFixture(t)
	f.tw.add(5, &entity.NPC{ServerID: 706, Name: "Delvin", NPCType: 0x80, MaxHP: 100, SpawnX: 26000, SpawnY: 54.1, SpawnZ: 15000})
	f.exec(`INSERT INTO quests (questid, name, enabled, giver) VALUES (82, 'Delvin', 1, 'Malsis')`)
	f.exec(`INSERT INTO queststeps (questid, step, kind, target, journal, events) VALUES (82, 1, 'talk', 'Malsis', 'Talk.',
		'[{"on":"start","do":"spawn","npc":"Delvin","minutes":30}]')`)
	f.qe.Reload()
	if _, err := f.qe.SetCharacterQuest(f.sid, 82, QuestGive, 0); err != nil {
		t.Fatal(err)
	}
	f.qe.mu.Lock()
	n := 0
	for id := range f.qe.spawns {
		for i := range f.qe.spawns[id] {
			f.qe.spawns[id][i].until = time.Now().Add(-time.Second)
			n++
		}
	}
	f.qe.mu.Unlock()
	if n != 1 {
		t.Fatalf("%d spawns, want one", n)
	}
	f.qe.expireSpawns()
	for _, c := range f.tw.w.NPCs {
		if c.QuestSpawn && !c.Dead {
			t.Fatal("a spawn past its time out of any fight stayed")
		}
	}
}
