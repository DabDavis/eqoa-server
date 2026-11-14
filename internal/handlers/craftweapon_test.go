package handlers

import (
	"fmt"
	"testing"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/protocol"
)

// Weaponsmithing through the wire (0x1101 in, 0x1102 out), on records shaped
// as tools/seed_tradeskills.py --craft weapon writes them: Sandstorm's
// Assistant Weaponsmith Hammer (wear 22, the Weapon skill's tool; level 11,
// tier 3), Bronze Ore, the Axe Head Mold and a Magnesium Alloy; the recipe a
// fiery bronze hatchet (skill 0, the Bronze tab, trade level 3, uncommon:
// difficulty 32, XP 150), the alloy kept on a failure; the product Prima's
// one-handed bronze damage (136) with its proc in its words. A failure loses
// the ore and the mold and keeps the alloy; a success makes the hatchet and
// raises Weapon Crafting, and the made record procs as its words say. The
// mundane bronze hatchet (ore and mold) is found by an Experiment. The
// control: the armorsmith's hammer in Held does not do.
func TestWeaponsmithingEndToEnd(t *testing.T) {
	f := newCraftFixture(t)
	hammer := f.item("Assistant Weaponsmith Hammer", 22, 11, 1, 3000, 0, 980933879, "A regular forge hammer suitable for forging bronze.")
	ore := f.item("Bronze Ore", -1, 1, 99, 0, 1, 1577500738, "")
	mold := f.item("Axe Head Mold", -1, 1, 99, 0, 1, -649479524, "A clay mold used in the making of weapons.")
	alloy := f.item("Magnesium Alloy", -1, 0, 50, 0, 1, 7010, "")
	const procWords = "A weapon forged of bronze. Procs 10-20 fire damage, a 10% chance each hit."
	fiery := f.item("fiery bronze hatchet", 12, 13, 1, 200*13+200, 0, 1822970346, procWords)
	plain := f.item("bronze hatchet", 12, 13, 1, 200*13+200, 0, 1822970346, "A weapon forged of bronze.")
	for _, p := range []int{fiery, plain} {
		if _, err := f.db.Exec(`UPDATE itempattern SET attacktype = 1, weapondamage = 136, duration = 57, model = 552762123,
			classuse = 127 WHERE patternid = ?`, p); err != nil {
			t.Fatal(err)
		}
	}
	const fieryRecipe, plainRecipe = 501, 502
	f.recipe(fieryRecipe, 0, 2, 3, "fiery bronze hatchet", fiery, 1, 32, 150, true, 0, fmt.Sprintf(
		`[{"item": %d, "count": 1, "keep": false}, {"item": %d, "count": 1, "keep": false}, {"item": %d, "count": 1, "keep": true}]`,
		ore, mold, alloy))
	f.recipe(plainRecipe, 0, 2, 3, "bronze hatchet", plain, 1, 30, 75, true, 0, fmt.Sprintf(
		`[{"item": %d, "count": 1, "keep": false}, {"item": %d, "count": 1, "keep": false}]`, ore, mold))
	if err := f.db.SetCharCraftSkill(f.sid, 0, database.CraftSkill{Level: 3, Row: true}); err != nil {
		t.Fatal(err)
	}
	f.give(hammer, 1, 3000, equipHeld)
	f.give(ore, 3, 0, -1)
	f.give(mold, 3, 0, -1)
	f.give(alloy, 2, 0, -1)
	f.loadList()
	f.learn(fieryRecipe)

	// the Weapons book (skill 0): the recipe under Bronze (tab 2), uncommon
	book := onlyOne(t, f.u32req(craftReqBook, 0))
	want := reply(15).u32(0).u32(1).zz(0).zz(0).zz(fieryRecipe).u8(3).u8(0).u8(2).u8(0).wstr("fiery bronze hatchet").b
	mustBytes(t, "15", book, want)

	// a failure: the ore and the mold lost, the alloy kept, the hammer worn
	f.dice = []float64{0.99}
	got := f.create(fieryRecipe)
	mustOps(t, got, opcodeCraftReply, protocol.OpcodeAdjustItemHP, protocol.OpcodeRemoveInvItem, protocol.OpcodeRemoveInvItem)
	if f.stack(ore) != 2 || f.stack(mold) != 2 || f.stack(alloy) != 2 || f.stack(fiery) != 0 {
		t.Fatalf("after a failure: ore %d, molds %d, alloys %d, hatchets %d; want 2, 2, 2, 0",
			f.stack(ore), f.stack(mold), f.stack(alloy), f.stack(fiery))
	}

	// a success: the hatchet made (0x003B), everything taken, Weapon Crafting's XP
	f.clock = f.clock.Add(craftCooldown)
	f.dice = []float64{0.1}
	got = f.create(fieryRecipe)
	mustOps(t, got, opcodeCraftReply, protocol.OpcodeAdjustItemHP, protocol.OpcodeRemoveInvItem,
		protocol.OpcodeRemoveInvItem, protocol.OpcodeRemoveInvItem, protocol.OpcodeAddInvItem)
	if got[0][2] != craftRepMade {
		t.Fatalf("a success: reply %d", got[0][2])
	}
	if qty, hp, _, p := netItemHead(t, got[5][2:]); qty != 1 || hp != 2800 || p != int64(fiery) {
		t.Fatalf("0x003B: %d of %d at %d; want 1 of the fiery hatchet at 2800", qty, p, hp)
	}
	if f.stack(ore) != 1 || f.stack(mold) != 1 || f.stack(alloy) != 1 || f.stack(fiery) != 1 {
		t.Fatalf("after the make: ore %d, molds %d, alloys %d, hatchets %d", f.stack(ore), f.stack(mold), f.stack(alloy), f.stack(fiery))
	}
	if s := f.skill(0); s.Level != 3 || s.XP != 150 {
		t.Fatalf("Weapon Crafting after the make: %+v", s)
	}
	// the made weapon's proc, read off its record as a wielder's hand reads it
	rec, err := database.NewItemPatternStore(f.db).GetItemPattern(fiery)
	if err != nil {
		t.Fatal(err)
	}
	wp, ok := entity.WeaponProcOf(rec.PatternID, rec.ItemName, rec.ItemDesc)
	if !ok || wp.Kind != entity.ProcHit || wp.Damage != 10 || wp.DamageTo != 20 || wp.Resist != entity.StatFR || wp.Chance != 10 {
		t.Fatalf("the hatchet's proc: %+v %v", wp, ok)
	}

	// the mundane one found by an Experiment: ore and mold, a discovery offered
	f.clock = f.clock.Add(craftCooldown)
	f.dice = []float64{0.1}
	got = f.combine(ore, mold)
	if got[0][2] != craftRepExperiment || got[0][3] != craftExpDiscovered {
		t.Fatalf("an Experiment of ore and mold: % X", got[0][:4])
	}
	if f.stack(plain) != 1 || f.stack(ore) != 0 || f.stack(mold) != 0 {
		t.Fatalf("after the Experiment: hatchets %d, ore %d, molds %d", f.stack(plain), f.stack(ore), f.stack(mold))
	}

	// the control: the Armor skill's hammer (wear 23) in Held makes no weapon
	g := newCraftFixture(t)
	gOre := g.item("Bronze Ore", -1, 1, 99, 0, 1, 1, "")
	gMold := g.item("Axe Head Mold", -1, 1, 99, 0, 1, 2, "")
	gProduct := g.item("bronze hatchet", 12, 13, 1, 2800, 0, 3, "")
	g.recipe(plainRecipe, 0, 2, 3, "bronze hatchet", gProduct, 1, 30, 75, true, 0, fmt.Sprintf(
		`[{"item": %d, "count": 1, "keep": false}, {"item": %d, "count": 1, "keep": false}]`, gOre, gMold))
	if err := g.db.SetCharCraftSkill(g.sid, 0, database.CraftSkill{Level: 3, Row: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := g.db.Exec(`UPDATE itempattern SET levelreq = 11 WHERE patternid = ?`, g.hammer); err != nil {
		t.Fatal(err)
	}
	g.give(g.hammer, 1, 100, equipHeld)
	g.give(gOre, 1, 0, -1)
	g.give(gMold, 1, 0, -1)
	g.loadList()
	g.learn(plainRecipe)
	got = g.create(plainRecipe)
	mustOps(t, got, opcodeCraftReply, protocol.OpcodeColoredChat)
	mustBytes(t, "the armorsmith's hammer", got[1], redLine("You need your Weapon Crafting tool equipped."))
	if g.stack(gOre) != 1 || g.stack(gProduct) != 0 {
		t.Fatal("a refusal took the ore or made the hatchet")
	}
}
