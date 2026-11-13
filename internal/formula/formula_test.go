package formula

import (
	"errors"
	"math"
	"math/rand"
	"os"
	"strings"
	"testing"
)

var testVars = map[string]int{"base": 0, "level": 1, "x": 2, "class_name": 3, "target": 4, "family": 5}

func env(vals ...Value) *Env {
	v := make([]Value, 64)
	copy(v, vals)
	return &Env{V: v}
}

func run(t *testing.T, src string, e *Env) float64 {
	t.Helper()
	p, err := Compile(src, testVars)
	if err != nil {
		t.Fatalf("Compile(%q): %v", src, err)
	}
	got, err := p.Run(e)
	if err != nil {
		t.Fatalf("Run(%q): %v", src, err)
	}
	return got
}

func TestOperators(t *testing.T) {
	cases := []struct {
		src  string
		v    []Value
		want float64
	}{
		{"base * 2", []Value{Number(10)}, 20},
		{"1 + 2 * 3", nil, 7},
		{"(1 + 2) * 3", nil, 9},
		{"10 - 4 - 3", nil, 3},
		{"2 ^ 3 ^ 2", nil, 512},
		{"-2 ^ 2 + 5", nil, 1},
		{"7 % 4", nil, 3},
		{"base > 5 ? 1 : 2", []Value{Number(6)}, 1},
		{"base > 5 ? 1 : base > 2 ? 3 : 4", []Value{Number(3)}, 3},
		{"1 < 2 && 2 < 3", nil, 1},
		{"1 > 2 || !0", nil, 1},
		{"base == 3", []Value{Number(3)}, 1},
		{"base != 3", []Value{Number(3)}, 0},
		{`class_name == "juggler"`, []Value{{}, {}, {}, String("juggler")}, 1},
		{`class_name == 3`, []Value{{}, {}, {}, String("3")}, 0},
		{"min(5, base, 9)", []Value{Number(7)}, 5},
		{"max(level, 1)", nil, 1},
		{"clamp(base, 1, 10)", []Value{Number(50)}, 10},
		{"if(x, 2, 3)", nil, 3},
		{"round(2.5) + floor(1.9) + ceil(1.1) + trunc(-1.5) + 1", nil, 3 + 1 + 2 - 1 + 1},
		{"abs(-3) + sqrt(16) + pow(2, 10) + exp(0) + log(1) + log10(100)", nil, 3 + 4 + 1024 + 1 + 0 + 2},
		{"sign(-5) + sign(0) + sign(2) + 1", nil, 1},
		{"1.5e2 + .5", nil, 150.5},
		{"base *\n 2 # a comment", []Value{Number(4)}, 8},
	}
	for _, c := range cases {
		if got := run(t, c.src, env(c.v...)); got != c.want {
			t.Errorf("%q = %v, want %v", c.src, got, c.want)
		}
	}
}

func TestResultChecks(t *testing.T) {
	for _, src := range []string{"base / 0", "sqrt(0 - 1)", "0 - 5", `"text"`} {
		p, err := Compile(src, testVars)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := p.Run(env(Number(1))); err == nil {
			t.Errorf("%q ran", src)
		}
	}
}

func TestShortEnvIsAnErrorNotAPanic(t *testing.T) {
	p, _ := Compile("x + 1", testVars)
	if _, err := p.Run(&Env{V: make([]Value, 1)}); err == nil {
		t.Error("short env ran")
	}
}

func TestCompileErrorsNeverQuoteTheFormula(t *testing.T) {
	for _, src := range []string{
		"base * zorblax", "base * frobnicate(2)", "base * (31337 +", "base * 31337 31337",
		"base $ 2", "min()", "pow(1)", "1 ? 2", "", `"open`, `1 + "zorblax"`,
		`lookup("zorblax", 1)`, `lookup(base, 1)`, "zorblax.field", "uniform(1)",
	} {
		_, err := Compile(src, testVars)
		if err == nil {
			t.Errorf("%q compiled", src)
			continue
		}
		var se *SourceError
		if errors.As(err, &se) && se.Err.Col < 1 {
			t.Errorf("%q: %v has no column", src, err)
		}
		msg := err.Error()
		for _, frag := range []string{"zorblax", "frobnicate", "31337", "$", "open"} {
			if strings.Contains(msg, frag) {
				t.Errorf("%q: error %q quotes the formula", src, msg)
			}
		}
	}
}

func TestBounds(t *testing.T) {
	if _, err := Compile(strings.Repeat("(", MaxDepth+5)+"1"+strings.Repeat(")", MaxDepth+5), testVars); err == nil {
		t.Error("deep nesting compiled")
	}
	if _, err := Compile(strings.Repeat("-", MaxDepth+5)+"1", testVars); err == nil {
		t.Error("deep unary chain compiled")
	}
	if _, err := Compile(strings.Repeat("1+", MaxNodes+5)+"1", testVars); err == nil {
		t.Error("huge formula compiled")
	}
	if _, err := Compile(strings.Repeat(" ", MaxSource+1), testVars); err == nil {
		t.Error("over-long source compiled")
	}
	steps := make([]NamedExpr, MaxSteps+1)
	for i := range steps {
		steps[i] = NamedExpr{Name: "s" + strings.Repeat("a", i), Expr: Expr{Text: "1"}}
	}
	if _, err := (&Compiler{Inputs: testVars}).Compile(Source{Steps: steps, Result: Expr{Text: "1"}}); err == nil {
		t.Error("too many steps compiled")
	}
}

func toy(t testing.TB) (map[string]*Program, int) {
	t.Helper()
	data, err := os.ReadFile("testdata/toy.yaml")
	if err != nil {
		t.Fatal(err)
	}
	known := func(n string) bool { return strings.HasPrefix(n, "toy_") }
	progs, tables, errs := ParseFile(data, known, testVars)
	if len(errs) > 0 {
		t.Fatal(errs)
	}
	return progs, tables
}

func TestToyFile(t *testing.T) {
	progs, tables := toy(t)
	if tables != 2 || len(progs) != 2 {
		t.Fatalf("%d tables, programs %v", tables, Names(progs))
	}
	e := env(Number(7))
	e.Rand = &Forced{}
	if got, err := progs["toy_one"].Run(e); err != nil || got != 14 {
		t.Errorf("toy_one = %v %v", got, err)
	}
	p := progs["toy_two"]
	if !p.Draws() {
		t.Error("toy_two makes draws")
	}
	cases := []struct {
		class, target, family string
		draws                 []float64
		want                  float64
	}{
		// the juggler's profile (scale 2), pick's draw 0.1 of weights 1+3:
		// the first slot (mult 1); the golem by name (bonus 5); roll 0.25
		{"juggler", "pudding golem", "", []float64{0.1, 0.25}, 7*2*1 + 5 + 0.25},
		// draw 0.5 lands in the second slot (mult 10)
		{"juggler", "pudding golem", "", []float64{0.5, 0}, 7*2*10 + 5},
		// an unknown class: the default profile; an unknown creature of the
		// wobbly family: the family's record (bonus 1)
		{"bellringer", "custard golem", "wobbly", []float64{0.9, 0}, 7 + 1},
		// neither name nor family: the default record, its bonus missing: 0
		{"bellringer", "teapot", "teaware", []float64{0, 0}, 7},
	}
	for _, c := range cases {
		e := env(Number(7), Number(0), Number(0), String(c.class), String(c.target), String(c.family))
		e.Rand = &Forced{Draws: c.draws}
		got, err := p.Run(e)
		if err != nil || got != c.want {
			t.Errorf("%+v = %v %v, want %v", c, got, err, c.want)
		}
	}
	// the clamps: a base of 0 still gives 1 at least (min), a huge one 10000
	e = env(Number(0), Number(0), Number(0), String("bellringer"), String("x"), String("y"))
	e.Rand = &Forced{Draws: []float64{0, 0}}
	if got, _ := p.Run(e); got != 1 {
		t.Errorf("min clamp: %v", got)
	}
	e.V[0] = Number(1e9)
	e.Rand = &Forced{Draws: []float64{0, 0}}
	if got, _ := p.Run(e); got != 10000 {
		t.Errorf("max clamp: %v", got)
	}
	// a forced vector too short: the run fails, it makes nothing up
	e.Rand = &Forced{Draws: []float64{0}}
	if _, err := p.Run(e); err == nil {
		t.Error("ran past the forced draws")
	}
}

func TestTableFunctions(t *testing.T) {
	data := []byte(`
tables:
  t:
    a: {n: 2, list: [5, 6, 7], sub: {deep: 3}}
formulas:
  toy_has: 'has(lookup("t", "a"), "n") + has(lookup("t", "a"), "zz") * 10'
  toy_len: 'len(lookup("t", "a").list) + at(lookup("t", "a").list, 1)'
  toy_deep: 'lookup("t", "a").sub.deep'
  toy_missing: 'lookup("t", "a").zz'
  toy_numkey: 'field(lookup("t", 5, "a"), "n", 0)'
  toy_pick_index: 'pick_index(0, 1, 0) + pick(lookup("t", "a").list)'
`)
	progs, _, errs := ParseFile(data, func(n string) bool { return strings.HasPrefix(n, "toy_") }, testVars)
	if len(errs) > 0 {
		t.Fatal(errs)
	}
	for name, want := range map[string]float64{"toy_has": 1, "toy_len": 9, "toy_deep": 3, "toy_numkey": 2} {
		if got, err := progs[name].Run(env()); err != nil || got != want {
			t.Errorf("%s = %v %v, want %v", name, got, err, want)
		}
	}
	e := env()
	e.Rand = &Forced{Draws: []float64{0.5, 0.99}}
	if got, err := progs["toy_pick_index"].Run(e); err != nil || got != 1+7 {
		t.Errorf("pick_index + pick = %v %v", got, err)
	}
	_, err := progs["toy_missing"].Run(env())
	var re *RunError
	if !errors.As(err, &re) || strings.Contains(err.Error(), "zz") {
		t.Errorf("a missing field: %v", err)
	}
}

func TestParseFileErrors(t *testing.T) {
	known := func(n string) bool { return n == "a" }
	cases := []string{
		"formulas:\n  c: 1",                               // unknown name
		"formulas:\n  a: 1\n  a: 2",                       // duplicate
		"formulas:\n  a: {result: 1, mid: 2}",             // bad part
		"formulas:\n  a: {min: 1}",                        // no result
		"formulas:\n  a: base * hush1234",                 // unknown name in it
		"formulas:\n  a: {steps: [{base: 1}], result: 1}", // a step named as an input
		"formulas:\n  a: {steps: [{s: 1}, {s: 2}], result: s}",
		"formulas:\n  a: 'lookup(\"nosuchtable\", 1)'",
		"other: 1",
		"tables:\n  t: [1, 2]",
		"tables:\n  t: {k: {v: null}}",
		"formulas: [\n  hush1234",
		"tables:\n  t: {k: 1, k: 2}",
	}
	for _, src := range cases {
		got, _, errs := ParseFile([]byte(src), known, testVars)
		if len(errs) == 0 || got != nil {
			t.Errorf("%q: no error (%v)", src, got)
		}
		for _, e := range errs {
			if strings.Contains(e.Error(), "hush1234") || strings.Contains(e.Error(), "base *") || strings.Contains(e.Error(), "nosuchtable") {
				t.Errorf("%q: error %q quotes the file", src, e)
			}
		}
	}
	_, _, errs := ParseFile([]byte("formulas:\n  a: base +\n  c: 1\n"), known, testVars)
	if len(errs) != 2 {
		t.Errorf("want every mistake, got %v", errs)
	}
	_, _, errs = ParseFile([]byte("formulas:\n  a:\n    result: |\n      1 +\n      2 $ 3\n"), known, testVars)
	if len(errs) != 1 || !strings.Contains(errs[0].Error(), "line 5") {
		t.Errorf("a block expression's error placed %v", errs)
	}
}

// The samplers against their distributions' moments, on a seeded source.
func TestSamplers(t *testing.T) {
	r := rand.New(rand.NewSource(7))
	s := RandSource{Float64: r.Float64, NormFloat64: r.NormFloat64}
	const n = 40000
	for _, ab := range [][2]float64{{2, 5}, {0.5, 0.5}, {30, 3}, {1, 1}} {
		a, b := ab[0], ab[1]
		sum, sq := 0.0, 0.0
		for range n {
			x := s.Beta(a, b)
			if !(x >= 0 && x <= 1) {
				t.Fatalf("Beta(%v,%v) = %v", a, b, x)
			}
			sum += x
			sq += x * x
		}
		mean, v := sum/n, sq/n-(sum/n)*(sum/n)
		wantMean, wantVar := a/(a+b), a*b/((a+b)*(a+b)*(a+b+1))
		if math.Abs(mean-wantMean) > 0.01 || math.Abs(v-wantVar) > 0.1*wantVar+0.001 {
			t.Errorf("Beta(%v,%v): mean %.4f var %.5f, want %.4f %.5f", a, b, mean, v, wantMean, wantVar)
		}
	}
	if !math.IsNaN(s.Beta(0, 1)) || !math.IsNaN(s.Beta(1, -1)) || !math.IsNaN(s.Normal(0, -1)) {
		t.Error("bad parameters did not fail")
	}
	sum := 0.0
	for range n {
		sum += s.Normal(10, 2)
	}
	if m := sum / n; math.Abs(m-10) > 0.05 {
		t.Errorf("Normal mean %v", m)
	}
}

// One hit's cost: a table lookup with a fallback chain, a weighted pick, a
// beta draw and a little arithmetic.
func BenchmarkLookupBeta(b *testing.B) {
	data := []byte(`
tables:
  profiles:
    juggler: {a: 2, b: 5, scale: 1.5, slots: [{weight: 1, mult: 1}, {weight: 3, mult: 2}]}
    default: {a: 1, b: 1, scale: 1, slots: [{weight: 1, mult: 1}]}
formulas:
  toy:
    steps:
      - prof: lookup("profiles", target, class_name, "default")
      - slot: pick(prof.slots)
      - roll: beta(prof.a, prof.b)
    result: base * prof.scale * slot.mult * (0.5 + roll)
    min: 1
`)
	progs, _, errs := ParseFile(data, func(n string) bool { return n == "toy" }, testVars)
	if len(errs) > 0 {
		b.Fatal(errs)
	}
	p := progs["toy"]
	r := rand.New(rand.NewSource(1))
	e := env(Number(100), Number(50), Number(40), String("juggler"), String("nobody"))
	e.Rand = RandSource{Float64: r.Float64, NormFloat64: r.NormFloat64}
	b.ReportAllocs()
	for b.Loop() {
		if _, err := p.Run(e); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkArithmetic(b *testing.B) {
	p, err := Compile("clamp(base * (1 + (level - x) / 50) + (x > 3 ? 10 : 0), 1, 100000)", testVars)
	if err != nil {
		b.Fatal(err)
	}
	e := env(Number(100), Number(50), Number(40))
	b.ReportAllocs()
	for b.Loop() {
		_, _ = p.Run(e)
	}
}
