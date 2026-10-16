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
	OpcodeDiscVersion GameOpcode = 0x0000 // Game Version check
	OpcodeAuthenticate2 GameOpcode = 0x0001 // Username/Auth (game server connection)
	OpcodeInteract      GameOpcode = 0x0004 // NPC/Object interaction
	OpcodeSendReport    GameOpcode = 0x0009 // Send Report (client)
	OpcodePrepareItem   GameOpcode = 0x000A // Prepare Item on Hotbar Request (client)
	OpcodeClientSayChat GameOpcode = 0x000B // Chat Input from Client
	OpcodeMemoryDump    GameOpcode = 0x000D // Memory Dump - entity data on world enter
	OpcodeChangeChatMode GameOpcode = 0x000E // Change chat mode
	OpcodeAttack        GameOpcode = 0x000F // Auto Attack

	// 0x001X - Targeting, Time, Loot, NPC Interaction
	OpcodeClassicTargetInfo    GameOpcode = 0x0010 // Classic Player Target Info
	OpcodeRandomName           GameOpcode = 0x0012 // Random name generation
	OpcodeTime                 GameOpcode = 0x0013 // Epoch Time stamps
	OpcodeCharacterInWorld     GameOpcode = 0x0014 // Character entered world
	OpcodeLootCorpse           GameOpcode = 0x0015 // Looting Corpse request
	OpcodeClientCloseLoot      GameOpcode = 0x0016 // Close loot window
	OpcodeLoot                 GameOpcode = 0x0018 // Loot Window (server)
	OpcodeClientLoot           GameOpcode = 0x0019 // Client loot item
	OpcodeLootResult           GameOpcode = 0x001A // Loot result (1=ok/2=partial/3=fail)
	OpcodeNPCInteraction       GameOpcode = 0x001C // NPC type dispatch (merchant/trainer/quest)
	OpcodeUpdateTrainingPoints GameOpcode = 0x001D // Spend and Remove Training Points

	// 0x002X - Character/Toon Opcodes
	OpcodeGrantXP         GameOpcode = 0x0020 // Receive XP
	OpcodePrepareWeapon   GameOpcode = 0x0021 // Prepare Weapon from Hotbar Request (client)
	OpcodeSelectedChar    GameOpcode = 0x002A // Character Select (client selects char)
	OpcodeCreateCharacter GameOpcode = 0x002B // Character Create (client creates char)
	OpcodeCharacterSelect GameOpcode = 0x002C // Character View (server sends char list)
	OpcodeDelCharacter    GameOpcode = 0x002D // Character Delete
	OpcodeWorldEntry      GameOpcode = 0x002E // World entry trigger (PS2 retail)
	OpcodeNameTaken       GameOpcode = 0x002F // Name already taken response

	// 0x003X - Skills, Quests, Inventory
	OpcodeUseSkill            GameOpcode = 0x0032 // Use Skill Request (client)
	OpcodeAddSkillToBar       GameOpcode = 0x0033 // Add Skill to Bar Request (client)
	OpcodeQuestPopupMulti     GameOpcode = 0x0034 // Quest Popup with multiple sections (server)
	OpcodeDialogueBoxOption   GameOpcode = 0x0035 // Client Popup selection
	OpcodeQuestStageComplete  GameOpcode = 0x0036 // Quest stage complete + XP reward
	OpcodeAddInvItem          GameOpcode = 0x003B // Add Inventory Item (server)
	OpcodeRemoveInvItem       GameOpcode = 0x003C // Delete Inventory Item Request (client)
	OpcodeArrangeItem         GameOpcode = 0x003D // Move Inventory Item Request (client)
	OpcodeInteractItem        GameOpcode = 0x003E // Unequip/Equip/Use Item Request (client)
	OpcodeEquipItem           GameOpcode = 0x003F // Server confirmation to equip

	// 0x004X - Pets, Quests, Merchants
	OpcodeUnequipItem         GameOpcode = 0x0040 // Unequip item
	OpcodePetCommands         GameOpcode = 0x0041 // Pet Commands
	OpcodeQuestDialogueResp   GameOpcode = 0x0045 // Quest dialogue option selected
	OpcodeDialogueBox         GameOpcode = 0x0046 // Quest Popup (server)
	OpcodeEnableChannel       GameOpcode = 0x0049 // Enable channel
	OpcodeMerchantBuy         GameOpcode = 0x004A // Buying item from Merchant
	OpcodeMerchantSell        GameOpcode = 0x004B // Selling item to Merchant
	OpcodeMerchantDiag        GameOpcode = 0x004C // Merchant Interaction

	// 0x005X - Trade, Tunar
	OpcodePlayerTunar GameOpcode = 0x0052 // Update Tunar on toon (server)
	OpcodeTradeUI     GameOpcode = 0x0056 // Trade UI
	OpcodeTradeRequest GameOpcode = 0x0057 // Trade Request (client)

	// 0x006X - Items, Quests
	OpcodeItemListUpdate GameOpcode = 0x0065 // Full item definitions (merchant/quest rewards)

	// 0x005X-006X - Items, Durability, Blacksmith
	OpcodeCharacterDied   GameOpcode = 0x005E // Character died
	OpcodeAdjustItemHP    GameOpcode = 0x0060 // Item HP (Durability)
	OpcodeBlackSmithMenu  GameOpcode = 0x0061 // Interact with Blacksmith / Open Repair Window
	OpcodeRequestRepair   GameOpcode = 0x0062 // Repair Item
	OpcodeCloseBlacksmithMenu GameOpcode = 0x0063 // "You are too far from the blacksmith"
	OpcodeHotkeyMenu      GameOpcode = 0x006C // Hotkey Menu (server)

	// 0x007X - Quests, Pets, Groups
	OpcodeUpdateQuestProgress GameOpcode = 0x007B // Quest progress update
	OpcodeAddQuestLog         GameOpcode = 0x007C // Add Quest to Log (server)
	OpcodeDeleteQuest         GameOpcode = 0x007D // Remove Quest from Log (client/server)
	OpcodeAddPetToGroup       GameOpcode = 0x007E // Add Pet to Group
	OpcodeClearQuestTracker   GameOpcode = 0x007F // Clear active quest tracker

	// 0x008X - LFG Options
	OpcodeLootOptions GameOpcode = 0x0085 // Loot options

	// 0x00BX - Combat, Skills, Merchants, Entities
	OpcodeCastingCombat        GameOpcode = 0x00B1 // Casting Combat
	OpcodeFullEntityCreate     GameOpcode = 0x00B3 // Entity spawn (PS2 retail: reliable entity creation)
	OpcodeSkillBarAck          GameOpcode = 0x00B4 // Add Skill to Bar Acknowledge (server)
	OpcodeEntityUpdate         GameOpcode = 0x00B5 // Entity property update (PS2 retail)
	OpcodeMerchantBox          GameOpcode = 0x00B7 // Buy Sell Window for Merchant
	OpcodeEntityDetailUpdate   GameOpcode = 0x00B8 // Entity detailed stats/inventory

	// 0x00CX - Weather, Cooldowns, Errors
	OpcodeWeatherControl   GameOpcode = 0x00C5 // Weather Control
	OpcodeSkyboxWeather    GameOpcode = 0x00C7 // Skybox Weather Control
	OpcodeRollRequest      GameOpcode = 0x00C8 // Roll Request
	OpcodeFactionMessages  GameOpcode = 0x00C9 // Faction messages
	OpcodeLootMessages     GameOpcode = 0x00CA // Loot messages
	OpcodeCooldownTimer    GameOpcode = 0x00CD // Cooldown Timer
	OpcodeErrorMessage     GameOpcode = 0x00CE // Chat Text Error

	// 0x00DX - Damage, NPC Messages, Misc
	OpcodeNPCMessage       GameOpcode = 0x00D3 // NPC speech: name(64) + type + text
	OpcodeInventoryFull    GameOpcode = 0x00D7 // Inventory full
	OpcodeHPResourceUpdate GameOpcode = 0x00D9 // HP/resource delta with buff/debuff type
	OpcodeDamageIndication GameOpcode = 0x00DB // Damage/Healing Indication
	OpcodeForceDisconnect  GameOpcode = 0x00EC // Force disconnect client

	// 0x00FX - Entity State, Speed
	OpcodeEntityStateRefresh  GameOpcode = 0x00FC // Entity state refresh (reconnect)
	OpcodeEntityDataUpdate    GameOpcode = 0x00FD // Entity data update with validation
	OpcodePlayerSpeed         GameOpcode = 0x00F8 // Player speed
	OpcodeFullEntityRefresh   GameOpcode = 0x00FF // Complete entity property reset
)

// =============================================================================
// 0x03XX - Buddy List
// =============================================================================

const (
	OpcodeBuddyRemove       GameOpcode = 0x0369 // Buddy list remove Request (client)
	OpcodeBuddyListEntry    GameOpcode = 0x03D5 // Buddy list entry (server sends per buddy)
	OpcodeBuddyOnlineStatus GameOpcode = 0x03D6 // Buddy online/offline notification (server)
	OpcodeBuddyListFull     GameOpcode = 0x03D7 // Buddy list full
	OpcodeAddBuddyFailed    GameOpcode = 0x03D8 // Add buddy failed
)

// =============================================================================
// 0x05XX - Guilds
// =============================================================================

const (
	OpcodeGuildInfo        GameOpcode = 0x0468 // Guild info (server→client, main guild data)
	OpcodeGuildCreate      GameOpcode = 0x0469 // Create guild (client→server)
	OpcodeGuildInvite      GameOpcode = 0x046A // Invite player (client→server)
	OpcodeGuildLeave       GameOpcode = 0x046B // Leave guild (client→server)
	OpcodeGuildKick        GameOpcode = 0x046C // Kick member (client→server)
	OpcodeGuildPromote     GameOpcode = 0x046D // Promote member (client→server)
	OpcodeGuildDemote      GameOpcode = 0x046E // Demote member (client→server)
	OpcodeGuildSetMOTD     GameOpcode = 0x046F // Set MOTD (client→server)
	OpcodeGuildWorldEntry  GameOpcode = 0x0579 // Guild world entry (PS2 retail: full subsystem init)
	OpcodeGuildInviteStuff GameOpcode = 0x057A // Guild invite stuff
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
	OpcodeAdminCode                  GameOpcode = 0x0700 // Admin Code
	OpcodeServerLoginResponse        GameOpcode = 0x0728 // Post-auth state setup (PS2 retail)
	OpcodeBadLoginPassword           GameOpcode = 0x0729 // Bad Login Password
	OpcodeNotSubscribed              GameOpcode = 0x072B // Not Subscribed
	OpcodeTarget                     GameOpcode = 0x0760 // Target
	OpcodeTargetInformation          GameOpcode = 0x0761 // Target Player Info
	OpcodeBuddyInvite                GameOpcode = 0x076F // Buddy Invite
	OpcodeGroupInviteAcceptedMessage GameOpcode = 0x0770 // Group Invite Accepted Message
	OpcodeBuddyListStuff             GameOpcode = 0x0771 // Buddy List Stuff
	OpcodeCreateGroup                GameOpcode = 0x0772 // Display Group Info
	OpcodeGroupInviteResponse        GameOpcode = 0x0773 // Group Invite
	OpcodeSomething2                 GameOpcode = 0x0774 // Unknown
	OpcodeResourceUpdate             GameOpcode = 0x0775 // Resource update (paired with DamageIndication on eqoa.live)
	OpcodeDisconnectNoError          GameOpcode = 0x0777 // Disconnect No Error
	OpcodeOutMatched                 GameOpcode = 0x077A // Outmatched warning
	OpcodeFactionPage               GameOpcode = 0x07E2 // Server Faction Page Response
	OpcodeEntityUpdateAudio          GameOpcode = 0x07A1 // Entity update + stop audio (PS2 retail)
	OpcodeEntityUpdateMinimal        GameOpcode = 0x07A2 // Entity update + cleanup (PS2 retail)
	OpcodeChatMessage                GameOpcode = 0x07A3 // Chat message (sender + metadata)
	OpcodeLoggedInFromAnotherLocationDC GameOpcode = 0x07A4 // Logged in from another location
	OpcodeZoneTransferInit           GameOpcode = 0x07B0 // Zone transfer init (PS2 retail)
	OpcodeZoneTransferConfirm        GameOpcode = 0x07B2 // Zone transfer confirm (PS2 retail)
	OpcodeGameServers                GameOpcode = 0x07B3 // Server Listing
	OpcodeEntityUpdateDirect         GameOpcode = 0x07C0 // Bare position/zone update (PS2 retail)
	OpcodeAdministratorShutdown      GameOpcode = 0x07C1 // Administrator Shutdown
	OpcodeBestEffortPing             GameOpcode = 0x07D0 // Best Effort Ping
	OpcodeCamera1                    GameOpcode = 0x07D1 // Server Unknown (camera related)
	OpcodeCharacterModifiedDisconnect GameOpcode = 0x07D2 // Character Modified Disconnect
	OpcodeServerReady                GameOpcode = 0x07E0 // Server ready/keepalive (no payload)
	// OpcodeConnStats is CLIENT->SERVER: the client's own DRDP counters as a
	// UTF-16 report (int32 charCount + UTF-16LE). Diagnostic only, no reply.
	OpcodeConnStats                  GameOpcode = 0x07E1 // Client connection statistics report
	OpcodeEntityUpdateFullUI         GameOpcode = 0x07F4 // HUD + map + full entity update (PS2 retail)
	OpcodeEntityUpdateMapPanel       GameOpcode = 0x07F0 // Map + entity update (PS2 retail)
	OpcodeWorldUnavailable           GameOpcode = 0x07F3 // World Unavailable
	OpcodeCamera2                    GameOpcode = 0x07F5 // Server Unknown (camera related)
	OpcodeTeleport                   GameOpcode = 0x00B2 // Coaching and Teleport (C# used 0x07F6, eqoa.live uses 0x00B2)
)

// =============================================================================
// 0x08XX - LFG System
// =============================================================================

const (
	OpcodeLFGShowHide   GameOpcode = 0x0801 // Show/Hide Group Chat + LFG commands
	OpcodeLFGUnknown    GameOpcode = 0x0840 // Unknown LFG
)

// =============================================================================
// 0x09XX - Authentication
// =============================================================================

const (
	OpcodeAuthenticate   GameOpcode = 0x0904 // Username/Auth (login server connection)
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
	OpcodeIgnoreList GameOpcode = 0x1005 // Ignore List
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
	case OpcodeWorldEntry:
		return "WorldEntry"
	case OpcodeFullEntityCreate:
		return "FullEntityCreate"
	case OpcodeEntityUpdate:
		return "EntityUpdate"
	case OpcodeEntityDetailUpdate:
		return "EntityDetailUpdate"
	case OpcodeEntityStateRefresh:
		return "EntityStateRefresh"
	case OpcodeEntityDataUpdate:
		return "EntityDataUpdate"
	case OpcodeFullEntityRefresh:
		return "FullEntityRefresh"
	case OpcodeEntityUpdateFullUI:
		return "EntityUpdateFullUI"
	case OpcodeEntityUpdateMapPanel:
		return "EntityUpdateMapPanel"
	case OpcodeEntityUpdateAudio:
		return "EntityUpdateAudio"
	case OpcodeEntityUpdateMinimal:
		return "EntityUpdateMinimal"
	case OpcodeEntityUpdateDirect:
		return "EntityUpdateDirect"
	case OpcodeServerLoginResponse:
		return "ServerLoginResponse"
	case OpcodeGuildWorldEntry:
		return "GuildWorldEntry"
	case OpcodeZoneTransferInit:
		return "ZoneTransferInit"
	case OpcodeZoneTransferConfirm:
		return "ZoneTransferConfirm"
	case OpcodeNPCInteraction:
		return "NPCInteraction"
	case OpcodeNPCMessage:
		return "NPCMessage"
	case OpcodeHPResourceUpdate:
		return "HPResourceUpdate"
	case OpcodeLootResult:
		return "LootResult"
	case OpcodeItemListUpdate:
		return "ItemListUpdate"
	case OpcodeForceDisconnect:
		return "ForceDisconnect"
	case OpcodeChatMessage:
		return "ChatMessage"
	case OpcodeTime:
		return "Time"
	case OpcodeGameServers:
		return "GameServers"
	case OpcodeBestEffortPing:
		return "BestEffortPing"
	case OpcodeCamera1:
		return "Camera1"
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
	case OpcodeGroupInviteResponse:
		return "GroupInviteResponse"
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
	case OpcodeIgnoreList:
		return "IgnoreList"
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
