package entity

import (
	"math"
	"strings"
	"sync/atomic"
	"time"

	"github.com/eqoa/server/internal/session"
)

// Slot 16 is the cropped Neriak room. The duel ID is its instance key in
// the same visibility, pet and area-effect system used by heroic dungeons.
const (
	DuelArenaWorld   uint8   = 16
	arenaX           float32 = 24637.0
	arenaZ           float32 = 8679.25
	arenaFloor       float32 = 7.6
	arenaUnavailable         = "Both players need the Neriak Duel Arena loaded and available to accept this challenge."
)

var duelArenaEnabled atomic.Bool

// IsDuelArenaWorld distinguishes the configured arena from another use of slot 16.
func IsDuelArenaWorld(world uint8) bool {
	return world == DuelArenaWorld && duelArenaEnabled.Load()
}

type arenaStay struct {
	from            GateSpot
	facing          float32
	instance        uint32 // the instance at the return point
	match           uint32
	ready, observer bool
}

// Check the actual four environment leaves before offering the arena.
func (w *World) ArenaLavaReady() bool {
	w.mu.RLock()
	defer w.mu.RUnlock()
	if w.env == nil {
		return false
	}
	for _, dx := range []float32{-16, 16} {
		for _, dz := range []float32{-16, 16} {
			f, ok := w.env.Flags(DuelArenaWorld, arenaX+dx, -12, arenaZ+dz)
			if !ok || f&envLava == 0 {
				return false
			}
		}
	}
	return true
}

func (w *World) SetDuelArena(move func(*session.Session, GateSpot, float32)) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.duels.arenaMove = move
	wasEnabled := duelArenaEnabled.Swap(move != nil)
	if move != nil || wasEnabled {
		SetInstancedWorld(DuelArenaWorld, move != nil)
	}
}

func (w *World) ArenaLoaded(sess *session.Session) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if p := w.sessionPlayerLocked(sess); p != nil && p.World == DuelArenaWorld && p.arenaStay != nil {
		p.arenaStay.ready = true
	}
}

func (w *World) arenaPairReadyLocked(a, b *Player) bool {
	if w.duels.arenaMove == nil || a.arenaStay != nil || b.arenaStay != nil || a.World == DuelArenaWorld || b.World == DuelArenaWorld || a.ctf.match != 0 || b.ctf.match != 0 {
		return false
	}
	for _, p := range []*Player{a, b} {
		s := w.findSessionByEntityIDLocked(p.ServerID)
		if s == nil || s.PendingTermination() || !s.HoldsServerWorld(DuelArenaWorld) || p.tourRide != nil {
			return false
		}
	}
	return true
}

func arenaContains(x, y, z float32) bool {
	dx, dz := math.Abs(float64(x-arenaX)), math.Abs(float64(z-arenaZ))
	return dx <= 48.3 && dz <= 48.3 && dx+dz <= 68.3 && y >= -25 && y <= 62
}

func (w *World) arenaWarpLocked(p *Player, to GateSpot, facing float32, instance uint32) {
	p.World, p.Instance, p.Facing = to.World, instance, facing
	w.putAtLocked(p, to.World, to.X, to.Y, to.Z)
	p.noteStood()
	p.mv.init = false
	p.lavaNext = time.Time{}
	if s := w.findSessionByEntityIDLocked(p.ServerID); s != nil {
		w.duels.arenaMove(s, to, facing)
	}
}

func (w *World) arenaLandLocked(p *Player, d *Duel, z, facing float32, observer bool) {
	p.arenaStay = &arenaStay{from: GateSpot{World: p.World, X: p.X, Y: p.Y, Z: p.Z}, facing: p.Facing, instance: p.Instance, match: d.ID, observer: observer}
	w.arenaWarpLocked(p, GateSpot{World: DuelArenaWorld, X: arenaX, Y: arenaFloor, Z: z}, facing, d.ID)
}

func (w *World) enterArenaLocked(d *Duel, a, b *Player, now time.Time) {
	d.Phase, d.World, d.Expires = duelCountdown, DuelArenaWorld, now.Add(90*time.Second)
	for i, p := range []*Player{a, b} {
		p.disengage()
		w.clearPlayerBuffsLocked(p)
		p.StatsDirty = true
		// Two dry positions on the centre platform, facing one another.
		w.arenaLandLocked(p, d, arenaZ+float32(2*i-1)*4, float32(i)*math.Pi, false)
	}
}

func (w *World) leaveArenaLocked(p *Player) {
	s := p.arenaStay
	if s == nil {
		return
	}
	p.arenaStay = nil
	w.arenaWarpLocked(p, s.from, s.facing, s.instance)
}

func (w *World) closeArenaLocked(d *Duel) {
	if !d.Arena {
		return
	}
	for _, p := range w.Players {
		if s := p.arenaStay; s != nil && s.match == d.ID {
			w.leaveArenaLocked(p)
		}
	}
}

// ArenaReturn supplies the durable character position while the match exists.
// Duels end on restart, so a login must use the fighter's own origin.
func (w *World) ArenaReturn(p *Player) (GateSpot, float32, bool) {
	w.mu.RLock()
	defer w.mu.RUnlock()
	if s := p.arenaStay; s != nil {
		return s.from, s.facing, true
	}
	return GateSpot{}, 0, false
}

// A world-tagged position from the room confirms arrival. Old-world reports
// during either load cannot move a fighter through the sealed perimeter.
func (w *World) arenaStepLocked(p *Player, u ClientUpdate) bool {
	if w.duels.arenaMove == nil {
		return false
	}
	if u.HasWorld && u.World == DuelArenaWorld && p.World != DuelArenaWorld {
		return true
	}
	s := p.arenaStay
	if s == nil || p.World != DuelArenaWorld {
		return false
	}
	if u.HasWorld && u.World != p.World {
		return true
	}
	if s.observer {
		return false
	}
	if !arenaContains(u.X, u.Y, u.Z) {
		w.sendMoveBackLocked(p, p.World, p.X, p.Y, p.Z)
		return true
	}
	if u.HasWorld {
		s.ready = true
	}
	return false
}

// ObserveArena is reached through the GM flight command. Party invitations,
// player travel, and GM rank without active flight confer no arena membership.
func (w *World) ObserveArena(sess *session.Session, name string) string {
	w.mu.Lock()
	defer w.mu.Unlock()
	gm := w.sessionPlayerLocked(sess)
	if gm == nil || !gm.GM || !gm.Flying {
		return "Turn on GM flight before entering a duel."
	}
	if gm.arenaStay != nil || w.duels.of[gm.ServerID] != nil || !sess.HoldsServerWorld(DuelArenaWorld) {
		return arenaUnavailable
	}
	p := w.inWorldPlayerLocked(strings.TrimSpace(name))
	if p == nil || p.arenaStay == nil || p.arenaStay.observer {
		return "That player is not in an arena duel."
	}
	d := w.duels.of[p.ServerID]
	if d == nil || !d.Arena {
		return "That duel has closed."
	}
	w.arenaLandLocked(gm, d, arenaZ+7, math.Pi, true)
	return "You are observing the Neriak duel. Turn flight off to return."
}
