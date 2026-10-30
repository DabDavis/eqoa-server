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
	OpcodePrepareItem    GameOpcode = 0x000A // an item onto a HUD quick slot: client u32 key + u8 slot, server u8 list index + u8 slot (Msg000A)
	OpcodeClientSayChat  GameOpcode = 0x000B // Chat Input from Client
	OpcodeMemoryDump     GameOpcode = 0x000D // Memory Dump - entity data on world enter
	OpcodeChangeChatMode GameOpcode = 0x000E // Change chat mode
	OpcodeAttack         GameOpcode = 0x000F // Auto Attack

	// The emotes, client -> server, bare (CLIENT's PlayEmote, 0x00BCFC38): the
	// client plays the action on itself; the server shows it to the others.
	OpcodeEmoteWave  GameOpcode = 0x001E // action 56
	OpcodeEmoteBow   GameOpcode = 0x0042 // action 57
	OpcodeEmotePoint GameOpcode = 0x0043 // action 60
	OpcodeEmoteCheer GameOpcode = 0x0044 // action 61

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
	OpcodePrepareWeapon    GameOpcode = 0x0021 // a weapon set (SOE's live client, not the disc): client u8 set + name, server u8 set + name + u32 main + u32 off (gearsets.go)
	OpcodeUseWeaponSet     GameOpcode = 0x0022 // the client takes up weapon set u8 (0-3, 4 bare hands; SOE's live client)
	OpcodeSelectedChar     GameOpcode = 0x002A // Character Select (client selects char)
	OpcodeCreateCharacter  GameOpcode = 0x002B // Character Create (client creates char)
	OpcodeCharacterSelect  GameOpcode = 0x002C // Character View (server sends char list)
	OpcodeDelCharacter     GameOpcode = 0x002D // Character Delete
	OpcodeCreateCharFailed GameOpcode = 0x002E // no payload: the login window shows word 269 "Character create failed"
	OpcodeNameTaken        GameOpcode = 0x002F // Name already taken response

	// 0x003X - Skills, Quests, Inventory
	OpcodeUseSkill          GameOpcode = 0x0032 // Use Skill Request (client)
	OpcodeAddSkillToBar     GameOpcode = 0x0033 // Add Skill to Bar Request (client)
	OpcodeArrangeAbility    GameOpcode = 0x003A // Send003A: stamp, u8 from display index, u8 to (client; answered by 0x00B6)
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
	OpcodeAssistTarget GameOpcode = 0x007B // out, Msg007B: u32 entity id to target (and ask 0x0010 about); -1 shows word 838. In: Assist Leader (DoAction 0x0B090000), a u32, the sender's target
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
	OpcodeAssist          GameOpcode = 0x00CB // in: Assist Target (DoAction 0x0B0E0006), a u32, the sender's target; answered by 0x007B
	OpcodeCooldownTimer   GameOpcode = 0x00CD // Cooldown Timer
	OpcodeErrorMessage    GameOpcode = 0x00CE // Chat Text Error

	// 0x00DX - Damage, NPC Messages, Misc
	OpcodeSummonPrompt     GameOpcode = 0x00D3 // Msg00D3: ReadString(name, 64) + u8 (0 summon, word 610; else resurrect, 611)
	OpcodeCancelPending    GameOpcode = 0x00D7 // Msg00D7: clears the merchant/loot request flags and closes the bank; shows nothing
	OpcodeSwingOutOfReach  GameOpcode = 0x00D8 // client -> server, bare: AutoAttackTick swung at a target out of reach (retries in 2 s)
	OpcodeExpDebt          GameOpcode = 0x00D9 // one raw u32 into the player record +0x2C (FrontiersForge: exp debt, unconfirmed)
	OpcodeDamageIndication GameOpcode = 0x00DB // Damage/Healing Indication
	OpcodeForceDisconnect  GameOpcode = 0x00EC // Disconnect(8): word 619 "You have logged in from another location."

	// 0x00FX - Entity State, Speed
	OpcodeMoveLock        GameOpcode = 0x00FC // u8 into VIClient+0x6DC8; while non-zero the walker zeroes forward speed
	OpcodeAbilitySetP14   GameOpcode = 0x00FD // Msg00FD: stamp, u32 ability id, u32 value for VIAbility+0x14 (0 = off the bar)
	OpcodePlayerSpeed     GameOpcode = 0x00F8 // Player speed
	OpcodeUnmemorizeAll   GameOpcode = 0x00FE // Send00FE: stamp (client; answered by 0x00FF)
	OpcodeAbilityClearBar GameOpcode = 0x00FF // Msg00FF: stamp; every ability (display < 0xFC) off the bar -- the answer to 0x00FE
)

// =============================================================================
// 0x03XX - Buddy List
// =============================================================================

const (
	OpcodeBuddyAdd       GameOpcode = 0x0366 // VIClient::AddBuddy: string name; the server asks that player (0x076F)
	OpcodeBuddyAccept    GameOpcode = 0x0367 // AnswerBuddyRequest: u32 request id (0x076F's)
	OpcodeBuddyRefuse    GameOpcode = 0x0368 // RejectBuddyRequest / Msg076F: u32 request id, u8 reason (1 busy, 2 refused, 8 list full)
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
	OpcodeAdminCode                     GameOpcode = 0x0700 // Admin Code: the GM's channel (handlers/gmchannel.go)
	OpcodeGMCommand                     GameOpcode = 0x0701 // a GM's TEST menu command (client)
	OpcodeGMTrigger                     GameOpcode = 0x0702 // the TEST menu's TRIGGER (client)
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
	OpcodeServerUpdatingDC              GameOpcode = 0x07C0 // Disconnect(17): word 1024 "The server has been shutdown by a server administrator."
	OpcodeAdministratorShutdown         GameOpcode = 0x07C1 // Disconnect(18): word 1024 "The server has been shutdown by a server administrator."
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
	// OpcodeLiveFeatures (once "Camera2", the C# reference's 0x1B) is SOE's live
	// client's server feature word: a u32 into VIClient+0x813C (live
	// 0x012C9548, read by GetFeatures 0x012C79E0), 0x1B in both retail save
	// states: 0x01 the auction's menu entries, 0x02 Ogre and Alchemist in the
	// pickers, 0x08 the auction's stacks, 0x10 the duel part's challenge and
	// surrender. The disc has no case. Sent once, by handlers/livefeatures.go.
	OpcodeLiveFeatures GameOpcode = 0x07F5
	OpcodeTeleport     GameOpcode = 0x00B2 // Msg00B2: u8 world, pos, HPR, u32 token -- Respawn, answered by 0x0049 = zigzag(token)
)

// =============================================================================
// 0x08XX - LFG System
// =============================================================================

// SOE's live client's group search (VIWndGroupSearch; handlers/lfg.go): its
// part at VIClient+0x308F8 is the disc's VIClientPart2EFC8 grown, and sends
// 0x0801 {u8 sub, ...} where the disc sends 0x0B01 {u8 1, u8 show}: sub 1 the
// Group menu's Show/Hide Chat, 2 register, 3 unregister, 4 a search. Its
// answer is 0x0840 {u8 0x40, zz count, the rows}. A PS2 never sends 0x0801
// and has no case for 0x0840.
const (
	OpcodeLFG        GameOpcode = 0x0801 // live: sub 1 show/hide group chat, 2 register, 3 unregister, 4 search
	OpcodeLFGResults GameOpcode = 0x0840 // live: sub 0x40, a search's players
	OpcodeGroupShow  GameOpcode = 0x0B01 // disc: {u8 1, u8 show}, the Group menu's Show Chat (1) / Hide Chat (0)
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
	OpcodeIgnoreAdd       GameOpcode = 0x1000 // VIClientPart38300::Add: string name (VIWndIgnoreList's Add Target)
	OpcodeIgnoreRemove    GameOpcode = 0x1002 // VIClientPart38300::RemoveAt: u32 id, raw
	OpcodeNoCollideList   GameOpcode = 0x1005 // VIClientPart38300: a counted list of non-collideable names, appended
	OpcodeNoCollideAdd    GameOpcode = 0x1003 // one {name(32), id} added to the non-collideable list
	OpcodeNoCollideRemove GameOpcode = 0x1004 // one entry removed by id
)

// =============================================================================
// 0x12XX - Banking, Auctions
// =============================================================================

const (
	OpcodeAuctionRequest   GameOpcode = 0x1205 // VIBankControl's auction requests: a sub-op byte (5 sell .. 9)
	OpcodeAuctionStuff     GameOpcode = 0x1206 // the auction's replies: sub-ops 128..137 (Recv1206)
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

// SOE's live client's duel (eqoa-live-1500, entity/duelwire.go): 0x1501 up,
// 0x1502 down, each {u8 sub, u32 duel id, ...}. The Frontiers disc has
// neither. Sandstorm's S2 player search runs the other way (0x1502 up,
// 0x1500/0x1501 down), so neither direction clashes.
const (
	OpcodeDuelRequest GameOpcode = 0x1501 // client: challenge, decline, accept, surrender, the wager's
	OpcodeDuel        GameOpcode = 0x1502 // server: the invitation, its answers, the duel's state
)

// =============================================================================
// Not retail: this server's and its clients' own
// =============================================================================

const (
	// OpcodeEntityView is what a client keeps in view (session/entityslots.go):
	// how many entity slots and how far, two u32s after the opcode, both ways.
	// The server offers the most it will fill and the farthest it will look
	// once the player is in the world, and a client that keeps more than
	// CLIENT's answers with its own two (any time after, again whenever they
	// change). No retail message has it: CLIENT's dispatcher handles nothing
	// above 0x1402 and has no default case, and S2's own are 0x1500-0x1507, so
	// a PS2 ignores the offer and never asks.
	OpcodeEntityView GameOpcode = 0x7E00

	// OpcodePowerBars is others' power for the bars a client draws under
	// their health (entity/powerbars.go): a u32 after the opcode, both ways.
	// The server offers it (1) once the player is in the world; a client
	// that draws the bars answers 1 to have them and 0 to stop, at any time
	// after. A PS2 ignores the offer as it ignores OpcodeEntityView's.
	OpcodePowerBars GameOpcode = 0x7E01

	// OpcodePowerBarsData is the power, server to a client that asked: a
	// u8 count, then for each a u32 id (as that client knows the entity)
	// and a u8 power of 255. Each one is the whole set: an id it leaves out
	// has no bar.
	OpcodePowerBarsData GameOpcode = 0x7E02

	// OpcodeQuestMarkers is the quest marks a client draws on its map
	// (handlers/questmarks.go): a u32 after the opcode, both ways, as
	// OpcodePowerBars: the server offers it (1) once the player is in the
	// world; a client that draws them answers 1 to have them and 0 to stop.
	// A PS2 ignores the offer.
	OpcodeQuestMarkers GameOpcode = 0x7E03

	// OpcodeQuestMarkersData is the marks, server to a client that asked: a
	// u8 count, then for each a u8 kind (1 a quest the player may take from
	// that NPC, 2 a step of the player's to do at it, 3 a kill or loot step's
	// creatures), a u8 world, f32 x and z (the NPC's spawn), the NPC's name
	// and a u8 count of quests, each its name, u8 lowest and highest levels,
	// its line (the offer's start, the step's journal line) and its detail
	// (the rewards, what the step needs); a text is a u8 byte count and
	// UTF-8. Each one is the whole set (handlers/questmarks.go).
	OpcodeQuestMarkersData GameOpcode = 0x7E04

	// OpcodeQuestProgress is not retail either: the quest tracker for a
	// client's HUD (handlers/questprogress.go), offered and answered as
	// OpcodeQuestMarkers is (a u32: the server's offer 1, the client's answer
	// 1 to have it, 0 to stop).
	OpcodeQuestProgress GameOpcode = 0x7E05

	// OpcodeQuestProgressData is the tracker, server to a client that asked:
	// a u8 count of quests (the journal's, in its order), each its name and a
	// u8 count of objectives, each its text, u16 had and u16 needed (needed
	// 0: nothing to count); a text is a u8 byte count and UTF-8. Each one is
	// the whole tracker.
	OpcodeQuestProgressData GameOpcode = 0x7E06

	// OpcodeGMTriggers is not retail either: the names of a GM's triggers for
	// its TEST menu (handlers/gmtriggers.go), a u32 after the opcode, both
	// ways. The server offers it (1) to a GM's client right after the GM
	// channel's 0x0700 {35, n}; a client that shows the names answers 1 to
	// have them and 0 to stop. A PS2 ignores the offer.
	OpcodeGMTriggers GameOpcode = 0x7E07

	// OpcodeGMTriggersData is the names, server to a client that asked: a u8
	// count, then each trigger of the menu's eight lines: a u8 index (0-7,
	// the line top to bottom, the number 0x0702 sends back), u8 flags (bit 0
	// on, bit 1 running now) and its name (a u8 byte count and UTF-8, at most
	// 32 bytes). Each one is the whole set: a line it leaves out shows the
	// client's own word, TRIGGER. Sent after the client's 1 and whenever a
	// trigger is saved, deleted, starts or ends.
	OpcodeGMTriggersData GameOpcode = 0x7E08

	// OpcodeDuelWindow is not retail: whether a client has SOE's live duel
	// window (VIWndDuel, entity/duel.go), a u32 after the opcode, both ways.
	// The server offers it (1) once the player is in the world; a client
	// that shows the live window answers 1 to be sent the live game's duel
	// messages (OpcodeDuel) in place of boxes, and to see its opponent as a
	// player (its own TargetInRange asks its duel part), 0 to go back. A PS2
	// ignores the offer and duels by boxes.
	OpcodeDuelWindow GameOpcode = 0x7E09

	// OpcodeMailForm is not retail: whether a client sends SOE's live mail
	// forms (several recipients, the To line read back; handlers/mail.go), a
	// u32 after the opcode, both ways. The server offers it (1) once the
	// player is in the world; a client that sends the live forms answers 1
	// and from then sends them (0 goes back to the disc's). Each request is
	// answered by its own form whatever the answer, which is kept on the
	// session for the log. A PS2 ignores the offer; the server never offers 0.
	OpcodeMailForm GameOpcode = 0x7E0A
	// OpcodeAuctionForm is not retail: whether a client has SOE's live
	// auction forms (the stack UI of feature bit 0x08, handlers/auction.go),
	// a u32 after the opcode, both ways. The server offers it (1) once the
	// player is in the world; a client whose sell form names a quantity
	// answers 1, and from then on its Sell's count is how many items go up
	// (one of a stack is one item, not the whole record) and it is shown a
	// lot a disc client put up whole in its own terms (per item, bid for
	// whole); 0 goes back. Without it a Sell of count 1 puts up the whole
	// bank record, as the disc's form (whose count is the constant 1) means
	// it; a count over 1 is a quantity from any client. A PS2 ignores it.
	OpcodeAuctionForm GameOpcode = 0x7E0B
	// OpcodeLiveLook is not retail: whether a client reads SOE's live game's
	// look (entity/livelook.go): the 201-byte entity state channel and the
	// 0x002C record of eqoa-live-1500's CLIENT, which carry a colour (RGBA)
	// for each armour slot, held item and the robe where the disc's carry a
	// tint index. A u32 after the opcode, both ways: the server offers it (1)
	// after a session's first character list; a client that reads the live
	// look answers 1 to have it and 0 to go back. A PS2 ignores the offer and
	// is sent the disc's look, as is every client of a server that never
	// offers it (Sandstorm).
	OpcodeLiveLook GameOpcode = 0x7E0C

	// OpcodeLiveLookInForce is the server's answer to OpcodeLiveLook, a u32:
	// the look every character list from here on is in (1 the live game's,
	// 0 the disc's), so a client never reads a list in the wrong one (lists
	// are reliable, and come after it). A client still choosing a character
	// is then sent its list again in that look. The entity channels say
	// theirs by their length (162 the disc's, 201 the live game's).
	OpcodeLiveLookInForce GameOpcode = 0x7E0D

	// Voice chat (internal/voice, entity/voice.go): non-retail opcodes in a
	// block of their own; a PS2 ignores them. Strings are a u16 byte length
	// and UTF-8.
	//
	// OpcodeVoiceOffer: a u32 after the opcode. The server offers voice (1)
	// at the first 0x0014; a client answers with flags, bit 0 voice on (0
	// off), at once and whenever they change.
	OpcodeVoiceOffer GameOpcode = 0x7E10

	// OpcodeVoiceConfig, server to a client whose voice is on: a u8 count of
	// ICE servers, each a url, a username and a credential (this server's
	// TURN, when it has one, with REST credentials: "<unix expiry>:v<voice
	// id>" and base64 HMAC-SHA1 of it under the TURN secret; none: 0), then a
	// u32 of seconds they last; sent again before then.
	OpcodeVoiceConfig GameOpcode = 0x7E11

	// OpcodeVoiceRoster, server to a client whose voice is on, whenever it
	// changes: a u8 count, then each it may hear as a u32 voice id, a name and
	// a u8 of channels (bit 0 a groupmate, bit 1 in its room). Empty when its
	// voice goes off.
	OpcodeVoiceRoster GameOpcode = 0x7E12

	// OpcodeVoiceSignal, both ways, between a client and the forwarding
	// server: a u32 seq (the sender's message number), a u8 kind (1 offer,
	// client to server; 2 answer; 3 ICE candidate, JSON; 4 bye), a u16 part
	// and u16 parts, and a chunk of at most 900 bytes (reassembled by seq).
	OpcodeVoiceSignal GameOpcode = 0x7E13

	// OpcodeVoiceRoom, client to server: a u8 op, 1 join (a room's name, its
	// lock's H or empty, a u8 of flags: bit 0 unlisted, for the join that
	// makes it), 2 leave, 3 list.
	OpcodeVoiceRoom GameOpcode = 0x7E14

	// OpcodeVoiceLobby, server to client after a list and after its own join
	// or leave: a u16 count, then each listed room's name, a u16 of members
	// and a u8, 1 when it is locked; most members first, at most 200.
	OpcodeVoiceLobby GameOpcode = 0x7E15

	// OpcodeVoiceRoomState, server to client after a join or a leave: a u8
	// result (0 in the room, 1 wrong password, 2 name refused, 3 left) and the
	// room's name.
	OpcodeVoiceRoomState GameOpcode = 0x7E16
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
	// GameVersionSandstormS2Live is what Sandstorm's live S2 (October 2026)
	// sends instead: a u32 21, then its own identity fields
	// (claude-notes/sandstorm-s2-live-20261015.md); its world table is S2's
	// eight, with no hard-disk loader after it.
	GameVersionSandstormS2Live = 0x15
	GameVersionUnknown         = 0x00
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
	case OpcodeSwingOutOfReach:
		return "SwingOutOfReach"
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
	case OpcodeLiveFeatures:
		return "LiveFeatures"
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
	case OpcodeEmoteWave:
		return "EmoteWave"
	case OpcodeEmoteBow:
		return "EmoteBow"
	case OpcodeEmotePoint:
		return "EmotePoint"
	case OpcodeEmoteCheer:
		return "EmoteCheer"
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
	case OpcodeDuel:
		return "Duel"
	case OpcodeDuelWindow:
		return "DuelWindow"
	case OpcodeMailForm:
		return "MailForm"
	case OpcodeAuctionForm:
		return "AuctionForm"
	case OpcodeLFG:
		return "LFG"
	case OpcodeLFGResults:
		return "LFGResults"
	case OpcodeGroupShow:
		return "GroupShow"
	case OpcodeLiveLook:
		return "LiveLook"
	case OpcodeLiveLookInForce:
		return "LiveLookInForce"
	case OpcodeNoCollideList:
		return "NoCollideList"
	case OpcodeFactionPage:
		return "FactionPage"
	case OpcodeClientFaction:
		return "ClientFaction"
	case OpcodeFactionMessages:
		return "FactionMessages"
	case OpcodeEntityView:
		return "EntityView"
	case OpcodePowerBars:
		return "PowerBars"
	case OpcodePowerBarsData:
		return "PowerBarsData"
	case OpcodeQuestMarkers:
		return "QuestMarkers"
	case OpcodeQuestProgress:
		return "QuestProgress"
	case OpcodeQuestProgressData:
		return "QuestProgressData"
	case OpcodeQuestMarkersData:
		return "QuestMarkersData"
	case OpcodeGMTriggers:
		return "GMTriggers"
	case OpcodeGMTriggersData:
		return "GMTriggersData"
	case OpcodeVoiceOffer:
		return "VoiceOffer"
	case OpcodeVoiceConfig:
		return "VoiceConfig"
	case OpcodeVoiceRoster:
		return "VoiceRoster"
	case OpcodeVoiceSignal:
		return "VoiceSignal"
	case OpcodeVoiceRoom:
		return "VoiceRoom"
	case OpcodeVoiceLobby:
		return "VoiceLobby"
	case OpcodeVoiceRoomState:
		return "VoiceRoomState"
	default:
		return "Unknown"
	}
}
