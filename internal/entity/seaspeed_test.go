package entity

import (
	"testing"
	"time"

	"github.com/eqoa/server/internal/database"
)

// A Mermaid (petition 1650, the GM Jibrish: "it should detect if its in water and
// swim at SoW speed by default") in water moves at Spirit of the Wolf's speed, on
// land at its own, by its client's walk flags (in a liquid, or at its surface);
// with Spirit of the Wolf on it is SoW's speed either way, never past it; a
// dead one is at its own. The controls: a human in the same water, and a Jal'Raeth
// (a land body of racedefs), keep their run speed.
func TestAMermaidSwimsAtSpiritOfTheWolfsSpeed(t *testing.T) {
	withRaces(t,
		database.RaceDefRow{ID: 32, Name: "Mermaid", WireRace: 8, BodyMale: signed(bodyMerman), BodyFemale: signed(bodyMermaid)},
		database.RaceDefRow{ID: 30, Name: "Jal'Raeth", WireRace: 8, BodyMale: signed(bodyJalRaeth), BodyFemale: signed(bodyJalRaeth)})
	w := NewWorld()
	add := func(id EntityID, race int) *Player {
		p := &Player{ServerID: id, Name: "P", Race: race, HP: 10, MaxHP: 10, Speed: 10, X: 100, Z: 100}
		w.Players[id] = p
		w.SpatialIndex.Insert(id, p.X, p.Z)
		return p
	}
	mermaid, human, jal := add(1, 32), add(2, 0), add(3, 30)
	at := time.Now()
	step := func(p *Player, flags uint32) float32 {
		at = at.Add(100 * time.Millisecond)
		w.ApplyClientUpdate(p.ServerID, ClientUpdate{X: p.X, Y: p.Y, Z: p.Z, WalkFlags: flags, HasWalkFlags: true, At: at})
		return p.SpeedNow()
	}
	const sow = 10 * speedBuff
	const water, surface, land = 0x0004, 0x0008, 0x0001
	if s := step(mermaid, water); s != sow {
		t.Errorf("a mermaid under water: %v, want %v", s, sow)
	}
	if s := step(mermaid, surface); s != sow {
		t.Errorf("a mermaid at the surface: %v", s)
	}
	if s := step(mermaid, land); s != 10 {
		t.Errorf("a mermaid on land: %v", s)
	}
	mermaid.Effects = append(mermaid.Effects, &ActiveEffect{Speed: true})
	if a, b := step(mermaid, water), step(mermaid, land); a != sow || b != sow {
		t.Errorf("with Spirit of the Wolf: water %v, land %v; want %v both (no stacking)", a, b, sow)
	}
	mermaid.Effects = nil
	mermaid.HP = 0
	if s := step(mermaid, water); s != 10 {
		t.Errorf("a dead mermaid in water: %v", s)
	}
	for _, p := range []*Player{human, jal} {
		if s := step(p, water); s != 10 {
			t.Errorf("the control: race %d in water at %v", p.Race, s)
		}
	}
}

// The sea folk breathe water (the owner, 2026-11-12): a Mermaid holds her
// breath for good, so the breath system takes none and never drowns her, and
// her regen runs underwater; a Jal'Raeth does not.
func TestAMermaidBreathesUnderwater(t *testing.T) {
	withOptionRaces(t,
		database.RaceDefRow{ID: 32, Name: "Mermaid", WireRace: 8, BodyMale: signed(bodyMerman), BodyFemale: signed(bodyMermaid)},
		database.RaceDefRow{ID: 30, Name: "Jal'Raeth", WireRace: 8, BodyMale: signed(bodyJalRaeth), BodyFemale: signed(bodyJalRaeth)})
	if !(&Player{Race: 32}).holdsBreath() {
		t.Error("a mermaid holds her breath")
	}
	if (&Player{Race: 30}).holdsBreath() {
		t.Error("the control: a Jal'Raeth does not")
	}
}
