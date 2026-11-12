package entity

import (
	"encoding/binary"
	"math"
	"regexp"
)

// boardplate.go — a bulletin board's name plate (the owner, 2026-11-12: "for
// the bulletin board lets add a nameplate above").
//
// CLIENT puts a name plate on a CSprite with a name node alone:
// VIClientPart06D00::Apply calls VICSprite::SetName only when the actor's
// sprite is of type 5, and SetName attaches the plate (a VINameSprite) at the
// CSprite's name node, doing nothing for one without (m_node[Name] == -1).
// No board has one: the papered bulletin board (PaperedBoardBody: Highpass's
// board, the Mercenary Recruitment boards', the Real Estate Listings', the
// PvP Events') is a SimpleSprite, and the towns' announcement boards stand on
// the sign post's CSprite (0x6A82352C) and the Guild Board and the Duel
// Tournament Board on 0x6DE5B2CC, both without a name node (read off CHAR.ESF
// 2026-11-12: node slots all -1 but the post's 4). No byte of the channel
// gives any of them one, so no flag on the board can.
//
// So each board in a player's view brings a second entity with it, its plate:
// a human's body (bodyHumanMale, whose plate stands humanPlate over its feet)
// under the board's name, with its skins hidden (byte 154 bit 0:
// VICreatureEntity::ApplyLook's SetSkinsVisible(0), which leaves held items, a
// helm and the name, and it holds none), not selectable (byte 26 bit 0 clear:
// CLIENT's target cycle and pick skip it, vanilla's too), standing on the
// board's spot at the size that puts its name plateAbove over the board's top.
// It is no NPC of the world: it is never in World.NPCs or the spatial index,
// so no system (aggro, combat, quests, the GM panel) ever sees it; the
// visibility system adds it after its board (boardPlatesLocked) and serializes
// it from the board as it stands now (serializePlate), so a board moved,
// renamed, resized or out of view takes its plate with it.
//
// A board is an NPC on the papered board, whatever its name, or one on
// another body without a name node (boardTops) named as a board
// (boardNamed): the sign post's other NPCs are signs, and 0x6DE5B2CC's are
// graves, a bookshelf, dig sites, which get none.

// PaperedBoardBody is the papered bulletin board's sprite (0x260EB42F,
// 638497839: a zone actor of 23 Tunaria zones, cmd/artbuild's ss-art.csf).
const PaperedBoardBody uint32 = 0x260EB42F

// boardTops is each board body's top over its feet at size 1, by its box: the
// papered board's 3.0 (board1111's measure), the sign post's 3.766 and
// 0x6DE5B2CC's 1.75 (their CSprite headers).
var boardTops = map[uint32]float32{PaperedBoardBody: 3.0, SignpostBody: 3.766, 0x6DE5B2CC: 1.75}

// boardNamed is a board by its name: "... Announcements" (and Surefall's
// "Anouncements"), "... Board", "... Listings".
var boardNamed = regexp.MustCompile(`(?i)(\bann?ouncements?|\bboard|\blistings?)\s*$`)

// humanPlate is where the human's plate stands over its feet at size 1: its
// name node at rest (1.499) and the plate 0.6 over it (vanilla's, scaled with
// the body), read off CHAR.ESF's 0x70D898C6 through vanilla's CSprite.
const humanPlate float32 = 2.096

// plateAbove is how far over a board's top its name is.
const plateAbove float32 = 0.5

// plateIDBase puts a plate's entity id far past every NPC row and allocated
// id (an entity id, never sent: the client knows it by its object id): a
// board's plate is plateIDBase + the board's id.
const plateIDBase EntityID = 0x6F000000

// hasPlate reports whether n is a board shown with a plate: named, alive, a
// board by its body as it is drawn now (and its name, off the papered board).
func (n *NPC) hasPlate() bool {
	if n == nil || n.Name == "" || n.Dead {
		return false
	}
	if n.isBoard() {
		return true
	}
	return n.Title != "" // a title over its name (npctitle.go)
}

// isBoard reports n a board (hasPlate's): its plate shows its own name.
func (n *NPC) isBoard() bool {
	model, _ := n.Body()
	if _, ok := boardTops[model]; !ok {
		return false
	}
	return model == PaperedBoardBody || boardNamed.MatchString(n.Name)
}

// plateSize is the size of board's plate body: its name plateAbove over the
// board's top, at the board's size.
func plateSize(board *NPC) float32 {
	model, size := board.Body()
	if size <= 0 {
		size = 1
	}
	return (boardTops[model]*size + plateAbove) / humanPlate
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
// body at plateSize, with nothing worn or held, its
// skins hidden and not selectable.
func serializePlate(objectID uint32, board *NPC) []byte {
	if !board.isBoard() {
		return serializeTitlePlate(objectID, board) // npctitle.go
	}
	p := &NPC{
		ServerID: plateID(board),
		Name:     board.Name,
		X:        board.X, Y: board.Y, Z: board.Z,
		Facing:  board.Facing,
		World:   board.World,
		ModelID: bodyHumanMale,
		Size:    plateSize(board),
		NPCType: board.NPCType,
		Level:   board.Level,
		Race:    "0",
	}
	return plateBytes(objectID, p)
}

// plateLoco is a plate's locomotion (byte 58), which holds its body still
// (the owner, 2026-11-12: "the bulletin board name plates move with the
// invisible idle of the placeholder"). CLIENT's name plate hangs on the
// body's name node (VICSprite::SetName's Attach), so it rides the body's
// animation: the idle (0) breathes, and every 20-40 s SetAnimation swaps in
// the fidget (15), so the plate swayed and bobbed. Slot 63 is no slot of the
// human's or of any NPC body sampled: VICSprite::SetDefaultEntries fills it
// with the idle at sync 1, played once, and UpdateCreatures' SetLocomotion of
// the same state every frame never restarts it (SetAnimation: the same id
// with a play is done), so the body holds the idle's last frame from then on.
// Measured through vanilla's port (bit for bit the decomp's), the human's
// name node stands at 2.096 (humanPlate, as at rest) and moves by nothing
// once its one play ends (1.85 s); on the idle the human's swings 0.30 x
// 0.22 across and 0.044 up and down. 55 bodies (the 40 most used NPC bodies
// and the 20 player bodies) all stand still on 63 within 3 s.
const plateLoco byte = 63

// plateBytes is p's channel as a plate: not selectable, never a fight's, its
// skins hidden, nothing held, holding still (plateLoco).
func plateBytes(objectID uint32, p *NPC) []byte {
	buf := SerializeNPCUpdate(objectID, p)
	buf[26] = 0 // not selectable (CLIENT's VIEntity +0x18 bit 0)
	buf[58] = plateLoco
	buf[143] = 0 // never a fight's
	buf[154] = 1 // skins hidden: the name alone shows
	binary.LittleEndian.PutUint32(buf[59:63], 0)
	return buf
}
