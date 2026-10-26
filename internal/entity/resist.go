package entity

import (
	"fmt"
	"math/rand"
)

// resist.go — what a player's resist takes off a spell's damage, as jib has
// the live game's (F3 20261026-015346): "if you have 315 cap, it hits you
// for 75%", and at that cap "a 2.5% chance to completely fail and hit for
// full, and an additional 5% shot to roll a partial", the partial "between
// 74% and 45%". Read here as: a resist counts up to ResistCap and takes
// ResistCapShare of the damage there, in proportion below it (the line below
// the cap is ours); at the cap a spell's resist fails altogether 2.5% of the
// time (all of it lands) and in part another 5% (it takes a share between
// 45% and 74%), else it takes its 75%. His dice ("if both dice hit, the full
// failure works. Otherwise, the partial failure") come to the same odds.
// Used where a resist cuts a spell: a sparring dummy's element strike
// (sparring.go) and a pact's price on its caster (lifetap.go). Until then the
// share was resist / (resist + 200), with no cap (200 took half, 600 took 75%).

const (
	// ResistCap is the most of a resist that counts (jib's "315 cap").
	ResistCap = 315
	// ResistCapShare is the share a resist at the cap takes.
	ResistCapShare = 0.75

	resistFullFailure    = 0.025 // a resist at the cap fails altogether
	resistPartialFailure = 0.05  // and, besides, in part
	resistPartialLow     = 0.45  // a partial failure's share, from
	resistPartialHigh    = 0.74  // to
)

// ResistOutcome is how a resist went on one spell.
type ResistOutcome int

const (
	ResistHeld    ResistOutcome = iota // it took its share
	ResistPartial                      // at the cap, it failed in part
	ResistFailed                       // at the cap, it failed altogether
)

// resistRand is the resist's dice (tests replace it).
var resistRand = rand.Float64

// ResistShare is the share of a spell's damage a resist takes: in
// proportion up to ResistCapShare at ResistCap, no more past it.
func ResistShare(resist int) float64 {
	r := min(max(resist, 0), ResistCap)
	return ResistCapShare * float64(r) / ResistCap
}

// RollResist is the share a resist takes off one spell's damage: its
// ResistShare, but at the cap the dice may fail it, wholly or in part.
func RollResist(resist int) (float64, ResistOutcome) {
	share := ResistShare(resist)
	if resist < ResistCap {
		return share, ResistHeld
	}
	switch d := resistRand(); {
	case d < resistFullFailure:
		return 0, ResistFailed
	case d < resistFullFailure+resistPartialFailure:
		return resistPartialLow + resistRand()*(resistPartialHigh-resistPartialLow), ResistPartial
	}
	return share, ResistHeld
}

// resistWords says in the chat what a resist took.
func resistWords(share float64, how ResistOutcome) string {
	switch how {
	case ResistFailed:
		return "your resist failed"
	case ResistPartial:
		return fmt.Sprintf("your resist took only %.0f%%", share*100)
	}
	return fmt.Sprintf("your resist took %.0f%%", share*100)
}
