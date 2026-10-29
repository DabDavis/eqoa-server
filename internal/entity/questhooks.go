package entity

import (
	"log"
	"math"
	"time"

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
	NPCID    EntityID // the creature itself, whose corpse a loot step's drop goes on
	Died     time.Time
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
	c := KillCredit{NPCName: npc.Name, NPCSpawn: spawn, NPCID: npc.ServerID, Died: npc.DeathTime, Level: npc.Level, World: npc.World}
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
// Sandstorm's line (factionLine) unless Faction Messages are off.
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
			w.sendChatToSession(sess, factionLine(h.Faction, h.Delta, now))
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

// QuestSpawnAhead is how far in front of the player a quest's spawn stands.
const QuestSpawnAhead float32 = 4

// SpawnForQuest places a copy of the NPC named name (SpawnNPCCopy's pick) a
// few steps in front of the player, facing it: a quest step's spawn event.
// A hostile one may be attacked (the NPC bit 0x80 cleared) and attacks the
// player at once. It returns the copy's id and the name as the NPC spells it.
func (w *World) SpawnForQuest(name string, p *Player, hostile bool) (EntityID, string, bool) {
	if p == nil {
		return 0, "", false
	}
	w.mu.RLock()
	x, y, z, world, facing := p.X, p.Y, p.Z, p.World, float64(p.Facing)
	w.mu.RUnlock()
	// the facing CLIENT reads: the direction (sin h, cos h) (npcmove.go)
	sx := x + QuestSpawnAhead*float32(math.Sin(facing))
	sz := z + QuestSpawnAhead*float32(math.Cos(facing))
	id, spelled, ok := w.SpawnNPCCopy(name, sx, y, sz, headingTo(x-sx, z-sz), world)
	if !ok {
		return 0, "", false
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	if c := w.NPCs[id]; c != nil {
		c.QuestSpawn = true
		if hostile {
			c.NPCType &^= 0x80
			if cur, found := w.Players[p.ServerID]; found && cur == p && !p.IsDead() {
				w.addThreatLocked(c, p.ServerID, 1)
			}
		}
	}
	return id, spelled, true
}

// PlayVisualOn plays a spell visual from sess's player onto itself, as
// PlayVisual does onto its target: a quest's visual event.
func (w *World) PlayVisualOn(sess *session.Session, fx uint32, castTime time.Duration) bool {
	castTime = max(0, min(castTime, MaxVisualCast))
	w.mu.Lock()
	defer w.mu.Unlock()
	p := w.sessionPlayerLocked(sess)
	if p == nil || !sess.CharacterInWorld() {
		return false
	}
	w.broadcastCastLocked(p, p.ServerID, castTime, fx)
	return true
}

// NPCAlive is whether the NPC of that id is in the world and not dead.
func (w *World) NPCAlive(id EntityID) bool {
	w.mu.RLock()
	defer w.mu.RUnlock()
	n, ok := w.NPCs[id]
	return ok && !n.Dead
}
