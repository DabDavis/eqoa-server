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

// Dyes is a colour per dye slot (database.DyeChest ... DyeRobe, and the
// cape's, DyeCape: capes.go), 0xRRGGBBAA, 0 none.
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

// RobeLook is a robe's disc tint and its own colour off its record's colour
// (itempattern.color; the user, 2026-11-05: "sky blue for spirit maker",
// "spectral robe is black"): 0-14 is a disc tint, drawn in its own colour;
// white (0xFFFFFFFF) the plain robe; anything else a colour (0xRRGGBBAA),
// drawn so by a client of the live game's look, and on the disc's in the
// tint nearest it (nearestTint). Until 2026-11-06 no robe's colour reached
// anyone: the channel's byte 110 carried the wearer's sex.
func RobeLook(color int64) (tint byte, rgba uint32) {
	c := uint32(color)
	switch {
	case c < uint32(len(database.DiscTints)):
		return byte(c), 0
	case c == 0xFFFFFFFF:
		return 0, 0
	}
	return nearestTint(c), c
}

// greyTints are the disc's neutral tints: white, dark grey and silver.
var greyTints = map[int]bool{0: true, 1: true, 10: true}

// nearestTint is the disc tint nearest a colour: of the greys (greyTints)
// for a grey, so black is drawn dark grey and not navy, else of them all, by
// the distance between the two in RGB.
func nearestTint(c uint32) byte {
	r, g, b := int(c>>24), int(c>>16&0xFF), int(c>>8&0xFF)
	grey := max(r, g, b)-min(r, g, b) <= 24
	best, bestD := 0, -1
	for i, t := range database.DiscTints {
		if grey && !greyTints[i] {
			continue
		}
		dr, dg, db := r-int(t[0]), g-int(t[1]), b-int(t[2])
		if d := dr*dr + dg*dg + db*db; bestD < 0 || d < bestD {
			best, bestD = i, d
		}
	}
	return byte(best)
}

// LookDyes is what the live game's look draws the player in: its dyes
// (chardyes), and its worn robe's, armour's and held items' own colours where
// it has no dye for the slot (RobeColor, ArmorColors: handlers' wornLook), and
// its held items' own colours over any dye of their hand (HeldColors; the user, 2026-11-06: "shirt of blackened links should be pitch
// black"; 2026-11-13: a sword "with a evil aura"). A held item's goes out as
// the channel's bytes 99-110 (LiveChannel), which until then carried only a
// weapon's dye; a PS2 is sent the disc's 162 bytes, which have no place for it.
func (p *Player) LookDyes() *Dyes {
	d := p.Dyes
	changed := false
	if p.RobeColor != 0 && d[database.DyeRobe] == 0 {
		d[database.DyeRobe], changed = p.RobeColor, true
	}
	for i, c := range p.ArmorColors {
		if c != 0 && d[i] == 0 {
			d[i], changed = c, true
		}
	}
	// a held item's own colour wins over a dye (the owner, 2026-11-13: "make the
	// item color win over dyes"): a dyed hand takes its dye only while what it
	// holds has no colour of its own
	for i, c := range p.HeldColors {
		if c != 0 && d[database.DyePrimary+i] != c {
			d[database.DyePrimary+i], changed = c, true
		}
	}
	if !changed {
		return &p.Dyes
	}
	return &d
}

// ArmorColor is an armour piece's own colour for the live game's look off its
// record's colour (itempattern.color): a colour (0xRRGGBBAA; black is
// 0x000000FF) is drawn so; white, a disc tint's index (0-14, as RobeLook
// reads one: the crafted sets carry their metal's there, which no client
// draws on armour) or none is 0, nothing of its own.
func ArmorColor(color int64) uint32 {
	c := uint32(color)
	if c < uint32(len(database.DiscTints)) || c == 0xFFFFFFFF {
		return 0
	}
	return c
}

// ArmorTint is the disc tint a PS2 draws an armour piece of that colour
// (ArmorColor's: 0, none) in: the nearest of the fifteen (nearestTint), 0 white
// for none.
func ArmorTint(c uint32) byte {
	if c == 0 {
		return 0
	}
	return nearestTint(c)
}

func putRGBA(b []byte, c uint32) {
	binary.BigEndian.PutUint32(b, c)
}

// dyeOr is a slot's colour as the live look sends it: its dye, a White dye
// as database.WhiteDyeWire (drawn bleached, where white is the texture as it
// is), else the undyed colour.
func dyeOr(d *Dyes, slot int, undyed uint32) uint32 {
	if d != nil && d[slot] != 0 {
		if d[slot] == database.WhiteDye {
			return database.WhiteDyeWire
		}
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

// This server's armour (the owner, 2026-11-13: "lets also add armor plate type 9 the
// sandstorm patch, but we dont need the patch the armor is on the ISO", and "for the
// plate armor, there is gold trim, but currently is muted due to the colors, lets add
// trim color dyeing also"; claude-notes/armor-set9-trim-20261113.md):
//
//   - set 9, the ornate gold-and-steel plate CHAR.ESF holds under the plain plate's
//     five DictIDs (claude-notes/charesf-shadowed-armor.md), which Sandstorm's S2
//     registers under five of its own and draws as set 9 (wearing set 4's helm). An
//     item record's model 9 on an armour slot is set 9 (handlers' wornLook), and so is
//     the channel's byte. CLIENT clamps a set past 8 to none (SetArmorSlot: the bare
//     body), so every client that does not draw this server's armour is sent set 4,
//     the plain plate, in its place (PlainArmor): a PS2, and a client of the live look
//     that did not ask for it;
//   - the trim, the gold of the plate sets 4 and 9: a client of the live look that
//     draws this server's armour is sent, after the live game's 201-byte channel and
//     71-byte record, a colour per armour slot (chest, bracer, gloves, legs, boots,
//     helm; 0xRRGGBBAA, red first) its trim is drawn in, 0 none: its trim as the art
//     has it, under the slot's colour as before (chardyes' trim slots).
//
// The client asks with bit 2 of its answer to the live look's offer
// (handlers/livelook.go); the look in force says it is granted.

// ChannelArmor is the channel's armour sets, chest .. helm (bytes 93-98).
const ChannelArmor = 93

// OrnateSet is set 9; PlainPlateSet is set 4, which it is sent as.
const (
	OrnateSet     = 9
	PlainPlateSet = 4
)

// ArmorTrimSize is the trims after the live channel and record: six colours.
const ArmorTrimSize = 6 * 4

// ArmorStateSize and ArmorGearSize are the channel and the record's raw part
// to a client that draws this server's armour.
const (
	ArmorStateSize = LiveEntityStateSize + ArmorTrimSize
	ArmorGearSize  = LiveGearSize + ArmorTrimSize
)

// PlainArmor makes set 9 set 4 in the armour sets a, as a client that does
// not draw set 9 is sent them.
func PlainArmor(a []byte) {
	for i, v := range a {
		if v == OrnateSet {
			a[i] = PlainPlateSet
		}
	}
}

func hasOrnate(a []byte) bool {
	for _, v := range a {
		if v == OrnateSet {
			return true
		}
	}
	return false
}

// putTrims appends the six trim colours (0 none).
func putTrims(out []byte, d *Dyes) []byte {
	for i := 0; i < database.DyeTrims; i++ {
		out = binary.BigEndian.AppendUint32(out, dyeOr(d, database.DyeChestTrim+i, 0))
	}
	return out
}

// ArmorChannel is LiveChannel with the trim colours after it: a client that
// draws this server's armour's.
func ArmorChannel(disc []byte, d *Dyes) []byte {
	return putTrims(LiveChannel(disc, d), d)
}

// ArmorGear is LiveGear with the trim colours after it.
func ArmorGear(disc []byte, d *Dyes) []byte {
	return putTrims(LiveGear(disc, d), d)
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
// live look, with the trim colours after them (ArmorChannel) when it also
// draws this server's armour, else the buffer itself. A client that does not
// draw this server's armour is sent set 9 as set 4 (PlainArmor).
func ForViewer(sess *session.Session, disc []byte, d *Dyes) []byte {
	if sess != nil && sess.LiveLook() && len(disc) == EntityStateSize {
		if sess.ArmorLook() {
			return ArmorChannel(disc, d)
		}
		out := LiveChannel(disc, d)
		PlainArmor(out[ChannelArmor : ChannelArmor+6])
		return out
	}
	if len(disc) == EntityStateSize && disc[5] == 0x82 && hasOrnate(disc[ChannelArmor:ChannelArmor+6]) {
		out := append([]byte(nil), disc...)
		PlainArmor(out[ChannelArmor : ChannelArmor+6])
		return out
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
