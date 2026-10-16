package entity

import (
	"testing"

	"github.com/eqoa/server/internal/database"
)

// A character is saved standing ON the ground, and a height even slightly low
// puts the camera UNDER the terrain, where the player falls instead of standing.
// Observed in world 22, whose zone is placed at an offset: characters stored at
// y=58.3 and 57.8 where the ground is 59.3.
func TestSpawnArrivesAboveTheStoredHeight(t *testing.T) {
	w := NewWorld()
	rec := &database.CharacterRecord{
		CharName: "Faller", Level: 1, X: 8996.8, Y: 58.3, Z: 9002.3, World: 22,
	}
	p := w.CreatePlayer(nil, rec)
	if p == nil {
		t.Fatal("no player")
	}
	if p.Y <= rec.Y {
		t.Errorf("spawned at y=%.1f, at or below the stored %.1f — that is the "+
			"under-the-terrain case", p.Y, rec.Y)
	}
	// The ground it has to clear is about a unit up; the drop must stay small
	// enough to be invisible.
	if got := p.Y - rec.Y; got != SpawnLift {
		t.Errorf("lifted by %.1f, want %.1f", got, SpawnLift)
	}
}

// A character at the world floor is in the void, not a rounding error from the
// ground. Lifting them three units pretends their row is fine when it is not.
func TestSpawnDoesNotLiftAFallenCharacter(t *testing.T) {
	w := NewWorld()
	rec := &database.CharacterRecord{
		CharName: "Stranded", Level: 1, X: 8436.8, Y: WorldFloorY, Z: 8289.1, World: 22,
	}
	p := w.CreatePlayer(nil, rec)
	if p == nil {
		t.Fatal("no player")
	}
	if p.Y != WorldFloorY {
		t.Errorf("a character at the world floor spawned at y=%.1f; the floor "+
			"is not a height to build on", p.Y)
	}
}

// The ratchet: spawn a unit low -> fall to the floor -> the floor is saved ->
// every later login starts in the void. Refusing to write the fall is what
// breaks it, and it is the half that would have kept two live characters
// playable.
func TestAFallIsNotSaved(t *testing.T) {
	stood := float32(59.3)
	if got, ok := SafeSaveHeight(WorldFloorY, stood); ok || got != stood {
		t.Errorf("saving from the world floor gave (%.1f, %v), want (%.1f, false)",
			got, ok, stood)
	}
	// Below the floor counts too — nothing legitimate is under it.
	if _, ok := SafeSaveHeight(WorldFloorY-500, stood); ok {
		t.Error("a height below the world floor was accepted")
	}
	// And an ordinary height is still written, or logging out anywhere would
	// rewind the character to wherever they last stood.
	if got, ok := SafeSaveHeight(72.5, stood); !ok || got != 72.5 {
		t.Errorf("an ordinary height gave (%.1f, %v), want (72.5, true)", got, ok)
	}
}
