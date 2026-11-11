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
	OpcodeHotkeySet           GameOpcode = 0x006A // a quick-chat set made (client: Send006A)
	OpcodeHotkeyDrop          GameOpcode = 0x006B // a quick-chat set taken away (client: Send006B)
	OpcodeHotkeyChoice        GameOpcode = 0x006C // a quick-chat set's choice changed (client: Send006C)

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
	OpcodeSummonAnswer     GameOpcode = 0x00D4 // the prompt's answer (SendD4On / SendD4Off): u8 1 Yes, 0 No
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
	// have the menu's eight lines' (OpcodeGMTriggersData), 2 to have every
	// trigger in their place (OpcodeGMTriggerList, the menu a line a trigger),
	// and 0 to stop; another value is taken as 1, as a server from before the
	// list took any but 0. A PS2 ignores the offer.
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

	// OpcodeBugReportOffer is not retail: whether a client opens Support's
	// request form on Bug Report from a key of its own (vanilla's F3) and
	// sends a note of its own side (OpcodeBugReportNote) with each petition
	// it files, a u32 after the opcode, both ways. The server offers it (1)
	// once the player is in the world (handlers/bugreport.go), followed by a
	// second u32: the most characters a request's or reply's text may have
	// (handlers.CSTextMost, 2047; since 2026-11-08, when the server began to
	// put a long message's pieces back together, session/fragments.go). A
	// client that does answers 1 (a u32 alone), and the server's line on how
	// to report a bug names its key. A PS2 ignores the offer, and a client of a
	// server that never offers it shows no such key.
	OpcodeBugReportOffer GameOpcode = 0x7E0E
	// OpcodeBugReportNote is not retail: a client's note of its own side
	// (its build, platform and graphics, frame rate, what it shows of itself
	// and its target) for the petition it files next, by Support's form or
	// by /bug, from a client that answered OpcodeBugReportOffer 1: a u32
	// version (1), then a u32 byte count and that many bytes of UTF-8 text,
	// "key: value" lines, at most BugNoteMax bytes. The server keeps it on the
	// session for the next petition filed within a minute and puts it in that
	// petition's server view, for the GMs alone.
	OpcodeBugReportNote GameOpcode = 0x7E0F
	// OpcodeMemberInvites is not retail: any member of a group may invite
	// into it (the user, 2026-11-06), where CLIENT lets only the leader
	// (InviteEntity, InviteToGroup: word 640). A u32 after the opcode, 1, from
	// the server once the player is in the world (handlers/group.go); a
	// client that has it invites as any member, and sends no answer. A PS2
	// ignores it and keeps the leader's rule, and a client of a server that
	// never offers it does too.
	OpcodeMemberInvites GameOpcode = 0x7E17
	// OpcodeRaidRosterOffer is not retail: the whole of a raid group for a client
	// that can show it, where CLIENT holds four (entity/raid.go). A u32 after the
	// opcode, 1, from the server once the player is in the world (handlers/group.go).
	// A PS2 ignores it and keeps its window of four.
	OpcodeRaidRosterOffer GameOpcode = 0x7E18
	// OpcodeRaidRosterAnswer is a client's answer to the offer: a u32, 1 to be sent
	// the roster, 0 to stop.
	OpcodeRaidRosterAnswer GameOpcode = 0x7E19
	// OpcodeRaidRoster is the roster, to a client that asked, on a change and at most
	// once a second: u16 members in all, u16 this message's first, u8 how many it
	// holds, then each: u32 the id the client knows it by, u8 and the bytes of its
	// name, u8 class, u8 level, u8 HP of 255, u8 power of 255. In the order they
	// joined, the leader first, the client's own character among them. 0 members:
	// its group is no raid group (now). entity/raidroster.go.
	OpcodeRaidRoster GameOpcode = 0x7E1A

	// OpcodeRaceLabels is a race of racedefs' own words for a client that shows
	// them (entity/racelabels.go, handlers/racelabels.go): a u32 after the
	// opcode, both ways, as OpcodePowerBars: the server offers it (1) once the
	// player is in the world; a client that can answers 1 to have them and 0 to
	// stop. A PS2 ignores the offer and keeps the wire race's word.
	OpcodeRaceLabels GameOpcode = 0x7E1C

	// OpcodeRaceLabelsData is the list, server to a client that asked: a u8 count
	// of races, then each race's name and short name, a u16 count of its
	// characters and each one's name (a string is a u8 byte count and UTF-8).
	// Each one is the whole list: a name it leaves out reads as its wire race.
	OpcodeRaceLabelsData GameOpcode = 0x7E1D

	// OpcodeCreationOffer is not retail: what character creation takes past
	// the game's own races and classes (entity/creation.go, handlers/creation.go,
	// world.open_races and world.all_classes), a u32 after the opcode, both ways,
	// as OpcodeLiveLook: the server offers it (1) after a session's first
	// character list, only while it takes something past the game's; a client
	// whose create pages can show it answers 1 and is sent OpcodeCreationRaces
	// at once (0 asks nothing). A PS2 ignores the offer and creates from the
	// disc's pages; a client that never answers is never sent the races.
	OpcodeCreationOffer GameOpcode = 0x7E1E

	// OpcodeCreationRaces is the races creation takes past the game's own, to a
	// client that answered OpcodeCreationOffer: u8 version (1), u8 flags (bit 0
	// every class for every race), u8 races, then each: u8 id (characters.race),
	// u8 like (the game's race whose stage place, cameras, music and starting
	// city its create pages take), u8 city (a human's when like is 0: 1 east, 2
	// west; else 0), 7 s8 bonuses (STR STA AGI DEX WIS INT CHA), u16 its classes
	// (bit N class N), u32 male and u32 female body (CHAR.ESF DictIDs), then its
	// name and short name (each a u8 byte count and UTF-8) and its description
	// (a u16 byte count and UTF-8).
	OpcodeCreationRaces GameOpcode = 0x7E1F

	// OpcodeFly is not retail: a GM's flight (#fly, handlers/fly.go), a u32
	// after the opcode, both ways, and never to a client that has not said it
	// flies. From a client: 1, it flies (vanilla says so with its answer to the
	// GM channel's OpcodeGMTriggers offer, so a session of a PS2's, which has no
	// such answer, never has this opcode sent to it, not even as an offer). From
	// the server: 1 flight is on, 0 it is off. A client's own word sets nothing
	// but what the server may send it: only #fly turns flight on.
	OpcodeFly GameOpcode = 0x7E1B

	// OpcodeCombatRecord is not retail: a character's combat record, its PvP and its
	// duels (entity/combatrecord.go, handlers/combatrecord.go), for a client with a
	// window of it: a u32 after the opcode, both ways, as OpcodeRaceLabels. The server
	// offers it (1) once the player is in the world; a client that shows it answers 1
	// (0 to stop) and from then is sent its record (OpcodeCombatRecordData) whenever a
	// count changes it. A PS2 ignores the offer, and a client of a server that never
	// makes it shows nothing of it.
	OpcodeCombatRecord GameOpcode = 0x7E20

	// OpcodeCombatRecordData is the record. From a client that answered
	// OpcodeCombatRecord, a u32 (1): send it now (its window opening). From the
	// server, in answer and on a change: a u8 version (1), the character's name, a u8
	// count of rows, then each row: a u8 style (0 a heading, 1 a label and its value),
	// its label and its value, every string a u8 byte count and UTF-8. The rows are
	// the server's words and figures as they stand when sent ("2 hours ago"), drawn
	// as they come.
	OpcodeCombatRecordData GameOpcode = 0x7E21

	// OpcodeServerWorlds is not retail: the worlds this server has past the
	// disc's, each made of the disc's own zones where they stand on the disc
	// (internal/worlds, cmd/worldbuild, handlers/serverworlds.go), a u32 after
	// the opcode, both ways, as OpcodeCreationOffer: the server offers them (1)
	// after a session's first character list, while it has any; a client that
	// can build them off its own disc (eqoa-rust-client-vanilla) answers 1 and
	// is sent OpcodeServerWorldsData. A PS2 ignores the offer and is never sent
	// into them. 0x7E22-0x7E26 were taken by other lanes the same day: these
	// stand clear of them.
	OpcodeServerWorlds GameOpcode = 0x7E30

	// OpcodeServerWorldsData is the worlds, to a client that answered
	// OpcodeServerWorlds: u8 version (1), u8 worlds, then each: u8 its slot, its
	// name, u32 the built file's size and u32 its CRC-32, u8 zones, then each:
	// the disc's world file it is on, u16 its index there, u32 its record's size
	// and u32 its CRC-32 (header and body, as on the disc), u8 its cell's column
	// and u8 row, its name, and u8 the race whose home it is (racedefs' id, 0
	// none: a client names it the race's starting place); every string a u8
	// byte count and UTF-8. From
	// the client: the worlds it holds now, built and checked (u8 count, then each
	// slot); the server sends it into those alone (LoadableWorlds).
	OpcodeServerWorldsData GameOpcode = 0x7E31

	// OpcodeServerWorldsChars is which of a session's characters a login needs
	// the server's own worlds for, server to a client that answered
	// OpcodeServerWorlds 1 (petition 1468, 2026-11-10: every pick in a page
	// waited for the worlds to be built, a wait almost no character needs):
	// u8 version (1), u8 count, then each character's ServerID (u32,
	// little-endian, as 0x002C carries it) whose login starts in one of the
	// server's worlds for a client that holds it and on its race's mainland
	// spot for one that does not (handlers.loginServerWorld: saved there, and
	// a place a login resumes; one standing there in the world, left behind
	// by a dropped client, too). Each one is the whole set: a character it
	// leaves out is placed the same whatever the client holds, and its pick
	// need wait for nothing. Sent at once after the client's 1, BEFORE
	// OpcodeServerWorldsData (a list of 70 KB is seconds on the way, and a
	// pick made meanwhile goes by this), and again after every later
	// character list of the session. A client that reads it holds a pick only
	// for a character it names, natively as in a page; one that does not (an
	// older vanilla: its dispatcher passes an unknown opcode on and nothing
	// takes it) holds as it did. Never sent to a session that did not take
	// the offer, so never to a PS2. (0x7E55 is left unused, this lane's.)
	OpcodeServerWorldsChars GameOpcode = 0x7E54

	// OpcodeRaceOptions is not retail: a custom race's own character options
	// (entity/raceoptions.go, handlers/raceoptions.go): a u32 after the opcode,
	// both ways, as OpcodeRaceLabels. The server offers it (1) after a session's
	// first character list; a client that can answers 1 (0 to stop) and is sent
	// the options each race has (OpcodeRaceOptionsData, once a session) and the
	// looks the characters chose (OpcodeRaceLooks, then whenever they change). A
	// PS2 ignores the offer: a body chosen is the character's model on every
	// client, and nothing else of a look reaches one.
	OpcodeRaceOptions GameOpcode = 0x7E22

	// OpcodeRaceOptionsData is each race's options, to a client that answered
	// OpcodeRaceOptions: u8 version (1), u8 races, then each: u8 race (as
	// characters.race holds it), u8 rows, then each row: its key and label, u8
	// kind (1 a body, 2 a colour, 3 a texture), u8 parts, then each part: u32 the
	// body it is of, u8 entries and each palette entry (u8), u8 rects and each
	// rect's four u8 (u0, v0, u1, v1 in 255ths of the texture); then u8 steps for
	// a male and each step: its name, u32 value (a body's or a texture's DictID,
	// or a colour), u16 size (a body's scale against its race's own, 1000ths; 0
	// for a colour or a texture); then u8 steps for a female (0: the male's) and
	// each as the male's. Every string is a u8 byte count and UTF-8.
	OpcodeRaceOptionsData GameOpcode = 0x7E23

	// OpcodeRaceLooks is the looks the characters chose, to a client that
	// answered OpcodeRaceOptions: u8 version (1), u16 characters, then each: its
	// name (a u8 byte count and UTF-8; a GM's both with and without the tag its
	// channel name carries), u32 the body the look is of (its own model: a look
	// is drawn only on it, never under an illusion or another body), u8 parts,
	// then each part: u8 kind (2 a colour, 3 a texture), u32 value, u8 entries and
	// each palette entry, u8 rects and each rect's four u8. Each one is the whole
	// list: a name it leaves out is drawn in its body's own colours.
	OpcodeRaceLooks GameOpcode = 0x7E24

	// OpcodeJump is not retail: a player's jump (entity/jump.go,
	// handlers/jump.go; world.jump), a u32 after the opcode, both ways. CLIENT's
	// walk has no jump, so the server offers it (1) only to a client of its own:
	// one that answered the bug report offer (OpcodeBugReportOffer), which a PS2
	// never does, so a PS2 is never sent it. A client that jumps answers 1 (0: it
	// does not), and the movement check takes a jump's arc from that player
	// (movecheck.go). The server says 0 to a client it offered jumps to before a
	// restart into a server without them.
	OpcodeJump GameOpcode = 0x7E28

	// OpcodeItemLists is not retail: a pack and a bank that hold more than
	// CLIENT's 40 records each (world.pack_slots, world.bank_slots;
	// entity/itemslots.go, handlers/itemlists.go). The server offers them after
	// a session's first character list, once, only while it holds more: a u32
	// 1, then u32 the pack's records and u32 the bank's, little-endian. A
	// client that holds more answers 1 and the records it holds of each (u32,
	// u32), or 0 to stop; the server takes each between 40 and its own. A PS2
	// ignores the offer (CLIENT's dispatcher has no default case) and holds 40.
	OpcodeItemLists GameOpcode = 0x7E29
	// OpcodeItemListsInForce is the sizes in force, server to a client that
	// answered OpcodeItemLists: u32 the pack's records, u32 the bank's, sent
	// before every MemoryDump (a login, a respawn, a restart's), whose lists
	// hold that many at most and which every add after it fills to. A client
	// never told holds 40 of each. (0x7E2B is left unused, the item lists'.)
	OpcodeItemListsInForce GameOpcode = 0x7E2A

	// OpcodeMapTravel is not retail: a GM's travel to a spot of the map
	// (handlers/maptravel.go), offered at a GM's world entry with the GM's
	// channel (SendGMChannel): a u32 after the opcode, 1. A client that has a
	// map (vanilla) lets the GM ask for a spot of it then (OpcodeMapTravelTo); a
	// PS2 ignores the offer (CLIENT's dispatcher has no default case), and no
	// one else is offered it.
	OpcodeMapTravel GameOpcode = 0x7E34
	// OpcodeMapTravelTo is a GM's ask, client to server: u32 world (the
	// MemoryDump's world index), f32 x, f32 z, little-endian. The server moves
	// a GM's character there as #goto x y z does, on the ground's floor, and
	// refuses anyone else's (logged, nothing said, nothing moved).
	OpcodeMapTravelTo GameOpcode = 0x7E35

	// 0x7E10-0x7E16 were voice chat's (offer, config, roster, signal, room,
	// lobby, room state), removed with it on 2026-11-08: left unused, as a
	// client from before then may still send them

	// OpcodeCapes is not retail: a cape, an equip slot of this server's own
	// (world.capes; entity/capes.go, handlers/capes.go), for a client that
	// draws capes. A u32 after the opcode, both ways, as OpcodeLiveLook: the
	// server offers it (1) after a session's first character list, only while
	// world.capes is on; a client that draws capes answers 1 (0 to stop). From
	// its 1 on, the items it is sent keep a cape's wear slot (28) and a worn
	// cape's equip slot (22), it may put a cape on and take it off with the
	// game's own messages (0x003E, 0x003F, 0x0040), and it is sent
	// OpcodeCapesList with each character list and OpcodeCapesInView in the
	// world. A PS2 ignores the offer (CLIENT's dispatcher has no default case)
	// and is sent a cape as a plain item (wear -1), never worn. (0x7E22-0x7E24
	// are the race options'.)
	OpcodeCapes GameOpcode = 0x7E25

	// OpcodeCapesInView is the capes worn in a client's view, server to a
	// client that answered OpcodeCapes 1, in the world: a u8 count, then for
	// each a u32 id (as that client knows the entity: its own object id for
	// its own character, the visibility channel's for another), a u8 style
	// (the cape record's model, never 0) and a u32 colour (0xRRGGBBAA: the
	// cape's dye, an Armor Dyer's or #dye's, else its record's colour; 0 the
	// art's own), little-endian. Players only. Each one is the whole set: an
	// id it leaves out wears no cape. Sent when the set changes, and again at
	// each MemoryDump and world change.
	OpcodeCapesInView GameOpcode = 0x7E26

	// OpcodeCapesList is the capes the characters of a list wear, server to a
	// client that answered OpcodeCapes 1, after each character list (0x002C)
	// and when its answer comes while it is choosing: a u8 count, then for each
	// character that wears one a u32 its ServerID (as 0x002C carries it), a u8
	// style and a u32 colour, as OpcodeCapesInView. The whole list each time.
	OpcodeCapesList GameOpcode = 0x7E27

	// OpcodePetEffects is not retail: the effects on the player's pet, for a
	// client that draws them under its pet's bar (entity/peteffects.go,
	// handlers/peteffects.go; the user, 2026-11-09: "put the buff icons under
	// the pet name plate"). A u32 after the opcode, both ways, as
	// OpcodePowerBars: the server offers it (1) once the player is in the
	// world; a client that draws them answers 1 to have them and 0 to stop, at
	// once and whenever that changes. A PS2 ignores the offer (CLIENT's
	// dispatcher has no case above 0x1402 and no default) and never asks, so
	// it is sent none: the pet's effects are on no PS2's screen, its own
	// effects window (channel 0x43) the player's alone.
	OpcodePetEffects GameOpcode = 0x7E32

	// OpcodePetEffectsData is the pet's effects, server to a client that
	// asked, whenever they change (one goes on, runs out, is cast again, the
	// pet goes or another comes) and at once after the ask: a u8 count, then
	// for each, in the order they went on, a u32 icon (a DictID, as channel
	// 0x43's records carry it), a u32 its time left in milliseconds as the
	// message is made, and a u8 byte count and the name in UTF-8 (at most 31
	// characters, as channel 0x43's). Each one is the whole set; 0 effects:
	// none, or no pet. At most 16 (entity.PetEffectsMost).
	OpcodePetEffectsData GameOpcode = 0x7E33

	// OpcodeGMTriggerList is every GM trigger, server to a client that
	// answered OpcodeGMTriggers 2 (handlers/gmtriggers.go; the user, 2026-11-09:
	// "can we add the triggers to client menu also in gm menus"), in place of
	// OpcodeGMTriggersData: a u16 count, then each row of gmtriggers by slot: a
	// u32 slot, u8 flags (bit 0 on, bit 1 running now) and its name (a u8 byte
	// count and UTF-8, at most 32 bytes; 0 for a trigger with none), little-
	// endian. Each one is the whole table, off triggers too. Sent after the
	// client's 2 and whenever a trigger is saved, deleted, starts or ends. A
	// client fires a line by its slot with OpcodeGMTriggerFire.
	OpcodeGMTriggerList GameOpcode = 0x7E36

	// OpcodeGMTriggerFire is a GM's firing of a trigger by its slot, client to
	// server: a u32 slot, little-endian. The server fires it as 0x0702 and
	// #trigger <n> do, for a GM alone (anyone else's is logged and dropped),
	// and tells the GM what it did. A client sends it only for a line of
	// OpcodeGMTriggerList's; 0x0702's byte reaches slots 1 to 256 and no
	// further.
	OpcodeGMTriggerFire GameOpcode = 0x7E37

	// OpcodeNumberTagsOffer is not retail: what made each figure, for a client
	// that colours its numbers by it (entity/numbertags.go,
	// handlers/numbertags.go; the user, 2026-11-09, of a player's ask to "see/
	// change colors of damage/procs/healing for player/pets similar to the way
	// SS does": "we can add the number colors to extended menu"). Server to
	// client once the player is in the world: a u8, 1, the tag's layout below.
	// A PS2 ignores it (CLIENT's dispatcher has no case above 0x1402 and no
	// default) and never answers.
	OpcodeNumberTagsOffer GameOpcode = 0x7E38

	// OpcodeNumberTagsAnswer is a client's answer to OpcodeNumberTagsOffer,
	// client to server, at once and whenever it changes: a u8, 1 to have its
	// figures tagged, 0 to stop. From a 1 on, each 0x00DB the session is sent
	// of a figure its player or its player's pet made carries one byte after
	// the retail body (u32 attacker, zigzag amount, u32 defender): its low 7
	// bits what made it (1 melee, 2 a spell or an ability, 3 a damage over
	// time's tick, 4 a damage shield, 5 a proc, 6 a heal, 7 a heal over time's
	// tick), bit 7 set when the player's pet made it. Every other 0x00DB (a
	// figure on the player that another made, any to a session that never
	// answered 1) is the retail body alone.
	OpcodeNumberTagsAnswer GameOpcode = 0x7E39

	// The raid's windows (the GM Jibrish's petitions 1448 and 1451: the leader's
	// mark, loot rules, need before greed's rolls, the master looter's list;
	// entity/raidrules.go, handlers/raidloot.go). Not retail: CLIENT's dispatcher
	// has no case above 0x1402 and no default, so a PS2 ignores every one and is
	// told the same in chat lines (and answers them by /t need, /t assign ...).
	// Every id in them is the one the RECEIVING client knows the player by (its
	// own, a visibility channel's, or groupFarID's stand-in, as 0x7E1A's); a name
	// is a u8 byte count and its bytes.
	//
	// OpcodeRaidOffer, server to client once the player is in the world: a u8, 1.
	OpcodeRaidOffer GameOpcode = 0x7E42
	// OpcodeRaidAnswer, client to server at once after the offer and on a change:
	// a u8, 1 to be sent the raid's state, its rolls as windows and the master
	// looter's list (0x7E44, 0x7E45, 0x7E47), 0 for the chat lines a PS2 gets.
	OpcodeRaidAnswer GameOpcode = 0x7E43
	// OpcodeRaidState, server to client to one that answered 1, on a change (looked
	// at once a second, and at once after a raid command), and all zero once when
	// its player is in no raid: u32 leader (0: no raid), u32 helper (0 none), u32
	// main assist (0 none), u8 loot rule (0 free for all, 1 need before greed, 2
	// master looter), u8 flags (bit 0 in combat: no rule change and no disband, bit
	// 1 the client's player leads, bit 2 it is the helper).
	OpcodeRaidState GameOpcode = 0x7E44
	// OpcodeRaidRoll, server to client, one roll for an answered client offered it:
	// u32 roll id (1-999, round), u16 seconds left; 0 seconds is the roll over for
	// that client (chosen, decided, the corpse gone) and nothing follows; else u8
	// flags (bit 0 it may Need, by the equip rules) and the item as an inventory
	// record (0x003B's body: writeSingleItem, its key the roll id).
	OpcodeRaidRoll GameOpcode = 0x7E45
	// OpcodeRaidRollChoice, client to server: u32 roll id, u8 choice (1 need, 2
	// greed, 3 pass); taken, it is answered with 0x7E45 of 0 seconds.
	OpcodeRaidRollChoice GameOpcode = 0x7E46
	// OpcodeRaidMasterLoot, server to client, after 0x0018 when a master looter opens
	// a corpse it holds, and u8 0 alone when that hold ends: u8 1, u16 seconds left
	// of the hold, u8 count, then each u32 id and its name (the raid's members and
	// those kicked from it in that fight).
	OpcodeRaidMasterLoot GameOpcode = 0x7E47
	// OpcodeRaidCommand, client to server: u8 op, then 1 loot rule (u8), 2 leave the
	// raid, 3 assign (u32 the open corpse's record key, u32 the player's id), 4
	// abandon the open corpse, 5 raid-invite (u32 target), 6 the raid flag, 7
	// assist the main assist, 8 leader (u32 target), 9 helper (u32 target, 0
	// none), 10 main assist (u32 target, 0 none), 11 kick (u32 target).
	OpcodeRaidCommand GameOpcode = 0x7E48
	// OpcodeDPSMeterOffer is not retail: a damage meter, who in the player's
	// group did how much damage in the fight and how fast (entity/dpsmeter.go,
	// handlers/dpsmeter.go; the owner, 2026-11-10: "lets add dps meters, will
	// work solo, group, raid"). Server to client once the player is in the
	// world: a u8, 1, OpcodeDPSMeterData's layout. A PS2 ignores it (CLIENT's
	// dispatcher has no case above 0x1402 and no default) and never answers.
	OpcodeDPSMeterOffer GameOpcode = 0x7E3A

	// OpcodeDPSMeterAnswer is a client's answer to OpcodeDPSMeterOffer, client
	// to server, at once and whenever it changes: a u8, 1 to be sent the meter
	// (OpcodeDPSMeterData), 0 to stop.
	OpcodeDPSMeterAnswer GameOpcode = 0x7E3B

	// OpcodeDPSMeterData is the meter, server to a client that answered 1: at
	// most once a second while a fight in its group is on and something in it
	// changed, and once more as the fight ends (a fight ends DPSMeterIdle after
	// the last damage anyone in the group did). Little-endian: a u8 layout (1),
	// u32 the fight's number (another number, another fight; numbers are the
	// server's, never 0), u8 1 when the fight is over (this is its last
	// message) else 0, u32 the fight's length in ms (from its first damage to
	// its latest), u32 the damage of all its lines (what each line's share is
	// of), u16 its lines in all, u16 the receiver's own line's place among them
	// (1 the most damage), u8 the lines here (at most entity.DPSMeterLinesMost,
	// the most damage first, the receiver's own always among them: in the last
	// place when its place is past them), then each line: u8 flags (1 the receiver's own; 2 no longer
	// in its group: one that did damage in this fight and left), u8 its class,
	// u32 its damage (its pet's included), u32 its pet's share of that, and its
	// name (a u8 byte count and UTF-8, at most 23 bytes). Every member of the
	// group in the world has a line, none of damage too.
	OpcodeDPSMeterData GameOpcode = 0x7E3C

	// OpcodeDPSMeterReset is a client's reset, client to server: a u8, 1. The
	// fight under way, if any, is the receiver's no more (nothing is sent of it
	// again); the next damage in its group starts another, under another
	// number.
	OpcodeDPSMeterReset GameOpcode = 0x7E3D
)

// A tint on the bodies of the NPCs of a name, for a client that draws one
// (entity/npctint.go, handlers/npctint.go, table npctint; the server owner,
// 2026-11-10: Yhafiran, "the black and green oriental dragon", wears the
// disc's only green oriental dragon, and "vanilla draws Yhafiran tinted toward
// black and green").
const (
	// OpcodeNPCTintsOffer is not retail: the server's offer of its NPCs'
	// body tints, server to client once the player is in the world: a u8, 1,
	// the layout of OpcodeNPCTints below. A PS2 ignores it (CLIENT's
	// dispatcher has no case above 0x1402 and no default) and never answers,
	// so it draws every NPC in its body's own colours.
	OpcodeNPCTintsOffer GameOpcode = 0x7E3E

	// OpcodeNPCTintsAnswer is a client's answer to OpcodeNPCTintsOffer, client
	// to server: a u8, 1 to be sent the tints, 0 to stop (a wider answer is
	// read by its first byte). A client answers at once: one that answers 1
	// drops what it held and is sent the tints there are (OpcodeNPCTints),
	// none when there are none.
	OpcodeNPCTintsAnswer GameOpcode = 0x7E3F

	// OpcodeNPCTints is the tints, server to a client that answered 1, at
	// once and whenever they change (a GM's edit of npctint): a u16 count,
	// then each tint: its NPC name (a u8 byte count and UTF-8, as
	// database.NPCClassKey keys it: lower case, trimmed, no trailing '$', cut
	// to the 24 bytes the entity channel carries a name in) and a u32 colour
	// 0xRRGGBBAA, little-endian. The client draws the body of every NPC whose
	// name keys the same (a player's never) with each of its colours' red,
	// green and blue times the tint's (255ths), as much of it as the alpha
	// says (0xFF the whole, 0 none). Each one is the whole set; 0 names: none.
	// At most entity.NPCTintsMost.
	OpcodeNPCTints GameOpcode = 0x7E40
)

// A monster's name and level when the player talks to it (□, Q on vanilla's
// keyboard) and it has nothing to say, for a client that shows them in the
// target's details box (handlers/mobinfo.go, world.mob_info; the GM Jibrish's
// petition 1459, 2026-11-10: "press the mob information button on monster (Q
// by default on keyboard). This should ONLY show the monsters name and level,
// nothing else.").
const (
	// OpcodeMobInfoOffer is not retail: the server's offer, server to client
	// once the player is in the world, only where world.mob_info is on: a u8,
	// 1, the layout of OpcodeMobInfo. A PS2 ignores it (CLIENT's dispatcher
	// has no case above 0x1402 and no default) and never answers: its talk to
	// such a monster is answered with a dialogue box (0x0046) of the same two
	// lines.
	OpcodeMobInfoOffer GameOpcode = 0x7E4C

	// OpcodeMobInfoAnswer is a client's answer to OpcodeMobInfoOffer, client to
	// server, at once: a u8, 1 to be sent OpcodeMobInfo in place of the
	// dialogue box, 0 for the box (a wider answer is read by its first byte).
	OpcodeMobInfoAnswer GameOpcode = 0x7E4D

	// OpcodeMobInfo is a monster's name and level, server to a client that
	// answered 1, in answer to its 0x0004 on a living NPC without the NPC bit
	// (what CLIENT lets a player attack) that has nothing to say or stands out
	// of the talk's reach. Little-endian: a u8 layout (1), the u32 id the
	// 0x0004 named (the monster as that client knows it), a u8 its level (the
	// NPC's Level: the con colour's, the entity channel's byte 140), and its
	// name (a u8 byte count and the bytes, at most 63, of what a PS2 can type:
	// handlers/consoletext.go). The client shows the two as CLIENT's
	// VIWndGame::DrawTargetInfo shows a pet's (the name, then word 503 "Level"
	// and the level), in the target's details box, while its □ is held.
	OpcodeMobInfo GameOpcode = 0x7E4E
)

// Item links in chat, EQ1's: a player puts a link to one of its own items in a
// line, and whoever clicks it sees the item as the linker held it then
// (handlers/itemlinks.go; the GM Jibrish's petition 1463, 2026-11-10). A PS2,
// and any client that does not take links, sees the item's name in the line,
// plain, the retail message byte for byte. All little-endian.
const (
	// OpcodeItemLinksOffer is not retail, both ways. Server to client at every
	// world entry: a u8, 1, the layout of OpcodeItemLinkLine and
	// OpcodeItemLinkRecord. A PS2 ignores it (CLIENT's dispatcher has no case
	// above 0x1402 and no default) and never answers. Client to server: a u8,
	// 1 this client takes links (it is sent OpcodeItemLinkLine and may send
	// OpcodeItemLinkSay), 0 it does not (plain lines from then on); a wider
	// answer is read by its first byte. Kept on the session, through a keep
	// restart; a session that never answers takes none.
	OpcodeItemLinksOffer GameOpcode = 0x7E4F

	// OpcodeItemLinkSay is a typed line with links, client to server, sent by
	// a client that answered 1 in place of the retail chat message when the
	// line holds a link: u8 route (0 as 0x000B, routed by the line and then by
	// the chat mode; 1 as 0x0C01 sub 2, a shout; 2 as 0x0578 sub 12, the
	// guild's; any other dropped), u32 target (0x000B's; route 0 only), the
	// text as 0x000B carries it (u32 count of UTF-16 units, 1..1023, then the
	// units: the line as typed, each link's item name in its place), u8 link
	// count, then each link: u16 start, u16 length (UTF-16 units into the
	// text, at least 1), u8 list (0 the pack, worn items among it; 1 the
	// bank), u32 key (the record's key as the client files it,
	// VIItemList::GetSlotId, as 0x003E sends it). A link that does not
	// resolve to a record the sender holds is its plain text; past four in a
	// line the rest are plain text too.
	OpcodeItemLinkSay GameOpcode = 0x7E50

	// OpcodeItemLinkLine is a chat line with its links, server to a client
	// that answered 1, in place of the retail message it would get: u8 link
	// count (1..4), each link u16 start, u16 length (UTF-16 units into the
	// text the inner message carries, as that client shows it: after the
	// speaker's words and that hearer's profanity mask), u32 link id; then the
	// inner message whole, its u16 opcode first, exactly as a client without
	// links is sent it: 0x0A7A (a say), 0x0A7B (a shout, a tell and its echo,
	// a whisper, a raid's line), 0x069E (the group's) or 0x0468 sub 5 (the
	// guild's). A line without links is never wrapped.
	OpcodeItemLinkLine GameOpcode = 0x7E51

	// OpcodeItemLinkAsk is a client's "show me this link", client to server: a
	// u32 link id. Answered with OpcodeItemLinkRecord, from a session in the
	// world, at most 30 in 10 s a session.
	OpcodeItemLinkAsk GameOpcode = 0x7E52

	// OpcodeItemLinkRecord is a link's item, server to the asker: u32 link id,
	// u8 status (1 the item follows; 0 no such link: never made, or expired an
	// hour after it was last linked), then for 1 the item as 0x003B's body
	// (the record as every record outside the MemoryDump goes: worn slot -1,
	// quick slot -1), its key the link id, as the linker held it when it was
	// linked. Nothing after the record.
	OpcodeItemLinkRecord GameOpcode = 0x7E53
)

// The profanity filter as a client's own setting (the GM Jibrish's petition
// 1490, 2026-11-11: "Under options then interface add an option to turn the
// profanity filter on or off"). The filter itself is retail's 0x0045 and the
// server's (handlers/profanity.go): on at every login, masking what that one
// player hears. A client with the setting is told it may say so at world
// entry and answers with it; a PS2 ignores the offer, keeps the filter on and
// may still turn it over with 0x0045 (or /filter).
const (
	// OpcodeProfanityChoice is not retail, both ways. Server to client at
	// every world entry: a u8, 1, the layout. Client to server, at once and
	// whenever its setting changes: a u8, 1 the filter on (lines masked), 0
	// off (lines as said); a wider answer is read by its first byte. Nothing
	// is answered. The choice is the player's own (lineFor) and is kept
	// through a keep restart (entity.KeptPlayer). Only the profanity mask
	// turns: what a PS2 cannot be sent (consoletext.go) stays filtered for
	// every client.
	OpcodeProfanityChoice GameOpcode = 0x7E60
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
	case OpcodeHotkeyChoice:
		return "HotkeyChoice"
	case OpcodeHotkeySet:
		return "HotkeySet"
	case OpcodeHotkeyDrop:
		return "HotkeyDrop"
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
	case OpcodeBugReportOffer:
		return "BugReportOffer"
	case OpcodeBugReportNote:
		return "BugReportNote"
	case OpcodeMemberInvites:
		return "MemberInvites"
	case OpcodeRaidRosterOffer:
		return "RaidRosterOffer"
	case OpcodeRaidRosterAnswer:
		return "RaidRosterAnswer"
	case OpcodeRaidRoster:
		return "RaidRoster"
	case OpcodeCombatRecord:
		return "CombatRecord"
	case OpcodeCombatRecordData:
		return "CombatRecordData"
	case OpcodeFly:
		return "Fly"
	case OpcodeJump:
		return "Jump"
	case OpcodeCreationOffer:
		return "CreationOffer"
	case OpcodeCreationRaces:
		return "CreationRaces"
	case OpcodeServerWorlds:
		return "ServerWorlds"
	case OpcodeServerWorldsData:
		return "ServerWorldsData"
	case OpcodeServerWorldsChars:
		return "ServerWorldsChars"

	case OpcodeCapes:
		return "Capes"
	case OpcodeCapesInView:
		return "CapesInView"
	case OpcodeCapesList:
		return "CapesList"
	case OpcodePetEffects:
		return "PetEffects"
	case OpcodePetEffectsData:
		return "PetEffectsData"
	case OpcodeRaceOptions:
		return "RaceOptions"
	case OpcodeRaceOptionsData:
		return "RaceOptionsData"
	case OpcodeRaceLooks:
		return "RaceLooks"
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
	case OpcodeGMTriggerList:
		return "GMTriggerList"
	case OpcodeGMTriggerFire:
		return "GMTriggerFire"
	case OpcodeNumberTagsOffer:
		return "NumberTagsOffer"
	case OpcodeNumberTagsAnswer:
		return "NumberTagsAnswer"
	case OpcodeRaidOffer:
		return "RaidOffer"
	case OpcodeRaidAnswer:
		return "RaidAnswer"
	case OpcodeRaidState:
		return "RaidState"
	case OpcodeRaidRoll:
		return "RaidRoll"
	case OpcodeRaidRollChoice:
		return "RaidRollChoice"
	case OpcodeRaidMasterLoot:
		return "RaidMasterLoot"
	case OpcodeRaidCommand:
		return "RaidCommand"
	case OpcodeDPSMeterOffer:
		return "DPSMeterOffer"
	case OpcodeDPSMeterAnswer:
		return "DPSMeterAnswer"
	case OpcodeDPSMeterData:
		return "DPSMeterData"
	case OpcodeDPSMeterReset:
		return "DPSMeterReset"
	case OpcodeMapTravel:
		return "MapTravel"
	case OpcodeMapTravelTo:
		return "MapTravelTo"
	case OpcodeNPCTintsOffer:
		return "NPCTintsOffer"
	case OpcodeNPCTintsAnswer:
		return "NPCTintsAnswer"
	case OpcodeNPCTints:
		return "NPCTints"
	case OpcodeItemLinksOffer:
		return "ItemLinksOffer"
	case OpcodeItemLinkSay:
		return "ItemLinkSay"
	case OpcodeItemLinkLine:
		return "ItemLinkLine"
	case OpcodeItemLinkAsk:
		return "ItemLinkAsk"
	case OpcodeItemLinkRecord:
		return "ItemLinkRecord"
	default:
		return "Unknown"
	}
}
