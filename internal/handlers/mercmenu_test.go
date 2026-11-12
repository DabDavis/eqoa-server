package handlers

import (
	"slices"
	"strings"
	"testing"

	"github.com/eqoa/server/internal/entity"
)

// A talk to one's own merc opens its menu (mercmenu.go): its advice, its
// spells each on or off (a choice turns one off, kept in its row and its
// book), its stance, a healer's mode, its dismissal asked again. /merc menu
// opens the same (a PS2's □ on a pet sends nothing).
func TestMercMenu(t *testing.T) {
	r, m, db := newMercFixture(t)
	for _, sp := range []struct {
		id         int
		name, line string
	}{{700901, "Test Courage", "Target, Increase AC by 50, 30min"}, {700902, "Test Smite", "Target, Decrease Hitpoints by 90"},
		{700903, "Test Mend", "Target, Increase Hitpoints by 200"}} {
		if _, err := db.Exec(`INSERT INTO spellpattern (spellid, abilitylvl, unk2, unk3, range, casttime, power, iconColor, icon, scope,
			recast, eqprequire, spellname, spelldesc) VALUES (?, 30, 0, 0, 16880, 3, 100, 0, 0, 1, 10, 0, ?, '')`, sp.id, sp.name); err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec(`INSERT INTO spelleffect (spellid, line, fxdict, source) VALUES (?, ?, 0, 'test')`, sp.id, sp.line); err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec(`INSERT INTO classspells (class, spellid, source) VALUES (9, ?, 'eqoa-data')`, sp.id); err != nil {
			t.Fatal(err)
		}
	}
	m.hire(r.anna, r.pa, 9, 0, 0)
	r.gw.AskMercGroup(r.anna)
	rows, _ := db.Mercs(r.pa.CharServerID)
	name := rows[0].Name
	merc := r.gw.NPCByID(r.gw.MercOf(r.anna))
	if merc == nil {
		t.Fatal("no merc out")
	}
	r.drain()
	r.talk(r.anna, merc)
	got := r.msgs(r.anna)
	if len(got) != 1 || !strings.HasSuffix(got[0], "|Advice|Spells|Stance: assist|Be offensive|Dismiss") {
		t.Fatalf("the menu: %q", got)
	}
	adv := r.answer(got, "Advice")
	if !slices.ContainsFunc(adv, func(s string) bool { return strings.Contains(s, "a cleric out of power saves no one") }) {
		t.Errorf("advice: %q", adv)
	}
	r.say(r.anna, "/merc menu")
	got = r.answer(r.msgs(r.anna), "Spells")
	if len(got) != 1 || !strings.Contains(got[0], name+"'s spells (1 of ") {
		t.Fatalf("the spells' box: %q", got)
	}
	opts := strings.Split(got[0][strings.Index(got[0], "|")+1:], "|")
	first := strings.TrimSuffix(opts[0], ": on")
	if first == opts[0] {
		t.Fatalf("first spell %q not on", opts[0])
	}
	got = r.answer(got, opts[0])
	if !hasMsg(got, "chat:"+name+" leaves "+first+" alone.") || !slices.ContainsFunc(got, func(s string) bool { return strings.Contains(s, "|"+first+": off") }) {
		t.Errorf("turned off: %q", got)
	}
	row, _, _ := db.Merc(r.pa.CharServerID, 1)
	if len(row.SpellsOff) != 1 {
		t.Fatalf("its row: %v", row.SpellsOff)
	}
	spells, _ := r.gw.MercSpells(r.anna, 1)
	if i := slices.IndexFunc(spells, func(s entity.MercSpell) bool { return s.ID == row.SpellsOff[0] }); i < 0 || !spells[i].Off {
		t.Errorf("its kit: %+v", spells)
	}
	// stance
	r.talk(r.anna, merc)
	r.answer(r.answer(r.msgs(r.anna), "Stance: assist"), "Passive")
	if info, _ := m.menuMerc(r.anna, 1); info.Stance != 1 {
		t.Errorf("stance %v", info.Stance)
	}
	// mode
	r.talk(r.anna, merc)
	r.answer(r.msgs(r.anna), "Be offensive")
	if row, _, _ := db.Merc(r.pa.CharServerID, 1); !row.Offensive {
		t.Error("not offensive")
	}
	// dismiss: No, then Yes
	r.talk(r.anna, merc)
	r.answer(r.answer(r.msgs(r.anna), "Dismiss"), "No")
	if len(r.gw.MercInfosOf(r.anna)) != 1 {
		t.Fatal("dismissed on No")
	}
	r.talk(r.anna, merc)
	r.answer(r.answer(r.msgs(r.anna), "Dismiss"), "Yes, dismiss "+name)
	if len(r.gw.MercInfosOf(r.anna)) != 0 {
		t.Error("not dismissed")
	}
}

// A magician merc's menu has Pet: its usual earth, its kinds and Best; a
// choice is /merc pet's (kept in its row).
func TestMercMenuPet(t *testing.T) {
	r, m, db := newMercFixture(t)
	m.hire(r.anna, r.pa, 10, 0, 0)
	r.gw.AskMercGroup(r.anna)
	merc := r.gw.NPCByID(r.gw.MercOf(r.anna))
	r.drain()
	r.talk(r.anna, merc)
	got := r.msgs(r.anna)
	if len(got) != 1 || !strings.HasSuffix(got[0], "|Advice|Spells|Stance: assist|Pet: Earth (its usual)|Dismiss") {
		t.Fatalf("the menu: %q", got)
	}
	got = r.answer(got, "Pet: Earth (its usual)")
	if len(got) != 1 || !strings.HasSuffix(got[0], "|Earth|Water|Fire|Air|Best") {
		t.Fatalf("the pets' box: %q", got)
	}
	r.answer(got, "Air")
	if row, _, _ := db.Merc(r.pa.CharServerID, 1); row.Pet != "air" {
		t.Errorf("the pick kept: %q", row.Pet)
	}
}
