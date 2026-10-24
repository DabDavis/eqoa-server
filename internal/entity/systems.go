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
	due, gates := w.tickLocked(now)
	w.runRespawns(due)
	w.runGates(gates)
}

// tickLocked is the tick's systems under the world lock. It returns the
// players whose respawn came due, for runRespawns, and those a gate sends
// home, for runGates.
func (w *World) tickLocked(now time.Time) ([]respawnDue, []respawnDue) {
	w.mu.Lock()
	defer w.mu.Unlock()

	w.TickCount++

	w.aiSystem(now)
	w.attackSystem(now)
	gates := w.castSystem(now)
	w.effectSystem(now)
	w.respawnSystem(now)
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
	return due, gates
}

// aiSystem runs every aggro'd NPC's melee in rounds, as Sandstorm's
// (rounds.go): its swing at its current enemy when a round begins, then the
// round's hits. An NPC drops an enemy that is gone, dead or out of its
// reach (10), and a new enemy starts a new round. Every tick; must be called
// with the lock held.
func (w *World) aiSystem(now time.Time) {
	type fight struct {
		npc   *NPC
		enemy *Player
	}
	var fights []fight
	var dummies []*NPC

	for _, npc := range w.NPCs {
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
			continue
		}

		// Validate enemy is still a living player in world
		enemy, ok := w.Players[npc.CurrentEnemy]
		if !ok {
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

		// Check melee range: distance <= 10 units
		dx := npc.X - enemy.X
		dz := npc.Z - enemy.Z
		distSq := dx*dx + dz*dz
		if distSq > 10*10 {
			// Out of range — clear aggro (leash)
			npc.Threats = nil
			npc.CurrentEnemy = 0
			npc.endRound()
			continue
		}

		if npc.RoundTarget != enemy.ServerID {
			npc.endRound()
			npc.RoundTarget = enemy.ServerID
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

// respawnSystem checks dead NPCs and respawns them after their timer expires.
// Must be called with lock held.
func (w *World) respawnSystem(now time.Time) {
	for _, npc := range w.NPCs {
		if !npc.Dead {
			continue
		}
		if npc.DeathTime.IsZero() {
			continue
		}
		if now.Sub(npc.DeathTime) < npc.RespawnTime {
			continue
		}
		// A GM's /spawn leaves once its corpse time is up (devspawn.go).
		if npc.Temporary {
			w.removeNPCLocked(npc)
			log.Printf("RespawnSystem: removed spawned NPC '%s' (entity ID=%d)", npc.Name, npc.ServerID)
			continue
		}

		// Respawn: reset position, HP, clear dead flag
		oldX, oldZ := npc.X, npc.Z
		npc.X = npc.SpawnX
		npc.Y = npc.SpawnY
		npc.Z = npc.SpawnZ
		npc.Facing = npc.SpawnFacing
		npc.HP = npc.MaxHP
		npc.Dead = false
		npc.DeathTime = time.Time{}
		npc.Threats = nil
		npc.CurrentEnemy = 0

		w.SpatialIndex.Update(npc.ServerID, oldX, oldZ, npc.SpawnX, npc.SpawnZ)

		log.Printf("RespawnSystem: respawned NPC '%s' (entity ID=%d)", npc.Name, npc.ServerID)
	}
}

// regenSystem handles HP and power regeneration for players out of combat.
// Runs every 20 ticks (~2 seconds at 100ms tick rate).
// Must be called with lock held.
func (w *World) regenSystem(now time.Time) {
	if w.TickCount%20 != 0 {
		return
	}

	for _, p := range w.Players {
		// A corpse does not heal: the respawn restores it (respawn.go)
		if p.respawnPending() {
			continue
		}

		// Skip regen if player was in combat within the last 10 seconds
		if !p.LastCombatTime.IsZero() && now.Sub(p.LastCombatTime) < 10*time.Second {
			continue
		}

		// Skip regen while underwater (drowning should kill, not be out-healed)
		if p.Underwater {
			continue
		}

		// HP regen: 1% of max HP per tick
		if top := p.MaxHPNow(); p.HP < top {
			regen := top / 100
			if regen < 1 {
				regen = 1
			}
			p.HP += regen
			if p.HP > top {
				p.HP = top
			}
		}

		// Power regen: 1% of max power per tick
		if top := p.MaxPowerNow(); p.Power < top {
			regen := top / 100
			if regen < 1 {
				regen = 1
			}
			p.Power += regen
			if p.Power > top {
				p.Power = top
			}
		}
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

			stateData := w.serializeEntityForChannel(id, objectID)
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
