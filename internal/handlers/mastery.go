package handlers

import (
	"fmt"
	"log"
	"strconv"

	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/session"
)

// SetMasteryStore gives #grantcm where to keep the points it gives.
// Without one they last until the logout's save.
func (h *ChatHandler) SetMasteryStore(m masteryStore) {
	h.mastery = m
}

// SetClassMastery gives #cm and #cmreset the masteries' handler.
func (h *ChatHandler) SetClassMastery(cm *ClassMasteryHandler) {
	h.cm = cm
}

// giveMastery gives the session's character up to amount mastery points more,
// unspent, never past entity.MasteryCap spent and unspent together (#grantcm,
// #template): kept at once and told to the client as a gain is. It reports
// what it gave, the points now, whether the character is in the world, and
// a failure to keep them.
func (h *ChatHandler) giveMastery(sess *session.Session, amount int) (int, entity.Mastery, bool, error) {
	give, m, ok := h.gameWorld.GrantMastery(sess, amount)
	if !ok || give == 0 {
		return give, m, ok, nil
	}
	entity.SendMasteryPoints(sess, m)
	if p := h.gameWorld.SessionPlayer(sess); p != nil && h.mastery != nil {
		if err := h.mastery.SaveMastery(p.CharServerID, MasteryRow(m)); err != nil {
			log.Printf("GM: %d mastery points for '%s' not kept: %v", give, p.Name, err)
			return give, m, true, err
		}
	}
	return give, m, true, nil
}

// grantMasteryCommand is #grantcm <amount> (F3 20261025-165747): that many
// mastery points more for the GM's own character, unspent, never past
// entity.MasteryCap spent and unspent together (with 50 unspent and 50
// spent, #grantcm 1500 gives 1400 and #grantcm 100 gives 100). Kept at
// once, and told to the client as a gain is (0x1402 sub-op 3: CLIENT says
// "You have earned N mastery points.").
func (h *ChatHandler) grantMasteryCommand(sess *session.Session, args []string) {
	if h.gameWorld == nil {
		h.gmReply(sess, "Grant CM: no world")
		return
	}
	if len(args) < 1 {
		if p := h.gameWorld.SessionPlayer(sess); p != nil {
			if m, ok := h.gameWorld.MasteryOf(p.ServerID); ok {
				h.gmReply(sess, "Mastery points: "+entity.MasteryLine(m))
			}
		}
		h.gmReply(sess, fmt.Sprintf("Usage: #grantcm <amount> - up to %d in all, spent and unspent", entity.MasteryCap))
		return
	}
	amount, err := strconv.Atoi(args[0])
	if err != nil || amount < 1 {
		h.gmReply(sess, "Usage: #grantcm <amount> - a number of points, 1 or more")
		return
	}
	give, m, ok, err := h.giveMastery(sess, amount)
	if !ok {
		h.gmReply(sess, "Grant CM: you are not in the world")
		return
	}
	p := h.gameWorld.SessionPlayer(sess)
	if err != nil {
		h.gmReply(sess, fmt.Sprintf("Granted %d mastery points, but not kept: %v", give, err))
		return
	}
	switch {
	case give == amount:
		h.gmReply(sess, fmt.Sprintf("Granted %d mastery points: %s", give, entity.MasteryLine(m)))
	case give > 0:
		h.gmReply(sess, fmt.Sprintf("Granted %d of %d mastery points, the most below %d: %s", give, amount, entity.MasteryCap, entity.MasteryLine(m)))
	default:
		h.gmReply(sess, fmt.Sprintf("No mastery points granted: already %d, the most: %s", entity.MasteryCap, entity.MasteryLine(m)))
	}
	if p != nil {
		log.Printf("GM: %s granted %d of %d mastery points (%s)", p.Name, give, amount, entity.MasteryLine(m))
	}
}
