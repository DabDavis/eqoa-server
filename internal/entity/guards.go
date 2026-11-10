package entity

import (
	"log"
	"regexp"
	"strings"
	"time"
)

// Town guards come to the help of a player a creature hits in front of them
// (F3 20261030-063534, jib: "guards should assist players that get attacked
// in front of them, within a range of 30 yards. If they engage a monster
// though, no loot or xp should be given to the player if the guards hit
// it."; the user, 2026-11-04: "lets stop guards from attacking mobs unless a
// player is being hit within 10m"). From 2026-10-31 to then a guard took on
// any creature that came near ("if a creature wanders within range of the
// guard, it should aggro whether player is involved or not"); its leash of
// 100 is that day's too.
//
// Who is a town guard (townGuardLocked): an NPC whose npcsocial row is social
// and whose faction there (the row's, else the one its kill lowers most) is a
// town's guard faction (GuardTown): "Guards of <town>", or a guard's word in
// a faction DialogTown places in a town (The Freeport Militia, The Qeynos
// Guard, The Highpass Guard, Klick`Anon Watch). Those are exactly
// tools/seed_npc_social.py's rows: every town's guards, found off
// factionhits' "Citizens of" rows and a guard's word in the name (298 names
// on eqoaclassic). Not a guard by its name alone, which takes in the hostile
// camps' human-bodied spawns ("Jextalan guard", town-dialog-20261029.md), nor
// a camp a GM made social under a faction of its own; a GM makes another
// guard by its row.
//
// What it does (guardWatchSystem, every aggroScanTicks): a town guard that is
// free (alive, not fighting, not on its way home, not hidden, no NPC bit)
// takes on the nearest creature within DefaultSocialRadius (30, the row's own
// radius when it says) of it that is fighting a player (its current enemy)
// who stands within GuardHelpRadius (10) of the guard, alive and in its world
// (fightsPlayerNearLocked), as its own fight; a creature that only comes
// near, or fights a player farther off, is left alone. Only a generic
// creature (guardsPreyLocked); left alone too:
//   - a player's pet or a charmed creature (Owner, Charm): a player's own;
//   - a town guard, or anything of a guard faction (its npcfaction or social
//     faction): guards do not fight guards, a town's or a camp's;
//   - anything of the guard's own faction, or of its town's people (npcfaction
//     placed in the guard's town): a town's own;
//   - a players' race body or none (townsfolk, merchants, quest givers,
//     bandits; model 0: aggro.go's rule for "no creature"), a sign post's or
//     an invisible blocker's body: people and things, not creatures; with no
//     race bodies known (an empty charactermodel) a creature cannot be told
//     from a townsperson, and no guard looks;
//   - anything with the NPC bit (0x80: no one may attack it), a training,
//     regular or sparring dummy, a quest's spawn (a player's own kill), one
//     kept out of the world (SpawnOnly) or hidden, the dead;
//   - anything a GM's npcaggro row makes peaceful (Aggressive false);
//   - a resident: one whose own spawn point lies within the guard's radius of
//     its post. "Wanders within range" is a creature that comes in; one put
//     there by the data would be killed at every respawn. On eqoaclassic's
//     01:24 UTC database that is 9 spawns and nothing else: 6 of Highpass
//     Hold's pickclaws among its guards (L19-58, the Pickclaw Sashes and
//     Captain Scilond's quests' targets), Qeynos's coin chest, Freeport's
//     Khal Spy and a newbie quests' bat (tools/guards_measure.py);
//   - a town's own: one whose spawn point lies in a town (towns.go's: within
//     townMargin of a chain of its people), as the wander lets one that
//     lives there walk in it and keeps every other out. Tethelin's newbie
//     wildlife lives on the ground under its walkways, in its guards' reach
//     (the user, 2026-11-04: "the wildlife should be below the walkways",
//     then "leave town wildlife alone"): a newbie fighting a bat beside a
//     guard keeps the kill. A creature from outside hitting a player in
//     front of a guard is still taken on.
//
// The fight (npcFoeLocked, npcFightNPCLocked) is a fight's: the chase and the
// way home (chase.go), with a guard's leash GuardLeash from where it began,
// its post, in a fight with a creature (a fight with a player keeps the
// NPC's own); its rounds as a creature's on a pet (npcHitOnPet, balanced for
// its level against the other's: npcHitLocked). Whom the creature fights is
// its threat as ever, so it turns on a guard that hurts it more than a player
// has.
//
// The kill is the guards': a creature a guard has damaged (guardHit, until
// its fight ends with it alive, or its respawn) gives no experience
// (awardKillXPLocked), no faction hits and no quest kill credit
// (factionKillLocked), and its corpse is not lootable (GuardsKill,
// handlers/loot.go), whoever lands the last blow. Faction hits and quest
// credit go with the experience, as one kill's credit does everywhere else
// here, so a kill the guards made earns a player nothing of a kill's.

// GuardLeash is how far from its post a town guard follows a creature before
// it gives the fight up and walks back (the user's, 2026-10-31).
const GuardLeash float32 = 100

// GuardHelpRadius is how near a guard the player a creature fights must stand
// for the guard to take the creature on (the user's "within 10m", 2026-11-04).
const GuardHelpRadius float32 = 10

// fightsPlayerNearLocked reports creature c fighting a player who stands
// within GuardHelpRadius of guard g, alive and in g's world (the file's doc).
// Must be called with the lock held.
func (w *World) fightsPlayerNearLocked(c, g *NPC) bool {
	p, ok := w.Players[c.CurrentEnemy]
	return ok && p != nil && !p.IsDead() && p.World == g.World && dist3(p.X, p.Y, p.Z, g.X, g.Y, g.Z) <= GuardHelpRadius
}

// SignpostBody and BlockerBody are bodies of things, never creatures: every
// sign post and board's (CHAR.ESF 0x6A82352C, handlers/signs.go's SignBody),
// and Sandstorm's invisible blocker's (CHAR.ESF 0x6F412342, collision alone).
const (
	SignpostBody uint32 = 0x6A82352C
	BlockerBody  uint32 = 0x6F412342
)

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

// guardFaction reports a guard faction of any kind: a town's (GuardTown), or
// any faction with a guard's word (a camp's "Jextalan Guards").
func guardFaction(faction string) bool {
	faction = strings.TrimSpace(faction)
	return faction != "" && (strings.HasPrefix(faction, "Guards of ") || guardFactionWord.MatchString(faction))
}

// townGuardLocked reports a town's guard (the file's doc), kept on the NPC
// until the GMs' social rules change. Must be called with the lock held.
func (w *World) townGuardLocked(npc *NPC) bool {
	if w.npcSocial.gen == 0 {
		return false
	}
	if npc.guardGen != w.npcSocial.gen {
		npc.guardGen = w.npcSocial.gen
		npc.townGuard = false
		if r, ok := w.npcSocial.ruleFor(npc.Name); ok && r.Social {
			npc.townGuard = GuardTown(w.socialFactionLocked(npc)) != ""
		}
	}
	return npc.townGuard
}

// guardRadiusLocked is how near a guard a creature must come: its row's
// radius, else DefaultSocialRadius. Must be called with the lock held.
func (w *World) guardRadiusLocked(g *NPC) float32 {
	if r, ok := w.npcSocial.ruleFor(g.Name); ok && r.Radius > 0 {
		return r.Radius
	}
	return DefaultSocialRadius
}

// guardFreeLocked reports a town guard free to take a creature on (the
// file's doc). Must be called with the lock held.
func (w *World) guardFreeLocked(g *NPC, now time.Time) bool {
	return !g.Dead && g.Owner == 0 && g.Dummy == NotDummy && g.NPCType&0x80 == 0 && !g.SpawnOnly &&
		g.CurrentEnemy == 0 && !g.chase.returning && !g.hiddenAt(now) && w.townGuardLocked(g)
}

// guardsPreyLocked reports a creature guard g takes on when it comes near:
// a generic creature, none of the file's doc's exclusions. Must be called
// with the lock held.
func (w *World) guardsPreyLocked(g, c *NPC, now time.Time) bool {
	if c == g || c.Dead || c.Owner != 0 || c.Charm != nil || c.Dummy != NotDummy || c.NPCType&0x80 != 0 ||
		c.World != g.World || c.SpawnOnly || c.QuestSpawn || c.challenge != nil || c.hiddenAt(now) {
		return false
	}
	body := uint32(c.ModelID)
	if body == 0 || body == SignpostBody || body == BlockerBody || w.playerModels[body] {
		return false // people and things, not creatures
	}
	if r, ruled := w.aggro.ruleFor(c.Name); ruled && !r.Aggressive {
		return false // a GM's peaceful
	}
	if dist3(c.SpawnX, c.SpawnY, c.SpawnZ, g.SpawnX, g.SpawnY, g.SpawnZ) <= w.guardRadiusLocked(g) {
		return false // a resident of the guard's post, not one that wandered in
	}
	if w.inTownLocked(c.World, c.SpawnX, c.SpawnZ) {
		return false // a town's own, which lives there (the file's doc)
	}
	if w.townGuardLocked(c) {
		return false
	}
	own := w.npcFactionLocked(c)
	if guardFaction(own) || guardFaction(w.socialFactionLocked(c)) {
		return false
	}
	faction := w.socialFactionLocked(g)
	if w.ofFactionLocked(c, faction) {
		return false
	}
	town := GuardTown(faction)
	return town == "" || DialogTown(own) != town
}

// guardWatchSystem is the town guards' look round their posts: each free one
// takes on the nearest creature within its radius hitting a player within
// GuardHelpRadius of it (the file's doc). Every
// aggroScanTicks ticks, over the guards' set (npcsets.go: the NPCs whose
// social rule could make them a town's guard); must be called with the lock
// held.
func (w *World) guardWatchSystem(now time.Time) {
	if w.TickCount%aggroScanTicks != 0 || w.npcSocial.gen == 0 || len(w.playerModels) == 0 || w.SpatialIndex == nil {
		return
	}
	w.syncNPCSetsLocked()
	if w.sets.guardGen != w.npcSocial.gen {
		w.refillGuardsLocked() // the social rules changed
	}
	if npcSetsCheck {
		w.checkGuardSetLocked()
	}
	// a guard takes on what fights a player before it: with no player in the
	// world nothing does, and this loop was most of an empty server's turn
	if len(w.Players) == 0 {
		return
	}
	for id, g := range w.sets.guard.m {
		if g = w.setMember(w.sets.guard.m, id, g); g == nil {
			continue
		}
		if !w.guardFreeLocked(g, now) {
			continue
		}
		r := w.guardRadiusLocked(g)
		var prey *NPC
		var near float32
		for _, id := range w.SpatialIndex.QueryRadius(g.X, g.Z, r) {
			c, ok := w.NPCs[id]
			if !ok || !w.guardsPreyLocked(g, c, now) || !w.fightsPlayerNearLocked(c, g) {
				continue
			}
			if d := dist3(g.X, g.Y, g.Z, c.X, c.Y, c.Z); d <= r && (prey == nil || d < near) {
				prey, near = c, d
			}
		}
		if prey != nil {
			w.addThreatLocked(g, prey.ServerID, 1)
			log.Printf("Guards: %s takes on %s, %.1f from it", g.Name, prey.Name, near)
		}
	}
}

// guardLeashLocked is the leash of npc's fight now: GuardLeash for a town
// guard fighting a creature, else 0 (its own). Must be called with the lock
// held.
func (w *World) guardLeashLocked(npc *NPC) float32 {
	if w.townGuardLocked(npc) && w.npcFoeLocked(npc) != nil {
		return GuardLeash
	}
	return 0
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
// (rounds.go): its swing to everyone who sees it, then its hits. A scripted
// fight's two (scriptedFoes: a trigger's spawn set on foe with casts by
// SetNPCOn, and since petition 1437 the NPC it was set on fighting it back, and
// either's summoned adds) fight by their whole class templates: their casts
// held as with a player, their out-of-reach casts while closing in, their
// specials after their rounds and their melee procs on their hits
// (classtemplate.go, creatureproc.go); any other two NPCs (a town's guard and a
// creature) by their melee alone.
func (w *World) npcFightNPCLocked(npc, foe *NPC, now time.Time) {
	in, giveUp := w.chaseLocked(npc, foe.X, foe.Y, foe.Z, foe.VelX, foe.VelZ, now)
	if giveUp {
		w.giveUpLocked(npc)
		return
	}
	if npc.RoundTarget != foe.ServerID {
		npc.endRound()
		npc.RoundTarget = foe.ServerID
	}
	byTemplate := npc.tmpl != nil && scriptedFoes(npc, foe)
	if !in {
		npc.loseDueHits(now) // closing in: no hit lands from out of reach
		if byTemplate {
			w.templateFarTurnLocked(npc, foe.ServerID, now) // its casts from afar (classtemplate.go)
		}
		return
	}
	if byTemplate && w.templateTurnLocked(npc, foe.ServerID, now) {
		npc.loseDueHits(now) // casting: no swing meanwhile (classtemplate.go)
		return
	}
	defer func() {
		if byTemplate && !npc.Dead && !foe.Dead && npc.CurrentEnemy == foe.ServerID {
			w.templateSpecialLocked(npc, foe.ServerID, now) // its special (classtemplate.go)
		}
	}()
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
// against foe's (npcHitLocked), an add's as its creature's at its share
// (summon.go), as foe's template takes it; foe's hate for npc, a guard's mark
// on it (guardHit), an enraged one's rampage (enrage.go), foe's heal at a hit
// (hitheal.go), a scripted fight's melee procs (creatureproc.go), and its
// death. It reports a kill.
func (w *World) npcHitNPCLocked(npc, foe *NPC, now time.Time) bool {
	src := npc.figures()
	n := foe.meleeTaken(npc.shared(w.npcHitLocked(src, foe.Level, src.meleeOut(npcHitOnPet(src, foe, meleeIntn)))))
	foe.HP = max(foe.HP-n, 0)
	npc.LastCombat = now
	if w.townGuardLocked(npc) {
		foe.guardHit = true
	}
	w.addThreatLocked(foe, npc.ServerID, n)
	log.Printf("Combat: %s hit %s for %d (HP %d/%d)", npc.Name, foe.Name, n, foe.HP, foe.MaxHP)
	w.rampageLocked(npc, foe.ServerID, n, now)
	if foe.HP > 0 {
		w.hitHealLocked(foe, n, nil)
		return npc.tmpl != nil && scriptedFoes(npc, foe) && w.creatureProcOnNPCLocked(npc, foe, n, now)
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
