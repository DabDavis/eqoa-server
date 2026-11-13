package entity

import (
	"fmt"
	"testing"
	"time"

	"github.com/eqoa/server/internal/database"
)

// The Ghilan Noble as tools/ghilan_noble_2001_20261113.py leaves her row (the
// magician Waldorf's ticket 2001: "Ghilan Noble doesn't seem to cast her
// blessing on herself in addition to the caster ... can you confirm that Ghilan
// Noble also casts a debuff on the mob. That was one of the first spells she
// would use"): Ghilan's Frost Curse (35 off a creature's cold resist for a
// minute, a 2 s cast, its effects as the tool writes them) first, her heals,
// Ghilan's Blessing on her owner and on herself, a nuke. The visuals are made up.

const fxGhilanCurse uint32 = 0x0B200005

const ghilan2001Kit = `[{"every":30.0,"first":1.0,"kind":"debuff","spell_id":907237},` +
	`{"every":30.0,"first":1.0,"kind":"heal","spell_id":902165,"target":"either","below":60},` +
	`{"every":1800.0,"first":3.0,"kind":"buff","spell_id":902167,"target":"owner"},` +
	`{"every":1800.0,"first":5.0,"kind":"buff","spell_id":902167,"target":"self"},` +
	`{"every":30.0,"first":5.0,"kind":"nuke","spell_id":902166}]`

func ghilan2001Rig(t *testing.T) *petRig {
	t.Helper()
	r, _ := sheetRig(t,
		`INSERT INTO spellpattern (spellid, abilitylvl, unk2, unk3, range, casttime, power, iconColor, icon, scope, recast,
			eqprequire, spellname, spelldesc) VALUES
			(902165, 55, 0, 0, 16880, 3, 220, 0, 0, 1, 30, -1, 'Water Heal Level 55', ''),
			(902166, 55, 0, 0, 16880, 2, 220, 0, 0, 1, 12, -1, 'Water Nuke Level 55', ''),
			(902167, 1, 0, 0, 16528, 3, 0, 0, 0, 1, 1800, -1, 'Ghilan''s Blessing', ''),
			(907237, 60, 0, 0, 16880, 2, 0, 0, 0, 1, 30, -1, 'Ghilan''s Frost Curse', '')`,
		fmt.Sprintf(`INSERT INTO spelleffect (spellid, line, fxdict, source, effects) VALUES
			(902165, 'Target, Increase Hitpoints by 1925 ; Increase 25%% based on CHA', %d, 'test', ''),
			(902166, 'Target, Decrease Hitpoints by 440 ; Increase 25%% based on WIS', %d, 'test', ''),
			(902167, 'Target, Increase WIS by 50, Increase CR by 50, Increase AR by 50, Increase HP per tic by 130, 30min', %d, 'test', ''),
			(907237, 'Target, Decrease CR by 35, 1min', %d, 'test', '{"v":1,"scope":"target","target_type":"Enemy","harmful":true,"duration_s":60,"stack":0,"level":60,"min_level":1,"req_equipment":"Any","sheet_type":"Statistic Change","effects":[{"type":"Statistic Change","kind":"stat","stat":"cr","amount":-35,"resist":"cr"}]}')`,
			fxGhilanHeal, fxGhilanNuke, fxGhilanBless, fxGhilanCurse),
		`INSERT INTO petstats (spellid, sheet_pet_id, name, model, level, type, element, ac, str, sta, agi, dex, wis,
			intelligence, cha, hp, power, hot, pot, ar, cr, lr, fr, pr, dr, str_max, sta_max, agi_max, dex_max, wis_max,
			int_max, cha_max, hp_factor, off_mod, def_mod, wpn1_dmg, wpn1_type, abilities, size, source)
			VALUES (701088, '003E', 'Ghilan Noble', 2430398935, 60, 'priest', 'water', 2200, 435, 490, 380, 270, 600, 270,
			545, 4650, 6338, 103, 140, 67, 100, 67, 67, 67, 67, 600, 600, 600, 600, 600, 600, 600, 40, 0, 4, 385, 'blunt',
			'`+ghilan2001Kit+`', 2.0, 'test')`)
	r.p.Class, r.p.Level = 10, 60
	return r
}

// Sent at a crab, she casts her curse first, 1 s in (a 2 s cast her owner sees
// her begin), it lands as 35 off the crab's cold resist for a minute (what it
// resists of a cold spell less: workbook.go's npcResistKindPct), she keeps it up
// (cast again as it runs out, never two of hers on it), and she has her blessing
// on her owner and on herself, once each.
func TestTicket2001GhilanNobleCursesFirstAndBlessesHerself(t *testing.T) {
	r := ghilan2001Rig(t)
	f := swarmFightFor(t, r, summonGhilanNoble, 4650, 130*time.Second)
	var first *petSeen // her first spell of her row (her melee's own visuals aside)
	for i, c := range f.casts {
		if c.fx == fxGhilanCurse || c.fx == fxGhilanHeal || c.fx == fxGhilanBless || c.fx == fxGhilanNuke {
			first = &f.casts[i]
			break
		}
	}
	if first == nil || first.fx != fxGhilanCurse || first.at != 10700 || first.ms != 2000 {
		t.Fatalf("her first spell in the fight: %+v; want the curse at 10700 ms, a 2000 ms cast", first)
	}
	if crab := r.w.objectIDForEntityLocked(r.sess, r.enemy.ServerID); first.target != crab {
		t.Errorf("the curse cast at object %d, want the crab's %d", first.target, crab)
	}
	at, _ := f.of(fxGhilanCurse)
	if len(at) < 2 || len(at) > 3 {
		t.Errorf("the curse cast at %v over 130 s: want it kept up (a minute each, again as it runs out)", at)
	}
	if n := f.logged(`debuffs Desert Crab with "Ghilan's Frost Curse" for 1m0s`); n != len(at) {
		t.Errorf("%d curses landed of %d cast", n, len(at))
	}
	if f.most > 1 {
		t.Errorf("%d of her curses on the crab at once, want one", f.most)
	}
	now := r.at(9700*time.Millisecond + 130*time.Second)
	if off := npcResistDebuff(r.enemy, StatCR, now); off != 35 {
		t.Errorf("the crab's cold resist is %d down at the end, want 35 (kept up)", off)
	}
	if npcResistKindPct(r.enemy, StatCR, now) <= 0 || npcResistDebuff(r.enemy, StatFR, now) != 0 {
		t.Errorf("the curse takes %d points off its chance to resist cold, fire down %d; want some, none",
			npcResistKindPct(r.enemy, StatCR, now), npcResistDebuff(r.enemy, StatFR, now))
	}
	if n := f.logged(`buffs its owner with "Ghilan's Blessing"`); n != 1 {
		t.Errorf("her blessing on her owner %d times, want once", n)
	}
	if n := f.logged(`buffs itself with "Ghilan's Blessing"`); n != 1 {
		t.Errorf("her blessing on herself %d times, want once", n)
	}
	if !hasEffect(f.pet.Effects, 902167) || !hasEffect(r.p.Effects, 902167) {
		t.Errorf("Ghilan's Blessing on her %v, on her owner %v; want both", hasEffect(f.pet.Effects, 902167),
			hasEffect(r.p.Effects, 902167))
	}
}

// Her curse landing on a creature that has not noticed anyone angers it at
// her (a hostile act, as her hit is), and a charmed creature's own debuff (a
// creature's, at a player) is never cast by it as a pet.
func TestTicket2001APetsDebuffAngersAndACharmedOneCastsNone(t *testing.T) {
	r := ghilan2001Rig(t)
	pet, _ := r.summon(t, summonGhilanNoble, 0)
	ps := r.w.pets[r.p.ServerID]
	crab := r.enemy
	ps.target = crab.ServerID
	curse := &ps.petAbilities()[0]
	if curse.Kind != database.AbilityDebuff {
		t.Fatalf("her first ability is a %q, want the debuff", curse.Kind)
	}
	r.w.mu.Lock()
	s := curse.spellFor(petSpellClass(curse, pet), pet.Level)
	r.w.petLandLocked(&petCast{ps: ps, tmpl: ps.tmpl, a: curse, spell: s, target: crab.ServerID}, r.at(20*time.Second))
	wanted := r.w.petWantsLocked(ps, curse, r.at(21*time.Second))
	r.w.mu.Unlock()
	if crab.Threats[pet.ServerID] <= 0 || crab.CurrentEnemy != pet.ServerID {
		t.Errorf("the crab's hate on her %d, its enemy %d; want her (%d)", crab.Threats[pet.ServerID], crab.CurrentEnemy, pet.ServerID)
	}
	if wanted {
		t.Errorf("the curse wanted again with 59 s of it left on the crab")
	}

	r2 := ghilan2001Rig(t)
	pet2, _ := r2.summon(t, summonGhilanNoble, 0)
	ps2 := r2.w.pets[r2.p.ServerID]
	pet2.Charm = &charmHold{}
	r2.w.mu.Lock()
	r2.w.petTemplateTurnLocked(ps2, r2.enemy, r2.at(30*time.Second))
	r2.w.mu.Unlock()
	for _, c := range r2.w.petCasts {
		if c.a.Kind == database.AbilityDebuff {
			t.Errorf("a charmed pet began its debuff %q", c.spell.Name)
		}
	}
}

// Over a real server's database with tools/ghilan_noble_2001_20261113.py
// applied (EQOA_REAL_DB, copied): Summon Ghilan Noble (701088) makes her with
// the curse first in her kit, her first spell at the crab is the curse, it
// lands 35 off its cold resist, and her blessing goes on her owner and on her.
func TestRealTicket2001GhilanNoble(t *testing.T) {
	db := simRealDB(t)
	sum, ok, err := db.TemplateSpellByID(701088)
	if err != nil || !ok {
		t.Fatalf("the summoning 701088: there %v, %v", ok, err)
	}
	r := newPetRig(t)
	if _, err := r.w.LoadClassTemplates(db); err != nil {
		t.Fatal(err)
	}
	r.p.Class, r.p.Level = 10, 60
	f := swarmFightFor(t, r, *templateSpellOf(sum), 4650, 70*time.Second)
	ps := r.w.pets[r.p.ServerID]
	ab := ps.petAbilities()
	if len(ab) != 9 || ab[0].Kind != database.AbilityDebuff || ab[0].pinned == nil || ab[0].pinned.Name != "Ghilan's Frost Curse" {
		t.Fatalf("her kit: %d abilities, the first %+v: the tool not applied to this database?", len(ab), ab[0].TemplateAbility)
	}
	curse := ab[0].pinned
	kit := map[uint32]bool{}
	for _, a := range ab {
		if a.pinned != nil {
			kit[a.pinned.visual()] = true
		}
	}
	var first *petSeen // her first spell of her kit (her melee rounds' own 0x00B1s aside)
	for i, c := range f.casts {
		if kit[c.fx] {
			first = &f.casts[i]
			break
		}
	}
	if first == nil || first.fx != curse.visual() || first.at != 10700 {
		t.Errorf("her first spell: %+v; want the curse (visual %#x) at 10700", first, curse.visual())
	}
	if off := npcResistDebuff(r.enemy, StatCR, r.at(9700*time.Millisecond+70*time.Second)); off != 35 {
		t.Errorf("the crab's cold resist %d down, want 35", off)
	}
	if f.logged(`buffs its owner with "Ghilan's Blessing"`) != 1 || f.logged(`buffs itself with "Ghilan's Blessing"`) != 1 {
		t.Errorf("her blessing on her owner %d times, on herself %d; want once each",
			f.logged(`buffs its owner with "Ghilan's Blessing"`), f.logged(`buffs itself with "Ghilan's Blessing"`))
	}
	t.Logf("casts: %+v", f.casts)
}
