package database

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strings"
)

// classtemplates and npctemplate: the NPCs' class templates
// (entity/classtemplate.go; jib's F3 20261029-025234-7430ece1: "Monsters need
// classes. Classes should determine stat profile ... These are example
// templates. We need templates ready so we can apply them to mobs in the
// future, do not apply them yet").
//
// classtemplates holds a template a row:
//
//   - name, its key (TemplateKey: lower case, trimmed);
//   - class, the class an NPC of the template fights as (WAR=0 ... ALC=14):
//     its melee swing (CLIENT's GetSwing, entity/rounds.go) and the class
//     whose spells (classspells) its abilities pick from; -1 keeps the NPC's
//     own (npcclass);
//   - its stat profile, each a percent of what its level gives: hp_pct its
//     maximum hit points, taken_pct the share of a player's or a pet's melee
//     hit on it that lands (its armour: 85 takes 15% less), melee_pct its own
//     melee hits;
//   - its special melee (a kick): special its name ("" none), special_chance
//     the percent of its rounds it comes in, special_damage its damage a
//     level, fixed and through any armour as Sandstorm's creatures' specials
//     are, special_fx its 0x00B1 visual (0: Sandstorm's creature special,
//     0x66CD778E); and on a clock as well (the user, 2026-10-30: SOE's
//     roamers were warriors "stripped down to just a 'crit' attack once
//     every 30 secs or so"): special_every seconds from one to the next (0
//     none), the first special_first seconds into the fight, each of them
//     special_jitter seconds either way, counting its time at its enemy
//     only (not closing in, rooted away or casting); and either only from special_from
//     on, a level (0 any: the roamer's 10, the user's "they get it around
//     level 10 starting, but before only auto attack"), the creature's own
//     at each fight;
//   - abilities, a JSON list of TemplateAbility: what it casts in a fight;
//   - note; source gm, or seed for the rows the table is made with.
//
// npctemplate gives the NPCs of a NAME a template: name as npcclass keys one
// (NPCClassKey), template a classtemplates name. With no row there, no NPC
// fights by a template; the table is made empty and nothing fills it.
const classTemplatesSchema = `
	CREATE TABLE IF NOT EXISTS classtemplates (
		name           TEXT PRIMARY KEY,
		class          INTEGER NOT NULL DEFAULT -1,
		hp_pct         INTEGER NOT NULL DEFAULT 100,
		taken_pct      INTEGER NOT NULL DEFAULT 100,
		melee_pct      INTEGER NOT NULL DEFAULT 100,
		special        TEXT NOT NULL DEFAULT '',
		special_chance INTEGER NOT NULL DEFAULT 0,
		special_damage REAL NOT NULL DEFAULT 0,
		special_fx     INTEGER NOT NULL DEFAULT 0,
		special_every  REAL NOT NULL DEFAULT 0,
		special_first  REAL NOT NULL DEFAULT 0,
		special_jitter REAL NOT NULL DEFAULT 0,
		special_from   INTEGER NOT NULL DEFAULT 0,
		abilities      TEXT NOT NULL DEFAULT '[]',
		note           TEXT NOT NULL DEFAULT '',
		source         TEXT NOT NULL DEFAULT 'gm'
	);`

const npcTemplateSchema = `
	CREATE TABLE IF NOT EXISTS npctemplate (
		name     TEXT PRIMARY KEY,
		template TEXT NOT NULL DEFAULT '',
		note     TEXT NOT NULL DEFAULT '',
		source   TEXT NOT NULL DEFAULT 'gm'
	);`

// The kinds of ability a template's NPC casts, each picked from its class's
// spells (entity/classtemplate.go): a hit on one target, a hit on an area, a
// hit that heals its caster, a heal on itself.
const (
	AbilityNuke    = "nuke"
	AbilityAoE     = "aoe"
	AbilityLifetap = "lifetap"
	AbilityHeal    = "heal"
)

// AbilityKinds are the kinds, in the order the help lists them.
var AbilityKinds = []string{AbilityNuke, AbilityAoE, AbilityLifetap, AbilityHeal}

// MaxTemplateAbilities is the most abilities a template has.
const MaxTemplateAbilities = 8

// TemplateAbility is one of a template's abilities, as its JSON says it. A
// field left out (or 0) is its default.
type TemplateAbility struct {
	// Kind is what it casts: nuke, aoe, lifetap or heal. The spell is the
	// highest of that kind the template's class has at or below the NPC's
	// level (classspells; a mastery's, an epic's or a master class's never).
	Kind string `json:"kind"`
	// Spell pins the spell by its name (any class's), whatever the level.
	Spell string `json:"spell,omitempty"`
	// Line is words the spell's name must hold (a spell line by name:
	// "storm", "tap"); empty, any of its kind.
	Line string `json:"line,omitempty"`
	// Every is the seconds from one of its casts' start to its next: the
	// NPC's own recast for it (the spell's own is not used).
	Every float64 `json:"every,omitempty"`
	// First is the seconds into a fight before its first cast.
	First float64 `json:"first,omitempty"`
	// Chain casts it as soon as the ability before it in the list is cast
	// (that cast's time over), instead of on a clock of its own: the
	// wizard's area spell and then its single target's.
	Chain bool `json:"chain,omitempty"`
	// Chance is the percent of its due casts that go off (0: 100).
	Chance int `json:"chance,omitempty"`
	// DamagePct is its spell's figures, hits and heals, in percent (0: 100).
	DamagePct int `json:"damage_pct,omitempty"`
	// MaxRecast leaves out the spells whose own recast is longer, in
	// seconds (0: any): a wizard's 180 s novas.
	MaxRecast float64 `json:"max_recast,omitempty"`
	// MaxCast leaves out the spells whose own cast time is longer, in
	// seconds (0: any): the wizard's Phoenix Glory, 15 s.
	MaxCast float64 `json:"max_cast,omitempty"`
	// Below is a heal's: cast when the NPC's hit points are below this
	// percent of its maximum (0: 50).
	Below int `json:"below,omitempty"`
	// From is the NPC level from which it casts it at all (0: any).
	From int `json:"from,omitempty"`
}

// ChanceOr is the ability's chance, its default (100) for none.
func (a TemplateAbility) ChanceOr() int {
	if a.Chance <= 0 {
		return 100
	}
	return a.Chance
}

// DamageOr is the ability's figures' percent, its default (100) for none.
func (a TemplateAbility) DamageOr() int {
	if a.DamagePct <= 0 {
		return 100
	}
	return a.DamagePct
}

// BelowOr is a heal's threshold, its default (50) for none.
func (a TemplateAbility) BelowOr() int {
	if a.Below <= 0 {
		return 50
	}
	return a.Below
}

// ClassTemplate is one row of classtemplates, read.
type ClassTemplate struct {
	Name          string // TemplateKey
	Class         int    // -1: the NPC's own (npcclass)
	HPPct         int
	TakenPct      int
	MeleePct      int
	Special       string
	SpecialChance int
	SpecialDamage float64
	SpecialFX     uint32 // 0: Sandstorm's creature special
	// SpecialEvery, SpecialFirst and SpecialJitter are the timed special's
	// clock in seconds: one every SpecialEvery (0: none), the first
	// SpecialFirst into the fight, each SpecialJitter either way
	SpecialEvery, SpecialFirst, SpecialJitter float64
	// SpecialFrom is the level a creature specials from (0: any), its own
	SpecialFrom int
	Abilities   []TemplateAbility
	Note        string
}

// TemplateKey is a template's name as classtemplates and npctemplate key it:
// lower case, trimmed.
func TemplateKey(name string) string {
	return strings.ToLower(strings.TrimSpace(name))
}

// ParseTemplateAbilities reads a template's abilities column: a JSON list of
// TemplateAbility (empty: none). A field it does not know is refused, so a
// misspelt one is not silently its default; each ability is then held to
// CheckTemplateAbilities.
func ParseTemplateAbilities(text string) ([]TemplateAbility, error) {
	text = strings.TrimSpace(text)
	if text == "" || text == "null" {
		return nil, nil
	}
	dec := json.NewDecoder(strings.NewReader(text))
	dec.DisallowUnknownFields()
	var list []TemplateAbility
	if err := dec.Decode(&list); err != nil {
		return nil, fmt.Errorf("abilities: not a JSON list of abilities: %v", err)
	}
	if dec.More() {
		return nil, fmt.Errorf("abilities: text after the list")
	}
	for i := range list {
		list[i].Kind = strings.ToLower(strings.TrimSpace(list[i].Kind))
		list[i].Spell = strings.TrimSpace(list[i].Spell)
		list[i].Line = strings.TrimSpace(list[i].Line)
	}
	if err := CheckTemplateAbilities(list); err != nil {
		return nil, err
	}
	return list, nil
}

// TemplateAbilitiesJSON is abilities as the column keeps them.
func TemplateAbilitiesJSON(list []TemplateAbility) string {
	if len(list) == 0 {
		return "[]"
	}
	var b bytes.Buffer
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(list); err != nil {
		return "[]"
	}
	return strings.TrimSpace(b.String())
}

// CheckTemplateAbilities is what a template's abilities must be for an NPC to
// fight by them: at most MaxTemplateAbilities, each of a kind it knows, on a
// clock of 2 s to an hour unless it chains (and the first cannot: nothing is
// before it), its percents and seconds within bounds.
func CheckTemplateAbilities(list []TemplateAbility) error {
	if len(list) > MaxTemplateAbilities {
		return fmt.Errorf("abilities: %d, at most %d", len(list), MaxTemplateAbilities)
	}
	finite := func(v float64) bool { return !math.IsNaN(v) && !math.IsInf(v, 0) }
	for i, a := range list {
		n := i + 1
		switch {
		case !knownKind(a.Kind):
			return fmt.Errorf("ability %d: kind %q: one of %s", n, a.Kind, strings.Join(AbilityKinds, ", "))
		case a.Chain && i == 0:
			return fmt.Errorf("ability 1 chains, and nothing is before it")
		case !finite(a.Every) || !finite(a.First) || !finite(a.MaxRecast) || !finite(a.MaxCast):
			return fmt.Errorf("ability %d: a number that is not one", n)
		case !a.Chain && (a.Every < 2 || a.Every > 3600):
			return fmt.Errorf("ability %d: every %.1f: 2 to 3600 seconds from one cast's start to the next (or chain)", n, a.Every)
		case a.First < 0 || a.First > 3600:
			return fmt.Errorf("ability %d: first %.1f: 0 to 3600 seconds into a fight", n, a.First)
		case a.Chance < 0 || a.Chance > 100:
			return fmt.Errorf("ability %d: chance %d: 0 (100) to 100 percent", n, a.Chance)
		case a.DamagePct < 0 || a.DamagePct > 1000:
			return fmt.Errorf("ability %d: damage_pct %d: 0 (100) to 1000 percent", n, a.DamagePct)
		case a.MaxRecast < 0 || a.MaxRecast > 86400:
			return fmt.Errorf("ability %d: max_recast %.1f: 0 (any) to 86400 seconds", n, a.MaxRecast)
		case a.MaxCast < 0 || a.MaxCast > 60:
			return fmt.Errorf("ability %d: max_cast %.1f: 0 (any) to 60 seconds", n, a.MaxCast)
		case a.Below < 0 || a.Below > 100:
			return fmt.Errorf("ability %d: below %d: 0 (50) to 100 percent", n, a.Below)
		case a.From < 0 || a.From > 100:
			return fmt.Errorf("ability %d: from %d: 0 (any) to 100", n, a.From)
		case len(a.Spell) > 64 || len(a.Line) > 64:
			return fmt.Errorf("ability %d: a spell or line of more than 64 characters", n)
		}
	}
	return nil
}

func knownKind(k string) bool {
	for _, x := range AbilityKinds {
		if k == x {
			return true
		}
	}
	return false
}

// CheckClassTemplate is what a template must be for an NPC to fight by it: a
// name, a class of the fifteen (or -1), its percents within a tenth to ten
// times (a melee taken from 1%), a special's chance 0-100 and, when it comes
// at all, a damage above 0 a level and at most 200, and abilities
// CheckTemplateAbilities takes.
func CheckClassTemplate(t ClassTemplate) error {
	switch {
	case TemplateKey(t.Name) == "":
		return fmt.Errorf("no name")
	case t.Class < -1 || t.Class > 14:
		return fmt.Errorf("class %d: -1 (the NPC's own) or 0 (Warrior) to 14 (Alchemist)", t.Class)
	case t.HPPct < 10 || t.HPPct > 1000:
		return fmt.Errorf("hp_pct %d: 10 to 1000 percent of its level's hit points", t.HPPct)
	case t.TakenPct < 1 || t.TakenPct > 1000:
		return fmt.Errorf("taken_pct %d: 1 to 1000 percent of a melee hit on it", t.TakenPct)
	case t.MeleePct < 10 || t.MeleePct > 1000:
		return fmt.Errorf("melee_pct %d: 10 to 1000 percent of its melee hits", t.MeleePct)
	case t.SpecialChance < 0 || t.SpecialChance > 100:
		return fmt.Errorf("special_chance %d: 0 to 100 percent of its rounds", t.SpecialChance)
	case math.IsNaN(t.SpecialDamage) || t.SpecialDamage < 0 || t.SpecialDamage > 200:
		return fmt.Errorf("special_damage %.2f: 0 to 200 a level", t.SpecialDamage)
	case t.SpecialChance > 0 && t.SpecialDamage <= 0:
		return fmt.Errorf("a special in %d%% of its rounds needs a special_damage above 0", t.SpecialChance)
	case math.IsNaN(t.SpecialEvery) || t.SpecialEvery < 0 || t.SpecialEvery > 3600 || (t.SpecialEvery > 0 && t.SpecialEvery < 5):
		return fmt.Errorf("special_every %.1f: 0 (none) or 5 to 3600 seconds from one special to the next", t.SpecialEvery)
	case math.IsNaN(t.SpecialFirst) || t.SpecialFirst < 0 || t.SpecialFirst > 3600:
		return fmt.Errorf("special_first %.1f: 0 to 3600 seconds into a fight", t.SpecialFirst)
	case math.IsNaN(t.SpecialJitter) || t.SpecialJitter < 0 || (t.SpecialJitter > 0 && 2*t.SpecialJitter >= t.SpecialEvery):
		return fmt.Errorf("special_jitter %.1f: 0 to under half of special_every (%.1f) seconds either way", t.SpecialJitter, t.SpecialEvery)
	case t.SpecialEvery > 0 && t.SpecialDamage <= 0:
		return fmt.Errorf("a special every %.1f s needs a special_damage above 0", t.SpecialEvery)
	case t.SpecialFrom < 0 || t.SpecialFrom > 100:
		return fmt.Errorf("special_from %d: 0 (any level) to 100", t.SpecialFrom)
	case len(t.Special) > 32:
		return fmt.Errorf("special: a name of 32 characters at most")
	}
	return CheckTemplateAbilities(t.Abilities)
}

// SeedClassTemplates are jib's five examples and the user's roamer, the rows
// classtemplates is made with (source seed). Their numbers are ours: no recording has a creature
// casting, and Sandstorm's creatures' specials are the one measured thing
// (the melee's kick: 251 from a level 34 Desert Crab, 252 from a level 35
// Gharial, fixed and through 1,763 AC alike, in about one round of five:
// claude-notes/training-dummies-20261021.md, defence-sandstorm-20261022.md).
var SeedClassTemplates = []ClassTemplate{
	{Name: "tank", Class: 0, HPPct: 130, TakenPct: 85, MeleePct: 100,
		Note: "jib's tank: a little tankier, 30% more hit points and 15% less of a melee hit lands (ours)"},
	{Name: "melee", Class: 4, HPPct: 100, TakenPct: 100, MeleePct: 115, Special: "Kick", SpecialChance: 20, SpecialDamage: 7.4,
		Note: "jib's melee: hits 15% harder and kicks in a fifth of its rounds for 7.4 a level, fixed and through any armour " +
			"(Sandstorm's creature special: a level 34 crab's 251, in about one round of five)"},
	{Name: "wizard", Class: 13, HPPct: 85, TakenPct: 100, MeleePct: 100,
		Abilities: []TemplateAbility{
			{Kind: AbilityAoE, Every: 15, First: 2, MaxRecast: 10, MaxCast: 5, DamagePct: 35},
			{Kind: AbilityNuke, Chain: true, MaxRecast: 10, MaxCast: 5, DamagePct: 35},
		},
		Note: "jib's wizard: chain-casts two wizard spells of its level, an area one and then one of a single target, every " +
			"15 s; the 180 s novas and Phoenix Glory's 15 s cast left out (max_recast 10, max_cast 5), the figures 35% of a player's (ours)"},
	{Name: "necromancer", Class: 11, HPPct: 90, TakenPct: 100, MeleePct: 100,
		Abilities: []TemplateAbility{{Kind: AbilityLifetap, Every: 5, First: 1, DamagePct: 50}},
		Note:      "jib's necromancer: a necromancer life tap of its level every 5 s, healing it for what it takes; the figure half a player's (ours)"},
	{Name: "shadowknight", Class: 3, HPPct: 120, TakenPct: 90, MeleePct: 100,
		Abilities: []TemplateAbility{{Kind: AbilityLifetap, Every: 15, First: 3, MaxRecast: 90, DamagePct: 50}},
		Note: "jib's shadow knight: tankier (20% more hit points, 10% less of a melee hit lands), a shadow knight life tap of " +
			"its level every 15 s (the 300 s Blood line left out: max_recast 90); the figure half a player's (ours)"},
	{Name: "roamer", Class: 0, HPPct: 100, TakenPct: 100, MeleePct: 100, Special: "Critical Hit", SpecialDamage: 7.4,
		SpecialEvery: 30, SpecialFirst: 15, SpecialJitter: 5, SpecialFrom: 10,
		Note: "SOE's basic roamer (the user, 2026-10-30): a warrior \"stripped down to just a 'crit' attack once every 30 " +
			"secs or so\", which \"they get around level 10 starting, but before only auto attack\": from level 10 a " +
			"Critical Hit every 25-35 s from 10-20 s into a fight, 7.4 a level, fixed and through any armour (Sandstorm's " +
			"crab's 251 at 34); tools/seed_roamers.py gives it to the creatures"},
}

// classTemplateColumns are the columns added to classtemplates since it was
// first made (2026-10-29): a table made before gets each at 0, so no template
// has a timed special until a GM gives it one.
var classTemplateColumns = []struct{ col, decl string }{
	{"special_every", "REAL NOT NULL DEFAULT 0"},
	{"special_first", "REAL NOT NULL DEFAULT 0"},
	{"special_jitter", "REAL NOT NULL DEFAULT 0"},
	{"special_from", "INTEGER NOT NULL DEFAULT 0"},
}

// ensureClassTemplates makes tables classtemplates and npctemplate
// (initSchema). A classtemplates made now is given the seeds; one made
// before keeps what GMs left in it, so a seed a GM deleted stays deleted, and
// gets the columns added since (classTemplateColumns).
func (db *SQLiteDB) ensureClassTemplates() error {
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = 'classtemplates'`).Scan(&n); err != nil {
		return fmt.Errorf("classtemplates: %w", err)
	}
	if _, err := db.Exec(classTemplatesSchema); err != nil {
		return fmt.Errorf("classtemplates: %w", err)
	}
	for _, c := range classTemplateColumns {
		has, err := hasColumn(db.DB, "classtemplates", c.col)
		if err != nil {
			return fmt.Errorf("classtemplates.%s: %w", c.col, err)
		}
		if !has {
			if _, err := db.Exec(`ALTER TABLE classtemplates ADD COLUMN ` + c.col + ` ` + c.decl); err != nil {
				return fmt.Errorf("classtemplates.%s: %w", c.col, err)
			}
		}
	}
	if _, err := db.Exec(npcTemplateSchema); err != nil {
		return fmt.Errorf("npctemplate: %w", err)
	}
	if n > 0 {
		return nil
	}
	for _, t := range SeedClassTemplates {
		if _, err := db.Exec(`INSERT OR IGNORE INTO classtemplates (name, class, hp_pct, taken_pct, melee_pct, special,
			special_chance, special_damage, special_fx, special_every, special_first, special_jitter, special_from, abilities,
			note, source) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 'seed')`,
			t.Name, t.Class, t.HPPct, t.TakenPct, t.MeleePct, t.Special, t.SpecialChance, t.SpecialDamage, int64(t.SpecialFX),
			t.SpecialEvery, t.SpecialFirst, t.SpecialJitter, t.SpecialFrom, TemplateAbilitiesJSON(t.Abilities), t.Note); err != nil {
			return fmt.Errorf("classtemplates: the seed %q: %w", t.Name, err)
		}
	}
	return nil
}

// ClassTemplates is every row of classtemplates an NPC can fight by, keyed by
// TemplateKey; a row CheckClassTemplate refuses (or whose abilities are not
// JSON) is left out and named in bad.
func (db *SQLiteDB) ClassTemplates() (templates map[string]ClassTemplate, bad []string, err error) {
	rows, err := db.Query(`SELECT name, class, hp_pct, taken_pct, melee_pct, special, special_chance, special_damage,
		special_fx, special_every, special_first, special_jitter, special_from, abilities, note FROM classtemplates
		WHERE name IS NOT NULL`)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()
	templates = make(map[string]ClassTemplate)
	for rows.Next() {
		var t ClassTemplate
		var fx int64
		var abilities sql.NullString
		if err := rows.Scan(&t.Name, &t.Class, &t.HPPct, &t.TakenPct, &t.MeleePct, &t.Special, &t.SpecialChance,
			&t.SpecialDamage, &fx, &t.SpecialEvery, &t.SpecialFirst, &t.SpecialJitter, &t.SpecialFrom, &abilities, &t.Note); err != nil {
			return nil, nil, err
		}
		t.Name = TemplateKey(t.Name)
		t.Special = strings.TrimSpace(t.Special)
		t.SpecialFX = uint32(fx)
		list, perr := ParseTemplateAbilities(abilities.String)
		if perr == nil {
			t.Abilities = list
			perr = CheckClassTemplate(t)
		}
		if perr != nil {
			bad = append(bad, fmt.Sprintf("%q: %v", t.Name, perr))
			continue
		}
		templates[t.Name] = t
	}
	return templates, bad, rows.Err()
}

// NPCTemplates is npctemplate: each NPC name (NPCClassKey) and the template
// (TemplateKey) its NPCs fight by. A row with no name or no template is left
// out.
func (db *SQLiteDB) NPCTemplates() (map[string]string, error) {
	rows, err := db.Query(`SELECT name, template FROM npctemplate WHERE name IS NOT NULL AND template IS NOT NULL`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make(map[string]string)
	for rows.Next() {
		var name, template string
		if err := rows.Scan(&name, &template); err != nil {
			return nil, err
		}
		name, template = NPCClassKey(name), TemplateKey(template)
		if name != "" && template != "" {
			out[name] = template
		}
	}
	return out, rows.Err()
}

// TemplateSpell is a spell as a template's NPC casts it: its ability record
// whole, its spelleffect line and visual, and its spellarea row if it has one.
type TemplateSpell struct {
	Pattern SpellPatternRecord
	Line    string
	FX      uint32
	Area    SpellAreaRow
	HasArea bool
}

// templateSpellColumns are a TemplateSpell's, spellpattern sp left joined to
// spelleffect se and spellarea sa.
const templateSpellColumns = `sp.spellid, sp.abilitylvl, sp.unk2, sp.unk3, sp.range, sp.casttime, sp.power, sp.iconColor,
	sp.icon, sp.scope, sp.recast, sp.eqprequire, sp.spellname, COALESCE(sp.spelldesc, ''), COALESCE(se.line, ''),
	COALESCE(se.fxdict, 0), sa.centre, sa.radius, sa.maxtargets`

func scanTemplateSpell(scan func(...any) error) (TemplateSpell, error) {
	var s TemplateSpell
	var fx int64
	var centre, maxTargets sql.NullInt64
	var radius sql.NullFloat64
	p := &s.Pattern
	if err := scan(&p.SpellID, &p.AbilityLvl, &p.Unk2, &p.Unk3, &p.Range, &p.CastTime, &p.Power, &p.IconColor, &p.Icon,
		&p.Scope, &p.Recast, &p.EqpRequire, &p.SpellName, &p.SpellDesc, &s.Line, &fx, &centre, &radius, &maxTargets); err != nil {
		return s, err
	}
	s.FX = uint32(fx)
	if centre.Valid {
		s.HasArea = true
		s.Area = SpellAreaRow{Centre: int(centre.Int64), Radius: radius.Float64, MaxTargets: int(maxTargets.Int64)}
	}
	return s, nil
}

// TemplateSpells is every spell a character of the class gets by level (not
// a mastery's, an epic's or a master class's: Granted), by level, then id:
// what a template's abilities pick from.
func (db *SQLiteDB) TemplateSpells(class int) ([]TemplateSpell, error) {
	rows, err := db.Query(`SELECT `+templateSpellColumns+`, COALESCE(cs.source, '')
		FROM classspells cs
		JOIN spellpattern sp ON sp.spellid = cs.spellid
		LEFT JOIN spelleffect se ON se.spellid = sp.spellid
		LEFT JOIN spellarea sa ON sa.spellid = sp.spellid
		WHERE cs.class = ?
		ORDER BY sp.abilitylvl, sp.spellid`, class)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []TemplateSpell
	for rows.Next() {
		var source string
		s, err := scanTemplateSpell(func(dest ...any) error { return rows.Scan(append(dest, &source)...) })
		if err != nil {
			return nil, err
		}
		if Granted(source) {
			out = append(out, s)
		}
	}
	return out, rows.Err()
}

// TemplateSpellNamed is the spell of that name (any case) an ability pins:
// of the records of the name, the one with a spelleffect line first, then
// the lowest id; ok false when there is none.
func (db *SQLiteDB) TemplateSpellNamed(name string) (TemplateSpell, bool, error) {
	row := db.QueryRow(`SELECT `+templateSpellColumns+`
		FROM spellpattern sp
		LEFT JOIN spelleffect se ON se.spellid = sp.spellid
		LEFT JOIN spellarea sa ON sa.spellid = sp.spellid
		WHERE lower(trim(sp.spellname)) = ?
		ORDER BY (COALESCE(se.line, '') = ''), sp.spellid LIMIT 1`, strings.ToLower(strings.TrimSpace(name)))
	s, err := scanTemplateSpell(row.Scan)
	if errors.Is(err, sql.ErrNoRows) {
		return TemplateSpell{}, false, nil
	}
	if err != nil {
		return TemplateSpell{}, false, err
	}
	return s, true, nil
}

// RangeBits is the ability record's range as the client reads it (a float's
// bits): range in the high half, the two small fields' zigzag bytes below
// (handlers' abilityRangeBits, writeAbilityRecord's).
func (sp *SpellPatternRecord) RangeBits() uint32 {
	lowByte := func(v int) uint32 {
		if v < 0 || v > 63 {
			return 0
		}
		return uint32(v << 1) // zigzag of a small non-negative value, one byte
	}
	return uint32(uint16(sp.Range))<<16 | lowByte(sp.Unk3)<<8 | lowByte(sp.Unk2)
}
