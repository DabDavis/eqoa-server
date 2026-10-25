package entity

import (
	"log"
	"math/rand"
	"time"

	"github.com/eqoa/server/internal/session"
)

// Swimming animation byte values from the client (C# AnimationState enum).
// The client sends these in position updates (byte 35) when the player is in water.
const (
	AnimSwimFast       = 0x0C
	AnimSwimSlow       = 0x0D
	AnimSwimRightPunch = 0x2C
)

// Tick runs one game tick: increments counter, then runs all systems.
// Called from the network tick goroutine. Acquires the world lock for the
// full duration so that handler goroutines are serialized against the tick;
// the players whose respawn came due are then handed to the respawn hook
// with the lock let go (respawn.go).
func (w *World) Tick(now time.Time) {
	due, gates, gone := w.tickLocked(now)
	w.runRespawns(due)
	w.runGates(gates)
	w.runCorpsesGone(gone)
}

// tickLocked is the tick's systems under the world lock. It returns the
// players whose respawn came due, for runRespawns, those a gate sends home,
// for runGates, and the creatures whose corpses decayed, for runCorpsesGone.
func (w *World) tickLocked(now time.Time) ([]respawnDue, []respawnDue, []EntityID) {
	w.mu.Lock()
	defer w.mu.Unlock()

	w.TickCount++

	w.aiSystem(now)
	w.homeSystem(now)
	w.attackSystem(now)
	w.petSystem(now)
	gates := w.castSystem(now)
	w.effectSystem(now)
	w.consumableSystem(now)
	gone := w.corpseSystem(now)
	w.roamSystem(now)
	w.pathSystem(now)
	due := w.playerRespawnSystem(now)
	w.regenSystem(now)
	w.breathSystem(now)
	w.stateSyncSystem()
	w.entityChannelSyncSystem()
	w.statChannelSyncSystem()
	w.buffChannelSyncSystem()
	w.guildAnnounceSystem()
	w.visibilitySystem()
	w.groupSyncSystem()
	return due, gates, gone
}

// aiSystem runs every aggro'd NPC's melee in rounds, as Sandstorm's
// (rounds.go): its swing at its current enemy when a round begins, then the
// round's hits. Out of its reach it runs at its enemy, and past its leash it
// gives up and goes home (chase.go); it drops an enemy that is gone or dead,
// and a new enemy starts a new round. Every tick; must be called with the
// lock held.
func (w *World) aiSystem(now time.Time) {
	type fight struct {
		npc   *NPC
		enemy *Player
	}
	var fights []fight
	var dummies []*NPC
	var petFights [][2]*NPC // a creature and the pet it fights (pet.go)

	for _, npc := range w.NPCs {
		if npc.Owner != 0 {
			continue // a pet fights in petSystem (pet.go)
		}
		if npc.Dummy != NotDummy {
			if npc.DummyTarget != 0 {
				dummies = append(dummies, npc) // its own target and hits (dummy.go)
			}
			continue
		}
		if npc.CurrentEnemy == 0 || npc.Dead {
			if !npc.RoundStart.IsZero() {
				npc.endRound()
			}
			if npc.chase.on && !npc.Dead {
				w.goHomeLocked(npc) // its fight is over: back where it began (chase.go)
			}
			continue
		}

		// Validate enemy is still a living player in world
		enemy, ok := w.Players[npc.CurrentEnemy]
		if !ok {
			if pet := w.petEnemyLocked(npc); pet != nil {
				petFights = append(petFights, [2]*NPC{npc, pet})
				continue
			}
			npc.Threats = nil
			npc.CurrentEnemy = 0
			npc.endRound()
			continue
		}

		// Drop aggro on a corpse. Without this the NPC keeps "killing" a
		// player who is already at 0 HP: every pass re-sent CastingCombat +
		// DamageIndication + CharacterDied, forever, all of them reliable.
		// Observed on the PS2 as ~40 reliable messages/second that the client
		// could not drain — it stopped acking and the game appeared to freeze.
		if enemy.IsDead() {
			npc.Threats = nil
			npc.CurrentEnemy = 0
			npc.endRound()
			continue
		}

		// Out of its reach it runs at its enemy; past its leash, or with
		// its enemy far off or in another world, it gives up and goes home
		// (chase.go). Until 2026-10-25 an enemy more than 10 off was dropped
		// at once and the NPC never moved (F3 20261025-165533).
		in, giveUp := w.chaseLocked(npc, enemy.X, enemy.Z, enemy.VelX, enemy.VelZ, now)
		if giveUp || enemy.World != npc.World {
			w.giveUpLocked(npc)
			continue
		}

		if npc.RoundTarget != enemy.ServerID {
			npc.endRound()
			npc.RoundTarget = enemy.ServerID
		}
		if !in {
			npc.loseDueHits(now) // closing in: no hit lands from out of reach
			continue
		}
		fights = append(fights, fight{npc: npc, enemy: enemy})
	}

	// The rounds after the checks: a player one NPC kills is off every
	// NPC's list (playerDiedLocked) before the next one swings.
	for _, f := range fights {
		npc, enemy := f.npc, f.enemy
		if enemy.IsDead() || npc.CurrentEnemy != enemy.ServerID {
			continue
		}
		w.npcRoundLocked(npc, enemy, now, func() bool {
			w.npcAttackPlayer(npc, enemy.ServerID, now)
			return enemy.IsDead() || npc.CurrentEnemy != enemy.ServerID
		})
	}
	for _, f := range petFights {
		if npc, pet := f[0], f[1]; !npc.Dead && !pet.Dead && npc.CurrentEnemy == pet.ServerID {
			w.npcFightPetLocked(npc, pet, now)
		}
	}
	for _, npc := range dummies {
		w.dummyFightLocked(npc, now)
	}
}

// npcAttackPlayer is one of an NPC's hits on a player (a round's swing is
// npcRoundLocked's): the damage, its 0x00DB to the player, and a death.
// Must be called with lock held.
func (w *World) npcAttackPlayer(npc *NPC, playerID EntityID, now time.Time) {
	player, ok := w.Players[playerID]
	if !ok {
		return
	}
	// Never attack a corpse — see the aggro guard in aiSystem. Repeated here
	// so any future caller inherits it rather than re-opening the loop.
	if player.IsDead() {
		return
	}

	// Sandstorm's roll on the player's AC, AGI and archetype (defence.go)
	damage := npcMeleeRoll(npc).hitOn(player, rand.Intn)
	player.HP -= damage
	if player.HP < 0 {
		player.HP = 0
	}
	player.LastCombatTime = now

	// Find player session to send feedback
	sess := w.findSessionByEntityIDLocked(playerID)
	if sess == nil {
		return
	}

	// The hit's figure on the player (source=NPC, target=player); the
	// round's swing (0x00B1) went out at its start, to everyone near
	npcObjectID := w.objectIDForEntityLocked(sess, npc.ServerID)
	w.sendDamageIndication(sess, npcObjectID, -damage, sess.PlayerObjectID())

	log.Printf("Combat: NPC '%s' hit player %s for %d damage (HP: %d/%d)",
		npc.Name, sess.RemoteAddr(), damage, player.HP, player.MaxHP)

	// Check for player death
	if player.HP <= 0 {
		w.playerDiedLocked(sess, player, now)
		log.Printf("Combat: player %s has died!", sess.RemoteAddr())
	}
}

// regenSystem gives every player its HoT in hit points and its PoT in power
// every RegenInterval (6 s), in a fight or out of one (regen.go: the live
// game's rule and Sandstorm's capture). Must be called with lock held.
func (w *World) regenSystem(now time.Time) {
	// a tick every RegenInterval (regen.go), at the world's 100 ms ticks
	if w.TickCount%uint64(RegenInterval/(100*time.Millisecond)) != 0 {
		return
	}

	for _, p := range w.Players {
		// A corpse does not heal: the respawn restores it (respawn.go)
		if p.respawnPending() {
			continue
		}

		// Skip regen while underwater (drowning should kill, not be out-healed)
		if p.Underwater {
			continue
		}

		// HoT and PoT, in a fight or out of one (regen.go)
		p.regenTick()
	}
}

// stateSyncSystem serializes player state into session state channel 0 buffers.
// Runs every tick to keep the client's view of the player state up-to-date.
// The XOR delta encoding in the RDP layer handles efficient network transmission.
// Must be called with lock held.
func (w *World) stateSyncSystem() {
	if w.SessionManager == nil {
		return
	}

	w.SessionManager.ForEach(func(sess *session.Session) bool {
		if !sess.CharacterInWorld() {
			return true
		}

		player := w.sessionPlayerLocked(sess)
		if player == nil {
			return true
		}

		objectID := sess.PlayerObjectID()
		if objectID == 0 {
			return true
		}

		stateData := SerializePlayerUpdate(objectID, player)

		conn := sess.Connection()
		if conn != nil {
			if state := conn.GetServerObject(0); state != nil {
				state.SetData(stateData)
			}
		}

		return true
	})
}

// entityChannelSyncSystem re-serializes all occupied entity channels every tick.
// This mirrors stateSyncSystem (which handles channel 0 for the player) but for NPCs
// and other players. Without this, entity channels are only written once during
// visibility assignment and never update — so position/HP/animation changes are never sent.
// Must be called with lock held.
func (w *World) entityChannelSyncSystem() {
	if w.SessionManager == nil {
		return
	}

	w.SessionManager.ForEach(func(sess *session.Session) bool {
		if !sess.CharacterInWorld() {
			return true
		}

		conn := sess.Connection()
		if conn == nil {
			return true
		}

		vis := GetOrCreateVisibility(sess)

		for i, id := range vis.ChannelEntity {
			if id == 0 {
				continue
			}
			objectID := vis.ChannelObjectID[i]

			stateData := w.serializeEntityForChannel(sess, id, objectID)
			if stateData != nil {
				if state := conn.GetServerObject(i + 1); state != nil {
					state.SetData(stateData)
				}
			}
		}

		return true
	})
}

// isSwimAnimation returns true if the animation byte indicates swimming.
func isSwimAnimation(anim byte) bool {
	return anim == AnimSwimFast || anim == AnimSwimSlow || anim == AnimSwimRightPunch
}

// breathSystem handles breath depletion underwater and drowning damage.
// Runs every 6 ticks (0.6s). Breath is 0-255, 255 full, the byte the client's
// breath gauge draws (the dump's and stat channel 0x42's byte 232): under
// liquid it loses BreathStep every 6 s, as Sandstorm's did in
// pcaps/breath_capture.pcap (in the water at 34.6 s, 0xDF at 35.9 s, 0 at
// 78.3 s); once it is gone, 2 HP every 6 s, with DamageIndication (0x00DB)
// and the drowning action (0x0775, action 34), as Sandstorm does.
// Underwater is the client's own IsUnderLiquid, out of the walker flags in
// its 0x40 (UnderLiquid): in a liquid and not at its surface, which is when
// the client draws the breath gauge. The swim animation alone said nothing
// about the surface, so a player swimming along the top drowned (F3
// 20261020-205003). A client that has sent no flags yet falls back to it.
// Must be called with lock held.
func (w *World) breathSystem(now time.Time) {
	if w.TickCount%6 != 0 {
		return
	}

	for _, p := range w.Players {
		if p.HP <= 0 || p.respawnPending() {
			continue
		}

		if p.HasWalkFlags {
			p.Underwater = UnderLiquid(p.WalkFlags)
		} else {
			p.Underwater = isSwimAnimation(p.Animation)
		}

		if p.Underwater {
			// Every 6 s under liquid: 32 of the 255 breath, then once it is
			// gone the HP (pcaps/breath_capture.pcap, Sandstorm: 0xFF to 0
			// in eight steps, 48 s of air). The breath reaches the client on
			// the stat channel (byte 232, statChannelSyncSystem).
			p.DrowningCounter += 6 // 6 ticks per breathSystem call
			if p.DrowningCounter >= 60 && p.Breath > 0 {
				p.DrowningCounter = 0
				p.Breath = max(0, p.Breath-BreathStep)
			} else if p.Breath <= 0 {
				// Out of breath: drowning damage every 6 s
				if p.DrowningCounter >= 60 {
					p.DrowningCounter = 0

					// Deal 2 HP drowning damage
					p.HP -= 2
					if p.HP < 0 {
						p.HP = 0
					}

					sess := w.findSessionByEntityIDLocked(p.ServerID)
					if sess == nil {
						if p.HP <= 0 {
							w.playerDiedLocked(nil, p, now)
						}
						continue
					}

					playerObjectID := sess.PlayerObjectID()
					w.sendDamageIndication(sess, playerObjectID, -2, playerObjectID)
					w.sendEntityAction(sess, playerObjectID, actionDrowning)

					log.Printf("Drowning: %s took 2 drowning damage (HP: %d/%d, breath: %d)",
						p.Name, p.HP, p.MaxHP, p.Breath)

					if p.HP <= 0 {
						w.playerDiedLocked(sess, p, now)
						log.Printf("Drowning: %s has drowned!", p.Name)
					}
				}
			}
		} else {
			// Above water: breath comes back, 4 every 0.6 s (full again in
			// about 38 s; the capture never surfaced, so this rate is ours)
			p.Breath = min(BreathFull, p.Breath+4)
			p.DrowningCounter = 0
		}
	}
}

// statChannelSyncSystem serializes stat data into session stat channel (0x42) buffers.
// Runs every 20 ticks (2 seconds) to keep HP/Power bars in sync with regen,
// and on the tick after a player's stats changed (StatsDirty: a cast's cost
// goes out with its start, as on Sandstorm). Must be called with lock held.
func (w *World) statChannelSyncSystem() {
	if w.SessionManager == nil {
		return
	}

	round := w.TickCount%20 == 0

	w.SessionManager.ForEach(func(sess *session.Session) bool {
		if !sess.CharacterInWorld() {
			return true
		}

		player := w.sessionPlayerLocked(sess)
		if player == nil || !(round || player.StatsDirty) {
			return true
		}

		statData := SerializeStatUpdate(player)

		conn := sess.Connection()
		if conn != nil {
			statChannel := conn.GetClientStatUpdate()
			if statChannel != nil {
				statChannel.SetData(statData)
			}
		}

		return true
	})
}

// buffChannelSyncSystem serializes each player's active effects into its
// session's effects channel (0x43), at the value's own length (SetSizedData).
// Runs every 20 ticks (2 seconds), matching stat channel sync rate, and on
// a StatsDirty tick, which it ends (it runs after statChannelSyncSystem); a
// value that has not changed sends nothing. Must be called with lock held.
func (w *World) buffChannelSyncSystem() {
	if w.SessionManager == nil {
		return
	}

	round := w.TickCount%20 == 0

	w.SessionManager.ForEach(func(sess *session.Session) bool {
		if !sess.CharacterInWorld() {
			return true
		}

		player := w.sessionPlayerLocked(sess)
		if player == nil || !(round || player.StatsDirty) {
			return true
		}
		player.StatsDirty = false

		buffData := SerializeBuffUpdate(player)

		conn := sess.Connection()
		if conn != nil {
			buffChannel := conn.GetClientBuffUpdate()
			if buffChannel != nil {
				buffChannel.SetSizedData(buffData)
			}
		}

		return true
	})
}
