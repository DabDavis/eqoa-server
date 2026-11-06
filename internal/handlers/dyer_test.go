package handlers

import (
	"strings"
	"testing"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
)

// An Armor Dyer: talked to, it asks what to dye, then the colour, and dyes
// it at once in chardyes and on the character; "All my armor" is the six
// armour slots and the robe, "Take every dye off" every slot; an answered box
// answered again does nothing; another NPC is no dyer.
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
	if !hasMsg(got, "ask#*") || !strings.Contains(got[len(got)-1], ":What shall I dye?|All my armor|Chest|Bracers|") {
		t.Fatalf("the dyer's question: %q", got)
	}
	got = pick(got, 1) // Chest
	if !strings.Contains(got[len(got)-1], ":Which color for chest?|White|Black|Grey|Silver|Red|") {
		t.Fatalf("the colours: %q", got)
	}
	colours := got
	got = pick(got, 4) // Red
	if c := dyes(); c[database.DyeChest] != 0xD10000FF || c[database.DyeLegs] != 0 {
		t.Errorf("chest red: %08X, legs %08X", c[database.DyeChest], c[database.DyeLegs])
	}
	if !hasMsg(got, "box:Armor Dyer: There: red.") {
		t.Errorf("said: %q", got)
	}
	if _, _, d2, ok := tw.w.DyeTarget(tw.s); !ok || d2[database.DyeChest] != 0xD10000FF {
		t.Errorf("on the character: %v %08X", ok, d2[database.DyeChest])
	}
	if !d.Answer(tw.s, tw.p, boxID(colours), 0) || dyes()[database.DyeChest] != 0xD10000FF {
		t.Errorf("an answered box changed the chest to %08X", dyes()[database.DyeChest])
	}

	pick(pick(talk(), 0), 15) // All my armor, Navy
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

	pick(talk(), byte(len(dyerSlots)-1)) // Take every dye off
	if c := dyes(); c != ([database.DyeSlots]uint32{}) {
		t.Errorf("every dye off: %08X", c)
	}
	if d.Talk(tw.s, tw.p, &entity.NPC{ServerID: 3102, Name: "Armor Merchant", NPCType: 0x81}) {
		t.Error("an Armor Merchant answered as a dyer")
	}
}
