package entity

import (
	"fmt"
	"log"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/eqoa/server/internal/session"
)

// weaponproc.go — a weapon's own proc: what a weapon does beside its hit,
// now and then, while it is worn in a hand. The user, 2026-11-06: "lets make
// sure procs work, the weapons like selos rapiers". Until then no weapon
// procced: 150 rounds of a bard's two Selo's Jeweled Rapiers on a training
// dummy landed 600 hits and nothing else (TestRealSelosRapierFight, measured
// before this file).
//
// No record the client reads carries a proc (VINetItem has 23 fields, a name,
// a description and its stats), so a weapon's is the server's alone, and it
// is read from two places:
//   - the record's own description, where the players who wrote the item
//     database put what each proc did ("Procs a 100 point DD Spell.", "Proc
//     a 25 point lifetap.", "Procs a 70 point Disease DoT.", "400 DD
//     offensive proc, works only on goblins.", "Procs 150 Ice Damage.",
//     "Procs: Life Tap"): WeaponProcFromText;
//   - for the level 49 epic weapons, whose records here say nothing of it,
//     EQOA-Data's weapons.json (Information/databases/items), whose "proc"
//     stat gives each its figure (Selo's Jeweled Rapier 140, Scimitar of Fury
//     and Virtus Ferrum 100, Blacktongue "50 DoT", Sunsilver "Anti-aggro"):
//     weaponProcsByName, with the few the descriptions name and give no
//     figure for (ours, as each entry says).
// itempattern.weaponproc is not a proc: Return Home's import put the record's
// thirteenth field there, the range (a bow's 25 to 50, a thrown knife's 25),
// and nothing reads it.
//
// How often and how it lands is not recorded anywhere (Sandstorm's recordings
// hold no weapon proc), so it is proc.go's figures for the enchanter's procs,
// by DEX where theirs go by INT (petition 1360, Zaeire: "weapon proc chance
// scales off dex while ability proc chance scales off int"): each landed hit
// of the hand that holds the weapon procs at 3% and one more for each 20 DEX,
// at most 25% (weaponProcChancePct), unless its words give its own chance
// ("Procs 2000 slashing damage, a 10% chance each hit.": "N% chance", "N% a
// hit" or "per swing", from 1 to 100), which then holds at any DEX (the same
// petition: "Katana of severing proc should be set to 2k with a 10% chance to
// proc"; procChanceRe), as weaponProcsByName's Chance does; the
// creature resists it all
// or nothing at 5% against a wielder of its level, 3 more a level above (its
// element's debuffs taken off: npcSpellResistPct); the damage as it is, no
// mod (an epic quest guide: "weapon/spell procs and abilities hit full").
// Of a round's hits (rounds.go) the odd ones are the main hand's and the
// even ones the off hand's when both hands hold a weapon, as CLIENT's swing
// alternates them (func_00CC5190's dual-wield flag); a two-hander, a ranged
// weapon or one weapon alone makes every hit. What lands:
//   - a hit: its damage to the creature, told as a spell's hit (0x00DB from
//     the wielder), its hate and its kill the wielder's, the stand-in
//     strike's visual (fxDamage) from the wielder at it;
//   - a tap: the same, and what landed back to the wielder as a heal (0x00DB
//     on it), with Life Tap's own visual;
//   - a damage over time: its figure every six seconds for 30 s (ours), the
//     first at once, a proc on a creature already bearing it begun again;
//   - Sunsilver's anti-aggro: the creature's hate for the wielder halved
//     (ours: no figure is known), and a buff's visual on the wielder;
//   - a root or a snare (petition 1821, the GM Jibrish: "Enable code for
//     weapons to have working root or snare procs"): any record whose words
//     name one in the sentence of a proc ("Procs a root.", "Procs: Snare",
//     "Procs a 6 second root.", "Procs a 40% snare for 10 seconds.", "root
//     proc"; procHoldRe), alone or beside its damage ("Procs a 100 point DD
//     and roots the target."): lands as a spell's root or slow of the wielder's
//     (weaponHoldLocked), after the proc's own resist roll, with the
//     creature's root resist by its con (root.go: a snare's as a root's), a
//     root broken by later hits as any root; 12 s, a snare 50% for 18 s, when
//     the words give no time or slow (ours). A duel holds none (duelprocs.go);
//   - a proc for a kind of creature alone (Gorereaver's goblins) fires on a
//     creature whose name names it, and not otherwise.
//   - a form (the Staff of Faykind's; the user, 2026-11-07: "lets add staff
//     of faykind. it procs a pixie form, that lasts 10 min, the pixie form can
//     proc up to 800 per round, the proc percentage was around 30% for the
//     transform and the damage proc"): its wielder shown in a body for its
//     time (an illusion, illusion.go, kept over a logout and ended as any is),
//     unresisted; 30 s, and while it lasts a proc renews it to 30 s from then,
//     the form and its round's tally as they are (the user, 2026-11-08: "staff
//     of faykind pixie form should only last 30 secs unless it procs again";
//     it was ten minutes and never renewed). The damage is the form's own
//     proc (proc.go: ActiveEffect.Proc, its chance and its most a round in
//     EffectRules), so it goes with the form and not with the weapon.
// Not read: what no source gives a figure or an effect for (the Staff of
// infectious Dread's "Infectious Dread", which no spell here is, the
// "...of Toxicity" set's reek of poison, the Sword of Virtue's bare "proc 5"
// in EQOA-Data, which its description does not call a proc).

// ProcKind is what a weapon proc does.
type ProcKind int

const (
	ProcHit     ProcKind = iota // damage now
	ProcTap                     // damage now, what landed back to the wielder
	ProcDoT                     // damage every six seconds for a while
	ProcCalm                    // the creature's hate for the wielder halved
	ProcRestore                 // hit points and power back to the wielder and its group near
	ProcForm                    // the wielder shown in a body for a while, the body's own proc beside it
	ProcRoot                    // the creature held in place for a while (procRootFor)
	ProcSnare                   // the creature slowed for a while (procSnareFor, procSnarePct)
)

// procRootFor is how long a weapon's root holds (ours: no source gives one), unless
// it breaks first as any root does (root.go); a record's words may give their own
// ("Procs a 6 second root": WeaponProc.HoldFor).
const procRootFor = 12 * time.Second

// procSnareFor and procSnarePct are a weapon's snare's time and slow when its words
// give none (ours: a line's snare's half pace, workbook.go's snareFactor, for 18 s).
const (
	procSnareFor = 18 * time.Second
	procSnarePct = 50
)

// procRestoreReach is how far from the wielder a group member a restoring
// proc reaches stands at most (ours: the group spells' radius).
const procRestoreReach = 30

// WeaponProc is a weapon's proc.
type WeaponProc struct {
	Item string // the record's name, for the log
	Kind ProcKind
	// Damage is a hit's or a tap's figure, or a DoT's a tick; a range rolls
	// from Damage to DamageTo each proc
	Damage, DamageTo int
	// Power is a restoring proc's power back (its hit points are Damage)
	Power    int
	Duration time.Duration // a DoT's
	Resist   Stat          // the element the creature resists it by (AR when none is said)
	Only     string        // the creatures it fires on alone, by a word of their name ("goblin")
	FX       uint32        // its visual
	pattern  int           // the record, for a DoT's or a form's effect id
	// Chance is its chance a hit in percent, 0 for the rule by DEX
	// (weaponProcChancePct); a form's own proc has the same chance.
	Chance int
	// ChanceFrom and ChanceMost are a chance by DEX of its own, in place of Chance: ChanceFrom and one more for each
	// weaponProcDexPer of its bearer's DEX, at most ChanceMost (the owner, 2026-11-13, of the Staff of Faykind: "let
	// dex raise the chance"); a form's own proc has the same.
	ChanceFrom, ChanceMost int
	// Form, Body and Icon are a form's name on the effects list, its body and
	// its icon; Duration its time. Damage to DamageTo are its own proc's, and
	// RoundMost the most that proc does in one of its bearer's rounds.
	Form      string
	Body      uint32
	Icon      uint32
	RoundMost int
	// DurationTo is a form's longest time when it is a range: each proc gives a time from Duration to DurationTo
	// (the owner, 2026-11-13, of the Staff of Faykind: "form lasts 45-60 secs"); 0 Duration alone
	DurationTo time.Duration
	// FormStat and FormStatBy are a stat the form raises while it lasts, by that much
	// (the Staff of Faykind's fairy form: 15 DEX); Scale and ScalePct a share of its
	// bearer's stat the form's own proc adds at each proc ("380+70%Dex"), and the
	// form's proc is resisted by Resist (its "CR proc"; none said: AR).
	FormStat   Stat
	FormStatBy int
	Scale      Stat
	ScalePct   int
	// Root and Snare are a hold its proc lays as well as (or, for the kinds ProcRoot
	// and ProcSnare, instead of) its damage: a root, a snare of SnarePct (0:
	// procSnarePct); HoldFor its time (0: procRootFor, procSnareFor). Petition 1821
	// (the GM Jibrish: "Enable code for weapons to have working root or snare
	// procs"): any record whose words name one (WeaponProcFromText's procHoldRe).
	Root, Snare bool
	SnarePct    int
	HoldFor     time.Duration
}

// rootFor is how long its root holds.
func (wp *WeaponProc) rootFor() time.Duration {
	if wp.HoldFor > 0 {
		return wp.HoldFor
	}
	return procRootFor
}

// snareFor and snarePct are its snare's time and slow.
func (wp *WeaponProc) snareFor() time.Duration {
	if wp.HoldFor > 0 {
		return wp.HoldFor
	}
	return procSnareFor
}

func (wp *WeaponProc) snarePct() int {
	if wp.SnarePct > 0 {
		return min(wp.SnarePct, 100)
	}
	return procSnarePct
}

// holdWords is what its hold says, "" for none: "a root for 12 s", "a 50% snare for 18 s".
func (wp *WeaponProc) holdWords() string {
	switch {
	case wp.Root || wp.Kind == ProcRoot:
		return fmt.Sprintf("a root for %.0f s", wp.rootFor().Seconds())
	case wp.Snare || wp.Kind == ProcSnare:
		return fmt.Sprintf("a %d%% snare for %.0f s", wp.snarePct(), wp.snareFor().Seconds())
	}
	return ""
}

// chancePct is the chance, in percent, that a hit of a bearer of that DEX procs
// it.
func (wp *WeaponProc) chancePct(dex int) int {
	if wp.ChanceFrom > 0 {
		return dexChancePct(wp.ChanceFrom, wp.ChanceMost, dex)
	}
	if wp.Chance > 0 {
		return wp.Chance
	}
	return weaponProcChancePct(dex)
}

// dexChancePct is a chance from from and one more for each weaponProcDexPer of dex, at most most.
func dexChancePct(from, most, dex int) int {
	return min(from+max(dex, 0)/weaponProcDexPer, max(most, from))
}

// weaponProcChancePct is the chance, in percent, that a hit procs a weapon of
// no chance of its own for a wielder of that DEX: 3%, one more each 20, at
// most 25%, by DEX (the file's doc): what proc.go's INT rule was until the beta
// sheet's took its place on 2026-11-11, kept for a weapon (petition 1360).
func weaponProcChancePct(dex int) int {
	return min(weaponProcBasePct+max(dex, 0)/weaponProcDexPer, weaponProcMostPct)
}

// A weapon's proc chance by DEX (weaponProcChancePct).
const (
	weaponProcBasePct = 3
	weaponProcDexPer  = 20
	weaponProcMostPct = 25
)

// procDoTFor is how long a weapon's damage over time lasts (ours).
const procDoTFor = 30 * time.Second

// fxProcLifeTap is Life Tap's visual (spelleffect 880's fxdict, 0xAF72B893).
const fxProcLifeTap uint32 = 0xAF72B893

// weaponProcsByName are the weapons whose proc no description here gives, by
// their record's name in any case.
var weaponProcsByName = map[string]WeaponProc{
	// EQOA-Data's "proc" figures of the level 49 epic weapons
	"selo's jeweled rapier": {Kind: ProcHit, Damage: 140},
	// petition 1048, the GM Jibrish: "Hammer of the paragon should be hp50 50
	// power AoE proc" (and 1047, Etman: it never procced in 150 swings): 50
	// hit points and 50 power to its wielder and the group round it
	"hammer of the paragon": {Kind: ProcRestore, Damage: 50, Power: 50},
	"scimitar of fury":      {Kind: ProcHit, Damage: 100},
	"virtus ferrum":         {Kind: ProcHit, Damage: 100},
	// its "50 DoT": a dagger's, read as a poison (ours), for procDoTFor
	"blacktongue": {Kind: ProcDoT, Damage: 50, Resist: StatPR},
	// its "Anti-aggro"; this server's record of it is "Sunsilver Blade" (701007)
	"sunsilver":       {Kind: ProcCalm},
	"sunsilver blade": {Kind: ProcCalm},
	// EQOA-Data's 5, and the records' "a chance to do additional poison damage"
	"greater corrosive stick":  {Kind: ProcHit, Damage: 5, Resist: StatPR},
	"superior corrosive stick": {Kind: ProcHit, Damage: 5, Resist: StatPR},
	// described with no figure (ours): the Flamberge of Horror's 25, the one
	// lifetap with a figure; Blacktongue's 50, the other level 49 DoT; the
	// low end of the level 47-49 weapons' hits (Thornsting's 75-100)
	"bonesaw":               {Kind: ProcTap, Damage: 25},
	"flamberge of wounding": {Kind: ProcDoT, Damage: 50},
	"neuronite dirk":        {Kind: ProcHit, Damage: 75},
	// the file's doc. The body is the pixies' of the disc (CHAR.ESF 0x94BA77CD,
	// the NPCs "pixie" and "pixie nettle"), the icon Faerie Form's. Its form's
	// proc and stat are QuestMasters' list of procing items (2006, the owner's
	// source 2026-11-12: "staff of faykind (All/All): procs a fairy form that
	// gives you 15 Dex and gives you a 380+70%Dex CR proc"; "Faystaff ...
	// PROCs are scalable 350-999?, I think is CR based"); until then 400-800,
	// AR. The owner's "up to 800" a round (2026-11-07) still holds.
	// The owner, 2026-11-13: "make it scale to 800 and let dex raise the chance": 380 + 89% of DEX, 800 at the DEX
	// cap of 475 (the round's most 800 holds it there), and its chance, the form's and the form's proc's, 20% and one
	// more for each 20 DEX, at most 45% (30% at 200 DEX, 43% at the cap; until then a flat 30% and 70% of DEX); and
	// "form lasts 45-60 secs" (until then 30 s), each proc's time drawn anew.
	"staff of faykind": {Kind: ProcForm, ChanceFrom: 20, ChanceMost: 45, Form: "Pixie Form", Body: 0x94BA77CD, Icon: 0xDE1BC5EC,
		Duration: 45 * time.Second, DurationTo: 60 * time.Second, Damage: 380, Scale: StatDex, ScalePct: 89, Resist: StatCR, RoundMost: 800,
		FormStat: StatDex, FormStatBy: 15},
	// QuestMasters' list (2006): "noroxus spine spear (War, Ran, Pal, and SK):
	// procs root" (no figure: procRootFor)
	"noroxus spine spear": {Kind: ProcRoot},
	// the same list: "Song of Melospiza, lvl 40 caster weapon, ... anti-aggro
	// proc?" (this server's record is "Song of the Melospiza", 2294), as
	// Sunsilver's
	"song of the melospiza": {Kind: ProcCalm},
	"song of melospiza":     {Kind: ProcCalm},
	// the same list: "Deadened axe/dagger/hammer (34ish life tap proc)", by name
	// so that the records' "wards its wielder against fire" does not make the
	// tap a fire one (WeaponProcFromText's element is any word of the record's)
	"deadened waraxe": {Kind: ProcTap, Damage: 34},
	"deadened hammer": {Kind: ProcTap, Damage: 34},
}

// weaponProcLooks are records made in another weapon's look, keeping that weapon's figures and its proc under a name of
// their own: each takes its original's entry in weaponProcsByName (petition 1371, Uzii, 2026-11-13: "steelwind staff
// not proccing", the Staff of Faykind's figures in the Steelwind's look, made for transfer 93, kept its stats and lost
// its pixie form, its proc being the name's).
var weaponProcLooks = map[string]string{
	"steelwind staff": "staff of faykind",
}

func init() {
	for look, original := range weaponProcLooks {
		if wp, ok := weaponProcsByName[original]; ok {
			weaponProcsByName[look] = wp
		}
	}
}

// procSpellsByName are the spells a description names its proc by ("Procs:
// Life Tap"), as this server keeps them: Life Tap is spellpattern 880,
// "Target, Drain HP by 20 give self".
var procSpellsByName = map[string]WeaponProc{
	"life tap": {Kind: ProcTap, Damage: 20, FX: fxProcLifeTap},
}

var (
	procNamedRe  = regexp.MustCompile(`(?i)\bprocs?\s*:\s*([a-z' ]+?)\s*(?:\.|$)`)
	procFigureRe = regexp.MustCompile(`(?i)\bprocs?\s+(?:a\s+|for\s+around\s+)?(\d+)(?:\s*-\s*(\d+))?`)
	ddFigureRe   = regexp.MustCompile(`(?i)\b(\d+)\s+dd\b[^.]*\bproc`)
	onlyOnRe     = regexp.MustCompile(`(?i)\bworks only on ([a-z]+?)s?\b`)
	// procChanceRe is a proc's own chance in its words: "10% chance", "a 10%
	// chance each hit", "10% a hit", "10% per swing" (never a scale's "30%
	// based on DEX")
	procChanceRe = regexp.MustCompile(`(?i)\b(\d{1,3})\s*%\s*(?:chance\b|(?:a|an|per|each)\s+(?:hit|swing)s?\b)`)
	// procHoldRe is a proc that holds or slows, in one sentence: "Procs a root.",
	// "Procs: Snare", "procs a 6 second root", "root proc", "a snaring proc", "Proc:
	// Ensnare 40%"; never a "Rootweave" nor a root of a tree with no proc beside it
	procHoldRe = regexp.MustCompile(`(?i)\bprocs?\b[^.\n]*?\b(roots?|rooting|snares?|snaring|ensnares?)\b|\b(root|rooting|snare|snaring|ensnare)\s+procs?\b`)
	// notAFigureRe is what after a proc's number makes it a time or a share, not
	// its damage ("Procs a 6 second root", "Procs a 40% snare")
	notAFigureRe = regexp.MustCompile(`(?i)^\s*(?:-\s*)?(?:%|s\b|secs?\b|seconds?\b)`)
	// holdForRe is a hold's own time: "6 second", "6 sec", "6s", "for 6 seconds"
	holdForRe = regexp.MustCompile(`(?i)\b(\d{1,3})\s*(?:-\s*)?(?:s|secs?|seconds?)\b`)
	// snarePctRe is a snare's own slow: "a 40% snare", "snare 40%", "slows by 40%"
	snarePctRe = regexp.MustCompile(`(?i)\b(\d{1,3})\s*%\s*(?:snare|slow|ensnare)|\b(?:snares?|ensnares?|slows?)\s*(?:of|by|for)?\s*(\d{1,3})\s*%`)
)

// procHoldOf is the hold a description's proc lays, its sentence's: root or snare,
// its own time and slow when its words give them; false for none.
func procHoldOf(desc string) (root, snare bool, pct int, hold time.Duration, ok bool) {
	loc := procHoldRe.FindStringSubmatchIndex(desc)
	if loc == nil {
		return false, false, 0, 0, false
	}
	word := ""
	for i := 2; i+1 < len(loc); i += 2 {
		if loc[i] >= 0 {
			word = strings.ToLower(desc[loc[i]:loc[i+1]])
		}
	}
	root = strings.HasPrefix(word, "root")
	snare = !root
	// the sentence it is in: its time and slow are its own
	start, end := strings.LastIndexAny(desc[:loc[0]], ".\n")+1, len(desc)
	if i := strings.IndexAny(desc[loc[1]:], ".\n"); i >= 0 {
		end = loc[1] + i
	}
	sentence := desc[start:end]
	if m := holdForRe.FindStringSubmatch(sentence); m != nil {
		if n, _ := strconv.Atoi(m[1]); n > 0 {
			hold = time.Duration(n) * time.Second
		}
	}
	if snare {
		if m := snarePctRe.FindStringSubmatch(sentence); m != nil {
			v := m[1]
			if v == "" {
				v = m[2]
			}
			if n, _ := strconv.Atoi(v); n >= 1 && n <= 100 {
				pct = n
			}
		}
	}
	return root, snare, pct, hold, true
}

// WeaponProcFromText is the proc a weapon's description gives, false when it
// gives none, or none with a figure (weaponProcsByName has those).
func WeaponProcFromText(desc string) (WeaponProc, bool) {
	lower := strings.ToLower(desc)
	if m := procNamedRe.FindStringSubmatch(desc); m != nil {
		if wp, ok := procSpellsByName[strings.ToLower(strings.TrimSpace(m[1]))]; ok {
			return wp, true
		}
	}
	var lo, hi int
	if m := procFigureRe.FindStringSubmatchIndex(desc); m != nil && !notAFigureRe.MatchString(desc[m[1]:]) {
		lo, _ = strconv.Atoi(desc[m[2]:m[3]])
		if m[4] >= 0 {
			hi, _ = strconv.Atoi(desc[m[4]:m[5]])
		}
	} else if m := ddFigureRe.FindStringSubmatch(desc); m != nil {
		lo, _ = strconv.Atoi(m[1])
	}
	root, snare, pct, hold, holds := procHoldOf(desc)
	if lo <= 0 && !holds {
		return WeaponProc{}, false
	}
	wp := WeaponProc{Kind: ProcHit, Damage: lo, Root: root, Snare: snare, SnarePct: pct, HoldFor: hold}
	if lo <= 0 { // a hold alone: no damage beside it
		wp.Kind, wp.Root, wp.Snare = ProcRoot, false, false
		if snare {
			wp.Kind = ProcSnare
		}
	}
	if hi > lo {
		wp.DamageTo = hi
	}
	switch {
	case strings.Contains(lower, "lifetap"), strings.Contains(lower, "life tap"):
		wp.Kind = ProcTap
	case strings.Contains(lower, " dot"):
		wp.Kind = ProcDoT
	}
	wp.Resist = elementOf(lower)
	if m := onlyOnRe.FindStringSubmatch(desc); m != nil {
		wp.Only = strings.ToLower(m[1])
	}
	if m := procChanceRe.FindStringSubmatch(desc); m != nil {
		if c, _ := strconv.Atoi(m[1]); c >= 1 && c <= 100 {
			wp.Chance = c
		}
	}
	return wp, true
}

// procElements are the words that name a proc's element, in the order
// they are looked for.
var procElements = []struct {
	re *regexp.Regexp
	st Stat
}{{regexp.MustCompile(`\bdisease`), StatDR}, {regexp.MustCompile(`\bpoison`), StatPR},
	{regexp.MustCompile(`\bice\b`), StatCR}, {regexp.MustCompile(`\bfrost`), StatCR}, {regexp.MustCompile(`\bcold\b`), StatCR},
	{regexp.MustCompile(`\bfire\b`), StatFR}, {regexp.MustCompile(`\bflame`), StatFR}, {regexp.MustCompile(`\blightning`), StatLR},
	// petition 1053, Hoofbite: Davoh's Dirk "on hit, deals 100 piercing damage"
	{regexp.MustCompile(`\bpierc`), StatPierce},
	// petition 1115, Hoofbite: Keelcleaver's "150 dmg slashing proc"
	{regexp.MustCompile(`\bslash`), StatSlash}}

// elementOf is the element a description's proc is of, by its words: AR
// (arcane, the enchanter's procs' and the server's default) when it names
// none.
func elementOf(lower string) Stat {
	for _, e := range procElements {
		if e.re.MatchString(lower) {
			return e.st
		}
	}
	return StatAR
}

// WeaponProcOf is the proc of an item worn in a hand, by its name, else its
// description; false for an item that has none. A copy's is its base's
// (ascended.go: an Ascended record's by its base's name, a tenth more; an
// import's damped).
func WeaponProcOf(patternID int, itemName, desc string) (WeaponProc, bool) {
	_, lookup, imported, ascended := copyRecord(patternID, itemName)
	wp, ok := weaponProcsByName[strings.ToLower(strings.TrimSpace(lookup))]
	if !ok {
		wp, ok = WeaponProcFromText(desc)
	}
	if !ok {
		return WeaponProc{}, false
	}
	if wp.Resist == StatNone {
		wp.Resist = StatAR
	}
	if wp.FX == 0 {
		switch wp.Kind {
		case ProcTap:
			wp.FX = fxProcLifeTap
		case ProcCalm, ProcRestore, ProcForm, ProcRoot, ProcSnare:
			wp.FX = fxBuff
		default:
			wp.FX = fxDamage
		}
	}
	wp.Item, wp.pattern = itemName, patternID
	if ascended { // a tuned boss's Ascended record: a tenth more (ascended.go)
		wp = wp.ascended()
	}
	if imported { // an import's copy: its damper (imported.go)
		wp = wp.imported()
	}
	return wp, true
}

// String is the proc as #stats says it.
func (wp WeaponProc) String() string {
	figure := strconv.Itoa(wp.Damage)
	if wp.DamageTo > wp.Damage {
		figure = fmt.Sprintf("%d-%d", wp.Damage, wp.DamageTo)
	}
	var s string
	switch wp.Kind {
	case ProcTap:
		s = figure + " tap"
	case ProcDoT:
		s = fmt.Sprintf("%s a tick for %.0f s", figure, procDoTFor.Seconds())
	case ProcCalm:
		s = "its hate halved"
	case ProcRestore:
		s = fmt.Sprintf("%s hit points and %d power to its wielder and group", figure, wp.Power)
	case ProcForm:
		long := wp.formLong()
		if wp.Scale != StatNone && wp.ScalePct != 0 {
			figure += fmt.Sprintf(" + %d%% of %s", wp.ScalePct, procStatWord(wp.Scale))
		}
		if wp.Resist != StatNone && wp.Resist != StatAR {
			figure += " " + statName(wp.Resist)
		}
		form := wp.Form
		if wp.FormStat != StatNone && wp.FormStatBy != 0 {
			form += fmt.Sprintf(" (%+d %s)", wp.FormStatBy, procStatWord(wp.FormStat))
		}
		chance := wp.formChance()
		s = fmt.Sprintf("%s for %s, which procs %s at %s a hit, at most %d a round", form, long, figure, chance, wp.RoundMost)
	case ProcRoot, ProcSnare:
		s = wp.holdWords()
	default:
		s = figure
	}
	if h := wp.holdWords(); h != "" && wp.Kind != ProcRoot && wp.Kind != ProcSnare {
		s += " and " + h
	}
	if wp.Chance > 0 && wp.Kind != ProcForm { // its own chance (a form's is in its words)
		s += fmt.Sprintf(" at %d%% a hit", wp.Chance)
	}
	if wp.Only != "" {
		s += ", on " + wp.Only + "s alone"
	}
	return s
}

// weaponProcSpellID is the id a weapon's damage over time goes by on the
// creature, apart from every spell's and the workbook procs' (procSpellID).
func weaponProcSpellID(pattern int) int { return -3_000_000 - pattern }

// handProcLocked is the proc of the hand that made the player's hit, hit its
// number in the round (RoundHits, from 1): with a weapon in each hand the odd
// ones are the main hand's and the even ones the off hand's, else every hit
// is the main hand's. Nil for a hand with no proc.
func handProcLocked(p *Player, hit int) *WeaponProc {
	g := &p.Gear
	if g.OffArmed && !g.WeaponTwoHanded && hit > 0 && hit%2 == 0 {
		return g.OffProc
	}
	return g.MainProc
}

// weaponProcLocked runs the proc of the hand that landed the player's hit on
// a creature still standing, if it procs (the file's doc). It reports a
// kill.
func (w *World) weaponProcLocked(sess *session.Session, p *Player, npc *NPC, now time.Time) bool {
	wp := handProcLocked(p, p.RoundHits)
	if wp == nil || npc.Dead || p.IsDead() {
		return false
	}
	if wp.Only != "" && !strings.Contains(strings.ToLower(npc.Name), wp.Only) {
		return false
	}
	if wp.Kind == ProcForm {
		w.formProcLocked(sess, p, wp, now)
		return false
	}
	if procIntn(100) >= wp.chancePct(p.StatNow(StatDex)) {
		return false
	}
	// resisted, a proc that hurts does the least it can (workbook.go's leastHarm); one that
	// only calms does nothing
	resisted := wp.Kind != ProcRestore && npc.Dummy == NotDummy && procIntn(100) < npcSpellResistPct(p.Level, npc, wp.Resist, now)
	if resisted && (wp.Kind == ProcCalm || wp.Kind == ProcRoot || wp.Kind == ProcSnare) {
		log.Printf("Proc: %s's %q resisted by %s", p.Name, wp.Item, npc.Name)
		return false
	}
	n := wp.Damage
	if resisted {
		log.Printf("Proc: %s's %q resisted by %s: the least it does, %d", p.Name, wp.Item, npc.Name, n)
	} else if wp.DamageTo > wp.Damage {
		n += procIntn(wp.DamageTo - wp.Damage + 1)
	}
	switch wp.Kind {
	case ProcRestore:
		w.restoreProcLocked(sess, p, wp, n)
		return false
	case ProcCalm:
		w.broadcastCastLocked(p, p.ServerID, 0, wp.FX)
		if t, ok := npc.Threats[p.ServerID]; ok && npc.Dummy == NotDummy {
			npc.Threats[p.ServerID] = max(t/2, 1)
			w.addThreatLocked(npc, p.ServerID, 0) // its enemy the highest again
		}
		log.Printf("Proc: %s's %q calms %s (its hate %d)", p.Name, wp.Item, npc.Name, npc.Threats[p.ServerID])
		return false
	case ProcRoot, ProcSnare:
		w.broadcastCastLocked(p, npc.ServerID, 0, wp.FX)
		w.weaponHoldLocked(sess, p, wp, npc, now)
		return npc.Dead
	case ProcDoT:
		w.broadcastCastLocked(p, npc.ServerID, 0, wp.FX)
		log.Printf("Proc: %s's %q lays %d a tick on %s", p.Name, wp.Item, n, npc.Name)
		w.landOnNPCLocked(weaponDoTCast(p, wp, npc.ServerID, n), p, sess, npc, now)
		if !resisted && !npc.Dead {
			w.weaponHoldLocked(sess, p, wp, npc, now) // its hold beside it, if any
		}
		return npc.Dead
	}
	w.broadcastCastLocked(p, npc.ServerID, 0, wp.FX)
	before := npc.HP
	log.Printf("Proc: %s's %q hits %s for %d", p.Name, wp.Item, npc.Name, n)
	killed := w.spellHitLocked(p, sess, npc, n, numberProc)
	if wp.Kind == ProcTap && !p.IsDead() {
		back := before - npc.HP
		if npc.Dummy != NotDummy {
			back = n // a dummy is told the figure and takes nothing
		}
		w.procHealLocked(p, sess, p, back)
	}
	if !killed && !resisted && !npc.Dead {
		w.weaponHoldLocked(sess, p, wp, npc, now) // its hold beside the hit, if any
	}
	return killed
}

// weaponHoldLocked lays a weapon proc's root or snare on the creature (none when it
// has neither) as a spell of the wielder's lands: the creature's root resist by its
// con (root.go's rootResistLocked: the balance's root_resist_pct, an arcane debuff
// lowering it; a snare resisted as a root is), a root broken by a later hit as any
// root, and its hate. The proc's own resist roll came first (weaponProcLocked). Must
// be called with the lock held.
func (w *World) weaponHoldLocked(sess *session.Session, p *Player, wp *WeaponProc, npc *NPC, now time.Time) {
	switch {
	case wp.Root || wp.Kind == ProcRoot:
		log.Printf("Proc: %s's %q roots %s for %v", p.Name, wp.Item, npc.Name, wp.rootFor())
		w.landOnNPCLocked(weaponRootCast(p, wp, npc.ServerID), p, sess, npc, now)
	case wp.Snare || wp.Kind == ProcSnare:
		log.Printf("Proc: %s's %q snares %s (%d%%) for %v", p.Name, wp.Item, npc.Name, wp.snarePct(), wp.snareFor())
		w.landOnNPCLocked(weaponSnareCast(p, wp, npc.ServerID), p, sess, npc, now)
	}
}

// formTime is how long one proc's form lasts: Duration, or a time from it to DurationTo.
func (wp *WeaponProc) formTime() time.Duration {
	if wp.DurationTo <= wp.Duration {
		return wp.Duration
	}
	return wp.Duration + time.Duration(procIntn(int((wp.DurationTo-wp.Duration)/time.Second)+1))*time.Second
}

// formLong is a form's time as #stats and the panel say it: "30 s", "45-60 s", "5 min".
func (wp *WeaponProc) formLong() string {
	if wp.DurationTo > wp.Duration {
		return fmt.Sprintf("%.0f-%.0f s", wp.Duration.Seconds(), wp.DurationTo.Seconds())
	}
	if wp.Duration < time.Minute {
		return fmt.Sprintf("%.0f s", wp.Duration.Seconds())
	}
	return fmt.Sprintf("%.0f min", wp.Duration.Minutes())
}

// formChance is a form proc's chance as #stats and the panel say it.
func (wp *WeaponProc) formChance() string {
	if wp.ChanceFrom > 0 {
		return fmt.Sprintf("%d%% (+1%% each %d DEX, at most %d%%)", wp.ChanceFrom, weaponProcDexPer, wp.ChanceMost)
	}
	return fmt.Sprintf("%d%%", wp.Chance)
}

// formProcLocked gives the wielder the weapon's form, if it procs and the form
// is not on it already (the file's doc). Must be called with the lock held.
func (w *World) formProcLocked(sess *session.Session, p *Player, wp *WeaponProc, now time.Time) {
	id := weaponProcSpellID(wp.pattern)
	if procIntn(100) >= wp.chancePct(p.StatNow(StatDex)) {
		return
	}
	// the form on: the proc renews its time, nothing else (the file's doc)
	for _, e := range p.Effects {
		if e.SpellID == id && now.Before(e.Until) {
			long := wp.formTime()
			e.Until = now.Add(long)
			w.broadcastCastLocked(p, p.ServerID, 0, wp.FX)
			log.Printf("Proc: %s's %q renews its %s for %v", p.Name, wp.Item, wp.Form, long)
			return
		}
	}
	cs := &castState{spell: CastSpell{SpellID: id, Name: wp.Form, Icon: wp.Icon, Level: p.Level, Def: SpellDef{Target: TargetSelf,
		Duration: wp.formTime(), FormKind: FormSelf, FormModel: wp.Body, ProcChance: wp.Chance, ProcRound: wp.RoundMost,
		ProcChanceFrom: wp.ChanceFrom, ProcChanceMost: wp.ChanceMost,
		Effects: formEffects(wp)}},
		caster: p.ServerID, targets: []EntityID{p.ServerID}}
	w.broadcastCastLocked(p, p.ServerID, 0, wp.FX)
	log.Printf("Proc: %s's %q gives it %s", p.Name, wp.Item, wp.Form)
	w.landOnPlayerLocked(cs, p, sess, p, now)
}

// procStatWord is a stat's short name as #stats says a proc's scale and a form's stat.
func procStatWord(s Stat) string {
	if w, ok := map[Stat]string{StatStr: "STR", StatSta: "STA", StatAgi: "AGI", StatDex: "DEX", StatWis: "WIS",
		StatInt: "INT", StatCha: "CHA"}[s]; ok {
		return w
	}
	return statName(s)
}

// formEffects are what a weapon's form does while it lasts: the body, its own proc (its
// bearer's stat's share at each proc, resisted by its element) and the stat it raises.
func formEffects(wp *WeaponProc) []SpellEffect {
	proc := SpellEffect{Kind: EffectProc, Amount: wp.Damage, AmountMax: wp.DamageTo, Resist: wp.Resist}
	if wp.Scale != StatNone && wp.ScalePct != 0 {
		proc.Scale, proc.ScalePct, proc.ScaleBearer = wp.Scale, wp.ScalePct, true
	}
	effects := []SpellEffect{{Kind: EffectIllusion}, proc}
	if wp.FormStat != StatNone && wp.FormStatBy != 0 {
		effects = append(effects, SpellEffect{Kind: EffectStat, Stat: wp.FormStat, Amount: wp.FormStatBy, AmountMax: wp.FormStatBy})
	}
	return effects
}

// weaponRootCast is a weapon proc's root as a spell of p's at target, for its time.
func weaponRootCast(p *Player, wp *WeaponProc, target EntityID) *castState {
	return &castState{spell: CastSpell{SpellID: weaponHoldSpellID(wp), Name: wp.Item, Level: p.Level, Scope: 1,
		Def: SpellDef{Effects: []SpellEffect{{Kind: EffectRoot}}, Duration: wp.rootFor(), Harmful: true}},
		caster: p.ServerID, targets: []EntityID{target}}
}

// weaponSnareCast is a weapon proc's snare as a spell of p's at target: a workbook
// slow of its percent (workbook.go's SpeedPct, which snareFactor reads), for its time.
func weaponSnareCast(p *Player, wp *WeaponProc, target EntityID) *castState {
	return &castState{spell: CastSpell{SpellID: weaponHoldSpellID(wp), Name: wp.Item, Level: p.Level, Scope: 1,
		Def: SpellDef{Effects: []SpellEffect{{Kind: EffectSnare, Amount: -wp.snarePct()}}, Duration: wp.snareFor(),
			Harmful: true, Workbook: true}},
		caster: p.ServerID, targets: []EntityID{target}}
}

// weaponHoldSpellID is the id a weapon's hold goes by on the creature: the proc's own
// (weaponProcSpellID) for a hold alone, apart from it beside a damage over time.
func weaponHoldSpellID(wp *WeaponProc) int {
	if wp.Kind == ProcRoot || wp.Kind == ProcSnare {
		return weaponProcSpellID(wp.pattern)
	}
	return -4_000_000 - wp.pattern
}

// weaponDoTCast is a weapon proc's damage over time as a spell of p's at target, n a tick.
func weaponDoTCast(p *Player, wp *WeaponProc, target EntityID, n int) *castState {
	return &castState{spell: CastSpell{SpellID: weaponProcSpellID(wp.pattern), Name: wp.Item, Level: p.Level, Scope: 1,
		Def:     SpellDef{Effects: []SpellEffect{{Kind: EffectHPTick, Amount: -n, AmountMax: -n}}, Duration: procDoTFor, Harmful: true},
		figures: figuresProc}, // its ticks a proc's (numbertags.go)
		caster: p.ServerID, targets: []EntityID{target}}
}

// weaponProcWords is the #stats line of the player's weapon procs, its
// chance a hit at its DEX for those of no chance of their own (a proc's own
// is in its words); "" with none.
func weaponProcWords(p *Player) string {
	g := &p.Gear
	var parts []string
	byDEX := false
	if g.MainProc != nil {
		parts = append(parts, fmt.Sprintf("%s %s", g.MainProc.Item, g.MainProc))
		byDEX = byDEX || g.MainProc.Chance <= 0
	}
	if g.OffProc != nil && g.OffArmed && !g.WeaponTwoHanded {
		parts = append(parts, fmt.Sprintf("off hand %s %s", g.OffProc.Item, g.OffProc))
		byDEX = byDEX || g.OffProc.Chance <= 0
	}
	if len(parts) == 0 {
		return ""
	}
	if !byDEX {
		return "Weapon procs: " + strings.Join(parts, "; ")
	}
	return fmt.Sprintf("Weapon procs: %s; %d%% a hit of its hand (DEX %d)", strings.Join(parts, "; "),
		weaponProcChancePct(p.StatNow(StatDex)), p.StatNow(StatDex))
}

// restoreProcLocked lands a restoring proc: n hit points and wp.Power power to
// the wielder and to each living member of its group in its world within
// procRestoreReach, each heal told as a spell's is (healLocked: 0x00DB to the
// wielder and the one healed), a buff's visual on the wielder. Must be called
// with the lock held.
func (w *World) restoreProcLocked(sess *session.Session, p *Player, wp *WeaponProc, n int) {
	w.broadcastCastLocked(p, p.ServerID, 0, wp.FX)
	to := []*Player{p}
	if g := w.groups[p.GroupID]; p.GroupID != 0 && g != nil {
		for _, id := range g.Members {
			m := w.Players[id]
			if m == nil || m == p || m.IsDead() || m.World != p.World {
				continue
			}
			if dx, dz := m.X-p.X, m.Z-p.Z; dx*dx+dz*dz > procRestoreReach*procRestoreReach {
				continue
			}
			to = append(to, m)
		}
	}
	for _, t := range to {
		w.procHealLocked(p, sess, t, n)
		if wp.Power > 0 && t.Power < t.MaxPowerNow() {
			t.Power = min(t.Power+wp.Power, t.MaxPowerNow())
			t.StatsDirty = true
		}
	}
	log.Printf("Proc: %s's %q restores %d hit points and %d power to %d", p.Name, wp.Item, n, wp.Power, len(to))
}
