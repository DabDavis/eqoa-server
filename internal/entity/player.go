package entity

import (
	"time"

	"github.com/eqoa/server/internal/voice"
)

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

	// Equipment: the three items the channel shows (the main hand, the off
	// hand, the shield) and the two hands' attack types (handlers.wornLook).
	// Robe is the worn robe's style and RobeWorn whether one is worn: style
	// 0 is a robe (the first), so it cannot also mean none, and the zero
	// Player wears none.
	Robe          uint32
	RobeWorn      bool
	Primary       uint32
	Secondary     uint32
	Shield        uint32
	PrimaryType   byte
	SecondaryType byte
	Chest         byte
	Bracer        byte
	Gloves        byte
	Legs          byte
	Boots         byte
	Helm          byte

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

	// MoveRateBonus is what movement masteries add to the movement rate of 20
	// (cast.go's MoveRate): the MOVEMENT RATE of the masteries the character
	// bought (cmbuy.go).
	MoveRateBonus int

	// Factions is the character's standing with each faction it has moved
	// off the faction's starting standing (faction.go; table charfactions).
	Factions map[string]int

	// Target
	TargetObjectID uint32

	// Attributes
	Str, Sta, Agi, Dex, Wis, Int, Cha int

	// Experience
	TotalXP   int
	Debt      int
	XPInLevel int // XP earned within current level (sent to client)
	UnusedTP  int
	TotalTP   int

	// Rez is what a resurrection may still give back of the latest death's
	// debt (resurrect.go)
	Rez RezChance

	// Class mastery points and the share of experience going to them
	// (mastery.go); the stat caps are the level's plus what its masteries
	// raise (StatCapOf)
	Mastery Mastery
	// CMs is the masteries it bought, by cmcatalog id, and what each cost
	// (table charcms); CMEffects what they give, summed by the window's 29
	// stats (cmbuy.go), which RecalcStats puts on its stats.
	CMs       map[int]int
	CMEffects [CMSlots]int

	// Currency
	Tunar, BankTunar int

	// Combat — SOE retail formulas (see claude-notes/combat-formula-final.md)
	AC                                                            int
	OffMod, DefMod                                                int // Computed offense/defense modifiers
	BaseStr, BaseSta, BaseAgi, BaseDex, BaseWis, BaseInt, BaseCha int // Stats before equipment
	WeaponDamage                                                  int // From equipped weapon item
	PoisonR, DiseaseR, FireR, ColdR, LightningR, ArcaneR          int

	// Gear is what the worn items add (gear.go); RecalcStats puts it on.
	Gear Gear

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

	// PendingBind is the bind point a spiritmaster's open "Do you wish to set
	// this as your bind point?" offers (Set: a question is open;
	// spiritmaster.go in handlers).
	PendingBind Bind

	// PendingYesNo marks PendingCoachDests as a waysender's one route asked
	// as Yes or No (waysender.go in handlers) rather than a list of places.
	PendingYesNo bool

	// Bind is where Return Home and a death take the player: the spot before
	// a spiritmaster it bound to, kept in charbind; unset, the login spawn.
	Bind Bind

	// PendingSpar is a sparring dummy's open menu (sparring.go in handlers).
	PendingSpar SparPick

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
	// player named it by, when the next swing is due and what it swings,
	// and when the press's button is read (a tap starts no round).
	AttackTarget   EntityID
	AttackObjectID uint32
	NextSwing      time.Time
	Swing          Swing
	ButtonReadAt   time.Time
	// roundWaits: the next round waits for the button and the reach, and
	// begins with the tick that sees both (attack.go)
	roundWaits bool

	// The melee round in hand (attack.go): when it began and how many of its
	// hits have come due.
	RoundStart time.Time
	RoundHits  int

	// ProfanityFilterOff: the player turned the profanity filter off
	// (handlers/profanity.go, opcode 0x0045); it is on at every login.
	ProfanityFilterOff bool

	// PowerBars: the player's client draws others' power bars and asked
	// for their power (powerbars.go, opcode 0x7E01); powerBarsSent is the
	// set it was last sent, so only a change goes out, and powerBarsDue
	// sends the next one whatever it is (just asked).
	PowerBars     bool
	powerBarsSent string
	powerBarsDue  bool

	// Voice: the player's client has voice chat on (voice.go, opcode
	// 0x7E10); VoiceID is its voice id in the forwarding server while it is
	// (never 0), the rest its voice's state
	VoiceOn         bool
	VoiceID         uint32
	voiceGroup      string // the group's channel the forwarding server has
	voiceSink       *voiceSink
	voiceCredsUntil time.Time
	voiceRate       voice.Rate
	voiceAsm        voice.Assembler
	voiceDropLogged time.Time

	// Breath / underwater
	Breath          int    // 0-255 (255 full), as the dump and stat channel byte 232 carry it; 32 every 6 s under liquid
	Underwater      bool   // under a liquid's surface (UnderLiquid on WalkFlags)
	DrowningCounter int    // ticks under liquid since the last 6 s step (breath, then drowning)
	WalkFlags       uint32 // the last 0x40's walker flags (clientupdate.go)
	HasWalkFlags    bool

	// Stood is the last place the player was known to stand, on a floor or
	// in a liquid (clientupdate.go's Standing): where it arrived (a login, a
	// server-side move, a respawn) or a 0x40 whose walker flags say so;
	// StoodWorld its world. A logout in the air saves it (SavePlace).
	StoodX, StoodY, StoodZ float32
	StoodWorld             uint8
	HasStood               bool

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

	// Casting (cast.go): the cast on, each ability's lockout, the spells'
	// effects on the player and what they add to its stats.
	Casting  *castState
	Lockouts []castLockout
	Effects  []*ActiveEffect
	Mods     StatMods

	// StatsDirty sends the stat and effect channels on the next tick, not
	// at the next 2 s round: a cast's cost goes out with its start.
	StatsDirty bool
}

// BuffSlot is one active effect as CLIENT's Channel43 reads it into
// m_effectIcon and m_effectName.
type BuffSlot struct {
	Icon uint32 // the icon's DictID: VIWndInfluence blits it whole, by DictID
	Name string // the client shows MaxEffectName characters at most
}

// Breath is a byte, as the client's gauge reads it (VIPlayer's m_p30 over
// 255): full at BreathFull, BreathStep lost every 6 s under liquid (Sandstorm,
// pcaps/breath_capture.pcap: 0xFF, 0xDF, ... 0x1F, 0).
const (
	BreathFull = 255
	BreathStep = 32
)

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

	// Fee is the tunar the trip costs (a waysender's; 0 free), MinLevel the
	// level it needs (0 any), Via the NPC who sends the player.
	Fee      int
	MinLevel int
	Via      string
}

// SparPick is a sparring dummy's menu as far as the player has gone: the
// dummy, the question open (1 its level, 2 its element; 0 none) and the
// level picked (an index of SparLevels).
type SparPick struct {
	NPC   EntityID
	Asked int
	Level int
}

// Bind is a player's bind point: where a spiritmaster bound it (Set).
type Bind struct {
	Set             bool
	World           uint8
	X, Y, Z, Facing float32
	// By is the spiritmaster who bound it; NPC its entity, for the bind's
	// visual (not kept: a spiritmaster's id is the running world's)
	By  string
	NPC EntityID
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
