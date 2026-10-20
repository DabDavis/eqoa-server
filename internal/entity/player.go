package entity

import "time"

// Player is a flat struct containing all player fields.
// Replaces 15+ ECS components with direct field access.
type Player struct {
	// Identity
	ServerID EntityID
	Name     string

	// Spatial
	X, Y, Z float32
	Facing  float32

	// Model
	ModelID int64
	Size    float32

	// Appearance
	Race, Sex, HumType                           int
	HairColor, HairLength, HairStyle, FaceOption int

	// Equipment
	Robe      uint32
	Primary   uint32
	Secondary uint32
	Shield    uint32
	Chest     byte
	Bracer    byte
	Gloves    byte
	Legs      byte
	Boots     byte
	Helm      byte

	// PlayerInfo
	AccountID    int
	CharServerID uint32 // Database server_id (primary key)
	Class        int

	// Health
	HP, MaxHP      int
	LastCombatTime time.Time

	// DiedAt is when the player last died (0x005E went out), zero while
	// alive. It stays set until respawn.go brings the player back, so a
	// death is told once and nothing heals or hits the corpse meanwhile.
	DiedAt time.Time

	// Power
	Power, MaxPower int

	// Core stats
	Level   int
	Speed   float32
	Faction byte

	// Target
	TargetObjectID uint32

	// Attributes
	Str, Sta, Agi, Dex, Wis, Int, Cha int

	// Experience
	TotalXP     int
	Debt        int
	XPInLevel   int // XP earned within current level (sent to client)
	UnusedTP    int
	TotalTP     int
	BaseMaxStat int

	// Currency
	Tunar, BankTunar int

	// Combat — SOE retail formulas (see claude-notes/combat-formula-final.md)
	AC                                                            int
	OffMod, DefMod                                                int // Computed offense/defense modifiers
	BaseStr, BaseSta, BaseAgi, BaseDex, BaseWis, BaseInt, BaseCha int // Stats before equipment
	WeaponDamage                                                  int // From equipped weapon item
	PoisonR, DiseaseR, FireR, ColdR, LightningR, ArcaneR          int

	// World is the world index the player is currently in (0=Tunaria … 8=Guk).
	// Loaded from characters.world and written back on every world-changing
	// teleport, so logging out in Guk and back in returns you to Guk rather
	// than dropping you at Guk's coordinates inside Tunaria.
	World uint8

	// Coaching / Teleport
	TeleportCounter   uint32
	PendingCoachDests []CoachDest
	CoachPage         int

	// Which hard-disk-setup dialogue this player has open, if any. Separate
	// from PendingCoachDests because the option handler has to know WHICH menu
	// a numbered pick belongs to: option 2 means "Odus" on a travel NPC and
	// "FORMAT THE DISK" on this one, and a menu that answered the wrong
	// question would be answering it destructively. See HDDMenu* in handlers.
	PendingHDDMenu int

	// Animation (from client position update byte 35)
	Animation byte

	// Velocity in units a second: the walker's own, out of the client's 0x40
	// (clientupdate.go), relayed in the entity state bytes 40-42; zero after
	// a server-side move (UpdatePosition).
	VelX, VelY, VelZ float32

	// AutoAttack is the 0x40's byte 36, CLIENT's m_autoAttack: the attack
	// button held (or toggled on). attack.go reads it at every swing.
	AutoAttack bool

	// The fight a 0x000F engaged (attack.go): the NPC, the object id the
	// player named it by, when the next swing is due and what it swings.
	AttackTarget   EntityID
	AttackObjectID uint32
	NextSwing      time.Time
	Swing          Swing

	// Breath / underwater
	Breath          int    // 0-127, depletes underwater (eqoa.live sends 127 in MemoryDump)
	Underwater      bool   // under a liquid's surface (UnderLiquid on WalkFlags)
	DrowningCounter int    // tick counter for 6s drowning damage interval
	WalkFlags       uint32 // the last 0x40's walker flags (clientupdate.go)
	HasWalkFlags    bool

	// Group (group.go): the group's id, 0 for none
	GroupID uint32

	// Guild
	GuildID       uint32
	GuildName     string
	GuildRank     int // 0=leader, 1-7=ranks
	GuildMOTD     string
	GuildCrestID  uint32
	GuildTaxRate  byte
	GuildLevel    int
	GuildBankGold int
	GuildXP       int
	GuildHallZone int // -1 = no hall
	GuildHallInst int // instance ID

	// Active effects, in the order the client shows them (state channel
	// 0x43, SerializeBuffUpdate); at most MaxEffects (AddEffect).
	Buffs []BuffSlot
}

// BuffSlot is one active effect as CLIENT's Channel43 reads it into
// m_effectIcon and m_effectName.
type BuffSlot struct {
	Icon uint32 // the icon's DictID: VIWndInfluence blits it whole, by DictID
	Name string // the client shows MaxEffectName characters at most
}

// CoachDest describes a coaching or warp teleport destination.
type CoachDest struct {
	Name            string
	X, Y, Z, Facing float32

	// World is the destination world index, sent RAW (not zigzag) in the
	// 0x00B2 Teleport byte — verified by capture, see
	// claude-notes/memcard-world-loading.md. Coach routes are all intra-world
	// and leave this 0; a Gatekeeper/Harbormaster route is the only thing that
	// currently sets it, and 8 is what reaches guk through the card loader.
	World uint8
}

// GetEntityID returns the player's entity ID.
func (p *Player) GetEntityID() EntityID {
	return p.ServerID
}

// GetPosition returns the player's world position.
func (p *Player) GetPosition() (x, y, z float32) {
	return p.X, p.Y, p.Z
}

// IsPlayerEntity returns true for players.
func (p *Player) IsPlayerEntity() bool {
	return true
}

// IsDead returns true if the player's HP is zero.
func (p *Player) IsDead() bool {
	return p.HP <= 0
}
