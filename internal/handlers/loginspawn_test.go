package handlers

import (
	"testing"

	"github.com/eqoa/server/internal/database"
)

// A login starts in Freeport beside Archivist Vaal when the row's position is
// in a world not every client has: a character stranded in a world whose ESF
// is not on this console's disk is exactly the case the override exists to
// make recoverable by reconnecting.
func TestLoginSpawnOverridesTheSavedPosition(t *testing.T) {
	rec := &database.CharacterRecord{
		CharName: "Stranded",
		X:        13000, Y: 400, Z: 23000, World: 9,
	}
	loginSpawn(nil, rec)

	if rec.World != 0 {
		t.Errorf("world = %d, want 0 (Freeport is in Tunaria)", rec.World)
	}
	// Vaal's row is the source of truth; with no world loaded this resolves to
	// the literal fallback in teleDests, plus the stand-off offset.
	wantX := teleDests[loginSpawnDest].X + loginSpawnOffset
	wantZ := teleDests[loginSpawnDest].Z + loginSpawnOffset
	if rec.X != wantX || rec.Z != wantZ {
		t.Errorf("landed at (%.1f, %.1f), want (%.1f, %.1f)",
			rec.X, rec.Z, wantX, wantZ)
	}
	if rec.Y != teleDests[loginSpawnDest].Y {
		t.Errorf("y = %.1f, want %.1f — the offset must not move the height, "+
			"which is how you get a spawn that falls",
			rec.Y, teleDests[loginSpawnDest].Y)
	}
}

// The player must not stand INSIDE the anchor NPC. A zero offset would put them
// at exactly Vaal's coordinates.
func TestLoginSpawnStandsOffTheAnchor(t *testing.T) {
	if loginSpawnOffset == 0 {
		t.Fatal("loginSpawnOffset is 0 — every player would spawn inside Vaal")
	}
	rec := &database.CharacterRecord{CharName: "Nudge"}
	loginSpawn(nil, rec)
	d := teleDests[loginSpawnDest]
	if rec.X == d.X && rec.Z == d.Z {
		t.Error("spawn is exactly on the anchor NPC's own point")
	}
}

// The destination the spawn names has to exist. Renaming it out from under
// loginSpawn would otherwise put every login at the origin — 25,000 units from
// Freeport, under no terrain at all — so the fallback is "keep the saved
// position" and this test is what keeps that path from being the normal one.
func TestLoginSpawnDestExists(t *testing.T) {
	d, ok := teleDests[loginSpawnDest]
	if !ok {
		t.Fatalf("no %q in teleDests", loginSpawnDest)
	}
	if d.NPC == "" {
		t.Error("the spawn destination has no anchor NPC, so moving the NPC " +
			"would no longer move the spawn")
	}
	if d.World != 0 {
		t.Errorf("anchor is in world %d; Freeport is world 0", d.World)
	}
}

// An unknown destination leaves the record ALONE rather than zeroing it.
func TestLoginSpawnKeepsThePositionWhenTheDestIsGone(t *testing.T) {
	restore := withoutLoginSpawnDest(t)
	defer restore()

	rec := &database.CharacterRecord{
		CharName: "Untouched",
		X:        111, Y: 222, Z: 333, World: 9,
	}
	loginSpawn(nil, rec)
	if rec.X != 111 || rec.Y != 222 || rec.Z != 333 || rec.World != 9 {
		t.Errorf("record was modified: (%.0f, %.0f, %.0f) world=%d",
			rec.X, rec.Y, rec.Z, rec.World)
	}
}

// A nil record is what a lookup failure upstream hands us; it must not panic.
func TestLoginSpawnToleratesANilRecord(t *testing.T) {
	loginSpawn(nil, nil)
}

// withoutLoginSpawnDest removes the spawn destination for one test. It reaches
// into the map because loginSpawnDest is a const — the value is not meant to be
// configurable at run time — so the map is the only reachable half of the
// not-found path.
func withoutLoginSpawnDest(t *testing.T) func() {
	t.Helper()
	orig, had := teleDests[loginSpawnDest]
	delete(teleDests, loginSpawnDest)
	return func() {
		if had {
			teleDests[loginSpawnDest] = orig
		}
	}
}

// A login resumes where the character logged out when that is a place every
// client can stand (F3 20261020-195202): a disc world, above the floor.
func TestLoginSpawnResumesASavedPositionInADiscWorld(t *testing.T) {
	for _, w := range []int{0, 3, discWorlds - 1} {
		rec := &database.CharacterRecord{CharName: "Back", X: 17091.6, Y: 141.1, Z: 15220.2, World: w}
		loginSpawn(nil, rec)
		if rec.X != 17091.6 || rec.Y != 141.1 || rec.Z != 15220.2 || rec.World != w {
			t.Errorf("world %d: moved to (%.1f, %.1f, %.1f) world=%d", w, rec.X, rec.Y, rec.Z, rec.World)
		}
	}
}

// What does not resume starts beside Vaal: a character that never logged out
// (the origin), a world past the disc's six, a fall to the world floor.
func TestLoginSpawnStartsAtVaalWhenTheSavedPositionCannotResume(t *testing.T) {
	vaal := teleDests[loginSpawnDest]
	for _, rec := range []*database.CharacterRecord{
		{CharName: "New"},
		{CharName: "Hdd", X: 9000, Y: 60, Z: 9000, World: 22},
		{CharName: "S2", X: 1000, Y: 60, Z: 1000, World: discWorlds},
		{CharName: "Fell", X: 25000, Y: -1000, Z: 15000, World: 0},
	} {
		loginSpawn(nil, rec)
		if rec.World != 0 || rec.X != vaal.X+loginSpawnOffset || rec.Z != vaal.Z+loginSpawnOffset {
			t.Errorf("%s: at (%.1f, %.1f) world=%d, want beside Vaal", rec.CharName, rec.X, rec.Z, rec.World)
		}
	}
}
