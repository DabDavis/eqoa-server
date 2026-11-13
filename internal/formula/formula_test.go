package formula

import (
	"errors"
	"math"
	"strings"
	"testing"
)

var testVars = map[string]int{"base": 0, "level": 1, "x": 2}

func eval(t *testing.T, src string, v ...float64) float64 {
	t.Helper()
	p, err := Compile(src, testVars)
	if err != nil {
		t.Fatalf("Compile(%q): %v", src, err)
	}
	vals := make([]float64, len(testVars))
	copy(vals, v)
	return p.Eval(vals)
}

func TestOperators(t *testing.T) {
	cases := []struct {
		src  string
		v    []float64
		want float64
	}{
		{"base * 2", []float64{10}, 20},
		{"1 + 2 * 3", nil, 7},
		{"(1 + 2) * 3", nil, 9},
		{"10 - 4 - 3", nil, 3},
		{"2 ^ 3 ^ 2", nil, 512},
		{"-2 ^ 2", nil, -4},
		{"7 % 4", nil, 3},
		{"base > 5 ? 1 : 2", []float64{6}, 1},
		{"base > 5 ? 1 : base > 2 ? 3 : 4", []float64{3}, 3},
		{"1 < 2 && 2 < 3", nil, 1},
		{"1 > 2 || !0", nil, 1},
		{"base == 3", []float64{3}, 1},
		{"base != 3", []float64{3}, 0},
		{"min(5, base, 9)", []float64{7}, 5},
		{"max(level, 1)", []float64{0, 0}, 1},
		{"clamp(base, 1, 10)", []float64{50}, 10},
		{"if(x, 2, 3)", []float64{0, 0, 0}, 3},
		{"round(2.5) + floor(1.9) + ceil(1.1) + trunc(-1.5)", nil, 3 + 1 + 2 - 1},
		{"abs(-3) + sqrt(16) + pow(2, 10)", nil, 3 + 4 + 1024},
		{"sign(-5) + sign(0) + sign(2)", nil, 0},
		{"1.5e2 + .5", nil, 150.5},
		{"base *\n 2", []float64{4}, 8},
	}
	for _, c := range cases {
		if got := eval(t, c.src, c.v...); got != c.want {
			t.Errorf("%q = %v, want %v", c.src, got, c.want)
		}
	}
}

func TestNaNAndInfReturned(t *testing.T) {
	if got := eval(t, "base / 0", 1); !math.IsInf(got, 1) {
		t.Errorf("1/0 = %v", got)
	}
	if got := eval(t, "sqrt(0 - base)", 1); !math.IsNaN(got) {
		t.Errorf("sqrt(-1) = %v", got)
	}
}

func TestShortSliceIsNaNNotPanic(t *testing.T) {
	p, err := Compile("x + 1", testVars)
	if err != nil {
		t.Fatal(err)
	}
	if got := p.Eval([]float64{1}); !math.IsNaN(got) {
		t.Errorf("short slice = %v, want NaN", got)
	}
}

func TestUsesAndConstant(t *testing.T) {
	p, _ := Compile("x + x * level", testVars)
	if len(p.Uses()) != 2 || p.Constant() {
		t.Errorf("uses %v constant %v", p.Uses(), p.Constant())
	}
	p, _ = Compile("max(1, 2) * 3", testVars)
	if !p.Constant() || p.Eval(nil) != 6 {
		t.Errorf("constant %v = %v", p.Constant(), p.Eval(nil))
	}
}

// A compile error says where and what, never the formula's own text: not a
// name it did not know, not a number, not a fragment.
func TestErrorsNeverQuoteTheFormula(t *testing.T) {
	secrets := []string{
		"base * zorblax",
		"base * frobnicate(2)",
		"base * (31337 +",
		"base * 31337 31337",
		"base $ 2",
		"min()",
		"pow(1)",
		"1 ? 2",
		"",
	}
	for _, src := range secrets {
		_, err := Compile(src, testVars)
		if err == nil {
			t.Errorf("%q compiled", src)
			continue
		}
		var fe *Error
		if !errors.As(err, &fe) || fe.Col < 1 {
			t.Errorf("%q: %v is not a positioned *Error", src, err)
		}
		msg := err.Error()
		for _, frag := range []string{"zorblax", "frobnicate", "31337", "$"} {
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
}

func TestParseFile(t *testing.T) {
	known := func(n string) bool { return n == "a" || n == "b" }
	src := "# a comment\n\na = base * 2\na.min = 1\na.max = level * \\\n  100\nb = 3\n"
	got, errs := ParseFile([]byte(src), known, testVars)
	if len(errs) > 0 {
		t.Fatal(errs)
	}
	if len(got) != 2 || got["a"].Min == nil || got["a"].Max == nil || got["b"].Min != nil {
		t.Fatalf("entries %+v", got)
	}
	if v := got["a"].Max.Eval([]float64{0, 2, 0}); v != 200 {
		t.Errorf("continued line = %v", v)
	}
}

func TestParseFileErrors(t *testing.T) {
	known := func(n string) bool { return n == "a" }
	cases := []string{
		"a base * 2",          // no =
		"c = 1",               // unknown name
		"a = 1\na = 2",        // duplicate
		"a.mid = 1",           // bad part
		"a.min = 1",           // clamp with no formula
		"a = base * hush1234", // unknown variable
		"A = 1",               // not lower case
	}
	for _, src := range cases {
		got, errs := ParseFile([]byte(src), known, testVars)
		if len(errs) == 0 || got != nil {
			t.Errorf("%q: no error (%v)", src, got)
		}
		for _, e := range errs {
			if strings.Contains(e.Error(), "hush1234") || strings.Contains(e.Error(), "base *") {
				t.Errorf("%q: error %q quotes the formula", src, e)
			}
		}
	}
	_, errs := ParseFile([]byte("a = base +\nc = 1\n"), known, testVars)
	if len(errs) != 2 {
		t.Errorf("want every mistake, got %v", errs)
	}
	_, errs = ParseFile([]byte("\n\na = 1 +\\\n  (2"), known, testVars)
	if len(errs) != 1 || !strings.Contains(errs[0].Error(), "line 4") {
		t.Errorf("continued-line error placed %v", errs)
	}
}

func BenchmarkEval(b *testing.B) {
	p, err := Compile("clamp(base * (1 + (level - x) / 50) + (x > 3 ? 10 : 0), 1, 100000)", testVars)
	if err != nil {
		b.Fatal(err)
	}
	v := []float64{100, 50, 40}
	b.ReportAllocs()
	for b.Loop() {
		_ = p.Eval(v)
	}
}
