package handlers

import (
	"testing"
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

// The froglok's spawn (a human of the east's) is database.CreateNew's now,
// with every other race's: database/newchar_test.go.

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
