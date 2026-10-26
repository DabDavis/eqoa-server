package entity

import (
	"fmt"
	"log"
	"time"

	"github.com/eqoa/server/internal/session"
)

// respawn.go — a player's death and the respawn that ends it.
//
// What the client does with a death (CLIENT, decomp viclient.cpp): Msg005E
// lays the body down and starts ResetCamera's death camera, which swings round
// the body for 4 s and then fades the screen to black over 4 s more. While
// m_pC1B0 (dead) is set, SetKey and PadKeys return at once: the player can do
// nothing. The flag is cleared in exactly three places — Init, Notify (a
// disconnect) and Msg000D, the MemoryDump, which also plays the "respawn" cue
// and fades the screen back in over a second. Msg00B2 (a teleport) moves the
// player but leaves the flag set, so on a PS2 a warp alone keeps the screen
// black and the pad dead. The dump is what ends a death.
//
// What Sandstorm sends, measured on one death in pcaps/breath_capture.pcap
// (a level 1 character drowned): 0x005E at 163.36 s; nothing more for the
// player until 178.96 s, 15.6 s later, when one packet carries the player's
// state channel, a 0x007B, the stat channel with HP back at 16/16 (its whole
// maximum; it had counted down to 0), and then the MemoryDump at the
// character's bind point. The client answers it as it answers a login's dump
// (0x0014, 0x000E, 0x00C9, 0x00CA), and Sandstorm sends nothing in reply.
//
// Here: the death drops every NPC's threat on the player and is told once;
// PlayerRespawnDelay later the tick restores HP and Power to their maximum
// and hands the player to the respawn hook (OnPlayerRespawn), which the
// handlers set to move the player to the login spawn and send the dump.

// PlayerRespawnDelay is how long a dead player waits for the respawn: the
// 15.6 s between Sandstorm's 0x005E and its MemoryDump in
// pcaps/breath_capture.pcap (one death, so one sample), rounded down to the
// second; the client's own death camera and fade take the first 8 s of it.
const PlayerRespawnDelay = 15 * time.Second

// RespawnFunc moves a player who has just been restored to the respawn point
// and tells its client. It runs on the tick goroutine after the tick, with
// the World lock released.
type RespawnFunc func(sess *session.Session, p *Player)

// respawnDue is a player whose respawn came due this tick, with the session
// to tell.
type respawnDue struct {
	sess *session.Session
	p    *Player
}

// OnPlayerRespawn sets the hook that moves a restored player and tells its
// client. With none set, a respawn restores the player where it lies.
func (w *World) OnPlayerRespawn(fn RespawnFunc) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.respawn = fn
}

// respawnPending reports a player who has died and not yet respawned.
func (p *Player) respawnPending() bool {
	return !p.DiedAt.IsZero()
}

// playerDiedLocked is a player's death: the time kept for the respawn, its
// own fight over, every NPC's threat on it dropped, and 0x005E sent — once,
// however many more hits or drowning ticks the corpse would have taken. sess
// may be nil (no client to tell). Must be called with the lock held.
func (w *World) playerDiedLocked(sess *session.Session, p *Player, now time.Time) {
	if p.respawnPending() {
		return
	}
	p.HP = 0
	p.DiedAt = now
	p.AutoAttack = false
	p.disengage()
	w.clearSpellsLocked(p)
	w.dropAggroLocked(p.ServerID)
	debt := DeathDebt(p.Level)
	p.Debt += debt
	p.noteRezChance(debt, now) // resurrect.go: what a rez may give back of it
	if sess != nil {
		w.sendPlayerDied(sess)
		if debt > 0 {
			// the debt icon on the player's own target bar (0x00D9), and ours: a line
			sendExpDebt(sess, p.Debt)
			w.sendChatToSession(sess, fmt.Sprintf("Your death left an experience debt of %d (%d owed in all).", debt, p.Debt))
		}
	}
	log.Printf("Death: %s died at level %d, %d experience debt (%d owed); respawn in %v", p.Name, p.Level, debt, p.Debt, PlayerRespawnDelay)
}

// dropAggroLocked takes the entity off every NPC's threat table. An NPC whose
// current enemy it was turns to the next highest threat, or to nothing. Must
// be called with the lock held.
func (w *World) dropAggroLocked(id EntityID) {
	for _, npc := range w.NPCs {
		if _, ok := npc.Threats[id]; ok {
			delete(npc.Threats, id)
		}
		if npc.CurrentEnemy != id {
			continue
		}
		npc.CurrentEnemy = 0
		var top int64
		for other, t := range npc.Threats {
			if t > top {
				top = t
				npc.CurrentEnemy = other
			}
		}
	}
}

// playerRespawnSystem restores every dead player whose respawn is due — HP
// and Power to their maximum, full breath, no fight — and returns them with
// their sessions for the respawn hook, which runs once the lock is let go.
// Every tick; must be called with the lock held.
func (w *World) playerRespawnSystem(now time.Time) []respawnDue {
	var due []respawnDue
	for _, p := range w.Players {
		if !p.respawnPending() || now.Sub(p.DiedAt) < PlayerRespawnDelay {
			continue
		}
		p.DiedAt = time.Time{}
		p.HP = p.MaxHP
		p.Power = p.MaxPower
		p.Breath = BreathFull
		p.DrowningCounter = 0
		p.LastCombatTime = time.Time{}
		p.AutoAttack = false
		p.disengage()
		p.VelX, p.VelY, p.VelZ = 0, 0, 0
		var sess *session.Session
		if w.SessionManager != nil {
			sess = w.findSessionByEntityIDLocked(p.ServerID)
		}
		log.Printf("Respawn: %s restored to %d/%d HP, %d/%d power", p.Name, p.HP, p.MaxHP, p.Power, p.MaxPower)
		due = append(due, respawnDue{sess: sess, p: p})
	}
	return due
}

// runRespawns hands each player whose respawn came due to the respawn hook.
// Must be called WITHOUT the lock: the hook moves the player and sends the
// dump through methods that take it.
func (w *World) runRespawns(due []respawnDue) {
	if len(due) == 0 {
		return
	}
	w.mu.RLock()
	fn := w.respawn
	w.mu.RUnlock()
	if fn == nil {
		return
	}
	for _, d := range due {
		if d.sess == nil {
			continue
		}
		fn(d.sess, d.p)
	}
}

// PlaceRespawn puts a respawned player at the respawn point, standing still.
// It returns a copy of the player as placed, for the MemoryDump: the live
// struct can be moved again at any moment by a 0x40 the client sent before it
// heard of the respawn, and a dump built from that would put the player back
// where it died. It also reports whether the respawn changed the world.
func (w *World) PlaceRespawn(id EntityID, x, y, z float32, world uint8) (placed Player, worldChanged, ok bool) {
	w.mu.Lock()
	defer w.mu.Unlock()

	p, found := w.Players[id]
	if !found {
		return Player{}, false, false
	}
	oldX, oldZ := p.X, p.Z
	worldChanged = p.World != world
	p.World = world
	p.X, p.Y, p.Z = x, y, z
	p.VelX, p.VelY, p.VelZ = 0, 0, 0
	p.noteStood()
	w.SpatialIndex.Update(id, oldX, oldZ, x, z)
	return *p, worldChanged, true
}

// PlayerSnapshot is a copy of the player as it is now, taken under the lock.
func (w *World) PlayerSnapshot(id EntityID) (Player, bool) {
	w.mu.RLock()
	defer w.mu.RUnlock()

	p, found := w.Players[id]
	if !found {
		return Player{}, false
	}
	return *p, true
}

// SyncSelfChannels writes the session's player into its own state channel
// (0) and its stat channel (0x42) now rather than at the next sync, so a
// respawn's HP and place go out with it, as Sandstorm's do.
func (w *World) SyncSelfChannels(sess *session.Session) {
	w.mu.Lock()
	defer w.mu.Unlock()

	p := w.sessionPlayerLocked(sess)
	if p == nil {
		return
	}
	conn := sess.Connection()
	if conn == nil {
		return
	}
	if objectID := sess.PlayerObjectID(); objectID != 0 {
		if state := conn.GetServerObject(0); state != nil {
			state.SetData(SerializePlayerUpdate(objectID, p))
		}
	}
	if stat := conn.GetClientStatUpdate(); stat != nil {
		stat.SetData(SerializeStatUpdate(p))
	}
}
