package entity

import "time"

// NPC is a flat struct containing all NPC fields.
// Replaces 11+ ECS components with direct field access.
type NPC struct {
	// Identity
	ServerID EntityID
	Name     string

	// Spatial
	X, Y, Z float32
	Facing  float32

	// World is the world index this NPC lives in (0=Tunaria … 8=Guk), from the
	// npcs.world column. Until Guk there was exactly one world and nothing
	// needed to ask; now an NPC at Guk's (1200, 0, 1000) and a Tunarian at the
	// same coordinates are different places, so visibility has to filter on it.
	World uint8

	// Model
	ModelID int64
	Size    float32

	// NPCInfo
	NPCID   int
	NPCType int

	// NPCEquipment
	PrimaryWeapon   int
	SecondaryWeapon int
	Shield          int
	Torso           int
	Forearm         int
	NPCGloves       int
	NPCLegs         int
	Feet            int
	Head            int
	HairColor       int
	HairLength      int
	HairStyle       int
	Face            int
	RobeStyle       string
	Race            string

	// Health
	HP, MaxHP      int
	LastCombatTime time.Time

	// Core stats
	Level   int
	Speed   float32
	Faction byte
	Dead    bool

	// Class is the NPC's class (WAR=0 ... ALC=14), from the npcclass table
	// by its name; it picks the NPC's melee swing (rounds.go).
	Class int

	// Effects is the spells on it (cast.go): damage ticks, holds, slows.
	Effects []*ActiveEffect

	// Combat — computed from level via SOE formulas
	OffMod       int
	DefMod       int
	WeaponDamage int

	// SpawnPoint
	SpawnX, SpawnY, SpawnZ float32
	SpawnFacing            float32
	RespawnTime            time.Duration
	DeathTime              time.Time

	// AggroBehavior
	AggroRadius float32
	LeashRadius float32
	RoamRadius  float32

	// AggroState
	Threats      map[EntityID]int64
	CurrentEnemy EntityID
	LastCombat   time.Time

	// Temporary is a GM's /spawn (devspawn.go): it has no database row, and
	// once dead it leaves the world after its corpse time instead of
	// respawning.
	Temporary bool

	// Its melee round (rounds.go): when it began, how many of its hits
	// have landed, and at whom.
	RoundStart  time.Time
	RoundHits   int
	RoundTarget EntityID

	// Dummy is a training dummy's kind (dummy.go), by its name, and the
	// player it fights.
	Dummy       DummyKind
	DummyTarget EntityID
}

// GetEntityID returns the NPC's entity ID.
func (n *NPC) GetEntityID() EntityID {
	return n.ServerID
}

// GetPosition returns the NPC's world position.
func (n *NPC) GetPosition() (x, y, z float32) {
	return n.X, n.Y, n.Z
}

// IsPlayerEntity returns false for NPCs.
func (n *NPC) IsPlayerEntity() bool {
	return false
}

// IsDead returns true if the NPC is dead.
func (n *NPC) IsDead() bool {
	return n.Dead
}
