package entity

import (
	"math"
	"strings"
	"testing"
	"time"

	"github.com/eqoa/server/internal/database"
)

// A pet of a petstats row (petstats.go) in the pet rig (pettemplate_test.go):
// the workbook's numbers are the pet's, its spells pinned by id are what it
// casts, its template row's class and swing kept; and the control, the same
// summoning with the row gone, is the pet it always was. The numbers here
// are made up, the shape the workbook's.

// sheetFireRow is a petstats row for the rig's Fire Elemental (spell 717):
// the second-commonest body among the rig's Fire Elementals, two weapons,
// a nuke pinned by id (Lava Strike, the fixture's magician line) and a taunt
// of hate alone.
const sheetFireRow = `INSERT INTO petstats (spellid, sheet_pet_id, name, model, primary_model, level, type, element, ac,
	str, sta, agi, dex, wis, intelligence, cha, hp, power, hot, pot, ar, cr, lr, fr, pr, dr, str_max, hp_factor, off_mod,
	def_mod, wpn1_dmg, wpn1_type, wpn2_dmg, wpn2_type, abilities, source)
	VALUES (717, 'T001', 'Test Flame', 2319540799, 3749499871, 38, 'melee', 'fire', 1100,
	55, 60, 61, 70, 10, 11, 12, 2500, 3000, 50, 70, 20, 21, 22, 23, 24, 25, 400, 36, 3, 1,
	450, 'slash', 400, 'pierce',
	'[{"kind":"nuke","spell_id":700198,"every":6,"first":1},{"kind":"taunt","hits":[[1,0,0]],"every":12,"first":3}]', 'eqoaemu')`

func sheetRig(t *testing.T, rows ...string) (*petRig, *database.SQLiteDB) {
	t.Helper()
	r, db := newPetTemplateRig(t, true)
	for _, q := range rows {
		if _, err := db.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := r.w.LoadClassTemplates(db); err != nil {
		t.Fatal(err)
	}
	return r, db
}

func TestAWorkbookPetIsItsRow(t *testing.T) {
	r, db := sheetRig(t, sheetFireRow)
	pet, _ := r.summon(t, fireElemental(), 0)
	ps := r.w.pets[r.p.ServerID]
	if pet.sheet == nil || pet.sheet.SheetPetID != "T001" {
		t.Fatalf("the pet has no row: %+v", pet.sheet)
	}
	if pet.Name != "Test Flame" || pet.Level != 38 || uint32(pet.ModelID) != 0x8A41623F || pet.Size != 1 {
		t.Errorf("name %q, level %d, body %#x, size %.2f: want the row's", pet.Name, pet.Level, uint32(pet.ModelID), pet.Size)
	}
	if pet.MaxHP != 2500 || pet.HP != 2500 || pet.MaxPower != 3000 || pet.WeaponDamage != 450 {
		t.Errorf("HP %d/%d, power %d, weapon %d: want the row's 2500 (no balance), 3000, 450", pet.HP, pet.MaxHP, pet.MaxPower, pet.WeaponDamage)
	}
	if pet.PrimaryWeapon != 3749499871 || pet.PrimaryType != 1 || pet.SecondaryType != 5 {
		t.Errorf("hands %d (%d, %d): want the row's model, a slash and a pierce", pet.PrimaryWeapon, pet.PrimaryType, pet.SecondaryType)
	}
	// its template row's class (pet fire: a warrior's round, four hits from
	// 35), none of the template's hit points or melee share, its own spells
	if ps.tmpl == nil || pet.Class != 0 || npcSwingKind(pet) != 5 || ps.tmpl.tmpl.HPPct != 100 || ps.tmpl.tmpl.MeleePct != 100 {
		t.Fatalf("template %v, class %d, swing %d: want pet fire's class and swing at 100%%", ps.tmpl != nil, pet.Class, npcSwingKind(pet))
	}
	ab := ps.petAbilities()
	if len(ab) != 2 || ab[0].pinned == nil || ab[0].pinned.SpellID != 700198 || ab[1].Kind != database.AbilityTaunt {
		t.Fatalf("abilities %+v: want Lava Strike pinned and a taunt", ab)
	}
	if ps.stance != petDefensive || ps.kind != petKindOurs {
		t.Errorf("stance %d, kind %v: want defensive, ours", ps.stance, ps.kind)
	}

	// its armour and archetype against a creature's hits, its weapon in its own
	zero, top := func(int) int { return 0 }, func(n int) int { return n - 1 }
	plain := *pet
	plain.sheet = nil
	if got, was := npcHitOnPet(r.enemy, pet, top), npcHitOnPet(r.enemy, &plain, top); got >= was {
		t.Errorf("a crab's top hit on it %d, on the pet with no row %d: its 1100 AC takes off more", got, was)
	}
	pet.RoundHits = 1
	hit, ok := sheetMeleeHit(pet, r.enemy, zero)
	d, a := 450.0, offenceArchs[archMelee]
	low := int(math.Round(a.LowShare*(offLowD+offLowDEX*70)*d + a.Low - a.Defence*targetDefence(r.enemy)))
	floor := int(math.Floor(offFloorD*d + offFloorSTR*55))
	if !ok || hit != max(low, floor, 1) {
		t.Errorf("its least hit %d (%v): want a melee player's with the 450 weapon, %d", hit, ok, max(low, floor, 1))
	}
	pet.RoundHits = 2
	if off, _ := sheetMeleeHit(pet, r.enemy, zero); off >= hit {
		t.Errorf("its off hand's least %d: want under its main hand's %d (400 against 450)", off, hit)
	}
	if share := pet.petResistShare(StatFR); share < 0 {
		t.Errorf("resist share %v", share)
	}

	// the taunt is hate alone: the top of the crab's hate, no damage
	r.enemy.Threats = map[EntityID]int64{r.p.ServerID: 500}
	hp := r.enemy.HP
	r.w.mu.Lock()
	r.w.petTemplateTauntLocked(ps, &ab[1], r.enemy, r.at(10*time.Second))
	r.w.mu.Unlock()
	if r.enemy.HP != hp || r.enemy.CurrentEnemy != pet.ServerID || r.enemy.Threats[pet.ServerID] <= 500 {
		t.Errorf("after the taunt: HP %d (was %d), enemy %d, its hate %d", r.enemy.HP, hp, r.enemy.CurrentEnemy, r.enemy.Threats[pet.ServerID])
	}

	// its HoT a tick, scaled as its hit points are (here not at all)
	pet.HP = 100
	pet.petRegen()
	if pet.HP != 150 {
		t.Errorf("after a tick HP %d: want 100 + its 50", pet.HP)
	}

	// the control: the row gone, the pet as it always was
	if _, err := db.Exec(`DELETE FROM petstats`); err != nil {
		t.Fatal(err)
	}
	if _, err := r.w.LoadClassTemplates(db); err != nil {
		t.Fatal(err)
	}
	r.w.mu.Lock()
	r.w.dismissPetLocked(ps, r.sess, "")
	r.w.mu.Unlock()
	r.p.Power = r.p.MaxPower
	pet, _ = r.summon(t, fireElemental(), 20*time.Second)
	ps = r.w.pets[r.p.ServerID]
	want := max(1, int(math.Round(float64(NPCMaxHP(44))*91/100))) // pet fire's 91% of its level's
	if pet.sheet != nil || pet.Name != "Fire Elemental" || pet.Level != 44 || uint32(pet.ModelID) != 0x8A416241 || pet.MaxHP != want {
		t.Errorf("with no row: %q level %d body %#x HP %d: want Fire Elemental, 44, 0x8A416241, %d", pet.Name, pet.Level,
			uint32(pet.ModelID), pet.MaxHP, want)
	}
	if ps.tmpl == nil || ps.tmpl.tmpl.Name != "pet fire" || len(ps.petAbilities()) != 3 {
		t.Errorf("with no row its template is pet fire's own: %+v", ps.tmpl)
	}
}

// A body no NPC of this server wears is still put on (the disc has it); a
// level above the caster's is the caster's; with no template row a pet swings
// as pet.go has it (an enchanter's once a round).
func TestAWorkbookPetsBodyAndSwing(t *testing.T) {
	r, _ := sheetRig(t, `INSERT INTO petstats (spellid, sheet_pet_id, name, model, level, type, hp, wpn1_dmg, wpn1_type)
		VALUES (498, 'T002', 'Test Animation', 305419896, 99, 'tank', 44, 7, 'blunt')`,
		`DELETE FROM pettemplate WHERE spell = 'pendril''s animation'`)
	r.p.Class = classEnchanter
	pet, _ := r.summon(t, pendrils, 0)
	if uint32(pet.ModelID) != 0x12345678 || pet.Level != r.p.Level || pet.MaxHP != 44 {
		t.Errorf("body %#x, level %d, HP %d: want the row's body, the caster's level, 44", uint32(pet.ModelID), pet.Level, pet.MaxHP)
	}
	if !pet.SwingFixed || npcSwingKind(pet) != 0 || pet.PrimaryType != 3 || pet.SecondaryType != attackTypeEmptyHand {
		t.Errorf("swing %v %d, hands %d %d: want an enchanter pet's one hit, a blunt and an empty hand", pet.SwingFixed,
			npcSwingKind(pet), pet.PrimaryType, pet.SecondaryType)
	}
}

// A pet of a row takes its row's stances alone ("Passive": summoned passive,
// told so when another is asked); a non-combat pet fights nothing, told to or
// not.
func TestAWorkbookPetsStances(t *testing.T) {
	r, _ := sheetRig(t, `INSERT INTO petstats (spellid, sheet_pet_id, name, level, type, stances, hp)
		VALUES (717, 'T003', 'Test Gadget', 9, 'non-combat', 'passive', 222)`)
	pet, _ := r.summon(t, fireElemental(), 0)
	ps := r.w.pets[r.p.ServerID]
	if ps.stance != petPassive {
		t.Fatalf("stance %d: want passive", ps.stance)
	}
	r.w.PetCommand(r.sess, byte(petAggressive), 0, r.at(10*time.Second))
	if ps.stance != petPassive || !sentText(r, "cannot be aggressive") {
		t.Errorf("stance %d after asking aggressive: want passive, and told", ps.stance)
	}
	r.w.PetCommand(r.sess, petCmdAttack, crabObject, r.at(11*time.Second))
	if ps.target != 0 || !sentText(r, "cannot attack") {
		t.Errorf("target %d: a non-combat pet takes no fight", ps.target)
	}
	_ = pet
}

// sentText is whether a line holding s went to the rig's session.
func sentText(r *petRig, s string) bool {
	for _, m := range r.sent() {
		if strings.Contains(chatText(m), s) {
			return true
		}
	}
	return false
}

// A heal of either lands on the lower share of its owner and itself; a
// spell's scale is the pet's stat (the workbook's Modifier and Percentage).
func TestAWorkbookPetsHealOfEitherAndScale(t *testing.T) {
	r, _ := sheetRig(t, `INSERT INTO petstats (spellid, sheet_pet_id, name, level, type, hp, str, abilities)
		VALUES (717, 'T004', 'Test Spring', 33, 'priest', 1000, 60,
		'[{"kind":"heal","spell_id":1100,"target":"either","below":60,"every":5,"first":0}]')`)
	pet, _ := r.summon(t, fireElemental(), 0)
	ps := r.w.pets[r.p.ServerID]
	ab := ps.petAbilities()
	if len(ab) != 1 || ab[0].pinned == nil || ab[0].pinned.SpellID != 1100 {
		t.Fatalf("abilities %+v: want Healing pinned", ab)
	}
	for _, c := range []struct {
		petHP, ownerHP int
		want           EntityID
	}{
		{400, 900, pet.ServerID},  // the pet lower
		{900, 300, r.p.ServerID},  // its owner lower
		{1000, 899, r.p.ServerID}, // the owner below its most only: still the lower
		{100, 80, r.p.ServerID},   // both low: 8.9% under 10%
	} {
		pet.HP, r.p.HP = c.petHP, c.ownerHP
		r.w.mu.Lock()
		r.w.petCasts = nil
		pet.fight().busy = time.Time{}
		ok := r.w.petCastLocked(ps, 0, nil, r.at(30*time.Second))
		var got EntityID
		if len(r.w.petCasts) > 0 {
			got = r.w.petCasts[0].target
		}
		r.w.mu.Unlock()
		if !ok || got != c.want {
			t.Errorf("pet %d, owner %d: heal on %d (%v), want %d", c.petHP, c.ownerHP, got, ok, c.want)
		}
	}
	e := []SpellEffect{{Kind: EffectHP, Amount: -100, AmountMax: -100, Scale: StatStr, ScalePct: 15}}
	if n, _ := sheetFigure(pet, e, 100, false); n != 109 {
		t.Errorf("a 100 hit + 15%% of STR 60: %d, want 109", n)
	}
	if n, _ := sheetFigure(pet, e, 50, false); n != 55 {
		t.Errorf("at 50%%: %d, want 55", n)
	}
	if n, _ := sheetFigure(pet, []SpellEffect{{Kind: EffectHP, Amount: 200, AmountMax: 200}}, 100, true); n != 200 {
		t.Errorf("a heal of 200: %d", n)
	}
}

// A row the world cannot use is left out and said; one whose spell is not
// there casts nothing, said.
func TestPetStatsProblems(t *testing.T) {
	r, _ := sheetRig(t,
		`INSERT INTO petstats (spellid, name, type) VALUES (717, 'Odd', 'wizard')`,
		`INSERT INTO petstats (spellid, name, abilities) VALUES (918, 'Gone', '[{"kind":"nuke","spell_id":999999,"every":6}]')`)
	if r.w.templates.sheets[717] != nil || r.w.templates.sheets[918] == nil {
		t.Fatalf("sheets %v: want 918 alone", r.w.templates.sheets)
	}
	var bad, gone bool
	for _, p := range r.w.ClassTemplateProblems() {
		bad = bad || strings.Contains(p, "spell 717") && strings.Contains(p, "wizard")
		gone = gone || strings.Contains(p, "no spell 999999")
	}
	if !bad || !gone {
		t.Errorf("problems %q: want the bad type and the missing spell", r.w.ClassTemplateProblems())
	}
}

// A row's own size is its pet's (petition 1283: the Ghilan Noble's body no NPC
// wears came out at 1, a third of hers); 0 keeps its body's (the control).
func TestAWorkbookPetsOwnSize(t *testing.T) {
	for _, c := range []struct{ size, want float32 }{{3, 3}, {0, 1.25}} {
		pet := &NPC{Size: 1.25}
		applyPetSheet(pet, &petSheet{PetStats: database.PetStats{Size: c.size}})
		if pet.Size != c.want {
			t.Errorf("row size %v: the pet's %v, want %v", c.size, pet.Size, c.want)
		}
	}
}
