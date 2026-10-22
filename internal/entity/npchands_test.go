package entity

import (
	"testing"

	"github.com/eqoa/server/internal/database"
)

// sandstormNPCWeapons is every NPC weapon Sandstorm's state channels show in
// the PS2 captures (pcaps/*sandstorm*, ps2_main_capture, breath_capture,
// login_position_capture; re-tools/decode_entity_channels.py --raw): the
// NPC, the hand, the model and its attack type (bytes 91 and 92).
var sandstormNPCWeapons = []struct {
	name  string
	hand  int
	model uint32
	typ   byte
}{
	{"Ambassador Do'Nerua", 0, 0xBF7E7DCB, 3},
	{"Captain Norgam", 0, 0x47D981E0, 1},
	{"Captain Phillips", 0, 0x359EF842, 1},
	{"Chemist R`Vorr", 0, 0x45708337, 1},
	{"Coachman Quicksteed", 0, 0xBF7E7DCB, 1},
	{"Coachman Ronks", 0, 0xBF7E7DCB, 1},
	{"Coachwoman Darva", 0, 0xBF7E7DCB, 1},
	{"Commander Nothard", 0, 0xF488193F, 1},
	{"Denouncer Alshea", 0, 0x5570F89B, 1},
	{"Guard Balruss", 0, 0x3637AD33, 1},
	{"Guard Dahvel", 0, 0x47D981E0, 1},
	{"Guard Feleth", 0, 0xDCFD20C1, 1},
	{"Guard Feleth", 1, 0xA7715103, 1},
	{"Guard Fitsroy", 0, 0x024DB2A6, 0},
	{"Guard Helfax", 0, 0xA7715103, 1},
	{"Guard Palinder", 0, 0x47D981E0, 1},
	{"Guard Rendelmark", 0, 0x3637AD33, 1},
	{"Guard Serenda", 0, 0x47D981E0, 1},
	{"Guard Soalen", 0, 0x47D981E0, 1},
	{"Guard Telsto", 0, 0x47D981E0, 1},
	{"Guard Thesk", 0, 0xA7715103, 1},
	{"Guard Uzal", 0, 0xDCFD20C1, 1},
	{"Guard Uzal", 1, 0xA7715103, 1},
	{"Guard Zual", 0, 0xA7715103, 1},
	{"Jasper Darvar", 0, 0x9461B864, 5},
	{"Merchant Abirai", 0, 0xBF7E7DCB, 5},
	{"Merchant Akaron", 0, 0x2D0FC238, 5},
	{"Merchant K`Lopt", 0, 0x1246E528, 1},
	{"Merchant Salania", 0, 0x77F774ED, 5},
	{"Smithy Galez", 0, 0x1246E528, 1},
	{"Smithy N`Darg", 0, 0x1246E528, 1},
	{"Yeoman Lorne", 0, 0x704B395B, 1},
	{"enchanted armor", 0, 0xBE0B682B, 1},
}

// Every NPC weapon Sandstorm shows comes out with its type; a model the
// catalog has only as a ranged weapon takes its type (a hand Sandstorm did
// not show), and an empty hand is 0. The control: without Sandstorm's own
// exceptions the rule alone misses six, so the test can tell.
func TestNPCHandTypes(t *testing.T) {
	ranged := map[uint32]byte{0x0DA9879A: 7, 0x891F30E3: 9, 0xB0A9CBD2: 10}
	missed := 0
	for _, w := range sandstormNPCWeapons {
		var p, s uint32
		if w.hand == 0 {
			p = w.model
		} else {
			s = w.model
		}
		pt, st := npcHandTypes(w.name, p, s, ranged)
		if got := [2]byte{pt, st}[w.hand]; got != w.typ {
			t.Errorf("%s hand %d %08X: type %d, Sandstorm's %d", w.name, w.hand, w.model, got, w.typ)
		}
		if w.typ != 1 {
			missed++
		}
	}
	for _, c := range []struct {
		name               string
		primary, secondary uint32
		wantP, wantS       byte
	}{
		{"Archer Kelikor", 0, 0x0DA9879A, 0, 7},     // a bow, in the off hand
		{"crossbowman", 0x891F30E3, 0, 9, 0},        // a crossbow in the main hand
		{"Gal`Saris dragoon", 0xB0A9CBD2, 0, 10, 0}, // a throwing knife
		{"Guard Fitsroy", 0x704B395B, 0, 1, 0},      // another weapon than Sandstorm showed him with
		{"a gnoll", 0, 0, 0, 0},                     // nothing held
	} {
		if p, s := npcHandTypes(c.name, c.primary, c.secondary, ranged); p != c.wantP || s != c.wantS {
			t.Errorf("%s: %d %d, want %d %d", c.name, p, s, c.wantP, c.wantS)
		}
	}
	if missed != len(sandstormHands) {
		t.Errorf("the control: the rule alone misses %d of Sandstorm's, the table holds %d", missed, len(sandstormHands))
	}
}

// NPCFromRecord gives the NPC its hands' types, and the channel carries them.
func TestNPCFromRecordHands(t *testing.T) {
	rec := &database.NPCRecord{ID: 9, NPCName: "Merchant Abirai", PrimaryWeapon: int(int32(-1082229301)), NPCLevel: 60}
	n := NPCFromRecord(rec, 0, nil)
	if n.PrimaryType != 5 || n.SecondaryType != 0 {
		t.Fatalf("types %d %d, want 5 0", n.PrimaryType, n.SecondaryType)
	}
	buf := SerializeNPCUpdate(100009, n)
	if buf[91] != 5 || buf[92] != 0 {
		t.Errorf("channel types %d %d, want 5 0", buf[91], buf[92])
	}
}
