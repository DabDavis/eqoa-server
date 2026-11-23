package entity

import (
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/eqoa/server/internal/formula"
	"github.com/eqoa/server/internal/session"
)

// Fallback hits (a formula that fails, the built-in figure standing) and
// shadow runs from other goroutines take no draw from the game's source
// either: the game's seeded sequence is the same with shadow off and on.
func TestShadowRNGFallbacksAndConcurrentCalls(t *testing.T) {
	r := newTemplateRig(t)
	guard := r.npc("Gnoll Guard")
	guard.Name = "Toy Golem"
	p, sess := r.player("Hero", guard.X+1, guard.Z)
	// npc_melee always fails after its draw (negative); player_melee draws and stands
	writeFormulas(t, r.w, "formulas:\n  npc_melee: 'uniform() - 5'\n  player_melee: 'base + uniform()'\n  player_spell: 'normal(base, 2) + beta(2, 2)'\n")
	seq := func(shadow bool) []int {
		gameDice(t, 99)
		var wg sync.WaitGroup
		start := make(chan struct{})
		if shadow {
			for range 4 {
				wg.Add(1)
				go func() {
					defer wg.Done()
					a, d := &Player{Name: "Other", Level: 10}, &NPC{Name: "Toy Rat", Level: 9}
					<-start
					// Exercise concurrent draws without filling the intentionally
					// lossy log queue whose combat records this test counts.
					for range 30 {
						r.w.formulaDamage(20, dmgIn{src: srcPlayer, kind: dkSpell, ap: a, dn: d})
					}
				}()
			}
		}
		close(start)
		var out []int
		for range 30 {
			p.HP, p.MaxHP = 100_000, 100_000
			guard.HP, guard.MaxHP, guard.Dead = 1_000_000, 1_000_000, false
			r.w.mu.Lock()
			r.w.npcAttackPlayer(guard, p.ServerID, time.Now())
			r.w.playerHitLocked(sess, p, guard, r.objectID(guard.ServerID))
			out = append(out, 100_000-p.HP, 1_000_000-guard.HP, meleeIntn(1_000_000))
			r.w.mu.Unlock()
		}
		wg.Wait()
		return out
	}
	r.w.SetDamageFormulaMode(FormulaOff)
	off := seq(false)
	path := shadowOn(t, r.w, 0)
	on := seq(true)
	for i := range off {
		if off[i] != on[i] {
			t.Fatalf("roll %d: %d with shadow off, %d on", i, off[i], on[i])
		}
	}
	fallbacks := 0
	for _, rec := range readShadow(t, r.w, path) {
		if rec.Path == "npc_melee" {
			if rec.Fallback != "negative" || rec.Value != rec.BuiltinPre || rec.Draws != 1 {
				t.Fatalf("a fallback's record: %+v", rec)
			}
			fallbacks++
		}
	}
	if fallbacks != 30 {
		t.Errorf("%d fallback records, want 30", fallbacks)
	}
}

// npc_melee in live mode stands before the mitigation: the formula's figure
// takes the built-in's place and the mitigation's whole lifecycle runs on it
// once (its share off, a capped one's pool spent, broken when spent). In
// shadow the built-in lands and spends, and the formula's figure is previewed
// through the mitigation as it stood before the hit, spending nothing.
func TestNPCMeleeFormulaBeforeMitigation(t *testing.T) {
	r := newTemplateRig(t)
	guard := r.npc("Gnoll Guard")
	guard.Name = "Toy Golem"
	p, _ := r.player("Hero", guard.X+1, guard.Z)
	fixDice(t)
	swing := func() int {
		p.HP, p.MaxHP = 100_000, 100_000
		r.w.mu.Lock()
		defer r.w.mu.Unlock()
		r.w.npcAttackPlayer(guard, p.ServerID, time.Now())
		return 100_000 - p.HP
	}
	pre := swing() // no file, no mitigation: the built-in figure before any
	if pre < 4 {
		t.Fatalf("the built-in swing is %d", pre)
	}
	ward := func(pct, left int, capped bool) *ActiveEffect {
		e := &ActiveEffect{SpellID: 9902, Name: "Toy Ward", Until: time.Now().Add(time.Hour),
			Rules: EffectRules{MitigatePct: pct, MitigateLeft: left, MitigateCapped: capped}}
		p.Effects = []*ActiveEffect{e}
		return e
	}
	off := func(n, pct, left int, capped bool) int { // what the mitigation takes off n
		return n - (mitSnap{on: true, pct: pct, capped: capped, left: left}).take(n)
	}
	type tc struct {
		name, formula string
		pct, left     int
		capped        bool
	}
	cases := []tc{
		{"capped", "base * 3", 50, 1_000_000, true},
		{"partial", "base * 3", 30, 0, false},
		{"exhausted", "base * 3", 50, 2, true},
		{"invulnerable", "base * 3", 100, 0, false},
		{"zero", "0", 50, 1_000, true},
		{"none", "base * 3", 0, 0, false},
	}
	for _, c := range cases {
		formulaFig := 3 * pre
		if c.formula == "0" {
			formulaFig = 0
		}
		writeFormulas(t, r.w, "formulas:\n  npc_melee: '"+c.formula+"'\n")

		// live
		r.w.SetDamageFormulaMode(FormulaLive)
		e := ward(c.pct, c.left, c.capped)
		if c.pct == 0 {
			p.Effects = nil
		}
		took := 0
		if c.pct > 0 {
			took = off(formulaFig, c.pct, c.left, c.capped)
		}
		if got := swing(); got != formulaFig-took {
			t.Errorf("%s, live: landed %d, want %d", c.name, got, formulaFig-took)
		}
		if c.capped && e.Rules.MitigateLeft != c.left-took {
			t.Errorf("%s, live: %d left, want %d", c.name, e.Rules.MitigateLeft, c.left-took)
		}
		if c.name == "exhausted" && p.hasEffect(9902) {
			t.Errorf("%s, live: the spent ward stayed", c.name)
		}

		// shadow
		path := shadowOn(t, r.w, 0)
		e = ward(c.pct, c.left, c.capped)
		if c.pct == 0 {
			p.Effects = nil
		}
		builtOff := 0
		if c.pct > 0 {
			builtOff = off(pre, c.pct, c.left, c.capped)
		}
		if got := swing(); got != pre-builtOff {
			t.Errorf("%s, shadow: landed %d, want the built-in %d", c.name, got, pre-builtOff)
		}
		if c.capped && e.Rules.MitigateLeft != c.left-builtOff {
			t.Errorf("%s, shadow: %d left, want %d (the built-in's spend alone)", c.name, e.Rules.MitigateLeft, c.left-builtOff)
		}
		rec := readShadow(t, r.w, path)[0]
		wantPrev := formulaFig
		if c.pct > 0 {
			wantPrev -= off(formulaFig, c.pct, c.left, c.capped) // the pre-hit state: c.left, not what the built-in left
		}
		if rec.BuiltinBefore != pre || rec.BuiltinAfter != pre-builtOff || rec.Mitigated != builtOff || rec.FormulaAt != "before_effects" ||
			rec.FormulaBefore == nil || *rec.FormulaBefore != formulaFig || rec.FormulaAfter != wantPrev || rec.Preview != wantPrev {
			t.Errorf("%s, shadow: %+v (want before %d after %d, formula %d -> %d)", c.name, rec, pre, pre-builtOff, formulaFig, wantPrev)
		}
		r.w.SetDamageFormulaShadow(ShadowLogConfig{})
	}

	// successive hits: each preview from the pool as that hit found it
	writeFormulas(t, r.w, "formulas:\n  npc_melee: 'base * 3'\n")
	path := shadowOn(t, r.w, 0)
	left := 2*pre + 1
	e := ward(50, left, true)
	for i := range 4 {
		swing()
		recs := readShadow(t, r.w, path)
		rec := recs[len(recs)-1]
		wantPrev := 3*pre - off(3*pre, 50, left, true)
		builtOff := off(pre, 50, left, true)
		if rec.FormulaAfter != wantPrev || rec.Mitigated != builtOff {
			t.Errorf("hit %d: preview %d mitigated %d, want %d %d (left %d)", i, rec.FormulaAfter, rec.Mitigated, wantPrev, builtOff, left)
		}
		left -= builtOff
		if left > 0 && e.Rules.MitigateLeft != left {
			t.Errorf("hit %d: %d left, want %d", i, e.Rules.MitigateLeft, left)
		}
		if left <= 0 {
			break
		}
	}
}

// The line's own fields: the file's hash and version, the sample and its
// window, the attack's kind; the summary by a formula's tag.
func TestShadowLineFileAndTagSummary(t *testing.T) {
	w := NewWorld()
	fixDice(t)
	writeFormulas(t, w, `version: toy-3
options:
  ranged: swing
formulas:
  npc_melee:
    steps:
      - shadow_band: 'a_level > 20 ? "high" : "low"'
    result: 'base * 2'
`)
	path := shadowOn(t, w, 0)
	for _, lvl := range []int{10, 30, 30} {
		w.formulaDamage(100, dmgIn{src: srcNPC, kind: dkMelee, an: &NPC{Name: "Toy Golem", Level: lvl}, dp: &Player{Name: "Hero", Level: 30}})
	}
	recs := readShadow(t, w, path)
	r := recs[0]
	if len(r.FileHash) != 16 || r.FileVersion != "toy-3" || r.Sample != 1 || r.Since == "" || r.Attack != "melee" || r.Arrow != 0 {
		t.Errorf("the line: %+v", r)
	}
	s, err := SummarizeShadowLog(path, ShadowSummaryOptions{Tag: "band"})
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]int{}
	for _, g := range s.ByTag {
		got[g.Key] = g.Hits
	}
	if got["npc_melee / band=high"] != 2 || got["npc_melee / band=low"] != 1 {
		t.Errorf("by tag: %v", got)
	}
	// an option the server does not know, or a word it does not take, refuses the file
	for _, bad := range []string{"options:\n  ranged: sometimes\nformulas:\n  npc: base\n", "options:\n  colour: red\nformulas:\n  npc: base\n",
		"version: [1, 2]\nformulas:\n  npc: base\n"} {
		if _, errs := compileDamageFormulas([]byte(bad)); len(errs) == 0 || strings.Contains(errs[0].Error(), "sometimes") || strings.Contains(errs[0].Error(), "red") {
			t.Errorf("%q: %v", bad, errs)
		}
	}
}

// The masteries' OFF and DEF: the points the ranks give (each rank its
// catalog row's offmod/defmod, summed) and how many ranks give any, with
// none, one and several ranks bought; apart from the effects' and the level
// estimate's.
func TestDamageFormulaMasteryOffDef(t *testing.T) {
	cat := &cmCatalog{byID: map[int]*CMRank{}}
	add := func(id, off, def int) {
		r := &CMRank{ID: id}
		r.Effects[cmSlotOff], r.Effects[cmSlotDef] = off, def
		cat.byID[id] = r
	}
	add(1, 2, 0)
	add(2, 3, 1)
	add(3, 0, 4)
	w := NewWorld()
	fixDice(t)
	writeFormulas(t, w, "formulas:\n  player_melee: 'a_cm_offmod * 1000 + a_cm_off_ranks * 100 + a_cm_defmod * 10 + a_cm_def_ranks'\n")
	for _, c := range []struct {
		owned []int
		want  int
	}{
		{nil, 0},
		{[]int{1}, 2100},
		{[]int{1, 2}, 5211},
		{[]int{1, 2, 3}, 5252},
	} {
		p := &Player{Name: "Hero", Level: 30, CMs: map[int]int{}}
		p.Mods.OffMod = 9 // an effect's: not the masteries'
		for _, id := range c.owned {
			p.CMs[id] = 1
		}
		p.refreshCMLocked(cat)
		if got := w.formulaDamage(1, dmgIn{src: srcPlayer, kind: dkMelee, ap: p, dn: &NPC{Name: "Toy Rat"}}); got != c.want {
			t.Errorf("ranks %v: %d, want %d", c.owned, got, c.want)
		}
	}
}

// shotRig is a ranger with a melee weapon in the hand (damage 25, slash)
// and a bow in the ranged slot (damage 60): the two deliberately different,
// so a formula reading ranged_weapon_dmg tells them apart.
type shotRig struct {
	r     *templateRig
	guard *NPC
	p     *Player
	sess  *session.Session
	shoot func(kind int) shotOut
}

type shotOut struct {
	dealt, rolls, power, threat int
	killed                      bool
}

func newShotRig(t *testing.T) *shotRig {
	r := newTemplateRig(t)
	guard := r.npc("Gnoll Guard")
	guard.Name = "Toy Golem"
	p, sess := r.player("Hero", guard.X+1, guard.Z)
	p.Class, p.Level = classRanger, 30
	p.Gear.WeaponDamage, p.Gear.WeaponSlot, p.Gear.WeaponType, p.PrimaryType = 25, 14, 1, 1
	p.Gear.RangedDamage, p.Gear.RangedType = 60, 7
	sr := &shotRig{r: r, guard: guard, p: p, sess: sess}
	sr.shoot = func(kind int) shotOut {
		rolls := 0
		oldIntn := meleeIntn
		meleeIntn = func(n int) int { rolls++; return n / 3 }
		defer func() { meleeIntn = oldIntn }()
		p.Swing.Kind = kind
		p.Power, p.MaxPower = 10_000, 10_000
		guard.HP, guard.MaxHP, guard.Dead, guard.Threats = 1_000_000, 1_000_000, false, nil
		r.w.mu.Lock()
		defer r.w.mu.Unlock()
		killed := r.w.playerHitLocked(sess, p, guard, r.objectID(guard.ServerID))
		return shotOut{1_000_000 - guard.HP, rolls, 10_000 - p.Power, int(guard.Threats[p.ServerID]), killed}
	}
	return sr
}

// An older file (no options) runs once on the whole shot, as before: a
// constant, or a figure not linear in base, lands the same on one- and
// two-arrow shots as it did when the formula ran once per swing.
func TestRangedOlderFileRunsOncePerShot(t *testing.T) {
	sr := newShotRig(t)
	fixDice(t)
	built1, built2 := sr.shoot(swingRange1), sr.shoot(swingRange4)
	if built2.dealt <= built1.dealt || built2.rolls != 2*built1.rolls {
		t.Fatalf("the built-in shots: %+v %+v", built1, built2)
	}
	writeFormulas(t, sr.r.w, "formulas:\n  player_melee: 17\n")
	for _, k := range []int{swingRange1, swingRange4} {
		if got := sr.shoot(k); got.dealt != 17 {
			t.Errorf("a constant 17, swing kind %d: dealt %d", k, got.dealt)
		}
	}
	writeFormulas(t, sr.r.w, "formulas:\n  player_melee: 'min(base, 5) * ranged_weapon_dmg + projectiles + arrow * 1000'\n")
	// once on the whole shot: min of the shot's figure, not a sum of each arrow's
	if got, want := sr.shoot(swingRange1), min(built1.dealt, 5)*60+1; got.dealt != want {
		t.Errorf("non-linear, one arrow: %d, want %d", got.dealt, want)
	}
	if got, want := sr.shoot(swingRange4), min(built2.dealt, 5)*60+2; got.dealt != want {
		t.Errorf("non-linear, two arrows: %d, want %d", got.dealt, want)
	}
}

// A bow's shot under ranged: arrow: two arrows, two formula runs, each on its
// share of the built-in figure with its own roll, reading the bow's damage
// (not the hand's), summed into the swing's one hit; the rolls, the shot's
// power and the threat as the built-in's otherwise. swing: one run; builtin:
// none.
func TestRangedShotPerArrow(t *testing.T) {
	sr := newShotRig(t)
	fixDice(t)
	built1, built2 := sr.shoot(swingRange1), sr.shoot(swingRange4)

	writeFormulas(t, sr.r.w, "options:\n  ranged: arrow\nformulas:\n  player_melee: 'base + arrow * 1000 + projectiles * 100000 + ranged_weapon_dmg * 10 + uniform() * 0'\n")
	f := fixDice(t, 0.5, 0.5, 0.5)
	live2 := sr.shoot(swingRange4)
	if f.Next != 2 {
		t.Errorf("two arrows ran %d formula runs, want 2", f.Next)
	}
	if want := built2.dealt + 1000 + 2000 + 2*200_000 + 2*600; live2.dealt != want {
		t.Errorf("live, two arrows: dealt %d, want %d", live2.dealt, want)
	}
	if live2.rolls != built2.rolls || live2.power != built2.power || live2.threat <= built2.threat {
		t.Errorf("live, two arrows: %+v against the built-in's %+v", live2, built2)
	}
	f = fixDice(t, 0.5)
	if live1 := sr.shoot(swingRange1); live1.dealt != built1.dealt+1000+100_000+600 || f.Next != 1 || live1.rolls != built1.rolls {
		t.Errorf("live, one arrow: %+v (%d runs), want dealt %d", live1, f.Next, built1.dealt+1000+100_000+600)
	}

	// shadow: two records, the arrows' shares summing to the built-in, the built-in landing
	path := shadowOn(t, sr.r.w, 0)
	game := fixDice(t)
	sf := fixShadowDice(t, 0.1, 0.9)
	if sh := sr.shoot(swingRange4); sh != built2 {
		t.Errorf("shadow landed %+v, the built-in %+v", sh, built2)
	}
	recs := readShadow(t, sr.r.w, path)
	if len(recs) != 2 || recs[0].Arrow != 1 || recs[1].Arrow != 2 || recs[0].Arrows != 2 || recs[0].Attack != "ranged" ||
		recs[0].Builtin+recs[1].Builtin != built2.dealt || sf.Next != 2 || game.Next != 0 || recs[0].WeapType != 7 ||
		recs[0].In["ranged_weapon_dmg"] != 60.0 || recs[0].In["weapon_dmg"] != 25.0 {
		t.Errorf("shadow's records: %+v (shadow draws %d, game draws %d)", recs, sf.Next, game.Next)
	}
	if recs[0].Raw <= 0 || recs[1].Raw <= 0 || recs[0].Value != recs[0].Builtin+1000+200_000+600 {
		t.Errorf("an arrow's own roll and run: %+v", recs[0])
	}

	// swing: once on the whole shot; builtin: the built-in figure, no run
	sr.r.w.SetDamageFormulaMode(FormulaLive)
	writeFormulas(t, sr.r.w, "options:\n  ranged: swing\nformulas:\n  player_melee: 'base + arrow * 1000 + ranged_weapon_dmg'\n")
	if got := sr.shoot(swingRange4); got.dealt != built2.dealt+60 {
		t.Errorf("swing: dealt %d, want %d", got.dealt, built2.dealt+60)
	}
	writeFormulas(t, sr.r.w, "options:\n  ranged: builtin\nformulas:\n  player_melee: 'base + ranged_weapon_dmg'\n")
	if got := sr.shoot(swingRange4); got != built2 {
		t.Errorf("builtin: %+v, want %+v", got, built2)
	}
}

// A two-arrow shot that kills only by the two arrows' figures summed: through
// playerHitLocked, one kill (one death, one award of experience: what a
// built-in kill gives), as a built-in shot's kill.
func TestRangedTwoArrowKill(t *testing.T) {
	sr := newShotRig(t)
	fixDice(t)
	g, p, w := sr.guard, sr.p, sr.r.w
	kill := func(hp int) (bool, int) {
		oldIntn := meleeIntn
		meleeIntn = func(n int) int { return n / 3 }
		defer func() { meleeIntn = oldIntn }()
		p.Swing.Kind = swingRange4
		p.Power, p.MaxPower = 10_000, 10_000
		g.HP, g.MaxHP, g.Dead, g.Threats = hp, 1_000_000, false, nil
		xp := p.TotalXP
		w.mu.Lock()
		defer w.mu.Unlock()
		killed := w.playerHitLocked(sr.sess, p, g, sr.r.objectID(g.ServerID))
		return killed, p.TotalXP - xp
	}
	builtKilled, builtXP := kill(1) // the built-in shot's kill
	if !builtKilled || !g.Dead {
		t.Fatalf("the built-in shot did not kill")
	}

	// each arrow 600 (the bow's 60 x 10): one alone leaves 1000 standing
	writeFormulas(t, w, "options:\n  ranged: arrow\nformulas:\n  player_melee: 'ranged_weapon_dmg * 10 + arrow * 0'\n")
	w.SetDamageFormulaMode(FormulaLive)
	killed, xp := kill(1100)
	if !killed || !g.Dead || g.HP != 0 || xp != builtXP {
		t.Errorf("the two arrows' kill: killed %v, dead %v, HP %d, experience %d (the built-in's %d)", killed, g.Dead, g.HP, xp, builtXP)
	}
	if killed, _ := kill(1300); killed || g.Dead || g.HP != 100 {
		t.Errorf("1,200 on 1,300: killed %v, HP %d", killed, g.HP)
	}
}

// A reload between a shot's arrows does not reach the shot: the set and the
// mode are read once for the swing, so both arrows run on, and are logged
// with, the file the swing began with.
func TestRangedShotOneSnapshot(t *testing.T) {
	sr := newShotRig(t)
	w := sr.r.w
	fixDice(t)
	writeFormulas(t, w, "options:\n  ranged: arrow\nformulas:\n  player_melee: '100 + uniform() * 0'\n")
	first := w.dmgf.set.Load()
	other, errs := compileDamageFormulas([]byte("options:\n  ranged: swing\nformulas:\n  player_melee: '5000 + uniform() * 0'\n"))
	if len(errs) > 0 || other.hash == first.hash {
		t.Fatal(errs)
	}
	swapAtFirstDraw := func() formula.Rand {
		n := 0
		return drawHook{func() float64 {
			if n++; n == 1 {
				w.dmgf.set.Store(other) // a reload, published mid-shot
			}
			return 0.5
		}}
	}
	old, oldShadow := formulaRand, shadowRand
	t.Cleanup(func() { formulaRand, shadowRand = old, oldShadow })

	formulaRand = swapAtFirstDraw()
	if got := sr.shoot(swingRange4); got.dealt != 200 {
		t.Errorf("live: dealt %d, want 200 (both arrows the first file's)", got.dealt)
	}

	w.dmgf.set.Store(first)
	path := shadowOn(t, w, 0)
	shadowRand = swapAtFirstDraw()
	sr.shoot(swingRange4)
	recs := readShadow(t, w, path)
	if len(recs) != 2 || recs[0].FileHash != first.hash || recs[1].FileHash != first.hash || recs[0].Value != 100 || recs[1].Value != 100 {
		t.Errorf("shadow's records: %+v", recs)
	}
	if w.dmgf.set.Load() != other {
		t.Error("the reload did not happen")
	}
}

// drawHook is a formula.Rand whose every draw is f's.
type drawHook struct{ f func() float64 }

func (d drawHook) Uniform() float64            { return d.f() }
func (d drawHook) Normal(_, _ float64) float64 { return d.f() }
func (d drawHook) Beta(_, _ float64) float64   { return d.f() }

// arrowShare's shares sum to the figure, by the arrows' rolls.
func TestArrowShare(t *testing.T) {
	parts := [maxShotArrows]int{30, 70}
	if a, b := arrowShare(101, 100, parts, 2, 0), arrowShare(101, 100, parts, 2, 1); a != 30 || b != 71 {
		t.Errorf("%d %d", a, b)
	}
	if a, b := arrowShare(9, 0, [maxShotArrows]int{}, 2, 0), arrowShare(9, 0, [maxShotArrows]int{}, 2, 1); a != 9 || b != 0 {
		t.Errorf("no roll: %d %d", a, b)
	}
	if a := arrowShare(50, 40, [maxShotArrows]int{40}, 1, 0); a != 50 {
		t.Errorf("one arrow: %d", a)
	}
}

// PvP has no formula unless the file names pvp_* or pvp: a file of the other
// sources leaves it on the built-in, live and in shadow (nothing recorded).
func TestPvPKeepsTheBuiltIn(t *testing.T) {
	w := NewWorld()
	fixDice(t)
	writeFormulas(t, w, "formulas:\n  player: base * 5\n  npc: base * 5\n  pet: base * 5\n")
	in := dmgIn{src: srcPvP, kind: dkMelee, ap: &Player{Name: "A"}, dp: &Player{Name: "B"}}
	if got := w.formulaDamage(10, in); got != 10 {
		t.Errorf("live PvP: %d", got)
	}
	path := shadowOn(t, w, 0)
	w.formulaDamage(10, in)
	w.formulaDamage(10, dmgIn{src: srcPlayer, kind: dkMelee, ap: &Player{Name: "A"}, dn: &NPC{Name: "Toy Rat"}})
	if recs := readShadow(t, w, path); len(recs) != 1 || recs[0].Path != "player_melee" {
		t.Errorf("records: %+v", recs)
	}
	writeFormulas(t, w, "formulas:\n  pvp: base * 5\n")
	w.SetDamageFormulaMode(FormulaLive)
	if got := w.formulaDamage(10, in); got != 50 {
		t.Errorf("a file naming pvp: %d", got)
	}
}

// A melee hit whose weapon type is a ranged weapon's: weapon_type unknown
// (-1) and weapon_mismatch, in the inputs and the line; a shot's is its bow's.
func TestWeaponTypeMismatch(t *testing.T) {
	w := NewWorld()
	fixDice(t)
	p := &Player{Name: "Hero", Level: 30}
	p.Gear.WeaponSlot, p.Gear.WeaponType, p.Gear.RangedType = 18, 7, 7 // a bow alone, swung in melee
	writeFormulas(t, w, "formulas:\n  player_melee: '(weapon_type + 10) * 10 + weapon_mismatch'\n")
	n := &NPC{Name: "Toy Rat"}
	if got := w.formulaDamage(1, dmgIn{src: srcPlayer, kind: dkMelee, ap: p, dn: n}); got != 91 {
		t.Errorf("melee with a bow: %d, want 91", got)
	}
	p.Swing.Kind = swingRange1
	if got := w.formulaDamage(1, dmgIn{src: srcPlayer, kind: dkMelee, ap: p, dn: n, ranged: true}); got != 170 {
		t.Errorf("a shot: %d, want 170", got)
	}
	path := shadowOn(t, w, 0)
	p.Swing.Kind = 0
	w.formulaDamage(1, dmgIn{src: srcPlayer, kind: dkMelee, ap: p, dn: n})
	if rec := readShadow(t, w, path)[0]; rec.WeapType != -1 || !rec.WeaponMismatch || rec.In["weapon_mismatch"] != 1.0 || rec.In["weapon_kind"] != "" {
		t.Errorf("the line: %+v", rec)
	}
}

// weapon_type routes to the defender's typed melee resist of its kind:
// slash, blunt, pierce (a bow's shot pierce), with positive and negative
// resists, a creature's and a player's; an unknown type to none.
func TestWeaponTypeRoutesToTypedResist(t *testing.T) {
	w := NewWorld()
	fixDice(t)
	mods := StatMods{Slash: 11, Blunt: -22, Pierce: 33}
	creature := &NPC{Name: "Toy Golem", Effects: []*ActiveEffect{{Name: "Toy Hex", Until: time.Now().Add(time.Hour), Mods: mods}}}
	player := &Player{Name: "Foe"}
	player.Mods = mods
	writeFormulas(t, w, "formulas:\n  player_melee: 'd_kind_resist + 1000 + (weapon_kind == \"slash\") * 100000 + (weapon_kind == \"blunt\") * 200000 + (weapon_kind == \"pierce\") * 300000 + d_kind_debuff_pct * 10'\n  pvp_melee: 'd_kind_resist + 1000'\n")
	for _, c := range []struct {
		typ    int
		ranged bool
		want   int // against the creature
	}{
		{1, false, 100000 + 1000 + 11},
		{2, false, 100000 + 1000 + 11},
		{3, false, 200000 + 1000 - 22 + 220},
		{5, false, 300000 + 1000 + 33},
		{7, true, 300000 + 1000 + 33},
		{7, false, 1000}, // a bow swung in melee: unknown, no resist
	} {
		a := &Player{Name: "Hero", PrimaryType: byte(c.typ)}
		if c.typ == 7 {
			a.Gear.WeaponSlot, a.Gear.WeaponType, a.Gear.RangedType = 18, 7, 7
		}
		if c.ranged {
			a.Swing.Kind = swingRange1
		}
		if got := w.formulaDamage(1, dmgIn{src: srcPlayer, kind: dkMelee, ap: a, dn: creature, ranged: c.ranged}); got != c.want {
			t.Errorf("type %d (ranged %v) on a creature: %d, want %d", c.typ, c.ranged, got, c.want)
		}
		wantP := 1000 + map[int]int{1: 11, 2: 11, 3: -22, 5: 33, 7: 33}[c.typ]
		if c.typ == 7 && !c.ranged {
			wantP = 1000
		}
		if got := w.formulaDamage(1, dmgIn{src: srcPvP, kind: dkMelee, ap: a, dp: player, ranged: c.ranged}); got != wantP {
			t.Errorf("type %d (ranged %v) on a player: %d, want %d", c.typ, c.ranged, got, wantP)
		}
	}
}

// Under ranged: builtin a bow's shot runs no formula, and shadow mode still
// writes its line: the built-in figure as the formula's, no draw, the
// fallback ranged_builtin, the attack's kind; the summary counts each shot as
// a fallback, by path and by reason. A melee swing runs the formula as ever.
func TestRangedBuiltinShotsAreCountedFallbacks(t *testing.T) {
	sr := newShotRig(t)
	fixDice(t)
	built1, built2 := sr.shoot(swingRange1), sr.shoot(swingRange4)
	writeFormulas(t, sr.r.w, "options:\n  ranged: builtin\nformulas:\n  player_melee: 'base + ranged_weapon_dmg + uniform() * 0'\n")
	path := shadowOn(t, sr.r.w, 0)
	sf := fixShadowDice(t, 0.5)
	if got := sr.shoot(swingRange4); got != built2 {
		t.Errorf("shadow, two arrows: %+v, want %+v", got, built2)
	}
	if got := sr.shoot(swingRange1); got != built1 {
		t.Errorf("shadow, one arrow: %+v, want %+v", got, built1)
	}
	sr.shoot(0) // a melee swing: the formula runs (one draw)
	recs := readShadow(t, sr.r.w, path)
	if len(recs) != 3 {
		t.Fatalf("%d lines, want 3: %+v", len(recs), recs)
	}
	for i, want := range []shotOut{built2, built1} {
		r := recs[i]
		if r.Fallback != FallbackRangedBuiltin || r.Attack != "ranged" || r.Draws != 0 || r.Value != r.Builtin || r.Builtin != want.dealt ||
			r.FormulaAfter != r.BuiltinAfter || r.Fail != "" || r.In["ranged_weapon_dmg"] != 60.0 || r.In["d_dummy"] == nil {
			t.Errorf("shot %d's line: %+v", i, r)
		}
	}
	if recs[0].Arrows != 2 || recs[1].Arrows != 1 {
		t.Errorf("the shots' arrows: %d %d", recs[0].Arrows, recs[1].Arrows)
	}
	if m := recs[2]; m.Fallback != "" || m.Attack != "melee" || m.Draws != 1 || m.Value != m.Builtin+60 {
		t.Errorf("the melee swing's line: %+v", m)
	}
	if sf.Next != 1 {
		t.Errorf("shadow draws %d, want the melee swing's 1", sf.Next)
	}
	s, err := SummarizeShadowLog(path, ShadowSummaryOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if s.FallbackReasons["player_melee / ranged_builtin"] != 2 || len(s.ByPath) != 1 || s.ByPath[0].Hits != 3 || s.ByPath[0].Fallbacks != 2 ||
		s.ByPath[0].Covered != 1 || s.Total.Fallbacks != 2 {
		t.Errorf("the summary: reasons %v, by path %+v", s.FallbackReasons, s.ByPath)
	}
}
