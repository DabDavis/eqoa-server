package entity

// The formula mode and the shadow log (combat.formula_mode,
// combat.formula_shadow_log; docs/damage-formulas.md).
//
// The owner, 2026-11-14: "lets do shadow mode on heirloom". A loaded formula
// file does one of three things, the mode's:
//
//   - off: nothing. The built-in figure lands everywhere (the file is still
//     read, so reload_formulas names what it holds).
//   - live: a formula's result replaces the built-in figure of its path.
//   - shadow: a formula is run on every hit of its path as live would run it,
//     and its figure is NOT applied: the built-in lands, unchanged, and the
//     pair (built-in, formula) is recorded with the hit's inputs, one JSON line
//     a hit, in a private file of the operator's (combat.formula_shadow_log)
//     outside the repository and outside the server's log. Every path a
//     formula covers is shadowed alike (every formulaDamage call: a player's,
//     a creature's, a pet's, a merc's and PvP's melee, spells, DoTs, shields,
//     procs and specials), so a file defining npc_melee alone shadows the
//     creatures' melee on players alone.
//
// The shadow log costs the tick no I/O: a hit's record is a small value
// handed to a buffered channel (dropped, and counted, when it is full) and a
// goroutine of its own encodes, writes (buffered, flushed every second) and
// rotates the file by size (to <file>.1, the one before it dropped). A sample
// rate below 1 records that share of the hits, the others not run at all. The
// formula's draws in shadow come from the same source as live's, and the
// built-in figure is made before the formula runs, so shadow changes no
// figure that lands. The file is created mode 600, its directory 700.
//
// What a record holds: when, the path's and the formula's names, the
// built-in figure (base: what landed), the formula's, raw, what a mitigation
// took off before (a creature's melee), how many draws the formula made, a
// run's failure (part and kind, as the log's; base landed), the inputs every
// record carries (shadowInputs) and every other input the formula reads, and
// the value of every step whose name begins with "shadow_" (a number or a
// string; the formula file's own way to record which of its records a hit
// matched). Never the formula's text.

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"math"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/eqoa/server/internal/formula"
)

// FormulaMode is what a loaded formula does. Its zero value is live, a World
// as build194 made it (and the tests'); the server sets it from the config
// and the shard's database (internal/formulamode).
type FormulaMode int32

const (
	FormulaLive   FormulaMode = iota // a formula's result replaces the built-in figure
	FormulaOff                       // the built-in figure everywhere
	FormulaShadow                    // the built-in lands; the formula's is recorded beside it
)

// String is the mode's name as the config and the commands say it.
func (m FormulaMode) String() string {
	switch m {
	case FormulaLive:
		return "live"
	case FormulaShadow:
		return "shadow"
	}
	return "off"
}

// ParseFormulaMode reads combat.formula_mode: "" or "off" (built-in), "live"
// or "shadow".
func ParseFormulaMode(s string) (FormulaMode, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "", "off", "default", "builtin", "built-in":
		return FormulaOff, nil
	case "live":
		return FormulaLive, nil
	case "shadow":
		return FormulaShadow, nil
	}
	return FormulaOff, fmt.Errorf("combat.formula_mode %q: off, live or shadow", s)
}

// SetDamageFormulaMode sets what a loaded formula does, at once.
func (w *World) SetDamageFormulaMode(m FormulaMode) {
	if old := FormulaMode(w.dmgf.mode.Swap(int32(m))); old != m {
		log.Printf("Damage formulas: mode %s (was %s)", m, old)
	}
}

// DamageFormulaMode is what a loaded formula does now.
func (w *World) DamageFormulaMode() FormulaMode { return FormulaMode(w.dmgf.mode.Load()) }

// ShadowLogConfig is the shadow log's settings.
type ShadowLogConfig struct {
	Path     string  // the file; "" for none (shadow mode then records nothing)
	Sample   float64 // the share of hits recorded, (0, 1]; 0: all
	MaxBytes int64   // the size at which it rotates; 0: DefaultShadowMaxBytes
}

// DefaultShadowMaxBytes is the shadow log's size at which it rotates when the
// config names none.
const DefaultShadowMaxBytes = 64 << 20

// shadowQueue is how many records wait for the writer at most; beyond it a
// record is dropped (and counted), never waited for.
const shadowQueue = 8192

// shadowFlushEvery is how often the writer flushes its buffer.
const shadowFlushEvery = time.Second

// dmgShadow is the shadow log's writer.
type dmgShadow struct {
	cfg     ShadowLogConfig
	ch      chan *shadowRec
	dropped atomic.Int64
	written atomic.Int64
	done    chan struct{}
	stop    chan struct{}

	f    *os.File
	bw   *bufio.Writer
	size int64
}

// SetDamageFormulaShadow names the shadow log (combat.formula_shadow_log) and
// starts its writer; a writer before it is closed first (its buffer
// flushed). An empty path stops it.
func (w *World) SetDamageFormulaShadow(cfg ShadowLogConfig) error {
	if old := w.dmgf.shadow.Swap(nil); old != nil {
		old.close()
	}
	cfg.Path = strings.TrimSpace(cfg.Path)
	if cfg.Path == "" {
		return nil
	}
	if cfg.Sample <= 0 || cfg.Sample > 1 {
		cfg.Sample = 1
	}
	if cfg.MaxBytes <= 0 {
		cfg.MaxBytes = DefaultShadowMaxBytes
	}
	sh := &dmgShadow{cfg: cfg, ch: make(chan *shadowRec, shadowQueue), done: make(chan struct{}), stop: make(chan struct{})}
	if err := sh.open(); err != nil {
		return err
	}
	go sh.run()
	w.dmgf.shadow.Store(sh)
	log.Printf("Damage formulas: shadow log configured (sample %.3g, rotated at %d MB)", cfg.Sample, cfg.MaxBytes>>20)
	return nil
}

// DamageFormulaShadowLog is the shadow log's file ("" for none), and how
// many records it has written and dropped since it was set.
func (w *World) DamageFormulaShadowLog() (path string, written, dropped int64) {
	sh := w.dmgf.shadow.Load()
	if sh == nil {
		return "", 0, 0
	}
	return sh.cfg.Path, sh.written.Load(), sh.dropped.Load()
}

// FlushDamageFormulaShadow writes every record handed to the shadow log so
// far and flushes it (a test's, and the shutdown's): it closes the writer and
// starts it again.
func (w *World) FlushDamageFormulaShadow() error {
	sh := w.dmgf.shadow.Load()
	if sh == nil {
		return nil
	}
	return w.SetDamageFormulaShadow(sh.cfg)
}

// open opens (creating) the file, mode 600, in a directory of mode 700.
func (sh *dmgShadow) open() error {
	dir := filepath.Dir(sh.cfg.Path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("the shadow log's directory: %w", err)
	}
	if fi, err := os.Stat(dir); err == nil && fi.Mode().Perm()&0o077 != 0 {
		if err := os.Chmod(dir, 0o700); err != nil {
			return fmt.Errorf("the shadow log's directory is open to others and cannot be closed: %w", err)
		}
	}
	f, err := os.OpenFile(sh.cfg.Path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return fmt.Errorf("the shadow log: %w", err)
	}
	if err := f.Chmod(0o600); err != nil {
		f.Close()
		return fmt.Errorf("the shadow log: %w", err)
	}
	fi, err := f.Stat()
	if err != nil {
		f.Close()
		return fmt.Errorf("the shadow log: %w", err)
	}
	sh.f, sh.bw, sh.size = f, bufio.NewWriterSize(f, 64<<10), fi.Size()
	return nil
}

// run is the writer: a record a line, flushed every shadowFlushEvery, until
// close.
func (sh *dmgShadow) run() {
	defer close(sh.done)
	t := time.NewTicker(shadowFlushEvery)
	defer t.Stop()
	var failLogged time.Time
	fail := func(err error) {
		if time.Since(failLogged) > time.Minute {
			log.Printf("Damage formulas: the shadow log cannot be written: %v", err)
			failLogged = time.Now()
		}
	}
	for {
		select {
		case r := <-sh.ch:
			if err := sh.write(r); err != nil {
				fail(err)
			}
		case <-t.C:
			if err := sh.bw.Flush(); err != nil {
				fail(err)
			}
		case <-sh.stop:
			for {
				select {
				case r := <-sh.ch:
					sh.write(r)
				default:
					sh.bw.Flush()
					sh.f.Close()
					return
				}
			}
		}
	}
}

func (sh *dmgShadow) close() {
	close(sh.stop)
	<-sh.done
}

// write writes a record, rotating the file first when it would pass its size.
func (sh *dmgShadow) write(r *shadowRec) error {
	line, err := json.Marshal(r.out())
	if err != nil {
		return err
	}
	line = append(line, '\n')
	if sh.size > 0 && sh.size+int64(len(line)) > sh.cfg.MaxBytes {
		if err := sh.rotate(); err != nil {
			return err
		}
	}
	n, err := sh.bw.Write(line)
	sh.size += int64(n)
	if err == nil {
		sh.written.Add(1)
	}
	return err
}

// rotate moves the file to <file>.1 (the one there before dropped) and
// starts a new one.
func (sh *dmgShadow) rotate() error {
	sh.bw.Flush()
	sh.f.Close()
	if err := os.Rename(sh.cfg.Path, sh.cfg.Path+".1"); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return sh.open()
}

// shadowInputs are the inputs every record carries, whatever its formula
// reads (each read off the hit only for a record).
var shadowInputs = []string{
	"a_kind", "a_name", "a_family", "a_race", "a_class_name", "a_level", "a_offmod", "a_defmod",
	"d_kind", "d_name", "d_family", "d_race", "d_class_name", "d_level", "d_offmod", "d_defmod",
	"d_ac", "d_agi", "a_str", "a_dex",
	"weapon_dmg", "weapon_delay", "two_handed", "ranged", "duel", "by_pet",
	"spell_id", "spell_level", "resist", "con",
}

var shadowInputSlots = func() []int {
	out := make([]int, 0, len(shadowInputs))
	for _, n := range shadowInputs {
		i, ok := dmgVarIndex[n]
		if !ok {
			panic("shadowInputs: no input " + n)
		}
		out = append(out, i)
	}
	return out
}()

// ShadowTagPrefix begins the name of a formula's step that the shadow log
// records.
const ShadowTagPrefix = "shadow_"

// shadowVal is a value as a record keeps it.
type shadowVal struct {
	name string
	v    formula.Value
}

// shadowRec is one hit's record.
type shadowRec struct {
	at                         time.Time
	path, entry                string
	builtin, formula, raw, mit int
	draws                      int
	fail                       string
	weaponType                 int
	dResist                    int
	in, tags                   []shadowVal
}

// ShadowRecord is a record's line in the shadow log.
type ShadowRecord struct {
	At        string         `json:"t"`
	Path      string         `json:"path"`           // the hit's path: npc_melee ...
	Formula   string         `json:"formula"`        // the formula that ran (the path's, or its source's)
	Builtin   int            `json:"builtin"`        // the built-in figure: what landed
	Value     int            `json:"value"`          // the formula's figure (builtin when it failed)
	Raw       int            `json:"raw"`            // the path's figure before the balance's shares
	Mitigated int            `json:"mitigated"`      // what a mitigation took off before the built-in figure
	Draws     int            `json:"draws"`          // the random draws the formula made
	Fail      string         `json:"fail,omitempty"` // the run's failure (part and kind)
	WeapType  int            `json:"weapon_type"`    // the attacker's weapon's attack type (0 for none)
	DResist   int            `json:"d_resist"`       // the defender's resist the hit is against (0 for none)
	In        map[string]any `json:"in"`             // the inputs
	Tags      map[string]any `json:"tags,omitempty"` // the formula's shadow_ steps
}

func valOut(v formula.Value) any {
	if v.K == formula.Str {
		return v.S
	}
	if math.IsNaN(v.N) || math.IsInf(v.N, 0) {
		return nil
	}
	return v.N
}

func (r *shadowRec) out() ShadowRecord {
	o := ShadowRecord{At: r.at.UTC().Format(time.RFC3339Nano), Path: r.path, Formula: r.entry, Builtin: r.builtin,
		Value: r.formula, Raw: r.raw, Mitigated: r.mit, Draws: r.draws, Fail: r.fail, WeapType: r.weaponType,
		DResist: r.dResist, In: make(map[string]any, len(r.in))}
	for _, v := range r.in {
		o.In[v.name] = valOut(v.v)
	}
	if len(r.tags) > 0 {
		o.Tags = make(map[string]any, len(r.tags))
		for _, v := range r.tags {
			o.Tags[strings.TrimPrefix(v.name, ShadowTagPrefix)] = valOut(v.v)
		}
	}
	return o
}

// countRand counts a run's draws.
type countRand struct {
	r formula.Rand
	n int
}

func (c *countRand) Uniform() float64            { c.n++; return c.r.Uniform() }
func (c *countRand) Normal(m, s float64) float64 { c.n++; return c.r.Normal(m, s) }
func (c *countRand) Beta(a, b float64) float64   { c.n++; return c.r.Beta(a, b) }

// shadowSample is the shadow log's sampling draw (its own source: the game's
// draws are not taken from).
var shadowSample = struct {
	sync.Mutex
	r *rand.Rand
}{r: rand.New(rand.NewSource(time.Now().UnixNano()))}

func (sh *dmgShadow) sampled() bool {
	if sh.cfg.Sample >= 1 {
		return true
	}
	shadowSample.Lock()
	defer shadowSample.Unlock()
	return shadowSample.r.Float64() < sh.cfg.Sample
}

// shadowDamage runs e on the hit and hands the pair to the shadow log; it
// lands nothing.
func (w *World) shadowDamage(e *dmgEntry, base int, in *dmgIn) {
	sh := w.dmgf.shadow.Load()
	if sh == nil || !sh.sampled() {
		return
	}
	r := dmgScratch.Get().(*dmgRun)
	defer dmgScratch.Put(r)
	cr := &countRand{r: formulaRand}
	x, err := e.runOn(r, base, in, cr)
	rec := &shadowRec{at: time.Now(), path: dmgPathName(in.src, in.kind), entry: e.name, builtin: base, formula: x,
		raw: in.raw, mit: in.mitigated, draws: cr.n}
	if err != nil {
		rec.fail = err.Error()
	}
	if in.ap != nil {
		rec.weaponType = in.ap.Gear.WeaponType
	}
	if in.resist != 0 {
		rec.dResist = sideStat(in.dp, in.dn, in.resist)
	}
	read := make(map[int]bool, len(e.prog.Uses()))
	rec.in = make([]shadowVal, 0, len(shadowInputSlots)+len(e.prog.Uses()))
	for _, i := range e.prog.Uses() {
		read[i] = true
		rec.in = append(rec.in, shadowVal{dmgVars[i].name, r.v[i]})
	}
	for _, i := range shadowInputSlots {
		if !read[i] {
			rec.in = append(rec.in, shadowVal{dmgVars[i].name, dmgVars[i].get(&r.in)})
		}
	}
	if err == nil {
		for i, n := range e.prog.StepNames() {
			if v := r.v[e.prog.StepSlot(i)]; strings.HasPrefix(n, ShadowTagPrefix) && (v.K == formula.Num || v.K == formula.Str) {
				rec.tags = append(rec.tags, shadowVal{n, v})
			}
		}
	}
	select {
	case sh.ch <- rec:
	default:
		sh.dropped.Add(1)
	}
}
