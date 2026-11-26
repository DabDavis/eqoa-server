package entity

import (
	"testing"
	"time"
)

// A template reload (every GM edit of a classtemplates or spell row through
// the panel reads them all again) builds new template objects. A creature
// whose template's content did not change fights on: its abilities' clocks,
// its summons called, its enrage, its last stand and its ward kept. One whose
// template did change starts its fight by the new one afresh (its clocks from
// first, its enrage's size back), as a new template always has.

const reloadBossAbilities = `[{"kind":"nuke","spell":"fire bolt","every":10,"first":2},` +
	`{"kind":"summon","below":25,"count":1,"npc":"Gnoll Pup","share":50},` +
	`{"kind":"enrage","below":10,"grow":200,"every":3,"rampage":50}]`

// reloadRig is a template rig with the Gnoll Guard a boss of reloadBossAbilities
// mid-fight: its nuke's clock at 7 s, its summon called, enraged at twice its
// size, stood its last stand and warded.
func reloadRig(t *testing.T) (*templateRig, *NPC) {
	t.Helper()
	r := newTemplateRig(t)
	for _, q := range []string{
		`INSERT INTO classtemplates (name, class, hp_pct, abilities) VALUES ('boss', 0, 200, '` + reloadBossAbilities + `')`,
		`INSERT INTO classtemplates (name, class, abilities) VALUES ('other', 13, '[{"kind":"nuke","every":10}]')`,
		`INSERT INTO npctemplate (name, template) VALUES ('gnoll mage', 'other')`,
	} {
		if _, err := r.db.Exec(q); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}
	r.assign("gnoll guard", "boss")
	guard := r.npc("Gnoll Guard")
	if guard.tmpl == nil || len(guard.tmpl.abilities) != 1 || len(guard.tmpl.summons) != 1 || guard.tmpl.enrage == nil {
		t.Fatalf("the boss's template: %+v", guard.tmpl)
	}
	p, _ := r.player("Tank", guard.X+1, guard.Z)
	r.fight(guard, p.ServerID)
	guard.HP = guard.MaxHP / 20
	f := guard.fight()
	f.begin(guard.tmpl, r.at(0))
	f.next[0] = r.at(7 * time.Second)
	f.summoned = 1
	f.adds = []EntityID{4242}
	f.enraged = &enrageOn{at: r.at(time.Second), size: guard.Size, next: r.at(4 * time.Second), e: guard.tmpl.enrage}
	guard.Size *= 2
	f.lastStand = &lastStandOn{}
	f.warded = &wardOn{}
	return r, guard
}

func TestAnUnchangedTemplatesReloadKeepsTheFight(t *testing.T) {
	r, guard := reloadRig(t)
	f, old, size, maxHP, hp := guard.tfight, guard.tmpl, guard.Size, guard.MaxHP, guard.HP
	for _, edit := range []string{
		`UPDATE classtemplates SET hp_pct = 300 WHERE name = 'other'`,        // another template's row
		`UPDATE classtemplates SET note = 'tuned 11-26' WHERE name = 'boss'`, // its own note alone
		`UPDATE spellpattern SET casttime = 3 WHERE spellid = 1632`,          // a spell row (its nuke's)
		`UPDATE classtemplates SET abilities = abilities WHERE name = 'boss'`,
	} {
		if _, err := r.db.Exec(edit); err != nil {
			t.Fatal(err)
		}
		if _, err := r.w.LoadClassTemplates(r.db); err != nil {
			t.Fatal(err)
		}
		if guard.tmpl == old {
			t.Fatalf("after %q: the old template object kept, want the reload's", edit)
		}
		if guard.tfight != f {
			t.Fatalf("after %q: the fight was ended", edit)
		}
		if !f.on || !f.next[0].Equal(r.at(7*time.Second)) || f.summoned != 1 || len(f.adds) != 1 ||
			f.enraged == nil || f.lastStand == nil || f.warded == nil {
			t.Fatalf("after %q: the fight's state lost: %+v", edit, f)
		}
		if guard.Size != size || guard.MaxHP != maxHP || guard.HP != hp {
			t.Fatalf("after %q: size %v HP %d/%d, want %v %d/%d", edit, guard.Size, guard.HP, guard.MaxHP, size, hp, maxHP)
		}
		old = guard.tmpl
	}
	// the reloaded template is the one its casts read: the spell edit is in it
	if s := guard.tmpl.abilities[0].pinned; s == nil || s.CastTime != 3*time.Second {
		t.Errorf("the reload's nuke: %+v, want Fire Bolt of a 3 s cast", s)
	}
}

func TestAChangedTemplatesReloadStartsTheFightAgain(t *testing.T) {
	for _, edit := range []string{
		`UPDATE classtemplates SET hp_pct = 300 WHERE name = 'boss'`,
		`UPDATE classtemplates SET abilities = '` +
			`[{"kind":"nuke","spell":"fire bolt","every":12,"first":2},` +
			`{"kind":"summon","below":25,"count":1,"npc":"Gnoll Pup","share":50},` +
			`{"kind":"enrage","below":10,"grow":200,"every":3,"rampage":50}]' WHERE name = 'boss'`,
	} {
		t.Run(edit[:40], func(t *testing.T) {
			r, guard := reloadRig(t)
			size := guard.tfight.enraged.size
			if _, err := r.db.Exec(edit); err != nil {
				t.Fatal(err)
			}
			if _, err := r.w.LoadClassTemplates(r.db); err != nil {
				t.Fatal(err)
			}
			if guard.tfight != nil {
				t.Fatalf("a changed template kept the old fight: %+v", guard.tfight)
			}
			if guard.Size != size {
				t.Errorf("size %v after the enrage's end, want %v", guard.Size, size)
			}
			// its next fight begins by the new template's clocks
			f := guard.fight()
			f.begin(guard.tmpl, r.at(time.Minute))
			if !f.next[0].Equal(r.at(time.Minute+2*time.Second)) || f.summoned != 0 || f.enraged != nil {
				t.Errorf("the new fight: %+v", f)
			}
		})
	}
}

// Taking a creature's template away still ends its fight (the control).
func TestATemplateTakenAwayEndsTheFight(t *testing.T) {
	r, guard := reloadRig(t)
	r.assign("gnoll guard", "")
	if guard.tmpl != nil || guard.tfight != nil {
		t.Errorf("template %v, fight %+v; want none of either", guard.tmpl, guard.tfight)
	}
}
