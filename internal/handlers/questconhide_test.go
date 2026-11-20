package handlers

import (
	"testing"
	"time"

	"github.com/eqoa/server/internal/entity"
)

// A spawn's con (database.QuestEvent.Con) gives its copy the raiser's level
// and that level's hit points, so it cons the same to each (Heirloom petition
// 2517: Trissen "was a low level green when he should have spawned red to the
// character that talked to him"); a hide event takes the human a lycanthrope
// stands for out of view while it fights (petition 2516: "the original NPC is
// still standing there in human form"). The controls: a spawn with no con
// keeps its row's level, and an NPC of another name or far off stays in view.
func TestASpawnsConAndAHiddenHuman(t *testing.T) {
	f := newQuestFixture(t)
	f.tw.p.Level, f.tw.p.HP, f.tw.p.MaxHP = 60, 100, 100
	f.tw.add(5, &entity.NPC{ServerID: 706, Name: "Trissen", Level: 30, MaxHP: 100, HP: 100, SpawnX: 25000, SpawnY: 54.1, SpawnZ: 15000})
	f.tw.add(6, &entity.NPC{ServerID: 707, Name: "Lynda Elmquist", NPCType: 0x80, Level: 30, MaxHP: 100, HP: 100, SpawnX: 25405, SpawnY: 54.1, SpawnZ: 15727})
	f.tw.add(7, &entity.NPC{ServerID: 708, Name: "Lynda Elmquist", NPCType: 0x80, Level: 30, MaxHP: 100, HP: 100, SpawnX: 25600, SpawnY: 54.1, SpawnZ: 15727})
	f.tw.add(8, &entity.NPC{ServerID: 709, Name: "Lycanthrope Lynda", Level: 63, MaxHP: 100, HP: 100, SpawnX: 24000, SpawnY: 54.1, SpawnZ: 15000})
	f.tw.add(9, &entity.NPC{ServerID: 710, Name: "Bucktooth", Level: 30, MaxHP: 100, HP: 100, SpawnX: 24000, SpawnY: 54.1, SpawnZ: 15000})
	f.exec(`INSERT INTO quests (questid, name, enabled, giver) VALUES (81, 'Trissen', 1, 'Yindrius')`)
	f.exec(`INSERT INTO queststeps (questid, step, kind, target, journal, events) VALUES (81, 1, 'kill', 'Trissen', 'Kill Trissen.',
		'[{"on":"start","do":"spawn","npc":"Trissen","hostile":true,"con":"player+3"},{"on":"start","do":"spawn","npc":"Bucktooth","hostile":true},
		  {"on":"start","do":"hide","npc":"Lynda Elmquist","minutes":10},{"on":"start","do":"spawn","npc":"Lycanthrope Lynda","hostile":true}]')`)
	f.exec(`INSERT INTO queststeps (questid, step, kind, target, journal) VALUES (81, 2, 'talk', 'Yindrius', 'Back.')`)
	f.qe.Reload()
	if _, err := f.qe.SetCharacterQuest(f.sid, 81, QuestGive, 0); err != nil {
		t.Fatal(err)
	}
	copyOf := func(name string) *entity.NPC {
		for _, n := range f.tw.w.NPCs {
			if n.QuestSpawn && n.Name == name {
				return n
			}
		}
		t.Fatalf("no copy of %s", name)
		return nil
	}
	if c := copyOf("Trissen"); c.Level != 63 || c.MaxHP <= 100 || c.HP != c.MaxHP {
		t.Errorf("Trissen's copy: level %d (want the raiser's 60 + 3), HP %d of %d", c.Level, c.HP, c.MaxHP)
	}
	if c := copyOf("Bucktooth"); c.Level != 30 {
		t.Errorf("the control, no con: level %d, want its row's 30", c.Level)
	}
	if c := copyOf("Lycanthrope Lynda"); c.Level != 63 {
		t.Errorf("the lycanthrope's level %d", c.Level)
	}
	if h := f.tw.w.NPCs[707].HiddenUntil; time.Until(h) < 9*time.Minute {
		t.Errorf("Lynda beside the player hidden until %v, want 10 minutes on", h)
	}
	if h := f.tw.w.NPCs[708].HiddenUntil; !h.IsZero() {
		t.Errorf("the control: a Lynda 200 off hidden until %v", h)
	}
}
