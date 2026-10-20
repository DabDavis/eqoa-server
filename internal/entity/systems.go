package entity

import (
	"log"
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
// full duration so that handler goroutines are serialized against the tick.
func (w *World) Tick(now time.Time) {
	w.mu.Lock()
	defer w.mu.Unlock()

	w.TickCount++

	w.aiSystem(now)
	w.attackSystem(now)
	w.respawnSystem(now)
	w.regenSystem(now)
	w.breathSystem()
	w.stateSyncSystem()
	w.entityChannelSyncSystem()
	w.statChannelSyncSystem()
	w.buffChannelSyncSystem()
	w.guildAnnounceSystem()
	w.visibilitySystem()
	w.groupSyncSystem()
}

// aiSystem handles NPC combat: aggro'd NPCs attack their current enemy every ~3 seconds.
// Runs every 30 ticks (~3 seconds at 100ms tick rate), matching C# 3000ms attack timer.
// Must be called with lock held.
func (w *World) aiSystem(now time.Time) {
	if w.TickCount%30 != 0 {
		return
	}

	type npcAttack struct {
		npc     *NPC
		enemyID EntityID
	}
	var attacks []npcAttack

	for _, npc := range w.NPCs {
		if npc.Dead {
			continue
		}
		if npc.CurrentEnemy == 0 {
			continue
		}

		// Validate enemy is still a living player in world
		enemy, ok := w.Players[npc.CurrentEnemy]
		if !ok {
			npc.Threats = nil
			npc.CurrentEnemy = 0
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
			continue
		}

		attacks = append(attacks, npcAttack{npc: npc, enemyID: npc.CurrentEnemy})
	}

	// Process attacks
	for _, atk := range attacks {
		w.npcAttackPlayer(atk.npc, atk.enemyID, now)
	}
}

// npcAttackPlayer applies NPC damage to a player.
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

	// SOE retail damage formula: NPC attacks player
	weaponDmg := RollWeaponDamage(npc.WeaponDamage)
	damage := CalculateMeleeDamage(npc.OffMod, weaponDmg, npc.Level, player.Level, player.DefMod)
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

	// Send combat action + damage pop-up on the player (source=NPC, target=player)
	npcObjectID := w.objectIDForEntityLocked(sess, npc.ServerID)
	playerObjectID := sess.PlayerObjectID()
	// Sandstorm's NPC swing, live 2026-10-16: 0x00B1 with the swing's delay
	// and VISwing::Effect's id — 3000 and Attack1 (0xDA9BEA11) on all six of
	// a Crab's, the kind-0 swing, which is also this system's 3 s cadence.
	// Effect 0 named no effect at all, so a client had no swing to play.
	w.sendCastingCombat(sess, npcObjectID, playerObjectID, uint32(SwingDelay(0)/time.Millisecond), HashResourceID(SwingEffectName(0)))
	w.sendDamageIndication(sess, npcObjectID, -damage, playerObjectID)

	log.Printf("Combat: NPC '%s' hit player %s for %d damage (HP: %d/%d)",
		npc.Name, sess.RemoteAddr(), damage, player.HP, player.MaxHP)

	// Check for player death
	if player.HP <= 0 {
		w.sendPlayerDied(sess)
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
		// Skip regen if player was in combat within the last 10 seconds
		if !p.LastCombatTime.IsZero() && now.Sub(p.LastCombatTime) < 10*time.Second {
			continue
		}

		// Skip regen while underwater (drowning should kill, not be out-healed)
		if p.Underwater {
			continue
		}

		// HP regen: 1% of max HP per tick
		if p.HP < p.MaxHP {
			regen := p.MaxHP / 100
			if regen < 1 {
				regen = 1
			}
			p.HP += regen
			if p.HP > p.MaxHP {
				p.HP = p.MaxHP
			}
		}

		// Power regen: 1% of max power per tick
		if p.Power < p.MaxPower {
			regen := p.MaxPower / 100
			if regen < 1 {
				regen = 1
			}
			p.Power += regen
			if p.Power > p.MaxPower {
				p.Power = p.MaxPower
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

// entityChannelSyncSystem re-serializes all occupied entity channels (1-28) every tick.
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

		for i := 0; i < MaxVisibleEntities; i++ {
			id := vis.ChannelEntity[i]
			if id == 0 {
				continue
			}
			objectID := vis.ChannelObjectID[i]

			stateData := w.serializeEntityForChannel(id, objectID)
			if stateData != nil {
				ch := i + 1 + NPCChannelOffset
				if state := conn.GetServerObject(ch); state != nil {
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
// Runs every 6 ticks (0.6s). Breath depletes from 127→0 over ~76.2s (close to
// observed 78.6s on eqoa.live). When breath hits 0, deals 2 HP self-damage
// every 60 ticks (6s). The client manages the breath bar UI independently —
// the server only sends drowning damage via DamageIndication (0x00DB) and the
// drowning action (0x0775, action 34), as Sandstorm does.
// Underwater detection uses the client's animation byte (swim animations)
// rather than a global Y threshold, so it works correctly per-zone.
// Must be called with lock held.
func (w *World) breathSystem() {
	if w.TickCount%6 != 0 {
		return
	}

	for _, p := range w.Players {
		if p.HP <= 0 {
			continue
		}

		p.Underwater = isSwimAnimation(p.Animation)

		if p.Underwater {
			if p.Breath > 0 {
				// The breath bar is the client's own; nothing is sent.
				p.Breath--
			} else {
				// Out of breath — accumulate drowning counter
				p.DrowningCounter += 6 // 6 ticks per breathSystem call
				if p.DrowningCounter >= 60 {
					p.DrowningCounter = 0

					// Deal 2 HP drowning damage
					p.HP -= 2
					if p.HP < 0 {
						p.HP = 0
					}

					sess := w.findSessionByEntityIDLocked(p.ServerID)
					if sess == nil {
						continue
					}

					playerObjectID := sess.PlayerObjectID()
					w.sendDamageIndication(sess, playerObjectID, -2, playerObjectID)
					w.sendEntityAction(sess, playerObjectID, actionDrowning)

					log.Printf("Drowning: %s took 2 drowning damage (HP: %d/%d, breath: %d)",
						p.Name, p.HP, p.MaxHP, p.Breath)

					if p.HP <= 0 {
						w.sendPlayerDied(sess)
						log.Printf("Drowning: %s has drowned!", p.Name)
					}
				}
			}
		} else {
			// Above water — regenerate breath
			if p.Breath < 127 {
				p.Breath += 2
				if p.Breath > 127 {
					p.Breath = 127
				}
			}
			p.DrowningCounter = 0
		}
	}
}

// statChannelSyncSystem serializes stat data into session stat channel (0x42) buffers.
// Runs every 20 ticks (2 seconds) to keep HP/Power bars in sync with regen.
// Must be called with lock held.
func (w *World) statChannelSyncSystem() {
	if w.SessionManager == nil {
		return
	}

	if w.TickCount%20 != 0 {
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
// Runs every 20 ticks (2 seconds), matching stat channel sync rate; a value
// that has not changed sends nothing. Must be called with lock held.
func (w *World) buffChannelSyncSystem() {
	if w.SessionManager == nil {
		return
	}

	if w.TickCount%20 != 0 {
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
