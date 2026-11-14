package elemental

import (
	"math"
	"testing"
)

func ptr(value float64) *float64 { return &value }

func closeEnough(a, b float64) bool {
	return math.Abs(a-b) <= 1e-9
}

func checkResult(t *testing.T, in Input, want Result) {
	t.Helper()
	got, err := Calculate(in)
	if err != nil {
		t.Fatalf("Calculate(%+v): %v", in, err)
	}
	if !closeEnough(got.ReducedDamage, want.ReducedDamage) ||
		!closeEnough(got.SampledDamage, want.SampledDamage) ||
		!closeEnough(got.ExpectedDamage, want.ExpectedDamage) ||
		!closeEnough(got.ReducedProbability, want.ReducedProbability) || got.Outcome != want.Outcome {
		t.Fatalf("Calculate(%+v) = %+v; want %+v", in, got, want)
	}
}

func TestPlayerMagnitudeOnly(t *testing.T) {
	for _, tc := range []struct {
		name       string
		resistance float64
		want       float64
	}{
		{"zero", 0, 1000},
		{"middle", 175, 500},
		{"cap", 315, 100},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// Empty mode is the documented default.
			checkResult(t, Input{Target: Player, D0: 1000, Resistance: tc.resistance},
				Result{tc.want, tc.want, tc.want, 1, OutcomeReduced})
			checkResult(t, Input{Target: Player, D0: 1000, Resistance: tc.resistance, Mode: ModeMagnitudeOnly},
				Result{tc.want, tc.want, tc.want, 1, OutcomeReduced})
		})
	}
}

func TestFixedFullChanceAndBoundary(t *testing.T) {
	base := Input{Target: Player, D0: 1000, Resistance: 315, Mode: ModeFixedFullChance, FixedFullProbability: 0.1}
	base.Roll = ptr(math.Nextafter(0.1, 0))
	checkResult(t, base, Result{100, 1000, 190, 0.9, OutcomeFull})
	base.Roll = ptr(0.1) // Equality is the reduced branch.
	checkResult(t, base, Result{100, 100, 190, 0.9, OutcomeReduced})
	base.FixedFullProbability = 0
	base.Roll = ptr(0)
	checkResult(t, base, Result{100, 100, 100, 1, OutcomeReduced})
	base.D0 = 0.01
	zeroChance, err := Calculate(base)
	if err != nil || zeroChance.ExpectedDamage != zeroChance.ReducedDamage {
		t.Fatalf("zero failure probability: result=%+v error=%v", zeroChance, err)
	}
	base.D0 = 1000
	base.FixedFullProbability = 1
	base.Roll = ptr(math.Nextafter(1, 0))
	checkResult(t, base, Result{100, 1000, 1000, 0, OutcomeFull})
}

func TestResistanceScaledAndBoundary(t *testing.T) {
	base := Input{Target: Player, D0: 1000, Resistance: 315, Mode: ModeResistanceScaled}
	base.Roll = ptr(math.Nextafter(0.1, 0))
	checkResult(t, base, Result{100, 1000, 190, 0.9, OutcomeFull})
	base.Roll = ptr(0.1)
	checkResult(t, base, Result{100, 100, 190, 0.9, OutcomeReduced})
	base.Resistance = 175
	base.Roll = ptr(0.5)
	checkResult(t, base, Result{500, 500, 750, 0.5, OutcomeReduced})
	base.Resistance = 0
	base.Roll = ptr(0.5)
	checkResult(t, base, Result{1000, 1000, 1000, 0, OutcomeFull})
}

func TestNPCDeterministicForEveryPlayerMode(t *testing.T) {
	for _, mode := range []Mode{"", ModeMagnitudeOnly, ModeFixedFullChance, ModeResistanceScaled} {
		t.Run(string(mode), func(t *testing.T) {
			// Player-only fields are ignored for an NPC, including invalid draws.
			in := Input{Target: NPC, D0: 1000, Resistance: 100, Mode: mode, FixedFullProbability: math.NaN(), Roll: ptr(-1)}
			checkResult(t, in, Result{0, 0, 0, 1, OutcomeReduced})
			in.Resistance = 50
			checkResult(t, in, Result{500, 500, 500, 1, OutcomeReduced})
		})
	}
}

func TestInvalidInputs(t *testing.T) {
	valid := Input{Target: Player, D0: 1000, Resistance: 315}
	for _, tc := range []struct {
		name string
		edit func(*Input)
	}{
		{"missing target", func(i *Input) { i.Target = "" }},
		{"invalid target", func(i *Input) { i.Target = "boss" }},
		{"negative damage", func(i *Input) { i.D0 = -1 }},
		{"nan damage", func(i *Input) { i.D0 = math.NaN() }},
		{"positive infinite damage", func(i *Input) { i.D0 = math.Inf(1) }},
		{"negative infinite damage", func(i *Input) { i.D0 = math.Inf(-1) }},
		{"negative resistance", func(i *Input) { i.Resistance = -1 }},
		{"player over cap", func(i *Input) { i.Resistance = 316 }},
		{"NPC over cap", func(i *Input) { i.Target = NPC; i.Resistance = 101 }},
		{"nan resistance", func(i *Input) { i.Resistance = math.NaN() }},
		{"positive infinite resistance", func(i *Input) { i.Resistance = math.Inf(1) }},
		{"negative infinite resistance", func(i *Input) { i.Resistance = math.Inf(-1) }},
		{"invalid mode", func(i *Input) { i.Mode = "unknown" }},
		{"invalid NPC mode", func(i *Input) { i.Target = NPC; i.Resistance = 100; i.Mode = "unknown" }},
		{"negative fixed chance", func(i *Input) { i.Mode = ModeFixedFullChance; i.FixedFullProbability = -0.1; i.Roll = ptr(0) }},
		{"fixed chance over one", func(i *Input) { i.Mode = ModeFixedFullChance; i.FixedFullProbability = 1.1; i.Roll = ptr(0) }},
		{"nan fixed chance", func(i *Input) { i.Mode = ModeFixedFullChance; i.FixedFullProbability = math.NaN(); i.Roll = ptr(0) }},
		{"infinite fixed chance", func(i *Input) { i.Mode = ModeFixedFullChance; i.FixedFullProbability = math.Inf(1); i.Roll = ptr(0) }},
		{"missing roll", func(i *Input) { i.Mode = ModeResistanceScaled }},
		{"negative roll", func(i *Input) { i.Mode = ModeResistanceScaled; i.Roll = ptr(-0.1) }},
		{"roll at one", func(i *Input) { i.Mode = ModeResistanceScaled; i.Roll = ptr(1) }},
		{"nan roll", func(i *Input) { i.Mode = ModeResistanceScaled; i.Roll = ptr(math.NaN()) }},
		{"infinite roll", func(i *Input) { i.Mode = ModeResistanceScaled; i.Roll = ptr(math.Inf(1)) }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			in := valid
			tc.edit(&in)
			if got, err := Calculate(in); err == nil {
				t.Fatalf("Calculate(%+v) = %+v; want an error", in, got)
			}
		})
	}
}

func TestFiniteLimitsAndZeroDamage(t *testing.T) {
	checkResult(t, Input{Target: Player, D0: 0, Resistance: 315}, Result{0, 0, 0, 1, OutcomeReduced})
	in := Input{Target: Player, D0: math.MaxFloat64, Resistance: 315, Mode: ModeFixedFullChance, FixedFullProbability: 0.1, Roll: ptr(0.5)}
	got, err := Calculate(in)
	if err != nil || !finite(got.ReducedDamage) || !finite(got.ExpectedDamage) || got.ExpectedDamage > in.D0 {
		t.Fatalf("large finite damage: result=%+v error=%v", got, err)
	}
}

func TestPositiveDamageUnderflow(t *testing.T) {
	for _, tc := range []struct {
		name   string
		target Target
		resist float64
	}{
		{"player 315", Player, 315},
		{"NPC 99", NPC, 99},
	} {
		t.Run(tc.name, func(t *testing.T) {
			in := Input{Target: tc.target, D0: math.SmallestNonzeroFloat64, Resistance: tc.resist}
			if got, err := Calculate(in); err == nil {
				t.Fatalf("unrepresentable positive damage: result=%+v; want error", got)
			}
		})
	}

	// At the NPC immunity endpoint, zero is the intended value even for tiny D0.
	got, err := Calculate(Input{Target: NPC, D0: math.SmallestNonzeroFloat64, Resistance: 100})
	if err != nil || got.ReducedDamage != 0 || got.SampledDamage != 0 {
		t.Fatalf("NPC 100 with tiny D0: result=%+v error=%v", got, err)
	}
	// A representable subnormal reduced result remains positive.
	got, err = Calculate(Input{Target: Player, D0: 10 * math.SmallestNonzeroFloat64, Resistance: 315})
	if err != nil || got.ReducedDamage != math.SmallestNonzeroFloat64 {
		t.Fatalf("representable positive damage: result=%+v error=%v", got, err)
	}
}
