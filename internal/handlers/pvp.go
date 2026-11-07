package handlers

import (
	"log"
	"time"

	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/session"
)

// pvpCommand is /pvp from any client (entity/pvp.go has the rules): the flag turned over
// when it may be, and, when it was, its new face told to everyone targeting that player.
//
// CLIENT reads a target's face only from a 0x0010, and asks for one only when its target
// changes. Msg0010 takes any reply whose token matches the one it holds, so a reply nobody
// asked for sets the face of the target that client has now: the same reply a re-target
// would get (playerTargetInfo), under its own token. A client that changed its target as this
// went out has the reply to its own request right behind it, which sets it right.
func pvpCommand(gw *entity.World, sess *session.Session, now time.Time) {
	p := gw.PvPCommand(sess, now)
	if p == nil {
		return
	}
	for _, v := range gw.TargetersOf(p) {
		info := playerTargetInfo(gw, v.Viewer, p, v.ObjectID, v.Token)
		info.conColor = calcConColor(v.Viewer.Level, p.Level)
		sendTargetInfo(v.Sess, &info)
		log.Printf("PvP: %s told %s's face: %d", v.Viewer.Name, p.Name, info.faceColor)
	}
}
