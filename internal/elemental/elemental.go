// Copied from the elemental resistance research package of 2026-11-14 (EQOA-Elemental-Resistance-for-Dab-2026-11-14,
// reference-go/resist.go, prepared for this server by a GM collaborator through the owner), unchanged but for
// the package name. The server adapter is entity/elemental.go.

// Package elemental provides a provisional, single-element damage calculation.
package elemental

import (
	"fmt"
	"math"
)

// Target selects the defender's resistance scale.
type Target string

const (
	Player Target = "player"
	NPC    Target = "npc"
)

// Mode selects a player-only resistance scenario. Its zero value uses
// ModeMagnitudeOnly; the other modes must be explicitly selected.
type Mode string

const (
	ModeMagnitudeOnly    Mode = "magnitude_only"
	ModeFixedFullChance  Mode = "fixed_full_chance"
	ModeResistanceScaled Mode = "resistance_scaled"
)

// Outcome names the branch selected, even when both branches deal equal damage.
type Outcome string

const (
	OutcomeReduced Outcome = "reduced"
	OutcomeFull    Outcome = "full"
)

// Input describes one elemental effect, after its base/stat calculation.
// Resistance must already be the effective value for the chosen element.
// For stochastic player modes, Roll must point to a caller-owned value in
// [0, 1). FixedFullProbability is used only by ModeFixedFullChance on players.
// NPC calculations do not use Roll or FixedFullProbability.
type Input struct {
	Target               Target
	D0                   float64
	Resistance           float64
	Mode                 Mode
	FixedFullProbability float64
	Roll                 *float64
}

// Result contains the reduced branch, the selected branch, and its mean over
// repeated draws under the chosen scenario. ReducedProbability describes the
// chance of selecting the reduced branch, not necessarily a visible difference.
type Result struct {
	ReducedDamage      float64
	SampledDamage      float64
	ExpectedDamage     float64
	ReducedProbability float64
	Outcome            Outcome
}

func finite(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0)
}

// Calculate applies a provisional response to one elemental effect. A player
// roll strictly below the full-hit probability selects the full-damage branch;
// equality selects the reduced branch. A positive reduced result that underflows
// to zero returns an error. No random state is read or changed.
func Calculate(in Input) (Result, error) {
	var divisor, maximum float64
	switch in.Target {
	case Player:
		divisor, maximum = 350, 315
	case NPC:
		divisor, maximum = 100, 100
	default:
		return Result{}, fmt.Errorf("invalid target %q", in.Target)
	}

	if !finite(in.D0) || in.D0 < 0 {
		return Result{}, fmt.Errorf("D0 must be finite and nonnegative")
	}
	if !finite(in.Resistance) || in.Resistance < 0 || in.Resistance > maximum {
		return Result{}, fmt.Errorf("resistance must be finite and in [0, %g]", maximum)
	}

	mode := in.Mode
	if mode == "" {
		mode = ModeMagnitudeOnly
	}
	switch mode {
	case ModeMagnitudeOnly, ModeFixedFullChance, ModeResistanceScaled:
	default:
		return Result{}, fmt.Errorf("invalid mode %q", mode)
	}

	// Divide first to avoid overflowing D0 while evaluating the numerator.
	reduced := in.D0 * ((divisor - in.Resistance) / divisor)
	if in.D0 > 0 && in.Resistance < divisor && reduced == 0 {
		return Result{}, fmt.Errorf("positive reduced damage underflows float64")
	}
	result := Result{
		ReducedDamage:      reduced,
		SampledDamage:      reduced,
		ExpectedDamage:     reduced,
		ReducedProbability: 1,
		Outcome:            OutcomeReduced,
	}
	if in.Target == NPC || mode == ModeMagnitudeOnly {
		return result, nil
	}

	var fullProbability float64
	switch mode {
	case ModeFixedFullChance:
		if !finite(in.FixedFullProbability) || in.FixedFullProbability < 0 || in.FixedFullProbability > 1 {
			return Result{}, fmt.Errorf("fixed full-hit probability must be finite and in [0, 1]")
		}
		fullProbability = in.FixedFullProbability
		result.ReducedProbability = 1 - fullProbability
	case ModeResistanceScaled:
		result.ReducedProbability = in.Resistance / divisor
		fullProbability = (divisor - in.Resistance) / divisor
	}

	if in.Roll == nil || !finite(*in.Roll) || *in.Roll < 0 || *in.Roll >= 1 {
		return Result{}, fmt.Errorf("stochastic player mode requires a finite roll in [0, 1)")
	}

	// Preserve the deterministic endpoint exactly. Otherwise this equivalent
	// form keeps the mean bounded for very large finite D0.
	if result.ReducedProbability != 1 {
		result.ExpectedDamage = in.D0 - result.ReducedProbability*(in.D0-reduced)
	}
	if *in.Roll < fullProbability {
		result.SampledDamage = in.D0
		result.Outcome = OutcomeFull
	}
	return result, nil
}
