package entity

import (
	"log"
	"regexp"
	"strings"
	"time"
)

// Town guards defend players (F3 20261030-063534, jib: "guards should assist
// players that get attacked in front of them, within a range of 30 yards. If
// they engage a monster though, no loot or xp should be given to the player
// if the guards hit it."; the user chose it 2026-10-31).
//
// Who is a town guard (townGuardLocked): an NPC whose npcsocial row is social
// and whose faction there is a town's guard faction, a guard's word in a
// faction DialogTown places in a town (The Freeport Militia, The Qeynos
// Guard, The Highpass Guard, Klick`Anon Watch) or "Guards of <town>". Those
// are exactly tools/seed_npc_social.py's rows: every town's guards, found off
// factionhits' "Citizens of" rows and a guard's word in the name (298 names
// on eqoaclassic). Not a guard by its name alone, which takes in the hostile
// camps' human-bodied spawns ("Jextalan guard", town-dialog-20261029.md), nor
// a camp a GM made social under a faction of its own; a GM makes another
// guard by its row.
//
// When (guardsDefendLocked): a creature fighting a player, in its reach, is
// looked at by the town guards within DefaultSocialRadius (30, the row's own
// radius when it says) of that player, at most once a second a creature. A
// guard takes the creature on as its own fight when it is free (alive, not
// fighting, not on its way home, not hidden, no NPC bit) and would not attack
// that player itself (threatensLocked: the red standing, below -350, or a
// fight with it). It leaves alone a creature that is a town guard itself, of
// its own faction or of its town's people: a town's fight with a player it
// will not take up. The fight is a fight's, as with a player (chase.go): the
// chase, the leash (LeashRadius from where it began, its post) and the way
// home.
//
// An NPC fighting another that is no pet (npcFoeLocked: a guard and its
// creature, either way) fights it in the rounds it fights a player in, its
// hits a creature's on a creature (npcHitOnPet, balanced for its level
// against the other's: npcHitLocked); whom the creature fights is its threat
// as ever, so it turns on a guard that hurts it more than the player has.
//
// The kill is the guards': a creature a guard has damaged (guardHit, until
// its fight ends with it alive, or its respawn) gives no experience
// (awardKillXPLocked), no faction hits and no quest kill credit
// (factionKillLocked), and its corpse is not lootable (GuardsKill,
// handlers/loot.go), whoever lands the last blow. Faction hits and quest
// credit go with the experience, as one kill's credit does everywhere else
// here, so a kill the guards made earns a player nothing of a kill's.

// guardLookEvery is how often a creature fighting a player is looked at by
// the guards near it.
const guardLookEvery = time.Second

// guardFactionWord names a guard faction by a whole word: the seed's
// FACTION_WORDS (tools/seed_npc_social.py).
var guardFactionWord = regexp.MustCompile(`(?i)\b(guard|guards|militia|watch|defenders|wardens|warden)\b`)

// GuardTown is the town a faction is the guards of: "Guards of X" is X's, a
// guard's word in a faction DialogTown places is that town's; "" none.
func GuardTown(faction string) string {
	faction = strings.TrimSpace(faction)
	if t, ok := strings.CutPrefix(faction, "Guards of "); ok {
		return strings.TrimSpace(t)
	}
	if !guardFactionWord.MatchString(faction) {
		return ""
	}
	return DialogTown(faction)
}

// townGuardLocked reports a town's guard (the file's doc). Must be called
// with the lock held.
func (w *World) townGuardLocked(npc *NPC) bool {
	r, ok := w.npcSocial.ruleFor(npc.Name)
	if !ok || !r.Social {
		return false
	}
	return GuardTown(w.socialFactionLocked(npc)) != ""
}

// guardRadiusLocked is how near a guard must stand to a player it defends:
// its row's radius, else DefaultSocialRadius. Must be called with the lock
// held.
func (w *World) guardRadiusLocked(g *NPC) float32 {
	if r, ok := w.npcSocial.ruleFor(g.Name); ok && r.Radius > 0 {
		return r.Radius
	}
	return DefaultSocialRadius
}

// ofGuardsTownLocked reports a creature a guard will not take on: a town
// guard, of the guard's own faction, or of its town's people. Must be called
// with the lock held.
func (w *World) ofGuardsTownLocked(g, creature *NPC) bool {
	if w.townGuardLocked(creature) {
		return true
	}
	faction := w.socialFactionLocked(g)
	if w.ofFactionLocked(creature, faction) {
		return true
	}
	town := GuardTown(faction)
	return town != "" && DialogTown(w.npcFactionLocked(creature)) == town
}

// defenderLocked reports a town guard that takes creature on for p (the
// file's doc). Must be called with the lock held.
func (w *World) defenderLocked(g, creature *NPC, p *Player, now time.Time) bool {
	if g == creature || g.Dead || g.Owner != 0 || g.Dummy != NotDummy || g.NPCType&0x80 != 0 || g.SpawnOnly ||
		g.World != p.World || g.CurrentEnemy != 0 || g.chase.returning || g.hiddenAt(now) {
		return false
	}
	if !w.townGuardLocked(g) {
		return false
	}
	if r := w.guardRadiusLocked(g); dist3(g.X, g.Y, g.Z, p.X, p.Y, p.Z) > r {
		return false
	}
	if _, hurt := g.Threats[p.ServerID]; hurt || w.threatensLocked(g, p) {
		return false // it would attack the player itself
	}
	return !w.ofGuardsTownLocked(g, creature)
}

// guardsDefendLocked is creature's fight with p seen by the town guards near
// p: each that defends p takes the creature on (the file's doc). Must be
// called with the lock held.
func (w *World) guardsDefendLocked(creature *NPC, p *Player, now time.Time) {
	if len(w.npcSocial.rules) == 0 || w.SpatialIndex == nil || creature.Owner != 0 || creature.Dummy != NotDummy ||
		creature.Dead || p.IsDead() || now.Before(creature.guardLookAt) {
		return
	}
	creature.guardLookAt = now.Add(guardLookEvery)
	if w.townGuardLocked(creature) {
		return // a guard on a player is its town's business
	}
	for _, id := range w.SpatialIndex.QueryRadius(p.X, p.Z, w.npcSocial.reach) {
		g, ok := w.NPCs[id]
		if !ok || !w.defenderLocked(g, creature, p, now) {
			continue
		}
		w.addThreatLocked(g, creature.ServerID, 1)
		log.Printf("Guards: %s takes on %s, which fights %s %.1f off it", g.Name, creature.Name, p.Name,
			dist3(g.X, g.Y, g.Z, p.X, p.Y, p.Z))
	}
}

// npcFoeLocked is the NPC npc fights when it is no player and no pet: a
// guard's creature, or the guard a creature fights (the file's doc), alive
// and in its world; nil otherwise. Must be called with the lock held.
func (w *World) npcFoeLocked(npc *NPC) *NPC {
	foe := w.NPCs[npc.CurrentEnemy]
	if foe == nil || foe == npc || foe.Owner != 0 || foe.Dead || foe.Dummy != NotDummy || foe.World != npc.World {
		return nil
	}
	return foe
}

// npcFightNPCLocked is npc's fight with foe this tick, as with a player
// (aiSystem): the chase and the leash (chase.go), then its rounds
// (rounds.go): its swing to everyone who sees it, then its hits.
func (w *World) npcFightNPCLocked(npc, foe *NPC, now time.Time) {
	in, giveUp := w.chaseLocked(npc, foe.X, foe.Z, foe.VelX, foe.VelZ, now)
	if giveUp {
		w.giveUpLocked(npc)
		return
	}
	if npc.RoundTarget != foe.ServerID {
		npc.endRound()
		npc.RoundTarget = foe.ServerID
	}
	if !in {
		npc.loseDueHits(now) // closing in: no hit lands from out of reach
		return
	}
	length, delay, fx, hits := npcRound(npc)
	if npc.RoundStart.IsZero() || !now.Before(npc.RoundStart.Add(length)) {
		next := npc.RoundStart.Add(length)
		if npc.RoundStart.IsZero() || now.Sub(next) >= length {
			next = now
		}
		npc.RoundStart, npc.RoundHits = next, 0
		w.broadcastNPCSwingLocked(npc, foe.ServerID, uint32(delay/time.Millisecond), fx)
	}
	for npc.RoundHits < len(hits) && !now.Before(npc.RoundStart.Add(hits[npc.RoundHits])) {
		npc.RoundHits++
		if w.npcHitNPCLocked(npc, foe, now) {
			return
		}
	}
}

// npcHitNPCLocked is one of npc's hits on foe: a creature's roll on a
// creature (npcHitOnPet), its class template's, balanced for its level
// against foe's (npcHitLocked), as foe's template takes it; foe's hate for
// npc, a guard's mark on it (guardHit), and its death. It reports a kill.
func (w *World) npcHitNPCLocked(npc, foe *NPC, now time.Time) bool {
	n := foe.meleeTaken(w.npcHitLocked(npc, foe.Level, npc.meleeOut(npcHitOnPet(npc, foe, meleeIntn))))
	foe.HP = max(foe.HP-n, 0)
	npc.LastCombat = now
	if w.townGuardLocked(npc) {
		foe.guardHit = true
	}
	w.addThreatLocked(foe, npc.ServerID, n)
	log.Printf("Combat: %s hit %s for %d (HP %d/%d)", npc.Name, foe.Name, n, foe.HP, foe.MaxHP)
	if foe.HP > 0 {
		return false
	}
	foe.Effects = nil
	w.killNPC(foe)
	w.dropAggroLocked(foe.ServerID)
	log.Printf("Combat: %s killed by %s", foe.Name, npc.Name)
	return true
}

// guardsKillLocked reports a kill that is the guards' (the file's doc).
func guardsKillLocked(npc *NPC) bool {
	return npc != nil && npc.guardHit
}

// GuardsKill reports whether the NPC's corpse, or its fight, is the guards'
// (the file's doc): a guard has damaged it since its fight began.
func (w *World) GuardsKill(id EntityID) bool {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return guardsKillLocked(w.NPCs[id])
}
