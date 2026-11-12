package entity

import (
	"math/rand/v2"
	"time"
)

// mercoffbeat.go — mercs off the beat of each other (the owner, 2026-11-12:
// "Jibrish's mercs are nuking in unison, let's make them offbeat a bit
// each"). Three mercs of one class at one level have one book, one cast time
// and one lockout: hired together and turned on a foe on the same tick, they
// began every spell on the same tick for the whole fight. Since then:
//
//   - a merc turned on a new foe begins its attacks (merccast.go's
//     mercAttackTurnLocked: its hits, damage over time, a tank's strikes)
//     mercReactLeast to mercReactMost after, its own moment each time (a
//     heal, a buff, a taunt and its melee swing are not held);
//   - a spell's lockout after a cast is longer by 0 to mercRecastLagMost,
//     its own each cast, so two mercs drift apart the more they cast. A
//     spell whose recast is no longer than its cast and castLockoutExtra
//     (most nukes: the lockout ends as the merc is free again) loses nothing
//     to it in a fight unless it is the only spell ready.
//
// mercOffbeat off (the tests' rigs that count casts tick for tick) takes
// both away.

var mercOffbeat = true

const (
	mercReactLeast    = 300 * time.Millisecond
	mercReactMost     = 1500 * time.Millisecond
	mercRecastLagMost = 600 * time.Millisecond
)

// mercReactedLocked is whether the merc may begin its attacks at t now: a
// new foe sets its moment (the file's doc). Must be called with the lock
// held.
func (ms *mercState) mercReactedLocked(t *NPC, now time.Time) bool {
	if !mercOffbeat || t == nil {
		return true
	}
	if ms.reactTo != t.ServerID {
		ms.reactTo = t.ServerID
		ms.reactAt = now.Add(mercReactLeast + time.Duration(rand.Int64N(int64(mercReactMost-mercReactLeast))))
	}
	return !now.Before(ms.reactAt)
}

// mercRecastLag is what a merc's lockout of s is longer by this cast (the
// file's doc).
func mercRecastLag(s *CastSpell) time.Duration {
	if !mercOffbeat {
		return 0
	}
	return time.Duration(rand.Int64N(int64(mercRecastLagMost)))
}
