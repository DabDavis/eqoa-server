package entity

import (
	"log"
	"math/rand"
	"regexp"
	"strings"
	"time"

	"github.com/eqoa/server/internal/session"
)

// Training dummies as Sandstorm's, measured 2026-10-21 with eqoa-rs's
// `eqoa harvest` attack step on the Freeport "Training Dummy Lv50" and
// Muniel's Tea Garden's ladder of Lv20-60
// (claude-notes/training-dummies-20261021.md):
//
//   - A dummy never dies. What it is hit for, by swing or spell, is told
//     (0x00DB) and taken off nothing; its health byte never moved.
//   - It fights the player whose hit landed first, for as long as that
//     player stays within its reach, attacking it or not (a dummy went on
//     with its rounds for 80 s while its player fought the dummies beside
//     it), and nobody else. Ours since petition 1484 (dummyEngagedLocked):
//     among several in its reach, the one with its most hate, a pet or a
//     mercenary too.
//   - It fights in rounds as every NPC does (rounds.go): the Lv20-30 swing
//     Attack2 every 4 s and hit at 0.5 and 1.5 s; from Lv35, Attack4 every
//     6 s with hits at 0.5, 1, 1.7 and 2.4 s.
//   - A dummy's hit is its level's roll on the player's AC, AGI and
//     archetype (defence.go, dummyRolls): the floor on a well-armoured tank
//     (two tanks of 1763 and 1430 AC took the Lv45's 24 every time), up to
//     its top on little armour (an enchanter of 167 AC took 24-124). A
//     Training Dummy's every hit is followed at once by a heal of 300
//     (0x00DB +300, the dummy the healer), so the player never dies of it;
//     a Regular Dummy heals nothing.
//
// It gives no experience and never takes a target's aggro (CurrentEnemy
// stays 0; its hate is its own, dummyHate), so the player's swings end with the attack button, as against
// any creature that is not fighting back.

// DummyKind says whether an NPC is a training dummy, and which.
type DummyKind uint8

const (
	NotDummy      DummyKind = iota
	TrainingDummy           // "Training Dummy LvN": a heal after each hit
	RegularDummy            // "Regular Dummy LvN": a creature's hits, no heal
	SparringDummy           // "Sparring Dummy": its level and element the player's pick (sparring.go)
)

// DummyKindOf is the kind the NPC's name gives it.
func DummyKindOf(name string) DummyKind {
	n := strings.ToLower(strings.TrimSpace(name))
	switch {
	case strings.HasPrefix(n, "training dummy"):
		return TrainingDummy
	case strings.HasPrefix(n, "regular dummy"):
		return RegularDummy
	case strings.HasPrefix(n, "sparring dummy"):
		return SparringDummy
	}
	return NotDummy
}

// dummyHitsName reads a sparring dummy's "x2" off the end of its name.
var dummyHitsName = regexp.MustCompile(`(?i)\bx([2-9])\s*$`)

// DummyHitsOf is how many times as hard a sparring dummy of that name hits
// (petition 1324, jibrish, 2026-11-09: "Add a second sparring dummy that hits
// twice as hard as the previous one but is otherwise the same"): the digit of
// a name ending "x2" to "x9", else 1.
func DummyHitsOf(name string) int {
	if m := dummyHitsName.FindStringSubmatch(strings.TrimSpace(name)); m != nil {
		return int(m[1][0] - '0')
	}
	return 1
}

// sparHits is a sparring dummy's hit or strike as hard as it hits.
func (n *NPC) sparHits(damage int) int {
	return damage * max(n.DummyHits, 1)
}

// dummyHeal is what a Training Dummy gives back after each hit.
const dummyHeal = 300

// dummyReach is how near the player must stay for the dummy to go on
// fighting it. A dummy went on at 7.1 and let go of a player 12.8 away.
const dummyReach = 10

// dummyEngagedLocked is a hit landing on a dummy, n told: its hate for the one
// that landed it, a player, a pet or a mercenary (by), and its fight begun if it
// had none. The first round starts at once, as Sandstorm's did within a tick of
// the first hit.
//
// A dummy's hate (the magician Lifeispain's petition 1484: "Sparing/Training
// dummy's in FP won't attack a pet, even if the caster is well out of striking
// distance. They just stand there. They should attack whichever has the most
// hate"): until then a pet's or a mercenary's hit set the dummy on its owner,
// and with the owner out of its reach the fight ended that tick, so a dummy
// stood while a pet beat on it. Now it keeps what each one in its reach has been
// told on it, and fights the one with the most (dummyFoeLocked), a pet as a
// creature fights a pet. The hate is the dummy's own (dummyHate): it is on no
// one's threat list, calls no help, and is gone with whoever leaves its reach,
// as its one player's fight was.
func (w *World) dummyEngagedLocked(npc *NPC, by EntityID, n int, now time.Time) {
	if npc.dummyHate == nil {
		npc.dummyHate = make(map[EntityID]int64)
	}
	npc.dummyHate[by] += int64(max(n, 1))
	if npc.DummyTarget != 0 {
		return
	}
	npc.DummyTarget = by
	npc.endRound()
	w.wakeNPCLocked(npc) // npcsets.go
	log.Printf("Dummy: %s fights %s", npc.Name, w.entityNameLocked(by))
}

// dummyTopHateLocked puts by at the top of the dummy's hate: a pet's or a
// mercenary's taunt on it. Must be called with the lock held.
func (w *World) dummyTopHateLocked(npc *NPC, by EntityID, now time.Time) {
	var top int64
	for id, v := range npc.dummyHate {
		if id != by && v > top {
			top = v
		}
	}
	w.dummyEngagedLocked(npc, by, int(max(top+1-npc.dummyHate[by], 1)), now)
}

// dummyInReachLocked is whether the dummy can go on fighting the one of the id:
// a player in the world with a session, or a player's pet or mercenary, alive
// and within its reach. Must be called with the lock held.
func (w *World) dummyInReachLocked(npc *NPC, id EntityID) bool {
	if p := w.Players[id]; p != nil {
		return !p.IsDead() && w.findSessionByEntityIDLocked(id) != nil && dummyNear(npc, p)
	}
	pet := w.NPCs[id]
	if pet == nil || pet.Owner == 0 || pet.Dead || pet.World != npc.World {
		return false
	}
	dx, dz := npc.X-pet.X, npc.Z-pet.Z
	return dx*dx+dz*dz <= dummyReach*dummyReach
}

// dummyFoeLocked is whom the dummy fights now: of those with its hate still in
// its reach the one with the most, the one it fights already among equals (a
// second player's first hit does not take it), else the lowest id; 0 for no
// one. Whoever is gone, dead or out of its reach loses its hate. A fight begun
// by hand (DummyTarget alone) counts as that one's hate. Must be called with
// the lock held.
func (w *World) dummyFoeLocked(npc *NPC) EntityID {
	if npc.DummyTarget != 0 && npc.dummyHate == nil {
		npc.dummyHate = map[EntityID]int64{npc.DummyTarget: 1}
	}
	var best EntityID
	var most int64
	for id, v := range npc.dummyHate {
		if !w.dummyInReachLocked(npc, id) {
			delete(npc.dummyHate, id)
			continue
		}
		if v > most || (v == most && (id == npc.DummyTarget || (best != npc.DummyTarget && id < best))) {
			best, most = id, v
		}
	}
	return best
}

// dummyFightLocked runs a fighting dummy's round (aiSystem's pass, every
// tick) on the one with its most hate: its fight ends when no one it hates
// is left in its reach. Must be called with the lock held.
func (w *World) dummyFightLocked(npc *NPC, now time.Time) {
	who := w.dummyFoeLocked(npc)
	if who == 0 || npc.Dead {
		npc.endDummyFight()
		return
	}
	if who != npc.DummyTarget {
		log.Printf("Dummy: %s turns on %s", npc.Name, w.entityNameLocked(who))
		npc.DummyTarget = who // its round goes on: the hits still due land on the new one
	}
	p := w.Players[who]
	if p == nil {
		pet := w.NPCs[who]
		w.npcRoundAtLocked(npc, who, now, func() bool { return w.dummyHitPetLocked(npc, pet, now) })
		return
	}
	sess := w.findSessionByEntityIDLocked(p.ServerID)
	start := npc.RoundStart
	w.npcRoundLocked(npc, p, now, func() bool { return w.dummyHitLocked(sess, npc, p, now) })
	if npc.Dummy == SparringDummy && npc.DummyTarget == p.ServerID {
		w.sparRoundLocked(sess, npc, p, start, now)
	}
}

// dummyHitPetLocked is one of a dummy's hits on a pet or a mercenary, as a
// creature's on it (npcFightPetLocked: its roll on the pet's armour, the pet's
// template's share), by the dummy's kind as on a player: a Regular Dummy's
// takes its hit points and may kill it, a Training Dummy's is followed by its
// heal of 300, a Sparring Dummy's (as hard as it hits) stops at the last hit
// point and ends its fight with the pet instead. A sparring dummy's element
// strike is a player's alone (a pet has no resists to roll). The pet's damage
// shield and defensive procs answer it. It reports the fight with the pet
// over. Must be called with the lock held.
func (w *World) dummyHitPetLocked(npc, pet *NPC, now time.Time) bool {
	n := w.petTakenLocked(pet, npc.sparHits(npcHitOnPet(npc, pet, meleeIntn))) // pettemplate.go
	pet.LastCombatTime = now
	if npc.Dummy == SparringDummy && pet.HP-n < 1 {
		pet.HP = min(pet.HP, 1)
		delete(npc.dummyHate, pet.ServerID)
		npc.endRound()
		if sess := w.findSessionByEntityIDLocked(pet.Owner); sess != nil {
			w.sendChatToSession(sess, npc.Name+": That will do. "+pet.Name+" is not fit to go on.")
		}
		log.Printf("Sparring: %s stopped short of killing the pet %q", npc.Name, pet.Name)
		return true
	}
	pet.HP = max(pet.HP-n, 0)
	log.Printf("Dummy: %s hit the pet %q for %d (HP %d/%d)", npc.Name, pet.Name, n, pet.HP, pet.MaxHP)
	if pet.HP == 0 {
		delete(npc.dummyHate, pet.ServerID)
		w.killNPC(pet)
		w.dropAggroLocked(pet.ServerID)
		return true
	}
	w.petShieldsLocked(pet, npc, now) // told, taken off nothing (dshield.go)
	if npc.Dummy == TrainingDummy {
		pet.HP = min(pet.HP+dummyHeal, pet.MaxHP)
	}
	return false
}

// dummyHitLocked is one of a dummy's hits on the player, and a Training
// Dummy's heal after it. It reports the player's death.
func (w *World) dummyHitLocked(sess *session.Session, npc *NPC, p *Player, now time.Time) bool {
	damage := npcMeleeRoll(npc).hitOn(p, rand.Intn)
	if npc.Dummy == SparringDummy {
		damage = npc.sparHits(damage) // "Sparring Dummy x2": twice as hard
		if w.sparHitLocked(sess, npc, p, damage, "", now) {
			return true
		}
		w.playerShieldsLocked(sess, p, npc, now) // the player's shields: told, taken off nothing (dshield.go)
		w.defenceProcsLocked(sess, p, npc, now)
		return false
	}
	p.HP = max(p.HP-damage, 0)
	p.LastCombatTime = now
	w.breakInvisLocked(p, "a hit") // invis.go: every invisibility, whatever gave it
	npcObjectID := w.objectIDForEntityLocked(sess, npc.ServerID)
	w.sendDamageIndication(sess, npcObjectID, -damage, sess.PlayerObjectID(), 0) // the dummy's on the player: untagged
	if p.HP <= 0 {
		npc.endDummyFight()
		w.playerDiedLocked(sess, p, now)
		return true
	}
	w.playerShieldsLocked(sess, p, npc, now) // the player's shields: told, taken off nothing (dshield.go)
	w.defenceProcsLocked(sess, p, npc, now)  // before the dummy's own heal, so they are seen
	if npc.Dummy == TrainingDummy {
		p.HP = min(p.HP+dummyHeal, p.MaxHPNow())
		w.sendDamageIndication(sess, npcObjectID, dummyHeal, sess.PlayerObjectID(), 0) // its heal of the player: untagged
	}
	return false
}

// dummyNear is whether the player is within the dummy's reach.
func dummyNear(npc *NPC, p *Player) bool {
	dx, dz := npc.X-p.X, npc.Z-p.Z
	return p.World == npc.World && dx*dx+dz*dz <= dummyReach*dummyReach
}

func (n *NPC) endDummyFight() {
	n.DummyTarget = 0
	n.dummyHate = nil
	n.endRound()
}
