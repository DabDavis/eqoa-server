package udp

import (
	"log"
	"time"

	"github.com/eqoa/server/internal/session"
)

// saveRez keeps the player's chance of a rez over the logout, what is left of
// its time (entity/resurrect.go), beside the debt savePlayerPosition keeps:
// as saveEffects, an old session's Player is not saved (the new login kept
// it).
func (l *Listener) saveRez(sess *session.Session) {
	player := l.gameWorld.SessionPlayer(sess)
	if player == nil || !l.gameWorld.IsLivePlayer(player) {
		return
	}
	row, ok := l.gameWorld.SavedRez(player.ServerID, time.Now())
	if !ok {
		return
	}
	if err := l.charStore.SaveRez(player.CharServerID, row); err != nil {
		log.Printf("saveRez: '%s': %v", player.Name, err)
	} else if row.Debt > 0 {
		log.Printf("saveRez: '%s' may still be rezzed for a share of %d, %v left", player.Name, row.Debt,
			time.Duration(row.LeftMS)*time.Millisecond)
	}
}
