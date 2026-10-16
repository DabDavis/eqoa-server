package handlers

import (
	"path/filepath"
	"testing"

	"github.com/eqoa/server/internal/database"
)

// Race 10 is the first of the eleven race slots retail declares but never fills
// (gRaces[21] @ 0x004F3FE0, GetRace's bound is already `< 21`). Stage 1 makes it
// a Human East clone with the froglok's CHAR.ESF model. These tests pin the two
// things the server contributes: the model that goes on the wire, and the spawn.

func TestFrogloklModelIDIsTheCharESFSprite(t *testing.T) {
	h := &CharacterHandler{}
	// 0x141BFE0C is a CSpriteHeader (0x2710) in CHAR.ESF, the same object type
	// every playable race's model resolves to.
	const want = int64(0x141BFE0C)
	for _, sex := range []Gender{GenderMale, GenderFemale} {
		if got := h.getModelID(RaceFroglok, sex); got != want {
			t.Errorf("getModelID(RaceFroglok, %v) = 0x%X, want 0x%X", sex, got, want)
		}
	}
}

func TestFrogloklDoesNotDisturbExistingRaces(t *testing.T) {
	h := &CharacterHandler{}
	// A map miss falls back to the human male model, so a race whose entry got
	// clobbered would silently look human rather than fail. Check the two ends
	// of the table explicitly.
	cases := []struct {
		race Race
		sex  Gender
		want int64
	}{
		{RaceHuman, GenderMale, 0x70D898C6},
		{RaceHuman, GenderFemale, 0x0D537335},
		{RaceOgre, GenderMale, -2071956336},   // 0x84807490
		{RaceOgre, GenderFemale, -1396700337}, // 0xACC00B4F
	}
	for _, c := range cases {
		if got := h.getModelID(c.race, c.sex); got != c.want {
			t.Errorf("getModelID(%d, %v) = 0x%X, want 0x%X", c.race, c.sex, got, c.want)
		}
	}
}

// openDefaults loads the shipped database read-only. Skips rather than fails
// when it is absent, so the suite still runs on a bare checkout.
func openDefaults(t *testing.T) *database.DefaultCharacterStore {
	t.Helper()
	path, err := filepath.Abs("../../eqoa.db")
	if err != nil {
		t.Fatal(err)
	}
	db, err := database.NewSQLite(path)
	if err != nil {
		t.Skipf("no eqoa.db to read defaults from: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	s := database.NewDefaultCharacterStore(db)
	if s.Count() == 0 {
		t.Skip("defaultcharacter table is empty")
	}
	return s
}

func TestFrogloklSpawnsWhereHumanEastDoes(t *testing.T) {
	h := &CharacterHandler{defaults: openDefaults(t)}

	// Every class Human East actually has a row for. Class 9 (Cleric) is
	// deliberately absent from the list: mask 0x7E69 permits it but the table
	// has no row, which applyFrogloklSpawn logs and falls back on.
	for _, class := range []int{0, 3, 5, 6, 10, 11, 12, 13, 14} {
		want := h.defaults.GetDefaults(class, int(RaceHuman), humanTypeEast)
		if want == nil {
			t.Fatalf("Human East class %d has no row; the fixture assumption is stale", class)
		}
		if want.HumType != humanTypeEast {
			t.Fatalf("class %d resolved to humType %d, expected East", class, want.HumType)
		}
		rec := &database.CharacterRecord{
			CharName: "Ribbit", Race: int(RaceFroglok), TClass: class,
		}
		h.applyFrogloklSpawn(rec)
		if rec.X != want.X || rec.Y != want.Y || rec.Z != want.Z {
			t.Errorf("class %d: spawn (%.1f,%.1f,%.1f), want (%.1f,%.1f,%.1f)",
				class, rec.X, rec.Y, rec.Z, want.X, want.Y, want.Z)
		}
		if rec.World != want.World || rec.Facing != want.Facing {
			t.Errorf("class %d: world/facing %d/%.2f, want %d/%.2f",
				class, rec.World, rec.Facing, want.World, want.Facing)
		}
		if rec.X == 0 && rec.Y == 0 && rec.Z == 0 {
			t.Errorf("class %d: spawn left at the origin", class)
		}
	}
}

func TestApplyFrogloklSpawnLeavesOtherRacesAlone(t *testing.T) {
	h := &CharacterHandler{defaults: openDefaults(t)}
	// Scoping matters: defaultcharacter has never been applied to any race, and
	// stage 1 must not quietly start relocating the existing ten.
	for race := 0; race < 10; race++ {
		rec := &database.CharacterRecord{CharName: "Bob", Race: race, TClass: 0}
		h.applyFrogloklSpawn(rec)
		if rec.X != 0 || rec.Y != 0 || rec.Z != 0 || rec.World != 0 {
			t.Errorf("race %d was moved to (%.1f,%.1f,%.1f) world %d; only race 10 should be",
				race, rec.X, rec.Y, rec.Z, rec.World)
		}
	}
}

func TestApplyFrogloklSpawnSurvivesAMissingStore(t *testing.T) {
	h := &CharacterHandler{} // defaults nil, as in tests that build the handler bare
	rec := &database.CharacterRecord{CharName: "Ribbit", Race: int(RaceFroglok)}
	h.applyFrogloklSpawn(rec) // must not panic
	if rec.X != 0 {
		t.Errorf("spawn mutated without a defaults store")
	}
}

// TestRaceUseForWireWidensOnlyTheAllRacesIdiom pins the item race-mask widening.
//
// itempattern.raceuse is a bitmask indexed by race id, and the corpus makes the
// two halves of the rule concrete: 7329 of the 7874 patterns in the C# reference
// dump are 1023 (= "any race", must widen), while values like 991 and 271 omit
// specific races on purpose (must not).
func TestRaceUseForWireWidensOnlyTheAllRacesIdiom(t *testing.T) {
	const frogBit = 1 << 10
	cases := []struct {
		mask int
		want int
		why  string
	}{
		{1023, 1023 | frogBit, "all ten shipped races = any race"},
		{2047, 2047, "already carries the froglok bit"},
		{65535, 65535, "all bits already set"},
		{-1, -1, "all bits already set"},
		{0, 0, "no race bits — not the all-races idiom"},
		{1, 1, "Human only, a deliberate restriction"},
		{991, 991, "omits Troll (bit 5) on purpose"},
		{271, 271, "caster-only set"},
		{608, 608, "Troll/Barbarian/Halfling/Ogre only"},
		{1022, 1022, "omits Human — one bit short of the idiom"},
	}
	for _, c := range cases {
		if got := raceUseForWire(c.mask); got != c.want {
			t.Errorf("raceUseForWire(%d) = %d, want %d (%s)", c.mask, got, c.want, c.why)
		}
	}

	// The widening must never CLOSE a race that was already open: whatever the
	// mask, every bit set before is still set after.
	for mask := 0; mask < 2048; mask++ {
		got := raceUseForWire(mask)
		if got&mask != mask {
			t.Fatalf("raceUseForWire(%d) = %d cleared a race bit", mask, got)
		}
		// And the froglok bit ends up set exactly when the mask already had it
		// or names every shipped race — never for a genuine restriction.
		wantFrog := mask&frogBit != 0 || mask&allShippedRacesMask == allShippedRacesMask
		if (got&frogBit != 0) != wantFrog {
			t.Fatalf("raceUseForWire(%d) = %d: froglok bit set=%v, want %v",
				mask, got, got&frogBit != 0, wantFrog)
		}
	}
}
