package handlers

import (
	"fmt"
	"strings"

	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/session"
)

// moogie.go — Mr. Moogie's talk (petition 1246, Dwarfrish: "Mr. Moogie should have several options
// for player sto choose from / "Clear my buffs" - the NPC will strip ALL buffs on the player and say
// in chat that X targets buffs are clear / "Fully heal me" - heals the players Hp and mana to full
// ... The NPC should not be interactable when pvp flagged, or in a duel"; the user: build it). A
// talk asks in a box under moogieBoxMark; the answer does it (entity/moogie.go), refused again if
// the player has since flagged or begun a duel. The words are ours.

// moogieBoxMark marks his question's id (beside the quests' 0x51 and the others to 0x5A).
const moogieBoxMark = 0x5B000000

// His question and its answers, in the box's order, and what he says.
const (
	// the GM Jibrish's petition 1258: "flavor text mentioning how he is the king of this jungle,
	// and you need to prove your worth to him"
	moogieQuestion = "Mr. Moogie: 'I am king of this jungle, and you have yet to prove your worth to me. Still, I can clear your buffs or heal you.'" // a box's question: 127 at most (questAskChars)
	moogieClearOpt = "Clear my buffs"
	moogieHealOpt  = "Fully heal me"
	moogieNoOpt    = "Nothing, thank you"
	moogieCleared  = "Mr. Moogie says, '%s's buffs are clear.'"
	moogieHealed   = "Mr. Moogie says, 'There you are, %s, whole again.'"
)

// moogieAsks is the boxes he has asked, by the character's serverid: the box and which Mr. Moogie.
type moogieAsks struct {
	seq   uint32
	asked map[uint32]moogieAsk
}

type moogieAsk struct {
	box uint32
	npc entity.EntityID
}

// moogieTalk is a talk to npc: his question when npc is Mr. Moogie, else false.
func (h *InteractHandler) moogieTalk(sess *session.Session, p *entity.Player, npc *entity.NPC) bool {
	if npc == nil || !strings.EqualFold(npc.Name, entity.MoogieName) || h.gameWorld == nil {
		return false
	}
	if why := h.gameWorld.MoogieBusy(sess, npc.ServerID); why != "" {
		SendColoredChat(sess, why, 255, 63, 63)
		return true
	}
	h.boardMu.Lock()
	h.moogie.seq++
	id := moogieBoxMark | (h.moogie.seq & 0xFFFFFF)
	if h.moogie.asked == nil {
		h.moogie.asked = map[uint32]moogieAsk{}
	}
	h.moogie.asked[p.CharServerID] = moogieAsk{box: id, npc: npc.ServerID}
	h.boardMu.Unlock()
	SendQuestPopupMulti(sess, id, moogieQuestion, []string{moogieClearOpt, moogieHealOpt, moogieNoOpt})
	return true
}

// answerMoogie is HandleDialogueOption's question for his box: the first answer clears the buffs,
// the second heals, each once. It reports whether the box was his (an old one does nothing).
func (h *InteractHandler) answerMoogie(sess *session.Session, p *entity.Player, box uint32, option byte) bool {
	if box&0xFF000000 != moogieBoxMark {
		return false
	}
	h.boardMu.Lock()
	asked, ok := h.moogie.asked[p.CharServerID]
	if ok && asked.box == box {
		delete(h.moogie.asked, p.CharServerID)
	}
	h.boardMu.Unlock()
	if !ok || asked.box != box || h.gameWorld == nil || option > 1 {
		return true
	}
	if why := h.gameWorld.MoogieBusy(sess, asked.npc); why != "" {
		SendColoredChat(sess, why, 255, 63, 63)
		return true
	}
	if option == 0 {
		if name, _, done := h.gameWorld.MoogieClear(sess); done {
			SendClientMessage(sess, fmt.Sprintf(moogieCleared, name))
		}
		return true
	}
	if name, done := h.gameWorld.MoogieHeal(sess); done {
		SendClientMessage(sess, fmt.Sprintf(moogieHealed, name))
	}
	return true
}
