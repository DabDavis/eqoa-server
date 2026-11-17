package entity

import (
	"testing"
	"time"
)

// The Tinker's Construct as tools/construct_quad_20261117.py makes it, in the
// pet rig: a workbook row (made-up numbers in the workbook's shape) with a
// taunt and Tinker Kick (first 0, every 12: its recast) and arcane resist at
// the cap, its pettemplate row "construct" on "pet construct" with swing 5.
const (
	constructKickSpell = `INSERT INTO spellpattern (spellid, abilitylvl, unk2, unk3, range, casttime, power, iconColor, icon, scope,
		recast, eqprequire, spellname, spelldesc) VALUES (902052, 51, 0, 0, 16544, 0, 200, 0, 0, 1, 12, -1, 'Tinker Kick',
		'A blunt strike at the target.')`
	constructKickLine = `INSERT INTO spelleffect (spellid, line, fxdict, source)
		VALUES (902052, 'Target, Decrease Hitpoints by 200', 4080256363, 'test')`
	constructRow = `INSERT INTO petstats (spellid, sheet_pet_id, name, model, level, type, element, ac, str, sta, agi, dex,
		hp, power, ar, cr, lr, fr, pr, dr, str_max, off_mod, def_mod, wpn1_dmg, wpn1_type, wpn2_dmg, wpn2_type, abilities, source)
		VALUES (901835, '0008', 'Construct', 2633457281, 51, 'tank', 'arcane', 2550, 458, 509, 407, 305, 6184, 5191,
		315, 56, 56, 56, 76, 76, 560, 2, 3, 459, 'blunt', 408, 'slash',
		'[{"every":12.0,"first":1.0,"kind":"taunt","hits":[[1,0,0]]},{"every":12.0,"first":0.0,"kind":"nuke","spell_id":902052}]', 'eqoaemu')`
	constructTemplate = `INSERT INTO classtemplates (name, class, hp_pct, taken_pct, melee_pct, abilities, source)
		VALUES ('pet construct', 0, 100, 100, 100, '[]', 'constructquad1117')`
	constructPetTemplate = `INSERT INTO pettemplate (spell, weapon, template, swing_class, swing, round_ms, source)
		VALUES ('construct', 0, 'pet construct', -1, 5, 0, 'constructquad1117')`
)

var constructSpell = summonSpell(901835, 60, "Construct", "Summons a Construct to aid you")

// Four swings a 6 s round (Attack4), each one of its hits, for an owner of any
// class: an enchanter's pets swing once a round without the template row.
func TestTheConstructQuads(t *testing.T) {
	for _, class := range []int{classEnchanter, 9, 1} {
		r, _ := sheetRig(t, constructKickSpell, constructKickLine, constructRow, constructTemplate, constructPetTemplate)
		r.p.Level = 60
		f := petTemplateFight(t, r, constructSpell, class, 24*time.Second, false)
		if k := npcSwingKind(f.pet); k != 5 {
			t.Fatalf("owner class %d: swing %d, want Attack4 (5)", class, k)
		}
		if length, _, _, hits := npcRound(f.pet); length != 6*time.Second || len(hits) != 4 {
			t.Errorf("owner class %d: a %v round of %d hits, want 6 s and four", class, length, len(hits))
		}
		attack4 := HashResourceID("Attack4")
		rounds := f.cast(attack4)
		// the kick's damage lands too: count the melee as the rounds' swings
		if rounds != 4 && rounds != 5 {
			t.Errorf("owner class %d: %d rounds in 24 s, want 4 (a 6 s round)", class, rounds)
		}
		if len(f.crabHit) < 4*4 {
			t.Errorf("owner class %d: %d hits on the crab in 24 s, want at least four a round", class, len(f.crabHit))
		}
	}
	// the control: the template row gone, an enchanter's construct swings once
	r, db := sheetRig(t, constructKickSpell, constructKickLine, constructRow, constructTemplate, constructPetTemplate)
	if _, err := db.Exec(`DELETE FROM pettemplate WHERE spell = 'construct'`); err != nil {
		t.Fatal(err)
	}
	if _, err := r.w.LoadClassTemplates(db); err != nil {
		t.Fatal(err)
	}
	r.p.Level = 60
	f := petTemplateFight(t, r, constructSpell, classEnchanter, time.Second, false)
	if k := npcSwingKind(f.pet); k != 0 {
		t.Errorf("no template row, an enchanter's construct: swing %d, want 0 (once a round)", k)
	}
}

// Tinker Kick opens the fight and comes again every time its 12 s cooldown is
// up while it fights.
func TestTheConstructKicksOnItsCooldown(t *testing.T) {
	r, _ := sheetRig(t, constructKickSpell, constructKickLine, constructRow, constructTemplate, constructPetTemplate)
	r.p.Level = 60
	f := petTemplateFight(t, r, constructSpell, 9, 40*time.Second, false)
	var at []int
	for _, c := range f.casts {
		if c.fx == 4080256363 {
			at = append(at, c.at)
		}
	}
	if len(at) != 4 {
		t.Fatalf("kicks at %v ms: want four in 40 s (0, 12, 24, 36 s into the fight)", at)
	}
	for i := 1; i < len(at); i++ {
		if gap := at[i] - at[i-1]; gap != 12000 {
			t.Errorf("kicks at %v ms: a gap of %d ms, want its 12 s recast", at, gap)
		}
	}
	if at[0] > 9700+500 {
		t.Errorf("its first kick at %d ms, the fight from 9700: want it at once", at[0])
	}
}

// Arcane resist at the cap: 315 (ResistCap, the most that counts), which takes
// what the cap takes of an arcane spell; no more counts past it.
func TestTheConstructsArcaneResistIsAtTheCap(t *testing.T) {
	r, _ := sheetRig(t, constructKickSpell, constructKickLine, constructRow, constructTemplate, constructPetTemplate)
	pet, _ := r.summon(t, constructSpell, 0)
	if ar := pet.petStat(StatAR); ar != ResistCap || ResistCap != playerResistMax {
		t.Fatalf("AR %d, want the cap %d", ar, ResistCap)
	}
	capped, _ := playerResistShareNow(ResistCap, nil)
	over, _ := playerResistShareNow(ResistCap+200, nil)
	below, _ := playerResistShareNow(95, nil)
	got := pet.petResistShare(StatAR)
	if got != capped || over != capped || !(below < capped) {
		t.Errorf("share at its AR %.3f, the cap's %.3f, past it %.3f, at the old 95 %.3f", got, capped, over, below)
	}
}
