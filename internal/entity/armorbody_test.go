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

// A hill giantess whose body is racedefs' female one (the GM Jibrish's petition 1647: the hill
// giant, the orc and the satyr given their own female bodies) keeps it in any armour, as a female
// giant chosen on the appearance page does: the armour bodies are a male giant's each. The
// controls: a hill giant man takes them, and so does a woman made before, wearing the male's body.
func TestAGiantessKeepsHerOwnBodyInArmour(t *testing.T) {
	const giantess = 0xF34501CB
	withOptionRaces(t, database.RaceDefRow{ID: 11, Name: "Hill Giant", WireRace: 9, BodyMale: -5117396, BodyFemale: signed(giantess)})
	w := NewWorld()
	w.SetRaceArmorBodies([]database.RaceArmorBody{{Race: 11, Armor: "cloth", Body: 0xE617303B}, {Race: 11, Armor: "plate", Body: 0x2D686972}})
	her := &Player{ServerID: 1, Name: "Olga", Race: 11, Sex: 1, ModelID: signed(giantess), Size: 1.8}
	him := &Player{ServerID: 2, Name: "Bulbor", Race: 11, ModelID: -5117396, Size: 1.8}
	old := &Player{ServerID: 3, Name: "Before", Race: 11, Sex: 1, ModelID: -5117396, Size: 1.8}
	for _, p := range []*Player{her, him, old} {
		w.Players[p.ServerID] = p
	}
	for _, c := range []struct {
		armor        ArmorKind
		her, him, ol uint32
	}{{ArmorNone, giantess, 0xFFB1EA2C, 0xFFB1EA2C}, {ArmorCloth, giantess, 0xE617303B, 0xE617303B}, {ArmorPlate, giantess, 0x2D686972, 0x2D686972}} {
		for _, p := range []*Player{her, him, old} {
			w.SetGear(p.ServerID, Gear{Armor: c.armor}, false)
		}
		if b, s := her.Body(); b != c.her || s != 1.8 {
			t.Errorf("the giantess in %v: %#x at %v, want her own %#x", c.armor, b, s, c.her)
		}
		if b, _ := him.Body(); b != c.him {
			t.Errorf("the control, a hill giant in %v: %#x, want %#x", c.armor, b, c.him)
		}
		if b, _ := old.Body(); b != c.ol {
			t.Errorf("the control, a woman in the male's body in %v: %#x, want %#x", c.armor, b, c.ol)
		}
	}
	if ChosenBody(11, giantess) {
		t.Error("her race's own female body is no chosen one")
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
