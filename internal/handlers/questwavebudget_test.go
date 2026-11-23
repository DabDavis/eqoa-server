package handlers

import (
	"reflect"
	"testing"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
)

// Board entry 1290: a hunt wave and a lost-spawn recovery make no more of the step's target creatures than the
// kills still wanted, counted in creatures (an event's Count), not in events; an event of a creature the step does
// not count keeps its own Count and comes only with a wave that brings a target.
func TestWaveCountsCreatureBudget(t *testing.T) {
	tgt := func(n int) database.QuestEvent {
		return database.QuestEvent{Do: database.EventSpawn, NPC: "Wave Target", Count: n}
	}
	tgt2 := func(n int) database.QuestEvent {
		return database.QuestEvent{Do: database.EventSpawn, NPC: "wave other", Count: n}
	}
	pup := func(n int) database.QuestEvent {
		return database.QuestEvent{Do: database.EventSpawn, NPC: "Wave Pup", Count: n}
	}
	names := []string{"Wave Target", "Wave Other"}
	for _, c := range []struct {
		name string
		evs  []database.QuestEvent
		left int
		want []int
	}{
		{"left 1, count 3", []database.QuestEvent{tgt(3)}, 1, []int{1}},
		{"left 3, counts 2+3", []database.QuestEvent{tgt(2), tgt2(3)}, 3, []int{2, 1}},
		{"left 2, counts 2+3: the second none", []database.QuestEvent{tgt(2), tgt2(3)}, 2, []int{2, 0}},
		{"count unset is one", []database.QuestEvent{tgt(0), tgt2(0)}, 5, []int{1, 1}},
		{"companion with a wave", []database.QuestEvent{pup(2), tgt(3)}, 1, []int{2, 1}},
		{"nothing left: no wave, no companion", []database.QuestEvent{pup(2), tgt(3)}, 0, []int{0, 0}},
		{"plenty left: as written", []database.QuestEvent{tgt(3), pup(1)}, 10, []int{3, 1}},
	} {
		if got := waveCounts(c.evs, names, c.left); !reflect.DeepEqual(got, c.want) {
			t.Errorf("%s: %v, want %v", c.name, got, c.want)
		}
	}
}

// waveFixture: quest 141, step 1 a kill of four "Wave Target"s whose start brings three targets and a "Wave Pup"
// (a companion, not counted); the target's original stands out of the world beside the player (so a lost wave
// comes again there: respawnLost).
func waveFixture(t *testing.T) *questFixture {
	t.Helper()
	f := newQuestFixture(t)
	f.tw.p.HP, f.tw.p.MaxHP = 100, 100
	f.exec(`INSERT INTO quests (questid, name, enabled, giver) VALUES (141, 'Wave Budget', 1, 'Nobody')`)
	f.exec(`INSERT INTO queststeps (questid, step, kind, target, count, journal, events) VALUES (141, 1, 'kill', 'Wave Target', 4,
		'Kill four.', '[{"on":"start","do":"spawn","npc":"Wave Target","count":3,"hostile":true},{"on":"start","do":"spawn","npc":"Wave Pup","hostile":true}]')`)
	f.qe.Reload()
	for i, n := range []string{"Wave Target", "Wave Pup"} {
		o := &entity.NPC{ServerID: entity.EntityID(141001 + i), Name: n, Level: 10, Size: 1, HP: 100, MaxHP: 100,
			SpawnX: f.tw.p.X + 5, SpawnY: f.tw.p.Y, SpawnZ: f.tw.p.Z, X: f.tw.p.X + 5, Y: f.tw.p.Y, Z: f.tw.p.Z}
		f.tw.w.NPCs[o.ServerID] = o
		f.tw.w.SpatialIndex.Insert(o.ServerID, o.X, o.Z)
	}
	f.tw.w.SetSpawnOnly(map[string]bool{database.NPCClassKey("Wave Target"): true, database.NPCClassKey("Wave Pup"): true})
	return f
}

// liveNamed counts the quest's spawns of a name alive.
func (f *questFixture) liveNamed(name string) []*entity.NPC {
	var out []*entity.NPC
	for _, c := range f.questCopies() {
		if !c.Dead && c.Name == name {
			out = append(out, c)
		}
	}
	return out
}

func TestNextWaveHeldToTheKillsLeft(t *testing.T) {
	f := waveFixture(t)
	if _, err := f.qe.SetCharacterQuest(f.sid, 141, QuestSetStep, 1); err != nil {
		t.Fatal(err)
	}
	if a, b := len(f.liveNamed("Wave Target")), len(f.liveNamed("Wave Pup")); a != 3 || b != 1 {
		t.Fatalf("the step's start: %d targets, %d pups; want 3 and 1", a, b)
	}
	for _, c := range f.liveNamed("Wave Pup") {
		f.tw.w.KillNPCNow(c.ServerID) // not a kill the step counts
	}
	for _, c := range f.liveNamed("Wave Target") {
		f.tw.w.KillNPCNow(c.ServerID)
		f.killFor("Wave Target", c.ServerID)
	}
	if s, _ := f.progress(141); s != 1 {
		t.Fatalf("three of four: the quest is at step %d", s)
	}
	// one kill left: the next wave brings ONE target (it brought three before), and its pup
	if a, b := len(f.liveNamed("Wave Target")), len(f.liveNamed("Wave Pup")); a != 1 || b != 1 {
		t.Fatalf("the next wave with one kill left: %d targets, %d pups; want 1 and 1", a, b)
	}
	last := f.liveNamed("Wave Target")[0]
	f.tw.w.KillNPCNow(last.ServerID)
	f.killFor("Wave Target", last.ServerID)
	if s, d := f.progress(141); s == 1 && d == 0 {
		t.Fatalf("four killed, the step not done (%d, %d)", s, d)
	}
	if n := len(f.liveNamed("Wave Target")); n != 0 {
		t.Fatalf("the step done, %d more targets came", n)
	}
}

// A partly done hunt whose spawns were lost (the player away) gets back only what it still wants; with none
// wanted, none.
func TestLostWaveHeldToTheKillsLeft(t *testing.T) {
	f := waveFixture(t)
	if _, err := f.qe.SetCharacterQuest(f.sid, 141, QuestSetStep, 1); err != nil {
		t.Fatal(err)
	}
	ts := f.liveNamed("Wave Target")
	for _, c := range ts[:2] {
		f.tw.w.KillNPCNow(c.ServerID)
		f.killFor("Wave Target", c.ServerID)
	}
	f.tw.s.SetCharacterInWorld(false)
	f.qe.expireSpawns()
	f.tw.s.SetCharacterInWorld(true)
	for _, c := range f.questCopies() {
		if !c.Dead {
			f.tw.w.KillNPCNow(c.ServerID)
		}
	}
	f.qe.mu.Lock()
	f.qe.spawns = nil // corpses gone
	f.qe.mu.Unlock()
	f.qe.respawnLost()
	if a, b := len(f.liveNamed("Wave Target")), len(f.liveNamed("Wave Pup")); a != 2 || b != 1 {
		t.Fatalf("two of four killed, lost: %d targets, %d pups came again; want 2 and 1", a, b)
	}
	f.qe.respawnLost()
	if a := len(f.liveNamed("Wave Target")); a != 2 {
		t.Fatalf("a second recovery while the first stands: %d targets", a)
	}
}
