package handlers

import (
	"log"
	"time"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/session"
)

// boardinvis.go — a town's announcement board offers Superior Invisibility (petition
// 1140, the GM's Jibmage: "For all announcement boards that currently give boon and sow,
// when players talk to them also give it a dialogue option so they can optionally get a 10
// minute superior invisibility cast on them"). After the boon and the Spirit of the Wolf,
// the board asks; a yes lands the spell's own record on the player, as if it had cast it on
// itself, at no power and with no lockout. Any other answer, or the box closed, leaves it.
// The words are ours. A hostile act ends the invisibility as it ends any (invis.go).
//
// The same box offers the way back to where the player last died, while a rez's chance
// still holds that place (petition 1218, the GM Jibrish: "Add to all signposts /
// announcement boards in town an additional option players can select to get a 0% XP
// resurrect that sends them back to the point they died at"): entity's TakeDeathPlace. A
// town's other signs ask it alone, of one who has a place to go back to; a road's do not.

// BoardInvisSpellID is Superior Invisibility's record (spellpattern, spelleffect).
const BoardInvisSpellID = 700685

// boardBoxMark marks a board's question's id (beside the quests' 0x51 and a genie's 0x58).
const boardBoxMark = 0x59000000

// The board's question, in the box's order.
const (
	boardInvisQuestion = "The board also offers Superior Invisibility, for 10 minutes. Will you take it?"
	boardInvisYes      = "Yes, make me invisible"
	boardInvisNo       = "No, thank you"
	// with the way back beside it, and the way back alone (a town's sign's)
	boardBothQuestion = "The board also offers Superior Invisibility, for 10 minutes, or the way back to where you last died, with none of the lost experience."
	boardBackOption   = "Take me back to where I died"
	signBackQuestion  = "This can send you back to where you last died, with none of the lost experience. Will you go?"
	signBackYes       = "Yes, take me back"
)

// BoardInvisSpell is the spell's record as the World lands it.
func BoardInvisSpell(store *database.SpellbookStore) (entity.CastSpell, bool) {
	sp, err := store.SpellPattern(BoardInvisSpellID)
	if err != nil || sp == nil {
		log.Printf("Boards: Superior Invisibility (%d): %v", BoardInvisSpellID, err)
		return entity.CastSpell{}, false
	}
	row, ok, err := store.SpellEffectRow(BoardInvisSpellID)
	if err != nil || !ok {
		log.Printf("Boards: Superior Invisibility's effects (%d): %v", BoardInvisSpellID, err)
		return entity.CastSpell{}, false
	}
	return entity.SpellOfRow(sp, 0, row.Line, row.Effects, row.FX), true
}

// SetBoardInvis gives the handler the board's offer: what reads its spell. Without one the boards
// offer nothing.
func (h *InteractHandler) SetBoardInvis(spell func() (entity.CastSpell, bool)) {
	h.boardMu.Lock()
	defer h.boardMu.Unlock()
	h.boardSpell = spell
	h.boardAsked = map[uint32]uint32{}
}

// askBoardInvis asks the player, board for a board and otherwise a town's sign: Superior
// Invisibility on a board that offers it, the way back when the player has a place to go back
// to, or nothing.
func (h *InteractHandler) askBoardInvis(sess *session.Session, p *entity.Player, board bool) {
	back := h.gameWorld != nil && h.gameWorld.DeathPlaceLive(sess, time.Now())
	h.boardMu.Lock()
	invis := board && h.boardSpell != nil
	if !invis && !back {
		h.boardMu.Unlock()
		return
	}
	question, options, backAt := boardInvisQuestion, []string{boardInvisYes, boardInvisNo}, -1
	switch {
	case invis && back:
		question, options, backAt = boardBothQuestion, []string{boardInvisYes, boardBackOption, boardInvisNo}, 1
	case back:
		question, options, backAt = signBackQuestion, []string{signBackYes, boardInvisNo}, 0
	}
	h.boardSeq++
	id := boardBoxMark | (h.boardSeq & 0xFFFFFF)
	if h.boardAsked == nil {
		h.boardAsked = map[uint32]uint32{}
	}
	if h.boardBack == nil {
		h.boardBack = map[uint32]int{}
	}
	h.boardAsked[p.CharServerID], h.boardBack[p.CharServerID] = id, backAt
	h.boardMu.Unlock()
	SendQuestPopupMulti(sess, id, question, options)
}

// answerBoardInvis is HandleDialogueOption's question for a board's box: its way back sends the
// player there, its first answer otherwise lands the spell, each once. It reports whether the box
// was a board's (an old one does nothing).
func (h *InteractHandler) answerBoardInvis(sess *session.Session, p *entity.Player, box uint32, option byte) bool {
	if box&0xFF000000 != boardBoxMark {
		return false
	}
	h.boardMu.Lock()
	asked, ok := h.boardAsked[p.CharServerID]
	backAt := h.boardBack[p.CharServerID]
	if ok && asked == box {
		delete(h.boardAsked, p.CharServerID)
		delete(h.boardBack, p.CharServerID)
	}
	spell := h.boardSpell
	h.boardMu.Unlock()
	if !ok || asked != box || h.gameWorld == nil {
		return true
	}
	if backAt >= 0 && int(option) == backAt {
		if world, x, y, z, found := h.gameWorld.TakeDeathPlace(sess, time.Now()); found {
			h.travel(sess, p, entity.CoachDest{Name: "where it died", World: world, X: x, Y: y, Z: z, Facing: p.Facing})
		}
		return true
	}
	if option != 0 || spell == nil || backAt == 0 {
		return true
	}
	if s, found := spell(); found {
		h.gameWorld.LandOnSelf(sess, s, time.Now())
	}
	return true
}
