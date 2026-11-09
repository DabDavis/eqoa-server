package entity

import (
	"fmt"
	"testing"
	"time"

	"github.com/eqoa/server/internal/database"
)

// The Hydromancer's Ghilan Noble (the magician Waldorf's petition 1395: "Ghilan
// Noble for Hydromance MCA should get an added undead nuke against undead
// creatures. Hit around 1500 I believe."), as petfix180_20261109.py leaves its
// workbook row: its own kit, and Purifying Tide ("Target undead, Decrease
// Hitpoints by 1500") every 30 s from 8 s into a fight. A spell for undead alone
// goes at an undead creature alone (npcundead, else its name), as a player's
// is refused at the living: a pet does not cast it at a living foe (it waits,
// due, for an undead one), it lands on nothing that is not undead, and an area
// of it spares the living. The visuals here are made up; the rest is the
// script's and the live row's.

const (
	fxGhilanTide  uint32 = 0x0B200001
	fxGhilanNuke  uint32 = 0x0B200002
	fxGhilanHeal  uint32 = 0x0B200003
	fxGhilanBless uint32 = 0x0B200004
)

// ghilanKit is the row's abilities with the tide added after its own three:
// Water Heal Level 55 (its owner or itself below 60%), Ghilan's Blessing on its
// owner, Water Nuke Level 55 every 12 s.
const ghilanKit = `[{"every":30.0,"first":1.0,"kind":"heal","spell_id":902165,"target":"either","below":60},` +
	`{"every":1800.0,"first":3.0,"kind":"buff","spell_id":902167,"target":"owner"},` +
	`{"every":12.0,"first":5.0,"kind":"nuke","spell_id":902166},` +
	`{"every":30.0,"first":8.0,"kind":"nuke","spell_id":905130}]`

var summonGhilanNoble = summonSpell(701088, 60, "Summon Ghilan Noble", "Summons a Ghilan Noble to aid you")

// ghilanRig is the pet rig with the Ghilan Noble's spells and its live row
// (701088, the workbook's pet 003E) with the kit, and a level 60 magician.
func ghilanRig(t *testing.T) *petRig {
	t.Helper()
	r, _ := sheetRig(t,
		`INSERT INTO spellpattern (spellid, abilitylvl, unk2, unk3, range, casttime, power, iconColor, icon, scope, recast,
			eqprequire, spellname, spelldesc) VALUES
			(902165, 55, 0, 0, 16880, 3, 220, 0, 0, 1, 30, -1, 'Water Heal Level 55', ''),
			(902166, 55, 0, 0, 16880, 2, 220, 0, 0, 1, 12, -1, 'Water Nuke Level 55', ''),
			(902167, 1, 0, 0, 16528, 3, 0, 0, 0, 1, 1800, -1, 'Ghilan''s Blessing', ''),
			(905130, 60, 0, 0, 16880, 2, 0, 0, 0, 1, 30, -1, 'Purifying Tide', '')`,
		fmt.Sprintf(`INSERT INTO spelleffect (spellid, line, fxdict, source) VALUES
			(902165, 'Target, Increase Hitpoints by 1925 ; Increase 25%% based on CHA', %d, 'test'),
			(902166, 'Target, Decrease Hitpoints by 440 ; Increase 25%% based on WIS', %d, 'test'),
			(902167, 'Target, Increase WIS by 50, Increase CR by 50, Increase AR by 50, Increase HP per tic by 130, 30min', %d, 'test'),
			(905130, 'Target undead, Decrease Hitpoints by 1500', %d, 'test')`, fxGhilanHeal, fxGhilanNuke, fxGhilanBless, fxGhilanTide),
		`INSERT INTO petstats (spellid, sheet_pet_id, name, model, level, type, element, ac, str, sta, agi, dex, wis,
			intelligence, cha, hp, power, hot, pot, ar, cr, lr, fr, pr, dr, str_max, sta_max, agi_max, dex_max, wis_max,
			int_max, cha_max, hp_factor, off_mod, def_mod, wpn1_dmg, wpn1_type, abilities, size, source)
			VALUES (701088, '003E', 'Ghilan Noble', 2430398935, 60, 'priest', 'water', 2200, 435, 490, 380, 270, 600, 270,
			545, 4650, 6338, 103, 140, 67, 100, 67, 67, 67, 67, 600, 600, 600, 600, 600, 600, 600, 40, 0, 4, 385, 'blunt',
			'`+ghilanKit+`', 2.0, 'test')`)
	r.p.Class, r.p.Level = 10, 60
	return r
}

// Fought undead (the crab tagged so, npcundead's way), the Ghilan Noble casts
// Purifying Tide at 18.5 s (its 8 s clock waiting out the blessing's and its
// water nuke's casts) and every 30 s after, a 2 s cast, landing 1500 each; the
// control, the same crab living, never has it cast at it nor landed on it, and
// the rest of its kit goes as it does undead (the water nuke at the same times).
func TestTheGhilanNoblesTideIsForTheUndead(t *testing.T) {
	fight := func(undead bool) *swarmFight {
		r := ghilanRig(t)
		if undead {
			r.w.SetUndeadTags(map[string]bool{"desert crab": true})
		}
		f := swarmFightFor(t, r, summonGhilanNoble, 4650, 110*time.Second)
		if ps := r.w.pets[r.p.ServerID]; ps == nil || f.pet.sheet == nil || len(ps.petAbilities()) != 4 {
			t.Fatalf("no Ghilan Noble of its row with four abilities")
		}
		return f
	}
	dead, living := fight(true), fight(false)
	at, ms := dead.of(fxGhilanTide)
	if fmt.Sprint(at) != fmt.Sprint(steps(18500, 30000, 9700+110000)) {
		t.Errorf("the tide cast at an undead crab at %v, want %v", at, steps(18500, 30000, 9700+110000))
	}
	for _, m := range ms {
		if m != 2000 {
			t.Errorf("the tide's 0x00B1 says a %d ms cast, want 2000", m)
			break
		}
	}
	const landed = `lands "Purifying Tide" on Desert Crab for 1500 before`
	if n := dead.logged(landed); n != len(at) || n == 0 {
		t.Errorf("%d tides landed on the undead crab of %d cast", n, len(at))
	}
	if at, _ := living.of(fxGhilanTide); len(at) != 0 || living.logged("Purifying Tide") != 0 {
		t.Errorf("the control: the tide cast at the living crab at %v, %d lines of it", at, living.logged("Purifying Tide"))
	}
	deadNukes, _ := dead.of(fxGhilanNuke)
	livingNukes, _ := living.of(fxGhilanNuke)
	if len(deadNukes) < 8 || fmt.Sprint(deadNukes) != fmt.Sprint(livingNukes) {
		t.Errorf("its water nuke at %v undead and %v living: want the same, every 12 s", deadNukes, livingNukes)
	}
	if b, _ := living.of(fxGhilanBless); len(b) != 1 {
		t.Errorf("its blessing on its owner %d times, want once", len(b))
	}
}

// A spell for undead alone is not cast at the living (petCastLocked: a chained
// one is not either), lands on nothing a GM untagged in its cast, and an area of
// it (Annihilate Undead's form) lands on the undead round its target, a living
// creature among them spared.
func TestAPetsUndeadSpellGoesAtTheUndeadAlone(t *testing.T) {
	r := ghilanRig(t)
	pet, _ := r.summon(t, summonGhilanNoble, 0)
	ps := r.w.pets[r.p.ServerID]
	crab := r.enemy
	crab.HP, crab.MaxHP = 1_000_000, 1_000_000
	ps.target = crab.ServerID
	tide := &ps.petAbilities()[3]
	r.w.mu.Lock()
	cast := r.w.petCastLocked(ps, 3, crab, r.at(20*time.Second))
	wanted := r.w.petWantsLocked(ps, tide, r.at(20*time.Second))
	r.w.mu.Unlock()
	if cast || wanted || len(r.w.petCasts) != 0 {
		t.Fatalf("at a living crab: cast %v, wanted %v, %d casts on their way; want none", cast, wanted, len(r.w.petCasts))
	}
	r.w.SetUndeadTags(map[string]bool{"desert crab": true})
	r.w.mu.Lock()
	cast = r.w.petCastLocked(ps, 3, crab, r.at(30*time.Second))
	wanted = r.w.petWantsLocked(ps, tide, r.at(30*time.Second))
	r.w.mu.Unlock()
	if !cast || !wanted {
		t.Fatalf("at an undead crab: cast %v, wanted %v", cast, wanted)
	}
	r.w.SetUndeadTags(nil) // a GM's word in its cast: living again
	r.w.mu.Lock()
	r.w.petCastSystemLocked(r.at(34 * time.Second))
	r.w.mu.Unlock()
	if crab.HP != 1_000_000 {
		t.Errorf("the tide landed %d on a crab no longer undead", 1_000_000-crab.HP)
	}

	mk := func(id EntityID, name string, dx float32) *NPC {
		n := &NPC{ServerID: id, Name: name, Level: 40, HP: 5000, MaxHP: 5000, X: pet.X + 3 + dx, Y: pet.Y, Z: pet.Z, World: pet.World}
		r.w.NPCs[id] = n
		r.w.SpatialIndex.Insert(id, n.X, n.Z)
		return n
	}
	target, bones, living := mk(7201, "a decaying skeleton", 0), mk(7202, "a decaying skeleton", 6), mk(7203, "Test Crab", -6)
	sp := SpellOfRow(&database.SpellPatternRecord{SpellID: 9701, AbilityLvl: 60, Range: 16880, CastTime: 2, Scope: 1,
		SpellName: "Test Undoing"}, 0, "Target undead, Decrease Hitpoints by 100 aoe Decrease Hitpoints by 40", "", 0)
	if !sp.Def.undeadOnly() || sp.Area.Centre != AreaOnTarget {
		t.Fatalf("the test spell: undead alone %v, area %v", sp.Def.undeadOnly(), sp.Area.Centre)
	}
	a := &templateAbility{TemplateAbility: database.TemplateAbility{Kind: database.AbilityNuke}}
	r.w.mu.Lock()
	r.w.petLandLocked(&petCast{ps: ps, tmpl: ps.tmpl, a: a, spell: &sp, target: target.ServerID}, r.at(40*time.Second))
	r.w.mu.Unlock()
	if 5000-target.HP <= 5000-bones.HP || bones.HP >= 5000 || living.HP != 5000 {
		t.Errorf("took: the target %d, the skeleton beside it %d, the living crab %d; want the target most, the skeleton some, "+
			"the crab nothing", 5000-target.HP, 5000-bones.HP, 5000-living.HP)
	}
}
