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

	guilds      map[uint32]*Guild // guild.go
	nextGuildID uint32
	leaving     []func(*Player)                   // OnPlayerLeaving
	entered     []func(*session.Session, *Player) // OnPlayerEntered
	respawn     RespawnFunc                       // OnPlayerRespawn (respawn.go)
	announced   map[*Player]bool                  // guildAnnounceSystem: told its guild at login
	// OnEntityEnter is called when a new entity enters a player's visibility range.
	// Set by the handler layer to send FullEntityCreate (0x00B3) reliable opcodes.
	OnEntityEnter EntityEnterCallback

	// Groups (group.go)
	groups      map[uint32]*Group
	nextGroupID uint32
	invites     map[EntityID]map[EntityID]bool // invitee -> inviters

	// Buddy and ignore lists (social.go), in memory only
	social socialLists

	// The item catalog's ranged weapons by model (NPCStore.RangedModels),
	// for an NPC's hand types: set once by LoadNPCs, never changed after.
	rangedModels map[uint32]byte

	// Casts on their way to landing, and the hook a gate sends home with
	// (cast.go)
	casts    []*castState
	onGate   GateFunc
	affected map[EntityID]*NPC // creatures with a spell's effect on them

	// Summoned pets by their owners' ids (pet.go)
	pets map[EntityID]*petState
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
		id = w.allocFreeIDLocked()
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
		Level:        level,
		Speed:        RunSpeed,
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
		Tunar:        StartingTunar(record),
		BankTunar:    record.BankTunar,
		Breath:       BreathFull,

		// Base stats (before equipment) for combat formulas
		BaseStr: str, BaseSta: sta, BaseAgi: agi, BaseDex: dex,
		BaseWis: wis, BaseInt: intel, BaseCha: cha,
	}
	// the maxima and the combat modifiers from the stats; the worn items
	// come with SetGear
	p.RecalcStats(true)

	w.rejoinGuildLocked(p)
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

	p, ok := w.Players[id]
	if !ok {
		w.mu.Unlock()
		return
	}
	w.leaveGroupLocked(p)
	w.SpatialIndex.Remove(id, p.X, p.Z)
	delete(w.Players, id)
	fns := w.leavingLocked(p)
	w.mu.Unlock()
	log.Printf("Entity: removed player ID=%d", id)
	for _, fn := range fns {
		fn(p)
	}
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
	// Only while the map still holds THIS session's player. A character that
	// logged in again has a new Player under the same ServerID (CreatePlayer
	// replaced the old one), and the old session timing out later must not
	// take the new one with it: that left the live session with no map entry,
	// so UpdatePosition dropped every move and the visibility and interact
	// range checks kept the spawn point.
	w.mu.Lock()
	if cur, ok := w.Players[p.ServerID]; !ok || cur != p {
		w.mu.Unlock()
		return
	}
	w.leaveGroupLocked(p)
	w.SpatialIndex.Remove(p.ServerID, p.X, p.Z)
	delete(w.Players, p.ServerID)
	fns := w.leavingLocked(p)
	w.mu.Unlock()
	for _, fn := range fns {
		fn(p)
	}
}

// OnPlayerLeaving registers fn to run as a player leaves the world, after its
// entity goes, without the World lock held (the guild shows it offline and
// lapses its invitations there).
func (w *World) OnPlayerLeaving(fn func(p *Player)) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.leaving = append(w.leaving, fn)
}

// OnPlayerEntered registers fn to run when a session's client says it is in
// the world (its first 0x0014), without the World lock held (the mail tells
// of grams waiting there).
func (w *World) OnPlayerEntered(fn func(sess *session.Session, p *Player)) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.entered = append(w.entered, fn)
}

// PlayerEntered runs the OnPlayerEntered hooks for sess's player.
func (w *World) PlayerEntered(sess *session.Session) {
	w.mu.RLock()
	p := w.sessionPlayerLocked(sess)
	fns := make([]func(*session.Session, *Player), len(w.entered))
	copy(fns, w.entered)
	w.mu.RUnlock()
	if p == nil {
		return
	}
	for _, fn := range fns {
		fn(sess, p)
	}
}

// leavingLocked forgets the player's guild announcement and returns the
// departure hooks to run once the lock is let go.
func (w *World) leavingLocked(p *Player) []func(*Player) {
	delete(w.announced, p)
	fns := make([]func(*Player), len(w.leaving))
	copy(fns, w.leaving)
	return fns
}

// PlayerByName is the online player of that name, compared without case, or
// nil.
func (w *World) PlayerByName(name string) *Player {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil
	}
	w.mu.RLock()
	defer w.mu.RUnlock()
	for _, p := range w.Players {
		if strings.EqualFold(p.Name, name) {
			return p
		}
	}
	return nil
}

// UpdatePosition moves an entity: for a player, a server-side move (a
// teleport, a GM command), which arrives standing still. A client's own 0x40
// goes through ApplyClientUpdate, with the walker's velocity.
func (w *World) UpdatePosition(id EntityID, x, y, z, facing float32) {
	w.mu.Lock()
	defer w.mu.Unlock()

	if p, ok := w.Players[id]; ok {
		oldX, oldZ := p.X, p.Z
		p.VelX, p.VelY, p.VelZ = 0, 0, 0
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

// ProcessAttack lands one swing at once, in melee range: the GM kill command.
// A player's 0x000F engages a fight instead (EngageAttack, attack.go).
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

	// One swing, as the auto-attack loop lands it (attack.go): the SOE retail
	// formula, the damage figure without a 0x00B1 (a client plays its own
	// swing), the NPC's threat, the kill and XP.
	w.playerHitLocked(sess, attacker, npc, targetObjectID)
}

// KillNPCByObjectID is the GM's #kill: the NPC the session knows by that id
// dead at once (no swing, no XP). It reports the NPC's name, or false when
// the id names no living NPC.
func (w *World) KillNPCByObjectID(sess *session.Session, objectID uint32) (string, bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	ent, found := w.findEntityByObjectIDLocked(sess, objectID)
	if !found {
		return "", false
	}
	npc, ok := ent.(*NPC)
	if !ok || npc.Dead {
		return "", false
	}
	npc.HP = 0
	w.killNPC(npc)
	return npc.Name, true
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

	// each name's class (npcclass), for its melee swing
	classes, err := npcStore.NPCClasses()
	if err != nil {
		log.Printf("Entity LoadNPCs: NPC classes: %v (every NPC a warrior)", err)
	}
	// the catalog's ranged weapons, for the hands' attack types
	if w.rangedModels, err = npcStore.RangedModels(); err != nil {
		log.Printf("Entity LoadNPCs: ranged weapons: %v (every NPC weapon a melee one)", err)
	}

	count := 0
	hands := map[byte]int{} // the held items' attack types, both hands
	for _, rec := range npcs {
		npc := NPCFromRecord(rec, classes[database.NPCClassKey(rec.NPCName)], w.rangedModels)
		w.NPCs[npc.ServerID] = npc
		w.SpatialIndex.Insert(npc.ServerID, npc.X, npc.Z)
		if npc.PrimaryWeapon != 0 {
			hands[npc.PrimaryType]++
		}
		if npc.SecondaryWeapon != 0 {
			hands[npc.SecondaryType]++
		}

		count++
		log.Printf("Entity LoadNPCs: '%s' at (%.1f, %.1f, %.1f)", rec.NPCName, npc.X, npc.Y, npc.Z)
	}

	log.Printf("Entity LoadNPCs: loaded %d NPCs into world; held items by attack type %v (%d ranged weapon models)", count, hands, len(w.rangedModels))
}

// NPCFromRecord is the NPC a row of npcs spawns, at full health on its spawn
// point; class is the row's npcclass entry (a warrior when it has none), and
// ranged the catalog's ranged weapons (NPCStore.RangedModels), for its hands'
// attack types. The one constructor for a row: LoadNPCs at startup and the GM
// panel's edits (gmlive.go) both build through it, so an NPC edited live is
// the NPC the next restart would load.
func NPCFromRecord(rec *database.NPCRecord, class int, ranged map[uint32]byte) *NPC {
	posX := float32(rec.XCoord)
	posY := float32(rec.YCoord)
	posZ := float32(rec.ZCoord)
	facing := float32(rec.Facing) / 256.0 * 2.0 * math.Pi

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

	primaryType, secondaryType := npcHandTypes(rec.NPCName, uint32(rec.PrimaryWeapon), uint32(rec.SecondaryWeapon), ranged)
	return &NPC{
		ServerID:        EntityID(rec.ID),
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
		PrimaryType:     primaryType,
		SecondaryType:   secondaryType,
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

		Dummy: DummyKindOf(rec.NPCName),
		Class: class,
	}
}

// FindEntityByObjectID resolves a runtime objectID to an Entity.
func (w *World) FindEntityByObjectID(sess *session.Session, objectID uint32) (Entity, bool) {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.findEntityByObjectIDLocked(sess, objectID)
}

// findEntityByObjectIDLocked resolves an id the sender's client gave: its
// own player's, an object id from ITS visibility channels, or a group
// stand-in (entitySeenByLocked). Object ids are per session, each table
// counting its own from 100001, so no other session's table is read. Until
// 2026-10-23 every session's was, the first with the id taken: with two
// players online one's 0x0010 could name what the other calls by that
// number, and the target's details, a fight's 0x000F, Q's talk and a
// merchant went to that creature instead. Must be called with the lock held.
func (w *World) findEntityByObjectIDLocked(sess *session.Session, objectID uint32) (Entity, bool) {
	if sess == nil {
		return nil, false
	}
	e := w.entitySeenByLocked(sess, objectID)
	return e, e != nil
}

// SessionPlayer extracts the *Player from a session's ECS entity field.
func (w *World) SessionPlayer(sess *session.Session) *Player {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.sessionPlayerLocked(sess)
}

// IsLivePlayer reports whether p is the world's current Player for its
// ServerID — false for the Player of a session whose character logged in
// again (CreatePlayer replaced it).
func (w *World) IsLivePlayer(p *Player) bool {
	if p == nil {
		return false
	}
	w.mu.RLock()
	defer w.mu.RUnlock()
	cur, ok := w.Players[p.ServerID]
	return ok && cur == p
}

// LivePosition is where the character's player stands while some session
// still has it in the world, with savePlayerPosition's guards applied (a
// fall keeps the stored height prevY, a world that does not contain the
// position is world 0's), and whether there is a live one to take it from.
// A dead player has none: its place is not where the next login starts.
func (w *World) LivePosition(id EntityID, prevY float32) (x, y, z, facing float32, world uint8, ok bool) {
	w.mu.RLock()
	defer w.mu.RUnlock()
	p, found := w.Players[id]
	if !found || p.IsDead() {
		return 0, 0, 0, 0, 0, false
	}
	y, _ = SafeSaveHeight(p.Y, prevY)
	world, _ = CoherentWorld(p.World, p.X, p.Z)
	return p.X, y, p.Z, p.Facing, world, true
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
	if npc.Dummy != NotDummy {
		return // a dummy holds no grudge: it fights its one player (dummy.go)
	}
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
			player.RecalcStats(false)
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

// actionDrowning is the action Sandstorm plays on a drowning player with
// 0x0775: pcaps/breath_capture.pcap carries fifteen (self, 34).
const actionDrowning = 34

// sendEntityAction sends opcode 0x0775, which CLIENT reads as u32 id + u8
// action (dispatcher case 0x0775): it finds the entity and, if its sprite is
// a character sprite, plays the action on it -- for the player's own entity
// only action 34. It is not a resource update: the zigzag amount this used to
// write reached a PS2 as action 1 or 3 and did nothing, and the breath bar is
// the client's own.
func (w *World) sendEntityAction(sess *session.Session, objectID uint32, action byte) {
	msg := protocol.NewMessage(protocol.MessageTypeReliable, protocol.OpcodeEntityAction)
	writer := buffer.NewWriter(msg.Span())

	writer.WriteUint16(uint16(protocol.OpcodeEntityAction))
	writer.WriteUint32(objectID)
	writer.WriteByte(action)

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
	for i := range vs.ChannelEntity {
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
// move you around within a world. That distinction is the whole reason a world
// change needs one: a coach destination is an (x,y,z) in the world you are
// already in, whereas another world means changing the world index the client
// loads from. Only a client whose table has that world is offered it
// (GetWarpDestinations' loadable; handlers.LoadableWorlds).
var warpLocationNames = map[string]string{
	"Harbormaster Thand": "Freeport",
	// World 8 was guk while worlds came off the memory card (sql/guk_gatekeeper.sql);
	// it is our hard-disk loader's hdd:world8.ESF now, and the real Guk is in
	// Tunaria. Labelled "Guk" until 2026-10-25, which sent a GM's "Guk" to
	// world 8 (F3 20261025-000749).
	"Gatekeeper Krup": "World 8 (HDD)",
	// Odus is a RETAIL world on the disc. It was added as a control for the
	// world-change path (sql/odus_control_gatekeeper.sql) and labelled
	// "Odus (control)"; the path is settled and players use it, so it is Odus.
	"Gatekeeper Sulon": "Odus",
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
// each as a destination at that NPC's own spawn point and world, of the worlds
// loadable takes (nil: all of them).
//
// Mirrors GetCoachDestinations deliberately: routes are discovered from the
// NPCs that exist rather than kept in a second table that could disagree with
// them. Adding a Gatekeeper to a new world therefore wires it up in both
// directions with no code change — which is exactly what connects Freeport and
// the hard-disk worlds here, since Harbormaster Thand was already standing in
// Freeport. A world the asking client cannot load is not offered: picked, it
// moved nothing on the client while the server filed the player under it.
func (w *World) GetWarpDestinations(excludeName string, loadable func(world uint8) bool) []CoachDest {
	w.mu.RLock()
	defer w.mu.RUnlock()

	var dests []CoachDest
	for _, npc := range w.NPCs {
		if !IsWarpNPC(npc.Name) || npc.Name == excludeName {
			continue
		}
		if loadable != nil && !loadable(npc.World) {
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

// WarpPoints lists every cross-world travel NPC of the worlds loadable takes
// (nil: all of them), in the same order and with the same friendly labels a
// Harbormaster's own menu uses.
//
// No exclusion: a Harbormaster leaves ITSELF out because a player standing in
// front of one cannot usefully travel to it. A caller who is not standing
// anywhere wants the whole set, Freeport included.
func (w *World) WarpPoints(loadable func(world uint8) bool) []WarpPoint {
	w.mu.RLock()
	defer w.mu.RUnlock()

	var out []WarpPoint
	for _, npc := range w.NPCs {
		if !IsWarpNPC(npc.Name) {
			continue
		}
		if loadable != nil && !loadable(npc.World) {
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
