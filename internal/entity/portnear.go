package entity

import (
	"fmt"
	"log"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/eqoa/server/internal/database"
)

// portnear.go — two of a boss's moments (bossmoments.go), the owner's for
// Quintashi, 2026-11-13: "quintashi should have a death touch at the
// beginning of the fight for the first target. also he will port players
// close to the mobs in the chamber so they get attacked by them", then "make
// the port once every 3 mins".
//
// A death touch (database.AbilityDeathTouch) kills the one its creature
// fights, a player, outright: its hit points to 0, no resist, no mitigation,
// a death as any other. With first 0 it lands as the fight begins, on its
// first target; its every as long as a fight is once.
//
// A port (database.AbilityPortNear) sends Count players (0: one) of its
// creature's hate list, at random, beside a creature round it within Radius:
// one alive, not of the players' side, one a player may attack, not on its
// creature's hate list, of Foes' names when it has some; the players alive, in
// its world, not on their way somewhere already, and not the one it fights
// while another is there to send. Each lands on its own creature where one is
// to spare, and that creature and every other of the same kind within
// portNearReach of it (Foes' names, or any) take the player on: hate, a fight.

// portNearReach is how near the creature a player is sent beside the others
// that take it on stand.
const portNearReach float32 = 30

// deathTouchLocked is a death touch of npc's (the file's doc). Must be called
// with the lock held.
func (w *World) deathTouchLocked(npc *NPC, m *database.TemplateAbility, now time.Time) {
	p := w.Players[npc.CurrentEnemy]
	if p == nil || p.IsDead() || p.respawnPending() || p.World != npc.World || !w.npcSharesLocked(npc, p) {
		return
	}
	name := momentName(m)
	fx := m.FX
	if fx == 0 {
		fx = templateSpecialFX
	}
	took := p.HP
	w.broadcastNPCSwingLocked(npc, p.ServerID, 0, fx)
	p.HP = 0
	p.StatsDirty = true
	p.LastCombatTime = now
	sess := w.findSessionByEntityIDLocked(p.ServerID)
	if sess != nil {
		from := w.objectIDForEntityLocked(sess, npc.ServerID)
		if from == 0 {
			from = sess.PlayerObjectID()
		}
		w.sendDamageIndication(sess, from, -took, sess.PlayerObjectID(), 0)
		w.sendColoredLocked(sess, fmt.Sprintf("%s's %s takes your life.", npc.Name, name), duelOrange)
	}
	log.Printf("Template: %s's %s kills %s (%d hit points)", npc.Name, name, p.Name, took)
	w.playerDiedLocked(sess, p, now)
	if p.IsDead() {
		w.killedPlayerLocked(npc, p)
	}
}

// portNearLocked is a port of npc's (the file's doc). Must be called with the
// lock held.
func (w *World) portNearLocked(npc *NPC, m *database.TemplateAbility, now time.Time) {
	foes := map[string]bool{}
	for _, f := range m.Foes {
		foes[strings.ToLower(strings.TrimSpace(f))] = true
	}
	var near []*NPC
	for _, id := range w.SpatialIndex.QueryRadius(npc.X, npc.Z, float32(m.Radius)) {
		n := w.NPCs[id]
		if n == nil || n == npc || n.Dead || n.World != npc.World || n.Owner != 0 || n.Charm != nil || n.NPCType&0x80 != 0 ||
			n.Dummy != NotDummy || npc.Threats[n.ServerID] > 0 || dist3(npc.X, npc.Y, npc.Z, n.X, n.Y, n.Z) > float32(m.Radius) {
			continue
		}
		if len(foes) > 0 && !foes[strings.ToLower(n.Name)] {
			continue
		}
		near = append(near, n)
	}
	if len(near) == 0 {
		log.Printf("Template: %s's %s: no creature within %.0f to send anyone beside", npc.Name, momentName(m), m.Radius)
		return
	}
	sort.Slice(near, func(i, j int) bool { return near[i].ServerID < near[j].ServerID })
	var ps []*Player
	for id, th := range npc.Threats {
		p := w.Players[id]
		if p == nil || th <= 0 || p.IsDead() || p.respawnPending() || p.World != npc.World || !w.npcSharesLocked(npc, p) || w.travelDueLocked(p) {
			continue
		}
		if sess := w.findSessionByEntityIDLocked(p.ServerID); sess == nil || sess.StateChannelsHeld() {
			continue
		}
		ps = append(ps, p)
	}
	sortPlayersByID(ps)
	if len(ps) > 1 {
		ps = slices.DeleteFunc(ps, func(p *Player) bool { return p.ServerID == npc.CurrentEnemy })
	}
	count := max(m.Count, 1)
	name := momentName(m)
	for k := 0; k < count && len(ps) > 0; k++ {
		j := bossMomentIntn(len(ps))
		p := ps[j]
		ps = append(ps[:j], ps[j+1:]...)
		i := bossMomentIntn(len(near))
		at := near[i]
		if len(near) > 1 {
			near = append(near[:i], near[i+1:]...) // each on its own creature while there are some to spare
		}
		sess := w.findSessionByEntityIDLocked(p.ServerID)
		w.gates.due = append(w.gates.due, GateTravel{Sess: sess, P: p, Ring: 1, Spell: npc.Name + "'s " + name,
			To: GateSpot{World: at.World, X: at.X, Y: at.Y, Z: at.Z, Place: "beside " + at.Name}})
		w.sendColoredLocked(sess, fmt.Sprintf("%s's %s sweeps you away to %s!", npc.Name, name, at.Name), duelOrange)
		set := 0
		for _, id := range w.SpatialIndex.QueryRadius(at.X, at.Z, portNearReach) {
			n := w.NPCs[id]
			if n == nil || n == npc || n.Dead || n.World != at.World || n.Owner != 0 || n.Charm != nil || n.NPCType&0x80 != 0 ||
				n.Dummy != NotDummy || dist3(at.X, at.Y, at.Z, n.X, n.Y, n.Z) > portNearReach {
				continue
			}
			if len(foes) > 0 && !foes[strings.ToLower(n.Name)] {
				continue
			}
			w.addThreatLocked(n, p.ServerID, 1)
			set++
		}
		log.Printf("Template: %s's %s sends %s beside %s at (%.1f, %.1f, %.1f): %d creatures set on them",
			npc.Name, name, p.Name, at.Name, at.X, at.Y, at.Z, set)
	}
}
