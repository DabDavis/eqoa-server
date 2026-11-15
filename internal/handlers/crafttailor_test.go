package handlers

import (
	"fmt"
	"testing"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/protocol"
)

// Tailoring through the wire (0x1101 in, 0x1102 out), on records shaped as
// tools/seed_tradeskills.py --craft tailor writes them: Sandstorm's Adept
// Tailor's Kit (wear 24, the Tailoring skill's tool; level 21, tier 5), a
// Sheet of Suede Hide, the Leather Gloves Pattern and a Tunare Hair Thread;
// the recipe Tunare Suede Gloves (skill 2, the Suede tab 7 of CLIENT's
// nineteen, trade level 5, uncommon: difficulty 52, XP 250), the thread kept
// on a failure; the product Prima's leather gloves AC at trade level 5 (43)
// and the thread's 7 agility and 7 wisdom. A failure loses the hide and the
// pattern and keeps the thread; a success makes the gloves and raises
// Tailoring. The mundane Suede Gloves (hide and pattern) are found by an
// Experiment. The controls: the armorsmith's hammer in Held does not do, and
// the Novice kit (tier 1) is not good enough for a trade level 5 recipe.
func TestTailoringEndToEnd(t *testing.T) {
	const tunareRecipe, plainRecipe = 601, 602
	setup := func(kitName string, kitWear, kitLevel int) (*craftFixture, int, int, int, int, int, int) {
		f := newCraftFixture(t)
		kit := f.item(kitName, kitWear, kitLevel, 1, 3000, 0, -712640932, "A tailoring kit.")
		hide := f.item("Sheet of Suede Hide", -1, 0, 99, 0, 1, -1063213915, "")
		pattern := f.item("Leather Gloves Pattern", -1, 1, 99, 0, 1, -749915304, "")
		thread := f.item("Tunare Hair Thread", -1, 0, 50, 0, 1, 441928322, "")
		tunare := f.item("Tunare Suede Gloves", 19, 23, 1, 200*23+800, 0, -833429507, "Leather armor sewn of suede.")
		plain := f.item("Suede Gloves", 19, 23, 1, 200*23+800, 0, -833429507, "Leather armor sewn of suede.")
		if _, err := f.db.Exec(`UPDATE itempattern SET AC = 43, model = 2, color = 14, duration = 65, classuse = 17407
			WHERE patternid IN (?, ?)`, tunare, plain); err != nil {
			t.Fatal(err)
		}
		if _, err := f.db.Exec(`UPDATE itempattern SET agi = 7, wis = 7 WHERE patternid = ?`, tunare); err != nil {
			t.Fatal(err)
		}
		f.recipe(tunareRecipe, craftTailoring, 7, 5, "Tunare Suede Gloves", tunare, 1, 52, 250, true, 0, fmt.Sprintf(
			`[{"item": %d, "count": 1, "keep": false}, {"item": %d, "count": 1, "keep": false}, {"item": %d, "count": 1, "keep": true}]`,
			hide, pattern, thread))
		f.recipe(plainRecipe, craftTailoring, 7, 5, "Suede Gloves", plain, 1, 50, 125, true, 0, fmt.Sprintf(
			`[{"item": %d, "count": 1, "keep": false}, {"item": %d, "count": 1, "keep": false}]`, hide, pattern))
		if err := f.db.SetCharCraftSkill(f.sid, craftTailoring, database.CraftSkill{Level: 5, Row: true}); err != nil {
			t.Fatal(err)
		}
		f.give(kit, 1, 3000, equipHeld)
		f.give(hide, 3, 0, -1)
		f.give(pattern, 3, 0, -1)
		f.give(thread, 2, 0, -1)
		f.loadList()
		f.learn(tunareRecipe)
		return f, kit, hide, pattern, thread, tunare, plain
	}
	f, _, hide, pattern, thread, tunare, plain := setup("Adept Tailor's Kit", wearCraftTool0+craftTailoring, 21)

	// the Tailoring book (skill 2): the recipe of skill 2 under Suede (tab 7)
	book := onlyOne(t, f.u32req(craftReqBook, craftTailoring))
	want := reply(15).u32(0).u32(1).zz(0).zz(0).zz(tunareRecipe).u8(5).u8(craftTailoring).u8(7).u8(0).wstr("Tunare Suede Gloves").b
	mustBytes(t, "15", book, want)

	// a failure: the hide and the pattern lost, the thread kept, the kit worn
	f.dice = []float64{0.99}
	got := f.create(tunareRecipe)
	mustOps(t, got, opcodeCraftReply, protocol.OpcodeAdjustItemHP, protocol.OpcodeRemoveInvItem, protocol.OpcodeRemoveInvItem)
	if f.stack(hide) != 2 || f.stack(pattern) != 2 || f.stack(thread) != 2 || f.stack(tunare) != 0 {
		t.Fatalf("after a failure: hides %d, patterns %d, threads %d, gloves %d; want 2, 2, 2, 0",
			f.stack(hide), f.stack(pattern), f.stack(thread), f.stack(tunare))
	}

	// a success: the gloves made (0x003B), everything taken, Tailoring's XP
	f.clock = f.clock.Add(craftCooldown)
	f.dice = []float64{0.1}
	got = f.create(tunareRecipe)
	mustOps(t, got, opcodeCraftReply, protocol.OpcodeAdjustItemHP, protocol.OpcodeRemoveInvItem,
		protocol.OpcodeRemoveInvItem, protocol.OpcodeRemoveInvItem, protocol.OpcodeAddInvItem)
	if got[0][2] != craftRepMade {
		t.Fatalf("a success: reply %d", got[0][2])
	}
	if qty, hp, _, p := netItemHead(t, got[5][2:]); qty != 1 || hp != 200*23+800 || p != int64(tunare) {
		t.Fatalf("0x003B: %d of %d at %d; want 1 of the Tunare gloves at %d", qty, p, hp, 200*23+800)
	}
	if f.stack(hide) != 1 || f.stack(pattern) != 1 || f.stack(thread) != 1 || f.stack(tunare) != 1 {
		t.Fatalf("after the make: hides %d, patterns %d, threads %d, gloves %d",
			f.stack(hide), f.stack(pattern), f.stack(thread), f.stack(tunare))
	}
	if s := f.skill(craftTailoring); s.Level != 5 || s.XP != 250 {
		t.Fatalf("Tailoring after the make: %+v", s)
	}

	// the mundane gloves found by an Experiment: hide and pattern, a discovery offered
	f.clock = f.clock.Add(craftCooldown)
	f.dice = []float64{0.1}
	got = f.combine(hide, pattern)
	if got[0][2] != craftRepExperiment || got[0][3] != craftExpDiscovered {
		t.Fatalf("an Experiment of hide and pattern: % X", got[0][:4])
	}
	if f.stack(plain) != 1 || f.stack(hide) != 0 || f.stack(pattern) != 0 {
		t.Fatalf("after the Experiment: gloves %d, hides %d, patterns %d", f.stack(plain), f.stack(hide), f.stack(pattern))
	}

	// the controls: an armorsmith's hammer (wear 23) makes no gloves; the Novice kit (level 1, tier 1) is too weak
	for _, c := range []struct {
		name      string
		wear, lvl int
		line      string
	}{
		{"Adept Armorsmith Hammer", wearCraftTool0 + craftArmor, 21, "You need your Tailoring tool equipped."},
		{"Novice Tailor's Kit", wearCraftTool0 + craftTailoring, 1, "Your Novice Tailor's Kit is not good enough for that."},
	} {
		g, _, gHide, _, _, gTunare, _ := setup(c.name, c.wear, c.lvl)
		got := g.create(tunareRecipe)
		mustOps(t, got, opcodeCraftReply, protocol.OpcodeColoredChat)
		mustBytes(t, c.name, got[1], redLine(c.line))
		if g.stack(gHide) != 3 || g.stack(gTunare) != 0 {
			t.Fatalf("%s: a refusal took the hide or made the gloves", c.name)
		}
	}
}
