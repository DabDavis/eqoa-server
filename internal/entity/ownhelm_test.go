package entity

import (
	"testing"

	"github.com/eqoa/server/internal/database"
)

// A hill giant in plate is shown in the fire giant reaver's body, which has a
// great helm of its own: it is sent no worn helm (set and tint 0), the rest of
// its armour as worn and the helm still on the player (the owner, 2026-11-16,
// Fellstar: "the equipped plate helm is bigger encompassing both looks
// silly"). The controls: the hill giant's own body with a bare chest, the same
// giant under an illusion, and a human, all show the helm; a character that
// chose the reaver's body on the create page goes without it as well.
func TestABodyWithItsOwnHelmIsSentNoWornHelm(t *testing.T) {
	const hillGiant, reaver = 0xFFB1EA2C, 0x2D686972
	w := NewWorld()
	w.SetRaceArmorBodies([]database.RaceArmorBody{{Race: 11, Armor: "plate", Body: reaver}})
	g := geared(11)
	g.ServerID, g.ModelID, g.Helm, g.Chest = 1, signed(hillGiant), 4, 4
	w.Players[g.ServerID] = g

	helmed := func(p *Player) bool {
		b := SerializePlayerUpdate(7, p)
		return b[98] == 4 && b[104] != 0 && b[93] == 4
	}
	if !helmed(g) {
		t.Error("the control: a hill giant with a bare chest shows the helm")
	}
	w.SetGear(g.ServerID, Gear{Armor: ArmorPlate}, false)
	if body, _ := g.Body(); body != reaver {
		t.Fatalf("a hill giant in plate shows %#x, want the reaver's", body)
	}
	b := SerializePlayerUpdate(7, g)
	if b[98] != 0 || b[104] != 0 {
		t.Errorf("the reaver's body is sent helm set %d tint %d, want none", b[98], b[104])
	}
	if b[93] != 4 || b[99] == 0 {
		t.Errorf("its chest goes out as worn: set %d tint %d", b[93], b[99])
	}
	if g.Helm != 4 || g.HiddenParts != 0 {
		t.Errorf("the player's own helm is untouched: %d, hidden %#x", g.Helm, g.HiddenParts)
	}
	g.Effects = []*ActiveEffect{{SpellID: 9, Form: 0x70D898C6}}
	if !helmed(g) {
		t.Error("the control: under an illusion of another body the helm shows")
	}
	g.Effects = nil

	chose := geared(11)
	chose.ModelID, chose.Helm = signed(reaver), 4
	if b := SerializePlayerUpdate(7, chose); b[98] != 0 {
		t.Errorf("a giant that chose the reaver's body is sent helm set %d", b[98])
	}
	human := geared(0)
	human.Helm, human.Chest = 4, 4
	if !helmed(human) {
		t.Error("the control: a human shows its helm")
	}
}

// Every body listed as wearing its own helm is one a player may be shown in: a
// body step of some race's options, or an armour body (the reaver's is both).
func TestOwnHelmBodiesAreRaceBodies(t *testing.T) {
	offered := map[uint32]bool{}
	for _, rows := range raceOptionRows {
		for _, r := range rows {
			if r.Kind != KindBody {
				continue
			}
			for _, s := range append(append([]OptionStep{}, r.Male...), r.Female...) {
				offered[s.Value] = true
			}
		}
	}
	for body, why := range ownHelmBodies {
		if !offered[body] {
			t.Errorf("%#x (%s) is no race's body option", body, why)
		}
		if !BodyWearsHelm(body) {
			t.Errorf("%#x is listed", body)
		}
	}
	if BodyWearsHelm(0xFFB1EA2C) || BodyWearsHelm(0x70D898C6) {
		t.Error("the controls: the hill giant's own body and the human's wear none of their own")
	}
}
