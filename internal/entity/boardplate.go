package entity

import (
	"encoding/binary"
	"math"
)

// boardplate.go — a bulletin board's name plate (the owner, 2026-11-12: "for
// the bulletin board lets add a nameplate above").
//
// The papered bulletin board (PaperedBoardBody: Highpass's board, which the
// Mercenary Recruitment boards, the Real Estate Listings and the PvP Events
// board stand on, sent to every client in ss-art.csf) is a SimpleSprite, and
// CLIENT puts a name plate on a CSprite alone: VIClientPart06D00::Apply calls
// VICSprite::SetName only when the actor's sprite is of type 5, and the plate
// is a VINameSprite attached at the CSprite's name node. No byte of the
// channel gives any other sprite one, so no flag on the board can.
//
// So each such board in a player's view brings a second entity with it, its
// plate: a human's body (bodyHumanMale) under the board's name, with its
// skins hidden (byte 154 bit 0: VICreatureEntity::ApplyLook's
// SetSkinsVisible(0), which leaves held items, a helm and the name, and it
// holds none), not selectable (byte 26 bit 0 clear: CLIENT's target cycle
// and pick skip it, vanilla's too), standing on the board's spot at the size
// that puts its name node over the board's top (plateScale). It is no NPC of
// the world: it is never in World.NPCs or the spatial index, so no system
// (aggro, combat, quests, the GM panel) ever sees it; the visibility system
// adds it after its board (boardPlatesLocked) and serializes it from the
// board as it stands now (serializePlate), so a board moved, renamed or out of
// view takes its plate with it. Boards on a CSprite body (the announcement
// boards' sign post, the Guild Board) have their own plates and get none.

// PaperedBoardBody is the papered bulletin board's sprite (0x260EB42F,
// 638497839: a zone actor of 23 Tunaria zones, cmd/artbuild's ss-art.csf).
const PaperedBoardBody uint32 = 0x260EB42F

// plateBodies are the board bodies that get a plate: bodies that are no
// CSprite, which every client shows without a name.
var plateBodies = map[uint32]bool{PaperedBoardBody: true}

// plateIDBase puts a plate's entity id far past every NPC row and allocated
// id (an entity id, never sent: the client knows it by its object id): a
// board's plate is plateIDBase + the board's id.
const plateIDBase EntityID = 0x6F000000

// plateScale is the plate body's size for a board of size 1: the human's
// name node (1.733 over its feet; the plate is drawn 0.6 over it, scaled
// with it) at 1.5 puts the name at 3.5, half a unit over the board's top
// (its box is 3.0 high at size 1).
const plateScale float32 = 1.5

// hasPlate reports whether n is a board shown with a plate: named, alive, on
// a plate body as it is drawn now.
func (n *NPC) hasPlate() bool {
	if n == nil || n.Name == "" || n.Dead {
		return false
	}
	model, _ := n.Body()
	return plateBodies[model]
}

// plateID is the id of n's plate.
func plateID(n *NPC) EntityID { return plateIDBase + n.ServerID }

// plateBoardLocked is the board whose plate id is id, nil when id is no
// plate of a board that has one. Must be called with the lock held.
func (w *World) plateBoardLocked(id EntityID) *NPC {
	if id < plateIDBase {
		return nil
	}
	n, ok := w.NPCs[id-plateIDBase]
	if !ok || !n.hasPlate() {
		return nil
	}
	return n
}

// boardPlatesLocked adds to a view's candidates each board's plate, just
// after its board (the same distance), so the two come and go together
// unless the slots end between them. Must be called with the lock held.
func (w *World) boardPlatesLocked(cands []entityDist) []entityDist {
	for _, c := range cands {
		if n, ok := w.NPCs[c.ID]; ok && n.hasPlate() {
			cands = append(cands, entityDist{ID: plateID(n), DistSq: math.Nextafter32(c.DistSq, math.MaxFloat32)})
		}
	}
	return cands
}

// serializePlate is the channel of board's plate under objectID: the
// board's own (its place, facing, world, name, NPC bits), on the human's
// body at plateScale times the board's size, with nothing worn or held, its
// skins hidden and not selectable.
func serializePlate(objectID uint32, board *NPC) []byte {
	_, size := board.Body()
	if size <= 0 {
		size = 1
	}
	p := &NPC{
		ServerID: plateID(board),
		Name:     board.Name,
		X:        board.X, Y: board.Y, Z: board.Z,
		Facing:  board.Facing,
		World:   board.World,
		ModelID: bodyHumanMale,
		Size:    size * plateScale,
		NPCType: board.NPCType,
		Level:   board.Level,
		Race:    "0",
	}
	buf := SerializeNPCUpdate(objectID, p)
	buf[26] = 0  // not selectable (CLIENT's VIEntity +0x18 bit 0)
	buf[143] = 0 // never a fight's
	buf[154] = 1 // skins hidden: the name alone shows
	binary.LittleEndian.PutUint32(buf[59:63], 0)
	return buf
}
