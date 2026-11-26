package entity

import "github.com/eqoa/server/internal/session"

// The fight a player is in keeps its place in the player's view (Heirloom
// petition 3156, Grimmy, 2026-11-26, a raid on Bertoxxulous: "when the mob
// goes away from too many avatars, I lose con on it. I should keep its
// con."). A crowd of players, mercenaries and pets nearer than the creature
// filled every entity slot (48 asked, 48 shown, her target "not shown"), so
// the visibility pass, nearest first, gave the creature's slot to one of
// them as it stepped back: CLIENT deleted it, and with it her target and
// its con. The pass now puts the player's fight ahead of the nearer crowd
// (visibilitySystem's keep, after a quest's walls and blockers):
//
//   - the player's own target (its last 0x0010);
//   - its group's main assist's target (raidrules.go), and its pet's,
//     mercenaries' and familiar's enemies;
//   - every creature hitting it or holding its hate.
//
// Only an entity already a candidate is kept, so one past the client's view
// distance still leaves it; its pets and its tour's riders (DistSq -1,
// pinPetLocked, pinTourLocked) stay kept as they were. Kept entities among
// themselves go nearest first, so a fight with more creatures than slots
// keeps the nearest of them.

// keepFightLocked marks the candidates of p's fight kept (keepFight). Must
// be called with the lock held.
func (w *World) keepFightLocked(sess *session.Session, p *Player, cands []entityDist) {
	ids := w.fightOfLocked(sess, p)
	for i := range cands {
		c := &cands[i]
		if c.DistSq < 0 { // a pet's, a merc's, a tour's: kept as ever
			c.keep = true
			continue
		}
		if _, ok := ids[c.ID]; ok {
			c.keep, c.DistSq = true, -1 // its target(s): ahead of the rest of the fight
			continue
		}
		if n, ok := w.NPCs[c.ID]; ok && !n.Dead && n.fightsLocked(p) {
			c.keep = true
		}
	}
}

// fightsLocked: n is hitting p, or holds p's hate.
func (n *NPC) fightsLocked(p *Player) bool {
	if n.CurrentEnemy == p.ServerID {
		return true
	}
	return n.Threats[p.ServerID] > 0
}

// fightOfLocked is the targets p's view keeps first: its own, its group's
// main assist's, its pet's, mercenaries' and familiar's enemies.
func (w *World) fightOfLocked(sess *session.Session, p *Player) map[EntityID]struct{} {
	ids := make(map[EntityID]struct{}, 4)
	add := func(e Entity) {
		if e != nil && e.GetEntityID() != p.ServerID {
			ids[e.GetEntityID()] = struct{}{}
		}
	}
	if p.TargetObjectID != 0 && p.TargetObjectID != NoAssist {
		add(w.entitySeenByLocked(sess, p.TargetObjectID))
	}
	if g := w.groups[p.GroupID]; p.GroupID != 0 && g != nil && g.rules.assist != 0 && g.rules.assist != p.ServerID {
		if ma := w.Players[g.rules.assist]; ma != nil {
			add(w.targetOfLocked(ma))
		}
	}
	enemy := func(n *NPC) {
		if n != nil && !n.Dead && n.CurrentEnemy != 0 {
			if e, ok := w.NPCs[n.CurrentEnemy]; ok {
				add(e)
			}
		}
	}
	enemy(w.petOfLocked(p))
	for _, ms := range w.mercsOfLocked(p) {
		enemy(ms.npc)
	}
	if fs := w.familiars[p.ServerID]; fs != nil && fs.owner == p {
		enemy(fs.npc)
	}
	return ids
}
