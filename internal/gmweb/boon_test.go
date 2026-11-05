package gmweb

import (
	"os"
	"strings"
	"testing"
	"time"

	"github.com/eqoa/server/internal/config"
	"github.com/eqoa/server/internal/entity"
)

// The towns' boon's settings (world.boon_*) are an admin's, live and kept: a
// GM may not change them; the admin's change is the world's at once, shown by
// get_settings and written to config.yaml, which loads them again; a number
// out of its range is refused and changes nothing. The control: before the
// change the world has the defaults, double for four hours.
func TestBoonSettingsAreLive(t *testing.T) {
	f := newFixture(t)
	if got := f.world.Boon(); got != entity.DefaultBoon {
		t.Fatalf("the control: the world starts at %+v", got)
	}
	gm := f.browser()
	gm.login("gm")
	_, out := gm.op("get_settings", nil)
	if live, _ := out["live"].(map[string]any); live["boon_xp_pct"] != float64(200) || live["boon_minutes"] != float64(240) {
		t.Errorf("get_settings before: %v", live)
	}
	if code, _ := gm.op("update_settings", map[string]any{"boon_xp_pct": 300}); code != 403 {
		t.Fatalf("a GM changed the boon: %d", code)
	}
	c := f.browser()
	c.login("boss")
	code, out := c.op("update_settings", map[string]any{"boon_xp_pct": 300, "boon_minutes": 90})
	if code != 200 {
		t.Fatalf("update: %d %v", code, out)
	}
	want := entity.Boon{Percent: 300, Duration: 90 * time.Minute}
	if got := f.world.Boon(); got != want {
		t.Fatalf("the world's boon: %+v, want %+v", got, want)
	}
	_, out = c.op("get_settings", nil)
	if live, _ := out["live"].(map[string]any); live["boon_xp_pct"] != float64(300) || live["boon_minutes"] != float64(90) {
		t.Errorf("get_settings: %v", live)
	}
	b, _ := os.ReadFile(f.cfg)
	for _, w := range []string{"boon_xp_pct: 300", "boon_duration: 1h30m0s", "# the test server"} {
		if !strings.Contains(string(b), w) {
			t.Errorf("config.yaml lacks %q:\n%s", w, b)
		}
	}
	reloaded, err := config.Load(f.cfg)
	if err != nil || reloaded.World.BoonXPPct == nil || *reloaded.World.BoonXPPct != 300 || reloaded.World.BoonDuration != 90*time.Minute {
		t.Fatalf("reload: %v %+v", err, reloaded.World)
	}
	for _, bad := range []map[string]any{{"boon_xp_pct": 99}, {"boon_xp_pct": 1001}, {"boon_minutes": 0}, {"boon_minutes": 10081}} {
		if code, out := c.op("update_settings", bad); code != 400 {
			t.Errorf("%v: %d %v, want refused", bad, code, out)
		}
	}
	if got := f.world.Boon(); got != want {
		t.Errorf("after the refusals: %+v", got)
	}
}
