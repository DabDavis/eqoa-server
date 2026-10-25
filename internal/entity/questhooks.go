package entity

import (
	"log"

	"github.com/eqoa/server/internal/session"
)

// What the quests (handlers/quests.go) need of the World: every kill a player
// is credited with, its experience, its faction changes and where a player
// stands, each taking the World's lock itself.

// KillShareRange is how near a kill a group member must stand to share it.
const KillShareRange float32 = 200

// KillCredit is a creature's death a player is credited with (a melee kill, a
// spell's, a pet's, a GM's #kill): the creature, and the players it counts for,
// the killer first, then the members of its group in the same world within
// KillShareRange.
type KillCredit struct {
	NPCName  string
	NPCSpawn EntityID // its spawn row (npcs.id); 0 for a GM's #spawn copy (no row, its original's name)
	Level    int
	World    uint8
	Players  []KillPlayer
}

// KillPlayer is one player a kill counts for.
type KillPlayer struct {
	Session  *session.Session
	ServerID uint32 // the character's (characters.serverid)
	Name     string
}

// KillFunc hears a kill. It is called with the World's lock held, so it must
// not call back into the World, nor block.
type KillFunc func(KillCredit)

// OnKill adds a hearer of every credited kill.
func (w *World) OnKill(fn KillFunc) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.onKill = append(w.onKill, fn)
}

// creditKillLocked tells the kill's hearers. Must be called with the lock held.
func (w *World) creditKillLocked(sess *session.Session, p *Player, npc *NPC) {
	if len(w.onKill) == 0 || p == nil || npc == nil {
		return
	}
	spawn := npc.ServerID
	if npc.Temporary {
		spawn = 0
	}
	c := KillCredit{NPCName: npc.Name, NPCSpawn: spawn, Level: npc.Level, World: npc.World}
	c.Players = append(c.Players, KillPlayer{Session: sess, ServerID: p.CharServerID, Name: p.Name})
	if g := w.groups[p.GroupID]; p.GroupID != 0 && g != nil {
		for _, id := range g.Members {
			m := w.Players[id]
			if m == nil || m == p || m.World != npc.World || m.IsDead() {
				continue
			}
			dx, dz := m.X-npc.X, m.Z-npc.Z
			if dx*dx+dz*dz > KillShareRange*KillShareRange {
				continue
			}
			c.Players = append(c.Players, KillPlayer{Session: w.sessionOfLocked(m), ServerID: m.CharServerID, Name: m.Name})
		}
	}
	for _, fn := range w.onKill {
		fn(c)
	}
}

// AwardXP gives a player experience as a kill does: levels, the masteries'
// share, the lines and the client's 0x0020.
func (w *World) AwardXP(sess *session.Session, p *Player, xp int) {
	if sess == nil || p == nil || xp <= 0 {
		return
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	w.awardXP(sess, p, xp)
}

// ApplyFactionHits moves a player's standings as a kill's hits do, each told in
// EverQuest's words unless Faction Messages are off.
func (w *World) ApplyFactionHits(sess *session.Session, p *Player, hits []FactionHit) {
	if p == nil || len(hits) == 0 {
		return
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	quiet := sess == nil || sess.FactionMessagesOff()
	for _, h := range hits {
		if h.Delta == 0 || h.Faction == "" {
			continue
		}
		old := w.standingLocked(p, h.Faction)
		now := clampFaction(old + h.Delta)
		if now != old {
			if p.Factions == nil {
				p.Factions = make(map[string]int)
			}
			p.Factions[h.Faction] = now
		}
		log.Printf("Factions: %s %s %d -> %d (a quest)", p.Name, h.Faction, old, now)
		if !quiet {
			w.sendChatToSession(sess, factionLine(h.Faction, h.Delta, now != old))
		}
	}
}

// PlayerPlace is where a player stands now, and whether it is a live player
// in the world.
func (w *World) PlayerPlace(p *Player) (world uint8, x, y, z float32, ok bool) {
	if p == nil {
		return 0, 0, 0, 0, false
	}
	w.mu.RLock()
	defer w.mu.RUnlock()
	cur, found := w.Players[p.ServerID]
	if !found || cur != p || p.IsDead() {
		return 0, 0, 0, 0, false
	}
	return p.World, p.X, p.Y, p.Z, true
}
