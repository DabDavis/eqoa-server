package entity

import (
	"encoding/binary"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/session"
)

// SOE's live game's look (protocol.OpcodeLiveLook, the user's "new state
// channel for eqoaclassic, but also legacy support so sandstorm runs"). The
// live CLIENT (eqoa-live-1500, resident in the Corsten and Lostologist save
// states) reads the entity state channel and the 0x002C character record with
// a colour (VIColor32: r, g, b, a, one byte each) for every armour slot, held
// item and the robe where the Frontiers disc's read a tint index:
//
//   - the channel, VICreatureEntity::ReadUpdate 0x013D2870 (WriteUpdate
//     0x013D2178 writes the same; Corsten's addresses), 201 bytes against the
//     disc's 162: bytes 0-98 as the disc's; 99-110 the three held items'
//     colours (entity +0x296; read and kept, drawn by nothing in that build);
//     111-140 per armour slot (chest, bracer, gloves, legs, boots, helm) its
//     colour (+0x2A2+4i) then a u8 (+0x2BE+i) that picks the slot's ArmorGroup
//     variant (VICSprite::GetArmorGroupResourceID, 0-3; the disc's art has no
//     ArmorGroups, so it picks nothing there); 141-144 the robe's colour
//     (+0x2BA); 145-149 hair, face and robe, the disc's 105-109; the disc's
//     tints (99-104) and robe tint (110) are gone; 150-200 the disc's 111-161.
//   - the record, CharacterList 0x012CC018: the zigzags as the disc's, then
//     the robe, the held items, their attack types, the armour sets, six
//     variants (the disc's tints), the held items' colours, the armour's and
//     the robe's: 71 raw bytes against the disc's 32 (no robe tint).
//
// ApplyLook hands each colour to VICSprite::SetArmorSlot / SetRobe / SetHelm,
// which with no ArmorGroup (the Frontiers disc's art) set it as the slot's
// material layer colour, exactly where the disc's set GetTintColor(tint): the
// live game sent an old tint as its colour (Lostologist's legs faaf32ff, tint
// 6). So a slot with no dye is sent the colour of the tint the disc's look
// carries (TintRGBA) and is drawn as a PS2 draws it.

// LiveEntityStateSize is the live game's entity channel, 201 bytes.
const LiveEntityStateSize = 201

// DiscGearSize and LiveGearSize are the raw part of a 0x002C record.
const (
	DiscGearSize = 32
	LiveGearSize = 71
)

// Dyes is a colour per dye slot (database.DyeChest ... DyeRobe),
// 0xRRGGBBAA, 0 none.
type Dyes [database.DyeSlots]uint32

// undyedItem is a held item's colour with no dye: the live client makes
// them white, and SOE's server sent white for nearly all.
const undyedItem = 0xFFFFFFFF

// TintRGBA is the colour a disc tint index draws an armour slot in, as
// SetArmorSlot takes it: past 14 it is 0 (its clamp).
func TintRGBA(t byte) uint32 {
	if int(t) >= len(database.DiscTints) {
		t = 0
	}
	return database.RGBA(database.DiscTints[t])
}

// RobeTintRGBA is the colour a disc tint index draws a robe in: SetRobe
// reads GetTintColor with no clamp of its own; 14 at most here, as the
// vanilla client takes it.
func RobeTintRGBA(t byte) uint32 {
	if int(t) >= len(database.DiscTints) {
		t = byte(len(database.DiscTints) - 1)
	}
	return database.RGBA(database.DiscTints[t])
}

func putRGBA(b []byte, c uint32) {
	binary.BigEndian.PutUint32(b, c)
}

func dyeOr(d *Dyes, slot int, undyed uint32) uint32 {
	if d != nil && d[slot] != 0 {
		return d[slot]
	}
	return undyed
}

// LiveChannel is an entity's state as the disc's look carries it (the
// serializers' 162 bytes) in the live game's 201: the dyes where there are
// any, else each armour slot and the robe in its tint's colour and each
// held item white; every variant 0.
func LiveChannel(disc []byte, d *Dyes) []byte {
	out := make([]byte, LiveEntityStateSize)
	if len(disc) < EntityStateSize {
		disc = append(append([]byte(nil), disc...), make([]byte, EntityStateSize-len(disc))...)
	}
	copy(out[:99], disc[:99])
	for i := 0; i < 3; i++ {
		putRGBA(out[99+4*i:], dyeOr(d, database.DyePrimary+i, undyedItem))
	}
	for i := 0; i < 6; i++ {
		putRGBA(out[111+5*i:], dyeOr(d, database.DyeChest+i, TintRGBA(disc[99+i])))
		out[115+5*i] = 0
	}
	putRGBA(out[141:], dyeOr(d, database.DyeRobe, RobeTintRGBA(disc[110])))
	copy(out[145:150], disc[105:110])
	copy(out[150:], disc[111:EntityStateSize])
	return out
}

// LiveGear is the raw part of a 0x002C record (characterGear's 32 bytes:
// robe, its tint, the held items, their attack types, the armour sets and
// tints) as the live game's record carries it, coloured as LiveChannel.
func LiveGear(disc []byte, d *Dyes) []byte {
	if len(disc) < DiscGearSize {
		disc = append(append([]byte(nil), disc...), make([]byte, DiscGearSize-len(disc))...)
	}
	out := make([]byte, LiveGearSize)
	copy(out[0:4], disc[0:4])     // the robe
	copy(out[4:16], disc[5:17])   // the held items
	copy(out[16:19], disc[17:20]) // their attack types
	copy(out[19:25], disc[20:26]) // the armour sets; 25-30, the variants, 0
	for i := 0; i < 3; i++ {
		putRGBA(out[31+4*i:], dyeOr(d, database.DyePrimary+i, undyedItem))
	}
	for i := 0; i < 6; i++ {
		putRGBA(out[43+4*i:], dyeOr(d, database.DyeChest+i, TintRGBA(disc[26+i])))
	}
	putRGBA(out[67:], dyeOr(d, database.DyeRobe, RobeTintRGBA(disc[4])))
	return out
}

// ForViewer is an entity's state (the disc's 162 bytes) as sess's client
// reads it: the live game's 201 with its dyes when that client asked for the
// live look, else the buffer itself.
func ForViewer(sess *session.Session, disc []byte, d *Dyes) []byte {
	if sess != nil && sess.LiveLook() && len(disc) == EntityStateSize {
		return LiveChannel(disc, d)
	}
	return disc
}

// SetEntityState puts an entity's state on its channel at the state's own
// length: a client's look may change while a channel holds the other's, and
// SetData only copies into the buffer it has (it cannot grow or shrink one).
// A channel of the disc's length that is given the disc's look (every PS2's,
// all the time) takes SetData, as it always has.
func SetEntityState(state *session.StateUpdateData, data []byte) {
	if state == nil {
		return
	}
	if state.Len() == len(data) {
		state.SetData(data)
		return
	}
	state.SetSizedData(data)
}

// DyeTarget is the character #dye works on: sess's target when that is a
// player, else sess's own; its character id, name and dyes.
func (w *World) DyeTarget(sess *session.Session) (serverID uint32, name string, d Dyes, ok bool) {
	w.mu.RLock()
	defer w.mu.RUnlock()
	gm := w.sessionPlayerLocked(sess)
	if gm == nil {
		return 0, "", Dyes{}, false
	}
	p := gm
	if gm.TargetObjectID != 0 && gm.TargetObjectID != NoAssist {
		if e, found := w.findEntityByObjectIDLocked(sess, gm.TargetObjectID); found {
			if t, isPlayer := e.(*Player); isPlayer {
				p = t
			}
		}
	}
	return p.CharServerID, p.Name, p.Dyes, true
}

// SetDyes puts a character's dyes on it where it stands (the next tick sends
// them to every client of the live look); false when it is not in the world.
func (w *World) SetDyes(serverID uint32, d Dyes) (string, bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	p := w.playerByCharacterLocked(serverID)
	if p == nil {
		return "", false
	}
	p.Dyes = d
	return p.Name, true
}
