package entity

import (
	"fmt"
	"log"
	"time"
)

// lava.go — the lava burns a player in it (the GM Jibrish's petition 2281, 2026-11-17,
// owner-approved: "Enable lava damage per tick").
//
// The lava is the game's own: the zone leaves' lava flag (4) in each world's environment bake
// (internal/ground's env.go, the files beside the ground files), the flag CLIENT's walk sets
// its inLava and atLavaSurface bits from (VIWalkControl's CalcFloor, decomp
// src/viwalkcontrol.cpp). A player is in it when the point just over its feet is (lavaFeet: a
// foot in the lava, wading, counts).
//
// No rule of the live game is known: CLIENT carries VIWalkControl::IsInLava and nothing calls
// it, the client hurts no one itself, and no server code of SOE's survives. So the figure is
// the operator's (config world.lava_*, LavaRule): a share of the player's most hit points
// and/or a flat figure, every Every; with neither, none (the default: off). Optionally its
// fire resistance takes its share as a fire spell's would (resolvePlayerElemental, the
// server's resist model, FR). Not burnt: the dead and those waiting to respawn, a GM (the
// account's at its login), a GM flying (#fly), a session loading a world. No levitation
// exists on this server (no spell gives one); a player over the lava, not in it, is not
// burnt. A burn says its figure in red with the number on the player (as a hazard's,
// hazards.go), and a death of it is a death as a drowning's.

// LavaRule is what the lava does: PctMaxHP percent of the player's most hit points (its
// MaxHPNow) plus Flat, at least 1 when either is set, every Every (0: 3 s); FireResist lets
// its FR take its share. The zero rule burns no one.
type LavaRule struct {
	PctMaxHP   float64
	Flat       int
	Every      time.Duration
	FireResist bool
}

// On reports a rule that burns.
func (r LavaRule) On() bool { return r.PctMaxHP > 0 || r.Flat > 0 }

// every is its tick.
func (r LavaRule) every() time.Duration {
	if r.Every <= 0 {
		return lavaEveryDefault
	}
	return r.Every
}

func (r LavaRule) String() string {
	if !r.On() {
		return "lava: off"
	}
	return fmt.Sprintf("lava: %g%% of most HP + %d every %v (fire resist %v)", r.PctMaxHP, r.Flat, r.every(), r.FireResist)
}

// lavaEveryDefault is a rule's tick with none given; lavaFeet how far over its feet a player
// is looked at for the lava.
const (
	lavaEveryDefault         = 3 * time.Second
	lavaFeet         float32 = 0.1
)

// lavaWords is a burn's line (ours).
const lavaWords = "The lava burns you for %d."

// SetLava sets the lava's rule.
func (w *World) SetLava(r LavaRule) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.lava = r
}

// Lava is the lava's rule.
func (w *World) Lava() LavaRule {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.lava
}

// playerInLavaLocked reports the player's feet in the lava; false where the server knows
// none. Must be called with the lock held.
func (w *World) playerInLavaLocked(p *Player) bool {
	if w.env == nil {
		return false
	}
	f, ok := w.env.Flags(p.World, p.X, p.Y+lavaFeet, p.Z)
	return ok && f&envLava != 0
}

// burn is what the rule takes from p now.
func (r LavaRule) burn(p *Player) int {
	n := int(float64(p.MaxHPNow())*r.PctMaxHP/100) + r.Flat
	n = max(n, 1)
	if r.FireResist {
		n = resolvePlayerElemental(playerElem{d0: n, resist: p.StatNow(StatFR)}).Taken
	}
	return n
}

// lavaSystem burns each player in the lava (the file's doc), on its own clock
// (Player.lavaNext): at once on stepping in, then every tick it stays. Must be called with the
// lock held.
func (w *World) lavaSystem(now time.Time) {
	if !w.lava.On() || w.env == nil || w.TickCount%hazardEvery != 0 {
		return
	}
	for _, p := range w.Players {
		if p.HP <= 0 || p.respawnPending() || p.GM || p.flyingNow(now) {
			continue
		}
		if !w.playerInLavaLocked(p) {
			p.lavaNext = time.Time{} // out of it: the next step in burns
			continue
		}
		if now.Before(p.lavaNext) {
			continue
		}
		sess := w.findSessionByEntityIDLocked(p.ServerID)
		if sess != nil && sess.StateChannelsHeld() {
			continue // loading a world
		}
		p.lavaNext = now.Add(w.lava.every())
		n := min(w.lava.burn(p), p.HP)
		if n <= 0 {
			continue
		}
		p.HP -= n
		p.StatsDirty = true
		log.Printf("Lava: %s at (%.1f, %.1f, %.1f) world=%d takes %d (HP %d/%d)", p.Name, p.X, p.Y, p.Z, p.World, n, p.HP, p.MaxHPNow())
		if sess != nil {
			me := sess.PlayerObjectID()
			w.sendDamageIndication(sess, me, -n, me, 0)
			w.sendColoredLocked(sess, fmt.Sprintf(lavaWords, n), castRefusalRGB)
		}
		if p.HP <= 0 {
			w.playerDiedLocked(sess, p, now)
		}
	}
}
