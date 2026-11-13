package entity

import (
	"fmt"
	"log"
	"sort"
	"time"

	"github.com/eqoa/server/internal/database"
)

// unreached.go — a boss that cannot reach anyone (database.AbilityUnreached, one of its moments:
// bossmoments.go; the GM Jibwiz's petition 1916, Mr. Moogie: "if someone is on his hate list for more than
// 60 seconds, and he has not been able to hit any target on his hate list for more than 60 seconds, he
// nukes them unresistable for 20,000 every 6 seconds. Essentially if he's fighting 10 people, hitting one,
// then this does not go off").
//
// Its fight keeps when its creature's last melee hit landed (any landed swing of its rounds on a player, a
// pet, a mercenary or a scripted fight's creature: npcAttackPlayer, the pets' fight, npcHitNPCLocked; a
// miss is none), the fight's start before one, and when each of its hate list came on it. At each of the
// moment's times (its Every, from its First), once Unhit seconds (60) have gone with no hit landed, every
// player of its hate list that has been on it Unhit seconds or more, alive, where it is and not loading,
// takes Damage, pure (no resist, armour, mitigation or share: as a ward's answer, ward.go), told as the
// creature's hit with a line; a GM's #testmode stands at 1. One hit landed puts it off again for Unhit
// seconds. Pets and mercenaries on its list are not struck: "nukes them" is the players on it.

// meleeLanded marks a melee hit of n of the NPC's that landed (n > 0) in its template's fight.
func (n *NPC) meleeLanded(dmg int, now time.Time) {
	if dmg > 0 && n.tfight != nil {
		n.tfight.landedAt = now
	}
}

// unreachedListLocked keeps when each of npc's hate list came on it, for a template with an unreached
// moment. Must be called with the lock held.
func (w *World) unreachedListLocked(npc *NPC, f *templateFight, now time.Time) {
	has := false
	for i := range npc.tmpl.moments {
		has = has || npc.tmpl.moments[i].Kind == database.AbilityUnreached
	}
	if !has {
		return
	}
	if f.listedAt == nil {
		f.listedAt = map[EntityID]time.Time{}
	}
	for id := range npc.Threats {
		if _, ok := f.listedAt[id]; !ok {
			f.listedAt[id] = now
		}
	}
	for id := range f.listedAt {
		if _, ok := npc.Threats[id]; !ok {
			delete(f.listedAt, id) // off the list: a new stay begins again
		}
	}
}

// unreachedMomentLocked is one of npc's unreached moments (the file's doc). Must be called with the lock
// held.
func (w *World) unreachedMomentLocked(npc *NPC, m *database.TemplateAbility, now time.Time) {
	f := npc.tfight
	if f == nil {
		return
	}
	wait := time.Duration(m.UnhitOr() * float64(time.Second))
	if now.Sub(f.landedAt) < wait {
		return
	}
	var ps []*Player
	for id, since := range f.listedAt {
		p := w.Players[id]
		if p == nil || now.Sub(since) < wait || p.IsDead() || p.respawnPending() || p.World != npc.World || !w.npcSharesLocked(npc, p) {
			continue
		}
		ps = append(ps, p)
	}
	if len(ps) == 0 {
		return
	}
	sort.Slice(ps, func(i, j int) bool { return ps[i].ServerID < ps[j].ServerID })
	name := momentName(m)
	fx := m.FX
	if fx == 0 {
		fx = templateSpecialFX
	}
	log.Printf("Template: %s's %s: no hit landed for %v, %d of its list on it %v or more", npc.Name, name,
		now.Sub(f.landedAt).Round(time.Second), len(ps), wait)
	for _, p := range ps {
		w.broadcastNPCSwingLocked(npc, p.ServerID, 0, fx)
		p.HP = max(p.HP-m.Damage, 0)
		p.StatsDirty = true
		p.LastCombatTime = now
		sess := w.findSessionByEntityIDLocked(p.ServerID)
		if sess != nil {
			from := w.objectIDForEntityLocked(sess, npc.ServerID)
			if from == 0 {
				from = sess.PlayerObjectID() // the creature out of its view: the figure still shown
			}
			w.sendDamageIndication(sess, from, -m.Damage, sess.PlayerObjectID(), 0) // the creature's on the player: untagged
			w.sendChatToSession(sess, fmt.Sprintf("%s's %s hits you for %d.", npc.Name, name, m.Damage))
		}
		log.Printf("Template: %s's %s hits %s for %d (HP %d/%d)", npc.Name, name, p.Name, m.Damage, p.HP, p.MaxHPNow())
		if p.HP > 0 {
			continue
		}
		w.playerDiedLocked(sess, p, now)
		if p.IsDead() {
			w.killedPlayerLocked(npc, p) // its line at the kill (selfbuff.go)
		}
	}
}
