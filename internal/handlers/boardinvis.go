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

// BoardInvisSpellID is Superior Invisibility's record (spellpattern, spelleffect).
const BoardInvisSpellID = 700685

// boardBoxMark marks a board's question's id (beside the quests' 0x51 and a genie's 0x58).
const boardBoxMark = 0x59000000

// The board's question, in the box's order.
const (
	boardInvisQuestion = "The board also offers Superior Invisibility, for 10 minutes. Will you take it?"
	boardInvisYes      = "Yes, make me invisible"
	boardInvisNo       = "No, thank you"
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

// askBoardInvis asks the player, unless the boards offer nothing.
func (h *InteractHandler) askBoardInvis(sess *session.Session, p *entity.Player) {
	h.boardMu.Lock()
	if h.boardSpell == nil {
		h.boardMu.Unlock()
		return
	}
	h.boardSeq++
	id := boardBoxMark | (h.boardSeq & 0xFFFFFF)
	h.boardAsked[p.CharServerID] = id
	h.boardMu.Unlock()
	SendQuestPopupMulti(sess, id, boardInvisQuestion, []string{boardInvisYes, boardInvisNo})
}

// answerBoardInvis is HandleDialogueOption's question for a board's box: its first answer lands the
// spell, once. It reports whether the box was a board's (an old one does nothing).
func (h *InteractHandler) answerBoardInvis(sess *session.Session, p *entity.Player, box uint32, option byte) bool {
	if box&0xFF000000 != boardBoxMark {
		return false
	}
	h.boardMu.Lock()
	asked, ok := h.boardAsked[p.CharServerID]
	if ok && asked == box {
		delete(h.boardAsked, p.CharServerID)
	}
	spell := h.boardSpell
	h.boardMu.Unlock()
	if !ok || asked != box || option != 0 || spell == nil || h.gameWorld == nil {
		return true
	}
	if s, found := spell(); found {
		h.gameWorld.LandOnSelf(sess, s, time.Now())
	}
	return true
}
