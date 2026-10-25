package entity

import (
	"encoding/binary"
	"math"
	"strconv"
	"unicode/utf16"
)

// EntityStateSize is the state buffer size for object channels 0x00-0x1C.
//
// 162, not the old 201: every object channel in pcaps/ps2_sandstorm.pcap
// decodes to exactly 162 bytes — all 24 occupied channels, every update in the
// capture. Our own layout ends there too, with "tsrq" at 158-161.
//
// The size IS the fix. Zeroing bytes 162-200 (see the note at the end of
// SerializePlayerUpdate) was not enough on its own: RunLengthEncode does not
// stop at the last non-zero byte, it encodes the trailing zero run as a
// 0x80 0x27 pair, so the client kept receiving 201 bytes and writing 39 zeros
// past the end of its entity struct on every tick.
//
// This must stay equal to session.entityStateSize, which allocates the channel
// buffer — SetData only copies into that buffer and cannot shrink it, so a
// shorter serializer alone changes nothing on the wire.
const EntityStateSize = 162

// StatUpdateSize is the state buffer size for channel 0x42. Retail's decodes to
// exactly 233 bytes on every capture, ending with the 0xFF at offset 232.
const StatUpdateSize = 233

// The active effects' channel (0x43) is as long as its count: a u32 count,
// then a 132-byte record per effect, 4 bytes when there is none. Every value
// in the captures is 4 + 132n (136 up to 796 bytes, and 4 again when the last
// effect ends), and CLIENT's Channel43 (0x00BD3D18) reads exactly that: the
// count (at most 8), then each record's u32 icon and its name, jumping to
// 4 + 132(i+1) after each. Unlike the channels above it is not a fixed buffer:
// session.StateUpdateData.SetSizedData sends it at its own length. It was
// once 136 bytes of two 66-byte slots, which CLIENT read as one record with a
// second's icon and name in the middle of it.
const (
	EffectRecordSize = 132
	// MaxEffects is how many the client keeps (m_effectIcon[8]); a larger
	// count is read as 8.
	MaxEffects = 8
	// MaxEffectName is the longest name the client shows: ReadString(name,
	// 64) on a wide string takes n + 1 <= 32 characters, and shows a longer
	// name empty.
	MaxEffectName = 31
)

// velocityToByte is a velocity in units a second as the entity state's
// signed byte: CLIENT's ChannelEntity (0x00BD41B8) reads bytes 40-42 as
// (signed char) × 21/128 units a second — the Go client's world lane checked
// the scale on two Sandstorm captures (creatures travel 1.019 and 1.011 of
// velocity × time between updates). The old ×4 of a per-update position
// difference moved a running player at about a quarter of its speed.
func velocityToByte(v float32) byte {
	scaled := math.Round(float64(v) * 128 / 21)
	if scaled > 127 {
		scaled = 127
	} else if scaled < -128 {
		scaled = -128
	}
	return byte(int8(scaled))
}

// SerializePlayerUpdate builds a 162-byte state channel 0 buffer from a Player struct.
// Byte-for-byte identical output to the ECS SerializeObjectUpdate.
func SerializePlayerUpdate(objectID uint32, p *Player) []byte {
	buf := make([]byte, EntityStateSize)

	// Byte 0: isActive flag
	buf[0] = 1

	// Bytes 1-4: ObjectID (LE)
	binary.LittleEndian.PutUint32(buf[1:5], objectID)

	// Position: 3-byte big-endian mediums at offsets 6, 9, 12
	posX := int32(p.X * 128.0)
	posY := int32(p.Y * 128.0)
	posZ := int32(p.Z * 128.0)

	buf[6] = byte(posX >> 16)
	buf[7] = byte(posX >> 8)
	buf[8] = byte(posX)
	buf[9] = byte(posY >> 16)
	buf[10] = byte(posY >> 8)
	buf[11] = byte(posY)
	buf[12] = byte(posZ >> 16)
	buf[13] = byte(posZ >> 8)
	buf[14] = byte(posZ)

	// Byte 5: Entity type 0x82
	buf[5] = 0x82

	// Offset 15: Facing (0=south, 128=north). A client's heading comes in
	// over [-pi, pi) (ClientUpdateFacing): turned into [0, 2pi) first, as
	// converting a negative float to a byte is not defined.
	f := math.Mod(float64(p.Facing), 2*math.Pi)
	if f < 0 {
		f += 2 * math.Pi
	}
	buf[15] = byte(f / (2 * math.Pi) * 256)

	// Offset 17: World (ObjectUpdate offset 16 + the isActive prefix byte).
	//
	// MEASURED, not assumed. In pcaps/sandstorm-worldchange-20260807.pcap the
	// player coaches from Odus (world 2) to Freeport (world 0); the channel 0
	// update that follows the load (packet 162, ref=1) carries an XOR delta of
	// 0x02 at exactly this byte, alongside deltas on position (6-14) and facing
	// (15) and nothing else. 2 XOR 0 = 2: retail moves this field WITH the
	// player. Hardcoding 0 was correct while world 0 was the only world and
	// silently wrong afterwards.
	buf[17] = p.World

	// Offset 26: HPFlag (1 = show HP bar)
	buf[26] = 1

	// Offset 27: HPBar (0-255 proportional)
	if top := p.MaxHPNow(); top > 0 {
		buf[27] = byte(min(p.HP, top) * 255 / top)
	} else {
		buf[27] = 255
	}

	// Offset 28-31: ModelID (LE)
	modelID := uint32(p.ModelID)
	binary.LittleEndian.PutUint32(buf[28:32], modelID)

	// Offset 32-35: ModelID2 (LE, 0 in eqoa.live — only set for upgraded models)
	// Leave as 0x00000000

	// Offset 36-39: Scale (float32 LE)
	binary.LittleEndian.PutUint32(buf[36:40], math.Float32bits(p.Size))

	// Offset 40-42: Velocity X/Y/Z, units a second × 128/21 (velocityToByte)
	buf[40] = velocityToByte(p.VelX)
	buf[41] = velocityToByte(p.VelY)
	buf[42] = velocityToByte(p.VelZ)

	// Offset 50-57: Constant across ALL entities in PS2 memory (float32: 54.245, -2.528)
	buf[50] = 0xFF
	buf[51] = 0xFA
	buf[52] = 0x58
	buf[53] = 0x42
	buf[54] = 0xC2
	buf[55] = 0xCA
	buf[56] = 0x21
	buf[57] = 0xC0

	// Offset 58: Animation from client update (0=idle, 1=walk, etc.)
	buf[58] = p.Animation

	// Offset 59-62: Target ObjectID (LE)
	binary.LittleEndian.PutUint32(buf[59:63], p.TargetObjectID)

	// eqoa.live: bytes 63-64 are 0x00, 0x00 (not 0x01, 0x05)

	// Offset 67-70: Primary weapon (LE)
	binary.LittleEndian.PutUint32(buf[67:71], p.Primary)
	// Offset 71-74: Secondary weapon (LE)
	binary.LittleEndian.PutUint32(buf[71:75], p.Secondary)
	// Offset 75-78: Shield (LE)
	binary.LittleEndian.PutUint32(buf[75:79], p.Shield)

	// Offset 79-90: the three override items (m_itemOver), none.
	// Offset 91-92: the two hands' attack types (m_itemType), as every player
	// in the Sandstorm captures has them: ApplyItems attaches items 0 and 1
	// with them, and the swing is picked by them (a two-hander's 2, 4 or 6
	// off item 0; 0 has no attack animation).
	buf[91] = p.PrimaryType
	buf[92] = p.SecondaryType

	// Offset 93-98: Armor slots
	buf[93] = p.Chest
	buf[94] = p.Bracer
	buf[95] = p.Gloves
	buf[96] = p.Legs
	buf[97] = p.Boots
	buf[98] = p.Helm

	// === Java eqoa.live offsets from byte 99 onwards ===

	// Offset 105-108: Hair/Face (Java Entity.java)
	buf[105] = byte(p.HairColor)
	buf[106] = byte(p.HairLength)
	buf[107] = byte(p.HairStyle)
	buf[108] = byte(p.FaceOption)

	// Offset 109: Robe style (0xFF = no robe, low byte of Robe uint32). Style
	// 0 is a robe: until 2026-10-25 a robe whose model is 0 was sent as none.
	if p.RobeWorn {
		buf[109] = byte(p.Robe)
	} else {
		buf[109] = 0xFF
	}

	// Offset 110: Sex (0=male, 1=female) — for face texture lookup
	buf[110] = byte(p.Sex)

	// Offset 111: eqoa.live constant = 6 across ALL entities
	buf[111] = 6

	// Offset 116-139: Name (24 bytes null-padded, Java Entity.java offset 116)
	nameBytes := []byte(p.Name)
	for i := 0; i < len(nameBytes) && i < 24; i++ {
		buf[116+i] = nameBytes[i]
	}

	// Offset 140: Level (Java Entity.java)
	buf[140] = byte(p.Level)

	// Offset 141: Movement (0 = normal per Java)
	buf[141] = 0

	// Offset 142: NameColor — eqoa.live: 2 for players, 0 for NPCs
	buf[142] = 2

	// Offset 143: NamePlate — eqoa.live: 1 for players (targetable)
	buf[143] = 1

	// Offset 144: Race (Java-shifted)
	buf[144] = byte(p.Race)
	// Offset 145: Class
	buf[145] = byte(p.Class)

	// Offset 145-146: NPCType (uint16 LE). Byte 145 is the low half, which
	// carries the class, and 146 is the HIGH half, which must be ZERO for a
	// player — 0x8000 is the NPC bit. Retail agrees on every player in
	// pcaps/ps2_sandstorm.pcap: Mcgregor 145=05 146=00 (0x0005), Luck
	// 145=0d 146=00, Pinehills 145=09 146=00. Not one has the high bit.
	//
	// Writing 0x80 here set the NPC bit on every player, the mirror of the
	// off-by-one in SerializeNPCUpdate that cleared it on every NPC — so the
	// client had players and NPCs swapped in both directions and ran the wrong
	// appearance path for each.
	buf[146] = 0x00
	buf[147] = 0x00

	// Offset 148-151: Pattern (0xFFFFFFFF) — eqoa.live constant
	buf[148] = 0xFF
	buf[149] = 0xFF
	buf[150] = 0xFF
	buf[151] = 0xFF

	// Offset 153: eqoa.live constant = 0x18 (24) across ALL entities
	buf[153] = 0x18

	// Offset 156: eqoa.live constant = 1. This comment used to read "154-157:
	// zeros in eqoa.live" and that is wrong — measured against
	// pcaps/ps2_sandstorm.pcap, byte 156 is 0x01 on 56 of 56 NPCs and 13 of 14
	// players, while 152/154/155/157 really are zero on every one of them. It
	// sits next to 153=0x18, which we already send, so it is the same class of
	// constant and was simply missed.
	buf[156] = 0x01

	// Offset 152, 154-155, 157: zero on every entity in the capture, but
	// 154 is 1 on an invisible player: Sandstorm set it on the caster's own
	// channel when Camouflage landed (2026-10-21, cast.go).
	if p.Invisible() {
		buf[154] = 1
	}

	// Offset 158-161: End signature "tsrq" (eqoa.live byte order)
	buf[158] = 't'
	buf[159] = 's'
	buf[160] = 'r'
	buf[161] = 'q'

	// NOTHING IS WRITTEN PAST 161 — see stateTailMustStayZero in
	// statesync_tail_test.go. A duplicate "C# tail" block used to be written
	// here (Level/Movement/NameColor/NamePlate/Race/Class/NPCType/Pattern/
	// Online/"trsq" at 179-200) for backward compatibility with the C#
	// reference server's layout. No real server sends any of it: across four
	// channels of pcaps/ps2_sandstorm.pcap — a level 51 character in full gear
	// and three NPCs — every byte above 161 is zero. Those offsets are live
	// fields in the client's own entity struct, and filling them corrupted the
	// entity: the PS2 could not move, its buff bar filled up, and it announced
	// spells it had not learned.

	return buf
}

// SerializeNPCUpdate builds a 162-byte state channel buffer for an NPC.
// Byte-for-byte identical output to the ECS SerializeNPCObjectUpdate.
func SerializeNPCUpdate(objectID uint32, n *NPC) []byte {
	buf := make([]byte, EntityStateSize)

	// Byte 0: isActive flag
	buf[0] = 1

	// Bytes 1-4: ObjectID (LE)
	binary.LittleEndian.PutUint32(buf[1:5], objectID)

	// Position: 3-byte big-endian mediums at offsets 6, 9, 12
	posX := int32(n.X * 128.0)
	posY := int32(n.Y * 128.0)
	posZ := int32(n.Z * 128.0)

	buf[6] = byte(posX >> 16)
	buf[7] = byte(posX >> 8)
	buf[8] = byte(posX)
	buf[9] = byte(posY >> 16)
	buf[10] = byte(posY >> 8)
	buf[11] = byte(posY)
	buf[12] = byte(posZ >> 16)
	buf[13] = byte(posZ >> 8)
	buf[14] = byte(posZ)

	buf[5] = 0x82

	// Facing, as the player's (byte 15): turned into [0, 2pi) first, as
	// converting a negative float to a byte is not defined
	f := math.Mod(float64(n.Facing), 2*math.Pi)
	if f < 0 {
		f += 2 * math.Pi
	}
	buf[15] = byte(f / (2 * math.Pi) * 256)

	buf[17] = n.World // World — see SerializePlayerUpdate for the measurement

	// HP
	buf[26] = 1
	if n.MaxHP > 0 {
		buf[27] = byte(n.HP * 255 / n.MaxHP)
	} else {
		buf[27] = 255
	}

	// Model
	modelID := uint32(n.ModelID)
	binary.LittleEndian.PutUint32(buf[28:32], modelID)
	// Offset 32-35: ModelID2 (0 in eqoa.live — only set for upgraded models)
	// Leave as 0x00000000
	binary.LittleEndian.PutUint32(buf[36:40], math.Float32bits(n.Size))

	// Offset 40-42: the velocity (npcmove.go), units a second × 128/21, which
	// CLIENT carries a creature along between updates (ExtrapolateMove); 0
	// standing.
	buf[40] = velocityToByte(n.VelX)
	buf[41] = velocityToByte(n.VelY)
	buf[42] = velocityToByte(n.VelZ)

	// Offset 50-57: eqoa.live constant for NPCs (float32: 54.245, -2.528)
	// Player entities have different values here (position-related)
	buf[50] = 0xFF
	buf[51] = 0xFA
	buf[52] = 0x58
	buf[53] = 0x42
	buf[54] = 0xC2
	buf[55] = 0xCA
	buf[56] = 0x21
	buf[57] = 0xC0

	// Offset 58: the animation CLIENT's sprite plays (UpdateCreatures'
	// SetLocomotion, a VICSpriteAnimID): 0 standing, 1 walking (npcmove.go);
	// a corpse's 14 is serializeEntityForChannel's
	buf[58] = n.Loco

	// Target = 0 (no target) — eqoa.live: 0 for NPCs, player OID for pets
	binary.LittleEndian.PutUint32(buf[59:63], 0x00000000)

	// eqoa.live: bytes 63-64 are 0x00, 0x00 (not 0x01, 0x05)

	// NPC Equipment, and the two hands' attack types (91, 92; npcHandTypes):
	// with none an armed NPC's swing has no attack animation on the PS2
	binary.LittleEndian.PutUint32(buf[67:71], uint32(n.PrimaryWeapon))
	binary.LittleEndian.PutUint32(buf[71:75], uint32(n.SecondaryWeapon))
	binary.LittleEndian.PutUint32(buf[75:79], uint32(n.Shield))
	buf[91] = n.PrimaryType
	buf[92] = n.SecondaryType

	buf[93] = byte(n.Torso)
	buf[94] = byte(n.Forearm)
	buf[95] = byte(n.NPCGloves)
	buf[96] = byte(n.NPCLegs)
	buf[97] = byte(n.Feet)
	buf[98] = byte(n.Head)

	// Offset 99-104: Gear Colors (tint index per armor slot).
	//
	// THESE MUST BE A VALID TINT INDEX, NOT 0xFF. This was the naked-NPC bug:
	// NPCs rendered with no clothes and invisible hands and feet, while the
	// player — whose serializer never writes 99-104, so it sends 0 — looked
	// fine. Measured, not guessed: matched race-0/face-2 human NPCs on OUR
	// server, same wire bytes, differing only by whether S2 was applied:
	//
	//   unpatched client (clothed)  VICSprite textSlot tints = [0,0,0,0,0]
	//   S2 applied      (naked)     VICSprite textSlot tints = [255,255,255,255,255]
	//
	// $Tints (0x004E0C08) holds 15 entries and GetTintColor (0x004294E8) is
	// `return &Tints[index*4]` with NO bounds check — the clamp to 0-14 lives
	// in SetArmorSlot (0x00422530), and Sandstorm's S2 patches
	// SetArmorSlot+0x44. Under retail our 0xFF was clamped to 0 and nothing
	// showed; under S2 it survives and GetTintColor reads 1020 bytes past a
	// 60-byte table, yielding garbage RGBA (alpha included) — geometry drawn
	// with a junk colour, i.e. invisible.
	//
	// 0 is $Tints[0] = white/no-tint, which is what the working client ends up
	// holding anyway, so this is retail-equivalent AND safe under S2.
	buf[99] = 0
	buf[100] = 0
	buf[101] = 0
	buf[102] = 0
	buf[103] = 0
	buf[104] = 0

	// Hair/Face at Java offsets 105-108
	buf[105] = byte(n.HairColor)
	buf[106] = byte(n.HairLength)
	buf[107] = byte(n.HairStyle)
	buf[108] = byte(n.Face)

	// === Java eqoa.live offsets from byte 99 onwards ===

	// Offset 109: Robe style (0xFF = no robe, 0-3 = robe styles)
	robeVal := byte(0xFF)
	if v, err := strconv.Atoi(n.RobeStyle); err == nil {
		robeVal = byte(v)
	}
	buf[109] = robeVal

	// Offset 110: Sex (0=male default for NPCs)
	buf[110] = 0

	// Offset 111: eqoa.live constant = 6 across ALL entities
	buf[111] = 6

	// Offset 116-139: Name (24 bytes null-padded)
	nameBytes := []byte(n.Name)
	for i := 0; i < len(nameBytes) && i < 24; i++ {
		buf[116+i] = nameBytes[i]
	}

	// Offset 140: Level
	buf[140] = byte(n.Level)

	// Offset 141: Movement (0 = normal per Java)
	buf[141] = 0

	// Offset 142: NameColor — eqoa.live: 0 for ALL NPCs, 2 for players only;
	// 3 for a pet (pet.go: CLIENT's m_p256, player-side, as Sandstorm's
	// summoned Fire Elemental carried it)
	buf[142] = 0
	if n.Owner != 0 {
		buf[142] = 3
	}

	// Offset 143: NamePlate/Targetable — eqoa.live: 1 for hostile/combat NPCs, 0 for merchants/quest
	if n.NPCType == 0x0000 {
		buf[143] = 1 // Hostile/combat NPCs are targetable
	} else {
		buf[143] = 0 // Merchants, quest NPCs, special NPCs
	}

	// Offset 144: Race
	if v, err := strconv.Atoi(n.Race); err == nil {
		buf[144] = byte(v)
	}

	// Offset 146-147: NPCType (uint16 LE)
	// NPCType is at 145-146, NOT 146-148. Verified against retail in
	// pcaps/ps2_sandstorm.pcap: "Merchant Shadehand" carries 145=00 146=81
	// (0x8100 LE) with 147=00, and every retail NPC has the 0x8000 bit set.
	//
	// Writing it one byte late cleared that bit — the client read 145-146 as
	// 0x0000, which marks the entity as a PLAYER, so every NPC was pushed
	// through VICSpriteCust and asked for a skin from $Race[race*8+face*2+sex],
	// racial clothes and hand/foot textures that an NPC model does not have.
	// The symptom was missing skin, missing starting clothes and invisible
	// hands and feet on almost everything, with no error anywhere.
	//
	// THE OFFSET FIX WAS NOT ENOUGH — the same symptom came back through the
	// VALUE. npcs.npc_type is stored BYTE-REVERSED relative to the wire, the
	// same convention modelid uses (see root CLAUDE.md, "Model IDs by Race").
	// Every internal test reads the low byte (world.go: NPCType&0x80 for
	// unattackable and for faction), so the column is self-consistent — it is
	// only the wire write that has to swap back. Measured against
	// packets.db's live capture, the mapping is exact:
	//     ours 0x0080 x263 -> retail 0x8000     ours 0x0180 x69 -> 0x8001
	//     ours 0x0081 x239 -> retail 0x8100
	// Writing it unswapped put the NPC bit in the low byte, so the client read
	// the high bit clear and took the player path again: 2426 of 2429 NPCs.
	//
	// The "every retail NPC has the 0x8000 bit set" claim above is NOT true and
	// nothing should be forced on top of the swap. Decoding all 49 NPCs in
	// pcaps/ps2_sandstorm.pcap: 16 of them carry 0x0000 — City Cat, Bat,
	// Guard Balruss (armor 04 04 04 04 04 04), Guard Soalen (06 06 06 06 06 07)
	// — and they render clothed. Sending 0x0000 through unchanged is retail's
	// own behaviour; an earlier version of this forced zeros to 0x8000 and that
	// was inventing traffic no server sends.
	npcType := uint16(n.NPCType)
	binary.LittleEndian.PutUint16(buf[145:147], npcType<<8|npcType>>8)

	// Offset 148-151: Pattern (0xFFFFFFFF) — eqoa.live constant
	buf[148] = 0xFF
	buf[149] = 0xFF
	buf[150] = 0xFF
	buf[151] = 0xFF

	// Offset 153: eqoa.live constant = 0x18 (24) across ALL entities
	buf[153] = 0x18

	// Offset 156: eqoa.live constant = 1. This comment used to read "154-157:
	// zeros in eqoa.live" and that is wrong — measured against
	// pcaps/ps2_sandstorm.pcap, byte 156 is 0x01 on 56 of 56 NPCs and 13 of 14
	// players, while 152/154/155/157 really are zero on every one of them. It
	// sits next to 153=0x18, which we already send, so it is the same class of
	// constant and was simply missed.
	buf[156] = 0x01

	// Offset 152, 154-155, 157: zero on every entity in the capture.

	// Offset 158-161: End signature "tsrq" (eqoa.live byte order)
	buf[158] = 't'
	buf[159] = 's'
	buf[160] = 'r'
	buf[161] = 'q'

	// NOTHING IS WRITTEN PAST 161 — same rule as SerializePlayerUpdate, and
	// the capture shows it holds for NPCs too: retail's three NPC channels are
	// all zero above 161.

	return buf
}

// SerializeStatUpdate builds a 233-byte stat channel buffer from a Player struct.
// Byte-for-byte identical output to the ECS SerializeStatUpdate.
func SerializeStatUpdate(p *Player) []byte {
	// 233 bytes: the buffer ends with the single 0xFF at offset 232, exactly as
	// every capture does. It was 236 (0xEC), writing three bytes past the end
	// of the client's stat buffer.
	buf := make([]byte, StatUpdateSize)

	writeInt32 := func(offset int, val int32) {
		binary.LittleEndian.PutUint32(buf[offset:offset+4], uint32(val))
	}

	// Current stats (offsets 0-27), with what spells add (cast.go)
	writeInt32(0, int32(p.StatNow(StatStr)))
	writeInt32(4, int32(p.StatNow(StatSta)))
	writeInt32(8, int32(p.StatNow(StatAgi)))
	writeInt32(12, int32(p.StatNow(StatDex)))
	writeInt32(16, int32(p.StatNow(StatWis)))
	writeInt32(20, int32(p.StatNow(StatInt)))
	writeInt32(24, int32(p.StatNow(StatCha)))

	// HP/Power (offsets 28-43); the maximums with spells' (Rockskin's +30
	// max HP came here on Sandstorm, the base at 148 kept)
	writeInt32(28, int32(p.HP))
	writeInt32(32, int32(p.MaxHPNow()))
	writeInt32(36, int32(p.Power))
	writeInt32(40, int32(p.MaxPowerNow()))

	// Combat modifiers (offsets 44-56) — SOE layout
	writeInt32(44, 100)                      // field_100 (always 100 on SOE retail)
	writeInt32(48, int32(p.OffMod))          // OFF_MOD (total)
	writeInt32(52, int32(p.DefMod))          // DEF_MOD (total)
	writeInt32(56, int32(p.StatNow(StatAC))) // Rockskin's AC 150 came here on Sandstorm

	// Resists (offsets 88-111), with spells'
	writeInt32(88, int32(p.StatNow(StatPR)))
	writeInt32(92, int32(p.StatNow(StatDR)))
	writeInt32(96, int32(p.StatNow(StatFR)))
	writeInt32(100, int32(p.StatNow(StatCR)))
	writeInt32(104, int32(p.StatNow(StatLR)))
	writeInt32(108, int32(p.StatNow(StatAR)))

	// Base stats (offsets 116-143) — stats before equipment
	writeInt32(116, int32(p.BaseStr))
	writeInt32(120, int32(p.BaseSta))
	writeInt32(124, int32(p.BaseAgi))
	writeInt32(128, int32(p.BaseDex))
	writeInt32(132, int32(p.BaseWis))
	writeInt32(136, int32(p.BaseInt))
	writeInt32(140, int32(p.BaseCha))

	// Base maxima (offsets 148, 156), as Sandstorm sends them (gear.go): the
	// hit points of all the stamina without anything's own, the power of the
	// base stats alone. 144 and 152 stay 0, not the current HP and power.
	writeInt32(144, 0)
	writeInt32(148, int32(p.BaseMaxHP()))
	writeInt32(152, 0)
	writeInt32(156, int32(p.BaseMaxPower()))

	// Base AC (offset 172): 0, as on Sandstorm; the AC is all the worn items'
	// and the spells' (56). The client adds the worn items' to the base half
	// itself (GetStatTotal), so a base AC would count them twice.
	writeInt32(172, 0)

	// Offset 232: breath, 255 full (VIPlayer's m_p30, the breath gauge's
	// fill: DrawStatus draws it over 255). Not an end marker: every capture
	// sent 0xFF only because nobody in them was under water; in
	// pcaps/breath_capture.pcap it steps 0xFF, 0xDF, ... 0x1F, 0x00, 32 every
	// 6 s, and then the HP goes (F3 20261021-011015: the gauge stood full).
	buf[232] = byte(max(0, min(255, p.Breath)))

	return buf
}

// SerializeBuffUpdate is the player's active effects as state channel 0x43
// carries them: the u32 count, then each effect's 132-byte record, the u32
// icon (a DictID: VIWndInfluence blits it whole), the name's length in
// characters (u32) and the name in UTF-16LE, zeros to the record's end. At
// most MaxEffects, and each name cut to MaxEffectName characters.
//
// The count is the number of records that follow, nothing else. It was once
// given TickCount/20, which climbs and wraps: the client was told of up to 255
// entries, and the channel changed every 2 s forever where a real server sends
// only when an effect starts or ends.
func SerializeBuffUpdate(p *Player) []byte {
	n := min(len(p.Buffs), MaxEffects)
	buf := make([]byte, 4+n*EffectRecordSize)
	binary.LittleEndian.PutUint32(buf, uint32(n))
	for i, e := range p.Buffs[:n] {
		r := buf[4+i*EffectRecordSize : 4+(i+1)*EffectRecordSize]
		binary.LittleEndian.PutUint32(r, e.Icon)
		name := utf16.Encode([]rune(e.Name))
		if len(name) > MaxEffectName {
			name = name[:MaxEffectName]
			// not half a surrogate pair
			if c := name[len(name)-1]; c >= 0xD800 && c < 0xDC00 {
				name = name[:len(name)-1]
			}
		}
		binary.LittleEndian.PutUint32(r[4:], uint32(len(name)))
		for j, c := range name {
			binary.LittleEndian.PutUint16(r[8+2*j:], c)
		}
	}
	return buf
}
