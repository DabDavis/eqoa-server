package entity

import (
	"log"
	"sort"
	"time"

	"github.com/eqoa/server/internal/session"
)

const (
	// MaxVisibleEntities is the max number of NPC/player entities we can track:
	// channels 1-23. CLIENT keeps 24 entity slots, the object channels
	// 0x00-0x17 (VIClient::ProcessStateChannel sends only `type < 24` to
	// ChannelEntity), and channel 0 is the player's own.
	MaxVisibleEntities = 23
	// VisibilityRadius is the query radius for nearby entities.
	VisibilityRadius float32 = 300.0
	// VisibilityTickInterval controls how often (in ticks) the visibility system runs.
	VisibilityTickInterval uint64 = 5
	// MaxNewEntitiesPerTick limits how many new entities are assigned per visibility tick.
	MaxNewEntitiesPerTick = MaxVisibleEntities
	// NPCChannelOffset: NPC channels start at 1 (channel 0 = player self).
	NPCChannelOffset = 0
)

// VisibilityState tracks which entity occupies each state channel (1-23) for a session.
type VisibilityState struct {
	ChannelEntity   [MaxVisibleEntities]EntityID // Entity in each channel
	ChannelObjectID [MaxVisibleEntities]uint32   // ObjectID assigned to each channel's entity
	nextObjectID    uint32                       // Counter for assigning stable object IDs
}

// GetOrCreateVisibility retrieves or creates a VisibilityState from a session.
func GetOrCreateVisibility(sess *session.Session) *VisibilityState {
	v := sess.VisibilityState()
	if v != nil {
		if vs, ok := v.(*VisibilityState); ok {
			return vs
		}
	}
	vs := &VisibilityState{
		nextObjectID: 100000, // NPC objectIDs start at 100001+ (player IDs are 500000000+)
	}
	sess.SetVisibilityState(vs)
	return vs
}

// entityDist pairs an entity ID with its distance from the observer.
type entityDist struct {
	ID     EntityID
	DistSq float32
}

// visibilitySystem assigns nearby entities to state channels 1-23 for each player session.
func (w *World) visibilitySystem() {
	if w.SessionManager == nil {
		return
	}

	if w.TickCount%VisibilityTickInterval != 0 {
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

		conn := sess.Connection()
		if conn == nil {
			return true
		}

		vis := GetOrCreateVisibility(sess)

		// Query nearby entities from spatial index
		nearbyIDs := w.SpatialIndex.QueryRadius(player.X, player.Z, VisibilityRadius)

		// Filter out self, dead entities (unless recent corpse), and build distance list
		var candidates []entityDist
		for _, id := range nearbyIDs {
			if id == player.ServerID {
				continue
			}

			// Check NPC
			if npc, ok := w.NPCs[id]; ok {
				// The spatial index is keyed on (x, z) ONLY, so it cannot tell
				// two worlds apart. guk's island sits at (750..1700, 750..1250)
				// and Tunaria's NPCs at 13000..25000, so today the radius alone
				// happens to separate them — but that is a coincidence of where
				// the stub world was authored, not a rule, and the next world
				// placed over inhabited coordinates would have its NPCs show up
				// as ghosts in Tunaria. Filter on the world itself.
				if npc.World != player.World {
					continue
				}
				if !npc.HiddenUntil.IsZero() && time.Now().Before(npc.HiddenUntil) {
					continue
				}
				if npc.Dead {
					if npc.DeathTime.IsZero() || time.Since(npc.DeathTime) > 30*time.Second {
						continue
					}
				}
				dx := npc.X - player.X
				dz := npc.Z - player.Z
				candidates = append(candidates, entityDist{ID: id, DistSq: dx*dx + dz*dz})
				continue
			}

			// Check other players
			if other, ok := w.Players[id]; ok {
				if other.World != player.World {
					continue
				}
				dx := other.X - player.X
				dz := other.Z - player.Z
				candidates = append(candidates, entityDist{ID: id, DistSq: dx*dx + dz*dz})
			}
		}

		// Sort by distance (closest first)
		sort.Slice(candidates, func(i, j int) bool {
			return candidates[i].DistSq < candidates[j].DistSq
		})

		// Cap at MaxVisibleEntities
		if len(candidates) > MaxVisibleEntities {
			candidates = candidates[:MaxVisibleEntities]
		}

		// Build set of currently visible entity IDs for fast lookup
		visibleSet := make(map[EntityID]bool, len(candidates))
		for _, c := range candidates {
			visibleSet[c.ID] = true
		}

		// Phase 1: Identify channels to keep (entity still visible)
		usedChannels := make(map[EntityID]int) // entity ID -> channel index
		for i := 0; i < MaxVisibleEntities; i++ {
			id := vis.ChannelEntity[i]
			if id == 0 {
				continue
			}
			if visibleSet[id] {
				usedChannels[id] = i
			}
		}

		// Phase 2: Deactivate channels for entities no longer visible
		for i := 0; i < MaxVisibleEntities; i++ {
			id := vis.ChannelEntity[i]
			if id == 0 {
				continue
			}
			if _, kept := usedChannels[id]; !kept {
				// The entity's own id and type with the flag cleared, which
				// CLIENT's ChannelEntity reads as "delete it, make nothing";
				// 162 zero bytes would be type 0 (see MarkRemoved).
				if state := conn.GetServerObject(i + 1 + NPCChannelOffset); state != nil {
					state.MarkRemoved()
				}
				vis.ChannelEntity[i] = 0
				vis.ChannelObjectID[i] = 0
			}
		}

		// Phase 3: Assign new entities to free channels
		newCount := 0
		for _, c := range candidates {
			if newCount >= MaxNewEntitiesPerTick {
				break
			}
			if _, already := usedChannels[c.ID]; already {
				continue
			}
			// Find a free channel
			freeIdx := -1
			for i := 0; i < MaxVisibleEntities; i++ {
				if vis.ChannelEntity[i] == 0 {
					freeIdx = i
					break
				}
			}
			if freeIdx < 0 {
				break
			}
			// Assign a stable object ID
			vis.nextObjectID++
			objectID := vis.nextObjectID

			npcName := "???"
			if npc, ok := w.NPCs[c.ID]; ok {
				npcName = npc.Name
			} else if p, ok := w.Players[c.ID]; ok {
				npcName = p.Name
			}
			log.Printf("CHANNEL_MAP: NEW entity=%q objID=%d -> channel=%d (array idx=%d) distSq=%.0f",
				npcName, objectID, freeIdx+1+NPCChannelOffset, freeIdx, c.DistSq)

			vis.ChannelEntity[freeIdx] = c.ID
			vis.ChannelObjectID[freeIdx] = objectID
			usedChannels[c.ID] = freeIdx
			newCount++

			// Send FullEntityCreate (0x00B3) reliable opcode for PS2 clients
			if w.OnEntityEnter != nil {
				var ent Entity
				if npc, ok := w.NPCs[c.ID]; ok {
					ent = npc
				} else if p, ok := w.Players[c.ID]; ok {
					ent = p
				}
				if ent != nil {
					w.OnEntityEnter(sess, objectID, ent)
				}
			}
		}

		// Phase 4: Serialize state for all occupied channels
		var occupiedChannels []int
		for i := 0; i < MaxVisibleEntities; i++ {
			id := vis.ChannelEntity[i]
			if id == 0 {
				continue
			}
			objectID := vis.ChannelObjectID[i]

			stateData := w.serializeEntityForChannel(id, objectID)
			if stateData != nil {
				ch := i + 1 + NPCChannelOffset
				state := conn.GetServerObject(ch)
				if state != nil {
					state.SetData(stateData)
				}
				occupiedChannels = append(occupiedChannels, ch)
			}
		}

		log.Printf("CHANNEL_MAP: SUMMARY candidates=%d occupied=%d/%d new=%d channels=%v",
			len(candidates), len(occupiedChannels), MaxVisibleEntities, newCount, occupiedChannels)

		return true
	})
}

// serializeEntityForChannel builds a 162-byte state buffer for an entity.
func (w *World) serializeEntityForChannel(id EntityID, objectID uint32) []byte {
	// Check NPC first
	if npc, ok := w.NPCs[id]; ok {
		buf := SerializeNPCUpdate(objectID, npc)
		if npc.Dead {
			buf[58] = 0x0e // AnimationState.Die
		}
		return buf
	}

	// Check player (other players on visibility channels)
	if p, ok := w.Players[id]; ok {
		// NOTHING IS OVERRIDDEN HERE. SerializePlayerUpdate already writes the
		// player's NPCType — 145 = class, 146 = 0x00 — and that is what retail
		// sends for every player in pcaps/ps2_sandstorm.pcap.
		//
		// Two writes used to follow, labelled "NPCType 0x0080 (player bit
		// set)", and both were the bugs statesync.go's own comments record as
		// fixed:
		//
		//   PutUint16(buf[146:148], 0x0080)  set byte 146 to 0x80, which is the
		//     NPC bit — the exact mistake that had the client running the NPC
		//     appearance path for players and the player path for NPCs.
		//   PutUint16(buf[185:187], 0x0080)  wrote into the "C# tail" no real
		//     server sends, at offsets that are live fields in the client's own
		//     entity struct. Once the buffer shrank from 201 to its true wire
		//     size of 162 this was simply out of range, and it PANICKED the
		//     tick goroutine the first time a second player came into view:
		//     slice bounds [:187] with capacity 162.
		//
		// A player is distinguished from an NPC by the high half being ZERO,
		// which is what the serializer already does, so there is nothing for
		// the visibility path to add.
		return SerializePlayerUpdate(objectID, p)
	}

	return nil
}
