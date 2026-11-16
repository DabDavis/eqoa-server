package handlers

import (
	"strings"
	"testing"
	"time"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
)

// A Pet Groomer (petgroomer.go): with no pet out it asks for one; with a pet
// of a body of one area it dyes its whole body; once the body has two areas
// (petdyeareas) it asks for each, both, or every dye off, then a family of the
// dyers' colours and a colour, and dyes it at once in petdyes (by the body)
// and on the character; an answered box answered again does nothing; the
// Equipment Dyer is no groomer and a groomer no dyer.
func TestThePetGroomer(t *testing.T) {
	f := newGenieFixture(t)
	f.p.Class, f.p.World, f.p.Power, f.p.MaxPower = 10, 0, 2000, 2000
	const look = 0x8A416241
	body := &entity.NPC{ServerID: 9001, Name: "Fire Elemental", Level: 30, HP: 900, MaxHP: 900, ModelID: look,
		Size: 1, NPCType: 0x80, X: 5000, Z: 5000}
	f.w.NPCs[body.ServerID] = body
	g := NewGroomer(f.w, f.db)
	groomer := &entity.NPC{ServerID: 3102, Name: "Pet Groomer", NPCType: 0x80}
	if IsArmorDyer(groomer.Name) || IsPetGroomer(EquipmentDyerName) || !IsPetGroomer(" pet groomer ") {
		t.Fatal("a groomer is a dyer, or a dyer a groomer")
	}
	questMsgs(f.sess)
	talk := func() []string {
		t.Helper()
		if !g.Talk(f.sess, f.p, groomer) {
			t.Fatal("the groomer did not answer")
		}
		return questMsgs(f.sess)
	}
	pick := func(msgs []string, n byte) []string {
		t.Helper()
		id := boxID(msgs)
		if id == 0 || !IsGroomerBox(id) || IsDyerBox(id) {
			t.Fatalf("no groomer's question in %q", msgs)
		}
		if !g.Answer(f.sess, f.p, id, n) {
			t.Fatalf("box %08X not the groomer's", id)
		}
		return questMsgs(f.sess)
	}
	dyes := func() [database.PetDyeAreas]uint32 {
		t.Helper()
		d, err := f.db.PetDyesOf(f.p.CharServerID)
		if err != nil {
			t.Fatal(err)
		}
		return d[look]
	}

	if got := talk(); boxID(got) != 0 || !hasMsg(got, "box:Pet Groomer: I groom and dye pets*") {
		t.Fatalf("no pet out: %q", got)
	}
	elemental := entity.CastSpell{SpellID: 717, Level: 30, Name: "Fire Elemental", Def: entity.ParseSpellText("Summons a Fire Elemental to aid you")}
	if got := f.w.SummonByItem(f.sess, elemental, f.now, time.Time{}); got != entity.ItemSummoned {
		t.Fatalf("no pet: %v", got)
	}
	got := talk()
	if !strings.HasSuffix(got[len(got)-1], ":What shall I dye on your fire elemental?|Its whole body|Remove its dye") {
		t.Fatalf("one area: %q", got)
	}
	got = pick(pick(got, 0), 1) // its whole body; reds and pinks
	got = pick(got, 0)          // red
	if d := dyes(); d != [2]uint32{0xD10000FF, 0} {
		t.Fatalf("whole body red: %08X", d)
	}
	if !hasMsg(got, "box:Pet Groomer: There: red.") {
		t.Errorf("said: %q", got)
	}
	if pet, ok := f.w.PetLookOf(f.sess); !ok || pet.Dye != (entity.PetDye{0xD10000FF, 0}) || pet.Look != look {
		t.Errorf("on the character: %+v %v", pet, ok)
	}

	f.w.SetPetDyeAreas(map[uint32]database.PetDyeArea{look: {Look: look, Area1: "Flames", Area2: "Core", Mask: 0b10}})
	got = talk()
	if !strings.HasSuffix(got[len(got)-1], ":What shall I dye on your fire elemental?|Flames|Core|Both areas|Remove all dyes") {
		t.Fatalf("two areas: %q", got)
	}
	colours := pick(pick(got, 1), 4) // Core; blues and teals
	pick(colours, 5)                 // navy
	navy := database.RGBA(database.DiscTints[4])
	if d := dyes(); d != [2]uint32{0xD10000FF, navy} {
		t.Fatalf("the core navy: %08X", d)
	}
	if !g.Answer(f.sess, f.p, boxID(colours), 0) || dyes() != [2]uint32{0xD10000FF, navy} {
		t.Errorf("an answered box changed the dyes to %08X", dyes())
	}
	pick(pick(pick(talk(), 2), 5), 1) // both; purples; violet
	if d := dyes(); d != [2]uint32{0x8A2BE2FF, 0x8A2BE2FF} {
		t.Fatalf("both violet: %08X", d)
	}
	pick(talk(), 3) // remove all dyes
	if d := dyes(); d != [2]uint32{} {
		t.Fatalf("every dye off: %08X", d)
	}
	if pet, _ := f.w.PetLookOf(f.sess); pet.Dye != (entity.PetDye{}) {
		t.Errorf("the dyes off on the character: %08X", pet.Dye)
	}
	if g.Talk(f.sess, f.p, &entity.NPC{ServerID: 3103, Name: EquipmentDyerName}) {
		t.Error("the Equipment Dyer answered as a groomer")
	}
}
