package handlers

import (
	"log"
	"math/rand"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/eqoa/pkg/pkg/buffer"
	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// Random name components for name generation
var (
	namePrefixes = []string{
		"Al", "Ar", "Bel", "Car", "Dal", "El", "Fen", "Gar", "Hal", "Ith",
		"Jor", "Kel", "Lor", "Mal", "Nar", "Or", "Pel", "Quin", "Ral", "Sel",
		"Tal", "Ul", "Val", "Wil", "Xan", "Yor", "Zan",
	}
	nameSuffixes = []string{
		"ador", "anus", "ard", "ath", "dor", "eon", "eth", "ian", "iel", "ion",
		"ius", "las", "lin", "mon", "nar", "or", "ric", "ron", "thas", "wen",
	}
)

// Race constants (matching PS2 client values)
// Note: eqoa.live uses different ordering (Erudite=5, Troll=6, Barbarian=8)
// but we keep the client's ordering for compatibility
type Race int

const (
	RaceHuman     Race = 0
	RaceElf       Race = 1
	RaceDarkElf   Race = 2
	RaceGnome     Race = 3
	RaceDwarf     Race = 4
	RaceTroll     Race = 5 // Client ordering
	RaceBarbarian Race = 6 // Client ordering
	RaceHalfling  Race = 7
	RaceErudite   Race = 8 // Client ordering
	RaceOgre      Race = 9
	// RaceFroglok occupies gRaces[10], the first of the eleven race slots retail
	// declares but never fills. Requires the S2 race patch on the client
	// (re-tools/extend_s2_race.py); an unpatched client has no menu row for it
	// and reads slot 10 as all -1.
	RaceFroglok Race = 10
)

// allShippedRacesMask is the "any race" idiom in the item data: bits 0-9, one
// per race SOE shipped. itempattern.raceuse is a bitmask indexed by race id —
// 7329 of the 7874 patterns in the C# reference dump (EQOA_Master.sql) carry
// exactly this value, and the single-bit values elsewhere in that dump read
// correctly by race (1 = Human "Spiritrend", 4 = DarkElf "dark cloth pants",
// 64 = Barbarian "shepherd's mantle", 256 = Erudite "recruit's belt").
const allShippedRacesMask = 0x3FF

// addedRacesMask is one bit per race added past SOE's ten. Extend it when a new
// race lands; nothing else about the widening below has to change.
const addedRacesMask = 1 << uint(RaceFroglok)

// raceUseForWire widens an item's race mask to cover the races we add.
//
// A pattern that names every shipped race means "any race", so a character of a
// race that did not exist when the data was authored must be able to use it.
// Without this, 38 of the 81 patterns currently in eqoa.db — and 7329 of the
// 7874 in the reference dump — are closed to every race past 9, which reads
// in-game as an item the character inexplicably cannot equip.
//
// Deliberately NOT a blanket `mask | addedRacesMask`: a mask that omits even one
// shipped race is a genuine restriction and must stay closed (991 omits Troll,
// 271 is a caster-only set, 1 is Human-only), and widening those would hand out
// items SOE gated on purpose. Masks that already carry the added bits, and the
// 31 patterns at 0, pass through untouched.
//
// Applied at the wire write rather than in the table so it survives a re-import
// of the reference data, where all 7329 rows come back as 1023.
func raceUseForWire(mask int) int {
	if mask&allShippedRacesMask == allShippedRacesMask {
		return mask | addedRacesMask
	}
	return mask
}

// humanTypeEast is the "Humans of the east" city index. IsValidRaceClassCityCombo
// hardcodes class mask 0x7E69 for it, and city is a human-only concept — every
// other race reads its own mask and ignores the field.
const humanTypeEast = 1

// FrogloklModelID is the CHAR.ESF CSprite the froglok renders as in-world. There
// is no female froglok, so both sexes use it. It is deliberately NOT in any
// CHARSEL container, so the character create/select screens cannot draw it —
// stage 1 accepts a human stand-in on those two screens. See
// claude-notes/playable-race-system.md.
const FrogloklModelID int64 = 0x141BFE0C

// Class constants (matching eqoa.live scp.js)
type Class int

const (
	ClassWarrior      Class = 0
	ClassRanger       Class = 1
	ClassPaladin      Class = 2
	ClassShadowKnight Class = 3
	ClassMonk         Class = 4
	ClassBard         Class = 5
	ClassRogue        Class = 6
	ClassDruid        Class = 7
	ClassShaman       Class = 8
	ClassCleric       Class = 9
	ClassMagician     Class = 10
	ClassNecromancer  Class = 11
	ClassEnchanter    Class = 12
	ClassWizard       Class = 13
	ClassAlchemist    Class = 14
)

// Gender constants
type Gender int

const (
	GenderMale   Gender = 0
	GenderFemale Gender = 1
)

// swapBytes swaps the byte order of a uint32 (for big-endian color values)
func swapBytes(val uint32) uint32 {
	return ((val & 0xFF) << 24) | ((val & 0xFF00) << 8) | ((val & 0xFF0000) >> 8) | ((val & 0xFF000000) >> 24)
}

// CharacterHandler handles character-related messages.
type CharacterHandler struct {
	store          *database.SQLiteCharacterStore
	sessionManager *session.Manager
	gameWorld      *entity.World
	inventoryStore *database.InventoryStore
	spellbookStore *database.SpellbookStore
	defaults       *database.DefaultCharacterStore
}

// NewCharacterHandler creates a new character handler with SQLite persistence.
func NewCharacterHandler(store *database.SQLiteCharacterStore, sessionManager *session.Manager, invStore *database.InventoryStore, spellStore *database.SpellbookStore) *CharacterHandler {
	h := &CharacterHandler{
		store:          store,
		sessionManager: sessionManager,
		inventoryStore: invStore,
		spellbookStore: spellStore,
		defaults:       database.NewDefaultCharacterStore(store.DB()),
	}

	// Log existing characters
	count, err := store.Count()
	if err != nil {
		log.Printf("CharacterHandler: failed to count characters: %v", err)
	} else {
		log.Printf("CharacterHandler initialized with SQLite database (%d existing characters)", count)
	}

	return h
}

// SetGameWorld sets the game world for entity management.
//
// No OnEntityEnter callback is registered: entities are created purely by the
// state channels. FullEntityCreate (0x00B3) used to be sent for every entity
// coming into range, but no real server sends it — 0 occurrences across all
// five PS2 captures, against 173 in one session of ours. The retail client
// spawns everything it sees from the 28 state channels alone, and the Go client
// does too (it has only ever been tested against a server that never sends
// 0x00B3).
func (h *CharacterHandler) SetGameWorld(gw *entity.World) {
	h.gameWorld = gw
}

// CharacterCount returns the number of characters stored
func (h *CharacterHandler) CharacterCount() int {
	count, _ := h.store.Count()
	return count
}

// ListCharacters logs all stored characters (for debugging)
func (h *CharacterHandler) ListCharacters() {
	chars, err := h.store.GetAll()
	if err != nil {
		log.Printf("=== Error listing characters: %v ===", err)
		return
	}
	log.Printf("=== Stored Characters (%d) ===", len(chars))
	for _, c := range chars {
		log.Printf("  ServerID=%d: '%s' Level %d Race=%d Class=%d", c.ServerID, c.CharName, c.Level, c.Race, c.TClass)
	}
}

// RegisterHandlers registers character handlers with the registry.
func (h *CharacterHandler) RegisterHandlers(r *Registry) {
	r.Register(protocol.OpcodeCreateCharacter, h.HandleCreateCharacter)
	r.Register(protocol.OpcodeSelectedChar, h.HandleSelectedChar)
	r.Register(protocol.OpcodeRandomName, h.HandleRandomName)
	r.Register(protocol.OpcodeCharacterInWorld, h.HandleCharacterInWorld)
	r.Register(protocol.OpcodeDelCharacter, h.HandleDelCharacter)
}

// HandleCreateCharacter handles character creation requests (opcode 0x2B).
func (h *CharacterHandler) HandleCreateCharacter(sess *session.Session, msg *protocol.Message) error {
	data := msg.Data()
	log.Printf("CreateCharacter: received %d bytes from %s", len(data), sess.RemoteAddr())
	log.Printf("CreateCharacter: hex dump: %x", data)

	if len(data) < 4 {
		log.Printf("CreateCharacter: insufficient data from %s", sess.RemoteAddr())
		return nil
	}

	reader := buffer.NewReader(data)

	// Read name length (int32)
	nameLen, err := reader.ReadInt32()
	if err != nil {
		log.Printf("CreateCharacter: failed to read name length: %v", err)
		return nil
	}

	if nameLen <= 0 || nameLen > 24 {
		log.Printf("CreateCharacter: invalid name length %d", nameLen)
		h.SendNameTaken(sess)
		return nil
	}

	// Read character name
	name, err := reader.ReadStringUTF8(int(nameLen))
	if err != nil {
		log.Printf("CreateCharacter: failed to read name: %v", err)
		return nil
	}

	log.Printf("CreateCharacter: name='%s' (len=%d)", name, nameLen)

	// Validate name
	if !h.isValidName(name) {
		log.Printf("CreateCharacter: invalid name '%s'", name)
		h.SendNameTaken(sess)
		return nil
	}

	// Check if name is already taken (in database)
	exists, err := h.store.NameExists(name)
	if err != nil {
		log.Printf("CreateCharacter: database error checking name: %v", err)
		h.SendNameTaken(sess)
		return nil
	}
	if exists {
		count, _ := h.store.Count()
		log.Printf("CreateCharacter: name '%s' already taken (database has %d characters)", name, count)
		h.SendNameTaken(sess)
		return nil
	}

	// Read character attributes with ZIGZAG decoding (client sends zigzag)
	// Race indices now match eqoa.live (no mapping needed)
	level, _ := reader.Read7BitEncodedInt64()
	race, _ := reader.Read7BitEncodedInt64()
	class, _ := reader.Read7BitEncodedInt64()
	gender, _ := reader.Read7BitEncodedInt64()
	hairColor, _ := reader.Read7BitEncodedInt64()
	hairLength, _ := reader.Read7BitEncodedInt64()
	hairStyle, _ := reader.Read7BitEncodedInt64()
	faceOption, _ := reader.Read7BitEncodedInt64()
	humanType, _ := reader.Read7BitEncodedInt64()

	log.Printf("CreateCharacter: level=%d race=%d class=%d gender=%d", level, race, class, gender)
	log.Printf("CreateCharacter: hair(color=%d len=%d style=%d) face=%d humanType=%d",
		hairColor, hairLength, hairStyle, faceOption, humanType)

	// Read stats (int32 each)
	strength, _ := reader.ReadInt32()
	stamina, _ := reader.ReadInt32()
	agility, _ := reader.ReadInt32()
	dexterity, _ := reader.ReadInt32()
	wisdom, _ := reader.ReadInt32()
	intelligence, _ := reader.ReadInt32()
	charisma, _ := reader.ReadInt32()

	log.Printf("CreateCharacter: stats STR=%d STA=%d AGI=%d DEX=%d WIS=%d INT=%d CHA=%d",
		strength, stamina, agility, dexterity, wisdom, intelligence, charisma)

	// Create character record for database
	accountID := sess.AccountID()
	if accountID == 0 {
		log.Printf("CreateCharacter: no account ID set for session, using default account 1")
		accountID = 1 // Fallback to account 1 if not set
	}

	// PS2 limit: 8 characters per account
	charCount, _ := h.store.CountByAccount(uint32(accountID))
	if charCount >= 8 {
		log.Printf("CreateCharacter: account %d already has %d characters (max 8)", accountID, charCount)
		h.SendNameTaken(sess) // reuse NameTaken response to signal rejection
		return nil
	}
	record := &database.CharacterRecord{
		AccountID:    accountID,
		CharName:     name,
		Level:        int(level),
		Race:         int(race),
		TClass:       int(class),
		Sex:          int(gender),
		HairColor:    int(hairColor),
		HairLength:   int(hairLength),
		HairStyle:    int(hairStyle),
		FaceOption:   int(faceOption),
		HumType:      int(humanType),
		ModelID:      h.getModelID(Race(race), Gender(gender)),
		TPStrength:   int(strength),
		TPStamina:    int(stamina),
		TPAgility:    int(agility),
		TPDexterity:  int(dexterity),
		TPWisdom:     int(wisdom),
		TPIntel:      int(intelligence),
		TPCharisma:   int(charisma),
		CurrentHP:    100,
		CurrentPower: 100,
		Breath:       255,
		Speed:        8.0,
		UnusedTP:     20 - int(strength+stamina+agility+dexterity+wisdom+intelligence+charisma),
		TotalTP:      20,
	}

	h.applyFrogloklSpawn(record)

	// Save to database
	if err := h.store.Create(record); err != nil {
		if err == database.ErrCharacterExists {
			log.Printf("CreateCharacter: name '%s' already exists in database", name)
			h.SendNameTaken(sess)
			return nil
		}
		log.Printf("CreateCharacter: failed to save character: %v", err)
		h.SendNameTaken(sess)
		return nil
	}

	count, _ := h.store.Count()
	log.Printf("*** CHARACTER CREATED: '%s' (ServerID=%d, Level %d Race=%d Class=%d) - Total chars: %d ***",
		name, record.ServerID, record.Level, record.Race, record.TClass, count)

	// Don't create entity here — player entities are created when entering the world
	// (CreatePlayer in HandleSelectedChar). Creating them at character creation would
	// leave ghost entities in the spatial index that consume NPC render slots.

	// Send updated character list
	sess.Queue().SetBodyFlags(protocol.BodyFlagGuaranteedAck)
	h.SendCharacterSelectEQOA(sess)

	return nil
}

// applyFrogloklSpawn gives a newly created froglok Human East's start point.
//
// Scoped to race 10 on purpose. The defaultcharacter table is loaded at startup
// but GetDefaults has no callers, so today EVERY race is created at (0,0,0) and
// world 0; switching that on for all ten existing races is a separate change
// with its own blast radius. Race 10 has no rows of its own and the whole point
// of stage 1 is that it borrows Human East, so it reads race 0 / humType 1.
func (h *CharacterHandler) applyFrogloklSpawn(record *database.CharacterRecord) {
	if record.Race != int(RaceFroglok) || h.defaults == nil {
		return
	}
	d := h.defaults.GetDefaults(record.TClass, int(RaceHuman), humanTypeEast)
	if d == nil {
		log.Printf("CreateCharacter: no defaultcharacter row for Human East class %d; "+
			"froglok '%s' spawns at the origin", record.TClass, record.CharName)
		return
	}
	// Human East is missing a Cleric (class 9) row, so GetDefaults' fallback can
	// hand back a Western row. Say so rather than spawning silently in the wrong city.
	if d.HumType != humanTypeEast {
		log.Printf("CreateCharacter: Human East has no class %d row; froglok '%s' "+
			"falls back to humType %d", record.TClass, record.CharName, d.HumType)
	}
	record.X, record.Y, record.Z = d.X, d.Y, d.Z
	record.Facing = d.Facing
	record.World = d.World
	log.Printf("CreateCharacter: froglok '%s' spawning at Human East (%.1f, %.1f, %.1f) world %d",
		record.CharName, d.X, d.Y, d.Z, d.World)
}

// getModelID returns the model ID for a race/gender combination
// Values from eqoa.live charactermodel table (already uses client race values)
func (h *CharacterHandler) getModelID(race Race, gender Gender) int64 {
	// Male model IDs (from charactermodel table: sex=0)
	// Stored as int64 to match C# bigint column
	maleModels := map[Race]int64{
		RaceHuman:     1893243078,  // 0x70D898C6
		RaceElf:       -657100808,  // 0xD8D56FF8
		RaceDarkElf:   2128125354,  // 0x7ED89DAA
		RaceGnome:     -1449366763, // 0xA99C6B15
		RaceDwarf:     -1436875705, // 0xAA5B0447
		RaceTroll:     1282385202,  // 0x4C6FA532
		RaceBarbarian: 1640644319,  // 0x61CA3EDF
		RaceHalfling:  -1545912350, // 0xA3DB3FE2
		RaceErudite:   839650119,   // 0x320C0B47
		RaceOgre:      -2071956336, // 0x84807490
		RaceFroglok:   FrogloklModelID,
	}

	// Female model IDs (from charactermodel table: sex=1)
	femaleModels := map[Race]int64{
		RaceHuman:     223572789,       // 0x0D537335
		RaceElf:       -511108617,      // 0xE18919F7
		RaceDarkElf:   -1935979719,     // 0x8C9B4B39
		RaceGnome:     1279296764,      // 0x4C4084FC
		RaceDwarf:     889978302,       // 0x350BFDBE
		RaceTroll:     1306732289,      // 0x4DE32701
		RaceBarbarian: -1001728746,     // 0xC44AD516
		RaceHalfling:  -1977112783,     // 0x8A27A731
		RaceErudite:   1327119550,      // 0x4F1A3CBE
		RaceOgre:      -1396700337,     // 0xACC00B4F
		RaceFroglok:   FrogloklModelID, // no female froglok exists
	}

	if gender == GenderFemale {
		if model, ok := femaleModels[race]; ok {
			return model
		}
	} else {
		if model, ok := maleModels[race]; ok {
			return model
		}
	}
	return 1893243078 // Default: male human model (0x70D898C6)
}

// HandleSelectedChar handles character selection (opcode 0x2A).
func (h *CharacterHandler) HandleSelectedChar(sess *session.Session, msg *protocol.Message) error {
	data := msg.Data()
	log.Printf("SelectedChar: received %d bytes from %s", len(data), sess.RemoteAddr())

	// Prevent duplicate processing - only send MemoryDump once
	if sess.InGame() {
		log.Printf("SelectedChar: already in game, just ACKing (duplicate packet)")
		sess.Queue().SetBodyFlags(protocol.BodyFlagGuaranteedAck)
		return nil
	}

	if len(data) < 4 {
		log.Printf("SelectedChar: insufficient data from %s", sess.RemoteAddr())
		return nil
	}

	reader := buffer.NewReader(data)
	charID, _ := reader.ReadUint32()

	log.Printf("SelectedChar: serverID=%d from %s", charID, sess.RemoteAddr())

	// Look up character by server_id (client sends server_id, not database id)
	record, err := h.store.GetByServerID(charID)
	if err != nil {
		log.Printf("SelectedChar: character with server_id %d not found: %v", charID, err)
		sess.Queue().SetBodyFlags(protocol.BodyFlagGuaranteedAck)
		return nil
	}

	log.Printf("SelectedChar: selected '%s' (level %d race=%d class=%d)", record.CharName, record.Level, record.Race, record.TClass)

	// Promote to a master session HERE, not when the character list was sent.
	// Sandstorm's order (pcaps/ps2_sandstorm.pcap): character list at t=1.14s on
	// the still-ClientInit session, then ServerInit|NewInstance + sessionID at
	// t=4.61s — after a character is chosen — immediately followed by Camera1,
	// Time, and the MemoryDump fragments.
	//
	// Promotion clears the message queue and resets the sequence space, so it
	// must happen before anything for this world entry is queued.
	if h.sessionManager != nil && sess.PlayerObjectID() == 0 {
		playerObjectID := h.sessionManager.GeneratePlayerObjectID()
		log.Printf("SelectedChar: generated playerObjectID=%d (0x%X)", playerObjectID, playerObjectID)
		h.sessionManager.PromoteSessionToMaster(sess, playerObjectID)
		// Camera1 + Time are sent by SendMemoryDump below — queueing them here
		// too duplicated them, and with the wrong Camera1 value.
	}

	// Mark as in-game FIRST to prevent duplicate processing (matching working version)
	sess.SetState(session.StateInGame)
	sess.SetInGame(true)
	// Note: CharacterInWorld is set later in HandleCharacterInWorld after client confirms
	sess.Queue().SetBodyFlags(protocol.BodyFlagGuaranteedAck)

	// EVERY LOGIN STARTS IN FREEPORT, beside Archivist Vaal. The character's
	// saved position is overridden here, before CreatePlayer reads it, so this
	// is the one place a session's starting place is decided.
	loginSpawn(h.gameWorld, record)

	// Create player entity directly (replaces QueueCommandSync + CmdEnterWorld)
	var player *entity.Player
	if h.gameWorld != nil {
		player = h.gameWorld.CreatePlayer(sess, record)
	}

	// Load equipped items from DB and apply visual equipment to player
	if player != nil && h.inventoryStore != nil {
		h.applyEquipmentFromDB(player, record.ServerID)
	}

	// NOT sent: WorldEntry (0x002E). Sandstorm never sends it — zero
	// occurrences across all three PS2 captures. Its promoted session goes
	// straight from Camera1 + Time into the MemoryDump fragments.

	// Send memory dump reading all data from player struct (single source of truth)
	h.SendMemoryDump(sess)

	return nil
}

// HandleRandomName handles random name generation requests (opcode 0x12).
func (h *CharacterHandler) HandleRandomName(sess *session.Session, msg *protocol.Message) error {
	log.Printf("RandomName: request from %s", sess.RemoteAddr())

	// Generate random name
	name := h.generateRandomName()

	// Send response with same opcode
	response := protocol.NewMessage(protocol.MessageTypeReliable, protocol.OpcodeRandomName)
	writer := buffer.NewWriter(response.Span())

	// Write opcode in data (C# reference format)
	writer.WriteUint16(uint16(protocol.OpcodeRandomName))
	// Write name with length prefix (7-bit encoded length + UTF8 string)
	writer.Write7BitEncodedUInt64(uint64(len(name)))
	writer.WriteStringUTF8Raw(name)
	response.SetSize(writer.Position())

	sess.Queue().AddWithSequence(response)
	log.Printf("RandomName: sent '%s' to %s", name, sess.RemoteAddr())

	return nil
}

// HandleCharacterInWorld handles the character entering the world (opcode 0x14).
func (h *CharacterHandler) HandleCharacterInWorld(sess *session.Session, msg *protocol.Message) error {
	// Only process fully once - subsequent calls just need ACK
	if sess.CharacterInWorld() {
		// Client keeps sending this while waiting - just ACK
		sess.Queue().SetBodyFlags(protocol.BodyFlagGuaranteedAck)
		return nil
	}

	log.Printf("CharacterInWorld: first call from %s - entering world", sess.RemoteAddr())

	// Mark session as in-game
	sess.SetCharacterInWorld(true)
	sess.SetInGame(true)
	sess.SetState(session.StateInGame)

	// ACK the message
	sess.Queue().SetBodyFlags(protocol.BodyFlagGuaranteedAck)

	// Send another time sync now that we're in world
	h.SendTime(sess)

	// Send player speed (needed for movement)
	h.SendPlayerSpeed(sess, 20.0)

	// Send server ready signal (Sandstorm sends this after world entry)
	SendServerReady(sess)

	AutoHDD(h.gameWorld, sess)

	return nil
}

// loginSpawnDest is the #tele shortcut every login lands on. Naming the
// shortcut rather than three floats means the NPC row is the single source of
// truth for where players start, the same way it already is for #tele: moving
// Archivist Vaal moves the spawn with him, and `#tele vaal` and logging in
// cannot disagree about where "beside Vaal" is.
const loginSpawnDest = "vaal"

// loginSpawnOffset stands the player a few units off the anchor NPC rather than
// inside him. X and Z only: the ground around Freeport's plaza is flat at
// y=54.1 (Coachman Ronks sits at the same height 112 units away), so moving
// the height is how you get a spawn that falls.
const loginSpawnOffset = 8.0

// loginSpawn overwrites a character's saved position with the fixed start
// point, in place, before the player entity is built from the record.
//
// WHY EVERY LOGIN AND NOT JUST A NEW CHARACTER. A saved position is a thing
// that can strand a session: under terrain, in a world whose ESF is not on this
// console's disk, or at coordinates a PNACH wrote (see
// claude-notes/position-spoofing-fix.md). Logging in somewhere known-good makes
// every one of those recoverable by reconnecting, which is worth more here than
// resuming where a player left off.
//
// It replaces autoGoto/EQOA_AUTO_GOTO, which did the same job late — a timed GM
// teleport AFTER world entry, so the client first entered the saved world and
// then warped out of it.
//
// The record is NOT saved back: this changes where a session starts, not what
// the database holds. A character who walks somewhere and logs out still has
// that position on the row; it is simply not where the next login begins.
func loginSpawn(gw *entity.World, record *database.CharacterRecord) {
	if record == nil {
		return
	}
	x, y, z, world, via, ok := ResolveTeleDest(gw, loginSpawnDest)
	if !ok {
		// Only reachable if the shortcut is renamed out from under this, which
		// would otherwise spawn everyone at the origin — 25,000 units from
		// Freeport, under no terrain at all. Leave the saved position alone.
		log.Printf("loginSpawn: no %q destination; '%s' keeps its saved position "+
			"(%.1f, %.1f, %.1f) world=%d", loginSpawnDest, record.CharName,
			record.X, record.Y, record.Z, record.World)
		return
	}
	x += loginSpawnOffset
	z += loginSpawnOffset

	w, err := ValidateTeleport(x, y, z, world)
	if err != nil {
		log.Printf("loginSpawn: %s is not a placeable spawn (%v); '%s' keeps its "+
			"saved position", via, err, record.CharName)
		return
	}
	world = w

	log.Printf("loginSpawn: '%s' starts at %s (%.1f, %.1f, %.1f) world=%d "+
		"(saved was %.1f, %.1f, %.1f world=%d)", record.CharName, via,
		x, y, z, world, record.X, record.Y, record.Z, record.World)

	record.X, record.Y, record.Z = x, y, z
	record.World = int(world)
}

// HandleServerSelection handles server selection (opcode 0x2D).
func (h *CharacterHandler) HandleServerSelection(sess *session.Session, msg *protocol.Message) error {
	data := msg.Data()
	log.Printf("ServerSelection: received %d bytes from %s", len(data), sess.RemoteAddr())
	if len(data) > 0 {
		log.Printf("ServerSelection: data hex: %x", data)
	}

	// Set ack flags
	sess.Queue().SetBodyFlags(protocol.BodyFlagGuaranteedAck)

	// Only send character list if we haven't already
	if sess.State() >= session.StateCharacterSelect {
		log.Printf("ServerSelection: already in StateCharacterSelect or later, skipping character list resend")
		return nil
	}

	// Update session state
	sess.SetState(session.StateCharacterSelect)

	// Send character list
	h.SendCharacterSelectEQOA(sess)

	return nil
}

// applyEquipmentFromDB loads the character's equipped items and sets visual
// equipment bytes on the Player struct so the state channel reflects them.
func (h *CharacterHandler) applyEquipmentFromDB(player *entity.Player, serverID uint32) {
	items, err := h.inventoryStore.GetCharacterItems(serverID)
	if err != nil {
		log.Printf("applyEquipment: failed to load items for serverID=%d: %v", serverID, err)
		return
	}

	for _, item := range items {
		if item.EquipLoc < 0 {
			continue // not equipped
		}
		model := item.Pattern.Model
		switch item.Pattern.EquipSlot {
		case 1: // Head
			player.Helm = byte(model)
		case 5: // Chest
			player.Chest = byte(model)
		case 7: // Bracers
			player.Bracer = byte(model)
		case 10: // Legs
			player.Legs = byte(model)
		case 11: // Feet
			player.Boots = byte(model)
		case 19: // Hands/Gloves
			player.Gloves = byte(model)
		case 2: // Robe
			player.Robe = uint32(model)
		case 14: // Weapon
			if player.Primary == 0 {
				player.Primary = uint32(model)
			} else {
				player.Secondary = uint32(model)
			}
		case 13: // Shield
			player.Shield = uint32(model)
		}
	}

	log.Printf("applyEquipment: serverID=%d chest=%d bracer=%d gloves=%d legs=%d boots=%d helm=%d robe=%d primary=0x%08X secondary=0x%08X shield=0x%08X",
		serverID, player.Chest, player.Bracer, player.Gloves, player.Legs, player.Boots, player.Helm,
		player.Robe, player.Primary, player.Secondary, player.Shield)
}

// generateRandomName generates a random character name.
func (h *CharacterHandler) generateRandomName() string {
	prefix := namePrefixes[rand.Intn(len(namePrefixes))]
	suffix := nameSuffixes[rand.Intn(len(nameSuffixes))]
	return prefix + suffix
}

// isValidName checks if a name is valid.
func (h *CharacterHandler) isValidName(name string) bool {
	// Check length
	if len(name) < 3 || len(name) > 15 {
		return false
	}

	// Check for valid characters (letters only)
	for _, c := range name {
		if !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')) {
			return false
		}
	}

	// Check for reserved names
	reserved := []string{"admin", "gm", "moderator", "server", "system"}
	nameLower := strings.ToLower(name)
	for _, r := range reserved {
		if nameLower == r {
			return false
		}
	}

	return true
}

// HandleDelCharacter handles character deletion requests (opcode 0x2D).
// The client sends the character's ServerID as a zigzag varint.
// On success we send back a fresh 0x2C character list (same as Sandstorm).
func (h *CharacterHandler) HandleDelCharacter(sess *session.Session, msg *protocol.Message) error {
	reader := buffer.NewReader(msg.Data())
	serverIDRaw, err := reader.Read7BitEncodedInt64()
	if err != nil {
		log.Printf("DelCharacter: failed to read serverID from %s: %v", sess.RemoteAddr(), err)
		return nil
	}
	serverID := uint32(serverIDRaw)

	log.Printf("DelCharacter: request to delete serverID=%d from %s", serverID, sess.RemoteAddr())

	// Only allow deleting characters belonging to this account.
	record, err := h.store.GetByServerID(serverID)
	if err != nil || record == nil {
		log.Printf("DelCharacter: serverID=%d not found", serverID)
		h.SendCharacterSelectEQOA(sess)
		return nil
	}
	if record.AccountID != int(sess.AccountID()) {
		log.Printf("DelCharacter: serverID=%d belongs to account %d, not %d — rejected",
			serverID, record.AccountID, sess.AccountID())
		h.SendCharacterSelectEQOA(sess)
		return nil
	}

	if err := h.store.Delete(serverID); err != nil {
		log.Printf("DelCharacter: failed to delete serverID=%d: %v", serverID, err)
	} else {
		log.Printf("DelCharacter: deleted serverID=%d ('%s')", serverID, record.CharName)
	}

	h.SendCharacterSelectEQOA(sess)
	return nil
}

// SendNameTaken sends a name taken response (opcode 0x2F) and resends character list.
func (h *CharacterHandler) SendNameTaken(sess *session.Session) {
	msg := protocol.NewMessage(protocol.MessageTypeReliable, protocol.OpcodeNameTaken)
	writer := buffer.NewWriter(msg.Span())
	writer.WriteUint16(uint16(protocol.OpcodeNameTaken)) // Opcode in data
	msg.SetSize(writer.Position())
	sess.Queue().AddWithSequence(msg)
	log.Printf("Sent NameTaken to %s", sess.RemoteAddr())

	// Resend character list to keep client on character select screen
	h.SendCharacterSelectEQOA(sess)
}

// SendCharacterSelectEQOA sends the character list (opcode 0x2C).
func (h *CharacterHandler) SendCharacterSelectEQOA(sess *session.Session) {
	msg := protocol.NewMessage(protocol.MessageTypeReliable, protocol.OpcodeCharacterSelect)
	writer := buffer.NewWriter(msg.Span())

	// Write opcode in data (C# reference format: writer.Write(message.Opcode);)
	writer.WriteUint16(uint16(protocol.OpcodeCharacterSelect))

	// Get characters for this account from database
	var records []*database.CharacterRecord
	var err error
	accountID := sess.AccountID()

	log.Printf("SendCharacterSelectEQOA: session accountID=%d from %s", accountID, sess.RemoteAddr())

	if accountID > 0 {
		records, err = h.store.GetByAccountID(accountID)
		if err != nil {
			log.Printf("SendCharacterSelectEQOA: failed to get characters for account %d: %v", accountID, err)
			records = []*database.CharacterRecord{}
		}
		log.Printf("SendCharacterSelectEQOA: found %d characters for account %d", len(records), accountID)
		for i, r := range records {
			log.Printf("SendCharacterSelectEQOA:   [%d] ServerID=%d Name='%s' Race=%d Class=%d ModelID=0x%X", i, r.ServerID, r.CharName, r.Race, r.TClass, r.ModelID)
		}
	} else {
		// No account - show all characters (legacy behavior)
		log.Printf("SendCharacterSelectEQOA: WARNING - no accountID set, showing ALL characters (legacy mode)")
		records, err = h.store.GetAll()
		if err != nil {
			log.Printf("SendCharacterSelectEQOA: failed to get characters: %v", err)
			records = []*database.CharacterRecord{}
		}
		log.Printf("SendCharacterSelectEQOA: legacy mode returning %d characters", len(records))
	}

	// Character count is ZIGZAG, like every field below it. Verified against a
	// real Sandstorm capture (pcaps/ps2_sandstorm.pcap): 7 characters are sent
	// with a count byte of 0x0E = zigzag(7). NOTE: CLAUDE.md and MEMORY.md both
	// claim this field is UNSIGNED — that is wrong, and "fixing" it to unsigned
	// makes the PS2 see half the characters.
	writer.Write7BitEncodedInt64(int64(len(records)))

	for _, c := range records {
		// Name (int32 length prefix + UTF-8 string)
		writer.WriteInt32(int32(len(c.CharName)))
		writer.WriteStringUTF8Raw(c.CharName)

		// Use zigzag encoding for character fields (matches C# Write7BitEncodedInt64)
		// ModelID is int64 from C# bigint column, cast to int32 for zigzag encoding
		writer.Write7BitEncodedInt64(int64(c.ServerID))
		writer.Write7BitEncodedInt64(int64(int32(c.ModelID)))
		writer.Write7BitEncodedInt64(int64(c.TClass))
		writer.Write7BitEncodedInt64(int64(c.Race))
		writer.Write7BitEncodedInt64(int64(c.Level))

		// Appearance fields - zigzag
		writer.Write7BitEncodedInt64(int64(c.HairColor))
		writer.Write7BitEncodedInt64(int64(c.HairLength))
		writer.Write7BitEncodedInt64(int64(c.HairStyle))
		writer.Write7BitEncodedInt64(int64(c.FaceOption))

		// Equipment block — 32 bytes exactly.
		// PS2 func_00BD2E80 reads:
		//   raw(4,1)  robeModel         [0:4]   uint32, 0xFFFFFFFF = none
		//   raw(1,1)  robeTint          [4]     uint8
		//   raw(4,3)  weapons           [5:17]  3× uint32 (primary, secondary, shield)
		//   raw(1,3)  unknown           [17:20] 3 bytes
		//   raw(1,6)  armorSlots        [20:26] chest,bracer,gloves,legs,boots,helm (0xFF=none)
		//   raw(1,6)  tints             [26:32] per-slot tint index
		writer.WriteUint32(0xFFFFFFFF) // robeModel (none)
		writer.WriteByte(0)            // robeTint
		writer.WriteUint32(0)          // primaryWeapon
		writer.WriteUint32(0)          // secondaryWeapon
		writer.WriteUint32(0)          // shield
		writer.WriteByte(0)            // unknown
		writer.WriteByte(0)            // unknown
		writer.WriteByte(0)            // unknown
		writer.WriteByte(0xFF)         // chest (none)
		writer.WriteByte(0xFF)         // bracer (none)
		writer.WriteByte(0xFF)         // gloves (none)
		writer.WriteByte(0xFF)         // legs (none)
		writer.WriteByte(0xFF)         // boots (none)
		writer.WriteByte(0xFF)         // helm (none)
		writer.WriteByte(0)            // chest tint
		writer.WriteByte(0)            // bracer tint
		writer.WriteByte(0)            // gloves tint
		writer.WriteByte(0)            // legs tint
		writer.WriteByte(0)            // boots tint
		writer.WriteByte(0)            // helm tint
	}

	msg.SetSize(writer.Position())
	sess.Queue().AddWithSequence(msg)

	log.Printf("Sent character list (%d chars, %d bytes) to %s",
		len(records), writer.Position(), sess.RemoteAddr())
	log.Printf("CharacterList hex dump: %x", msg.Span()[:writer.Position()])
}

// worldPaths maps a world index to the esfPath the client loads it from.
//
// Indices and the first eight paths match the client's own world table, which
// Sandstorm's S2 relocated and our SD rebuilds at WorldCount entries; keeping
// the two in step matters because a Teleport reaches a world through its table
// entry while a login reaches it through this string. "hdd:" is the prefix the
// SD's VIFile hooks claim, routing that entry to pfs0: on the PS2 hard disk;
// every other path falls through to retail's disc loader.
//
// READ OFF THE SD WE ACTUALLY SERVE, not from intent:
//
//	[0..7]   S2's, verbatim — the disc worlds, except 6 (unused) and
//	         7, which is a second copy of 5
//	[8..29]  ours, named by CONVENTION: slot N is "worldN" loading
//	         "hdd:worldN.ESF". init() in chat.go fills them, so this map and
//	         the SD's table cannot be edited apart.
//
// The convention is what makes a new world a file drop: put worldN.ESF in the
// server's worlds/ directory and slot N already points at it. The filename is
// the price — an ESF must be called worldN.ESF to be reachable at slot N.
//
// A SLOT IS NOT A WORLD. Every slot 8..29 names a file, and most of those files
// do not exist; entering one fails the client's open. worlds.Available reports
// which the server actually holds, and the startup log prints it.
//
// Guk was world 8 while worlds came off the MEMORY CARD ("mc:guk.esf", routed
// by re-tools/sd/guk_loader.S to VIMemCardReadFile). That route is retired —
// the SD we serve now hooks only "hdd:" — so an "mc:" path here would be handed
// to retail's disc loader, which cannot open it.
//
// A MISSING ENTRY IS NOT A HARMLESS GAP. worldPathFor falls back to Tunaria, so
// a world absent here silently logs a character back into world 0 at the
// coordinates they left the other world at — and where those coordinates are
// also legal in Tunaria (world 9 IS Tunaria zone 127, lifted), nothing
// downstream can tell. CoherentWorld cannot catch it either: the position is
// inside worldBounds[9], so the pair looks consistent. The observed symptom was
// Harbormaster Machado, a world 9 NPC, standing in world 0 — he passed the
// visibility filter because the SERVER still had the player in world 9 while
// the client had loaded tunaria.esf.
var worldPaths = map[uint8]string{
	0: "data\\tunaria.esf",
	1: "data2\\Rathe.esf",
	2: "data2\\Odus.esf",
	3: "data2\\Lavastm.esf",
	4: "data2\\Planesky.esf",
	5: "data3\\Secrets.esf",
	// 6 held "hdd:HDDSETUP.ESF", the setup SCREEN -- never a world, never
	// opened, a world change to it made the payload ExecPS2 EQSCENE.BIN. The
	// screen is gone, so the slot goes back to the convention every slot above
	// 7 already follows: an ordinary downloadable world. It is S2's dead
	// "Underfoot" entry underneath, which names an ESF that is on no disc, so
	// there was never anything to preserve here.
	6: "hdd:world6.ESF",
	7: "data3\\Secrets.esf",
	// 8..WorldCount-1 are added by init() in chat.go as hdd:worldN.ESF.
}

// worldPathFor returns the esfPath for a world index, falling back to Tunaria.
// The fallback is deliberate: an unknown index means a character row we cannot
// place, and dropping them in Tunaria is recoverable where sending a path the
// client cannot open is a dead session.
func worldPathFor(world uint8) string {
	if p, ok := worldPaths[world]; ok {
		return p
	}
	log.Printf("worldPathFor: unknown world %d, falling back to Tunaria", world)
	return worldPaths[0]
}

// SendMemoryDump sends the memory dump for entering the world (opcode 0x0D).
// Based on eqoa.live (Java) world entry sequence: Camera1(-1) → Ping+Time → MemoryDump.
// Reads all character data from the Player struct (single source of truth).
func (h *CharacterHandler) SendMemoryDump(sess *session.Session) {
	// Per C# reference: ObjectID = MySession.SessionID
	playerObjectID := sess.PlayerObjectID()
	if playerObjectID == 0 {
		playerObjectID = h.sessionManager.GeneratePlayerObjectID()
		sess.SetPlayerObjectID(playerObjectID)
		log.Printf("SendMemoryDump: WARNING - playerObjectID not set, generated %d", playerObjectID)
	}

	// Read all data from the Player struct
	gw := h.gameWorld
	if gw == nil {
		log.Printf("SendMemoryDump: ERROR - no game world")
		return
	}
	player := gw.SessionPlayer(sess)
	if player == nil {
		log.Printf("SendMemoryDump: ERROR - no player entity for session")
		return
	}

	log.Printf("SendMemoryDump: using playerObjectID=%d (0x%X) for '%s'", playerObjectID, playerObjectID, player.Name)

	// Send Camera1 (0x07D1) with value -1 (0xFFFFFFFF) before MemoryDump
	camMsg := protocol.NewMessage(protocol.MessageTypeReliable, protocol.OpcodeCamera1)
	camWriter := buffer.NewWriter(camMsg.Span())
	camWriter.WriteUint16(uint16(protocol.OpcodeCamera1))
	camWriter.WriteInt32(-1)
	camMsg.SetSize(camWriter.Position())
	sess.Queue().AddWithSequence(camMsg)
	log.Printf("SendMemoryDump: sent Camera1 with value -1 (world entry signal)")

	// Time only — no Ping. Sandstorm's first promoted packet is exactly
	// Camera1 at seq 1 and Time at seq 2; a Ping in between consumes a
	// sequence number and shifts the MemoryDump fragments by one.
	h.sendTimeOnly(sess)

	msg := protocol.NewMessage(protocol.MessageTypeReliable, protocol.OpcodeMemoryDump)
	// MemoryDump can be 30KB+ with items/spells/hotkeys; write to a large buffer then SetData
	dumpBuf := make([]byte, 65536)
	writer := buffer.NewWriter(dumpBuf)

	// Write opcode in data
	writer.WriteUint16(uint16(protocol.OpcodeMemoryDump))

	// === DumpCharacter section ===
	writer.WriteByte(0) // Unknown byte

	// The world file the client loads on entry.
	//
	// This is NOT decoration and it is NOT the world table: per
	// claude-notes/memcard-world-loading.md the load picks its path from one of
	// two sources, gated at client+0x662C — the login path sets the gate and
	// uses THIS string (copied to the override buffer at client+0xC1DC), while
	// a 0x00B2 Teleport leaves it clear and indexes the table instead. Both
	// converge on the same `jal ParseWorld` our SD hooks, so a "mc:" path here
	// loads off the memory card exactly as the teleport route does.
	//
	// Hardcoding tunaria was correct while world 0 was the only world; with a
	// player able to log out inside guk it would drop them at guk's
	// coordinates in Tunaria, i.e. under the map.
	worldPath := worldPathFor(player.World)
	writer.WriteInt32(int32(len(worldPath)))
	writer.WriteStringUTF8Raw(worldPath)
	if player.World != 0 {
		log.Printf("SendMemoryDump: world %d -> %q", player.World, worldPath)
	}

	// Character data (zigzag encoded) — all from Player struct
	writer.Write7BitEncodedInt64(int64(player.CharServerID))
	writer.WriteInt32(int32(len(player.Name)))
	writer.WriteStringUTF8Raw(player.Name)
	writer.Write7BitEncodedInt64(int64(player.Class))
	writer.Write7BitEncodedInt64(int64(player.Race))
	writer.Write7BitEncodedInt64(int64(player.Level))
	writer.Write7BitEncodedInt64(int64(player.XPInLevel))
	writer.Write7BitEncodedInt64(int64(player.Debt))
	writer.WriteByte(byte(player.Breath)) // Breath (255 = full)
	writer.Write7BitEncodedInt64(int64(player.Tunar))
	writer.Write7BitEncodedInt64(int64(player.BankTunar))
	writer.Write7BitEncodedInt64(int64(player.UnusedTP))
	writer.Write7BitEncodedInt64(int64(player.BaseMaxStat))

	// THE WORLD INDEX, and it must agree with the path string above.
	//
	// It was hardcoded 0 while world 0 was the only world, and stayed 0 after
	// the path string learned about the others — so a login into world 9 shipped
	// "hdd:world9.ESF" in one field and "I am in Tunaria" in the next. Measured
	// 2026-08-15: the client opened `data\tunaria.esf` and never touched the
	// HDD world at all (ST_INIT 0, ST_OPENS 0), which is the login-vs-teleport
	// asymmetry recorded in claude-notes/hdd-world-delivery.md — a teleport
	// carries the index and works, a login carried this zero.
	writer.Write7BitEncodedInt64(int64(player.World))

	// Position (floats) — from Player struct
	writer.WriteFloat32(player.X)
	writer.WriteFloat32(player.Y)
	writer.WriteFloat32(player.Z)
	writer.WriteFloat32(player.Facing)

	// Two unknown floats
	writer.WriteFloat32(0.0)
	writer.WriteFloat32(0.0)

	// === Hotkeys section ===
	writeDefaultHotkeys(writer)

	// Two unknown int32s after hotkeys
	writer.WriteInt32(0)
	writer.WriteInt32(0)

	// === Quests section ===
	writer.WriteUint32(0) // Quest count (uint32, not zigzag)

	// === Inventory + Bank sections ===
	// Load items from database (charinventory joined with itempattern)
	var invItems, bankItems []database.CharInventoryEntry
	if h.inventoryStore != nil {
		allItems, err := h.inventoryStore.GetCharacterItems(player.CharServerID)
		if err != nil {
			log.Printf("SendMemoryDump: inventory query error: %v", err)
		} else {
			for i := range allItems {
				if allItems[i].Location == 1 {
					bankItems = append(bankItems, allItems[i])
				} else {
					invItems = append(invItems, allItems[i])
				}
			}
		}
	}

	// Inventory (zigzag count + uint32 count + items)
	writer.Write7BitEncodedInt64(int64(len(invItems)))
	writer.WriteUint32(uint32(len(invItems)))
	for i := range invItems {
		writeSingleItem(writer, &invItems[i], i)
	}
	log.Printf("SendMemoryDump: wrote %d inventory items", len(invItems))
	// The client's inventory list as this dump leaves it (key = position,
	// counter = count): InteractItem's key resolves through it, and its reply
	// carries the position in THIS list.
	sess.SetInventoryList(newInvList(invItems))
	// Dump the list the client will index into, so any disagreement about
	// order shows up here.
	for i := range invItems {
		log.Printf("INVLIST[%02d] key=%-4d equiploc=%-3d slot=%-3d model=%-4d %q",
			i, invItems[i].ItemID, invItems[i].EquipLoc,
			invItems[i].Pattern.EquipSlot, invItems[i].Pattern.Model,
			invItems[i].Pattern.ItemName)
	}

	// NOTE: Sandstorm format has NO weapon hotbar section between inventory
	// and bank. Bank immediately follows inventory.

	// Bank (zigzag count + uint32 count + items)
	writer.Write7BitEncodedInt64(int64(len(bankItems)))
	writer.WriteUint32(uint32(len(bankItems)))
	for i := range bankItems {
		writeSingleItem(writer, &bankItems[i], i)
	}
	log.Printf("SendMemoryDump: wrote %d bank items", len(bankItems))

	writeMemoryDumpTail(writer, player)

	dumpData := dumpBuf[:writer.Position()]
	// Debug: EQOA_DUMP_MEMDUMP=<path> writes the payload (after the opcode) to
	// disc so it can be diffed against a captured retail dump.
	if path := os.Getenv("EQOA_DUMP_MEMDUMP"); path != "" {
		if err := os.WriteFile(path, dumpData[2:], 0o644); err != nil {
			log.Printf("SendMemoryDump: debug write %s: %v", path, err)
		} else {
			log.Printf("SendMemoryDump: debug wrote %d payload bytes to %s", len(dumpData)-2, path)
		}
	}
	msg.SetData(dumpData)
	sess.Queue().AddWithSequence(msg)

	log.Printf("SendMemoryDump: sent %d bytes for '%s' to %s", writer.Position(), player.Name, sess.RemoteAddr())
	// Log first 80 bytes of MemoryDump payload (after opcode) for wire comparison
	logLen := len(dumpData) - 2 // skip opcode
	if logLen > 80 {
		logLen = 80
	}
	if logLen > 0 {
		log.Printf("SendMemoryDump: first %d payload bytes: %x", logLen, dumpData[2:2+logLen])
	}

	// Send ignore list and buddy list after memory dump (matches eqoa.live)
	h.SendIgnoreList(sess)
	h.SendBuddyList(sess)

	// Initialize state channel 0 from Player struct
	h.initPlayerStateChannel(sess, player)

	// Initialize stat channel (0x42) from Player struct
	h.initPlayerStatChannel(sess, player)

	// Initialize buff channel (0x43) from Player struct
	h.initPlayerBuffChannel(sess, player)

	// IMPORTANT: Generate and queue state channel updates immediately
	// The tick routine won't run until CharacterInWorld is true, but the client
	// needs state channels BEFORE it will send CharacterInWorld
	h.flushStateChannels(sess)
}

// weaponHotbarRows is the fixed 6x4 weapon-hotbar block the client reads with
// func_00BFD178. Every captured dump carries these exact values — identical
// across two different retail characters in three captures (ps2_sandstorm,
// ps2_main_capture, ps2_sandstorm_new_instance) — so they are markers for
// "no weapon set assigned", not per-character data.
var weaponHotbarRows = [6][4]uint32{
	{99, 1, 0, 98},
	{99, 1, 0, 98},
	{99, 1, 0, 98},
	{99, 1, 0, 98},
	{99, 99, 99, 98},
	{99, 99, 99, 98},
}

// memoryDumpTrailerLen is the run of zero bytes every captured dump ends with,
// past the last field the client is known to read. Sending them costs nothing
// and covers any trailing read this format description does not yet model —
// a dump that ends one byte early is thrown away whole.
const memoryDumpTrailerLen = 39

// writeMemoryDumpTail writes everything the client reads after the bank/equipment
// container, in the order func_00BD0630 reads it:
//
//	zigzag  spellCount            func_00BBCD10  (retail sends 0 — see below)
//	zigzag  buffCount             func_00BBD0A8
//	raw1    equipment flag        (1 in every capture)
//	raw1    hasPendingLoot
//	zigzag  skillCount + rows     func_00BDCDC0
//	raw4    run speed             (0x410D5555 = 8.833 in every capture)
//	raw4    unknown               (0)
//	6x4     raw4 weapon hotbar    func_00BFD178
//	raw4 + 4x zigzag + raw4       func_00BF71A0 CM header
//	29x zigzag, 7x zigzag         CM stat arrays
//
// This block used to be written as "float speed + 26x int32 + CM zigzags",
// which came out 8 bytes shorter than the client reads. The client's stream
// hit the end of the buffer part-way through the first CM array, failed the
// whole MemoryDump, and dropped the session back to server select — which is
// what "picking a character returns to server select" was.
//
// Abilities: retail does NOT use the spell list for them. Both captured
// characters send spellCount 0 and carry their songs/spells as skill rows
// (Sandstorm's bard sends 42, including "Chant of Battle"). Our stored
// spellbook is in a different row format than func_00BDCBE0 reads, so it is
// not sent at all rather than sent malformed; wiring it into the skill table
// is separate work.
func writeMemoryDumpTail(w *buffer.Writer, player *entity.Player) {
	w.Write7BitEncodedInt64(0) // spellCount   — abilities ride the skill table
	w.Write7BitEncodedInt64(0) // buffCount
	w.WriteByte(0x01)          // equipment trailing flag
	w.WriteByte(0x00)          // hasPendingLoot
	w.Write7BitEncodedInt64(0) // skillCount

	w.WriteFloat32(player.Speed)
	w.WriteUint32(0)

	for _, row := range weaponHotbarRows {
		for _, v := range row {
			w.WriteUint32(v)
		}
	}

	// CM (Combat Mastery) block — func_00BF71A0.
	w.WriteUint32(0)           // +0x41C, raw
	w.Write7BitEncodedInt64(0) // +0x0C
	w.Write7BitEncodedInt64(0) // +0x10
	w.Write7BitEncodedInt64(0) // +0x14
	w.Write7BitEncodedInt64(0) // +0x08 (read after +0x14)
	w.WriteUint32(0)           // +0xA8, raw
	for i := 0; i < 29; i++ {  // func_00BF7060
		w.Write7BitEncodedInt64(0)
	}
	for i := 0; i < 7; i++ { // func_00BF6FF0
		w.Write7BitEncodedInt64(0)
	}

	for i := 0; i < memoryDumpTrailerLen; i++ {
		w.WriteByte(0x00)
	}
}

// writeSingleItem serializes one DumpItem entry to the MemoryDump wire format.
// Wire format: 29 zigzag fields (with key as uint32) + name + desc + stat pairs.
// Mirrors the client's parseSingleItem() exactly.
// writeSingleItem writes one item row. `key` is the item's POSITION in its
// container (inventory or bank), 0-based — not a database id.
//
// This is the C# `Item.DumpItem(ref writer, int key)` called as
// `DumpItem(ref writer, item.ClientIndex)`, where ClientIndex is assigned
// `(byte)_itemContainer.Count` when the item is added. The client keeps it as
// the record's key and echoes it back in InteractItem (0x003E). A key equals a
// position only until a record leaves the list (the client packs the list and
// never renumbers), so requests resolve keys through the session's invList
// (itemlist.go), never by indexing. Sending charinventory.itemid here instead
// made clicking one row act on another.
func writeSingleItem(w *buffer.Writer, e *database.CharInventoryEntry, key int) {
	p := &e.Pattern

	// Per-instance fields
	w.Write7BitEncodedInt64(int64(e.StackLeft))
	w.Write7BitEncodedInt64(int64(e.RemainHP))
	w.Write7BitEncodedInt64(int64(e.Charges))
	w.Write7BitEncodedInt64(int64(e.EquipLoc))
	w.Write7BitEncodedInt64(int64(e.Location))
	w.WriteUint32(uint32(key)) // key = container position (see doc comment)

	// Pattern fields
	w.Write7BitEncodedInt64(int64(p.PatternID)) // ItemID on wire
	w.Write7BitEncodedInt64(int64(p.ItemCost))
	w.Write7BitEncodedInt64(int64(p.Unk1))
	w.Write7BitEncodedInt64(p.ItemIcon)
	w.Write7BitEncodedInt64(int64(p.Unk2))
	w.Write7BitEncodedInt64(int64(p.EquipSlot))
	w.Write7BitEncodedInt64(int64(p.Unk3))
	w.Write7BitEncodedInt64(int64(p.Trade))
	w.Write7BitEncodedInt64(int64(p.Rent))
	w.Write7BitEncodedInt64(int64(p.Unk4))
	w.Write7BitEncodedInt64(int64(p.AttackType))
	w.Write7BitEncodedInt64(int64(p.WeaponDamage))
	w.Write7BitEncodedInt64(int64(p.Unk5))
	w.Write7BitEncodedInt64(int64(p.LevelReq))
	w.Write7BitEncodedInt64(int64(p.MaxStack))
	w.Write7BitEncodedInt64(int64(p.MaxHP))
	w.Write7BitEncodedInt64(int64(p.Duration))
	w.Write7BitEncodedInt64(int64(p.ClassUse))
	w.Write7BitEncodedInt64(int64(raceUseForWire(p.RaceUse)))
	w.Write7BitEncodedInt64(int64(p.ProcAnim))
	w.Write7BitEncodedInt64(int64(p.Lore))
	w.Write7BitEncodedInt64(int64(p.Unk6))
	w.Write7BitEncodedInt64(int64(p.Craft))

	// Item name + description (WriteStringUnicode = int32 charCount + UTF-16LE)
	w.WriteStringUnicode(p.ItemName)
	w.WriteStringUnicode(p.ItemDesc)

	// Stats: collect non-zero stats as index/value pairs
	// Index mapping: 0=str 1=sta 2=agi 3=dex 4=wis 5=int 6=cha
	//   8=HPMAX 10=POWMAX 12=PoT 13=HoT 14=AC
	//   22=PR 23=DR 24=FR 25=CR 26=LR 27=AR
	stats := itemPatternStats(p)
	w.Write7BitEncodedInt64(int64(len(stats)))
	// Write in ascending index order (matches C# iteration)
	indices := make([]int, 0, len(stats))
	for idx := range stats {
		indices = append(indices, idx)
	}
	sort.Ints(indices)
	for _, idx := range indices {
		w.Write7BitEncodedInt64(int64(idx))
		w.Write7BitEncodedInt64(int64(stats[idx]))
	}
}

// itemPatternStats builds a stat index→value map from an ItemPatternRecord's nullable columns.
func itemPatternStats(p *database.ItemPatternRecord) map[int]int {
	stats := make(map[int]int)
	addStat := func(idx int, v *int) {
		if v != nil && *v != 0 {
			stats[idx] = *v
		}
	}
	addStat(0, p.Str)
	addStat(1, p.Sta)
	addStat(2, p.Agi)
	addStat(3, p.Dex)
	addStat(4, p.Wis)
	addStat(5, p.Intelligence)
	addStat(6, p.Cha)
	addStat(8, p.HPMAX)
	addStat(10, p.POWMAX)
	addStat(12, p.PoT)
	addStat(13, p.HoT)
	addStat(14, p.AC)
	addStat(22, p.PR)
	addStat(23, p.DR)
	addStat(24, p.FR)
	addStat(25, p.CR)
	addStat(26, p.LR)
	addStat(27, p.AR)
	return stats
}

// writeSingleSpell serializes one DumpSpell entry to the MemoryDump wire format.
// Wire format: 9 zigzag + uint16(range) + 7 zigzag + name + desc + byte(0x00).
//
// NOT CURRENTLY CALLED. This format is not what the PS2 reads: the rows landed
// where func_00BD0630 reads the skill table (func_00BDCBE0: 15 zigzag + raw4 +
// two UTF-16 blobs + a sub-entry count), so any character with spells sent the
// PS2 malformed rows. Retail carries abilities as skill rows — Sandstorm's bard
// sends 42 of them, spell count 0 — so delivering our spellbook means mapping
// it onto that row format, not this one. Kept as the DB→wire field mapping that
// work will start from.
func writeSingleSpell(w *buffer.Writer, e *database.CharSpellEntry) {
	sp := &e.Pattern

	// 9 zigzag fields
	w.Write7BitEncodedInt64(int64(sp.SpellID))
	w.Write7BitEncodedInt64(int64(e.AddedOrder))
	w.Write7BitEncodedInt64(int64(e.OnHotBar))
	w.Write7BitEncodedInt64(int64(e.WhereOnHotBar))
	w.Write7BitEncodedInt64(int64(e.Unk1))
	w.Write7BitEncodedInt64(int64(e.ShowHide))
	w.Write7BitEncodedInt64(int64(sp.AbilityLvl))
	w.Write7BitEncodedInt64(int64(sp.Unk2))
	w.Write7BitEncodedInt64(int64(sp.Unk3))

	// Range as uint16 (half-float bits stored as integer in DB)
	w.WriteUint16(uint16(sp.Range))

	// 7 zigzag fields
	w.Write7BitEncodedInt64(int64(sp.CastTime))
	w.Write7BitEncodedInt64(int64(sp.Power))
	w.Write7BitEncodedInt64(sp.IconColor)
	w.Write7BitEncodedInt64(sp.Icon)
	w.Write7BitEncodedInt64(int64(sp.Scope))
	w.Write7BitEncodedInt64(int64(sp.Recast))
	w.Write7BitEncodedInt64(int64(sp.EqpRequire))

	// Spell name + description (WriteStringUnicode = int32 charCount + UTF-16LE)
	w.WriteStringUnicode(sp.SpellName)
	w.WriteStringUnicode(sp.SpellDesc)

	// Terminator
	w.WriteByte(0x00)
}

// flushStateChannels immediately generates and queues state channel messages.
// This is needed because the tick routine only runs for CharacterInWorld sessions,
// but the client needs state channels before it sends CharacterInWorld.
func (h *CharacterHandler) flushStateChannels(sess *session.Session) {
	conn := sess.Connection()
	if conn == nil {
		return
	}

	// Generate object updates for dirty channels (0-22)
	for i := 0; i < session.MaxServerObjects; i++ {
		state := conn.GetServerObject(i)
		if state == nil {
			continue
		}
		if data, seq, xorByte := state.GenerateXORDelta(); data != nil {
			msg := protocol.NewMessage(protocol.MessageType(i), 0)
			msg.Sequence = seq
			msg.XORByte = xorByte
			msg.SetData(data)
			sess.Queue().Add(msg)
			log.Printf("flushStateChannels: channel %d, seq=%d, xorByte=%d, dataLen=%d",
				i, seq, xorByte, len(data))
			if i == 0 && len(data) > 184 {
				log.Printf("flushStateChannels ch0: buf[140]=%d buf[183]=%d buf[184]=%d",
					data[140], data[183], data[184])
			}
		}
	}

	// Generate stat update (channel 0x42)
	statUpdate := conn.GetClientStatUpdate()
	if data, seq, xorByte := statUpdate.GenerateXORDelta(); data != nil {
		msg := protocol.NewMessage(protocol.MessageTypeStatUpdate, 0)
		msg.Sequence = seq
		msg.XORByte = xorByte
		msg.SetData(data)
		sess.Queue().Add(msg)
		log.Printf("flushStateChannels: stat update, seq=%d, xorByte=%d, dataLen=%d",
			seq, xorByte, len(data))
	}

	// Generate buff update (channel 0x43)
	buffUpdate := conn.GetClientBuffUpdate()
	if data, seq, xorByte := buffUpdate.GenerateXORDelta(); data != nil {
		msg := protocol.NewMessage(protocol.MessageTypeBuffUpdate, 0)
		msg.Sequence = seq
		msg.XORByte = xorByte
		msg.SetData(data)
		sess.Queue().Add(msg)
		log.Printf("flushStateChannels: buff update, seq=%d, xorByte=%d, dataLen=%d",
			seq, xorByte, len(data))
	}
}

// initPlayerStateChannel builds state channel 0 from the Player struct.
func (h *CharacterHandler) initPlayerStateChannel(sess *session.Session, player *entity.Player) {
	conn := sess.Connection()
	if conn == nil {
		log.Printf("initPlayerStateChannel: no connection for session")
		return
	}

	channel0 := conn.GetServerObject(0)
	if channel0 == nil {
		log.Printf("initPlayerStateChannel: channel 0 is nil")
		return
	}

	objectID := sess.PlayerObjectID()
	stateData := entity.SerializePlayerUpdate(objectID, player)

	channel0.SetActive(true)
	channel0.SetData(stateData)

	log.Printf("initPlayerStateChannel: set channel 0 (%d bytes) for '%s' objectID=%d",
		len(stateData), player.Name, objectID)
}

// initPlayerStatChannel builds the stat channel (0x42) from the Player struct.
func (h *CharacterHandler) initPlayerStatChannel(sess *session.Session, player *entity.Player) {
	conn := sess.Connection()
	if conn == nil {
		log.Printf("initPlayerStatChannel: no connection for session")
		return
	}

	statChannel := conn.GetClientStatUpdate()
	if statChannel == nil {
		log.Printf("initPlayerStatChannel: stat channel is nil")
		return
	}

	statData := entity.SerializeStatUpdate(player)
	statChannel.SetActive(true)
	statChannel.SetData(statData)

	log.Printf("initPlayerStatChannel: set stat channel (236 bytes) HP=%d/%d Power=%d/%d",
		player.HP, player.MaxHP, player.Power, player.MaxPower)
}

// initPlayerBuffChannel builds the buff channel (0x43) from the Player struct.
func (h *CharacterHandler) initPlayerBuffChannel(sess *session.Session, player *entity.Player) {
	conn := sess.Connection()
	if conn == nil {
		return
	}

	buffChannel := conn.GetClientBuffUpdate()
	if buffChannel == nil {
		return
	}

	buffData := entity.SerializeBuffUpdate(player)
	buffChannel.SetActive(true)
	buffChannel.SetData(buffData)

	log.Printf("initPlayerBuffChannel: set buff channel (136 bytes)")
}

// SendDelCharacter confirms character deletion.
func (h *CharacterHandler) SendDelCharacter(sess *session.Session, charID int) {
	msg := protocol.NewMessage(protocol.MessageTypeReliable, protocol.OpcodeDelCharacter)
	writer := buffer.NewWriter(msg.Span())
	writer.WriteUint16(uint16(protocol.OpcodeDelCharacter)) // Opcode in data
	writer.WriteUint32(uint32(charID))
	msg.SetSize(writer.Position())
	sess.Queue().AddWithSequence(msg)
}

// SendCamera1 sends Camera1 (opcode 0x07D1) with value 0x03.
// Sandstorm sends this as the first message on a freshly promoted master
// session, before Time and the MemoryDump fragments. eqoa.live only ever sends
// Camera1, never Camera2 (0x07F5).
// SendCamera1 sends opcode 0x07D1. The payload is int32 -1 on every session —
// see the note in AuthHandler where the same message is built.
func (h *CharacterHandler) SendCamera1(sess *session.Session) {
	msg := protocol.NewMessage(protocol.MessageTypeReliable, protocol.OpcodeCamera1)
	writer := buffer.NewWriter(msg.Span())
	writer.WriteUint16(uint16(protocol.OpcodeCamera1))
	writer.WriteInt32(-1)
	msg.SetSize(writer.Position())
	sess.Queue().AddWithSequence(msg)
}

// SendTime sends the current game time (opcode 0x13).
func (h *CharacterHandler) SendTime(sess *session.Session) {
	// First send a ping message with version (0xF9 type)
	// Note: Ping messages don't have opcode in data - they're a special type
	pingMsg := protocol.NewMessage(protocol.MessageTypePing, 0)
	pingWriter := buffer.NewWriter(pingMsg.Span())
	pingWriter.WriteByte(protocol.GameVersionEQOAVanilla) // 0x12
	pingMsg.SetSize(pingWriter.Position())
	sess.Queue().AddWithSequence(pingMsg)

	h.sendTimeOnly(sess)
}

// SendIgnoreList sends the player ignore list (opcode 0x1005).
// Based on C# ServerPlayerIgnoreList.PlayerIgnoreList()
func (h *CharacterHandler) SendIgnoreList(sess *session.Session) {
	msg := protocol.NewMessage(protocol.MessageTypeReliable, protocol.OpcodeIgnoreList)
	writer := buffer.NewWriter(msg.Span())

	// Write opcode in data (C# reference format)
	writer.WriteUint16(uint16(protocol.OpcodeIgnoreList))
	// Write empty ignore list count (0 ignored players)
	writer.WriteByte(0)

	msg.SetSize(writer.Position())
	sess.Queue().AddWithSequence(msg)
	log.Printf("Sent IgnoreList (empty) to %s", sess.RemoteAddr())
}

// SendBuddyList sends the buddy list (opcode 0x03D5).
// eqoa.live sends one message per buddy. Empty = no messages sent.
func (h *CharacterHandler) SendBuddyList(sess *session.Session) {
	// No buddies to send for now — stub.
	// When buddy system is implemented, send one 0x03D5 message per buddy entry.
	log.Printf("BuddyList: no buddies to send to %s", sess.RemoteAddr())
}

// sendTimeOnly sends just the Time message (opcode 0x13) without Ping.
// Used in MemoryDump sequence where C# only sends Time, not Ping.
func (h *CharacterHandler) sendTimeOnly(sess *session.Session) {
	msg := protocol.NewMessage(protocol.MessageTypeReliable, protocol.OpcodeTime)
	writer := buffer.NewWriter(msg.Span())

	// Write opcode in data
	writer.WriteUint16(uint16(protocol.OpcodeTime))
	// Write time data - two uint64 LE epoch-millisecond timestamps
	now := uint64(time.Now().UnixMilli())
	writer.WriteUint64(now)
	writer.WriteUint64(now + 1350000)
	msg.SetSize(writer.Position())

	sess.Queue().AddWithSequence(msg)
	log.Printf("sendTimeOnly: sent Time message to %s (epoch=%d)", sess.RemoteAddr(), now)
}

// SendPlayerSpeed sends the player speed message (opcode 0xF8).
// This is needed for the character to be able to move.
func (h *CharacterHandler) SendPlayerSpeed(sess *session.Session, speed float32) {
	msg := protocol.NewMessage(protocol.MessageTypeReliable, protocol.OpcodePlayerSpeed)
	writer := buffer.NewWriter(msg.Span())

	// Write opcode in data
	writer.WriteUint16(uint16(protocol.OpcodePlayerSpeed))
	// Write speed as float32
	writer.WriteFloat32(speed)

	msg.SetSize(writer.Position())
	sess.Queue().AddWithSequence(msg)
	log.Printf("Sent PlayerSpeed (%.1f) to %s", speed, sess.RemoteAddr())
}

// hotkey represents a single directional hotkey entry with 4 slots (N/W/E/S).
type hotkey struct {
	dirCode      int64
	nLabel, nMsg string
	wLabel, wMsg string
	eLabel, eMsg string
	sLabel, sMsg string
}

// defaultHotkeys are the 25 default hotkey entries from the eqoa.live reference server.
// Labels starting with "|" followed by a number are client command codes.
// Labels of " " (single space) indicate submenu navigation nodes.
// %4 = target name, %2 = class name, %3 = level.
var defaultHotkeys = []hotkey{
	{0, " ", "Responses", " ", "Options", " ", "Group", " ", "Communicate"},                                                                                                                                                // Main Menu
	{1, "Yes", "Yes", "No", "No", "I don't know", "I don't know", "", "More Responses"},                                                                                                                                    // N
	{5, " ", "Chat Mode", " ", "Pet Command", " ", "Grouping", " ", "Chat Filter"},                                                                                                                                         // W
	{6, "|14", "Normal Say", "|17", "Guild Speak", "|15", "Group Speak", "|16", "Shout"},                                                                                                                                   // WN
	{9, " ", "Attacking", " ", "Creation", " ", "Readiness", " ", "Important!"},                                                                                                                                            // E
	{10, "I am attacking %4", "Pulling", "|22", "Assist Target", "|21", "Assist Leader", "I am attacking %4, please assist me!", "Assist Me"},                                                                              // EN
	{13, " ", "Player", "", "Navigation", " ", "Ask Help", " ", "Action"},                                                                                                                                                  // S
	{14, "Hail %4", "Hail", "|19", "Tell", "|18", "Reply", "Goodbye %4", "Goodbye"},                                                                                                                                        // SN
	{22, "|24", "Passive", " ", "More Pet Commands", "|25", "Defensive", "|26", "Aggressive"},                                                                                                                              // WW
	{26, "|4", "Invite", " ", "Organization", "%2 %3 seeking xp  group!", "Need Group", " ", "Hunting"},                                                                                                                    // EW
	{38, "|4", "Invite", "|5", "Boot Member", "|15", "Group Speak", "|6", "Leave Group"},                                                                                                                                   // WE
	{42, "Ready?", "Ready?", "Low on health / power.", "Health/Power", "I am good to go!", "Good to Go!", "I need a short break, be right back in a minute!", "Break 1min"},                                                // EE
	{46, "Do you have spare time to answer a question?", "Request", "Where can I find ...", "Where to find?", "I am lost, where am I?", "Where am I?", "Where can I find the nearest town with merchants?", "Where Town?"}, // SE
	{50, "Thanks", "Thanks", "Nevermind", "Nevermind", "", "Greetings", "", "Notifications"},                                                                                                                               // NS
	{51, "%4 is training people!!!!!", "training people", "Looking for a healer lvs 1-2 PST", "looking 4", "|7", "ignore", "Watch out for trainers!!!!!!", "trainers"},                                                     // NSN
	{54, "|7", "Ignore Target", "|12", "Ignore Guild", "|10", "Ignore Shouts", " ", "Restore Commands"},                                                                                                                    // WS
	{58, "RUN! This is a battle we cannot win!", "Retreat!", "Someone just went Link Dead!", "Link Dead!", "HELP! I'm being attacked, peel it off of me!", "Peel!", "MEDIC! I need healing badly!", "Medic!"},              // ES
	{62, "|0", "Wave", "|2", "Point", "|3", "Cheer", "|1", "Bow"},                                                                                                                                                          // SS
	{87, "|27", "Neutral", "|30", "Pet Attack", "|29", "Pet Backoff", "|28", "Dismiss Pet"},                                                                                                                                // WWW
	{91, "Roll for loot please!", "Request Roll", "Loot up if you want this.", "Loot up!", "Would you like to group?", "Want Group?", "|23", "Roll 0-99"},                                                                  // EWW
	{219, "Follow me.", "Follow me", "Let's find another place to hunt.", "New Hunt", "Who wants to handle pulling?", "Puller?", "Where shall we hunt?", "Where hunt?"},                                                    // EWS
	{247, "|9", "Privacy List", "|13", "Restore guild", "|11", "Restore shout", "|8", "Stop Ignoring"},                                                                                                                     // WSS
	{30, "North", "North", "West", "West", "East", "East", "South", "South"},                                                                                                                                               // SW
	{179, "Good", "Good", "Bad", "Bad", "How's it?", "How's it?", "Okay", "Okay"},                                                                                                                                          // NSE
	{243, "Sorry, busy", "Sorry, busy", "Quit", "Quit", "Long break", "Long break", "Keyboard", "Keyboard"},                                                                                                                // NSS
}

// writeDefaultHotkeys writes the 25 default hotkey entries to the MemoryDump.
// Wire format (from eqoa.live pcap): count(zigzag) + foreach: direction(zigzag) +
// 4x(byte(slotID) + WriteStringUnicode(label) + WriteStringUnicode(message))
func writeDefaultHotkeys(writer *buffer.Writer) {
	writer.Write7BitEncodedInt64(int64(len(defaultHotkeys)))

	for _, hk := range defaultHotkeys {
		writer.Write7BitEncodedInt64(hk.dirCode)

		// N slot (0)
		writer.WriteByte(0)
		writer.WriteStringUnicode(hk.nLabel)
		writer.WriteStringUnicode(hk.nMsg)

		// W slot (2)
		writer.WriteByte(2)
		writer.WriteStringUnicode(hk.wLabel)
		writer.WriteStringUnicode(hk.wMsg)

		// E slot (4)
		writer.WriteByte(4)
		writer.WriteStringUnicode(hk.eLabel)
		writer.WriteStringUnicode(hk.eMsg)

		// S slot (6)
		writer.WriteByte(6)
		writer.WriteStringUnicode(hk.sLabel)
		writer.WriteStringUnicode(hk.sMsg)
	}
}
