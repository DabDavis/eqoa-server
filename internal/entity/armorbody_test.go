package entity

import (
	"testing"

	"github.com/eqoa/server/internal/database"
)

// A hill giant shows another giant's body for the armour on its chest (the user, 2026-11-08: "lets
// add armor display to player hill giants", then sand giants for cloth and fire giants for plate):
// cloth the sand giant's, chain the fire giant's, plate the fire giant reaver's; leather, which has no
// row, and a bare chest its own; an illusion over all of it as ever. The control: another race in
// plate keeps its own body.
func TestAHillGiantWearsAGiantsArmouredBody(t *testing.T) {
	w := NewWorld()
	giant := &Player{ServerID: 1, Name: "Lagerbottoms", Race: 11, ModelID: 0xFFB1EA2C, Size: 1.8}
	human := &Player{ServerID: 2, Name: "Plain", Race: 0, ModelID: 0x70D898C6, Size: 1}
	w.Players[giant.ServerID], w.Players[human.ServerID] = giant, human
	if n := w.SetRaceArmorBodies([]database.RaceArmorBody{{Race: 11, Armor: "cloth", Body: 0xE617303B},
		{Race: 11, Armor: "chain", Body: 0x72A8FDBB}, {Race: 11, Armor: "Plate", Body: 0x2D686972}, {Race: 11, Armor: "mithril", Body: 1}}); n != 3 {
		t.Fatalf("%d bodies read, want 3 (a kind it does not know left out)", n)
	}
	for _, c := range []struct {
		armor ArmorKind
		want  uint32
	}{{ArmorNone, 0xFFB1EA2C}, {ArmorCloth, 0xE617303B}, {ArmorLeather, 0xFFB1EA2C}, {ArmorChain, 0x72A8FDBB}, {ArmorPlate, 0x2D686972}} {
		w.SetGear(giant.ServerID, Gear{Armor: c.armor}, false)
		if body, size := giant.Body(); body != c.want || size != 1.8 {
			t.Errorf("%v: body %#x size %v, want %#x at its own size", c.armor, body, size, c.want)
		}
	}
	giant.Effects = []*ActiveEffect{{SpellID: 9, Form: 0x94BA77CD}}
	if body, _ := giant.Body(); body != 0x94BA77CD {
		t.Errorf("an illusion over plate: %#x", body)
	}
	w.SetGear(human.ServerID, Gear{Armor: ArmorPlate}, false)
	if body, _ := human.Body(); body != 0x70D898C6 {
		t.Errorf("the control, a human in plate: %#x", body)
	}
}

// The kinds by the chest piece's material.
func TestArmorKindByMaterial(t *testing.T) {
	for model, want := range map[int64]ArmorKind{0: ArmorNone, 1: ArmorCloth, 2: ArmorLeather, 6: ArmorLeather, 3: ArmorChain,
		7: ArmorChain, 8: ArmorChain, 4: ArmorPlate, 5: ArmorPlate, 9: ArmorPlate, 10: ArmorNone} {
		if got := ArmorKindOf(model); got != want {
			t.Errorf("material %d: %v, want %v", model, got, want)
		}
	}
}
