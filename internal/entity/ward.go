package entity

import (
	"fmt"
	"log"
	"time"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/session"
)

// ward.go — a creature's ward (database.AbilityWard; the GM Jibrish's petition 1892: "Add a buff to mr moogie
// that fires off at 15% remaining HP. This gives mr. moogie a 500 a hit heal DS that heals him when hit, and
// gives him a 5000 damage a hit additional damage shield that damages everyone with pure damage when hes hit").
//
// When its hit points first fall below its ward's share (Below) in a fight, its ward comes up, once a fight,
// until that fight ends (its death, its hate list empty, a leash home, a GM's edit of the templates): a heal
// above the share leaves it up, and the next fight may raise it again. Its line in /say (Say) as it comes.
// While it is up, every hit that lands on it, taking hit points, and leaves it standing (every hit a hit
// heal answers: hitheal.go) heals it Heal more, beside its hit heal, and is answered with Damage on whoever
// landed it: the player (its melee, bow, spells, their ticks, procs and damage shields), the pet, the
// mercenary, the NPC (a scripted fight's). Pure: no resist, armour, mitigation or share cuts it ("pure
// damage"); a GM's #testmode stands at 1 as from any hit. "Everyone" is read as everyone who hits it: each
// one's hit answered on that one. The answers land at the creature's next fight tick (a tenth of a second),
// not inside the hit, so no hit's own code sees its hitter die under it; each hit is its own answer, in the
// order the hits landed, the hitter's death ending its answers.

// wardOn is a ward up this fight: when it came up, its ability, and the answers owed (the hitters' ids in the
// order their hits landed, one a hit).
type wardOn struct {
	at  time.Time
	a   *database.TemplateAbility
	due []EntityID
}

// wardName is what a ward's lines call it.
func wardName(a *database.TemplateAbility) string {
	if a.Name != "" {
		return a.Name
	}
	return database.DefaultWardName
}

// wardHeal is what npc's ward adds to each hit's heal now (its hit heal's: hitheal.go), 0 with none up.
func (n *NPC) wardHeal() int {
	if n.tfight == nil || n.tfight.warded == nil {
		return 0
	}
	return n.shared(n.tfight.warded.a.Heal)
}

// wardHitLocked owes the one who landed a hit on npc its ward's answer (the file's doc). Must be called with
// the lock held.
func (w *World) wardHitLocked(npc *NPC, by EntityID) {
	if npc.tfight == nil || npc.tfight.warded == nil || npc.tfight.warded.a.Damage <= 0 || by == 0 || by == npc.ServerID {
		return
	}
	npc.tfight.warded.due = append(npc.tfight.warded.due, by)
}

// wardPhaseLocked is npc's ward in its fight f this tick: up below its share, then the answers owed. Must be
// called with the lock held.
func (w *World) wardPhaseLocked(npc *NPC, f *templateFight, now time.Time) {
	a := npc.tmpl.ward
	if a == nil {
		return
	}
	if f.warded == nil {
		if npc.HP*100 >= npc.MaxHP*a.Below {
			return
		}
		f.warded = &wardOn{at: now, a: a}
		if a.Say != "" {
			w.sayNearLocked(npc.Name, npc.World, npc.X, npc.Z, a.Say) // abilitytimes.go
		}
		log.Printf("Template: %s's %s comes up (HP %d/%d, below %d%%): +%d heal and %d pure damage back at every hit", npc.Name,
			wardName(a), npc.HP, npc.MaxHP, a.Below, a.Heal, a.Damage)
		return
	}
	due := f.warded.due
	f.warded.due = nil
	dead := map[EntityID]bool{}
	for _, id := range due {
		if !dead[id] && w.wardAnswerLocked(npc, id, now) {
			dead[id] = true
		}
	}
}

// wardAnswerLocked lands npc's ward's damage on id, one who hit it: a player, else an NPC (a pet, a
// mercenary, a scripted fight's). It reports id's death. Must be called with the lock held.
func (w *World) wardAnswerLocked(npc *NPC, id EntityID, now time.Time) bool {
	a := npc.tfight.warded.a
	n, name := npc.shared(a.Damage), wardName(a)
	if p := w.Players[id]; p != nil {
		if p.IsDead() || p.respawnPending() || p.World != npc.World {
			return false
		}
		w.broadcastNPCSwingLocked(npc, p.ServerID, 0, fxShieldHit)
		p.HP = max(p.HP-n, 0)
		p.StatsDirty = true
		p.LastCombatTime = now
		sess := w.findSessionByEntityIDLocked(id)
		if sess != nil {
			if nid := w.objectIDForEntityLocked(sess, npc.ServerID); nid != 0 {
				w.sendDamageIndication(sess, nid, -n, sess.PlayerObjectID(), 0) // the creature's on the player: untagged
			}
			w.sendChatToSession(sess, fmt.Sprintf("%s's %s hits you for %d.", npc.Name, name, n))
		}
		log.Printf("Template: %s's %s answers %s for %d (HP %d/%d)", npc.Name, name, p.Name, n, p.HP, p.MaxHPNow())
		if p.HP > 0 {
			return false
		}
		w.playerDiedLocked(sess, p, now)
		w.killedPlayerLocked(npc, p) // its line at the kill (selfbuff.go)
		return p.IsDead()            // a GM's #testmode stands at 1 HP
	}
	o := w.NPCs[id]
	if o == nil || o.Dead || o.World != npc.World {
		return false
	}
	w.broadcastNPCSwingLocked(npc, o.ServerID, 0, fxShieldHit)
	o.HP = max(o.HP-n, 0)
	o.LastCombatTime = now
	log.Printf("Template: %s's %s answers %q for %d (HP %d/%d)", npc.Name, name, o.Name, n, o.HP, o.MaxHP)
	if o.HP > 0 {
		return false
	}
	o.Effects = nil
	w.killNPC(o) // a mercenary's death is its system's to see (merc.go), a pet's its owner's
	w.dropAggroLocked(o.ServerID)
	w.killSaidLocked(npc, o.Name) // its line at the kill (selfbuff.go)
	return true
}

// wardWords is #stats' words for a template's ward, "" for none.
func wardWords(npc *NPC) string {
	a := npc.tmpl.ward
	if a == nil {
		return ""
	}
	s := fmt.Sprintf("%s below %d%%: +%d heal and %d pure damage on the hitter at every hit", wardName(a), a.Below, npc.shared(a.Heal),
		npc.shared(a.Damage))
	if f := npc.tfight; f != nil && f.warded != nil {
		s += " (up now)"
	}
	return s
}

// hitLandedLocked is what follows a hit of n that landed on npc and left it standing, by the entity by (a
// player, a pet, a mercenary, an NPC; 0 for none), told to sess (the hitter's, or its owner's: nil for
// none): its heal at every hit (hitheal.go, a ward's added) and its ward's answer owed. Must be called with
// the lock held.
func (w *World) hitLandedLocked(npc *NPC, n int, sess *session.Session, by EntityID) {
	if n <= 0 || npc.Dead || npc.HP <= 0 {
		return
	}
	w.hitHealLocked(npc, n, sess)
	w.wardHitLocked(npc, by)
}
