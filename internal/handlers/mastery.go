package handlers

import (
	"fmt"
	"log"
	"strconv"
	"strings"

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

// grantMasteryCommand is #grantcm <amount> [name] (F3 20261025-165747): that
// many mastery points more, unspent, never past entity.MasteryCap spent and
// unspent together (with 50 unspent and 50 spent, #grantcm 1500 gives 1400 and
// #grantcm 100 gives 100), for the player named (online), else the GM's target
// player, else the GM itself (the owner, 2026-11-21: "make /grantcm targetable
// by GMs so GMs can target players to grant them CMs"). Kept at once, and told
// to that player's client as a gain is (0x1402 sub-op 3: CLIENT says "You have
// earned N mastery points."), another player told in a line who granted them.
func (h *ChatHandler) grantMasteryCommand(sess *session.Session, args []string) {
	if h.gameWorld == nil {
		h.gmReply(sess, "Grant CM: no world")
		return
	}
	gm := h.gameWorld.SessionPlayer(sess)
	if gm == nil {
		h.gmReply(sess, "Grant CM: you are not in the world")
		return
	}
	if len(args) < 1 {
		if m, ok := h.gameWorld.MasteryOf(gm.ServerID); ok {
			h.gmReply(sess, "Mastery points: "+entity.MasteryLine(m))
		}
		h.gmReply(sess, fmt.Sprintf("Usage: #grantcm <amount> [name] - up to %d in all, spent and unspent; to the named player, else your target, else you", entity.MasteryCap))
		return
	}
	amount, err := strconv.Atoi(args[0])
	if err != nil || amount < 1 {
		h.gmReply(sess, "Usage: #grantcm <amount> [name] - a number of points, 1 or more")
		return
	}
	var t *entity.Player
	var ts *session.Session
	var ok bool
	if name := strings.TrimSpace(strings.Join(args[1:], " ")); name != "" {
		t, ts, ok = h.gmNamed(sess, name, "Grant CM")
	} else {
		t, ts, ok = h.gmPlayerTarget(sess, gm, gm.TargetObjectID, "Grant CM")
	}
	if !ok {
		return
	}
	give, m, ok, err := h.giveMastery(ts, amount)
	if !ok {
		h.gmReply(sess, "Grant CM: "+t.Name+" is not in the world")
		return
	}
	who, to := "", "" // the GM's own, said as before
	if t != gm {
		who, to = t.Name+" ", " to "+t.Name
	}
	if err != nil {
		h.gmReply(sess, fmt.Sprintf("Granted %s%d mastery points, but not kept: %v", who, give, err))
		return
	}
	switch {
	case give == amount:
		h.gmReply(sess, fmt.Sprintf("Granted %s%d mastery points: %s", who, give, entity.MasteryLine(m)))
	case give > 0:
		h.gmReply(sess, fmt.Sprintf("Granted %s%d of %d mastery points, the most below %d: %s", who, give, amount, entity.MasteryCap, entity.MasteryLine(m)))
	default:
		h.gmReply(sess, fmt.Sprintf("No mastery points granted%s: already %d, the most: %s", to, entity.MasteryCap, entity.MasteryLine(m)))
	}
	if t != gm && give > 0 {
		h.gmReply(ts, fmt.Sprintf("%s granted you %d mastery points.", gm.Name, give))
	}
	log.Printf("GM: %s granted %s %d of %d mastery points (%s)", gm.Name, t.Name, give, amount, entity.MasteryLine(m))
}
