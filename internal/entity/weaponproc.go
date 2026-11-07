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
// hold no weapon proc), so it is proc.go's rule for the enchanter's procs,
// chosen there: each landed hit of the hand that holds the weapon procs at
// 3% and one more for each 20 INT, at most 25%; the creature resists it all
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
//   - a proc for a kind of creature alone (Gorereaver's goblins) fires on a
//     creature whose name names it, and not otherwise.
//   - a form (the Staff of Faykind's; the user, 2026-11-07: "lets add staff
//     of faykind. it procs a pixie form, that lasts 10 min, the pixie form can
//     proc up to 800 per round, the proc percentage was around 30% for the
//     transform and the damage proc"): its wielder shown in a body for its
//     time (an illusion, illusion.go, kept over a logout and ended as any is),
//     unresisted, not begun again while it lasts. The damage is the form's own
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
	// Chance is its chance a hit in percent, 0 for proc.go's rule by INT; a
	// form's own proc has the same chance.
	Chance int
	// Form, Body and Icon are a form's name on the effects list, its body and
	// its icon; Duration its time. Damage to DamageTo are its own proc's, and
	// RoundMost the most that proc does in one of its bearer's rounds.
	Form      string
	Body      uint32
	Icon      uint32
	RoundMost int
}

// chancePct is the chance, in percent, that a hit of a bearer of that INT procs
// it.
func (wp *WeaponProc) chancePct(intel int) int {
	if wp.Chance > 0 {
		return wp.Chance
	}
	return procChancePct(intel)
}

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
	// the NPCs "pixie" and "pixie nettle"), the icon Faerie Form's. "Up to 800"
	// gives the top alone; half of it as the low end is ours
	"staff of faykind": {Kind: ProcForm, Chance: 30, Form: "Pixie Form", Body: 0x94BA77CD, Icon: 0xDE1BC5EC,
		Duration: 10 * time.Minute, Damage: 400, DamageTo: 800, RoundMost: 800},
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
)

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
	if m := procFigureRe.FindStringSubmatch(desc); m != nil {
		lo, _ = strconv.Atoi(m[1])
		hi, _ = strconv.Atoi(m[2])
	} else if m := ddFigureRe.FindStringSubmatch(desc); m != nil {
		lo, _ = strconv.Atoi(m[1])
	}
	if lo <= 0 {
		return WeaponProc{}, false
	}
	wp := WeaponProc{Kind: ProcHit, Damage: lo}
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
// description; false for an item that has none.
func WeaponProcOf(patternID int, itemName, desc string) (WeaponProc, bool) {
	wp, ok := weaponProcsByName[strings.ToLower(strings.TrimSpace(itemName))]
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
		case ProcCalm, ProcRestore, ProcForm:
			wp.FX = fxBuff
		default:
			wp.FX = fxDamage
		}
	}
	wp.Item, wp.pattern = itemName, patternID
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
		s = fmt.Sprintf("%s for %.0f min, which procs %s at %d%% a hit, at most %d a round", wp.Form, wp.Duration.Minutes(), figure, wp.Chance, wp.RoundMost)
	default:
		s = figure
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
	if procIntn(100) >= wp.chancePct(p.StatNow(StatInt)) {
		return false
	}
	if wp.Kind != ProcRestore && npc.Dummy == NotDummy && procIntn(100) < npcSpellResistPct(p.Level, npc, wp.Resist, now) {
		log.Printf("Proc: %s's %q resisted by %s", p.Name, wp.Item, npc.Name)
		return false
	}
	n := wp.Damage
	if wp.DamageTo > wp.Damage {
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
	case ProcDoT:
		w.broadcastCastLocked(p, npc.ServerID, 0, wp.FX)
		log.Printf("Proc: %s's %q lays %d a tick on %s", p.Name, wp.Item, n, npc.Name)
		w.landOnNPCLocked(weaponDoTCast(p, wp, npc.ServerID, n), p, sess, npc, now)
		return npc.Dead
	}
	w.broadcastCastLocked(p, npc.ServerID, 0, wp.FX)
	before := npc.HP
	log.Printf("Proc: %s's %q hits %s for %d", p.Name, wp.Item, npc.Name, n)
	killed := w.spellHitLocked(p, sess, npc, n)
	if wp.Kind == ProcTap && !p.IsDead() {
		back := before - npc.HP
		if npc.Dummy != NotDummy {
			back = n // a dummy is told the figure and takes nothing
		}
		w.healLocked(p, sess, p, back)
	}
	return killed
}

// formProcLocked gives the wielder the weapon's form, if it procs and the form
// is not on it already (the file's doc). Must be called with the lock held.
func (w *World) formProcLocked(sess *session.Session, p *Player, wp *WeaponProc, now time.Time) {
	id := weaponProcSpellID(wp.pattern)
	if p.hasEffect(id) || procIntn(100) >= wp.chancePct(p.StatNow(StatInt)) {
		return
	}
	cs := &castState{spell: CastSpell{SpellID: id, Name: wp.Form, Icon: wp.Icon, Level: p.Level, Def: SpellDef{Target: TargetSelf,
		Duration: wp.Duration, FormKind: FormSelf, FormModel: wp.Body, ProcChance: wp.Chance, ProcRound: wp.RoundMost,
		Effects: []SpellEffect{{Kind: EffectIllusion}, {Kind: EffectProc, Amount: wp.Damage, AmountMax: wp.DamageTo}}}},
		caster: p.ServerID, targets: []EntityID{p.ServerID}}
	w.broadcastCastLocked(p, p.ServerID, 0, wp.FX)
	log.Printf("Proc: %s's %q gives it %s", p.Name, wp.Item, wp.Form)
	w.landOnPlayerLocked(cs, p, sess, p, now)
}

// weaponDoTCast is a weapon proc's damage over time as a spell of p's at target, n a tick.
func weaponDoTCast(p *Player, wp *WeaponProc, target EntityID, n int) *castState {
	return &castState{spell: CastSpell{SpellID: weaponProcSpellID(wp.pattern), Name: wp.Item, Level: p.Level, Scope: 1,
		Def: SpellDef{Effects: []SpellEffect{{Kind: EffectHPTick, Amount: -n, AmountMax: -n}}, Duration: procDoTFor, Harmful: true}},
		caster: p.ServerID, targets: []EntityID{target}}
}

// weaponProcWords is the #stats line of the player's weapon procs, its
// chance a hit at its INT; "" with none.
func weaponProcWords(p *Player) string {
	g := &p.Gear
	var parts []string
	if g.MainProc != nil {
		parts = append(parts, fmt.Sprintf("%s %s", g.MainProc.Item, g.MainProc))
	}
	if g.OffProc != nil && g.OffArmed && !g.WeaponTwoHanded {
		parts = append(parts, fmt.Sprintf("off hand %s %s", g.OffProc.Item, g.OffProc))
	}
	if len(parts) == 0 {
		return ""
	}
	return fmt.Sprintf("Weapon procs: %s; %d%% a hit of its hand (INT %d)", strings.Join(parts, "; "),
		procChancePct(p.StatNow(StatInt)), p.StatNow(StatInt)) // a form's own chance is in its words
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
		w.healLocked(p, sess, t, n)
		if wp.Power > 0 && t.Power < t.MaxPowerNow() {
			t.Power = min(t.Power+wp.Power, t.MaxPowerNow())
			t.StatsDirty = true
		}
	}
	log.Printf("Proc: %s's %q restores %d hit points and %d power to %d", p.Name, wp.Item, n, wp.Power, len(to))
}
