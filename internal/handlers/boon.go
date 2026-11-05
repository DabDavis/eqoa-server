package handlers

import (
	"time"

	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/session"
)

// boon.go — a player entering the world with no boon on is told, once a
// session, where the towns' boon is (entity/boon.go, signs.go; the user,
// 2026-11-05: "say Boon will be available at all City Announcements when you
// send restart notice": a running server's notice is the one it started with,
// so the word goes to each player as it comes back in).

// boonNoticeAfter is how long after a world entry the line goes out: the
// world loaded, a few seconds after the line on reporting.
const boonNoticeAfter = 20 * time.Second

// noticeBoon tells the player where the boon is, boonNoticeAfter after its
// world entry, unless the boards give none, it has one on by then, or it has
// gone (or been told this session).
func (h *InteractHandler) noticeBoon(sess *session.Session, _ *entity.Player) {
	time.AfterFunc(boonNoticeAfter, func() { h.tellBoon(sess) })
}

func (h *InteractHandler) tellBoon(sess *session.Session) {
	if h.gameWorld == nil || !sess.CharacterInWorld() || sess.PendingTermination() {
		return
	}
	if _, replaced := sess.ReplacedFor(); replaced {
		return
	}
	line, ok := h.gameWorld.BoonNotice(sess)
	if !ok || !sess.TakeBoonNotice() {
		return
	}
	SendClientMessage(sess, line)
}
