package entity

import (
	"log"
	"time"

	"github.com/eqoa/server/internal/session"
)

// questdeath.go — a quest's price paid with the player's life (the quest
// engine's death event, handlers/questevents.go).
//
// Heirloom petition 3488 (Nathan, 2026-11-28; the owner approved it as asked):
// in SOE's game turning in Envaric Ascension killed the player ("Its price is
// your life"), who woke at the spirit master and then had the port spell
// told. The death here is a death as any other on the client (0x005E, the
// death camera, the respawn at the bind point PlayerRespawnDelay later), but
// it costs nothing: no experience debt, no tunar, and it is never a Hardcore
// character's final death (the quest asks it; nothing killed the player).

// QuestDeath lays p down dead as a quest's price, told to sess (its own
// session). It reports false for a player not here, dead already, or whose
// death is held off (a duel's arena, a GM's test mode).
func (w *World) QuestDeath(sess *session.Session, p *Player) bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.questDeathLocked(sess, p, time.Now())
}

func (w *World) questDeathLocked(sess *session.Session, p *Player, now time.Time) bool {
	if p == nil || w.Players[p.ServerID] != p || p.IsDead() || p.Fallen {
		return false
	}
	p.priceDeath = true
	w.playerFallsLocked(sess, p, now, 0, false)
	p.priceDeath = false
	if !p.respawnPending() {
		return false
	}
	p.StatsDirty = true
	log.Printf("Death: %s gave their life as a quest's price (no debt, no tunar, never a Hardcore fall)", p.Name)
	return true
}
