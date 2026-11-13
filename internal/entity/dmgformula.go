package entity

// The operator's damage formulas (combat.formula_file; docs/damage-formulas.md).
//
// The owner, 2026-11-13: "we have a damage formula we want to implement but
// want to keep it secret for now. without it being on the github also. we can
// add support for custom formula that is read of the VPS in a file". So the
// server carries only the support: a file the operator keeps on the box names
// formulas, one for each damage path (player_melee, npc_spell, ...: the
// <source>_<kind> table below, or <source> alone for every kind of that
// source), and where one is given its result replaces the built-in figure of
// that path's hit. It is read with the formula package's language (no I/O, no
// loops, bounded), compiled once, swapped in atomically.
//
// Every formula is given the built-in result as base (the figure that would
// have landed, every share of the balance taken), so a formula may adjust it
// or ignore it, and the inputs of the hit (levels, stats, weapon, spell,
// flags, a random number). A result that is NaN, infinite or negative falls
// back to base for that hit, logged at most once a minute per formula; a file
// that does not parse is refused whole (the formulas loaded before stay). The
// formulas' text is never logged, sent, audited or shown: a load logs their
// names, an error the line and column.
//
// What a formula does not replace: the steps that come after the figure on
// purpose (a creature held at 1 hit point by its boss moment takes nothing:
// deathhold.go; a training dummy takes nothing and fights back), and damage
// that is no combat hit (hazards, drowning, a boss moment's scripted blast, a
// guard's or a charmed creature's hit on another creature, a creature's hit on
// a pet or a mercenary).

import (
	"errors"
	"fmt"
	"log"
	"math"
	"math/rand"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/eqoa/server/internal/formula"
)

// dmgSource is who dealt a hit, the first half of a formula's name.
type dmgSource int

const (
	srcPlayer dmgSource = iota // a player's on a creature
	srcPvP                     // a player's (or its pet's) on a player: a duel or two flagged players
	srcNPC                     // a creature's on a player
	srcPet                     // a player's pet's on a creature
	srcMerc                    // a player's mercenary's on a creature
	nDmgSources
)

var dmgSourceNames = [nDmgSources]string{"player", "pvp", "npc", "pet", "merc"}

// dmgKind is what made the hit, the second half of a formula's name; its
// values are the damage number tags' (numbertags.go), which the formulas see
// as kind.
type dmgKind int

const (
	dkMelee   dmgKind = 1
	dkSpell   dmgKind = 2
	dkDoT     dmgKind = 3
	dkShield  dmgKind = 4
	dkProc    dmgKind = 5
	dkSpecial dmgKind = 6 // a creature template's special or kick
	nDmgKinds         = 7
)

var dmgKindNames = [nDmgKinds]string{"", "melee", "spell", "dot", "shield", "proc", "special"}

// dmgKindOf is a damage number's tag as a formula kind (a pet's or a merc's
// bit off; an untagged figure is melee).
func dmgKindOf(k numberKind) dmgKind {
	switch d := dmgKind(k &^ (numberByPet | numberByMerc)); d {
	case dkMelee, dkSpell, dkDoT, dkShield, dkProc:
		return d
	}
	return dkMelee
}

// dmgPathName is a formula's name: "<source>_<kind>".
func dmgPathName(s dmgSource, k dmgKind) string { return dmgSourceNames[s] + "_" + dmgKindNames[k] }

// dmgPathExists is whether the server has that path (npc_special is the
// creatures' alone).
func dmgPathExists(s dmgSource, k dmgKind) bool {
	return k >= dkMelee && k < nDmgKinds && (k != dkSpecial || s == srcNPC)
}

// DamageFormulaNames is every name a formula file may define: each path's,
// then each source's (for every kind of that source a path of its own does
// not cover).
func DamageFormulaNames() []string {
	var out []string
	for s := dmgSource(0); s < nDmgSources; s++ {
		for k := dkMelee; k < nDmgKinds; k++ {
			if dmgPathExists(s, k) {
				out = append(out, dmgPathName(s, k))
			}
		}
	}
	for s := dmgSource(0); s < nDmgSources; s++ {
		out = append(out, dmgSourceNames[s])
	}
	return out
}

func knownDamageFormula(name string) bool {
	for _, n := range DamageFormulaNames() {
		if n == name {
			return true
		}
	}
	return false
}

// dmgVars are the inputs a formula may read, in the order of their slots;
// each is read off the hit only when a loaded formula reads it.
// docs/damage-formulas.md describes each; a test holds the two equal.
var dmgVars = []struct {
	name string
	get  func(in *dmgIn) formula.Value
}{
	{"base", func(in *dmgIn) formula.Value { return num(in.base) }},
	{"raw", func(in *dmgIn) formula.Value { return num(in.raw) }},
	{"kind", func(in *dmgIn) formula.Value { return num(int(in.kind)) }},
	{"mode", func(in *dmgIn) formula.Value {
		if in.ranged {
			return formula.String("ranged")
		}
		return formula.String(dmgKindNames[in.kind])
	}},
	{"pvp", func(in *dmgIn) formula.Value { return flag(in.src == srcPvP) }},
	{"duel", func(in *dmgIn) formula.Value { return flag(in.duel) }},
	{"ranged", func(in *dmgIn) formula.Value { return flag(in.ranged) }},
	{"by_pet", func(in *dmgIn) formula.Value { return flag(in.byPet) }},
	{"spell_id", func(in *dmgIn) formula.Value {
		if in.spell != nil {
			return num(in.spell.SpellID)
		}
		return num(in.spellID)
	}},
	{"spell_level", func(in *dmgIn) formula.Value {
		if in.spell != nil {
			return num(in.spell.Level)
		}
		return num(in.spellLevel)
	}},
	{"resist", func(in *dmgIn) formula.Value { return num(int(in.resist)) }},
	{"con", func(in *dmgIn) formula.Value { return num(sideLevel(in.dp, in.dn) - sideLevel(in.ap, in.an)) }},

	{"a_kind", func(in *dmgIn) formula.Value { return formula.String(in.attackerKind()) }},
	{"a_name", func(in *dmgIn) formula.Value { return formula.String(sideName(in.ap, in.an, in.src == srcMerc)) }},
	{"a_family", func(in *dmgIn) formula.Value { return formula.String(sideFamily(in.ap, in.an)) }},
	{"a_race", func(in *dmgIn) formula.Value { return formula.String(sideRace(in.ap, in.an, in.src == srcMerc)) }},
	{"a_class", func(in *dmgIn) formula.Value { return num(sideClass(in.ap, in.an)) }},
	{"a_class_name", func(in *dmgIn) formula.Value { return formula.String(ClassName(sideClass(in.ap, in.an))) }},
	{"a_level", func(in *dmgIn) formula.Value { return num(sideLevel(in.ap, in.an)) }},
	{"a_offmod", func(in *dmgIn) formula.Value { off, _ := sideMods(in.ap, in.an); return num(off) }},
	{"a_defmod", func(in *dmgIn) formula.Value { _, def := sideMods(in.ap, in.an); return num(def) }},
	{"a_eff_level", func(in *dmgIn) formula.Value {
		off, _ := sideMods(in.ap, in.an)
		return num(sideLevel(in.ap, in.an) + off)
	}},
	{"a_hp", func(in *dmgIn) formula.Value { return num(sideHP(in.ap, in.an)) }},
	{"a_maxhp", func(in *dmgIn) formula.Value { return num(sideMaxHP(in.ap, in.an)) }},
	{"a_power", func(in *dmgIn) formula.Value { return num(sidePower(in.ap, in.an)) }},
	{"a_maxpower", func(in *dmgIn) formula.Value { return num(sideMaxPower(in.ap, in.an)) }},
	{"a_str", attStat(StatStr)}, {"a_sta", attStat(StatSta)}, {"a_agi", attStat(StatAgi)}, {"a_dex", attStat(StatDex)},
	{"a_wis", attStat(StatWis)}, {"a_int", attStat(StatInt)}, {"a_cha", attStat(StatCha)}, {"a_ac", attStat(StatAC)},

	{"weapon_dmg", func(in *dmgIn) formula.Value {
		if in.ap == nil {
			return num(0)
		}
		dmg, _, _ := in.ap.meleeWeapon()
		return num(dmg)
	}},
	{"weapon_cond", func(in *dmgIn) formula.Value {
		if in.ap == nil {
			return num(0)
		}
		_, cond, _ := in.ap.meleeWeapon()
		return formula.Number(cond)
	}},
	{"weapon_delay", func(in *dmgIn) formula.Value {
		if in.ap == nil {
			return num(0)
		}
		return formula.Number(in.ap.Swing.Delay.Seconds())
	}},
	{"two_handed", func(in *dmgIn) formula.Value {
		if in.ap == nil {
			return num(0)
		}
		_, _, two := in.ap.meleeWeapon()
		return flag(two)
	}},

	{"d_kind", func(in *dmgIn) formula.Value { return formula.String(in.defenderKind()) }},
	{"d_name", func(in *dmgIn) formula.Value { return formula.String(sideName(in.dp, in.dn, false)) }},
	{"d_family", func(in *dmgIn) formula.Value { return formula.String(sideFamily(in.dp, in.dn)) }},
	{"d_race", func(in *dmgIn) formula.Value { return formula.String(sideRace(in.dp, in.dn, false)) }},
	{"d_class", func(in *dmgIn) formula.Value { return num(sideClass(in.dp, in.dn)) }},
	{"d_class_name", func(in *dmgIn) formula.Value { return formula.String(ClassName(sideClass(in.dp, in.dn))) }},
	{"d_level", func(in *dmgIn) formula.Value { return num(sideLevel(in.dp, in.dn)) }},
	{"d_offmod", func(in *dmgIn) formula.Value { off, _ := sideMods(in.dp, in.dn); return num(off) }},
	{"d_defmod", func(in *dmgIn) formula.Value { _, def := sideMods(in.dp, in.dn); return num(def) }},
	{"d_eff_level", func(in *dmgIn) formula.Value {
		_, def := sideMods(in.dp, in.dn)
		return num(sideLevel(in.dp, in.dn) + def)
	}},
	{"d_hp", func(in *dmgIn) formula.Value { return num(sideHP(in.dp, in.dn)) }},
	{"d_maxhp", func(in *dmgIn) formula.Value { return num(sideMaxHP(in.dp, in.dn)) }},
	{"d_power", func(in *dmgIn) formula.Value { return num(sidePower(in.dp, in.dn)) }},
	{"d_maxpower", func(in *dmgIn) formula.Value { return num(sideMaxPower(in.dp, in.dn)) }},
	{"d_defence", func(in *dmgIn) formula.Value {
		if in.dp == nil && in.dn != nil {
			return formula.Number(targetDefence(in.dn))
		}
		return num(0)
	}},
	{"d_dummy", func(in *dmgIn) formula.Value {
		if in.dp == nil && in.dn != nil {
			return num(int(in.dn.Dummy))
		}
		return num(0)
	}},
	{"d_str", defStat(StatStr)}, {"d_sta", defStat(StatSta)}, {"d_agi", defStat(StatAgi)}, {"d_dex", defStat(StatDex)},
	{"d_wis", defStat(StatWis)}, {"d_int", defStat(StatInt)}, {"d_cha", defStat(StatCha)}, {"d_ac", defStat(StatAC)},
	{"d_fr", defStat(StatFR)}, {"d_cr", defStat(StatCR)}, {"d_pr", defStat(StatPR)}, {"d_dr", defStat(StatDR)},
	{"d_lr", defStat(StatLR)}, {"d_ar", defStat(StatAR)},

	{"owner_level", func(in *dmgIn) formula.Value {
		if in.owner == nil {
			return num(0)
		}
		return num(in.owner.Level)
	}},
	{"owner_class", func(in *dmgIn) formula.Value {
		if in.owner == nil {
			return num(-1)
		}
		return num(in.owner.Class)
	}},
	{"owner_class_name", func(in *dmgIn) formula.Value {
		if in.owner == nil {
			return formula.String("")
		}
		return formula.String(ClassName(in.owner.Class))
	}},
}

func num(n int) formula.Value { return formula.Number(float64(n)) }

func flag(b bool) formula.Value {
	if b {
		return formula.Number(1)
	}
	return formula.Number(0)
}

// The kinds a side may be.
const (
	kindPlayer   = "player"
	kindCreature = "creature"
	kindPet      = "pet"
	kindMerc     = "merc"
)

func (in *dmgIn) attackerKind() string {
	switch {
	case in.src == srcMerc:
		return kindMerc
	case in.src == srcPet || in.byPet:
		return kindPet
	case in.ap != nil:
		return kindPlayer
	}
	return kindCreature
}

func (in *dmgIn) defenderKind() string {
	switch {
	case in.dp != nil:
		return kindPlayer
	case in.dn != nil && in.dn.Owner != 0:
		return kindPet
	}
	return kindCreature
}

func sideName(p *Player, n *NPC, preferNPC bool) string {
	if n != nil && (p == nil || preferNPC) {
		return n.Name
	}
	if p != nil {
		return p.Name
	}
	return ""
}

// sideFamily is a creature's family key: the class template it fights by
// (classtemplate.go), "" for none and for a player.
func sideFamily(p *Player, n *NPC) string {
	if p == nil && n != nil {
		return n.TemplateName()
	}
	return ""
}

// sideRace is a player's race's name (RaceName), a creature's race as its
// row has it (npcs.race).
func sideRace(p *Player, n *NPC, preferNPC bool) string {
	if n != nil && (p == nil || preferNPC) {
		return n.Race
	}
	if p != nil {
		return RaceName(p.Race)
	}
	return ""
}

func sideClass(p *Player, n *NPC) int {
	switch {
	case p != nil:
		return p.Class
	case n != nil:
		return n.Class
	}
	return -1
}

func sideLevel(p *Player, n *NPC) int {
	switch {
	case p != nil:
		return p.Level
	case n != nil:
		return n.Level
	}
	return 0
}

// sideMods is a side's OFF and DEF mods (workbookmore.go): a player's from
// its effects, a creature's its own.
func sideMods(p *Player, n *NPC) (off, def int) {
	switch {
	case p != nil:
		return p.Mods.OffMod, p.Mods.DefMod
	case n != nil:
		return n.OffMod, n.DefMod
	}
	return 0, 0
}

func sideHP(p *Player, n *NPC) int {
	switch {
	case p != nil:
		return p.HP
	case n != nil:
		return n.HP
	}
	return 0
}

func sideMaxHP(p *Player, n *NPC) int {
	switch {
	case p != nil:
		return p.MaxHPNow()
	case n != nil:
		return n.MaxHP
	}
	return 0
}

func sidePower(p *Player, n *NPC) int {
	switch {
	case p != nil:
		return p.Power
	case n != nil:
		return n.Power
	}
	return 0
}

func sideMaxPower(p *Player, n *NPC) int {
	switch {
	case p != nil:
		return p.MaxPowerNow()
	case n != nil:
		return n.MaxPower
	}
	return 0
}

// sideStat is a stat as the client sees it (StatNow) for a player; for a
// creature, which has no stats of its own, what its effects add.
func sideStat(p *Player, n *NPC, s Stat) int {
	switch {
	case p != nil:
		return p.StatNow(s)
	case n != nil:
		m := n.effectMods()
		switch s {
		case StatStr:
			return m.Str
		case StatSta:
			return m.Sta
		case StatAgi:
			return m.Agi
		case StatDex:
			return m.Dex
		case StatWis:
			return m.Wis
		case StatInt:
			return m.Int
		case StatCha:
			return m.Cha
		case StatAC:
			return m.AC
		case StatFR:
			return m.FR
		case StatCR:
			return m.CR
		case StatPR:
			return m.PR
		case StatDR:
			return m.DR
		case StatLR:
			return m.LR
		case StatAR:
			return m.AR
		}
	}
	return 0
}

func attStat(s Stat) func(in *dmgIn) formula.Value {
	return func(in *dmgIn) formula.Value { return num(sideStat(in.ap, in.an, s)) }
}

func defStat(s Stat) func(in *dmgIn) formula.Value {
	return func(in *dmgIn) formula.Value { return num(sideStat(in.dp, in.dn, s)) }
}

// dmgVarIndex is an input's slot by its name.
var dmgVarIndex = func() map[string]int {
	m := make(map[string]int, len(dmgVars))
	for i, v := range dmgVars {
		m[v.name] = i
	}
	return m
}()

// DamageFormulaVars is every input's name, in order (the docs' test).
func DamageFormulaVars() []string {
	out := make([]string, len(dmgVars))
	for i, v := range dmgVars {
		out[i] = v.name
	}
	return out
}

// formulaRand is the formulas' draws: the server's own random source; a
// test's to fix (a formula.Forced replays a vector).
var formulaRand formula.Rand = formula.RandSource{Float64: rand.Float64, NormFloat64: rand.NormFloat64}

// maxFormulaDamage bounds a formula's result (a finite but absurd figure is
// held here rather than overflowing an int downstream).
const maxFormulaDamage = 1_000_000_000

// dmgIn is one hit's inputs. ap/an are the attacker as a player or an NPC
// (a merc both: its body's stats and its NPC; a pet its NPC), dp/dn the
// defender's; owner a pet's or a merc's.
type dmgIn struct {
	src                 dmgSource
	kind                dmgKind
	base                int
	ap                  *Player
	an                  *NPC
	dp                  *Player
	dn                  *NPC
	owner               *Player
	raw                 int // the path's figure before the balance's shares (the caller's; base when the path has none)
	spell               *CastSpell
	spellID, spellLevel int
	resist              Stat
	ranged, duel, byPet bool
}

// dmgEntry is one loaded formula.
type dmgEntry struct {
	name string
	prog *formula.Program

	failMu     sync.Mutex
	failLogged time.Time
	failQuiet  int
}

// dmgFormulaSet is one load of the file: a formula for each path, a path
// with none of its own taking its source's.
type dmgFormulaSet struct {
	paths  [nDmgSources][nDmgKinds]*dmgEntry
	names  []string
	tables int
}

// dmgFormulaState is the World's: the set in use (nil: the built-in
// everywhere), and the file it came from.
type dmgFormulaState struct {
	set atomic.Pointer[dmgFormulaSet]

	mu      sync.Mutex // the file's fields; a load at a time
	path    string
	modTime time.Time
	size    int64
	missing bool // the last look found no file (logged once)
}

// dmgRun is a run's scratch (the hit, its slots: the inputs, then the
// steps; the env), reused so that a hit allocates nothing.
type dmgRun struct {
	in  dmgIn
	v   []formula.Value
	env formula.Env
}

var dmgScratch = sync.Pool{New: func() any {
	return &dmgRun{v: make([]formula.Value, len(dmgVars)+formula.MaxSteps)}
}}

// formulaDamage is a hit's damage: base, the built-in figure, unless the
// operator's file has a formula for its path, whose result it is then (base
// again when that run fails). Cheap when no formula is loaded: one atomic
// load.
func (w *World) formulaDamage(base int, in dmgIn) int {
	set := w.dmgf.set.Load()
	if set == nil {
		return base
	}
	e := set.paths[in.src][in.kind]
	if e == nil {
		return base
	}
	return e.apply(base, &in)
}

// apply runs the formula on the hit.
func (e *dmgEntry) apply(base int, hit *dmgIn) int {
	r := dmgScratch.Get().(*dmgRun)
	defer dmgScratch.Put(r)
	r.in = *hit
	r.in.base = base
	for _, i := range e.prog.Uses() {
		r.v[i] = dmgVars[i].get(&r.in)
	}
	r.env = formula.Env{V: r.v, Rand: formulaRand}
	x, err := e.prog.Run(&r.env)
	if err != nil {
		e.failed(err)
		return base
	}
	return int(math.Round(math.Min(x, maxFormulaDamage)))
}

// failed logs a formula's failed run, at most once a minute (with how many
// it kept quiet), naming the formula and the part and kind of the mistake,
// never its text or a value.
func (e *dmgEntry) failed(err error) {
	e.failMu.Lock()
	defer e.failMu.Unlock()
	now := time.Now()
	if !e.failLogged.IsZero() && now.Sub(e.failLogged) < time.Minute {
		e.failQuiet++
		return
	}
	log.Printf("Damage formula %s: %v; the built-in figure used (%d more since the last line)", e.name, err, e.failQuiet)
	e.failLogged, e.failQuiet = now, 0
}

// SetDamageFormulaFile names the formula file (combat.formula_file; "" for
// none) and loads it. A file that is missing or does not parse leaves the
// built-in figures (logged); the server starts either way.
func (w *World) SetDamageFormulaFile(path string) {
	w.dmgf.mu.Lock()
	w.dmgf.path = strings.TrimSpace(path)
	w.dmgf.modTime, w.dmgf.size, w.dmgf.missing = time.Time{}, 0, false
	w.dmgf.mu.Unlock()
	if w.dmgf.path == "" {
		w.dmgf.set.Store(nil)
		return
	}
	if _, err := w.ReloadDamageFormulas(); err != nil {
		log.Printf("Damage formulas: %v; the built-in figures are used", err)
	}
}

// ReloadDamageFormulas reads the formula file again (the GM panel's
// reload_formulas, and the watcher at a change). It returns the names of
// the formulas now in use. A file that does not parse is refused whole and
// the formulas in use stay; a file that is gone leaves the built-in figures.
// Neither the error nor the log carries a formula's text.
func (w *World) ReloadDamageFormulas() ([]string, error) {
	w.dmgf.mu.Lock()
	defer w.dmgf.mu.Unlock()
	return w.loadDamageFormulasLocked()
}

// errNoFormulaFile is a reload with no combat.formula_file set.
var errNoFormulaFile = errors.New("no combat.formula_file is set")

func (w *World) loadDamageFormulasLocked() ([]string, error) {
	st := &w.dmgf
	if st.path == "" {
		return nil, errNoFormulaFile
	}
	fi, err := os.Stat(st.path)
	if err != nil {
		st.modTime, st.size = time.Time{}, 0
		if st.set.Swap(nil) != nil || !st.missing {
			log.Printf("Damage formulas: the file cannot be read (%v); the built-in figures are used", errors.Unwrap(err))
		}
		st.missing = true
		return nil, fmt.Errorf("the formula file cannot be read: %v", errors.Unwrap(err))
	}
	st.missing = false
	st.modTime, st.size = fi.ModTime(), fi.Size()
	if fi.Size() > formula.MaxFileSize {
		log.Printf("Damage formulas: the file is larger than %d bytes, refused; the formulas in use stay", formula.MaxFileSize)
		return nil, fmt.Errorf("the formula file is larger than %d bytes", formula.MaxFileSize)
	}
	data, err := os.ReadFile(st.path)
	if err != nil {
		return nil, fmt.Errorf("the formula file cannot be read: %v", errors.Unwrap(err))
	}
	set, errs := compileDamageFormulas(data)
	if len(errs) > 0 {
		for _, e := range errs {
			log.Printf("Damage formulas: %v", e)
		}
		log.Printf("Damage formulas: the file has %d mistake(s), refused; the formulas in use stay", len(errs))
		return nil, fmt.Errorf("the formula file has %d mistake(s), refused (the first: %v); the formulas in use stay", len(errs), errs[0])
	}
	st.set.Store(set)
	log.Printf("Damage formulas loaded: %d (%s), %d table(s)", len(set.names), strings.Join(set.names, ", "), set.tables)
	return set.names, nil
}

// compileDamageFormulas compiles a file's text into a set.
func compileDamageFormulas(data []byte) (*dmgFormulaSet, []error) {
	progs, tables, errs := formula.ParseFile(data, knownDamageFormula, dmgVarIndex)
	if len(errs) > 0 {
		return nil, errs
	}
	set := &dmgFormulaSet{names: formula.Names(progs), tables: tables}
	byName := map[string]*dmgEntry{}
	for name, p := range progs {
		if p.Slots() > len(dmgVars)+formula.MaxSteps {
			return nil, []error{fmt.Errorf("%s: more slots than a run has", name)}
		}
		byName[name] = &dmgEntry{name: name, prog: p}
	}
	for s := dmgSource(0); s < nDmgSources; s++ {
		for k := dkMelee; k < nDmgKinds; k++ {
			if !dmgPathExists(s, k) {
				continue
			}
			if e := byName[dmgPathName(s, k)]; e != nil {
				set.paths[s][k] = e
			} else {
				set.paths[s][k] = byName[dmgSourceNames[s]] // nil when neither
			}
		}
	}
	return set, nil
}

// damageFormulaChanged is whether the file's modification time or size is
// not what the last load saw (or it came or went).
func (w *World) damageFormulaChanged() bool {
	st := &w.dmgf
	st.mu.Lock()
	defer st.mu.Unlock()
	if st.path == "" {
		return false
	}
	fi, err := os.Stat(st.path)
	if err != nil {
		return !st.missing
	}
	return st.missing || !fi.ModTime().Equal(st.modTime) || fi.Size() != st.size
}

// DamageFormulaWatchEvery is how often the watcher looks at the file.
const DamageFormulaWatchEvery = time.Second

// WatchDamageFormulas reloads the formula file when it changes (its
// modification time or size), looking every DamageFormulaWatchEvery until
// stop closes. A stat a second, off the tick: the tick never touches the
// file.
func (w *World) WatchDamageFormulas(stop <-chan struct{}) {
	t := time.NewTicker(DamageFormulaWatchEvery)
	defer t.Stop()
	for {
		select {
		case <-stop:
			return
		case <-t.C:
			if w.damageFormulaChanged() {
				if _, err := w.ReloadDamageFormulas(); err != nil && !errors.Is(err, errNoFormulaFile) {
					log.Printf("Damage formulas: reload at a change: %v", err)
				}
			}
		}
	}
}

// DamageFormulaStatus is what the GM panel may show: the names in use, never
// a formula's text.
func (w *World) DamageFormulaStatus() (configured bool, names []string) {
	w.dmgf.mu.Lock()
	configured = w.dmgf.path != ""
	w.dmgf.mu.Unlock()
	if set := w.dmgf.set.Load(); set != nil {
		names = append(names, set.names...)
	}
	return configured, names
}
