package entity

import (
	"fmt"
	"log"
	"math/rand"
	"time"

	"github.com/eqoa/server/internal/session"
)

// root.go — a root on a creature, held to its con (the user, 2026-10-29:
// "Root breaks on damage and higher cons resist it").
//
// Until then a root (a spell's EffectRoot: the druid's Root, the wizard's
// Arcane Bonds, the ranger's Tangling Weeds, the alchemist's bonds) always
// took and held for its whole duration whatever hit the creature, so a class
// with one rooted any red and burned it down from range, never touched
// (claude-notes/balance-20261029.md: the fight simulator's root-and-nuke
// play).
//
// Now, by the creature's level less the root's caster's (Balance by con's
// root_resist_pct and root_break_pct, not scaled by con_pct):
//   - at its landing the root may be resisted: the spell's root (and a
//     snare's slow, the resist a snare's too) does not take, the rest of it
//     lands (a damaging root's hit), the caster told in red; an arcane
//     resist debuff on the creature (enchanter.go) lowers the chance, as a
//     charm's;
//   - every hit on a rooted creature that leaves it standing (a player's
//     melee, a spell's hit or tick, a proc, a damage shield, a pet's hit, at
//     the attacker's con; a pet's at its balance level) may free it: every
//     root on it ends, and each root's caster is told. A spell that roots
//     and hits is not freed by its own hit (the hit lands first).
// A snare does not break: it only slows.

// rootIntn is the roots' dice (a test's and the simulator's own).
var rootIntn = rand.Intn

const (
	rootResisted  = "Your target resists the root."
	snareResisted = "Your target resists the snare."
	rootBroken    = "%s breaks free of your %s."
)

// rootResistLocked rolls a root's (a snare's) resist as it lands on the
// creature from caster: resisted, the effect's root and slow are taken off it
// and the caster told; it reports whether anything of the effect is left to
// put on. Must be called with the lock held.
func (w *World) rootResistLocked(caster *Player, casterSess *session.Session, npc *NPC, ae *ActiveEffect, now time.Time) bool {
	if (!ae.Root && !ae.Snare) || !balanced(npc) {
		return true
	}
	chance := w.balance.Con(npc.Level-caster.Level).RootResistPct - float64(npcResistOffPct(npc, now))
	if chance <= 0 || float64(rootIntn(10000)) >= chance*100 {
		return true
	}
	text := snareResisted
	if ae.Root {
		text = rootResisted
	}
	log.Printf("Root: %s resists %s's %q (%.0f%%: %d over its caster)", npc.Name, caster.Name, ae.Name, chance, npc.Level-caster.Level)
	if casterSess != nil {
		w.sendColoredLocked(casterSess, text, castRefusalRGB)
	}
	ae.Root, ae.Snare = false, false
	return ae.lastsAnything()
}

// lastsAnything is whether an effect does anything while it lasts.
func (ae *ActiveEffect) lastsAnything() bool {
	return ae.HPTick != 0 || ae.PowerTick != 0 || ae.Mods != (StatMods{}) || ae.Root || ae.Snare || ae.Speed ||
		ae.Invis || ae.Proc != 0 || ae.Shield != nil || ae.PowerCapped || ae.Form != 0
}

// rootBreakLocked is a hit on the creature from an attacker of the level: a
// rooted creature still standing may break free (the file's doc). Must be
// called with the lock held.
func (w *World) rootBreakLocked(npc *NPC, attackerLevel int) {
	if npc.Dead || npc.HP <= 0 || !balanced(npc) {
		return
	}
	rooted := false
	for _, ae := range npc.Effects {
		if ae.Root {
			rooted = true
			break
		}
	}
	if !rooted {
		return
	}
	chance := w.balance.Con(npc.Level - attackerLevel).RootBreakPct
	if chance <= 0 || float64(rootIntn(10000)) >= chance*100 {
		return
	}
	left := npc.Effects[:0]
	for _, ae := range npc.Effects {
		if ae.Root {
			ae.Root = false
			log.Printf("Root: %s breaks free of %q (%.0f%%)", npc.Name, ae.Name, chance)
			if caster := w.Players[ae.Caster]; caster != nil {
				if sess := w.findSessionByEntityIDLocked(caster.ServerID); sess != nil {
					w.sendChatToSession(sess, fmt.Sprintf(rootBroken, npc.Name, ae.Name))
				}
			}
			if !ae.lastsAnything() {
				continue
			}
		}
		left = append(left, ae)
	}
	npc.Effects = left
}
