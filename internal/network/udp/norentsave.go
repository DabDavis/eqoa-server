package udp

import (
	"log"
	"time"

	"github.com/eqoa/server/internal/session"
)

// saveLogout keeps when the player left the world (table charlogout), for
// its No Rent items: a login more than handlers.NoRentAway after it takes
// them (handlers/norent.go). Saved at a session's end and at a shutdown; as
// saveEffects, an old session's Player is not saved (the character is in the
// world still, taken over by its new login).
func (l *Listener) saveLogout(sess *session.Session, at time.Time) {
	player := l.gameWorld.SessionPlayer(sess)
	if player == nil || !l.gameWorld.IsLivePlayer(player) {
		return
	}
	if err := l.charStore.SaveLogout(player.CharServerID, at); err != nil {
		log.Printf("saveLogout: '%s': %v", player.Name, err)
	}
}
