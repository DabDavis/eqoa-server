package entity

import (
	"fmt"
	"testing"
	"time"

	"github.com/eqoa/server/internal/database"
)

// The master class pets' element curses as tools/pet_element_debuffs_20261113.py
// writes them (the owner's sheet, Magician rows 46-47: "-35 debuff ability for
// each *mancer pet of their element"; a follow-up to ticket 2001's Ghilan's
// Frost Curse): the Efreeti Lord's fire, the Djinni Vizier's lightning (air),
// the Badi Elder's disease (earth), each first in its kit, 35 off for a minute,
// a 2 s cast. The rows are each pet's live one cut to the curse and its first
// ability before it; the visuals are made up.

type elementPet struct {
	summon            CastSpell
	name, kind        string // its row's name and type
	curseID           int
	curse, stat, line string
	st                Stat
	fx                uint32
	first             string // the ability before the curse, as its kit has it
	firstRows         string // that ability's spell rows, if any
}

var elementPets = []elementPet{
	{summonSpell(701087, 60, "Summon Efreeti Lord", "Summons an Efreeti Lord to aid you"), "Efreeti Lord", "melee",
		907238, "Efreeti's Flame Curse", "fr", "Target, Decrease FR by 35, 1min", StatFR, 0x0B300001,
		`{"every":30.0,"first":1.0,"kind":"nuke","spell_id":905453}`,
		`(905453, 'Target, Decrease Hitpoints by 2000', 1338050561, 'test', '{"v":1,"scope":"target","target_type":"Enemy","harmful":true,"stack":0,"level":60,"min_level":1,"req_equipment":"Any","effects":[{"type":"Dmg/Heal","kind":"hp","amount":-2000,"resist":"fr"}]}')`},
	{summonSpell(900761, 60, "Summon Djinni Vizier (60)", "Summons a Djinni Vizier to aid you"), "Djinni Vizier", "caster",
		907239, "Djinni's Storm Curse", "lr", "Target, Decrease LR by 35, 1min", StatLR, 0x0B300002,
		`{"every":12.0,"first":1.0,"kind":"nuke","spell_id":905450}`,
		`(905450, 'Target, Decrease Hitpoints by 1000', 2972655021, 'test', '{"v":1,"scope":"target","target_type":"Enemy","harmful":true,"stack":0,"level":60,"min_level":1,"req_equipment":"Any","effects":[{"type":"Dmg/Heal","kind":"hp","amount":-1000,"resist":"lr"}]}')`},
	{summonSpell(701086, 60, "Summon Badi Elder", "Summons a Badi Elder to aid you"), "Badi Elder", "tank",
		907240, "Badi's Blight Curse", "dr", "Target, Decrease DR by 35, 1min", StatDR, 0x0B300003,
		`{"every":15.0,"first":1.0,"kind":"taunt","hits":[[1,0,0]]}`, ``},
}

func elementRig(t *testing.T, e elementPet) *petRig {
	t.Helper()
	kit := fmt.Sprintf(`[{"every":30.0,"first":1.0,"kind":"debuff","spell_id":%d},%s]`, e.curseID, e.first)
	rows := []string{
		fmt.Sprintf(`INSERT INTO spellpattern (spellid, abilitylvl, unk2, unk3, range, casttime, power, iconColor, icon, scope,
			recast, eqprequire, spellname, spelldesc) VALUES
			(%d, 60, 0, 0, 16880, 2, 0, 0, 0, 1, 30, -1, '%s', ''),
			(905453, 60, 0, 0, 16880, 1, 0, 0, 0, 1, 30, -1, 'Efreeti''s Inferno', ''),
			(905450, 60, 0, 0, 16880, 0, 0, 0, 0, 1, 12, -1, 'Vizier''s Bolt', '')`, e.curseID, sqlQuote(e.curse)),
		fmt.Sprintf(`INSERT INTO spelleffect (spellid, line, fxdict, source, effects) VALUES
			(%d, '%s', %d, 'test', '{"v":1,"scope":"target","target_type":"Enemy","harmful":true,"duration_s":60,"stack":0,"level":60,"min_level":1,"req_equipment":"Any","sheet_type":"Statistic Change","effects":[{"type":"Statistic Change","kind":"stat","stat":"%s","amount":-35,"resist":"%s"}]}')`,
			e.curseID, e.line, e.fx, e.stat, e.stat),
		fmt.Sprintf(`INSERT INTO petstats (spellid, sheet_pet_id, name, model, level, type, element, ac, str, sta, agi, dex, wis,
			intelligence, cha, hp, power, hot, pot, ar, cr, lr, fr, pr, dr, str_max, sta_max, agi_max, dex_max, wis_max,
			int_max, cha_max, hp_factor, off_mod, def_mod, wpn1_dmg, wpn1_type, abilities, size, source)
			VALUES (%d, '003D', '%s', 3142338730, 55, '%s', 'fire', 1650, 545, 435, 435, 545, 270, 380, 160, 4540, 5733, 100,
			127, 55, 55, 55, 55, 55, 55, 600, 600, 600, 600, 600, 600, 600, 43, 4, 1, 660, 'slash', '%s', 2.0, 'test')`,
			e.summon.SpellID, e.name, e.kind, kit),
	}
	if e.firstRows != "" {
		rows = append(rows, `INSERT INTO spelleffect (spellid, line, fxdict, source, effects) VALUES `+e.firstRows)
	}
	r, _ := sheetRig(t, rows...)
	r.p.Class, r.p.Level = 10, 60
	return r
}

func sqlQuote(s string) string {
	out := ""
	for _, c := range s {
		if c == '\'' {
			out += "''"
		} else {
			out += string(c)
		}
	}
	return out
}

// Sent at a crab, each pet casts its curse first, 1 s in, a 2 s cast; it lands
// as 35 off the crab's resist of its element (and no other) for a minute, kept
// up over 130 s with never two on at once.
func TestPetElementCursesFirstAndKeptUp(t *testing.T) {
	for _, e := range elementPets {
		t.Run(e.name, func(t *testing.T) {
			r := elementRig(t, e)
			f := swarmFightFor(t, r, e.summon, 4540, 130*time.Second)
			ab := r.w.pets[r.p.ServerID].petAbilities()
			if len(ab) != 2 || ab[0].Kind != database.AbilityDebuff {
				t.Fatalf("its kit: %+v", ab)
			}
			at, ms := f.of(e.fx)
			if len(at) < 2 || len(at) > 3 || at[0] != 10700 || ms[0] != 2000 {
				t.Errorf("the curse cast at %v (%v ms): want first at 10700, a 2000 ms cast, kept up", at, ms)
			}
			for _, c := range f.casts { // its melee rounds' own 0x00B1s aside
				if c.at < at[0] && (c.fx == 1338050561 || c.fx == 2972655021) {
					t.Errorf("its nuke %#x cast at %d before the curse", c.fx, c.at)
				}
			}
			if n := f.logged(fmt.Sprintf(`debuffs Desert Crab with %q for 1m0s`, e.curse)); n != len(at) {
				t.Errorf("%d curses landed of %d cast", n, len(at))
			}
			if f.most > 1 {
				t.Errorf("%d curses on the crab at once", f.most)
			}
			now := r.at(9700*time.Millisecond + 130*time.Second)
			for _, st := range []Stat{StatFR, StatCR, StatLR, StatPR, StatDR} {
				want := 0
				if st == e.st {
					want = 35
				}
				if off := npcResistDebuff(r.enemy, st, now); off != want {
					t.Errorf("resist %v down %d at the end, want %d", st, off, want)
				}
			}
		})
	}
}

// Over a real server's database with tools/pet_element_debuffs_20261113.py
// applied (EQOA_REAL_DB, copied): each of the three summonings makes its pet
// with its curse first and the rest of its kit after, the curse its first
// spell at the crab, landing 35 off its element's resist.
func TestRealPetElementCurses(t *testing.T) {
	db := simRealDB(t)
	for _, e := range elementPets {
		t.Run(e.name, func(t *testing.T) {
			sum, ok, err := db.TemplateSpellByID(e.summon.SpellID)
			if err != nil || !ok {
				t.Fatalf("the summoning %d: there %v, %v", e.summon.SpellID, ok, err)
			}
			r := newPetRig(t)
			if _, err := r.w.LoadClassTemplates(db); err != nil {
				t.Fatal(err)
			}
			r.p.Class, r.p.Level = 10, 60
			f := swarmFightFor(t, r, *templateSpellOf(sum), 4540, 70*time.Second)
			ab := r.w.pets[r.p.ServerID].petAbilities()
			if len(ab) < 2 || ab[0].Kind != database.AbilityDebuff || ab[0].pinned == nil || ab[0].pinned.Name != e.curse {
				t.Fatalf("its kit: %d abilities, the first %+v: the tool not applied to this database?", len(ab), ab[0].TemplateAbility)
			}
			curse := ab[0].pinned
			if curse.CastTime != 2*time.Second {
				t.Errorf("the curse's cast %v, want 2s", curse.CastTime)
			}
			kit := map[uint32]bool{}
			for _, a := range ab {
				if a.pinned != nil {
					kit[a.pinned.visual()] = true
				}
			}
			var first *petSeen
			for i, c := range f.casts {
				if kit[c.fx] {
					first = &f.casts[i]
					break
				}
			}
			if first == nil || first.fx != curse.visual() {
				t.Errorf("its first spell: %+v; want the curse (visual %#x)", first, curse.visual())
			}
			if off := npcResistDebuff(r.enemy, e.st, r.at(9700*time.Millisecond+70*time.Second)); off != 35 {
				t.Errorf("the crab's %v %d down, want 35", e.st, off)
			}
			t.Logf("%s: %d abilities, casts %+v", e.name, len(ab), f.casts)
		})
	}
}
