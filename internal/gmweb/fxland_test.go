package gmweb

import (
	"strings"
	"testing"
	"time"
)

// A GM's fxland row is the world's at once (entity/fxland.go, work board #1643): an edit of land_ms is
// the next cast's, a new row is taken, a land_ms out of 0-10000 or a visual of 0 is refused and the world
// keeps what it had, and a deleted row is gone from it.
func TestFXLandEditsAreLive(t *testing.T) {
	f := newFixture(t)
	c := f.browser()
	c.login("gm")
	const blades = 0x07B3C0CB
	if _, ok := f.world.FXLandHit(blades); ok {
		t.Fatal("the control: the seed is the world's before the table is read")
	}
	code, out := c.op("update_row", map[string]any{"table": "fxland", "key": map[string]any{"fxdict": blades},
		"values": map[string]any{"land_ms": 1500}})
	if code != 200 {
		t.Fatalf("update: %d %v", code, out)
	}
	if d, ok := f.world.FXLandHit(blades); !ok || d != 1500*time.Millisecond {
		t.Errorf("after the update: %v %v, want 1.5s", d, ok)
	}
	if d, ok := f.world.FXLandHit(0xEA636DCD); !ok || d != 2*time.Second {
		t.Errorf("a seed row the edit read with it: %v %v", d, ok)
	}
	if live, _ := out["live"].(string); !strings.Contains(live, "at once") {
		t.Errorf("live: %v", out["live"])
	}
	for _, bad := range []map[string]any{{"land_ms": -1}, {"land_ms": 10001}, {"land_ms": "slow"}} {
		if code, out := c.op("update_row", map[string]any{"table": "fxland", "key": map[string]any{"fxdict": blades}, "values": bad}); code != 400 {
			t.Errorf("update %v: %d %v, want refused", bad, code, out)
		}
	}
	if code, out := c.op("insert_row", map[string]any{"table": "fxland", "values": map[string]any{"fxdict": 0, "land_ms": 100}}); code != 400 {
		t.Errorf("a visual of 0: %d %v, want refused", code, out)
	}
	if code, out := c.op("insert_row", map[string]any{"table": "fxland", "values": map[string]any{"fxdict": 0x0A0B0C0D, "land_ms": 2500, "note": "test"}}); code != 200 {
		t.Fatalf("insert: %d %v", code, out)
	}
	if d, ok := f.world.FXLandHit(0x0A0B0C0D); !ok || d != 2500*time.Millisecond {
		t.Errorf("after the insert: %v %v", d, ok)
	}
	if d, _ := f.world.FXLandHit(blades); d != 1500*time.Millisecond {
		t.Errorf("after the refusals: %v, want 1.5s", d)
	}
	if code, out := c.op("delete_row", map[string]any{"table": "fxland", "key": map[string]any{"fxdict": blades}}); code != 200 {
		t.Fatalf("delete: %d %v", code, out)
	}
	if _, ok := f.world.FXLandHit(blades); ok {
		t.Error("after the delete the row is still the world's")
	}
}
