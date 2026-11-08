package entity

import (
	"fmt"
	"log"
	"math"
	"strings"
	"time"

	"github.com/eqoa/server/internal/session"
)

// creatureshield.go: a creature's own damage shield (petition 1154, Jibrish:
// "Add a 500 damage shield to lord nagafen, resisted by FR"). dshield.go is a
// player's or a pet's answering a creature; this is the other way round: a
// template's shield_damage answers each melee hit that lands on a creature of
// it, player's or pet's, cut by the attacker's resist as a creature's spell is
// (resist.go). A bow's shot and a thrown potion are not answered: a damage
// shield answers what touches it. A hit that kills the creature is not either.

// shieldResistStat is the resist a template's shield_resist names, StatNone
// for none.
func shieldResistStat(s string) Stat {
	switch strings.ToUpper(strings.TrimSpace(s)) {
	case "FR":
		return StatFR
	case "CR":
		return StatCR
	case "LR":
		return StatLR
	case "AR":
		return StatAR
	case "PR":
		return StatPR
	case "DR":
		return StatDR
	}
	return StatNone
}

// creatureShieldLocked answers a player's melee hit on the creature with its
// template's damage shield: its 0x00B1 of cast 0, the figure, a line and any
// death. It reports whether the player died of it, which ends the player's
// round. Called after the hit, while the creature stands.
func (w *World) creatureShieldLocked(sess *session.Session, p *Player, npc *NPC, now time.Time) bool {
	t := npc.tmpl
	if t == nil || t.ShieldDamage <= 0 || npc.Dead || p.IsDead() {
		return false
	}
	if k := p.Swing.Kind; k >= swingRange1 && k <= swingRange4 {
		return false // a bow's shot
	}
	taken, words := t.ShieldDamage, ""
	if st := shieldResistStat(t.ShieldResist); st != StatNone {
		share, how := t.rollResist(p.StatNow(st))
		taken = max(int(math.Round(float64(taken)*(1-share))), 1)
		words = " (" + resistWords(share, how) + ")"
	}
	w.broadcastNPCSwingLocked(npc, p.ServerID, 0, fxShieldHit)
	p.HP = max(p.HP-taken, 0)
	p.StatsDirty = true
	p.LastCombatTime = now
	if sess != nil {
		if nid := w.objectIDForEntityLocked(sess, npc.ServerID); nid != 0 {
			w.sendDamageIndication(sess, nid, -taken, sess.PlayerObjectID())
		}
		w.sendChatToSession(sess, fmt.Sprintf("%s's damage shield hits you for %d%s.", npc.Name, taken, words))
	}
	log.Printf("Shield: %s's damage shield hits %s for %d of %d (HP %d/%d)", npc.Name, p.Name, taken, t.ShieldDamage, p.HP, p.MaxHPNow())
	if p.HP > 0 {
		return false
	}
	w.playerDiedLocked(sess, p, now)
	return true
}

// creatureShieldOnPetLocked is creatureShieldLocked for a pet's melee hit: a
// workbook pet's resists cut it (petstats.go), any other pet takes it whole. It
// reports whether the pet died of it, which ends the pet's round.
func (w *World) creatureShieldOnPetLocked(ps *petState, npc *NPC, now time.Time) bool {
	t, pet := npc.tmpl, ps.npc
	if t == nil || t.ShieldDamage <= 0 || npc.Dead || pet.Dead {
		return false
	}
	taken := t.ShieldDamage
	if st := shieldResistStat(t.ShieldResist); st != StatNone {
		if share := pet.petResistShare(st); share > 0 {
			taken = max(int(math.Round(float64(taken)*(1-share))), 1)
		}
	}
	w.broadcastNPCSwingLocked(npc, pet.ServerID, 0, fxShieldHit)
	pet.HP = max(pet.HP-taken, 0)
	pet.LastCombatTime = now
	log.Printf("Shield: %s's damage shield hits the pet %q for %d (HP %d/%d)", npc.Name, pet.Name, taken, pet.HP, pet.MaxHP)
	if pet.HP > 0 {
		return false
	}
	w.killNPC(pet)
	w.dropAggroLocked(pet.ServerID)
	return true
}

// A creature's melee proc (jibrish's agent's board entry 230: "Lord Nagafen ... should have a proc that
// fires off 25% of the time on melee hits. It should do 2000 damage and be mitigated by fire resistance";
// petition 1161, Jibrish: about 5 more on the rate, 3000 before FR). Not the template's special, which rolls a
// round or runs on a clock and goes through any armour: this rolls each melee hit that lands.

// procName is what a template's proc is called to the one it hits.
func procName(t *ClassTemplate) string {
	if t.ProcName != "" {
		return t.ProcName
	}
	return "proc"
}

// creatureProcLocked rolls the creature's melee proc after a hit of its landed on a standing player: the
// creature's 0x00B1, the figure, a line and any death. It reports whether the player died of it. A template's
// dice are templateIntn.
func (w *World) creatureProcLocked(sess *session.Session, p *Player, npc *NPC, now time.Time) bool {
	t := npc.tmpl
	if t == nil || t.ProcChance <= 0 || t.ProcDamage <= 0 || p.IsDead() || templateIntn(100) >= t.ProcChance {
		return false
	}
	taken, words := t.ProcDamage, ""
	if st := shieldResistStat(t.ProcResist); st != StatNone {
		share, how := t.rollResist(p.StatNow(st))
		taken = max(int(math.Round(float64(taken)*(1-share))), 1)
		words = " (" + resistWords(share, how) + ")"
	}
	w.broadcastNPCSwingLocked(npc, p.ServerID, 0, templateSpecialFX)
	p.HP = max(p.HP-taken, 0)
	p.StatsDirty = true
	p.LastCombatTime = now
	if sess != nil {
		if nid := w.objectIDForEntityLocked(sess, npc.ServerID); nid != 0 {
			w.sendDamageIndication(sess, nid, -taken, sess.PlayerObjectID())
		}
		w.sendChatToSession(sess, fmt.Sprintf("%s's %s hits you for %d%s.", npc.Name, procName(t), taken, words))
	}
	log.Printf("Template: %s's %s hits %s for %d of %d (HP %d/%d)", npc.Name, procName(t), p.Name, taken, t.ProcDamage, p.HP, p.MaxHPNow())
	if p.HP > 0 {
		return false
	}
	w.playerDiedLocked(sess, p, now)
	return p.IsDead() // a GM's #testmode stands at 1 HP: the hit goes on
}

// creatureProcOnPetLocked is creatureProcLocked on a pet: a workbook pet's resists cut it (petstats.go), any
// other takes it whole. It reports whether the pet died of it.
func (w *World) creatureProcOnPetLocked(npc, pet *NPC, now time.Time) bool {
	t := npc.tmpl
	if t == nil || t.ProcChance <= 0 || t.ProcDamage <= 0 || pet.Dead || templateIntn(100) >= t.ProcChance {
		return false
	}
	taken := t.ProcDamage
	if st := shieldResistStat(t.ProcResist); st != StatNone {
		if share := pet.petResistShare(st); share > 0 {
			taken = max(int(math.Round(float64(taken)*(1-share))), 1)
		}
	}
	w.broadcastNPCSwingLocked(npc, pet.ServerID, 0, templateSpecialFX)
	pet.HP = max(pet.HP-taken, 0)
	pet.LastCombatTime = now
	log.Printf("Template: %s's %s hits the pet %q for %d (HP %d/%d)", npc.Name, procName(t), pet.Name, taken, pet.HP, pet.MaxHP)
	if pet.HP > 0 {
		return false
	}
	w.killNPC(pet)
	w.dropAggroLocked(pet.ServerID)
	return true
}
