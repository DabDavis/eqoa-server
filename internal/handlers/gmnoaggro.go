package handlers

import (
	"log"

	"github.com/eqoa/server/internal/session"
)

// gmnoaggro.go — a GM's Safe Mode, the TEST menu's (gmchannel.go) or #aggro off
// (aggrocmd.go): no creature attacks the GM's character on sight
// (entity/aggro.go). The owner, 2026-11-16: "also keep gm safemode on across
// restarts". Kept in table chargmnoaggro, it outlives a logout and every
// restart, as a player's /testinvis does (safemode.go), until Safe Mode or
// #aggro on turns it off. It is given back at the GM's world entry
// (SendGMChannel, so an account no longer a GM's is given nothing), and a
// restart that keeps the players carries it besides (entity/keepgm.go), on
// before the resumed client reports; neither tells the GM anything.

// gmNoAggroStore is where the mode is kept (database.SQLiteDB).
type gmNoAggroStore interface {
	GMNoAggro(serverID uint32) (bool, error)
	SetGMNoAggro(serverID uint32, on bool) error
}

// SetGMNoAggroStore sets where a GM's Safe Mode is kept.
func (h *ChatHandler) SetGMNoAggroStore(s gmNoAggroStore) { h.noAggro = s }

// setGMNoAggro turns the GM's Safe Mode on or off, kept first: false (the GM
// told why) when it could not be kept or the GM is not in the world.
func (h *ChatHandler) setGMNoAggro(sess *session.Session, on bool) bool {
	p := h.gameWorld.SessionPlayer(sess)
	if p == nil {
		return false
	}
	if h.noAggro != nil {
		if err := h.noAggro.SetGMNoAggro(p.CharServerID, on); err != nil {
			log.Printf("GM Safe Mode: '%s' not kept: %v", p.Name, err)
			h.gmReply(sess, "Safe Mode could not be changed just now: try again.")
			return false
		}
	}
	return h.gameWorld.SetAggroIgnored(sess, on)
}

// restoreGMNoAggro gives a GM entering the world (SendGMChannel, once its
// player is known a GM's) the Safe Mode its character was left in.
func (h *ChatHandler) restoreGMNoAggro(sess *session.Session) {
	if h.noAggro == nil || h.gameWorld == nil {
		return
	}
	p := h.gameWorld.SessionPlayer(sess)
	if p == nil {
		return
	}
	on, err := h.noAggro.GMNoAggro(p.CharServerID)
	if err != nil {
		log.Printf("GM channel: '%s' Safe Mode: %v", p.Name, err)
		return
	}
	if on && h.gameWorld.SetAggroIgnored(sess, true) {
		log.Printf("GM channel: '%s' is in Safe Mode (#aggro off)", p.Name)
	}
}
