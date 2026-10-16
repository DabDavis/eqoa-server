package handlers

import (
	"encoding/binary"
	"log"
	"math"
	"strconv"

	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
	"github.com/eqoa/pkg/pkg/buffer"
)

// =============================================================================
// PS2-correct opcode senders
//
// These implement the reliable opcode protocol that PS2 retail expects.
// Sandstorm skips most of these, packing everything into state channels.
// Our server sends both: reliable opcodes for guaranteed entity identity,
// and state channels for continuous position/velocity streaming.
// =============================================================================

// SendServerLoginResponse sends opcode 0x0728 — post-auth state setup.
// PS2 retail sends this after auth, before CharacterSelect.
// It signals state reset and prepares the client for character selection.
func (h *CharacterHandler) SendServerLoginResponse(sess *session.Session) {
	msg := protocol.NewMessage(protocol.MessageTypeReliable, protocol.OpcodeServerLoginResponse)
	writer := buffer.NewWriter(msg.Span())
	writer.WriteUint16(uint16(protocol.OpcodeServerLoginResponse))

	// State reset indicator — PS2 expects state 6 or 7 here
	writer.Write7BitEncodedInt64(7) // state = 7 (full reset)

	// Server count (for server list — we only have 1)
	writer.Write7BitEncodedInt64(1)

	// Server entry: ID + name
	writer.Write7BitEncodedInt64(1)                    // server ID
	writer.WriteInt32(int32(len("EQOA")))              // name length
	writer.WriteStringUTF8Raw("EQOA")                  // server name

	msg.SetSize(writer.Position())
	sess.Queue().AddWithSequence(msg)
	log.Printf("PS2: sent ServerLoginResponse (0x0728) to %s", sess.RemoteAddr())
}

// SendWorldEntry sends opcode 0x002E — proper world entry trigger.
// PS2 retail sends this instead of using MemoryDump as the trigger.
// Contains character identity, appearance, equipment, and entity init data.
func (h *CharacterHandler) SendWorldEntry(sess *session.Session, player *entity.Player) {
	msg := protocol.NewMessage(protocol.MessageTypeReliable, protocol.OpcodeWorldEntry)
	dataBuf := make([]byte, 4096)
	writer := buffer.NewWriter(dataBuf)
	writer.WriteUint16(uint16(protocol.OpcodeWorldEntry))

	// Character identity
	writer.Write7BitEncodedInt64(int64(player.CharServerID))
	writer.WriteInt32(int32(len(player.Name)))
	writer.WriteStringUTF8Raw(player.Name)

	// Appearance data
	writer.Write7BitEncodedInt64(int64(player.Race))
	writer.Write7BitEncodedInt64(int64(player.Sex))
	writer.Write7BitEncodedInt64(int64(player.Class))
	writer.Write7BitEncodedInt64(int64(player.Level))

	// Model
	writer.Write7BitEncodedInt64(player.ModelID)

	// Position
	writer.WriteFloat32(player.X)
	writer.WriteFloat32(player.Y)
	writer.WriteFloat32(player.Z)
	writer.WriteFloat32(player.Facing)

	// Equipment visual slots
	writer.WriteUint32(player.Primary)
	writer.WriteUint32(player.Secondary)
	writer.WriteUint32(player.Shield)
	writer.WriteByte(player.Chest)
	writer.WriteByte(player.Bracer)
	writer.WriteByte(player.Gloves)
	writer.WriteByte(player.Legs)
	writer.WriteByte(player.Boots)
	writer.WriteByte(player.Helm)

	// Appearance
	writer.WriteByte(byte(player.HairColor))
	writer.WriteByte(byte(player.HairLength))
	writer.WriteByte(byte(player.HairStyle))
	writer.WriteByte(byte(player.FaceOption))
	if player.Robe != 0 {
		writer.WriteByte(byte(player.Robe))
	} else {
		writer.WriteByte(0xFF)
	}

	// Entity init — ObjectID for self
	playerObjectID := sess.PlayerObjectID()
	writer.WriteUint32(playerObjectID)

	// HP/Power
	writer.Write7BitEncodedInt64(int64(player.HP))
	writer.Write7BitEncodedInt64(int64(player.MaxHP))
	writer.Write7BitEncodedInt64(int64(player.Power))
	writer.Write7BitEncodedInt64(int64(player.MaxPower))

	data := dataBuf[:writer.Position()]
	msg.SetData(data)
	sess.Queue().AddWithSequence(msg)
	log.Printf("PS2: sent WorldEntry (0x002E) %d bytes for '%s' to %s",
		writer.Position(), player.Name, sess.RemoteAddr())
}

// SendFullEntityCreate sends opcode 0x00B3 — reliable entity creation.
//
// NOT CALLED. The comment below claimed PS2 retail sends this when an entity
// enters visibility range. It does not: 0x00B3 appears 0 times across all five
// PS2 captures in pcaps/, while one session of ours sent 173. Retail's client
// spawns everything from the 28 state channels. Kept only as the wire format,
// should a use for it ever turn up.
//
// Contains: entity ID, name, model, level, equipment, position.
func SendFullEntityCreate(sess *session.Session, objectID uint32, ent entity.Entity) {
	msg := protocol.NewMessage(protocol.MessageTypeReliable, protocol.OpcodeFullEntityCreate)
	dataBuf := make([]byte, 512)
	writer := buffer.NewWriter(dataBuf)
	writer.WriteUint16(uint16(protocol.OpcodeFullEntityCreate))

	// ObjectID as zigzag varint (client reads Read7BitEncodedInt64)
	writer.Write7BitEncodedInt64(int64(objectID))

	switch e := ent.(type) {
	case *entity.NPC:
		writeEntityCreateNPC(writer, e)
	case *entity.Player:
		writeEntityCreatePlayer(writer, e)
	}

	data := dataBuf[:writer.Position()]
	msg.SetData(data)
	sess.Queue().AddWithSequence(msg)
}

// writeEntityCreateNPC writes NPC fields for FullEntityCreate.
// Wire format: name(24) + pos(3×zigzag) + level(zigzag) + modelID(uint32) + hp(byte)
//   + dead(byte) + entityType(byte) + equipment(3×uint32 + 6×byte + 6×byte + 4×byte + byte + byte)
func writeEntityCreateNPC(writer *buffer.Writer, n *entity.NPC) {
	// Name (24 bytes, null-padded)
	nameBytes := make([]byte, 24)
	copy(nameBytes, []byte(n.Name))
	writer.WriteBytes(nameBytes)

	// Position as zigzag varints (coord * 128.0, matching client's division by 128.0)
	writer.Write7BitEncodedInt64(int64(n.X * 128.0))
	writer.Write7BitEncodedInt64(int64(n.Y * 128.0))
	writer.Write7BitEncodedInt64(int64(n.Z * 128.0))

	// Level as zigzag varint
	writer.Write7BitEncodedInt64(int64(n.Level))

	// ModelID (uint32 LE)
	writer.WriteUint32(uint32(n.ModelID))

	// HP bar (0-255 proportional)
	if n.MaxHP > 0 {
		writer.WriteByte(byte(n.HP * 255 / n.MaxHP))
	} else {
		writer.WriteByte(255)
	}

	// Dead flag
	if n.Dead {
		writer.WriteByte(1)
	} else {
		writer.WriteByte(0)
	}

	// Entity type: 0=NPC
	writer.WriteByte(0)

	// Equipment: weapons (3×uint32)
	writer.WriteUint32(uint32(n.PrimaryWeapon))
	writer.WriteUint32(uint32(n.SecondaryWeapon))
	writer.WriteUint32(uint32(n.Shield))

	// Armor slots (6 bytes)
	writer.WriteByte(byte(n.Torso))
	writer.WriteByte(byte(n.Forearm))
	writer.WriteByte(byte(n.NPCGloves))
	writer.WriteByte(byte(n.NPCLegs))
	writer.WriteByte(byte(n.Feet))
	writer.WriteByte(byte(n.Head))

	// Armor tints (6 bytes, 0xFF = default for NPCs)
	writer.WriteByte(0xFF)
	writer.WriteByte(0xFF)
	writer.WriteByte(0xFF)
	writer.WriteByte(0xFF)
	writer.WriteByte(0xFF)
	writer.WriteByte(0xFF)

	// Appearance: hairColor, hairLength, hairStyle, face
	writer.WriteByte(byte(n.HairColor))
	writer.WriteByte(byte(n.HairLength))
	writer.WriteByte(byte(n.HairStyle))
	writer.WriteByte(byte(n.Face))

	// Robe (0xFF = no robe)
	robeVal := byte(0xFF)
	if v, err := strconv.Atoi(n.RobeStyle); err == nil {
		robeVal = byte(v)
	}
	writer.WriteByte(robeVal)

	// Sex (0=male default for NPCs)
	writer.WriteByte(0)
}

// writeEntityCreatePlayer writes Player fields for FullEntityCreate.
// Wire format: name(24) + pos(3×zigzag) + level(zigzag) + modelID(uint32) + hp(byte)
//   + dead(byte) + entityType(byte=1) + equipment(3×uint32 + 6×byte + 6×byte + 4×byte + byte + byte)
func writeEntityCreatePlayer(writer *buffer.Writer, p *entity.Player) {
	// Name (24 bytes, null-padded)
	nameBytes := make([]byte, 24)
	copy(nameBytes, []byte(p.Name))
	writer.WriteBytes(nameBytes)

	// Position as zigzag varints (coord * 128.0)
	writer.Write7BitEncodedInt64(int64(p.X * 128.0))
	writer.Write7BitEncodedInt64(int64(p.Y * 128.0))
	writer.Write7BitEncodedInt64(int64(p.Z * 128.0))

	// Level as zigzag varint
	writer.Write7BitEncodedInt64(int64(p.Level))

	// ModelID (uint32 LE)
	writer.WriteUint32(uint32(p.ModelID))

	// HP bar (0-255 proportional)
	if p.MaxHP > 0 {
		writer.WriteByte(byte(p.HP * 255 / p.MaxHP))
	} else {
		writer.WriteByte(255)
	}

	// Dead flag
	if p.IsDead() {
		writer.WriteByte(1)
	} else {
		writer.WriteByte(0)
	}

	// Entity type: 1=Player
	writer.WriteByte(1)

	// Equipment: weapons (3×uint32)
	writer.WriteUint32(p.Primary)
	writer.WriteUint32(p.Secondary)
	writer.WriteUint32(p.Shield)

	// Armor slots (6 bytes)
	writer.WriteByte(p.Chest)
	writer.WriteByte(p.Bracer)
	writer.WriteByte(p.Gloves)
	writer.WriteByte(p.Legs)
	writer.WriteByte(p.Boots)
	writer.WriteByte(p.Helm)

	// Armor tints (6 bytes, 0x00 = default for players)
	writer.WriteByte(0)
	writer.WriteByte(0)
	writer.WriteByte(0)
	writer.WriteByte(0)
	writer.WriteByte(0)
	writer.WriteByte(0)

	// Appearance: hairColor, hairLength, hairStyle, face
	writer.WriteByte(byte(p.HairColor))
	writer.WriteByte(byte(p.HairLength))
	writer.WriteByte(byte(p.HairStyle))
	writer.WriteByte(byte(p.FaceOption))

	// Robe (0xFF = no robe, low byte of Robe uint32)
	if p.Robe != 0 {
		writer.WriteByte(byte(p.Robe))
	} else {
		writer.WriteByte(0xFF)
	}

	// Sex (0=male, 1=female)
	writer.WriteByte(byte(p.Sex))
}

// SendEntityUpdate sends opcode 0x00B5 — entity property update.
// PS2 retail sends this when an entity's properties change (level up, equip change, etc).
func SendEntityUpdate(sess *session.Session, objectID uint32, ent entity.Entity) {
	msg := protocol.NewMessage(protocol.MessageTypeReliable, protocol.OpcodeEntityUpdate)
	dataBuf := make([]byte, 256)
	writer := buffer.NewWriter(dataBuf)
	writer.WriteUint16(uint16(protocol.OpcodeEntityUpdate))

	// ObjectID
	writer.WriteUint32(objectID)

	// Update fields depend on entity type
	switch e := ent.(type) {
	case *entity.NPC:
		// Position
		writer.WriteFloat32(e.X)
		writer.WriteFloat32(e.Y)
		writer.WriteFloat32(e.Z)
		writer.WriteFloat32(e.Facing)
		// HP bar
		if e.MaxHP > 0 {
			writer.WriteByte(byte(e.HP * 255 / e.MaxHP))
		} else {
			writer.WriteByte(255)
		}
		// Level
		writer.WriteByte(byte(e.Level))
		// Dead
		if e.Dead {
			writer.WriteByte(1)
		} else {
			writer.WriteByte(0)
		}
	case *entity.Player:
		// Position
		writer.WriteFloat32(e.X)
		writer.WriteFloat32(e.Y)
		writer.WriteFloat32(e.Z)
		writer.WriteFloat32(e.Facing)
		// HP bar
		if e.MaxHP > 0 {
			writer.WriteByte(byte(e.HP * 255 / e.MaxHP))
		} else {
			writer.WriteByte(255)
		}
		// Level
		writer.WriteByte(byte(e.Level))
		// Dead
		if e.IsDead() {
			writer.WriteByte(1)
		} else {
			writer.WriteByte(0)
		}
	}

	data := dataBuf[:writer.Position()]
	msg.SetData(data)
	sess.Queue().AddWithSequence(msg)
}

// SendNPCInteraction sends opcode 0x001C — NPC type dispatch.
// Client reads: zigzag(interactionType) + optional string(npcName).
// Type: 3=merchant, 4=blacksmith, 6=quest, 7=banker, 16=trainer, 18=info.
func SendNPCInteraction(sess *session.Session, npcObjectID uint32, interactionType int, npcName string) {
	msg := protocol.NewMessage(protocol.MessageTypeReliable, protocol.OpcodeNPCInteraction)
	dataBuf := make([]byte, 256)
	writer := buffer.NewWriter(dataBuf)
	writer.WriteUint16(uint16(protocol.OpcodeNPCInteraction))

	// Interaction type as zigzag (client reads Read7BitEncodedInt64)
	writer.Write7BitEncodedInt64(int64(interactionType))

	// NPC name (length-prefixed string)
	if npcName != "" {
		writer.WriteInt32(int32(len(npcName)))
		writer.WriteStringUTF8Raw(npcName)
	}

	data := dataBuf[:writer.Position()]
	msg.SetData(data)
	sess.Queue().AddWithSequence(msg)
	log.Printf("PS2: sent NPCInteraction (0x001C) type=%d npc=%q to %s",
		interactionType, npcName, sess.RemoteAddr())
}

// SendNPCMessage sends opcode 0x00D3 — NPC speech.
// PS2 format: 64-byte name + type byte + formatted text.
func SendNPCMessage(sess *session.Session, npcName string, msgType byte, text string) {
	msg := protocol.NewMessage(protocol.MessageTypeReliable, protocol.OpcodeNPCMessage)
	dataBuf := make([]byte, 1024)
	writer := buffer.NewWriter(dataBuf)
	writer.WriteUint16(uint16(protocol.OpcodeNPCMessage))

	// NPC name (64 bytes, null-padded)
	nameBytes := make([]byte, 64)
	copy(nameBytes, []byte(npcName))
	writer.WriteBytes(nameBytes)

	// Message type
	writer.WriteByte(msgType)

	// Text (length-prefixed)
	writer.WriteInt32(int32(len(text)))
	writer.WriteStringUTF8Raw(text)

	data := dataBuf[:writer.Position()]
	msg.SetData(data)
	sess.Queue().AddWithSequence(msg)
}

// SendHPResourceUpdate sends opcode 0x00D9 — HP/resource update.
// Client reads: zigzag(entityID) + zigzag(amount) + optional zigzag(buffType).
func SendHPResourceUpdate(sess *session.Session, entityObjectID uint32, amount int32, buffType int64) {
	msg := protocol.NewMessage(protocol.MessageTypeReliable, protocol.OpcodeHPResourceUpdate)
	dataBuf := make([]byte, 64)
	writer := buffer.NewWriter(dataBuf)
	writer.WriteUint16(uint16(protocol.OpcodeHPResourceUpdate))

	writer.Write7BitEncodedInt64(int64(entityObjectID))
	writer.Write7BitEncodedInt64(int64(amount))
	if buffType != 0 {
		writer.Write7BitEncodedInt64(buffType)
	}

	data := dataBuf[:writer.Position()]
	msg.SetData(data)
	sess.Queue().AddWithSequence(msg)
}

// SendLootResult sends opcode 0x001A — loot outcome.
// Status: 1=ok, 2=partial, 3=fail.
func SendLootResult(sess *session.Session, status byte, items []LootItem) {
	msg := protocol.NewMessage(protocol.MessageTypeReliable, protocol.OpcodeLootResult)
	dataBuf := make([]byte, 2048)
	writer := buffer.NewWriter(dataBuf)
	writer.WriteUint16(uint16(protocol.OpcodeLootResult))

	writer.WriteByte(status)
	writer.Write7BitEncodedInt64(int64(len(items)))

	for _, item := range items {
		writer.WriteUint32(item.ItemID)
		writer.Write7BitEncodedInt64(int64(item.Quantity))
		writer.WriteInt32(int32(len(item.Name)))
		writer.WriteStringUTF8Raw(item.Name)
	}

	data := dataBuf[:writer.Position()]
	msg.SetData(data)
	sess.Queue().AddWithSequence(msg)
}

// LootItem describes an item in a loot result.
type LootItem struct {
	ItemID   uint32
	Quantity int
	Name     string
}

// SendZoneTransferInit sends opcode 0x07B0 — zone transfer initiation.
// PS2 retail sends this to start a zone transition.
func SendZoneTransferInit(sess *session.Session, zoneName string, x, y, z, facing float32) {
	msg := protocol.NewMessage(protocol.MessageTypeReliable, protocol.OpcodeZoneTransferInit)
	dataBuf := make([]byte, 256)
	writer := buffer.NewWriter(dataBuf)
	writer.WriteUint16(uint16(protocol.OpcodeZoneTransferInit))

	// Zone name (length-prefixed)
	writer.WriteInt32(int32(len(zoneName)))
	writer.WriteStringUTF8Raw(zoneName)

	// Destination position
	writer.WriteFloat32(x)
	writer.WriteFloat32(y)
	writer.WriteFloat32(z)
	writer.WriteFloat32(facing)

	// Loading state indicator
	writer.WriteByte(1) // 1 = begin loading

	data := dataBuf[:writer.Position()]
	msg.SetData(data)
	sess.Queue().AddWithSequence(msg)
	log.Printf("PS2: sent ZoneTransferInit (0x07B0) zone=%q to %s", zoneName, sess.RemoteAddr())
}

// SendZoneTransferConfirm sends opcode 0x07B2 — zone load complete.
// PS2 retail sends this after the client finishes loading the new zone.
func SendZoneTransferConfirm(sess *session.Session) {
	msg := protocol.NewMessage(protocol.MessageTypeReliable, protocol.OpcodeZoneTransferConfirm)
	writer := buffer.NewWriter(msg.Span())
	writer.WriteUint16(uint16(protocol.OpcodeZoneTransferConfirm))
	writer.WriteByte(1) // zone load complete
	msg.SetSize(writer.Position())
	sess.Queue().AddWithSequence(msg)
	log.Printf("PS2: sent ZoneTransferConfirm (0x07B2) to %s", sess.RemoteAddr())
}

// SendItemListUpdate sends opcode 0x0065 — full item definitions.
// Used for merchant inventory and quest reward lists.
func SendItemListUpdate(sess *session.Session, listType byte, items []ItemDefinition) {
	msg := protocol.NewMessage(protocol.MessageTypeReliable, protocol.OpcodeItemListUpdate)
	dataBuf := make([]byte, 8192)
	writer := buffer.NewWriter(dataBuf)
	writer.WriteUint16(uint16(protocol.OpcodeItemListUpdate))

	writer.WriteByte(listType) // 0=merchant, 1=quest reward, etc.
	writer.Write7BitEncodedInt64(int64(len(items)))

	for _, item := range items {
		writer.WriteUint32(item.PatternID)
		writer.WriteInt32(int32(len(item.Name)))
		writer.WriteStringUTF8Raw(item.Name)
		writer.Write7BitEncodedInt64(int64(item.Cost))
		writer.Write7BitEncodedInt64(int64(item.Level))
		writer.WriteByte(item.Slot)
	}

	data := dataBuf[:writer.Position()]
	msg.SetData(data)
	sess.Queue().AddWithSequence(msg)
	log.Printf("PS2: sent ItemListUpdate (0x0065) type=%d count=%d to %s",
		listType, len(items), sess.RemoteAddr())
}

// ItemDefinition describes an item for ItemListUpdate.
type ItemDefinition struct {
	PatternID uint32
	Name      string
	Cost      int
	Level     int
	Slot      byte
}

// SendQuestProgress sends opcode 0x007B — quest progress update.
func SendQuestProgress(sess *session.Session, questID uint32, text string, progress int64) {
	msg := protocol.NewMessage(protocol.MessageTypeReliable, protocol.OpcodeUpdateQuestProgress)
	dataBuf := make([]byte, 512)
	writer := buffer.NewWriter(dataBuf)
	writer.WriteUint16(uint16(protocol.OpcodeUpdateQuestProgress))

	writer.WriteUint32(questID)
	writer.WriteInt32(int32(len(text)))
	writer.WriteStringUTF8Raw(text)
	writer.Write7BitEncodedInt64(progress)

	data := dataBuf[:writer.Position()]
	msg.SetData(data)
	sess.Queue().AddWithSequence(msg)
}

// SendQuestStageComplete sends opcode 0x0036 — quest stage completion + XP reward.
func SendQuestStageComplete(sess *session.Session, questID uint32, xpReward int64, stageType byte) {
	msg := protocol.NewMessage(protocol.MessageTypeReliable, protocol.OpcodeQuestStageComplete)
	dataBuf := make([]byte, 64)
	writer := buffer.NewWriter(dataBuf)
	writer.WriteUint16(uint16(protocol.OpcodeQuestStageComplete))

	writer.WriteUint32(questID)
	writer.Write7BitEncodedInt64(xpReward)
	writer.WriteByte(stageType) // 22-type dispatch in PS2

	data := dataBuf[:writer.Position()]
	msg.SetData(data)
	sess.Queue().AddWithSequence(msg)
}

// SendForceDisconnect sends opcode 0x00EC — force client disconnect.
func SendForceDisconnect(sess *session.Session) {
	msg := protocol.NewMessage(protocol.MessageTypeReliable, protocol.OpcodeForceDisconnect)
	writer := buffer.NewWriter(msg.Span())
	writer.WriteUint16(uint16(protocol.OpcodeForceDisconnect))
	msg.SetSize(writer.Position())
	sess.Queue().AddWithSequence(msg)
	log.Printf("PS2: sent ForceDisconnect (0x00EC) to %s", sess.RemoteAddr())
}

// SendChatMessage sends opcode 0x07A3 — chat message with sender metadata.
func SendChatMessage(sess *session.Session, senderName string, chatType byte, msgID int64, text string) {
	msg := protocol.NewMessage(protocol.MessageTypeReliable, protocol.OpcodeChatMessage)
	dataBuf := make([]byte, 1024)
	writer := buffer.NewWriter(dataBuf)
	writer.WriteUint16(uint16(protocol.OpcodeChatMessage))

	// Sender name (24 bytes, null-padded)
	nameBytes := make([]byte, 24)
	copy(nameBytes, []byte(senderName))
	writer.WriteBytes(nameBytes)

	// Chat type
	writer.WriteByte(chatType)

	// Message ID (zigzag)
	writer.Write7BitEncodedInt64(msgID)

	// Text (length-prefixed)
	writer.WriteInt32(int32(len(text)))
	writer.WriteStringUTF8Raw(text)

	data := dataBuf[:writer.Position()]
	msg.SetData(data)
	sess.Queue().AddWithSequence(msg)
}

// =============================================================================
// Combat / XP / Death opcodes (PS2-correct wire formats)
// Verified against Go client handlers in internal/game/state.go
// =============================================================================

// SendGrantXP sends opcode 0x0020 — level/XP update.
// Client reads: zigzag(level) + zigzag(xpInLevel).
func SendGrantXP(sess *session.Session, level int, xpInLevel int64) {
	msg := protocol.NewMessage(protocol.MessageTypeReliable, protocol.OpcodeGrantXP)
	dataBuf := make([]byte, 32)
	writer := buffer.NewWriter(dataBuf)
	writer.WriteUint16(uint16(protocol.OpcodeGrantXP))

	writer.Write7BitEncodedInt64(int64(level))
	writer.Write7BitEncodedInt64(xpInLevel)

	data := dataBuf[:writer.Position()]
	msg.SetData(data)
	sess.Queue().AddWithSequence(msg)
	log.Printf("PS2: sent GrantXP (0x0020) level=%d xp=%d to %s", level, xpInLevel, sess.RemoteAddr())
}

// SendDamageIndication sends opcode 0x00DB — floating damage/heal number.
// Client reads: uint32(sourceObjID) + zigzag(damage) + uint32(targetObjID).
// Negative damage = damage dealt, positive = heal.
func SendDamageIndication(sess *session.Session, sourceObjectID uint32, damage int64, targetObjectID uint32) {
	msg := protocol.NewMessage(protocol.MessageTypeReliable, protocol.OpcodeDamageIndication)
	dataBuf := make([]byte, 32)
	writer := buffer.NewWriter(dataBuf)
	writer.WriteUint16(uint16(protocol.OpcodeDamageIndication))

	writer.WriteUint32(sourceObjectID)
	writer.Write7BitEncodedInt64(damage) // negative=damage, positive=heal
	writer.WriteUint32(targetObjectID)

	data := dataBuf[:writer.Position()]
	msg.SetData(data)
	sess.Queue().AddWithSequence(msg)
}

// SendCastingCombat sends opcode 0x00B1 — combat action (melee swing or spell cast).
// PS2 func_00BD1F10 reads:
//
//	raw(4,1) sourceEntityID
//	raw(4,1) targetEntityID
//	raw(1,1) additionalTargetCount (uint8)
//	[N × raw(4,1)] additionalTargetIDs (if count > 0)
//	raw(4,1) castTimeMS (uint32, not uint16!)
//	raw(4,1) spellEffectDictID (for VISpellEffectSystem::Play via FindTyped)
//
// castTimeMS >= 3000 = auto-attack, > 0 && < 4000 = spell cast, 0 = instant.
func SendCastingCombat(sess *session.Session, casterObjectID, targetObjectID uint32, castTimeMS uint32, spellFXDictID uint32) {
	msg := protocol.NewMessage(protocol.MessageTypeReliable, protocol.OpcodeCastingCombat)
	dataBuf := make([]byte, 32)
	writer := buffer.NewWriter(dataBuf)
	writer.WriteUint16(uint16(protocol.OpcodeCastingCombat))

	writer.WriteUint32(casterObjectID)
	writer.WriteUint32(targetObjectID)
	writer.WriteByte(0)              // additionalTargetCount = 0
	writer.WriteUint32(castTimeMS)   // uint32 (PS2 raw(4,1))
	writer.WriteUint32(spellFXDictID) // spell effect DictID

	data := dataBuf[:writer.Position()]
	msg.SetData(data)
	sess.Queue().AddWithSequence(msg)
}

// SendCooldownTimer sends opcode 0x00CD — spell cooldown notification.
// Client reads: byte(addedOrder) + 4 zero bytes + zigzag(cooldownMs).
// addedOrder identifies which spell (matched to hotbar by client).
func SendCooldownTimer(sess *session.Session, addedOrder byte, cooldownMs int64) {
	msg := protocol.NewMessage(protocol.MessageTypeReliable, protocol.OpcodeCooldownTimer)
	dataBuf := make([]byte, 32)
	writer := buffer.NewWriter(dataBuf)
	writer.WriteUint16(uint16(protocol.OpcodeCooldownTimer))

	writer.WriteByte(addedOrder)
	writer.WriteUint32(0) // 4 reserved zero bytes
	writer.Write7BitEncodedInt64(cooldownMs)

	data := dataBuf[:writer.Position()]
	msg.SetData(data)
	sess.Queue().AddWithSequence(msg)
}

// SendCharacterDied sends opcode 0x005E — player death notification.
// Client reads: no unique fields (just opcode). Sets dead state immediately.
func SendCharacterDied(sess *session.Session) {
	msg := protocol.NewMessage(protocol.MessageTypeReliable, protocol.OpcodeCharacterDied)
	dataBuf := make([]byte, 8)
	writer := buffer.NewWriter(dataBuf)
	writer.WriteUint16(uint16(protocol.OpcodeCharacterDied))

	data := dataBuf[:writer.Position()]
	msg.SetData(data)
	sess.Queue().AddWithSequence(msg)
	log.Printf("PS2: sent CharacterDied (0x005E) to %s", sess.RemoteAddr())
}

// SendResourceUpdate sends opcode 0x0775 — HP/resource animation trigger.
// PS2: uint32(objectID) + uint8(animationAction). Client currently reads zigzag.
func SendResourceUpdate(sess *session.Session, objectID uint32, action byte) {
	msg := protocol.NewMessage(protocol.MessageTypeReliable, protocol.OpcodeResourceUpdate)
	dataBuf := make([]byte, 16)
	writer := buffer.NewWriter(dataBuf)
	writer.WriteUint16(uint16(protocol.OpcodeResourceUpdate))

	writer.Write7BitEncodedInt64(int64(objectID))
	writer.WriteByte(action)

	data := dataBuf[:writer.Position()]
	msg.SetData(data)
	sess.Queue().AddWithSequence(msg)
}

// SendPlayerSpeed sends opcode 0x00F8 — player movement speed.
// Client reads: float32 (4 bytes LE).
func SendPlayerSpeed(sess *session.Session, speed float32) {
	msg := protocol.NewMessage(protocol.MessageTypeReliable, protocol.OpcodePlayerSpeed)
	dataBuf := make([]byte, 16)
	writer := buffer.NewWriter(dataBuf)
	writer.WriteUint16(uint16(protocol.OpcodePlayerSpeed))

	// Speed as float32 (client reads ReadFloat32 which reads 4 bytes LE)
	writer.WriteUint32(math.Float32bits(speed))

	data := dataBuf[:writer.Position()]
	msg.SetData(data)
	sess.Queue().AddWithSequence(msg)
}

// =============================================================================
// Entity update cascade senders (PS2 world recovery opcodes)
// These are used during reconnect/zone load to restore entity state.
// =============================================================================

// SendEntityUpdateFullUI sends opcode 0x07F4 — enables HUD + map + full entity update.
func SendEntityUpdateFullUI(sess *session.Session, objectID uint32, ent entity.Entity) {
	sendEntityUpdateCascade(sess, protocol.OpcodeEntityUpdateFullUI, objectID, ent)
}

// SendEntityUpdateMapPanel sends opcode 0x07F0 — enables map + entity update.
func SendEntityUpdateMapPanel(sess *session.Session, objectID uint32, ent entity.Entity) {
	sendEntityUpdateCascade(sess, protocol.OpcodeEntityUpdateMapPanel, objectID, ent)
}

// SendEntityUpdateDirect sends opcode 0x07C0 — bare position/zone update.
func SendEntityUpdateDirect(sess *session.Session, objectID uint32, ent entity.Entity) {
	sendEntityUpdateCascade(sess, protocol.OpcodeEntityUpdateDirect, objectID, ent)
}

// sendEntityUpdateCascade is the shared body for entity update cascade opcodes.
// PS2 uses fall-through entry points that all converge to the same update logic.
func sendEntityUpdateCascade(sess *session.Session, opcode protocol.GameOpcode, objectID uint32, ent entity.Entity) {
	msg := protocol.NewMessage(protocol.MessageTypeReliable, opcode)
	dataBuf := make([]byte, 256)
	writer := buffer.NewWriter(dataBuf)
	writer.WriteUint16(uint16(opcode))

	writer.WriteUint32(objectID)

	x, y, z := ent.GetPosition()
	writer.WriteFloat32(x)
	writer.WriteFloat32(y)
	writer.WriteFloat32(z)

	// Entity name
	var name string
	switch e := ent.(type) {
	case *entity.NPC:
		name = e.Name
	case *entity.Player:
		name = e.Name
	}
	nameBytes := make([]byte, 24)
	copy(nameBytes, []byte(name))
	writer.WriteBytes(nameBytes)

	// Flag byte (varies by opcode in PS2 but we send 0 for basic update)
	writer.WriteByte(0)

	data := dataBuf[:writer.Position()]
	msg.SetData(data)
	sess.Queue().AddWithSequence(msg)
}

// =============================================================================
// Helpers used by the visibility system to send FullEntityCreate
// =============================================================================

// SerializeFullEntityCreate builds a reliable message buffer for 0x00B3.
// Called from visibility.go when a new entity enters a player's view.
func SerializeFullEntityCreate(objectID uint32, ent entity.Entity) []byte {
	dataBuf := make([]byte, 512)
	writer := buffer.NewWriter(dataBuf)
	writer.WriteUint16(uint16(protocol.OpcodeFullEntityCreate))

	// ObjectID as zigzag varint (matching SendFullEntityCreate and client's parseEntityReliable)
	writer.Write7BitEncodedInt64(int64(objectID))

	switch e := ent.(type) {
	case *entity.NPC:
		writeEntityCreateNPC(writer, e)
	case *entity.Player:
		writeEntityCreatePlayer(writer, e)
	}

	return dataBuf[:writer.Position()]
}

// SendPlayerSpeedPS2 is the CharacterHandler method version of SendPlayerSpeed.
// Delegates to the standalone function.
func (h *CharacterHandler) SendPlayerSpeedPS2(sess *session.Session, speed float32) {
	SendPlayerSpeed(sess, speed)
}

// =============================================================================
// Chat / Message opcodes
// =============================================================================

// SendColoredChat sends opcode 0x0A7B — colored text message (MOTD, shout, etc).
// Client reads: int32(charCount) + UTF-16LE(chars) + 4 color bytes (RGBA).
func SendColoredChat(sess *session.Session, text string, r, g, b, a uint8) {
	msg := protocol.NewMessage(protocol.MessageTypeReliable, protocol.OpcodeColoredChat)
	dataBuf := make([]byte, 4096)
	writer := buffer.NewWriter(dataBuf)
	writer.WriteUint16(uint16(protocol.OpcodeColoredChat))

	// UTF-16LE string with int32 char count prefix
	writer.WriteStringUnicode(text)

	// RGBA color bytes
	writer.WriteByte(r)
	writer.WriteByte(g)
	writer.WriteByte(b)
	writer.WriteByte(a)

	data := dataBuf[:writer.Position()]
	msg.SetData(data)
	sess.Queue().AddWithSequence(msg)
}

// SendClientMessage sends opcode 0x0A7A — system/broadcast message (white text).
// Client reads: int32(charCount) + UTF-16LE(chars).
func SendClientMessage(sess *session.Session, text string) {
	msg := protocol.NewMessage(protocol.MessageTypeReliable, protocol.OpcodeClientMessage)
	dataBuf := make([]byte, 4096)
	writer := buffer.NewWriter(dataBuf)
	writer.WriteUint16(uint16(protocol.OpcodeClientMessage))

	writer.WriteStringUnicode(text)

	data := dataBuf[:writer.Position()]
	msg.SetData(data)
	sess.Queue().AddWithSequence(msg)
}

// SendErrorMessage sends opcode 0x00CE — error text (red).
// Client reads: int32(charCount) + UTF-16LE(chars).
func SendErrorMessage(sess *session.Session, text string) {
	msg := protocol.NewMessage(protocol.MessageTypeReliable, protocol.OpcodeErrorMessage)
	dataBuf := make([]byte, 4096)
	writer := buffer.NewWriter(dataBuf)
	writer.WriteUint16(uint16(protocol.OpcodeErrorMessage))

	writer.WriteStringUnicode(text)

	data := dataBuf[:writer.Position()]
	msg.SetData(data)
	sess.Queue().AddWithSequence(msg)
}

// =============================================================================
// Server status / keepalive opcodes
// =============================================================================

// SendServerReady sends opcode 0x07E0 — server keepalive/ready signal.
// No payload — just the opcode. Sandstorm sends this periodically.
func SendServerReady(sess *session.Session) {
	msg := protocol.NewMessage(protocol.MessageTypeReliable, protocol.OpcodeServerReady)
	writer := buffer.NewWriter(msg.Span())
	writer.WriteUint16(uint16(protocol.OpcodeServerReady))
	msg.SetSize(writer.Position())
	sess.Queue().AddWithSequence(msg)
}

// =============================================================================
// Combat feedback opcodes
// =============================================================================

// SendOutMatched sends opcode 0x077A — "You are outmatched!" warning.
// No payload — just the opcode. Sent when player targets a mob too far above their level.
func SendOutMatched(sess *session.Session) {
	msg := protocol.NewMessage(protocol.MessageTypeReliable, protocol.OpcodeOutMatched)
	writer := buffer.NewWriter(msg.Span())
	writer.WriteUint16(uint16(protocol.OpcodeOutMatched))
	msg.SetSize(writer.Position())
	sess.Queue().AddWithSequence(msg)
}

// SendClassicTargetInfo sends opcode 0x0010 — target con/face colors.
// Client reads: byte(faceColor) + byte(conColor) + padding.
func SendClassicTargetInfo(sess *session.Session, faceColor, conColor byte) {
	msg := protocol.NewMessage(protocol.MessageTypeReliable, protocol.OpcodeClassicTargetInfo)
	dataBuf := make([]byte, 8)
	writer := buffer.NewWriter(dataBuf)
	writer.WriteUint16(uint16(protocol.OpcodeClassicTargetInfo))

	writer.WriteByte(faceColor)
	writer.WriteByte(conColor)
	writer.WriteByte(0) // padding byte

	data := dataBuf[:writer.Position()]
	msg.SetData(data)
	sess.Queue().AddWithSequence(msg)
}

// =============================================================================
// Economy / currency opcodes
// =============================================================================

// SendPlayerTunar sends opcode 0x0052 — update player's tunar (gold) amount.
// Client reads: zigzag(tunarAmount).
func SendPlayerTunar(sess *session.Session, tunar int64) {
	msg := protocol.NewMessage(protocol.MessageTypeReliable, protocol.OpcodePlayerTunar)
	dataBuf := make([]byte, 16)
	writer := buffer.NewWriter(dataBuf)
	writer.WriteUint16(uint16(protocol.OpcodePlayerTunar))

	writer.Write7BitEncodedInt64(tunar)

	data := dataBuf[:writer.Position()]
	msg.SetData(data)
	sess.Queue().AddWithSequence(msg)
}

// =============================================================================
// Weather opcodes
// =============================================================================

// SendWeatherControl sends opcode 0x00C5 — a weather change FADED over 30
// seconds: CLIENT 0x00BD2CC0 calls VIAtmosphere::SetWeather(type, false).
// Client reads: byte(weatherType), the WXTYPE: 0 clear, 1..3 light/medium/heavy
// rain, 4..6 light/medium/heavy snow.
func SendWeatherControl(sess *session.Session, weatherType byte) {
	msg := protocol.NewMessage(protocol.MessageTypeReliable, protocol.OpcodeWeatherControl)
	dataBuf := make([]byte, 8)
	writer := buffer.NewWriter(dataBuf)
	writer.WriteUint16(uint16(protocol.OpcodeWeatherControl))
	writer.WriteByte(weatherType)
	data := dataBuf[:writer.Position()]
	msg.SetData(data)
	sess.Queue().AddWithSequence(msg)
}

// SendSkyboxWeather sends opcode 0x00C7 — an INSTANT weather change: CLIENT
// 0x00BD2CEC calls VIAtmosphere::SetWeather(type, true).
// Client reads: byte(weatherType). Same types as WeatherControl.
func SendSkyboxWeather(sess *session.Session, weatherType byte) {
	msg := protocol.NewMessage(protocol.MessageTypeReliable, protocol.OpcodeSkyboxWeather)
	dataBuf := make([]byte, 8)
	writer := buffer.NewWriter(dataBuf)
	writer.WriteUint16(uint16(protocol.OpcodeSkyboxWeather))
	writer.WriteByte(weatherType)
	data := dataBuf[:writer.Position()]
	msg.SetData(data)
	sess.Queue().AddWithSequence(msg)
}

// =============================================================================
// NPC Dialogue opcodes
// =============================================================================

// SendDialogueBox sends opcode 0x0046 — quest/NPC dialogue text (no options).
// Client reads: uint32(counter) + unicodeString(text).
func SendDialogueBox(sess *session.Session, counter uint32, text string) {
	msg := protocol.NewMessage(protocol.MessageTypeReliable, protocol.OpcodeDialogueBox)
	dataBuf := make([]byte, 4096)
	writer := buffer.NewWriter(dataBuf)
	writer.WriteUint16(uint16(protocol.OpcodeDialogueBox))

	writer.WriteUint32(counter)
	writer.WriteStringUnicode(text)

	data := dataBuf[:writer.Position()]
	msg.SetData(data)
	sess.Queue().AddWithSequence(msg)
}

// SendQuestPopupMulti sends opcode 0x0034 — dialogue with multiple choice options.
// Client reads: uint32(counter) + unicodeString(greeting) + byte(optionCount) + N × unicodeString(option).
func SendQuestPopupMulti(sess *session.Session, counter uint32, greeting string, options []string) {
	msg := protocol.NewMessage(protocol.MessageTypeReliable, protocol.OpcodeQuestPopupMulti)
	dataBuf := make([]byte, 8192)
	writer := buffer.NewWriter(dataBuf)
	writer.WriteUint16(uint16(protocol.OpcodeQuestPopupMulti))

	writer.WriteUint32(counter)
	writer.WriteStringUnicode(greeting)

	// Option count (byte, max 255)
	count := len(options)
	if count > 255 {
		count = 255
	}
	writer.WriteByte(byte(count))

	for i := 0; i < count; i++ {
		writer.WriteStringUnicode(options[i])
	}

	data := dataBuf[:writer.Position()]
	msg.SetData(data)
	sess.Queue().AddWithSequence(msg)
}

// =============================================================================
// Group opcodes
// =============================================================================

// SendGroupInviteResponse sends opcode 0x0773 — group invite to a player.
// Client reads: uint32(groupID) + uint32(inviterOID) + uint32(nameLen) + ASCII(name).
func SendGroupInviteResponse(sess *session.Session, groupID, inviterObjectID uint32, inviterName string) {
	msg := protocol.NewMessage(protocol.MessageTypeReliable, protocol.OpcodeGroupInviteResponse)
	dataBuf := make([]byte, 256)
	writer := buffer.NewWriter(dataBuf)
	writer.WriteUint16(uint16(protocol.OpcodeGroupInviteResponse))

	writer.WriteUint32(groupID)
	writer.WriteUint32(inviterObjectID)
	writer.WriteUint32(uint32(len(inviterName)))
	writer.WriteStringUTF8Raw(inviterName)

	data := dataBuf[:writer.Position()]
	msg.SetData(data)
	sess.Queue().AddWithSequence(msg)
}

// SendGroupChat sends opcode 0x069E — group chat message.
// Client reads: uint32(senderOID) + unicodeString(text).
func SendGroupChat(sess *session.Session, senderObjectID uint32, text string) {
	msg := protocol.NewMessage(protocol.MessageTypeReliable, protocol.OpcodeGroupChat)
	dataBuf := make([]byte, 1024)
	writer := buffer.NewWriter(dataBuf)
	writer.WriteUint16(uint16(protocol.OpcodeGroupChat))

	writer.WriteUint32(senderObjectID)
	writer.WriteStringUnicode(text)

	data := dataBuf[:writer.Position()]
	msg.SetData(data)
	sess.Queue().AddWithSequence(msg)
}

// =============================================================================
// Training / skills opcodes
// =============================================================================

// SendUpdateTrainingPoints sends opcode 0x001D — training point update.
// Client reads: zigzag(value).
func SendUpdateTrainingPoints(sess *session.Session, points int64) {
	msg := protocol.NewMessage(protocol.MessageTypeReliable, protocol.OpcodeUpdateTrainingPoints)
	dataBuf := make([]byte, 16)
	writer := buffer.NewWriter(dataBuf)
	writer.WriteUint16(uint16(protocol.OpcodeUpdateTrainingPoints))

	writer.Write7BitEncodedInt64(points)

	data := dataBuf[:writer.Position()]
	msg.SetData(data)
	sess.Queue().AddWithSequence(msg)
}

// =============================================================================
// Disconnect opcodes
// =============================================================================

// SendDisconnectNoError sends opcode 0x0777 — clean disconnect (no error message).
func SendDisconnectNoError(sess *session.Session) {
	msg := protocol.NewMessage(protocol.MessageTypeReliable, protocol.OpcodeDisconnectNoError)
	writer := buffer.NewWriter(msg.Span())
	writer.WriteUint16(uint16(protocol.OpcodeDisconnectNoError))
	msg.SetSize(writer.Position())
	sess.Queue().AddWithSequence(msg)
}

// SendLoggedInElsewhere sends opcode 0x07A4 — "logged in from another location" disconnect.
func SendLoggedInElsewhere(sess *session.Session) {
	msg := protocol.NewMessage(protocol.MessageTypeReliable, protocol.OpcodeLoggedInFromAnotherLocationDC)
	writer := buffer.NewWriter(msg.Span())
	writer.WriteUint16(uint16(protocol.OpcodeLoggedInFromAnotherLocationDC))
	msg.SetSize(writer.Position())
	sess.Queue().AddWithSequence(msg)
}

// =============================================================================
// Loot opcodes
// =============================================================================

// SendLoot sends opcode 0x0018 — open loot window with items.
// Wire format mirrors the item parser used by AddInvItem/EquipItem.
func SendLoot(sess *session.Session, items []LootWindowItem) {
	msg := protocol.NewMessage(protocol.MessageTypeReliable, protocol.OpcodeLoot)
	dataBuf := make([]byte, 4096)
	writer := buffer.NewWriter(dataBuf)
	writer.WriteUint16(uint16(protocol.OpcodeLoot))

	writer.Write7BitEncodedInt64(int64(len(items)))
	for _, item := range items {
		writer.WriteUint32(item.ItemID)
		writer.Write7BitEncodedInt64(int64(item.Quantity))
		writer.WriteStringUnicode(item.Name)
		writer.Write7BitEncodedInt64(int64(item.IconID))
	}

	data := dataBuf[:writer.Position()]
	msg.SetData(data)
	sess.Queue().AddWithSequence(msg)
}

// LootWindowItem describes an item in the loot popup.
type LootWindowItem struct {
	ItemID   uint32
	Quantity int
	Name     string
	IconID   int
}

// =============================================================================
// Quest opcodes
// =============================================================================

// SendDeleteQuest sends opcode 0x007D — remove quest from journal.
func SendDeleteQuest(sess *session.Session, questID uint32) {
	msg := protocol.NewMessage(protocol.MessageTypeReliable, protocol.OpcodeDeleteQuest)
	dataBuf := make([]byte, 16)
	writer := buffer.NewWriter(dataBuf)
	writer.WriteUint16(uint16(protocol.OpcodeDeleteQuest))
	writer.WriteUint32(questID)
	data := dataBuf[:writer.Position()]
	msg.SetData(data)
	sess.Queue().AddWithSequence(msg)
}

// SendClearQuestTracker sends opcode 0x007F — clear active quest tracker.
func SendClearQuestTracker(sess *session.Session) {
	msg := protocol.NewMessage(protocol.MessageTypeReliable, protocol.OpcodeClearQuestTracker)
	writer := buffer.NewWriter(msg.Span())
	writer.WriteUint16(uint16(protocol.OpcodeClearQuestTracker))
	msg.SetSize(writer.Position())
	sess.Queue().AddWithSequence(msg)
}

// Ensure binary import is used (for entity cascade helpers)
var _ = binary.LittleEndian
