package entity

import (
	"fmt"
	"log"
	"math"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/eqoa/pkg/pkg/buffer"
	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// EntityEnterCallback is called when a new entity enters a player's visibility.
// sess = the observing player's session, objectID = the assigned object ID,
// ent = the entity that just became visible.
type EntityEnterCallback func(sess *session.Session, objectID uint32, ent Entity)

// World is the game world state. All mutations go through methods that
// acquire the mutex internally.
type World struct {
	mu             sync.RWMutex
	Players        map[EntityID]*Player
	NPCs           map[EntityID]*NPC
	SpatialIndex   *QuadTree
	SessionManager *session.Manager
	TickCount      uint64
	nextEntityID   EntityID

	// OnEntityEnter is called when a new entity enters a player's visibility range.
	// Set by the handler layer to send FullEntityCreate (0x00B3) reliable opcodes.
	OnEntityEnter EntityEnterCallback
}

// NewWorld creates and initializes a new game world.
func NewWorld() *World {
	w := &World{
		Players:      make(map[EntityID]*Player),
		NPCs:         make(map[EntityID]*NPC),
		SpatialIndex: NewQuadTree(Bounds{X: 0, Y: 0, Width: 50000, Height: 50000}),
		nextEntityID: 1000, // Reserve IDs below 1000
	}
	log.Println("Entity World initialized")
	return w
}

// allocID returns a new unique entity ID. Must be called with lock held.
func (w *World) allocID() EntityID {
	w.nextEntityID++
	return w.nextEntityID
}

// CreatePlayer creates a new player entity from a database record and links it to the session.
func (w *World) CreatePlayer(sess *session.Session, record *database.CharacterRecord) *Player {
	// Character-based dedup: if the same character is already in-world from an old session,
	// flag that session for termination. This handles PS2 reconnects without killing
	// different characters from the same IP (e.g. headless client).
	if w.SessionManager != nil && sess != nil {
		w.SessionManager.RemoveSessionForCharacter(uint32(record.ServerID), sess.InstanceID())
	}

	// Tag this session with the character's serverID for future dedup lookups
	if sess != nil {
		sess.SetCharacterServerID(uint32(record.ServerID))
	}

	w.mu.Lock()
	defer w.mu.Unlock()

	if record == nil {
		log.Println("Entity CreatePlayer: nil record")
		return nil
	}

	id := EntityID(record.ServerID)
	// Remove existing player with same serverID (ghost from old session)
	if existing, exists := w.Players[id]; exists {
		w.SpatialIndex.Remove(id, existing.X, existing.Z)
		delete(w.Players, id)
		log.Printf("Entity CreatePlayer: removed ghost player '%s' (serverID=%d)", existing.Name, id)
	}
	if _, exists := w.NPCs[id]; exists {
		id = w.allocID()
	}

	// Look up race/class base stats (C# DefaultCharacter table)
	bs := LookupBaseStats(record.Race, record.TClass)
	str := bs.Str + record.TPStrength
	sta := bs.Sta + record.TPStamina
	agi := bs.Agi + record.TPAgility
	dex := bs.Dex + record.TPDexterity
	wis := bs.Wis + record.TPWisdom
	intel := bs.Int + record.TPIntel
	cha := bs.Cha + record.TPCharisma

	level := record.Level
	if level < 1 {
		level = 1
	}

	maxHP := CalculateMaxHP(level, record.TClass, sta)
	maxPower := CalculateMaxPower(level, record.TClass, str, sta, agi, dex, wis, intel, cha)

	// Use DB position if it's been saved (non-zero), otherwise use default spawn
	posX := float32(25285.0)
	posY := float32(54.12)
	posZ := float32(15720.0)
	posFacing := float32(3.15526)
	if record.X != 0 || record.Z != 0 {
		posX = record.X
		posY = record.Y
		posZ = record.Z
		posFacing = record.Facing

		// Spawn a little ABOVE the stored height rather than exactly on it, and
		// let the client's floor snap settle the last few units. A stored
		// height that is even slightly low puts the camera under the terrain
		// and the player falls; arriving high costs an invisible drop. See
		// SpawnLift.
		//
		// A character already at the world floor is not lifted: they are not
		// a rounding error away from the ground, they are in the void, and
		// three units does not reach it. Their row needs fixing, and pretending
		// otherwise would just hide it.
		if posY > WorldFloorY {
			posY += SpawnLift
		}
	}

	p := &Player{
		ServerID:     id,
		Name:         record.CharName,
		X:            posX,
		Y:            posY,
		Z:            posZ,
		Facing:       posFacing,
		World:        coherentWorldOnLoad(record.CharName, uint8(record.World), posX, posZ),
		ModelID:      record.ModelID,
		Size:         RaceScale(record.Race),
		Race:         record.Race,
		Sex:          record.Sex,
		HumType:      record.HumType,
		HairColor:    record.HairColor,
		HairLength:   record.HairLength,
		HairStyle:    record.HairStyle,
		FaceOption:   record.FaceOption,
		AccountID:    record.AccountID,
		CharServerID: record.ServerID,
		Class:        record.TClass,
		HP:           maxHP,
		MaxHP:        maxHP,
		Power:        maxPower,
		MaxPower:     maxPower,
		Level:        level,
		Speed:        20.0,
		Faction:      FactionFriendly,
		Str:          str,
		Sta:          sta,
		Agi:          agi,
		Dex:          dex,
		Wis:          wis,
		Int:          intel,
		Cha:          cha,
		UnusedTP:     record.UnusedTP,
		TotalTP:      record.TotalTP,
		BaseMaxStat:  475,
		Tunar:        5000,
		AC:           50,
		Breath:       127,

		// Base stats (before equipment) for combat formulas
		BaseStr: str, BaseSta: sta, BaseAgi: agi, BaseDex: dex,
		BaseWis: wis, BaseInt: intel, BaseCha: cha,

		// SOE combat modifiers — computed from base stats
		OffMod:       CalculateOffMod(str, dex, level),
		DefMod:       CalculateDefMod(50, agi, level), // AC=50 default
		WeaponDamage: level / 3,                       // placeholder until item system
	}

	w.Players[id] = p
	w.SpatialIndex.Insert(id, p.X, p.Z)

	if sess != nil {
		sess.SetECSEntity(p)
	}

	log.Printf("Entity: created player entity for '%s' (serverID=%d)", record.CharName, record.ServerID)
	return p
}

// RemovePlayer removes a player entity from the world.
func (w *World) RemovePlayer(id EntityID) {
	w.mu.Lock()
	defer w.mu.Unlock()

	p, ok := w.Players[id]
	if !ok {
		return
	}
	w.SpatialIndex.Remove(id, p.X, p.Z)
	delete(w.Players, id)
	log.Printf("Entity: removed player ID=%d", id)
}

// RemovePlayerBySession removes the player entity associated with a session.
func (w *World) RemovePlayerBySession(sess *session.Session) {
	if sess == nil {
		return
	}
	p := w.SessionPlayer(sess)
	if p == nil {
		return
	}
	w.RemovePlayer(p.ServerID)
}

// UpdatePosition updates an entity's position in the world.
func (w *World) UpdatePosition(id EntityID, x, y, z, facing float32) {
	w.mu.Lock()
	defer w.mu.Unlock()

	if p, ok := w.Players[id]; ok {
		oldX, oldZ := p.X, p.Z
		// Compute velocity from position delta (units per tick)
		p.VelX = x - p.X
		p.VelY = y - p.Y
		p.VelZ = z - p.Z
		p.X = x
		p.Y = y
		p.Z = z
		if facing != 0 {
			p.Facing = facing
		}
		w.SpatialIndex.Update(id, oldX, oldZ, x, z)
		return
	}

	if n, ok := w.NPCs[id]; ok {
		oldX, oldZ := n.X, n.Z
		n.X = x
		n.Y = y
		n.Z = z
		if facing != 0 {
			n.Facing = facing
		}
		w.SpatialIndex.Update(id, oldX, oldZ, x, z)
	}
}

// SetAnimation updates a player's animation byte (from client position update).
func (w *World) SetAnimation(id EntityID, animation byte) {
	w.mu.Lock()
	defer w.mu.Unlock()

	if p, ok := w.Players[id]; ok {
		p.Animation = animation
	}
}

// SetTarget sets a player's target object ID.
func (w *World) SetTarget(id EntityID, targetObjectID uint32) {
	w.mu.Lock()
	defer w.mu.Unlock()

	if p, ok := w.Players[id]; ok {
		p.TargetObjectID = targetObjectID
	}
}

// ProcessAttack handles an attack from a player session against a target.
func (w *World) ProcessAttack(sess *session.Session, targetObjectID uint32, damage int) {
	w.mu.Lock()
	defer w.mu.Unlock()

	if sess == nil {
		return
	}

	attacker := w.sessionPlayerLocked(sess)
	if attacker == nil {
		return
	}

	targetEntity, found := w.findEntityByObjectIDLocked(sess, targetObjectID)
	if !found {
		log.Printf("Combat: target objectID %d not found", targetObjectID)
		return
	}

	// Must be an NPC
	npc, ok := targetEntity.(*NPC)
	if !ok {
		log.Printf("Combat: target objectID %d is not an NPC", targetObjectID)
		return
	}

	// Must be in melee range
	if !InRange(attacker, npc, MeleeRange) {
		return
	}

	// Must not be unattackable (NPCType flag 0x80)
	if npc.NPCType&0x80 != 0 {
		log.Printf("Combat: target objectID %d is unattackable (type=0x%X)", targetObjectID, npc.NPCType)
		return
	}

	// Must not be dead
	if npc.Dead {
		log.Printf("Combat: target objectID %d is already dead", targetObjectID)
		return
	}

	// Calculate damage using SOE retail formula
	weaponDmg := RollWeaponDamage(attacker.WeaponDamage)
	damage = CalculateMeleeDamage(attacker.OffMod, weaponDmg, attacker.Level, npc.Level, npc.DefMod)
	npc.HP -= damage
	if npc.HP < 0 {
		npc.HP = 0
	}

	killed := npc.HP <= 0

	// Set combat timestamp on attacker
	attacker.LastCombatTime = time.Now()

	targetName := npc.Name

	// Send combat action (triggers attack animation on client)
	playerObjectID := sess.PlayerObjectID()
	w.sendCastingCombat(sess, playerObjectID, targetObjectID, 4000, 0) // 4000ms = auto-attack

	// Send visual damage pop-up on the NPC (source=player, target=NPC, negative=damage)
	w.sendDamageIndication(sess, playerObjectID, -damage, targetObjectID)

	log.Printf("Combat: %s hit %s for %d damage (HP: %d/%d)",
		sess.RemoteAddr(), targetName, damage, npc.HP, npc.MaxHP)

	// Update aggro on the NPC
	w.updateAggro(npc, attacker.ServerID, damage)

	// Check for kill
	if killed {
		w.killNPC(npc)

		// Award XP
		xpAward := npc.Level * 100
		w.awardXP(sess, attacker, xpAward)
	}
}

// LoadNPCs loads all NPCs from the database into the world.
func (w *World) LoadNPCs(npcStore *database.NPCStore) {
	w.mu.Lock()
	defer w.mu.Unlock()

	npcs, err := npcStore.GetAllNPCs()
	if err != nil {
		log.Printf("Entity LoadNPCs: failed to load NPCs: %v", err)
		return
	}

	if len(npcs) == 0 {
		log.Println("Entity LoadNPCs: no NPCs found in database")
		return
	}

	count := 0
	for _, rec := range npcs {
		posX := float32(rec.XCoord)
		posY := float32(rec.YCoord)
		posZ := float32(rec.ZCoord)
		facing := float32(rec.Facing) / 256.0 * 2.0 * math.Pi

		id := EntityID(rec.ID)

		// Faction from NPCType bits:
		//   0x80 = non-hostile (interactable NPC)
		//   0x01 = merchant/vendor
		//   0x80 + 0x01 = friendly merchant → FactionFriendly
		//   0x80 only   = neutral NPC (coachman, patron) → FactionNeutral
		//   0x00         = town NPC without flags (guards, named) → FactionNeutral
		//   Other without 0x80 = hostile mob → FactionHostile
		faction := FactionNeutral
		if rec.NPCType&0x80 != 0 {
			if rec.NPCType&0x01 != 0 {
				faction = FactionFriendly // merchants, vendors, bankers
			} else {
				faction = FactionNeutral
			}
		} else if rec.NPCType != 0 {
			// Non-zero NPCType without 0x80 bit = likely hostile
			faction = FactionHostile
		}
		// NPCType == 0: defaults to FactionNeutral (town guards, named NPCs)

		npc := &NPC{
			ServerID:        id,
			Name:            rec.NPCName,
			X:               posX,
			Y:               posY,
			Z:               posZ,
			Facing:          facing,
			World:           uint8(rec.World),
			ModelID:         int64(rec.ModelID),
			Size:            rec.Size * raceScaleFromString(rec.Race),
			NPCID:           rec.NPCID,
			NPCType:         rec.NPCType,
			PrimaryWeapon:   rec.PrimaryWeapon,
			SecondaryWeapon: rec.SecondaryWeapon,
			Shield:          rec.Shield,
			Torso:           rec.Torso,
			Forearm:         rec.Forearm,
			NPCGloves:       rec.Gloves,
			NPCLegs:         rec.Legs,
			Feet:            rec.Feet,
			Head:            rec.Head,
			HairColor:       rec.HairColor,
			HairLength:      rec.HairLength,
			HairStyle:       rec.HairStyle,
			Face:            rec.Face,
			RobeStyle:       rec.RobeStyle,
			Race:            rec.Race,
			HP:              NPCMaxHP(rec.NPCLevel),
			MaxHP:           NPCMaxHP(rec.NPCLevel),
			Level:           rec.NPCLevel,
			Speed:           8.0,
			Faction:         faction,
			SpawnX:          posX,
			SpawnY:          posY,
			SpawnZ:          posZ,
			SpawnFacing:     facing,
			RespawnTime:     60 * time.Second,
			AggroRadius:     30.0,
			LeashRadius:     100.0,
			RoamRadius:      50.0,

			// SOE combat stats — estimated from level
			OffMod:       NPCOffMod(rec.NPCLevel),
			DefMod:       NPCDefMod(rec.NPCLevel),
			WeaponDamage: NPCWeaponDamage(rec.NPCLevel),
		}

		w.NPCs[id] = npc
		w.SpatialIndex.Insert(id, posX, posZ)

		count++
		log.Printf("Entity LoadNPCs: '%s' at (%.1f, %.1f, %.1f)", rec.NPCName, posX, posY, posZ)
	}

	log.Printf("Entity LoadNPCs: loaded %d NPCs into world", count)
}

// FindEntityByObjectID resolves a runtime objectID to an Entity.
func (w *World) FindEntityByObjectID(sess *session.Session, objectID uint32) (Entity, bool) {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.findEntityByObjectIDLocked(sess, objectID)
}

// findEntityByObjectIDLocked resolves a runtime objectID. Must be called with lock held.
func (w *World) findEntityByObjectIDLocked(sess *session.Session, objectID uint32) (Entity, bool) {
	var found Entity
	entityFound := false

	w.SessionManager.ForEach(func(s *session.Session) bool {
		// Check if this session's player has the objectID
		if s.PlayerObjectID() == objectID && s.CharacterInWorld() {
			p := w.sessionPlayerLocked(s)
			if p != nil {
				found = p
				entityFound = true
				return false
			}
		}

		// Check this session's NPC visibility channels
		vis := s.VisibilityState()
		if vis == nil {
			return true
		}
		vs, ok := vis.(*VisibilityState)
		if !ok {
			return true
		}
		for i := 0; i < MaxVisibleEntities; i++ {
			if vs.ChannelObjectID[i] == objectID {
				id := vs.ChannelEntity[i]
				if npc, ok := w.NPCs[id]; ok {
					found = npc
					entityFound = true
					return false
				}
				if p, ok := w.Players[id]; ok {
					found = p
					entityFound = true
					return false
				}
			}
		}
		return true
	})

	return found, entityFound
}

// SessionPlayer extracts the *Player from a session's ECS entity field.
func (w *World) SessionPlayer(sess *session.Session) *Player {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.sessionPlayerLocked(sess)
}

// sessionPlayerLocked extracts the *Player from a session. Must be called with lock held.
func (w *World) sessionPlayerLocked(sess *session.Session) *Player {
	if sess == nil {
		return nil
	}
	val := sess.ECSEntity()
	if val == nil {
		return nil
	}
	if p, ok := val.(*Player); ok {
		return p
	}
	return nil
}

// ForEachPlayer iterates all players calling fn for each. Holds read lock.
func (w *World) ForEachPlayer(fn func(p *Player)) {
	w.mu.RLock()
	defer w.mu.RUnlock()
	for _, p := range w.Players {
		fn(p)
	}
}

// PlayerCount returns the number of player entities.
func (w *World) PlayerCount() int {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return len(w.Players)
}

// NPCCount returns the number of NPC entities.
func (w *World) NPCCount() int {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return len(w.NPCs)
}

// FindSessionByEntityID finds the session for a given entity ID.
func (w *World) FindSessionByEntityID(id EntityID) *session.Session {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.findSessionByEntityIDLocked(id)
}

// findSessionByEntityIDLocked finds the session for a given entity ID.
// Must be called with lock held.
func (w *World) findSessionByEntityIDLocked(id EntityID) *session.Session {
	var found *session.Session
	w.SessionManager.ForEach(func(s *session.Session) bool {
		p := w.sessionPlayerLocked(s)
		if p != nil && p.ServerID == id && s.CharacterInWorld() {
			found = s
			return false
		}
		return true
	})
	return found
}

// --- Combat helpers (must be called with lock held) ---

// killNPC marks an NPC as dead.
func (w *World) killNPC(npc *NPC) {
	npc.Dead = true
	npc.DeathTime = time.Now()
	npc.Threats = nil
	npc.CurrentEnemy = 0

	log.Printf("Combat: NPC '%s' killed (entity ID=%d)", npc.Name, npc.ServerID)
}

// updateAggro adds or updates threat on an NPC.
func (w *World) updateAggro(npc *NPC, attackerID EntityID, damage int) {
	if npc.Threats == nil {
		npc.Threats = make(map[EntityID]int64)
	}
	npc.Threats[attackerID] += int64(damage)
	npc.LastCombat = time.Now()

	// Update current enemy to highest threat
	var maxThreat int64
	for id, t := range npc.Threats {
		if t > maxThreat {
			maxThreat = t
			npc.CurrentEnemy = id
		}
	}
}

// awardXP grants XP to a player, handles level-ups, and sends GrantXP.
func (w *World) awardXP(sess *session.Session, player *Player, xp int) {
	player.TotalXP += xp
	player.XPInLevel += xp

	// Process level-ups
	if player.Level < 60 {
		for {
			threshold, ok := CharXPDict[player.Level]
			if !ok || threshold <= 0 {
				break
			}
			if player.XPInLevel < threshold {
				break
			}
			player.XPInLevel -= threshold
			player.Level++
			log.Printf("Combat: player %s leveled up to %d!", sess.RemoteAddr(), player.Level)
			w.sendChatToSession(sess, fmt.Sprintf("You have reached level %d!", player.Level))
			if player.Level >= 60 {
				break
			}
		}
	}

	w.sendGrantXP(sess, player.Level, player.XPInLevel)
	w.sendChatToSession(sess, fmt.Sprintf("You received %d XP.", xp))
	log.Printf("Combat: awarded %d XP to %s (level %d, xpInLevel: %d/%d)", xp, sess.RemoteAddr(), player.Level, player.XPInLevel, CharXPDict[player.Level])
}

// sendGrantXP sends OpcodeGrantXP (0x0020).
func (w *World) sendGrantXP(sess *session.Session, level, xpInLevel int) {
	msg := protocol.NewMessage(protocol.MessageTypeReliable, protocol.OpcodeGrantXP)
	writer := buffer.NewWriter(msg.Span())

	writer.WriteUint16(uint16(protocol.OpcodeGrantXP))
	writer.Write7BitEncodedInt64(int64(level))
	writer.Write7BitEncodedInt64(int64(xpInLevel))

	msg.SetSize(writer.Position())
	sess.Queue().AddWithSequence(msg)
}

// sendDamageIndication sends a visual damage/heal pop-up (0x00DB).
// Wire format: opcode(2) + uint32(sourceObjectID) + 7BitEncodedInt64(amount) + uint32(targetObjectID)
// Negative amount = damage, positive = healing.
func (w *World) sendDamageIndication(sess *session.Session, sourceObjectID uint32, amount int, targetObjectID uint32) {
	msg := protocol.NewMessage(protocol.MessageTypeReliable, protocol.OpcodeDamageIndication)
	writer := buffer.NewWriter(msg.Span())

	writer.WriteUint16(uint16(protocol.OpcodeDamageIndication))
	writer.WriteUint32(sourceObjectID)
	writer.Write7BitEncodedInt64(int64(amount))
	writer.WriteUint32(targetObjectID)

	msg.SetSize(writer.Position())
	sess.Queue().AddWithSequence(msg)
}

// sendResourceUpdate sends opcode 0x0775 (paired with DamageIndication on eqoa.live).
// Wire format: opcode(2) + uint32(objectID) + zigzag(amount)
// Negative amount depletes the resource (like damage depletes HP).
func (w *World) sendResourceUpdate(sess *session.Session, objectID uint32, amount int) {
	msg := protocol.NewMessage(protocol.MessageTypeReliable, protocol.OpcodeResourceUpdate)
	writer := buffer.NewWriter(msg.Span())

	writer.WriteUint16(uint16(protocol.OpcodeResourceUpdate))
	writer.WriteUint32(objectID)
	writer.Write7BitEncodedInt64(int64(amount))

	msg.SetSize(writer.Position())
	sess.Queue().AddWithSequence(msg)
}

// sendCastingCombat sends opcode 0x00B1 — combat action (melee swing or spell cast).
// Wire format: opcode(2) + casterObjID(4) + targetObjID(4) + unk(1) + timerMs(2) + pad(2) + animHash(4).
// Timer >= 3000 = auto-attack, timer > 0 && < 4000 = spell cast.
// sendCastingCombat sends opcode 0x00B1 — combat action.
// PS2 func_00BD1F10: source(u32) + target(u32) + addlCount(u8) + castTimeMS(u32) + spellFXDictID(u32)
func (w *World) sendCastingCombat(sess *session.Session, casterObjectID, targetObjectID uint32, castTimeMS uint32, spellFXDictID uint32) {
	msg := protocol.NewMessage(protocol.MessageTypeReliable, protocol.OpcodeCastingCombat)
	writer := buffer.NewWriter(msg.Span())

	writer.WriteUint16(uint16(protocol.OpcodeCastingCombat))
	writer.WriteUint32(casterObjectID)
	writer.WriteUint32(targetObjectID)
	writer.WriteByte(0)               // additionalTargetCount = 0
	writer.WriteUint32(castTimeMS)    // uint32 (PS2 raw(4,1))
	writer.WriteUint32(spellFXDictID) // spell effect DictID

	msg.SetSize(writer.Position())
	sess.Queue().AddWithSequence(msg)
}

// sendCharacterDied sends opcode 0x005E — player death notification.
func (w *World) sendCharacterDied(sess *session.Session) {
	msg := protocol.NewMessage(protocol.MessageTypeReliable, protocol.OpcodeCharacterDied)
	writer := buffer.NewWriter(msg.Span())
	writer.WriteUint16(uint16(protocol.OpcodeCharacterDied))
	msg.SetSize(writer.Position())
	sess.Queue().AddWithSequence(msg)
}

// objectIDForEntityLocked finds the objectID assigned to an entity for a given session.
// Returns 0 if the entity is not visible to the session.
// Must be called with lock held.
func (w *World) objectIDForEntityLocked(sess *session.Session, entityID EntityID) uint32 {
	vis := sess.VisibilityState()
	if vis == nil {
		return 0
	}
	vs, ok := vis.(*VisibilityState)
	if !ok {
		return 0
	}
	for i := 0; i < MaxVisibleEntities; i++ {
		if vs.ChannelEntity[i] == entityID {
			return vs.ChannelObjectID[i]
		}
	}
	return 0
}

// sendChatToSession sends a chat message to a specific session.
func (w *World) sendChatToSession(sess *session.Session, message string) {
	msg := protocol.NewMessage(protocol.MessageTypeReliable, protocol.OpcodeClientMessage)
	writer := buffer.NewWriter(msg.Span())

	writer.WriteUint16(uint16(protocol.OpcodeClientMessage))
	writer.WriteStringUnicode(message)

	msg.SetSize(writer.Position())
	sess.Queue().AddWithSequence(msg)
}

// coherentWorldOnLoad rescues a character whose saved world its position
// contradicts, and says so loudly — a silent correction here would hide the
// failed world load that caused it.
func coherentWorldOnLoad(name string, world uint8, x, z float32) uint8 {
	fixed, ok := CoherentWorld(world, x, z)
	if !ok {
		log.Printf("Entity CreatePlayer: '%s' saved in world %d at (%.1f, %.1f) "+
			"which is outside that world — a world change must have failed to "+
			"load. Placing in world %d so they are not stranded.",
			name, world, x, z, fixed)
	}
	return fixed
}

// coachLocationNames maps coachman NPC names to friendly area labels.
var coachLocationNames = map[string]string{
	"Coachman Quicksteed": "Highpass",
	"Coachman Ronks":      "Freeport",
	"Coachman D'Larik":    "Fort Seriak",
	"Coachman Doba":       "Moradhim",
	"Coachman Farn":       "Muniels Tea Garden",
	"Coachman Huff":       "Rivervale",
	"Coachman Lothi":      "Neriak",
	"Coachwoman Darva":    "Darvor Manor",
}

// sortDests puts a destination list in a STABLE order.
//
// Both destination lists are built by ranging over w.NPCs, and Go RANDOMISES
// map iteration order on every call — while the player picks by INDEX. Caught
// live on 2026-08-06: two lists built one second apart came out
// [Odus (control) Guk] then [Guk Odus (control)], and a player aiming for Odus
// was teleported to guk instead.
//
// It is worse than it looks. The pick stays internally consistent with the
// list that was last sent, so the server logs a perfectly sensible teleport to
// a place the player did not choose and nothing anywhere reports an error —
// the only symptom is arriving somewhere else.
//
// Sorting by the DISPLAYED label is what makes the menu answer to what the
// player actually read.
func sortDests(d []CoachDest) {
	sort.Slice(d, func(i, j int) bool { return d[i].Name < d[j].Name })
}

// IsCoachman returns true if the NPC name indicates a coachman/coachwoman.
func IsCoachman(name string) bool {
	return strings.HasPrefix(name, "Coachman ") || strings.HasPrefix(name, "Coachwoman ")
}

// warpLocationNames maps warp NPC names to the place they stand in.
//
// Gatekeepers and Harbormasters are the CROSS-WORLD travel NPCs; coachmen only
// move you around within a world. That distinction is the whole reason guk
// needs one: a coach destination is an (x,y,z) in the world you are already in,
// whereas reaching guk means changing the world index the client loads from.
var warpLocationNames = map[string]string{
	"Harbormaster Thand": "Freeport",
	"Gatekeeper Krup":    "Guk",
	// A control, not content: Odus is a RETAIL world on the disc, so picking it
	// exercises the same 0x00B2 world change that guk uses without involving
	// the memory-card loader at all. If Odus renders, the world-change path is
	// sound and guk's content is at fault; if Odus is empty too, the path
	// itself is broken and guk was never the problem. Remove with the row in
	// sql/odus_control_gatekeeper.sql once that is settled.
	"Gatekeeper Sulon": "Odus (control)",
	// World 7 (`secrets2`) — a control that needs no patch deploy at all.
	// Sandstorm's table entry 7 is a verbatim duplicate of entry 5 (`secrets`),
	// i.e. a DISC world at a high index, already inside the bound our SD raised
	// from 6 to 9. If world 7 crashes the same way world 8 does, the fault is in
	// the world-change path rather than the memory-card loader, and we learn
	// that without shipping anything. See sql/secrets2_control_gatekeeper.sql.
	"Gatekeeper Rilis": "Secrets (world 7 control)",
	// World 9 — Tunaria zone 127 (Widows Peak) lifted whole by cmd/zonelift and
	// streamed off the PS2 HARD DISK. Not authored content: the Zone record is
	// byte-identical to retail's, so this route is the disk backend on trial,
	// not the geometry. See sql/world9_harbormaster.sql.
	"Harbormaster Machado": "Widows Peak (world 9, HDD)",
	// World 22 — an AUTHORED world, one zone, streamed off the hard disk and
	// FETCHED ON DEMAND: nothing describes it to the client but the file in the
	// server's worlds/ directory, since slot 22 already names hdd:world22.ESF.
	// This label is the whole of what a player sees, so rename it here when the
	// world gets a name of its own. See sql/world22_harbormaster.sql.
	"Harbormaster Deuces": "World 22 (authored, HDD)",
}

// IsWarpNPC returns true if the NPC name indicates a cross-world travel NPC.
func IsWarpNPC(name string) bool {
	return strings.HasPrefix(name, "Gatekeeper ") || strings.HasPrefix(name, "Harbormaster ")
}

// GetWarpDestinations returns every warp NPC except the one named excludeName,
// each as a destination at that NPC's own spawn point and world.
//
// Mirrors GetCoachDestinations deliberately: routes are discovered from the
// NPCs that exist rather than kept in a second table that could disagree with
// them. Adding a Gatekeeper to a new world therefore wires it up in both
// directions with no code change — which is exactly what connects Freeport and
// guk here, since Harbormaster Thand was already standing in Freeport.
func (w *World) GetWarpDestinations(excludeName string) []CoachDest {
	w.mu.RLock()
	defer w.mu.RUnlock()

	var dests []CoachDest
	for _, npc := range w.NPCs {
		if !IsWarpNPC(npc.Name) || npc.Name == excludeName {
			continue
		}
		label := npc.Name
		if friendly, ok := warpLocationNames[npc.Name]; ok {
			label = friendly
		}
		dests = append(dests, CoachDest{
			Name:   label,
			X:      npc.SpawnX,
			Y:      npc.SpawnY,
			Z:      npc.SpawnZ,
			Facing: npc.SpawnFacing,
			World:  npc.World,
		})
	}
	sortDests(dests)
	return dests
}

// GetCoachDestinations returns all coachman destinations except the one named
// excludeName, restricted to coachmen standing in world `inWorld`.
// Each destination uses the coachman's spawn position as the teleport target.
//
// The world filter is not currently load-bearing — every coachman is in
// Tunaria — but a coach route carries no world change, so an out-of-world
// coachman appearing in this list would teleport the player to another world's
// coordinates while leaving them in this one. Cheaper to exclude than to debug.
func (w *World) GetCoachDestinations(excludeName string, inWorld uint8) []CoachDest {
	w.mu.RLock()
	defer w.mu.RUnlock()

	var dests []CoachDest
	for _, npc := range w.NPCs {
		if !IsCoachman(npc.Name) {
			continue
		}
		if npc.Name == excludeName || npc.World != inWorld {
			continue
		}
		label := npc.Name
		if friendly, ok := coachLocationNames[npc.Name]; ok {
			label = friendly
		}
		dests = append(dests, CoachDest{
			Name:   label,
			X:      npc.SpawnX,
			Y:      npc.SpawnY,
			Z:      npc.SpawnZ,
			Facing: npc.SpawnFacing,
			World:  npc.World,
		})
	}
	sortDests(dests)
	return dests
}

// sendPlayerDied sends OpcodeCharacterDied (0x005E).
func (w *World) sendPlayerDied(sess *session.Session) {
	msg := protocol.NewMessage(protocol.MessageTypeReliable, protocol.OpcodeCharacterDied)
	writer := buffer.NewWriter(msg.Span())
	writer.WriteUint16(uint16(protocol.OpcodeCharacterDied))
	msg.SetSize(writer.Position())
	sess.Queue().AddWithSequence(msg)
}

// --- Guild System ---

var nextGuildID uint32 = 1

// CreateGuild creates a new guild and makes the player its leader.
func (w *World) CreateGuild(name string, leader *Player) uint32 {
	w.mu.Lock()
	defer w.mu.Unlock()

	for _, p := range w.Players {
		if p.GuildName == name && p.GuildID != 0 {
			return 0
		}
	}

	guildID := nextGuildID
	nextGuildID++

	leader.GuildID = guildID
	leader.GuildName = name
	leader.GuildRank = 0
	leader.GuildLevel = 1
	leader.GuildMOTD = "Welcome to " + name + "!"
	leader.GuildHallZone = -1
	leader.GuildHallInst = -1

	return guildID
}

// RemoveFromGuild removes a player from their guild.
func (w *World) RemoveFromGuild(player *Player) {
	w.mu.Lock()
	defer w.mu.Unlock()

	player.GuildID = 0
	player.GuildName = ""
	player.GuildRank = 0
	player.GuildMOTD = ""
	player.GuildCrestID = 0
	player.GuildTaxRate = 0
	player.GuildLevel = 0
	player.GuildBankGold = 0
	player.GuildXP = 0
	player.GuildHallZone = -1
	player.GuildHallInst = -1
}

// SetGuildMOTD updates the MOTD for all online members.
func (w *World) SetGuildMOTD(guildID uint32, motd string) {
	w.mu.Lock()
	defer w.mu.Unlock()

	for _, p := range w.Players {
		if p.GuildID == guildID {
			p.GuildMOTD = motd
		}
	}
}

// AddToGuild adds a player to an existing guild by copying info from an existing member.
func (w *World) AddToGuild(player *Player, guildID uint32) bool {
	w.mu.Lock()
	defer w.mu.Unlock()

	for _, p := range w.Players {
		if p.GuildID == guildID {
			player.GuildID = guildID
			player.GuildName = p.GuildName
			player.GuildRank = 7
			player.GuildMOTD = p.GuildMOTD
			player.GuildLevel = p.GuildLevel
			player.GuildCrestID = p.GuildCrestID
			player.GuildHallZone = p.GuildHallZone
			player.GuildHallInst = p.GuildHallInst
			return true
		}
	}
	return false
}

// GuildMembers returns all online players in a guild.
func (w *World) GuildMembers(guildID uint32) []*Player {
	w.mu.RLock()
	defer w.mu.RUnlock()

	var members []*Player
	for _, p := range w.Players {
		if p.GuildID == guildID {
			members = append(members, p)
		}
	}
	return members
}

// SpawnOfNPC returns the spawn point and world of the NPC with this exact
// name, or ok=false if no such NPC is loaded.
//
// It reports the SPAWN rather than the live position deliberately: a wandering
// or pushed NPC would otherwise make a named teleport land somewhere new every
// time it was used, and the spawn is the point the DB row actually asserts.
func (w *World) SpawnOfNPC(name string) (x, y, z, facing float32, world uint8, ok bool) {
	w.mu.RLock()
	defer w.mu.RUnlock()

	for _, npc := range w.NPCs {
		if npc.Name == name {
			return npc.SpawnX, npc.SpawnY, npc.SpawnZ, npc.SpawnFacing, npc.World, true
		}
	}
	return 0, 0, 0, 0, 0, false
}

// GetNPCsInRadius returns NPCs within radius of (x,z).
func (w *World) GetNPCsInRadius(x, z, radius float32) []*NPC {
	w.mu.RLock()
	defer w.mu.RUnlock()

	r2 := radius * radius
	var result []*NPC
	for _, npc := range w.NPCs {
		dx := npc.X - x
		dz := npc.Z - z
		if dx*dx+dz*dz <= r2 {
			result = append(result, npc)
		}
	}
	return result
}

// BeginWarp commits a player's destination world and hands back the next
// teleport counter, under the world lock.
//
// IT EXISTS BECAUSE OF WHO CALLS IT. `player.World = x` and
// `player.TeleportCounter++` are written directly all over the handlers, which
// is safe there because those run on the goroutine the UDP listener serialises.
// The account pages run on an HTTP goroutine and race the tick loop, so the same
// two writes have to be one locked operation — and the counter especially, since
// an increment read-modify-writes.
//
// `worldChanged` is reported rather than recomputed by the caller for the same
// reason: comparing before and after outside the lock is the race this closes.
// `facing` comes back so a caller with no opinion can keep the one the player
// already has without a second locked read.
func (w *World) BeginWarp(id EntityID, world byte) (facing float32, counter uint32, worldChanged, ok bool) {
	w.mu.Lock()
	defer w.mu.Unlock()

	p, found := w.Players[id]
	if !found {
		return 0, 0, false, false
	}
	worldChanged = p.World != world
	p.World = world
	p.TeleportCounter++
	return p.Facing, p.TeleportCounter, worldChanged, true
}

// WarpPoint is one cross-world travel NPC, identified by its own NAME rather
// than by its label.
//
// GetWarpDestinations returns labels because a dialogue box shows labels and
// nothing has to refer to a route afterwards. Something outside the game has to
// name a destination back to the server, and a label is display text — it is
// renamed the moment a world gets a real name, and every saved link would break.
// The NPC name is the thing that already keys the rest of this system.
type WarpPoint struct {
	NPC             string
	Label           string
	World           uint8
	X, Y, Z, Facing float32
}

// WarpPoints lists every cross-world travel NPC, in the same order and with the
// same friendly labels a Harbormaster's own menu uses.
//
// No exclusion: a Harbormaster leaves ITSELF out because a player standing in
// front of one cannot usefully travel to it. A caller who is not standing
// anywhere wants the whole set, Freeport included.
func (w *World) WarpPoints() []WarpPoint {
	w.mu.RLock()
	defer w.mu.RUnlock()

	var out []WarpPoint
	for _, npc := range w.NPCs {
		if !IsWarpNPC(npc.Name) {
			continue
		}
		label := npc.Name
		if friendly, ok := warpLocationNames[npc.Name]; ok {
			label = friendly
		}
		out = append(out, WarpPoint{
			NPC: npc.Name, Label: label, World: npc.World,
			X: npc.SpawnX, Y: npc.SpawnY, Z: npc.SpawnZ, Facing: npc.SpawnFacing,
		})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].World != out[j].World {
			return out[i].World < out[j].World
		}
		return out[i].Label < out[j].Label
	})
	return out
}
