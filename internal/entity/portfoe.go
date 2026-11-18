package entity

import (
	"fmt"
	"log"

	"github.com/eqoa/server/internal/database"
)

// portfoe.go — a boss's moment (bossmoments.go) that sends the one it fights
// away (database.AbilityPortFoe; the owner's Evil Melee epic, 2026-11-18, the
// forum's t155 and t152: Ahriaana the Ultimate "ports those with aggro off the
// island", "not all the time", and the ported "get another gem to go back";
// the GM Jibrish's board 615, code ask 1: "the player with her aggro is sent
// off the plane, to its bind point ... pets never").
//
// When it is due, the one its creature fights now is looked at: a player
// alive, in its world, sharing its view, not on its way somewhere already and
// with its session's world loaded, is sent to its own bind point (Return
// Home's place: GateTravel.Home) or, with To, to that place; its creature
// forgets it (forgetLocked: off its hate list, its next enemy the next on
// it), so the fight goes on with whoever is left. A pet or a mercenary
// holding it is never sent (the era's way to fight her: pets): the moment
// passes. Its pet stays on the hate list as it was.

// portFoeLocked is a portfoe of npc's (the file's doc). Must be called with the
// lock held.
func (w *World) portFoeLocked(npc *NPC, m *database.TemplateAbility) {
	p := w.Players[npc.CurrentEnemy]
	if p == nil || p.IsDead() || p.respawnPending() || p.World != npc.World || !w.npcSharesLocked(npc, p) || w.travelDueLocked(p) {
		return
	}
	sess := w.findSessionByEntityIDLocked(p.ServerID)
	if sess == nil || sess.StateChannelsHeld() {
		return
	}
	name := momentName(m)
	tr := GateTravel{Sess: sess, P: p, Spell: npc.Name + "'s " + name}
	where, line := "its bind point", "home"
	if len(m.To) == 4 {
		tr.To = GateSpot{World: uint8(m.To[0]), X: float32(m.To[1]), Y: float32(m.To[2]), Z: float32(m.To[3]), Place: "away"}
		where, line = fmt.Sprintf("(%.1f, %.1f, %.1f) world %d", m.To[1], m.To[2], m.To[3], int(m.To[0])), "away"
	} else {
		tr.Home = p.ServerID
	}
	fx := m.FX
	if fx == 0 {
		fx = templateSpecialFX
	}
	w.broadcastNPCSwingLocked(npc, p.ServerID, 0, fx)
	w.gates.due = append(w.gates.due, tr)
	w.sendColoredLocked(sess, fmt.Sprintf("%s's %s sends you %s!", npc.Name, name, line), duelOrange)
	w.forgetLocked(npc, p)
	log.Printf("Template: %s's %s sends %s (its enemy) to %s; now on %d", npc.Name, name, p.Name, where, npc.CurrentEnemy)
}
