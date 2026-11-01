package database

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
)

// petstats: a summoned pet's own numbers, by its summoning spell
// (entity/petstats.go). The user's choice of 2026-11-01 for EQOAEmu's spell
// workbook, "Workbook wins, re-fit": the workbook gives every pet of the game
// its body, level, armour, stats, hit points, power, regeneration, resists,
// weapons and spells, and where it gives a number that number replaces ours;
// the balance's pet columns then scale on top of it and are fitted again.
// tools/import_eqoaemu_pets.py fills the table from the operator's copy of
// the workbook; with no row a pet is what it always was (pet.go,
// pettemplate.go).
//
// A row:
//
//   - spellid, the summoning spell (spellpattern), and sheet_pet_id the
//     workbook's id of its pet, for tracing;
//   - name, the pet's (”: the spell's less a leading "Summon ");
//   - model, its body (a CHAR.ESF CSprite's DictID as npcs.modelid keeps
//     one; 0: chosen as before), primary_model and secondary_model the
//     weapons it holds (an item's model as npcs.primary_weapon keeps one; 0
//     none);
//   - level, its level (0: the spell's), never above its caster's;
//   - type, how it fights: tank, melee, priest or caster, the archetype its
//     melee and its defence are a player's of (offence.go, defence.go), or
//     non-combat (it never fights); element, the workbook's word (unused);
//   - stances, the 0x0041 stances it takes, a list of passive, defensive,
//     neutral and aggressive (”: all four); one that takes no defensive
//     stance is summoned passive;
//   - ac its armour, str to cha its stats, each counting where a pet's do
//     (its effects' on top: pet.go npcHitOnPet, petMeleeHit), within
//     str_max to cha_max (0: none);
//   - hp its hit points before the balance's pet columns (0: its level's, as
//     before), power its power, hot and pot what it heals and gains a tick
//     (0: a 50th of each, as before; hot scaled as its hit points are);
//   - ar to dr its resists, against a creature's spells;
//   - hp_factor, off_mod and def_mod as the workbook gives them (no roll uses
//     them, a player's neither);
//   - wpn1_dmg and wpn1_type, wpn2_dmg and wpn2_type: its weapons' damage
//     and kind (slash, blunt, pierce): each melee hit a player's of its type
//     with that weapon (offence.go), its off hand's every other hit of a
//     round; 0: a creature's roll of its level, as before;
//   - abilities, a JSON list of TemplateAbility as classtemplates has them,
//     its spells pinned by spell_id: what it casts, in place of its pet
//     template's abilities (keep_template 1: those as well);
//   - note; source eqoaemu (the tool's rows) or gm.
const petStatsSchema = `
	CREATE TABLE IF NOT EXISTS petstats (
		spellid         INTEGER PRIMARY KEY,
		sheet_pet_id    TEXT NOT NULL DEFAULT '',
		name            TEXT NOT NULL DEFAULT '',
		model           INTEGER NOT NULL DEFAULT 0,
		primary_model   INTEGER NOT NULL DEFAULT 0,
		secondary_model INTEGER NOT NULL DEFAULT 0,
		level           INTEGER NOT NULL DEFAULT 0,
		type            TEXT NOT NULL DEFAULT '',
		element         TEXT NOT NULL DEFAULT '',
		stances         TEXT NOT NULL DEFAULT '',
		ac              INTEGER NOT NULL DEFAULT 0,
		str             INTEGER NOT NULL DEFAULT 0,
		sta             INTEGER NOT NULL DEFAULT 0,
		agi             INTEGER NOT NULL DEFAULT 0,
		dex             INTEGER NOT NULL DEFAULT 0,
		wis             INTEGER NOT NULL DEFAULT 0,
		intelligence    INTEGER NOT NULL DEFAULT 0,
		cha             INTEGER NOT NULL DEFAULT 0,
		hp              INTEGER NOT NULL DEFAULT 0,
		power           INTEGER NOT NULL DEFAULT 0,
		hot             INTEGER NOT NULL DEFAULT 0,
		pot             INTEGER NOT NULL DEFAULT 0,
		ar              INTEGER NOT NULL DEFAULT 0,
		cr              INTEGER NOT NULL DEFAULT 0,
		lr              INTEGER NOT NULL DEFAULT 0,
		fr              INTEGER NOT NULL DEFAULT 0,
		pr              INTEGER NOT NULL DEFAULT 0,
		dr              INTEGER NOT NULL DEFAULT 0,
		str_max         INTEGER NOT NULL DEFAULT 0,
		sta_max         INTEGER NOT NULL DEFAULT 0,
		agi_max         INTEGER NOT NULL DEFAULT 0,
		dex_max         INTEGER NOT NULL DEFAULT 0,
		wis_max         INTEGER NOT NULL DEFAULT 0,
		int_max         INTEGER NOT NULL DEFAULT 0,
		cha_max         INTEGER NOT NULL DEFAULT 0,
		hp_factor       INTEGER NOT NULL DEFAULT 0,
		off_mod         INTEGER NOT NULL DEFAULT 0,
		def_mod         INTEGER NOT NULL DEFAULT 0,
		wpn1_dmg        INTEGER NOT NULL DEFAULT 0,
		wpn1_type       TEXT NOT NULL DEFAULT '',
		wpn2_dmg        INTEGER NOT NULL DEFAULT 0,
		wpn2_type       TEXT NOT NULL DEFAULT '',
		abilities       TEXT NOT NULL DEFAULT '[]',
		keep_template   INTEGER NOT NULL DEFAULT 0,
		note            TEXT NOT NULL DEFAULT '',
		source          TEXT NOT NULL DEFAULT 'gm'
	);`

// The pets' types (petstats.type): an archetype, or a pet that never fights.
const (
	PetTypeTank      = "tank"
	PetTypeMelee     = "melee"
	PetTypePriest    = "priest"
	PetTypeCaster    = "caster"
	PetTypeNonCombat = "non-combat"
)

// PetTypes are the types a row may have (” as well: a melee pet's line, a
// creature's roll).
var PetTypes = []string{PetTypeTank, PetTypeMelee, PetTypePriest, PetTypeCaster, PetTypeNonCombat}

// PetStances are the 0x0041 stances by name, in their command's order (1-4).
var PetStances = []string{"passive", "defensive", "neutral", "aggressive"}

// PetWeaponTypes are a pet weapon's kinds, and the one-handed attack type
// each is to CLIENT (the hands' attack types on the channel: 1 a one-handed
// slash, 3 a blunt, 5 a pierce; Sandstorm's recorded Fire Elemental's hands
// were 1 and 5, the workbook's slash and pierce).
var PetWeaponTypes = map[string]byte{"slash": 1, "blunt": 3, "pierce": 5}

// PetStats is one row of petstats, read.
type PetStats struct {
	SpellID                      int
	SheetPetID                   string
	Name                         string
	Model                        uint32
	PrimaryModel, SecondaryModel uint32
	Level                        int
	Type                         string
	Element                      string
	Stances                      []string // lower case, PetStances' words; nil: all four
	AC                           int
	Str, Sta, Agi, Dex           int
	Wis, Int, Cha                int
	HP, Power, HoT, PoT          int
	AR, CR, LR, FR, PR, DR       int
	StrMax, StaMax, AgiMax       int
	DexMax, WisMax, IntMax       int
	ChaMax                       int
	HPFactor, OffMod, DefMod     int
	Wpn1Dmg                      int
	Wpn1Type                     string
	Wpn2Dmg                      int
	Wpn2Type                     string
	Abilities                    []TemplateAbility
	KeepTemplate                 bool
	Note                         string
}

// TakesStance is whether the pet takes the stance (1 passive ... 4
// aggressive): every pet with no stances listed.
func (p *PetStats) TakesStance(stance int) bool {
	if len(p.Stances) == 0 {
		return true
	}
	if stance < 1 || stance > len(PetStances) {
		return false
	}
	for _, s := range p.Stances {
		if s == PetStances[stance-1] {
			return true
		}
	}
	return false
}

// ParsePetStances reads a stances column: words of PetStances, split by
// commas or spaces, any case (”: nil, all four).
func ParsePetStances(text string) ([]string, error) {
	var out []string
	for _, f := range strings.FieldsFunc(strings.ToLower(text), func(r rune) bool { return r == ',' || r == ' ' || r == ';' || r == '/' }) {
		if !oneOf(f, PetStances) {
			return nil, fmt.Errorf("stance %q: one of %s", f, strings.Join(PetStances, ", "))
		}
		dup := false
		for _, s := range out {
			dup = dup || s == f
		}
		if !dup {
			out = append(out, f)
		}
	}
	return out, nil
}

// CheckPetStats is what a petstats row must be for a pet to be summoned by
// it: a summoning spell, a type and weapon kinds the server knows, a level of
// at most 100, and no number below 0 or past a million (its abilities are
// held to CheckTemplateAbilities when they are read).
func CheckPetStats(p PetStats) error {
	nums := []struct {
		name string
		v    int
	}{
		{"ac", p.AC}, {"str", p.Str}, {"sta", p.Sta}, {"agi", p.Agi}, {"dex", p.Dex}, {"wis", p.Wis},
		{"intelligence", p.Int}, {"cha", p.Cha}, {"hp", p.HP}, {"power", p.Power}, {"hot", p.HoT}, {"pot", p.PoT},
		{"ar", p.AR}, {"cr", p.CR}, {"lr", p.LR}, {"fr", p.FR}, {"pr", p.PR}, {"dr", p.DR},
		{"str_max", p.StrMax}, {"sta_max", p.StaMax}, {"agi_max", p.AgiMax}, {"dex_max", p.DexMax},
		{"wis_max", p.WisMax}, {"int_max", p.IntMax}, {"cha_max", p.ChaMax},
		{"hp_factor", p.HPFactor}, {"off_mod", p.OffMod}, {"def_mod", p.DefMod},
		{"wpn1_dmg", p.Wpn1Dmg}, {"wpn2_dmg", p.Wpn2Dmg},
	}
	switch {
	case p.SpellID <= 0:
		return fmt.Errorf("spellid %d: a summoning spell's id", p.SpellID)
	case p.Level < 0 || p.Level > 100:
		return fmt.Errorf("level %d: 0 (the spell's) to 100", p.Level)
	case p.Type != "" && !oneOf(p.Type, PetTypes):
		return fmt.Errorf("type %q: one of %s (or empty)", p.Type, strings.Join(PetTypes, ", "))
	case p.Wpn1Type != "" && PetWeaponTypes[p.Wpn1Type] == 0:
		return fmt.Errorf("wpn1_type %q: slash, blunt or pierce (or empty)", p.Wpn1Type)
	case p.Wpn2Type != "" && PetWeaponTypes[p.Wpn2Type] == 0:
		return fmt.Errorf("wpn2_type %q: slash, blunt or pierce (or empty)", p.Wpn2Type)
	}
	for _, n := range nums {
		if n.v < 0 || n.v > 1000000 {
			return fmt.Errorf("%s %d: 0 to 1000000", n.name, n.v)
		}
	}
	return nil
}

// ensurePetStats makes petstats (initSchema), empty: the tool fills it.
func (db *SQLiteDB) ensurePetStats() error {
	if _, err := db.Exec(petStatsSchema); err != nil {
		return fmt.Errorf("petstats: %w", err)
	}
	return nil
}

const petStatsColumns = `spellid, sheet_pet_id, name, model, primary_model, secondary_model, level, type, element, stances,
	ac, str, sta, agi, dex, wis, intelligence, cha, hp, power, hot, pot, ar, cr, lr, fr, pr, dr,
	str_max, sta_max, agi_max, dex_max, wis_max, int_max, cha_max, hp_factor, off_mod, def_mod,
	wpn1_dmg, wpn1_type, wpn2_dmg, wpn2_type, abilities, keep_template, note`

// PetStatsRows is petstats' rows by summoning spell; bad says each row it
// could not use and why (it is left out: its pets are as before).
func (db *SQLiteDB) PetStatsRows() (rows map[int]PetStats, bad []string, err error) {
	q, err := db.Query(`SELECT ` + petStatsColumns + ` FROM petstats`)
	if err != nil {
		return nil, nil, err
	}
	defer q.Close()
	rows = map[int]PetStats{}
	for q.Next() {
		var p PetStats
		var model, primary, secondary int64
		var stances, abilities string
		var keep int
		if err := q.Scan(&p.SpellID, &p.SheetPetID, &p.Name, &model, &primary, &secondary, &p.Level, &p.Type, &p.Element,
			&stances, &p.AC, &p.Str, &p.Sta, &p.Agi, &p.Dex, &p.Wis, &p.Int, &p.Cha, &p.HP, &p.Power, &p.HoT, &p.PoT,
			&p.AR, &p.CR, &p.LR, &p.FR, &p.PR, &p.DR, &p.StrMax, &p.StaMax, &p.AgiMax, &p.DexMax, &p.WisMax, &p.IntMax,
			&p.ChaMax, &p.HPFactor, &p.OffMod, &p.DefMod, &p.Wpn1Dmg, &p.Wpn1Type, &p.Wpn2Dmg, &p.Wpn2Type, &abilities,
			&keep, &p.Note); err != nil {
			return nil, nil, err
		}
		p.Model, p.PrimaryModel, p.SecondaryModel = uint32(model), uint32(primary), uint32(secondary)
		p.Name = strings.TrimSpace(p.Name)
		p.Type = strings.ToLower(strings.TrimSpace(p.Type))
		p.Wpn1Type = strings.ToLower(strings.TrimSpace(p.Wpn1Type))
		p.Wpn2Type = strings.ToLower(strings.TrimSpace(p.Wpn2Type))
		p.KeepTemplate = keep != 0
		var perr error
		if p.Stances, perr = ParsePetStances(stances); perr == nil {
			if p.Abilities, perr = ParseTemplateAbilities(abilities); perr == nil {
				perr = CheckPetStats(p)
			}
		}
		if perr != nil {
			bad = append(bad, fmt.Sprintf("spell %d (%s): %v", p.SpellID, p.Name, perr))
			continue
		}
		rows[p.SpellID] = p
	}
	return rows, bad, q.Err()
}

// TemplateSpellByID is the spell of that id an ability pins (spell_id: a
// workbook pet's own spells); ok false when there is none.
func (db *SQLiteDB) TemplateSpellByID(id int) (TemplateSpell, bool, error) {
	row := db.QueryRow(`SELECT `+templateSpellColumns+`
		FROM spellpattern sp
		LEFT JOIN spelleffect se ON se.spellid = sp.spellid
		LEFT JOIN spellarea sa ON sa.spellid = sp.spellid
		WHERE sp.spellid = ?`, id)
	s, err := scanTemplateSpell(row.Scan)
	if errors.Is(err, sql.ErrNoRows) {
		return TemplateSpell{}, false, nil
	}
	if err != nil {
		return TemplateSpell{}, false, err
	}
	return s, true, nil
}
