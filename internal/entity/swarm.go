package entity

import (
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/eqoa/server/internal/database"
)

// swarm.go — a boss's announced wind-up (database.AbilitySwarm): Festering Jack's Swarm Mentality on
// the Isle of Dread (Packmaster's petition 3122 and his addition, the owner 2026-11-25: "do the
// Festering Jack proposal"). As Packmaster remembers the live fight:
//
//	"it anounces that he is going to use it in say chat, then a few seconds later he quads, and
//	immediately after the quad hit he aoe deathtouches anyone within a small range from HIS location.
//	The secret well kept, is that a rogue could use harrass on him to stop the deathtouch."
//	"he roots himself in place right before he announces Swarm Mentality and stays rooted until he
//	uses it ... People can hide out of his line of sight because he's rooted"
//
// So, a boss moment (bossmoments.go: its clock Every, First, Chance, begun with its fight):
//
//   - it begins: the creature roots itself where it stands (rooted, roam.go: no chase, no walk home,
//     no wander) and says its Say in /say (none: "<Name>!"); only with someone to fight, and never a
//     second while one is under way;
//   - Hold seconds later (SwarmHoldOr) it lands SwarmHitsOr of its melee hits on the one it fights
//     (npcMeleeOnPlayerLocked / npcMeleeOnPetLocked, each rolled as a round's hit), then kills outright
//     every player within SwarmRadiusOr of it to whom it has a clear line from its own heights (sight.go,
//     as a sethp's area: a rock or a wall between them spares one), no resist, no mitigation; its root
//     ends;
//   - a spell of the players' named in SwarmBreaksOr (the rogue's Harass) that lands on it while it is
//     rooted (not resisted: landOnNPCLocked) breaks it: no hits, no death, its root ends, and a line to
//     the caster and every player on its hate list says so;
//   - its fight's end (endTemplateFight) or its death takes it, root and all.
//
// The words of the lines are ours.

// swarmOn is a swarm under way in a fight: the ability and when it lands.
type swarmOn struct {
	m  *database.TemplateAbility
	at time.Time
}

// swarmName is what a swarm's lines call it.
func swarmName(m *database.TemplateAbility) string {
	if m.Name != "" {
		return m.Name
	}
	return database.DefaultSwarmName
}

// swarmRooted is whether npc stands rooted for its swarm.
func (n *NPC) swarmRooted() bool {
	return n.tfight != nil && n.tfight.swarm != nil
}

// swarmStartLocked begins npc's swarm m (the file's doc). Must be called with the lock held.
func (w *World) swarmStartLocked(npc *NPC, m *database.TemplateAbility, now time.Time) {
	f := npc.fight()
	if f.swarm != nil || npc.Dead || npc.CurrentEnemy == 0 {
		return
	}
	f.swarm = &swarmOn{m: m, at: now.Add(time.Duration(m.SwarmHoldOr() * float64(time.Second)))}
	npc.stopMoving()
	say := m.Say
	if say == "" {
		say = swarmName(m) + "!"
	}
	w.sayNearLocked(npc.Name, npc.World, npc.X, npc.Z, say)
	log.Printf("Template: %s roots itself for its %s (in %.1f s)", npc.Name, swarmName(m), m.SwarmHoldOr())
}

// swarmDueLocked lands npc's swarm in its fight f when its time has come: its hits on the one it fights,
// then its death on everyone it reaches (the file's doc). Must be called with the lock held.
func (w *World) swarmDueLocked(npc *NPC, f *templateFight, now time.Time) {
	s := f.swarm
	if s == nil || now.Before(s.at) {
		return
	}
	f.swarm = nil
	m := s.m
	name := swarmName(m)
	if p := w.Players[npc.CurrentEnemy]; p != nil {
		for i := 0; i < m.SwarmHitsOr() && !npc.Dead && !p.IsDead() && p.World == npc.World; i++ {
			w.npcMeleeOnPlayerLocked(npc, p, 100, true, now)
		}
	} else if pet := w.NPCs[npc.CurrentEnemy]; pet != nil && pet.Owner != 0 {
		for i := 0; i < m.SwarmHitsOr() && !npc.Dead && !pet.Dead; i++ {
			if w.npcMeleeOnPetLocked(npc, pet, 100, true, now) {
				break
			}
		}
	}
	if npc.Dead {
		return
	}
	ps := w.swarmReachLocked(npc, m, now)
	log.Printf("Template: %s's %s (radius %g): %d players in its sight", npc.Name, name, m.SwarmRadiusOr(), len(ps))
	if len(ps) == 0 {
		return
	}
	fx := m.FX
	if fx == 0 {
		fx = templateSpecialFX
	}
	others := make([]EntityID, 0, len(ps))
	for _, p := range ps {
		others = append(others, p.ServerID)
	}
	w.broadcastNPCCastLocked(npc, npc.ServerID, 0, fx, others...)
	for _, p := range ps {
		if !p.IsDead() {
			w.deathTouchPlayerLocked(npc, p, name, now)
		}
	}
}

// swarmReachLocked is whom npc's swarm kills now: the players within its radius, alive, of its world
// and side, that it notices and has a clear line to from its own heights (sight.go), in id order. Must
// be called with the lock held.
func (w *World) swarmReachLocked(npc *NPC, m *database.TemplateAbility, now time.Time) []*Player {
	if w.SpatialIndex == nil {
		return nil
	}
	r := float32(m.SwarmRadiusOr())
	place := npcAreaPlace(npc, npc.X, npc.Y, npc.Z, false)
	var ps []*Player
	for _, id := range w.SpatialIndex.QueryRadius(npc.X, npc.Z, r) {
		p := w.Players[id]
		if p == nil || p.World != npc.World || !w.areaTakesPlayerLocked(npc, p) { // safemode.go
			continue
		}
		if dist3(p.X, p.Y, p.Z, npc.X, npc.Y, npc.Z) <= r && w.reachesLocked(place, p.X, p.Y, p.Z) {
			ps = append(ps, p)
		}
	}
	sortPlayersByID(ps)
	return ps
}

// swarmBreakLocked is a player's spell landing on npc (landOnNPCLocked): when npc is rooted for its swarm
// and the spell is one that breaks it, the swarm ends with nothing (the file's doc). It reports a break.
// Must be called with the lock held.
func (w *World) swarmBreakLocked(npc *NPC, caster *Player, spell string, now time.Time) bool {
	if npc == nil || npc.tfight == nil || npc.tfight.swarm == nil || npc.Dead {
		return false
	}
	m := npc.tfight.swarm.m
	spell = strings.TrimSpace(spell)
	breaks := false
	for _, b := range m.SwarmBreaksOr() {
		if strings.EqualFold(strings.TrimSpace(b), spell) {
			breaks = true
			break
		}
	}
	if !breaks {
		return false
	}
	npc.tfight.swarm = nil
	line := fmt.Sprintf("%s's %s breaks %s's %s!", caster.Name, spell, npc.Name, swarmName(m))
	told := map[EntityID]bool{}
	tell := func(p *Player) {
		if p == nil || told[p.ServerID] {
			return
		}
		told[p.ServerID] = true
		if sess := w.findSessionByEntityIDLocked(p.ServerID); sess != nil {
			w.sendColoredLocked(sess, line, duelOrange)
		}
	}
	tell(caster)
	for id := range npc.Threats {
		tell(w.Players[id])
	}
	log.Printf("Template: %s's %s broken by %s's %s", npc.Name, swarmName(m), caster.Name, spell)
	return true
}
