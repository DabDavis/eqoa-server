package handlers

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/session"
)

// Jarek's creation (the user, 2026-10-30, at Jarek N`Toxis's hut on Neriak's
// level 10 quest: "his creation is already spawned waiting, but should spawn
// after I complete the steps to bring him the crab and fungus cockroach parts.
// he then makes his creation and thats when you fight it"): the quests as
// tools/quests/data has them, and the engine making the creation from an
// original that stands out of the world (entity/spawnonly.go).

// contentQuest is a quest of tools/quests/data, what these tests read of it.
type contentQuest struct {
	Key   string `json:"key"`
	Name  string `json:"name"`
	Giver string `json:"giver"`
	Steps []struct {
		Kind   string `json:"kind"`
		Target string `json:"target"`
		Items  []struct {
			Item  string `json:"item"`
			Count int    `json:"count"`
		} `json:"items"`
		Count   int            `json:"count"`
		Chance  float64        `json:"chance"`
		Journal string         `json:"journal"`
		Says    string         `json:"says"`
		Events  []contentEvent `json:"events"`
	} `json:"steps"`
}

// contentEvent is an event as a content file writes it, what these tests read
// of it (a give's items are names there, ids in the database).
type contentEvent struct {
	On      string `json:"on"`
	Do      string `json:"do"`
	NPC     string `json:"npc,omitempty"`
	Hostile bool   `json:"hostile,omitempty"`
}

// contentQuests is a content file's quests by key.
func contentQuests(t *testing.T, file string) map[string]contentQuest {
	t.Helper()
	raw, err := os.ReadFile("../../tools/quests/data/" + file)
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Quests []contentQuest `json:"quests"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	out := map[string]contentQuest{}
	for _, q := range doc.Quests {
		out[q.Key] = q
	}
	return out
}

// jarekQuests are the creation's quests: Neriak's four of level 10 and the
// Klick`Anon rogue's.
var jarekQuests = []struct{ file, key string }{
	{"neriak-nektulos.json", "1-20/neriak-nektulos/magician/L10"},
	{"neriak-nektulos.json", "1-20/neriak-nektulos/necromancer/L10"},
	{"neriak-nektulos.json", "1-20/neriak-nektulos/shadowknight/L10"},
	{"neriak-nektulos.json", "1-20/neriak-nektulos/wizard/L10"},
	{"klick-anon.json", "1-20/klick-anon/rogue/L10"},
}

// Each of the creation's quests makes it when Jarek has the parts (step 4's
// done: a spawn of Jarek's Creation that attacks at once) and has it killed
// for its drop (step 5: a loot step of Jarek's Creation, every kill). Until
// 2026-10-30 Neriak's four spawned a fungus cockroach, not hostile, and step 5
// took the shell off any fungus cockroach, while the creation stood at the hut.
func TestJareksCreationQuestsMakeTheCreation(t *testing.T) {
	files := map[string]map[string]contentQuest{}
	for _, jq := range jarekQuests {
		if files[jq.file] == nil {
			files[jq.file] = contentQuests(t, jq.file)
		}
		q, ok := files[jq.file][jq.key]
		if !ok || len(q.Steps) < 5 {
			t.Errorf("%s: not in %s", jq.key, jq.file)
			continue
		}
		give, loot := q.Steps[3], q.Steps[4]
		if give.Kind != database.StepGive || give.Target != "Jarek N`Toxis" || len(give.Events) != 1 {
			t.Errorf("%s step 4: %s at %q, events %+v", jq.key, give.Kind, give.Target, give.Events)
			continue
		}
		if e := give.Events[0]; e.On != database.EventDone || e.Do != database.EventSpawn || e.NPC != "Jarek's Creation" || !e.Hostile {
			t.Errorf("%s step 4's event: %+v, want Jarek's Creation spawned hostile on its done", jq.key, e)
		}
		if loot.Kind != database.StepLoot || loot.Target != "Jarek's Creation" || loot.Chance != 1 || len(loot.Items) != 1 {
			t.Errorf("%s step 5: %s of %q at %v, items %+v", jq.key, loot.Kind, loot.Target, loot.Chance, loot.Items)
		}
	}
}

// jarekFixture is the fixture with the shadow knight's Jarek's Creation as the
// content file has it (quest 120), its items made by name; Jarek N`Toxis in
// view beside the player, and the creation's original out of the world 6 off
// (in the spatial index, where the world finds it). It returns the items by
// name and the object id Jarek is known by.
func jarekFixture(t *testing.T) (*questFixture, map[string]int, uint32, *entity.NPC) {
	t.Helper()
	f := newQuestFixture(t)
	f.tw.p.HP, f.tw.p.MaxHP = 100, 100 // alive, to be attacked
	q := contentQuests(t, "neriak-nektulos.json")["1-20/neriak-nektulos/shadowknight/L10"]
	items := map[string]int{}
	f.exec(`INSERT INTO quests (questid, name, enabled, giver) VALUES (120, ?, 1, ?)`, q.Name, q.Giver)
	for i, st := range q.Steps {
		var list []string
		for _, it := range st.Items {
			if items[it.Item] == 0 {
				items[it.Item] = f.pattern(it.Item, 20)
			}
			list = append(list, fmt.Sprintf("%d*%d", items[it.Item], max(it.Count, 1)))
		}
		evs := ""
		if len(st.Events) > 0 {
			b, err := json.Marshal(st.Events)
			if err != nil {
				t.Fatal(err)
			}
			evs = string(b)
		}
		f.exec(`INSERT INTO queststeps (questid, step, kind, target, items, count, chance, journal, says, events)
			VALUES (120, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, i+1, st.Kind, st.Target, strings.Join(list, ", "), max(st.Count, 1),
			st.Chance, st.Journal, st.Says, evs)
	}
	f.qe.Reload()
	jarek := f.tw.add(1, &entity.NPC{ServerID: 1735, Name: "Jarek N`Toxis", NPCType: 0x80, MaxHP: 100,
		SpawnX: 25404, SpawnY: 54.1, SpawnZ: 15726})
	creation := &entity.NPC{ServerID: 122241, Name: "Jarek's Creation", Level: 11, ModelID: 0x15764DA8, Size: 1,
		HP: 100, MaxHP: 100, SpawnX: 25409, SpawnY: 54.1, SpawnZ: 15722, X: 25409, Y: 54.1, Z: 15722}
	f.tw.w.NPCs[creation.ServerID] = creation
	f.tw.w.SpatialIndex.Insert(creation.ServerID, creation.X, creation.Z)
	f.tw.w.SetSpawnOnly(map[string]bool{database.NPCClassKey("Jarek's Creation"): true})
	return f, items, jarek, creation
}

// questCopies are the quests' spawns in the world.
func (f *questFixture) questCopies() []*entity.NPC {
	var out []*entity.NPC
	for _, n := range f.tw.w.NPCs {
		if n.QuestSpawn {
			out = append(out, n)
		}
	}
	return out
}

// liveCopies is how many of the quests' spawns are alive (a dead one stays in
// the world until the corpse system's tick, which no test runs, takes it).
func (f *questFixture) liveCopies() int {
	n := 0
	for _, c := range f.questCopies() {
		if !c.Dead {
			n++
		}
	}
	return n
}

// Step 4 done at Jarek (the parts handed over) makes Jarek's Creation before
// the player, in the world and attacking it, from an original that stands out
// of it; a kill of the creation drops the shell for the player and taking it
// does step 5. The controls: the original stays out of the world and fights no
// one, and a fungus cockroach (step 5's creature until 2026-10-30) drops
// nothing.
func TestJareksCreationIsMadeThenFoughtAndLooted(t *testing.T) {
	f, items, jarek, creation := jarekFixture(t)
	if _, err := f.qe.SetCharacterQuest(f.sid, 120, QuestSetStep, 4); err != nil {
		t.Fatal(err)
	}
	f.own(items["spikeshell crab claw"], 1)
	f.own(items["fungus cockroach leg"], 1)
	if len(f.questCopies()) != 0 {
		t.Fatal("a creation before Jarek has the parts")
	}
	f.msgs()
	f.tw.talk(jarek)
	f.want(f.msgs(), "box:Jarek N`Toxis: Now watch...")
	if s, _ := f.progress(120); s != 5 {
		t.Fatalf("the parts handed over, the quest is at step %d", s)
	}
	cs := f.questCopies()
	if len(cs) != 1 {
		t.Fatalf("%d creations made, want one", len(cs))
	}
	c := cs[0]
	if c.Name != "Jarek's Creation" || c.SpawnOnly || !c.Temporary || c.ModelID != creation.ModelID {
		t.Fatalf("the creation made: %q, spawn-only %v, temporary %v, body %#x", c.Name, c.SpawnOnly, c.Temporary, c.ModelID)
	}
	if c.NPCType&0x80 != 0 || c.CurrentEnemy != f.tw.p.ServerID || entity.DistanceBetween(f.tw.p, c) > 5 {
		t.Fatalf("the creation: type %#x, enemy %d, %.1f off; want attackable, at the player, before it",
			c.NPCType, c.CurrentEnemy, entity.DistanceBetween(f.tw.p, c))
	}
	if !creation.SpawnOnly || creation.CurrentEnemy != 0 {
		t.Errorf("the original: spawn-only %v, enemy %d", creation.SpawnOnly, creation.CurrentEnemy)
	}

	drops := []lootDrop{}
	f.qe.drop = func(npc entity.EntityID, _ time.Time, char uint32, rec database.CharInventoryEntry) {
		drops = append(drops, lootDrop{npc, char, rec.Pattern.PatternID})
	}
	f.qe.roll = func() float64 { return 0.99 } // the step's chance is 1: every kill
	f.killFor("fungus cockroach", 1331)
	if len(drops) != 0 {
		t.Fatalf("a fungus cockroach dropped %v", drops)
	}
	f.killFor("Jarek's Creation", c.ServerID)
	shell := items["mutated shell"]
	if len(drops) != 1 || drops[0] != (lootDrop{c.ServerID, f.sid, shell}) {
		t.Fatalf("the creation's drops: %v, want the shell on it", drops)
	}
	f.qe.Looted(f.tw.s, f.tw.p, shell, 1)
	if s, _ := f.progress(120); s != 6 {
		t.Fatalf("the shell taken, the quest is at step %d, want 6", s)
	}
}

// A player who lost the creation (gone with its player, its life up, its
// corpse gone unlooted) gets it again on coming within 30 of where its
// original stands: once, and not while one stands or lies dead. The controls:
// from 40 off nothing comes, and for a creature that stands in the world
// nothing is ever made again.
func TestALostSpawnOfASpawnOnlyCreatureComesBack(t *testing.T) {
	f, items, jarek, creation := jarekFixture(t)
	if _, err := f.qe.SetCharacterQuest(f.sid, 120, QuestSetStep, 4); err != nil {
		t.Fatal(err)
	}
	f.own(items["spikeshell crab claw"], 1)
	f.own(items["fungus cockroach leg"], 1)
	f.tw.talk(jarek)
	if len(f.questCopies()) != 1 {
		t.Fatalf("%d creations, want one", len(f.questCopies()))
	}
	f.qe.respawnLost()
	if len(f.questCopies()) != 1 {
		t.Fatalf("one standing, %d after a look", len(f.questCopies()))
	}

	away := func() {
		t.Helper()
		back := f.qe.sessionOf
		f.qe.sessionOf = func(uint32) *session.Session { return nil }
		f.qe.expireSpawns()
		f.qe.sessionOf = back
		if n := f.liveCopies(); n != 0 {
			t.Fatalf("the player away, %d creations stay", n)
		}
	}
	away()
	f.tw.p.X, f.tw.p.Z = creation.X+40, creation.Z
	f.qe.respawnLost()
	if n := len(f.questCopies()); n != 0 {
		t.Fatalf("40 off the original, %d creations came", n)
	}
	f.tw.p.X = creation.X + 20
	f.qe.respawnLost()
	cs := f.questCopies()
	if len(cs) != 1 || cs[0].CurrentEnemy != f.tw.p.ServerID || cs[0].SpawnOnly {
		t.Fatalf("20 off the original: %d creations (want one, at the player)", len(cs))
	}
	f.qe.respawnLost()
	if len(f.questCopies()) != 1 {
		t.Fatalf("a second creation while the first stands")
	}

	// killed, its corpse lying: none; the corpse gone unlooted: another
	c := cs[0]
	if !f.tw.w.KillNPCNow(c.ServerID) {
		t.Fatal("the creation could not be killed")
	}
	f.qe.respawnLost()
	if n := len(f.questCopies()); n != 1 {
		t.Fatalf("its corpse lying, %d creations (want the corpse alone)", n)
	}
	died, _ := f.tw.w.CorpseLies(c.ServerID)
	f.tw.w.ClearCorpse(c.ServerID, died)
	f.qe.respawnLost()
	if n := f.liveCopies(); n != 1 {
		t.Fatalf("the corpse gone unlooted, %d creations alive, want one", n)
	}

	// the control: a creature in the world is never made again
	f.tw.w.SetSpawnOnly(nil)
	away()
	f.qe.respawnLost()
	if n := f.liveCopies(); n != 0 {
		t.Errorf("a creature that stands in the world was made again: %d", n)
	}
}
