package handlers

import (
	"encoding/json"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/protocol"
)

// The Plane of Sky's scripted events as tools/sky_events_20261114.py writes them (its --dump),
// played on the triggers in world 4: Night 1's waves from a talk to the Voice, Night 3's race
// for the maiden won and lost, Desolation 1's ring from the patrollers to the Arbiter's three
// lives.

// skySet writes the tool's triggers and deaths into the test world, its prototypes (the rows
// a step copies by "#id", or by name) standing at the trigger's place in world 4.
func skySet(w *trigWorld) {
	w.t.Helper()
	py, err := exec.LookPath("python3")
	if err != nil {
		w.t.Skip("no python3 to dump the set")
	}
	out, err := exec.Command(py, "-I", filepath.Join("..", "..", "tools", "sky_events_20261114.py"), "--dump").Output()
	if err != nil {
		w.t.Fatal(err)
	}
	var f zpSetFile
	if err := json.Unmarshal(out, &f); err != nil {
		w.t.Fatal(err)
	}
	for _, t := range f.Triggers {
		w.setAt(t.Slot, t.Name, t.Scope, t.Radius, t.World, t.X, t.Y, t.Z, string(t.Steps), true)
		var steps []database.TriggerStep
		json.Unmarshal(t.Steps, &steps)
		for _, s := range steps {
			if s.Do != database.TriggerSpawn {
				continue
			}
			x, y, z := t.X, t.Y, t.Z
			if s.World != nil {
				x, y, z = s.X, s.Y, s.Z
			}
			id, ok := entity.ParseRowKey(s.NPC)
			if !ok {
				if w.gw.SpawnName(s.NPC, 4) != s.NPC || w.e.gw.CountNear(s.NPC, 4, x, z, 1e6) > 0 {
					continue
				}
				id = entity.EntityID(500000 + len(w.gw.NPCs))
			}
			if w.gw.NPCs[id] == nil {
				name := s.NPC
				if ok {
					name = skyRowName(s.NPC)
				}
				w.gw.NPCs[id] = &entity.NPC{ServerID: id, Name: name, HP: 100, MaxHP: 100, Level: 64, X: x, Y: y, Z: z, World: 4, SpawnOnly: true}
			}
		}
	}
	for _, d := range f.Deaths {
		if _, err := w.db.Exec(`INSERT INTO npcdeath (name, trigger, chance) VALUES (?, ?, ?)`, database.NPCClassKey(d.Name), d.Trigger, d.Chance); err != nil {
			w.t.Fatal(err)
		}
	}
	if err := w.e.Reload(); err != nil {
		w.t.Fatal(err)
	}
}

// skyRowName is the name of the row a key stands for, as the tool's rows are.
func skyRowName(key string) string {
	return map[string]string{"#119680": "ancient peasant", "#119684": "ancient vassal", "#119687": "ancient denizen",
		"#119690": "ancient squire", "#119691": "Ancient Jester", "#119727": "Night Maiden", "#119534": "Erych Arbiter",
		"#119543": "erych patroller", "#119529": "erych guardian", "#119530": "erych guardian", "#119531": "erych guardian",
		"#119532": "erych guardian", "#119533": "erych guardian"}[key]
}

// skyAlive is the living trigger copies of that name.
func (w *trigWorld) skyAlive(name string) []*entity.NPC {
	var out []*entity.NPC
	for _, n := range w.spawned() {
		if strings.EqualFold(n.Name, name) {
			out = append(out, n)
		}
	}
	return out
}

func (w *trigWorld) killAll(name string) {
	for _, n := range w.skyAlive(name) {
		w.kill(n)
		w.e.Killed(entity.KillCredit{NPCName: n.Name, NPCID: n.ServerID})
	}
}

func TestSkyNight1VoiceWaves(t *testing.T) {
	w := newTrigWorld(t)
	skySet(w)
	voice := &entity.NPC{ServerID: 600001, Name: "Voice of the Ancients", HP: 100, MaxHP: 100, NPCType: 0x80, X: 4164, Y: 612.38, Z: 6080, World: 4}
	w.gw.NPCs[voice.ServerID] = voice
	w.gp.World, w.gp.X, w.gp.Y, w.gp.Z = 4, 4164, 612.38, 6100
	if !w.e.Talk(w.gs, w.gp, voice) {
		t.Fatal("the Voice did not answer")
	}
	if n := len(w.skyAlive("ancient peasant")); n != 4 {
		t.Fatalf("%d peasants, want 4", n)
	}
	waves := []struct {
		name  string
		count int
		after time.Duration // after the last wave's deaths
	}{{"ancient vassal", 3, 300 * time.Second}, {"ancient denizen", 3, 180 * time.Second}, {"ancient squire", 1, 300 * time.Second}, {"Ancient Jester", 1, 0}}
	clock, last := time.Duration(0), "ancient peasant"
	for _, wave := range waves {
		w.killAll(last)
		clock += time.Second
		w.at(clock)
		if wave.after > 0 {
			w.at(clock + wave.after - 2*time.Second)
			if n := len(w.skyAlive(wave.name)); n != 0 {
				t.Fatalf("%s came before its time", wave.name)
			}
		}
		clock += wave.after + time.Second
		w.at(clock)
		if n := len(w.skyAlive(wave.name)); n != wave.count {
			t.Fatalf("%d of %s, want %d", n, wave.name, wave.count)
		}
		last = wave.name
	}
	if !w.e.Talk(w.gs, w.gp, voice) {
		t.Fatal("the busy Voice said nothing")
	}
	if got := strings.Join(questMsgs(w.gs), "|"); !strings.Contains(got, "already walk") {
		t.Fatalf("no busy words: %s", got)
	}
	w.killAll("Ancient Jester")
	w.at(clock + time.Second)
	w.at(clock + 302*time.Second)
	if len(w.e.Runs()) != 0 {
		t.Fatal("the run outlived its rest")
	}
}

func TestSkyNight3MaidenRace(t *testing.T) {
	w := newTrigWorld(t)
	skySet(w)
	for id, n := range map[entity.EntityID]string{119705: "Arch Bishop Millian", 119712: "Arch Bishop Lancel"} {
		w.gw.NPCs[id] = &entity.NPC{ServerID: id, Name: n, HP: 100, MaxHP: 100, Level: 64, X: 4316, Y: 632, Z: 6707, World: 4}
	}
	w.gp.World, w.gp.X, w.gp.Y, w.gp.Z = 4, 4122, 569.38, 6538 // the landing: 130 off the courtyard's mouth
	w.e.StartNear()
	if len(w.skyAlive("Night Maiden")) != 0 {
		t.Fatal("the landing started the race")
	}
	w.gp.X, w.gp.Z = 4250, 6585
	w.clear()
	w.e.StartNear()
	if len(w.skyAlive("Night Maiden")) != 1 || len(w.skyAlive("night duke")) != 2 {
		t.Fatalf("maiden %d, dukes %d", len(w.skyAlive("Night Maiden")), len(w.skyAlive("night duke")))
	}
	// lost: the time runs out with Lancel alive
	w.e.Killed(entity.KillCredit{NPCName: "Arch Bishop Millian", NPCID: 119705})
	w.at(301 * time.Second)
	if len(w.skyAlive("Night Maiden")) != 0 || len(w.e.Runs()) != 0 {
		t.Fatal("the lost race kept the maiden or its run")
	}
	if box := sentAll(w.gs, protocol.OpcodeQuestPopupMulti); len(box) != 2 {
		t.Fatalf("%d windows, want the start's and the failure's", len(box))
	}
	// won: both in time
	w.e.StartNear()
	w.e.Killed(entity.KillCredit{NPCName: "Arch Bishop Lancel", NPCID: 119712})
	w.e.Killed(entity.KillCredit{NPCName: "Arch Bishop Millian", NPCID: 119705})
	w.at(310 * time.Second)
	if len(w.skyAlive("Night Maiden")) != 1 || len(w.skyAlive("night duke")) != 0 {
		t.Fatal("won: the maiden stays, the dukes go")
	}
}

func TestSkyDeso1Ring(t *testing.T) {
	w := newTrigWorld(t)
	skySet(w)
	scout := &entity.NPC{ServerID: 600002, Name: "erych scout", HP: 100, MaxHP: 100, Level: 63, X: 4745, Y: 578, Z: 6258, World: 4}
	w.gw.NPCs[scout.ServerID] = scout
	w.gp.World, w.gp.X, w.gp.Y, w.gp.Z = 4, 4760, 569, 6260
	w.e.rand = func() float64 { return 0.01 } // every roll hits: a patroller from each death
	for i := 0; i < 5; i++ {
		w.e.Killed(entity.KillCredit{NPCName: "erych scout", NPCID: scout.ServerID})
	}
	if n := len(w.skyAlive("erych patroller")); n != 5 {
		t.Fatalf("%d patrollers, want its max 5", n)
	}
	w.killAll("erych patroller")
	w.at(time.Second)
	if n := len(w.skyAlive("erych guardian")); n != 5 {
		t.Fatalf("%d guardians after five patrollers", n)
	}
	w.killAll("erych guardian")
	w.at(2 * time.Second)
	clock := 2 * time.Second
	for life := 1; life <= 3; life++ {
		if n := len(w.skyAlive("Erych Arbiter")); n != 1 {
			t.Fatalf("life %d: %d Arbiters", life, n)
		}
		w.killAll("Erych Arbiter")
		clock += time.Second
		w.at(clock)
		clock += 11 * time.Second
		w.at(clock)
	}
	if n := len(w.skyAlive("Erych Arbiter")); n != 0 {
		t.Fatal("a fourth life")
	}
	if got := strings.Join(yellow(t, w.gs), "|"); !strings.Contains(got, "for the last time") {
		t.Fatalf("no last words: %s", got)
	}
}
