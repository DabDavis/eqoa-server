package formula

import "math"

// RandSource is a Rand on two sources of the caller's: Float64 uniform in
// [0, 1) and NormFloat64 standard normal (math/rand's, or a seeded
// *rand.Rand's in a test). Beta is X/(X+Y) of two Gamma draws, each by
// Marsaglia and Tsang's method (ACM TOMS 26(3), 2000), boosted for a shape
// under 1 (Gamma(a) = Gamma(a+1)·U^(1/a)).
type RandSource struct {
	Float64     func() float64
	NormFloat64 func() float64
}

func (s RandSource) Uniform() float64 { return s.Float64() }

func (s RandSource) Normal(mu, sigma float64) float64 {
	if math.IsNaN(mu) || math.IsNaN(sigma) || sigma < 0 {
		return math.NaN()
	}
	return mu + sigma*s.NormFloat64()
}

func (s RandSource) Beta(a, b float64) float64 {
	if !(a > 0) || !(b > 0) || math.IsInf(a, 0) || math.IsInf(b, 0) {
		return math.NaN()
	}
	x, y := s.gamma(a), s.gamma(b)
	if math.IsNaN(x) || math.IsNaN(y) || x+y <= 0 {
		return math.NaN()
	}
	return x / (x + y)
}

// gammaTries bounds Marsaglia-Tsang's rejection loop (it accepts more than
// 95% of tries at any shape, so 64 failures in a row do not happen; if they
// did the draw fails, and the hit falls back, rather than looping on).
const gammaTries = 64

// gamma is a Gamma(a, 1) draw, NaN when it gives up.
func (s RandSource) gamma(a float64) float64 {
	if a < 1 {
		u := s.Float64()
		g := s.gamma(a + 1)
		return g * math.Pow(u, 1/a)
	}
	d := a - 1.0/3
	c := 1 / math.Sqrt(9*d)
	for range gammaTries {
		x := s.NormFloat64()
		v := 1 + c*x
		if v <= 0 {
			continue
		}
		v = v * v * v
		u := s.Float64()
		if u < 1-0.0331*x*x*x*x || math.Log(u) < 0.5*x*x+d*(1-v+math.Log(v)) {
			return d * v
		}
	}
	return math.NaN()
}

// Forced is a Rand that replays a fixed vector, for checking a formula (or a
// port of it) against known draws: each draw a formula makes, whatever its
// kind (uniform, normal, beta, and the uniform of a pick or pick_index),
// returns the next value of Draws as its result, unchanged (a uniform(lo, hi)
// scales it). Once Draws runs out a draw fails (NaN), so a vector too short
// for its formula shows as a failed run, not as made-up draws.
type Forced struct {
	Draws []float64
	Next  int
}

func (f *Forced) next() float64 {
	if f.Next >= len(f.Draws) {
		return math.NaN()
	}
	x := f.Draws[f.Next]
	f.Next++
	return x
}

func (f *Forced) Uniform() float64            { return f.next() }
func (f *Forced) Normal(_, _ float64) float64 { return f.next() }
func (f *Forced) Beta(_, _ float64) float64   { return f.next() }
