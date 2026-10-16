package handlers

import (
	"testing"

	"github.com/eqoa/server/internal/database"
)

// Every login starts in Freeport beside Archivist Vaal, whatever the row says.
//
// The saved position is the thing this overrides, so the test hands it one that
// is obviously somewhere else: a character stranded in a world whose ESF is not
// on this console's disk is exactly the case the override exists to make
// recoverable by reconnecting.
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
		X:        111, Y: 222, Z: 333, World: 2,
	}
	loginSpawn(nil, rec)
	if rec.X != 111 || rec.Y != 222 || rec.Z != 333 || rec.World != 2 {
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
