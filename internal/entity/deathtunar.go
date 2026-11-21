package entity

import (
	"fmt"
	"log"

	"github.com/eqoa/server/internal/economy"
	"github.com/eqoa/server/internal/session"
)

// deathtunar.go — a PvE death takes half the tunar the character carries (work
// board #1762, jibrish, approved by the owner 2026-11-21; SOE's FAQ: "You will
// lose some experience and a portion of the money on your character at the
// time of death", and an in-era player: "you lose half your tunar after
// death").
//
//   - A death to a creature or the world (playerDiedLocked: melee, spells,
//     lava, drowning, the sun, a GM's KILL) takes floor(purse ×
//     DeathTunarPct / 100) of the purse, at the point its experience debt is
//     added. The tunar goes nowhere: not to the corpse (a player has none),
//     not to the creature.
//   - The bank (the character's and the account's) is never touched.
//   - A PvP death, a duel's included (pvp.go pvpDeathLocked), takes none, as
//     it takes no debt; nor a death in a capture-the-flag match or arena
//     (ctf.go), a GM's #testmode (no death at all), a Metal Moogie challenge
//     lost (1 HP left), or a Hardcore character's fall (the character is gone).
//   - A resurrection gives none of it back (it gives back debt alone).
//   - The client's purse is sent (0x0052) with a line, "You lost N tunar.",
//     and the purse is written at once through the OnPurse hook, as a
//     pickpocket's take is: the logout, a replacing login and SaveAll do not
//     write the purse; every change to it is written when it is made.

// DefaultDeathTunarPct is DeathTunarPct with world.death_tunar_pct unset.
const DefaultDeathTunarPct = 50

// DeathTunarPct is the share of the carried tunar a PvE death takes, 0-100
// (world.death_tunar_pct; 0 turns it off). Set at the start, before any
// character is in the world.
var DeathTunarPct = DefaultDeathTunarPct

// SetDeathTunarPct sets DeathTunarPct: nil is the default, else held to 0-100.
func SetDeathTunarPct(pct *int) {
	if pct == nil {
		DeathTunarPct = DefaultDeathTunarPct
		return
	}
	DeathTunarPct = max(0, min(100, *pct))
}

// deathTunarLine is what the player is told of the tunar its death took (ours).
const deathTunarLine = "You lost %d tunar."

// deathTunarOf is what a death takes of a purse at a share: floor(purse × pct
// / 100), none of an empty or negative purse.
func deathTunarOf(purse, pct int) int {
	if purse <= 0 || pct <= 0 {
		return 0
	}
	return int(int64(purse) * int64(min(pct, 100)) / 100)
}

// deathTunarLocked takes a PvE death's share of p's carried tunar (the file's
// doc): the tunar taken, 0 for none. Must be called with the lock held.
func (w *World) deathTunarLocked(sess *session.Session, p *Player) int {
	if p.GMTestMode || w.ctfMatchOfLocked(p) != nil || w.ctfArenaAnyLocked(p.World) {
		return 0
	}
	n := deathTunarOf(p.Tunar, DeathTunarPct)
	if n <= 0 {
		return 0
	}
	p.Tunar -= n
	economy.Spent(economy.DeathLoss, p.CharServerID, p.Name, n)
	log.Printf("Death: %s lost %d tunar (%d carried now)", p.Name, n, p.Tunar)
	if sess != nil {
		sendPurse(sess, p.Tunar)
		w.sendChatToSession(sess, fmt.Sprintf(deathTunarLine, n))
	}
	if fn := w.onPurse; fn != nil {
		go fn(p.CharServerID, p.Tunar)
	}
	return n
}
