package entity

import (
	"bufio"
	"math"
	"os"
	"strconv"
	"strings"
	"testing"
)

// The player's melee roll held to Sandstorm's swings (offence.go):
// testdata/sandstorm_offence_swings.txt is every swing three characters
// landed on the training dummies and a Gharial, their weapon, its
// condition, STR and DEX held one condition at a time.

type offenceCondition struct {
	class, dmg int
	cond       float64
	twoHanded  bool
	str, dex   int
	level      int
	creature   bool
	swings     []int
	line       string
}

func sandstormOffenceSwings(t *testing.T) []offenceCondition {
	t.Helper()
	f, err := os.Open("testdata/sandstorm_offence_swings.txt")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	var out []offenceCondition
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	num := func(s string) int {
		n, err := strconv.Atoi(s)
		if err != nil {
			t.Fatalf("%q: %v", s, err)
		}
		return n
	}
	for sc.Scan() {
		line := sc.Text()
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		head, rest, ok := strings.Cut(line, ":")
		f := strings.Fields(head)
		if !ok || len(f) != 7 {
			t.Fatalf("bad line %q", line)
		}
		cur, most, _ := strings.Cut(f[2], "/")
		c := offenceCondition{class: num(f[0]), dmg: num(f[1]), cond: float64(num(cur)) / float64(num(most)),
			twoHanded: f[3] == "2", str: num(f[4]), dex: num(f[5]), line: head}
		if strings.HasSuffix(f[6], "g") {
			c.creature, c.level = true, num(strings.TrimSuffix(f[6], "g"))
		} else {
			c.level = num(f[6])
		}
		for _, h := range strings.Fields(rest) {
			c.swings = append(c.swings, num(h))
		}
		out = append(out, c)
	}
	if len(out) < 30 {
		t.Fatalf("%d conditions read", len(out))
	}
	return out
}

func (c offenceCondition) actors() (*Player, *NPC) {
	p := &Player{Class: c.class, Str: c.str, Dex: c.dex,
		Gear: Gear{WeaponDamage: c.dmg, WeaponCond: c.cond, WeaponTwoHanded: c.twoHanded}}
	npc := &NPC{Level: c.level, Dummy: TrainingDummy}
	if c.creature {
		npc.Dummy = NotDummy
	}
	return p, npc
}

// swingChance is how likely the roll makes the swing: the floor's share,
// or one in the roll's span above the floor, or none.
func swingChance(floor, low, high, hit int) float64 {
	if high <= floor {
		if hit == floor {
			return 1
		}
		return 0
	}
	span := float64(high - low + 1)
	switch {
	case hit == floor:
		return math.Min(math.Max(float64(floor-low+1)/span, 0), 1)
	case hit > floor && hit >= low && hit <= high:
		return 1 / span
	}
	return 0
}

// offenceNLL is minus the log of how likely the roll makes Sandstorm's
// swings, mut a planted change first; impossible names the first swing the
// roll cannot make.
func offenceNLL(conds []offenceCondition, mut func(*Player, *NPC)) (nll float64, impossible string) {
	for _, c := range conds {
		p, npc := c.actors()
		if mut != nil {
			mut(p, npc)
		}
		floor, low, high := playerMeleeRoll(p, npc)
		for _, h := range c.swings {
			pr := swingChance(floor, low, high, h)
			if pr == 0 {
				return math.Inf(1), c.line + ": Sandstorm's " + strconv.Itoa(h) + ", the roll " + strconv.Itoa(max(low, floor)) + ".." + strconv.Itoa(max(high, floor))
			}
			nll -= math.Log(pr)
		}
	}
	return nll, ""
}

func TestPlayerMeleeAsSandstorm(t *testing.T) {
	nll, impossible := offenceNLL(sandstormOffenceSwings(t), nil)
	if impossible != "" {
		t.Fatal(impossible)
	}
	t.Logf("-log likelihood of Sandstorm's swings: %.1f", nll)
}

// The controls: each planted bug must make Sandstorm's swings far less
// likely (by 10 in the log) or impossible.
func TestPlayerMeleeControls(t *testing.T) {
	conds := sandstormOffenceSwings(t)
	base, _ := offenceNLL(conds, nil)
	controls := map[string]func(*Player, *NPC){
		"STR counting for nothing":       func(p *Player, n *NPC) { p.Str = 0 },
		"DEX counting for nothing":       func(p *Player, n *NPC) { p.Dex = 0 },
		"every archetype a tank's":       func(p *Player, n *NPC) { p.Class = 3 },
		"every archetype a caster's":     func(p *Player, n *NPC) { p.Class = 11 },
		"the target's level not counted": func(p *Player, n *NPC) { n.Level = 20 },
		"a weapon's condition not counted": func(p *Player, n *NPC) {
			p.Gear.WeaponCond = 1
		},
		"a two-hander as a one-hander": func(p *Player, n *NPC) { p.Gear.WeaponTwoHanded = false },
		"the Lv55 dummy as its level":  func(p *Player, n *NPC) { n.Dummy = NotDummy },
	}
	for name, mut := range controls {
		nll, impossible := offenceNLL(conds, mut)
		if impossible == "" && nll < base+10 {
			t.Errorf("%s: -log likelihood %.1f against the roll's %.1f, so the test cannot see it", name, nll, base)
		}
	}
}

// The constants are the fit's: no one of them moved a step either way makes
// Sandstorm's swings likelier by more than a little.
func TestPlayerMeleeConstantsAreTheFit(t *testing.T) {
	conds := sandstormOffenceSwings(t)
	base, _ := offenceNLL(conds, nil)
	f := func(v *float64) func(float64) { return func(d float64) { *v += d } }
	arch := func(a archetype, field func(*offenceArch) *float64) func(float64) {
		return func(d float64) { *field(&offenceArchs[a]) += d }
	}
	knobs := []struct {
		name string
		move func(float64)
		step float64
	}{
		{"the top's share of D", f(&offHighD), 0.004},
		{"the top's STR", f(&offHighSTR), 0.02},
		{"the bottom's share of D", f(&offLowD), 0.004},
		{"the bottom's DEX", f(&offLowDEX), 0.00005},
		{"a two-hander's D", f(&offTwoHand), 0.01},
		{"a tank's top", arch(archTank, func(a *offenceArch) *float64 { return &a.High }), 2},
		{"a tank's bottom", arch(archTank, func(a *offenceArch) *float64 { return &a.Low }), 2},
		{"a tank's defence", arch(archTank, func(a *offenceArch) *float64 { return &a.Defence }), 0.02},
		{"a caster's top", arch(archCaster, func(a *offenceArch) *float64 { return &a.High }), 2},
		{"a caster's bottom", arch(archCaster, func(a *offenceArch) *float64 { return &a.Low }), 2},
		{"a caster's defence", arch(archCaster, func(a *offenceArch) *float64 { return &a.Defence }), 0.02},
		{"a caster's bottom share", arch(archCaster, func(a *offenceArch) *float64 { return &a.LowShare }), 0.03},
		{"a melee's top", arch(archMelee, func(a *offenceArch) *float64 { return &a.High }), 2},
		{"a melee's defence", arch(archMelee, func(a *offenceArch) *float64 { return &a.Defence }), 0.03},
	}
	for _, k := range knobs {
		for _, sign := range []float64{1, -1} {
			k.move(sign * k.step)
			nll, _ := offenceNLL(conds, nil)
			k.move(-sign * k.step)
			if nll < base-1 {
				t.Errorf("%s moved %+g: -log likelihood %.1f, better than the constant's %.1f", k.name, sign*k.step, nll, base)
			}
		}
	}
	if after, _ := offenceNLL(conds, nil); after != base {
		t.Fatalf("the knobs were not put back: %.3f, was %.3f", after, base)
	}
}

// Every floor Sandstorm's swings came to (a least that was a quarter of
// them or more) is the roll's, exactly: the floor is a whole number of the
// weapon, its condition and the STR.
func TestPlayerMeleeFloors(t *testing.T) {
	for _, c := range sandstormOffenceSwings(t) {
		least := c.swings[0]
		for _, h := range c.swings {
			least = min(least, h)
		}
		n := 0
		for _, h := range c.swings {
			if h == least {
				n++
			}
		}
		if n < 3 || 4*n < len(c.swings) {
			continue
		}
		p, npc := c.actors()
		if floor, _, _ := playerMeleeRoll(p, npc); floor != least {
			t.Errorf("%s: Sandstorm's floor %d (%d times), the roll's %d", c.line, least, n, floor)
		}
	}
}

// A broken weapon still hits: a fifth of the roll, a third of the floor.
func TestBrokenWeaponWorth(t *testing.T) {
	if r, f := rollWorth(0), floorWorth(0); math.Abs(r-0.195) > 0.001 || math.Abs(f-0.343) > 0.001 {
		t.Errorf("a broken weapon's worth: roll %.3f, floor %.3f", r, f)
	}
	if r, f := rollWorth(1), floorWorth(1); r != 1 || f != 1 {
		t.Errorf("a weapon in repair's worth: roll %v, floor %v", r, f)
	}
}
