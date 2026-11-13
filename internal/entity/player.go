package entity

import (
	"time"
)

// Player is a flat struct containing all player fields.
// Replaces 15+ ECS components with direct field access.
type Player struct {
	// Identity
	ServerID EntityID
	Name     string

	// genie is its lamp's genie and the window it opened (genie.go)
	genie *genieState
	// portal is the NPC of the portal it opened, 0 for none (summonportal.go)
	portal EntityID
	// progressSaved is its figures last written while it plays (progresssave.go)
	progressSaved progressMark

	// Spatial
	X, Y, Z float32
	Facing  float32

	// Model
	ModelID int64
	Size    float32

	// gmScaleBase is its size before a GM's /scale (gmscale.go), 0 when unscaled.
	gmScaleBase float32

	// gmSpeedBase is its run speed before a GM's /speed (gmscale.go), 0 when unchanged.
	gmSpeedBase float32

	// Appearance
	Race, Sex, HumType                           int
	HairColor, HairLength, HairStyle, FaceOption int

	// Equipment: the three items the channel shows (the main hand, the off
	// hand, the shield) and the two hands' attack types (handlers.wornLook).
	// Robe is the worn robe's style and RobeWorn whether one is worn: style
	// 0 is a robe (the first), so it cannot also mean none, and the zero
	// Player wears none.
	Robe     uint32
	RobeWorn bool
	// RobeTint is the worn robe's disc tint (the channel's byte 110) and
	// RobeColor its own colour for the live game's look (0: its tint's), both
	// off its record's colour (RobeLook)
	RobeTint  byte
	RobeColor uint32
	// ArmorColors are the worn armour's own colours for the live game's look,
	// by dye slot (database.DyeChest ... DyeHelm), 0 none: a record's colour
	// that is a colour (0xRRGGBBAA, not a disc tint's index or white), drawn
	// where the slot has no dye, as a robe's is (ArmorColor, LookDyes)
	ArmorColors [6]uint32
	// HeldColors are the held items' own colours for the live game's look
	// (main hand, off hand, shield: the channel's items 0-2, by dye slot
	// database.DyePrimary + i), 0 none: the record's colour when it is a
	// colour, as ArmorColors (ArmorColor), drawn where the hand has no dye.
	// A client that draws it recolours the item's glow (vanilla; the user,
	// 2026-11-13: the Heirloom Sword of Twisted Virtue's crimson aura)
	HeldColors [3]uint32
	// WornPieces are the item records (itempattern.patternid) worn on the
	// pieces an aura can be on (AuraChest ... AuraRobe), WornMask which are
	// worn: each piece's aura is its item's (pieceaura.go, table itemaura)
	WornPieces    [AuraPieces]int
	WornMask      uint8
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
	// Dyes: a colour per armour slot, held item and the robe (livelook.go,
	// table chardyes), drawn by a client of the live game's look; 0 none.
	Dyes Dyes
	// CapeStyle is the worn cape's style (its record's model, 1-255; 0 none
	// worn) and CapeColor its colour (0xRRGGBBAA, 0 the art's own): an equip
	// slot of this server's own, told only to the clients that draw capes
	// (capes.go; handlers.wornLook sets them)
	CapeStyle uint8
	CapeColor uint32
	// HiddenLocs are the equip slots whose look the character hides (a bit
	// each, table charhiddenlooks) and HiddenParts what of the look that
	// hides, by HideChest...HideCape (handlers.wornLook sets them): sent as
	// nothing worn, the items kept (hidelook.go)
	HiddenLocs  uint32
	HiddenParts uint16

	// PlayerInfo
	AccountID    int
	CharServerID uint32 // Database server_id (primary key)
	Class        int

	// Health
	HP, MaxHP      int
	LastCombatTime time.Time

	// follow is the formation its followers keep round it (followslot.go)
	follow followFrame

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
	// seaSpeed: a sea folk race in water moves at Spirit of the Wolf's rate (seaspeed.go)
	seaSpeed bool

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
	// BonusTP is how many of TotalTP no level gave: an item's gift (potion.go,
	// table charbonustp), left out of the levels' top-up (training.go)
	BonusTP int

	// Rez is what a resurrection may still give back of the latest death's
	// debt (resurrect.go)
	Rez RezChance
	// RezReturn is a rez's offer of the way back to the body (resurrect.go)
	RezReturn RezReturn

	// Class mastery points and the share of experience going to them
	// (mastery.go); the stat caps are the level's plus what its masteries
	// raise (StatCapOf)
	Mastery Mastery
	// CMs is the masteries it bought, by cmcatalog id, and what each cost
	// (table charcms); CMEffects what they give, summed by the window's 29
	// stats (cmbuy.go), which RecalcStats puts on its stats.
	CMs       map[int]int
	CMEffects [CMSlots]int
	// WereOwnBody is whether a rank it owns keeps it in its own body under a
	// were form (NoWereIllusion, wereownbody.go), summed with CMEffects
	WereOwnBody bool
	// CMBreathPct is how much longer its breath lasts under water, in percent,
	// from the ranks it owns (CMRank.BreathPct: Hold Breath)
	CMBreathPct int

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
	// Instance is the copy of an instanced world it stands in (homes.go: a home, by its owner's character id);
	// nothing outside an instanced world
	Instance uint32

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
	// buttonUpAt is when it last went up (attack.go pressUnsentLocked)
	buttonUpAt time.Time

	// The fight a 0x000F engaged (attack.go): the NPC, the object id the
	// player named it by, when the next swing is due and what it swings,
	// and when the press's button is read (a tap starts no round).
	AttackTarget   EntityID
	AttackObjectID uint32
	NextSwing      time.Time
	Swing          Swing
	// swingNext is what the next round swings with when the hands changed in
	// the fight (swingNew; attack.go HandsChanged)
	swingNext    Swing
	swingNew     bool
	ButtonReadAt time.Time
	// roundWaits: the next round waits for the button and the reach, and
	// begins with the tick that sees both (attack.go)
	roundWaits bool
	// swingTimer is CLIENT's own swing timer as the server follows it
	// (VIClient m_p071E8): when the client's next swing is due, a round's
	// start and its delay, 2 s after a swing out of reach; kept over a
	// disengage, as the client's is (attack.go pressed)
	swingTimer time.Time

	// The melee round in hand (attack.go): when it began and how many of its
	// hits have come due.
	RoundStart time.Time
	// what a limited proc has done in the round of procRound (procsOfRound)
	procRound     time.Time
	procRoundDone int
	RoundHits     int

	// ProfanityFilterOff: the player turned the profanity filter off
	// (handlers/profanity.go: opcode 0x0045, /filter, or its client's setting
	// by OpcodeProfanityChoice); it is on at every login.
	ProfanityFilterOff bool
	// ProfanityFilterMedium: its client's setting is Medium (petition 1625):
	// only racial slurs masked, not swears. Read only while the filter is on
	// (ProfanityFilterOff false); High (every word) otherwise.
	ProfanityFilterMedium bool

	// PowerBars: the player's client draws others' power bars and asked
	// for their power (powerbars.go, opcode 0x7E01); powerBarsSent is the
	// set it was last sent, so only a change goes out, and powerBarsDue
	// sends the next one whatever it is (just asked).
	PowerBars     bool
	powerBarsSent string
	powerBarsDue  bool

	// RaidRoster: the player's client shows a whole raid group and asked for it
	// (raidroster.go, opcodes 0x7E18-0x7E1A); raidRosterSent is what it was last
	// sent, so only a change goes out.
	RaidRoster     bool
	raidRosterSent string
	// RaidRosterGroups: it answered the offer RaidRosterWithGroups, each record
	// with its group number after it (raidsubgroups.go)
	RaidRosterGroups bool
	// RaidRosterKinds: it answered RaidRosterWithKinds, each record's kind (a
	// merc's or a player's) after its group number (raidroster.go)
	RaidRosterKinds bool

	// RaidWindows: the player's client shows the raid's windows and answered the
	// raid offer (raidrules.go, opcodes 0x7E42-0x7E48): its raid's state, its rolls
	// and the master looter's list; raidStateSent is the state it was last sent.
	RaidWindows   bool
	raidStateSent string

	// ReadyWindows: the player's client shows a ready check's window and answered its
	// offer (readycheck.go, opcodes 0x7E49-0x7E4B, 0x7E56); else it is told in a line.
	ReadyWindows bool

	// PetEffects: the player's client draws its pet's effects under the pet's
	// bar and asked for them (peteffects.go, opcodes 0x7E32 and 0x7E33);
	// petEffectsSent is what made the set it was last sent (each effect's
	// icon, name and end, not its time left), so only a change goes out, and
	// petEffectsDue sends the next one whatever it is (just asked).
	PetEffects     bool
	petEffectsSent string
	petEffectsDue  bool

	// BuffWindow: the player's client draws its own buff window and asked for its set
	// (buffwindow.go, opcodes 0x7E57-0x7E59); buffWindowSent is what made the set it was
	// last sent, buffWindowDue sends the next whatever it is (just asked).
	BuffWindow     bool
	buffWindowSent string
	buffWindowDue  bool

	// QuestHeadMarks: the player's client draws the quest marks over the
	// NPCs' heads and asked for them (questheadmarks.go, opcodes
	// 0x7E61-0x7E63); questHeadSent is the set it was last sent,
	// questHeadDue sends the next whatever it is (just asked).
	QuestHeadMarks bool
	questHeadSent  string
	questHeadDue   bool

	// dps: the player's client draws a damage meter and asked for it
	// (dpsmeter.go, opcodes 0x7E3A-0x7E3D): its fight as that client is
	// shown it; nil when it never asked or stopped
	dps *dpsMeter
	// dpsSolo: the damage meter's fight of the player alone, in no group
	// (dpsmeter.go; a group's is Group.dps), counted whatever its client draws
	dpsSolo *dpsFight

	// DuelWindow: the player's client has the live game's duel window and
	// answered the duel offer (duel.go, opcode 0x7E09): its duels go by
	// 0x1501/0x1502, not boxes, and its opponent stays a player to it
	DuelWindow bool

	// ctf: its part in an arena game's match, capture the flag's or the siege's (ctf.go, siege.go)
	ctf ctfPlayer

	// pvp: its PvP flag and what goes with it (pvp.go); targetToken its last
	// 0x0010's token, for a target's face told unasked
	pvp         pvpFlag
	targetToken uint32

	// RaceLabels: the player's client shows a race of racedefs' own words and
	// asked for them (racelabels.go, opcode 0x7E1C); raceLabelsSent is the list
	// it was last sent, so only a change goes out ("" unknown, after a restart).
	RaceLabels     bool
	raceLabelsSent string

	// RecordWindow: the player's client has a combat record window and answered
	// the offer (combatrecord.go, opcode 0x7E20): its record is sent to it when it
	// changes. record is what its target window shows of it (TargetInfoLines),
	// read at the login and after each count.
	RecordWindow bool
	record       RecordSummary

	// Breath / underwater
	Breath          int    // 0-255 (255 full), as the dump and stat channel byte 232 carry it; 32 every 6 s under liquid
	Underwater      bool   // under a liquid's surface (UnderLiquid on WalkFlags)
	DrowningCounter int    // ticks under liquid since the last 6 s step (breath, then drowning)
	WalkFlags       uint32 // the last 0x40's walker flags (clientupdate.go)
	HasWalkFlags    bool

	// mv is the server's own view of the player's moves (movecheck.go):
	// the place last taken from its 0x40, the speed in hand, the fall
	mv moveState
	// orb is its standing with the teleport orbs (orbs.go): the orb it is
	// in, its place at the last report, its orbs' rest
	orb orbWatch
	// hazardNext is when a hazard may hit it again (hazards.go); hazardX/hazardZ where its last
	// hit took it, while hazardHit (an on_move hazard's still player is not hit again)
	hazardNext       time.Time
	hazardX, hazardZ float32
	hazardHit        bool

	// Stood is the last place the player was known to stand, on a floor or
	// in a liquid (clientupdate.go's Standing): where it arrived (a login, a
	// server-side move, a respawn) or a 0x40 whose walker flags say so;
	// StoodWorld its world. A logout in the air saves it (SavePlace).
	StoodX, StoodY, StoodZ float32
	StoodWorld             uint8
	HasStood               bool

	// Group (group.go): the group's id, 0 for none
	GroupID uint32
	// raidReminder: its new client is to be told its group is a raid group (raid.go)
	raidReminder bool
	// LFG: registered in the live group search (lfg.go: 0x0801 sub 2, /lfg
	// on), as a character looking for a group or, leading one, as a group
	// looking for members; it lasts until unregistered or the logout.
	LFG bool
	// GroupChatHidden: the Group menu's Hide Chat (lfg.go: the disc's
	// 0x0B01, the live 0x0801 sub 1); its group's chat is not sent to it.
	GroupChatHidden bool

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
	// artReady is when the combat arts' global cooldown is over (balance.go)
	artReady time.Time
	// potionReady is when each item with a cooldown may be used again, by
	// item (potion.go: itemeffect.cooldown), from the login's database row on
	potionReady map[int]time.Time
	// recoveryAnswered is when a cast into its own recovery was last
	// answered (cast.go recoveryAnswerLocked)
	recoveryAnswered time.Time
	// heldToldAt is when a press with a held item and no weapon was last
	// refused aloud (heldRefusalLocked).
	heldToldAt time.Time

	// StatsDirty sends the stat and effect channels on the next tick, not
	// at the next 2 s round: a cast's cost goes out with its start.
	StatsDirty bool

	// What the GM's TEST menu sets (gmchannel.go), until the next login: the
	// GM's own invisibility (no effect, so nothing breaks it), the body it
	// shows (0 its own), and a player frozen where it stands.
	GMHidden   bool
	GMForm     uint32
	GMFormSize float32
	Frozen     bool
	// GMTestMode is a GM's #testmode: every hit taken and shown in full, and no
	// death (a would-be death leaves 1 HP); GMTankProfile its #tankprofile1-3
	// (0: none): AGI 400 and AC 1000 x the profile, whatever it wears
	// (gmchannel.go; the GM Jibrish's petition 1272). Neither is kept past the
	// session.
	GMTestMode    bool
	GMTankProfile int
	// metalSpared is when the duel Metal Moogie won stops holding the player at
	// 1 HP (metalmoogie.go): what was on its way from him when it ended (a proc,
	// a shield's answer) kills no one.
	metalSpared time.Time

	// Flying: a GM's #fly is on (fly.go), until #fly off or the next login.
	// flyGrace: its last few reports on the way down from it (fly.go).
	Flying   bool
	flyGrace time.Time
	// Noclip: a GM's /noclip is on (noclip.go): flying, through every wall, floor
	// and ceiling, until /noclip again, #fly off or the next login
	Noclip bool

	// Jumps: the player's client jumps and answered the jump offer (jump.go,
	// opcode 0x7E28, world.jump): the movement check takes a jump's arc from it.
	// A new client's answer says again (a takeover clears it).
	Jumps bool

	// Vampire: the sun burns it by day (vampire.go; table charvampire, read
	// at its login, set by #vampire and the GM panel where it stands). GM:
	// its account was a GM's at its login (acctlevel 1 or more), which
	// VampireSun.GMs may spare.
	Vampire bool
	GM      bool
	// HideGMTag: a GM that hid its [GM] tag (/hide gmtag, handlers/hidegmtag.go):
	// its name is drawn bare, as a player's; it is a GM as before
	HideGMTag bool
	// ArmorBody is its race's body for the armour on its chest, 0 its own (armorbody.go)
	ArmorBody uint32
	// stuckNext is when /stuck may be used again (stuck.go)
	stuckNext time.Time
	// skyFallSince is when it began to be lost under the Plane of Sky's
	// islands (skyfall.go), zero when it is not
	skyFallSince time.Time
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
	// claude-notes/memcard-world-loading.md. A coach route's is its coachman's
	// world, nearly always the one you stand in (Oggok to Dark Solace is not);
	// a Gatekeeper/Harbormaster route sets it too, and 8 is what reaches guk
	// through the card loader.
	World uint8

	// Fee is the tunar the trip costs (a waysender's; 0 free), MinLevel the
	// level it needs (0 any), Via the NPC who sends the player.
	Fee      int
	MinLevel int
	Via      string

	// Ledgers marks an entry of a coachman's menu that is no trip: his offer
	// to sign the player into every stable's ledger at once (handlers/
	// coachledger.go), which asks Yes or No when picked.
	Ledgers bool
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
