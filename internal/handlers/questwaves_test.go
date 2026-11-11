package handlers

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/session"
)

// pardonFixture is the fixture with the good melees' Pages of Pardon as
// tools/quests/data/60.json has it (quest 130), its items made by name; the
// high shaman in view beside the player, and the bloodletters' original out of
// the world 380 off, as on eqoaclassic (petition 1539). It returns the items
// by name and the object id the shaman is known by.
func pardonFixture(t *testing.T) (*questFixture, map[string]int, uint32) {
	t.Helper()
	f := newQuestFixture(t)
	f.tw.p.HP, f.tw.p.MaxHP = 100, 100
	q := contentQuests(t, "60.json")["60/good-melee/ep1-2"]
	if len(q.Steps) != 7 {
		t.Fatalf("Pages of Pardon: %d steps", len(q.Steps))
	}
	items := map[string]int{}
	f.exec(`INSERT INTO quests (questid, name, enabled, giver) VALUES (130, ?, 1, ?)`, q.Name, q.Giver)
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
			VALUES (130, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, i+1, st.Kind, st.Target, strings.Join(list, ", "), max(st.Count, 1),
			st.Chance, st.Journal, st.Says, evs)
	}
	f.qe.Reload()
	f.tw.p.X, f.tw.p.Y, f.tw.p.Z = 24749.8, 10, 29335.8
	shaman := f.tw.add(1, &entity.NPC{ServerID: 122675, Name: "high shaman", NPCType: 0x80, MaxHP: 100,
		SpawnX: 24747.1, SpawnY: 10, SpawnZ: 29333.2})
	orig := &entity.NPC{ServerID: 122676, Name: "Froglok Bloodletter", Level: 58, ModelID: 0x141BFE0C, Size: 1,
		HP: 100, MaxHP: 100, SpawnX: 24673.6, SpawnY: 10, SpawnZ: 28952, X: 24673.6, Y: 10, Z: 28952}
	f.tw.w.NPCs[orig.ServerID] = orig
	f.tw.w.SpatialIndex.Insert(orig.ServerID, orig.X, orig.Z)
	f.tw.w.SetSpawnOnly(map[string]bool{database.NPCClassKey("Froglok Bloodletter"): true})
	return f, items, shaman
}

// liveBloodletters is the bloodletters spawned for the step, alive.
func (f *questFixture) liveBloodletters() []*entity.NPC {
	var out []*entity.NPC
	for _, c := range f.questCopies() {
		if !c.Dead && c.Name == "Froglok Bloodletter" {
			out = append(out, c)
		}
	}
	return out
}

// Petition 1539 (Froglegs: "gave High Shaman 3 pages then 2 bloodletters spawn,
// killed them and no more spawn"): the step wants six, and its spawns come in
// waves of two, the next when the last of a wave dies, until six are killed;
// the sixth's kill does the step and no seventh comes. A kill of one of a wave
// with the other alive brings none.
func TestPagesOfPardonsBloodlettersComeInWaves(t *testing.T) {
	f, items, shaman := pardonFixture(t)
	if _, err := f.qe.SetCharacterQuest(f.sid, 130, QuestSetStep, 4); err != nil {
		t.Fatal(err)
	}
	for _, n := range []string{"first page of pardon", "second page of pardon", "third page of pardon"} {
		f.own(items[n], 1)
	}
	f.tw.talk(shaman)
	if s, _ := f.progress(130); s != 5 {
		t.Fatalf("the pages handed over, the quest is at step %d", s)
	}
	for kills := 0; kills < 6; {
		wave := f.liveBloodletters()
		if len(wave) != 2 {
			t.Fatalf("after %d kills, %d bloodletters alive, want a wave of two", kills, len(wave))
		}
		for i, b := range wave {
			if !f.tw.w.KillNPCNow(b.ServerID) {
				t.Fatal("a bloodletter could not be killed")
			}
			f.killFor("Froglok Bloodletter", b.ServerID)
			kills++
			if i == 0 && len(f.liveBloodletters()) != 1 {
				t.Fatalf("one of a wave killed, %d alive, want the other alone", len(f.liveBloodletters()))
			}
		}
	}
	if s, _ := f.progress(130); s != 6 {
		t.Fatalf("six killed, the quest is at step %d, want 6", s)
	}
	if n := len(f.liveBloodletters()); n != 0 {
		t.Fatalf("the step done, %d bloodletters came", n)
	}
}

// A player who lost the step's bloodletters (Froglegs after a restart: two
// killed, none standing, the high shaman beside her) gets a wave again by the
// high shaman, where the step brought them, not only by their original 380
// off. The control: 40 off the shaman, none.
func TestLostBloodlettersComeAgainByTheHighShaman(t *testing.T) {
	f, _, _ := pardonFixture(t)
	if _, err := f.qe.SetCharacterQuest(f.sid, 130, QuestSetStep, 5); err != nil {
		t.Fatal(err)
	}
	if n := len(f.liveBloodletters()); n != 2 {
		t.Fatalf("set to step 5, %d bloodletters, want its wave", n)
	}
	back := f.qe.sessionOf // the player away (a restart): the spawns go
	f.qe.sessionOf = func(uint32) *session.Session { return nil }
	f.qe.expireSpawns()
	f.qe.sessionOf = back
	if n := len(f.liveBloodletters()); n != 0 {
		t.Fatalf("the player away, %d bloodletters stay", n)
	}
	f.tw.p.X += 40
	f.qe.respawnLost()
	if n := len(f.liveBloodletters()); n != 0 {
		t.Fatalf("40 off the shaman, %d came", n)
	}
	f.tw.p.X -= 40
	f.qe.respawnLost()
	if n := len(f.liveBloodletters()); n != 2 {
		t.Fatalf("by the shaman, %d came, want a wave of two", n)
	}
}
