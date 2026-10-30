package handlers

import (
	"fmt"
	"github.com/eqoa/server/internal/auth"
	"log"
	"sync"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// Handler is a function that handles a game message.
type Handler func(sess *session.Session, msg *protocol.Message) error

// Registry maps opcodes to their handlers.
type Registry struct {
	handlers map[protocol.GameOpcode]Handler
	mu       sync.RWMutex

	// authHandler is retained so login tickets can be attached after the
	// handlers are built.
	authHandler *AuthHandler

	// invis ends an action's sender's invisibility (invisactions.go)
	invis InvisBreaker
}

// SetPasswordChecker lets the UDP handshake sign a username with no ticket in
// by the password its token carries. See AuthHandler.SetPasswordChecker.
func (r *Registry) SetPasswordChecker(c PasswordChecker) {
	if r.authHandler != nil {
		r.authHandler.SetPasswordChecker(c)
	}
}

// SetLoginTickets makes the UDP handshake require a completed TCP login. See
// AuthHandler.SetLoginTickets.
func (r *Registry) SetLoginTickets(t *auth.TicketStore) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.authHandler != nil {
		r.authHandler.SetLoginTickets(t)
	}
}

// NewRegistry creates a new handler registry.
func NewRegistry() *Registry {
	return &Registry{
		handlers: make(map[protocol.GameOpcode]Handler),
	}
}

// Register adds a handler for an opcode.
func (r *Registry) Register(opcode protocol.GameOpcode, handler Handler) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.handlers[opcode] = handler
}

// RegisterAll registers multiple handlers at once.
func (r *Registry) RegisterAll(handlers map[protocol.GameOpcode]Handler) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for opcode, handler := range handlers {
		r.handlers[opcode] = handler
	}
}

// Get retrieves a handler for an opcode.
func (r *Registry) Get(opcode protocol.GameOpcode) (Handler, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	handler, ok := r.handlers[opcode]
	return handler, ok
}

// Handle dispatches a message to the appropriate handler.
func (r *Registry) Handle(sess *session.Session, msg *protocol.Message) error {
	handler, ok := r.Get(msg.Opcode)
	if !ok {
		// LOG THE PAYLOAD, not just the opcode number.
		//
		// 0x00D8 arrived once from a live PS2, was logged as a bare opcode, and
		// is now unidentifiable: it appears in no capture, no decompilation
		// note and no opcode-verify table, so the one sample we had is gone.
		// An opcode number alone says nothing; the bytes are what let the next
		// occurrence be identified.
		data := msg.Data()
		const maxDump = 128
		dump := data
		truncated := ""
		if len(dump) > maxDump {
			dump = dump[:maxDump]
			truncated = fmt.Sprintf(" (first %d of %d)", maxDump, len(data))
		}
		log.Printf("Unhandled opcode: 0x%04X (%s) from %s len=%d%s hex=%x",
			uint16(msg.Opcode), msg.Opcode.String(), sess.RemoteAddr(),
			len(data), truncated, dump)
		return nil
	}

	r.breakInvisFor(sess, msg.Opcode) // an action shows an invisible player
	if err := handler(sess, msg); err != nil {
		log.Printf("Handler error for opcode 0x%04X: %v", msg.Opcode, err)
		return err
	}

	return nil
}

// HandleMessage processes a message based on its type.
func (r *Registry) HandleMessage(sess *session.Session, msg *protocol.Message) error {
	switch msg.Type {
	case protocol.MessageTypePing:
		return r.handlePing(sess, msg)
	case protocol.MessageTypeReliable, protocol.MessageTypeSegmentReliable:
		return r.Handle(sess, msg)
	case protocol.MessageTypeUnreliable:
		return r.handleUnreliable(sess, msg)
	default:
		if msg.Type.IsStateChannel() {
			return r.handleStateChannel(sess, msg)
		}
		log.Printf("Unknown message type: 0x%02X", msg.Type)
		return nil
	}
}

// handlePing processes a ping message.
func (r *Registry) handlePing(sess *session.Session, msg *protocol.Message) error {
	// Ping messages update activity and may trigger a pong response
	sess.UpdateActivity()

	// Queue acknowledgment flags
	sess.Queue().SetBodyFlags(protocol.BodyFlagGuaranteedAck | protocol.BodyFlagFlushAck)

	return nil
}

// handleUnreliable processes an unreliable message.
func (r *Registry) handleUnreliable(sess *session.Session, msg *protocol.Message) error {
	// Dispatch to handler if registered
	return r.Handle(sess, msg)
}

// handleStateChannel processes a state channel message.
func (r *Registry) handleStateChannel(sess *session.Session, msg *protocol.Message) error {
	// State channel messages contain client state updates (position, camera, etc.)
	conn := sess.Connection()

	// Update client state
	clientState := conn.GetClientState()
	clientState.SeqNum = msg.Sequence

	// Store the data for processing
	// This would be parsed for position updates, etc.

	// Flag that we need to ack this
	sess.Queue().SetBodyFlags(protocol.BodyFlagClientUpdateAck)

	return nil
}

// RegisterDefaultHandlers registers the default set of handlers.
func RegisterDefaultHandlers(r *Registry, charStore *database.SQLiteCharacterStore, accountStore *database.SQLiteAccountStore, sessionMgr *session.Manager, serverIP string, gameWorld *entity.World, invStore *database.InventoryStore, spellStore *database.SpellbookStore) *ServerHandler {
	// Create handler instances
	serverHandler := NewServerHandler(serverIP)
	charHandler := NewCharacterHandler(charStore, sessionMgr, invStore, spellStore)
	if gameWorld != nil {
		// any action but moving, targeting and chat shows an invisible player
		// (invisactions.go)
		r.SetInvisBreaker(gameWorld.BreakInvis)
		charHandler.SetGameWorld(gameWorld)
		// A dead player's respawn: the login spawn and a MemoryDump (respawn.go)
		gameWorld.OnPlayerRespawn(charHandler.respawnPlayer)
		// Return Home's landing: a teleport there (cast.go)
		gameWorld.OnGate(charHandler.gatePlayer)
		// the gates, portals, circles and evacuations (gates.go)
		gameWorld.OnTravel(charHandler.travelPlayer)
		gameWorld.SetLoadableWorlds(LoadableWorlds)
	}
	authHandler := NewAuthHandlerFull(serverHandler, charHandler, accountStore)
	r.mu.Lock()
	r.authHandler = authHandler
	r.mu.Unlock()
	chatHandler := NewChatHandler(sessionMgr)
	if gameWorld != nil {
		chatHandler.SetGameWorld(gameWorld)
	}
	if accountStore != nil {
		chatHandler.SetAccessLevels(accountStore)
	}
	// a GM's level and purse kept, and #template (devcmds.go, template.go)
	var progress progressStore
	if charStore != nil {
		progress = charStore
	}
	var purse purseStore
	if invStore != nil {
		purse = invStore
	}
	chatHandler.SetStores(progress, purse)
	if charStore != nil {
		chatHandler.SetMasteryStore(charStore)
		// the live client's resets and #banktunar (gmresets.go)
		chatHandler.SetResetStore(charStore)
	}
	chatHandler.SetTemplate(charHandler.ApplyTemplate)
	// #fx and #spellfx: the spells and their visuals (spellfx.go)
	if spellStore != nil {
		chatHandler.SetSpellVisuals(spellStore)
	}
	targetHandler := NewTargetHandler(sessionMgr)
	if gameWorld != nil {
		targetHandler.SetGameWorld(gameWorld)
	}
	combatHandler := NewCombatHandler(sessionMgr)
	if gameWorld != nil {
		combatHandler.SetGameWorld(gameWorld)
	}
	combatHandler.SetInventoryStore(invStore)
	equipHandler := NewEquipHandler(sessionMgr, invStore, spellStore, charHandler)
	if gameWorld != nil {
		equipHandler.SetGameWorld(gameWorld)
	}
	charHandler.gearSets = equipHandler.SendGearSets
	// potions: every item's use, the spells that give them, the throws that
	// take them (potion.go, entity/potion.go)
	if gameWorld != nil {
		equipHandler.LoadPotions()
		gameWorld.OnSummonItem(equipHandler.summonItem)
		gameWorld.OnPotionThrown(equipHandler.potionThrown)
	}
	interactHandler := NewInteractHandler(sessionMgr)
	if gameWorld != nil {
		interactHandler.SetGameWorld(gameWorld)
	}
	whoHandler := NewWhoHandler(sessionMgr)
	if gameWorld != nil {
		whoHandler.SetGameWorld(gameWorld)
	}
	classMasteryHandler := NewClassMasteryHandler(sessionMgr)
	// the share of experience going to mastery points, kept (entity/mastery.go)
	if gameWorld != nil {
		classMasteryHandler.SetGameWorld(gameWorld)
	}
	if charStore != nil {
		classMasteryHandler.SetStore(charStore)
	}
	if spellStore != nil {
		classMasteryHandler.SetSpellbook(spellStore)
	}
	chatHandler.SetClassMastery(classMasteryHandler)
	connStatsHandler := NewConnStatsHandler()
	// the GM's channel (gmchannel.go): sent at world entry, its items and
	// renames kept, a player's counters relayed, chat gated, a leaver forgotten
	var names nameStore
	if charStore != nil {
		names = charStore
	}
	chatHandler.SetGMStores(invStore, names)
	charHandler.gmChannel = chatHandler.SendGMChannel
	connStatsHandler.relay = chatHandler.gmNetStats
	if gameWorld != nil {
		gameWorld.OnPlayerLeaving(chatHandler.gmLeft)
	}
	trainingHandler := NewTrainingHandler(sessionMgr, charStore)
	if gameWorld != nil {
		trainingHandler.SetGameWorld(gameWorld)
	}
	factionHandler := NewFactionHandler(sessionMgr)
	if gameWorld != nil {
		factionHandler.SetGameWorld(gameWorld)
	}

	// Register auth handlers
	authHandler.RegisterHandlers(r)

	// Register character handlers
	charHandler.RegisterHandlers(r)

	// Register chat handlers
	chatHandler.RegisterHandlers(r)
	chatHandler.RegisterGMChannel(r)

	// Register target handlers
	targetHandler.RegisterHandlers(r)

	// Register combat handlers
	combatHandler.RegisterHandlers(r)

	// Register equip handlers (InteractItem 0x003E). Without this, equipping
	// anything in game fell through to "Unhandled opcode".
	equipHandler.RegisterHandlers(r)

	// Register interact handlers
	interactHandler.RegisterHandlers(r)

	// The merchant, bank and blacksmith windows (merchant.go, bank.go,
	// blacksmith.go): the requests InteractWithTarget sends instead of 0x0004.
	npcWindowHandler := NewNPCWindowHandler(sessionMgr, invStore)
	if gameWorld != nil {
		npcWindowHandler.SetGameWorld(gameWorld)
	}
	npcWindowHandler.RegisterHandlers(r)
	// The auction house (auction.go, database/auction.go): 0x1205 on its
	// tables, the sweep that closes lots at their time (at once at a start:
	// what ended while the server was down), and a player's bank made right
	// as it logs in
	if charStore != nil {
		house := NewAuctionHouse(charStore.DB(), npcWindowHandler)
		house.isGM = chatHandler.isGM
		charHandler.auction = house
		// the live forms' answer (livefeatures.go offers them)
		r.Register(protocol.OpcodeAuctionForm, house.HandleForm)
		if gameWorld != nil {
			house.SetGameWorld(gameWorld)
			house.Start()
		}
	}
	interactHandler.corpse = npcWindowHandler.OpenCorpse
	// a waysender's fee, and the bind points spiritmasters give
	interactHandler.spend = npcWindowHandler.spendTunar
	// the coachmen's GMs, offered every coach town (coachledger.go)
	interactHandler.isGM = chatHandler.isGM
	if charStore != nil {
		interactHandler.binds = charStore
		// the stables' ledgers (coachledger.go)
		interactHandler.ledger = charStore
		// what a town's people and the guards say (entity/dialog.go)
		interactHandler.SetDialogLines(charStore.DB().NPCDialog)
	}

	// Trading between players (trade.go): 0x0057 and 0x0056.
	tradeHandler := NewTradeHandler(sessionMgr, npcWindowHandler)
	if gameWorld != nil {
		tradeHandler.SetGameWorld(gameWorld)
	}
	tradeHandler.RegisterHandlers(r)

	// Tradeskills (craft.go): 0x1101, the Recipe Book and the Experiment;
	// the dump's six skill slots, a recipe scroll used, #craftskill and
	// #craftlearn
	var craftDB *database.SQLiteDB
	if charStore != nil {
		craftDB = charStore.DB()
	}
	craft := NewCraftHandler(craftDB, invStore)
	craft.SetGameWorld(gameWorld)
	craft.RegisterHandlers(r)
	charHandler.craftSlots = craft.DumpSlots
	equipHandler.craft = craft
	chatHandler.craft = craft

	// Fishing (fishing.go): 0x0F01, and the sessions a fishing pole used from
	// the HUD, a gear set holding one and /fish start
	var fishDB *database.SQLiteDB
	if charStore != nil {
		fishDB = charStore.DB()
	}
	fishing := NewFishingHandler(fishDB, charStore, invStore)
	fishing.SetGameWorld(gameWorld)
	fishing.RegisterHandlers(r)
	if gameWorld != nil {
		fishing.Start()
	}
	equipHandler.fishing = fishing
	chatHandler.fishing = fishing

	// Register who handlers
	whoHandler.RegisterHandlers(r)

	// Register training point handlers
	trainingHandler.RegisterHandlers(r)
	classMasteryHandler.RegisterHandlers(r)
	connStatsHandler.RegisterHandlers(r)

	// Register faction handlers
	factionHandler.RegisterHandlers(r)

	// Register guild handlers
	guildHandler := NewGuildHandler(sessionMgr)
	if gameWorld != nil {
		guildHandler.SetGameWorld(gameWorld)
	}
	guildHandler.chatGate = chatHandler.gmChatGate
	guildHandler.RegisterHandlers(r)

	// Register group handlers (the group is the World's: entity/group.go)
	NewGroupHandler(gameWorld).RegisterHandlers(r)
	// The live client's group search and the Group menu's Show/Hide Chat
	// (0x0801, the disc's 0x0B01; lfg.go, entity/lfg.go)
	NewLFGHandler(gameWorld).RegisterHandlers(r)

	// Register the buddy and ignore lists (entity/social.go)
	NewSocialHandler(gameWorld).RegisterHandlers(r)

	// Register the pet commands (0x0041; entity/pet.go)
	NewPetHandler(gameWorld).RegisterHandlers(r)

	// Register the tell-gram mail (0x0D01 in, 0x0D41 out; mail.go)
	NewMailHandler(gameWorld, charStore).RegisterHandlers(r)

	// Register customer service (0x0AA1 in, 0x07B0 out; support.go): the
	// petitions table, which the GM panel's Petitions page answers
	var petitions petitionStore
	if charStore != nil {
		petitions = charStore.DB()
	}
	NewSupportHandler(gameWorld, petitions).RegisterHandlers(r)

	// Register the spell book's requests (0x0033, 0x003A, 0x00FE)
	NewSpellbookHandler(gameWorld, spellStore).RegisterHandlers(r)

	// Register the world-load handler (0x0049). This is what re-describes the
	// scene after a teleport; without it a world change lands the player in an
	// empty world. See worldload.go.
	NewWorldLoadHandler().RegisterHandlers(r)

	// Register the client's entity view (0x7E00; entityslots.go): a client
	// that keeps more than CLIENT's 24 says how many, and how far
	NewEntityViewHandler().RegisterHandlers(r)

	// The quests (quests.go): an NPC talked to asks them first, their boxes'
	// answers, the kills and places that do steps, the journal and the
	// player's own delete (0x007D); the GM panel edits them (questgm.go)
	if charStore != nil && gameWorld != nil {
		if qe, err := NewQuestEngine(charStore.DB(), gameWorld, sessionMgr, invStore, spellStore); err != nil {
			log.Printf("Quests: not running: %v", err)
		} else {
			qe.drop = npcWindowHandler.AddQuestDrop // a loot step's drops (questloot.go)
			qe.Start()
			r.Register(protocol.OpcodeDeleteQuest, qe.HandleDrop)
		}
	}

	// The rehabilitators (rehab.go): a character's masteries and stat points
	// reset and paths chosen again, the Rose of Rehabilitation sold, on tables
	// rehabsettings and charresets
	if charStore != nil && gameWorld != nil {
		interactHandler.rehab = NewRehab(charStore.DB(), gameWorld, invStore, spellStore, classMasteryHandler, charStore)
	}

	// The GMs' triggers (gmtriggers.go): scripted events the TEST menu's
	// TRIGGER lines, #trigger and the GM panel fire
	if charStore != nil && gameWorld != nil {
		NewTriggerEngine(charStore.DB(), gameWorld, sessionMgr, invStore).Start()
	}

	// Register others' power (0x7E01; powerbars.go): a client that draws
	// power bars under its group's, its target's and their target's health
	NewPowerBarsHandler(gameWorld).RegisterHandlers(r)
	NewQuestMarkersHandler(gameWorld).RegisterHandlers(r)
	NewQuestProgressHandler(gameWorld).RegisterHandlers(r)
	NewVoiceHandler(gameWorld).RegisterHandlers(r)
	// Duels (duel.go): the live window's 0x1501 and its offer's answer, and
	// their wagers held in duel_stakes (stakes a stopped server held go back)
	NewDuelHandler(gameWorld).RegisterHandlers(r)
	// The live game's look (0x7E0C; livelook.go): the answer to its offer,
	// and #dye's table (dye.go)
	NewLiveLookHandler(charHandler).RegisterHandlers(r)
	if charStore != nil && charStore.DB() != nil {
		chatHandler.SetDyeStore(charStore.DB())
	}
	if charStore != nil && gameWorld != nil {
		StartDuelPurse(gameWorld, charStore.DB())
	}

	// Register server handlers
	serverHandler.RegisterHandlers(r)

	return serverHandler
}
