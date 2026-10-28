package database

import (
	"strings"
	"testing"
)

// gmtriggers is made with the schema, empty; a row reads back as written and
// a slot with none says so.
func TestGMTriggersTable(t *testing.T) {
	db, err := NewSQLite(t.TempDir() + "/gmtriggers.db")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if all, err := db.GMTriggers(); err != nil || len(all) != 0 {
		t.Fatalf("a new table: %v %v", all, err)
	}
	if _, err := db.Exec(`INSERT INTO gmtriggers (slot, name, enabled, scope, radius, world, x, y, z, steps, notes)
		VALUES (3, 'Invasion', 0, ' Near ', 50, 2, 1.5, 2.5, 3.5, '[{"do":"wait","seconds":1}]', 'n')`); err != nil {
		t.Fatal(err)
	}
	got, ok, err := db.GMTriggerBySlot(3)
	want := GMTrigger{Slot: 3, Name: "Invasion", Scope: "near", Radius: 50, World: 2, X: 1.5, Y: 2.5, Z: 3.5,
		Steps: `[{"do":"wait","seconds":1}]`, Notes: "n"}
	if err != nil || !ok || got != want {
		t.Fatalf("slot 3: %+v %v %v\nwant %+v", got, ok, err, want)
	}
	if _, ok, err := db.GMTriggerBySlot(4); ok || err != nil {
		t.Fatalf("an empty slot: %v %v", ok, err)
	}
	if all, _ := db.GMTriggers(); len(all) != 1 || all[0] != want {
		t.Fatalf("every row: %+v", all)
	}
}

// What keeps a trigger from running is named, each by its step; the control:
// a trigger of every action, written right, has no problem.
func TestTriggerProblems(t *testing.T) {
	good := GMTrigger{Slot: 1, Scope: TriggerScopeNear, World: -1, Steps: `[
		{"do":"broadcast","text":"a"},{"do":"popup","text":"b"},{"do":"say","npc":"Guard","text":"c"},
		{"do":"give","items":"4866*2"},{"do":"visual","fx":"0x1DAEB5B6","seconds":2},
		{"do":"teleport","world":0,"x":25000,"y":50,"z":15000},{"do":"spawn","npc":"Orc","count":50,"world":2,"x":1,"z":1},
		{"do":"despawn"},{"do":"stop"},{"do":"wait","seconds":3600}]`}
	if p := TriggerProblems(good); len(p) != 0 {
		t.Fatalf("a good trigger: %q", p)
	}
	for _, c := range []struct {
		change func(*GMTrigger)
		want   string
	}{
		{func(g *GMTrigger) { g.Slot = 0 }, "slot 0"},
		{func(g *GMTrigger) { g.Scope = "everyone" }, `scope "everyone"`},
		{func(g *GMTrigger) { g.Radius = -1 }, "radius -1.0"},
		{func(g *GMTrigger) { g.World = 30 }, "world 30"},
		{func(g *GMTrigger) { g.Steps = `{"do":"wait"}` }, "not a JSON list"},
		{func(g *GMTrigger) { g.Steps = `` }, "no steps"},
		{func(g *GMTrigger) { g.Steps = `[{"do":"broadcast"}]` }, "step 1: broadcast says nothing"},
		{func(g *GMTrigger) { g.Steps = `[{"do":"give","items":"x"}]` }, "step 1: give names no items"},
		{func(g *GMTrigger) { g.Steps = `[{"do":"visual","fx":"red"}]` }, `step 1: visual's fx "red"`},
		{func(g *GMTrigger) { g.Steps = `[{"do":"teleport","x":1,"z":1}]` }, "step 1: teleport names no world"},
		{func(g *GMTrigger) { g.Steps = `[{"do":"teleport","world":0}]` }, "step 1: teleport's place is 0, 0"},
		{func(g *GMTrigger) { g.Steps = `[{"do":"spawn"}]` }, "step 1: spawn names no NPC"},
		{func(g *GMTrigger) { g.Steps = `[{"do":"spawn","npc":"Orc","count":51}]` }, "step 1: spawn's count 51"},
		{func(g *GMTrigger) { g.Steps = `[{"do":"spawn","npc":"Orc","minutes":241}]` }, "step 1: spawn's stay 241.0"},
		{func(g *GMTrigger) { g.Steps = `[{"do":"wait"}]` }, "step 1: wait 0.0 s"},
		{func(g *GMTrigger) { g.Steps = `[{"do":"wait","seconds":1},{"do":"Dance"}]` }, `step 2: do "dance"`},
	} {
		g := good
		c.change(&g)
		if p := TriggerProblems(g); len(p) == 0 || !strings.Contains(strings.Join(p, "; "), c.want) {
			t.Errorf("want %q: %q", c.want, p)
		}
	}
}

// The TEST menu's name: the trigger's, cut to 32 bytes on a character's edge;
// none, "Trigger n".
func TestTriggerMenuName(t *testing.T) {
	for _, c := range []struct {
		t    GMTrigger
		want string
	}{
		{GMTrigger{Slot: 2, Name: " Orc raid "}, "Orc raid"},
		{GMTrigger{Slot: 5}, "Trigger 5"},
		{GMTrigger{Slot: 1, Name: strings.Repeat("ö", 17)}, strings.Repeat("ö", 16)},
		{GMTrigger{Slot: 1, Name: strings.Repeat("a", 40)}, strings.Repeat("a", 32)},
	} {
		if got := c.t.MenuName(); got != c.want {
			t.Errorf("%q: %q, want %q", c.t.Name, got, c.want)
		}
	}
}
