package entity

import (
	"encoding/binary"
	"testing"

	"github.com/eqoa/server/internal/database"
)

func withWispRace(t *testing.T) {
	withOptionRaces(t,
		database.RaceDefRow{ID: 30, Name: "Jal'Raeth", WireRace: 8, BodyMale: signed(bodyJalRaeth), BodyFemale: signed(bodyJalRaeth)},
		database.RaceDefRow{ID: 31, Name: "Wisp", WireRace: 0, BodyMale: signed(bodyHumanMale), BodyFemale: signed(bodyHumanFemale)})
}

// a geared player: a sword, a shield, plate on every slot, a robe
func geared(race int) *Player {
	p := &Player{Name: "Glim", Race: race, ModelID: signed(bodyHumanMale), Size: 1,
		Primary: 0x11111111, Secondary: 0x22222222, Shield: 0x33333333, PrimaryType: 1, SecondaryType: 2,
		Chest: 3, Bracer: 3, Gloves: 3, Legs: 3, Boots: 3, Helm: 3, RobeWorn: true, Robe: 4, RobeTint: 5}
	for i := range p.ArmorColors {
		p.ArmorColors[i] = 0xFF0000FF
	}
	return p
}

// A Wisp in its own form is sent with nothing worn (held items, armour, tints,
// robe), its attack types kept; a Wisp under an illusion (a glamour) and a human
// (the control) in all of it. The character list's gear goes bare the same way.
func TestAWispInItsOwnFormShowsNothingWorn(t *testing.T) {
	withWispRace(t)
	if !IsWispRace(31) || IsWispRace(30) || IsWispRace(0) {
		t.Fatal("the Wisp is race 31 alone")
	}
	worn := func(b []byte) bool {
		return binary.LittleEndian.Uint32(b[67:71]) != 0 && binary.LittleEndian.Uint32(b[75:79]) != 0 &&
			b[93] != 0 && b[98] != 0 && b[109] != 0xFF
	}
	bare := func(b []byte) bool {
		for i := 67; i < 79; i++ {
			if b[i] != 0 {
				return false
			}
		}
		for i := 93; i < 105; i++ {
			if b[i] != 0 {
				return false
			}
		}
		return b[109] == 0xFF && b[110] == 0
	}

	w := geared(31)
	if !w.WispForm() || w.wispMortal() {
		t.Fatal("a Wisp in its own body is in its own form")
	}
	b := SerializePlayerUpdate(7, w)
	if !bare(b) || b[91] != 1 || b[92] != 2 {
		t.Errorf("a Wisp in its own form: % x (attack types %d %d)", b[67:111], b[91], b[92])
	}
	if string(b[116:120]) != "Glim" || binary.LittleEndian.Uint32(b[28:32]) != bodyHumanMale {
		t.Error("the rest of the channel is as ever")
	}
	if w.Primary == 0 || w.Chest == 0 {
		t.Error("the Wisp's own gear is untouched")
	}

	w.Effects = []*ActiveEffect{{SpellID: 496, Name: "Glamour: Human", Form: bodyHumanMale}}
	if w.WispForm() || !w.wispMortal() {
		t.Fatal("a glamoured Wisp is in a mortal form")
	}
	if b := SerializePlayerUpdate(7, w); !worn(b) {
		t.Errorf("a glamoured Wisp shows its gear: % x", b[67:111])
	}
	w.Effects = nil
	w.GMForm = 0x12345678
	if w.WispForm() {
		t.Error("a GM's Change Sprite is another body")
	}

	if b := SerializePlayerUpdate(7, geared(0)); !worn(b) {
		t.Errorf("the control, a human: % x", b[67:111])
	}

	g := make([]byte, 32)
	for i := range g {
		g[i] = 7
	}
	g[17], g[18] = 1, 2
	HideCharacterGear(g)
	if binary.LittleEndian.Uint32(g[0:4]) != 0xFFFFFFFF || g[4] != 0 || g[5] != 0 || g[16] != 0 || g[17] != 1 || g[18] != 2 ||
		g[20] != 0xFF || g[25] != 0xFF || g[26] != 0 || g[31] != 0 {
		t.Errorf("the character list's gear, bare: % x", g)
	}
}

// The looks leave a mortal-form Wisp's wisp out (a glamour of a human is the
// Wisp's own model, so a client could not tell otherwise); every other Wisp
// keeps it. The tick notes the forms and the looks are made again from the
// characters last read, and only on a change.
func TestAMortalWispLosesItsWispFromTheLooks(t *testing.T) {
	withWispRace(t)
	looks := []database.CharLook{{Part: "wisp", Kind: database.LookTexture, Value: bodyWillOWisp}}
	chars := []database.LookCharacter{
		{Name: "Glim", Race: 31, ModelID: signed(bodyHumanMale), Looks: looks},
		{Name: "Wil", Race: 31, ModelID: signed(bodyHumanMale), Looks: looks},
	}
	all := encodeRaceLooks(chars, nil)
	if string(all) != string(EncodeRaceLooks(chars)) || binary.LittleEndian.Uint16(all[1:3]) != 2 {
		t.Fatalf("both Wisps: % x", all)
	}
	one := encodeRaceLooks(chars, map[string]bool{"Glim": true})
	if binary.LittleEndian.Uint16(one[1:3]) != 1 || string(one[4:7]) != "Wil" {
		t.Errorf("Glim in a mortal form is left out: % x", one)
	}

	w := NewWorld()
	w.noteWispFormsLocked(nil)
	if w.wispLooks.key != "" || w.wispLooks.mortalNames()["Glim"] {
		t.Fatal("no Wisp in a mortal form")
	}
	w.noteWispFormsLocked([]string{"Wil", "Glim"})
	if m := w.wispLooks.mortalNames(); !m["Glim"] || !m["Wil"] || len(m) != 2 {
		t.Errorf("both mortal: %v", m)
	}
	w.noteWispFormsLocked([]string{"Glim", "Wil"})
	if m := w.wispLooks.mortalNames(); len(m) != 2 {
		t.Errorf("the same set, unchanged: %v", m)
	}
	w.noteWispFormsLocked(nil)
	if len(w.wispLooks.mortalNames()) != 0 {
		t.Error("the glamours ended: every wisp back")
	}
}

// The Wisp's rows as tools/wisp_kit_20261111.py writes them parse to their
// figures and are listed for race 31 alone (its bit, 1 << 31), any class; the
// control: the human (race 0), whose mask is 1, and the Jal'Raeth (30).
func TestTheWispsMasteriesAreTheWisps(t *testing.T) {
	withWispRace(t)
	row := func(id, cat int, effects string) database.CMRankRow {
		return database.CMRankRow{ID: id, Category: cat, Classes: 32767, Races: 1 << 31, Effects: effects, Enabled: true}
	}
	will := ParseCMRank(row(2141, 1, "PoT:10"))
	mc := ParseCMRank(row(2143, CMCategoryMaster, "DEXMax:50,INTMax:25,WISMax:25,Pow:500,STRMax:-25,STAMax:-25,AGIMax:-25,AC:-250"))
	if will.Effects[cmSlotPoT] != 10 {
		t.Errorf("Will-of-the-Wisp: %v", will.Effects)
	}
	e := mc.Effects
	if e[10] != 50 || e[12] != 25 || e[11] != 25 || e[cmSlotPower] != 500 || e[7] != -25 || e[8] != -25 || e[9] != -25 || e[cmSlotAC] != -250 {
		t.Errorf("Transcendent: %v", e)
	}
	for class := 0; class < 15; class++ {
		if !will.OfferedTo(class, 31) || !mc.OfferedTo(class, 31) {
			t.Errorf("a Wisp of class %d is not offered them", class)
		}
	}
	if will.OfferedTo(0, 0) || will.OfferedTo(0, 30) || mc.OfferedTo(13, 0) {
		t.Error("the controls: a human or a Jal'Raeth is offered the Wisp's")
	}
}
