package protocol

// GameOpcode represents a game protocol opcode.
type GameOpcode uint16

// PacketHeader special values.
const (
	PacketHeaderServerTransfer PacketHeader = 0xFFFF
	PacketHeaderNewConnection  PacketHeader = 0xFFFE
)

// PacketHeader represents special packet header values.
type PacketHeader uint16

// UnreliableTypes constants.
const (
	UnreliableTypeClientActorUpdate byte = 0x40
)

// =============================================================================
// 0x00XX - Core Game Opcodes
// =============================================================================

const (
	// 0x000X - Basic operations
	OpcodeDiscVersion    GameOpcode = 0x0000 // Game Version check
	OpcodeAuthenticate2  GameOpcode = 0x0001 // Username/Auth (game server connection)
	OpcodeInteract       GameOpcode = 0x0004 // NPC/Object interaction
	OpcodeSendReport     GameOpcode = 0x0009 // Send Report (client)
	OpcodePrepareItem    GameOpcode = 0x000A // Prepare Item on Hotbar Request (client)
	OpcodeClientSayChat  GameOpcode = 0x000B // Chat Input from Client
	OpcodeMemoryDump     GameOpcode = 0x000D // Memory Dump - entity data on world enter
	OpcodeChangeChatMode GameOpcode = 0x000E // Change chat mode
	OpcodeAttack         GameOpcode = 0x000F // Auto Attack

	// 0x001X - Targeting, Time, Loot, NPC Interaction
	OpcodeClassicTargetInfo    GameOpcode = 0x0010 // Classic Player Target Info
	OpcodeRandomName           GameOpcode = 0x0012 // Random name generation
	OpcodeTime                 GameOpcode = 0x0013 // Epoch Time stamps
	OpcodeCharacterInWorld     GameOpcode = 0x0014 // Character entered world
	OpcodeLootCorpse           GameOpcode = 0x0015 // Looting Corpse request
	OpcodeClientCloseLoot      GameOpcode = 0x0016 // Close loot window
	OpcodeLoot                 GameOpcode = 0x0018 // Loot Window (server)
	OpcodeClientLoot           GameOpcode = 0x0019 // Client loot item
	OpcodeLootTakeAll          GameOpcode = 0x001A // Msg001A 0x00BD98B0: no payload; every corpse item moves to the inventory
	OpcodePlayerP44            GameOpcode = 0x001C // one raw u32 into VIPlayer+0x44 (the stat-training window reads it with the CM count)
	OpcodeUpdateTrainingPoints GameOpcode = 0x001D // Spend and Remove Training Points

	// 0x002X - Character/Toon Opcodes
	OpcodeGrantXP          GameOpcode = 0x0020 // Receive XP
	OpcodePrepareWeapon    GameOpcode = 0x0021 // Prepare Weapon from Hotbar Request (client)
	OpcodeSelectedChar     GameOpcode = 0x002A // Character Select (client selects char)
	OpcodeCreateCharacter  GameOpcode = 0x002B // Character Create (client creates char)
	OpcodeCharacterSelect  GameOpcode = 0x002C // Character View (server sends char list)
	OpcodeDelCharacter     GameOpcode = 0x002D // Character Delete
	OpcodeCreateCharFailed GameOpcode = 0x002E // no payload: the login window shows word 269 "Character create failed"
	OpcodeNameTaken        GameOpcode = 0x002F // Name already taken response

	// 0x003X - Skills, Quests, Inventory
	OpcodeUseSkill          GameOpcode = 0x0032 // Use Skill Request (client)
	OpcodeAddSkillToBar     GameOpcode = 0x0033 // Add Skill to Bar Request (client)
	OpcodeQuestPopupMulti   GameOpcode = 0x0034 // Quest Popup with multiple sections (server)
	OpcodeDialogueBoxOption GameOpcode = 0x0035 // Client Popup selection
	OpcodeTrainingPoints    GameOpcode = 0x0036 // u8 sound flag + zigzag TOTAL training points (VIPlayer+0x40); a change shows word 625
	OpcodeAddInvItem        GameOpcode = 0x003B // Add Inventory Item (server)
	OpcodeRemoveInvItem     GameOpcode = 0x003C // Delete Inventory Item Request (client)
	OpcodeArrangeItem       GameOpcode = 0x003D // Move Inventory Item Request (client)
	OpcodeInteractItem      GameOpcode = 0x003E // Unequip/Equip/Use Item Request (client)
	OpcodeEquipItem         GameOpcode = 0x003F // Server confirmation to equip

	// 0x004X - Pets, Quests, Merchants
	OpcodeUnequipItem     GameOpcode = 0x0040 // Unequip item
	OpcodePetCommands     GameOpcode = 0x0041 // Pet Commands
	OpcodeProfanityFilter GameOpcode = 0x0045 // Msg0045: zigzag on/off, shown as "Profanity Filter: On/Off" (550-552)
	OpcodeDialogueBox     GameOpcode = 0x0046 // Quest Popup (server)
	OpcodeEnableChannel   GameOpcode = 0x0049 // Enable channel
	OpcodeMerchantBuy     GameOpcode = 0x004A // Buying item from Merchant
	OpcodeMerchantSell    GameOpcode = 0x004B // Selling item to Merchant
	OpcodeMerchantDiag    GameOpcode = 0x004C // Merchant Interaction

	// 0x005X - Trade, Tunar
	OpcodePlayerTunar  GameOpcode = 0x0052 // Update Tunar on toon (server)
	OpcodeTradeUI      GameOpcode = 0x0056 // Trade UI
	OpcodeTradeRequest GameOpcode = 0x0057 // Trade Request (client)

	// 0x006X - Items, Quests
	OpcodeCreateCharBadName GameOpcode = 0x0065 // no payload: word 828 "Failed: invalid name"

	// 0x005X-006X - Items, Durability, Blacksmith
	OpcodeCharacterDied       GameOpcode = 0x005E // Character died
	OpcodeAdjustItemHP        GameOpcode = 0x0060 // Item HP (Durability)
	OpcodeBlackSmithMenu      GameOpcode = 0x0061 // Interact with Blacksmith / Open Repair Window
	OpcodeRequestRepair       GameOpcode = 0x0062 // Repair Item
	OpcodeCloseBlacksmithMenu GameOpcode = 0x0063 // "You are too far from the blacksmith"
	OpcodeHotkeyMenu          GameOpcode = 0x006C // Hotkey Menu (server)

	// 0x007X - Quests, Pets, Groups
	OpcodeAssistTarget GameOpcode = 0x007B // Msg007B: u32 entity id to target (and ask 0x0010 about); -1 shows word 838
	OpcodeAddQuestLog  GameOpcode = 0x007C // Add Quest to Log (server)
	OpcodeDeleteQuest  GameOpcode = 0x007D // Remove Quest from Log (client/server)
	OpcodeSetPet       GameOpcode = 0x007E // one raw u32 into m_petId
	OpcodeClearPet     GameOpcode = 0x007F // no payload: m_petId = -1, the pet name cleared

	// 0x008X - LFG Options
	OpcodeLootOptions GameOpcode = 0x0085 // Loot options

	// 0x00BX - Combat, Skills, Merchants, Entities
	OpcodeCastingCombat      GameOpcode = 0x00B1 // Casting Combat
	OpcodeAbilityAdd         GameOpcode = 0x00B3 // Msg00B3: u32 stamp, u32 announce flag (word 721), one VIAbility record
	OpcodeAbilityMemorize    GameOpcode = 0x00B4 // Msg00B4: the echo of 0x0033 -- stamp, u8 display index, u8 slot; toggles memorized
	OpcodeAbilitySlotMove    GameOpcode = 0x00B5 // Msg00B5: stamp, u8 from slot, u8 to slot
	OpcodeAbilityDisplayMove GameOpcode = 0x00B6 // Msg00B6: stamp, u8 from, u8 to -- the same move on the display index
	OpcodeMerchantBox        GameOpcode = 0x00B7 // Buy Sell Window for Merchant
	OpcodeAbilityRemove      GameOpcode = 0x00B8 // Msg00B8: stamp, zigzag display index -- erases that ability

	// 0x00CX - Weather, Cooldowns, Errors
	OpcodeWeatherControl  GameOpcode = 0x00C5 // Weather Control
	OpcodeSkyboxWeather   GameOpcode = 0x00C7 // Skybox Weather Control
	OpcodeRollRequest     GameOpcode = 0x00C8 // Roll Request
	OpcodeFactionMessages GameOpcode = 0x00C9 // Faction messages
	OpcodeLootMessages    GameOpcode = 0x00CA // Loot messages
	OpcodeCooldownTimer   GameOpcode = 0x00CD // Cooldown Timer
	OpcodeErrorMessage    GameOpcode = 0x00CE // Chat Text Error

	// 0x00DX - Damage, NPC Messages, Misc
	OpcodeSummonPrompt     GameOpcode = 0x00D3 // Msg00D3: ReadString(name, 64) + u8 (0 summon, word 610; else resurrect, 611)
	OpcodeCancelPending    GameOpcode = 0x00D7 // Msg00D7: clears the merchant/loot request flags and closes the bank; shows nothing
	OpcodeExpDebt          GameOpcode = 0x00D9 // one raw u32 into the player record +0x2C (FrontiersForge: exp debt, unconfirmed)
	OpcodeDamageIndication GameOpcode = 0x00DB // Damage/Healing Indication
	OpcodeForceDisconnect  GameOpcode = 0x00EC // Disconnect(8): word 619 "You have logged in from another location."

	// 0x00FX - Entity State, Speed
	OpcodeMoveLock        GameOpcode = 0x00FC // u8 into VIClient+0x6DC8; while non-zero the walker zeroes forward speed
	OpcodeAbilitySetP14   GameOpcode = 0x00FD // Msg00FD: stamp, u32 ability id, u32 value for VIAbility+0x14 (0 = off the bar)
	OpcodePlayerSpeed     GameOpcode = 0x00F8 // Player speed
	OpcodeAbilityClearBar GameOpcode = 0x00FF // Msg00FF: stamp; every ability (display < 0xFC) off the bar -- the answer to 0x00FE
)

// =============================================================================
// 0x03XX - Buddy List
// =============================================================================

const (
	OpcodeBuddyRemove    GameOpcode = 0x0369 // Buddy list remove Request (client)
	OpcodeBuddyListEntry GameOpcode = 0x03D5 // Buddy list entry (server sends per buddy)
	OpcodeBuddyRemoved   GameOpcode = 0x03D6 // zigzag buddy id: erased from the buddy list
	OpcodeBuddyListFull  GameOpcode = 0x03D7 // Buddy list full
	OpcodeAddBuddyFailed GameOpcode = 0x03D8 // Add buddy failed
)

// =============================================================================
// 0x05XX - Guilds
// =============================================================================

const (
	OpcodeGuildInfo          GameOpcode = 0x0468 // Guild info (server→client, main guild data)
	OpcodeGuildCreate        GameOpcode = 0x0469 // Create guild (client→server)
	OpcodeGuildInvite        GameOpcode = 0x046A // Invite player (client→server)
	OpcodeGuildLeave         GameOpcode = 0x046B // Leave guild (client→server)
	OpcodeGuildKick          GameOpcode = 0x046C // Kick member (client→server)
	OpcodeGuildPromote       GameOpcode = 0x046D // Promote member (client→server)
	OpcodeGuildDemote        GameOpcode = 0x046E // Demote member (client→server)
	OpcodeGuildSetMOTD       GameOpcode = 0x046F // Set MOTD (client→server)
	OpcodeGuildCommandFailed GameOpcode = 0x0579 // a failed guild command (words 566-568), not a world entry
	OpcodeGuildInviteStuff   GameOpcode = 0x057A // Guild invite stuff
)

// =============================================================================
// 0x06XX - Groups
// =============================================================================

const (
	OpcodeAcceptGroupInvite  GameOpcode = 0x0622 // Accept Group Invite
	OpcodeLeaveGroup         GameOpcode = 0x0623 // Leave Group
	OpcodeDeclineGroupInvite GameOpcode = 0x0624 // Decline Group Invite
	OpcodeGroupInvite        GameOpcode = 0x0625 // Group Invite Request (client)
	OpcodeBootGroupMember    GameOpcode = 0x0626 // Boot Group Member
	OpcodeDisbandGroup       GameOpcode = 0x0627 // Disband Group
	OpcodeServerDisbandGroup GameOpcode = 0x062F // Server Disband Group
	OpcodeRemoveGroupMember  GameOpcode = 0x069A // Remove Group Member
	OpcodeGroupStuff         GameOpcode = 0x069C // Group Stuff
	OpcodeGroupChat          GameOpcode = 0x069E // Group Chat
)

// =============================================================================
// 0x07XX - Server Messages, Camera, Errors
// =============================================================================

const (
	OpcodeAdminCode                     GameOpcode = 0x0700 // Admin Code
	OpcodeServerNotReadyDC              GameOpcode = 0x0728 // Disconnect(6): word 620 "Server not ready, please try again later. (nodbs)"
	OpcodeServerNotReadyLoginDC         GameOpcode = 0x072A // Disconnect(7): word 621 "Server not ready, please try again later. (loginto)"
	OpcodeBadLoginPassword              GameOpcode = 0x0729 // Disconnect(3): word 270 "Bad Login/Password"
	OpcodeNotSubscribed                 GameOpcode = 0x072B // Disconnect(4): word 593 "Not Subscribed."
	OpcodeTarget                        GameOpcode = 0x0760 // Target
	OpcodeTargetInformation             GameOpcode = 0x0761 // Target Player Info
	OpcodeBuddyInvite                   GameOpcode = 0x076F // Buddy Invite
	OpcodeGroupInviteAcceptedMessage    GameOpcode = 0x0770 // Group Invite Accepted Message
	OpcodeBuddyListStuff                GameOpcode = 0x0771 // Buddy List Stuff
	OpcodeCreateGroup                   GameOpcode = 0x0772 // Display Group Info
	OpcodeGroupInviteReceived           GameOpcode = 0x0773 // Msg0773: u32 invite id, u32, ReadString(name, 24); grouped = auto-decline
	OpcodeGroupInviteCancelled          GameOpcode = 0x0774 // Msg0774: the invitation was withdrawn
	OpcodeEntityAction                  GameOpcode = 0x0775 // u32 entity id + u8 VICSprite action; for the player only 34 plays
	OpcodeDisconnectNoError             GameOpcode = 0x0777 // Disconnect No Error
	OpcodeOutMatched                    GameOpcode = 0x077A // Outmatched warning
	OpcodeFactionPage                   GameOpcode = 0x07E2 // Server Faction Page Response
	OpcodeZoneUnavailableDC             GameOpcode = 0x07A1 // Msg07A1: 8 bytes, then Disconnect(13): word 677 (the area is temporarily unavailable)
	OpcodeAdminEjectDC                  GameOpcode = 0x07A2 // Disconnect(14): word 716 "You have been ejected by a server administrator."
	OpcodeReplyName                     GameOpcode = 0x07A3 // ReadString(name, 24): the name /r replies to
	OpcodeLoggedInFromAnotherLocationDC GameOpcode = 0x07A4 // Disconnect(8): word 619, as 0x00EC
	OpcodeZoneTransferInit              GameOpcode = 0x07B0 // Recv07B0 0x00BE71B0: a sub-type byte, six sub-handlers (not decompiled)
	OpcodeZoneTransferConfirm           GameOpcode = 0x07B2 // one raw u32 into +0x7578 (the constructor sets 100); meaning unknown
	OpcodeGameServers                   GameOpcode = 0x07B3 // Server Listing
	OpcodeServerUpdatingDC              GameOpcode = 0x07C0 // Disconnect(17): word 718 "We are updating the game, please try again later."
	OpcodeAdministratorShutdown         GameOpcode = 0x07C1 // Disconnect(18): word 718 "We are updating the game, please try again later."
	OpcodeBestEffortPing                GameOpcode = 0x07D0 // Best Effort Ping
	OpcodeExpansionFlags                GameOpcode = 0x07D1 // u32 into +0x50304, ANDed with the disc's expansion flags (race camera, frame rate)
	OpcodeCharacterModifiedDisconnect   GameOpcode = 0x07D2 // Disconnect(19): word 1420 (modified by a customer service representative)
	OpcodeServerReady                   GameOpcode = 0x07E0 // Server ready/keepalive (no payload)
	// OpcodeConnStats is CLIENT->SERVER: the client's own DRDP counters as a
	// UTF-16 report (int32 charCount + UTF-16LE). Diagnostic only, no reply.
	OpcodeConnStats                 GameOpcode = 0x07E1 // Client connection statistics report
	OpcodeCharSelectNeedsFrontiers  GameOpcode = 0x07F4 // char select: word 1662 (that character needs the Frontiers disc); 0x07F2 too
	OpcodeCharSelectAreaUnavailable GameOpcode = 0x07F0 // char select: word 676 (the character's area is unavailable); 0x07F1/0x07F3 too
	OpcodeWorldUnavailable          GameOpcode = 0x07F3 // char select: word 676, as 0x07F0
	OpcodeCamera2                   GameOpcode = 0x07F5 // Server Unknown (camera related)
	OpcodeTeleport                  GameOpcode = 0x00B2 // Msg00B2: u8 world, pos, HPR, u32 token -- Respawn, answered by 0x0049 = zigzag(token)
)

// =============================================================================
// 0x08XX - LFG System
// =============================================================================

const (
	OpcodeLFGShowHide GameOpcode = 0x0801 // Show/Hide Group Chat + LFG commands
	OpcodeLFGUnknown  GameOpcode = 0x0840 // Unknown LFG
)

// =============================================================================
// 0x09XX - Authentication
// =============================================================================

const (
	OpcodeAuthenticate     GameOpcode = 0x0904 // Username/Auth (login server connection)
	OpcodeDisconnectClient GameOpcode = 0x09B0 // Disconnect Client
)

// =============================================================================
// 0x0AXX - Chat Messages
// =============================================================================

const (
	OpcodeClientMessage GameOpcode = 0x0A7A // Client Message
	OpcodeColoredChat   GameOpcode = 0x0A7B // Message (Colored Text)
	OpcodeClientFaction GameOpcode = 0x0AC4 // View Faction Request (client)
)

// =============================================================================
// 0x0CXX - Shout
// =============================================================================

const (
	OpcodeClientShout GameOpcode = 0x0C01 // Client Shout
)

// =============================================================================
// 0x0EXX - Who List
// =============================================================================

const (
	OpcodeWhoList         GameOpcode = 0x0E01 // Who List Request (client)
	OpcodeWhoListResponse GameOpcode = 0x0E02 // Who List Generate (Server)
)

// =============================================================================
// 0x10XX - Ignore List
// =============================================================================

const (
	OpcodeNoCollideList   GameOpcode = 0x1005 // VIClientPart38300: a counted list of non-collideable names, appended
	OpcodeNoCollideAdd    GameOpcode = 0x1003 // one {name(32), id} added to the non-collideable list
	OpcodeNoCollideRemove GameOpcode = 0x1004 // one entry removed by id
)

// =============================================================================
// 0x12XX - Banking, Auctions
// =============================================================================

const (
	OpcodeAuctionStuff     GameOpcode = 0x1206 // Won Auction item Info
	OpcodeBankUI           GameOpcode = 0x124D // Bank UI
	OpcodeRemoveBankItem   GameOpcode = 0x1250 // Response to withdraw Item from bank (server)
	OpcodeAddBankItem      GameOpcode = 0x1251 // Auction Item Win / Add bank item
	OpcodeConfirmBankTunar GameOpcode = 0x1253 // Tunar in Bank Update (server)
	OpcodeBankItem         GameOpcode = 0x1254 // Withdraw Item from bank Request (client)
	OpcodeDepositBankTunar GameOpcode = 0x1255 // Withdraw Tunar from bank Request (client)
)

// =============================================================================
// 0x14XX - Class Mastery
// =============================================================================

const (
	OpcodeClassMastery       GameOpcode = 0x1401 // Class Mastery Request (client)
	OpcodeClassMasteryServer GameOpcode = 0x1402 // Class Mastery Response (server)
)

// =============================================================================
// 0x15XX - Dueling
// =============================================================================

const (
	OpcodeDuelRequest GameOpcode = 0x1501 // Duel Request (client)
)

// =============================================================================
// Game Version Constants
// =============================================================================

const (
	GameVersionEQOAFrontiers = 0x25
	GameVersionEQOAVanilla   = 0x12
	// GameVersionSandstormS2 is what a client running the Sandstorm S2 patch
	// reports on its SECOND session — the world-transfer / session-handoff one
	// that follows character select. S2 rewrites `li $v0, 18` to `li $v0, 19` at
	// CLIENT 0x00BC1EC8, in the outgoing write at 0x00BC1DCC, so the handshake
	// self-identifies as patched (S2 also appends two u32 fields there, which
	// the DiscVersion handler ignores — it reads only the leading byte).
	//
	// The first session still reports 0x12, so an unpatched and a patched client
	// are indistinguishable until after character select. Rejecting 0x13 answers
	// GameVersionUnknown and the client shows string 719, "We are updating the
	// game, please try again later." — which reads like a patch-server problem
	// and is not one.
	GameVersionSandstormS2 = 0x13
	GameVersionUnknown     = 0x00
)

// =============================================================================
// Session Opcodes
// =============================================================================

const (
	SessionOpcodeNewSession      int8 = 0x21
	SessionOpcodeCloseSession    int8 = 0x14
	SessionOpcodeContinueSession int8 = 0x01
)

// =============================================================================
// Bundle Opcodes
// =============================================================================

const (
	BundleOpcodeProcessAll              byte = 99
	BundleOpcodeProcessReport           byte = 35
	BundleOpcodeProcessMessages         byte = 32
	BundleOpcodeProcessMessageAndReport byte = 19
	BundleOpcodeNewProcessReport        byte = 3
	BundleOpcodeNewProcessMessages      byte = 0
)

// String returns the string representation of an opcode.
func (op GameOpcode) String() string {
	switch op {
	case OpcodeDiscVersion:
		return "DiscVersion"
	case OpcodeAuthenticate:
		return "Authenticate"
	case OpcodeAuthenticate2:
		return "Authenticate2"
	case OpcodeRandomName:
		return "RandomName"
	case OpcodeCharacterSelect:
		return "CharacterSelect"
	case OpcodeSelectedChar:
		return "SelectedChar"
	case OpcodeCreateCharacter:
		return "CreateCharacter"
	case OpcodeDelCharacter:
		return "DelCharacter"
	case OpcodeNameTaken:
		return "NameTaken"
	case OpcodeCharacterInWorld:
		return "CharacterInWorld"
	case OpcodeMemoryDump:
		return "MemoryDump"
	case OpcodeCreateCharFailed:
		return "CreateCharFailed"
	case OpcodeAbilityAdd:
		return "AbilityAdd"
	case OpcodeAbilitySlotMove:
		return "AbilitySlotMove"
	case OpcodeAbilityRemove:
		return "AbilityRemove"
	case OpcodeMoveLock:
		return "MoveLock"
	case OpcodeAbilitySetP14:
		return "AbilitySetP14"
	case OpcodeAbilityClearBar:
		return "AbilityClearBar"
	case OpcodeCharSelectNeedsFrontiers:
		return "CharSelectNeedsFrontiers"
	case OpcodeCharSelectAreaUnavailable:
		return "CharSelectAreaUnavailable"
	case OpcodeZoneUnavailableDC:
		return "ZoneUnavailableDC"
	case OpcodeAdminEjectDC:
		return "AdminEjectDC"
	case OpcodeServerUpdatingDC:
		return "ServerUpdatingDC"
	case OpcodeServerNotReadyDC:
		return "ServerNotReadyDC"
	case OpcodeGuildCommandFailed:
		return "GuildCommandFailed"
	case OpcodeZoneTransferInit:
		return "ZoneTransferInit"
	case OpcodeZoneTransferConfirm:
		return "ZoneTransferConfirm"
	case OpcodePlayerP44:
		return "PlayerP44"
	case OpcodeSummonPrompt:
		return "SummonPrompt"
	case OpcodeExpDebt:
		return "ExpDebt"
	case OpcodeLootTakeAll:
		return "LootTakeAll"
	case OpcodeCreateCharBadName:
		return "CreateCharBadName"
	case OpcodeForceDisconnect:
		return "ForceDisconnect"
	case OpcodeReplyName:
		return "ReplyName"
	case OpcodeTime:
		return "Time"
	case OpcodeGameServers:
		return "GameServers"
	case OpcodeBestEffortPing:
		return "BestEffortPing"
	case OpcodeExpansionFlags:
		return "ExpansionFlags"
	case OpcodeCamera2:
		return "Camera2"
	case OpcodeTeleport:
		return "Teleport"
	case OpcodeClientSayChat:
		return "ClientSayChat"
	case OpcodeClientShout:
		return "ClientShout"
	case OpcodeColoredChat:
		return "ColoredChat"
	case OpcodeClientMessage:
		return "ClientMessage"
	case OpcodeTarget:
		return "Target"
	case OpcodeTargetInformation:
		return "TargetInformation"
	case OpcodeAttack:
		return "Attack"
	case OpcodeInteract:
		return "Interact"
	case OpcodeDisconnectClient:
		return "DisconnectClient"
	case OpcodeGroupInvite:
		return "GroupInvite"
	case OpcodeCreateGroup:
		return "CreateGroup"
	case OpcodeGroupInviteReceived:
		return "GroupInviteReceived"
	case OpcodeWhoList:
		return "WhoList"
	case OpcodeWhoListResponse:
		return "WhoListResponse"
	case OpcodeLoot:
		return "Loot"
	case OpcodeGrantXP:
		return "GrantXP"
	case OpcodePlayerTunar:
		return "PlayerTunar"
	case OpcodeEquipItem:
		return "EquipItem"
	case OpcodeUnequipItem:
		return "UnequipItem"
	case OpcodeAddInvItem:
		return "AddInvItem"
	case OpcodeRemoveInvItem:
		return "RemoveInvItem"
	case OpcodeMerchantBuy:
		return "MerchantBuy"
	case OpcodeMerchantSell:
		return "MerchantSell"
	case OpcodeMerchantBox:
		return "MerchantBox"
	case OpcodeDialogueBox:
		return "DialogueBox"
	case OpcodeAddQuestLog:
		return "AddQuestLog"
	case OpcodeDeleteQuest:
		return "DeleteQuest"
	case OpcodeBankUI:
		return "BankUI"
	case OpcodeWeatherControl:
		return "WeatherControl"
	case OpcodeSkyboxWeather:
		return "SkyboxWeather"
	case OpcodeDuelRequest:
		return "DuelRequest"
	case OpcodeNoCollideList:
		return "NoCollideList"
	case OpcodeFactionPage:
		return "FactionPage"
	case OpcodeClientFaction:
		return "ClientFaction"
	case OpcodeFactionMessages:
		return "FactionMessages"
	default:
		return "Unknown"
	}
}
