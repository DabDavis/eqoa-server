package entity

import (
	"encoding/binary"
	"testing"

	"github.com/eqoa/server/internal/database"
)

// A robe's record colour (the user, 2026-11-05: "sky blue for spirit maker",
// "spectral robe is black"): white is the plain robe, 0-14 a disc tint, any
// other colour drawn so by the live look and in its nearest disc tint by the
// disc's; a grey only by a grey tint, so black is dark grey, not navy.
func TestRobeLook(t *testing.T) {
	for _, c := range []struct {
		color int64
		tint  byte
		rgba  uint32
	}{
		{0xFFFFFFFF, 0, 0}, {0, 0, 0}, {2, 2, 0}, {13, 13, 0},
		{0x00B4FFFF, 2, 0x00B4FFFF}, // sky blue: tint 2's own colour
		{0x101010FF, 1, 0x101010FF}, // black: dark grey on a disc
		{0x0000C0FF, 4, 0x0000C0FF}, // a deep blue: navy
		{0xFAAF32FF, 6, 0xFAAF32FF}, // gold: tint 6's own
	} {
		if tint, rgba := RobeLook(c.color); tint != c.tint || rgba != c.rgba {
			t.Errorf("%08X: tint %d, colour %08X; want %d, %08X", uint32(c.color), tint, rgba, c.tint, c.rgba)
		}
	}
}

// The live look draws the worn robe in its own colour where the player has
// no dye for the robe; a GM's dye wins; with none, the dyes as they are.
func TestLookDyes(t *testing.T) {
	p := &Player{}
	if p.LookDyes() != &p.Dyes {
		t.Error("no robe colour: not the player's own dyes")
	}
	p.RobeColor = 0x101010FF
	d := p.LookDyes()
	if d[database.DyeRobe] != 0x101010FF || p.Dyes[database.DyeRobe] != 0 {
		t.Errorf("a robe's colour: %08X, the player's own %08X", d[database.DyeRobe], p.Dyes[database.DyeRobe])
	}
	p.Dyes[database.DyeRobe] = 0xD10000FF
	if d := p.LookDyes(); d[database.DyeRobe] != 0xD10000FF {
		t.Errorf("a GM's dye under a robe's colour: %08X", d[database.DyeRobe])
	}
}

// Byte 110 is the robe's tint (VICreatureEntity::ReadUpdate's m_robeTint),
// never the wearer's sex: a woman in no robe sends 0, in a sky blue robe 2;
// the live look carries the robe's own colour at 141.
func TestTheChannelCarriesTheRobesTint(t *testing.T) {
	p := &Player{ServerID: 3001, Name: "Robed", Level: 30, Sex: 1}
	if buf := SerializePlayerUpdate(1, p); buf[110] != 0 || buf[109] != 0xFF {
		t.Fatalf("no robe: robe %d, byte 110 %d (want 0xFF and 0: no sex here)", buf[109], buf[110])
	}
	p.Robe, p.RobeWorn = 0, true
	p.RobeTint, p.RobeColor = RobeLook(0x101010FF)
	buf := SerializePlayerUpdate(1, p)
	if buf[109] != 0 || buf[110] != 1 {
		t.Fatalf("the black robe: robe %d, tint %d; want 0 and 1", buf[109], buf[110])
	}
	live := LiveChannel(buf, p.LookDyes())
	if got := binary.BigEndian.Uint32(live[141:]); got != 0x101010FF {
		t.Errorf("the live look's robe: %08X, want 101010FF", got)
	}
	p.RobeTint, p.RobeColor = RobeLook(0x00B4FFFF)
	if buf := SerializePlayerUpdate(1, p); buf[110] != 2 {
		t.Errorf("the sky blue robe's tint: %d, want 2", buf[110])
	}
}
