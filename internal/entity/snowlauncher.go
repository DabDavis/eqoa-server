package entity

import (
	"fmt"
	"log"
	"time"

	"github.com/eqoa/server/internal/session"
)

// snowlauncher.go — the Automatic Snowball Launcher (the owner, 2026-11-16:
// "lets give zerkziee an automatic snowball launcher, infinite ammo").
//
// The launcher is an item record (handlers/snowlauncher.go: by its name, bound
// to its one character by table itemowner; tools/snowball_launcher_20261116.py
// makes it), worn nowhere and never spent: its ammunition never runs out. Each
// Use is a switch (ToggleSnowLauncher): on, it fires; on again, it stops.
//
// While on, every LauncherEvery at most it throws a snowball at its owner's
// target of the moment (Player.TargetObjectID, in his own session's ids),
// provided that target is in his world and instance, within SnowballRange and
// in sight (inSightLocked: no wall between). With no such target it waits,
// and fires as soon as there is one (LauncherEvery after its last shot).
//
//   - At a player (another, alive, not loading a world, not a GM hidden): the
//     Snowball's hit (moogieitems.go snowballHitLocked): SnowballFX, 1 off his
//     hit points, never below 1, each told. No PvP flag, no hate, no fight, no
//     duel. One player is hit by one launcher at most once in LauncherPlayerGap:
//     while it is not yet gone the launcher waits.
//   - At a creature: the Snowball refuses creatures outright ("You can only
//     throw a snowball at another player."), so it has no rule for their harm;
//     the launcher's snowball at a creature is for the look only: its visual,
//     and nothing more (no damage, no hate, no fight, nothing answers it). The
//     owner is told once a creature ("You pelt the gnoll with snowballs!").
//
// It stops by itself, its owner told, when he dies, goes to another world or
// instance, is moved far at once (a gate, a teleport, a GM's move: farther than
// petLost in a tick) or his client loads a world (the state channels held); a
// logout (or a login again) stops it unsaid. Not kept over a keep restart.

// The launcher's numbers.
const (
	// LauncherEvery is the least time between two of its snowballs
	LauncherEvery = 1500 * time.Millisecond
	// LauncherPlayerGap is the least time between two of its snowballs on one
	// player (the anti-spam)
	LauncherPlayerGap = 5 * time.Second
)

// The launcher's lines (the toggle's are the handler's).
const (
	launcherStopLine = "Your Automatic Snowball Launcher sputters and winds down."
	launcherNPCLine  = "You pelt %s with snowballs!"
)

// launcherState is one launcher switched on.
type launcherState struct {
	owner *Player
	// where its owner was: a world, an instance, a place last tick
	world    uint8
	instance uint32
	x, z     float32
	// next is when it may fire again
	next time.Time
	// hit is when each player was last hit by it
	hit map[EntityID]time.Time
	// toldNPC is the creature its owner was last told of
	toldNPC EntityID
}

// LauncherUse is what a use of the launcher came to.
type LauncherUse int

const (
	LauncherNone LauncherUse = iota // no player in the world
	LauncherDead                    // its user is dead: it stays off
	LauncherOn                      // switched on
	LauncherOff                     // switched off
)

// ToggleSnowLauncher is a use of the launcher by the session's player at now
// (the file's doc): on when off, off when on.
func (w *World) ToggleSnowLauncher(sess *session.Session, now time.Time) LauncherUse {
	w.mu.Lock()
	defer w.mu.Unlock()
	p := w.sessionPlayerLocked(sess)
	if p == nil {
		return LauncherNone
	}
	if st := w.launchers[p.ServerID]; st != nil && st.owner == p {
		delete(w.launchers, p.ServerID)
		log.Printf("Launcher: %s switches it off", p.Name)
		return LauncherOff
	}
	if p.IsDead() || p.respawnPending() {
		return LauncherDead
	}
	if w.launchers == nil {
		w.launchers = make(map[EntityID]*launcherState)
	}
	w.launchers[p.ServerID] = &launcherState{owner: p, world: p.World, instance: p.instanceKey(), x: p.X, z: p.Z,
		next: now, hit: make(map[EntityID]time.Time)}
	log.Printf("Launcher: %s switches it on", p.Name)
	return LauncherOn
}

// SnowLauncherOn is whether the session's player's launcher is on.
func (w *World) SnowLauncherOn(sess *session.Session) bool {
	w.mu.RLock()
	defer w.mu.RUnlock()
	p := w.sessionPlayerLocked(sess)
	st := w.launchers[p.idOrZero()]
	return p != nil && st != nil && st.owner == p
}

// launcherLeavingLocked is its owner leaving the world: it stops, unsaid.
// Must be called with the lock held.
func (w *World) launcherLeavingLocked(p *Player) {
	if st := w.launchers[p.ServerID]; st != nil && st.owner == p {
		delete(w.launchers, p.ServerID)
	}
}

// launcherMustStopLocked is why a launcher stops by itself ("" for none: the
// file's doc). Must be called with the lock held.
func (w *World) launcherMustStopLocked(st *launcherState, sess *session.Session) string {
	p := st.owner
	switch {
	case p.IsDead() || p.respawnPending():
		return "dead"
	case p.World != st.world:
		return "another world"
	case p.instanceKey() != st.instance:
		return "another instance"
	case distXZ(st.x, st.z, p.X, p.Z) > petLost:
		return "moved far"
	case sess != nil && sess.StateChannelsHeld():
		return "loading a world"
	}
	return ""
}

// snowLauncherSystem fires every launcher that is on and may (the file's
// doc), and stops those that must. Every tick; must be called with the lock
// held.
func (w *World) snowLauncherSystem(now time.Time) {
	for id, st := range w.launchers {
		p := st.owner
		if w.Players[id] != p {
			delete(w.launchers, id) // its owner logged out, or in again
			continue
		}
		sess := w.findSessionByEntityIDLocked(id)
		if why := w.launcherMustStopLocked(st, sess); why != "" {
			delete(w.launchers, id)
			if sess != nil {
				w.sendChatToSession(sess, launcherStopLine)
			}
			log.Printf("Launcher: %s's stops: %s", p.Name, why)
			continue
		}
		st.x, st.z = p.X, p.Z
		if sess == nil || now.Before(st.next) {
			continue
		}
		switch t := w.launcherTargetLocked(sess, st, now).(type) {
		case *Player:
			st.hit[t.ServerID] = now
			st.next = now.Add(LauncherEvery)
			w.snowballHitLocked(sess, p, t, SnowballFX)
		case *NPC:
			st.next = now.Add(LauncherEvery)
			w.broadcastCastLocked(p, t.ServerID, 0, SnowballFX)
			if st.toldNPC != t.ServerID {
				st.toldNPC = t.ServerID
				w.sendChatToSession(sess, fmt.Sprintf(launcherNPCLine, t.Name))
				log.Printf("Launcher: %s's snowballs at %q (%d): for the look", p.Name, t.Name, t.ServerID)
			}
		}
	}
}

// launcherTargetLocked is its owner's target if the launcher may fire at it
// now (the file's doc), else nil. Must be called with the lock held.
func (w *World) launcherTargetLocked(sess *session.Session, st *launcherState, now time.Time) Entity {
	p := st.owner
	for id, at := range st.hit {
		if now.Sub(at) >= LauncherPlayerGap {
			delete(st.hit, id) // spent
		}
	}
	switch t := w.entitySeenByLocked(sess, p.TargetObjectID).(type) {
	case *Player:
		if t == nil || t == p || t.IsDead() || t.respawnPending() || t.GMHidden || !SamePlace(p, t) || !w.noticeableSeenLocked(t) {
			return nil
		}
		if _, lately := st.hit[t.ServerID]; lately {
			return nil
		}
		if dist3(p.X, p.Y, p.Z, t.X, t.Y, t.Z) > SnowballRange || !w.inSightLocked(p.World, p.X, p.Y, p.Z, t.X, t.Y, t.Z) {
			return nil
		}
		return t
	case *NPC:
		if t == nil || t.IsDead() || t.isMonkey() || !w.npcSharesLocked(t, p) {
			return nil
		}
		if dist3(p.X, p.Y, p.Z, t.X, t.Y, t.Z) > SnowballRange || !w.inSightLocked(p.World, p.X, p.Y, p.Z, t.X, t.Y, t.Z) {
			return nil
		}
		return t
	}
	return nil
}
