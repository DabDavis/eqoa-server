package gmweb

import (
	"testing"
	"time"

	"github.com/eqoa/server/internal/weather"
)

// The weather's ops and tables: weather_set calls a GM's weather and is
// audited, weather_status lists it, weather_end ends it (audited); a bad row
// of the three tables is refused and a good one read into the engine at once.
func TestWeatherOpsAndTables(t *testing.T) {
	f := newFixture(t)
	f.world.Weather = weather.NewEngine()
	c := f.browser()
	c.login("gm")
	code, out := c.op("weather_set", map[string]any{"kind": "sand", "strength": "moderate", "minutes": 20, "world": 0,
		"x": 25000, "z": 15000, "where": "near", "near": 1, "text": "Sand!"})
	if code != 200 {
		t.Fatalf("weather_set: %d %v", code, out)
	}
	var n int
	f.db.QueryRow(`SELECT COUNT(*) FROM gm_audit WHERE action = 'live' AND detail LIKE 'weather sand moderate 20 min on 9 zones%'`).Scan(&n)
	if n != 1 {
		t.Fatalf("weather_set's audit rows: %d (%v)", n, out)
	}
	if code, out := c.op("weather_status", map[string]any{"world": 0, "x": 25000, "z": 15000}); code != 200 {
		t.Fatalf("weather_status: %d %v", code, out)
	} else if out["overrides"] == nil || out["square"] == nil {
		t.Fatalf("weather_status: %v", out)
	}
	ovs := f.world.Weather.Overrides(nowForTest())
	if len(ovs) != 1 || ovs[0].By != "gm" {
		t.Fatalf("overrides %+v", ovs)
	}
	if code, out := c.op("weather_end", map[string]any{"id": ovs[0].ID}); code != 200 {
		t.Fatalf("weather_end: %d %v", code, out)
	}
	f.db.QueryRow(`SELECT COUNT(*) FROM gm_audit WHERE action = 'live' AND detail LIKE 'weather #% ended'`).Scan(&n)
	if n != 1 {
		t.Fatalf("weather_end's audit rows: %d", n)
	}
	if code, _ := c.op("weather_end", map[string]any{"id": 999}); code != 404 {
		t.Fatalf("ending no such weather: %d", code)
	}
	// the tables: bad rows refused
	for _, bad := range []struct {
		table string
		row   map[string]any
	}{
		{"weatherprofiles", map[string]any{"profile": 300, "climate": "arctic"}},
		{"weatherprofiles", map[string]any{"profile": 3, "climate": "tundra"}},
		{"weatherzones", map[string]any{"world": 31, "col": 0, "row": 0, "climate": "none"}},
		{"weatherclimates", map[string]any{"climate": "temperate", "kind": "snow", "event_min": 6, "event_max": 12}},
		{"weatherclimates", map[string]any{"climate": "desert", "kind": "sand", "event_min": 3, "event_max": 30}},
		{"weatherclimates", map[string]any{"climate": "arctic", "kind": "snow", "event_min": 6, "event_max": 12, "fade_seconds": 0}},
	} {
		if code, out := c.op("insert_row", map[string]any{"table": bad.table, "values": bad.row}); code != 400 {
			t.Errorf("%s %v: %d %v, want 400", bad.table, bad.row, code, out)
		}
	}
	// good rows: read at once
	if code, out := c.op("insert_row", map[string]any{"table": "weatherprofiles", "values": map[string]any{"profile": 2, "climate": "arctic"}}); code != 200 {
		t.Fatalf("a good profile row: %d %v", code, out)
	}
	if code, out := c.op("insert_row", map[string]any{"table": "weatherclimates", "values": map[string]any{"climate": "arctic", "kind": "snow",
		"event_min": 6, "event_max": 12, "whiteout_chance": 0.5}}); code != 200 {
		t.Fatalf("a good climate row: %d %v", code, out)
	}
	if code, out := c.op("insert_row", map[string]any{"table": "weatherzones", "values": map[string]any{"world": 3, "col": 1, "row": 2}}); code != 200 {
		t.Fatalf("a good zone row: %d %v", code, out)
	}
	if _, pr, zn, cl := f.world.Weather.Counts(); pr != 1 || zn != 1 || cl != 1 {
		t.Fatalf("the engine after the edits: %d profiles, %d zones, %d climates", pr, zn, cl)
	}
}

func nowForTest() time.Time { return time.Now() }
