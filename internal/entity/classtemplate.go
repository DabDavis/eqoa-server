package entity

import (
	"fmt"
	"log"
	"math"
	"math/rand"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/session"
)

// classtemplate.go — NPC class templates (jib's F3 20261029-025234-7430ece1:
// "Monsters need classes. Classes should determine stat profile (tank classes
// a little tankier, melee hit a little harder and occasionally kick, wizards
// chain cast 2 level appropriate spells, one aoe one single target, necro's
// cast level appropriate life taps with 5 second delays, SK mobs are tankier
// with level appropriate life taps every 15 seconds). These are example
// templates. We need templates ready so we can apply them to mobs in the
// future, do not apply them yet.").
//
// A template is a row of classtemplates (database/classtemplates.go); the
// NPCs of a name fight by one when npctemplate names it for them (by name, as
// npcclass keys one), and with npctemplate empty, as it is made, none does.
// An NPC of a template:
//
//   - fights as its class (the template's, or its own for -1): CLIENT's
//     GetSwing for it (rounds.go), and the class whose spells its abilities
//     pick from;
//   - has hp_pct of the hit points it had without one (its level's), from its
//     spawn and at every respawn; taken_pct of a player's or a pet's melee hit
//     on it lands (offence.go, pet.go), and its own melee hits on a player or
//     a pet are melee_pct of its roll (defence.go), each at least 1;
//   - kicks (its special) in special_chance of its rounds, a second after the
//     round's last hit: a 0x00B1 of cast 0 with Sandstorm's creature special's
//     visual (0x66CD778E, or special_fx) and the level times special_damage,
//     fixed and through any armour, as Sandstorm's creatures' specials hit
//     (the training dummies' note: a level 34 crab's 251 on a tank of 1,763
//     AC and an enchanter of 167 alike); a damage shield answers it as melee;
//     and the same special on a clock (special_every, the user's SOE roamer:
//     a warrior "stripped down to just a 'crit' attack once every 30 secs or
//     so"): the first special_first into the fight, then one every
//     special_every, each special_jitter either way, on a clock of its time
//     AT its enemy (its turns in reach, templateContactGap apart at most):
//     time closing in, held off by a root or casting is not counted, as
//     Sandstorm's specials come with its rounds (one round in five) and a
//     creature out of reach has none; either only from special_from on, a
//     level (the roamer's 10: the user's "they get it around level 10
//     starting, but before only auto attack"), the creature's own;
//   - casts its abilities in its fights, from where it stands in reach of its
//     enemy (a player, or a player's pet), one at a time, standing still for
//     each cast (no chase, no way round, as a root holds it) and its melee
//     rounds going on through it while its enemy is in its reach (the GM
//     Jibrish's petition 1593, 2026-11-11: "Mobs should stop moving while
//     casting (but the spell always resolves unless otherwise specified);
//     however they should also keep quading IF in melee range of the
//     target"; castHeld). A round begun while it casts sends no swing of its
//     own: the cast's 0x00B1 stays what CLIENT draws, its hits land. Each ability is on a clock of its own (every, first into
//     the fight) or chained to the one before it (cast as soon as that one's
//     cast time is over: the wizard's area spell and then its single
//     target's), with a chance; a heal only below its share of hit points.
//
// What an ability casts is a spell of the game's own, picked as the
// necromancer's pets pick their taps (pettap.go): of the class's spells
// (classspells, never a mastery's, an epic's or a master class's), those of
// the ability's kind (a nuke: a harmful hit of one target; aoe: a harmful hit
// on an area, area.go's reading and a GM's spellarea row; lifetap: a hit of
// one target that heals its caster, lifetap.go's; heal: a heal), their names
// holding the ability's line words, their own recast and cast time no longer
// than its max_recast and max_cast, the highest at or below the NPC's level;
// or the spell its row names, whatever the level. It is cast as a player's
// is (cast.go): made from its ability record as a player's book entry is
// (SpellOf), its 0x00B1 to everyone who sees the NPC with its cast time and
// its visual (its own, else its kind's stand-in) and an area spell's others
// named, landing castLand after its cast time on its target and its others
// still there. What it does there is its hit points now: its hits rolled
// (no stat of the NPC's: it has none, as a pet's tap) at the ability's
// damage_pct, less the share the player's resist to the spell's element takes
// (resist.go; the element by its name's and line's words, else arcane, as a
// pact's price), its 0x00DB to the player and a line saying it, a death; a
// tap's drained share back on the NPC; a heal's figure on itself. What a
// spell would leave lasting on a player (a damage tick, a stat, a hold) is not
// put on, but for a debuff's (petition 1170, Lord Nagafen's: "a single target
// ability that cant be cured and lasts 2 minutes"): the stats its named spell
// takes, on the player it fights, for the spell's time; no_cure keeps every cure
// off it. It hits for nothing, and waits while what it fights is a pet. A strip
// (petition 1368, Mr. Moogie's) casts no spell of a book: at its landing every
// beneficial effect on what it fights, a player or a pet, ends, and a GM's
// #testmode, the god mode, goes off (strip.go).
//
// The numbers of jib's five are ours, the row's notes say which; the kick's
// are Sandstorm's crab's. No recording has a creature casting.

// templateSpecialFX is Sandstorm's creature special's 0x00B1 visual, cast 0
// (claude-notes/training-dummies-20261021.md: the Desert Crab's and the
// Gharial's specials).
const templateSpecialFX uint32 = 0x66CD778E

// templateSpecialAfter is how long after its round's last hit a special
// comes: inside every round's length (Attack1's last hit at 0.5 s of 3.2).
const templateSpecialAfter = time.Second

// templateIntn is the templates' dice (a test's own).
var templateIntn = rand.Intn

// ClassTemplate is a template as the world fights by it: its row, and its
// abilities with the spells they may cast.
type ClassTemplate struct {
	database.ClassTemplate
	abilities []templateAbility
	// healBelow is its heal shield's share of hit points (0 none), kindTaken
	// the share of a player's melee of a kind of weapon that lands (100 when
	// not named): AbilityHealShield, AbilityMeleeKind
	healBelow int
	kindTaken map[Stat]int
	// hitHeal is what every hit that lands on it heals it (AbilityHitHeal,
	// hitheal.go; 0 none), prefer the names of the foes it fights first
	// (AbilityPrefer, prefer.go: lower case, the first first)
	hitHeal int
	prefer  []string
	// summons and enrage are its fight's moments below a share of its hit
	// points (AbilitySummon, summon.go; AbilityEnrage, enrage.go: one at most)
	summons []templateSummon
	// moments is its boss's moments on clocks of their own (bossmoments.go)
	moments []database.TemplateAbility
	enrage  *database.TemplateAbility
	// lastStand is its fight's last stand (AbilityLastStand, laststand.go: one
	// at most)
	lastStand *database.TemplateAbility
	// ward and deathHold are its fight's ward below a share of its hit points
	// and its hold at 1 (AbilityWard, ward.go; AbilityDeathHold, deathhold.go:
	// one each at most)
	ward      *database.TemplateAbility
	deathHold *database.TemplateAbility
	// procs and veils are melee procs it always has beside its template's
	// (AbilityProc, AbilityVeil: creatureproc.go); selfBuffs the buffs those of
	// a pinned spell are, cast on itself as its fight begins and at each one's
	// time (selfbuff.go); killSays its lines at a kill (AbilityKillSay)
	procs     []templateProc
	veils     []templateVeil
	selfBuffs []templateSelfBuff
	killSays  []templateKillSay
	// farCasts: one of its abilities is cast only out of its reach (OutOfReach)
	farCasts bool
	// problems is what of it the world cannot use, for the log and the panel
	problems []string
}

// templateAbility is one of a template's abilities: its row's fields, its
// clock, and the spells it may cast by the class it is cast as (lowest level
// first), or the one its row pins.
type templateAbility struct {
	database.TemplateAbility
	every, first time.Duration
	byClass      [15][]*CastSpell
	pinned       *CastSpell
	// share is a creature's heal of its share that pins no spell: the cast it
	// goes out as (abilitytimes.go); a pet's heals its share with no cast
	share *CastSpell
}

// spellFor is the spell the ability casts for an NPC of the class and level:
// its pinned one, else the highest of its candidates at or below the level;
// nil for none.
func (a *templateAbility) spellFor(class, level int) *CastSpell {
	if a.pinned != nil {
		return a.pinned
	}
	if class < 0 || class > 14 {
		class = 0
	}
	var best *CastSpell
	for _, s := range a.byClass[class] {
		if s.Level <= level {
			best = s
		}
	}
	return best
}

// castTime is how long the ability's cast of s takes: its own cast (the
// record's whole seconds cannot hold the Swarming Deaths' half second), else
// the spell's.
func (a *templateAbility) castTime(s *CastSpell) time.Duration {
	if a.Cast > 0 {
		return time.Duration(math.Round(a.Cast*1000)) * time.Millisecond
	}
	return s.CastTime
}

// templateBase is what a templated NPC was without its template: put back
// when the template is taken away.
type templateBase struct {
	class, maxHP int
}

// templateFight is a templated NPC's fight by its template: whether its
// clocks are set, when each of its abilities next comes, its cast's time (no
// melee, no other cast until then), the chained ability owed after it (its
// index + 1; 0 none), the round its special was rolled for and when it
// lands, and when its timed special next comes (zero: not yet set).
type templateFight struct {
	on      bool
	next    [database.MaxTemplateAbilities]time.Time
	busy    time.Time
	chain   int
	round   time.Time
	special time.Time
	// timedLeft is its timed special's time at its enemy still to go, from
	// timedSeen, its last turn in reach (zero: its clock not begun)
	timedLeft time.Duration
	timedSeen time.Time
	// kickLeft and kickSeen are its kick's clock (KickEvery), as timedLeft's
	kickLeft time.Duration
	kickSeen time.Time
	// proc is the melee proc an ability's landing gave it for the rest of this
	// fight (abilitytimes.go), nil for none: the fight's end takes it
	proc *fightProc
	// summoned is the summons that have called their adds this fight (a bit an
	// index of the template's summons), adds the adds they called (summon.go);
	// enraged its enrage once it is on, nil before it (enrage.go)
	summoned uint32
	adds     []EntityID
	enraged  *enrageOn
	// lastStand is its last stand once it has stood this fight (laststand.go)
	lastStand *lastStandOn
	// warded is its ward once it is up this fight (ward.go), held its hold at 1
	// once it has held (deathhold.go)
	warded *wardOn
	held   *holdOn
	// buffNext is when each of its template's buffs on itself is cast next
	// (selfbuff.go: index as selfBuffs'; nil before its first)
	buffNext []time.Time
	// momentNext is when each of its template's moments comes next
	// (bossmoments.go: index as moments'; nil before its first)
	momentNext []time.Time
	// landedAt is when its last melee hit landed on anyone this fight (its
	// fight's start before one), listedAt when each of its hate list came on
	// it: an unreached moment's (unreached.go)
	landedAt time.Time
	listedAt map[EntityID]time.Time
}

// templateContactGap is the longest gap between two of a templated NPC's
// turns at its enemy that its timed special's clock counts through: a
// longer one (it closed in, was rooted away or cast) is time away from its
// enemy, left out. A world tick is 100 ms.
const templateContactGap = time.Second

// castHeld is whether a templated NPC's cast holds it (petition 1593): from
// its cast's start to its busy time's end (its cast time and castBusy, the
// next chained cast's after it) it stands where it is, turning to its enemy as
// a rooted creature does (chaseLocked), and its rounds' swings are not sent
// (npcSwingLocked), its hits landing. The cast lands whatever its enemy does
// meanwhile (templateSystem: only its own death, a charm or another template
// stops it).
func (n *NPC) castHeld(now time.Time) bool {
	return n.tfight != nil && now.Before(n.tfight.busy)
}

// npcSwingLocked sends an NPC's round's swing (broadcastNPCSwingLocked) but
// while its cast holds it (castHeld): the cast's own 0x00B1 is what CLIENT
// draws of it until then. Must be called with the lock held.
func (w *World) npcSwingLocked(npc *NPC, target EntityID, delayMS uint32, fx uint32, now time.Time) {
	if npc.castHeld(now) {
		return
	}
	w.broadcastNPCSwingLocked(npc, target, delayMS, fx)
}

// fight is the NPC's fight by its template, begun if it had none.
func (n *NPC) fight() *templateFight {
	if n.tfight == nil {
		n.tfight = &templateFight{}
	}
	return n.tfight
}

// endTemplateFight forgets the NPC's fight by its template: the next one
// starts its clocks again. Its enrage's size goes back (enrage.go); the adds
// its summons called go with the fight (summon.go's addsSystem).
func (n *NPC) endTemplateFight() {
	if n.tfight != nil {
		if e := n.tfight.enraged; e != nil {
			n.Size = e.size
		}
		n.tfight = nil
	}
}

// TemplateName is the class template the NPC fights by, "" for none.
func (n *NPC) TemplateName() string {
	if n.tmpl == nil {
		return ""
	}
	return n.tmpl.Name
}

// meleeOut is a melee hit of the NPC's as its template makes it (melee_pct),
// at least 1.
func (n *NPC) meleeOut(hit int) int {
	if n.tmpl == nil || n.tmpl.MeleePct == 100 {
		return n.stoodOut(hit) // its last stand's (laststand.go)
	}
	return n.stoodOut(max(1, int(math.Round(float64(hit)*float64(n.tmpl.MeleePct)/100))))
}

// meleeTaken is a player's or a pet's melee hit on the NPC as its template
// takes it (taken_pct), at least 1.
func (n *NPC) meleeTaken(hit int) int {
	if n.tmpl == nil {
		return hit
	}
	if n.healShieldUp() {
		n.HP = min(n.MaxHP, n.HP+hit) // and 1 lands, as every hit does
		return 1
	}
	if n.tmpl.TakenPct == 100 {
		return hit
	}
	return max(1, int(math.Round(float64(hit)*float64(n.tmpl.TakenPct)/100)))
}

// healShieldUp is whether a melee hit on it heals it now: its template's heal
// shield, below its share of hit points.
func (n *NPC) healShieldUp() bool {
	return n.tmpl != nil && n.tmpl.healBelow > 0 && !n.Dead && n.HP*100 < n.MaxHP*n.tmpl.healBelow
}

// kindTakenOf is a player's melee hit of a kind of weapon as its template takes it
// (AbilityMeleeKind), at least 1.
func (n *NPC) kindTakenOf(kind Stat, hit int) int {
	if n.tmpl == nil || n.tmpl.kindTaken == nil {
		return hit
	}
	pct, ok := n.tmpl.kindTaken[kind]
	if !ok {
		return hit
	}
	return max(1, hit*pct/100)
}

// classTemplates is the world's templates by name (database.TemplateKey) and
// the NPC names (database.NPCClassKey) that fight by them; the pets'
// templates by summoning (pettemplate.go) and the summoning spells that are
// a weapon version; problems what of the tables the world could not use.
type classTemplates struct {
	byName map[string]*ClassTemplate
	use    map[string]string
	pets   map[database.PetTemplateKey]*petTemplate
	weapon map[int]bool
	// sheets is the workbook's pets by summoning spell (petstats.go)
	sheets   map[int]*petSheet
	problems []string
}

// templateForLocked is the template the NPC fights by: its name's
// npctemplate row's, if the template is there. Never a pet's nor a training
// dummy's (they fight their own ways). Must be called with the lock held.
func (w *World) templateForLocked(npc *NPC) *ClassTemplate {
	if len(w.templates.use) == 0 || npc.Owner != 0 || npc.Dummy != NotDummy {
		return nil
	}
	// its own row's first ("#n", npcrow.go), then its name's
	name := ""
	if r := npc.row(); r != 0 {
		name = w.templates.use[RowKey(r)]
	}
	if name == "" {
		name = w.templates.use[database.NPCClassKey(npc.Name)]
	}
	if name == "" {
		return nil
	}
	return w.templates.byName[name]
}

// applyTemplateLocked gives the NPC the template its name has now, or takes
// one away: its class (the template's, else its own) and its maximum hit
// points (hp_pct of its own), its hit points at the new maximum when it is
// not fighting, else in the same share, and a fresh fight by it. An NPC whose
// template has not changed is left as it is. Must be called with the lock
// held.
func (w *World) applyTemplateLocked(npc *NPC) {
	if npc.add != nil && npc.add.pct > 0 {
		return // an add of a share keeps its creature's template it was called with (summon.go)
	}
	t := w.templateForLocked(npc)
	if t == npc.tmpl {
		return
	}
	if npc.tmpl == nil {
		npc.tbase = templateBase{class: npc.Class, maxHP: npc.MaxHP}
	}
	npc.tmpl = t
	npc.endTemplateFight()
	class, maxHP := npc.tbase.class, npc.tbase.maxHP
	if t != nil {
		if t.Class >= 0 {
			class = t.Class
		}
		maxHP = max(1, int(math.Round(float64(npc.tbase.maxHP)*float64(t.HPPct)/100)))
	}
	if npc.Class != class {
		npc.Class = class
		npc.endRound()
	}
	if npc.MaxHP != maxHP {
		switch {
		case npc.Dead:
		case npc.CurrentEnemy == 0 || npc.HP >= npc.MaxHP || npc.MaxHP <= 0:
			npc.HP = maxHP
		default:
			npc.HP = max(1, int(math.Round(float64(npc.HP)*float64(maxHP)/float64(npc.MaxHP))))
		}
		npc.MaxHP = maxHP
	}
	if t == nil {
		npc.tbase = templateBase{}
	}
}

// templateKindTakes reports whether a spell is of an ability's kind (the
// file's doc). Never a summoning, a charm, an illusion, a proc or a spell for
// undead alone; a heal never a pact (its price on its caster).
func templateKindTakes(kind string, s *CastSpell) bool {
	d := &s.Def
	if d.Summons() || d.Charms() || d.FormKind != FormNone || d.undeadOnly() || d.hasProc() {
		return false
	}
	var hits, drains, heals, price, roots, powers, ticks, tickDrains bool
	for _, e := range d.Effects {
		if e.Kind == EffectRoot {
			roots = true
		}
		if e.Kind == EffectHPTick && e.Amount < 0 && !e.Self {
			ticks = true // a damage over time
			tickDrains = tickDrains || e.Drain != DrainNone
		}
		if e.Kind == EffectPower && e.Amount < 0 && !e.Self {
			powers = true // a mana tap (templatePowerFigure)
		}
		if e.Kind != EffectHP {
			continue
		}
		if e.Self {
			price = price || e.Amount < 0 // a pact's: not a heal
			continue
		}
		switch {
		case e.Amount < 0 && e.Drain == DrainCaster:
			hits, drains = true, true
		case e.Amount < 0:
			hits = true
		case e.Amount > 0 && e.Drain == DrainNone:
			heals = true
		}
	}
	area := s.Area.Centre != AreaNone
	single := s.Scope == 1 && d.Target == TargetSingle && !area
	if kind == database.AbilityBuff {
		// a buff a pet casts on itself or its owner: lasting, on one target,
		// a stat raised (the ability's stat: templateCandidates)
		return !d.Harmful && single && d.lasts() && !price && !hits
	}
	switch kind {
	case database.AbilityNuke:
		return d.Harmful && single && (hits || powers) && !drains
	case database.AbilityAoE:
		return d.Harmful && area && hits && !drains
	case database.AbilityLifetap:
		return d.Harmful && single && drains
	case database.AbilityRoot:
		return d.Harmful && roots && !hits
	case database.AbilityDoT:
		// its ticks alone are what lands, no one's take (pettemplate.go's
		// petDoTLocked): no instant hit, no drain
		return d.Harmful && single && ticks && !hits && !drains && !tickDrains
	case database.AbilityHeal:
		// a heal is at once: a lasting spell that raises hit points is a
		// buff (the cleric's Courage line), a heal over time a tick's
		return !d.Harmful && heals && !price && !area && !d.lasts()
	}
	return false
}

// templateCandidates is the spells of a class's (by level) an ability may
// cast: of its kind, its line's words in their names, their recast and cast
// time within its bounds.
func templateCandidates(a database.TemplateAbility, spells []*CastSpell) []*CastSpell {
	words := strings.Fields(strings.ToLower(a.Line))
	element, stat := abilityElement(a.Element), abilityStat(a.Stat)
	var out []*CastSpell
	for _, s := range spells {
		if !templateKindTakes(a.Kind, s) {
			continue
		}
		if element != StatNone && s.resist != element || stat != StatNone && !spellRaises(s, stat) {
			continue
		}
		if a.Stat == "ds" && !s.Def.hasShield() { // a damage shield (dshield.go)
			continue
		}
		if a.MaxRecast > 0 && s.Recast.Seconds() > a.MaxRecast || a.MaxCast > 0 && s.CastTime.Seconds() > a.MaxCast {
			continue
		}
		name, ok := strings.ToLower(s.Name), true
		for _, w := range words {
			ok = ok && strings.Contains(name, w)
		}
		if ok {
			out = append(out, s)
		}
	}
	return out
}

// templateSpellOf is a template spell's record as a templated NPC casts it:
// as a player's book entry is cast (SpellOf), a spellarea row over its area.
func templateSpellOf(ts database.TemplateSpell) *CastSpell {
	s := SpellOfRow(&ts.Pattern, 0, ts.Line, ts.Effects, ts.FX)
	if ts.HasArea {
		s.Area = s.Area.With(AreaRule{Centre: ts.Area.Centre, Radius: float32(ts.Area.Radius), Max: ts.Area.MaxTargets, Cone: float32(ts.Area.Cone)})
	}
	s.resist = templateResist(ts.Pattern.SpellName, ts.Line)
	if st, ok := workbookResist(&s.Def); ok { // the workbook's resist basis (workbook.go)
		s.resist = st
	}
	s.unresistable = strings.Contains(strings.ToLower(ts.Line), "unresistable")
	return &s
}

// LoadClassTemplates reads tables classtemplates and npctemplate into the
// world, at the start and after a GM's edit of either or of a spell's rows,
// and gives every NPC the template its name has now. Call it without the
// database held. It reports how many NPCs fight by one.
func (w *World) LoadClassTemplates(db *database.SQLiteDB) (int, error) {
	rows, bad, err := db.ClassTemplates()
	if err != nil {
		return 0, fmt.Errorf("classtemplates: %w", err)
	}
	use, err := db.NPCTemplates()
	if err != nil {
		return 0, fmt.Errorf("npctemplate: %w", err)
	}
	var problems []string
	for _, b := range bad {
		problems = append(problems, "classtemplates "+b+" (not used)")
	}
	// each class's spells, read once a class
	classSpells := map[int][]*CastSpell{}
	spellsOf := func(class int) ([]*CastSpell, error) {
		if s, ok := classSpells[class]; ok {
			return s, nil
		}
		recs, err := db.TemplateSpells(class)
		if err != nil {
			return nil, fmt.Errorf("the spells of class %d: %w", class, err)
		}
		out := make([]*CastSpell, 0, len(recs))
		for _, r := range recs {
			out = append(out, templateSpellOf(r))
		}
		classSpells[class] = out
		return out, nil
	}
	byName := make(map[string]*ClassTemplate, len(rows))
	names := make([]string, 0, len(rows))
	for name := range rows {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		row := rows[name]
		t := &ClassTemplate{ClassTemplate: row}
		classes := []int{row.Class}
		if row.Class < 0 {
			classes = classes[:0]
			for c := 0; c <= 14; c++ {
				classes = append(classes, c)
			}
		}
		for i, ra := range row.Abilities {
			a := templateAbility{TemplateAbility: ra,
				every: time.Duration(ra.Every * float64(time.Second)), first: time.Duration(ra.First * float64(time.Second))}
			from := classes
			if ra.SpellsOf != nil {
				from = []int{*ra.SpellsOf} // a pet's from another class's lines (pettemplate.go)
			}
			switch ra.Kind {
			case database.AbilityHealShield:
				t.healBelow = ra.Below
				continue
			case database.AbilityMeleeKind:
				if t.kindTaken == nil {
					t.kindTaken = map[Stat]int{}
				}
				t.kindTaken[map[string]Stat{"slash": StatSlash, "pierce": StatPierce, "blunt": StatBlunt}[ra.MeleeKind]] = ra.TakenPct
				continue
			case database.AbilityHitHeal:
				t.hitHeal += ra.Heal // hitheal.go
				continue
			case database.AbilitySelfDoT, database.AbilityRaidHeal, database.AbilityPowerZero, database.AbilitySetHP, database.AbilityUnreached:
				t.moments = append(t.moments, ra) // bossmoments.go
				continue
			case database.AbilityPrefer:
				for _, f := range ra.Foes {
					t.prefer = append(t.prefer, strings.ToLower(f)) // prefer.go
				}
				continue
			case database.AbilitySummon:
				t.summons = append(t.summons, templateSummon{TemplateAbility: ra}) // summon.go
				continue
			case database.AbilityEnrage:
				if t.enrage == nil {
					e := ra
					t.enrage = &e // enrage.go
				} else {
					t.problems = append(t.problems, fmt.Sprintf("ability %d: a second enrage (the first is its own)", i+1))
				}
				continue
			case database.AbilityLastStand:
				if t.lastStand == nil {
					l := ra
					t.lastStand = &l // laststand.go
				} else {
					t.problems = append(t.problems, fmt.Sprintf("ability %d: a second last stand (the first is its own)", i+1))
				}
				continue
			case database.AbilityWard:
				if t.ward == nil {
					wd := ra
					t.ward = &wd // ward.go
				} else {
					t.problems = append(t.problems, fmt.Sprintf("ability %d: a second ward (the first is its own)", i+1))
				}
				continue
			case database.AbilityDeathHold:
				if t.deathHold == nil {
					h := ra
					t.deathHold = &h // deathhold.go
				} else {
					t.problems = append(t.problems, fmt.Sprintf("ability %d: a second death hold (the first is its own)", i+1))
				}
				continue
			case database.AbilityVeil:
				v, buff, problem, err := veilOf(db, ra) // creatureproc.go
				if err != nil {
					return 0, fmt.Errorf("classtemplates %q: %w", name, err)
				}
				if problem != "" {
					t.problems = append(t.problems, fmt.Sprintf("ability %d: %s (its veil is its own, no buff cast)", i+1, problem))
				}
				t.veils = append(t.veils, v)
				if buff != nil {
					t.selfBuffs = append(t.selfBuffs, *buff) // selfbuff.go
				}
				continue
			case database.AbilityProc:
				p, buff, problem, err := procOf(db, ra) // creatureproc.go
				if err != nil {
					return 0, fmt.Errorf("classtemplates %q: %w", name, err)
				}
				if problem != "" {
					t.problems = append(t.problems, fmt.Sprintf("ability %d: %s (it procs nothing)", i+1, problem))
				} else {
					t.procs = append(t.procs, p)
					if buff != nil {
						t.selfBuffs = append(t.selfBuffs, *buff) // selfbuff.go
					}
				}
				continue
			case database.AbilityKillSay:
				t.killSays = append(t.killSays, killSayOf(ra)) // selfbuff.go
				continue
			}
			t.farCasts = t.farCasts || ra.OutOfReach
			if ra.Spell == "" && ra.SpellID == 0 && (ra.Kind == database.AbilityTaunt || ra.Kind == database.AbilityHeal && ra.HealPct > 0) {
				if ra.Kind == database.AbilityHeal {
					a.share = shareHealSpell(ra) // a creature's cast of it (abilitytimes.go)
				}
				t.abilities = append(t.abilities, a) // a taunt and a heal of its share cast no spell
				continue
			}
			if ra.Spell == "" && ra.SpellID == 0 && ra.Kind == database.AbilityStrip {
				a.pinned = stripSpell(ra) // strip.go: a cast of its own name, no book's spell
				t.abilities = append(t.abilities, a)
				continue
			}
			if ra.Spell == "" && ra.SpellID == 0 && ra.Damage > 0 && ra.Name != "" {
				a.pinned = fixedHitSpell(ra) // a fixed hit of its own name and look, no book's spell
				t.abilities = append(t.abilities, a)
				continue
			}
			if ra.Spell != "" || ra.SpellID != 0 {
				ts, ok, err := pinnedTemplateSpell(db, ra) // by name, or by id (petstats.go)
				if err != nil {
					return 0, fmt.Errorf("classtemplates %q: %w", name, err)
				}
				if ok {
					a.pinned = templateSpellOf(ts)
					if ra.Kind == database.AbilityDebuff && !debuffStats(a.pinned) {
						t.problems = append(t.problems, fmt.Sprintf("ability %d: %q takes no stat for any time (it casts nothing)", i+1, ts.Pattern.SpellName))
						a.pinned = nil
					}
				} else {
					t.problems = append(t.problems, fmt.Sprintf("ability %d: %s (it casts nothing)", i+1, pinWords(ra)))
				}
			} else {
				found := false
				for _, c := range from {
					if len(ra.Classes) > 0 && !slices.Contains(ra.Classes, c) {
						continue // not a class that casts it (petition 1900)
					}
					spells, err := spellsOf(c)
					if err != nil {
						return 0, err
					}
					a.byClass[c] = templateCandidates(ra, spells)
					found = found || len(a.byClass[c]) > 0
				}
				switch {
				case !found && ra.Damage > 0:
					a.pinned = fixedHitSpell(ra) // a fixed hit needs no spell: its default name and its kind's look
				case !found:
					t.problems = append(t.problems, fmt.Sprintf("ability %d: no %s spell of the class's at any level (it casts nothing)", i+1, ra.Kind))
				}
			}
			t.abilities = append(t.abilities, a)
		}
		for _, p := range t.problems {
			problems = append(problems, fmt.Sprintf("classtemplates %q %s", name, p))
		}
		byName[name] = t
	}
	keys := make([]string, 0, len(use))
	for k := range use {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		if byName[use[k]] == nil {
			problems = append(problems, fmt.Sprintf("npctemplate %q names no template %q (its NPCs fight as before)", k, use[k]))
		}
	}
	// the pets' (pettemplate.go): each row and its template, and the
	// summonings that are a weapon version
	petRows, badPets, err := db.PetTemplates()
	if err != nil {
		return 0, fmt.Errorf("pettemplate: %w", err)
	}
	weapon, err := db.PetWeaponSpells()
	if err != nil {
		return 0, fmt.Errorf("the pets' weapon versions: %w", err)
	}
	for _, b := range badPets {
		problems = append(problems, "pettemplate "+b+" (not used)")
	}
	pets := make(map[database.PetTemplateKey]*petTemplate, len(petRows))
	petKeys := make([]database.PetTemplateKey, 0, len(petRows))
	for k := range petRows {
		petKeys = append(petKeys, k)
	}
	sort.Slice(petKeys, func(i, j int) bool {
		if petKeys[i].Spell != petKeys[j].Spell {
			return petKeys[i].Spell < petKeys[j].Spell
		}
		return !petKeys[i].Weapon && petKeys[j].Weapon
	})
	for _, k := range petKeys {
		row := petRows[k]
		pt := &petTemplate{PetTemplate: row, tmpl: byName[row.Template]}
		if pt.tmpl == nil {
			problems = append(problems, fmt.Sprintf("pettemplate %q (weapon %v) names no template %q (its pets fight as before)",
				k.Spell, k.Weapon, row.Template))
		}
		pets[k] = pt
	}

	// the workbook's pets (petstats.go)
	sheets, sheetProblems, err := loadPetSheets(db)
	if err != nil {
		return 0, err
	}
	problems = append(problems, sheetProblems...)

	w.mu.Lock()
	defer w.mu.Unlock()
	w.templates = classTemplates{byName: byName, use: use, pets: pets, weapon: weapon, sheets: sheets, problems: problems}
	n := 0
	for _, npc := range w.NPCs {
		w.applyTemplateLocked(npc)
		if npc.tmpl != nil {
			n++
		}
	}
	return n, nil
}

// ClassTemplateSummary is the templates for the server's log: how many NPCs
// fight by one, the templates there are, and what of them cannot be used.
func (w *World) ClassTemplateSummary() []string {
	w.mu.RLock()
	defer w.mu.RUnlock()
	var names []string
	for name := range w.templates.byName {
		names = append(names, name)
	}
	sort.Strings(names)
	n, named := 0, map[string]bool{}
	for _, npc := range w.NPCs {
		if npc.tmpl != nil {
			n++
			named[database.NPCClassKey(npc.Name)] = true
		}
	}
	first := fmt.Sprintf("no NPC fights by a class template (npctemplate gives none); %d ready: %s", len(names), strings.Join(names, ", "))
	if n > 0 {
		first = fmt.Sprintf("%d NPCs of %d names fight by a class template (npctemplate); %d templates: %s", n, len(named), len(names), strings.Join(names, ", "))
	}
	out := []string{first}
	if len(w.templates.sheets) > 0 {
		out = append(out, w.petSheetSummaryLocked()) // the workbook's pets (petstats.go)
	}
	return append(out, w.templates.problems...)
}

// ClassTemplateProblems is what of the tables the world could not use.
func (w *World) ClassTemplateProblems() []string {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return append([]string(nil), w.templates.problems...)
}

// NPCTemplateCount is how many NPCs fight by the template (by its name), and
// how many by any.
func (w *World) NPCTemplateCount(template string) (of, all int) {
	key := database.TemplateKey(template)
	w.mu.RLock()
	defer w.mu.RUnlock()
	for _, npc := range w.NPCs {
		if npc.tmpl != nil {
			all++
			if npc.tmpl.Name == key {
				of++
			}
		}
	}
	return of, all
}

// templateTurnLocked is a templated NPC's turn in its fight with target (a
// player, or a player's pet) in its reach: the chained ability it owes once its
// cast's time is over, else the first of its abilities that is due. It reports
// whether the NPC is casting; its melee goes on beside it (castHeld). Must be
// called with the lock held.
func (w *World) templateTurnLocked(npc *NPC, target EntityID, now time.Time) bool {
	t := npc.tmpl
	if t == nil || len(t.abilities) == 0 {
		return false
	}
	f := npc.fight()
	f.begin(t, now)
	if now.Before(f.busy) {
		return true
	}
	if f.chain > 0 {
		i := f.chain - 1
		f.chain = 0
		if i < len(t.abilities) && w.templateCastLocked(npc, i, target, now) {
			return true
		}
	}
	for i := range t.abilities {
		a := &t.abilities[i]
		if a.Chain || a.OutOfReach || now.Before(f.next[i]) || npc.Level < a.From || petOnly(a) || npc.coolingDown(a, now) {
			continue // a cooldown across fights (abilitytimes.go): it waits, due; one cast from afar alone: templateFarTurnLocked
		}
		if !w.templateWantsLocked(npc, a, target, now) {
			continue // it waits, due
		}
		f.next[i] = now.Add(a.every)
		if c := a.ChanceOr(); c < 100 && templateIntn(100) >= c {
			continue
		}
		if w.templateCastLocked(npc, i, target, now) {
			return true
		}
	}
	return false
}

// begin sets the fight's clocks at its first turn: each ability first its
// first into the fight.
func (f *templateFight) begin(t *ClassTemplate, now time.Time) {
	if f.on {
		return
	}
	f.on = true
	for i := range t.abilities {
		f.next[i] = now.Add(t.abilities[i].first)
	}
}

// templateWantsLocked is whether a due ability is wanted at target now: a heal
// below its share, a debuff at a player, a damage over time not already up on
// what it fights (creaturedot.go), and any other cast with a Below only once its
// creature's hit points are below that share; anything else yes. One not
// wanted waits, due. Must be called with the lock held.
func (w *World) templateWantsLocked(npc *NPC, a *templateAbility, target EntityID, now time.Time) bool {
	if a.Kind != database.AbilityHeal && a.Below > 0 && npc.HP*100 >= npc.MaxHP*a.Below {
		// a cast held for its creature's hit points below its share (the owner,
		// 2026-11-13: Lord Koreshaa's "when he gets down to 1/3 hp 3500 ar nuke
		// he fires once"): until then it waits, due
		return false
	}
	switch a.Kind {
	case database.AbilityHeal:
		return npc.HP*100 < npc.MaxHP*a.BelowOr() // whole enough: it waits
	case database.AbilityDebuff:
		return w.Players[target] != nil // a pet: it waits
	case database.AbilityDoT:
		return w.creatureDoTWantedLocked(npc, a, target, now) // creaturedot.go
	}
	return true
}

// templateFarTurnLocked is a templated NPC's turn while what it fights (target:
// a player, a player's pet, or the NPC of a scripted fight) stands beyond its
// melee reach, rooted, snared or kiting (the GM Jibrish's petition 1442: Metal
// Moogie V2's "ranged attack they can use if the target is out of range ... ONLY
// if the target is out of normal melee attack range, due to root or snare or
// kiting"): the first of its out-of-reach abilities (OutOfReach) due and wanted,
// with target within its range (RangeOr, from centre to centre) and in its sight
// (sight.go); the rest, and one whose target is too far or hidden, wait, due. Its
// melee and its other abilities are its turn's in reach (templateTurnLocked),
// where these wait. It reports whether a cast began. Must be called with the
// lock held.
func (w *World) templateFarTurnLocked(npc *NPC, target EntityID, now time.Time) bool {
	t := npc.tmpl
	if t == nil || !t.farCasts || npc.Dead {
		return false
	}
	f := npc.fight()
	f.begin(t, now)
	if now.Before(f.busy) {
		return false
	}
	x, y, z, world, ok := w.fightSpotLocked(target)
	if !ok || world != npc.World {
		return false
	}
	for i := range t.abilities {
		a := &t.abilities[i]
		if !a.OutOfReach || now.Before(f.next[i]) || npc.Level < a.From || npc.coolingDown(a, now) {
			continue
		}
		if dist3(npc.X, npc.Y, npc.Z, x, y, z) > float32(a.RangeOr()) || !w.inSightLocked(npc.World, npc.X, npc.Y, npc.Z, x, y, z) {
			continue // too far, or out of its sight: it waits, due
		}
		if !w.templateWantsLocked(npc, a, target, now) {
			continue
		}
		f.next[i] = now.Add(a.every)
		if c := a.ChanceOr(); c < 100 && templateIntn(100) >= c {
			continue
		}
		if w.templateCastLocked(npc, i, target, now) {
			return true
		}
	}
	return false
}

// fightSpotLocked is where what an NPC fights stands: a player, or an NPC (a pet,
// a scripted fight's foe); ok false for one gone. Must be called with the lock
// held.
func (w *World) fightSpotLocked(id EntityID) (x, y, z float32, world uint8, ok bool) {
	if p := w.Players[id]; p != nil {
		return p.X, p.Y, p.Z, p.World, true
	}
	if n := w.NPCs[id]; n != nil && !n.Dead {
		return n.X, n.Y, n.Z, n.World, true
	}
	return 0, 0, 0, 0, false
}

// npcCast is a templated NPC's cast on its way to landing (templateSystem).
type npcCast struct {
	npc    *NPC
	tmpl   *ClassTemplate // the template it was cast by: another since drops it
	spell  *CastSpell
	pct    int
	heal   bool
	debuff bool // what lasts of its spell, no hit (templateDebuffLocked)
	noCure bool // a debuff's: no cure takes it off
	strip  bool // every beneficial effect off its target, nothing of its spell (strip.go)
	dot    bool // its spell's damage over time on what it fights, kept up (creaturedot.go)
	// ability is the index of the ability cast in tmpl's, for what its landing
	// does past itself (abilitytimes.go); healPct a heal of the NPC's share
	ability int
	healPct int
	// fixed is a fixed figure's hit in place of its spell's figures (Damage, at
	// an add's share), cut by fixedResist alone; tap: all of what lands of it
	// back on the NPC (a lifetap's)
	fixed       int
	fixedResist Stat
	tap         bool
	target      EntityID
	others      []EntityID
	lands       time.Time
}

// templateCastLocked begins the NPC's cast of its ability i at target (a
// heal on itself), as a player's begins (cast.go): its 0x00B1 to everyone who
// sees the NPC, its cast time and its visual, an area spell's others named;
// it lands castLand after its cast time. It reports whether a cast began: not
// when the ability has no spell at the NPC's level, nor when the target may
// not be hit. Must be called with the lock held.
func (w *World) templateCastLocked(npc *NPC, i int, target EntityID, now time.Time) bool {
	t := npc.tmpl
	a := &t.abilities[i]
	src := npc.figures() // an add's are its creature's level and class (summon.go)
	if len(a.Classes) > 0 && !slices.Contains(a.Classes, src.Class) {
		return false // a class that does not cast it (petition 1900)
	}
	spell := a.spellFor(src.Class, src.Level)
	if spell == nil {
		spell = a.share // a heal of its share that pins no spell (abilitytimes.go)
	}
	if spell == nil || npc.coolingDown(a, now) {
		return false
	}
	ct := a.castTime(spell)
	c := &npcCast{npc: npc, tmpl: t, spell: spell, pct: npc.shared(a.DamageOr()), target: target,
		debuff: a.Kind == database.AbilityDebuff, noCure: a.NoCure, strip: a.Kind == database.AbilityStrip,
		dot: a.Kind == database.AbilityDoT, ability: i, lands: now.Add(ct + castLand)}
	if a.Damage > 0 {
		c.fixed, c.fixedResist, c.tap = npc.stoodOut(npc.shared(a.Damage)), shieldResistStat(a.Resist), a.Kind == database.AbilityLifetap
	}
	if a.Kind == database.AbilityHeal {
		c.heal, c.target, c.healPct = true, npc.ServerID, a.HealPct
	} else {
		if !w.templateMayHitLocked(npc, target) {
			return false
		}
		// a strip is its target's alone, whatever its spell's area; and round an
		// NPC a trigger set it on, an area spell takes it alone: the players and
		// pets near (a raid fighting beside it) are no one's foes
		if spell.Area.Centre != AreaNone && !c.strip && w.castFoeLocked(npc, target) == nil {
			c.others = w.templateAreaLocked(npc, spell, target)
		}
	}
	w.npcCasts = append(w.npcCasts, c)
	npc.startCooldown(a, now) // from its cast's start, across fights (abilitytimes.go)
	f := npc.fight()
	f.busy = now.Add(ct + castBusy)
	if j := i + 1; j < len(t.abilities) && t.abilities[j].Chain {
		f.chain = j + 1
	}
	w.broadcastNPCSwingLocked(npc, c.target, uint32(ct/time.Millisecond), spell.visual(), c.others...)
	figures := fmt.Sprintf("%d%% of its figures", c.pct)
	if c.fixed > 0 {
		figures = fmt.Sprintf("a fixed %d", c.fixed)
	}
	log.Printf("Template: %s (%s, level %d) begins %q (spell %d) on %s: %v cast, %s, %d others",
		npc.Name, t.Name, npc.Level, spell.Name, spell.SpellID, w.entityNameLocked(c.target), ct, figures, len(c.others))
	return true
}

// templateMayHitLocked reports whether the NPC's harmful spell may be cast at
// target: a living player in its world (not across the water's edge from it,
// water.go), or a living pet of a player's there, or the NPC of a scripted
// fight (scriptedFoes: the NPC a trigger set it on with casts, or one set so on
// it).
func (w *World) templateMayHitLocked(npc *NPC, target EntityID) bool {
	if p := w.Players[target]; p != nil {
		return !p.IsDead() && !p.respawnPending() && w.npcSharesLocked(npc, p) && !w.apartLocked(p, npc)
	}
	n := w.NPCs[target]
	return n != nil && (n.Owner != 0 || scriptedFoes(npc, n)) && !n.Dead && w.npcsShareLocked(n, npc) &&
		(w.env == nil || w.npcInWaterLocked(n) == w.npcInWaterLocked(npc))
}

// castFoeLocked is the NPC a templated NPC's cast or special at target lands on
// when target is the NPC of a scripted fight (scriptedFoes: no pet, alive), nil
// otherwise. Must be called with the lock held.
func (w *World) castFoeLocked(npc *NPC, target EntityID) *NPC {
	if target == 0 || target == npc.ServerID {
		return nil
	}
	foe := w.NPCs[target]
	if foe == nil || foe.Owner != 0 || foe.Dead || !w.npcsShareLocked(foe, npc) || !scriptedFoes(npc, foe) {
		return nil
	}
	return foe
}

// scriptedFoes reports a scripted fight between two NPCs, which each fights by
// its whole class template (its casts, specials, kicks and procs) where any
// other fight between two NPCs is their melee alone (a town's guard and a
// creature): one a trigger's sic set on the other with casts (castsAt,
// SetNPCOn), either way round, so the NPC it was set on fights it back so too
// (the GM Jibrish's petition 1437: Mr. Moogie answered Tunare and her pets by
// his melee alone, his Lay Hands, his strip and his crit never fired); and an
// add a summon called (summon.go) fights as its creature does, either way round.
func scriptedFoes(npc, foe *NPC) bool {
	return scriptedPair(npc, foe) || scriptedPair(npc.summoner(), foe) || scriptedPair(npc, foe.summoner())
}

// scriptedPair is scriptedFoes' test of two NPCs as they are.
func scriptedPair(a, b *NPC) bool {
	return a != nil && b != nil && a != b && (a.castsAt == b.ServerID || b.castsAt == a.ServerID)
}

// castFoeHitLocked lands n of a templated NPC's cast or special on the NPC a
// trigger set it on (castFoeLocked): its hit points, its hate for the caster,
// and its death, as an NPC's melee hit on it does (npcHitNPCLocked). Must be
// called with the lock held.
func (w *World) castFoeHitLocked(npc, foe *NPC, n int, what string, now time.Time) {
	n = w.npcTakesLocked(foe, n, now) // nothing while held at 1 (deathhold.go)
	foe.HP = max(foe.HP-n, 0)
	npc.LastCombat = now
	w.addThreatLocked(foe, npc.ServerID, n)
	log.Printf("Template: %s's %s hits %s for %d (HP %d/%d)", npc.Name, what, foe.Name, n, foe.HP, foe.MaxHP)
	if foe.HP > 0 {
		w.hitLandedLocked(foe, n, nil, npc.ServerID) // its heal at every hit that lands, its ward's answer (hitheal.go, ward.go)
		return
	}
	foe.Effects = nil
	w.killNPC(foe)
	w.dropAggroLocked(foe.ServerID)
	log.Printf("Combat: %s killed by %s", foe.Name, npc.Name)
	w.killSaidLocked(npc, foe.Name) // its line at the kill (selfbuff.go)
}

// templateAreaCentreLocked is where a templated NPC's area spell lies now:
// round its target, or itself for a caster's area. Must be called with the
// lock held.
func (w *World) templateAreaCentreLocked(npc *NPC, spell *CastSpell, target EntityID) (x, y, z float32) {
	if spell.Area.Centre == AreaOnTarget {
		if p := w.Players[target]; p != nil {
			return p.X, p.Y, p.Z
		}
		if pet := w.NPCs[target]; pet != nil {
			return pet.X, pet.Y, pet.Z
		}
	}
	return npc.X, npc.Y, npc.Z
}

// templateConeLocked is a templated NPC's cone (area.go's areaCone), aimed at
// its target; nil for an area not a cone. Must be called with the lock held.
func (w *World) templateConeLocked(npc *NPC, spell *CastSpell, target EntityID) *areaCone {
	if spell.Area.Centre != AreaCone {
		return nil
	}
	var tx, tz float32
	aim := false
	if p := w.Players[target]; p != nil {
		tx, tz, aim = p.X, p.Z, true
	} else if o := w.NPCs[target]; o != nil && o != npc {
		tx, tz, aim = o.X, o.Z, true
	}
	return spell.Area.coneOf(npc.X, npc.Y, npc.Z, npc.Facing, aim, tx, tz)
}

// templateAreaPlaceLocked is where a templated NPC's area spell stands now
// (sight.go). Must be called with the lock held.
func (w *World) templateAreaPlaceLocked(npc *NPC, spell *CastSpell, target EntityID) areaPlace {
	x, y, z := w.templateAreaCentreLocked(npc, spell, target)
	return areaPlace{world: npc.World, caster: [3]float32{npc.X, npc.Y, npc.Z}, centre: [3]float32{x, y, z}, onOther: spell.Area.Centre == AreaOnTarget}
}

// templateSparedLocked reports one a templated NPC's area spell does not reach
// as it lands (sight.go), noting it either way: its own target among them, whose
// line from the caster counts though it stands at the centre. A spell of no area
// spares no one, and one gone is the landing's to pass over. Must be called with
// the lock held.
func (w *World) templateSparedLocked(c *npcCast, place areaPlace, id EntityID, seen *areaLog) bool {
	if c.spell.Area.Centre == AreaNone {
		return false
	}
	var name string
	var x, y, z float32
	if p := w.Players[id]; p != nil {
		name, x, y, z = p.Name, p.X, p.Y, p.Z
	} else if pet := w.NPCs[id]; pet != nil {
		name, x, y, z = pet.Name, pet.X, pet.Y, pet.Z
	} else {
		return false
	}
	reached := w.reachesLocked(place, x, y, z)
	seen.note(reached, name, x, y, z)
	return !reached
}

// templateAreaLocked is a templated NPC's area spell's others as its cast
// begins, as a player's are chosen (area.go): round its target (or itself,
// a caster's area), within its radius and reached by it (sight.go), the
// nearest first, its cap with the target counted; the players a creature may
// notice (aggro.go: not hidden, not a GM who turned aggro off, not loading a
// world; one in safe mode only when it is part of npc's fight), not across
// the water's edge from it, and players' pets.
func (w *World) templateAreaLocked(npc *NPC, spell *CastSpell, target EntityID) []EntityID {
	if w.SpatialIndex == nil {
		return nil
	}
	a := &spell.Area
	r := a.radiusFor(true)
	cx, cy, cz := w.templateAreaCentreLocked(npc, spell, target)
	place := w.templateAreaPlaceLocked(npc, spell, target)
	cone := w.templateConeLocked(npc, spell, target) // area.go: a cone in front of it, at its target
	var seen areaLog
	defer func() { seen.write("begins", npc.Name, spell.Name, place) }()
	var near []areaNear
	for _, id := range w.SpatialIndex.QueryRadius(cx, cz, queryRadius(cone, r)) {
		if id == target {
			continue
		}
		if p := w.Players[id]; p != nil {
			// a player in safe mode (/testinvis, safemode.go) only once it is part of this fight
			if w.npcSharesLocked(npc, p) && w.noticeableByLocked(npc, p) && (!w.safeLocked(p) || inFightLocked(npc, p)) && !w.apartLocked(p, npc) {
				if in, d := inArea(cone, cx, cy, cz, r, p.X, p.Y, p.Z, 0); in {
					reached := w.reachesLocked(place, p.X, p.Y, p.Z)
					seen.note(reached, p.Name, p.X, p.Y, p.Z)
					if reached {
						near = append(near, areaNear{id, d})
					}
				}
			}
			continue
		}
		if pet := w.NPCs[id]; pet != nil && pet.Owner != 0 && !pet.Dead && w.npcsShareLocked(pet, npc) {
			if in, d := inArea(cone, cx, cy, cz, r, pet.X, pet.Y, pet.Z, npcReachExtra(pet)); in {
				reached := w.reachesLocked(place, pet.X, pet.Y, pet.Z)
				seen.note(reached, pet.Name, pet.X, pet.Y, pet.Z)
				if reached {
					near = append(near, areaNear{id, d})
				}
			}
		}
	}
	sort.SliceStable(near, func(i, j int) bool { return near[i].dist < near[j].dist })
	if room := max(a.most()-1, 0); len(near) > room {
		near = near[:room]
	}
	out := make([]EntityID, 0, len(near))
	for _, n := range near {
		out = append(out, n.id)
	}
	return out
}

// templateSystem lands every templated NPC's cast whose time has come: on a
// caster still there, alive, not since a pet, by the template it was cast
// by. Every tick; must be called with the lock held.
func (w *World) templateSystem(now time.Time) {
	if len(w.npcCasts) == 0 {
		return
	}
	var due []*npcCast
	keep := w.npcCasts[:0]
	for _, c := range w.npcCasts {
		if now.Before(c.lands) {
			keep = append(keep, c)
		} else {
			due = append(due, c)
		}
	}
	for i := len(keep); i < len(w.npcCasts); i++ {
		w.npcCasts[i] = nil
	}
	w.npcCasts = keep
	for _, c := range due {
		npc := c.npc
		if npc.Dead || npc.Owner != 0 || w.NPCs[npc.ServerID] != npc || npc.tmpl != c.tmpl {
			continue
		}
		w.templateLandLocked(c, now)
		w.templateLandedLocked(c) // its line in /say, the proc it gives (abilitytimes.go)
	}
}

// templateLandLocked lands one templated NPC's cast whose time has come, by its
// kind: a heal on itself, a debuff, a strip, else its hits on its target and its
// others. Must be called with the lock held.
func (w *World) templateLandLocked(c *npcCast, now time.Time) {
	npc := c.npc
	switch {
	case c.heal:
		w.templateHealLocked(c)
		return
	case c.debuff:
		w.templateDebuffLocked(c, now)
		return
	case c.strip:
		w.templateStripLocked(c, now) // strip.go
		return
	case c.dot:
		w.templateDoTLocked(c, now) // creaturedot.go
		return
	}
	place := w.templateAreaPlaceLocked(npc, c.spell, c.target)
	var seen areaLog
	if !w.templateSparedLocked(c, place, c.target, &seen) {
		w.templateHitLocked(c, c.target, c.spell.Def.Effects, now)
	}
	share := c.spell.Def.Effects
	if e := c.spell.Area.Effects; e != nil {
		share = e
	}
	for _, id := range c.others {
		if npc.Dead {
			break
		}
		if !w.templateSparedLocked(c, place, id, &seen) {
			w.templateHitLocked(c, id, share, now)
		}
	}
	seen.write("lands", npc.Name, c.spell.Name, place)
}

// templateFigure is what effects' hits come to from a templated NPC at its
// ability's percent, and the drained share of it: each hit rolled in its
// range, no stat of the NPC's added (it has none), within its cap.
func templateFigure(effects []SpellEffect, pct int) (hit, drained int) {
	for _, e := range effects {
		if e.Kind != EffectHP || e.Amount >= 0 || e.Self {
			continue
		}
		n := int(math.Round(float64(-capAmount(rollAmount(e), e.Cap)) * float64(pct) / 100))
		hit += n
		if e.Drain != DrainNone {
			drained += n
		}
	}
	return hit, drained
}

// powerDrainCapShare is the share of a mana tap a resist at the cap takes:
// the GM Jibrish's petition 1295, of the pariahs of Xrag's: "It should take
// 1k mana as a baseline, but resist down to 200 mana with max AR". A spell's
// damage keeps ResistCapShare (resist.go).
const powerDrainCapShare = 0.8

// templatePowerFigure is the power a templated NPC's spell takes from one it
// hits, its abilities' share of its figures (pct), before a resist: its
// effects that lower the target's power now (a mana tap).
func templatePowerFigure(effects []SpellEffect, pct int) int {
	n := 0
	for _, e := range effects {
		if e.Kind != EffectPower || e.Amount >= 0 || e.Self {
			continue
		}
		n += int(math.Round(float64(-capAmount(rollAmount(e), e.Cap)) * float64(pct) / 100))
	}
	return n
}

// templateHitLocked lands a templated NPC's harmful spell on one it hits (its
// target or one of its others): a player takes the figure less its resist's
// share (resist.go), told it in a 0x00DB from the NPC and a line, and dies of
// it at nothing; a pet takes it whole. The drained share of what was taken
// heals the NPC. Must be called with the lock held.
func (w *World) templateHitLocked(c *npcCast, id EntityID, effects []SpellEffect, now time.Time) {
	npc := c.npc
	hit, drained := templateFigure(effects, c.pct)
	power := templatePowerFigure(effects, c.pct)
	if c.fixed > 0 {
		// a fixed figure in place of its spell's (Damage): a tap's all of it back
		hit, drained, power = c.fixed, 0, 0
		if c.tap {
			drained = c.fixed
		}
	}
	if hit <= 0 && power <= 0 {
		return
	}
	var sess *session.Session
	if p := w.Players[id]; p != nil {
		if p.IsDead() || p.respawnPending() || !w.npcSharesLocked(npc, p) {
			return
		}
		resist := c.spell.resist
		if resist == StatNone {
			resist = StatAR
		}
		share, how := c.tmpl.rollResist(p.StatNow(resist))
		if c.spell.unresistable {
			share, how = 0, ResistFailed
		}
		if c.fixed > 0 {
			// a fixed figure lands as it is but for its own resist's share
			share, how = 0, ResistFailed
			if c.fixedResist != StatNone {
				share, how = c.tmpl.rollResist(p.StatNow(c.fixedResist))
			}
		}
		p.StatsDirty = true
		p.LastCombatTime = now
		w.breakInvisLocked(p, "a hit") // invis.go: every invisibility, whatever gave it
		sess = w.findSessionByEntityIDLocked(p.ServerID)
		if power > 0 {
			// a mana tap: its resist takes up to powerDrainCapShare at the cap
			ps := min(share*powerDrainCapShare/ResistCapShare, 1)
			lost := min(max(int(math.Round(float64(power)*(1-ps))), 1), p.Power)
			p.Power -= lost
			if sess != nil {
				w.sendChatToSession(sess, fmt.Sprintf("%s's %s drains %d of your power (%s).", npc.Name, c.spell.Name, lost, resistWords(ps, how)))
			}
			log.Printf("Template: %s's %q drains %s's power by %d of %d (resist %.0f%%; power %d/%d)", npc.Name, c.spell.Name, p.Name, lost, power,
				ps*100, p.Power, p.MaxPower)
		}
		if hit > 0 {
			taken := max(int(math.Round(float64(hit)*(1-share))), 1)
			drained = int(math.Round(float64(drained) * (1 - share)))
			p.HP = max(p.HP-taken, 0)
			if sess != nil {
				if nid := w.objectIDForEntityLocked(sess, npc.ServerID); nid != 0 {
					w.sendDamageIndication(sess, nid, -taken, sess.PlayerObjectID(), 0) // the creature's on the player: untagged
				}
				line := fmt.Sprintf("%s's %s hits you for %d (%s).", npc.Name, c.spell.Name, taken, resistWords(share, how))
				if c.fixed > 0 && c.fixedResist == StatNone {
					line = fmt.Sprintf("%s's %s hits you for %d.", npc.Name, c.spell.Name, taken) // no resist of it
				}
				w.sendChatToSession(sess, line)
			}
			log.Printf("Template: %s's %q hits %s for %d of %d (resist %.0f%%; HP %d/%d)", npc.Name, c.spell.Name, p.Name, taken, hit,
				share*100, p.HP, p.MaxHPNow())
			if p.HP <= 0 {
				w.playerDiedLocked(sess, p, now)
				w.killedPlayerLocked(npc, p) // its line at the kill (selfbuff.go)
			}
		}
	} else if foe := w.castFoeLocked(npc, id); foe != nil {
		// the NPC a trigger set it on with casts: its hit less the foe's template's
		// resist of the spell's element (creatureResists, as a player's spell on it)
		if hit <= 0 {
			return
		}
		resist := c.spell.resist
		if c.fixed > 0 {
			resist = c.fixedResist // a fixed figure's own, none for none
		}
		taken := -creatureResists(resist, foe, -hit, now)
		if taken < hit {
			drained = int(math.Round(float64(drained) * float64(taken) / float64(hit)))
		}
		w.castFoeHitLocked(npc, foe, taken, fmt.Sprintf("%q", c.spell.Name), now)
	} else if pet := w.NPCs[id]; pet != nil && pet.Owner != 0 && !pet.Dead {
		share := pet.petResistShare(c.spell.resist) // a workbook pet's resists (petstats.go); any other takes it whole
		if c.fixed > 0 {
			share = 0
			if c.fixedResist != StatNone {
				share = pet.petResistShare(c.fixedResist)
			}
		}
		if power > 0 {
			lost := min(int(math.Round(float64(power)*(1-min(share*powerDrainCapShare/ResistCapShare, 1)))), pet.Power)
			pet.Power -= max(lost, 0)
			log.Printf("Template: %s's %q drains the pet %q's power by %d (power %d/%d)", npc.Name, c.spell.Name, pet.Name, lost, pet.Power, pet.MaxPower)
		}
		if hit > 0 {
			if share > 0 {
				hit = max(int(math.Round(float64(hit)*(1-share))), 1)
				drained = int(math.Round(float64(drained) * (1 - share)))
			}
			pet.HP = max(pet.HP-hit, 0)
			pet.LastCombatTime = now
			log.Printf("Template: %s's %q hits the pet %q for %d (HP %d/%d)", npc.Name, c.spell.Name, pet.Name, hit, pet.HP, pet.MaxHP)
			if pet.HP == 0 {
				w.killNPC(pet)
				w.dropAggroLocked(pet.ServerID)
			}
		}
		if owner := w.Players[pet.Owner]; owner != nil {
			sess = w.findSessionByEntityIDLocked(owner.ServerID)
		}
	} else {
		return
	}
	if drained > 0 && npc.HP < npc.MaxHP {
		npc.HP = min(npc.HP+drained, npc.MaxHP)
		w.tellNPCHealLocked(sess, npc, drained)
	}
}

// debuffStats reports whether a spell can be a debuff's: it lasts and takes a
// stat, which is all a debuff puts on.
func debuffStats(s *CastSpell) bool {
	if s.Def.Duration <= 0 {
		return false
	}
	for _, e := range s.Def.Effects {
		if e.Kind == EffectStat && !e.Set && e.Amount < 0 {
			return true
		}
		// a damage over time (the GM Jibrish's petition 1259: the Ebony
		// Recluse's "poison dot that lasts 30 seconds ... that ticks for 300")
		if e.Kind == EffectHPTick && e.Amount < 0 {
			return true
		}
	}
	return false
}

// templateTickFigure is a damage tick's figure from a templated NPC at its
// ability's percent: the tick rolled in its range, within its cap.
func templateTickFigure(e SpellEffect, pct int) int {
	return int(math.Round(float64(-capAmount(rollAmount(e), e.Cap)) * float64(pct) / 100))
}

// templateDebuffLocked lands a templated NPC's debuff on the player it fights:
// the stats its spell takes, for the spell's time; a recast starts it again. Must
// be called with the lock held.
func (w *World) templateDebuffLocked(c *npcCast, now time.Time) {
	npc := c.npc
	p := w.Players[c.target]
	if p == nil || p.IsDead() || p.respawnPending() || !w.npcSharesLocked(npc, p) {
		return
	}
	d := &c.spell.Def
	dur := d.Duration
	if dur <= 0 && c.dot {
		dur = tickDuration // a damage over time's tick line with no time (cast.go)
	}
	ae := &ActiveEffect{SpellID: c.spell.SpellID, Name: c.spell.Name, Icon: c.spell.Icon, Caster: npc.ServerID, Until: now.Add(dur)}
	ae.Rules = rulesOf(d)
	ae.Rules.NoCure = c.noCure
	ae.Rules.Harm = true // a creature's debuff is harm whatever its record reads: a strip leaves it on (strip.go)
	tickResist := StatNone
	for _, e := range d.Effects {
		if e.Kind == EffectStat && !e.Set && !c.dot { // a damage over time is its ticks alone (creaturedot.go)
			ae.Mods.add(e.Stat, e.Amount)
		}
		// a damage tick, at the ability's percent: each tick hurts as the
		// creature's spell does, at its element's resist (npcTickLocked)
		if e.Kind == EffectHPTick && e.Amount < 0 {
			ae.HPTick -= templateTickFigure(e, c.pct)
			if e.Resist >= StatFR && e.Resist <= StatAR {
				tickResist = e.Resist
			}
		}
	}
	if ae.HPTick < 0 {
		// its element: the tick's own resist, else the spell's, else its name's
		// ("venom": poison), else arcane (a workbook record's resist basis looks
		// at its instant damage alone, which a damage over time has none of)
		ae.NextTick, ae.npcFoe, ae.npcResist = now, npc.ServerID, tickResist
		if ae.npcResist == StatNone {
			ae.npcResist = c.spell.resist
		}
		if ae.npcResist == StatNone {
			ae.npcResist = templateResist(c.spell.Name, "")
		}
		if ae.npcResist == StatNone {
			ae.npcResist = StatAR
		}
		if c.spell.unresistable {
			ae.npcResist = StatNone
		}
	}
	p.LastCombatTime = now
	w.breakInvisLocked(p, "a hit") // invis.go: every invisibility, whatever gave it
	w.addEffectLocked(p, ae, now)
	if sess := w.findSessionByEntityIDLocked(p.ServerID); sess != nil {
		w.sendChatToSession(sess, fmt.Sprintf("%s's %s takes hold of you.", npc.Name, c.spell.Name))
	}
	log.Printf("Template: %s's %q lands on %s for %v", npc.Name, c.spell.Name, p.Name, dur)
}

// npcTickLocked is a tick of a creature's damage over time on t (runTickLocked):
// the tick less t's resist share to its element, rolled on the creature's
// template as its spell's hit is (templateHitLocked), at least 1, told in a
// 0x00DB from the creature and a line, and a death at nothing. A tick whose
// creature is dead or gone does nothing (what is left of it runs out). Must be
// called with the lock held.
func (w *World) npcTickLocked(t *Player, ae *ActiveEffect, now time.Time) {
	if ae.npcFoe == 0 || ae.HPTick >= 0 || t.IsDead() || t.respawnPending() {
		return
	}
	npc := w.NPCs[ae.npcFoe]
	if npc == nil || npc.Dead || !w.npcSharesLocked(npc, t) {
		return
	}
	share, how := 0.0, ResistFailed
	if ae.npcResist != StatNone && npc.tmpl != nil {
		share, how = npc.tmpl.rollResist(t.StatNow(ae.npcResist))
		if ae.npcAtCap > 0 {
			share *= ae.npcAtCap / ResistCapShare // its own share at the cap (bossmoments.go)
		}
	}
	taken := max(int(math.Round(float64(-ae.HPTick)*(1-share))), 1)
	t.HP = max(t.HP-taken, 0)
	t.StatsDirty = true
	t.LastCombatTime = now
	w.breakInvisLocked(t, "a hit") // invis.go: every invisibility, whatever gave it
	sess := w.findSessionByEntityIDLocked(t.ServerID)
	if sess != nil {
		if nid := w.objectIDForEntityLocked(sess, npc.ServerID); nid != 0 {
			w.sendDamageIndication(sess, nid, -taken, sess.PlayerObjectID(), 0) // the creature's on the player: untagged
		}
		w.sendChatToSession(sess, fmt.Sprintf("%s's %s hits you for %d (%s).", npc.Name, ae.Name, taken, resistWords(share, how)))
	}
	log.Printf("Template: %s's %q ticks on %s for %d of %d (resist %.0f%%; HP %d/%d)", npc.Name, ae.Name, t.Name, taken, -ae.HPTick,
		share*100, t.HP, t.MaxHPNow())
	if t.HP <= 0 {
		w.playerDiedLocked(sess, t, now)
		w.killedPlayerLocked(npc, t) // its line at the kill (selfbuff.go)
	}
}

// templateHealLocked lands a templated NPC's heal on itself: its figure at
// the ability's percent, or its share of its most hit points (heal_pct), when
// it is short of its maximum.
func (w *World) templateHealLocked(c *npcCast) {
	npc := c.npc
	n := 0
	for _, e := range c.spell.Def.Effects {
		if e.Kind == EffectHP && e.Amount > 0 && !e.Self && e.Drain == DrainNone {
			n += int(math.Round(float64(capAmount(rollAmount(e), e.Cap)) * float64(c.pct) / 100))
		}
	}
	if c.healPct > 0 {
		// a heal of its share in place of its spell's figure, what it lacks of
		// it (abilitytimes.go: Mr. Moogie's Lay Hands, all of it)
		n = min(petShareAmount(npc.MaxHP, c.healPct), npc.MaxHP-npc.HP)
	}
	if n <= 0 || npc.HP >= npc.MaxHP {
		return
	}
	npc.HP = min(npc.HP+n, npc.MaxHP)
	var sess *session.Session
	if p := w.Players[npc.CurrentEnemy]; p != nil {
		sess = w.findSessionByEntityIDLocked(p.ServerID)
	}
	w.tellNPCHealLocked(sess, npc, n)
	log.Printf("Template: %s heals itself with %q for %d (HP %d/%d)", npc.Name, c.spell.Name, n, npc.HP, npc.MaxHP)
}

// tellNPCHealLocked tells sess (the one the NPC fights or hit) of the NPC's
// heal on itself: a 0x00DB from the NPC to the NPC, as a player's on itself
// is told.
func (w *World) tellNPCHealLocked(sess *session.Session, npc *NPC, n int) {
	if sess == nil {
		return
	}
	if nid := w.objectIDForEntityLocked(sess, npc.ServerID); nid != 0 {
		w.sendDamageIndication(sess, nid, n, nid, 0) // the creature's on itself: untagged
	}
}

// templateSpecialLocked is a templated NPC's special in its fight with target
// (after its round's hits, at each of its turns in reach): on its clock of
// time at its enemy (special_every), and by chance (special_chance) rolled
// once a round, landing templateSpecialAfter past the round's last hit; one
// its NPC was too busy or too far to land in time (casting, closing in) is
// lost with its round. While its cast holds it (castHeld) neither clock
// counts and none comes: its melee rounds go on through a cast (petition
// 1593), its specials wait as they did. Must be called with the lock held.
func (w *World) templateSpecialLocked(npc *NPC, target EntityID, now time.Time) {
	t := npc.tmpl
	if t == nil || npc.castHeld(now) {
		return
	}
	w.templateKickLocked(npc, target, now)
	if t.SpecialDamage <= 0 || npc.Level < t.SpecialFrom {
		return // a creature below its template's special_from fights with its melee alone
	}
	f := npc.fight()
	if t.SpecialEvery > 0 {
		switch gap := now.Sub(f.timedSeen); {
		case f.timedSeen.IsZero():
			f.timedLeft = templateJitter(t.SpecialFirst, t.SpecialJitter)
		case gap > 0 && gap <= templateContactGap:
			f.timedLeft -= gap // at its enemy since its last turn
		}
		f.timedSeen = now
		if f.timedLeft <= 0 && !now.Before(f.busy) {
			f.timedLeft = templateJitter(t.SpecialEvery, t.SpecialJitter)
			w.templateSpecialHitLocked(npc, target, now)
		}
	}
	if t.SpecialChance <= 0 || npc.RoundStart.IsZero() {
		return
	}
	if !npc.RoundStart.Equal(f.round) {
		f.round, f.special = npc.RoundStart, time.Time{}
		if templateIntn(100) < t.SpecialChance {
			_, _, _, hits := npcRound(npc)
			f.special = npc.RoundStart.Add(hits[len(hits)-1] + templateSpecialAfter)
		}
	}
	if f.special.IsZero() || now.Before(f.special) {
		return
	}
	late := now.Sub(f.special)
	f.special = time.Time{}
	if late > templateSpecialAfter {
		return // its round went by out of reach or casting: the kick with it
	}
	w.templateSpecialHitLocked(npc, target, now)
}

// templateJitter is base seconds, jitter seconds either way (the templates'
// dice), as a duration; never below 0.
func templateJitter(base, jitter float64) time.Duration {
	d := base
	if jitter > 0 {
		d += float64(templateIntn(int(2*jitter*1000)+1))/1000 - jitter
	}
	return time.Duration(math.Max(d, 0) * float64(time.Second))
}

// templateSpecialHitLocked lands a templated NPC's special on target (a
// player, or a player's pet): its 0x00B1 of cast 0 with its visual, the level
// times special_damage, fixed and through any armour, a death, and the
// target's damage shields' answer as to a melee hit. Must be called with the
// lock held.
func (w *World) templateSpecialHitLocked(npc *NPC, target EntityID, now time.Time) {
	t := npc.tmpl
	// an add's is its creature's of its level, at its share (summon.go)
	w.templateStrikeLocked(npc, target, now, npc.shared(max(int(math.Round(float64(npc.figureLevel())*t.SpecialDamage)), 1)), specialName(t))
}

// templateKickLocked is its template's kick on its own clock (KickEvery): the first KickFirst
// into the fight, then one every KickEvery, counted as the timed special's is (time at its enemy,
// none while it casts), each KickDamage, fixed and through any armour. Must be called with the
// lock held.
func (w *World) templateKickLocked(npc *NPC, target EntityID, now time.Time) {
	t := npc.tmpl
	if t.KickDamage <= 0 || t.KickEvery <= 0 {
		return
	}
	f := npc.fight()
	switch gap := now.Sub(f.kickSeen); {
	case f.kickSeen.IsZero():
		f.kickLeft = templateJitter(t.KickFirst, 0)
	case gap > 0 && gap <= templateContactGap:
		f.kickLeft -= gap // at its enemy since its last turn
	}
	f.kickSeen = now
	if f.kickLeft <= 0 && !now.Before(f.busy) {
		f.kickLeft = templateJitter(t.KickEvery, 0)
		w.templateStrikeLocked(npc, target, now, npc.stoodOut(npc.shared(t.KickDamage)), "Kick")
	}
}

// templateStrikeLocked is a fixed hit of its template's (its special, its kick) on target: a
// player's or a pet's, through any armour, with the special's visual; name says it in the log.
// Must be called with the lock held.
func (w *World) templateStrikeLocked(npc *NPC, target EntityID, now time.Time, damage int, name string) {
	t := npc.tmpl
	fx := t.SpecialFX
	if fx == 0 {
		fx = templateSpecialFX
	}
	if foe := w.castFoeLocked(npc, target); foe != nil {
		// the NPC a trigger set it on with casts (SetNPCOn): through any armour too
		w.broadcastNPCSwingLocked(npc, target, 0, fx)
		w.castFoeHitLocked(npc, foe, damage, name, now)
		return
	}
	if p := w.Players[target]; p != nil {
		if p.IsDead() || w.apartLocked(p, npc) {
			return
		}
		w.broadcastNPCSwingLocked(npc, target, 0, fx)
		p.HP = max(p.HP-damage, 0)
		p.StatsDirty = true
		p.LastCombatTime = now
		w.breakInvisLocked(p, "a hit") // invis.go: every invisibility, whatever gave it
		sess := w.findSessionByEntityIDLocked(p.ServerID)
		if sess != nil {
			w.sendDamageIndication(sess, w.objectIDForEntityLocked(sess, npc.ServerID), -damage, sess.PlayerObjectID(), 0) // the creature's on the player: untagged
		}
		log.Printf("Template: %s's %s hits %s for %d (HP %d/%d)", npc.Name, name, p.Name, damage, p.HP, p.MaxHPNow())
		if p.HP <= 0 {
			w.playerDiedLocked(sess, p, now)
			w.killedPlayerLocked(npc, p) // its line at the kill (selfbuff.go)
			return
		}
		w.wereBiteLocked(sess, p, npc)                 // a were's bite may infect (wereinfect.go)
		if !w.playerShieldsLocked(sess, p, npc, now) { // a melee hit's answer (dshield.go)
			w.defenceProcsLocked(sess, p, npc, now)
		}
		return
	}
	pet := w.NPCs[target]
	if pet == nil || pet.Owner == 0 || pet.Dead {
		return
	}
	w.broadcastNPCSwingLocked(npc, target, 0, fx)
	pet.HP = max(pet.HP-damage, 0)
	pet.LastCombatTime = now
	log.Printf("Template: %s's %s hits the pet %q for %d (HP %d/%d)", npc.Name, name, pet.Name, damage, pet.HP, pet.MaxHP)
	if pet.HP == 0 {
		w.killNPC(pet)
		w.dropAggroLocked(pet.ServerID)
		return
	}
	w.petShieldsLocked(pet, npc, now)
}

func specialName(t *ClassTemplate) string {
	if t.Special != "" {
		return t.Special
	}
	return "special"
}

// templateResist is the resist a spell's hit is cut by: its element by the
// words of its name, then its line, else arcane (as a pact's price,
// lifetap.go). Ours: the lines rarely name one ("- Lightning Based AoE").
func templateResist(name, line string) Stat {
	for _, text := range []string{name, line} {
		words := strings.FieldsFunc(strings.ToLower(text), func(r rune) bool { return r < 'a' || r > 'z' })
		for _, el := range templateElements {
			for _, w := range words {
				for _, stem := range el.stems {
					if strings.HasPrefix(w, stem) {
						return el.resist
					}
				}
			}
		}
	}
	return StatAR
}

// templateElements are the elements' words, by the stem a word of a spell's
// name begins with, cold before fire before lightning ("Ice Storm", "Fire
// Storm", "Energy Storm").
var templateElements = []struct {
	resist Stat
	stems  []string
}{
	{StatCR, []string{"ice", "icy", "frost", "freez", "froz", "cold", "chill", "snow", "sleet", "winter", "glaci", "hail", "blizzard", "frigid"}},
	{StatFR, []string{"fire", "flam", "burn", "blaz", "inferno", "ember", "pyre", "incinerat", "scorch", "phoenix", "lava", "magma", "immolat", "combust"}},
	{StatLR, []string{"lightning", "static", "shock", "electr", "thunder", "energy", "spark"}},
	{StatPR, []string{"poison", "venom", "toxi"}},
	{StatDR, []string{"disease", "plague", "pestilen", "blight", "rot", "fever"}},
}

// templateLineLocked says a templated NPC's template for #stats.
func (w *World) templateLineLocked(npc *NPC, now time.Time) string {
	t := npc.tmpl
	parts := []string{fmt.Sprintf("Class template %s: a %s, hit points %d (%d%% of its %d), melee taken %d%%, its melee %d%%",
		t.Name, nameIn(classNames, npc.Class), npc.MaxHP, t.HPPct, npc.tbase.maxHP, t.TakenPct, t.MeleePct)}
	specials := t.SpecialDamage > 0 && (t.SpecialChance > 0 || t.SpecialEvery > 0)
	if specials && t.SpecialFrom > npc.Level {
		parts = append(parts, fmt.Sprintf("no %s below level %d", specialName(t), t.SpecialFrom))
		specials = false
	}
	if specials && t.SpecialChance > 0 {
		parts = append(parts, fmt.Sprintf("%s in %d%% of its rounds for %d", specialName(t), t.SpecialChance,
			max(int(math.Round(float64(npc.Level)*t.SpecialDamage)), 1)))
	}
	if specials && t.SpecialEvery > 0 {
		parts = append(parts, fmt.Sprintf("%s every %g-%g s at its enemy for %d, the first %g-%g s in", specialName(t),
			t.SpecialEvery-t.SpecialJitter, t.SpecialEvery+t.SpecialJitter, max(int(math.Round(float64(npc.Level)*t.SpecialDamage)), 1),
			math.Max(t.SpecialFirst-t.SpecialJitter, 0), t.SpecialFirst+t.SpecialJitter))
	}
	for i := range t.abilities {
		a := &t.abilities[i]
		when := fmt.Sprintf("every %v", a.every)
		if a.Chain {
			when = "chained"
		}
		if a.Kind == database.AbilityStrip {
			parts = append(parts, stripStatsWords(a, when)) // strip.go
			continue
		}
		what := "nothing at its level"
		s := a.spellFor(npc.Class, npc.Level)
		if s == nil {
			s = a.share
		}
		if s != nil {
			what = fmt.Sprintf("%s (level %d, %v cast)", s.Name, s.Level, a.castTime(s))
		}
		line := fmt.Sprintf("%s %s %s at %d%%", a.Kind, what, when, a.DamageOr())
		if a.Damage > 0 {
			line = fmt.Sprintf("%s %s %s for a fixed %d", a.Kind, what, when, npc.shared(a.Damage))
		}
		if a.Kind == database.AbilityHeal && a.HealPct > 0 && s != nil {
			line = fmt.Sprintf("heal %s (%v cast) of %d%% of its hit points below %d%% %s", s.Name, a.castTime(s), a.HealPct, a.BelowOr(), when)
		}
		if a.OutOfReach {
			line += fmt.Sprintf(", only out of its reach within %g", a.RangeOr())
		}
		if a.Kind != database.AbilityHeal && a.Below > 0 {
			line += fmt.Sprintf(", only below %d%% of its hit points", a.Below)
		}
		parts = append(parts, line+abilityTimesWords(npc, a, now)) // abilitytimes.go
	}
	if t.hitHeal > 0 {
		parts = append(parts, fmt.Sprintf("heals %d at every hit that lands on it", npc.shared(t.hitHeal))) // hitheal.go
	}
	if w := preferWords(t); w != "" {
		parts = append(parts, w) // prefer.go
	}
	if npc.add == nil {
		for i := range t.summons {
			s := &t.summons[i]
			parts = append(parts, fmt.Sprintf("summon below %d%%: %d %s, %s", s.Below, s.Count, s.NPC, shareWords(s.Share, npc))) // summon.go
		}
		if w := enrageWords(npc); w != "" {
			parts = append(parts, w) // enrage.go
		}
		if w := lastStandWords(npc); w != "" {
			parts = append(parts, w) // laststand.go
		}
		if w := wardWords(npc); w != "" {
			parts = append(parts, w) // ward.go
		}
		if w := deathHoldWords(npc); w != "" {
			parts = append(parts, w) // deathhold.go
		}
	}
	parts = append(parts, procsWords(npc)...) // creatureproc.go
	if fp := npc.fightProcNow(); fp != nil {
		parts = append(parts, fmt.Sprintf("%s on for this fight: %d%% of its melee hits %d more", fp.name, fp.chance, fp.damage))
	}
	if w := addWords(npc); w != "" {
		parts = append(parts, w) // summon.go
	}
	return strings.Join(parts, "; ")
}
