package entity

import (
	"fmt"
	"log"
	"time"

	"github.com/eqoa/server/internal/database"
)

// kickzone.go — a creature guarding the ground round it (database.AbilityKickZone; the GM
// Jibwiz's petition 1956, 2026-11-13, the Anvil of Sarek: "non agro (white face), non attackable,
// permanently in that spot. If a player hits the distance my character is currently at from him he
// should do a kick animation and hit them for 10,000 unresistable damage every second. That's all
// he does.").
//
// A creature whose class template has a kickzone keeps it in a fight or out of one: every Every
// seconds (0: one) while one is in it, each living player in its world (and its instance) within
// Radius of it across and no more than Radius above or below is kicked: the creature turns to it
// with its special's swing (0x00B1, the ability's FX for another), and the player takes Damage at
// once, fixed (no resist, no armour, no balance), told as a hit from the creature (0x00DB), and
// may die of it. Not kicked: the dead, one on its way back from a death, a GM flying (#fly). Its
// clock waits while no one is in it, so the first to step in is kicked at once. It is the whole of
// what the zone does: no hate, no fight. Being unattackable and not aggressive are the creature's
// own rows (the NPC bit, npcaggro), not the zone's.

// kickZoneState is a creature with a kick zone, and when each of its zones may kick next.
type kickZoneState struct {
	npc  *NPC
	next []time.Time
}

// kickZonesOf is the kick zones of a template (nil for none).
func (t *ClassTemplate) kickZonesOf() []database.TemplateAbility {
	if t == nil {
		return nil
	}
	return t.kickZones
}

// kickZoneWatchLocked keeps the world's list of creatures with a kick zone as npc's template
// changes. Must be called with the lock held.
func (w *World) kickZoneWatchLocked(npc *NPC) {
	zs := npc.tmpl.kickZonesOf()
	if len(zs) == 0 {
		delete(w.kickZones, npc.ServerID)
		return
	}
	if w.kickZones == nil {
		w.kickZones = make(map[EntityID]*kickZoneState)
	}
	w.kickZones[npc.ServerID] = &kickZoneState{npc: npc, next: make([]time.Time, len(zs))}
}

// kickZoneSystem kicks the players in each creature's zone that is due (the file's doc). Every
// tick; must be called with the lock held.
func (w *World) kickZoneSystem(now time.Time) {
	for id, ks := range w.kickZones {
		npc := ks.npc
		if w.NPCs[id] != npc {
			delete(w.kickZones, id) // gone from the world, or made again (its own entry then)
			continue
		}
		zs := npc.tmpl.kickZonesOf()
		if npc.Dead || npc.SpawnOnly || len(zs) != len(ks.next) {
			continue
		}
		for i := range zs {
			if now.Before(ks.next[i]) {
				continue
			}
			if w.kickZoneLocked(npc, &zs[i], now) {
				every := zs[i].Every
				if every <= 0 {
					every = 1
				}
				ks.next[i] = now.Add(time.Duration(every * float64(time.Second)))
			}
		}
	}
}

// kickZoneLocked kicks every player in npc's zone z; it reports whether any was. Must be called
// with the lock held.
func (w *World) kickZoneLocked(npc *NPC, z *database.TemplateAbility, now time.Time) bool {
	if w.SpatialIndex == nil {
		return false
	}
	r := float32(z.Radius)
	var in []*Player
	for _, p := range w.Players {
		if p.World != npc.World || p.IsDead() || p.HP <= 0 || p.respawnPending() || p.flyingNow(now) ||
			!w.npcSharesLocked(npc, p) {
			continue
		}
		if dy := p.Y - npc.Y; dy > r || dy < -r || distXZ(p.X, p.Z, npc.X, npc.Z) > r {
			continue
		}
		in = append(in, p)
	}
	if len(in) == 0 {
		return false
	}
	name := z.Name
	if name == "" {
		name = database.DefaultKickZoneName
	}
	fx := z.FX
	if fx == 0 {
		fx = templateSpecialFX
	}
	if z.Say != "" {
		w.sayNearLocked(npc.Name, npc.World, npc.X, npc.Z, z.Say)
	}
	for _, p := range in {
		w.broadcastNPCSwingLocked(npc, p.ServerID, 0, fx)
		n := min(z.Damage, p.HP)
		p.HP -= n
		p.StatsDirty = true
		p.LastCombatTime = now
		sess := w.findSessionByEntityIDLocked(p.ServerID)
		if sess != nil {
			from := w.objectIDForEntityLocked(sess, npc.ServerID)
			if from == 0 {
				from = sess.PlayerObjectID()
			}
			w.sendDamageIndication(sess, from, -z.Damage, sess.PlayerObjectID(), 0)
			w.sendColoredLocked(sess, fmt.Sprintf("%s's %s hits you for %d.", npc.Name, name, z.Damage), duelOrange)
		}
		log.Printf("Kick zone: %s's %s hits %s for %d (HP %d/%d)", npc.Name, name, p.Name, z.Damage, p.HP, p.MaxHPNow())
		if p.HP <= 0 {
			w.playerDiedLocked(sess, p, now)
			if p.IsDead() {
				w.killedPlayerLocked(npc, p)
			}
		}
	}
	return true
}
