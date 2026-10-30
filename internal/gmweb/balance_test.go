package gmweb

import (
	"strings"
	"testing"

	"github.com/eqoa/server/internal/entity"
)

// A GM's balance rows are the world's at once: an NPC's hit points become its
// level's under the new row (keeping their share), the con row the next
// hit's, and a row out of bounds is refused.
func TestBalanceEditsAreLive(t *testing.T) {
	f := newFixture(t)
	f.exec(`DELETE FROM balancelevel`) // the seed rows out: the formula's hit points, then one row
	f.exec(`DELETE FROM balancecon`)
	c := f.browser()
	c.login("gm")
	st, ok := f.world.NPCStateOf(500)
	if !ok {
		t.Fatal("the fixture's NPC 500 is not in the world")
	}
	base := entity.NPCMaxHP(st.Level)

	code, out := c.op("insert_row", map[string]any{"table": "balancelevel",
		"values": map[string]any{"level": 1, "hp_pct": 300, "damage_pct": 150}})
	if code != 200 {
		t.Fatalf("insert: %d %v", code, out)
	}
	if st, _ := f.world.NPCStateOf(500); st.MaxHP != 3*base || st.HP != 3*base {
		t.Errorf("after a 300%% row: %d/%d HP, want %d", st.HP, st.MaxHP, 3*base)
	}
	if !strings.Contains(out["live"].(string), "hit points") {
		t.Errorf("live: %v", out["live"])
	}

	code, out = c.op("update_row", map[string]any{"table": "balancelevel",
		"key": map[string]any{"level": 1}, "values": map[string]any{"hp_pct": 0}})
	if code == 200 {
		t.Errorf("a 0%% row was taken: %v", out)
	}
	code, out = c.op("delete_row", map[string]any{"table": "balancelevel", "key": map[string]any{"level": 1}})
	if code != 200 {
		t.Fatalf("delete: %d %v", code, out)
	}
	if st, _ := f.world.NPCStateOf(500); st.MaxHP != base {
		t.Errorf("after the row went: %d HP, want the formula's %d", st.MaxHP, base)
	}
	code, out = c.op("insert_row", map[string]any{"table": "balancecon",
		"values": map[string]any{"diff": 3, "damage_pct": 160, "taken_pct": 70}})
	if code != 200 {
		t.Fatalf("insert con: %d %v", code, out)
	}
	if !strings.Contains(out["live"].(string), "next hit") {
		t.Errorf("live: %v", out["live"])
	}
	code, out = c.op("insert_row", map[string]any{"table": "balancecon",
		"values": map[string]any{"diff": 40, "damage_pct": 160, "taken_pct": 70}})
	if code == 200 {
		t.Errorf("a diff of 40 was taken: %v", out)
	}
}

// The arts' cooldown is a time: refused below 0 or past a minute, taken
// within.
func TestBalanceArtCooldownBounds(t *testing.T) {
	f := newFixture(t)
	c := f.browser()
	c.login("gm")
	if code, out := c.op("insert_row", map[string]any{"table": "balancelevel",
		"values": map[string]any{"level": 77, "art_cooldown_ms": -5}}); code == 200 {
		t.Errorf("a cooldown of -5 ms was taken: %v", out)
	}
	if code, out := c.op("insert_row", map[string]any{"table": "balancelevel",
		"values": map[string]any{"level": 77, "art_cooldown_ms": 70000}}); code == 200 {
		t.Errorf("a cooldown of 70 s was taken: %v", out)
	}
	if code, out := c.op("insert_row", map[string]any{"table": "balancelevel",
		"values": map[string]any{"level": 77, "art_cooldown_ms": 2000}}); code != 200 {
		t.Errorf("a cooldown of 2 s was refused: %d %v", code, out)
	}
}

// Balance by class is the GM's like the others: a row changed is taken and
// said to be live, a class past 14 and a share of 0 are refused.
func TestBalanceClassEdits(t *testing.T) {
	f := newFixture(t)
	c := f.browser()
	c.login("gm")
	code, out := c.op("update_row", map[string]any{"table": "balanceclass",
		"key": map[string]any{"class": 6}, "values": map[string]any{"melee_taken_pct": 50}})
	if code != 200 {
		t.Fatalf("update: %d %v", code, out)
	}
	if !strings.Contains(out["live"].(string), "next hit") {
		t.Errorf("live: %v", out["live"])
	}
	if code, out := c.op("insert_row", map[string]any{"table": "balanceclass",
		"values": map[string]any{"class": 20, "melee_taken_pct": 80}}); code == 200 {
		t.Errorf("class 20 was taken: %v", out)
	}
	if code, out := c.op("update_row", map[string]any{"table": "balanceclass",
		"key": map[string]any{"class": 6}, "values": map[string]any{"melee_taken_pct": 0}}); code == 200 {
		t.Errorf("a share of 0 was taken: %v", out)
	}
}

// A root's chances are percents of 0-100.
func TestBalanceRootChanceBounds(t *testing.T) {
	f := newFixture(t)
	c := f.browser()
	c.login("gm")
	for _, bad := range []map[string]any{{"root_resist_pct": 120}, {"root_break_pct": -1}} {
		if code, out := c.op("update_row", map[string]any{"table": "balancecon", "key": map[string]any{"diff": 3},
			"values": bad}); code == 200 {
			t.Errorf("%v was taken: %v", bad, out)
		}
	}
	if code, out := c.op("update_row", map[string]any{"table": "balancecon", "key": map[string]any{"diff": 3},
		"values": map[string]any{"root_resist_pct": 40, "root_break_pct": 30}}); code != 200 {
		t.Errorf("40/30 was refused: %d %v", code, out)
	}
}

// A creature's heal out of a fight is a percent of 0-100.
func TestBalanceRegenBounds(t *testing.T) {
	f := newFixture(t)
	c := f.browser()
	c.login("gm")
	if code, out := c.op("update_row", map[string]any{"table": "balancelevel", "key": map[string]any{"level": 30},
		"values": map[string]any{"regen_pct": 150}}); code == 200 {
		t.Errorf("a regen of 150%% was taken: %v", out)
	}
	if code, out := c.op("update_row", map[string]any{"table": "balancelevel", "key": map[string]any{"level": 30},
		"values": map[string]any{"regen_pct": 8}}); code != 200 {
		t.Errorf("a regen of 8%% was refused: %d %v", code, out)
	}
}

// Balance by class and level is the GM's like the others: a row is taken
// and live, a class past 14 or a level past 100 refused.
func TestBalanceClassLevelEdits(t *testing.T) {
	f := newFixture(t)
	c := f.browser()
	c.login("gm")
	code, out := c.op("insert_row", map[string]any{"table": "balanceclasslevel",
		"values": map[string]any{"class": 3, "level": 45, "melee_taken_pct": 125}})
	if code != 200 {
		t.Fatalf("insert: %d %v", code, out)
	}
	if !strings.Contains(out["live"].(string), "next hit") {
		t.Errorf("live: %v", out["live"])
	}
	for _, bad := range []map[string]any{{"class": 20, "level": 10, "melee_taken_pct": 90}, {"class": 3, "level": 140, "melee_taken_pct": 90},
		{"class": 3, "level": 20, "melee_taken_pct": 0}} {
		if code, out := c.op("insert_row", map[string]any{"table": "balanceclasslevel", "values": bad}); code == 200 {
			t.Errorf("%v was taken: %v", bad, out)
		}
	}
}

// The spectre's instant, in its own melee hits, is 0-20: refused below 0,
// past 20 and when not a number, taken within and said to be live at its
// next instant.
func TestBalancePetSpecialHitsBounds(t *testing.T) {
	f := newFixture(t)
	c := f.browser()
	c.login("gm")
	for _, bad := range []any{-1, 25, "three"} {
		if code, out := c.op("update_row", map[string]any{"table": "balancelevel", "key": map[string]any{"level": 30},
			"values": map[string]any{"pet_special_hits": bad}}); code == 200 {
			t.Errorf("%v hits was taken: %v", bad, out)
		}
	}
	code, out := c.op("update_row", map[string]any{"table": "balancelevel", "key": map[string]any{"level": 30},
		"values": map[string]any{"pet_special_hits": 5}})
	if code != 200 {
		t.Fatalf("5 hits was refused: %d %v", code, out)
	}
	if live, _ := out["live"].(string); !strings.Contains(live, "instant") {
		t.Errorf("live: %v", out["live"])
	}
}
