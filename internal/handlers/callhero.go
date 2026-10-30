package handlers

import (
	"fmt"
	"log"
	"time"

	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/session"
)

// Call of the Hero's question and its answer (entity/callhero.go; the owner's
// ruling of 2026-10-29: it summoned a player "if the player accepted the call
// of the hero"). The World's hero-call hook asks each member a call reaches
// with a Yes/No box, 0x0034 as a spiritmaster's bind question is asked
// (spiritmaster.go), its id the World's under entity.HeroCallBoxMark, which
// the answer (0x0035) echoes, so HandleDialogueOption hands it here before
// any NPC's question. The answer goes back to the World: a Yes in time lands
// the member beside its caster through the travel hook (gates.go's
// travelPlayer), as a group gate lands its members. The question's words are
// ours.

// heroCallQuestion is what a call asks, the caster's name first.
const heroCallQuestion = "%s calls you to their side. Accept?"

// askHeroCall is the World's hero-call hook: the member's Yes/No box.
func askHeroCall(c entity.HeroCall) {
	if c.Sess == nil || c.P == nil {
		return
	}
	SendQuestPopupMulti(c.Sess, c.Box, fmt.Sprintf(heroCallQuestion, c.Caster), []string{"Yes", "No"})
	log.Printf("Call: %s's %q asks %s (box %#x)", c.Caster, c.Spell, c.P.Name, c.Box)
}

// answerHeroCall takes a 0x0035 to a call's box: the World's answer, and
// its line to the member when it has one.
func (h *InteractHandler) answerHeroCall(sess *session.Session, box uint32, option byte) {
	if h.gameWorld == nil {
		return
	}
	if line := h.gameWorld.AnswerHeroCall(sess, box, option, time.Now()); line != "" {
		sess.Queue().AddWithSequence(buildClientMessage(line))
	}
}
