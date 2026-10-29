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

	// NPCEquipment; the two hands' attack types are npcHandTypes'
	PrimaryWeapon   int
	SecondaryWeapon int
	PrimaryType     byte
	SecondaryType   byte
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

	// Movement on the state channel (npcmove.go): the velocity in units a
	// second (bytes 40-42) and the animation CLIENT plays (byte 58, a
	// VICSpriteAnimID: 0 stand, 1 walk, 3 run; a corpse's 14 is written by
	// the channel). All zero for an NPC standing still, as before.
	VelX, VelY, VelZ float32
	Loco             byte

	// SpawnPoint
	SpawnX, SpawnY, SpawnZ float32
	SpawnFacing            float32
	// RespawnTime is how long after its corpse goes (corpse.go) it comes back.
	RespawnTime time.Duration
	DeathTime   time.Time
	// CorpseGone is when its corpse left the world: emptied and closed, or
	// decayed (corpse.go). Zero while it is alive or its corpse still lies.
	CorpseGone time.Time

	// roam is its wander round its spawn (roam.go).
	roam roamState

	// chase is a fight's movement: its chase, its leash and the way home
	// (chase.go).
	chase chaseState

	// path is where it is on the path a GM gave it (path.go).
	path pathState

	// onGround: it spawned on the terrain of the world's ground, so its moves
	// take its floor (groundnpc.go); groundKnown once that was looked at.
	groundKnown, onGround bool

	// AggroBehavior. RoamRadius is how far from its spawn a creature wanders
	// (roam.go); 0 for none.
	AggroRadius float32
	LeashRadius float32
	RoamRadius  float32

	// AggroState
	Threats      map[EntityID]int64
	CurrentEnemy EntityID
	LastCombat   time.Time

	// nature is whether it attacks on sight (aggro.go's hostileLocked): 0
	// not looked at, 1 it does, -1 it does not; natureGen the GMs' rules it
	// was looked at under.
	nature    int8
	natureGen uint32

	// Temporary is a GM's /spawn (devspawn.go): it has no database row, and
	// once dead it leaves the world after its corpse time instead of
	// respawning.
	Temporary bool
	// QuestSpawn is a Temporary copy a quest step's spawn event placed
	// (SpawnForQuest): the quests take it away when the step is done.
	QuestSpawn bool
	// EventAggro is a GM trigger's hostile spawn (gmtriggers.go): it
	// attacks any player on sight, in a town too, whatever their levels.
	EventAggro bool

	// Its melee round (rounds.go): when it began, how many of its hits
	// have landed, and at whom.
	RoundStart  time.Time
	RoundHits   int
	RoundTarget EntityID

	// Dummy is a training dummy's kind (dummy.go), by its name, and the
	// player it fights.
	Dummy       DummyKind
	DummyTarget EntityID

	// HiddenUntil keeps it out of every view until then (gmlive.go): an NPC
	// the GM panel changed leaves each client's channels and comes back as
	// a new entity, made from its new record.
	HiddenUntil time.Time

	// Owner is the player whose pet it is (pet.go), 0 for any other NPC.
	Owner EntityID

	// CorpseDecay is how long its corpse lies when not the world's
	// (`world.corpse_decay`, corpse.go): a pet's (pet.go); 0 the world's.
	CorpseDecay time.Duration

	// SwingFixed makes SwingKind its swing whatever its class and level
	// (rounds.go): an enchanter's pet's (pet.go).
	SwingFixed bool
	SwingKind  int

	// A sparring dummy's element (an index of SparElements), the rounds it
	// has swung in its fight and the start of the round it last struck with
	// that element in (sparring.go).
	SparElement int
	SparRounds  int
	SparStruck  time.Time

	// Charm is a charm's hold on it (charm.go): what it was before, put
	// back when the charm ends or it comes back after dying; nil for a
	// creature no one charmed.
	Charm *charmHold

	// calmTicks counts the six-second ticks it has been out of a fight;
	// past npcCalmTicks it heals fast (npcregen.go). Any threat puts it
	// back to 0. healing: it has begun to heal and is not yet whole (for the
	// log's one line a fight).
	calmTicks int
	healing   bool

	// GMHeld keeps it from coming back after its death: a GM's Kill
	// Spawner (gmchannel.go), until Unkill Spawner or a restart.
	GMHeld bool

	// tmpl is the class template it fights by (classtemplate.go), nil for
	// none; tbase what it was without one, kept while it has one; tfight its
	// fight by it (its abilities' clocks, its cast's time, its special), nil
	// out of one: tens of thousands of NPCs, a handful fighting by one.
	tmpl   *ClassTemplate
	tbase  templateBase
	tfight *templateFight
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
