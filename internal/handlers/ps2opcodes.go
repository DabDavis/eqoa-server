package handlers

import (
	"log"
	"math"
	"unicode/utf16"

	"github.com/eqoa/pkg/pkg/buffer"
	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// =============================================================================
// Reliable (0xFB) opcode senders, each written the way CLIENT's dispatcher
// reads it: VIClient::ProcessMessage in the CLIENT decomp
// (decomp/modules/client/src/viclient.cpp) and the per-opcode map in
// claude-notes/decomp-phase62-dispatcher-survey-20261015.md. The address of
// the handler that reads each one is beside it.
//
// The rewritten ones are a build function, which returns the payload with the
// opcode first and is what the tests check, and a Send wrapper that queues it.
// =============================================================================

// build returns the opcode followed by whatever fill writes.
func build(op protocol.GameOpcode, size int, fill func(w *buffer.Writer)) []byte {
	buf := make([]byte, size)
	w := buffer.NewWriter(buf)
	w.WriteUint16(uint16(op))
	if fill != nil {
		fill(w)
	}
	return buf[:w.Position()]
}

// queueReliable queues one guaranteed message whose payload starts with op.
func queueReliable(sess *session.Session, op protocol.GameOpcode, payload []byte) {
	msg := protocol.NewMessage(protocol.MessageTypeReliable, op)
	msg.SetData(payload)
	sess.Queue().AddWithSequence(msg)
}

// writeString8 writes a string as VIMemStream::ReadString(char *, MaxBytes)
// (0x00CB8740) reads one: a u32 byte count, then the bytes. CLIENT keeps it
// only when count+1 <= MaxBytes and otherwise skips it and keeps "", so it is
// cut to maxBytes-1 here.
func writeString8(w *buffer.Writer, s string, maxBytes int) {
	b := []byte(s)
	if n := maxBytes - 1; len(b) > n {
		b = b[:n]
	}
	w.WriteUint32(uint32(len(b)))
	w.WriteBytes(b)
}

// writeWideString writes a string as VIMemStream::ReadString(unsigned short *,
// MaxBytes) (0x00CB87E8) reads one: a u32 count of UTF-16 units, then the
// units. CLIENT keeps it only when count+1 <= MaxBytes/2, so it is cut to that
// (never between the halves of a surrogate pair). The count is of units:
// buffer.WriteStringUnicode counts runes, which differs past the BMP.
func writeWideString(w *buffer.Writer, s string, maxBytes int) {
	u := utf16.Encode([]rune(s))
	if n := maxBytes/2 - 1; len(u) > n {
		u = u[:n]
		if k := len(u); k > 0 && u[k-1] >= 0xD800 && u[k-1] < 0xDC00 {
			u = u[:k-1]
		}
	}
	w.WriteUint32(uint32(len(u)))
	for _, c := range u {
		w.WriteUint16(c)
	}
}

// writeFixedWide writes exactly n UTF-16 units, NUL-padded: a field CLIENT
// reads raw with Read(buf, 2, n), not as a counted string.
func writeFixedWide(w *buffer.Writer, s string, n int) {
	u := utf16.Encode([]rune(s))
	if len(u) > n-1 {
		u = u[:n-1]
	}
	for i := 0; i < n; i++ {
		var c uint16
		if i < len(u) {
			c = u[i]
		}
		w.WriteUint16(c)
	}
}

// =============================================================================
// Disconnects and the login window
// =============================================================================

// buildDisconnect is one of the opcodes CLIENT answers with
// Disconnect(reason, 0) and nothing else. What the console then shows is
// ConnectStatusString's word for the reason (0x00BC2268):
//
//	0x0728 reason 6   620 "Server not ready, please try again later. (nodbs)"
//	0x0729 reason 3   270 "Bad Login/Password"
//	0x072A reason 7   621 "Server not ready, please try again later. (loginto)"
//	0x072B reason 4   593 "Not Subscribed."
//	0x00EC, 0x07A4 8  619 "You have logged in from another location."
//	0x07A1 reason 13  677 (the character's area is temporarily unavailable)
//	0x07A2 reason 14  716 "You have been ejected by a server administrator."
//	0x07C0 reason 17  718 "We are updating the game, please try again later."
//	0x07C1 reason 18  718, as 0x07C0
//	0x07D2 reason 19  1420 (modified by a customer service representative)
//
// 0x07A1 alone reads eight bytes first (Msg07A1), so it carries eight zeros.
// 0x0777 is the quiet one (SendDisconnectNoError).
func buildDisconnect(op protocol.GameOpcode) []byte {
	return build(op, 16, func(w *buffer.Writer) {
		if op == protocol.OpcodeZoneUnavailableDC {
			w.WriteUint32(0)
			w.WriteUint32(0)
		}
	})
}

// SendDisconnect sends one of buildDisconnect's opcodes.
func SendDisconnect(sess *session.Session, op protocol.GameOpcode) {
	queueReliable(sess, op, buildDisconnect(op))
	log.Printf("PS2: sent disconnect 0x%04X to %s", uint16(op), sess.RemoteAddr())
}

// SendForceDisconnect sends opcode 0x00EC: Disconnect(8), word 619 "You have
// logged in from another location."
func SendForceDisconnect(sess *session.Session) {
	SendDisconnect(sess, protocol.OpcodeForceDisconnect)
}

// SendLoggedInElsewhere sends opcode 0x07A4: Disconnect(8), as 0x00EC.
func SendLoggedInElsewhere(sess *session.Session) {
	SendDisconnect(sess, protocol.OpcodeLoggedInFromAnotherLocationDC)
}

// SendDisconnectNoError sends opcode 0x0777 — Msg0777: a quiet disconnect back
// to the login window, with no notice.
func SendDisconnectNoError(sess *session.Session) {
	queueReliable(sess, protocol.OpcodeDisconnectNoError, build(protocol.OpcodeDisconnectNoError, 2, nil))
}

// buildCreateCharFailed is one of the three character-create failures, none
// with a payload: each shows a word in the login window and takes one off
// CLIENT's character count. 0x002E word 269 "Character create failed", 0x0065
// word 828 "Failed: invalid name", 0x002F word 829 "Failed: name already taken".
func buildCreateCharFailed(op protocol.GameOpcode) []byte {
	return build(op, 2, nil)
}

// SendCreateCharFailed sends one of buildCreateCharFailed's opcodes.
func SendCreateCharFailed(sess *session.Session, op protocol.GameOpcode) {
	queueReliable(sess, op, buildCreateCharFailed(op))
}

// buildCharSelectNotice is one of the character-select notices (no payload):
// 0x07F2 and 0x07F4 show word 1662 (that character needs the Frontiers disc),
// 0x07F0, 0x07F1 and 0x07F3 word 676 (the character's area of the world is
// unavailable).
func buildCharSelectNotice(op protocol.GameOpcode) []byte {
	return build(op, 2, nil)
}

// SendCharSelectNotice sends one of buildCharSelectNotice's opcodes.
func SendCharSelectNotice(sess *session.Session, op protocol.GameOpcode) {
	queueReliable(sess, op, buildCharSelectNotice(op))
}

// =============================================================================
// The ability list (VIClient+0x2BB2C). 0x00B3..0x00FF edit it; none of them
// is an entity message.
// =============================================================================

// writeAbilityRecord writes one spellbook entry as VIAbility::Read
// (0x00BDCBE0) reads it, which is also how the MemoryDump carries the list:
// seven zigzag ints (id, display index, valid, slot, lockout, +0x14, level),
// the range as a raw float32, seven more (cast time, power cost, the two icon
// hashes, scope, cooldown, equip mask), a wide name of at most 63 characters,
// a wide description of at most 127 and a zigzag count of sub-records (none).
//
// The database holds the range the way the C# reference parsed it: the
// float's high half as Range, and its two low bytes as the zigzag "Unk2" and
// "Unk3" that came before it. So the float is put back together from all
// three; for the low bytes to be single zigzag bytes, Unk2 and Unk3 must be in
// 0..63, and Sandstorm's always are (they are 0).
//
// The icon hashes are written as signed 32-bit values: CLIENT gathers a
// zigzag int into 32 bits, so a hash stored unsigned would lose its top bit.
func writeAbilityRecord(w *buffer.Writer, e *database.CharSpellEntry) {
	sp := &e.Pattern
	for _, v := range []int64{int64(sp.SpellID), int64(e.AddedOrder), int64(e.OnHotBar),
		int64(e.WhereOnHotBar), int64(e.Unk1), int64(e.ShowHide), int64(sp.AbilityLvl)} {
		w.Write7BitEncodedInt64(v)
	}
	lowByte := func(v int) uint32 {
		if v < 0 || v > 63 {
			return 0
		}
		return uint32(v << 1) // zigzag of a small non-negative value, one byte
	}
	w.WriteUint32(uint32(uint16(sp.Range))<<16 | lowByte(sp.Unk3)<<8 | lowByte(sp.Unk2))
	for _, v := range []int64{int64(sp.CastTime), int64(sp.Power), int64(int32(sp.IconColor)),
		int64(int32(sp.Icon)), int64(sp.Scope), int64(sp.Recast), int64(sp.EqpRequire)} {
		w.Write7BitEncodedInt64(v)
	}
	writeWideString(w, sp.SpellName, 128)
	writeWideString(w, sp.SpellDesc, 256)
	w.Write7BitEncodedInt64(0) // no sub-records
}

// buildAbilityAdd is opcode 0x00B3 — Msg00B3 (0x00BCF3A0): the u32 ability
// stamp (VIClient+0x2BB60, sent back first in 0x0032/0x0033/0x00FE), a u32
// flag that shows word 721 "You have gained a new ability." when set, then one
// ability record. CLIENT's map insert keeps an existing record of the same id.
func buildAbilityAdd(stamp uint32, announce bool, e *database.CharSpellEntry) []byte {
	return build(protocol.OpcodeAbilityAdd, 1024, func(w *buffer.Writer) {
		w.WriteUint32(stamp)
		var flag uint32
		if announce {
			flag = 1
		}
		w.WriteUint32(flag)
		writeAbilityRecord(w, e)
	})
}

// SendAbilityAdd sends opcode 0x00B3.
func SendAbilityAdd(sess *session.Session, stamp uint32, announce bool, e *database.CharSpellEntry) {
	queueReliable(sess, protocol.OpcodeAbilityAdd, buildAbilityAdd(stamp, announce, e))
}

// SendCooldownTimer sends opcode 0x00CD — the case at 0x00BD2168 and Msg00CD:
// u8 display index, the u32 ability stamp (0 here), zigzag lockout in ms, which
// lands on the first ability with that display index.
func SendCooldownTimer(sess *session.Session, addedOrder byte, cooldownMs int64) {
	msg := protocol.NewMessage(protocol.MessageTypeReliable, protocol.OpcodeCooldownTimer)
	dataBuf := make([]byte, 32)
	writer := buffer.NewWriter(dataBuf)
	writer.WriteUint16(uint16(protocol.OpcodeCooldownTimer))

	writer.WriteByte(addedOrder)
	writer.WriteUint32(0) // the ability stamp
	writer.Write7BitEncodedInt64(cooldownMs)

	data := dataBuf[:writer.Position()]
	msg.SetData(data)
	sess.Queue().AddWithSequence(msg)
}

// =============================================================================
// The player record
// =============================================================================

// SendGrantXP sends opcode 0x0020 — the case at 0x00BD19D8: zigzag level, then
// zigzag exp, into the player record; a higher level plays sound 16.
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

// buildTrainingPoints is opcode 0x0036 — the case at 0x00BD26CC: a u8, then
// the TOTAL training points as a zigzag int into VIPlayer+0x40. When the total
// changes, QuestStage (0x00BD9AA8) shows word 625 "You have received more
// training points!" and refreshes the window; the byte only decides whether
// sound 7 plays.
func buildTrainingPoints(sound bool, total int32) []byte {
	return build(protocol.OpcodeTrainingPoints, 16, func(w *buffer.Writer) {
		var b byte
		if sound {
			b = 1
		}
		w.WriteByte(b)
		w.Write7BitEncodedInt64(int64(total))
	})
}

// SendTrainingPoints sends opcode 0x0036.
func SendTrainingPoints(sess *session.Session, sound bool, total int32) {
	queueReliable(sess, protocol.OpcodeTrainingPoints, buildTrainingPoints(sound, total))
}

// buildTrainingPointsSpent is opcode 0x001D — Msg001D (0x00BDCAF0): one zigzag
// int SUBTRACTED from the training points (VIPlayer+0x40), not a new total.
func buildTrainingPointsSpent(spent int32) []byte {
	return build(protocol.OpcodeUpdateTrainingPoints, 16, func(w *buffer.Writer) {
		w.Write7BitEncodedInt64(int64(spent))
	})
}

// SendTrainingPointsSpent sends opcode 0x001D.
func SendTrainingPointsSpent(sess *session.Session, spent int32) {
	queueReliable(sess, protocol.OpcodeUpdateTrainingPoints, buildTrainingPointsSpent(spent))
}

// buildPlayerP44 is opcode 0x001C: one raw u32 into VIPlayer+0x44, which the
// stat-training window reads together with the class mastery count
// (GetP44Total). It is not an NPC interaction.
func buildPlayerP44(v uint32) []byte {
	return build(protocol.OpcodePlayerP44, 8, func(w *buffer.Writer) { w.WriteUint32(v) })
}

// SendPlayerP44 sends opcode 0x001C.
func SendPlayerP44(sess *session.Session, v uint32) {
	queueReliable(sess, protocol.OpcodePlayerP44, buildPlayerP44(v))
}

// buildExpDebt is opcode 0x00D9: one raw u32 into the player record's +0x2C
// (FrontiersForge calls the field exp debt; unconfirmed). HP is never carried
// here: it is the stat channel's (0x42, m_stat[7]).
func buildExpDebt(v uint32) []byte {
	return build(protocol.OpcodeExpDebt, 8, func(w *buffer.Writer) { w.WriteUint32(v) })
}

// SendExpDebt sends opcode 0x00D9.
func SendExpDebt(sess *session.Session, v uint32) {
	queueReliable(sess, protocol.OpcodeExpDebt, buildExpDebt(v))
}

// SendPlayerTunar sends opcode 0x0052 — the case at 0x00BD263C: a zigzag int
// into the player record's tunar.
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

// SendPlayerSpeed sends opcode 0x00F8 — the case at 0x00BD2BE0: a raw float32,
// then VIClient::SetSpeed.
func SendPlayerSpeed(sess *session.Session, speed float32) {
	msg := protocol.NewMessage(protocol.MessageTypeReliable, protocol.OpcodePlayerSpeed)
	dataBuf := make([]byte, 16)
	writer := buffer.NewWriter(dataBuf)
	writer.WriteUint16(uint16(protocol.OpcodePlayerSpeed))

	writer.WriteUint32(math.Float32bits(speed))

	data := dataBuf[:writer.Position()]
	msg.SetData(data)
	sess.Queue().AddWithSequence(msg)
}

// SendPlayerSpeedPS2 is the CharacterHandler method version of SendPlayerSpeed.
func (h *CharacterHandler) SendPlayerSpeedPS2(sess *session.Session, speed float32) {
	SendPlayerSpeed(sess, speed)
}

// =============================================================================
// Combat
// =============================================================================

// SendDamageIndication sends opcode 0x00DB — the case at 0x00BD2D70: u32
// attacker, zigzag amount, u32 defender. CLIENT draws |amount| over the
// defender when the attacker is the player, and over the player otherwise;
// a positive amount is green (a heal).
func SendDamageIndication(sess *session.Session, sourceObjectID uint32, damage int64, targetObjectID uint32) {
	msg := protocol.NewMessage(protocol.MessageTypeReliable, protocol.OpcodeDamageIndication)
	dataBuf := make([]byte, 32)
	writer := buffer.NewWriter(dataBuf)
	writer.WriteUint16(uint16(protocol.OpcodeDamageIndication))

	writer.WriteUint32(sourceObjectID)
	writer.Write7BitEncodedInt64(damage)
	writer.WriteUint32(targetObjectID)

	data := dataBuf[:writer.Position()]
	msg.SetData(data)
	sess.Queue().AddWithSequence(msg)
}

// SendCastingCombat sends opcode 0x00B1 — the case at 0x00BD1F10:
//
//	u32 caster, u32 target, u8 n, n × u32 further targets,
//	u32 time in ms, u32 spell effect DictID (VISpellEffectSystem::Play)
func SendCastingCombat(sess *session.Session, casterObjectID, targetObjectID uint32, castTimeMS uint32, spellFXDictID uint32) {
	msg := protocol.NewMessage(protocol.MessageTypeReliable, protocol.OpcodeCastingCombat)
	dataBuf := make([]byte, 32)
	writer := buffer.NewWriter(dataBuf)
	writer.WriteUint16(uint16(protocol.OpcodeCastingCombat))

	writer.WriteUint32(casterObjectID)
	writer.WriteUint32(targetObjectID)
	writer.WriteByte(0) // no further targets
	writer.WriteUint32(castTimeMS)
	writer.WriteUint32(spellFXDictID)

	data := dataBuf[:writer.Position()]
	msg.SetData(data)
	sess.Queue().AddWithSequence(msg)
}

// SendCharacterDied sends opcode 0x005E — Msg005E (0x00BDAEB8): no payload.
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

// buildEntityAction is opcode 0x0775 — the case at 0x00BD1930: a raw u32
// entity id and a u8 VICSprite action, played on that entity's character
// sprite. For the player's own entity only action 34 plays.
func buildEntityAction(objectID uint32, action byte) []byte {
	return build(protocol.OpcodeEntityAction, 8, func(w *buffer.Writer) {
		w.WriteUint32(objectID)
		w.WriteByte(action)
	})
}

// SendEntityAction sends opcode 0x0775.
func SendEntityAction(sess *session.Session, objectID uint32, action byte) {
	queueReliable(sess, protocol.OpcodeEntityAction, buildEntityAction(objectID, action))
}

// SendOutMatched sends opcode 0x077A — the case at 0x00BD280C: no payload. The
// console shows word 1196 in red, stops auto-attack and answers 0x077B.
func SendOutMatched(sess *session.Session) {
	msg := protocol.NewMessage(protocol.MessageTypeReliable, protocol.OpcodeOutMatched)
	writer := buffer.NewWriter(msg.Span())
	writer.WriteUint16(uint16(protocol.OpcodeOutMatched))
	msg.SetSize(writer.Position())
	sess.Queue().AddWithSequence(msg)
}

// buildClassicTargetInfo is opcode 0x0010 — Msg0010 (0x00BD9D60): u8
// disposition, two more bytes, two fixed 64-unit wide fields read raw (the
// guild and title lines), and the u32 token of the 0x0010 request it answers.
// CLIENT keeps the three bytes only when that token matches the one it sent
// (SendOpIntInt(16, token, target)): without it the console compares whatever
// its stack held, which is how the 261-byte replies retail sends leave a PS2's
// target colours to chance.
func buildClassicTargetInfo(token uint32, disposition, b, c byte, guild, title string) []byte {
	return build(protocol.OpcodeClassicTargetInfo, 2+3+128+128+4, func(w *buffer.Writer) {
		w.WriteByte(disposition)
		w.WriteByte(b)
		w.WriteByte(c)
		writeFixedWide(w, guild, 64)
		writeFixedWide(w, title, 64)
		w.WriteUint32(token)
	})
}

// SendClassicTargetInfo sends opcode 0x0010.
func SendClassicTargetInfo(sess *session.Session, token uint32, disposition, b, c byte, guild, title string) {
	queueReliable(sess, protocol.OpcodeClassicTargetInfo, buildClassicTargetInfo(token, disposition, b, c, guild, title))
}

// buildAssistTarget is opcode 0x007B — Msg007B: a u32 entity id the client
// targets (and asks 0x0010 about); 0xFFFFFFFF shows word 838 "There is no
// target for assisting." It has nothing to do with quests.
func buildAssistTarget(objectID uint32) []byte {
	return build(protocol.OpcodeAssistTarget, 8, func(w *buffer.Writer) { w.WriteUint32(objectID) })
}

// SendAssistTarget sends opcode 0x007B.
func SendAssistTarget(sess *session.Session, objectID uint32) {
	queueReliable(sess, protocol.OpcodeAssistTarget, buildAssistTarget(objectID))
}

// =============================================================================
// Pets
// =============================================================================

// buildSetPet is opcode 0x007E: one raw u32 into m_petId.
func buildSetPet(petID uint32) []byte {
	return build(protocol.OpcodeSetPet, 8, func(w *buffer.Writer) { w.WriteUint32(petID) })
}

// SendSetPet sends opcode 0x007E.
func SendSetPet(sess *session.Session, petID uint32) {
	queueReliable(sess, protocol.OpcodeSetPet, buildSetPet(petID))
}

// buildClearPet is opcode 0x007F: no payload; m_petId = -1 and the pet's name
// is cleared. It does not touch the quest tracker.
func buildClearPet() []byte {
	return build(protocol.OpcodeClearPet, 2, nil)
}

// SendClearPet sends opcode 0x007F.
func SendClearPet(sess *session.Session) {
	queueReliable(sess, protocol.OpcodeClearPet, buildClearPet())
}

// =============================================================================
// Loot
// =============================================================================

// maxCorpseItems is a VIItemList's capacity. VIItemList::Read does not clamp
// the count it is sent, so more than this would run past the list on a PS2.
const maxCorpseItems = 40

// buildLoot is opcode 0x0018 — the case at 0x00BD1DD0: VIItemList::Read
// (0x00BBC8D0) into the corpse list, a zigzag count, the u32 key the next new
// record gets, and the records, each exactly as the MemoryDump writes an
// inventory row (writeSingleItem; its key is its position). CLIENT opens the
// loot window, or answers 0x0016 at once when it cannot (LootNoWindow).
func buildLoot(items []database.CharInventoryEntry) []byte {
	if len(items) > maxCorpseItems {
		items = items[:maxCorpseItems]
	}
	return build(protocol.OpcodeLoot, 16+len(items)*1024, func(w *buffer.Writer) {
		w.Write7BitEncodedInt64(int64(len(items)))
		w.WriteUint32(uint32(len(items)))
		for i := range items {
			writeSingleItem(w, &items[i], i)
		}
	})
}

// SendLoot sends opcode 0x0018.
func SendLoot(sess *session.Session, items []database.CharInventoryEntry) {
	queueReliable(sess, protocol.OpcodeLoot, buildLoot(items))
}

// buildLootTakeAll is opcode 0x001A — Msg001A (0x00BD98B0): no payload. Every
// corpse item the inventory has room for moves into it; a stack that did not
// all fit stays on the corpse.
func buildLootTakeAll() []byte {
	return build(protocol.OpcodeLootTakeAll, 2, nil)
}

// SendLootTakeAll sends opcode 0x001A.
func SendLootTakeAll(sess *session.Session) {
	queueReliable(sess, protocol.OpcodeLootTakeAll, buildLootTakeAll())
}

// =============================================================================
// Chat and text
// =============================================================================

// coloredChatPayload is opcode 0x0A7B — Msg0A7B (0x00BD3600): a wide string of
// at most 1023 characters, then THREE colour bytes (VIRGB). A fourth byte was
// once written here; CLIENT never reads it.
func coloredChatPayload(text string, r, g, b uint8) []byte {
	return build(protocol.OpcodeColoredChat, 2+4+2046+3, func(w *buffer.Writer) {
		writeWideString(w, text, 2048)
		w.WriteByte(r)
		w.WriteByte(g)
		w.WriteByte(b)
	})
}

// SendColoredChat sends opcode 0x0A7B.
func SendColoredChat(sess *session.Session, text string, r, g, b uint8) {
	queueReliable(sess, protocol.OpcodeColoredChat, coloredChatPayload(text, r, g, b))
}

// clientMessagePayload is opcode 0x0A7A — Msg0A7A (0x00BD36D0): a wide string of
// at most 1023 characters, drawn white.
func clientMessagePayload(text string) []byte {
	return build(protocol.OpcodeClientMessage, 2+4+2046, func(w *buffer.Writer) {
		writeWideString(w, text, 2048)
	})
}

// SendClientMessage sends opcode 0x0A7A.
func SendClientMessage(sess *session.Session, text string) {
	queueReliable(sess, protocol.OpcodeClientMessage, clientMessagePayload(text))
}

// buildErrorMessage is opcode 0x00CE — the case at 0x00BD1628: a wide string of
// at most 1023 characters, drawn red (1, .25, .25).
func buildErrorMessage(text string) []byte {
	return build(protocol.OpcodeErrorMessage, 2+4+2046, func(w *buffer.Writer) {
		writeWideString(w, text, 2048)
	})
}

// SendErrorMessage sends opcode 0x00CE.
func SendErrorMessage(sess *session.Session, text string) {
	queueReliable(sess, protocol.OpcodeErrorMessage, buildErrorMessage(text))
}

// buildReplyName is opcode 0x07A3 — the case at 0x00BD2D28: ReadString(name,
// 24), an 8-bit name of at most 23 characters, which becomes the name /r
// replies to (GetReplyName). It is not a chat line.
func buildReplyName(name string) []byte {
	return build(protocol.OpcodeReplyName, 2+4+23, func(w *buffer.Writer) {
		writeString8(w, name, 24)
	})
}

// SendReplyName sends opcode 0x07A3.
func SendReplyName(sess *session.Session, name string) {
	queueReliable(sess, protocol.OpcodeReplyName, buildReplyName(name))
}

// buildSummonPrompt is opcode 0x00D3 — the case at 0x00BD2C48 and Msg00D3
// (0x00BDB0E8): ReadString(name, 64), an 8-bit name of at most 63 characters,
// then a u8. The console prompts "<name> wants to summon you, do you accept?"
// (word 610) for 0, else "... resurrect you ..." (611). It is not NPC speech.
func buildSummonPrompt(name string, resurrect bool) []byte {
	return build(protocol.OpcodeSummonPrompt, 2+4+63+1, func(w *buffer.Writer) {
		writeString8(w, name, 64)
		var c byte
		if resurrect {
			c = 1
		}
		w.WriteByte(c)
	})
}

// SendSummonPrompt sends opcode 0x00D3.
func SendSummonPrompt(sess *session.Session, name string, resurrect bool) {
	queueReliable(sess, protocol.OpcodeSummonPrompt, buildSummonPrompt(name, resurrect))
}

// =============================================================================
// Server status and weather
// =============================================================================

// SendServerReady sends opcode 0x07E0 — Msg07E0: CLIENT answers with 0x07E1,
// its DRDP counters as text, and echoes whatever followed the opcode (nothing
// here, as in every captured one).
func SendServerReady(sess *session.Session) {
	msg := protocol.NewMessage(protocol.MessageTypeReliable, protocol.OpcodeServerReady)
	writer := buffer.NewWriter(msg.Span())
	writer.WriteUint16(uint16(protocol.OpcodeServerReady))
	msg.SetSize(writer.Position())
	sess.Queue().AddWithSequence(msg)
}

// buildZoneTransferConfirm is opcode 0x07B2 — the case at 0x00BD2DF8: one raw
// u32 into VIClient+0x7578, which the constructor sets to 100. What CLIENT does
// with it is not known yet; the old writer's single byte left it unread.
func buildZoneTransferConfirm(value uint32) []byte {
	return build(protocol.OpcodeZoneTransferConfirm, 8, func(w *buffer.Writer) { w.WriteUint32(value) })
}

// SendZoneTransferConfirm sends opcode 0x07B2.
func SendZoneTransferConfirm(sess *session.Session, value uint32) {
	queueReliable(sess, protocol.OpcodeZoneTransferConfirm, buildZoneTransferConfirm(value))
}

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
// Dialogs and the quest journal
// =============================================================================

// Room CLIENT has for the option dialog (Msg0034 0x00BDBD58).
const (
	maxDialogOptions = 6
)

// buildDialogueBox is opcode 0x0046 — the case at 0x00BD1A4C: a u32, then a
// wide string of at most 511 characters, shown with ShowDialog (queued when a
// dialog is already open).
func buildDialogueBox(counter uint32, text string) []byte {
	return build(protocol.OpcodeDialogueBox, 2+4+4+1022, func(w *buffer.Writer) {
		w.WriteUint32(counter)
		writeWideString(w, text, 1024)
	})
}

// SendDialogueBox sends opcode 0x0046.
func SendDialogueBox(sess *session.Session, counter uint32, text string) {
	queueReliable(sess, protocol.OpcodeDialogueBox, buildDialogueBox(counter, text))
}

// buildQuestPopupMulti is opcode 0x0034 — Msg0034 (0x00BDBD58): a u32 id the
// answer (0x0035) echoes, the question as a wide string of at most 127
// characters, a u8 option count and each option as a wide string of at most 63.
// CLIENT has room for six options; more would run past them.
func buildQuestPopupMulti(counter uint32, greeting string, options []string) []byte {
	if len(options) > maxDialogOptions {
		options = options[:maxDialogOptions]
	}
	return build(protocol.OpcodeQuestPopupMulti, 2+4+4+254+1+len(options)*(4+126), func(w *buffer.Writer) {
		w.WriteUint32(counter)
		writeWideString(w, greeting, 256)
		w.WriteByte(byte(len(options)))
		for _, o := range options {
			writeWideString(w, o, 128)
		}
	})
}

// SendQuestPopupMulti sends opcode 0x0034.
func SendQuestPopupMulti(sess *session.Session, counter uint32, greeting string, options []string) {
	queueReliable(sess, protocol.OpcodeQuestPopupMulti, buildQuestPopupMulti(counter, greeting, options))
}

// buildQuestAdd is opcode 0x007C — the case at 0x00BD2B60: the u32 journal
// token (VIClient+0x2BB0C, which the MemoryDump also sets) and a quest title as
// a wide string of at most 127 characters, added while there are fewer than
// eight.
func buildQuestAdd(journalToken uint32, title string) []byte {
	return build(protocol.OpcodeAddQuestLog, 2+4+4+254, func(w *buffer.Writer) {
		w.WriteUint32(journalToken)
		writeWideString(w, title, 256)
	})
}

// SendQuestAdd sends opcode 0x007C.
func SendQuestAdd(sess *session.Session, journalToken uint32, title string) {
	queueReliable(sess, protocol.OpcodeAddQuestLog, buildQuestAdd(journalToken, title))
}

// buildQuestRemove is opcode 0x007D — the case at 0x00BD2BA4 and Msg007D
// (0x00BDBB98): the u32 journal token, then the zigzag INDEX of the title to
// remove (the rest move up). An index of 8 or more is ignored.
func buildQuestRemove(journalToken uint32, index int32) []byte {
	return build(protocol.OpcodeDeleteQuest, 16, func(w *buffer.Writer) {
		w.WriteUint32(journalToken)
		w.Write7BitEncodedInt64(int64(index))
	})
}

// SendQuestRemove sends opcode 0x007D.
func SendQuestRemove(sess *session.Session, journalToken uint32, index int32) {
	queueReliable(sess, protocol.OpcodeDeleteQuest, buildQuestRemove(journalToken, index))
}

// =============================================================================
// Group
// =============================================================================

// buildGroupInvite is opcode 0x0773 — Msg0773 (0x00BD3458): the u32 invite id
// (which the accept and the decline, 0x0624, send back), a u32, and the
// inviter's name as ReadString(name, 24): 8-bit, at most 23 characters. A
// console already in a group declines it at once.
func buildGroupInvite(inviteID, x uint32, inviterName string) []byte {
	return build(protocol.OpcodeGroupInviteReceived, 2+4+4+4+23, func(w *buffer.Writer) {
		w.WriteUint32(inviteID)
		w.WriteUint32(x)
		writeString8(w, inviterName, 24)
	})
}

// SendGroupInvite sends opcode 0x0773.
func SendGroupInvite(sess *session.Session, inviteID, x uint32, inviterName string) {
	queueReliable(sess, protocol.OpcodeGroupInviteReceived, buildGroupInvite(inviteID, x, inviterName))
}

// buildGroupChat is opcode 0x069E — the case at 0x00BD15B8: a u32 CLIENT reads
// and never uses, then a wide string of at most 1023 characters, drawn in the
// group colour (.5, .5, 1).
func buildGroupChat(senderObjectID uint32, text string) []byte {
	return build(protocol.OpcodeGroupChat, 2+4+4+2046, func(w *buffer.Writer) {
		w.WriteUint32(senderObjectID)
		writeWideString(w, text, 2048)
	})
}

// SendGroupChat sends opcode 0x069E.
func SendGroupChat(sess *session.Session, senderObjectID uint32, text string) {
	queueReliable(sess, protocol.OpcodeGroupChat, buildGroupChat(senderObjectID, text))
}
