package entity

import (
	"fmt"
	"log"
	"math"
	"math/rand"
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
//   - casts its abilities in its fights, from where it stands in reach of its
//     enemy (a player, or a player's pet), one at a time, its melee waiting
//     while it casts. Each ability is on a clock of its own (every, first into
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
// put on: no player effect has an NPC's source yet.
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

// templateBase is what a templated NPC was without its template: put back
// when the template is taken away.
type templateBase struct {
	class, maxHP int
}

// templateFight is a templated NPC's fight by its template: whether its
// clocks are set, when each of its abilities next comes, its cast's time (no
// melee, no other cast until then), the chained ability owed after it (its
// index + 1; 0 none), and the round its special was rolled for and when it
// lands.
type templateFight struct {
	on      bool
	next    [database.MaxTemplateAbilities]time.Time
	busy    time.Time
	chain   int
	round   time.Time
	special time.Time
}

// fight is the NPC's fight by its template, begun if it had none.
func (n *NPC) fight() *templateFight {
	if n.tfight == nil {
		n.tfight = &templateFight{}
	}
	return n.tfight
}

// endTemplateFight forgets the NPC's fight by its template: the next one
// starts its clocks again.
func (n *NPC) endTemplateFight() {
	if n.tfight != nil {
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
		return hit
	}
	return max(1, int(math.Round(float64(hit)*float64(n.tmpl.MeleePct)/100)))
}

// meleeTaken is a player's or a pet's melee hit on the NPC as its template
// takes it (taken_pct), at least 1.
func (n *NPC) meleeTaken(hit int) int {
	if n.tmpl == nil || n.tmpl.TakenPct == 100 {
		return hit
	}
	return max(1, int(math.Round(float64(hit)*float64(n.tmpl.TakenPct)/100)))
}

// classTemplates is the world's templates by name (database.TemplateKey) and
// the NPC names (database.NPCClassKey) that fight by them; problems what of
// the tables the world could not use.
type classTemplates struct {
	byName   map[string]*ClassTemplate
	use      map[string]string
	problems []string
}

// templateForLocked is the template the NPC fights by: its name's
// npctemplate row's, if the template is there. Never a pet's nor a training
// dummy's (they fight their own ways). Must be called with the lock held.
func (w *World) templateForLocked(npc *NPC) *ClassTemplate {
	if len(w.templates.use) == 0 || npc.Owner != 0 || npc.Dummy != NotDummy {
		return nil
	}
	name := w.templates.use[database.NPCClassKey(npc.Name)]
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
	var hits, drains, heals, price bool
	for _, e := range d.Effects {
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
	switch kind {
	case database.AbilityNuke:
		return d.Harmful && single && hits && !drains
	case database.AbilityAoE:
		return d.Harmful && area && hits && !drains
	case database.AbilityLifetap:
		return d.Harmful && single && drains
	case database.AbilityHeal:
		return !d.Harmful && heals && !price && !area
	}
	return false
}

// templateCandidates is the spells of a class's (by level) an ability may
// cast: of its kind, its line's words in their names, their recast and cast
// time within its bounds.
func templateCandidates(a database.TemplateAbility, spells []*CastSpell) []*CastSpell {
	words := strings.Fields(strings.ToLower(a.Line))
	var out []*CastSpell
	for _, s := range spells {
		if !templateKindTakes(a.Kind, s) {
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
	s := SpellOf(&ts.Pattern, 0, ts.Line, ts.FX)
	if ts.HasArea {
		s.Area = s.Area.With(AreaRule{Centre: ts.Area.Centre, Radius: float32(ts.Area.Radius), Max: ts.Area.MaxTargets})
	}
	s.resist = templateResist(ts.Pattern.SpellName, ts.Line)
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
			if ra.Spell != "" {
				ts, ok, err := db.TemplateSpellNamed(ra.Spell)
				if err != nil {
					return 0, fmt.Errorf("classtemplates %q: %w", name, err)
				}
				if ok {
					a.pinned = templateSpellOf(ts)
				} else {
					t.problems = append(t.problems, fmt.Sprintf("ability %d: no spell named %q (it casts nothing)", i+1, ra.Spell))
				}
			} else {
				found := false
				for _, c := range classes {
					spells, err := spellsOf(c)
					if err != nil {
						return 0, err
					}
					a.byClass[c] = templateCandidates(ra, spells)
					found = found || len(a.byClass[c]) > 0
				}
				if !found {
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

	w.mu.Lock()
	defer w.mu.Unlock()
	w.templates = classTemplates{byName: byName, use: use, problems: problems}
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
	return append([]string{first}, w.templates.problems...)
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
// whether the NPC is casting, its melee waiting. Must be called with the lock
// held.
func (w *World) templateTurnLocked(npc *NPC, target EntityID, now time.Time) bool {
	t := npc.tmpl
	if t == nil || len(t.abilities) == 0 {
		return false
	}
	f := npc.fight()
	if !f.on {
		f.on = true
		for i := range t.abilities {
			f.next[i] = now.Add(t.abilities[i].first)
		}
	}
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
		if a.Chain || now.Before(f.next[i]) || npc.Level < a.From {
			continue
		}
		if a.Kind == database.AbilityHeal && npc.HP*100 >= npc.MaxHP*a.BelowOr() {
			continue // whole enough: it waits, due
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

// npcCast is a templated NPC's cast on its way to landing (templateSystem).
type npcCast struct {
	npc    *NPC
	tmpl   *ClassTemplate // the template it was cast by: another since drops it
	spell  *CastSpell
	pct    int
	heal   bool
	target EntityID
	others []EntityID
	lands  time.Time
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
	spell := a.spellFor(npc.Class, npc.Level)
	if spell == nil {
		return false
	}
	c := &npcCast{npc: npc, tmpl: t, spell: spell, pct: a.DamageOr(), target: target,
		lands: now.Add(spell.CastTime + castLand)}
	if a.Kind == database.AbilityHeal {
		c.heal, c.target = true, npc.ServerID
	} else {
		if !w.templateMayHitLocked(npc, target) {
			return false
		}
		if spell.Area.Centre != AreaNone {
			c.others = w.templateAreaLocked(npc, spell, target)
		}
	}
	w.npcCasts = append(w.npcCasts, c)
	f := npc.fight()
	f.busy = now.Add(spell.CastTime + castBusy)
	if j := i + 1; j < len(t.abilities) && t.abilities[j].Chain {
		f.chain = j + 1
	}
	w.broadcastNPCSwingLocked(npc, c.target, uint32(spell.CastTime/time.Millisecond), spell.visual(), c.others...)
	log.Printf("Template: %s (%s, level %d) begins %q (spell %d) on %s: %v cast, %d%% of its figures, %d others",
		npc.Name, t.Name, npc.Level, spell.Name, spell.SpellID, w.entityNameLocked(c.target), spell.CastTime, c.pct, len(c.others))
	return true
}

// templateMayHitLocked reports whether the NPC's harmful spell may be cast at
// target: a living player in its world (not across the water's edge from it,
// water.go), or a living pet of a player's there.
func (w *World) templateMayHitLocked(npc *NPC, target EntityID) bool {
	if p := w.Players[target]; p != nil {
		return !p.IsDead() && !p.respawnPending() && p.World == npc.World && !w.apartLocked(p, npc)
	}
	pet := w.NPCs[target]
	return pet != nil && pet.Owner != 0 && !pet.Dead && pet.World == npc.World &&
		(w.env == nil || w.npcInWaterLocked(pet) == w.npcInWaterLocked(npc))
}

// templateAreaLocked is a templated NPC's area spell's others as its cast
// begins, as a player's are chosen (area.go): round its target (or itself,
// a caster's area), within its radius, the nearest first, its cap with the
// target counted; the players a creature may notice (aggro.go: not hidden, not
// a GM who turned aggro off, not loading a world), not across the water's
// edge from it, and players' pets.
func (w *World) templateAreaLocked(npc *NPC, spell *CastSpell, target EntityID) []EntityID {
	if w.SpatialIndex == nil {
		return nil
	}
	a := &spell.Area
	r := a.radiusFor(true)
	cx, cy, cz := npc.X, npc.Y, npc.Z
	if a.Centre == AreaOnTarget {
		if p := w.Players[target]; p != nil {
			cx, cy, cz = p.X, p.Y, p.Z
		} else if pet := w.NPCs[target]; pet != nil {
			cx, cy, cz = pet.X, pet.Y, pet.Z
		}
	}
	var near []areaNear
	for _, id := range w.SpatialIndex.QueryRadius(cx, cz, r) {
		if id == target {
			continue
		}
		if p := w.Players[id]; p != nil {
			if p.World == npc.World && w.noticeableLocked(p) && !w.apartLocked(p, npc) {
				if d := dist3(p.X, p.Y, p.Z, cx, cy, cz); d <= r {
					near = append(near, areaNear{id, d})
				}
			}
			continue
		}
		if pet := w.NPCs[id]; pet != nil && pet.Owner != 0 && !pet.Dead && pet.World == npc.World {
			if d := dist3(pet.X, pet.Y, pet.Z, cx, cy, cz); d <= r {
				near = append(near, areaNear{id, d})
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
		if c.heal {
			w.templateHealLocked(c)
			continue
		}
		w.templateHitLocked(c, c.target, c.spell.Def.Effects, now)
		share := c.spell.Def.Effects
		if e := c.spell.Area.Effects; e != nil {
			share = e
		}
		for _, id := range c.others {
			if npc.Dead {
				break
			}
			w.templateHitLocked(c, id, share, now)
		}
	}
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

// templateHitLocked lands a templated NPC's harmful spell on one it hits (its
// target or one of its others): a player takes the figure less its resist's
// share (resist.go), told it in a 0x00DB from the NPC and a line, and dies of
// it at nothing; a pet takes it whole. The drained share of what was taken
// heals the NPC. Must be called with the lock held.
func (w *World) templateHitLocked(c *npcCast, id EntityID, effects []SpellEffect, now time.Time) {
	npc := c.npc
	hit, drained := templateFigure(effects, c.pct)
	if hit <= 0 {
		return
	}
	var sess *session.Session
	if p := w.Players[id]; p != nil {
		if p.IsDead() || p.respawnPending() || p.World != npc.World {
			return
		}
		resist := c.spell.resist
		if resist == StatNone {
			resist = StatAR
		}
		share, how := RollResist(p.StatNow(resist))
		taken := max(int(math.Round(float64(hit)*(1-share))), 1)
		drained = int(math.Round(float64(drained) * (1 - share)))
		p.HP = max(p.HP-taken, 0)
		p.StatsDirty = true
		p.LastCombatTime = now
		sess = w.findSessionByEntityIDLocked(p.ServerID)
		if sess != nil {
			if nid := w.objectIDForEntityLocked(sess, npc.ServerID); nid != 0 {
				w.sendDamageIndication(sess, nid, -taken, sess.PlayerObjectID())
			}
			w.sendChatToSession(sess, fmt.Sprintf("%s's %s hits you for %d (%s).", npc.Name, c.spell.Name, taken, resistWords(share, how)))
		}
		log.Printf("Template: %s's %q hits %s for %d of %d (resist %.0f%%; HP %d/%d)", npc.Name, c.spell.Name, p.Name, taken, hit,
			share*100, p.HP, p.MaxHPNow())
		if p.HP <= 0 {
			w.playerDiedLocked(sess, p, now)
		}
	} else if pet := w.NPCs[id]; pet != nil && pet.Owner != 0 && !pet.Dead {
		pet.HP = max(pet.HP-hit, 0)
		pet.LastCombatTime = now
		log.Printf("Template: %s's %q hits the pet %q for %d (HP %d/%d)", npc.Name, c.spell.Name, pet.Name, hit, pet.HP, pet.MaxHP)
		if pet.HP == 0 {
			w.killNPC(pet)
			w.dropAggroLocked(pet.ServerID)
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

// templateHealLocked lands a templated NPC's heal on itself: its figure at
// the ability's percent, when it is short of its maximum.
func (w *World) templateHealLocked(c *npcCast) {
	npc := c.npc
	n := 0
	for _, e := range c.spell.Def.Effects {
		if e.Kind == EffectHP && e.Amount > 0 && !e.Self && e.Drain == DrainNone {
			n += int(math.Round(float64(capAmount(rollAmount(e), e.Cap)) * float64(c.pct) / 100))
		}
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
		w.sendDamageIndication(sess, nid, n, nid)
	}
}

// templateSpecialLocked is a templated NPC's special in its fight with target
// (after its round's hits): rolled once a round, landing templateSpecialAfter
// past the round's last hit; one its NPC was too busy or too far to land in
// time (casting, closing in) is lost with its round. Must be called with the
// lock held.
func (w *World) templateSpecialLocked(npc *NPC, target EntityID, now time.Time) {
	t := npc.tmpl
	if t == nil || t.SpecialChance <= 0 || t.SpecialDamage <= 0 || npc.RoundStart.IsZero() {
		return
	}
	f := npc.fight()
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
	damage := max(int(math.Round(float64(npc.Level)*t.SpecialDamage)), 1)
	fx := t.SpecialFX
	if fx == 0 {
		fx = templateSpecialFX
	}
	if p := w.Players[target]; p != nil {
		if p.IsDead() || w.apartLocked(p, npc) {
			return
		}
		w.broadcastNPCSwingLocked(npc, target, 0, fx)
		p.HP = max(p.HP-damage, 0)
		p.StatsDirty = true
		p.LastCombatTime = now
		sess := w.findSessionByEntityIDLocked(p.ServerID)
		if sess != nil {
			w.sendDamageIndication(sess, w.objectIDForEntityLocked(sess, npc.ServerID), -damage, sess.PlayerObjectID())
		}
		log.Printf("Template: %s's %s hits %s for %d (HP %d/%d)", npc.Name, specialName(t), p.Name, damage, p.HP, p.MaxHPNow())
		if p.HP <= 0 {
			w.playerDiedLocked(sess, p, now)
			return
		}
		w.playerShieldsLocked(sess, p, npc, now) // a melee hit's answer (dshield.go)
		return
	}
	pet := w.NPCs[target]
	if pet == nil || pet.Owner == 0 || pet.Dead {
		return
	}
	w.broadcastNPCSwingLocked(npc, target, 0, fx)
	pet.HP = max(pet.HP-damage, 0)
	pet.LastCombatTime = now
	log.Printf("Template: %s's %s hits the pet %q for %d (HP %d/%d)", npc.Name, specialName(t), pet.Name, damage, pet.HP, pet.MaxHP)
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
func (w *World) templateLineLocked(npc *NPC) string {
	t := npc.tmpl
	parts := []string{fmt.Sprintf("Class template %s: a %s, hit points %d (%d%% of its %d), melee taken %d%%, its melee %d%%",
		t.Name, nameIn(classNames, npc.Class), npc.MaxHP, t.HPPct, npc.tbase.maxHP, t.TakenPct, t.MeleePct)}
	if t.SpecialChance > 0 && t.SpecialDamage > 0 {
		parts = append(parts, fmt.Sprintf("%s in %d%% of its rounds for %d", specialName(t), t.SpecialChance,
			max(int(math.Round(float64(npc.Level)*t.SpecialDamage)), 1)))
	}
	for i := range t.abilities {
		a := &t.abilities[i]
		what := "nothing at its level"
		if s := a.spellFor(npc.Class, npc.Level); s != nil {
			what = fmt.Sprintf("%s (level %d, %v cast)", s.Name, s.Level, s.CastTime)
		}
		when := fmt.Sprintf("every %v", a.every)
		if a.Chain {
			when = "chained"
		}
		parts = append(parts, fmt.Sprintf("%s %s %s at %d%%", a.Kind, what, when, a.DamageOr()))
	}
	return strings.Join(parts, "; ")
}
