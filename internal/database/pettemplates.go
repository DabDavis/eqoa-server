package database

import (
	"fmt"
	"strings"
)

// pettemplate: the class template a summoned pet fights by, by its summoning
// spell (entity/pettemplate.go; the user, 2026-10-30: "necro pets are like
// SKs that quad, or 2H if the player bought the CMs. Magician pets are more
// varied ... enchanter pets are all the same").
//
// A row:
//
//   - spell, the summoning spell's name (TemplateKey), and weapon, 1 for its
//     "with weapons" / "with a weapon" version (the necromancer's Death
//     Sacrament pets: a spell whose description says so,
//     PetWeaponSpells); a weapon version with no row of its own takes its
//     name's base row;
//   - template, a classtemplates name: the pet's class (its spells, and its
//     swing by CLIENT's GetSwing), hit points, the share of a creature's
//     melee hit on it that lands, its own melee hits, and its abilities;
//   - swing_class, the class whose CLIENT swing it swings by at its level in
//     place of the template's (-1: the template's): a necromancer's pet is a
//     shadow knight that swings a warrior's round;
//   - swing, a CLIENT swing kind (0-11) it swings whatever its level in place
//     of that (-1: none), and round_ms its round's length (0: the swing's
//     own delay): Sandstorm's recorded rounds for the Death Sacrament pets.
//     A fixed swing keeps the damage a second its class's own round gave the
//     pet (its hits scaled);
//   - note; source gm, or seed for the rows the table is made with.
//
// With no row a pet fights as before (pet.go: a warrior of its level,
// petrotation.go's recorded rounds for the necromancer's zombie, mummy and
// spectre). The table and its seed templates are made once: on a database
// without pettemplate, the seed rows go into classtemplates (a GM's row of a
// name kept) and pettemplate; one a GM emptied stays empty.
const petTemplateSchema = `
	CREATE TABLE IF NOT EXISTS pettemplate (
		spell       TEXT NOT NULL,
		weapon      INTEGER NOT NULL DEFAULT 0,
		template    TEXT NOT NULL DEFAULT '',
		swing_class INTEGER NOT NULL DEFAULT -1,
		swing       INTEGER NOT NULL DEFAULT -1,
		round_ms    INTEGER NOT NULL DEFAULT 0,
		note        TEXT NOT NULL DEFAULT '',
		source      TEXT NOT NULL DEFAULT 'gm',
		PRIMARY KEY (spell, weapon)
	);`

// PetTemplate is one row of pettemplate, read.
type PetTemplate struct {
	Spell      string // TemplateKey of the summoning spell's name
	Weapon     bool
	Template   string // TemplateKey
	SwingClass int    // -1: the template's class
	Swing      int    // -1: none
	RoundMS    int    // 0: the swing's delay
	Note       string
}

// PetTemplateKey is a pettemplate row's key.
type PetTemplateKey struct {
	Spell  string
	Weapon bool
}

// CheckPetTemplate is what a pettemplate row must be: a spell and a template
// named, a swing class of the fifteen or -1, a swing of CLIENT's twelve or
// -1, and a round of 1 to 20 s or 0.
func CheckPetTemplate(p PetTemplate) error {
	switch {
	case TemplateKey(p.Spell) == "":
		return fmt.Errorf("no spell")
	case TemplateKey(p.Template) == "":
		return fmt.Errorf("no template")
	case p.SwingClass < -1 || p.SwingClass > 14:
		return fmt.Errorf("swing_class %d: -1 (the template's) or 0 (Warrior) to 14 (Alchemist)", p.SwingClass)
	case p.Swing < -1 || p.Swing > 11:
		return fmt.Errorf("swing %d: -1 (none) or a CLIENT swing kind, 0 to 11", p.Swing)
	case p.RoundMS != 0 && (p.RoundMS < 1000 || p.RoundMS > 20000):
		return fmt.Errorf("round_ms %d: 0 (the swing's) or 1000 to 20000", p.RoundMS)
	}
	return nil
}

func intp(v int) *int { return &v }

// SeedPetClassTemplates are the pets' templates, given to classtemplates
// with pettemplate (their numbers ours, the notes say from what; the
// balance's re-fit tunes them).
var SeedPetClassTemplates = []ClassTemplate{
	{Name: "pet necromancer", Class: 3, HPPct: 100, TakenPct: 100, MeleePct: 100,
		Note: "A necromancer's pet: a shadow knight (its class) that swings a warrior's round (pettemplate swing_class 0: two " +
			"hits from 20, four from 35, the user's 'SKs that quad'; CLIENT's shadow knight never swings four); its taps, " +
			"openers and the spectre's instant are petrotation.go's"},
	{Name: "pet necromancer weapon", Class: 3, HPPct: 100, TakenPct: 100, MeleePct: 120,
		Note: "Death Sacrament's pet (the 'with weapons' and 'with a weapon' summonings, the mastery's 'Pet weapons (Increases " +
			"melee damage)'): Sandstorm's recorded rounds (pettemplate swing, round_ms), its hits scaled to the base pet's " +
			"damage a second, then 20% more (melee_pct 120, ours)"},
	{Name: "pet water", Class: 9, HPPct: 100, TakenPct: 100, MeleePct: 100,
		Abilities: []TemplateAbility{
			{Kind: AbilityHeal, Target: AbilityOnOwner, Below: 60, Every: 12, First: 1},
			{Kind: AbilityHeal, Below: 50, Every: 15, First: 1},
			{Kind: AbilityNuke, Every: 10, First: 3, MaxRecast: 10, MaxCast: 5, DamagePct: 30},
		},
		Note: "The magician's water pet: a cleric (one hit a round) that heals its owner below 60% with the cleric's instant heal " +
			"of its level (Field Dress 20, Bind Wounds 30, Quick Stitching 40, Remedy 49; none below 20), itself below half, and casts a cleric's offensive spell (Strike ... Vengeance) every 10 s at 30% (ours)"},
	{Name: "pet earth", Class: 0, HPPct: 130, TakenPct: 80, MeleePct: 90,
		Abilities: []TemplateAbility{
			{Kind: AbilityTaunt, Every: 9, First: 1},
			{Kind: AbilityBuff, Stat: "ac", SpellsOf: intp(7), Every: 20, First: 2},
			{Kind: AbilityBuff, Stat: "agi", SpellsOf: intp(1), Every: 20, First: 4},
		},
		Note: "The magician's earth pet: a tank (a warrior's round, four hits from 35; 30% more hit points, 20% less of a melee " +
			"hit lands, its own 10% lighter) that taunts every 9 s (a melee hit's damage, the top of the hate) and keeps its " +
			"owner in the druid's skin line (AC) and the ranger's AGI line of its level (ours)"},
	{Name: "pet fire", Class: 0, HPPct: 100, TakenPct: 100, MeleePct: 125,
		Abilities: []TemplateAbility{
			{Kind: AbilityNuke, Element: "fire", SpellsOf: intp(10), Every: 12, First: 4, MaxRecast: 10, MaxCast: 5, DamagePct: 25},
			{Kind: AbilityBuff, Stat: "str", SpellsOf: intp(8), Every: 20, First: 2, MaxRecast: 60},
			{Kind: AbilityBuff, Stat: "dex", SpellsOf: intp(8), Every: 20, First: 4, MaxRecast: 60},
		},
		Note: "The magician's fire pet: the hardest melee (a warrior's round, 25% harder), a magician's fire spell of its level " +
			"(Lava Burst ... Inferno) every 12 s at 25%, and its owner kept in the shaman's STR (Tribal) and DEX (Finesse) " +
			"lines (ours)"},
	{Name: "pet air", Class: 13, HPPct: 80, TakenPct: 115, MeleePct: 100,
		Abilities: []TemplateAbility{
			{Kind: AbilityNuke, Element: "lightning", Every: 9, First: 2, MaxRecast: 10, MaxCast: 5, DamagePct: 12},
			{Kind: AbilityNuke, Element: "fire", Every: 13, First: 5, MaxRecast: 10, MaxCast: 5, DamagePct: 12},
			{Kind: AbilityNuke, Element: "cold", Every: 17, First: 8, MaxRecast: 10, MaxCast: 5, DamagePct: 12},
		},
		Note: "The magician's air pet: a wizard (one hit a round; 20% fewer hit points, 15% more of a melee hit lands) that " +
			"casts three wizard nukes of its level on clocks of their own, lightning (Static Shock ... Electrocution), fire " +
			"(Fire Bolt ... Burning Clash) and cold (Chilling Gaze ... Ice Comet; the Fury line is a mastery's), each at 12%: " +
			"level with the fire pet's damage (balancesim, a +1 with its owner: the Djinn 35 s against the Efreet's 37 at 50, " +
			"the Air Elemental 46 against the Fire Elemental's 41 at 45; at 18% 25 and 33) (ours)"},
	{Name: "pet animation", Class: 12, HPPct: 150, TakenPct: 70, MeleePct: 100,
		Abilities: []TemplateAbility{
			{Kind: AbilityTaunt, Every: 9, First: 1, Hits: [][3]float64{{1, 84, 88}}},
			{Kind: AbilityHeal, HealPct: 45, Every: 38, First: 1},
			{Kind: AbilityHeal, HealPct: 20, Every: 24, First: 1},
			{Kind: AbilityHeal, HealPct: 10, Every: 8, First: 1},
			{Kind: AbilityHeal, HealPct: 5, Every: 5, First: 1},
		},
		Note: "The enchanter's animations, golems and blades, all alike: one hit a round, 50% more hit points and 30% less of " +
			"a melee hit landing; a damaging taunt every 9 s for 84-88 at every level (the user: Storm of Blades' 84-88, " +
			"'the enc pet taunt doesnt scale'); four heals of their own share, 45% (38 s), 20% (24 s), 10% (8 s), 5% (5 s) of its most " +
			"hit points, the biggest due that it is missing (jib's four and the user's cooldowns and rule; the small ones' " +
			"clocks ours); no heal to full"},
}

// SeedPetTemplates are the pets the seeds are given to: every necromancer,
// magician and enchanter pet of the books (classspells), and the magician's
// water elementals the books lack.
var SeedPetTemplates = func() []PetTemplate {
	var out []PetTemplate
	add := func(template string, swingClass, swing, roundMS int, weapon bool, note string, spells ...string) {
		for _, s := range spells {
			out = append(out, PetTemplate{Spell: s, Weapon: weapon, Template: template, SwingClass: swingClass, Swing: swing,
				RoundMS: roundMS, Note: note})
		}
	}
	add("pet necromancer", 0, -1, 0, false, "a necromancer's pet",
		"shambling bones", "stumbling bones", "walking bones", "lumbering bones", "hulking bones", "wilting flesh",
		"decaying flesh", "rotting flesh", "putrid flesh", "preserved flesh", "howling death", "ancient death",
		"swarming bones", "swarming death")
	add("pet necromancer weapon", 0, 4, 0, true, "Death Sacrament's zombie and mummy: Attack2 every 4 s, as Sandstorm's 918 and 939",
		"decaying flesh", "rotting flesh", "putrid flesh", "preserved flesh")
	add("pet necromancer weapon", 0, 7, 5200, true, "Death Sacrament's spectre: Attack3 (5000 ms) every 5.2 s, as Sandstorm's 956",
		"howling death", "ancient death")
	add("pet earth", -1, -1, 0, false, "the magician's earth pet",
		"earth elementaling", "earth elementalkin", "earth elemental", "summon badi", "summon badi elder")
	add("pet water", -1, -1, 0, false, "the magician's water pet (the elementals are in no book here; the Ghilan are)",
		"water elementaling", "water elementalkin", "water elemental", "summon ghilan", "summon ghilan noble")
	add("pet fire", -1, -1, 0, false, "the magician's fire pet",
		"fire elementaling", "fire elementalkin", "fire elemental", "summon efreet", "summon efreeti lord")
	add("pet air", -1, -1, 0, false, "the magician's air pet",
		"air elementaling", "air elementalkin", "air elemental", "summon djinn")
	add("pet animation", -1, -1, 0, false, "an enchanter's pet",
		"pendril's animation", "mircyl's animation", "mercyls animation", "shalee's animation", "gymon's animation",
		"clay golem", "earthen golem", "stone golem", "ferrous golem", "iron golem", "blade swarm", "steel tempest",
		"storm of blades")
	return out
}()

// ensurePetTemplates makes pettemplate (initSchema, after classtemplates).
// Made now, it is given its seed rows, and classtemplates the pets'
// templates (a GM's row of a name kept); made before, it keeps what GMs
// left in it.
func (db *SQLiteDB) ensurePetTemplates() error {
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = 'pettemplate'`).Scan(&n); err != nil {
		return fmt.Errorf("pettemplate: %w", err)
	}
	if _, err := db.Exec(petTemplateSchema); err != nil {
		return fmt.Errorf("pettemplate: %w", err)
	}
	if n > 0 {
		return nil
	}
	for _, t := range SeedPetClassTemplates {
		if _, err := db.Exec(`INSERT OR IGNORE INTO classtemplates (name, class, hp_pct, taken_pct, melee_pct, abilities, note, source)
			VALUES (?, ?, ?, ?, ?, ?, ?, 'seed')`, t.Name, t.Class, t.HPPct, t.TakenPct, t.MeleePct,
			TemplateAbilitiesJSON(t.Abilities), t.Note); err != nil {
			return fmt.Errorf("classtemplates %q: %w", t.Name, err)
		}
	}
	for _, p := range SeedPetTemplates {
		if _, err := db.Exec(`INSERT OR IGNORE INTO pettemplate (spell, weapon, template, swing_class, swing, round_ms, note, source)
			VALUES (?, ?, ?, ?, ?, ?, ?, 'seed')`, p.Spell, p.Weapon, p.Template, p.SwingClass, p.Swing, p.RoundMS, p.Note); err != nil {
			return fmt.Errorf("pettemplate %q: %w", p.Spell, err)
		}
	}
	return nil
}

// PetTemplates is pettemplate's rows by key; bad says each row it could not
// use and why (it is left out).
func (db *SQLiteDB) PetTemplates() (rows map[PetTemplateKey]PetTemplate, bad []string, err error) {
	q, err := db.Query(`SELECT spell, weapon, template, swing_class, swing, round_ms, note FROM pettemplate`)
	if err != nil {
		return nil, nil, err
	}
	defer q.Close()
	rows = map[PetTemplateKey]PetTemplate{}
	for q.Next() {
		var p PetTemplate
		var weapon int
		if err := q.Scan(&p.Spell, &weapon, &p.Template, &p.SwingClass, &p.Swing, &p.RoundMS, &p.Note); err != nil {
			return nil, nil, err
		}
		p.Spell, p.Template, p.Weapon = TemplateKey(p.Spell), TemplateKey(p.Template), weapon != 0
		if err := CheckPetTemplate(p); err != nil {
			bad = append(bad, fmt.Sprintf("%q (weapon %v): %v", p.Spell, p.Weapon, err))
			continue
		}
		rows[PetTemplateKey{p.Spell, p.Weapon}] = p
	}
	return rows, bad, q.Err()
}

// PetWeaponSpells is the summoning spells that are a "with weapons" or
// "with a weapon" version (spellpattern's description says so: the
// necromancer's Death Sacrament pets), by id.
func (db *SQLiteDB) PetWeaponSpells() (map[int]bool, error) {
	q, err := db.Query(`SELECT spellid, spelldesc FROM spellpattern
		WHERE lower(spelldesc) LIKE '%with weapon%' OR lower(spelldesc) LIKE '%with a weapon%'`)
	if err != nil {
		return nil, err
	}
	defer q.Close()
	out := map[int]bool{}
	for q.Next() {
		var id int
		var desc string
		if err := q.Scan(&id, &desc); err != nil {
			return nil, err
		}
		if d := strings.ToLower(desc); strings.Contains(d, "summon") {
			out[id] = true
		}
	}
	return out, q.Err()
}
