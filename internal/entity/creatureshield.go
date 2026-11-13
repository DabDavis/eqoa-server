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
	taken, words := npc.shared(t.ShieldDamage), "" // an add's at its share (summon.go)
	if st := shieldResistStat(t.ShieldResist); st != StatNone {
		share, how := t.rollResist(p.StatNow(st))
		taken = max(int(math.Round(float64(taken)*(1-share))), 1)
		words = " (" + resistWords(share, how) + ")"
	}
	taken = w.formulaDamage(taken, dmgIn{src: srcNPC, kind: dkShield, an: npc, dp: p, raw: t.ShieldDamage,
		resist: shieldResistStat(t.ShieldResist)}) // dmgformula.go
	w.broadcastNPCSwingLocked(npc, p.ServerID, 0, fxShieldHit)
	p.HP = max(p.HP-taken, 0)
	p.StatsDirty = true
	p.LastCombatTime = now
	if sess != nil {
		if nid := w.objectIDForEntityLocked(sess, npc.ServerID); nid != 0 {
			w.sendDamageIndication(sess, nid, -taken, sess.PlayerObjectID(), 0) // the creature's on the player: untagged
		}
		w.sendChatToSession(sess, fmt.Sprintf("%s's damage shield hits you for %d%s.", npc.Name, taken, words))
	}
	log.Printf("Shield: %s's damage shield hits %s for %d of %d (HP %d/%d)", npc.Name, p.Name, taken, t.ShieldDamage, p.HP, p.MaxHPNow())
	if p.HP > 0 {
		return false
	}
	w.playerDiedLocked(sess, p, now)
	w.killedPlayerLocked(npc, p) // its line at the kill (selfbuff.go)
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
	taken := npc.shared(t.ShieldDamage)
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

// creatureProcLocked rolls the creature's melee procs after a hit of its landed on a standing player: its
// template's, then the one an ability's landing gave it for this fight (abilitytimes.go), each the
// creature's 0x00B1, the figure, a line and any death. It reports whether the player died of one. A
// template's dice are templateIntn.
func (w *World) creatureProcLocked(sess *session.Session, p *Player, npc *NPC, now time.Time) bool {
	t := npc.tmpl
	if t == nil || p.IsDead() {
		return false
	}
	if t.ProcChance > 0 && t.ProcDamage > 0 && templateIntn(100) < t.ProcChance &&
		w.creatureProcHitLocked(sess, p, npc, procName(t), npc.shared(t.ProcDamage), t.ProcResist, now) { // an add's at its share (summon.go)
		return true
	}
	if fp := npc.fightProcNow(); fp != nil && !p.IsDead() && templateIntn(100) < fp.chance &&
		w.creatureProcHitLocked(sess, p, npc, fp.name, fp.damage, "", now) { // fixed, unresisted
		return true
	}
	return !p.IsDead() && w.abilityProcsOnPlayerLocked(sess, p, npc, now) // its own procs and veils (creatureproc.go)
}

// creatureProcHitLocked is one of a creature's melee procs on a player: the figure, cut by the resist resist
// names ("" none), its 0x00B1, a line and any death. It reports whether the player died of it.
func (w *World) creatureProcHitLocked(sess *session.Session, p *Player, npc *NPC, name string, damage int, resist string, now time.Time) bool {
	taken, words := damage, ""
	if st := shieldResistStat(resist); st != StatNone {
		share, how := npc.tmpl.rollResist(p.StatNow(st))
		taken = max(int(math.Round(float64(taken)*(1-share))), 1)
		words = " (" + resistWords(share, how) + ")"
	}
	taken = w.formulaDamage(taken, dmgIn{src: srcNPC, kind: dkProc, an: npc, dp: p, raw: damage, resist: shieldResistStat(resist)}) // dmgformula.go
	w.broadcastNPCSwingLocked(npc, p.ServerID, 0, templateSpecialFX)
	p.HP = max(p.HP-taken, 0)
	p.StatsDirty = true
	p.LastCombatTime = now
	if sess != nil {
		if nid := w.objectIDForEntityLocked(sess, npc.ServerID); nid != 0 {
			w.sendDamageIndication(sess, nid, -taken, sess.PlayerObjectID(), 0) // the creature's on the player: untagged
		}
		w.sendChatToSession(sess, fmt.Sprintf("%s's %s hits you for %d%s.", npc.Name, name, taken, words))
	}
	log.Printf("Template: %s's %s hits %s for %d of %d (HP %d/%d)", npc.Name, name, p.Name, taken, damage, p.HP, p.MaxHPNow())
	if p.HP > 0 {
		return false
	}
	w.playerDiedLocked(sess, p, now)
	w.killedPlayerLocked(npc, p) // its line at the kill (selfbuff.go)
	return p.IsDead()            // a GM's #testmode stands at 1 HP: the hit goes on
}

// creatureProcOnPetLocked is creatureProcLocked on a pet: a workbook pet's resists cut the template's (petstats.go),
// any other takes it whole, and the fight's proc lands whole. It reports whether the pet died of one.
func (w *World) creatureProcOnPetLocked(npc, pet *NPC, now time.Time) bool {
	t := npc.tmpl
	if t == nil || pet.Dead {
		return false
	}
	if t.ProcChance > 0 && t.ProcDamage > 0 && templateIntn(100) < t.ProcChance &&
		w.creatureProcPetHitLocked(npc, pet, procName(t), npc.shared(t.ProcDamage), t.ProcResist, now) {
		return true
	}
	if fp := npc.fightProcNow(); fp != nil && !pet.Dead && templateIntn(100) < fp.chance &&
		w.creatureProcPetHitLocked(npc, pet, fp.name, fp.damage, "", now) {
		return true
	}
	return !pet.Dead && w.abilityProcsOnPetLocked(npc, pet, now) // its own procs and veils (creatureproc.go)
}

// creatureProcPetHitLocked is one of a creature's melee procs on a pet. It reports whether the pet died of it.
func (w *World) creatureProcPetHitLocked(npc, pet *NPC, name string, damage int, resist string, now time.Time) bool {
	taken := damage
	if st := shieldResistStat(resist); st != StatNone {
		if share := pet.petResistShare(st); share > 0 {
			taken = max(int(math.Round(float64(taken)*(1-share))), 1)
		}
	}
	w.broadcastNPCSwingLocked(npc, pet.ServerID, 0, templateSpecialFX)
	pet.HP = max(pet.HP-taken, 0)
	pet.LastCombatTime = now
	log.Printf("Template: %s's %s hits the pet %q for %d (HP %d/%d)", npc.Name, name, pet.Name, taken, pet.HP, pet.MaxHP)
	if pet.HP > 0 {
		return false
	}
	w.killNPC(pet)
	w.dropAggroLocked(pet.ServerID)
	return true
}
