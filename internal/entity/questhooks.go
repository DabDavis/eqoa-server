package entity

import (
	"fmt"
	"log"
	"math"
	"strings"
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
	// Group is the killer's group (Player.GroupID, a raid's too) at the death,
	// 0 for none: the loot's kill group (handlers/lootrights.go)
	Group uint32
	// Raid is the killer's raid's loot rule and seats when it has one but free for
	// all (raidrules.go's RaidKill), nil otherwise
	Raid *RaidKill
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

// LevelUpFunc hears a player's level rise (an award's: awardXP). It is
// called with the World's lock held, so it must not call back into the World,
// nor block (the heirlooms' refresh goes on its own goroutine:
// handlers/heirloom.go).
type LevelUpFunc func(sess *session.Session, p *Player)

// OnLevelUp adds a hearer of every level up.
func (w *World) OnLevelUp(fn LevelUpFunc) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.onLevelUp = append(w.onLevelUp, fn)
}

// creditKillLocked tells the kill's hearers. Must be called with the lock held.
func (w *World) creditKillLocked(sess *session.Session, p *Player, npc *NPC) {
	if p == nil || npc == nil {
		return
	}
	raid := w.raidKillLocked(p, npc) // the kicked members' claims taken either way (raidrules.go)
	if npc.challenge != nil || npc.Name == MetalMoogieName || npc.Name == MetalMoogieV2Name {
		// a Metal Moogie's corpse is his challenger's alone (metalmoogie.go's
		// ChallengeLooter), never a raid's to roll (the owner, 2026-11-13: "moogie
		// tribute should only go to the player who summons metal moogie")
		raid = nil
	}
	if len(w.onKill) == 0 {
		return
	}
	spawn := npc.ServerID
	if npc.Temporary {
		spawn = 0
	}
	c := KillCredit{NPCName: npc.Name, NPCSpawn: spawn, NPCID: npc.ServerID, Died: npc.DeathTime, Level: npc.Level, World: npc.World, Raid: raid,
		Group: p.GroupID}
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
	return w.SpawnForQuestOf(name, p, hostile, 0, 1)
}

// SpawnForQuestOf is SpawnForQuest's i-th copy of n (a spawn event's count):
// the n round the player's front, a step apart, the middle one before it.
func (w *World) SpawnForQuestOf(name string, p *Player, hostile bool, i, n int) (EntityID, string, bool) {
	if p == nil {
		return 0, "", false
	}
	w.mu.RLock()
	x, y, z, world, facing := p.X, p.Y, p.Z, p.World, float64(p.Facing)
	w.mu.RUnlock()
	if n > 1 {
		facing += (float64(i) - float64(n-1)/2) * 0.7
	}
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

// QuestAttackReach is how near the player an attack event's NPC stands: the
// one it has just talked to, or one in the same room.
const QuestAttackReach float32 = 30

// QuestAttack turns the NPC called name nearest the player (in its world,
// within QuestAttackReach, alive and in view) on the player, as a hostile
// quest spawn attacks (SpawnForQuest): a quest step's attack event, the man
// the player came to see refusing it. A pet, a charmed creature, a dummy and
// one with the NPC bit (a townsperson players could not hit back; a seed
// set's attackable list clears it) are left alone. It returns the NPC's id,
// and why it did not turn ("" when it did).
func (w *World) QuestAttack(name string, p *Player) (EntityID, string) {
	name = strings.TrimSpace(name)
	if p == nil || name == "" {
		return 0, "no NPC named"
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	if cur, ok := w.Players[p.ServerID]; !ok || cur != p || p.IsDead() {
		return 0, "the player is not in the world"
	}
	now := time.Now()
	var best *NPC
	bestD := QuestAttackReach * QuestAttackReach
	for _, n := range w.NPCs {
		if n.Dead || n.SpawnOnly || n.World != p.World || now.Before(n.HiddenUntil) ||
			!strings.EqualFold(strings.TrimSpace(n.Name), name) {
			continue
		}
		dx, dz := n.X-p.X, n.Z-p.Z
		if d := dx*dx + dz*dz; d <= bestD {
			best, bestD = n, d
		}
	}
	switch {
	case best == nil:
		return 0, fmt.Sprintf("none within %.0f", QuestAttackReach)
	case best.Owner != 0 || best.Charm != nil:
		return best.ServerID, "a pet"
	case best.Dummy != NotDummy:
		return best.ServerID, "a training dummy"
	case best.NPCType&0x80 != 0:
		return best.ServerID, "it has the NPC bit: players could not hit it back"
	}
	w.addThreatLocked(best, p.ServerID, 1)
	return best.ServerID, ""
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
