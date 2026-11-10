package handlers

import (
	"strings"
	"testing"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
)

// An Armor Dyer: talked to, it asks what to dye (one piece of armour, and
// the robe, shield or a weapon, a question further), a family of colours and
// a colour, and dyes it at once in chardyes and on the character; "All my
// armor" is the six armour slots and the robe, "No dye" one slot's dye off,
// "Take every dye off" every slot; an answered box answered again does
// nothing; another NPC is no dyer.
func TestTheArmorDyer(t *testing.T) {
	tw := newTalkWorld(t, 100, 100)
	db, err := database.NewSQLite(t.TempDir() + "/dyer.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	rec := &database.CharacterRecord{CharName: "Talker", AccountID: 1, Level: 10}
	if err := database.NewSQLiteCharacterStore(db).Create(rec); err != nil {
		t.Fatal(err)
	}
	tw.p.CharServerID = rec.ServerID
	d := NewDyer(tw.w, db)
	dyer := &entity.NPC{ServerID: 3101, Name: "Armor Dyer", NPCType: 0x80}
	questMsgs(tw.s)

	talk := func() []string {
		t.Helper()
		if !d.Talk(tw.s, tw.p, dyer) {
			t.Fatal("the dyer did not answer")
		}
		return questMsgs(tw.s)
	}
	pick := func(msgs []string, n byte) []string {
		t.Helper()
		id := boxID(msgs)
		if id == 0 || !IsDyerBox(id) {
			t.Fatalf("no dyer's question in %q", msgs)
		}
		if !d.Answer(tw.s, tw.p, id, n) {
			t.Fatalf("box %08X not the dyer's", id)
		}
		return questMsgs(tw.s)
	}
	dyes := func() [database.DyeSlots]uint32 {
		t.Helper()
		c, err := db.CharDyes(rec.ServerID)
		if err != nil {
			t.Fatal(err)
		}
		return c
	}

	got := talk()
	if !hasMsg(got, "ask#*") || !strings.Contains(got[len(got)-1], ":What shall I dye?|All my armor|One piece of armor|Robe, shield or weapons|Take every dye off") {
		t.Fatalf("the dyer's question: %q", got)
	}
	got = pick(got, 1) // One piece of armor
	if !strings.Contains(got[len(got)-1], ":Which one?|Chest|Bracers|Gloves|Legs|Boots|Helm") {
		t.Fatalf("the pieces: %q", got)
	}
	got = pick(got, 0) // Chest
	if !strings.Contains(got[len(got)-1], ":Which colors for chest?|Whites, greys and blacks|Reds and pinks|") {
		t.Fatalf("the families: %q", got)
	}
	got = pick(got, 1) // Reds and pinks
	if !strings.Contains(got[len(got)-1], ":Which color for chest?|Red|Crimson|Maroon|Coral|Rose|Pink") {
		t.Fatalf("the reds: %q", got)
	}
	colours := got
	got = pick(got, 0) // Red
	if c := dyes(); c[database.DyeChest] != 0xD10000FF || c[database.DyeLegs] != 0 {
		t.Errorf("chest red: %08X, legs %08X", c[database.DyeChest], c[database.DyeLegs])
	}
	if !hasMsg(got, "box:Armor Dyer: There: red.") {
		t.Errorf("said: %q", got)
	}
	if _, _, d2, ok := tw.w.DyeTarget(tw.s); !ok || d2[database.DyeChest] != 0xD10000FF {
		t.Errorf("on the character: %v %08X", ok, d2[database.DyeChest])
	}
	if !d.Answer(tw.s, tw.p, boxID(colours), 1) || dyes()[database.DyeChest] != 0xD10000FF {
		t.Errorf("an answered box changed the chest to %08X", dyes()[database.DyeChest])
	}

	pick(pick(pick(talk(), 0), 4), 5) // All my armor, Blues and teals, Navy
	navy := database.RGBA(database.DiscTints[4])
	c := dyes()
	for _, s := range []int{database.DyeChest, database.DyeBracer, database.DyeGloves, database.DyeLegs, database.DyeBoots, database.DyeHelm, database.DyeRobe} {
		if c[s] != navy {
			t.Errorf("all armor navy: %s %08X", database.DyeSlotNames[s], c[s])
		}
	}
	if c[database.DyeShield] != 0 || c[database.DyePrimary] != 0 {
		t.Errorf("all armor reached the shield %08X / primary %08X", c[database.DyeShield], c[database.DyePrimary])
	}

	// the shield violet, then its dye off (the first family's "No dye")
	pick(pick(pick(pick(talk(), 2), 1), 5), 1) // Robe, shield or weapons; Shield; Purples; Violet
	if c := dyes(); c[database.DyeShield] != 0x8A2BE2FF || c[database.DyeChest] != navy {
		t.Errorf("the shield violet: %08X (chest %08X)", c[database.DyeShield], c[database.DyeChest])
	}
	got = pick(pick(pick(talk(), 2), 1), 0) // ...; Shield; Whites, greys and blacks
	if !strings.Contains(got[len(got)-1], "|No dye") {
		t.Fatalf("no dye among the whites: %q", got)
	}
	pick(got, byte(len(dyerShades(0))-1))
	if c := dyes(); c[database.DyeShield] != 0 || c[database.DyeChest] != navy {
		t.Errorf("the shield's dye off: %08X (chest %08X)", c[database.DyeShield], c[database.DyeChest])
	}

	pick(talk(), 3) // Take every dye off
	if c := dyes(); c != ([database.DyeSlots]uint32{}) {
		t.Errorf("every dye off: %08X", c)
	}
	if d.Talk(tw.s, tw.p, &entity.NPC{ServerID: 3102, Name: "Armor Merchant", NPCType: 0x81}) {
		t.Error("an Armor Merchant answered as a dyer")
	}
}

// Every question the dyer can ask fits CLIENT's box: six answers at most of
// 63 characters, a question of 127 (the box cut the rest until 2026-11-06:
// players saw six of twelve choices and six of twenty-one colours); and it
// offers more colours than it did (the user: "lets add more armor dye
// colors"), each its own.
func TestTheArmorDyersQuestionsFitTheBox(t *testing.T) {
	fits := func(what string, answers []string) {
		t.Helper()
		if len(answers) == 0 || len(answers) > maxDialogOptions {
			t.Errorf("%s: %d answers", what, len(answers))
		}
		for _, a := range answers {
			if len([]rune(a)) > 63 {
				t.Errorf("%s: %q is long", what, a)
			}
		}
	}
	for _, menu := range [][]dyerChoice{dyerChoices, dyerChoicesCapes} { // without capes and with (capes.go)
		fits("what", choiceLabels(menu))
		for _, c := range menu {
			if c.sub != nil {
				fits(c.label, choiceLabels(c.sub))
			}
		}
	}
	var families []string
	seen := map[string]bool{}
	colours := 0
	for i, f := range database.DyeFamilies {
		families = append(families, f.Label)
		var shades []string
		for _, c := range dyerShades(i) {
			shades = append(shades, c.Label)
			if c.RGBA != 0 {
				colours++
				if seen[c.Label] {
					t.Errorf("%s twice", c.Label)
				}
				seen[c.Label] = true
			}
		}
		fits(f.Label, shades)
	}
	fits("families", families)
	if colours < 30 {
		t.Errorf("%d colours: the dyer offered 20 before", colours)
	}
	for _, q := range []string{"What shall I dye?", "Which one?", "Which colors for secondary weapon?", "Which color for secondary weapon?"} {
		if len([]rune(q)) > questAskChars {
			t.Errorf("%q is past the box's %d", q, questAskChars)
		}
	}
}
