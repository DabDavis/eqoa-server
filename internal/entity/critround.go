package entity

import "time"

// critround.go — a creature's critical hit in place of a round (work 1340, the
// GM Jibrish's instruction of 2026-11-13: "all NPC crits replace a normal quad
// round and open every engagement").
//
// A creature with a timed special (its class template's special_every, the
// roamer's Critical Hit, the Plane of Sky twins') and no owner fights its
// player or pet in rounds as ever (rounds.go), and at each round's start picks
// what the round is: its crit, when the crit is ready, else its normal round.
// A crit round sends none of the normal round's swing and lands none of its
// hits: at the round's first hit time (the normal impact, 0.5 s in) the crit
// lands alone, through templateSpecialHitLocked as the special always did (its
// 0x00B1 of cast 0 with its visual, the level times special_damage). The next
// round, on the same cadence, is its normal round again unless the crit is
// ready once more. The crit is ready at the start of every fight (a new
// engagement: the fight's end forgets it, endTemplateFight), so a creature's
// first attack is its crit, whatever its special_first; after each it is ready
// again special_every later (special_jitter either way), counted as the timed
// special's clock always was, on the creature's time AT its enemy (its turns in
// reach, templateContactGap apart at most; none while a cast holds it). A crit
// that comes ready between two rounds waits for the next round's start. With a
// 6 s round and a 10 s crit: crit, quad, crit, quad, crit at 0, 6, 12, 18 and
// 24 s.
//
// A crit round whose impact goes by out of reach (its enemy stepped away:
// loseDueHits) loses its crit unlanded, and the crit is ready again at once:
// what was spent is the one that landed. A crit round's per-round chance special
// (special_chance) is not rolled: the crit is the round. A target changed in
// the fight is no new engagement: the crit's clock runs on.
//
// What is left on the old clock: a creature's fight with another NPC
// (guards.go, a trigger's), which keeps templateSpecialLocked's own clock, and
// a player's pet's specials (pettemplate.go), which are no creature's crits.

// critRounds reports a creature whose timed special comes as a round of its
// own (the file's doc): a template's timed special of some damage, at its level,
// on a creature of no owner.
func (n *NPC) critRounds() bool {
	t := n.tmpl
	return t != nil && n.Owner == 0 && t.SpecialEvery > 0 && t.SpecialDamage > 0 && n.Level >= t.SpecialFrom
}

// critClock counts the creature's time at its enemy on its crit's clock
// since its last turn in reach, a fight's first turn finding its crit ready.
// Must be called with the lock held.
func critClock(f *templateFight, now time.Time) {
	switch gap := now.Sub(f.timedSeen); {
	case f.timedSeen.IsZero():
		f.timedLeft = 0 // a new engagement opens with its crit
	case gap > 0 && gap <= templateContactGap:
		f.timedLeft -= gap // at its enemy since its last turn
	}
	f.timedSeen = now
}

// critRoundLocked is what a creature's round starting now is: its crit (true,
// the crit's readiness spent) or its normal round. Must be called with the lock
// held, at the round's start.
func (w *World) critRoundLocked(npc *NPC, now time.Time) bool {
	if !npc.critRounds() {
		return false
	}
	f := npc.fight()
	f.critRounds = true
	if npc.castHeld(now) {
		return false // its cast holds it: no clock counts and no special comes (templateSpecialLocked)
	}
	critClock(f, now)
	if f.timedLeft > 0 {
		return false
	}
	t := npc.tmpl
	f.timedLeft = templateJitter(t.SpecialEvery, t.SpecialJitter)
	return true
}

// npcRoundStartLocked starts the creature's round at target now: a crit round
// sends no swing (its crit's own 0x00B1 comes at its impact), a normal one its
// swing, none while it casts. Must be called with the lock held.
func (w *World) npcRoundStartLocked(npc *NPC, target EntityID, now time.Time, delay time.Duration, fx uint32) {
	npc.RoundHits = 0
	npc.RoundCrit = w.critRoundLocked(npc, now)
	if !npc.RoundCrit {
		w.npcSwingLocked(npc, target, uint32(delay/time.Millisecond), fx, now) // none while it casts (classtemplate.go)
	}
}

// critDue reports a crit round whose impact (its first hit's time) is due now.
func (n *NPC) critDue(hits []time.Duration, now time.Time) bool {
	return n.RoundCrit && n.RoundHits == 0 && len(hits) > 0 && !now.Before(n.RoundStart.Add(hits[0]))
}

// critStrikeLocked lands a crit round's crit on target, the round's hits spent
// with it. Must be called with the lock held.
func (w *World) critStrikeLocked(npc *NPC, target EntityID, hits []time.Duration, now time.Time) {
	npc.RoundHits = len(hits)
	w.critHitLocked(npc, target, now)
}

// critHitLocked is a creature's crit on target (a player, a pet). Must be
// called with the lock held.
func (w *World) critHitLocked(npc *NPC, target EntityID, now time.Time) {
	if w.wereCritLocked(npc, target, now) {
		return // a were's crit with its infection (werecrit.go)
	}
	w.templateSpecialHitLocked(npc, target, now)
}

// loseCrit is a crit round's impact gone by out of reach: unlanded, the round
// spent, the crit ready again.
func (n *NPC) loseCrit(hits []time.Duration) {
	n.RoundHits = len(hits)
	if n.tfight != nil {
		n.tfight.timedLeft = 0
	}
}
