package entity

import (
	"bufio"
	"math"
	"os"
	"strconv"
	"strings"
	"testing"
)

// The NPC melee roll held to Sandstorm's hits (defence.go):
// testdata/sandstorm_defence_hits.txt is every hit two Training Dummies
// landed on four characters at ACs and AGIs held one at a time, and the
// roll must be able to make each of them, be what they say better than
// anything one planted bug makes of it, and have its constants where those
// hits put them.

type defenceCondition struct {
	level, class, ac, agi int
	hits                  []int
}

func sandstormDefenceHits(t *testing.T) []defenceCondition {
	t.Helper()
	f, err := os.Open("testdata/sandstorm_defence_hits.txt")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	var out []defenceCondition
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	for sc.Scan() {
		line := sc.Text()
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		head, rest, ok := strings.Cut(line, ":")
		f := strings.Fields(head)
		if !ok || len(f) != 4 {
			t.Fatalf("bad line %q", line)
		}
		var c defenceCondition
		for i, p := range []*int{&c.level, &c.class, &c.ac, &c.agi} {
			if *p, err = strconv.Atoi(f[i]); err != nil {
				t.Fatal(err)
			}
		}
		for _, h := range strings.Fields(rest) {
			n, err := strconv.Atoi(h)
			if err != nil {
				t.Fatal(err)
			}
			c.hits = append(c.hits, n)
		}
		out = append(out, c)
	}
	if len(out) < 30 {
		t.Fatalf("%d conditions read", len(out))
	}
	return out
}

// rollRange is the roll's least and most on the player, through hitOn
// itself: the lowest draw and the highest.
func rollRange(r meleeRoll, p *Player) (lo, hi int) {
	return r.hitOn(p, func(int) int { return 0 }), r.hitOn(p, func(n int) int { return n - 1 })
}

// hitChance is how likely the roll is to make the hit on the player: the
// floor's share, or one in its width above the floor, or none.
func hitChance(r meleeRoll, p *Player, hit int) float64 {
	_, top := rollRange(r, p)
	floor := max(r.Floor, 1)
	if top <= floor {
		if hit == floor {
			return 1
		}
		return 0
	}
	base := top - r.Width
	switch {
	case hit == floor:
		return math.Min(math.Max(float64(floor-base+1)/float64(r.Width+1), 0), 1)
	case hit > floor && hit >= base && hit <= top:
		return 1 / float64(r.Width+1)
	}
	return 0
}

// defenceNLL is minus the log of how likely the roll makes Sandstorm's hits,
// mut a planted change to the player and the roll first; impossible names
// the first hit the roll cannot make.
func defenceNLL(conds []defenceCondition, mut func(*Player, *meleeRoll)) (nll float64, impossible string) {
	for _, c := range conds {
		// level 50, as the recorded characters about were: its stat cap
		// (515, mastery.go) is past every AGI recorded (397 at most), so
		// the stats are as Sandstorm's channel had them
		p := &Player{Level: 50, Class: c.class, AC: c.ac, Agi: c.agi}
		r := npcMeleeRoll(&NPC{Level: c.level, Dummy: TrainingDummy})
		if mut != nil {
			mut(p, &r)
		}
		for _, h := range c.hits {
			pr := hitChance(r, p, h)
			if pr == 0 {
				lo, hi := rollRange(r, p)
				return math.Inf(1), "Lv" + strconv.Itoa(c.level) + " dummy, class " + strconv.Itoa(c.class) +
					", AC " + strconv.Itoa(c.ac) + ", AGI " + strconv.Itoa(c.agi) + ": Sandstorm's " + strconv.Itoa(h) +
					", the roll " + strconv.Itoa(lo) + ".." + strconv.Itoa(hi)
			}
			nll -= math.Log(pr)
		}
	}
	return nll, ""
}

func TestMeleeRollsAsSandstorm(t *testing.T) {
	nll, impossible := defenceNLL(sandstormDefenceHits(t), nil)
	if impossible != "" {
		t.Fatal(impossible)
	}
	t.Logf("-log likelihood of Sandstorm's hits: %.1f", nll)
}

// The controls: each planted bug must make Sandstorm's hits far less likely
// (by 10 in the log, odds of 22,000 to one) or impossible.
func TestMeleeRollControls(t *testing.T) {
	conds := sandstormDefenceHits(t)
	base, _ := defenceNLL(conds, nil)
	controls := map[string]func(*Player, *meleeRoll){
		"AGI counting for nothing":            func(p *Player, r *meleeRoll) { p.Agi = 0 },
		"every archetype a tank's":            func(p *Player, r *meleeRoll) { p.Class = 3 },
		"every archetype a caster's":          func(p *Player, r *meleeRoll) { p.Class = 11 },
		"one AC slope for both dummies":       func(p *Player, r *meleeRoll) { r.ACSlope = 0.1137 },
		"AC counting 10% less":                func(p *Player, r *meleeRoll) { r.ACSlope *= 0.9 },
		"the old dummy, its floor every time": func(p *Player, r *meleeRoll) { r.Top = -1e6 },
		"a roll 10% narrower":                 func(p *Player, r *meleeRoll) { r.Width = r.Width * 9 / 10 },
	}
	for name, mut := range controls {
		nll, impossible := defenceNLL(conds, mut)
		if impossible == "" && nll < base+10 {
			t.Errorf("%s: -log likelihood %.1f against the roll's %.1f, so the test cannot see it", name, nll, base)
		}
	}
}

// The constants are the fit's: no one of them moved a step either way makes
// Sandstorm's hits likelier by more than a little.
func TestMeleeRollConstantsAreTheFit(t *testing.T) {
	conds := sandstormDefenceHits(t)
	base, _ := defenceNLL(conds, nil)
	type knob struct {
		name string
		move func(d float64)
		step float64
	}
	roll := func(level int, f func(r *meleeRoll, d float64)) func(float64) {
		return func(d float64) {
			r := dummyRolls[TrainingDummy][level]
			f(&r, d)
			dummyRolls[TrainingDummy][level] = r
		}
	}
	knobs := []knob{
		{"the Lv45's top", roll(45, func(r *meleeRoll, d float64) { r.Top += d }), 1},
		{"the Lv55's top", roll(55, func(r *meleeRoll, d float64) { r.Top += d }), 1},
		{"the Lv45's width", roll(45, func(r *meleeRoll, d float64) { r.Width += int(d) }), 2},
		{"the Lv55's width", roll(55, func(r *meleeRoll, d float64) { r.Width += int(d) }), 2},
		{"the Lv45's AC slope", roll(45, func(r *meleeRoll, d float64) { r.ACSlope += d }), 0.002},
		{"the Lv55's AC slope", roll(55, func(r *meleeRoll, d float64) { r.ACSlope += d }), 0.002},
		{"a tank's AGI slope", func(d float64) { agiSlope[archTank] += d }, 0.01},
		{"a melee's AGI slope", func(d float64) { agiSlope[archMelee] += d }, 0.01},
		{"a caster's AGI slope", func(d float64) { agiSlope[archCaster] += d }, 0.01},
		{"a melee's offset", func(d float64) { archOffset[archMelee] += d }, 1},
		{"a caster's offset", func(d float64) { archOffset[archCaster] += d }, 1},
	}
	for _, k := range knobs {
		for _, sign := range []float64{1, -1} {
			k.move(sign * k.step)
			nll, _ := defenceNLL(conds, nil)
			k.move(-sign * k.step)
			if nll < base-1 {
				t.Errorf("%s moved %+g: -log likelihood %.1f, better than the constant's %.1f", k.name, sign*k.step, nll, base)
			}
		}
	}
	if after, _ := defenceNLL(conds, nil); after != base {
		t.Fatalf("the knobs were not put back: %.3f, was %.3f", after, base)
	}
}

func TestArchetypeOf(t *testing.T) {
	want := map[int]archetype{0: archTank, 2: archTank, 3: archTank, 1: archMelee, 4: archMelee, 5: archMelee, 6: archMelee,
		7: archPriest, 8: archPriest, 9: archPriest, 10: archCaster, 11: archCaster, 12: archCaster, 13: archCaster, 14: archCaster}
	for class, a := range want {
		if got := archetypeOf(class); got != a {
			t.Errorf("class %d: archetype %d, want %d", class, got, a)
		}
	}
}

// The beach's creatures: the Lv34 Desert Crab hit a shadow knight of 1763
// AC its floor, 17, every time, and an enchanter of 167 AC and 397 AGI 17,
// 17, 19 and 45; the Lv35 Gharial hit the knight 18 every time.
func TestCreatureRolls(t *testing.T) {
	knight := &Player{Class: 3, AC: 1763, Agi: 95}
	enchanter := &Player{Class: 12, AC: 167, Agi: 397}
	crab, gharial := creatureRoll(34), creatureRoll(35)
	if lo, hi := rollRange(crab, knight); lo != 17 || hi != 17 {
		t.Errorf("the crab on the knight: %d..%d, Sandstorm's 17 every time", lo, hi)
	}
	if lo, hi := rollRange(gharial, knight); lo != 18 || hi != 18 {
		t.Errorf("the gharial on the knight: %d..%d, Sandstorm's 18 every time", lo, hi)
	}
	if lo, hi := rollRange(crab, enchanter); lo != 17 || hi < 45 {
		t.Errorf("the crab on the enchanter: %d..%d, Sandstorm's 17 to 45", lo, hi)
	}
	// no hit is less than one, whatever the level
	if lo, _ := rollRange(creatureRoll(0), &Player{}); lo != 1 {
		t.Errorf("a level 0 creature's least: %d, want 1", lo)
	}
}

// The dummies every tank measured took the floor from, every time: a shadow
// knight of 1763 AC every Training Dummy to Lv55 and the Regular Lv55; a
// bard of 1430 AC and 207 AGI the Training Dummies to Lv45.
func TestDummyFloorsOnTanks(t *testing.T) {
	knight := &Player{Class: 3, AC: 1763, Agi: 95}
	bard := &Player{Class: 5, AC: 1430, Agi: 207}
	floors := map[int]int{20: 8, 25: 10, 30: 12, 35: 14, 45: 24, 50: 21, 55: 30}
	for level, f := range floors {
		r := npcMeleeRoll(&NPC{Level: level, Dummy: TrainingDummy})
		if lo, hi := rollRange(r, knight); lo != f || hi != f {
			t.Errorf("Training Dummy Lv%d on the knight: %d..%d, Sandstorm's %d", level, lo, hi, f)
		}
		if level <= 45 {
			if lo, hi := rollRange(r, bard); lo != f || hi != f {
				t.Errorf("Training Dummy Lv%d on the bard: %d..%d, Sandstorm's %d", level, lo, hi, f)
			}
		}
	}
	if lo, hi := rollRange(npcMeleeRoll(&NPC{Level: 55, Dummy: RegularDummy}), knight); lo != 30 || hi != 30 {
		t.Errorf("Regular Dummy Lv55 on the knight: %d..%d, Sandstorm's 30", lo, hi)
	}
}
