package handlers

import (
	"testing"

	"github.com/eqoa/server/internal/entity"
)

// /merc pet (mercpetkind.go; petition 1564: "pick which mag pet he will
// use"): a magician merc hired summons its usual earth line (no pick:
// 2026-11-12); /merc pet names the kinds; /merc pet air keeps "air" in its
// row and its kit picks the air elementals (the Djinn too); best keeps
// "best", the best of all. The controls: a word of no kind changes
// nothing, and a class with one kind or none has no choice.
func TestMercPetCommand(t *testing.T) {
	r, m, db := newMercFixture(t)
	for _, s := range []struct {
		id, level int
		name      string
	}{{632, 1, "Earth Elementaling"}, {648, 12, "Air Elementaling"}} {
		if _, err := db.Exec(`INSERT INTO spellpattern (spellid, abilitylvl, unk2, unk3, range, casttime, power, iconColor, icon, scope,
			recast, eqprequire, spellname, spelldesc) VALUES (?, ?, 0, 0, 16880, 3, 100, 0, 0, 0, 10, 0, ?, '')`, s.id, s.level, s.name); err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec(`INSERT INTO spelleffect (spellid, line, fxdict, source) VALUES (?, ?, 0, 'test')`, s.id,
			"Summons an "+s.name+" to aid you"); err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec(`INSERT INTO classspells (class, spellid, source) VALUES (10, ?, 'eqoa-data')`, s.id); err != nil {
			t.Fatal(err)
		}
	}
	m.hire(r.anna, r.pa, 10, 0, 0)
	row, ok, _ := db.Merc(r.pa.CharServerID, 1)
	if !ok || row.Pet != "" {
		t.Fatalf("the hire's row: %v pet %q", ok, row.Pet)
	}
	if k, _ := m.kit(row, 30); !entity.MercPetKindOf("Earth Elementaling", k.PetPick) || entity.MercPetKindOf("Air Elementaling", k.PetPick) {
		t.Errorf("no pick, its usual earth: %q", k.PetPick)
	}
	r.drain()
	r.say(r.anna, "/merc pet")
	if got := r.msgs(r.anna); !saidLine(got, " summons: Earth (its usual). Pets of a Magician: Earth, Water, Fire, Air, Efreet, Ghilan, or best.") {
		t.Errorf("/merc pet: %q", got)
	}
	r.say(r.anna, "/merc pet frog")
	if row, _, _ := db.Merc(r.pa.CharServerID, 1); row.Pet != "" {
		t.Errorf("a word of no kind set %q", row.Pet)
	}
	r.drain()
	r.say(r.anna, "/merc pet Air")
	if got := r.msgs(r.anna); !saidLine(got, " will summon Air.") {
		t.Errorf("/merc pet air: %q", got)
	}
	row, _, _ = db.Merc(r.pa.CharServerID, 1)
	if row.Pet != "air" {
		t.Fatalf("the pick kept: %q", row.Pet)
	}
	k, _ := m.kit(row, 30)
	if !entity.MercPetKindOf("Air Elementaling", k.PetPick) || !entity.MercPetKindOf("Summon Djinn", k.PetPick) ||
		entity.MercPetKindOf("Earth Elementaling", k.PetPick) {
		t.Errorf("the kit's pick: %q", k.PetPick)
	}
	r.say(r.anna, "/merc pet best")
	row, _, _ = db.Merc(r.pa.CharServerID, 1)
	if row.Pet != "best" {
		t.Errorf("best kept %q", row.Pet)
	}
	if k, _ := m.kit(row, 30); len(k.PetPick) != 0 {
		t.Errorf("best: a pick %q", k.PetPick)
	}
	// one kind: no choice
	if _, ok := mercPetKindNamed(13, "familiar"); ok {
		t.Error("a wizard has a choice of pet")
	}
	if k, ok := mercPetKindNamed(11, "BONES"); !ok || k.name != "Bones" {
		t.Errorf("bones: %v %v", k, ok)
	}
}
