package database

import "testing"

// npcpaths: the schema makes the table, All reads the good rows and names the
// bad ones beside them, and the points column round-trips.
func TestNPCPathStore(t *testing.T) {
	db, err := NewSQLite(t.TempDir() + "/paths.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	pause := 3.5
	pts := PathPointsJSON([]PathPoint{{X: 1, Y: 2, Z: 3}, {X: 4, Y: 5, Z: 6, Pause: &pause}})
	for _, r := range []struct {
		id           int
		mode, points string
	}{
		{7, "pingpong", pts},
		{8, "loop", "not json"},
		{9, "zigzag", pts},
		{10, "once", "[]"},
	} {
		if _, err := db.Exec(`INSERT INTO npcpaths (npcid, world, mode, pause, speed, points) VALUES (?, 1, ?, 2, 'run', ?)`, r.id, r.mode, r.points); err != nil {
			t.Fatal(err)
		}
	}
	got, bad, err := NewNPCPathStore(db).All()
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || len(bad) != 3 {
		t.Fatalf("%d paths and %d bad, want 1 and 3: %v", len(got), len(bad), bad)
	}
	r := got[0]
	if r.NPCID != 7 || r.World != 1 || r.Mode != "pingpong" || r.Pause != 2 || r.Speed != "run" ||
		len(r.Points) != 2 || r.Points[0].Pause != nil || r.Points[1].Pause == nil || *r.Points[1].Pause != 3.5 || r.Points[1].Z != 6 {
		t.Fatalf("read %+v", r)
	}
	if again := PathPointsJSON(r.Points); again != pts {
		t.Errorf("points written back as %s, were %s", again, pts)
	}
}

func TestPathModeAndSpeedNames(t *testing.T) {
	for in, want := range map[string]string{"": "loop", "Loop": "loop", "back and forth": "pingpong", "patrol": "pingpong", "once then hold": "once"} {
		if got, ok := PathMode(in); !ok || got != want {
			t.Errorf("PathMode(%q) = %q, %v; want %q", in, got, ok, want)
		}
	}
	if _, ok := PathMode("zigzag"); ok {
		t.Error("zigzag is a mode")
	}
	if got, ok := PathSpeed("RUN"); !ok || got != "run" {
		t.Errorf("PathSpeed(RUN) = %q, %v", got, ok)
	}
	if _, ok := PathSpeed("fly"); ok {
		t.Error("fly is a speed")
	}
}
