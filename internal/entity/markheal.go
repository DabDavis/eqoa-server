package entity

import (
	"log"
	"time"

	"github.com/eqoa/server/internal/session"
)

// markheal.go — a mark on a creature that heals whoever hits it (the user,
// 2026-11-08: "mark of faith should target enemy only and anyone hitting that
// target with the mark of faith is healed for 65"). The cleric's Mark of Faith
// (701183) was a friend's defensive proc, the friend healed 65 when hit; it is a
// harmful spell on an enemy now, a workbook mark_heal of 65 for its minute
// (EffectMarkHeal, EffectRules.MarkHeal). Each weapon hit a player (melee or a
// shot: playerHitLocked) or a pet (petLandHitLocked) lands on the marked
// creature heals that hitter the mark's figure, the strongest mark on it if two,
// told as a heal is and only when short of its most; a heal is its healer's hate
// (wornGiveLocked's rule), here the hitter's own. A spell's damage, a proc's or
// a shield's answer heals no one: a hit of a weapon alone.

// markHealOf is the heal npc's marks give a hitter at now, 0 for none.
func (n *NPC) markHealOf(now time.Time) int {
	best := 0
	for _, e := range n.Effects {
		if e.Rules.MarkHeal > best && now.Before(e.Until) {
			best = e.Rules.MarkHeal
		}
	}
	return best
}

// markHealPlayerLocked heals p, whose weapon hit npc, by npc's mark. Must be
// called with the lock held.
func (w *World) markHealPlayerLocked(sess *session.Session, p *Player, npc *NPC, now time.Time) {
	n := npc.markHealOf(now)
	if n <= 0 || p.IsDead() {
		return
	}
	top := p.MaxHPNow()
	if p.HP >= top {
		w.tellAmountLocked(p, sess, p.ServerID, n, numberProc) // full: the figure alone, as a proc's heal (procHealLocked)
		w.healHateLocked(p, p, n, 0)                            // and a fifth of it hate (1967, aggro.go)
		return
	}
	before := p.HP
	p.HP = min(p.HP+n, top)
	p.StatsDirty = true
	w.tellAmountLocked(p, sess, p.ServerID, n, numberProc) // a hit's heal: a proc's (numbertags.go)
	w.healHateLocked(p, p, n, p.HP-before)
	log.Printf("Mark: %s's hit on %s heals it for %d (%d/%d)", p.Name, npc.Name, n, p.HP, top)
}

// markHealPetLocked heals pet, whose hit landed on npc, by npc's mark. Must be
// called with the lock held.
func (w *World) markHealPetLocked(pet, npc *NPC, now time.Time) {
	n := npc.markHealOf(now)
	if n <= 0 || pet.Dead || pet.HP >= pet.MaxHP {
		return
	}
	pet.HP = min(pet.HP+n, pet.MaxHP)
	log.Printf("Mark: %s's hit on %s heals it for %d (%d/%d)", pet.Name, npc.Name, n, pet.HP, pet.MaxHP)
}
