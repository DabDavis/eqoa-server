package entity

import (
	"fmt"
	"log"
	"math"
	"time"

	"github.com/eqoa/server/internal/session"
)

// stuck.go — /stuck (the user, 2026-11-08: "zaeire is stuck, and we can add /stuck also"): a player
// sends itself home as a gate sends its caster (the gate hook: where a spiritmaster bound it, else
// where every login starts), once every StuckEvery. Refused, saying why: dead or about to rise, a
// creature fighting it, its PvP flag held from a fight (pvp.go's hold), or too soon since the last.

// StuckEvery is how often a player may use /stuck (ours).
const StuckEvery = 10 * time.Minute

// The lines (ours).
const (
	stuckLineDead  = "You cannot do that now."
	stuckLineFight = "You cannot use /stuck while you are in a fight."
	stuckLineSoon  = "You can use /stuck again in %d %s."
)

// Stuck is sess's player's /stuck: home now, or the line refusing it ("" when it went). Call it
// without the lock.
func (w *World) Stuck(sess *session.Session, now time.Time) string {
	w.mu.Lock()
	p := w.sessionPlayerLocked(sess)
	if p == nil {
		w.mu.Unlock()
		return ""
	}
	var why string
	switch {
	case p.IsDead() || p.respawnPending():
		why = stuckLineDead
	case w.fightingLocked(p) > 0 || (p.pvp.on && now.Before(p.pvp.hold)):
		why = stuckLineFight
	case now.Before(p.stuckNext):
		n, unit := int(math.Ceil(p.stuckNext.Sub(now).Minutes())), "minutes"
		if n == 1 {
			unit = "minute"
		}
		why = fmt.Sprintf(stuckLineSoon, n, unit)
	}
	if why != "" {
		w.mu.Unlock()
		log.Printf("Stuck: %s refused: %s", p.Name, why)
		return why
	}
	p.stuckNext = now.Add(StuckEvery)
	log.Printf("Stuck: %s at (%.1f, %.1f, %.1f) world %d goes home", p.Name, p.X, p.Y, p.Z, p.World)
	due := []respawnDue{{sess: sess, p: p}}
	w.mu.Unlock()
	w.runGates(due)
	return ""
}
