package entity

// dungeonlockout.go — a wait before a new instance of the same dungeon (board
// entry 1261, a GM's decision 2026-11-12: "ask for a 10 minute timer").
//
// Without it a heroic instance could be closed with /dungeon reset and a fresh
// one opened at once, every named back, so a rare drop could be farmed as fast
// as a group runs to its creature and back.
//
// When an instance closes (/dungeon reset, close_after with no one inside, or
// anything else that closes one), every character who stood in it, its opener
// too, is locked out of that dungeon for world.dungeons.lockout (10m unless
// set; 0 turns it off). While locked out a character may not open a new
// instance of it, nor enter one opened after its lockout began; it may go back
// into an instance that is still open and was opened before (its own group's
// included), and enter any other dungeon. A refusal says the minutes left,
// rounded up, and /dungeon lists each lockout with its minutes.
//
// Lockouts live in memory, as the instances do: a cold start clears both. A
// restart that keeps the sessions keeps both (keepdungeon.go: each instance's
// characters who stood in it, and every lockout still running).

import (
	"fmt"
	"log"
	"sort"
	"time"
)

// DefaultDungeonLockout is world.dungeons.lockout unset.
const DefaultDungeonLockout = 10 * time.Minute

// dungeonLineLocked is a refused entry's line (ours, the entry's words).
const dungeonLineLocked = "You may enter a new instance of %s in %d minutes."

// dungeonLock keys a lockout: a character and a dungeon (by key, so a recipe reordered still maps).
type dungeonLock struct {
	char uint32
	key  string
}

// markStood records that a character stood in the instance.
func (in *dungeonInst) markStood(char uint32) {
	if char == 0 {
		return
	}
	if in.stood == nil {
		in.stood = map[uint32]bool{}
	}
	in.stood[char] = true
}

// dungeonNowLocked is the time for a call that takes none.
func (w *World) dungeonNowLocked() time.Time {
	if w.dungeons.clock != nil {
		return w.dungeons.clock()
	}
	return time.Now()
}

// dungeonLockOutLocked locks everyone who stood in a closing instance out of its dungeon from now.
func (w *World) dungeonLockOutLocked(in *dungeonInst, now time.Time) {
	if w.dungeons.set.Lockout <= 0 || in == nil {
		return
	}
	if w.dungeons.lockouts == nil {
		w.dungeons.lockouts = map[dungeonLock]time.Time{}
	}
	in.markStood(in.owner)
	key := w.dungeons.defs[in.def].Key
	for c := range in.stood {
		w.dungeons.lockouts[dungeonLock{c, key}] = now
	}
	log.Printf("Dungeons: %d characters of instance %d locked out of %s for %v", len(in.stood), in.id,
		w.dungeons.defs[in.def].Name, w.dungeons.set.Lockout)
}

// dungeonLockLeftLocked is how long char's lockout from dungeon def runs past now, and when it began (0: none).
func (w *World) dungeonLockLeftLocked(char uint32, def int, now time.Time) (time.Duration, time.Time) {
	if w.dungeons.set.Lockout <= 0 {
		return 0, time.Time{}
	}
	began, ok := w.dungeons.lockouts[dungeonLock{char, w.dungeons.defs[def].Key}]
	if !ok {
		return 0, time.Time{}
	}
	left := began.Add(w.dungeons.set.Lockout).Sub(now)
	if left <= 0 {
		return 0, time.Time{}
	}
	return left, began
}

// dungeonLockedOutLocked is whether char may not go into dungeon def now: in is the open instance it would enter
// (nil: it would open one). An instance opened before its lockout began lets it back in.
func (w *World) dungeonLockedOutLocked(char uint32, def int, in *dungeonInst, now time.Time) (string, bool) {
	left, began := w.dungeonLockLeftLocked(char, def, now)
	if left <= 0 {
		return "", false
	}
	if in != nil && in.opened.Before(began) {
		return "", false // back into an instance still open from before
	}
	return fmt.Sprintf(dungeonLineLocked, w.dungeons.defs[def].Name, dungeonLockMinutes(left)), true
}

// dungeonLockMinutes is a wait in whole minutes, rounded up.
func dungeonLockMinutes(d time.Duration) int {
	return int((d + time.Minute - 1) / time.Minute)
}

// dungeonLockoutLinesLocked is /dungeon's line for each lockout char has (the dungeons' order).
func (w *World) dungeonLockoutLinesLocked(char uint32, now time.Time) []string {
	var out []string
	for i, d := range w.dungeons.defs {
		if left, _ := w.dungeonLockLeftLocked(char, i, now); left > 0 {
			out = append(out, fmt.Sprintf("Locked out of new instances of %s for %d more minutes.", d.Name,
				dungeonLockMinutes(left)))
		}
	}
	return out
}

// dungeonLockoutsPruneLocked drops lockouts that have run out.
func (w *World) dungeonLockoutsPruneLocked(now time.Time) {
	for k, began := range w.dungeons.lockouts {
		if !now.Before(began.Add(w.dungeons.set.Lockout)) {
			delete(w.dungeons.lockouts, k)
		}
	}
}

// KeptDungeonLock is a lockout a keep restart carries.
type KeptDungeonLock struct {
	Char  uint32    `json:"char"`
	Key   string    `json:"key"`
	Began time.Time `json:"began"`
}

// keptLockoutsLocked is every lockout still running at now, in order.
func (w *World) keptLockoutsLocked(now time.Time) []KeptDungeonLock {
	var out []KeptDungeonLock
	for k, began := range w.dungeons.lockouts {
		if w.dungeons.set.Lockout > 0 && now.Before(began.Add(w.dungeons.set.Lockout)) {
			out = append(out, KeptDungeonLock{Char: k.char, Key: k.key, Began: began})
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Char != out[j].Char {
			return out[i].Char < out[j].Char
		}
		return out[i].Key < out[j].Key
	})
	return out
}
