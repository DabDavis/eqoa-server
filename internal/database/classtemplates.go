package database

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"slices"
	"strconv"
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
		shield_damage  INTEGER NOT NULL DEFAULT 0,
		shield_resist  TEXT NOT NULL DEFAULT '',
		proc_chance    INTEGER NOT NULL DEFAULT 0,
		proc_damage    INTEGER NOT NULL DEFAULT 0,
		proc_name      TEXT NOT NULL DEFAULT '',
		proc_resist    TEXT NOT NULL DEFAULT '',
		resist_fail    REAL NOT NULL DEFAULT 0,
		resist_kind    TEXT NOT NULL DEFAULT '',
		resist_pct     INTEGER NOT NULL DEFAULT 0,
		kick_damage    INTEGER NOT NULL DEFAULT 0,
		kick_every     REAL NOT NULL DEFAULT 0,
		kick_first     REAL NOT NULL DEFAULT 0,
		fight_regen    INTEGER NOT NULL DEFAULT 0,
		run_speed      REAL NOT NULL DEFAULT 0,
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
// hit that heals its caster, a heal on itself. A pet of a template
// (pettemplate, entity/pettemplate.go) casts them too, and kinds of its own: a
// damaging taunt at what it fights, a buff on itself (or its owner), a root
// and a damage over time on what it fights.
const (
	AbilityNuke    = "nuke"
	AbilityAoE     = "aoe"
	AbilityLifetap = "lifetap"
	AbilityHeal    = "heal"
	AbilityTaunt   = "taunt"
	AbilityBuff    = "buff"
	// AbilityRoot is a pet's root at what it fights, and round it for an
	// area spell (the workbook's Djinni Vizier's Air Root: entity/pettemplate.go)
	AbilityRoot = "root"
	// AbilityDebuff is an NPC's lasting harm on the player it fights: the stats
	// its named spell takes, for the spell's time (entity/classtemplate.go).
	AbilityDebuff = "debuff"
	// AbilityStrip is an NPC's cast that takes every beneficial effect off what
	// it fights, a player or a pet (its buffs, damage shields, heals over time,
	// invulnerabilities and mitigations, speeds, forms), the harmful left on,
	// and a GM's #testmode (the server's god mode) off (entity/strip.go; the GM
	// Jibrish's petition 1368: Mr. Moogie's "caster debuff on his target every
	// 30 seconds that strips any and all buffs, including immunity to damage /
	// god mode"). It casts no spell of a book: Name is what its lines call it
	// (DefaultStripName for none), or it pins a spell (Spell or SpellID) whose
	// name, cast time and visual it takes, nothing else of it.
	AbilityStrip = "strip"
	// AbilityHealShield casts nothing: while its hit points are below its share
	// (Below; 100 always), a melee hit on it heals it and 1 lands, a player's or a
	// pet's alike; an art is a spell and lands as ever (the guide's Rallos Zek: "heal
	// shield", rogues' arts still "doing significant damage")
	AbilityHealShield = "healshield"
	// AbilityMeleeKind casts nothing: the share (TakenPct) of a player's melee of a
	// kind of weapon (MeleeKind: slash, pierce, blunt) that lands (the same: "he
	// resists slash pretty good by over 50%")
	AbilityMeleeKind = "meleekind"
	// AbilityDoT is a pet's damage over time on what it fights, kept up: cast
	// again as the one on its target runs out, so the new one lands as the old
	// ends, never two of its owner's on one creature (entity/pettemplate.go; the
	// GM Jibrish's petition 1388: the Swarming Deaths' "300 a tick dot (.5
	// second cast) it keeps up that lasts for 24 seconds"). A creature's is the
	// same on what it fights, a player or a creature, kept up as a pet's is
	// (entity/creaturedot.go; the owner, 2026-11-10: Innoruuk's of the Red
	// Swarming Death's kit).
	AbilityDoT = "dot"
	// AbilityHitHeal casts nothing: every hit that lands on its creature heals
	// it Heal hit points, the hit landing as ever (entity/hitheal.go; the GM
	// Jibrish's petition 1439: Mr. Moogie's "heal damage shield that heals mr
	// moogie for 150 every single time he is hit").
	AbilityHitHeal = "hitheal"
	// AbilityPrefer casts nothing: the foes (Foes, by name) its creature fights
	// first whenever one is on its hate list (entity/prefer.go; petition 1438:
	// Mr. Moogie and Tunare).
	AbilityPrefer = "prefer"
	// AbilitySummon calls Count copies of the NPC named NPC beside its creature,
	// once a fight, when its hit points first fall below Below percent, each
	// joining its fight; with Share each fights by its creature's own template
	// at that share of its figures (entity/summon.go; petition 1440: Mr.
	// Moogie's three Servants of Moogie at 25%).
	AbilitySummon = "summon"
	// AbilityEnrage is once a fight, below Below percent: its size Grow percent
	// more, every Every seconds the players of its hate list beyond its reach
	// brought beside it, and Rampage percent of each melee hit on the one it
	// fights landing on the second of its hate list (entity/enrage.go;
	// petition 1443: Mr. Moogie at 10%).
	AbilityEnrage = "enrage"
	// AbilityProc casts nothing: a melee proc its creature always has, on
	// every fight's hits that land, beside its template's: its pinned spell's
	// proc (a pet's buff: the owner's Innoruuk with Largesse of Plague) at the
	// chance a pet of its level has under it, or ProcChance, ProcDamage,
	// ProcName and Resist of its own (entity/creatureproc.go). FX is the look
	// of each proc's hit on its target; a pinned spell is a buff its creature
	// casts on itself as its fight begins and again at the buff's time.
	AbilityProc = "proc"
	// AbilityVeil casts nothing: each of its creature's melee hits that lands
	// also takes TapPct percent of the creature's own most hit points from what
	// it hit, into itself, cut by Resist (entity/creatureproc.go; the owner,
	// 2026-11-10: Innoruuk "similar to veil of death, but scale it to innoruuks
	// hp"). FX is each tap's look on its target (none for none); a pinned spell
	// names it and is a buff cast on itself, as a proc's.
	AbilityVeil = "veil"
	// AbilityKillSay casts nothing: its creature's line in /say (Say) when one of
	// its hits kills one of Foes (by name: the GM Jibrish's petition 1458, Mr.
	// Moogie at Innoruuk's death, "Dab got dunked again").
	AbilityKillSay = "killsay"
	// AbilityLastStand casts nothing: once a fight, when its creature's hit
	// points are at Below percent or under while a non-player is hitting it (an
	// NPC alive on its hate list: a pet, a mercenary, a creature a trigger set on
	// it), it heals to its full and its damage is DamagePct percent (200:
	// doubled) for the rest of that fight; its line in /say (Say)
	// (entity/laststand.go; the GM Jibrish's petition 1522: Mr. Moogie "If he
	// reaches 2% hp he completely heals to full and doubles his damage if a non
	// player character is hitting him").
	AbilityLastStand = "laststand"
)

// A boss's moments on a clock of its own (entity/bossmoments.go; the
// Cazic-Thule trials' Watchman, work board 616: "Casts a MASSIVE DoT on
// himself when he spawns", "Sometimes casts a 10,000 heal on your raid party
// members", "Casts a 0 Power Debuff on Target who has agro when the Watchman
// is engaged"): each every Every seconds of a fight, the first First into it,
// casting no spell.
const (
	// AbilitySelfDoT: Damage hit points off itself each time (never below 1)
	AbilitySelfDoT = "selfdot"
	// AbilityRaidHeal: Damage hit points onto Count players of its hate list
	// (at random, alive, within 60 of it)
	AbilityRaidHeal = "raidheal"
	// AbilityPowerZero: the power of the player it fights set to 0
	AbilityPowerZero = "powerzero"
	// AbilitySetHP: the hit points of the one it fights (Radius 0), or of
	// everyone round it within Radius (players and creatures alike, its own
	// side too, itself never), taken down to SetHP: never raised, never to
	// death (entity/bossmoments.go; the owner, 2026-11-13: "dreadnaughts are
	// supposed to have an AoE which reduces hp to 700 for players and mobs.
	// Cazic Thule sets to 10hp for his debuff")
	AbilitySetHP = "sethp"
)

// BossMomentKind is whether an ability is one of a boss's moments.
func BossMomentKind(k string) bool {
	return k == AbilitySelfDoT || k == AbilityRaidHeal || k == AbilityPowerZero || k == AbilitySetHP
}

// MostSetHPRadius is the widest a sethp's area reaches.
const MostSetHPRadius = 200

// AbilityKinds are the kinds, in the order the help lists them.
var AbilityKinds = []string{AbilityNuke, AbilityAoE, AbilityLifetap, AbilityHeal, AbilityTaunt, AbilityBuff, AbilityRoot, AbilityDebuff,
	AbilityStrip, AbilityHealShield, AbilityMeleeKind, AbilityDoT, AbilityHitHeal, AbilityPrefer, AbilitySummon, AbilityEnrage,
	AbilityProc, AbilityVeil, AbilityKillSay, AbilityLastStand, AbilitySelfDoT, AbilityRaidHeal, AbilityPowerZero, AbilitySetHP}

// passiveKind is whether an ability of the kind casts nothing on a clock of its
// own: what it does is a rule its creature keeps (a heal shield, a melee kind,
// a hit heal, its preferred foes, a proc, a veil, a line at a kill) or a moment
// of its fight (a summon and an enrage, below their share of hit points).
func passiveKind(k string) bool {
	switch k {
	case AbilityHealShield, AbilityMeleeKind, AbilityHitHeal, AbilityPrefer, AbilitySummon, AbilityEnrage, AbilityProc, AbilityVeil,
		AbilityKillSay, AbilityLastStand:
		return true
	}
	return false
}

// DefaultStripName is what a strip's lines call it when it has no name and
// pins no spell (ours), and StripNameMost the most characters of a name;
// DefaultShareHealName a creature's heal of its share's, DefaultFightProcName
// the proc an ability's landing gives (ours both). SayMost is the most
// characters of an ability's line in /say.
const (
	DefaultStripName     = "Dispel"
	StripNameMost        = 32
	DefaultShareHealName = "Heal"
	DefaultFightProcName = "Fury"
	SayMost              = 160
	// DefaultFixedHitName is what a fixed hit's lines call it when it has no
	// name and pins no spell, DefaultVeilName a veil's, DefaultRampageName an
	// enrage's rampage (ours all)
	DefaultFixedHitName = "Strike"
	DefaultVeilName     = "Veil"
	DefaultRampageName  = "Rampage"
	// DefaultOutOfReachRange is how far an out-of-reach ability reaches when
	// it says no range of its own (ours: past every spell's 30, as far as a
	// player kites a creature it has rooted)
	DefaultOutOfReachRange = 100
	// MostSummoned is the most adds a summon calls, MostFoes the most names a
	// prefer holds
	MostSummoned = 10
	MostFoes     = 8
)

// Who a pet's heal or buff lands on (TemplateAbility.Target); either is a
// heal's alone: its owner or itself, whichever is the lower share of its hit
// points below its threshold (a workbook pet's "Owner/Self" heal); group is a
// heal's alone too: everyone of its side in its spell's reach of it at once
// (itself, its owner, its owner's group's players, their pets and mercenaries:
// entity/petgroupheal.go, petition 1495's "AOE heal ... that heals players and
// pets in range"), cast when any of them is below its threshold; member is a
// heal's alone as well: the worst off (the lowest share of its hit points) of
// that same side in reach, below its threshold, the owner wherever it stands
// (entity/petmemberheal.go; the user, 2026-11-11: "lets allow mage water pets
// to heal other group/raid members also on heirloom server", whose
// world.pet_heal_group makes a magician's water pet's owner and either heals
// this).
const (
	AbilityOnOwner  = "owner"
	AbilityOnSelf   = "self"
	AbilityOnEither = "either"
	AbilityOnGroup  = "group"
	AbilityOnMember = "member"
)

// AbilityStats are the stats a buff may be picked by (TemplateAbility.Stat);
// "ds" is a damage shield's (entity/dshield.go: a spell with one).
var AbilityStats = []string{"ac", "str", "sta", "agi", "dex", "wis", "int", "cha", "hp", "power", "ds"}

// AbilityElements are the elements a harmful spell may be picked by
// (TemplateAbility.Element), as its resist names it.
var AbilityElements = []string{"fire", "cold", "lightning", "poison", "disease", "arcane"}

// MaxTemplateAbilities is the most abilities a template has (8 until Mr.
// Moogie's ninth, petition 1522).
const MaxTemplateAbilities = 12

// TemplateAbility is one of a template's abilities, as its JSON says it. A
// field left out (or 0) is its default.
type TemplateAbility struct {
	// Kind is what it casts: nuke, aoe, lifetap or heal. The spell is the
	// highest of that kind the template's class has at or below the NPC's
	// level (classspells; a mastery's, an epic's or a master class's never).
	Kind string `json:"kind"`
	// Spell pins the spell by its name (any class's), whatever the level.
	Spell string `json:"spell,omitempty"`
	// SpellID pins it by its id instead (petstats: a workbook pet's own
	// spells, which no class's book has).
	SpellID int `json:"spell_id,omitempty"`
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
	// Cast is the seconds its spell takes to cast, in place of the record's
	// own whole seconds (0: the record's): the Swarming Deaths' half-second
	// damage over time (petition 1388).
	Cast float64 `json:"cast,omitempty"`
	// Below is a heal's: cast when the NPC's hit points are below this
	// percent of its maximum (0: 50).
	Below int `json:"below,omitempty"`
	// From is the NPC level from which it casts it at all (0: any).
	From int `json:"from,omitempty"`

	// The fields below are a pet's (pettemplate); an NPC of a template
	// casts no taunt or buff, and its heals are on itself.

	// Target is who a pet's heal or buff lands on: "self" (the default:
	// the user, 2026-10-30, "the earth pet ... is casting the ac buff on the
	// player instead of itself") or "owner"; a heal's also "either", "group"
	// or "member" (AbilityOnEither and the rest).
	Target string `json:"target,omitempty"`
	// SpellsOf is the class (0-14) whose spells it picks from in place of
	// the template's: a pet's buff from a druid's or a shaman's line.
	SpellsOf *int `json:"spells_of,omitempty"`
	// Classes is the classes (0-14) of the creatures that cast it, for a
	// template of the creature's own class (class -1): a warrior of the
	// dungeons' tiers casts no nuke of its book (its arts) where their wizards
	// do (petition 1900). Empty: every class.
	Classes []int `json:"classes,omitempty"`
	// Stat is the stat a buff's spell must raise (AbilityStats).
	Stat string `json:"stat,omitempty"`
	// Element is the element a harmful spell must be of (AbilityElements,
	// as its resist goes: entity/classtemplate.go templateResist); empty,
	// any.
	Element string `json:"element,omitempty"`
	// HealPct is a heal of this percent of its target's most hit points in
	// place of a spell's figure: the enchanter's animations' four. A pet's
	// heals of its own share are cast by size: the biggest due whose share
	// it is missing, no overheal (entity/pettemplate.go).
	HealPct int `json:"heal_pct,omitempty"`
	// Free is a charmed pet's cast of it that costs it no power (entity/
	// charm.go: a charmed creature's spells cost it their power, the wiki's
	// Nasehir wizards "chain cast, never run OOM"); a summoned pet's never cost.
	Free bool `json:"free,omitempty"`
	// WhileOut is a buff on its owner that lasts only while the pet is out: it
	// comes off its owner when the pet goes (dismissed, dead, replaced; entity/
	// pet.go dismissPetLocked). The Undead Servant's (petition 1805, Skywalker:
	// "The pet Buffs 300 Hp and 300 Power along with a 80 lifetap proc").
	WhileOut bool `json:"while_out,omitempty"`
	// Hits is a taunt's damage by the pet's level: points [level, least,
	// most], a level between two the straight line between them, one below
	// the first the first's and one past the last the last's (one point: the
	// same at every level); it lands as it is, as a creature's special does
	// (the balance's pet damage not taken), at least 1. Empty: one of its
	// melee hits at damage_pct.
	Hits [][3]float64 `json:"hits,omitempty"`
	// NoCure is a debuff no cure takes off, a spell's or a potion's: it ends
	// with its time or its bearer's death.
	NoCure bool `json:"no_cure,omitempty"`
	// MeleeKind and TakenPct: AbilityMeleeKind's.
	MeleeKind string `json:"melee_kind,omitempty"`
	TakenPct  int    `json:"taken_pct,omitempty"`
	// Name is a strip's name in its lines (AbilityStrip; empty:
	// DefaultStripName), or a creature's heal of its share's (HealPct; empty:
	// DefaultShareHealName), for one that pins no spell.
	Name string `json:"name,omitempty"`

	// The fields below are a creature's (entity/abilitytimes.go; the GM
	// Jibrish's petition 1394: Mr. Moogie's "1 hour cool down complete heal at
	// 50% health, and if he uses it he says ... After Lh he also gets a proc
	// buff on melee attacks"); a pet's turn uses none of them.

	// Cooldown is the least seconds from one cast of it to the next across its
	// creature's fights: kept on the creature, not its fight, so a fight's end,
	// its death and a restart that keeps the sessions leave it running (0:
	// none, its clock alone: Every, begun again with each fight).
	Cooldown float64 `json:"cooldown,omitempty"`
	// Say is its creature's line in /say as it lands ("Name: text", heard as a
	// say is).
	Say string `json:"say,omitempty"`
	// ProcChance and ProcDamage are a melee proc its landing gives its creature
	// for the rest of that fight (its end or the creature's death takes it):
	// ProcChance percent of its melee hits that land deal ProcDamage more,
	// fixed, told by ProcName (DefaultFightProcName for none).
	ProcChance int    `json:"proc_chance,omitempty"`
	ProcDamage int    `json:"proc_damage,omitempty"`
	ProcName   string `json:"proc_name,omitempty"`

	// The fields below are a creature's of 2026-11-10 (the GM Jibrish's
	// petitions 1439-1443 and the owner's Innoruuk).

	// Damage is a nuke's or a lifetap's hit as a fixed figure in place of its
	// spell's figures (its damage_pct unused): it lands as it is but for
	// Resist's share, a lifetap's all of it back on its creature (petition
	// 1439: Mr. Moogie's 4,000; 1442: Metal Moogie V2's 2,000 at range). Its
	// spell, or Name and FX, is its look.
	Damage int `json:"damage,omitempty"`
	// Resist is the resist that cuts a fixed figure (Damage, a veil's tap, a
	// proc of its own figures): FR, CR, LR, AR, PR or DR; empty: none.
	Resist string `json:"resist,omitempty"`
	// FX is the 0x00B1 visual of an ability that pins no spell (a named strip,
	// heal of its share, fixed hit, proc or veil) in place of its kind's
	// stand-in; 0: the stand-in.
	FX uint32 `json:"fx,omitempty"`
	// OutOfReach casts it only while what its creature fights stands beyond its
	// melee reach (rooted, snared, kiting), within Range of it and in its sight;
	// never in reach, its clock waiting, due, meanwhile (petition 1442).
	OutOfReach bool `json:"out_of_reach,omitempty"`
	// Range is an out-of-reach ability's reach (0: DefaultOutOfReachRange).
	Range float64 `json:"range,omitempty"`
	// Heal is a hit heal's hit points at every hit that lands on its creature.
	Heal int `json:"heal,omitempty"`
	// Count, NPC and Share are a summon's: how many it calls, the name of the
	// NPC each is a copy of (a spawn-only original as a trigger's spawn is), and
	// the share of its creature's figures each fights by (0: the copy's own).
	Count int    `json:"count,omitempty"`
	NPC   string `json:"npc,omitempty"`
	Share int    `json:"share,omitempty"`
	// Grow and Rampage are an enrage's: how many percent bigger it grows (200:
	// three times its size), and the percent of each of its melee hits on the
	// one it fights that also lands on the second of its hate list (0 none).
	Grow    int `json:"grow,omitempty"`
	Rampage int `json:"rampage,omitempty"`
	// Foes are a prefer's: names (any case) of the foes it fights first, the
	// first of them first; and a killsay's: those whose death by its hit its
	// line is for.
	Foes []string `json:"foes,omitempty"`
	// TapPct is a veil's: the percent of its creature's own most hit points each
	// of its melee hits that lands takes from what it hit, into itself.
	TapPct float64 `json:"tap_pct,omitempty"`
	// SetHP and Radius are a sethp's (AbilitySetHP): the hit points it takes
	// down to (1 or more), and how far round its creature it reaches (0: the
	// one it fights alone).
	SetHP  int     `json:"set_hp,omitempty"`
	Radius float64 `json:"radius,omitempty"`
	// Tick, TickAtCap and TickFor are a sethp's damage over time on everyone
	// it reaches (the owner, 2026-11-13: the dreadnaught's "aoe that debuffs
	// to 700hp also has a damage over time ticks 650 with no CR, with max CR
	// is ticking around 80"): Tick hit points a tick at no resist (0: none),
	// TickAtCap at a resist (Resist) at the cap (ResistCap's 315; 0: the
	// server's share there, 90%), in proportion between, every tick (6 s)
	// for TickFor seconds (0: 24, a tick line's with no time).
	Tick      int     `json:"tick,omitempty"`
	TickAtCap int     `json:"tick_at_cap,omitempty"`
	TickFor   float64 `json:"tick_for,omitempty"`
}

// RangeOr is an out-of-reach ability's reach: its own, else
// DefaultOutOfReachRange.
func (a TemplateAbility) RangeOr() float64 {
	if a.Range <= 0 {
		return DefaultOutOfReachRange
	}
	return a.Range
}

// TargetOr is who a pet's heal or buff lands on: its own, else the pet
// itself.
func (a TemplateAbility) TargetOr() string {
	if a.Target != "" {
		return a.Target
	}
	return AbilityOnSelf
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
	// ShieldDamage is its damage shield: what each melee hit that lands on it
	// costs the attacker (0: none, entity/creatureshield.go), cut by the resist
	// ShieldResist names (FR, CR, LR, AR, PR or DR; empty: none)
	ShieldDamage int
	ShieldResist string
	// ProcChance is its melee proc: the percent of its melee hits that land that
	// also deal ProcDamage (a fixed figure, not by level), cut by the resist
	// ProcResist names (as ShieldResist; empty: none), told by ProcName
	// (entity/creatureshield.go). 0: none. A pet's template: the pet's own melee
	// hits on a creature (entity/pettemplate.go, petition 1388).
	ProcChance int
	ProcDamage int
	ProcName   string
	ProcResist string
	// ResistFail is how often a resist fails against its proc, shield and
	// spells, at any figure of it: this percent altogether, twice this in part
	// (entity/resist.go). 0: the game's own, only at the cap.
	ResistFail float64
	// ResistKind is the resist it is strong in (FR, CR, LR, AR, PR or DR, several
	// with commas; empty: none) and ResistPct the percent of spell damage of that
	// kind it does not take (entity/enchanter.go). 0: all of it. A kind may carry
	// its own percent, "LR:75", which then holds for it (ResistKindPct).
	ResistKind string
	ResistPct  int
	// KickDamage is a second timed melee beside the special: a Kick of this figure (fixed, not by
	// level, through any armour) every KickEvery seconds at its enemy, the first KickFirst into
	// the fight (0: none; the GM Jibrish's petition 1249: Mr. Moogie's "kick every 5 seconds for 4k")
	KickDamage           int
	KickEvery, KickFirst float64
	// FightRegen is the hit points it heals each regeneration tick (six seconds) while it
	// fights: a creature out of one heals by its level's balance row, one in a fight not at
	// all but by this (0; the GM Jibrish's petition 1254: Mr. Moogie's "2000 HP per tick
	// regeneration in combat")
	FightRegen int
	// RunSpeed is its pace in a chase and on its way home, in units a second (0: the
	// creatures' 9.9, entity.NPCRunSpeed; petition 1254: Mr. Moogie's "double that of spirit
	// of the wolf", 2 x 8.83 x 1.3)
	RunSpeed  float64
	Abilities []TemplateAbility
	Note      string
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
		list[i].Target = strings.ToLower(strings.TrimSpace(list[i].Target))
		list[i].Stat = strings.ToLower(strings.TrimSpace(list[i].Stat))
		list[i].Element = strings.ToLower(strings.TrimSpace(list[i].Element))
		list[i].Name = strings.TrimSpace(list[i].Name)
		list[i].Say = strings.TrimSpace(list[i].Say)
		list[i].ProcName = strings.TrimSpace(list[i].ProcName)
		list[i].NPC = strings.TrimSpace(list[i].NPC)
		list[i].Resist = strings.ToUpper(strings.TrimSpace(list[i].Resist))
		for j := range list[i].Foes {
			list[i].Foes[j] = strings.TrimSpace(list[i].Foes[j])
		}
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
		if a.Kind == AbilityHealShield {
			if a.Below < 1 || a.Below > 100 {
				return fmt.Errorf("ability %d: a heal shield's below %d: 1 to 100 percent of its hit points", n, a.Below)
			}
			continue
		}
		if a.Kind == AbilityMeleeKind {
			switch {
			case a.MeleeKind != "slash" && a.MeleeKind != "pierce" && a.MeleeKind != "blunt":
				return fmt.Errorf("ability %d: melee_kind %q: slash, pierce or blunt", n, a.MeleeKind)
			case a.TakenPct < 1 || a.TakenPct > 1000:
				return fmt.Errorf("ability %d: taken_pct %d: 1 to 1000 percent", n, a.TakenPct)
			}
			continue
		}
		if BossMomentKind(a.Kind) {
			if err := checkBossMoment(n, a); err != nil {
				return err
			}
			continue
		}
		if a.SetHP != 0 || a.Radius != 0 || a.Tick != 0 || a.TickAtCap != 0 || a.TickFor != 0 {
			return fmt.Errorf("ability %d: set_hp, radius and a tick are a sethp's", n)
		}
		if err := checkKindFields(n, a); err != nil {
			return err
		}
		if passiveKind(a.Kind) {
			if err := checkPassiveAbility(n, a); err != nil {
				return err
			}
			if err := checkPetAbility(n, a); err != nil {
				return err
			}
			continue
		}
		switch {
		case !knownKind(a.Kind):
			return fmt.Errorf("ability %d: kind %q: one of %s", n, a.Kind, strings.Join(AbilityKinds, ", "))
		case a.Chain && i == 0:
			return fmt.Errorf("ability 1 chains, and nothing is before it")
		case a.Chain && passiveKind(list[i-1].Kind):
			return fmt.Errorf("ability %d chains to a %s, which casts nothing", n, list[i-1].Kind)
		case !finite(a.Every) || !finite(a.First) || !finite(a.MaxRecast) || !finite(a.MaxCast) || !finite(a.Cast):
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
		case a.Cast < 0 || a.Cast > 60:
			return fmt.Errorf("ability %d: cast %.2f: 0 (its spell's own) to 60 seconds", n, a.Cast)
		case a.Below < 0 || a.Below > 100:
			return fmt.Errorf("ability %d: below %d: 0 (50) to 100 percent", n, a.Below)
		case a.From < 0 || a.From > 100:
			return fmt.Errorf("ability %d: from %d: 0 (any) to 100", n, a.From)
		case len(a.Spell) > 64 || len(a.Line) > 64:
			return fmt.Errorf("ability %d: a spell or line of more than 64 characters", n)
		case len(a.Name) > StripNameMost:
			return fmt.Errorf("ability %d: a name of more than %d characters", n, StripNameMost)
		case a.Name != "" && a.Kind != AbilityStrip && !(a.Kind == AbilityHeal && a.HealPct > 0) && a.Damage == 0:
			return fmt.Errorf("ability %d: a name is a strip's, a heal of its share's or a fixed hit's (its spell names any other)", n)
		case a.Name != "" && (a.Spell != "" || a.SpellID != 0):
			return fmt.Errorf("ability %d: named by its name or by the spell it pins, not both", n)
		}
		if err := checkFixedAbility(n, a); err != nil {
			return err
		}
		if err := checkCreatureAbility(n, a); err != nil {
			return err
		}
		if err := checkPetAbility(n, a); err != nil {
			return err
		}
	}
	return nil
}

// checkCreatureAbility holds a creature's fields of an ability (its cooldown, its
// line in /say, the proc its landing gives) to their bounds.
func checkCreatureAbility(n int, a TemplateAbility) error {
	switch {
	case math.IsNaN(a.Cooldown) || math.IsInf(a.Cooldown, 0):
		return fmt.Errorf("ability %d: a cooldown that is not a number", n)
	case a.Cooldown < 0 || a.Cooldown > 86400:
		return fmt.Errorf("ability %d: cooldown %.1f: 0 (none) to 86400 seconds across fights", n, a.Cooldown)
	case len(a.Say) > SayMost:
		return fmt.Errorf("ability %d: a say of more than %d characters", n, SayMost)
	case strings.ContainsAny(a.Say, "|\n\r"):
		return fmt.Errorf("ability %d: a say with a | or a line break", n)
	case a.ProcChance < 0 || a.ProcChance > 100:
		return fmt.Errorf("ability %d: proc_chance %d: 0 to 100 percent of its melee hits", n, a.ProcChance)
	case a.ProcDamage < 0 || a.ProcDamage > 100000:
		return fmt.Errorf("ability %d: proc_damage %d: 0 to 100000", n, a.ProcDamage)
	case (a.ProcChance > 0) != (a.ProcDamage > 0):
		return fmt.Errorf("ability %d: a proc needs both its proc_chance and its proc_damage", n)
	case a.ProcName != "" && a.ProcChance == 0:
		return fmt.Errorf("ability %d: a proc_name with no proc", n)
	case len(a.ProcName) > StripNameMost:
		return fmt.Errorf("ability %d: a proc_name of more than %d characters", n, StripNameMost)
	}
	return nil
}

// checkKindFields refuses a field of one kind of ability on another: a hit heal's
// heal, a summon's count, npc and share, an enrage's grow and rampage, a prefer's
// foes, a veil's tap_pct, a fixed hit's damage, an out-of-reach ability's gate and
// range, a resist and a visual where nothing takes them.
func checkKindFields(n int, a TemplateAbility) error {
	k := a.Kind
	pinned := a.Spell != "" || a.SpellID != 0
	switch {
	case a.Heal != 0 && k != AbilityHitHeal:
		return fmt.Errorf("ability %d: heal is a hitheal's", n)
	case (a.Count != 0 || a.NPC != "" || a.Share != 0) && k != AbilitySummon:
		return fmt.Errorf("ability %d: count, npc and share are a summon's", n)
	case (a.Grow != 0 || a.Rampage != 0) && k != AbilityEnrage:
		return fmt.Errorf("ability %d: grow and rampage are an enrage's", n)
	case len(a.Foes) > 0 && k != AbilityPrefer && k != AbilityKillSay:
		return fmt.Errorf("ability %d: foes are a prefer's or a killsay's", n)
	case a.TapPct != 0 && k != AbilityVeil:
		return fmt.Errorf("ability %d: tap_pct is a veil's", n)
	case a.Damage != 0 && k != AbilityNuke && k != AbilityLifetap:
		return fmt.Errorf("ability %d: a fixed damage is a nuke's or a lifetap's", n)
	case a.OutOfReach && k != AbilityNuke && k != AbilityLifetap && k != AbilityAoE && k != AbilityDebuff && k != AbilityStrip && k != AbilityDoT:
		return fmt.Errorf("ability %d: out_of_reach is a harmful cast's at what it fights (nuke, lifetap, aoe, debuff, strip, dot)", n)
	case a.Range != 0 && !a.OutOfReach:
		return fmt.Errorf("ability %d: a range is an out_of_reach ability's", n)
	case math.IsNaN(a.Range) || math.IsInf(a.Range, 0) || a.Range < 0 || a.Range > 1000:
		return fmt.Errorf("ability %d: range %.1f: 0 (%d) to 1000 units", n, a.Range, DefaultOutOfReachRange)
	case !shieldResistKnown(a.Resist):
		return fmt.Errorf("ability %d: resist %q: FR, CR, LR, AR, PR, DR or none", n, a.Resist)
	case a.Resist != "" && a.Damage == 0 && k != AbilityVeil && !(k == AbilityProc && !pinned):
		return fmt.Errorf("ability %d: a resist is a fixed damage's, a veil's or a proc's of its own figures (a spell's is its own)", n)
	case a.FX != 0 && pinned && k != AbilityProc && k != AbilityVeil:
		return fmt.Errorf("ability %d: a visual (fx) is an ability's that pins no spell (its spell's is its look), or a proc's or a veil's hit's", n)
	case a.FX != 0 && k != AbilityStrip && !(k == AbilityHeal && a.HealPct > 0) && a.Damage == 0 && k != AbilityProc && k != AbilityVeil:
		return fmt.Errorf("ability %d: a visual (fx) is a named strip's, heal of its share's, fixed hit's, proc's or veil's", n)
	}
	return nil
}

// checkBossMoment holds a boss's moment (BossMomentKind) to its fields: its
// clock and the chance it goes off when due, its figure (damage) for a self dot
// and a raid heal, a raid heal's count, a sethp's hit points and radius, a name
// for its lines; nothing of a cast's.
func checkBossMoment(n int, a TemplateAbility) error {
	finite := func(v float64) bool { return !math.IsNaN(v) && !math.IsInf(v, 0) }
	figure := a.Kind == AbilitySelfDoT || a.Kind == AbilityRaidHeal
	switch {
	case !finite(a.Every) || !finite(a.First) || !finite(a.Radius):
		return fmt.Errorf("ability %d: a number that is not one", n)
	case a.Every < 2 || a.Every > 3600:
		return fmt.Errorf("ability %d: every %.1f: 2 to 3600 seconds from one to the next", n, a.Every)
	case a.First < 0 || a.First > 3600:
		return fmt.Errorf("ability %d: first %.1f: 0 to 3600 seconds into a fight", n, a.First)
	case a.Chance < 0 || a.Chance > 100:
		return fmt.Errorf("ability %d: chance %d: 1 to 100 percent of its due times (0: every one)", n, a.Chance)
	case figure && (a.Damage < 1 || a.Damage > 1000000):
		return fmt.Errorf("ability %d: a %s's damage %d: 1 to 1000000 hit points", n, a.Kind, a.Damage)
	case !figure && a.Damage != 0:
		return fmt.Errorf("ability %d: a %s has no damage", n, a.Kind)
	case a.Kind == AbilityRaidHeal && (a.Count < 1 || a.Count > 40):
		return fmt.Errorf("ability %d: a raidheal's count %d: 1 to 40 players", n, a.Count)
	case a.Kind != AbilityRaidHeal && a.Count != 0:
		return fmt.Errorf("ability %d: a count is a raidheal's", n)
	case a.Kind == AbilitySetHP && (a.SetHP < 1 || a.SetHP > 1000000):
		return fmt.Errorf("ability %d: a sethp's set_hp %d: 1 to 1000000 hit points (it never kills)", n, a.SetHP)
	case a.Kind == AbilitySetHP && (a.Radius < 0 || a.Radius > MostSetHPRadius):
		return fmt.Errorf("ability %d: a sethp's radius %.1f: 0 (the one it fights) to %d", n, a.Radius, MostSetHPRadius)
	case a.Kind != AbilitySetHP && (a.SetHP != 0 || a.Radius != 0 || a.Tick != 0 || a.TickAtCap != 0 || a.TickFor != 0):
		return fmt.Errorf("ability %d: set_hp, radius and a tick are a sethp's", n)
	case !finite(a.TickFor) || a.Tick < 0 || a.Tick > 1000000 || a.TickAtCap < 0 || a.TickAtCap > a.Tick || a.TickFor < 0 || a.TickFor > 600:
		return fmt.Errorf("ability %d: a tick of %d (1 to 1000000), at the cap %d (0 to its tick), for %.1f s (0 to 600)", n, a.Tick, a.TickAtCap, a.TickFor)
	case a.Tick == 0 && (a.TickAtCap != 0 || a.TickFor != 0):
		return fmt.Errorf("ability %d: tick_at_cap and tick_for are a tick's", n)
	case !shieldResistKnown(a.Resist) || a.Resist != "" && a.Tick == 0:
		return fmt.Errorf("ability %d: resist %q: FR, CR, LR, AR, PR or DR, and a sethp's tick's alone", n, a.Resist)
	case len(a.Name) > StripNameMost:
		return fmt.Errorf("ability %d: a name of more than %d characters", n, StripNameMost)
	case a.Chain || a.Spell != "" || a.SpellID != 0 || a.Line != "" || a.DamagePct != 0 || a.Below != 0 || a.HealPct != 0 ||
		a.NPC != "" || a.Cooldown != 0 || a.Cast != 0:
		return fmt.Errorf("ability %d: a %s takes none of a cast's fields (chain, spell, line, damage_pct, below, cast, cooldown)", n, a.Kind)
	}
	return nil
}

// checkPassiveAbility holds an ability that casts nothing on a clock (passiveKind)
// to its kind's fields: no clock of its own (an enrage's every is its pulls'),
// no chain, a summon's and an enrage's share of hit points, a line in /say only
// for those two.
func checkPassiveAbility(n int, a TemplateAbility) error {
	finite := func(v float64) bool { return !math.IsNaN(v) && !math.IsInf(v, 0) }
	pinned := a.Spell != "" || a.SpellID != 0
	moment := a.Kind == AbilitySummon || a.Kind == AbilityEnrage || a.Kind == AbilityLastStand
	damagePct := a.DamagePct
	if a.Kind == AbilityLastStand {
		damagePct = 0 // a last stand's damage after it (checked below)
	}
	switch {
	case a.Chain:
		return fmt.Errorf("ability %d: a %s casts nothing to chain", n, a.Kind)
	case a.Every != 0 && a.Kind != AbilityEnrage:
		return fmt.Errorf("ability %d: a %s has no clock (every)", n, a.Kind)
	case a.First != 0 || a.Chance != 0 || damagePct != 0 || a.MaxRecast != 0 || a.MaxCast != 0 || a.Cast != 0 || a.Cooldown != 0 || a.HealPct != 0:
		return fmt.Errorf("ability %d: a %s takes none of a cast's fields (first, chance, damage_pct, max_recast, max_cast, cast, cooldown, heal_pct)", n, a.Kind)
	case a.Line != "" || (pinned && a.Kind != AbilityProc && a.Kind != AbilityVeil):
		return fmt.Errorf("ability %d: a %s casts no spell (line, spell, spell_id)", n, a.Kind)
	case moment && (a.Below < 1 || a.Below > 100):
		return fmt.Errorf("ability %d: a %s's below %d: 1 to 100 percent of its hit points", n, a.Kind, a.Below)
	case !moment && a.Below != 0:
		return fmt.Errorf("ability %d: below is a summon's, an enrage's or a last stand's", n)
	case a.Say != "" && !moment && a.Kind != AbilityKillSay:
		return fmt.Errorf("ability %d: a say is a summon's, an enrage's, a last stand's or a killsay's (or a cast's)", n)
	case len(a.Say) > SayMost:
		return fmt.Errorf("ability %d: a say of more than %d characters", n, SayMost)
	case strings.ContainsAny(a.Say, "|\n\r"):
		return fmt.Errorf("ability %d: a say with a | or a line break", n)
	case a.Name != "" && a.Kind != AbilityVeil:
		return fmt.Errorf("ability %d: a name is a veil's (a proc's is proc_name, or its spell's)", n)
	case a.Name != "" && pinned:
		return fmt.Errorf("ability %d: named by its name or by the spell it pins, not both", n)
	case len(a.Name) > StripNameMost:
		return fmt.Errorf("ability %d: a name of more than %d characters", n, StripNameMost)
	case a.From != 0:
		return fmt.Errorf("ability %d: from is a cast's (a %s is its creature's at any level)", n, a.Kind)
	}
	switch a.Kind {
	case AbilityHitHeal:
		if a.Heal < 1 || a.Heal > 100000 {
			return fmt.Errorf("ability %d: a hitheal's heal %d: 1 to 100000 hit points a hit", n, a.Heal)
		}
	case AbilityPrefer:
		if len(a.Foes) == 0 || len(a.Foes) > MostFoes {
			return fmt.Errorf("ability %d: a prefer's foes: 1 to %d names", n, MostFoes)
		}
		for _, f := range a.Foes {
			if f == "" || len(f) > 64 {
				return fmt.Errorf("ability %d: a prefer's foe %q: a name of 1 to 64 characters", n, f)
			}
		}
	case AbilitySummon:
		switch {
		case a.Count < 1 || a.Count > MostSummoned:
			return fmt.Errorf("ability %d: a summon's count %d: 1 to %d", n, a.Count, MostSummoned)
		case a.NPC == "" || len(a.NPC) > 64:
			return fmt.Errorf("ability %d: a summon's npc: the name of the NPC it calls copies of (1 to 64 characters)", n)
		case a.Share < 0 || a.Share > 1000:
			return fmt.Errorf("ability %d: a summon's share %d: 0 (the copies' own) to 1000 percent of its figures", n, a.Share)
		}
	case AbilityEnrage:
		switch {
		case !finite(a.Every) || a.Every < 0 || a.Every > 3600 || (a.Every > 0 && a.Every < 2):
			return fmt.Errorf("ability %d: an enrage's every %.1f: 0 (no pulls) or 2 to 3600 seconds from one pull to the next", n, a.Every)
		case a.Grow < 0 || a.Grow > 500:
			return fmt.Errorf("ability %d: an enrage's grow %d: 0 to 500 percent bigger", n, a.Grow)
		case a.Rampage < 0 || a.Rampage > 100:
			return fmt.Errorf("ability %d: an enrage's rampage %d: 0 to 100 percent of each hit", n, a.Rampage)
		case a.Grow == 0 && a.Every == 0 && a.Rampage == 0:
			return fmt.Errorf("ability %d: an enrage that does nothing (grow, every, rampage)", n)
		}
	case AbilityProc:
		switch {
		case pinned && (a.ProcChance != 0 || a.ProcDamage != 0 || a.ProcName != ""):
			return fmt.Errorf("ability %d: a proc of a spell is its spell's (no proc_chance, proc_damage, proc_name)", n)
		case !pinned && (a.ProcChance < 1 || a.ProcChance > 100 || a.ProcDamage < 1 || a.ProcDamage > 100000):
			return fmt.Errorf("ability %d: a proc pins a spell (spell or spell_id) or has a proc_chance (1-100) and a proc_damage (1-100000)", n)
		case len(a.ProcName) > StripNameMost:
			return fmt.Errorf("ability %d: a proc_name of more than %d characters", n, StripNameMost)
		case a.SpellID < 0 || (a.SpellID != 0 && a.Spell != ""):
			return fmt.Errorf("ability %d: a spell pinned by name or by id, not both", n)
		}
		return nil
	case AbilityVeil:
		switch {
		case !finite(a.TapPct) || a.TapPct <= 0 || a.TapPct > 100:
			return fmt.Errorf("ability %d: a veil's tap_pct %v: above 0 to 100 percent of its hit points", n, a.TapPct)
		case a.SpellID < 0 || (a.SpellID != 0 && a.Spell != ""):
			return fmt.Errorf("ability %d: a spell pinned by name or by id, not both", n)
		}
	case AbilityLastStand:
		if a.DamagePct < 0 || a.DamagePct > 1000 {
			return fmt.Errorf("ability %d: a last stand's damage_pct %d: 0 (100, as it was) to 1000 percent of its damage after it", n, a.DamagePct)
		}
	case AbilityKillSay:
		switch {
		case len(a.Foes) == 0 || len(a.Foes) > MostFoes:
			return fmt.Errorf("ability %d: a killsay's foes: 1 to %d names", n, MostFoes)
		case a.Say == "":
			return fmt.Errorf("ability %d: a killsay's say: its line", n)
		}
		for _, f := range a.Foes {
			if f == "" || len(f) > 64 {
				return fmt.Errorf("ability %d: a killsay's foe %q: a name of 1 to 64 characters", n, f)
			}
		}
	}
	if a.ProcChance != 0 || a.ProcDamage != 0 || a.ProcName != "" {
		return fmt.Errorf("ability %d: a proc's fields are a proc's or a cast's that gives one", n)
	}
	return nil
}

// checkFixedAbility holds a clocked ability's fixed figure (damage) to its bounds:
// not beside a damage_pct, which it takes the place of.
func checkFixedAbility(n int, a TemplateAbility) error {
	switch {
	case a.Damage < 0 || a.Damage > 1000000:
		return fmt.Errorf("ability %d: damage %d: 0 (its spell's figures) to 1000000, fixed", n, a.Damage)
	case a.Damage > 0 && a.DamagePct != 0:
		return fmt.Errorf("ability %d: a fixed damage takes its spell's figures' place: no damage_pct with it", n)
	}
	return nil
}

// checkPetAbility holds a pet's fields of an ability to what its kind takes.
func checkPetAbility(n int, a TemplateAbility) error {
	switch {
	case a.Target != "" && a.Target != AbilityOnOwner && a.Target != AbilityOnSelf && a.Target != AbilityOnEither &&
		a.Target != AbilityOnGroup && a.Target != AbilityOnMember:
		return fmt.Errorf("ability %d: target %q: owner, self, either, group or member", n, a.Target)
	case a.Target != "" && a.Kind != AbilityHeal && a.Kind != AbilityBuff:
		return fmt.Errorf("ability %d: a target is a heal's or a buff's", n)
	case a.Target == AbilityOnEither && a.Kind != AbilityHeal:
		return fmt.Errorf("ability %d: target either is a heal's", n)
	case a.Target == AbilityOnGroup && a.Kind != AbilityHeal:
		return fmt.Errorf("ability %d: target group is a heal's", n)
	case a.Target == AbilityOnMember && a.Kind != AbilityHeal:
		return fmt.Errorf("ability %d: target member is a heal's", n)
	case a.SpellID < 0:
		return fmt.Errorf("ability %d: spell_id %d: a spell's id (0 none)", n, a.SpellID)
	case a.SpellID != 0 && a.Spell != "":
		return fmt.Errorf("ability %d: a spell pinned by name and by id", n)
	case a.Kind == AbilityDebuff && a.SpellID == 0 && a.Spell == "":
		return fmt.Errorf("ability %d: a debuff names its spell (spell or spell_id)", n)
	case a.NoCure && a.Kind != AbilityDebuff:
		return fmt.Errorf("ability %d: no_cure is a debuff's", n)
	case a.WhileOut && (a.Kind != AbilityBuff || a.Target != AbilityOnOwner):
		return fmt.Errorf("ability %d: while_out is a buff's on its owner", n)
	case a.SpellsOf != nil && (*a.SpellsOf < 0 || *a.SpellsOf > 14):
		return fmt.Errorf("ability %d: spells_of %d: 0 (Warrior) to 14 (Alchemist)", n, *a.SpellsOf)
	case slices.ContainsFunc(a.Classes, func(c int) bool { return c < 0 || c > 14 }):
		return fmt.Errorf("ability %d: classes %v: each 0 (Warrior) to 14 (Alchemist)", n, a.Classes)
	case a.Stat != "" && !oneOf(a.Stat, AbilityStats):
		return fmt.Errorf("ability %d: stat %q: one of %s", n, a.Stat, strings.Join(AbilityStats, ", "))
	case a.Stat != "" && a.Kind != AbilityBuff:
		return fmt.Errorf("ability %d: a stat is a buff's", n)
	case a.Kind == AbilityBuff && a.Stat == "" && a.Spell == "" && a.SpellID == 0:
		return fmt.Errorf("ability %d: a buff needs the stat its spell raises (or a spell)", n)
	case a.Element != "" && !oneOf(a.Element, AbilityElements):
		return fmt.Errorf("ability %d: element %q: one of %s", n, a.Element, strings.Join(AbilityElements, ", "))
	case a.Element != "" && a.Kind != AbilityNuke && a.Kind != AbilityAoE && a.Kind != AbilityLifetap && a.Kind != AbilityDoT:
		return fmt.Errorf("ability %d: an element is a harmful spell's", n)
	case a.HealPct < 0 || a.HealPct > 100:
		return fmt.Errorf("ability %d: heal_pct %d: 0 (its spell's figure) to 100 percent", n, a.HealPct)
	case a.HealPct > 0 && a.Kind != AbilityHeal:
		return fmt.Errorf("ability %d: heal_pct is a heal's", n)
	case len(a.Hits) > 0 && a.Kind != AbilityTaunt:
		return fmt.Errorf("ability %d: hits are a taunt's", n)
	case len(a.Hits) > 8:
		return fmt.Errorf("ability %d: %d hits points, at most 8", n, len(a.Hits))
	}
	for i, h := range a.Hits {
		lvl, least, most := h[0], h[1], h[2]
		switch {
		case math.IsNaN(lvl) || math.IsNaN(least) || math.IsNaN(most):
			return fmt.Errorf("ability %d: hits point %d: a number that is not one", n, i+1)
		case lvl < 0 || lvl > 100 || lvl != math.Trunc(lvl):
			return fmt.Errorf("ability %d: hits point %d: level %g: a whole level, 0 to 100", n, i+1, lvl)
		case i > 0 && lvl <= a.Hits[i-1][0]:
			return fmt.Errorf("ability %d: hits point %d: the levels must climb", n, i+1)
		case least < 0 || most < least || most > 100000:
			return fmt.Errorf("ability %d: hits point %d: least %g, most %g: 0 <= least <= most <= 100000", n, i+1, least, most)
		}
	}
	return nil
}

func oneOf(s string, set []string) bool {
	for _, x := range set {
		if s == x {
			return true
		}
	}
	return false
}

func knownKind(k string) bool {
	for _, x := range AbilityKinds {
		if k == x {
			return true
		}
	}
	return false
}

// MaxTemplateHPPct is the most of its level's hit points a template gives: a
// raid boss's million (petitions 1144, 1153: 6,615% at level 63) with room over.
// MaxTemplateShield is the most its damage shield answers a melee hit with.
const (
	MaxTemplateHPPct = 100000
	// MaxTemplateMeleePct is the most of its roll a template's melee hits are: a
	// raid boss's 2,500 on a tank of 6,850 AC is 18 times a level 70's floor
	MaxTemplateMeleePct = 5000
	MaxTemplateShield   = 100000
	// MaxTemplateResistFail is the most of a template's resist_fail: three times it stays under all of them
	MaxTemplateResistFail = 30
)

// shieldResistKnown is whether a template's shield_resist names a resist.
func shieldResistKnown(s string) bool {
	switch strings.ToUpper(strings.TrimSpace(s)) {
	case "", "FR", "CR", "LR", "AR", "PR", "DR":
		return true
	}
	return false
}

// resistKindsKnown is whether a resist_kind is known: one, or several with commas, each
// with its own percent or not (ResistKindPct).
func resistKindsKnown(s string) bool {
	for _, k := range strings.Split(s, ",") {
		if _, _, ok := ResistKindPct(k); !ok {
			return false
		}
	}
	return true
}

// ResistKindPct reads one of a resist_kind's resists: its name (FR, CR, LR, AR, PR or
// DR, upper case; "" for none) and its own percent, "LR:75", 1 to 100 (0 when it gives
// none, and resist_pct is its figure). A raid boss can so be very strong in one resist
// and strong in the rest (QuestMasters' raid list, 2006: "Avatar of Fear- *LR* AR PR DR",
// the star its strongest: tools/questmasters_1112.py). False for anything else.
func ResistKindPct(tok string) (kind string, pct int, ok bool) {
	kind, fig, has := strings.Cut(strings.TrimSpace(tok), ":")
	kind = strings.ToUpper(strings.TrimSpace(kind))
	if !shieldResistKnown(kind) {
		return "", 0, false
	}
	if !has {
		return kind, 0, true
	}
	n, err := strconv.Atoi(strings.TrimSpace(fig))
	if err != nil || n < 1 || n > 100 || kind == "" {
		return "", 0, false
	}
	return kind, n, true
}

// CheckClassTemplate is what a template must be for an NPC to fight by it: a
// name, a class of the fifteen (or -1), its hit points within a tenth to
// MaxTemplateHPPct, its melee percents within a tenth to MaxTemplateMeleePct (a melee
// taken from 1%), a damage shield within bounds and a resist it knows, a
// special's chance 0-100 and, when it comes
// at all, a damage above 0 a level and at most 200, and abilities
// CheckTemplateAbilities takes.
func CheckClassTemplate(t ClassTemplate) error {
	switch {
	case TemplateKey(t.Name) == "":
		return fmt.Errorf("no name")
	case t.Class < -1 || t.Class > 14:
		return fmt.Errorf("class %d: -1 (the NPC's own) or 0 (Warrior) to 14 (Alchemist)", t.Class)
	case t.HPPct < 10 || t.HPPct > MaxTemplateHPPct:
		return fmt.Errorf("hp_pct %d: 10 to %d percent of its level's hit points", t.HPPct, MaxTemplateHPPct)
	case t.ShieldDamage < 0 || t.ShieldDamage > MaxTemplateShield:
		return fmt.Errorf("shield_damage %d: 0 to %d a melee hit", t.ShieldDamage, MaxTemplateShield)
	case !shieldResistKnown(t.ShieldResist):
		return fmt.Errorf("shield_resist %q: FR, CR, LR, AR, PR, DR or none", t.ShieldResist)
	case t.ProcChance < 0 || t.ProcChance > 100:
		return fmt.Errorf("proc_chance %d: 0 to 100 percent of its melee hits", t.ProcChance)
	case t.ProcDamage < 0 || t.ProcDamage > MaxTemplateShield:
		return fmt.Errorf("proc_damage %d: 0 to %d", t.ProcDamage, MaxTemplateShield)
	case t.ProcChance > 0 && t.ProcDamage == 0:
		return fmt.Errorf("a proc on %d%% of its hits needs a proc_damage above 0", t.ProcChance)
	case !shieldResistKnown(t.ProcResist):
		return fmt.Errorf("proc_resist %q: FR, CR, LR, AR, PR, DR or none", t.ProcResist)
	case math.IsNaN(t.ResistFail) || t.ResistFail < 0 || t.ResistFail > MaxTemplateResistFail:
		return fmt.Errorf("resist_fail %v: 0 to %d percent", t.ResistFail, MaxTemplateResistFail)
	case !resistKindsKnown(t.ResistKind):
		return fmt.Errorf("resist_kind %q: FR, CR, LR, AR, PR, DR, several with commas (each may have its own percent: LR:75), or none", t.ResistKind)
	case t.ResistPct < 0 || t.ResistPct > 100:
		return fmt.Errorf("resist_pct %d: 0 to 100 percent of spell damage of its kind", t.ResistPct)
	case t.ResistPct > 0 && strings.TrimSpace(t.ResistKind) == "":
		return fmt.Errorf("a resist_pct of %d needs a resist_kind", t.ResistPct)
	case t.TakenPct < 1 || t.TakenPct > 1000:
		return fmt.Errorf("taken_pct %d: 1 to 1000 percent of a melee hit on it", t.TakenPct)
	case t.MeleePct < 10 || t.MeleePct > MaxTemplateMeleePct:
		return fmt.Errorf("melee_pct %d: 10 to %d percent of its roll", t.MeleePct, MaxTemplateMeleePct)
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
	case t.KickDamage < 0 || t.KickDamage > 1000000:
		return fmt.Errorf("kick_damage %d: 0 (none) to 1,000,000", t.KickDamage)
	case math.IsNaN(t.KickEvery) || t.KickEvery < 0 || t.KickEvery > 3600 || (t.KickEvery > 0 && t.KickEvery < 1):
		return fmt.Errorf("kick_every %.1f: 0 (none) or 1 to 3600 seconds from one kick to the next", t.KickEvery)
	case math.IsNaN(t.KickFirst) || t.KickFirst < 0 || t.KickFirst > 3600:
		return fmt.Errorf("kick_first %.1f: 0 to 3600 seconds into a fight", t.KickFirst)
	case t.FightRegen < 0 || t.FightRegen > 1000000:
		return fmt.Errorf("fight_regen %d: 0 (none) to 1,000,000 hit points a regeneration tick", t.FightRegen)
	case math.IsNaN(t.RunSpeed) || t.RunSpeed < 0 || t.RunSpeed > 60 || (t.RunSpeed > 0 && t.RunSpeed < 1):
		return fmt.Errorf("run_speed %.2f: 0 (the creatures' 9.9) or 1 to 60 units a second", t.RunSpeed)
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

// SeedClassTemplates are jib's five examples and the user's wisp (his wizard
// at 28%) and roamer, the rows classtemplates is made with (source seed). Their numbers are ours: no recording has a creature
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
	{Name: "wisp", Class: 13, HPPct: 85, TakenPct: 100, MeleePct: 100,
		Abilities: []TemplateAbility{
			{Kind: AbilityAoE, Every: 15, First: 2, MaxRecast: 10, MaxCast: 5, DamagePct: 28},
			{Kind: AbilityNuke, Chain: true, MaxRecast: 10, MaxCast: 5, DamagePct: 28},
		},
		Note: "the user's wisp (2026-10-30, \"28%\"): jib's wizard with its abilities at 28% of a player's figures where " +
			"his are 35% (at 35% a rogue of 35-45 and a cleric of 35 lost every wisp yellow); tools/seed_roamers.py gives " +
			"it to the wisps"},
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
	{"shield_damage", "INTEGER NOT NULL DEFAULT 0"},
	{"shield_resist", "TEXT NOT NULL DEFAULT ''"},
	{"proc_chance", "INTEGER NOT NULL DEFAULT 0"},
	{"proc_damage", "INTEGER NOT NULL DEFAULT 0"},
	{"proc_name", "TEXT NOT NULL DEFAULT ''"},
	{"proc_resist", "TEXT NOT NULL DEFAULT ''"},
	{"resist_fail", "REAL NOT NULL DEFAULT 0"},
	{"resist_kind", "TEXT NOT NULL DEFAULT ''"},
	{"resist_pct", "INTEGER NOT NULL DEFAULT 0"},
	{"kick_damage", "INTEGER NOT NULL DEFAULT 0"},
	{"kick_every", "REAL NOT NULL DEFAULT 0"},
	{"kick_first", "REAL NOT NULL DEFAULT 0"},
	{"fight_regen", "INTEGER NOT NULL DEFAULT 0"},
	{"run_speed", "REAL NOT NULL DEFAULT 0"},
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
		special_fx, special_every, special_first, special_jitter, special_from, shield_damage, shield_resist, proc_chance, proc_damage, proc_name, proc_resist, resist_fail, resist_kind, resist_pct, kick_damage, kick_every, kick_first, fight_regen, run_speed, abilities, note FROM classtemplates
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
			&t.SpecialDamage, &fx, &t.SpecialEvery, &t.SpecialFirst, &t.SpecialJitter, &t.SpecialFrom, &t.ShieldDamage, &t.ShieldResist, &t.ProcChance, &t.ProcDamage, &t.ProcName, &t.ProcResist, &t.ResistFail, &t.ResistKind, &t.ResistPct, &t.KickDamage, &t.KickEvery, &t.KickFirst, &t.FightRegen, &t.RunSpeed, &abilities, &t.Note); err != nil {
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
// whole, its spelleffect line, effects (JSON) and visual, and its spellarea
// row if it has one.
type TemplateSpell struct {
	Pattern SpellPatternRecord
	Line    string
	Effects string
	FX      uint32
	Area    SpellAreaRow
	HasArea bool
}

// templateSpellColumns are a TemplateSpell's, spellpattern sp left joined to
// spelleffect se and spellarea sa.
const templateSpellColumns = `sp.spellid, sp.abilitylvl, sp.unk2, sp.unk3, sp.range, sp.casttime, sp.power, sp.iconColor,
	sp.icon, sp.scope, sp.recast, sp.eqprequire, sp.spellname, COALESCE(sp.spelldesc, ''), COALESCE(se.line, ''),
	COALESCE(se.effects, ''), COALESCE(se.fxdict, 0), sa.centre, sa.radius, sa.maxtargets, sa.cone`

func scanTemplateSpell(scan func(...any) error) (TemplateSpell, error) {
	var s TemplateSpell
	var fx int64
	var centre, maxTargets sql.NullInt64
	var radius, cone sql.NullFloat64
	p := &s.Pattern
	if err := scan(&p.SpellID, &p.AbilityLvl, &p.Unk2, &p.Unk3, &p.Range, &p.CastTime, &p.Power, &p.IconColor, &p.Icon,
		&p.Scope, &p.Recast, &p.EqpRequire, &p.SpellName, &p.SpellDesc, &s.Line, &s.Effects, &fx, &centre, &radius, &maxTargets, &cone); err != nil {
		return s, err
	}
	s.FX = uint32(fx)
	if centre.Valid {
		s.HasArea = true
		s.Area = SpellAreaRow{Centre: int(centre.Int64), Radius: radius.Float64, MaxTargets: int(maxTargets.Int64), Cone: cone.Float64}
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
// of the records of the name, the one with a spelleffect line or effects
// first, then the lowest id; ok false when there is none.
func (db *SQLiteDB) TemplateSpellNamed(name string) (TemplateSpell, bool, error) {
	row := db.QueryRow(`SELECT `+templateSpellColumns+`
		FROM spellpattern sp
		LEFT JOIN spelleffect se ON se.spellid = sp.spellid
		LEFT JOIN spellarea sa ON sa.spellid = sp.spellid
		WHERE lower(trim(sp.spellname)) = ?
		ORDER BY (COALESCE(se.line, '') = '' AND COALESCE(se.effects, '') = ''), sp.spellid LIMIT 1`, strings.ToLower(strings.TrimSpace(name)))
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
