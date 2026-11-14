package entity

import "time"

// NPC is a flat struct containing all NPC fields.
// Replaces 11+ ECS components with direct field access.
type NPC struct {
	// pickedAt is when a pickpocket last took from it (workbookmore.go)
	pickedAt time.Time
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
	// Instance is the copy of an instanced world it stands in (homes.go), 0 every copy (the yard's board); an
	// NPC with an owner stands in its owner's
	Instance uint32

	// Model
	ModelID int64
	Size    float32

	// gmScaleBase is its size before a GM's /scale (gmscale.go), 0 when unscaled.
	gmScaleBase float32

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
	Tint            int // its armour's tint (npcs.unk_o): the channel's 99-104, $Tints' 0-14
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
	// WornPieces and WornMask: the item records a mercenary wears on the
	// pieces an aura can be on (MercLook's; pieceaura.go), none for others
	WornPieces [AuraSlots]int
	WornMask   uint16

	// Health
	HP, MaxHP      int
	LastCombatTime time.Time
	// Power is a pet's (summoned or charmed), regenerated as a player's
	// (npcregen.go); a creature that is nobody's pet keeps none
	Power, MaxPower int

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
	// effMaxHP is what a pet's effects add to its maximum hit points now,
	// MaxHP holding it (syncEffectMaxHP, pet.go).
	effMaxHP int

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
	// swimming: it is at the water's surface, swimming (water.go's
	// swimLocked); surface the height of that surface, found last
	swimming bool
	surface  float32

	// SpawnPoint
	SpawnX, SpawnY, SpawnZ float32
	SpawnFacing            float32
	// RespawnTime is how long after its corpse goes (corpse.go) it comes back.
	RespawnTime time.Duration
	DeathTime   time.Time
	// CorpseGone is when its corpse left the world: emptied and closed, or
	// decayed (corpse.go). Zero while it is alive or its corpse still lies.
	CorpseGone time.Time
	// corpseHeld keeps its corpse lying until then at least: a raid's loot rolls and
	// its winners' time (corpse.go's HoldCorpse); zero none.
	corpseHeld time.Time

	// roam is its wander round its spawn (roam.go), and spots the places it
	// wanders between, with the ways between them it has tried (roam.go's
	// leg cache; nil until it first wanders on the ground). spots outlives
	// roam: a death, a fight or no player near leaves them.
	roam  roamState
	spots *wanderSpots

	// chase is a fight's movement: its chase, its leash and the way home
	// (chase.go).
	chase chaseState

	// way is a pet's or a mercenary's way round what stands between it and
	// whom it goes to (petwalk.go).
	way petWay

	// fslot is a follower's place round whom it follows (followslot.go).
	fslot followSlotCache

	// path is where it is on the path a GM gave it (path.go).
	path pathState

	// onGround: it spawned on the terrain of the world's ground, so its moves
	// take its floor (groundnpc.go); groundKnown once that was looked at;
	// spawnLift how far above that terrain it spawned (a house's floor over
	// the street: Neriak's tavern's 0.77), which its wander is tested for
	// walls above too (movement.go). spawnPlaced: the floor it spawned on is
	// something placed's (a mausoleum's slab, a tower's floor: a ground that
	// is Stands, groundnpc.go), so its wander may keep to such floors;
	// spawnDeck: that floor is further over the terrain than spawnedAbove,
	// where it stood off the ground before the ground knew placed floors, and
	// does not wander, as then (movement.go's movementLocked). spawnFloor:
	// the floor it spawned on, the slime's surface it keeps to for a
	// creature of the water in the slime (water.go's keepsInLocked).
	groundKnown, onGround  bool
	spawnPlaced, spawnDeck bool
	spawnLift              float32
	spawnFloor             float32
	// under: where it stands another floor is over its own within its body
	// (groundnpc.go's movedUnderLocked), wasUnder before its last grounding
	under, wasUnder bool

	// AggroBehavior. RoamRadius is how far from its spawn a creature wanders
	// (roam.go); 0 for none.
	AggroRadius float32
	LeashRadius float32
	RoamRadius  float32

	// AggroState
	Threats map[EntityID]int64
	// damageBy is the damage each player made on it this life, its pets' and mercs' counted to it (killcredit.go)
	damageBy     map[EntityID]int64
	CurrentEnemy EntityID
	LastCombat   time.Time

	// guardHit: a town guard has damaged it since its fight began, so its
	// kill is the guards' (guards.go); townGuard: it is a town's guard, as
	// looked at under the social rules of generation guardGen.
	guardHit  bool
	townGuard bool
	guardGen  uint32

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
	// castsAt is the NPC a trigger's sic set it on to fight by its whole class
	// template (SetNPCOn's casts, classtemplate.go): its casts and specials land
	// on that NPC too, where a fight between two NPCs is their melee alone. 0 none.
	castsAt EntityID
	// keptEvent is a trigger's spawn a keep restart made again under its id
	// (keeptrigger.go): the clients' slots go on describing it (keep.go's RestoreView)
	keptEvent bool
	// keptMoved is a creature a keep restart put on its row's new place, a data step
	// having moved its spawn (keepnpc.go): every client that had it in view is made to
	// let go of it (RestoreView), so it comes in again where it stands, not sliding there
	keptMoved bool

	// Its melee round (rounds.go): when it began, how many of its hits
	// have landed, and at whom.
	RoundStart  time.Time
	RoundHits   int
	RoundTarget EntityID

	// Dummy is a training dummy's kind (dummy.go), by its name, and the
	// one it fights: a player, or a player's pet or mercenary.
	Dummy       DummyKind
	DummyTarget EntityID
	// dummyHate is what each one in a dummy's reach has been told on it, a
	// player, a pet or a mercenary: it fights the one with the most (dummy.go)
	dummyHate map[EntityID]int64
	// DummyHits is how many times as hard a sparring dummy hits, its hits and
	// its element's strike, by its name ("Sparring Dummy x2": DummyHitsOf); 1
	// for every other
	DummyHits int

	// HiddenUntil keeps it out of every view until then (gmlive.go): an NPC
	// the GM panel changed leaves each client's channels and comes back as
	// a new entity, made from its new record.
	HiddenUntil time.Time

	// SpawnOnly keeps it out of the world for good (spawnonly.go, table
	// npcspawnonly): the original a quest's spawn event copies, in no view,
	// fighting no one, standing on its spawn. Its copies are not.
	SpawnOnly bool
	// Title is a line over its name (npctitle.go, table npctitle by its
	// row): "" none.
	Title string
	// TitleColor is its title plate's colour, CLIENT's NameColors index: 0
	// an NPC's gold, 1 blue, 2 green (npctitle.go).
	TitleColor uint8
	// CopyOf is the npcs row a copy (Temporary: a spawn's, SpawnNPCCopy) was
	// made from, 0 for any other NPC (npcrow.go): a table's row key ("#n")
	// holds for the copies of that row too.
	CopyOf EntityID

	// Owner is the player whose pet it is (pet.go), 0 for any other NPC.
	Owner EntityID
	// GenieOf is the player whose lamp brought it (genie.go): never a pet
	// (Owner 0), 0 for any other NPC.
	GenieOf EntityID
	// PortalOf is the player whose item opened it (summonportal.go): never a
	// pet (Owner 0), 0 for any other NPC.
	PortalOf EntityID

	// CorpseDecay is how long its corpse lies when not the world's
	// (`world.corpse_decay`, corpse.go): a pet's (pet.go); 0 the world's.
	CorpseDecay time.Duration

	// SwingFixed makes SwingKind its swing whatever its class and level
	// (rounds.go): an enchanter's pet's (pet.go), a necromancer's zombie's,
	// mummy's and spectre's (petrotation.go). SwingRound is its round's
	// length when not the swing's delay (the spectre's 5.2 s), 0 for that.
	SwingFixed bool
	SwingKind  int
	SwingRound time.Duration

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
	// Spawner (gmchannel.go), until Unkill Spawner or a cold restart (one
	// that keeps the sessions keeps it too: keepnpc.go).
	GMHeld bool

	// tmpl is the class template it fights by (classtemplate.go), nil for
	// none; tbase what it was without one, kept while it has one; tfight its
	// fight by it (its abilities' clocks, its cast's time, its special), nil
	// out of one: tens of thousands of NPCs, a handful fighting by one.
	tmpl   *ClassTemplate
	tbase  templateBase
	tfight *templateFight
	// abilityReady is when each of its template's abilities with a cooldown
	// may be cast again, by abilityKey (abilitytimes.go): on the creature, not
	// its fight, so a fight's end and its death leave it; a restart that keeps
	// the sessions carries it (keepnpc.go). nil for none.
	abilityReady map[string]time.Time

	// sheet is a workbook pet's petstats row (petstats.go), nil for every
	// other NPC
	sheet *petSheet

	// challenge is the duel Mr. Moogie's challenge stood it in (metalmoogie.go):
	// the one player it fights and the one alone who may hurt it, and whose its
	// corpse's loot is; nil for every other NPC. Its corpse keeps it.
	challenge *npcChallenge

	// siege marks a siege's King or gate (siege.go): its match, the one side
	// that may hurt it; nil for every other NPC. Its corpse keeps it.
	siege *npcSiege

	// add is its tie to the creature whose summon called it (summon.go): whose
	// fight it joined, and its share of that creature's figures; nil for every
	// other NPC. Its corpse keeps it (a corpse of nothing: handlers' loot).
	add *addTie
	// withMaster: it died with its master (masterdeath.go), its body lying
	// petDismissedLies, a corpse of nothing; its next life clears it
	withMaster bool
	// masterPet: a creature's own pet (companion.go's pairs, a GM trigger's resummoned spawn:
	// MarkMasterPet), set at its death (killNPC), a corpse of nothing whichever of it and its master died
	// first (ticket 2008); its next life clears it
	masterPet bool
	// NoLoot: a GM trigger's spawn of a corpse of nothing (its step's noloot: the Erych
	// Arbiter's first two lives, the shard and keys on her last)
	NoLoot bool
	// merc is a hired mercenary's (merc.go: Owner its hirer's), familiar a
	// familiar's (familiar.go: never owned); nil for every other NPC
	merc     *mercState
	familiar *familiarState
	// mercPet is the merc whose pet this is (mercpet.go: Owner its merc's
	// hirer's), nil for every other NPC
	mercPet *mercState
	// racialPet: summoned by a racial ability (racialpet.go), its hit points the
	// same whatever its owner's class
	racialPet bool
}

// isMerc reports a hired mercenary.
func (n *NPC) isMerc() bool { return n != nil && n.merc != nil }

// isFamiliar reports a familiar (familiar.go).
func (n *NPC) isFamiliar() bool { return n != nil && n.familiar != nil }

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
