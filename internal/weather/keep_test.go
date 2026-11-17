package weather

import (
	"encoding/json"
	"testing"
	"time"
)

// A GM's weather goes over a restart that keeps the players in (keep.go) as it
// stood: its id, its zones and their phases at the same wall-clock times, so
// the time it had left runs on; the next called takes an id past it; an ended
// one is still ending; one long over is left out.
func TestKeepOverridesRoundTrip(t *testing.T) {
	t0 := time.Date(2026, 11, 16, 23, 44, 35, 0, time.UTC)
	e := testEngine()
	snow, err := e.Call(0, 600, 600, Where{Kind: "here"}, Snow, Heavy, 5, "Jibrish", 0, t0)
	if err != nil {
		t.Fatal(err)
	}
	sand, err := e.Call(0, 2600, 600, Where{Kind: "near", Near: 1}, Sand, Moderate, 30, "Dab", 9, t0)
	if err != nil {
		t.Fatal(err)
	}
	restart := t0.Add(85 * time.Second) // build321's keep restart came at 23:46
	if !e.EndID(sand.ID, restart.Add(-time.Minute)) { // faded out before the restart
		t.Fatal("EndID")
	}
	kept := e.Keep(restart)
	b, err := json.Marshal(kept) // through the snapshot file
	if err != nil {
		t.Fatal(err)
	}
	var back []KeptOverride
	if err := json.Unmarshal(b, &back); err != nil {
		t.Fatal(err)
	}

	after := restart.Add(5 * time.Second)
	f := testEngine()
	n, bad := f.Restore(back, after)
	if n != 2 || len(bad) != 0 {
		t.Fatalf("restored %d, bad %v; want 2", n, bad)
	}
	was, now := e.Overrides(after), f.Overrides(after)
	if len(now) != 1 || now[0].ID != snow.ID || !now[0].Until.Equal(snow.Until) || now[0].By != "Jibrish" || now[0].Kind != Snow {
		t.Fatalf("in force after: %+v, want #%d snow by Jibrish until %v (the ended sand faded out)", now, snow.ID, snow.Until)
	}
	if len(was) != 1 || was[0].ID != now[0].ID {
		t.Fatalf("old engine in force: %+v", was)
	}
	// the same weather everywhere it was called, at every moment of its course
	for _, z := range append(append([]Zone(nil), snow.Zones...), sand.Zones...) {
		for at := restart; at.Before(t0.Add(32 * time.Minute)); at = at.Add(7 * time.Second) {
			a, b := e.Zone(z, at), f.Zone(z, at)
			if len(a) != len(b) {
				t.Fatalf("%s at %v: %d timelines, %d after", z.ID(), at, len(a), len(b))
			}
			for i := range a {
				if a[i].Override != b[i].Override || a[i].Revision != b[i].Revision || a[i].Seed != b[i].Seed ||
					a[i].WindX != b[i].WindX || a[i].Byte(at) != b[i].Byte(at) || a[i].Weights(at) != b[i].Weights(at) {
					t.Fatalf("%s at %v: %+v, after %+v", z.ID(), at, a[i], b[i])
				}
			}
		}
	}
	// still ending (an end of it again is nothing), and the next id is past both
	if f.EndID(sand.ID, after) {
		t.Fatal("the ended sand ended again")
	}
	next, err := f.Call(0, 600, 600, Where{Kind: "here"}, Rain, Light, 3, "Dab", 0, after)
	if err != nil || next.ID <= sand.ID {
		t.Fatalf("next id %d (%v), want past %d", next.ID, err, sand.ID)
	}
	// a restore of what it holds already is refused, not doubled
	if n, bad := f.Restore(back, after); n != 0 || len(bad) != 2 {
		t.Fatalf("restored again: %d, bad %v", n, bad)
	}
}

// One long over (past the keep, the longest cycle and a minute) is left out,
// and its id is still never handed out again.
func TestKeepOverridesLongOver(t *testing.T) {
	t0 := time.Date(2026, 11, 16, 12, 0, 0, 0, time.UTC)
	e := testEngine()
	o, err := e.Call(0, 600, 600, Where{Kind: "here"}, Snow, Heavy, 2, "Dab", 0, t0)
	if err != nil {
		t.Fatal(err)
	}
	kept := e.Keep(t0.Add(time.Minute))
	f := testEngine()
	if n, bad := f.Restore(kept, t0.Add(3*time.Hour)); n != 0 || len(bad) != 0 {
		t.Fatalf("restored %d, bad %v; want none", n, bad)
	}
	next, _ := f.Call(0, 600, 600, Where{Kind: "here"}, Rain, Light, 3, "Dab", 0, t0.Add(3*time.Hour))
	if next.ID <= o.ID {
		t.Fatalf("id %d handed out again", next.ID)
	}
	// malformed: no zones, a bad kind
	bad := []KeptOverride{{ID: 50, Kind: Snow}, {ID: 51, Kind: 99, Zones: []KeptZoneOver{{Phases: []KeptPhase{{At: t0}}}}}}
	if n, errs := f.Restore(bad, t0); n != 0 || len(errs) != 2 {
		t.Fatalf("malformed: %d, %v", n, errs)
	}
}
