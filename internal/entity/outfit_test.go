package entity

import (
	"encoding/binary"
	"testing"
	"time"

	"github.com/eqoa/server/internal/database"
)

// A toy illusion (outfit.go): Santa's Elf's use shows a gnome dressed in its
// outfit, every armour slot and the robe its own, to a PS2 as the nearest
// tints and to the live look as the colours; used again it is taken off.

const (
	testElfItem = 9100
)

const (
	elfRed   = 0xC81E1EFF
	elfGreen = 0x1E8C3CFF
	elfWhite = 0xF4F4F4FF
)

// elfOutfit is the tool's (tools/toy_shoppe_20261116.py ELF_OUTFIT).
func elfOutfit() database.ItemOutfitRow {
	o := database.ItemOutfitRow{PatternID: testElfItem, Sets: [6]byte{1, 1, 1, 1, 2, 1}, Robe: 0xFF}
	for slot, c := range map[int]uint32{database.DyeChest: elfRed, database.DyeChestTrim: elfWhite, database.DyeBracer: elfGreen,
		database.DyeGloves: elfWhite, database.DyeLegs: elfGreen, database.DyeBoots: elfRed, database.DyeHelm: elfGreen,
		database.DyeHelmTrim: elfRed} {
		o.Colors[slot] = c
	}
	return o
}

func santasElf() PotionEffect {
	pe := NewPotionEffect(testElfItem, "Santa's Elf", "Self, Unlimited Uses, 30min", "Santa's Elf", 0x11111111, "", 0, 0)
	pe.SetBody(testGnomeMale)
	pe.Cooldown = time.Minute
	return pe
}

// outfitRig is bodyRig with the gnome's two bodies, Santa's Elf's outfit, and
// a drinker in plate (set 4) with a robe and colours of its own.
func outfitRig(t *testing.T) *caster {
	t.Helper()
	c := bodyRig(t)
	c.w.SetRaceModels(map[[2]int]uint32{{9, 0}: testOgreMale, {9, 1}: testOgreFemale, {3, 0}: testGnomeMale, {3, 1}: testGnomeFemale})
	SetItemOutfits(map[int]database.ItemOutfitRow{testElfItem: elfOutfit()})
	t.Cleanup(func() { itemOutfits.Store(nil) })
	p := c.p
	p.Chest, p.Bracer, p.Gloves, p.Legs, p.Boots, p.Helm = 4, 4, 4, 4, 4, 4
	p.ArmorColors = [6]uint32{0x000000FF, 0, 0, 0, 0, 0x2850C8FF}
	p.Robe, p.RobeWorn, p.RobeTint = 2, true, 4
	p.Dyes[database.DyeLegs] = 0x7A3CB4FF
	p.HeldColors[0] = 0x123456FF
	return c
}

func armourBytes(b []byte) (sets, tints [6]byte, robe, robeTint byte) {
	copy(sets[:], b[93:99])
	copy(tints[:], b[99:105])
	return sets, tints, b[109], b[110]
}

func TestSantasElfDressesTheGnomeInItsOutfit(t *testing.T) {
	c := outfitRig(t)
	own := SerializePlayerUpdate(1, c.p)
	if r := c.use(t, santasElf(), 0, 0); r != "" {
		t.Fatalf("refused: %q", r)
	}
	b := SerializePlayerUpdate(1, c.p)
	if m := binary.LittleEndian.Uint32(b[28:32]); m != testGnomeMale {
		t.Fatalf("body %#x, want the gnome man's %#x", m, testGnomeMale)
	}
	sets, tints, robe, robeTint := armourBytes(b)
	if sets != [6]byte{1, 1, 1, 1, 2, 1} || tints != [6]byte{8, 5, 0, 5, 8, 5} || robe != 0xFF || robeTint != 0 {
		t.Errorf("a PS2 is sent sets %v tints %v robe %d/%d; want cloth, leather boots, red 8 green 5 white 0, no robe", sets, tints, robe, robeTint)
	}
	d := c.p.LookDyes()
	if d[database.DyeChest] != elfRed || d[database.DyeLegs] != elfGreen || d[database.DyeChestTrim] != elfWhite ||
		d[database.DyeHelmTrim] != elfRed || d[database.DyeRobe] != 0 {
		t.Errorf("the live look's colours %08X", d[:])
	}
	if d[database.DyePrimary] != 0x123456FF {
		t.Errorf("the held item's own colour %08X, want it kept", d[database.DyePrimary])
	}
	live := LiveChannel(b, d)
	if got := binary.BigEndian.Uint32(live[111:]); got != elfRed {
		t.Errorf("the live channel's chest %08X, want the elf's red", got)
	}
	if got := binary.BigEndian.Uint32(live[111+5*3:]); got != elfGreen {
		t.Errorf("the live channel's legs %08X, want green over the drinker's purple dye", got)
	}

	// used again: taken off, its own look back; again: nothing on to take off
	if !c.w.EndItemForm(c.sess, testElfItem, "Santa's Elf", c.at(time.Minute)) {
		t.Fatal("used again while on: not taken off")
	}
	after := SerializePlayerUpdate(1, c.p)
	if string(after[93:111]) != string(own[93:111]) || binary.LittleEndian.Uint32(after[28:32]) != uint32(c.p.ModelID) {
		t.Errorf("taken off: %v, want its own %v", after[93:111], own[93:111])
	}
	if c.w.EndItemForm(c.sess, testElfItem, "Santa's Elf", c.at(2*time.Minute)) {
		t.Error("nothing on, yet taken off")
	}
	if d := c.p.LookDyes(); d[database.DyeLegs] != 0x7A3CB4FF {
		t.Errorf("its own dye after: %08X", d[database.DyeLegs])
	}
}

// A woman is the gnome woman; a GM's sprite over it shows no outfit; an
// illusion of an item with no outfit keeps the bearer's own armour, and its
// item's use again is no taking off.
func TestOutfitOnlyWhileItsBodyIsShown(t *testing.T) {
	f := outfitRig(t)
	f.p.Sex = 1
	f.use(t, santasElf(), 0, 0)
	if m, _ := channelBody(f.p); m != testGnomeFemale {
		t.Errorf("a woman: %#x, want the gnome woman's", m)
	}
	f.p.GMForm = 0x55555555
	if sets, _, _, _ := armourBytes(SerializePlayerUpdate(1, f.p)); sets != [6]byte{4, 4, 4, 4, 4, 4} {
		t.Errorf("under a GM's sprite: sets %v, want its own plate", sets)
	}

	c := outfitRig(t)
	plain := santasElf()
	plain.PatternID = 9101 // no outfit
	c.use(t, plain, 0, 0)
	b := SerializePlayerUpdate(1, c.p)
	if m := binary.LittleEndian.Uint32(b[28:32]); m != testGnomeMale {
		t.Fatalf("body %#x", m)
	}
	if sets, _, robe, _ := armourBytes(b); sets != [6]byte{4, 4, 4, 4, 4, 4} || robe != 2 {
		t.Errorf("no outfit: sets %v robe %d, want its own", sets, robe)
	}
	if c.w.EndItemForm(c.sess, 9101, "Plain", c.at(time.Minute)) {
		t.Error("an item with no outfit took its illusion off")
	}
}

// The cooldown holds a second use, as any item's.
func TestSantasElfCooldown(t *testing.T) {
	c := outfitRig(t)
	c.use(t, santasElf(), 0, 0)
	c.w.EndItemForm(c.sess, testElfItem, "Santa's Elf", c.at(10*time.Second))
	if r := c.use(t, santasElf(), 0, 20*time.Second); r == "" {
		t.Error("used again within its minute")
	}
	if r := c.use(t, santasElf(), 0, 61*time.Second); r != "" {
		t.Errorf("after its minute: %q", r)
	}
}
