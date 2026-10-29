package gmweb

import (
	"fmt"
	"strings"
	"testing"

	"github.com/eqoa/server/internal/entity"
)

// The NPC class templates in the panel (entity/classtemplate.go): the five
// seeded rows are there and no NPC fights by one; an npctemplate row a GM
// inserts gives the NPCs of the name the template at once (its hit points
// where they stand), a template's edit is theirs at once too, a row the world
// could not fight by is refused (the world keeping what it had), and the
// delete and its revert take the template away and give it back, audited.
func TestClassTemplatesInThePanel(t *testing.T) {
	f := newFixture(t)
	c := f.browser()
	c.login("gm")
	const skeleton = entity.EntityID(502) // 'a decaying skeleton', level 3: 180 hit points
	hp := func() (int, string) {
		st, ok := f.world.NPCStateOf(skeleton)
		if !ok {
			t.Fatal("the skeleton is not in the world")
		}
		return st.MaxHP, st.Template
	}
	if n, tm := hp(); n != entity.NPCMaxHP(3) || tm != "" {
		t.Fatalf("before: %d HP, template %q", n, tm)
	}
	for _, name := range []string{"tank", "melee", "wizard", "necromancer", "shadowknight"} {
		code, out := c.op("get_row", map[string]any{"table": "classtemplates", "key": map[string]any{"name": name}})
		if code != 200 {
			t.Errorf("the seed %q: %d %v", name, code, out)
		}
	}
	if code, _ := c.op("get_row", map[string]any{"table": "classtemplates", "key": map[string]any{"name": "paladin"}}); code == 200 {
		t.Errorf("the control: a template never seeded read")
	}
	if n, ok := catalog["classtemplates"]; !ok || n.Cols["abilities"].Format != "json" || n.Cols["class"].Enum != "templateclass" {
		t.Errorf("the catalog's classtemplates: %+v", n)
	}
	if n, ok := catalog["npctemplate"]; !ok || n.Cols["template"].Ref != "classtemplates" {
		t.Errorf("the catalog's npctemplate: %+v", n)
	}

	code, out := c.op("insert_row", map[string]any{"table": "npctemplate",
		"values": map[string]any{"name": "a decaying skeleton", "template": "tank"}})
	if code != 200 {
		t.Fatalf("insert: %d %v", code, out)
	}
	if !strings.Contains(fmt.Sprint(out["live"]), `fight by the "tank" template`) {
		t.Errorf("live: %v", out["live"])
	}
	if n, tm := hp(); n != entity.NPCMaxHP(3)*130/100 || tm != "tank" {
		t.Errorf("after the insert: %d HP, template %q; want the tank's 130%%", n, tm)
	}

	code, out = c.op("update_row", map[string]any{"table": "classtemplates", "key": map[string]any{"name": "tank"},
		"values": map[string]any{"hp_pct": 200}})
	if code != 200 {
		t.Fatalf("update: %d %v", code, out)
	}
	if n, _ := hp(); n != entity.NPCMaxHP(3)*2 {
		t.Errorf("after the template's edit: %d HP, want 200%%", n)
	}
	if !strings.Contains(fmt.Sprint(out["live"]), `1 NPCs fight by "tank"`) {
		t.Errorf("live: %v", out["live"])
	}

	for _, bad := range []map[string]any{
		{"hp_pct": 0},
		{"class": 20},
		{"special_chance": 50},
		{"abilities": `[{"kind": "fireball", "every": 10}]`},
		{"abilities": `[{"kind": "nuke", "evry": 10}]`},
		{"abilities": "not json"},
	} {
		if code, out := c.op("update_row", map[string]any{"table": "classtemplates", "key": map[string]any{"name": "tank"}, "values": bad}); code != 400 {
			t.Errorf("update %v: %d %v, want refused", bad, code, out)
		}
	}
	if n, _ := hp(); n != entity.NPCMaxHP(3)*2 {
		t.Errorf("after the refusals: %d HP, want the edit's", n)
	}
	if code, out := c.op("insert_row", map[string]any{"table": "npctemplate", "values": map[string]any{"name": "an orc"}}); code != 400 {
		t.Errorf("an assignment of no template: %d %v, want refused", code, out)
	}
	if code, out := c.op("insert_row", map[string]any{"table": "classtemplates", "values": map[string]any{"name": "caster",
		"class": 13, "abilities": `[{"kind": "nuke", "every": 6, "line": "bolt"}]`}}); code != 200 {
		t.Errorf("a GM's template: %d %v", code, out)
	}
	// the guide's recipe: a seed copied with changes
	if code, out := c.op("clone_row", map[string]any{"table": "classtemplates", "key": map[string]any{"name": "wizard"},
		"changes": map[string]any{"name": "goblin wizard", "hp_pct": 70, "source": "gm"}}); code != 200 {
		t.Errorf("a seed cloned: %d %v", code, out)
	}
	if code, out := c.op("get_row", map[string]any{"table": "classtemplates", "key": map[string]any{"name": "goblin wizard"}}); code != 200 ||
		!strings.Contains(fmt.Sprint(out), "chain") {
		t.Errorf("the clone: %d %v, want the wizard's abilities", code, out)
	}

	code, out = c.op("delete_row", map[string]any{"table": "npctemplate", "key": map[string]any{"name": "a decaying skeleton"}})
	if code != 200 {
		t.Fatalf("delete: %d %v", code, out)
	}
	if n, tm := hp(); n != entity.NPCMaxHP(3) || tm != "" {
		t.Errorf("after the delete: %d HP, template %q", n, tm)
	}
	delAudit := out["audit"].(float64)
	if code, out = c.op("revert_change", map[string]any{"id": delAudit}); code != 200 {
		t.Fatalf("revert the delete: %d %v", code, out)
	}
	if n, tm := hp(); n != entity.NPCMaxHP(3)*2 || tm != "tank" {
		t.Errorf("after the revert: %d HP, template %q", n, tm)
	}

	// a row written outside the panel (SQL) reaches the world with reload_npcs
	mustExec(t, f.db, `INSERT INTO npctemplate (name, template) VALUES ('guard dahvel', 'shadowknight')`)
	if st, _ := f.world.NPCStateOf(500); st.Template != "" {
		t.Fatalf("the control: the guard by %q before the reload", st.Template)
	}
	if code, out := c.op("reload_npcs", map[string]any{}); code != 200 || !strings.Contains(fmt.Sprint(out["message"]), "2 fight by a class template") {
		t.Errorf("reload_npcs: %d %v", code, out)
	}
	if st, _ := f.world.NPCStateOf(500); st.Template != "shadowknight" || st.MaxHP != entity.NPCMaxHP(30)*120/100 || st.Class != 3 {
		t.Errorf("the guard after the reload: %q, %d HP, class %d", st.Template, st.MaxHP, st.Class)
	}
}
