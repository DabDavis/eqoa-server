package entity

import (
	"encoding/binary"
	"math"
	"testing"
	"time"

	"github.com/eqoa/server/internal/database"
)

// Potions that show a body, illusions on creatures and a thrown potion's two
// shares (2026-11-01), against the cast tests' world (newCaster: a druid of
// 23, Rotting Flesh at 2 off). Every body, id and figure is made up.

const (
	testOgreMale   uint32 = 0x0A0B0C01
	testOgreFemale uint32 = 0x0A0B0C02
	testRatBody    uint32 = 0x0D0E0F03
	testFleshBody  uint32 = 0x0F0F0F0F
)

// bodyRig is newCaster's world with a race's two bodies (race 9, as the
// network enum has the ogre), three rats wearing their body at 0.75, and
// Rotting Flesh on a body of its own at 1.5.
func bodyRig(t *testing.T) *caster {
	t.Helper()
	c := newCaster(t)
	c.w.SetRaceModels(map[[2]int]uint32{{9, 0}: testOgreMale, {9, 1}: testOgreFemale})
	for i := 0; i < 3; i++ {
		id := EntityID(6000 + i)
		c.w.NPCs[id] = &NPC{ServerID: id, Name: "Test Rat", Level: 1, HP: 10, MaxHP: 10, ModelID: int64(testRatBody),
			Size: 0.75, X: 5000, Z: 5000}
	}
	c.npc.ModelID, c.npc.Size = int64(testFleshBody), 1.5
	c.npc.SpawnX, c.npc.SpawnY, c.npc.SpawnZ = c.npc.X, c.npc.Y, c.npc.Z
	c.npc.HP, c.npc.MaxHP = 1000, 1000
	return c
}

// npcBody is the model and scale a creature's entity channel shows.
func npcBody(n *NPC) (uint32, float32) {
	b := SerializeNPCUpdate(1, n)
	return binary.LittleEndian.Uint32(b[28:32]), math.Float32frombits(binary.LittleEndian.Uint32(b[36:40]))
}

func pill() PotionEffect {
	return NewPotionEffect(9001, "Test Pill", "Self, Increase Maximum HP by 100, Increase Hitpoints by 100, 30min",
		"Illusion: Ogre", 0x11111111, "", 0, 0)
}

// A drink whose effect is named as an illusion spell is ("Illusion: Ogre")
// shows its drinker in that race's body of its own sex for the drink's time,
// on the effects list by that name, its heal after its raise; the control,
// the same line under a name of no illusion, shows none.
func TestDrunkPotionShowsTheBodyItsNameSays(t *testing.T) {
	c := bodyRig(t)
	if r := c.use(t, pill(), 0, 0); r != "" {
		t.Fatalf("refused: %q", r)
	}
	if m, _ := channelBody(c.p); m != testOgreMale {
		t.Fatalf("the drinker's body %#x, want the ogre's %#x", m, testOgreMale)
	}
	if c.p.HP != 600 || len(c.p.Buffs) != 1 || c.p.Buffs[0].Name != "Illusion: Ogre" {
		t.Errorf("HP %d (want 600: healed after the raise), effects %+v", c.p.HP, c.p.Buffs)
	}
	c.potionTick(31 * time.Minute)
	if m, _ := channelBody(c.p); m != uint32(c.p.ModelID) {
		t.Errorf("after its thirty minutes: %#x, want the drinker's own", m)
	}

	f := bodyRig(t)
	f.p.Sex = 1
	f.use(t, pill(), 0, 0)
	if m, _ := channelBody(f.p); m != testOgreFemale {
		t.Errorf("a woman drinks it: %#x, want the ogre woman's %#x", m, testOgreFemale)
	}

	ctl := bodyRig(t)
	plain := NewPotionEffect(9001, "Test Pill", "Self, Increase Maximum HP by 100, Increase Hitpoints by 100, 30min", "", 0, "", 0, 0)
	ctl.use(t, plain, 0, 0)
	if m, _ := channelBody(ctl.p); m != uint32(ctl.p.ModelID) {
		t.Errorf("the control: no illusion in its name, yet %#x", m)
	}
}

// A drink's row body (the workbook's sprite) is its drinker's: a race's body
// the drinker's own sex's of that race, a creature's as it is.
func TestDrunkPotionShowsItsRowsBody(t *testing.T) {
	c := bodyRig(t)
	c.p.Sex = 1
	pe := NewPotionEffect(9002, "Test Draught", "Self, Increase STR by 10, 30min", "", 0, "", 0, 0)
	pe.SetBody(testOgreMale)
	c.use(t, pe, 0, 0)
	if m, _ := channelBody(c.p); m != testOgreFemale {
		t.Errorf("the male ogre's body drunk by a woman: %#x, want the ogre woman's %#x", m, testOgreFemale)
	}
	d := bodyRig(t)
	rat := NewPotionEffect(9003, "Test Draught", "Self, Increase STR by 10, 30min", "", 0, "", 0, 0)
	rat.SetBody(testRatBody)
	d.use(t, rat, 0, 0)
	if m, _ := channelBody(d.p); m != testRatBody {
		t.Errorf("a creature's body: %#x, want %#x", m, testRatBody)
	}
}

func transmuter() PotionEffect {
	pe := NewPotionEffect(9004, "Test Transmuter", "Target, Decrease STR by 75, Decrease AC by 300, 24sec", "", 0, "", 0, 0)
	pe.SetBody(testRatBody)
	return pe
}

// A thrown potion's body goes on the creature it hits, at the size its body's
// wearers have, for the potion's time; then the creature's own again, and at
// its death (a corpse is its own body), its respawn and a GM's refresh of its
// row; a copy of it (a quest's spawn) is made with none. The thrower keeps
// its own. The control: the same potion with no body changes none.
func TestThrownPotionPutsItsBodyOnTheCreature(t *testing.T) {
	c := bodyRig(t)
	if r := c.use(t, transmuter(), c.npcO, 0); r != "" {
		t.Fatalf("refused: %q", r)
	}
	if m, s := npcBody(c.npc); m != testRatBody || s != 0.75 {
		t.Fatalf("the creature: %#x at %v, want the rat's %#x at its wearers' 0.75", m, s, testRatBody)
	}
	if m, _ := channelBody(c.p); m != uint32(c.p.ModelID) {
		t.Errorf("the thrower shows %#x", m)
	}
	c.tick(25 * time.Second)
	if m, s := npcBody(c.npc); m != testFleshBody || s != 1.5 {
		t.Errorf("after its 24 s: %#x at %v, want its own %#x at 1.5", m, s, testFleshBody)
	}

	// a copy made while it wears the body (SpawnNPCCopy) has none of it
	c.use(t, transmuter(), c.npcO, 30*time.Second)
	id, _, ok := c.w.SpawnNPCCopy("Rotting Flesh", 200, 10, 200, 0, 0)
	if !ok {
		t.Fatal("no copy")
	}
	if cp := c.w.NPCs[id]; len(cp.Effects) != 0 {
		t.Errorf("the copy carries %d effects", len(cp.Effects))
	} else if m, _ := npcBody(cp); m != testFleshBody {
		t.Errorf("the copy shows %#x", m)
	}
	// its death: the corpse is its own body
	c.w.mu.Lock()
	c.w.killNPC(c.npc)
	c.w.mu.Unlock()
	if m, _ := npcBody(c.npc); m != testFleshBody {
		t.Errorf("its corpse shows %#x", m)
	}
	c.tick(31 * time.Second)
	c.w.mu.Lock()
	c.w.respawnNPCLocked(c.npc)
	c.w.mu.Unlock()
	if m, _ := npcBody(c.npc); m != testFleshBody || len(c.npc.Effects) != 0 {
		t.Errorf("respawned: %#x with %d effects", m, len(c.npc.Effects))
	}

	// a GM's refresh of its row (the panel's edit, UpsertNPC)
	c.npc.HP = 1000
	c.use(t, transmuter(), c.npcO, 40*time.Second)
	if m, _ := npcBody(c.npc); m != testRatBody {
		t.Fatalf("thrown again: %#x", m)
	}
	rec := &database.NPCRecord{ID: int(c.npc.ServerID), NPCName: "Rotting Flesh", XCoord: 102, YCoord: 10, ZCoord: 100,
		HP: 1000, ModelID: int(testFleshBody), Size: 1.5, NPCLevel: 1}
	if _, err := c.w.UpsertNPC(rec, -1); err != nil {
		t.Fatal(err)
	}
	if m, _ := npcBody(c.w.NPCs[c.npc.ServerID]); m != testFleshBody {
		t.Errorf("after a GM's refresh: %#x", m)
	}

	ctl := bodyRig(t)
	plain := NewPotionEffect(9004, "Test Transmuter", "Target, Decrease STR by 75, Decrease AC by 300, 24sec", "", 0, "", 0, 0)
	ctl.use(t, plain, ctl.npcO, 0)
	if m, _ := npcBody(ctl.npc); m != testFleshBody || len(ctl.npc.Effects) != 1 {
		t.Errorf("the control: %#x with %d effects; want its own body under the debuff", m, len(ctl.npc.Effects))
	}
}

// A workbook illusion's sprite is the body the other way round, and a race's
// body by sprite is the bearer's own sex's of that race (the workbook gives
// the male one): the control is the sprite as written, which is no body.
func TestWorkbookSpriteIsTheBodyTheOtherWayRound(t *testing.T) {
	sprite := `{"v":1,"scope":"self","duration_s":600,"effects":[{"kind":"illusion","sprite":"0x010C0B0A"}]}`
	for _, sex := range []int{0, 1} {
		c := bodyRig(t)
		c.p.Sex = sex
		c.land(t, wbSpell(9100, 20, 0, "Test Ogre Mask", sprite), 0xFFFFFFFF, 0)
		want := []uint32{testOgreMale, testOgreFemale}[sex]
		if m, _ := channelBody(c.p); m != want {
			t.Errorf("sex %d: %#x, want %#x (never the sprite as written, 0x010C0B0A)", sex, m, want)
		}
	}
}

// A thrown potion with a target's own share (target_line: the workbook's
// first figure) hits its target with both, every other creature in its
// splash with the line alone, and what lasts of it lands on each once. The
// control: with no share of its own the target takes the line, as the rest.
func TestThrownSplashTargetTakesBothShares(t *testing.T) {
	c := bodyRig(t)
	c.w.SpatialIndex.Insert(c.npc.ServerID, c.npc.X, c.npc.Z)
	near := &NPC{ServerID: 3001, Name: "Rat", Level: 1, HP: 100, MaxHP: 100, X: 106, Y: 10, Z: 100}
	c.w.NPCs[near.ServerID] = near
	c.w.SpatialIndex.Insert(near.ServerID, near.X, near.Z)
	splash := func() PotionEffect {
		return NewPotionEffect(9005, "Test Cocktail", "Target, Decrease Hitpoints by 20, Decrease STR by 5, 18sec", "", 0, "", 0, 10)
	}
	pe := splash()
	pe.SetTargetShare("Target, Decrease Hitpoints by 15")
	if r := c.use(t, pe, c.npcO, 0); r != "" {
		t.Fatalf("refused: %q", r)
	}
	if c.npc.HP != 1000-35 || near.HP != 100-20 {
		t.Errorf("the target at %d, the one in its splash at %d; want %d and %d", c.npc.HP, near.HP, 1000-35, 100-20)
	}
	for _, n := range []*NPC{c.npc, near} {
		if len(n.Effects) != 1 || n.Effects[0].Mods.Str != -5 {
			t.Errorf("%s's effects %+v; want the line's -5 STR once", n.Name, n.Effects)
		}
	}

	ctl := bodyRig(t)
	ctl.use(t, splash(), ctl.npcO, 0)
	if ctl.npc.HP != 1000-20 {
		t.Errorf("the control: the target at %d, want %d (the line alone)", ctl.npc.HP, 1000-20)
	}
}

// A body the disc lacks (not in the bodies' table, reach.go's models.txt) is
// shown on no one, a drinker or a creature, though the rest of the use lands;
// the control: the same body in the table is shown.
func TestABodyTheDiscLacksIsShownOnNoOne(t *testing.T) {
	SetModelRadii(map[uint32]float32{testOgreMale: 1, testOgreFemale: 1, testFleshBody: 1})
	t.Cleanup(func() { SetModelRadii(nil) })
	c := bodyRig(t)
	rat := NewPotionEffect(9003, "Test Draught", "Self, Increase STR by 10, 30min", "", 0, "", 0, 0)
	rat.SetBody(testRatBody)
	c.use(t, rat, 0, 0)
	if m, _ := channelBody(c.p); m != uint32(c.p.ModelID) || c.p.Mods.Str != 10 {
		t.Errorf("a drink of a body the disc lacks: %#x, STR %+d; want its own body and the STR", m, c.p.Mods.Str)
	}
	c.use(t, transmuter(), c.npcO, 0)
	if m, _ := npcBody(c.npc); m != testFleshBody || len(c.npc.Effects) != 1 {
		t.Errorf("thrown: %#x with %d effects; want its own body under the debuff", m, len(c.npc.Effects))
	}

	SetModelRadii(map[uint32]float32{testRatBody: 1})
	ctl := bodyRig(t)
	ctl.use(t, transmuter(), ctl.npcO, 0)
	if m, _ := npcBody(ctl.npc); m != testRatBody {
		t.Errorf("the control: the rat's body in the table, yet %#x", m)
	}
}
